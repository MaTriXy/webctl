package cli

import (
	"errors"

	"github.com/spf13/cobra"
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
	f.StringVarP(&sf.provider, "provider", "p", "", "search provider: exa, parallel, or sonar (default: first configured)")
	f.IntVarP(&sf.num, "num", "n", 0, "number of results to request from the provider (default 10)")
	f.Float64VarP(&sf.minScore, "min-score", "m", -1, "minimum Jev relevance score to keep a result (default 1.0)")
	f.BoolVar(&sf.jsonOut, "json", false, "emit results as JSON")
	f.BoolVar(&sf.urlsOnly, "urls-only", false, "print only result URLs, one per line")
	f.BoolVar(&sf.noFilter, "no-filter", false, "skip Jev qualification and print raw provider results")
	f.BoolVarP(&sf.verbose, "verbose", "v", false, "show Jev confidence, probabilities, and filtered results")
	f.BoolVar(&sf.batch, "batch", false, "score all results in a single Jev request")
	f.StringVar(&sf.rubric, "rubric", "", "comma-separated score criteria, lowest to highest (overrides the default rubric)")
	f.StringVar(&sf.noul, "noul", "", "ask Jev a yes/no question about each result instead of scoring")
}

func runSearch(cmd *cobra.Command, args []string) error {
	return errors.New("search is not implemented yet")
}
