// Package evals runs end-to-end search-quality evaluations: each case issues a
// real search, qualifies the results with Jev, filters them, and then asks Jev
// (in one batch request) whether the surviving results cover a set of expected
// themes. It reports pass/fail per case together with Jev's confidence.
package evals

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/dorkitude/smart_search/internal/jev"
	"github.com/dorkitude/smart_search/internal/prompts"
	"github.com/dorkitude/smart_search/internal/provider"
)

// CasesFS holds the eval cases shipped with the binary (evals/cases/*.yaml).
//
//go:embed cases/*.yaml
var CasesFS embed.FS

// DefaultCoverageThreshold is the P(yes) at which a theme counts as covered.
const DefaultCoverageThreshold = 0.5

// Case is one eval scenario, loaded from YAML.
type Case struct {
	// Name identifies the case; defaults to the file name without extension.
	Name string `yaml:"name" json:"name"`
	// Query is the search to run.
	Query string `yaml:"query" json:"query"`
	// Provider overrides the runner's provider for this case (optional).
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty"`
	// Num is how many results to request (default 10).
	Num int `yaml:"num,omitempty" json:"num,omitempty"`
	// MinScore is the relevance cutoff (default 1.0; in noul mode, P(yes) ≥ 0.5).
	MinScore *float64 `yaml:"min_score,omitempty" json:"min_score,omitempty"`
	// Rubric overrides the default 0–3 relevance criteria.
	Rubric []string `yaml:"rubric,omitempty" json:"rubric,omitempty"`
	// Noul, when set, qualifies results with this yes/no question instead of scoring.
	Noul string `yaml:"noul,omitempty" json:"noul,omitempty"`
	// Batch forces Jev batch mode for qualification (the runner may also force it).
	Batch bool `yaml:"batch,omitempty" json:"batch,omitempty"`

	// ExpectedThemes must all be covered by the kept results for the case to pass.
	ExpectedThemes []string `yaml:"expected_themes" json:"expected_themes"`
	// MinResults / MaxResults bound the number of kept results (0 = unbounded).
	MinResults int `yaml:"min_results,omitempty" json:"min_results,omitempty"`
	MaxResults int `yaml:"max_results,omitempty" json:"max_results,omitempty"`
	// CoverageThreshold overrides DefaultCoverageThreshold for this case.
	CoverageThreshold *float64 `yaml:"coverage_threshold,omitempty" json:"coverage_threshold,omitempty"`
}

// Validate checks that the case is runnable.
func (c *Case) Validate() error {
	if strings.TrimSpace(c.Query) == "" {
		return fmt.Errorf("case %q: query is required", c.Name)
	}
	if c.Num < 0 {
		return fmt.Errorf("case %q: num must be ≥ 0", c.Name)
	}
	if c.MinResults < 0 || c.MaxResults < 0 {
		return fmt.Errorf("case %q: min_results/max_results must be ≥ 0", c.Name)
	}
	if c.MaxResults > 0 && c.MinResults > c.MaxResults {
		return fmt.Errorf("case %q: min_results (%d) exceeds max_results (%d)", c.Name, c.MinResults, c.MaxResults)
	}
	if len(c.Rubric) == 1 {
		return fmt.Errorf("case %q: rubric needs at least 2 criteria", c.Name)
	}
	if len(c.Rubric) > 0 && c.Noul != "" {
		return fmt.Errorf("case %q: rubric and noul are mutually exclusive", c.Name)
	}
	if c.CoverageThreshold != nil && (*c.CoverageThreshold < 0 || *c.CoverageThreshold > 1) {
		return fmt.Errorf("case %q: coverage_threshold must be in [0, 1]", c.Name)
	}
	for i, th := range c.ExpectedThemes {
		if strings.TrimSpace(th) == "" {
			return fmt.Errorf("case %q: expected_themes[%d] is empty", c.Name, i)
		}
	}
	return nil
}

// threshold returns the relevance cutoff for qualification.
func (c *Case) threshold() float64 {
	if c.MinScore != nil {
		return *c.MinScore
	}
	if c.Noul != "" {
		return 0.5
	}
	return 1.0
}

// ParseCase decodes one YAML case. name is used when the YAML omits "name".
func ParseCase(name string, data []byte) (Case, error) {
	var c Case
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Case{}, fmt.Errorf("parse %s: %w", name, err)
	}
	if c.Name == "" {
		c.Name = strings.TrimSuffix(strings.TrimSuffix(filepath.Base(name), ".yaml"), ".yml")
	}
	if err := c.Validate(); err != nil {
		return Case{}, err
	}
	return c, nil
}

