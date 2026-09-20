package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Degoog searches a self-hosted degoog instance
// (https://github.com/degoog-org/degoog) through its GET /api/search JSON
// route. Like SearXNG it is a metasearch you run yourself; the credential
// slot holds the instance URL.
type Degoog struct {
	baseURL string
	client  *http.Client
}

// NewDegoog constructs a Degoog provider for the instance at baseURL.
func NewDegoog(baseURL string, opts Options) *Degoog {
	if opts.BaseURL != "" {
		baseURL = opts.BaseURL
	}
	return &Degoog{baseURL: strings.TrimRight(baseURL, "/"), client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (d *Degoog) Name() string { return "degoog" }

type degoogResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	} `json:"results"`
}

// Search implements Provider. Degoog takes no result count; the limit is
// applied locally.
func (d *Degoog) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	if d.baseURL == "" {
		return nil, errors.New("degoog: instance URL is empty")
	}
	var resp degoogResponse
	if err := getJSON(ctx, d.client, "Degoog", d.baseURL+"/api/search?q="+url.QueryEscape(query), nil, &resp); err != nil {
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
		out = append(out, SearchResult{Title: collapseWhitespace(r.Title), URL: strings.TrimSpace(r.URL), Snippet: excerpt(r.Snippet)})
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (d *Degoog) Validate(ctx context.Context) error {
	_, err := d.Search(ctx, "hello", 1)
	return err
}
