package evals

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dorkitude/multi_search_web/internal/provider"
)

// Run is one invocation of the suite: its settings, tally, and every
// report. Runs are saved as JSON files, one per run, in a directory
// outside the repository (test output, not source).
type Run struct {
	ID         string    `json:"id"`
	Version    string    `json:"version"`
	GitSHA     string    `json:"git_sha,omitempty"`
	Provider   string    `json:"provider"`
	Modes      []Mode    `json:"modes"`
	Notes      string    `json:"notes,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Summary    Summary   `json:"summary"`
	Reports    []*Report `json:"reports"`
}

// NewRun stamps a run with an id of the form <version>-<UTC time>.
func NewRun(version, gitSHA, prov string, modes []Mode, notes string) *Run {
	now := time.Now().UTC()
	return &Run{
		ID:        version + "-" + now.Format("20060102T150405Z"),
		Version:   version,
		GitSHA:    gitSHA,
		Provider:  prov,
		Modes:     modes,
		Notes:     notes,
		StartedAt: now,
	}
}

// Path is where the run is saved under dir.
func (r *Run) Path(dir string) string { return filepath.Join(dir, r.ID+".json") }

// Save writes the run as indented JSON, creating dir if needed.
func (r *Run) Save(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	p := r.Path(dir)
	return p, os.WriteFile(p, data, 0o644)
}

// LoadRun reads one run file.
func LoadRun(path string) (*Run, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

// LoadRuns reads every run in dir, oldest first. A missing dir is empty.
func LoadRuns(dir string) ([]*Run, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var runs []*Run
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, err := LoadRun(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	return runs, nil
}

// ResolveRun finds a run by id, by "latest", by "latest:<version>", or by
// file path.
func ResolveRun(dir, ref string) (*Run, error) {
	if strings.HasSuffix(ref, ".json") {
		return LoadRun(ref)
	}
	runs, err := LoadRuns(dir)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("no runs in %s", dir)
	}
	switch {
	case ref == "latest":
		return runs[len(runs)-1], nil
	case strings.HasPrefix(ref, "latest:"):
		v := strings.TrimPrefix(ref, "latest:")
		for i := len(runs) - 1; i >= 0; i-- {
			if runs[i].Version == v {
				return runs[i], nil
			}
		}
		return nil, fmt.Errorf("no runs for version %s in %s", v, dir)
	}
	for _, r := range runs {
		if r.ID == ref {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no run %q in %s (ids look like %s)", ref, dir, runs[len(runs)-1].ID)
}

// Versions lists every version with a run, oldest first.
func Versions(runs []*Run) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range runs {
		if !seen[r.Version] {
			seen[r.Version] = true
			out = append(out, r.Version)
		}
	}
	return out
}

// Latest returns the newest run of version, or nil.
func Latest(runs []*Run, version string) *Run {
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Version == version {
			return runs[i]
		}
	}
	return nil
}

// RawSearches returns each case's raw provider results from a run, for
// re-judging identical inputs.
func (r *Run) RawSearches() map[string][]provider.SearchResult {
	out := map[string][]provider.SearchResult{}
	for _, rep := range r.Reports {
		if len(rep.Raw) > 0 {
			out[rep.Case] = rep.Raw
		}
	}
	return out
}

// ModeSummary aggregates one mode across a run.
type ModeSummary struct {
	Version        string
	Mode           Mode
	Cases          int
	Passed         int
	Results        int
	Chars          int
	Junk           int
	Flagged        int
	ExpectedHits   int
	ThemesTotal    int
	ThemesCovered  int
	PagesOK        int
	PagesFailed    int
	ChunksTotal    int
	ChunksKept     int
	CharsRaw       int
	DroppedCovered int
	DurationMs     int64
	SearchMs       int64
	TokensIn       int
	TokensOut      int
}

// Summarize aggregates a run per mode, in AllModes order.
func (r *Run) Summarize() []ModeSummary {
	byMode := map[Mode]*ModeSummary{}
	for _, rep := range r.Reports {
		for _, st := range rep.Stages {
			s, ok := byMode[st.Mode]
			if !ok {
				s = &ModeSummary{Version: r.Version, Mode: st.Mode}
				byMode[st.Mode] = s
			}
			s.Cases++
			if st.Passed && st.Error == "" && rep.Error == "" {
				s.Passed++
			}
			s.Results += st.Results
			s.Chars += st.Chars
			s.Junk += st.Junk
			s.Flagged += st.Flagged
			s.ExpectedHits += st.ExpectedHits
			s.ThemesTotal += len(st.Themes)
			s.ThemesCovered += st.Covered
			s.PagesOK += st.PagesOK
			s.PagesFailed += st.PagesFailed
			s.ChunksTotal += st.ChunksTotal
			s.ChunksKept += st.ChunksKept
			s.CharsRaw += st.CharsRaw
			s.DroppedCovered += st.DroppedCovered
			s.DurationMs += st.Duration.Milliseconds()
			s.SearchMs += rep.SearchDuration.Milliseconds()
			s.TokensIn += st.Usage.InputTokens
			s.TokensOut += st.Usage.OutputTokens
		}
	}
	var out []ModeSummary
	for _, m := range AllModes {
		if s, ok := byMode[m]; ok {
			out = append(out, *s)
		}
	}
	return out
}

// CaseRow is one case's stage in table form.
type CaseRow struct {
	Case           string
	Tags           string
	Mode           Mode
	Passed         bool
	Results        int
	Chars          int
	Junk           int
	Flagged        int
	ExpectedHits   int
	ThemesTotal    int
	ThemesCovered  int
	PagesOK        int
	PagesFailed    int
	ChunksTotal    int
	ChunksKept     int
	CharsRaw       int
	DroppedCovered int
	DurationMs     int64
	SearchMs       int64
	TokensIn       int
	Error          string
	Failures       string
}

// Rows flattens a run to one row per case and stage, cases in name order,
// stages in AllModes order. A case that could not run yields one error row.
func (r *Run) Rows() []CaseRow {
	reports := append([]*Report(nil), r.Reports...)
	sort.Slice(reports, func(i, j int) bool { return reports[i].Case < reports[j].Case })
	var out []CaseRow
	for _, rep := range reports {
		tags := strings.Join(rep.Tags, ",")
		if len(rep.Stages) == 0 {
			out = append(out, CaseRow{Case: rep.Case, Tags: tags, Error: rep.Error, DurationMs: rep.Duration.Milliseconds(), SearchMs: rep.SearchDuration.Milliseconds()})
			continue
		}
		for _, m := range AllModes {
			for _, st := range rep.Stages {
				if st.Mode != m {
					continue
				}
				errText := st.Error
				if errText == "" {
					errText = rep.Error
				}
				out = append(out, CaseRow{
					Case: rep.Case, Tags: tags, Mode: st.Mode, Passed: st.Passed && errText == "",
					Results: st.Results, Chars: st.Chars, Junk: st.Junk, Flagged: st.Flagged, ExpectedHits: st.ExpectedHits,
					ThemesTotal: len(st.Themes), ThemesCovered: st.Covered, PagesOK: st.PagesOK, PagesFailed: st.PagesFailed,
					ChunksTotal: st.ChunksTotal, ChunksKept: st.ChunksKept, CharsRaw: st.CharsRaw, DroppedCovered: st.DroppedCovered,
					DurationMs: st.Duration.Milliseconds(), SearchMs: rep.SearchDuration.Milliseconds(), TokensIn: st.Usage.InputTokens,
					Error: errText, Failures: strings.Join(st.Failures, "\n"),
				})
			}
		}
	}
	return out
}
