package dedupe

import (
	"context"
	"strings"
	"testing"

	"github.com/dorkitude/webctl/internal/provider"
)

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://www.Example.com/a/b/?utm_source=x&id=2#frag": "example.com/a/b?id=2",
		"http://m.example.com/a/b":                            "example.com/a/b",
		"https://amp.example.com/a/b/amp":                     "example.com/a/b",
		"not a url":                                           "not a url",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

const article = "Transformers rely entirely on an attention mechanism to draw global dependencies between input and output. " +
	"The Transformer allows for significantly more parallelization and can reach a new state of the art in translation quality " +
	"after being trained for as little as twelve hours on eight P100 GPUs. We show that the Transformer generalizes well to other tasks."

func TestCandidatesAndGroups(t *testing.T) {
	rs := []provider.SearchResult{
		{Title: "Attention Is All You Need", URL: "https://arxiv.org/abs/1706.03762", Content: article},
		{Title: "Attention Is All You Need (PDF)", URL: "https://proceedings.neurips.cc/paper/7181.pdf", Content: "Abstract. " + article + " Extra footer text from the proceedings site."},
		{Title: "Attention is all you need", URL: "https://www.arxiv.org/abs/1706.03762/?utm_source=t", Content: article}, // same URL after normalization
		{Title: "A totally different page about cooking pasta", URL: "https://food.example/pasta", Content: strings.Repeat("Boil the water, salt it well, add the pasta and stir occasionally until al dente. ", 4)},
		{Title: "Short", URL: "https://short.example/", Snippet: "too short to hash"},
	}
	pairs := Candidates(rs)
	if len(pairs) == 0 {
		t.Fatal("expected a candidate pair between the abstract and the PDF")
	}
	for _, p := range pairs {
		if p.A == 0 && p.B == 2 || p.A == 2 && p.B == 0 {
			t.Error("same-URL results must not be proposed")
		}
		if p.A == 3 || p.B == 3 || p.A == 4 || p.B == 4 {
			t.Errorf("unrelated or short results proposed: %+v", p)
		}
	}
	if pairs[0].A != 0 || pairs[0].B != 1 || pairs[0].Estimated < 0.35 {
		t.Errorf("top pair = %+v", pairs[0])
	}
	groups := Groups(len(rs), pairs, []bool{true})
	if len(groups) != 1 || len(groups[0]) != 2 || groups[0][0] != 0 || groups[0][1] != 1 {
		t.Errorf("groups = %v", groups)
	}
	if g := Groups(len(rs), pairs, []bool{false}); len(g) != 0 {
		t.Errorf("unconfirmed pairs must not group: %v", g)
	}
}

type yesConfirmer struct{ got []Pair }

func (y *yesConfirmer) ConfirmDuplicates(_ context.Context, _, _ string, _ []provider.SearchResult, pairs []Pair) ([]bool, error) {
	y.got = pairs
	out := make([]bool, len(pairs))
	for i := range out {
		out[i] = true
	}
	return out, nil
}

func TestRunWithAndWithoutConfirmer(t *testing.T) {
	rs := []provider.SearchResult{
		{Title: "A", URL: "https://a.example/1", Content: article},
		{Title: "B", URL: "https://b.example/1", Content: article + " plus a sentence."},
	}
	yc := &yesConfirmer{}
	groups, pairs, err := Run(context.Background(), yc, "q", "", rs)
	if err != nil || len(groups) != 1 || len(yc.got) != len(pairs) || len(pairs) != 1 {
		t.Errorf("run = %v %v %v", groups, pairs, err)
	}
	groups, _, err = Run(context.Background(), nil, "q", "", rs)
	if err != nil || len(groups) != 1 {
		t.Errorf("without confirmer, a near-identical pair should still group: %v %v", groups, err)
	}
}
