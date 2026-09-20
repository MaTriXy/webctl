package provider

import (
	"context"
	"net/http"
	"strings"
)

// TavilyBaseURL is the Tavily API root.
const TavilyBaseURL = "https://api.tavily.com"

// Tavily searches via Tavily's agent-oriented API (keyed; 1,000 credits a
// month free, a basic search is one credit). Results carry extracted text.
type Tavily struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewTavily constructs a Tavily provider.
func NewTavily(apiKey string, opts Options) *Tavily {
	base := opts.BaseURL
	if base == "" {
		base = TavilyBaseURL
	}
	return &Tavily{apiKey: apiKey, baseURL: strings.TrimRight(base, "/"), client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (t *Tavily) Name() string { return "tavily" }

type tavilyResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

// Search implements Provider.
func (t *Tavily) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	body := map[string]any{"query": query, "max_results": clampNum(numResults, 20), "search_depth": "basic"}
	var resp tavilyResponse
	if err := postJSON(ctx, t.client, "Tavily", t.baseURL+"/search", map[string]string{"Authorization": "Bearer " + t.apiKey}, body, &resp); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.URL) == "" {
			continue
		}
		content := strings.TrimSpace(r.Content)
		out = append(out, SearchResult{Title: collapseWhitespace(r.Title), URL: strings.TrimSpace(r.URL), Snippet: excerpt(content), Content: content})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (t *Tavily) Validate(ctx context.Context) error {
	_, err := t.Search(ctx, "hello world", 1)
	return err
}
