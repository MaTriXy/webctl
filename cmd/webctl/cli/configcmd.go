package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/dorkitude/webctl/internal/config"
	"github.com/dorkitude/webctl/internal/keys"
	"github.com/dorkitude/webctl/internal/provider"
)

// setting describes one key of config.yaml.
type setting struct {
	key   string
	help  string
	check func(string) (any, error)
}

// settings lists what `config set` accepts. Keys stay in a separate file
// (`webctl keys`); this is everything else.
var settings = []setting{
	{"provider", "default search provider (empty = auto chain)", func(v string) (any, error) {
		v = provider.Normalize(v)
		if v == "" {
			return "", nil
		}
		for _, n := range provider.Names() {
			if n == v {
				return v, nil
			}
		}
		return nil, fmt.Errorf("unknown provider %q (expected one of %s)", v, strings.Join(provider.Names(), ", "))
	}},
	{"num", "default number of results to request", func(v string) (any, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("num must be a positive integer, got %q", v)
		}
		return n, nil
	}},
	{"sources", "providers to query per search and fuse (1 = plain fallback chain)", func(v string) (any, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("sources must be a positive integer, got %q", v)
		}
		return n, nil
	}},
	{"min_score", "default relevance cutoff on the 0–10 scale", func(v string) (any, error) {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 10 {
			return nil, fmt.Errorf("min_score must be a number between 0 and 10, got %q", v)
		}
		return f, nil
	}},
	{"min_results", "backfill the kept set to at least this many from the best of the rest (0 = off)", func(v string) (any, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("min_results must be a non-negative integer, got %q", v)
		}
		return n, nil
	}},
	{"jev.base_url", "Jev API root", nonEmpty},
	{"jev.model", "Jev model name", nonEmpty},
	{"searxng_url", "SearXNG instance URL (keys.json's searxng_url wins when set)", func(v string) (any, error) { return strings.TrimRight(strings.TrimSpace(v), "/"), nil }},
	{"keys_file", "path of the keys file (default ~/secrets/keys.json)", func(v string) (any, error) { return strings.TrimSpace(v), nil }},
	{"cooldown.enabled", "skip providers after a 429/402 (true/false)", func(v string) (any, error) {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("cooldown.enabled must be true or false, got %q", v)
		}
		return b, nil
	}},
	{"cooldown.steps", "comma-separated windows per consecutive failure, e.g. 15m,1h,4h,12h,24h,72h", func(v string) (any, error) {
		var out []string
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if d, err := time.ParseDuration(s); err != nil || d <= 0 {
				return nil, fmt.Errorf("cooldown.steps: %q is not a positive duration", s)
			}
			out = append(out, s)
		}
		return out, nil
	}},
	{"cooldown.probe_interval", "at the top of the ladder, allow one probe request this often (e.g. 24h)", func(v string) (any, error) {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err != nil || d <= 0 {
			return nil, fmt.Errorf("cooldown.probe_interval: %q is not a positive duration", v)
		}
		return strings.TrimSpace(v), nil
	}},
	{"cooldown.quota_start", "strike a 402 (quota spent) starts at", func(v string) (any, error) {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("cooldown.quota_start must be a positive integer, got %q", v)
		}
		return n, nil
	}},
}

func nonEmpty(v string) (any, error) {
	if strings.TrimSpace(v) == "" {
		return nil, errors.New("value must not be empty")
	}
	return strings.TrimSpace(v), nil
}

func findSetting(key string) (setting, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, s := range settings {
		if s.key == key {
			return s, nil
		}
	}
	names := make([]string, len(settings))
	for i, s := range settings {
		names[i] = s.key
	}
	return setting{}, fmt.Errorf("unknown setting %q (expected one of %s)", key, strings.Join(names, ", "))
}

// configPath is ~/webctl/config.yaml, or --config-dir's.
func configPath() (string, error) {
	dir := configDir
	if dir == "" {
		var err error
		if dir, err = keys.DefaultDir(); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// readConfigFile loads config.yaml as nested maps; a missing file is empty.
func readConfigFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

func writeConfigFile(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// nested get/set/delete for dotted keys such as jev.model.
func getPath(m map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	for i, p := range parts {
		v, ok := m[p]
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return v, true
		}
		m, ok = v.(map[string]any)
		if !ok {
			return nil, false
		}
	}
	return nil, false
}

func setPath(m map[string]any, key string, val any) {
	parts := strings.Split(key, ".")
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = val
}

func deletePath(m map[string]any, key string) bool {
	parts := strings.Split(key, ".")
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			return false
		}
		m = next
	}
	last := parts[len(parts)-1]
	if _, ok := m[last]; !ok {
		return false
	}
	delete(m, last)
	return true
}

