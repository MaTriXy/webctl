package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dorkitude/smart_search/evals"
	"github.com/dorkitude/smart_search/internal/config"
	"github.com/dorkitude/smart_search/internal/jev"
	"github.com/dorkitude/smart_search/internal/provider"
)

// newEvalJev builds the Jev client for evals. Tests override it.
var newEvalJev = func(cfg *config.Config, key string) evals.JevClient {
	c := jev.NewClient(key)
	c.BaseURL = cfg.JevBaseURL
	c.Model = cfg.JevModel
	return c
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
	)
	cmd := &cobra.Command{
		Use:   "eval [case-name ...]",
		Short: "Run search-quality evals (live provider + Jev calls)",
		Long: `Runs each eval case through the full pipeline — provider search, Jev
qualification, threshold filter — then asks Jev in one batch request whether
the kept results cover the case's expected themes. Reports pass/fail per case
with Jev's confidence and exits non-zero if any case fails.

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
					fmt.Fprintf(out, "%-28s %s\n", c.Name, c.Query)
				}
				return nil
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			jevKey, err := cfg.JevKey()
			if err != nil {
				return err
			}
			defaultProvider, err := cfg.ResolveProvider(prov)
			if err != nil {
				return err
			}

			runner := &evals.Runner{
				Jev:               newEvalJev(cfg, jevKey),
				Provider:          prov,
				Batch:             batch,
				CoverageThreshold: threshold,
				NewProvider: func(name string) (provider.Provider, error) {
					if name == "" {
						name = defaultProvider
					}
					return newProvider(cfg, name)
				},
			}

			if !jsonOut {
				fmt.Fprintf(out, "=== smart_search eval: %d case(s), provider %s ===\n\n", len(cases), defaultProvider)
			}
			var progress func(*evals.Report)
			if !jsonOut {
				progress = func(r *evals.Report) { evals.WriteReport(out, r, verbose) }
			}
			reports := runner.RunAll(cmd.Context(), cases, progress)
			summary := evals.Summarize(reports)

			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if err := enc.Encode(struct {
					Reports []*evals.Report `json:"reports"`
					Summary evals.Summary   `json:"summary"`
				}{reports, summary}); err != nil {
					return err
				}
			} else {
				evals.WriteSummary(out, summary)
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
	return cmd
}
