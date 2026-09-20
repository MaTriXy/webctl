package provider

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

type stubProvider struct {
	name    string
	results []SearchResult
	err     error
}

func (s stubProvider) Name() string                   { return s.name }
func (s stubProvider) Validate(context.Context) error { return nil }
func (s stubProvider) Search(context.Context, string, int) ([]SearchResult, error) {
	return s.results, s.err
}

func rrf(ranks ...int) float64 {
	var s float64
	for _, r := range ranks {
		s += 1 / float64(RRFK+r)
	}
	return s
}

func TestFuseRRF(t *testing.T) {
	a := SearchResult{Title: "A", URL: "https://a.example/", Snippet: "a"}
	b := SearchResult{Title: "B", URL: "https://b.example/x", Snippet: "b"}
	c := SearchResult{Title: "C", URL: "https://c.example/", Snippet: "c"}
	lists := []Ranked{
		{Engine: "exa", Results: []SearchResult{a, b, c}},
		{Engine: "ddg", Results: []SearchResult{
			{Title: "", URL: "http://www.B.example/x#frag", Snippet: ""}, // same as b
			{Title: "C2", URL: "https://c.example", Snippet: "c2"},       // same as c
		}},
	}
	got := Fuse(lists, 0, 0)
	if len(got) != 3 {
		t.Fatalf("got %d fused results: %+v", len(got), got)
	}
	// b: rank 2 in exa, rank 1 in ddg → 1/62 + 1/61; c: 1/63 + 1/62; a: 1/61.
	want := []struct {
		url     string
		score   float64
		engines string
		title   string
	}{
		{"https://b.example/x", rrf(2, 1), "exa,ddg", "B"},
		{"https://c.example/", rrf(3, 2), "exa,ddg", "C"},
		{"https://a.example/", rrf(1), "exa", "A"},
	}
	for i, w := range want {
		g := got[i]
		if g.URL != w.url || math.Abs(g.Score-w.score) > 1e-12 || strings.Join(g.Engines, ",") != w.engines || g.Title != w.title {
			t.Errorf("[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestFuseFillsGapsTiesAndLimit(t *testing.T) {
	lists := []Ranked{
		{Engine: "e1", Results: []SearchResult{{URL: "https://x/1"}, {URL: "https://x/2", Title: "T2"}}},
		{Engine: "e2", Results: []SearchResult{{URL: "https://x/2", Snippet: "S2"}, {URL: "https://x/1", Title: "T1", Snippet: "S1"}}},
		{Engine: "e3", Results: []SearchResult{{URL: ""}, {URL: "https://x/3"}}},
	}
	got := Fuse(lists, 60, 2)
	if len(got) != 2 {
		t.Fatalf("limit not applied: %+v", got)
	}
	// x/1 and x/2 tie exactly (ranks 1+2 each); stable order keeps x/1 first.
	if got[0].URL != "https://x/1" || got[0].Title != "T1" || got[0].Snippet != "S1" {
		t.Errorf("[0] = %+v", got[0])
	}
	if got[1].URL != "https://x/2" || got[1].Title != "T2" || got[1].Snippet != "S2" {
		t.Errorf("[1] = %+v", got[1])
	}
	if got[0].Score != got[1].Score {
		t.Errorf("expected a tie, got %v vs %v", got[0].Score, got[1].Score)
	}
}

func TestFuseCustomK(t *testing.T) {
	got := Fuse([]Ranked{{Engine: "e", Results: []SearchResult{{URL: "https://x/"}}}}, 10, 0)
	if len(got) != 1 || math.Abs(got[0].Score-1.0/11) > 1e-12 {
		t.Errorf("got %+v", got)
	}
}

func TestSearchAll(t *testing.T) {
	provs := []Provider{
		stubProvider{name: "exa", results: []SearchResult{{URL: "https://a/"}}},
		stubProvider{name: "ddg", err: errors.New("blocked")},
		stubProvider{name: "searxng", results: nil},
	}
	lists, errs := SearchAll(context.Background(), provs, "q", 5)
	if len(lists) != 2 || lists[0].Engine != "exa" || lists[1].Engine != "searxng" {
		t.Errorf("lists = %+v", lists)
	}
	if len(errs) != 1 || errs["ddg"] == nil || errs["ddg"].Error() != "blocked" {
		t.Errorf("errs = %v", errs)
	}
}

func TestCanonicalURL(t *testing.T) {
	cases := map[string]string{
		"HTTPS://WWW.Example.com/a/#top": "example.com/a",
		"http://example.com/a/":          "example.com/a",
		"https://example.com":            "example.com",
		"https://example.com/p?q=1":      "example.com/p?q=1",
		"not a url/":                     "not a url",
	}
	for in, want := range cases {
		if got := canonicalURL(in); got != want {
			t.Errorf("canonicalURL(%q) = %q, want %q", in, got, want)
		}
	}
}
