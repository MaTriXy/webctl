package scrape

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

const page = `<!DOCTYPE html>
<html><head><title>Ignored title</title><style>p { color: red }</style>
<script>var x = "script text";</script></head>
<body>
<nav><a href="/">Home</a> | <a href="/about">About</a></nav>
<!-- a comment -->
<h1>Attention   Is All
You Need</h1>
<p>We propose a <b>new</b> architecture, the <i>Transformer</i>.</p>
<p>It relies&nbsp;entirely on attention.</p>
<ul><li>Item one</li><li>Item two</li></ul>
<noscript>Enable JS</noscript>
<div>Footer<br>line two</div>
<svg><text>vector text</text></svg>
</body></html>`

func TestHTMLToText(t *testing.T) {
	got, err := HTMLToText(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	want := "Home | About\n\nAttention Is All\nYou Need\n\nWe propose a new architecture, the Transformer.\n\nIt relies entirely on attention.\n\nItem one\n\nItem two\n\nFooter\nline two"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	for _, bad := range []string{"script text", "color: red", "Enable JS", "vector text", "Ignored title", "a comment"} {
		if strings.Contains(got, bad) {
			t.Errorf("output should not contain %q", bad)
		}
	}
}

func TestFetchHTMLAndTruncate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("User-Agent"), "smart_search") {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
	}))
	t.Cleanup(srv.Close)

	full, err := (&Fetcher{}).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(full, "Home | About") || !strings.HasSuffix(full, "line two") {
		t.Errorf("full = %q", full)
	}

	short, err := (&Fetcher{MaxChars: 40}).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(short) > 41 || !strings.HasSuffix(short, "…") {
		t.Errorf("short = %q (%d runes)", short, utf8.RuneCountInString(short))
	}
}

func TestFetchPlainTextAndErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/text", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "line   one\n\n\n\nline two   ")
	})
	mux.HandleFunc("/pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.4")
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/empty", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body><script>x()</script></body></html>")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f := &Fetcher{}

	if got, err := f.Fetch(context.Background(), srv.URL+"/text"); err != nil || got != "line one\n\nline two" {
		t.Errorf("text = %q, %v", got, err)
	}
	if _, err := f.Fetch(context.Background(), srv.URL+"/pdf"); err == nil || !strings.Contains(err.Error(), "unsupported content type") {
		t.Errorf("pdf err = %v", err)
	}
	if _, err := f.Fetch(context.Background(), srv.URL+"/missing"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("404 err = %v", err)
	}
	if _, err := f.Fetch(context.Background(), srv.URL+"/empty"); err == nil || !strings.Contains(err.Error(), "no text content") {
		t.Errorf("empty err = %v", err)
	}
	if _, err := f.Fetch(context.Background(), "http://127.0.0.1:1/nope"); err == nil {
		t.Error("connection failure should error")
	}
}

func TestFetchAllAlignedWithErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<p>"+r.URL.Path+"</p>")
	}))
	t.Cleanup(srv.Close)
	urls := []string{srv.URL + "/a", srv.URL + "/bad", srv.URL + "/c"}
	pages := (&Fetcher{}).FetchAll(context.Background(), urls, 2)
	if len(pages) != 3 || pages[0].Content != "/a" || pages[2].Content != "/c" || pages[1].Err == nil || pages[1].URL != urls[1] {
		t.Errorf("pages = %+v", pages)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Errorf("no-op truncate = %q", got)
	}
	text := "para one is here.\n\npara two is here and longer.\n\npara three."
	got := Truncate(text, 50)
	if got != "para one is here.\n\npara two is here and longer.…" {
		t.Errorf("paragraph-boundary truncate = %q", got)
	}
	got = Truncate("word "+strings.Repeat("x", 100), 20)
	if got != "word "+strings.Repeat("x", 15)+"…" && got != "word…" {
		t.Errorf("hard truncate = %q", got)
	}
	// Multibyte runes are not split.
	got = Truncate(strings.Repeat("é", 50), 10)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 11 {
		t.Errorf("rune truncate = %q", got)
	}
}
