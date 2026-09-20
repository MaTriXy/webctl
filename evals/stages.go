package evals

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dorkitude/webctl/internal/dedupe"
	"github.com/dorkitude/webctl/internal/jev"
	"github.com/dorkitude/webctl/internal/prompts"
	"github.com/dorkitude/webctl/internal/provider"
	"github.com/dorkitude/webctl/internal/scrape"
)

// Scraper is the slice of *scrape.Fetcher the scrape stage needs.
type Scraper interface {
	FetchAll(ctx context.Context, urls []string, concurrency int) []scrape.Page
}

// chunkFilterer is implemented by *jev.Client; the scrape stage needs it.
type chunkFilterer interface {
	FilterChunks(ctx context.Context, query string, chunks []string) ([]*jev.NoulAnswer, jev.Usage, error)
}

// dupConfirmer is implemented by *jev.Client; the filter stage folds
// near-duplicates with it, as the product does.
type dupConfirmer interface {
	ConfirmDuplicates(ctx context.Context, query string, results []provider.SearchResult, pairs []jev.DuplicatePair) ([]bool, jev.Usage, error)
}

type evalConfirmer struct {
	c     dupConfirmer
	usage jev.Usage
}

func (e *evalConfirmer) ConfirmDuplicates(ctx context.Context, query string, results []provider.SearchResult, pairs []dedupe.Pair) ([]bool, error) {
	jp := make([]jev.DuplicatePair, len(pairs))
	for i, p := range pairs {
		jp[i] = jev.DuplicatePair{A: p.A, B: p.B}
	}
	out, usage, err := e.c.ConfirmDuplicates(ctx, query, results, jp)
	e.usage.Add(usage)
	return out, err
}

// foldDuplicates collapses confirmed duplicate groups into their
// best-valued member and returns the survivors in order.
func (r *Runner) foldDuplicates(ctx context.Context, query string, qualified []jev.Qualified) ([]jev.Qualified, int, jev.Usage) {
	dc, ok := r.Jev.(dupConfirmer)
	if !ok {
		return qualified, 0, jev.Usage{}
	}
	results := make([]provider.SearchResult, len(qualified))
	for i, q := range qualified {
		results[i] = q.Result
	}
	ec := &evalConfirmer{c: dc}
	groups, _, err := dedupe.Run(ctx, ec, query, results)
	if err != nil {
		return qualified, 0, ec.usage
	}
	drop := map[int]bool{}
	for _, g := range groups {
		best := g[0]
		for _, i := range g[1:] {
			if qualified[i].Value() > qualified[best].Value() {
				best = i
			}
		}
		for _, i := range g {
			if i != best {
				drop[i] = true
			}
		}
	}
	out := make([]jev.Qualified, 0, len(qualified))
	for i, q := range qualified {
		if !drop[i] {
			out = append(out, q)
		}
	}
	return out, len(drop), ec.usage
}

// coverageSnippetChars caps the text per result sent to the theme judge in
// the scrape stage, where "snippet" is a page's kept chunks.
const coverageSnippetChars = 6000

// hostMatches reports whether rawURL's host is one of domains or a subdomain
// of one.
func hostMatches(rawURL string, domains []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "www."))
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// tally fills the delivery counters of st from results. flagged holds the
// URLs the source-quality audit marked low-value.
func (st *Stage) tally(c Case, results []provider.SearchResult, flagged map[string]bool, chars func(provider.SearchResult) int) {
	st.Results = len(results)
	for _, r := range results {
		st.Chars += chars(r)
		st.URLs = append(st.URLs, r.URL)
		if hostMatches(r.URL, c.JunkDomains) {
			st.Junk++
		}
		if hostMatches(r.URL, c.ExpectedDomains) {
			st.ExpectedHits++
		}
		if flagged[r.URL] {
			st.Flagged++
		}
	}
}

// Caps on the text sent to the theme judge, so a large delivery never
// exceeds Jev's request size. Per-result text is cut first, then results
// are dropped from the end.
const (
	judgeResultChars = 2500
	judgeTotalChars  = 30000
)

