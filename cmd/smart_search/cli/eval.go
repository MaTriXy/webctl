package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dorkitude/smart_search/evals"
	"github.com/dorkitude/smart_search/internal/config"
	"github.com/dorkitude/smart_search/internal/jev"
	"github.com/dorkitude/smart_search/internal/provider"
	"github.com/dorkitude/smart_search/internal/scrape"
	version_ "github.com/dorkitude/smart_search/internal/version"
)

// newEvalJev builds the Jev client for evals. Tests override it.
var newEvalJev = func(cfg *config.Config, key string) evals.JevClient {
	c := jev.NewClient(key)
	c.BaseURL = cfg.JevBaseURL
	c.Model = cfg.JevModel
	return c
}

// defaultEvalDB is where eval runs are stored unless --db says otherwise.
const defaultEvalDB = "evals/results.db"

func newEvalCmd() *cobra.Command {
	var (
		casesDir  string
		prov      string
		batch     bool
		jsonOut   bool
		verbose   bool
		list      bool
		threshold float64
		modesFlag string
		parallel  int
		dbPath    string
		notes     string
		fresh     bool
	)
	cmd := &cobra.Command{
		Use:   "eval [case-name ...]",
		Short: "Run search-quality evals (live provider + Jev calls)",
		Long: `Runs each eval case through one provider search and then three stages:

  nofilter  the raw results, as they would reach a context window without Jev
  filter    Jev qualification and threshold (the default pipeline)
  scrape    for cases marked scrape: fetch kept pages, keep Jev-approved chunks

Each stage records what it would deliver (results, characters, junk-domain
hits) and asks Jev in one batch request whether that delivery covers the
case's expected themes. A case passes when its filter stage passes. Every
stage is stored in a SQLite database tagged with the smart_search version.
Provider results are cached in that database for 24h so repeated runs
judge identical inputs; --fresh searches again.

Cases are YAML files embedded from evals/cases/, or a directory given with
--cases. See evals/README.md for the case format.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cases, err := evals.EmbeddedCases()
			if casesDir != "" {
				cases, err = evals.LoadCasesDir(casesDir)
			}
			if err != nil {
				return err
			}
			cases, err = evals.Filter(cases, args)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if list {
				for _, c := range cases {
					fmt.Fprintf(out, "%-28s %-32s %s\n", c.Name, strings.Join(c.Tags, ","), c.Query)
				}
				return nil
			}
			modes, err := evals.ParseModes(modesFlag)
			if err != nil {
				return err
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			jevKey, err := cfg.JevKey()
			if err != nil {
				return err
			}
			chain, err := cfg.Chain(prov)
			if err != nil {
				return err
			}
			defaultProvider := "auto(" + strings.Join(chain, ",") + ")"
			if prov != "" {
				defaultProvider = chain[0]
			}

			runner := &evals.Runner{
				Jev:               newEvalJev(cfg, jevKey),
				Provider:          prov,
				Batch:             batch,
				CoverageThreshold: threshold,
				Modes:             modes,
				Parallel:          parallel,
				Version:           version_.Version,
				Audit:             true,
				Scraper:           &scrape.Fetcher{},
				NewProvider: func(name string) (provider.Provider, error) {
					if name == "" {
						return newChain(cfg, chain, cmd.ErrOrStderr()), nil
					}
					return newProvider(cfg, name)
				},
			}

			ctx := cmd.Context()
			var db *evals.DB
			var runID int64
			if dbPath != "" {
				if db, err = evals.OpenDB(dbPath); err != nil {
					return err
				}
				defer db.Close()
				runID, err = db.StartRun(ctx, evals.RunInfo{Version: version_.Version, GitSHA: gitSHA(), Provider: defaultProvider, Modes: modes, Notes: notes})
				if err != nil {
					return err
				}
				runner.SearchCache = db
				runner.Fresh = fresh
			}

			if !jsonOut {
				fmt.Fprintf(out, "=== smart_search %s eval: %d case(s), provider %s, modes %s, parallel %d ===\n\n", version_.Version, len(cases), defaultProvider, modesFlag, parallel)
			}
			progress := func(r *evals.Report) {
				if !jsonOut {
					evals.WriteReport(out, r, verbose)
				}
				if db != nil {
					if err := db.RecordReport(ctx, runID, r); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "db: %v\n", err)
					}
				}
			}
			reports := runner.RunAll(ctx, cases, progress)
			summary := evals.Summarize(reports)
			if db != nil {
				if err := db.FinishRun(ctx, runID, summary); err != nil {
					return err
				}
			}

			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if err := enc.Encode(struct {
					Version string          `json:"version"`
					RunID   int64           `json:"run_id,omitempty"`
					Reports []*evals.Report `json:"reports"`
					Summary evals.Summary   `json:"summary"`
				}{version_.Version, runID, reports, summary}); err != nil {
					return err
				}
			} else {
				evals.WriteSummary(out, summary)
				if db != nil {
					fmt.Fprintf(out, "stored as run %d in %s\n", runID, dbPath)
				}
			}

			if summary.Passed != summary.Total {
				return fmt.Errorf("%d of %d eval case(s) did not pass", summary.Total-summary.Passed, summary.Total)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&casesDir, "cases", "", "directory of case YAML files (default: cases embedded from evals/cases)")
	f.StringVarP(&prov, "provider", "p", "", "search provider for every case (overrides per-case provider)")
	f.BoolVar(&batch, "batch", false, "force Jev batch mode for qualification")
	f.BoolVar(&jsonOut, "json", false, "emit reports and summary as JSON")
	f.BoolVarP(&verbose, "verbose", "v", false, "show kept URLs and Jev token usage per case")
	f.BoolVar(&list, "list", false, "list cases and exit without running them")
	f.Float64Var(&threshold, "coverage-threshold", evals.DefaultCoverageThreshold, "P(yes) needed to count a theme as covered")
	f.StringVar(&modesFlag, "modes", "nofilter,filter,scrape", "comma-separated stages to run")
	f.IntVar(&parallel, "parallel", 2, "cases to run concurrently (keyless search tiers throttle above this)")
	f.StringVar(&dbPath, "db", defaultEvalDB, "SQLite file to store results in (empty to skip)")
	f.StringVar(&notes, "notes", "", "free-text note stored on the run")
	f.BoolVar(&fresh, "fresh", false, "ignore cached provider results (default: results under 24h old from the db are reused so runs judge identical inputs)")
	cmd.AddCommand(newEvalReportCmd())
	return cmd
}

// gitSHA returns the short HEAD commit, or "" outside a repository.
func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func newEvalReportCmd() *cobra.Command {
	var (
		dbPath   string
		version  string
		perCase  bool
		compare  bool
		markdown bool
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Summarize stored eval runs per version and mode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := evals.OpenDB(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			ctx := context.Background()
			versions := []string{version}
			if version == "" {
				if versions, err = db.Versions(ctx); err != nil {
					return err
				}
			}
			out := cmd.OutOrStdout()
			for _, v := range versions {
				runID, err := db.LatestRunID(ctx, v)
				if err != nil {
					return err
				}
				if runID == 0 {
					fmt.Fprintf(out, "no runs for version %s\n", v)
					continue
				}
				sums, err := db.SummarizeRun(ctx, runID)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "\n## Version %s (run %d)\n\n", v, runID)
				evals.WriteModeTable(out, sums, markdown)
				if perCase || compare {
					rows, err := db.RunRows(ctx, runID)
					if err != nil {
						return err
					}
					if compare {
						fmt.Fprintln(out)
						evals.WriteCompareTable(out, rows, markdown)
					}
					if perCase {
						fmt.Fprintln(out)
						evals.WriteCaseTable(out, rows, markdown)
					}
				}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&dbPath, "db", defaultEvalDB, "SQLite file to read")
	f.StringVar(&version, "version", "", "only this version (default: every version, latest run each)")
	f.BoolVar(&perCase, "cases", false, "include a per-case, per-stage table")
	f.BoolVar(&compare, "compare", false, "include a per-case table comparing raw results with the filter's delivery")
	f.BoolVar(&markdown, "markdown", true, "emit Markdown tables")
	return cmd
}
