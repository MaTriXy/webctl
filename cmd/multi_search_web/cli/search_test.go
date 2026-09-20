package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dorkitude/multi_search_web/internal/config"
	"github.com/dorkitude/multi_search_web/internal/jev"
	"github.com/dorkitude/multi_search_web/internal/keys"
	"github.com/dorkitude/multi_search_web/internal/provider"
	"github.com/dorkitude/multi_search_web/internal/scrape"
)

// fakeProvider records the search it was asked to run and returns canned results.
type fakeProvider struct {
	name    string
	results []provider.SearchResult
	err     error
	// hang makes Search block until its context is done.
	hang bool

	gotQuery string
	gotNum   int
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Search(ctx context.Context, query string, num int) ([]provider.SearchResult, error) {
	f.gotQuery, f.gotNum = query, num
	if f.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
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

	// Chunk filtering: chunks containing a relevantWord are "yes"; a chunk
	// containing unansweredWord gets no answer; chunkErr fails the request.
	relevantWord   string
	unansweredWord string
	chunkErr       error
	mu             sync.Mutex
	chunkCalls     int
	gotChunks      [][]string
}

func (f *fakeQualifier) FilterChunks(_ context.Context, query string, chunks []string) ([]*jev.NoulAnswer, jev.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunkCalls++
	f.gotChunks = append(f.gotChunks, chunks)
	if f.chunkErr != nil {
		return nil, jev.Usage{}, f.chunkErr
	}
	out := make([]*jev.NoulAnswer, len(chunks))
	for i, c := range chunks {
		lc := strings.ToLower(c)
		if f.unansweredWord != "" && strings.Contains(lc, strings.ToLower(f.unansweredWord)) {
			continue
		}
		p := 0.1
		if f.relevantWord != "" && strings.Contains(lc, strings.ToLower(f.relevantWord)) {
			p = 0.9
		}
		out[i] = &jev.NoulAnswer{Probability: p}
	}
	return out, jev.Usage{}, nil
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
	// provs, when set, supplies a distinct fake per provider name; names not
	// present fall back to prov.
	provs map[string]*fakeProvider
	// built records the provider names constructed, in order.
	built []string
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
	// Fakes answer with three results, which would trigger a top-up on
	// every search; the dedicated test turns it back on.
	chainTopUp = false
	t.Cleanup(func() { chainTopUp = true })
	h := &harness{
		t:    t,
		dir:  dir,
		prov: &fakeProvider{name: "exa", results: []provider.SearchResult{blog, paper, wiki}},
		qual: &fakeQualifier{scores: map[string]float64{paper.URL: 2.9, wiki.URL: 2.4, blog.URL: 0.3}},
	}
	origProv, origQual := newProvider, newQualifier
	newProvider = func(cfg *config.Config, name string) (provider.Provider, error) {
		if _, err := cfg.ProviderKey(name); err != nil {
			return nil, err
		}
		h.built = append(h.built, name)
		if p, ok := h.provs[name]; ok {
			p.name = name
			return p, nil
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
	root.SetArgs(append([]string{"--config-dir", h.dir, "--keys-file", filepath.Join(h.dir, "keys.json")}, args...))
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
	if !strings.Contains(errOut, "exa: 3 results → 2 kept (min score 1.8)") {
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
		"✗ Filtered (below 1.8 threshold)",
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
	out, _, err := h.run("--urls-only", "--min-score", "2.5", "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != paper.URL+"\n" {
		t.Errorf("min-score 2.5 should keep only the paper, got %q", out)
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

func TestSearchChainFallsBackOnFailure(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	h.provs = map[string]*fakeProvider{
		"exa":      {err: errors.New("exa is down")},
		"parallel": {err: errors.New("parallel is down")},
		"youcom":   {err: errors.New("youcom is down")},
		"ddg":      {results: []provider.SearchResult{paper}},
	}
	out, errOut, err := h.run("q")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.built, ",") != "exa,parallel,youcom,ddg" {
		t.Errorf("chain order = %v, want exa,parallel,youcom,ddg", h.built)
	}
	if !strings.Contains(errOut, "exa failed (exa is down); trying parallel") || !strings.Contains(errOut, "youcom failed (youcom is down); trying ddg") {
		t.Errorf("fallback notice missing: %q", errOut)
	}
	if !strings.Contains(errOut, "ddg: 1 results → 1 kept") {
		t.Errorf("summary should name the provider that succeeded: %q", errOut)
	}
	if !strings.Contains(out, "Attention Is All You Need") {
		t.Errorf("output = %q", out)
	}
}

func TestSearchChainAllFail(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e", SearXNGURL: "http://sx", JevAPIKey: "j"})
	h.prov.err = errors.New("boom")
	_, _, err := h.run("q")
	if err == nil || !strings.Contains(err.Error(), "all 5 providers failed") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	if strings.Join(h.built, ",") != "exa,parallel,youcom,ddg,searxng" {
		t.Errorf("chain order = %v", h.built)
	}
}

func TestSearchZeroConfigUsesDDG(t *testing.T) {
	// Nothing configured: keyless exa first, then parallel, then ddg.
	h := newHarness(t, keys.Store{})
	_, _, err := h.run("--no-filter", "--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if h.prov.name != "exa" || strings.Join(h.built, ",") != "exa" {
		t.Errorf("provider = %q, built = %v; want exa", h.prov.name, h.built)
	}
	h = newHarness(t, keys.Store{})
	h.provs = map[string]*fakeProvider{
		"exa":      {err: errors.New("quota")},
		"parallel": {err: errors.New("quota")},
		"youcom":   {err: errors.New("quota")},
	}
	if _, _, err := h.run("--no-filter", "--urls-only", "q"); err != nil || strings.Join(h.built, ",") != "exa,parallel,youcom,ddg" {
		t.Errorf("zero-config fallback: %v, built = %v", err, h.built)
	}

	// Explicit ddg works without keys too; explicit searxng without a URL does not.
	h = newHarness(t, keys.Store{})
	if _, _, err := h.run("-p", "duckduckgo", "--no-filter", "--urls-only", "q"); err != nil || h.prov.name != "ddg" {
		t.Errorf("explicit ddg: %v, %q", err, h.prov.name)
	}
	_, _, err = h.run("-p", "searxng", "--no-filter", "q")
	if err == nil || !strings.Contains(err.Error(), "SEARXNG_URL") {
		t.Errorf("explicit searxng without URL: %v", err)
	}
}

func TestSearchConfiguredProviderWithoutKeyErrors(t *testing.T) {
	h := newHarness(t, keys.Store{JevAPIKey: "j"})
	t.Setenv("MULTI_SEARCH_WEB_PROVIDER", "sonar")
	_, _, err := h.run("q")
	if err == nil || !strings.Contains(err.Error(), `configured provider "sonar" is unusable`) {
		t.Errorf("err = %v", err)
	}
	if len(h.built) != 0 {
		t.Error("should fail before constructing any provider")
	}
}

func TestSearchMultiFusesAndTagsEngines(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	h.provs = map[string]*fakeProvider{
		"exa":      {results: []provider.SearchResult{blog, paper}},
		"parallel": {err: errors.New("quota")},
		"youcom":   {err: errors.New("quota")},
		"ddg":      {results: []provider.SearchResult{paper, wiki}},
	}
	out, _, err := h.run("--multi", "--no-filter", "--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	got := mustJSON[[]rawResult](t, out)
	// paper appears in both lists (ranks 2 and 1) → top; blog (rank 1) beats wiki (rank 2).
	wantURLs := []string{paper.URL, blog.URL, wiki.URL}
	wantEngines := []string{"exa,ddg", "exa", "ddg"}
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	for i := range got {
		if got[i].URL != wantURLs[i] || strings.Join(got[i].Engines, ",") != wantEngines[i] {
			t.Errorf("[%d] = %s %v; want %s %s", i, got[i].URL, got[i].Engines, wantURLs[i], wantEngines[i])
		}
	}

	// Filtered mode: the summary names every engine and engines survive Jev.
	h = newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	h.provs = map[string]*fakeProvider{
		"exa":      {results: []provider.SearchResult{blog, paper}},
		"parallel": {err: errors.New("quota")},
		"youcom":   {err: errors.New("quota")},
		"ddg":      {results: []provider.SearchResult{paper, wiki}},
	}
	out, errOut, err := h.run("--multi", "--json", "--verbose", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "exa+ddg: 3 results → 2 kept") {
		t.Errorf("summary = %q", errOut)
	}
	items := mustJSON[[]outputResult](t, out)
	if len(items) != 3 || items[0].URL != paper.URL || strings.Join(items[0].Engines, ",") != "exa,ddg" {
		t.Errorf("items = %+v", items)
	}
	out, _, err = h.run("--multi", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Engines: exa, ddg") {
		t.Errorf("pretty output should list engines:\n%s", out)
	}
}

func TestSearchMultiPartialFailureAndCap(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	h.provs = map[string]*fakeProvider{
		"exa":      {err: errors.New("quota")},
		"parallel": {err: errors.New("quota")},
		"youcom":   {err: errors.New("quota")},
		"ddg":      {results: []provider.SearchResult{paper, wiki, blog}},
	}
	out, errOut, err := h.run("--multi", "--no-filter", "--urls-only", "-n", "2", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "exa failed: quota") {
		t.Errorf("stderr = %q", errOut)
	}
	if out != paper.URL+"\n"+wiki.URL+"\n" {
		t.Errorf("urls = %q", out)
	}

	h.provs["ddg"].err = errors.New("blocked")
	_, _, err = h.run("--multi", "--no-filter", "q")
	if err == nil || !strings.Contains(err.Error(), "all 4 providers failed") || !strings.Contains(err.Error(), "blocked") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchRandomFallsBack(t *testing.T) {
	orig := shuffleChain
	shuffleChain = func(chain []string) []string {
		out := append([]string(nil), chain...)
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
		return out
	}
	t.Cleanup(func() { shuffleChain = orig })

	h := newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	h.provs = map[string]*fakeProvider{
		"ddg":      {err: errors.New("blocked")},
		"youcom":   {err: errors.New("blocked")},
		"parallel": {err: errors.New("blocked")},
		"exa":      {results: []provider.SearchResult{paper}},
	}
	_, errOut, err := h.run("--random", "--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.built, ",") != "ddg,youcom,parallel,exa" {
		t.Errorf("random order = %v, want reversed chain ddg,youcom,parallel,exa", h.built)
	}
	if !strings.Contains(errOut, "ddg failed (blocked); trying youcom") || !strings.Contains(errOut, "parallel failed (blocked); trying exa") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSearchModeFlagConflicts(t *testing.T) {
	h := newHarness(t, allKeys())
	for _, args := range [][]string{
		{"--multi", "--random", "q"},
		{"--multi", "-p", "exa", "q"},
		{"--random", "-p", "exa", "q"},
	} {
		if _, _, err := h.run(args...); err == nil {
			t.Errorf("%v should be rejected", args)
		}
	}
	if len(h.built) != 0 {
		t.Error("conflicting flags should fail before any search")
	}
}

// fakeScraper serves canned page text per URL and records what was fetched.
type fakeScraper struct {
	pages    map[string]string // URL → content
	errs     map[string]error  // URL → error
	gotURLs  []string
	maxChars int
}

func (f *fakeScraper) FetchAll(_ context.Context, urls []string, _ int) []scrape.Page {
	f.gotURLs = append(f.gotURLs, urls...)
	out := make([]scrape.Page, len(urls))
	for i, u := range urls {
		out[i] = scrape.Page{URL: u, Content: f.pages[u], Err: f.errs[u]}
	}
	return out
}

func (h *harness) withScraper(pages map[string]string, errs map[string]error) *fakeScraper {
	fs := &fakeScraper{pages: pages, errs: errs}
	orig := newScraper
	newScraper = func(maxChars int) scraper {
		fs.maxChars = maxChars
		return fs
	}
	h.t.Cleanup(func() { newScraper = orig })
	return fs
}

func TestSearchScrape(t *testing.T) {
	h := newHarness(t, allKeys())
	fs := h.withScraper(
		map[string]string{paper.URL: "Abstract\n\nWe propose the Transformer."},
		map[string]error{wiki.URL: errors.New("HTTP 403")},
	)
	out, errOut, err := h.run("--scrape", "--json", "--verbose", "--max-chars", "1234", "q")
	if err != nil {
		t.Fatal(err)
	}
	// Only kept results are fetched (the blog is dropped by Jev).
	if strings.Join(fs.gotURLs, ",") != paper.URL+","+wiki.URL {
		t.Errorf("fetched %v", fs.gotURLs)
	}
	if fs.maxChars != 1234 {
		t.Errorf("max chars = %d", fs.maxChars)
	}
	items := mustJSON[[]outputResult](t, out)
	if len(items) != 3 || items[0].Content != "Abstract\n\nWe propose the Transformer." || items[0].ScrapeError != "" {
		t.Errorf("paper = %+v", items[0])
	}
	if items[1].Content != "" || items[1].ScrapeError != "HTTP 403" {
		t.Errorf("wiki = %+v", items[1])
	}
	if items[2].Content != "" || items[2].ScrapeError != "" {
		t.Errorf("dropped blog should have no content: %+v", items[2])
	}
	if !strings.Contains(errOut, "scraped 2 page(s) (1 failed), 37 chars") {
		t.Errorf("scrape summary missing: %q", errOut)
	}

	out, _, err = h.run("--scrape", "q")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"    --- content (37 chars) ---\n    Abstract\n\n    We propose the Transformer.\n    --- end ---\n",
		"    ! scrape failed: HTTP 403\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("pretty output missing %q:\n%s", want, out)
		}
	}
}

func TestSearchScrapeNoFilterAndURLsOnly(t *testing.T) {
	h := newHarness(t, keys.Store{ExaAPIKey: "e"})
	fs := h.withScraper(map[string]string{blog.URL: "b", paper.URL: "p", wiki.URL: "w"}, nil)
	out, _, err := h.run("--scrape", "--no-filter", "--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	raw := mustJSON[[]rawResult](t, out)
	if len(raw) != 3 || raw[0].Content != "b" || raw[2].Content != "w" || len(fs.gotURLs) != 3 {
		t.Errorf("raw = %+v, fetched %v", raw, fs.gotURLs)
	}
	if !strings.Contains(out, `"content": "b"`) {
		t.Errorf("content field missing:\n%s", out)
	}

	fs.gotURLs = nil
	if _, _, err := h.run("--scrape", "--no-filter", "--urls-only", "q"); err != nil {
		t.Fatal(err)
	}
	if len(fs.gotURLs) != 0 {
		t.Error("--urls-only should not fetch pages")
	}

	if _, _, err := h.run("--scrape", "--max-chars", "0", "q"); err == nil || !strings.Contains(err.Error(), "--max-chars") {
		t.Errorf("err = %v", err)
	}
}

// Three ~1500-char paragraphs: each becomes its own 2000-char chunk.
var (
	chunkA  = strings.TrimSpace(strings.Repeat("Attention lets the model weigh tokens. ", 38))
	chunkB  = strings.TrimSpace(strings.Repeat("Subscribe to our newsletter and accept cookies. ", 31))
	chunkC  = strings.TrimSpace(strings.Repeat("Multi-head attention runs several heads. ", 36))
	bigPage = chunkA + "\n\n" + chunkB + "\n\n" + chunkC
)

func TestSearchFilterChunks(t *testing.T) {
	h := newHarness(t, allKeys())
	h.qual.relevantWord = "attention"
	h.withScraper(map[string]string{paper.URL: bigPage, wiki.URL: "Short page about attention."}, nil)

	out, errOut, err := h.run("--scrape", "--filter-chunks", "--json", "--verbose", "q")
	if err != nil {
		t.Fatal(err)
	}
	if h.qual.chunkCalls != 2 {
		t.Errorf("expected one batch request per page, got %d", h.qual.chunkCalls)
	}
	for _, chunks := range h.qual.gotChunks {
		for _, c := range chunks {
			if len([]rune(c)) > scrape.DefaultChunkChars {
				t.Errorf("chunk of %d runes exceeds %d", len([]rune(c)), scrape.DefaultChunkChars)
			}
		}
	}
	items := mustJSON[[]outputResult](t, out)
	if len(items) != 3 || items[0].URL != paper.URL {
		t.Fatalf("items = %+v", items)
	}
	p := items[0]
	if p.ChunksTotal == nil || *p.ChunksTotal != 3 || p.ChunksKept == nil || *p.ChunksKept != 2 {
		t.Errorf("paper chunk stats = %v/%v", p.ChunksKept, p.ChunksTotal)
	}
	if p.Content != chunkA+"\n\n"+chunkC {
		t.Errorf("paper content = %q", p.Content)
	}
	w := items[1]
	if w.ChunksTotal == nil || *w.ChunksTotal != 1 || *w.ChunksKept != 1 || w.Content != "Short page about attention." {
		t.Errorf("wiki = %+v", w)
	}
	if items[2].ChunksTotal != nil || items[2].Content != "" {
		t.Errorf("dropped result should not be scraped: %+v", items[2])
	}
	if !strings.Contains(errOut, "scraped 2 page(s); chunks 4 → 3 kept; ") || !strings.Contains(errOut, " chars\n") {
		t.Errorf("summary = %q", errOut)
	}

	out, _, err = h.run("--scrape", "--filter-chunks", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "    --- content (2/3 chunks kept, ") || !strings.Contains(out, "    --- end ---") {
		t.Errorf("pretty separator missing:\n%s", out)
	}
	if strings.Contains(out, "newsletter") {
		t.Errorf("dropped chunk leaked into pretty output")
	}
}

func TestSearchFilterChunksWithNoFilter(t *testing.T) {
	// --no-filter skips result qualification but chunk filtering still needs Jev.
	h := newHarness(t, keys.Store{ExaAPIKey: "e"})
	h.withScraper(map[string]string{paper.URL: bigPage}, nil)
	_, _, err := h.run("--scrape", "--filter-chunks", "--no-filter", "q")
	if err == nil || !strings.Contains(err.Error(), "no Jev API key") {
		t.Errorf("err = %v", err)
	}
	if h.prov.gotQuery != "" {
		t.Error("should fail before searching")
	}

	h = newHarness(t, keys.Store{ExaAPIKey: "e", JevAPIKey: "j"})
	h.qual.relevantWord = "attention"
	h.qual.unansweredWord = "Multi-head"
	h.prov.results = []provider.SearchResult{paper}
	h.withScraper(map[string]string{paper.URL: bigPage}, nil)
	out, _, err := h.run("--scrape", "--filter-chunks", "--no-filter", "--json", "q")
	if err != nil {
		t.Fatal(err)
	}
	if h.qual.calls != 0 || h.qual.chunkCalls != 1 {
		t.Errorf("qualify calls = %d, chunk calls = %d", h.qual.calls, h.qual.chunkCalls)
	}
	raw := mustJSON[[]rawResult](t, out)
	// Unanswered chunk (C) is dropped along with the irrelevant one (B).
	if len(raw) != 1 || raw[0].Content != chunkA || *raw[0].ChunksTotal != 3 || *raw[0].ChunksKept != 1 {
		t.Errorf("raw = %+v", raw)
	}
}

func TestSearchFilterChunksJevErrorKeepsContent(t *testing.T) {
	h := newHarness(t, allKeys())
	h.qual.chunkErr = errors.New("jev down")
	h.prov.results = []provider.SearchResult{paper}
	h.withScraper(map[string]string{paper.URL: bigPage}, nil)
	out, errOut, err := h.run("--scrape", "--filter-chunks", "--json", "--verbose", "q")
	if err != nil {
		t.Fatal(err)
	}
	items := mustJSON[[]outputResult](t, out)
	if len(items) != 1 || items[0].Content != bigPage || items[0].FilterError != "jev down" || *items[0].ChunksKept != 3 {
		t.Errorf("items = %+v", items)
	}
	if !strings.Contains(errOut, "1 page(s) unfiltered (Jev error)") {
		t.Errorf("summary = %q", errOut)
	}
	out, _, err = h.run("--scrape", "--filter-chunks", "q")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "! chunk filter failed: jev down (showing unfiltered content)") {
		t.Errorf("pretty output:\n%s", out)
	}
}

func TestSearchFilterChunksRequiresScrape(t *testing.T) {
	h := newHarness(t, allKeys())
	_, _, err := h.run("--filter-chunks", "q")
	if err == nil || !strings.Contains(err.Error(), "--filter-chunks requires --scrape") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchChainTimesOutSlowProvider(t *testing.T) {
	origAttempt, origBudget := attemptTimeout, chainBudget
	attemptTimeout, chainBudget = 30*time.Millisecond, 500*time.Millisecond
	t.Cleanup(func() { attemptTimeout, chainBudget = origAttempt, origBudget })

	h := newHarness(t, keys.Store{})
	h.provs = map[string]*fakeProvider{
		"exa":      {hang: true},
		"parallel": {results: []provider.SearchResult{paper}},
	}
	h.provs["youcom"] = &fakeProvider{results: []provider.SearchResult{paper}}
	start := time.Now()
	_, errOut, err := h.run("--no-filter", "--urls-only", "q")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.built, ",") != "exa,parallel" || !strings.Contains(errOut, "exa failed") {
		t.Errorf("built = %v, stderr = %q", h.built, errOut)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Errorf("slow provider held the chain for %s", time.Since(start))
	}

	// Every provider hanging exhausts the chain budget rather than the sum of attempts.
	attemptTimeout, chainBudget = time.Second, 50*time.Millisecond
	h = newHarness(t, keys.Store{})
	h.provs = map[string]*fakeProvider{"exa": {hang: true}, "parallel": {hang: true}, "youcom": {hang: true}, "ddg": {hang: true}}
	start = time.Now()
	_, _, err = h.run("--no-filter", "--urls-only", "q")
	if err == nil || !strings.Contains(err.Error(), "chain budget") {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("chain ran %s despite a 50ms budget", time.Since(start))
	}
}

func TestSearchChainTopUpLabel(t *testing.T) {
	h := newHarness(t, keys.Store{JevAPIKey: "j"})
	chainTopUp = true
	h.provs = map[string]*fakeProvider{
		"exa":      {results: []provider.SearchResult{paper}},
		"parallel": {results: []provider.SearchResult{wiki, paper}},
	}
	out, errOut, err := h.run("--no-filter", "--urls-only", "-n", "10", "q")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.built, ",") != "exa,parallel" || strings.Contains(errOut, "failed") {
		t.Errorf("built = %v, stderr = %q", h.built, errOut)
	}
	if !strings.Contains(out, paper.URL) || !strings.Contains(out, wiki.URL) {
		t.Errorf("fused output = %q", out)
	}
}
