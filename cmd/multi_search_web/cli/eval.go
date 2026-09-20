package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dorkitude/multi_search_web/evals"
	"github.com/dorkitude/multi_search_web/internal/config"
	"github.com/dorkitude/multi_search_web/internal/jev"
	"github.com/dorkitude/multi_search_web/internal/provider"
	"github.com/dorkitude/multi_search_web/internal/scrape"
	version_ "github.com/dorkitude/multi_search_web/internal/version"
)

// newEvalJev builds the Jev client for evals. Tests override it.
var newEvalJev = func(cfg *config.Config, key string) evals.JevClient {
	c := jev.NewClient(key)
	c.BaseURL = cfg.JevBaseURL
	c.Model = cfg.JevModel
	return c
}

// runsDir is where eval runs are written: <config dir>/evals unless
// --runs-dir says otherwise. Test output lives outside the repository.
func runsDir(cfg *config.Config, flag string) string {
	if flag != "" {
		return flag
	}
	return filepath.Join(cfg.Dir, "evals")
}

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
		dir       string
		notes     string
		reuse     string
		noSave    bool
		scrapeAll bool
	)
	cmd := &cobra.Command{
		Use:   "eval [case-name ...]",
		Short: "Run search-quality evals (live provider + Jev calls)",
		Long: `Runs each case through search, then the nofilter, filter, and scrape stages,
and passes a case when its filter stage passes. Each run is saved as one JSON
file under <config dir>/evals (--runs-dir). --reuse-searches <run> judges an
earlier run's provider results again, so two runs compare like for like.
See "docs evals" and evals/README.md.`,
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
			where := runsDir(cfg, dir)

			runner := &evals.Runner{
				Jev:               newEvalJev(cfg, jevKey),
				Provider:          prov,
				Batch:             batch,
				CoverageThreshold: threshold,
				Modes:             modes,
				Parallel:          parallel,
				Version:           version_.Version,
				Audit:             true,
				ScrapeAll:         scrapeAll,
				Scraper:           &scrape.Fetcher{},
				NewProvider: func(name string) (provider.Provider, error) {
					if name == "" {
						c := newChain(cfg, chain, cmd.ErrOrStderr(), verbose)
						c.Sources = cfg.Sources
						return c, nil
					}
					return newProvider(cfg, name)
				},
			}
			if reuse != "" {
				prev, err := evals.ResolveRun(where, reuse)
				if err != nil {
					return err
				}
				runner.ReuseSearches = prev.RawSearches()
				defaultProvider = "reused from " + prev.ID
			}

			run := evals.NewRun(version_.Version, gitSHA(), defaultProvider, modes, notes)
			if !jsonOut {
				fmt.Fprintf(out, "=== multi_search_web %s eval: %d case(s), provider %s, modes %s, parallel %d ===\n\n", version_.Version, len(cases), defaultProvider, modesFlag, parallel)
			}
			var progress func(*evals.Report)
			if !jsonOut {
				progress = func(r *evals.Report) { evals.WriteReport(out, r, verbose) }
			}
			run.Reports = runner.RunAll(cmd.Context(), cases, progress)
			run.Summary = evals.Summarize(run.Reports)
			run.FinishedAt = run.StartedAt.Add(0)
			for _, r := range run.Reports {
				if end := run.StartedAt.Add(r.Duration); end.After(run.FinishedAt) {
					run.FinishedAt = end
				}
			}
			saved := ""
			if !noSave {
				if saved, err = run.Save(where); err != nil {
					return err
				}
			}

			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if err := enc.Encode(run); err != nil {
					return err
				}
			} else {
				evals.WriteSummary(out, run.Summary)
				if saved != "" {
					fmt.Fprintf(out, "saved %s\n", saved)
				}
			}

			if run.Summary.Passed != run.Summary.Total {
				return fmt.Errorf("%d of %d eval case(s) did not pass", run.Summary.Total-run.Summary.Passed, run.Summary.Total)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&casesDir, "cases", "", "directory of case YAML files (default: embedded evals/cases)")
	f.StringVarP(&prov, "provider", "p", "", "one provider for every case (default: the auto chain)")
	f.BoolVar(&batch, "batch", false, "score each case's results in one Jev request")
	f.BoolVar(&jsonOut, "json", false, "print the run as JSON")
	f.BoolVarP(&verbose, "verbose", "v", false, "show every judged score and token usage")
	f.BoolVar(&list, "list", false, "list cases and exit")
	f.Float64Var(&threshold, "coverage-threshold", evals.DefaultCoverageThreshold, "P(yes) for a theme to count as covered")
	f.StringVar(&modesFlag, "modes", "nofilter,filter,scrape", "stages to run")
	f.IntVar(&parallel, "parallel", 2, "cases to run concurrently")
	f.StringVar(&dir, "runs-dir", "", "where runs are saved (default <config dir>/evals)")
	f.StringVar(&notes, "notes", "", "note stored with the run")
	f.StringVar(&reuse, "reuse-searches", "", "judge the provider results of this run again: an id, \"latest\", \"latest:<version>\", or a file")
	f.BoolVar(&noSave, "no-save", false, "do not write a run file")
	f.BoolVar(&scrapeAll, "scrape-all", false, "run the scrape stage on every case, not only those marked scrape: true")
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
		dir      string
		version  string
		runRef   string
		perCase  bool
		compare  bool
		markdown bool
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Summarize saved runs per version and mode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			where := runsDir(cfg, dir)
			runs, err := evals.LoadRuns(where)
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				return fmt.Errorf("no runs in %s", where)
			}
			var selected []*evals.Run
			switch {
			case runRef != "":
				r, err := evals.ResolveRun(where, runRef)
				if err != nil {
					return err
				}
				selected = []*evals.Run{r}
			case version != "":
				r := evals.Latest(runs, version)
				if r == nil {
					return fmt.Errorf("no runs for version %s in %s", version, where)
				}
				selected = []*evals.Run{r}
			default:
				for _, v := range evals.Versions(runs) {
					selected = append(selected, evals.Latest(runs, v))
				}
			}
			out := cmd.OutOrStdout()
			for _, r := range selected {
				fmt.Fprintf(out, "\n## Version %s (run %s, %s, %s)\n\n", r.Version, r.ID, r.Provider, r.StartedAt.Local().Format("2006-01-02 15:04"))
				evals.WriteModeTable(out, r.Summarize(), markdown)
				if compare {
					fmt.Fprintln(out)
					evals.WriteCompareTable(out, r.Rows(), markdown)
				}
				if perCase {
					fmt.Fprintln(out)
					evals.WriteCaseTable(out, r.Rows(), markdown)
				}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&dir, "runs-dir", "", "where runs are read from (default <config dir>/evals)")
	f.StringVar(&version, "version", "", "only the latest run of this version")
	f.StringVar(&runRef, "run", "", "one run: an id, \"latest\", \"latest:<version>\", or a file")
	f.BoolVar(&perCase, "cases", false, "add a per-case, per-stage table")
	f.BoolVar(&compare, "compare", false, "add a per-case table comparing raw results with the filter's delivery")
	f.BoolVar(&markdown, "markdown", true, "Markdown tables")
	return cmd
}