// forJudge returns results trimmed to the judge caps. A result's content
// (the provider's excerpt) stands in for its snippet when it says more, so
// coverage does not hinge on how terse a provider's snippets are.
func forJudge(results []provider.SearchResult) []provider.SearchResult {
	out := make([]provider.SearchResult, 0, len(results))
	total := 0
	for _, r := range results {
		if len([]rune(r.Content)) > len([]rune(r.Snippet)) {
			r.Snippet = r.Content
		}
		r.Content = ""
		r.Snippet = scrape.Truncate(r.Snippet, judgeResultChars)
		size := len([]rune(r.Snippet)) + len([]rune(r.Title)) + len([]rune(r.URL))
		if total+size > judgeTotalChars && len(out) > 0 {
			break
		}
		total += size
		out = append(out, r)
	}
	return out
}

// AuditKey is the Jev question key for the i-th audited result.
func AuditKey(i int) string { return "audit_" + strconv.Itoa(i) }

type auditItem struct {
	ID string `json:"id"`
	provider.SearchResult
}

type auditState struct {
	Query   string      `json:"query"`
	Results []auditItem `json:"results"`
}

// audit asks Jev, in one batch request, which results are low-value SEO,
// affiliate, or content-farm pages, and returns their URLs.
func (r *Runner) audit(ctx context.Context, query string, results []provider.SearchResult) (map[string]bool, jev.Usage, error) {
	flagged := map[string]bool{}
	if len(results) == 0 {
		return flagged, jev.Usage{}, nil
	}
	p, err := prompts.Load(prompts.SourceQuality)
	if err != nil {
		return flagged, jev.Usage{}, err
	}
	state := auditState{Query: query}
	questions := make(map[string]jev.Question, len(results))
	for i, res := range results {
		id := AuditKey(i)
		res.Content = ""
		state.Results = append(state.Results, auditItem{ID: id, SearchResult: res})
		instructions, err := p.Render(prompts.Data{Query: query, Title: res.Title, URL: res.URL, Snippet: res.Snippet, ID: id, Index: i})
		if err != nil {
			return flagged, jev.Usage{}, err
		}
		questions[id] = jev.NoulQuestion(instructions)
	}
	resp, err := r.Jev.SystemOne(ctx, &jev.SystemOneRequest{State: state, Questions: questions})
	if err != nil {
		return flagged, jev.Usage{}, fmt.Errorf("source-quality audit: %w", err)
	}
	for i, res := range results {
		if raw, ok := resp.Answers[AuditKey(i)]; ok {
			if ans, err := raw.AsNoul(); err == nil && ans.Yes() {
				flagged[res.URL] = true
			}
		}
	}
	return flagged, resp.Usage, nil
}

func snippetChars(r provider.SearchResult) int { return len([]rune(r.Snippet)) }

func (r *Runner) threshold(c Case) float64 {
	threshold := r.CoverageThreshold
	if c.CoverageThreshold != nil {
		threshold = *c.CoverageThreshold
	}
	if threshold <= 0 {
		threshold = DefaultCoverageThreshold
	}
	return threshold
}

// judge runs theme coverage into st and records failures.
func (r *Runner) judge(ctx context.Context, c Case, st *Stage, results []provider.SearchResult) error {
	if len(c.ExpectedThemes) == 0 {
		return nil
	}
	themes, usage, err := r.coverage(ctx, c.Query, c.ExpectedThemes, forJudge(results), r.threshold(c))
	st.Usage.Add(usage)
	if err != nil {
		return fmt.Errorf("theme coverage: %w", err)
	}
	st.Themes = themes
	for _, th := range themes {
		if th.Covered {
			st.Covered++
			continue
		}
		reason := fmt.Sprintf("theme %q not covered (P(yes)=%.2f)", th.Theme, th.Probability)
		if th.Error != "" {
			reason = fmt.Sprintf("theme %q not judged: %s", th.Theme, th.Error)
		}
		st.Failures = append(st.Failures, reason)
	}
	return nil
}

