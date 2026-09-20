package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/dorkitude/multi_search_web/internal/config"
	"github.com/dorkitude/multi_search_web/internal/dedupe"
	"github.com/dorkitude/multi_search_web/internal/jev"
	"github.com/dorkitude/multi_search_web/internal/keys"
	"github.com/dorkitude/multi_search_web/internal/provider"
	"github.com/dorkitude/multi_search_web/internal/scrape"
)

// searchFlags holds the flag values for the search (root) command.
type searchFlags struct {
	provider string
	num      int
	minScore float64
	jsonOut  bool
	urlsOnly bool
	noFilter bool
	verbose  bool
	batch    bool
	rubric   string
	noul     string
	multi    bool
	random   bool
	sources  int
	scrape   bool
	maxChars int
	chunks   bool
	noDedupe bool
}

var sf searchFlags

func addSearchFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&sf.provider, "provider", "p", "", "use exactly one provider: ketch, searxng, ddg, or keyed exa, parallel, sonar, youcom")
	f.IntVarP(&sf.num, "num", "n", 0, "results to request per provider (default 10)")
	f.IntVar(&sf.sources, "sources", 0, "providers to query and fuse (default 3)")
	f.BoolVar(&sf.multi, "multi", false, "query every available provider")
	f.BoolVar(&sf.random, "random", false, "query every available provider in random order")
	f.Float64VarP(&sf.minScore, "min-score", "m", -1, "keep results scoring at least this on the 0–3 rubric (default 1.8)")
	f.StringVar(&sf.rubric, "rubric", "", "custom score levels, comma-separated, lowest first")
	f.StringVar(&sf.noul, "noul", "", "ask this yes/no question per result instead of scoring")
	f.BoolVar(&sf.batch, "batch", false, "score all results in one Jev request")
	f.BoolVar(&sf.noFilter, "no-filter", false, "skip Jev; print provider results")
	f.BoolVar(&sf.noDedupe, "no-dedupe", false, "skip the Jev near-duplicate pass")
	f.BoolVar(&sf.scrape, "scrape", false, "fetch each kept result's page text")
	f.BoolVar(&sf.chunks, "filter-chunks", false, "with --scrape, keep only chunks Jev finds relevant")
	f.IntVar(&sf.maxChars, "max-chars", scrape.DefaultMaxChars, "with --scrape, cap text per page")
	f.BoolVar(&sf.jsonOut, "json", false, "JSON output")
	f.BoolVar(&sf.urlsOnly, "urls-only", false, "one URL per line")
	f.BoolVarP(&sf.verbose, "verbose", "v", false, "show scores, dropped results, every cooldown notice")
}

// qualifier is the slice of *jev.Client the search pipeline depends on.
// It exists so tests can substitute a fake without an HTTP server.
type qualifier interface {
	Qualify(ctx context.Context, query string, results []provider.SearchResult, opts jev.QualifyOptions) ([]jev.Qualified, jev.Usage, error)
	FilterChunks(ctx context.Context, query string, chunks []string) ([]*jev.NoulAnswer, jev.Usage, error)
	ConfirmDuplicates(ctx context.Context, query string, results []provider.SearchResult, pairs []jev.DuplicatePair) ([]bool, jev.Usage, error)
}

// jevConfirmer adapts a qualifier to dedupe.Confirmer, accumulating usage.
type jevConfirmer struct {
	q     qualifier
	usage jev.Usage
}

func (j *jevConfirmer) ConfirmDuplicates(ctx context.Context, query string, results []provider.SearchResult, pairs []dedupe.Pair) ([]bool, error) {
	jp := make([]jev.DuplicatePair, len(pairs))
	for i, p := range pairs {
		jp[i] = jev.DuplicatePair{A: p.A, B: p.B}
	}
	out, usage, err := j.q.ConfirmDuplicates(ctx, query, results, jp)
	j.usage.Add(usage)
	return out, err
}

