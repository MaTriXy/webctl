package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// BraveBaseURL is the Brave Search API root.
const BraveBaseURL = "https://api.search.brave.com"

// Brave searches via the Brave Search API (keyed; $5 monthly credit, then
// $5 per 1,000). Up to 20 results per request.
type Brave struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewBrave constructs a Brave provider.
func NewBrave(apiKey string, opts Options) *Brave {
	base := opts.BaseURL
	if base == "" {
		base = BraveBaseURL
	}
	return &Brave{apiKey: apiKey, baseURL: strings.TrimRight(base, "/"), client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (b *Brave) Name() string { return "brave" }

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// Search implements Provider.
func (b *Brave) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	q := url.Values{"q": {query}, "count": {fmt.Sprint(clampNum(numResults, 20))}, "text_decorations": {"false"}, "result_filter": {"web"}}
	var resp braveResponse
	if err := getJSON(ctx, b.client, "Brave", b.baseURL+"/res/v1/web/search?"+q.Encode(), map[string]string{"X-Subscription-Token": b.apiKey}, &resp); err != nil {
		// Brave reports a bad or missing subscription token as 422.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity && strings.Contains(strings.ToUpper(apiErr.Body), "TOKEN") {
			apiErr.Status = http.StatusUnauthorized
		}
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Web.Results))
	for _, r := range resp.Web.Results {
		if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.URL) == "" {
			continue
		}
		out = append(out, SearchResult{Title: collapseWhitespace(r.Title), URL: strings.TrimSpace(r.URL), Snippet: excerpt(r.Description)})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (b *Brave) Validate(ctx context.Context) error {
	_, err := b.Search(ctx, "hello world", 1)
	return err
}
