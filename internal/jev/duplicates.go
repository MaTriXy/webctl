package jev

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dorkitude/webctl/internal/prompts"
)

// PairKey is the question key for the i-th candidate pair.
func PairKey(i int) string { return "pair_" + strconv.Itoa(i) }

type dupItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type dupPair struct {
	ID string `json:"id"`
	A  string `json:"a"`
	B  string `json:"b"`
}

type dupState struct {
	Query   string    `json:"query"`
	Results []dupItem `json:"results"`
	Pairs   []dupPair `json:"pairs"`
}

// DuplicatePair names two results by index into the slice passed to
// ConfirmDuplicates.
type DuplicatePair struct{ A, B int }

// ConfirmDuplicates asks Jev, in ONE batch request, whether each candidate
// pair covers the same content. The returned slice is aligned with pairs;
// entries Jev did not answer are false.
func (c *Client) ConfirmDuplicates(ctx context.Context, query string, results []SearchResult, pairs []DuplicatePair) ([]bool, Usage, error) {
	out := make([]bool, len(pairs))
	if len(pairs) == 0 {
		return out, Usage{}, nil
	}
	p, err := prompts.Load(prompts.DuplicatePair)
	if err != nil {
		return nil, Usage{}, err
	}
	state := dupState{Query: query}
	used := map[int]bool{}
	for _, pr := range pairs {
		used[pr.A], used[pr.B] = true, true
	}
	for i, r := range results {
		if used[i] {
			state.Results = append(state.Results, dupItem{ID: "r" + strconv.Itoa(i), Title: r.Title, URL: r.URL, Snippet: r.Snippet})
		}
	}
	questions := make(map[string]Question, len(pairs))
	for i, pr := range pairs {
		id := PairKey(i)
		a, b := "r"+strconv.Itoa(pr.A), "r"+strconv.Itoa(pr.B)
		state.Pairs = append(state.Pairs, dupPair{ID: id, A: a, B: b})
		desc := fmt.Sprintf("%s = %s (%s); %s = %s (%s)", a, results[pr.A].Title, results[pr.A].URL, b, results[pr.B].Title, results[pr.B].URL)
		instructions, err := p.Render(prompts.Data{Query: query, ID: id, Index: i, Snippet: desc})
		if err != nil {
			return nil, Usage{}, err
		}
		questions[id] = NoulQuestion(instructions)
	}
	resp, err := c.SystemOne(ctx, &SystemOneRequest{State: state, Questions: questions})
	if err != nil {
		return nil, Usage{}, fmt.Errorf("jev duplicate check: %w", err)
	}
	for i := range pairs {
		if raw, ok := resp.Answers[PairKey(i)]; ok {
			if ans, err := raw.AsNoul(); err == nil {
				out[i] = ans.Yes()
			}
		}
	}
	return out, resp.Usage, nil
}
