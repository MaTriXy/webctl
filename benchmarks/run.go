package benchmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Run is a saved benchmark run: the matrix of cases × arms with every
// result and grade, written after each cell so a crash loses nothing.
type Run struct {
	Started    time.Time `json:"started"`
	Finished   time.Time `json:"finished,omitempty"`
	WebctlVer  string    `json:"webctl_version"`
	JudgeModel string    `json:"judge_model"`
	Arms       []Arm     `json:"arms"`
	Cases      []Case    `json:"cases"`
	// Results is keyed by case name then arm name.
	Results map[string]map[string]*Result `json:"results"`
	// LogDir holds raw harness and judge output.
	LogDir string `json:"log_dir"`
}

// Options control a run.
type Options struct {
	Arms  []Arm
	Cases []Case
	// Concurrency is how many agent runs proceed at once, overall. Each
	// harness is further limited to Concurrency/2 (at least 1) so one
	// harness's rate limit does not stall the others.
	Concurrency int
	// Timeout bounds one agent run.
	Timeout time.Duration
	// Judge grades answers; nil skips grading.
	Judge *Judge
	// OutPath is the run file, rewritten after every cell.
	OutPath string
	// LogDir receives raw output; default is OutPath minus ".json".
	LogDir string
	// Resume, if set, is an existing run whose completed cells are kept.
	Resume *Run
	// RerunArms lists arm names whose cells are discarded from Resume, so
	// they run again (and the case is judged again) against a changed tool.
	RerunArms []string
	// Progress receives one line per event; nil is silent.
	Progress io.Writer
	// WebctlVersion is recorded in the run file.
	WebctlVersion string
}

