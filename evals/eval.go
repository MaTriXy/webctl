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
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/dorkitude/webctl/internal/config"
	"github.com/dorkitude/webctl/internal/jev"
	"github.com/dorkitude/webctl/internal/prompts"
	"github.com/dorkitude/webctl/internal/provider"
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
	// Tags label the case for reporting (reddit, seo, ambiguous, ...).
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// Scrape runs the scrape-to-chunks stage on the kept results.
	Scrape bool `yaml:"scrape,omitempty" json:"scrape,omitempty"`
	// ExpectedDomains are hosts where the best answers live. If the raw
	// results include one and filtering drops them all, the filter stage fails.
	ExpectedDomains []string `yaml:"expected_domains,omitempty" json:"expected_domains,omitempty"`
	// JunkDomains are hand-labelled hosts that are noise for this query. A
	// kept junk result fails the filter stage.
	JunkDomains []string `yaml:"junk_domains,omitempty" json:"junk_domains,omitempty"`

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
	return config.DefaultMinScore
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

	// Modes selects the stages to run; nil means ModeFilter only.
	Modes []Mode
	// Scraper fetches pages for ModeScrape; nil uses scrape.Fetcher.
	Scraper Scraper
	// Parallel bounds concurrent cases in RunAll (≤0 means 1).
	Parallel int
	// Version is recorded on every report.
	Version string
	// Audit runs the source-quality audit on raw results (one extra Jev
	// batch call per case) so stages can count flagged pages.
	Audit bool
	// ScrapeAll runs the scrape stage on every case, not only those marked.
	ScrapeAll bool
	// ReuseSearches, when set, supplies each case's raw provider results
	// (by case name) from an earlier run, so this run judges identical
	// inputs. Cases not present are searched live.
	ReuseSearches map[string][]provider.SearchResult
}

// Mode is one evaluation stage.
type Mode string

const (
	// ModeNoFilter measures the raw provider results as they would reach a
	// context window without Jev.
	ModeNoFilter Mode = "nofilter"
	// ModeFilter is the standard pipeline: qualify, threshold, keep.
	ModeFilter Mode = "filter"
	// ModeScrape fetches kept pages and keeps only Jev-approved chunks.
	ModeScrape Mode = "scrape"
)

// AllModes lists every stage in run order.
var AllModes = []Mode{ModeNoFilter, ModeFilter, ModeScrape}

// ParseModes parses a comma-separated mode list.
func ParseModes(s string) ([]Mode, error) {
	var out []Mode
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch m := Mode(part); m {
		case ModeNoFilter, ModeFilter, ModeScrape:
			out = append(out, m)
		default:
			return nil, fmt.Errorf("unknown mode %q (expected nofilter, filter, scrape)", part)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no modes given")
	}
	return out, nil
}

func (r *Runner) modes() []Mode {
	if len(r.Modes) == 0 {
		return []Mode{ModeFilter}
	}
	return r.Modes
}

