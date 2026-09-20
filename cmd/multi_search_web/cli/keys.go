package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dorkitude/multi_search_web/internal/config"
	"github.com/dorkitude/multi_search_web/internal/jev"
	"github.com/dorkitude/multi_search_web/internal/keys"
	"github.com/dorkitude/multi_search_web/internal/provider"
)

const validateTimeout = 20 * time.Second

// ErrInvalidKey is returned when a provider rejects a key as unauthorized.
var ErrInvalidKey = errors.New("key rejected by provider")

// validateKey performs a lightweight hello-world call against the provider.
func validateKey(ctx context.Context, cfg *config.Config, name keys.Name, key string) error {
	ctx, cancel := context.WithTimeout(ctx, validateTimeout)
	defer cancel()

	if name != keys.Jev {
		p, err := provider.New(string(name), key, provider.Options{})
		if err != nil {
			return err
		}
		if err := p.Validate(ctx); err != nil {
			var apiErr *provider.APIError
			if errors.As(err, &apiErr) && apiErr.Unauthorized() {
				return fmt.Errorf("%w (HTTP %d: %s)", ErrInvalidKey, apiErr.Status, apiErr.Body)
			}
			return err
		}
		return nil
	}

	return validateJevKey(ctx, cfg, key)
}

// validateJevKey asks Jev a trivial noul question to confirm the key works.
func validateJevKey(ctx context.Context, cfg *config.Config, key string) error {
	c := jev.NewClient(key)
	c.BaseURL = cfg.JevBaseURL
	c.Model = cfg.JevModel
	c.NoRetry = true
	if err := c.Validate(ctx); err != nil {
		var apiErr *jev.APIError
		if errors.As(err, &apiErr) && apiErr.Unauthorized() {
			return fmt.Errorf("%w (HTTP %d: %s)", ErrInvalidKey, apiErr.Status, apiErr.Body)
		}
		return err
	}
	return nil
}

// newKeysCmd builds the `multi_search_web keys` command group.
func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Inspect and manage API keys non-interactively",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "Show which keys are configured and where they come from",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Keys file: %s\n\n", cfg.KeysPath)
			for _, n := range keys.All {
				src := cfg.KeySource[n]
				if src == "" {
					src = "-"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-20s %-14s %s\n", n.Display(), keys.Mask(cfg.Keys.Get(n)), src)
			}
			return nil
		},
	}

	var setValue string
	set := &cobra.Command{
		Use:   "set <provider|jev>",
		Short: "Set a key or the SearXNG URL (prompts with masked input unless --value is given)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := keys.Parse(args[0])
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			val := setValue
			if val == "" {
				if name.Secret() {
					val, err = keys.PromptMasked(fmt.Sprintf("%s API key: ", name.Display()))
				} else {
					val, err = keys.PromptLine(fmt.Sprintf("%s: ", name.Display()))
					val = strings.TrimSpace(val)
				}
				if err != nil {
					return err
				}
			}
			if val == "" {
				return errors.New("empty key; nothing saved")
			}
			store, err := keys.Load(cfg.KeysPath)
			if err != nil {
				return err
			}
			store.Set(name, val)
			if err := store.Save(cfg.KeysPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %s key to %s\n", name.Display(), cfg.KeysPath)
			if cleared := provider.NewCooldown(cfg.CooldownPath, cfg.Cooldown).Clear(string(name)); len(cleared) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Cleared cooldown for %s\n", strings.Join(cleared, ", "))
			}
			return nil
		},
	}
	set.Flags().StringVar(&setValue, "value", "", "key value (avoid in shared shells; prefer the masked prompt)")

	unset := &cobra.Command{
		Use:   "unset <exa|parallel|sonar|searxng|jev>",
		Short: "Remove a key from keys.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := keys.Parse(args[0])
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			store, err := keys.Load(cfg.KeysPath)
			if err != nil {
				return err
			}
			store.Set(name, "")
			if err := store.Save(cfg.KeysPath); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s key from %s\n", name.Display(), cfg.KeysPath)
			return nil
		},
	}

	validate := &cobra.Command{
		Use:   "validate [exa|parallel|sonar|searxng|jev]",
		Short: "Validate configured keys with a lightweight API call",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			targets := keys.All
			if len(args) == 1 {
				n, err := keys.Parse(args[0])
				if err != nil {
					return err
				}
				targets = []keys.Name{n}
			}
			var failed int
			for _, n := range targets {
				key := cfg.Keys.Get(n)
				if key == "" {
					if len(args) == 1 {
						return fmt.Errorf("no %s key configured", n.Display())
					}
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-20s ", n.Display())
				if err := validateKey(cmd.Context(), cfg, n, key); err != nil {
					failed++
					fmt.Fprintf(cmd.OutOrStdout(), "✗ %v\n", err)
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), "✓ valid")
			}
			if failed > 0 {
				return fmt.Errorf("%d key(s) failed validation", failed)
			}
			return nil
		},
	}

	cmd.AddCommand(list, set, unset, validate)
	return cmd
}

// stderr is a small helper for wizard output that shouldn't pollute stdout.
func stderr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}
