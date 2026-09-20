// Package cli wires up the multi_search_web cobra commands.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/dorkitude/multi_search_web/internal/config"
	version_ "github.com/dorkitude/multi_search_web/internal/version"
)

var (
	// version is set at build time via -ldflags "-X .../cli.version=v1.2.3";
	// otherwise the behavior version is reported.
	version = version_.Version

	// configDir overrides ~/multi_search_web when set via --config-dir.
	configDir string
	// keysFile overrides ~/secrets/keys.json when set via --keys-file.
	keysFile string

	// v is the shared viper instance; flags are bound into it per command.
	v *viper.Viper
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "multi_search_web [flags] <query>",
		Short: "Web search qualified by Jev's typed relevance scoring",
		Long: `Searches several web providers at once, folds duplicates, and has Jev
(TypeSafe's System One model) score every result for topic and source
quality, so only results worth reading reach your context window.

Required: a Jev key (multi_search_web setup). Nothing else: search runs
through ketch (github.com/1broseidon/ketch) and DuckDuckGo, or your own
SearXNG. Paid providers join only when you set a key. Throttled providers
back off (see "cooldown").

Help text is short by design. The full reference is compiled in:
  multi_search_web docs            topics
  multi_search_web docs <topic>    one page (search, providers, config, ...)
  multi_search_web docs all        everything`,
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
	root.PersistentFlags().StringVar(&configDir, "config-dir", "", "config directory (default ~/multi_search_web)")
	root.PersistentFlags().StringVar(&keysFile, "keys-file", "", "keys file (default ~/secrets/keys.json)")

	addSearchFlags(root)
	root.AddCommand(newSetupCmd())
	root.AddCommand(newKeysCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newCooldownCmd())
	root.AddCommand(newDocsCmd())
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
