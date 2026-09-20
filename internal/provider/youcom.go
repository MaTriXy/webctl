package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// YoucomMCPURL is You.com's hosted MCP server; the free profile needs no key.
const YoucomMCPURL = "https://api.you.com/mcp"

// Youcom searches via You.com's hosted MCP server: the free profile without
// a key, the authenticated server (Bearer header) with one.
type Youcom struct {
	apiKey string
	mcpURL string
	client *http.Client
}

// NewYoucom constructs a You.com provider. opts.BaseURL overrides the MCP
// endpoint as <BaseURL>/mcp.
func NewYoucom(apiKey string, opts Options) *Youcom {
	mcp := YoucomMCPURL
	if opts.BaseURL != "" {
		mcp = strings.TrimRight(opts.BaseURL, "/") + "/mcp"
	}
	return &Youcom{apiKey: apiKey, mcpURL: mcp, client: newHTTPClient(opts, DefaultTimeout)}
}

// Name implements Provider.
func (y *Youcom) Name() string { return "youcom" }

// Keyless reports whether this instance uses the free profile.
func (y *Youcom) Keyless() bool { return y.apiKey == "" }

type youcomPayload struct {
	Results struct {
		Web []struct {
			URL         string   `json:"url"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Snippets    []string `json:"snippets"`
		} `json:"web"`
	} `json:"results"`
}

// Search implements Provider with the you-search tool.
func (y *Youcom) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	limit := clampNum(numResults, 20)
	endpoint := y.mcpURL
	var headers map[string]string
	if y.Keyless() {
		endpoint += "?profile=free"
	} else {
		headers = map[string]string{"Authorization": "Bearer " + y.apiKey}
	}
	texts, err := callMCPToolWithHeaders(ctx, y.client, "You.com", endpoint, "you-search", map[string]any{
		"query":      query,
		"count":      limit,
		"extraction": "none",
	}, headers)
	if err != nil {
		return nil, err
	}
	var out []SearchResult
	for _, t := range texts {
		var payload youcomPayload
		if err := json.Unmarshal([]byte(t), &payload); err != nil {
			return nil, fmt.Errorf("You.com: decode results: %w", err)
		}
		for _, r := range payload.Results.Web {
			if len(out) >= limit {
				return out, nil
			}
			title, url := strings.TrimSpace(r.Title), strings.TrimSpace(r.URL)
			if title == "" || url == "" {
				continue
			}
			content := strings.TrimSpace(strings.Join(r.Snippets, "\n"))
			// The description is a line or two; the snippets carry the substance.
			snippet := strings.TrimSpace(r.Description + "\n" + content)
			out = append(out, SearchResult{Title: collapseWhitespace(title), URL: url, Snippet: excerpt(snippet), Content: content})
		}
	}
	return out, nil
}

// Validate implements Provider with a one-result search.
func (y *Youcom) Validate(ctx context.Context) error {
	_, err := y.Search(ctx, "hello world", 1)
	return err
}
