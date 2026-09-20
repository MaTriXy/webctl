package provider

import "testing"

func TestDedupe(t *testing.T) {
	in := []SearchResult{
		{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762"},
		{Title: "Attention is all you need", URL: "https://arxiv.org/pdf/1706.03762"},
		{Title: "Attention Is All You Need | NeurIPS", URL: "https://papers.nips.cc/paper/7181"},
		{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762/"}, // same URL, trailing slash
		{Title: "Go", URL: "https://go.dev"},                                           // short titles never collapse
		{Title: "Go", URL: "https://golang.org"},
		{Title: "A different paper", URL: "https://example.com/other"},
	}
	got := Dedupe(in)
	want := []string{"https://arxiv.org/abs/1706.03762", "https://go.dev", "https://golang.org", "https://example.com/other"}
	if len(got) != len(want) {
		t.Fatalf("got %d results: %+v", len(got), got)
	}
	for i := range want {
		if got[i].URL != want[i] {
			t.Errorf("[%d] = %s, want %s", i, got[i].URL, want[i])
		}
	}
	if got := normalizeTitle("  Attention Is All You Need! — NeurIPS"); got != "attention is all you need" {
		t.Errorf("normalizeTitle = %q", got)
	}
}
