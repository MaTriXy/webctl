package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// ParallelBaseURL is the default Parallel API root.
const ParallelBaseURL = "https://api.parallel.ai"

// Parallel searches via https://parallel.ai.
type Parallel struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewParallel constructs a Parallel provider.
func NewParallel(apiKey string, opts Options) *Parallel {
	base := opts.BaseURL
	if base == "" {
		base = ParallelBaseURL
	}
	return &Parallel{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(base, "/"),
		client:  newHTTPClient(opts, DefaultTimeout),
	}
}

// Name implements Provider.
func (p *Parallel) Name() string { return "parallel" }

type parallelRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// parallelResult is decoded leniently: Parallel has shipped both a single
// "snippet"/"description" string and an "excerpts" array for result text.
type parallelResult struct {
	Title       string      `json:"title"`
	URL         string      `json:"url"`
	Snippet     string      `json:"snippet"`
	Description string      `json:"description"`
	Excerpts    stringSlice `json:"excerpts"`
}

type parallelResponse struct {
	Results []parallelResult `json:"results"`
}

// Search implements Provider.
func (p *Parallel) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	req := parallelRequest{Query: query, MaxResults: clampNum(numResults, 100)}
	var resp parallelResponse
	if err := postJSON(ctx, p.client, "Parallel", p.baseURL+"/v1/search", p.headers(), req, &resp); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		snippet := r.Snippet
		if snippet == "" {
			snippet = strings.Join(r.Excerpts, " ")
		}
		if snippet == "" {
			snippet = r.Description
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
func (p *Parallel) Validate(ctx context.Context) error {
	req := parallelRequest{Query: "hello world", MaxResults: 1}
	return postJSON(ctx, p.client, "Parallel", p.baseURL+"/v1/search", p.headers(), req, nil)
}

func (p *Parallel) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + p.apiKey}
}

// stringSlice unmarshals from either a JSON array of strings or a single string.
type stringSlice []string

func (s *stringSlice) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*s = nil
		return nil
	}
	if b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*s = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}