// effective returns the resolved value of a setting and where it came from.
func effective(cfg *config.Config, file map[string]any, key string) (any, string) {
	env := config.EnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if v, ok := os.LookupEnv(env); ok && v != "" {
		return v, "env " + env
	}
	if v, ok := getPath(file, key); ok {
		return v, "config.yaml"
	}
	switch key {
	case "provider":
		return cfg.Provider, "default"
	case "num":
		return cfg.Num, "default"
	case "sources":
		return cfg.Sources, "default"
	case "min_score":
		return cfg.MinScore, "default"
	case "min_results":
		return cfg.MinResults, "default"
	case "jev.base_url":
		return cfg.JevBaseURL, "default"
	case "jev.model":
		return cfg.JevModel, "default"
	case "keys_file":
		return cfg.KeysPath, "default"
	case "cooldown.enabled":
		return cfg.Cooldown.Enabled, "default"
	case "cooldown.steps":
		return stepsString(cfg.Cooldown.Steps), "default"
	case "cooldown.probe_interval":
		return provider.FormatDuration(cfg.Cooldown.ProbeInterval), "default"
	case "cooldown.quota_start":
		return cfg.Cooldown.QuotaStart, "default"
	case "searxng_url":
		if cfg.KeySource[keys.SearXNG] != "" {
			return cfg.Keys.Get(keys.SearXNG), cfg.KeySource[keys.SearXNG]
		}
		return "", "default"
	}
	return nil, ""
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show, get, set, or unset settings in config.yaml",
		Long: `Precedence: flag, WEBCTL_* env, config.yaml, default. "config list"
names every setting; "docs config" explains them. Keys live elsewhere: see "keys".`,
	}

	show := &cobra.Command{
		Use:   "show",
		Short: "Print every setting's effective value and where it comes from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			path, err := configPath()
			if err != nil {
				return err
			}
			file, err := readConfigFile(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config file: %s\n\n", path)
			for _, s := range settings {
				v, src := effective(cfg, file, s.key)
				fmt.Fprintf(cmd.OutOrStdout(), "  %-14s %-40v %s\n", s.key, v, src)
			}
			return nil
		},
	}

	get := &cobra.Command{
		Use:   "get <setting>",
		Short: "Print one setting's effective value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := findSetting(args[0])
			if err != nil {
				return err
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			path, err := configPath()
			if err != nil {
				return err
			}
			file, err := readConfigFile(path)
			if err != nil {
				return err
			}
			v, _ := effective(cfg, file, s.key)
			fmt.Fprintln(cmd.OutOrStdout(), v)
			return nil
		},
	}

	set := &cobra.Command{
		Use:   "set <setting> <value>",
		Short: "Write one setting to config.yaml",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := findSetting(args[0])
			if err != nil {
				return err
			}
			val, err := s.check(args[1])
			if err != nil {
				return err
			}
			path, err := configPath()
			if err != nil {
				return err
			}
			file, err := readConfigFile(path)
			if err != nil {
				return err
			}
			setPath(file, s.key, val)
			if err := writeConfigFile(path, file); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %v in %s\n", s.key, val, path)
			return nil
		},
	}

	unset := &cobra.Command{
		Use:   "unset <setting>",
		Short: "Remove one setting from config.yaml so the default applies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := findSetting(args[0])
			if err != nil {
				return err
			}
			path, err := configPath()
			if err != nil {
				return err
			}
			file, err := readConfigFile(path)
			if err != nil {
				return err
			}
			if !deletePath(file, s.key) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s was not set in %s\n", s.key, path)
				return nil
			}
			if err := writeConfigFile(path, file); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s from %s\n", s.key, path)
			return nil
		},
	}

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the settings config set accepts",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			names := append([]setting(nil), settings...)
			sort.Slice(names, func(i, j int) bool { return names[i].key < names[j].key })
			for _, s := range names {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-14s %s\n", s.key, s.help)
			}
		},
	}

	cmd.AddCommand(show, get, set, unset, pathCmd, list)
	return cmd
}