// LoadCasesFS reads every *.yaml / *.yml case under dir in fsys, sorted by name.
func LoadCasesFS(fsys fs.FS, dir string) ([]Case, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read eval cases: %w", err)
	}
	var cases []Case
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		p := e.Name()
		if dir != "." && dir != "" {
			p = dir + "/" + e.Name()
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		c, err := ParseCase(p, data)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[c.Name]; dup {
			return nil, fmt.Errorf("duplicate case name %q in %s and %s", c.Name, prev, p)
		}
		seen[c.Name] = p
		cases = append(cases, c)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no eval cases (*.yaml) found in %s", dir)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Name < cases[j].Name })
	return cases, nil
}

// LoadCasesDir reads cases from a directory on disk.
func LoadCasesDir(dir string) ([]Case, error) {
	return LoadCasesFS(os.DirFS(dir), ".")
}

// EmbeddedCases returns the cases compiled into the binary.
func EmbeddedCases() ([]Case, error) {
	return LoadCasesFS(CasesFS, "cases")
}

// Filter keeps only cases whose names are in names (all cases if names is empty).
func Filter(cases []Case, names []string) ([]Case, error) {
	if len(names) == 0 {
		return cases, nil
	}
	byName := map[string]Case{}
	for _, c := range cases {
		byName[c.Name] = c
	}
	var out []Case
	for _, n := range names {
		c, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown eval case %q", n)
		}
		out = append(out, c)
	}
	return out, nil
}

// JevClient is the subset of *jev.Client the runner needs.
type JevClient interface {
	Qualify(ctx context.Context, query string, results []provider.SearchResult, opts jev.QualifyOptions) ([]jev.Qualified, jev.Usage, error)
	SystemOne(ctx context.Context, req *jev.SystemOneRequest) (*jev.SystemOneResponse, error)
}

// Runner executes eval cases.
type Runner struct {
	// NewProvider constructs the named search provider (name may be "" for the default).
	NewProvider func(name string) (provider.Provider, error)
	// Jev performs qualification and theme-coverage judgments.
	Jev JevClient

	// Provider, when set, overrides Case.Provider for every case.
	Provider string
	// Batch, when true, forces Jev batch mode for qualification.
	Batch bool
	// CoverageThreshold is the default P(yes) for a theme to count as covered.
	CoverageThreshold float64
}

// ThemeResult is Jev's judgment of whether one expected theme is covered.
type ThemeResult struct {
	Theme       string  `json:"theme"`
	Probability float64 `json:"probability"` // P(yes)
	Confidence  float64 `json:"confidence"`
	Covered     bool    `json:"covered"`
	Error       string  `json:"error,omitempty"`
}

// KeptResult is a result that survived filtering, with its relevance value.
type KeptResult struct {
	provider.SearchResult
	Value      float64 `json:"value"`
	Confidence float64 `json:"confidence"`
}

// Report is the outcome of one case.
type Report struct {
	Case     string        `json:"case"`
	Query    string        `json:"query"`
	Provider string        `json:"provider"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration_ns"`

	TotalResults int           `json:"total_results"`
	KeptResults  int           `json:"kept_results"`
	Kept         []KeptResult  `json:"kept"`
	Themes       []ThemeResult `json:"themes"`
	// Confidence is the mean of Jev's confidence across theme judgments (or,
	// with no themes, across relevance judgments of kept results).
	Confidence float64 `json:"confidence"`
	// Failures lists human-readable reasons the case failed.
	Failures []string `json:"failures,omitempty"`
	// Error is set when the case could not be run at all.
	Error string    `json:"error,omitempty"`
	Usage jev.Usage `json:"usage"`
}

