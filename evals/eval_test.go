package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dorkitude/webctl/internal/config"
	"github.com/dorkitude/webctl/internal/jev"
	"github.com/dorkitude/webctl/internal/provider"
)

type fakeProvider struct {
	name    string
	results []provider.SearchResult
	err     error
	gotNum  int
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Search(_ context.Context, _ string, num int) ([]provider.SearchResult, error) {
	f.gotNum = num
	return f.results, f.err
}
func (f *fakeProvider) Validate(context.Context) error { return nil }

// fakeJev scores results by URL and answers theme questions by theme text.
type fakeJev struct {
	scores     map[string]float64
	themeProbs map[string]float64 // theme text → P(yes); missing → no answer
	auditYes   map[string]bool    // URL → flagged by the source-quality audit
	qualifyErr error
	systemErr  error

	gotQualify  jev.QualifyOptions
	gotState    coverageState
	systemCalls int
}

func (f *fakeJev) Qualify(_ context.Context, _ jev.Ask, results []provider.SearchResult, opts jev.QualifyOptions) ([]jev.Qualified, jev.Usage, error) {
	f.gotQualify = opts
	if f.qualifyErr != nil {
		return nil, jev.Usage{}, f.qualifyErr
	}
	out := make([]jev.Qualified, len(results))
	for i, r := range results {
		out[i].Result = r
		v := f.scores[r.URL]
		if opts.Noul != "" {
			out[i].Noul = &jev.NoulAnswer{Probability: v}
		} else {
			out[i].Score = &jev.ScoreAnswer{Score: v, Confidence: 0.9, Legend: map[string]string{"0": "", "1": "", "2": "", "3": ""}}
		}
	}
	return out, jev.Usage{InputTokens: 10, OutputTokens: 1}, nil
}

func (f *fakeJev) SystemOne(_ context.Context, req *jev.SystemOneRequest) (*jev.SystemOneResponse, error) {
	f.systemCalls++
	if f.systemErr != nil {
		return nil, f.systemErr
	}
	// Round-trip the state through JSON the way the real API would see it.
	raw, _ := json.Marshal(req.State)
	resp := &jev.SystemOneResponse{Answers: map[string]jev.Answer{}, Usage: jev.Usage{InputTokens: 50, OutputTokens: 5}}
	var audit auditState
	if json.Unmarshal(raw, &audit) == nil && len(audit.Results) > 0 && audit.Results[0].ID == AuditKey(0) {
		for _, item := range audit.Results {
			p := 0.1
			if f.auditYes[item.URL] {
				p = 0.9
			}
			resp.Answers[item.ID] = jev.Answer{Type: jev.TypeNoul, Noul: &p}
		}
		return resp, nil
	}
	_ = json.Unmarshal(raw, &f.gotState)
	for _, th := range f.gotState.Themes {
		p, ok := f.themeProbs[th.Theme]
		if !ok {
			continue
		}
		pp := p
		resp.Answers[th.ID] = jev.Answer{Type: jev.TypeNoul, Noul: &pp}
	}
	return resp, nil
}

var (
	paper = provider.SearchResult{Title: "SAE paper", URL: "https://arxiv.org/abs/1", Snippet: "sparse autoencoders"}
	blog  = provider.SearchResult{Title: "blog", URL: "https://blog.example/x", Snippet: "hot takes"}
	wiki  = provider.SearchResult{Title: "circuits", URL: "https://transformer-circuits.pub/a", Snippet: "attention heads"}
)

func newRunner(t *testing.T) (*Runner, *fakeProvider, *fakeJev) {
	t.Helper()
	fp := &fakeProvider{name: "exa", results: []provider.SearchResult{blog, paper, wiki}}
	fj := &fakeJev{
		scores:     map[string]float64{paper.URL: 2.8, wiki.URL: 2.1, blog.URL: 0.4},
		themeProbs: map[string]float64{"sparse autoencoders": 0.93, "transformer circuits": 0.81},
	}
	r := &Runner{
		NewProvider: func(name string) (provider.Provider, error) {
			if name != "" {
				fp.name = name
			}
			return fp, nil
		},
		Jev:               fj,
		CoverageThreshold: DefaultCoverageThreshold,
	}
	return r, fp, fj
}

func baseCase() Case {
	return Case{
		Name:           "sae",
		Query:          "sparse autoencoders interpretability",
		Num:            20,
		ExpectedThemes: []string{"sparse autoencoders", "transformer circuits"},
		MinResults:     1,
		MaxResults:     5,
	}
}

func TestRunPasses(t *testing.T) {
	r, fp, fj := newRunner(t)
	rep := r.Run(context.Background(), baseCase())
	if rep.Error != "" || !rep.Passed {
		t.Fatalf("report = %+v", rep)
	}
	if fp.gotNum != 20 || rep.Provider != "exa" || rep.TotalResults != 3 || rep.KeptResults != 2 {
		t.Errorf("report = %+v", rep)
	}
	if rep.Kept[0].URL != paper.URL || rep.Kept[1].URL != wiki.URL {
		t.Errorf("kept should be sorted by value: %+v", rep.Kept)
	}
	if fj.systemCalls != 1 {
		t.Errorf("coverage should be one batch call, got %d", fj.systemCalls)
	}
	if len(fj.gotState.Results) != 2 || fj.gotState.Results[0].URL != paper.URL {
		t.Errorf("coverage state should contain only kept results: %+v", fj.gotState.Results)
	}
	if fj.gotState.Themes[1].ID != ThemeKey(1) || fj.gotState.Themes[1].Theme != "transformer circuits" {
		t.Errorf("themes in state = %+v", fj.gotState.Themes)
	}
	if len(rep.Themes) != 2 || !rep.Themes[0].Covered || rep.Themes[0].Probability != 0.93 {
		t.Errorf("themes = %+v", rep.Themes)
	}
	// mean of |p-0.5|*2 over 0.93 and 0.81 = (0.86 + 0.62) / 2 = 0.74
	if rep.Confidence < 0.739 || rep.Confidence > 0.741 {
		t.Errorf("confidence = %v", rep.Confidence)
	}
	if rep.Usage.InputTokens != 60 || rep.Duration <= 0 {
		t.Errorf("usage/duration = %+v %v", rep.Usage, rep.Duration)
	}
}

func TestRunThemeNotCovered(t *testing.T) {
	r, _, fj := newRunner(t)
	fj.themeProbs["transformer circuits"] = 0.2
	rep := r.Run(context.Background(), baseCase())
	if rep.Passed || len(rep.Failures) != 1 || !strings.Contains(rep.Failures[0], `"transformer circuits" not covered (P(yes)=0.20)`) {
		t.Errorf("report = %+v", rep)
	}

	// A per-case threshold can rescue it.
	c := baseCase()
	th := 0.1
	c.CoverageThreshold = &th
	if rep := r.Run(context.Background(), c); !rep.Passed {
		t.Errorf("per-case threshold ignored: %+v", rep)
	}
}

func TestRunThemeUnanswered(t *testing.T) {
	r, _, fj := newRunner(t)
	delete(fj.themeProbs, "transformer circuits")
	rep := r.Run(context.Background(), baseCase())
	if rep.Passed || !strings.Contains(strings.Join(rep.Failures, ";"), "not judged: no answer returned") {
		t.Errorf("report = %+v", rep)
	}
	if rep.Themes[1].Error == "" || rep.Themes[1].Confidence != 0 {
		t.Errorf("theme = %+v", rep.Themes[1])
	}
}

func TestRunCountBounds(t *testing.T) {
	r, _, _ := newRunner(t)
	c := baseCase()
	c.MinResults = 3
	rep := r.Run(context.Background(), c)
	if rep.Passed || !strings.Contains(rep.Failures[0], "too few results: 2 kept, need ≥ 3") {
		t.Errorf("report = %+v", rep)
	}
	c = baseCase()
	c.MaxResults = 1
	rep = r.Run(context.Background(), c)
	if rep.Passed || !strings.Contains(rep.Failures[0], "too many results: 2 kept, want ≤ 1") {
		t.Errorf("report = %+v", rep)
	}
}

func TestRunMinScoreAndModes(t *testing.T) {
	r, _, fj := newRunner(t)
	c := baseCase()
	min := 8.5
	c.MinScore = &min
	c.Rubric = []string{"a", "b", "c", "d"}
	c.Batch = true
	rep := r.Run(context.Background(), c)
	if rep.KeptResults != 1 || rep.Kept[0].URL != paper.URL {
		t.Errorf("min_score 8.5 should keep only the paper: %+v", rep.Kept)
	}
	if !fj.gotQualify.Batch || len(fj.gotQualify.Rubric) != 4 {
		t.Errorf("qualify opts = %+v", fj.gotQualify)
	}

	// Runner-level batch override.
	r.Batch = true
	r.Run(context.Background(), baseCase())
	if !fj.gotQualify.Batch {
		t.Error("runner Batch should force batch mode")
	}

	// Noul mode defaults to P(yes) ≥ 0.5.
	fj.scores = map[string]float64{paper.URL: 0.9, wiki.URL: 0.4, blog.URL: 0.1}
	c = baseCase()
	c.Noul = "Is it a paper?"
	rep = r.Run(context.Background(), c)
	if fj.gotQualify.Noul != "Is it a paper?" || rep.KeptResults != 1 {
		t.Errorf("noul run = %+v opts=%+v", rep, fj.gotQualify)
	}
}

func TestRunNoThemesUsesRelevanceConfidence(t *testing.T) {
	r, _, fj := newRunner(t)
	c := baseCase()
	c.ExpectedThemes = nil
	rep := r.Run(context.Background(), c)
	if !rep.Passed || fj.systemCalls != 0 {
		t.Errorf("no-theme case should pass without a coverage call: %+v calls=%d", rep, fj.systemCalls)
	}
	if rep.Confidence != 0.9 {
		t.Errorf("confidence = %v, want mean relevance confidence 0.9", rep.Confidence)
	}
}

func TestRunNoKeptResultsSkipsCoverage(t *testing.T) {
	r, fp, fj := newRunner(t)
	fp.results = nil
	c := baseCase()
	c.MinResults = 0
	rep := r.Run(context.Background(), c)
	if fj.systemCalls != 0 {
		t.Error("coverage must not call Jev with zero kept results")
	}
	if rep.Passed || len(rep.Themes) != 2 || rep.Themes[0].Covered {
		t.Errorf("all themes should be uncovered: %+v", rep)
	}
}

func TestRunProviderOverrideAndErrors(t *testing.T) {
	r, fp, fj := newRunner(t)
	r.Provider = "sonar"
	c := baseCase()
	c.Provider = "parallel"
	rep := r.Run(context.Background(), c)
	if rep.Provider != "sonar" || fp.name != "sonar" {
		t.Errorf("runner provider should override case provider: %+v", rep)
	}

	r.NewProvider = func(string) (provider.Provider, error) { return nil, errors.New("no key") }
	if rep := r.Run(context.Background(), baseCase()); rep.Error != "no key" || rep.Passed {
		t.Errorf("report = %+v", rep)
	}

	r, fp, fj = newRunner(t)
	fp.err = errors.New("boom")
	if rep := r.Run(context.Background(), baseCase()); !strings.Contains(rep.Error, "search: boom") {
		t.Errorf("report = %+v", rep)
	}
	fp.err = nil
	fj.qualifyErr = errors.New("jev down")
	if rep := r.Run(context.Background(), baseCase()); !strings.Contains(rep.Error, "qualify: jev down") {
		t.Errorf("report = %+v", rep)
	}
	fj.qualifyErr = nil
	fj.systemErr = errors.New("jev batch down")
	if rep := r.Run(context.Background(), baseCase()); !strings.Contains(rep.Error, "theme coverage: jev batch down") {
		t.Errorf("report = %+v", rep)
	}

	if rep := (&Runner{}).Run(context.Background(), baseCase()); rep.Error == "" {
		t.Error("runner without deps should report an error")
	}
}

func TestRunAllAndSummary(t *testing.T) {
	r, _, fj := newRunner(t)
	failing := baseCase()
	failing.Name = "fails"
	failing.MinResults = 10
	broken := baseCase()
	broken.Name = "broken"
	broken.Provider = "x"
	origNew := r.NewProvider
	r.NewProvider = func(name string) (provider.Provider, error) {
		if name == "x" {
			return nil, errors.New("nope")
		}
		return origNew(name)
	}
	_ = fj

	var seen []string
	reports := r.RunAll(context.Background(), []Case{baseCase(), failing, broken}, func(rep *Report) { seen = append(seen, rep.Case) })
	if strings.Join(seen, ",") != "sae,fails,broken" {
		t.Errorf("progress order = %v", seen)
	}
	s := Summarize(reports)
	if s != (Summary{Total: 3, Passed: 1, Failed: 1, Errors: 1}) {
		t.Errorf("summary = %+v", s)
	}

	var buf bytes.Buffer
	for _, rep := range reports {
		WriteReport(&buf, rep, true)
	}
	WriteSummary(&buf, s)
	out := buf.String()
	for _, want := range []string{
		"✓ PASS  sae", "themes 2/2", "✓ sparse autoencoders", "P(yes)=0.93",
		"✗ FAIL  fails", "✗ too few results: 2 kept, need ≥ 10",
		"! ERROR broken", "nope",
		"[1] 9.33 keep  https://arxiv.org/abs/1", "jev usage:",
		"1/3 passed, 1 failed, 1 errored",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// Cancelled context short-circuits remaining cases.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reports = r.RunAll(ctx, []Case{baseCase()}, nil)
	if len(reports) != 1 || reports[0].Error == "" {
		t.Errorf("cancelled run = %+v", reports)
	}
}

func TestParseCase(t *testing.T) {
	c, err := ParseCase("dir/my-case.yaml", []byte("query: q\nexpected_themes: [a, b]\nmin_score: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "my-case" || c.MinScore == nil || *c.MinScore != 2 || len(c.ExpectedThemes) != 2 {
		t.Errorf("case = %+v", c)
	}
	if c.threshold() != 2 {
		t.Errorf("threshold = %v", c.threshold())
	}
	if (&Case{}).threshold() != config.DefaultMinScore || (&Case{Noul: "x"}).threshold() != 0.5 {
		t.Error("default thresholds wrong")
	}

	bad := map[string]string{
		"missing query":   "expected_themes: [a]\n",
		"unknown field":   "query: q\nbogus: 1\n",
		"min > max":       "query: q\nmin_results: 5\nmax_results: 2\n",
		"one rubric item": "query: q\nrubric: [only]\n",
		"rubric and noul": "query: q\nrubric: [a, b]\nnoul: x\n",
		"bad threshold":   "query: q\ncoverage_threshold: 1.5\n",
		"empty theme":     "query: q\nexpected_themes: ['', a]\n",
		"negative num":    "query: q\nnum: -1\n",
		"not yaml":        "query: [\n",
	}
	for name, content := range bad {
		if _, err := ParseCase(name+".yaml", []byte(content)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestLoadCasesFSAndFilter(t *testing.T) {
	fsys := fstest.MapFS{
		"cases/b.yaml":    {Data: []byte("query: b\n")},
		"cases/a.yml":     {Data: []byte("name: alpha\nquery: a\n")},
		"cases/README.md": {Data: []byte("ignored")},
	}
	cases, err := LoadCasesFS(fsys, "cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != "alpha" || cases[1].Name != "b" {
		t.Errorf("cases = %+v", cases)
	}

	got, err := Filter(cases, []string{"b"})
	if err != nil || len(got) != 1 || got[0].Name != "b" {
		t.Errorf("Filter = %+v, %v", got, err)
	}
	if _, err := Filter(cases, []string{"zzz"}); err == nil || !strings.Contains(err.Error(), `unknown eval case "zzz"`) {
		t.Errorf("err = %v", err)
	}
	if all, _ := Filter(cases, nil); len(all) != 2 {
		t.Error("empty filter should return everything")
	}

	dup := fstest.MapFS{
		"x.yaml": {Data: []byte("name: same\nquery: a\n")},
		"y.yaml": {Data: []byte("name: same\nquery: b\n")},
	}
	if _, err := LoadCasesFS(dup, "."); err == nil || !strings.Contains(err.Error(), "duplicate case name") {
		t.Errorf("err = %v", err)
	}
	if _, err := LoadCasesFS(fstest.MapFS{"n.txt": {Data: []byte("x")}}, "."); err == nil || !strings.Contains(err.Error(), "no eval cases") {
		t.Errorf("err = %v", err)
	}
}

func TestLoadCasesDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.yaml"), []byte("query: one\nexpected_themes: [t]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases, err := LoadCasesDir(dir)
	if err != nil || len(cases) != 1 || cases[0].Name != "one" {
		t.Errorf("cases = %+v, %v", cases, err)
	}
	if _, err := LoadCasesDir(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing dir should error")
	}
}

func TestEmbeddedCasesAreValid(t *testing.T) {
	cases, err := EmbeddedCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 3 {
		t.Errorf("expected several embedded cases, got %d", len(cases))
	}
	for _, c := range cases {
		if len(c.ExpectedThemes) == 0 || c.MinResults == 0 {
			t.Errorf("%s: embedded cases should declare themes and min_results", c.Name)
		}
	}
}

func TestHostMatchesAndForJudge(t *testing.T) {
	if !hostMatches("https://www.reddit.com/r/x", []string{"reddit.com"}) || !hostMatches("https://old.reddit.com/r/x", []string{"reddit.com"}) {
		t.Error("subdomains should match")
	}
	if hostMatches("https://notreddit.com/", []string{"reddit.com"}) || hostMatches("://bad", []string{"reddit.com"}) {
		t.Error("unrelated hosts must not match")
	}
	long := strings.Repeat("x", judgeResultChars*2)
	var in []provider.SearchResult
	for i := 0; i < 40; i++ {
		in = append(in, provider.SearchResult{Title: "t", URL: "https://u", Snippet: long, Content: "c"})
	}
	out := forJudge(in)
	if len(out) == 0 || len(out) >= 40 || len([]rune(out[0].Snippet)) > judgeResultChars+1 || out[0].Content != "" {
		t.Errorf("forJudge kept %d results, first snippet %d chars", len(out), len(out[0].Snippet))
	}
}

func TestRunAuditFlagsAndStages(t *testing.T) {
	r, _, fj := newRunner(t)
	r.Audit = true
	r.Modes = AllModes
	fj.auditYes = map[string]bool{blog.URL: true}
	c := baseCase()
	c.JunkDomains = []string{"blog.example"}
	c.ExpectedDomains = []string{"arxiv.org"}
	rep := r.Run(context.Background(), c)
	if rep.Error != "" || rep.AuditError != "" || !rep.Passed {
		t.Fatalf("report = %+v", rep)
	}
	if len(rep.Stages) != 2 { // scrape is skipped unless the case asks for it
		t.Fatalf("stages = %+v", rep.Stages)
	}
	raw, filt := rep.Stages[0], rep.Stages[1]
	if raw.Mode != ModeNoFilter || raw.Results != 3 || raw.Junk != 1 || raw.Flagged != 1 || raw.ExpectedHits != 1 {
		t.Errorf("nofilter = %+v", raw)
	}
	if filt.Mode != ModeFilter || filt.Results != 2 || filt.Junk != 0 || filt.Flagged != 0 || filt.ExpectedHits != 1 || !filt.Passed {
		t.Errorf("filter = %+v", filt)
	}

	// Keeping junk fails the filter stage; dropping every expected-domain hit does too.
	fj.scores[blog.URL] = 2.9
	rep = r.Run(context.Background(), c)
	if rep.Passed || !strings.Contains(strings.Join(rep.Failures, ";"), "junk") {
		t.Errorf("junk kept should fail: %+v", rep.Failures)
	}
	fj.scores[blog.URL] = 0.4
	fj.scores[paper.URL] = 0.2
	rep = r.Run(context.Background(), c)
	if rep.Passed || !strings.Contains(strings.Join(rep.Failures, ";"), "dropped every result from arxiv.org") {
		t.Errorf("dropping the expected domain should fail: %+v", rep.Failures)
	}
}
