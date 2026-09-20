package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// DDGBaseURL is the DuckDuckGo HTML (no-JavaScript) endpoint root.
const DDGBaseURL = "https://html.duckduckgo.com"

// ddgUserAgent looks like a browser; DuckDuckGo serves the HTML endpoint to
// browsers and answers bare library UAs with a bot check.
const ddgUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// DDG scrapes DuckDuckGo's HTML endpoint. It needs no API key, so it is the
// fallback of last resort in the auto provider chain.
type DDG struct {
	baseURL string
	client  *http.Client
}

// NewDDG constructs a DuckDuckGo provider.
func NewDDG(opts Options) *DDG {
	base := opts.BaseURL
	if base == "" {
		base = DDGBaseURL
	}
	return &DDG{
		baseURL: strings.TrimRight(base, "/"),
		client:  newHTTPClient(opts, DefaultTimeout),
	}
}

// Name implements Provider.
func (d *DDG) Name() string { return "ddg" }

// Search implements Provider by POSTing the query as a form and parsing the
// result list out of the returned HTML.
func (d *DDG) Search(ctx context.Context, query string, numResults int) ([]SearchResult, error) {
	n := clampNum(numResults, 50)
	form := url.Values{"q": {query}, "b": {""}, "kl": {""}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/html/", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("DuckDuckGo: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", ddgUserAgent)

	resp, err := d.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("DuckDuckGo: request timed out: %w", err)
		}
		return nil, fmt.Errorf("DuckDuckGo: request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &APIError{Provider: "DuckDuckGo", Status: resp.StatusCode, Body: summarizeBody(snippet)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("DuckDuckGo: read response: %w", err)
	}
	results, noResults, err := parseDDG(body)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 && !noResults {
		return nil, errors.New("DuckDuckGo: no results found in response (possible bot check; try again later)")
	}
	if len(results) > n {
		results = results[:n]
	}
	return results, nil
}

// Validate implements Provider with a one-word search.
func (d *DDG) Validate(ctx context.Context) error {
	_, err := d.Search(ctx, "hello", 1)
	return err
}

// parseDDG extracts results from the HTML endpoint's markup. Each hit is a
// ".result" container holding a ".result__a" link and a ".result__snippet".
// noResults reports whether the page explicitly said there were no matches.
func parseDDG(body []byte) (results []SearchResult, noResults bool, err error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, false, fmt.Errorf("DuckDuckGo: parse HTML: %w", err)
	}
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			classes := classList(n)
			switch {
			case classes["no-results"]:
				noResults = true
			case classes["result"] && !classes["result--ad"]:
				link := findClass(n, "result__a")
				if link == nil {
					return
				}
				u := ddgTarget(attr(link, "href"))
				if u == "" || seen[u] {
					return
				}
				seen[u] = true
				r := SearchResult{Title: collapseWhitespace(textOf(link)), URL: u}
				if sn := findClass(n, "result__snippet"); sn != nil {
					r.Snippet = truncate(collapseWhitespace(textOf(sn)), 600)
				}
				results = append(results, r)
				return // don't descend into a result twice
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results, noResults, nil
}

// ddgTarget unwraps DuckDuckGo's redirect links ("//duckduckgo.com/l/?uddg=<url>")
// and normalizes scheme-relative hrefs.
func ddgTarget(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if strings.HasSuffix(u.Host, "duckduckgo.com") && u.Path == "/l/" {
		if target := u.Query().Get("uddg"); target != "" {
			return target
		}
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return href
}

// --- small HTML helpers shared with other scrapers ------------------------

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func classList(n *html.Node) map[string]bool {
	out := map[string]bool{}
	for _, c := range strings.Fields(attr(n, "class")) {
		out[c] = true
	}
	return out
}

// findClass returns the first descendant of n carrying the CSS class.
func findClass(n *html.Node, class string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && classList(c)[class] {
			return c
		}
		if found := findClass(c, class); found != nil {
			return found
		}
	}
	return nil
}

// textOf concatenates the text nodes under n.
func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
