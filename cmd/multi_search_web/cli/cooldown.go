package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dorkitude/multi_search_web/internal/provider"
)

func newCooldownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cooldown",
		Short: "Show providers that are being skipped after rate limits",
		Long: `A provider that answers 429 (rate limited) or 402 (quota spent) is skipped
for a growing window: cooldown.steps, one step per consecutive failure, up to
the last step. At the top of the ladder one probe request is allowed every
cooldown.probe_interval. Any success resets the provider. State is kept in
<config dir>/cooldown.json so every process on the machine honors it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			entries := provider.NewCooldown(cfg.CooldownPath, cfg.Cooldown).Entries()
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "State file: %s\nLadder: %s; probe every %s; enabled=%v\n\n", cfg.CooldownPath, stepsString(cfg.Cooldown.Steps), provider.FormatDuration(cfg.Cooldown.ProbeInterval), cfg.Cooldown.Enabled)
			if len(entries) == 0 {
				fmt.Fprintln(out, "No provider is cooling down.")
				return nil
			}
			names := make([]string, 0, len(entries))
			for n := range entries {
				names = append(names, n)
			}
			sort.Strings(names)
			now := time.Now()
			for _, n := range names {
				e := entries[n]
				state := fmt.Sprintf("until %s (%s left)", e.Until.Local().Format("Mon 15:04"), provider.FormatDuration(e.Until.Sub(now)))
				if !now.Before(e.Until) {
					state = "window elapsed; next search retries it"
				}
				fmt.Fprintf(out, "  %-14s strike %d of %d  HTTP %d  %s\n", n, e.Strikes, len(cfg.Cooldown.Steps), e.Status, state)
			}
			fmt.Fprintln(out, "\nClear with `multi_search_web cooldown clear [provider]`; tune with `multi_search_web config set cooldown.<setting>`.")
			return nil
		},
	}
	clear := &cobra.Command{
		Use:   "clear [provider]",
		Short: "Forget a provider's cooldown (all providers when none is named)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = provider.Normalize(args[0])
			}
			cleared := provider.NewCooldown(cfg.CooldownPath, cfg.Cooldown).Clear(name)
			if len(cleared) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing to clear.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Cleared %s\n", strings.Join(cleared, ", "))
			return nil
		},
	}
	cmd.AddCommand(clear)
	return cmd
}

func stepsString(steps []time.Duration) string {
	parts := make([]string, len(steps))
	for i, d := range steps {
		parts[i] = provider.FormatDuration(d)
	}
	return strings.Join(parts, " → ")
}
