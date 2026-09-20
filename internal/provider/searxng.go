package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SearXNG searches a self-hosted SearXNG metasearch instance. No API key is
// needed, only the instance URL (the instance must enable the JSON format).
type SearXNG struct {
	baseURL string
	client  *http.Client
}

// NewSearXNG constructs a SearXNG provider for the instance at instanceURL.
func NewSearXNG(instanceURL string, opts Options) *SearXNG {
	base := opts.BaseURL
	if base == "" {
		base = instanceURL
	}
	return &SearXNG{
		baseURL: strings.TrimRight(strings.TrimSpace(base), "/"),
		client:  newHTTPClient(opts, DefaultTimeout),
	}
}

// Name implements Provider.
func (s *SearXNG) Name() string { return "searxng" }

type searxngResponse struct {
	Results []struct {
		Title   string   `json:"title"`
		URL     string   `json:"url"`
		Content string   `json:"content"`
		Engine  string   `json:"engine"`
		Engines []string `json:"engines"`
	} `json:"results"`
}

// Search implements Provider with GET <url>/search?q=<query>&format=json.
func (s *SearXNG) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	if s.baseURL == "" {
		return nil, errors.New("SearXNG: instance URL is empty")
	}
	n := clampNum(numResults, 100)
	q := url.Values{"q": {query}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/search?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("SearXNG: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "smart_search/1.0 (+https://github.com/dorkitude/smart_search)")

	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("SearXNG: request timed out: %w", err)
		}
		return nil, fmt.Errorf("SearXNG: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		apiErr := &APIError{Provider: "SearXNG", Status: resp.StatusCode, Body: summarizeBody(snippet)}
		if resp.StatusCode == http.StatusForbidden {
			apiErr.Body = strings.TrimSpace(apiErr.Body + " (enable the json format in the instance's search.formats setting)")
		}
		return nil, apiErr
	}

	var out searxngResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("SearXNG: decode response: %w", err)
	}
	seen := map[string]bool{}
	results := make([]SearchResult, 0, len(out.Results))
	for _, r := range out.Results {
		u := strings.TrimSpace(r.URL)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		results = append(results, SearchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     u,
			Snippet: truncate(collapseWhitespace(r.Content), 600),
		})
		if len(results) >= n {
			break
		}
	}
	return results, nil
}

// Validate implements Provider with a one-word search.
func (s *SearXNG) Validate(ctx context.Context) error {
	_, err := s.Search(ctx, "hello", 1)
	return err
}
