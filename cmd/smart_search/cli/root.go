// Package cli wires up the smart_search cobra commands.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dorkitude/smart_search/internal/config"
	version_ "github.com/dorkitude/smart_search/internal/version"
)

var (
	// version is set at build time via -ldflags "-X .../cli.version=v1.2.3";
	// otherwise the behavior version is reported.
	version = version_.Version

	// configDir overrides ~/smart_search when set via --config-dir.
	configDir string
	// keysFile overrides ~/secrets/keys.json when set via --keys-file.
	keysFile string

	// v is the shared viper instance; flags are bound into it per command.
	v *viper.Viper
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "smart_search [flags] <query>",
		Short: "Web search qualified by Jev's typed relevance scoring",
		Long: `smart_search queries a web search backend (Exa, Parallel, Sonar, DuckDuckGo,
or a self-hosted SearXNG) and passes each result through Jev — TypeSafe's
System One model — for typed, probabilistic relevance qualification.
Low-scoring results are dropped so you spend fewer context tokens downstream.

With no keys configured, DuckDuckGo is used and --no-filter skips Jev.
Add --scrape to fetch page text, and --filter-chunks to keep only the
relevant parts.

Get started:
  smart_search setup
  smart_search "latest advances in mechanistic interpretability"`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runSearch(cmd, args)
		},
	}
	root.PersistentFlags().StringVar(&configDir, "config-dir", "", "config directory (default ~/smart_search)")
	root.PersistentFlags().StringVar(&keysFile, "keys-file", "", "API keys file (default ~/secrets/keys.json)")

	addSearchFlags(root)
	root.AddCommand(newSetupCmd())
	root.AddCommand(newKeysCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newEvalCmd())
	return root
}

// Execute runs the CLI and prints any error to stderr.
func Execute() error {
	v = config.New()
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return err
	}
	return nil
}

// loadConfig resolves configuration, honoring --config-dir and --keys-file.
func loadConfig() (*config.Config, error) {
	return config.Load(config.Options{Dir: configDir, KeysPath: keysFile, Viper: v})
}
