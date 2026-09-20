// Package keys manages the on-disk API key store (~/secrets/keys.json).
package keys

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Name identifies a key slot in the store.
type Name string

const (
	Exa      Name = "exa"
	Parallel Name = "parallel"
	Sonar    Name = "sonar"
	Youcom   Name = "youcom"
	SearXNG  Name = "searxng"
	Jev      Name = "jev"
)

// SearchProviders lists the key names that correspond to search providers.
// SearXNG's slot holds an instance URL rather than a secret; DuckDuckGo
// needs nothing and so has no slot.
var SearchProviders = []Name{Exa, Parallel, Sonar, Youcom, SearXNG}

// All lists every key name, search providers first.
var All = []Name{Exa, Parallel, Sonar, Youcom, SearXNG, Jev}

// Secret reports whether the slot holds a credential that should be masked.
func (n Name) Secret() bool { return n != SearXNG }

// EnvVar returns the environment variable that overrides this key.
func (n Name) EnvVar() string {
	switch n {
	case Exa:
		return "EXA_API_KEY"
	case Parallel:
		return "PARALLEL_API_KEY"
	case Sonar:
		return "SONAR_API_KEY"
	case Youcom:
		return "YOUCOM_API_KEY"
	case SearXNG:
		return "SEARXNG_URL"
	case Jev:
		return "JEV_API_KEY"
	}
	return ""
}

// Display returns a human-friendly label for the key.
func (n Name) Display() string {
	switch n {
	case Exa:
		return "Exa"
	case Parallel:
		return "Parallel"
	case Sonar:
		return "Sonar (Perplexity)"
	case Youcom:
		return "You.com"
	case SearXNG:
		return "SearXNG URL"
	case Jev:
		return "Jev (TypeSafe)"
	}
	return string(n)
}

// URL returns where a user can obtain the key.
func (n Name) URL() string {
	switch n {
	case Exa:
		return "https://exa.ai"
	case Parallel:
		return "https://parallel.ai"
	case Sonar:
		return "https://perplexity.ai"
	case Youcom:
		return "https://you.com/platform/api-keys"
	case SearXNG:
		return "https://docs.searxng.org"
	case Jev:
		return "https://typesafe.ai"
	}
	return ""
}

// Parse converts a user-supplied string into a Name.
func Parse(s string) (Name, error) {
	for _, n := range All {
		if string(n) == s {
			return n, nil
		}
	}
	return "", fmt.Errorf("unknown key %q (expected one of exa, parallel, sonar, youcom, searxng, jev)", s)
}

// Store is the JSON shape of keys.json. Empty strings mean "not configured".
type Store struct {
	ExaAPIKey      string `json:"exa_api_key"`
	ParallelAPIKey string `json:"parallel_api_key"`
	SonarAPIKey    string `json:"sonar_api_key"`
	YoucomAPIKey   string `json:"youcom_api_key,omitempty"`
	SearXNGURL     string `json:"searxng_url"`
	JevAPIKey      string `json:"jev_api_key"`
}

// DefaultDir returns ~/multi_search_web, the config directory.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "multi_search_web"), nil
}

// DefaultPath returns ~/secrets/keys.json. The file may be shared with other
// tools; Save preserves fields it does not know.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "secrets", "keys.json"), nil
}

// Load reads the store at path. A missing file yields an empty store, not an error.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Store{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w (fix the JSON or delete the file and re-run `multi_search_web setup`)", path, err)
	}
	return &s, nil
}

// Save writes the store to path with 0600 permissions, creating the parent dir
// (0700) if needed. Fields already in the file that Store does not define are
// kept, so a keys file shared with other tools is not clobbered.
func (s *Store) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	data, err := s.merged(path)
	if err != nil {
		return err
	}

	// Write to a temp file then rename so a crash never leaves a half-written keys.json.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".keys-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}

// merged returns the JSON to write: the file's existing fields, if any,
// overlaid with this store's. Empty values are written explicitly so an
// unset key reads back as unset.
func (s *Store) merged(path string) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if existing, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(existing, &fields); err != nil {
			return nil, fmt.Errorf("parse %s: %w (fix the JSON or delete the file)", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	own, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("encode keys: %w", err)
	}
	var ownFields map[string]json.RawMessage
	if err := json.Unmarshal(own, &ownFields); err != nil {
		return nil, fmt.Errorf("encode keys: %w", err)
	}
	for k, v := range ownFields {
		fields[k] = v
	}
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode keys: %w", err)
	}
	return append(data, '\n'), nil
}

// Get returns the key for name.
func (s *Store) Get(name Name) string {
	switch name {
	case Exa:
		return s.ExaAPIKey
	case Parallel:
		return s.ParallelAPIKey
	case Sonar:
		return s.SonarAPIKey
	case Youcom:
		return s.YoucomAPIKey
	case SearXNG:
		return s.SearXNGURL
	case Jev:
		return s.JevAPIKey
	}
	return ""
}

// Set assigns the key for name.
func (s *Store) Set(name Name, value string) {
	switch name {
	case Exa:
		s.ExaAPIKey = value
	case Parallel:
		s.ParallelAPIKey = value
	case Sonar:
		s.SonarAPIKey = value
	case Youcom:
		s.YoucomAPIKey = value
	case SearXNG:
		s.SearXNGURL = value
	case Jev:
		s.JevAPIKey = value
	}
}

// Has reports whether a non-empty key is configured for name.
func (s *Store) Has(name Name) bool { return s.Get(name) != "" }

// ConfiguredProviders returns the search providers with a non-empty key.
func (s *Store) ConfiguredProviders() []Name {
	var out []Name
	for _, n := range SearchProviders {
		if s.Has(n) {
			out = append(out, n)
		}
	}
	return out
}

// Mask returns a redacted preview of a key suitable for display (e.g. "sk-1…f9c2").
func Mask(key string) string {
	if key == "" {
		return "(not set)"
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		return key // URLs are not secrets
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}