// collapseDuplicates asks Jev which near-duplicate candidates are the same
// content and folds each group into its best-scored member, which keeps
// the union of engines and lists the others as Duplicates. It returns the
// survivors in their original order and how many were folded.
func collapseDuplicates(ctx context.Context, q qualifier, query string, qualified []jev.Qualified, engines map[string][]string) ([]jev.Qualified, int, jev.Usage, error) {
	results := make([]provider.SearchResult, len(qualified))
	for i, item := range qualified {
		results[i] = item.Result
	}
	jc := &jevConfirmer{q: q}
	groups, _, err := dedupe.Run(ctx, jc, query, results)
	if err != nil {
		return qualified, 0, jc.usage, err
	}
	drop := map[int]bool{}
	for _, g := range groups {
		best := g[0]
		for _, i := range g[1:] {
			if qualified[i].Value() > qualified[best].Value() {
				best = i
			}
		}
		keeper := &qualified[best]
		for _, i := range g {
			if i == best {
				continue
			}
			keeper.Duplicates = append(keeper.Duplicates, qualified[i].Result)
			drop[i] = true
			if engines != nil {
				engines[keeper.Result.URL] = unionStrings(engines[keeper.Result.URL], engines[qualified[i].Result.URL])
			}
		}
	}
	out := make([]jev.Qualified, 0, len(qualified))
	for i, item := range qualified {
		if !drop[i] {
			out = append(out, item)
		}
	}
	return out, len(drop), jc.usage, nil
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// scraper is the slice of *scrape.Fetcher the pipeline depends on.
type scraper interface {
	FetchAll(ctx context.Context, urls []string, concurrency int) []scrape.Page
}

// Construction hooks. Tests override these to inject fakes.
var (
	newScraper = func(maxChars int) scraper {
		return &scrape.Fetcher{MaxChars: maxChars}
	}
	newProvider = func(cfg *config.Config, name string) (provider.Provider, error) {
		cred, err := cfg.ProviderKey(name)
		if err != nil {
			return nil, err
		}
		return provider.New(name, cred, provider.Options{})
	}
	newQualifier = func(cfg *config.Config, key string) qualifier {
		c := jev.NewClient(key)
		c.BaseURL = cfg.JevBaseURL
		c.Model = cfg.JevModel
		return c
	}
)

// noulDefaultThreshold is the P(yes) cutoff used with --noul when --min-score
// is not given. The score-mode default (1.0) would drop every result, since
// P(yes) never exceeds 1.
const noulDefaultThreshold = 0.5

// searchOptions is the fully-resolved, validated input to the pipeline.
type searchOptions struct {
	Query string
	// Providers is the ordered chain to try (or, with modeMulti, to fuse).
	Providers []string
	Mode      searchMode
	// Sources is how many providers to gather from and fuse.
	Sources  int
	Num      int
	MinScore float64
	NoFilter bool
	Batch    bool
	Verbose  bool
	Rubric   []string
	Noul     string
	Format   outputFormat
	// Scrape fetches page content for kept results; MaxChars caps it per page.
	Scrape   bool
	MaxChars int
	// FilterChunks keeps only the scraped chunks Jev judges relevant.
	FilterChunks bool
	// NoDedupe skips the Jev duplicate pass.
	NoDedupe bool
}

type outputFormat int

// searchMode selects how the provider chain is used.
type searchMode int

const (
	modeChain  searchMode = iota // first success in chain order
	modeMulti                    // all providers in parallel, fused with RRF
	modeRandom                   // chain in random order
)

const (
	formatPretty outputFormat = iota
	formatJSON
	formatURLs
)

func runSearch(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	opts, err := resolveSearchOptions(cfg, sf, args)
	if err != nil {
		return err
	}
	return runPipeline(cmd.Context(), cfg, opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// resolveSearchOptions merges flags with config defaults and validates them.
func resolveSearchOptions(cfg *config.Config, f searchFlags, args []string) (searchOptions, error) {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		return searchOptions{}, errors.New("query is empty")
	}
	if f.jsonOut && f.urlsOnly {
		return searchOptions{}, errors.New("--json and --urls-only are mutually exclusive")
	}
	if f.rubric != "" && f.noul != "" {
		return searchOptions{}, errors.New("--rubric and --noul are mutually exclusive")
	}
	if f.multi && f.random {
		return searchOptions{}, errors.New("--multi and --random are mutually exclusive")
	}
	if f.provider != "" && (f.multi || f.random) {
		return searchOptions{}, errors.New("--provider cannot be combined with --multi or --random")
	}
	if f.scrape && f.maxChars <= 0 {
		return searchOptions{}, fmt.Errorf("--max-chars must be positive, got %d", f.maxChars)
	}
	if f.chunks && !f.scrape {
		return searchOptions{}, errors.New("--filter-chunks requires --scrape")
	}

	opts := searchOptions{
		Query:        query,
		Num:          cfg.Num,
		MinScore:     cfg.MinScore,
		NoFilter:     f.noFilter,
		Batch:        f.batch,
		Verbose:      f.verbose,
		Noul:         strings.TrimSpace(f.noul),
		Scrape:       f.scrape,
		MaxChars:     f.maxChars,
		FilterChunks: f.chunks,
		NoDedupe:     f.noDedupe,
	}
	if f.num > 0 {
		opts.Num = f.num
	}
	switch {
	case f.jsonOut:
		opts.Format = formatJSON
	case f.urlsOnly:
		opts.Format = formatURLs
	}

	explicitMin := f.minScore >= 0
	if explicitMin {
		opts.MinScore = f.minScore
	} else if opts.Noul != "" {
		opts.MinScore = noulDefaultThreshold
	}
	if opts.Noul != "" && opts.MinScore > 1 {
		return searchOptions{}, fmt.Errorf("--min-score %.2f is impossible with --noul: P(yes) is at most 1", opts.MinScore)
	}

	if f.rubric != "" {
		rubric, err := parseRubric(f.rubric)
		if err != nil {
			return searchOptions{}, err
		}
		opts.Rubric = rubric
		max := float64(len(rubric) - 1)
		if !explicitMin {
			// Same rule as the default rubric: lean toward the second-highest
			// level or better.
			opts.MinScore = math.Max(max-1.2, 0)
		}
		if opts.MinScore > max {
			return searchOptions{}, fmt.Errorf("--min-score %.2f exceeds the rubric's top score of %g", opts.MinScore, max)
		}
	}

	chain, err := cfg.Chain(f.provider)
	if err != nil {
		return searchOptions{}, err
	}
	opts.Providers = chain
	opts.Sources = cfg.Sources
	if f.sources > 0 {
		opts.Sources = f.sources
	}
	if f.provider != "" {
		opts.Sources = 1
	}
	switch {
	case f.multi:
		opts.Mode = modeMulti
		opts.Sources = len(chain)
	case f.random:
		opts.Mode = modeRandom
	}
	return opts, nil
}

// parseRubric splits a comma-separated criteria list, lowest to highest.
func parseRubric(s string) ([]string, error) {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("--rubric needs at least 2 comma-separated criteria, got %d", len(out))
	}
	return out, nil
}

// runPipeline executes search → qualify → filter → output.
func runPipeline(ctx context.Context, cfg *config.Config, opts searchOptions, out, errOut io.Writer) error {
	// An explicitly chosen provider must be usable; fail before any network call.
	if len(opts.Providers) == 1 {
		if _, err := cfg.ProviderKey(opts.Providers[0]); err != nil {
			return err
		}
	}
	// Resolve the Jev key before searching so a missing key fails fast
	// instead of after a paid provider call.
	var jevKey string
	if !opts.NoFilter || opts.FilterChunks {
		var err error
		if jevKey, err = cfg.JevKey(); err != nil {
			return err
		}
	}
	// chunkFilter is non-nil only when --filter-chunks needs Jev.
	var chunkFilter qualifier
	if opts.FilterChunks {
		chunkFilter = newQualifier(cfg, jevKey)
	}

	label, results, engines, err := runSearchStage(ctx, cfg, opts, errOut)
	if err != nil {
		return err
	}
	results = provider.Dedupe(results)

	if opts.NoFilter {
		if len(results) == 0 && opts.Format == formatPretty {
			fmt.Fprintf(errOut, "%s returned no results.\n", label)
		}
		raw := toRaw(results, engines)
		if opts.Scrape && opts.Format != formatURLs {
			urls := make([]string, len(raw))
			fallback := make([]string, len(raw))
			for i, r := range raw {
				urls[i], fallback[i] = r.URL, r.Content
			}
			pages := scrapePages(ctx, opts, chunkFilter, urls, fallback)
			for i := range raw {
				raw[i].Page = &pages[i]
			}
			writeScrapeSummary(errOut, pages, opts)
		}
		return writeRaw(out, raw, opts.Format)
	}

	if len(results) == 0 {
		if opts.Format == formatPretty {
			fmt.Fprintf(errOut, "%s returned no results.\n", label)
		}
		return writeQualified(out, nil, opts)
	}

	q := newQualifier(cfg, jevKey)
	qualified, usage, err := q.Qualify(ctx, opts.Query, results, jev.QualifyOptions{
		Rubric: opts.Rubric,
		Noul:   opts.Noul,
		Batch:  opts.Batch,
	})
	if err != nil {
		return err
	}

	// Engines often index the same page at different addresses: one Jev
	// pass over the near-duplicate candidates folds those together.
	folded := 0
	if !opts.NoDedupe {
		var dupUsage jev.Usage
		var dedupeErr error
		if engines == nil {
			engines = map[string][]string{}
		}
		qualified, folded, dupUsage, dedupeErr = collapseDuplicates(ctx, q, opts.Query, qualified, engines)
		usage.Add(dupUsage)
		if dedupeErr != nil && (opts.Format == formatPretty || opts.Verbose) {
			fmt.Fprintf(errOut, "duplicate check failed (%v); showing all results\n", dedupeErr)
		}
	}
	ranked := rank(qualified, opts.MinScore)
	for i := range ranked {
		ranked[i].Engines = engines[ranked[i].Result.URL]
	}
	if folded > 0 && (opts.Format == formatPretty || opts.Verbose) {
		fmt.Fprintf(errOut, "%d duplicate(s) folded into their best copy\n", folded)
	}
	if opts.Format == formatPretty || opts.Verbose {
		writeSummary(errOut, label, ranked, opts, usage)
	}
	if opts.Scrape && opts.Format != formatURLs {
		// Only kept results are worth fetching.
		var idx []int
		var urls, fallback []string
		for i, r := range ranked {
			if r.Kept {
				idx = append(idx, i)
				urls = append(urls, r.Result.URL)
				fallback = append(fallback, r.Result.Content)
			}
		}
		pages := scrapePages(ctx, opts, chunkFilter, urls, fallback)
		for j, i := range idx {
			ranked[i].Page = &pages[j]
		}
		if opts.Format == formatPretty || opts.Verbose {
			writeScrapeSummary(errOut, pages, opts)
		}
	}
	return writeQualified(out, ranked, opts)
}

// pageContent is a result's scraped text plus what happened to it.
type pageContent struct {
	Content string
	Err     error
	// Fallback is set when Content is the provider's excerpt because the
	// page could not be fetched.
	Fallback bool
	// Filtered is set when --filter-chunks ran on this page. FilterErr
	// records a Jev failure, in which case Content is the unfiltered text.
	Filtered    bool
	FilterErr   error
	ChunksTotal int
	ChunksKept  int
	// RawChars is the content length before chunk filtering.
	RawChars int
}

// scrapePages fetches every URL (aligned with urls), caps each page's text,
// and, when q is non-nil, keeps only the chunks Jev judges relevant. Each
// page's chunks go to Jev in a single batch request; pages run concurrently.
// fallback (aligned with urls) is the provider's excerpt, used when a fetch
// fails.
func scrapePages(ctx context.Context, opts searchOptions, q qualifier, urls, fallback []string) []pageContent {
	out := make([]pageContent, len(urls))
	if len(urls) == 0 {
		return out
	}
	pages := newScraper(opts.MaxChars).FetchAll(ctx, urls, scrape.DefaultConcurrency)
	for i, p := range pages {
		out[i] = pageContent{Content: p.Content, Err: p.Err}
		if p.Err != nil {
			// A page behind a wall still has the provider's excerpt.
			out[i].Content = scrape.Truncate(fallback[i], opts.MaxChars)
			out[i].Fallback = out[i].Content != ""
		}
		out[i].RawChars = len([]rune(out[i].Content))
	}
	if q == nil {
		return out
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, jev.DefaultConcurrency)
	for i := range out {
		if out[i].Content == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(pc *pageContent) {
			defer wg.Done()
			defer func() { <-sem }()
			filterChunks(ctx, q, opts.Query, pc)
		}(&out[i])
	}
	wg.Wait()
	return out
}

// filterChunks splits pc.Content, asks Jev about every chunk in one request,
// and reassembles the ones it says yes to.
func filterChunks(ctx context.Context, q qualifier, query string, pc *pageContent) {
	chunks := scrape.Split(pc.Content, scrape.DefaultChunkChars)
	pc.Filtered = true
	pc.ChunksTotal = len(chunks)
	answers, _, err := q.FilterChunks(ctx, query, chunks)
	if err != nil {
		pc.FilterErr = err
		pc.ChunksKept = len(chunks)
		return
	}
	kept := make([]string, 0, len(chunks))
	for i, c := range chunks {
		if i < len(answers) && answers[i] != nil && answers[i].Yes() {
			kept = append(kept, c)
		}
	}
	pc.ChunksKept = len(kept)
	pc.Content = scrape.Join(kept)
}

func writeScrapeSummary(w io.Writer, pages []pageContent, opts searchOptions) {
	if opts.Format != formatPretty && !opts.Verbose {
		return
	}
	var failed, fallback, rawChars, chars, total, kept, filterFailed int
	for _, p := range pages {
		if p.Err != nil {
			failed++
			if p.Fallback {
				fallback++
			}
		}
		rawChars += p.RawChars
		chars += len([]rune(p.Content))
		total += p.ChunksTotal
		kept += p.ChunksKept
		if p.FilterErr != nil {
			filterFailed++
		}
	}
	fmt.Fprintf(w, "scraped %d page(s)", len(pages))
	if failed > 0 {
		fmt.Fprintf(w, " (%d failed", failed)
		if fallback > 0 {
			fmt.Fprintf(w, ", %d using the provider excerpt", fallback)
		}
		fmt.Fprint(w, ")")
	}
	if opts.FilterChunks {
		fmt.Fprintf(w, "; chunks %d → %d kept; %d → %d chars", total, kept, rawChars, chars)
		if filterFailed > 0 {
			fmt.Fprintf(w, "; %d page(s) unfiltered (Jev error)", filterFailed)
		}
	} else {
		fmt.Fprintf(w, ", %d chars", chars)
	}
	fmt.Fprintln(w)
	if opts.Format == formatPretty {
		fmt.Fprintln(w)
	}
}

// shuffleChain randomizes the order of a copy of chain. Tests override it.
var shuffleChain = func(chain []string) []string {
	out := append([]string(nil), chain...)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// runSearchStage runs the provider stage for opts.Mode. It returns a label
// naming the engine(s) that answered, the results, and (in multi mode) the
// engines that returned each URL.
func runSearchStage(ctx context.Context, cfg *config.Config, opts searchOptions, errOut io.Writer) (string, []provider.SearchResult, map[string][]string, error) {
	names := opts.Providers
	if opts.Mode == modeRandom {
		names = shuffleChain(names)
	}
	c := newChain(cfg, names, errOut, opts.Verbose)
	c.Sources = opts.Sources
	results, err := c.Search(ctx, opts.Query, opts.Num)
	if err != nil {
		return "", nil, nil, err
	}
	return c.Name(), results, c.Engines(), nil
}

// Chain timing and top-up, overridable by tests.
var (
	attemptTimeout = provider.DefaultAttemptTimeout
	chainBudget    = provider.DefaultChainBudget
	chainTopUp     = true
)

// newCooldown opens the persistent cooldown store for cfg. Tests override it.
var newCooldown = func(cfg *config.Config) provider.Cooldowns {
	return provider.NewCooldown(cfg.CooldownPath, cfg.Cooldown)
}

// cooldownKey tracks keyed and keyless use of a provider separately.
func cooldownKey(cfg *config.Config, name string) string {
	if n, err := keys.Parse(provider.Normalize(name)); err == nil && cfg.Keys.Get(n) != "" {
		return name + "+key"
	}
	return name
}

// newChain wraps chain in a lazily-constructed provider.Chain that reports
// fall-throughs and cooldown skips to errOut. Skips are mentioned once an
// hour per provider unless verbose.
func newChain(cfg *config.Config, chain []string, errOut io.Writer, verbose bool) *provider.Chain {
	return &provider.Chain{
		Names:          chain,
		New:            func(name string) (provider.Provider, error) { return newProvider(cfg, name) },
		AttemptTimeout: attemptTimeout,
		Budget:         chainBudget,
		NoTopUp:        !chainTopUp,
		Cooldowns:      newCooldown(cfg),
		CooldownKey:    func(name string) string { return cooldownKey(cfg, name) },
		OnFallthrough: func(failed string, err error, next string) {
			if next == "" {
				fmt.Fprintf(errOut, "%s failed: %v\n", failed, err)
				return
			}
			fmt.Fprintf(errOut, "%s failed (%v); trying %s\n", failed, err, next)
		},
		OnSkip: func(name, reason string, announce bool) {
			if announce || verbose {
				fmt.Fprintln(errOut, reason)
			}
		},
	}
}

// searchChain tries each provider in order and returns the first successful
// search, reporting fall-throughs to errOut.
func searchChain(ctx context.Context, cfg *config.Config, chain []string, query string, num int, errOut io.Writer) (string, []provider.SearchResult, error) {
	c := newChain(cfg, chain, errOut, false)
	results, err := c.Search(ctx, query, num)
	if err != nil {
		return "", nil, err
	}
	return c.Name(), results, nil
}

// rankedResult is a qualified result plus the pipeline's keep/drop decision.
type rankedResult struct {
	jev.Qualified
	Kept bool
	// Engines lists the backends that returned this URL (multi mode only).
	Engines []string
	// Page is the scraped content (--scrape), nil when not fetched.
	Page *pageContent
}

// rank sorts qualified results by relevance (highest first) and marks which
// clear the threshold. Results Jev failed on sort last and are never kept.
func rank(qualified []jev.Qualified, minScore float64) []rankedResult {
	out := make([]rankedResult, 0, len(qualified))
	for _, q := range qualified {
		kept := q.Err == nil && (q.Score != nil || q.Noul != nil) && q.Value() >= minScore
		out = append(out, rankedResult{Qualified: q, Kept: kept})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kept != out[j].Kept {
			return out[i].Kept
		}
		return out[i].Value() > out[j].Value()
	})
	return out
}

func countKept(ranked []rankedResult) (kept, failed int) {
	for _, r := range ranked {
		if r.Kept {
			kept++
		}
		if r.Err != nil {
			failed++
		}
	}
	return kept, failed
}

func writeSummary(w io.Writer, providerName string, ranked []rankedResult, opts searchOptions, usage jev.Usage) {
	kept, failed := countKept(ranked)
	what := fmt.Sprintf("min score %g", opts.MinScore)
	if opts.Noul != "" {
		what = fmt.Sprintf("P(yes) ≥ %.2f", opts.MinScore)
	}
	fmt.Fprintf(w, "%s: %d results → %d kept (%s)", providerName, len(ranked), kept, what)
	if failed > 0 {
		fmt.Fprintf(w, "; %d not scored (Jev error)", failed)
	}
	fmt.Fprintln(w)
	if opts.Verbose {
		mode := "per-result"
		if opts.Batch {
			mode = "batch"
		}
		fmt.Fprintf(w, "jev: %s mode, %d input / %d output tokens\n", mode, usage.InputTokens, usage.OutputTokens)
	}
	if opts.Format == formatPretty {
		fmt.Fprintln(w)
	}
}

// --- Output ---------------------------------------------------------------

// outputResult is the JSON shape for a qualified result.
type outputResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`

	// Score mode.
	Score         *float64           `json:"score,omitempty"`
	MaxScore      *int               `json:"max_score,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`

	// Noul mode.
	Yes         *bool    `json:"yes,omitempty"`
	Probability *float64 `json:"probability,omitempty"`

	Kept    bool     `json:"kept"`
	Error   string   `json:"error,omitempty"`
	Engines []string `json:"engines,omitempty"`
	// Duplicates are other addresses of the same content that were folded
	// into this result.
	Duplicates []string `json:"duplicates,omitempty"`

	// --scrape fields.
	Content     string `json:"content,omitempty"`
	ScrapeError string `json:"scrape_error,omitempty"`
	// --filter-chunks fields.
	ChunksTotal *int   `json:"chunks_total,omitempty"`
	ChunksKept  *int   `json:"chunks_kept,omitempty"`
	FilterError string `json:"filter_error,omitempty"`
}

