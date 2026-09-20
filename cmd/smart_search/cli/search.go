package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dorkitude/smart_search/internal/config"
	"github.com/dorkitude/smart_search/internal/jev"
	"github.com/dorkitude/smart_search/internal/provider"
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
}

var sf searchFlags

func addSearchFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&sf.provider, "provider", "p", "", "search provider: exa, parallel, sonar, ddg, or searxng (default: auto — keyed providers with a key, then ddg, then searxng)")
	f.IntVarP(&sf.num, "num", "n", 0, "number of results to request from the provider (default 10)")
	f.Float64VarP(&sf.minScore, "min-score", "m", -1, "minimum Jev relevance score to keep a result (default 1.0; with --noul, minimum P(yes), default 0.5)")
	f.BoolVar(&sf.jsonOut, "json", false, "emit results as JSON")
	f.BoolVar(&sf.urlsOnly, "urls-only", false, "print only result URLs, one per line")
	f.BoolVar(&sf.noFilter, "no-filter", false, "skip Jev qualification and print raw provider results")
	f.BoolVarP(&sf.verbose, "verbose", "v", false, "show Jev confidence, probabilities, and filtered results")
	f.BoolVar(&sf.batch, "batch", false, "score all results in a single Jev request")
	f.StringVar(&sf.rubric, "rubric", "", "comma-separated score criteria, lowest to highest (overrides the default rubric)")
	f.StringVar(&sf.noul, "noul", "", "ask Jev a yes/no question about each result instead of scoring")
}

// qualifier is the slice of *jev.Client the search pipeline depends on.
// It exists so tests can substitute a fake without an HTTP server.
type qualifier interface {
	Qualify(ctx context.Context, query string, results []provider.SearchResult, opts jev.QualifyOptions) ([]jev.Qualified, jev.Usage, error)
}

// Construction hooks. Tests override these to inject fakes.
var (
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
	// Providers is the ordered chain to try; the first that succeeds wins.
	Providers []string
	Num       int
	MinScore  float64
	NoFilter  bool
	Batch     bool
	Verbose   bool
	Rubric    []string
	Noul      string
	Format    outputFormat
}

type outputFormat int

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

	opts := searchOptions{
		Query:    query,
		Num:      cfg.Num,
		MinScore: cfg.MinScore,
		NoFilter: f.noFilter,
		Batch:    f.batch,
		Verbose:  f.verbose,
		Noul:     strings.TrimSpace(f.noul),
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
		if max := float64(len(rubric) - 1); opts.MinScore > max {
			return searchOptions{}, fmt.Errorf("--min-score %.2f exceeds the rubric's top score of %g", opts.MinScore, max)
		}
	}

	chain, err := cfg.Chain(f.provider)
	if err != nil {
		return searchOptions{}, err
	}
	opts.Providers = chain
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
	if !opts.NoFilter {
		var err error
		if jevKey, err = cfg.JevKey(); err != nil {
			return err
		}
	}

	p, results, err := searchChain(ctx, cfg, opts.Providers, opts.Query, opts.Num, errOut)
	if err != nil {
		return err
	}

	if opts.NoFilter {
		if len(results) == 0 && opts.Format == formatPretty {
			fmt.Fprintf(errOut, "%s returned no results.\n", p.Name())
		}
		return writeRaw(out, results, opts.Format)
	}

	if len(results) == 0 {
		if opts.Format == formatPretty {
			fmt.Fprintf(errOut, "%s returned no results.\n", p.Name())
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

	ranked := rank(qualified, opts.MinScore)
	if opts.Format == formatPretty || opts.Verbose {
		writeSummary(errOut, p.Name(), ranked, opts, usage)
	}
	return writeQualified(out, ranked, opts)
}

// searchChain tries each provider in order and returns the first successful
// search. Failures are reported to errOut as the chain falls through; if every
// provider fails, the joined errors are returned.
func searchChain(ctx context.Context, cfg *config.Config, chain []string, query string, num int, errOut io.Writer) (provider.Provider, []provider.SearchResult, error) {
	var errs []error
	for i, name := range chain {
		p, err := newProvider(cfg, name)
		if err == nil {
			var results []provider.SearchResult
			if results, err = p.Search(ctx, query, num); err == nil {
				return p, results, nil
			}
		}
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
		if i+1 < len(chain) {
			fmt.Fprintf(errOut, "%s failed (%v); trying %s\n", name, err, chain[i+1])
		}
	}
	if len(errs) == 1 {
		return nil, nil, errs[0]
	}
	return nil, nil, fmt.Errorf("all %d providers failed: %w", len(errs), errors.Join(errs...))
}

// rankedResult is a qualified result plus the pipeline's keep/drop decision.
type rankedResult struct {
	jev.Qualified
	Kept bool
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

	Kept  bool   `json:"kept"`
	Error string `json:"error,omitempty"`
}

func toOutput(r rankedResult) outputResult {
	o := outputResult{
		Title:   r.Result.Title,
		URL:     r.Result.URL,
		Snippet: r.Result.Snippet,
		Kept:    r.Kept,
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
	return o
}

// writeRaw prints unqualified provider results (--no-filter).
func writeRaw(w io.Writer, results []provider.SearchResult, format outputFormat) error {
	switch format {
	case formatJSON:
		if results == nil {
			results = []provider.SearchResult{}
		}
		return writeJSON(w, results)
	case formatURLs:
		for _, r := range results {
			fmt.Fprintln(w, r.URL)
		}
		return nil
	}
	for i, r := range results {
		fmt.Fprintf(w, "[%d] %s — %s\n    %s\n", i+1, titleOf(r), hostOf(r.URL), r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(w, "    %s\n", clipSnippet(r.Snippet, 240))
		}
		fmt.Fprintln(w)
	}
	return nil
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
