package provider

import (
	"context"
	"strings"
	"testing"
)

const searxngJSON = `{"query":"attention","number_of_results":0,"results":[
	{"url":"https://arxiv.org/abs/1706.03762","title":" Attention Is All You Need ","content":"We propose\n\na new   architecture.","engine":"google","engines":["google","bing"]},
	{"url":"https://arxiv.org/abs/1706.03762","title":"dupe","content":"x","engine":"bing"},
	{"url":"https://example.com/b","title":"B","content":"","engine":"duckduckgo"},
	{"url":"","title":"no url","content":"skip me"}
],"suggestions":[],"unresponsive_engines":[]}`

func TestSearXNGSearch(t *testing.T) {
	srv, c := mockServer(t, 200, searxngJSON)
	p := NewSearXNG(srv.URL+"/", Options{})
	if p.Name() != "searxng" {
		t.Errorf("Name() = %q", p.Name())
	}
	got, err := p.Search(context.Background(), "attention is all", 10)
	if err != nil {
		t.Fatal(err)
	}
	if c.Method != "GET" || c.Path != "/search" {
		t.Errorf("request = %s %s", c.Method, c.Path)
	}
	want := []SearchResult{
		{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762", Snippet: "We propose a new architecture."},
		{Title: "B", URL: "https://example.com/b"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSearXNGQueryString(t *testing.T) {
	var gotQuery string
	srv, _ := mockServerFunc(t, func(q map[string][]string) (int, string) {
		gotQuery = q["q"][0] + "|" + q["format"][0]
		return 200, `{"results":[]}`
	})
	if _, err := NewSearXNG(srv.URL, Options{}).Search(context.Background(), "a b&c", 3); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "a b&c|json" {
		t.Errorf("query params = %q", gotQuery)
	}
}

func TestSearXNGTruncatesToNum(t *testing.T) {
	srv, _ := mockServer(t, 200, searxngJSON)
	got, err := NewSearXNG(srv.URL, Options{}).Search(context.Background(), "q", 1)
	if err != nil || len(got) != 1 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestSearXNGForbiddenHintsJSONFormat(t *testing.T) {
	srv, _ := mockServer(t, 403, `{"error":"forbidden"}`)
	_, err := NewSearXNG(srv.URL, Options{}).Search(context.Background(), "q", 1)
	if err == nil || !strings.Contains(err.Error(), "json format") {
		t.Errorf("err = %v", err)
	}
	if !IsUnauthorized(err) {
		t.Errorf("403 should be an APIError: %v", err)
	}
}

func TestSearXNGViaNew(t *testing.T) {
	srv, _ := mockServer(t, 200, searxngJSON)
	p, err := New("searxng", srv.URL, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := New("searxng", "", Options{}); err == nil || !strings.Contains(err.Error(), "URL is empty") {
		t.Errorf("empty URL err = %v", err)
	}
	if _, err := NewSearXNG("", Options{}).Search(context.Background(), "q", 1); err == nil {
		t.Error("empty instance URL should fail to search")
	}
}