// fillPage copies scraped content into the JSON fields.
func (o *outputResult) fillPage(p *pageContent) {
	if p == nil {
		return
	}
	o.Content, o.ScrapeError, o.ChunksTotal, o.ChunksKept, o.FilterError = pageFields(p)
}

// pageFields flattens a pageContent into the shared JSON field values.
func pageFields(p *pageContent) (content, scrapeErr string, total, kept *int, filterErr string) {
	content = p.Content
	if p.Err != nil {
		scrapeErr = p.Err.Error()
		if p.Fallback {
			scrapeErr += " (content is the provider excerpt)"
		}
	}
	if p.Filtered {
		t, k := p.ChunksTotal, p.ChunksKept
		total, kept = &t, &k
	}
	if p.FilterErr != nil {
		filterErr = p.FilterErr.Error()
	}
	return content, scrapeErr, total, kept, filterErr
}

func toOutput(r rankedResult) outputResult {
	o := outputResult{
		Title:   r.Result.Title,
		URL:     r.Result.URL,
		Snippet: r.Result.Snippet,
		Kept:    r.Kept,
		Engines: r.Engines,
	}
	for _, d := range r.Duplicates {
		o.Duplicates = append(o.Duplicates, d.URL)
	}
	if r.Score != nil {
		score, conf, max := r.Score.Score, r.Score.Confidence, r.Score.MaxScore()
		o.Score, o.Confidence, o.MaxScore = &score, &conf, &max
		o.Probabilities = r.Score.Probabilities
	}
	if r.Noul != nil {
		yes, p := r.Noul.Yes(), r.Noul.Probability
		o.Yes, o.Probability = &yes, &p
	}
	if r.Err != nil {
		o.Error = r.Err.Error()
	}
	o.fillPage(r.Page)
	return o
}

