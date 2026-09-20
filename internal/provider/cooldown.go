package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CooldownConfig sets how long a provider is left alone after it rate
// limits or reports a spent quota. Strikes climb Steps; at the top a single
// probe request is allowed every ProbeInterval.
type CooldownConfig struct {
	Enabled       bool
	Steps         []time.Duration
	ProbeInterval time.Duration
	// QuotaStart is the strike a 402 ("quota spent") jumps to on first
	// sight, since a spent quota does not come back in fifteen minutes.
	QuotaStart int
}

// DefaultCooldown is the built-in ladder.
var DefaultCooldown = CooldownConfig{
	Enabled:       true,
	Steps:         []time.Duration{15 * time.Minute, time.Hour, 4 * time.Hour, 12 * time.Hour, 24 * time.Hour, 72 * time.Hour},
	ProbeInterval: 24 * time.Hour,
	QuotaStart:    3,
}

// notifyEvery is how often a skipped provider is mentioned on stderr.
const notifyEvery = time.Hour

// CooldownEntry is one provider's state.
type CooldownEntry struct {
	Since     time.Time `json:"since"`
	Until     time.Time `json:"until"`
	Strikes   int       `json:"strikes"`
	Status    int       `json:"status"`
	Error     string    `json:"error,omitempty"`
	LastProbe time.Time `json:"last_probe,omitempty"`
	Notified  time.Time `json:"notified,omitempty"`
}

// Cooldown persists provider cooldowns in a JSON file so every process on
// the machine honors them. A zero Path keeps state in memory only.
type Cooldown struct {
	Path   string
	Config CooldownConfig
	// Now is overridable by tests.
	Now func() time.Time

	mu      sync.Mutex
	entries map[string]*CooldownEntry
	loaded  bool
}

// NewCooldown returns a store at path with cfg (zero cfg means DefaultCooldown).
func NewCooldown(path string, cfg CooldownConfig) *Cooldown {
	if len(cfg.Steps) == 0 {
		cfg = DefaultCooldown
	}
	return &Cooldown{Path: path, Config: cfg}
}

func (c *Cooldown) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Cooldown) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.entries = map[string]*CooldownEntry{}
	if c.Path == "" {
		return
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &c.entries)
	// Forget entries that expired more than a day ago.
	for k, e := range c.entries {
		if c.now().Sub(e.Until) > 24*time.Hour {
			delete(c.entries, k)
		}
	}
}

func (c *Cooldown) save() error {
	if c.Path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.Path, data, 0o600)
}

// Skip reports whether name should be left alone right now. When it should,
// reason describes the state for the user, and announce says whether the
// caller should print it (once per notifyEvery). A provider at the top of
// the ladder is allowed through once per ProbeInterval as a probe.
func (c *Cooldown) Skip(name string) (skip bool, reason string, announce bool) {
	if c == nil || !c.Config.Enabled {
		return false, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	e, ok := c.entries[name]
	if !ok {
		return false, "", false
	}
	now := c.now()
	if !now.Before(e.Until) {
		return false, "", false
	}
	if e.Strikes >= len(c.Config.Steps) && now.Sub(e.LastProbe) >= c.Config.ProbeInterval {
		e.LastProbe = now
		_ = c.save()
		return false, "", false
	}
	announce = now.Sub(e.Notified) >= notifyEvery
	if announce {
		e.Notified = now
		_ = c.save()
	}
	return true, c.describe(name, e), announce
}

// FormatDuration prints a duration without trailing zero units: 15m, 1h, 1h30m.
func FormatDuration(d time.Duration) string {
	s := d.Round(time.Minute).String()
	s = strings.TrimSuffix(s, "0s")
	s = strings.TrimSuffix(s, "0m")
	if s == "" {
		return "0m"
	}
	return s
}

func (c *Cooldown) describe(name string, e *CooldownEntry) string {
	ago := FormatDuration(c.now().Sub(e.Since))
	what := "rate limited"
	if e.Status == http.StatusPaymentRequired {
		what = "quota spent"
	}
	return fmt.Sprintf("%s skipped: cooling down until %s (%s %s ago, strike %d of %d); retry now with `multi_search_web cooldown clear %s`, adjust with `multi_search_web config set cooldown.steps ...`",
		strings.TrimSuffix(name, "+key"), e.Until.Local().Format("Mon 15:04"), what, ago, e.Strikes, len(c.Config.Steps), strings.TrimSuffix(name, "+key"))
}

// Note records the outcome of a request to name: a 429 or 402 adds a
// strike and sets the next window; any other outcome clears the entry.
func (c *Cooldown) Note(name string, err error) {
	if c == nil || !c.Config.Enabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	var apiErr *APIError
	limited := errors.As(err, &apiErr) && (apiErr.Status == http.StatusTooManyRequests || apiErr.Status == http.StatusPaymentRequired)
	if !limited {
		if _, ok := c.entries[name]; ok {
			delete(c.entries, name)
			_ = c.save()
		}
		return
	}
	now := c.now()
	e, ok := c.entries[name]
	if !ok {
		e = &CooldownEntry{Since: now}
		c.entries[name] = e
	}
	e.Strikes++
	if apiErr.Status == http.StatusPaymentRequired && e.Strikes < c.Config.QuotaStart {
		e.Strikes = c.Config.QuotaStart
	}
	step := min(e.Strikes, len(c.Config.Steps)) - 1
	e.Until = now.Add(c.Config.Steps[step])
	e.Status = apiErr.Status
	e.Error = truncate(err.Error(), 160)
	e.LastProbe = now
	e.Notified = time.Time{}
	_ = c.save()
}

// Clear forgets name, or every provider when name is "". It returns the
// names that were cleared.
func (c *Cooldown) Clear(name string) []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	var cleared []string
	for k := range c.entries {
		if name == "" || k == name || k == name+"+key" {
			cleared = append(cleared, k)
			delete(c.entries, k)
		}
	}
	sort.Strings(cleared)
	if len(cleared) > 0 {
		_ = c.save()
	}
	return cleared
}

// Entries returns a copy of the current state, keyed by provider.
func (c *Cooldown) Entries() map[string]CooldownEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	out := make(map[string]CooldownEntry, len(c.entries))
	for k, e := range c.entries {
		out[k] = *e
	}
	return out
}

// Active reports whether name is currently inside a window.
func (c *Cooldown) Active(name string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	e, ok := c.entries[name]
	return ok && c.now().Before(e.Until)
}
