package provider

import (
	"context"
	"net/http"
	"strings"
)

// ExaBaseURL is the default Exa API root.
const ExaBaseURL = "https://api.exa.ai"

// Exa searches via https://exa.ai.
type Exa struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewExa constructs an Exa provider.
func NewExa(apiKey string, opts Options) *Exa {
	base := opts.BaseURL
	if base == "" {
		base = ExaBaseURL
	}
	return &Exa{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(base, "/"),
		client:  newHTTPClient(opts, DefaultTimeout),
	}
}

// Name implements Provider.
func (e *Exa) Name() string { return "exa" }

type exaRequest struct {
	Query      string       `json:"query"`
	NumResults int          `json:"numResults"`
	Contents   *exaContents `json:"contents,omitempty"`
}

type exaContents struct {
	Highlights *exaHighlights `json:"highlights,omitempty"`
}

type exaHighlights struct {
	NumSentences     int `json:"numSentences,omitempty"`
	HighlightsPerURL int `json:"highlightsPerUrl,omitempty"`
}

type exaResponse struct {
	Results []struct {
		Title      string   `json:"title"`
		URL        string   `json:"url"`
		Highlights []string `json:"highlights"`
		Text       string   `json:"text"`
		Summary    string   `json:"summary"`
	} `json:"results"`
}

// Search implements Provider.
func (e *Exa) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	req := exaRequest{
		Query:      query,
		NumResults: clampNum(numResults, 100),
		// Ask for highlights so each result carries a snippet for Jev to judge.
		Contents: &exaContents{Highlights: &exaHighlights{NumSentences: 3, HighlightsPerURL: 1}},
	}
	var resp exaResponse
	if err := postJSON(ctx, e.client, "Exa", e.baseURL+"/search", e.headers(), req, &resp); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		snippet := strings.Join(r.Highlights, " ")
		if snippet == "" {
			snippet = r.Summary
		}
		if snippet == "" {
			snippet = r.Text
		}
		out = append(out, SearchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: truncate(collapseWhitespace(snippet), 600),
		})
	}
	return out, nil
}

// Validate implements Provider with a one-result hello-world search.
func (e *Exa) Validate(ctx context.Context) error {
	req := exaRequest{Query: "hello world", NumResults: 1}
	return postJSON(ctx, e.client, "Exa", e.baseURL+"/search", e.headers(), req, nil)
}

func (e *Exa) headers() map[string]string {
	return map[string]string{"x-api-key": e.apiKey}
}