// rawResult is the output shape for an unqualified result (--no-filter).
type rawResult struct {
	provider.SearchResult
	Engines []string `json:"engines,omitempty"`

	Page *pageContent `json:"-"`
	// --scrape / --filter-chunks fields, filled from Page before encoding.
	Content     string `json:"content,omitempty"`
	ScrapeError string `json:"scrape_error,omitempty"`
	ChunksTotal *int   `json:"chunks_total,omitempty"`
	ChunksKept  *int   `json:"chunks_kept,omitempty"`
	FilterError string `json:"filter_error,omitempty"`
}

func (r *rawResult) fillPage() {
	if r.Page == nil {
		return
	}
	r.Content, r.ScrapeError, r.ChunksTotal, r.ChunksKept, r.FilterError = pageFields(r.Page)
}

func toRaw(results []provider.SearchResult, engines map[string][]string) []rawResult {
	out := make([]rawResult, 0, len(results))
	for _, r := range results {
		out = append(out, rawResult{SearchResult: r, Engines: engines[r.URL], Content: r.Content})
	}
	return out
}

// writeRaw prints unqualified provider results (--no-filter).
func writeRaw(w io.Writer, results []rawResult, format outputFormat) error {
	for i := range results {
		results[i].fillPage()
	}
	switch format {
	case formatJSON:
		return writeJSON(w, results)
	case formatURLs:
		for _, r := range results {
			fmt.Fprintln(w, r.URL)
		}
		return nil
	}
	for i, r := range results {
		fmt.Fprintf(w, "[%d] %s — %s\n    %s\n", i+1, titleOf(r.SearchResult), hostOf(r.URL), r.URL)
		if len(r.Engines) > 0 {
			fmt.Fprintf(w, "    Engines: %s\n", strings.Join(r.Engines, ", "))
		}
		if r.Snippet != "" {
			fmt.Fprintf(w, "    %s\n", clipSnippet(r.Snippet, 240))
		}
		writeContent(w, r.Page)
		fmt.Fprintln(w)
	}
	return nil
}

