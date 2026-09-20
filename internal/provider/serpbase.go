package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// SerpBaseBaseURL is the SerpBase API root.
const SerpBaseBaseURL = "https://api.serpbase.dev"

// SerpBase returns Google results through the SerpBase API (keyed). It
// reports business errors in the body with HTTP 200: status 1001 bad key,
// 1020 credits exhausted, 1029 rate limited. About ten results per page.
type SerpBase struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewSerpBase constructs a SerpBase provider.
func NewSerpBase(apiKey string, opts Options) *SerpBase {
	base := opts.BaseURL
	if base == "" {
		base = SerpBaseBaseURL
	}
	return &SerpBase{apiKey: apiKey, baseURL: strings.TrimRight(base, "/"), client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (s *SerpBase) Name() string { return "serpbase" }

type serpBaseResponse struct {
	Status  int    `json:"status"`
	Error   string `json:"error"`
	Organic []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"organic"`
}

// Search implements Provider.
func (s *SerpBase) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	body := map[string]any{"q": query, "hl": "en", "gl": "us", "page": 1}
	headers := map[string]string{"X-API-Key": s.apiKey, "X-SerpBase-Source": "multi_search_web"}
	var resp serpBaseResponse
	if err := postJSON(ctx, s.client, "SerpBase", s.baseURL+"/google/search", headers, body, &resp); err != nil {
		return nil, err
	}
	switch resp.Status {
	case 0:
	case 1001:
		return nil, &APIError{Provider: "SerpBase", Status: http.StatusUnauthorized, Body: resp.Error}
	case 1020:
		return nil, &APIError{Provider: "SerpBase", Status: http.StatusPaymentRequired, Body: resp.Error}
	case 1029:
		return nil, &APIError{Provider: "SerpBase", Status: http.StatusTooManyRequests, Body: resp.Error}
	default:
		return nil, fmt.Errorf("SerpBase returned status %d: %s", resp.Status, resp.Error)
	}
	limit := clampNum(numResults, 50)
	out := make([]SearchResult, 0, len(resp.Organic))
	for _, r := range resp.Organic {
		if len(out) >= limit {
			break
		}
		if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Link) == "" {
			continue
		}
		out = append(out, SearchResult{Title: collapseWhitespace(r.Title), URL: strings.TrimSpace(r.Link), Snippet: excerpt(r.Snippet)})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (s *SerpBase) Validate(ctx context.Context) error {
	_, err := s.Search(ctx, "hello world", 1)
	return err
}
