// Package config resolves runtime configuration for smart_search.
//
// Precedence (highest first):
//  1. command-line flags (bound by the CLI layer)
//  2. environment variables (EXA_API_KEY, JEV_API_KEY, SEARXNG_URL, SMART_SEARCH_PROVIDER, ...)
//  3. ~/smart_search/config.yaml (optional)
//  4. ~/secrets/keys.json (for API keys only; --keys-file, SMART_SEARCH_KEYS_FILE,
//     or keys_file in config.yaml point elsewhere)
//  5. built-in defaults
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/dorkitude/smart_search/internal/keys"
	"github.com/dorkitude/smart_search/internal/provider"
)

// Defaults.
const (
	DefaultProvider = "" // auto: first usable in Chain order
	DefaultNum      = 10
	DefaultMinScore = 1.0
	DefaultJevURL   = "https://api.typesafe.ai"
	DefaultJevModel = "jev-latest"
	EnvPrefix       = "SMART_SEARCH"
)

// Config is the fully-resolved configuration.
type Config struct {
	// Dir is the smart_search home directory (~/smart_search).
	Dir string
	// KeysPath is the path to keys.json.
	KeysPath string

	// Provider is the preferred search provider name, or "" for auto.
	Provider string
	// Num is the default number of results to request.
	Num int
	// MinScore is the default relevance cutoff.
	MinScore float64

	// JevBaseURL and JevModel configure the Jev client.
	JevBaseURL string
	JevModel   string

	// Keys holds the resolved API keys and the SearXNG URL (env overrides file).
	Keys *keys.Store
	// KeySource records where each key came from ("env", "file", or "").
	KeySource map[keys.Name]string
}

// Options tweak how Load behaves. Zero value uses defaults.
type Options struct {
	// Dir overrides ~/smart_search. Mostly for tests.
	Dir string
	// KeysPath overrides the keys file (default ~/secrets/keys.json).
	KeysPath string
	// Viper lets the caller supply a pre-configured instance (e.g. with flags bound).
	Viper *viper.Viper
}

// New returns a Viper instance pre-wired with smart_search defaults and env bindings.
func New() *viper.Viper {
	v := viper.New()
	v.SetDefault("provider", DefaultProvider)
	v.SetDefault("num", DefaultNum)
	v.SetDefault("min_score", DefaultMinScore)
	v.SetDefault("jev.base_url", DefaultJevURL)
	v.SetDefault("jev.model", DefaultJevModel)
	// searxng_url may also be set at the top level of config.yaml.
	v.SetDefault("searxng_url", "")
	// keys_file: SMART_SEARCH_KEYS_FILE or config.yaml; "" means keys.DefaultPath().
	v.SetDefault("keys_file", "")

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Keys use their conventional unprefixed env vars.
	for _, n := range keys.All {
		_ = v.BindEnv(keyField(n), n.EnvVar())
	}
	return v
}

func keyField(n keys.Name) string { return "keys." + string(n) }

// Load resolves configuration from all sources.
func Load(opts Options) (*Config, error) {
	dir := opts.Dir
	if dir == "" {
		var err error
		if dir, err = keys.DefaultDir(); err != nil {
			return nil, err
		}
	}

	v := opts.Viper
	if v == nil {
		v = New()
	}

	// Optional config.yaml for persistent defaults.
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", filepath.Join(dir, "config.yaml"), err)
		}
	}

	keysPath := opts.KeysPath
	if keysPath == "" {
		keysPath = strings.TrimSpace(v.GetString("keys_file"))
	}
	if keysPath == "" {
		p, err := keys.DefaultPath()
		if err != nil {
			return nil, err
		}
		keysPath = p
	}
	store, err := keys.Load(keysPath)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Dir:        dir,
		KeysPath:   keysPath,
		Provider:   strings.ToLower(strings.TrimSpace(v.GetString("provider"))),
		Num:        v.GetInt("num"),
		MinScore:   v.GetFloat64("min_score"),
		JevBaseURL: strings.TrimRight(v.GetString("jev.base_url"), "/"),
		JevModel:   v.GetString("jev.model"),
		Keys:       &keys.Store{},
		KeySource:  map[keys.Name]string{},
	}

	// Env (via viper) wins over the file.
	for _, n := range keys.All {
		if val := strings.TrimSpace(v.GetString(keyField(n))); val != "" {
			cfg.Keys.Set(n, val)
			cfg.KeySource[n] = "env"
			continue
		}
		if val := store.Get(n); val != "" {
			cfg.Keys.Set(n, val)
			cfg.KeySource[n] = "file"
		}
	}
	// searxng_url in config.yaml fills the slot when neither env nor keys.json set it.
	if !cfg.Keys.Has(keys.SearXNG) {
		if val := strings.TrimSpace(v.GetString("searxng_url")); val != "" {
			cfg.Keys.Set(keys.SearXNG, val)
			cfg.KeySource[keys.SearXNG] = "config"
		}
	}

	if cfg.Num <= 0 {
		return nil, fmt.Errorf("num must be positive, got %d", cfg.Num)
	}
	return cfg, nil
}