// writeContent prints scraped page text under a separator, indented to sit
// inside the result block.
func writeContent(w io.Writer, p *pageContent) {
	if p == nil {
		return
	}
	if p.Err != nil {
		fmt.Fprintf(w, "    ! scrape failed: %v\n", p.Err)
		if !p.Fallback {
			return
		}
		fmt.Fprintln(w, "    (showing the provider's excerpt instead)")
	}
	if p.FilterErr != nil {
		fmt.Fprintf(w, "    ! chunk filter failed: %v (showing unfiltered content)\n", p.FilterErr)
	}
	switch {
	case p.Filtered && p.FilterErr == nil:
		fmt.Fprintf(w, "    --- content (%d/%d chunks kept, %d chars) ---\n", p.ChunksKept, p.ChunksTotal, len([]rune(p.Content)))
	default:
		fmt.Fprintf(w, "    --- content (%d chars) ---\n", len([]rune(p.Content)))
	}
	if p.Content == "" {
		fmt.Fprintln(w, "    (no relevant chunks)")
	}
	for _, line := range strings.Split(p.Content, "\n") {
		if line == "" {
			fmt.Fprintln(w)
			continue
		}
		fmt.Fprintf(w, "    %s\n", line)
	}
	fmt.Fprintln(w, "    --- end ---")
}

