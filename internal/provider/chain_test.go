package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type chainStub struct {
	name    string
	results []SearchResult
	err     error
	hang    bool
}

func (s *chainStub) Name() string { return s.name }
func (s *chainStub) Search(ctx context.Context, _ string, _ int) ([]SearchResult, error) {
	if s.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.results, s.err
}
func (s *chainStub) Validate(context.Context) error { return s.err }

// chainOf builds a Chain over stubs keyed by name; unknown names fail to build.
func chainOf(stubs ...*chainStub) *Chain {
	byName := map[string]*chainStub{}
	var names []string
	for _, s := range stubs {
		byName[s.name] = s
		names = append(names, s.name)
	}
	return &Chain{Names: names, New: func(name string) (Provider, error) {
		if s, ok := byName[name]; ok && s != nil {
			return s, nil
		}
		return nil, errors.New("cannot build " + name)
	}}
}

func TestChainFallsThroughOnErrorAndEmpty(t *testing.T) {
	hit := SearchResult{Title: "t", URL: "https://x"}
	var notices []string
	c := chainOf(
		&chainStub{name: "a", err: errors.New("down")},
		&chainStub{name: "b"}, // empty answer
		&chainStub{name: "c", results: []SearchResult{hit}},
	)
	c.OnFallthrough = func(failed string, err error, next string) {
		notices = append(notices, failed+"->"+next+":"+err.Error())
	}
	got, err := c.Search(context.Background(), "q", 5)
	if err != nil || len(got) != 1 || c.Name() != "c" {
		t.Fatalf("got %v, %v, name %q", got, err, c.Name())
	}
	if strings.Join(notices, " ") != "a->b:down b->c:no results" {
		t.Errorf("notices = %v", notices)
	}

	// A trailing empty answer is a success.
	c = chainOf(&chainStub{name: "a", err: errors.New("down")}, &chainStub{name: "b"})
	if got, err := c.Search(context.Background(), "q", 5); err != nil || len(got) != 0 || c.Name() != "b" {
		t.Errorf("trailing empty: %v, %v, %q", got, err, c.Name())
	}

	// Everything failing joins every error; a provider that cannot be built counts as failed.
	c = chainOf(&chainStub{name: "a", err: errors.New("one")}, &chainStub{name: "b", err: errors.New("two")})
	c.Names = append(c.Names, "missing")
	_, err = c.Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "all 3 providers failed") || !strings.Contains(err.Error(), "two") || !strings.Contains(err.Error(), "cannot build missing") {
		t.Errorf("err = %v", err)
	}
}

func TestChainTimeouts(t *testing.T) {
	hit := SearchResult{Title: "t", URL: "https://x"}
	c := chainOf(&chainStub{name: "slow", hang: true}, &chainStub{name: "fast", results: []SearchResult{hit}})
	c.AttemptTimeout, c.Budget = 20*time.Millisecond, time.Second
	start := time.Now()
	if got, err := c.Search(context.Background(), "q", 5); err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Errorf("slow provider held the chain for %s", time.Since(start))
	}

	c = chainOf(&chainStub{name: "a", hang: true}, &chainStub{name: "b", hang: true}, &chainStub{name: "c", hang: true})
	c.AttemptTimeout, c.Budget = time.Second, 30*time.Millisecond
	start = time.Now()
	_, err := c.Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "chain budget") {
		t.Errorf("err = %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("chain ran %s despite a 30ms budget", time.Since(start))
	}
}
