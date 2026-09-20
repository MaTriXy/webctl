package provider

import (
	"context"
	"net/http"
	"strings"
)

// KeenableBaseURL is the Keenable API root.
const KeenableBaseURL = "https://api.keenable.ai"

// Keenable searches an index built for agents. The public endpoint needs
// no key (rate limited, identified by the X-Keenable-Title header); a key
// switches to the authenticated endpoint. Results carry page text.
type Keenable struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewKeenable constructs a Keenable provider.
func NewKeenable(apiKey string, opts Options) *Keenable {
	base := opts.BaseURL
	if base == "" {
		base = KeenableBaseURL
	}
	return &Keenable{apiKey: apiKey, baseURL: strings.TrimRight(base, "/"), client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (k *Keenable) Name() string { return "keenable" }

type keenableResponse struct {
	Results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Snippet     string `json:"snippet"`
		Description string `json:"description"`
	} `json:"results"`
}

// Search implements Provider. Keenable takes no result count; the limit is
// applied locally.
func (k *Keenable) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	path, headers := "/v1/search/public", map[string]string{"X-Keenable-Title": "webctl"}
	if k.apiKey != "" {
		path = "/v1/search"
		headers["X-API-Key"] = k.apiKey
	}
	var resp keenableResponse
	if err := postJSON(ctx, k.client, "Keenable", k.baseURL+path, headers, map[string]any{"query": query, "mode": "pro"}, &resp); err != nil {
		return nil, err
	}
	limit := clampNum(numResults, 50)
	out := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		if len(out) >= limit {
			break
		}
		if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.URL) == "" {
			continue
		}
		text := strings.TrimSpace(r.Snippet)
		if text == "" {
			text = strings.TrimSpace(r.Description)
		}
		out = append(out, SearchResult{Title: collapseWhitespace(r.Title), URL: strings.TrimSpace(r.URL), Snippet: excerpt(text), Content: text})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (k *Keenable) Validate(ctx context.Context) error {
	_, err := k.Search(ctx, "hello world", 1)
	return err
}