// Run executes one case. Infrastructure errors (provider/Jev failures) are
// returned in Report.Error rather than as a Go error, so a suite keeps going.
func (r *Runner) Run(ctx context.Context, c Case) *Report {
	start := time.Now()
	rep := &Report{Case: c.Name, Query: c.Query}
	defer func() { rep.Duration = time.Since(start) }()

	fail := func(err error) *Report {
		rep.Error = err.Error()
		rep.Passed = false
		return rep
	}
	if r.Jev == nil || r.NewProvider == nil {
		return fail(errors.New("runner is missing Jev client or provider factory"))
	}

	providerName := c.Provider
	if r.Provider != "" {
		providerName = r.Provider
	}
	p, err := r.NewProvider(providerName)
	if err != nil {
		return fail(err)
	}
	rep.Provider = p.Name()

	num := c.Num
	if num <= 0 {
		num = 10
	}
	results, err := p.Search(ctx, c.Query, num)
	if err != nil {
		return fail(fmt.Errorf("search: %w", err))
	}
	rep.TotalResults = len(results)

	var kept []jev.Qualified
	if len(results) > 0 {
		qualified, usage, err := r.Jev.Qualify(ctx, c.Query, results, jev.QualifyOptions{
			Rubric: c.Rubric,
			Noul:   c.Noul,
			Batch:  c.Batch || r.Batch,
		})
		rep.Usage.Add(usage)
		if err != nil {
			return fail(fmt.Errorf("qualify: %w", err))
		}
		min := c.threshold()
		for _, q := range qualified {
			if q.Err == nil && (q.Score != nil || q.Noul != nil) && q.Value() >= min {
				kept = append(kept, q)
			}
		}
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].Value() > kept[j].Value() })
	}
	rep.KeptResults = len(kept)
	rep.Kept = make([]KeptResult, 0, len(kept))
	for _, q := range kept {
		rep.Kept = append(rep.Kept, KeptResult{SearchResult: q.Result, Value: q.Value(), Confidence: q.Confidence()})
	}

	// Count bounds.
	if c.MinResults > 0 && len(kept) < c.MinResults {
		rep.Failures = append(rep.Failures, fmt.Sprintf("too few results: %d kept, need ≥ %d", len(kept), c.MinResults))
	}
	if c.MaxResults > 0 && len(kept) > c.MaxResults {
		rep.Failures = append(rep.Failures, fmt.Sprintf("too many results: %d kept, want ≤ %d", len(kept), c.MaxResults))
	}

	// Theme coverage.
	if len(c.ExpectedThemes) > 0 {
		threshold := r.CoverageThreshold
		if c.CoverageThreshold != nil {
			threshold = *c.CoverageThreshold
		}
		if threshold <= 0 {
			threshold = DefaultCoverageThreshold
		}
		themes, usage, err := r.coverage(ctx, c.Query, c.ExpectedThemes, rep.Kept, threshold)
		rep.Usage.Add(usage)
		if err != nil {
			return fail(fmt.Errorf("theme coverage: %w", err))
		}
		rep.Themes = themes
		var sum float64
		for _, th := range themes {
			sum += th.Confidence
			if !th.Covered {
				reason := fmt.Sprintf("theme %q not covered (P(yes)=%.2f)", th.Theme, th.Probability)
				if th.Error != "" {
					reason = fmt.Sprintf("theme %q not judged: %s", th.Theme, th.Error)
				}
				rep.Failures = append(rep.Failures, reason)
			}
		}
		rep.Confidence = sum / float64(len(themes))
	} else if len(rep.Kept) > 0 {
		var sum float64
		for _, k := range rep.Kept {
			sum += k.Confidence
		}
		rep.Confidence = sum / float64(len(rep.Kept))
	}

	rep.Passed = len(rep.Failures) == 0
	return rep
}

// RunAll runs every case in order, stopping early only if ctx is cancelled.
func (r *Runner) RunAll(ctx context.Context, cases []Case, progress func(*Report)) []*Report {
	reports := make([]*Report, 0, len(cases))
	for _, c := range cases {
		if ctx.Err() != nil {
			rep := &Report{Case: c.Name, Query: c.Query, Error: ctx.Err().Error()}
			reports = append(reports, rep)
			if progress != nil {
				progress(rep)
			}
			continue
		}
		rep := r.Run(ctx, c)
		reports = append(reports, rep)
		if progress != nil {
			progress(rep)
		}
	}
	return reports
}

// ThemeKey is the Jev question key for the i-th expected theme.
func ThemeKey(i int) string { return "theme_" + strconv.Itoa(i) }

type coverageTheme struct {
	ID    string `json:"id"`
	Theme string `json:"theme"`
}

type coverageState struct {
	Query   string                  `json:"query"`
	Themes  []coverageTheme         `json:"themes"`
	Results []provider.SearchResult `json:"results"`
}

