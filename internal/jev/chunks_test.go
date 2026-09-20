package jev

import (
	"context"
	"strings"
	"testing"
)

func TestFilterChunksOneBatchRequest(t *testing.T) {
	js := newJevServer(t, func(req SystemOneRequest) (int, any) {
		answers := map[string]Answer{}
		for key := range req.Questions {
			p := 0.9
			if key == ChunkKey(1) {
				p = 0.1
			}
			answers[key] = Answer{Type: TypeNoul, Noul: &p}
		}
		// chunk_2 is deliberately unanswered.
		delete(answers, ChunkKey(2))
		return 200, SystemOneResponse{Answers: answers, Usage: Usage{InputTokens: 300, OutputTokens: 3}}
	})
	chunks := []string{"Transformers use attention.", "Subscribe to our newsletter!", "Multi-head attention…"}
	got, usage, err := js.client().FilterChunks(context.Background(), Ask{Query: "how do transformers work"}, chunks)
	if err != nil {
		t.Fatal(err)
	}
	if js.calls.Load() != 1 {
		t.Errorf("expected exactly one request, got %d", js.calls.Load())
	}
	if len(got) != 3 || got[0] == nil || !got[0].Yes() || got[1] == nil || got[1].Yes() || got[2] != nil {
		t.Errorf("answers = %+v", got)
	}
	if usage.InputTokens != 300 {
		t.Errorf("usage = %+v", usage)
	}

	req, _ := js.last()
	if len(req.Questions) != 3 {
		t.Errorf("questions = %d, want one per chunk", len(req.Questions))
	}
	q := req.Questions[ChunkKey(1)]
	if q.Type != TypeNoul || !strings.Contains(q.Instructions, "Subscribe to our newsletter!") || !strings.Contains(q.Instructions, "how do transformers work") || !strings.Contains(q.Instructions, `"chunk_1"`) {
		t.Errorf("question = %+v", q)
	}
	state, _ := req.State.(map[string]any)
	items, _ := state["chunks"].([]any)
	if state["query"] != "how do transformers work" || len(items) != 3 {
		t.Errorf("state = %+v", state)
	}
	first, _ := items[0].(map[string]any)
	if first["id"] != "chunk_0" || first["text"] != chunks[0] {
		t.Errorf("state chunk 0 = %+v", first)
	}
}

func TestFilterChunksEdgeCases(t *testing.T) {
	js := newJevServer(t, func(SystemOneRequest) (int, any) { return 500, `boom` })
	c := js.client()
	c.NoRetry = true
	if got, _, err := c.FilterChunks(context.Background(), Ask{Query: "q"}, nil); err != nil || len(got) != 0 || js.calls.Load() != 0 {
		t.Errorf("no chunks should short-circuit: %v, %v", got, err)
	}
	if _, _, err := c.FilterChunks(context.Background(), Ask{Query: ""}, []string{"x"}); err == nil {
		t.Error("empty query should error")
	}
	if _, _, err := c.FilterChunks(context.Background(), Ask{Query: "q"}, []string{"x"}); err == nil || !strings.Contains(err.Error(), "chunk filter") {
		t.Errorf("server error should propagate: %v", err)
	}
}
