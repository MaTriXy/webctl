// Package provider defines the search Provider interface and implementations
// for Exa, Parallel, and Sonar (Perplexity).
package provider

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SearchResult is a single, provider-agnostic web search hit.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Provider is a web search backend.
type Provider interface {
	// Name returns the short provider identifier (exa, parallel, sonar).
	Name() string
	// Search runs query and returns up to numResults results.
	Search(ctx context.Context, query string, numResults int) ([]SearchResult, error)
	// Validate performs a lightweight call to confirm the API key works.
	Validate(ctx context.Context) error
}

// Options customize provider construction. The zero value is fine.
type Options struct {
	// BaseURL overrides the provider's API root (useful for tests and proxies).
	BaseURL string
	// HTTPClient overrides the default client. A per-provider timeout is
	// applied only when this is nil.
	HTTPClient *http.Client
}

// DefaultTimeout bounds a single provider request.
const DefaultTimeout = 30 * time.Second

// SonarTimeout is longer because Sonar runs an LLM before answering.
const SonarTimeout = 90 * time.Second

// Names returns the supported provider names in display order.
func Names() []string { return []string{"exa", "parallel", "sonar"} }

// New constructs the named provider.
func New(name, apiKey string, opts Options) (Provider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("%s: API key is empty", name)
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "exa":
		return NewExa(apiKey, opts), nil
	case "parallel":
		return NewParallel(apiKey, opts), nil
	case "sonar", "perplexity":
		return NewSonar(apiKey, opts), nil
	}
	names := Names()
	sort.Strings(names)
	return nil, fmt.Errorf("unknown provider %q (expected one of %s)", name, strings.Join(names, ", "))
}

// clampNum normalizes a requested result count.
func clampNum(n, max int) int {
	if n <= 0 {
		return 10
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

// truncate shortens s to at most n runes, appending "…" when cut.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// collapseWhitespace flattens newlines and runs of spaces into single spaces.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
