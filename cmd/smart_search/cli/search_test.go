package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dorkitude/smart_search/internal/config"
	"github.com/dorkitude/smart_search/internal/jev"
	"github.com/dorkitude/smart_search/internal/keys"
	"github.com/dorkitude/smart_search/internal/provider"
)

// fakeProvider records the search it was asked to run and returns canned results.
type fakeProvider struct {
	name    string
	results []provider.SearchResult
	err     error

	gotQuery string
	gotNum   int
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Search(_ context.Context, query string, num int) ([]provider.SearchResult, error) {
	f.gotQuery, f.gotNum = query, num
	return f.results, f.err
}
func (f *fakeProvider) Validate(context.Context) error { return nil }

// fakeQualifier returns a score (or noul probability) per URL.
type fakeQualifier struct {
	scores map[string]float64 // URL → score / P(yes)
	errs   map[string]error   // URL → per-result error
	err    error              // whole-request error

	gotQuery string
	gotOpts  jev.QualifyOptions
	calls    int
}

func (f *fakeQualifier) Qualify(_ context.Context, query string, results []provider.SearchResult, opts jev.QualifyOptions) ([]jev.Qualified, jev.Usage, error) {
	f.calls++
	f.gotQuery, f.gotOpts = query, opts
	if f.err != nil {
		return nil, jev.Usage{}, f.err
	}
	out := make([]jev.Qualified, len(results))
	for i, r := range results {
		out[i].Result = r
		if err, ok := f.errs[r.URL]; ok {
			out[i].Err = err
			continue
		}
		val := f.scores[r.URL]
		if opts.Noul != "" {
			out[i].Noul = &jev.NoulAnswer{Probability: val}
			continue
		}
		max := 3
		if len(opts.Rubric) > 0 {
			max = len(opts.Rubric) - 1
		}
		probs := map[string]float64{}
		for lvl := 0; lvl <= max; lvl++ {
			probs[itoa(lvl)] = 0.05
		}
		probs[itoa(int(val+0.5))] = 0.8
		out[i].Score = &jev.ScoreAnswer{Score: val, Confidence: 0.8, Probabilities: probs}
	}
	return out, jev.Usage{InputTokens: 100 * len(results), OutputTokens: len(results)}, nil
}

func itoa(i int) string { return string(rune('0' + i)) }

var (
	paper = provider.SearchResult{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762", Snippet: "We propose the Transformer."}
	blog  = provider.SearchResult{Title: "SEO blog", URL: "https://www.content-farm.example/attention", Snippet: "Top 10 attention tips."}
	wiki  = provider.SearchResult{Title: "Transformer (deep learning)", URL: "https://en.wikipedia.org/wiki/Transformer", Snippet: "A transformer is a deep learning architecture."}
)

// harness wires fakes into the CLI and runs it against a temp config dir.
type harness struct {
	t    *testing.T
	dir  string
	prov *fakeProvider
	qual *fakeQualifier
}

func newHarness(t *testing.T, store keys.Store) *harness {
	t.Helper()
	for _, n := range keys.All {
		t.Setenv(n.EnvVar(), "")
	}
	for _, k := range []string{"PROVIDER", "NUM", "MIN_SCORE", "JEV_BASE_URL", "JEV_MODEL"} {
		t.Setenv(config.EnvPrefix+"_"+k, "")
	}
	dir := t.TempDir()
	if err := store.Save(filepath.Join(dir, "keys.json")); err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t:    t,
		dir:  dir,
		prov: &fakeProvider{name: "exa", results: []provider.SearchResult{blog, paper, wiki}},
		qual: &fakeQualifier{scores: map[string]float64{paper.URL: 2.9, wiki.URL: 1.6, blog.URL: 0.3}},
	}
	origProv, origQual := newProvider, newQualifier
	newProvider = func(name, key string) (provider.Provider, error) {
		if key == "" {
			return nil, errors.New("fake: empty key")
		}
		h.prov.name = name
		return h.prov, nil
	}
	newQualifier = func(cfg *config.Config, key string) qualifier {
		if key == "" {
			t.Error("qualifier constructed with empty key")
		}
		return h.qual
	}
	t.Cleanup(func() { newProvider, newQualifier = origProv, origQual })
	return h
}

// run executes the CLI with args, returning stdout, stderr, and the error.
func (h *harness) run(args ...string) (string, string, error) {
	h.t.Helper()
	v = config.New()
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"--config-dir", h.dir}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func allKeys() keys.Store {
	return keys.Store{ExaAPIKey: "e", ParallelAPIKey: "p", SonarAPIKey: "s", JevAPIKey: "j"}
}

func mustJSON[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
	return v
}

func TestSearchDefaultFiltersAndSorts(t *testing.T) {
	h := newHarness(t, allKeys())
	out, errOut, err := h.run("transformer", "circuits")
	if err != nil {
		t.Fatal(err)
	}
	if h.prov.gotQuery != "transformer circuits" || h.prov.gotNum != config.DefaultNum {
		t.Errorf("provider got %q / %d", h.prov.gotQuery, h.prov.gotNum)
	}
	if h.qual.gotQuery != "transformer circuits" || h.qual.gotOpts.Batch || h.qual.gotOpts.Noul != "" || h.qual.gotOpts.Rubric != nil {
		t.Errorf("qualifier opts = %+v", h.qual.gotOpts)
	}
	// Kept results are sorted by score, highest first; the blog is dropped.
	if !strings.Contains(out, "[1] Attention Is All You Need — arxiv.org") || !strings.Contains(out, "[2] Transformer (deep learning) — en.wikipedia.org") {
		t.Errorf("unexpected ordering:\n%s", out)
	}
	if strings.Contains(out, "SEO blog") {
		t.Errorf("filtered result should be hidden without --verbose:\n%s", out)
	}
	if !strings.Contains(out, "Score: 2.90 / 3  (confidence: 0.80)") {
		t.Errorf("score line missing:\n%s", out)
	}
	if strings.Contains(out, "Probabilities") || strings.Contains(out, "✓ Kept") {
		t.Errorf("verbose-only lines should not appear:\n%s", out)
	}
	if !strings.Contains(errOut, "exa: 3 results → 2 kept (min score 1)") {
		t.Errorf("summary missing from stderr: %q", errOut)
	}
}

func TestSearchVerbose(t *testing.T) {
	h := newHarness(t, allKeys())
	out, errOut, err := h.run("--verbose", "--batch", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !h.qual.gotOpts.Batch {
		t.Error("--batch should set QualifyOptions.Batch")
	}
	for _, want := range []string{
		"[3] SEO blog — content-farm.example",
		"Probabilities: {0: 0.05, 1: 0.05, 2: 0.05, 3: 0.80}",
		"✓ Kept",
		"✗ Filtered (below 1 threshold)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("verbose output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "jev: batch mode, 300 input / 3 output tokens") {
		t.Errorf("verbose usage line missing: %q", errOut)
	}
}

func TestSearchJSON(t *testing.T) {
	h := newHarness(t, allKeys())
	out, errOut, err := h.run("--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	if errOut != "" {
		t.Errorf("json mode should keep stderr quiet, got %q", errOut)
	}
	items := mustJSON[[]outputResult](t, out)
	if len(items) != 2 || items[0].URL != paper.URL || items[1].URL != wiki.URL {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Score == nil || *items[0].Score != 2.9 || items[0].MaxScore == nil || *items[0].MaxScore != 3 || !items[0].Kept {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[0].Yes != nil || items[0].Probability != nil {
		t.Errorf("score mode should omit noul fields: %+v", items[0])
	}
	// jq '.[].url' style access must work: a top-level array of objects.
	generic := mustJSON[[]map[string]any](t, out)
	if generic[0]["url"] != paper.URL {
		t.Errorf("generic[0].url = %v", generic[0]["url"])
	}

	// Verbose JSON includes filtered results, flagged kept=false.
	out, _, err = h.run("--json", "--verbose", "q")
	if err != nil {
		t.Fatal(err)
	}
	items = mustJSON[[]outputResult](t, out)
	if len(items) != 3 || items[2].Kept || items[2].URL != blog.URL {
		t.Errorf("verbose items = %+v", items)
	}
}

func TestSearchURLsOnly(t *testing.T) {
	h := newHarness(t, allKeys())
	out, errOut, err := h.run("--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != paper.URL+"\n"+wiki.URL+"\n" {
		t.Errorf("out = %q", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSearchMinScore(t *testing.T) {
	h := newHarness(t, allKeys())
	out, _, err := h.run("--urls-only", "--min-score", "2", "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != paper.URL+"\n" {
		t.Errorf("min-score 2 should keep only the paper, got %q", out)
	}

	out, _, err = h.run("--urls-only", "-m", "0", "q")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\n") != 3 {
		t.Errorf("min-score 0 should keep everything, got %q", out)
	}

	// config.yaml default is honored when the flag is absent.
	if err := os.WriteFile(filepath.Join(h.dir, "config.yaml"), []byte("min_score: 2.5\nnum: 4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err = h.run("--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != paper.URL+"\n" || h.prov.gotNum != 4 {
		t.Errorf("config.yaml defaults not applied: out=%q num=%d", out, h.prov.gotNum)
	}
}

func TestSearchNum(t *testing.T) {
	h := newHarness(t, allKeys())
	if _, _, err := h.run("--num", "25", "--no-filter", "q"); err != nil {
		t.Fatal(err)
	}
	if h.prov.gotNum != 25 {
		t.Errorf("num = %d, want 25", h.prov.gotNum)
	}
}

func TestSearchNoFilter(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e"}) // no Jev key at all
	out, errOut, err := h.run("--no-filter", "q")
	if err != nil {
		t.Fatal(err)
	}
	if h.qual.calls != 0 {
		t.Error("--no-filter must not call Jev")
	}
	if !strings.Contains(out, "[1] SEO blog") || !strings.Contains(out, "[3] Transformer (deep learning)") {
		t.Errorf("raw results should keep provider order:\n%s", out)
	}
	if strings.Contains(out, "Score:") || errOut != "" {
		t.Errorf("no-filter should not print scores or a summary: out=%q err=%q", out, errOut)
	}

	out, _, err = h.run("--no-filter", "--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	raw := mustJSON[[]provider.SearchResult](t, out)
	if len(raw) != 3 || raw[0] != blog {
		t.Errorf("raw json = %+v", raw)
	}

	out, _, err = h.run("--no-filter", "--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != blog.URL+"\n"+paper.URL+"\n"+wiki.URL+"\n" {
		t.Errorf("urls = %q", out)
	}
}

func TestSearchNoul(t *testing.T) {
	h := newHarness(t, allKeys())
	h.qual.scores = map[string]float64{paper.URL: 0.95, wiki.URL: 0.55, blog.URL: 0.2}
	out, errOut, err := h.run("--noul", "Is this a research paper?", "-v", "q")
	if err != nil {
		t.Fatal(err)
	}
	if h.qual.gotOpts.Noul != "Is this a research paper?" {
		t.Errorf("noul question not passed: %+v", h.qual.gotOpts)
	}
	if !strings.Contains(out, "P(yes): 0.95  (confidence: 0.90)") {
		t.Errorf("noul line missing:\n%s", out)
	}
	if !strings.Contains(errOut, "3 results → 2 kept (P(yes) ≥ 0.50)") {
		t.Errorf("noul default threshold should be 0.5: %q", errOut)
	}

	out, _, err = h.run("--noul", "Paper?", "--min-score", "0.9", "--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	items := mustJSON[[]outputResult](t, out)
	if len(items) != 1 || items[0].Yes == nil || !*items[0].Yes || *items[0].Probability != 0.95 || items[0].Score != nil {
		t.Errorf("noul json = %+v", items)
	}

	if _, _, err := h.run("--noul", "Paper?", "--min-score", "1.5", "q"); err == nil || !strings.Contains(err.Error(), "impossible with --noul") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchRubric(t *testing.T) {
	h := newHarness(t, allKeys())
	h.qual.scores = map[string]float64{paper.URL: 1.9, wiki.URL: 1.0, blog.URL: 0.1}
	out, _, err := h.run("--rubric", " bad, ok ,great,, ", "--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.qual.gotOpts.Rubric, "|") != "bad|ok|great" {
		t.Errorf("rubric = %v", h.qual.gotOpts.Rubric)
	}
	if out != paper.URL+"\n"+wiki.URL+"\n" {
		t.Errorf("out = %q", out)
	}
	if _, _, err := h.run("--rubric", "only-one", "q"); err == nil || !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("err = %v", err)
	}
	if _, _, err := h.run("--rubric", "a,b", "--min-score", "1.5", "q"); err == nil || !strings.Contains(err.Error(), "exceeds the rubric's top score of 1") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchProviderSelection(t *testing.T) {
	// Only Sonar configured → auto-selected even though the default is exa.
	h := newHarness(t, keys.Store{SonarAPIKey: "s", JevAPIKey: "j"})
	if _, _, err := h.run("--urls-only", "q"); err != nil {
		t.Fatal(err)
	}
	if h.prov.name != "sonar" {
		t.Errorf("auto-selected provider = %q, want sonar", h.prov.name)
	}

	// Explicit flag wins.
	h = newHarness(t, allKeys())
	if _, _, err := h.run("-p", "parallel", "--urls-only", "q"); err != nil {
		t.Fatal(err)
	}
	if h.prov.name != "parallel" {
		t.Errorf("provider = %q, want parallel", h.prov.name)
	}

	// Explicit provider without a key is an actionable error.
	h = newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	_, _, err := h.run("--provider", "sonar", "q")
	if err == nil || !strings.Contains(err.Error(), "SONAR_API_KEY") {
		t.Errorf("err = %v", err)
	}
	if h.prov.gotQuery != "" {
		t.Error("should fail before searching")
	}

	// Env var beats keys.json and enables the provider.
	h = newHarness(t, keys.Store{JevAPIKey: "j"})
	t.Setenv("PARALLEL_API_KEY", "from-env")
	if _, _, err := h.run("--urls-only", "q"); err != nil {
		t.Fatal(err)
	}
	if h.prov.name != "parallel" {
		t.Errorf("provider = %q, want parallel from env", h.prov.name)
	}
}

func TestSearchMissingJevKeyFailsBeforeSearch(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e"})
	_, _, err := h.run("q")
	if err == nil || !strings.Contains(err.Error(), "no Jev API key") || !strings.Contains(err.Error(), "--no-filter") {
		t.Errorf("err = %v", err)
	}
	if h.prov.gotQuery != "" {
		t.Error("provider should not be called when the Jev key is missing")
	}
}

func TestSearchFlagConflicts(t *testing.T) {
	h := newHarness(t, allKeys())
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--json", "--urls-only", "q"}, "mutually exclusive"},
		{[]string{"--rubric", "a,b", "--noul", "x?", "q"}, "mutually exclusive"},
		{[]string{"   "}, "query is empty"},
	} {
		_, _, err := h.run(tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestSearchNoArgsShowsHelp(t *testing.T) {
	h := newHarness(t, allKeys())
	out, _, err := h.run()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Usage:") || h.prov.gotQuery != "" {
		t.Errorf("expected help, got:\n%s", out)
	}
}

func TestSearchProviderError(t *testing.T) {
	h := newHarness(t, allKeys())
	h.prov.err = &provider.APIError{Provider: "Exa", Status: 401, Body: "nope"}
	_, _, err := h.run("q")
	if !provider.IsUnauthorized(err) {
		t.Errorf("provider error should propagate, got %v", err)
	}
	if h.qual.calls != 0 {
		t.Error("Jev should not be called after a provider failure")
	}
}

func TestSearchJevError(t *testing.T) {
	h := newHarness(t, allKeys())
	h.qual.err = errors.New("jev exploded")
	_, _, err := h.run("q")
	if err == nil || !strings.Contains(err.Error(), "jev exploded") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchPerResultJevError(t *testing.T) {
	h := newHarness(t, allKeys())
	h.qual.errs = map[string]error{wiki.URL: errors.New("timeout")}
	out, errOut, err := h.run("-v", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "1 kept") || !strings.Contains(errOut, "1 not scored (Jev error)") {
		t.Errorf("summary = %q", errOut)
	}
	if !strings.Contains(out, "! Jev error: timeout") || !strings.Contains(out, "✗ Filtered (not scored)") {
		t.Errorf("verbose output should explain the failure:\n%s", out)
	}
	// Errored results sort last, after filtered-but-scored ones.
	if strings.Index(out, "SEO blog") > strings.Index(out, "Transformer (deep learning)") {
		t.Errorf("errored result should sort after scored results:\n%s", out)
	}

	out, _, err = h.run("--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	if items := mustJSON[[]outputResult](t, out); len(items) != 1 {
		t.Errorf("errored result must not be kept: %+v", items)
	}
}

func TestSearchNoResults(t *testing.T) {
	h := newHarness(t, allKeys())
	h.prov.results = nil
	out, errOut, err := h.run("q")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" || !strings.Contains(errOut, "returned no results") {
		t.Errorf("out=%q err=%q", out, errOut)
	}
	if h.qual.calls != 0 {
		t.Error("Jev should not be called with zero results")
	}
	out, _, err = h.run("--json", "q")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("json with no results = %q, %v", out, err)
	}
	out, _, err = h.run("--json", "--no-filter", "q")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("raw json with no results = %q, %v", out, err)
	}
}

func TestSearchUntitledAndUnparseableURL(t *testing.T) {
	h := newHarness(t, allKeys())
	odd := provider.SearchResult{Title: "  ", URL: "not a url", Snippet: ""}
	h.prov.results = []provider.SearchResult{odd}
	h.qual.scores = map[string]float64{odd.URL: 3}
	out, _, err := h.run("q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[1] (untitled) — ?") {
		t.Errorf("fallback rendering missing:\n%s", out)
	}
}

func TestRank(t *testing.T) {
	score := func(s float64) *jev.ScoreAnswer { return &jev.ScoreAnswer{Score: s} }
	in := []jev.Qualified{
		{Result: provider.SearchResult{URL: "low"}, Score: score(0.5)},
		{Result: provider.SearchResult{URL: "err"}, Err: errors.New("x")},
		{Result: provider.SearchResult{URL: "high"}, Score: score(2.5)},
		{Result: provider.SearchResult{URL: "mid"}, Score: score(1.5)},
		{Result: provider.SearchResult{URL: "none"}}, // no answer, no error
	}
	got := rank(in, 1.0)
	var order []string
	for _, r := range got {
		order = append(order, r.Result.URL)
	}
	if strings.Join(order, ",") != "high,mid,low,err,none" {
		t.Errorf("order = %v", order)
	}
	if !got[0].Kept || !got[1].Kept || got[2].Kept || got[3].Kept || got[4].Kept {
		t.Errorf("kept flags wrong: %+v", got)
	}
	kept, failed := countKept(got)
	if kept != 2 || failed != 1 {
		t.Errorf("kept=%d failed=%d", kept, failed)
	}
}

func TestClipSnippet(t *testing.T) {
	if got := clipSnippet("a  b\n\nc", 10); got != "a b c" {
		t.Errorf("clipSnippet = %q", got)
	}
	if got := clipSnippet("héllo wörld", 5); got != "héllo…" {
		t.Errorf("clipSnippet = %q", got)
	}
}