// Execute runs the matrix. Cells already present in Resume with an answer
// are skipped; failed cells run again. Judging happens per case once every arm
// has answered, so grades are comparable within a case.
func Execute(ctx context.Context, opts Options) (*Run, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 3
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 6 * time.Minute
	}
	if opts.LogDir == "" {
		opts.LogDir = trimJSON(opts.OutPath)
	}
	if err := os.MkdirAll(opts.LogDir, 0o755); err != nil {
		return nil, err
	}
	run := &Run{
		Started:   time.Now(),
		WebctlVer: opts.WebctlVersion,
		Arms:      opts.Arms,
		Cases:     opts.Cases,
		Results:   map[string]map[string]*Result{},
		LogDir:    opts.LogDir,
	}
	if opts.Judge != nil {
		run.JudgeModel = opts.Judge.Model
		if run.JudgeModel == "" {
			run.JudgeModel = DefaultJudgeModel
		}
	}
	if opts.Resume != nil {
		run.Started = opts.Resume.Started
		rerun := map[string]bool{}
		for _, a := range opts.RerunArms {
			rerun[a] = true
		}
		for cn, arms := range opts.Resume.Results {
			run.Results[cn] = map[string]*Result{}
			for an, r := range arms {
				if rerun[an] {
					continue
				}
				if len(rerun) > 0 && r != nil {
					// Other arms keep their answers but are judged again
					// alongside the fresh ones, so grades stay comparable.
					cp := *r
					cp.Judge = nil
					r = &cp
				}
				run.Results[cn][an] = r
			}
		}
	}
	for _, c := range opts.Cases {
		if run.Results[c.Name] == nil {
			run.Results[c.Name] = map[string]*Result{}
		}
	}

	var mu sync.Mutex
	save := func() error {
		mu.Lock()
		defer mu.Unlock()
		return writeRun(opts.OutPath, run)
	}
	progress := func(format string, a ...any) {
		if opts.Progress != nil {
			fmt.Fprintf(opts.Progress, format+"\n", a...)
		}
	}

	runners := map[string]Runner{}
	for _, a := range opts.Arms {
		if _, ok := runners[a.Harness]; ok {
			continue
		}
		r, err := NewRunner(a.Harness)
		if err != nil {
			return nil, err
		}
		runners[a.Harness] = r
	}

	perHarness := opts.Concurrency / 2
	if perHarness < 1 {
		perHarness = 1
	}
	global := make(chan struct{}, opts.Concurrency)
	harnessSem := map[string]chan struct{}{}
	for h := range runners {
		harnessSem[h] = make(chan struct{}, perHarness)
	}

	// One goroutine per case runs its arms, then judges the case.
	var wg sync.WaitGroup
	caseSem := make(chan struct{}, opts.Concurrency) // cases in flight
	for _, c := range opts.Cases {
		wg.Add(1)
		caseSem <- struct{}{}
		go func(c Case) {
			defer wg.Done()
			defer func() { <-caseSem }()
			var awg sync.WaitGroup
			for _, arm := range opts.Arms {
				mu.Lock()
				existing := run.Results[c.Name][arm.Name]
				mu.Unlock()
				if existing != nil && existing.Answer != "" {
					continue // finished; failed cells run again
				}
				awg.Add(1)
				go func(arm Arm) {
					defer awg.Done()
					global <- struct{}{}
					harnessSem[arm.Harness] <- struct{}{}
					defer func() { <-harnessSem[arm.Harness]; <-global }()
					res := runCell(ctx, runners[arm.Harness], c, arm, opts)
					mu.Lock()
					run.Results[c.Name][arm.Name] = &res
					mu.Unlock()
					if res.Error != "" {
						progress("  %-24s %-22s FAILED %s", c.Name, arm.Name, res.Error)
					} else {
						progress("  %-24s %-22s %5.1fs  %7d tok  %d webctl, %d search", c.Name, arm.Name,
							float64(res.WallMs)/1000, res.Tokens.Total(), res.WebctlCalls, res.SearchCalls)
					}
					_ = save()
				}(arm)
			}
			awg.Wait()
			if opts.Judge == nil {
				return
			}
			mu.Lock()
			var results []Result
			var idx []string
			needs := false
			for _, arm := range opts.Arms {
				r := run.Results[c.Name][arm.Name]
				if r == nil {
					continue
				}
				results = append(results, *r)
				idx = append(idx, arm.Name)
				if r.Judge == nil && r.Answer != "" {
					needs = true
				}
			}
			mu.Unlock()
			if !needs {
				return
			}
			jctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			grades, err := opts.Judge.Grade(jctx, c, results)
			cancel()
			if err != nil {
				progress("  %-24s judge FAILED: %v", c.Name, err)
				return
			}
			mu.Lock()
			for i, an := range idx {
				g := grades[i]
				run.Results[c.Name][an].Judge = &g
			}
			mu.Unlock()
			line := fmt.Sprintf("  %-24s judged:", c.Name)
			for i, an := range idx {
				line += fmt.Sprintf(" %s=%.0f", an, grades[i].Score)
			}
			progress("%s", line)
			_ = save()
		}(c)
	}
	wg.Wait()
	run.Finished = time.Now()
	return run, save()
}

func runCell(ctx context.Context, r Runner, c Case, arm Arm, opts Options) Result {
	cctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	logDir := filepath.Join(opts.LogDir, c.Name)
	_ = os.MkdirAll(logDir, 0o755)
	workDir, err := os.MkdirTemp("", "webctl-bench-")
	if err != nil {
		return Result{Case: c.Name, Arm: arm.Name, Error: err.Error()}
	}
	defer os.RemoveAll(workDir)
	res, err := r.Run(cctx, arm, BuildPrompt(c, arm.Mode), workDir, logDir)
	res.Case, res.Arm = c.Name, arm.Name
	if err != nil {
		res.Error = err.Error()
		if cctx.Err() != nil {
			res.Error = "timeout after " + opts.Timeout.String()
		}
	}
	return res
}

func writeRun(path string, run *Run) error {
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadRun reads a saved run.
func LoadRun(path string) (*Run, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var run Run
	if err := json.Unmarshal(b, &run); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &run, nil
}

// Latest returns the newest *.json run in dir.
func Latest(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no runs in %s", dir)
	}
	sort.Strings(names)
	return filepath.Join(dir, names[len(names)-1]), nil
}

func trimJSON(p string) string {
	if filepath.Ext(p) == ".json" {
		return p[:len(p)-len(".json")]
	}
	return p + ".logs"
}
