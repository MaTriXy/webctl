package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SerplyBaseURL is the Serply API root.
const SerplyBaseURL = "https://api.serply.io"

// Serply returns Google results through the Serply API (keyed). At most
// ten organic results per request.
type Serply struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewSerply constructs a Serply provider.
func NewSerply(apiKey string, opts Options) *Serply {
	base := opts.BaseURL
	if base == "" {
		base = SerplyBaseURL
	}
	return &Serply{apiKey: apiKey, baseURL: strings.TrimRight(base, "/"), client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (s *Serply) Name() string { return "serply" }

type serplyResponse struct {
	Results []struct {
		Title       string `json:"title"`
		Link        string `json:"link"`
		Description string `json:"description"`
	} `json:"results"`
}

// Search implements Provider.
func (s *Serply) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	q := url.Values{"q": {query}, "num": {fmt.Sprint(clampNum(numResults, 10))}}
	var resp serplyResponse
	if err := getJSON(ctx, s.client, "Serply", s.baseURL+"/v1/search/?"+q.Encode(), map[string]string{"X-Api-Key": s.apiKey}, &resp); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Link) == "" {
			continue
		}
		out = append(out, SearchResult{Title: collapseWhitespace(r.Title), URL: strings.TrimSpace(r.Link), Snippet: excerpt(r.Description)})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (s *Serply) Validate(ctx context.Context) error {
	_, err := s.Search(ctx, "hello world", 1)
	return err
}