// writeQualified prints ranked results. Non-verbose output includes only kept
// results; verbose output includes everything with its keep/drop decision.
func writeQualified(w io.Writer, ranked []rankedResult, opts searchOptions) error {
	visible := ranked
	if !opts.Verbose {
		visible = visible[:0:0]
		for _, r := range ranked {
			if r.Kept {
				visible = append(visible, r)
			}
		}
	}

	switch opts.Format {
	case formatJSON:
		items := make([]outputResult, 0, len(visible))
		for _, r := range visible {
			items = append(items, toOutput(r))
		}
		return writeJSON(w, items)
	case formatURLs:
		for _, r := range visible {
			if r.Kept {
				fmt.Fprintln(w, r.Result.URL)
			}
		}
		return nil
	}

	for i, r := range visible {
		fmt.Fprintf(w, "[%d] %s — %s\n    %s\n", i+1, titleOf(r.Result), hostOf(r.Result.URL), r.Result.URL)
		switch {
		case r.Err != nil:
			fmt.Fprintf(w, "    ! Jev error: %v\n", r.Err)
		case r.Score != nil:
			fmt.Fprintf(w, "    Score: %.2f / %d  (confidence: %.2f)\n", r.Score.Score, r.Score.MaxScore(), r.Score.Confidence)
			if opts.Verbose && len(r.Score.Probabilities) > 0 {
				fmt.Fprintf(w, "    Probabilities: %s\n", r.Score.FormatProbabilities())
			}
		case r.Noul != nil:
			fmt.Fprintf(w, "    P(yes): %.2f  (confidence: %.2f)\n", r.Noul.Probability, r.Noul.Confidence())
		}
		if len(r.Engines) > 0 {
			fmt.Fprintf(w, "    Engines: %s\n", strings.Join(r.Engines, ", "))
		}
		for _, d := range r.Duplicates {
			fmt.Fprintf(w, "    Duplicate: %s\n", d.URL)
		}
		if opts.Verbose {
			switch {
			case r.Kept:
				fmt.Fprintln(w, "    ✓ Kept")
			case r.Err != nil:
				fmt.Fprintln(w, "    ✗ Filtered (not scored)")
			default:
				fmt.Fprintf(w, "    ✗ Filtered (below %g threshold)\n", opts.MinScore)
			}
		}
		if r.Result.Snippet != "" {
			limit := 240
			if opts.Verbose {
				limit = 600
			}
			fmt.Fprintf(w, "    %s\n", clipSnippet(r.Result.Snippet, limit))
		}
		writeContent(w, r.Page)
		fmt.Fprintln(w)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func titleOf(r provider.SearchResult) string {
	if t := strings.TrimSpace(r.Title); t != "" {
		return t
	}
	return "(untitled)"
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "?"
	}
	return strings.TrimPrefix(u.Host, "www.")
}

func clipSnippet(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