// stageNoFilter measures the raw results: everything the provider returned
// would reach the context window.
func (r *Runner) stageNoFilter(ctx context.Context, c Case, results []provider.SearchResult, flagged map[string]bool) Stage {
	start := time.Now()
	st := Stage{Mode: ModeNoFilter}
	st.tally(c, results, flagged, snippetChars)
	if err := r.judge(ctx, c, &st, results); err != nil {
		st.Error = err.Error()
	}
	st.Passed = st.Error == "" && len(st.Failures) == 0
	st.Duration = time.Since(start)
	return st
}

// stageFilter qualifies results with Jev, keeps those at or above the
// case threshold, and checks count bounds, theme coverage, junk, and
// expected-domain retention.
func (r *Runner) stageFilter(ctx context.Context, c Case, results []provider.SearchResult, flagged map[string]bool) (st Stage, out []KeptResult, judged []KeptResult, err error) {
	start := time.Now()
	st = Stage{Mode: ModeFilter}
	defer func() { st.Duration = time.Since(start) }()

	var kept []jev.Qualified
	if len(results) > 0 {
		qualified, usage, err := r.Jev.Qualify(ctx, c.Query, results, jev.QualifyOptions{
			Rubric: c.Rubric,
			Noul:   c.Noul,
			Batch:  c.Batch || r.Batch,
		})
		st.Usage.Add(usage)
		if err != nil {
			return st, nil, nil, fmt.Errorf("qualify: %w", err)
		}
		var folded int
		var dupUsage jev.Usage
		qualified, folded, dupUsage = r.foldDuplicates(ctx, c.Query, qualified)
		st.Usage.Add(dupUsage)
		st.Folded = folded
		min := c.threshold()
		for _, q := range qualified {
			if q.Err != nil || (q.Score == nil && q.Noul == nil) {
				continue
			}
			judged = append(judged, KeptResult{SearchResult: q.Result, Value: q.Value(), Confidence: q.Confidence()})
			if q.Value() >= min {
				kept = append(kept, q)
			}
		}
		sort.SliceStable(kept, func(i, j int) bool { return kept[i].Value() > kept[j].Value() })
		sort.SliceStable(judged, func(i, j int) bool { return judged[i].Value > judged[j].Value })
	}
	out = make([]KeptResult, 0, len(kept))
	plain := make([]provider.SearchResult, 0, len(kept))
	for _, q := range kept {
		out = append(out, KeptResult{SearchResult: q.Result, Value: q.Value(), Confidence: q.Confidence()})
		plain = append(plain, q.Result)
	}
	st.tally(c, plain, flagged, snippetChars)

	if c.MinResults > 0 && len(kept) < c.MinResults {
		st.Failures = append(st.Failures, fmt.Sprintf("too few results: %d kept, need ≥ %d", len(kept), c.MinResults))
	}
	if c.MaxResults > 0 && len(kept) > c.MaxResults {
		st.Failures = append(st.Failures, fmt.Sprintf("too many results: %d kept, want ≤ %d", len(kept), c.MaxResults))
	}
	if st.Junk > 0 {
		st.Failures = append(st.Failures, fmt.Sprintf("kept %d junk-domain result(s)", st.Junk))
	}
	if len(c.ExpectedDomains) > 0 && st.ExpectedHits == 0 {
		rawHits := 0
		for _, res := range results {
			if hostMatches(res.URL, c.ExpectedDomains) {
				rawHits++
			}
		}
		if rawHits > 0 {
			st.Failures = append(st.Failures, fmt.Sprintf("dropped every result from %s (%d in raw results)", strings.Join(c.ExpectedDomains, "/"), rawHits))
		}
	}
	if err := r.judge(ctx, c, &st, plain); err != nil {
		return st, nil, nil, err
	}
	st.Passed = len(st.Failures) == 0
	return st, out, judged, nil
}

