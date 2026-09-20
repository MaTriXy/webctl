package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const ddgHTML = `<!DOCTYPE html><html><body>
<div id="links" class="results">
  <div class="result results_links results_links_deep web-result result--ad">
    <div class="links_main links_deep result__body">
      <h2 class="result__title"><a rel="nofollow" class="result__a" href="https://ads.example/buy">Buy now</a></h2>
      <a class="result__snippet" href="https://ads.example/buy">Sponsored</a>
    </div>
  </div>
  <div class="result results_links results_links_deep web-result ">
    <div class="links_main links_deep result__body">
      <h2 class="result__title">
        <a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Farxiv.org%2Fabs%2F1706.03762&amp;rut=abc">Attention Is <b>All</b> You Need</a>
      </h2>
      <div class="result__extras"><a class="result__url" href="//duckduckgo.com/l/?uddg=...">arxiv.org/abs/1706.03762</a></div>
      <a class="result__snippet" href="//duckduckgo.com/l/?uddg=...">We propose  a new
        simple network architecture, the <b>Transformer</b>.</a>
    </div>
  </div>
  <div class="result results_links results_links_deep web-result ">
    <div class="links_main links_deep result__body">
      <h2 class="result__title"><a class="result__a" href="https://example.com/direct">Direct link</a></h2>
    </div>
  </div>
  <div class="result results_links results_links_deep web-result ">
    <div class="links_main links_deep result__body">
      <h2 class="result__title"><a class="result__a" href="https://example.com/direct">Duplicate</a></h2>
    </div>
  </div>
  <div class="result results_links results_links_deep web-result ">
    <div class="links_main links_deep result__body">
      <h2 class="result__title"><a class="result__a" href="javascript:void(0)">Bad scheme</a></h2>
    </div>
  </div>
</div></body></html>`

func ddgServer(t *testing.T, status int, body string) (*httptest.Server, *capture) {
	t.Helper()
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Method, c.Path, c.Headers = r.Method, r.URL.Path, r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		c.Body = map[string]any{"raw": string(raw)}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func TestDDGSearch(t *testing.T) {
	srv, c := ddgServer(t, 200, ddgHTML)
	p := NewDDG(Options{BaseURL: srv.URL})
	if p.Name() != "ddg" {
		t.Errorf("Name() = %q", p.Name())
	}

	got, err := p.Search(context.Background(), "attention is all you need", 10)
	if err != nil {
		t.Fatal(err)
	}
	if c.Method != http.MethodPost || c.Path != "/html/" {
		t.Errorf("request = %s %s, want POST /html/", c.Method, c.Path)
	}
	if ct := c.Headers.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(c.Body["raw"].(string), "q=attention+is+all+you+need") {
		t.Errorf("form body = %q", c.Body["raw"])
	}
	if ua := c.Headers.Get("User-Agent"); !strings.Contains(ua, "Mozilla") {
		t.Errorf("User-Agent = %q, want browser-like", ua)
	}

	want := []SearchResult{
		{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762", Snippet: "We propose a new simple network architecture, the Transformer."},
		{Title: "Direct link", URL: "https://example.com/direct"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDDGSearchTruncatesToNum(t *testing.T) {
	srv, _ := ddgServer(t, 200, ddgHTML)
	got, err := NewDDG(Options{BaseURL: srv.URL}).Search(context.Background(), "q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://arxiv.org/abs/1706.03762" {
		t.Errorf("got %+v", got)
	}
}

func TestDDGNoResults(t *testing.T) {
	srv, _ := ddgServer(t, 200, `<html><body><div class="no-results">No results.</div></body></html>`)
	got, err := NewDDG(Options{BaseURL: srv.URL}).Search(context.Background(), "q", 5)
	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v; want empty, nil", got, err)
	}
}

func TestDDGBotCheckIsAnError(t *testing.T) {
	srv, _ := ddgServer(t, 200, `<html><body><div class="anomaly-modal__title">Please verify you are human</div></body></html>`)
	_, err := NewDDG(Options{BaseURL: srv.URL}).Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "bot check") {
		t.Errorf("err = %v", err)
	}
}

func TestDDGHTTPError(t *testing.T) {
	srv, _ := ddgServer(t, 403, `Forbidden`)
	_, err := NewDDG(Options{BaseURL: srv.URL}).Search(context.Background(), "q", 5)
	if !IsUnauthorized(err) {
		t.Errorf("err = %v, want APIError 403", err)
	}
}

func TestDDGValidate(t *testing.T) {
	srv, _ := ddgServer(t, 200, ddgHTML)
	if err := NewDDG(Options{BaseURL: srv.URL}).Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDDGTarget(t *testing.T) {
	cases := map[string]string{
		"//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa%3Fb%3D1&rut=x": "https://example.com/a?b=1",
		"https://duckduckgo.com/l/?rut=x":                                      "",
		"//example.com/x":                                                      "https://example.com/x",
		"http://example.com/x":                                                 "http://example.com/x",
		"javascript:alert(1)":                                                  "",
		"":                                                                     "",
	}
	for in, want := range cases {
		if got := ddgTarget(in); got != want {
			t.Errorf("ddgTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewKeyless(t *testing.T) {
	for _, name := range []string{"ddg", "duckduckgo", "DDG"} {
		p, err := New(name, "", Options{})
		if err != nil || p.Name() != "ddg" {
			t.Errorf("New(%q) = %v, %v", name, p, err)
		}
		if !Keyless(name) {
			t.Errorf("Keyless(%q) = false", name)
		}
	}
	// Exa and Parallel are keyed but still usable keyless via MCP.
	if Keyless("sonar") || !Keyless("exa") || !Keyless("parallel") {
		t.Error("Keyless mismatch for sonar/exa/parallel")
	}
}

func TestDDGRetriesAcceptedAndKeepsCookies(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			http.SetCookie(w, &http.Cookie{Name: "kl", Value: "wt-wt", Path: "/"})
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if c, err := r.Cookie("kl"); err != nil || c.Value != "wt-wt" {
			t.Errorf("cookie from the 202 not sent back: %v", err)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<div class="result"><a class="result__a" href="https://a.example">A</a><a class="result__snippet">s</a></div>`)
	}))
	t.Cleanup(srv.Close)
	d := NewDDG(Options{BaseURL: srv.URL})
	d.sleep = func(context.Context, time.Duration) error { return nil }
	got, err := d.Search(context.Background(), "q", 5)
	if err != nil || len(got) != 1 || hits != 3 {
		t.Errorf("got %v, err %v, hits %d", got, err, hits)
	}

	// Persistent 202s end as an APIError, not a hang.
	hits = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	t.Cleanup(srv2.Close)
	d = NewDDG(Options{BaseURL: srv2.URL})
	d.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := d.Search(context.Background(), "q", 5); err == nil {
		t.Error("expected error after retries")
	}
}