// coverage asks Jev, in a single batch request, whether the kept results
// cover each theme. With no kept results every theme is reported uncovered
// without calling Jev.
func (r *Runner) coverage(ctx context.Context, query string, themes []string, kept []KeptResult, threshold float64) ([]ThemeResult, jev.Usage, error) {
	out := make([]ThemeResult, len(themes))
	for i, th := range themes {
		out[i] = ThemeResult{Theme: th, Confidence: 1}
	}
	if len(kept) == 0 {
		return out, jev.Usage{}, nil
	}

	p, err := prompts.Load(prompts.ThemeCoverage)
	if err != nil {
		return nil, jev.Usage{}, err
	}
	state := coverageState{Query: query, Results: make([]provider.SearchResult, 0, len(kept))}
	for _, k := range kept {
		state.Results = append(state.Results, k.SearchResult)
	}
	questions := make(map[string]jev.Question, len(themes))
	for i, th := range themes {
		id := ThemeKey(i)
		state.Themes = append(state.Themes, coverageTheme{ID: id, Theme: th})
		instructions, err := p.Render(prompts.Data{Query: query, Theme: th, ID: id, Index: i})
		if err != nil {
			return nil, jev.Usage{}, err
		}
		questions[id] = jev.NoulQuestion(instructions)
	}
	resp, err := r.Jev.SystemOne(ctx, &jev.SystemOneRequest{State: state, Questions: questions})
	if err != nil {
		return nil, jev.Usage{}, err
	}
	for i := range themes {
		raw, ok := resp.Answers[ThemeKey(i)]
		if !ok {
			out[i].Error = "no answer returned"
			out[i].Confidence = 0
			continue
		}
		ans, err := raw.AsNoul()
		if err != nil {
			out[i].Error = err.Error()
			out[i].Confidence = 0
			continue
		}
		out[i].Probability = ans.Probability
		out[i].Confidence = ans.Confidence()
		out[i].Covered = ans.Probability >= threshold
	}
	return out, resp.Usage, nil
}

// Summary aggregates pass/fail counts.
type Summary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Errors int `json:"errors"`
}

// Summarize tallies reports.
func Summarize(reports []*Report) Summary {
	s := Summary{Total: len(reports)}
	for _, r := range reports {
		switch {
		case r.Error != "":
			s.Errors++
		case r.Passed:
			s.Passed++
		default:
			s.Failed++
		}
	}
	return s
}

// WriteReport prints a human-readable report line (plus detail lines).
func WriteReport(w io.Writer, rep *Report, verbose bool) {
	status := "✓ PASS"
	if rep.Error != "" {
		status = "! ERROR"
	} else if !rep.Passed {
		status = "✗ FAIL"
	}
	fmt.Fprintf(w, "%-7s %-28s", status, rep.Case)
	if rep.Error != "" {
		fmt.Fprintf(w, "  %s\n", rep.Error)
		return
	}
	fmt.Fprintf(w, "  %3d → %-3d kept", rep.TotalResults, rep.KeptResults)
	if len(rep.Themes) > 0 {
		covered := 0
		for _, th := range rep.Themes {
			if th.Covered {
				covered++
			}
		}
		fmt.Fprintf(w, "   themes %d/%d", covered, len(rep.Themes))
	}
	fmt.Fprintf(w, "   confidence %.2f   %s\n", rep.Confidence, rep.Duration.Round(100*time.Millisecond))

	for _, th := range rep.Themes {
		mark := "✓"
		if !th.Covered {
			mark = "✗"
		}
		if th.Error != "" {
			fmt.Fprintf(w, "    %s %-40s %s\n", mark, th.Theme, th.Error)
			continue
		}
		fmt.Fprintf(w, "    %s %-40s P(yes)=%.2f\n", mark, th.Theme, th.Probability)
	}
	for _, f := range rep.Failures {
		if strings.HasPrefix(f, "theme ") {
			continue // already shown above
		}
		fmt.Fprintf(w, "    ✗ %s\n", f)
	}
	if verbose {
		for i, k := range rep.Kept {
			fmt.Fprintf(w, "    [%d] %.2f  %s\n", i+1, k.Value, k.URL)
		}
		fmt.Fprintf(w, "    jev usage: %d input / %d output tokens\n", rep.Usage.InputTokens, rep.Usage.OutputTokens)
	}
}

// WriteSummary prints the final tally.
func WriteSummary(w io.Writer, s Summary) {
	fmt.Fprintf(w, "\n%d/%d passed", s.Passed, s.Total)
	if s.Failed > 0 {
		fmt.Fprintf(w, ", %d failed", s.Failed)
	}
	if s.Errors > 0 {
		fmt.Fprintf(w, ", %d errored", s.Errors)
	}
	fmt.Fprintln(w)
}
