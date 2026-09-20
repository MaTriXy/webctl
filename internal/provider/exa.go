package provider

import (
	"context"
	"net/http"
	"strings"
)

// ExaBaseURL is the default Exa API root.
const ExaBaseURL = "https://api.exa.ai"

// ExaMCPURL is Exa's hosted MCP server, usable without a key.
const ExaMCPURL = "https://mcp.exa.ai/mcp"

// exaMCPMaxChars bounds the text Exa returns per result in keyless mode.
const exaMCPMaxChars = 3000

// Exa searches via https://exa.ai: the REST API with a key, the hosted MCP
// server without one.
type Exa struct {
	apiKey  string
	baseURL string
	mcpURL  string
	client  *http.Client
}

// NewExa constructs an Exa provider. An empty apiKey selects the keyless MCP
// endpoint; opts.BaseURL then overrides it as <BaseURL>/mcp.
func NewExa(apiKey string, opts Options) *Exa {
	base := opts.BaseURL
	mcp := ExaMCPURL
	if base == "" {
		base = ExaBaseURL
	} else {
		mcp = strings.TrimRight(base, "/") + "/mcp"
	}
	return &Exa{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(base, "/"),
		mcpURL:  mcp,
		client:  newHTTPClient(opts, DefaultTimeout),
	}
}

// Keyless reports whether this instance uses the MCP endpoint.
func (e *Exa) Keyless() bool { return e.apiKey == "" }

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
	if e.Keyless() {
		return e.searchMCP(ctx, query, numResults)
	}
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

// searchMCP calls the web_search_exa tool. Exa answers with one text block:
// result records separated by "---" lines, each a run of "Key: value" lines
// followed by the page excerpt.
func (e *Exa) searchMCP(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	texts, err := callMCPTool(ctx, e.client, "Exa", e.mcpURL, "web_search_exa", map[string]any{
		"query":                query,
		"numResults":           clampNum(numResults, 100),
		"type":                 "auto",
		"livecrawl":            "fallback",
		"contextMaxCharacters": exaMCPMaxChars,
	})
	if err != nil {
		return nil, err
	}
	var out []SearchResult
	for _, t := range texts {
		out = append(out, parseExaText(t)...)
	}
	return out, nil
}

// exaMetaPrefixes are the labelled lines in Exa's MCP text format; the
// exact set has drifted between releases ("Published date:" vs "Published:").
var exaMetaPrefixes = []string{"Highlights:", "Published date:", "Published:", "Author:", "Score:", "ID:", "Text:"}

// parseExaText converts Exa's MCP text into results. A record starts at
// each "Title:" line; separator lines and metadata are dropped, and the
// remaining lines (highlights, prefixed "- ", or page text) form Content.
func parseExaText(text string) []SearchResult {
	var out []SearchResult
	var cur *SearchResult
	var body []string
	flush := func() {
		if cur != nil && cur.Title != "" && cur.URL != "" {
			cur.Content = strings.Join(body, "\n")
			cur.Snippet = truncate(collapseWhitespace(cur.Content), 600)
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}
lines:
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Title:"):
			flush()
			cur = &SearchResult{Title: strings.TrimSpace(strings.TrimPrefix(line, "Title:"))}
			continue lines
		case cur == nil, line == "", line == "...", strings.Trim(line, "-") == "":
			continue lines
		case strings.HasPrefix(line, "URL:"):
			cur.URL = strings.TrimSpace(strings.TrimPrefix(line, "URL:"))
			continue lines
		}
		for _, p := range exaMetaPrefixes {
			if strings.HasPrefix(line, p) {
				continue lines
			}
		}
		body = append(body, strings.TrimPrefix(line, "- "))
	}
	flush()
	return out
}

// Validate implements Provider with a one-result hello-world search.
func (e *Exa) Validate(ctx context.Context) error {
	if e.Keyless() {
		_, err := e.searchMCP(ctx, "hello world", 1)
		return err
	}
	req := exaRequest{Query: "hello world", NumResults: 1}
	return postJSON(ctx, e.client, "Exa", e.baseURL+"/search", e.headers(), req, nil)
}

func (e *Exa) headers() map[string]string {
	return map[string]string{"x-api-key": e.apiKey}
}