// ProviderKey returns the credential for the named search provider: the API
// key for keyed providers, the instance URL for searxng, and "" for ddg. Exa
// and Parallel yield "" without error when no key is set, since they work
// keyless. The error is actionable when a required credential is missing.
func (c *Config) ProviderKey(name string) (string, error) {
	name = provider.Normalize(name)
	if name == "ddg" {
		return "", nil
	}
	n, err := keys.Parse(name)
	if err != nil || n == keys.Jev {
		return "", fmt.Errorf("unknown provider %q (expected one of %s)", name, strings.Join(provider.Names(), ", "))
	}
	key := c.Keys.Get(n)
	if key == "" {
		if provider.Keyless(name) && n != keys.SearXNG {
			return "", nil
		}
		if n == keys.SearXNG {
			return "", fmt.Errorf("no SearXNG URL configured: run `smart_search setup` or set %s", n.EnvVar())
		}
		return "", fmt.Errorf("no %s API key configured: run `smart_search setup` or set %s", n.Display(), n.EnvVar())
	}
	return key, nil
}

// Usable reports whether the named provider can be constructed with the
// current configuration.
func (c *Config) Usable(name string) bool {
	_, err := c.ProviderKey(name)
	return err == nil
}

// JevKey returns the Jev API key, with an actionable error if it's missing.
func (c *Config) JevKey() (string, error) {
	key := c.Keys.Get(keys.Jev)
	if key == "" {
		return "", fmt.Errorf("no Jev API key configured: run `smart_search setup`, set %s, or pass --no-filter to skip qualification", keys.Jev.EnvVar())
	}
	return key, nil
}

// Chain returns the providers to try, in order. An explicit choice yields a
// one-element chain (validated later by ProviderKey). Otherwise the preferred
// provider from config comes first (an error if it is unusable, since the
// user asked for it), then every keyed provider with a key in Names order,
// then the keyless fallbacks: exa and parallel over their hosted MCP servers,
// ddg, and searxng when its URL is set.
func (c *Config) Chain(explicit string) ([]string, error) {
	if explicit = provider.Normalize(explicit); explicit != "" {
		return []string{explicit}, nil
	}
	var chain []string
	add := func(name string) {
		for _, have := range chain {
			if have == name {
				return
			}
		}
		chain = append(chain, name)
	}
	if pref := provider.Normalize(c.Provider); pref != "" {
		if _, err := c.ProviderKey(pref); err != nil {
			return nil, fmt.Errorf("configured provider %q is unusable: %w", pref, err)
		}
		add(pref)
	}
	for _, name := range provider.Keyed() {
		if c.Keys.Get(keys.Name(name)) != "" {
			add(name)
		}
	}
	add("exa")
	add("parallel")
	add("ddg")
	if c.Usable("searxng") {
		add("searxng")
	}
	return chain, nil
}

// ResolveProvider returns the first provider in Chain(explicit).
func (c *Config) ResolveProvider(explicit string) (string, error) {
	chain, err := c.Chain(explicit)
	if err != nil {
		return "", err
	}
	return chain[0], nil
}
