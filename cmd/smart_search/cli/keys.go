package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dorkitude/smart_search/internal/config"
	"github.com/dorkitude/smart_search/internal/keys"
)

const validateTimeout = 20 * time.Second

// ErrInvalidKey is returned when a provider rejects a key as unauthorized.
var ErrInvalidKey = errors.New("key rejected by provider")

// validateKey performs a lightweight hello-world call against the provider.
func validateKey(ctx context.Context, cfg *config.Config, name keys.Name, key string) error {
	ctx, cancel := context.WithTimeout(ctx, validateTimeout)
	defer cancel()

	var (
		url     string
		headers = map[string]string{"Content-Type": "application/json"}
		body    any
	)
	switch name {
	case keys.Exa:
		url = "https://api.exa.ai/search"
		headers["x-api-key"] = key
		body = map[string]any{"query": "hello world", "numResults": 1}
	case keys.Parallel:
		url = "https://api.parallel.ai/v1/search"
		headers["Authorization"] = "Bearer " + key
		body = map[string]any{"query": "hello world", "max_results": 1}
	case keys.Sonar:
		url = "https://api.perplexity.ai/chat/completions"
		headers["Authorization"] = "Bearer " + key
		body = map[string]any{
			"model":      "sonar",
			"messages":   []map[string]string{{"role": "user", "content": "hello"}},
			"max_tokens": 1,
		}
	case keys.Jev:
		url = cfg.JevBaseURL + "/v1/systemone"
		headers["Authorization"] = "Bearer " + key
		body = map[string]any{
			"model": cfg.JevModel,
			"state": map[string]any{"text": "hello world"},
			"questions": map[string]any{
				"greeting": map[string]any{"type": "noul", "instructions": "Is the text a greeting?"},
			},
		}
	default:
		return fmt.Errorf("no validator for %q", name)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("reach %s: %w", url, err)
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w (HTTP %d)", ErrInvalidKey, resp.StatusCode)
	default:
		return fmt.Errorf("%s returned HTTP %d: %s", name.Display(), resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
}

// newKeysCmd builds the `smart_search keys` command group.
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
		Use:   "set <exa|parallel|sonar|jev>",
		Short: "Set a key (prompts with masked input unless --value is given)",
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
				val, err = keys.PromptMasked(fmt.Sprintf("%s API key: ", name.Display()))
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
			return nil
		},
	}
	set.Flags().StringVar(&setValue, "value", "", "key value (avoid in shared shells; prefer the masked prompt)")

	unset := &cobra.Command{
		Use:   "unset <exa|parallel|sonar|jev>",
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
		Use:   "validate [exa|parallel|sonar|jev]",
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
