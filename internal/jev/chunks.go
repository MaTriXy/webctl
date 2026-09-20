package jev

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/dorkitude/multi_search_web/internal/prompts"
)

// ChunkKey returns the question key used for the i-th chunk in FilterChunks.
func ChunkKey(i int) string { return "chunk_" + strconv.Itoa(i) }

type chunkItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type chunkState struct {
	Query  string      `json:"query"`
	Chunks []chunkItem `json:"chunks"`
}

// FilterChunks asks Jev, in ONE batch request, whether each chunk of a page
// is relevant to query. The returned slice is aligned with chunks; entries
// Jev did not answer are nil.
func (c *Client) FilterChunks(ctx context.Context, query string, chunks []string) ([]*NoulAnswer, Usage, error) {
	out := make([]*NoulAnswer, len(chunks))
	if len(chunks) == 0 {
		return out, Usage{}, nil
	}
	if query == "" {
		return nil, Usage{}, errors.New("jev: query is empty")
	}
	p, err := prompts.Load(prompts.ChunkRelevance)
	if err != nil {
		return nil, Usage{}, err
	}
	state := chunkState{Query: query, Chunks: make([]chunkItem, 0, len(chunks))}
	questions := make(map[string]Question, len(chunks))
	for i, text := range chunks {
		id := ChunkKey(i)
		state.Chunks = append(state.Chunks, chunkItem{ID: id, Text: text})
		instructions, err := p.Render(prompts.Data{Query: query, Chunk: text, ID: id, Index: i})
		if err != nil {
			return nil, Usage{}, err
		}
		questions[id] = NoulQuestion(instructions)
	}
	resp, err := c.SystemOne(ctx, &SystemOneRequest{State: state, Questions: questions})
	if err != nil {
		return nil, Usage{}, fmt.Errorf("jev chunk filter: %w", err)
	}
	for i := range chunks {
		if raw, ok := resp.Answers[ChunkKey(i)]; ok {
			if ans, err := raw.AsNoul(); err == nil {
				out[i] = ans
			}
		}
	}
	return out, resp.Usage, nil
}