func (r *Runner) hasMode(m Mode) bool {
	for _, have := range r.modes() {
		if have == m {
			return true
		}
	}
	return false
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

// Stage is the measured outcome of one mode on one case. Results, Chars,
// Junk, and ExpectedHits describe what would reach the context window.
type Stage struct {
	Mode         Mode `json:"mode"`
	Results      int  `json:"results"`
	Chars        int  `json:"chars"`
	Junk         int  `json:"junk"`
	ExpectedHits int  `json:"expected_hits"`
	// Flagged counts delivered results the source-quality audit judged to
	// be SEO, affiliate, or content-farm pages.
	Flagged int `json:"flagged"`
	// Folded counts results collapsed into a duplicate\'s best copy.
	Folded   int           `json:"folded,omitempty"`
	Themes   []ThemeResult `json:"themes,omitempty"`
	Covered  int           `json:"covered"`
	Passed   bool          `json:"passed"`
	Failures []string      `json:"failures,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration_ns"`
	Usage    jev.Usage     `json:"usage"`
	URLs     []string      `json:"urls,omitempty"`
	// Scrape-only: pages fetched, chunk counts, and chars before filtering.
	PagesOK     int `json:"pages_ok,omitempty"`
	PagesFailed int `json:"pages_failed,omitempty"`
	ChunksTotal int `json:"chunks_total,omitempty"`
	ChunksKept  int `json:"chunks_kept,omitempty"`
	CharsRaw    int `json:"chars_raw,omitempty"`
	// DroppedCovered counts expected themes that the discarded chunks still
	// cover: signal the chunk filter threw away. Lower is better.
	DroppedCovered int `json:"dropped_covered,omitempty"`
	// PageFailures lists "host: reason" for pages that could not be fetched.
	PageFailures []string `json:"page_failures,omitempty"`
}

// Report is the outcome of one case. The top-level fields describe the
// filter stage (the product's default behavior); Stages holds every mode
// that ran.
type Report struct {
	Case     string        `json:"case"`
	Query    string        `json:"query"`
	Tags     []string      `json:"tags,omitempty"`
	Version  string        `json:"version,omitempty"`
	Provider string        `json:"provider"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration_ns"`
	// SearchDuration is the provider call alone.
	SearchDuration time.Duration `json:"search_duration_ns"`
	// AuditError is set when the source-quality audit failed; Flagged
	// counts are then zero.
	AuditError string `json:"audit_error,omitempty"`
	// Raw is the provider's (deduplicated) result list, kept so a later
	// run can judge the same inputs with --reuse-searches.
	Raw    []provider.SearchResult `json:"raw_results,omitempty"`
	Stages []Stage                 `json:"stages,omitempty"`

	TotalResults int          `json:"total_results"`
	KeptResults  int          `json:"kept_results"`
	Kept         []KeptResult `json:"kept"`
	// Judged is every result with Jev's relevance value, best first,
	// including the ones the threshold dropped.
	Judged []KeptResult  `json:"judged,omitempty"`
	Themes []ThemeResult `json:"themes"`
	// Confidence is the mean of Jev's confidence across theme judgments (or,
	// with no themes, across relevance judgments of kept results).
	Confidence float64 `json:"confidence"`
	// Failures lists human-readable reasons the case failed.
	Failures []string `json:"failures,omitempty"`
	// Error is set when the case could not be run at all.
	Error string    `json:"error,omitempty"`
	Usage jev.Usage `json:"usage"`
}

// Run executes one case: one provider search, then every configured stage
// on those results. Infrastructure errors in the filter stage (provider or
// Jev failures) are returned in Report.Error rather than as a Go error, so
// a suite keeps going; other stages record their own Error.
func (r *Runner) Run(ctx context.Context, c Case) *Report {
	start := time.Now()
	rep := &Report{Case: c.Name, Query: c.Query, Tags: c.Tags, Version: r.Version}
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
	num := c.Num
	if num <= 0 {
		num = 10
	}
	results, reused := r.ReuseSearches[c.Name]
	if reused {
		rep.Provider = "reused"
	} else {
		p, err := r.NewProvider(providerName)
		if err != nil {
			return fail(err)
		}
		searchStart := time.Now()
		results, err = p.Search(ctx, c.Query, num)
		rep.SearchDuration = time.Since(searchStart)
		if err != nil {
			return fail(fmt.Errorf("search: %w", err))
		}
		rep.Provider = p.Name()
	}
	results = provider.Dedupe(results)
	rep.Raw = results
	rep.TotalResults = len(results)

	// One audit of the raw results labels low-value pages; every stage
	// counts how many of them it delivered.
	flagged := map[string]bool{}
	if r.Audit {
		var usage jev.Usage
		var err error
		if flagged, usage, err = r.audit(ctx, c.Query, results); err != nil {
			rep.AuditError = err.Error()
		}
		rep.Usage.Add(usage)
	}

	var kept []KeptResult
	filtered := false
	for _, mode := range r.modes() {
		switch mode {
		case ModeNoFilter:
			rep.Stages = append(rep.Stages, r.stageNoFilter(ctx, c, results, flagged))
		case ModeFilter:
			st, k, judged, err := r.stageFilter(ctx, c, results, flagged)
			rep.Stages = append(rep.Stages, st)
			if err != nil {
				return fail(err)
			}
			kept, filtered = k, true
			rep.KeptResults = len(kept)
			rep.Kept = kept
			rep.Judged = judged
			rep.Themes = st.Themes
			rep.Failures = st.Failures
			rep.Confidence = confidence(st.Themes, kept)
			rep.Usage.Add(st.Usage)
		case ModeScrape:
			if !c.Scrape && !r.ScrapeAll {
				continue
			}
			if !filtered {
				st, k, judged, err := r.stageFilter(ctx, c, results, flagged)
				if err != nil {
					return fail(err)
				}
				kept, filtered = k, true
				rep.Judged = judged
				rep.Usage.Add(st.Usage)
			}
			st := r.stageScrape(ctx, c, kept, flagged)
			rep.Stages = append(rep.Stages, st)
			rep.Usage.Add(st.Usage)
		}
	}

	rep.Passed = true
	for _, st := range rep.Stages {
		if st.Mode == ModeFilter || !filtered {
			rep.Passed = rep.Passed && st.Passed
		}
	}
	return rep
}

// confidence is the mean of Jev's confidence across theme judgments, or,
// with no themes, across the relevance judgments of kept results.
func confidence(themes []ThemeResult, kept []KeptResult) float64 {
	if len(themes) > 0 {
		var sum float64
		for _, th := range themes {
			sum += th.Confidence
		}
		return sum / float64(len(themes))
	}
	if len(kept) > 0 {
		var sum float64
		for _, k := range kept {
			sum += k.Confidence
		}
		return sum / float64(len(kept))
	}
	return 0
}

// RunAll runs every case, up to Parallel at a time, and returns reports in
// case order. progress is called as each case finishes (from one goroutine
// at a time). Cases not started before ctx is cancelled report the error.
func (r *Runner) RunAll(ctx context.Context, cases []Case, progress func(*Report)) []*Report {
	reports := make([]*Report, len(cases))
	workers := r.Parallel
	if workers <= 0 {
		workers = 1
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	sem := make(chan struct{}, workers)
	for i, c := range cases {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c Case) {
			defer wg.Done()
			defer func() { <-sem }()
			var rep *Report
			if ctx.Err() != nil {
				rep = &Report{Case: c.Name, Query: c.Query, Tags: c.Tags, Version: r.Version, Error: ctx.Err().Error()}
			} else {
				rep = r.Run(ctx, c)
			}
			mu.Lock()
			reports[i] = rep
			if progress != nil {
				progress(rep)
			}
			mu.Unlock()
		}(i, c)
	}
	wg.Wait()
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
func (r *Runner) coverage(ctx context.Context, query string, themes []string, results []provider.SearchResult, threshold float64) ([]ThemeResult, jev.Usage, error) {
	out := make([]ThemeResult, len(themes))
	for i, th := range themes {
		out[i] = ThemeResult{Theme: th, Confidence: 1}
	}
	if len(results) == 0 {
		return out, jev.Usage{}, nil
	}

	p, err := prompts.Load(prompts.ThemeCoverage)
	if err != nil {
		return nil, jev.Usage{}, err
	}
	state := coverageState{Query: query, Results: results}
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
	for _, st := range rep.Stages {
		writeStage(w, st)
	}

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
		for i, k := range rep.Judged {
			mark := "drop"
			if i < len(rep.Kept) {
				mark = "keep"
			}
			fmt.Fprintf(w, "    [%d] %.2f %s  %s\n", i+1, k.Value, mark, k.URL)
		}
		fmt.Fprintf(w, "    jev usage: %d input / %d output tokens\n", rep.Usage.InputTokens, rep.Usage.OutputTokens)
	}
}

// writeStage prints one stage's delivery numbers on a single line.
func writeStage(w io.Writer, st Stage) {
	mark := "✓"
	if st.Error != "" {
		mark = "!"
	} else if !st.Passed {
		mark = "✗"
	}
	fmt.Fprintf(w, "    %s %-8s %2d results  %6d chars", mark, st.Mode, st.Results, st.Chars)
	if len(st.Themes) > 0 {
		fmt.Fprintf(w, "  themes %d/%d", st.Covered, len(st.Themes))
	}
	if st.Junk > 0 {
		fmt.Fprintf(w, "  junk %d", st.Junk)
	}
	if st.Flagged > 0 {
		fmt.Fprintf(w, "  flagged %d", st.Flagged)
	}
	if st.Folded > 0 {
		fmt.Fprintf(w, "  folded %d", st.Folded)
	}
	if st.ExpectedHits > 0 {
		fmt.Fprintf(w, "  expected-domain %d", st.ExpectedHits)
	}
	if st.Mode == ModeScrape {
		fmt.Fprintf(w, "  pages %d ok/%d failed  chunks %d→%d  %d→%d chars  dropped-still-cover %d", st.PagesOK, st.PagesFailed, st.ChunksTotal, st.ChunksKept, st.CharsRaw, st.Chars, st.DroppedCovered)
	}
	fmt.Fprintf(w, "  %s", st.Duration.Round(100*time.Millisecond))
	if st.Error != "" {
		fmt.Fprintf(w, "  %s", st.Error)
	}
	fmt.Fprintln(w)
	for _, f := range st.Failures {
		if st.Mode == ModeFilter && strings.HasPrefix(f, "theme ") {
			continue // shown in the theme lines above
		}
		fmt.Fprintf(w, "        ✗ %s\n", f)
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