// stageScrape fetches each kept result's page, keeps the chunks Jev judges
// relevant, and re-judges theme coverage on that text. A page that cannot
// be fetched falls back to the provider's excerpt, as the product does.
func (r *Runner) stageScrape(ctx context.Context, c Case, kept []KeptResult, flagged map[string]bool) (st Stage) {
	start := time.Now()
	st = Stage{Mode: ModeScrape}
	defer func() { st.Duration = time.Since(start) }()

	cf, ok := r.Jev.(chunkFilterer)
	if !ok {
		st.Error = "Jev client cannot filter chunks"
		return st
	}
	if len(kept) == 0 {
		st.Passed = len(c.ExpectedThemes) == 0
		if !st.Passed {
			st.Failures = append(st.Failures, "no kept results to scrape")
		}
		return st
	}
	scraper := r.Scraper
	if scraper == nil {
		scraper = &scrape.Fetcher{}
	}
	urls := make([]string, len(kept))
	for i, k := range kept {
		urls[i] = k.URL
	}
	pages := scraper.FetchAll(ctx, urls, scrape.DefaultConcurrency)

	type outcome struct {
		text, dropped string
		total, keptN  int
		usage         jev.Usage
		err           error
	}
	outcomes := make([]outcome, len(kept))
	var wg sync.WaitGroup
	sem := make(chan struct{}, jev.DefaultConcurrency)
	for i := range kept {
		text := pages[i].Content
		if pages[i].Err != nil {
			st.PagesFailed++
			if u, err := url.Parse(kept[i].URL); err == nil {
				st.PageFailures = append(st.PageFailures, u.Hostname()+": "+pages[i].Err.Error())
			}
			text = kept[i].Content
		} else {
			st.PagesOK++
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, text string) {
			defer wg.Done()
			defer func() { <-sem }()
			chunks := scrape.Split(text, scrape.DefaultChunkChars)
			answers, usage, err := cf.FilterChunks(ctx, c.Query, chunks)
			o := outcome{total: len(chunks), usage: usage, err: err}
			if err != nil {
				o.text = text
				o.keptN = len(chunks)
			} else {
				var keep, drop []string
				for j, ch := range chunks {
					if j < len(answers) && answers[j] != nil && answers[j].Yes() {
						keep = append(keep, ch)
					} else {
						drop = append(drop, ch)
					}
				}
				o.text = scrape.Join(keep)
				o.dropped = scrape.Join(drop)
				o.keptN = len(keep)
			}
			outcomes[i] = o
		}(i, text)
	}
	wg.Wait()

	var delivered []provider.SearchResult
	for i, k := range kept {
		o := outcomes[i]
		st.Usage.Add(o.usage)
		st.ChunksTotal += o.total
		st.ChunksKept += o.keptN
		raw := pages[i].Content
		if pages[i].Err != nil {
			raw = k.Content
		}
		st.CharsRaw += len([]rune(raw))
		if o.err != nil {
			st.Failures = append(st.Failures, fmt.Sprintf("chunk filter failed for %s: %v", k.URL, o.err))
		}
		if strings.TrimSpace(o.text) == "" {
			continue
		}
		delivered = append(delivered, provider.SearchResult{
			Title:   k.Title,
			URL:     k.URL,
			Snippet: scrape.Truncate(o.text, coverageSnippetChars),
		})
	}
	st.tally(c, delivered, flagged, func(r provider.SearchResult) int { return len([]rune(r.Snippet)) })
	// Chars is the full kept text, not the capped judge input.
	st.Chars = 0
	for _, o := range outcomes {
		st.Chars += len([]rune(o.text))
	}
	if err := r.judge(ctx, c, &st, delivered); err != nil {
		st.Error = err.Error()
	}
	// Recall check: do the discarded chunks still cover any theme?
	var discarded []provider.SearchResult
	for i, k := range kept {
		if strings.TrimSpace(outcomes[i].dropped) != "" {
			discarded = append(discarded, provider.SearchResult{Title: k.Title, URL: k.URL, Snippet: scrape.Truncate(outcomes[i].dropped, coverageSnippetChars)})
		}
	}
	if len(discarded) > 0 && len(c.ExpectedThemes) > 0 && st.Error == "" {
		themes, usage, err := r.coverage(ctx, c.Query, c.ExpectedThemes, forJudge(discarded), r.threshold(c))
		st.Usage.Add(usage)
		if err != nil {
			st.Error = fmt.Sprintf("dropped-chunk coverage: %v", err)
		}
		for _, th := range themes {
			if th.Covered {
				st.DroppedCovered++
			}
		}
	}
	st.Passed = st.Error == "" && len(st.Failures) == 0
	return st
}
