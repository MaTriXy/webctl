package provider

import (
	"context"
	"errors"
	"fmt"
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

func TestChainTopsUpShortAnswers(t *testing.T) {
	mk := func(n int, prefix string) []SearchResult {
		var out []SearchResult
		for i := 0; i < n; i++ {
			out = append(out, SearchResult{Title: prefix + " " + string(rune('a'+i)), URL: "https://" + prefix + ".example/" + string(rune('a'+i))})
		}
		return out
	}
	shared := SearchResult{Title: "shared page title here", URL: "https://shared.example/x"}
	a := append([]SearchResult{shared}, mk(2, "a")...) // 3 of 10 requested: short
	b := append(mk(4, "b"), shared)
	c := chainOf(&chainStub{name: "a", results: a}, &chainStub{name: "b", results: b}, &chainStub{name: "c", hang: true})
	got, err := c.Search(context.Background(), "q", 10)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "a+b" || len(got) != 7 || got[0].URL != shared.URL {
		t.Errorf("name %q, %d results, first %s", c.Name(), len(got), got[0].URL)
	}

	// A full answer is not topped up; NoTopUp disables it; a failing next provider is skipped.
	c = chainOf(&chainStub{name: "a", results: mk(6, "a")}, &chainStub{name: "b", results: b})
	if got, _ := c.Search(context.Background(), "q", 10); len(got) != 6 || c.Name() != "a" {
		t.Errorf("full answer topped up: %d, %q", len(got), c.Name())
	}
	c = chainOf(&chainStub{name: "a", results: a}, &chainStub{name: "b", results: b})
	c.NoTopUp = true
	if got, _ := c.Search(context.Background(), "q", 10); len(got) != 3 {
		t.Errorf("NoTopUp ignored: %d", len(got))
	}
	c = chainOf(&chainStub{name: "a", results: a}, &chainStub{name: "b", err: errors.New("down")})
	if got, _ := c.Search(context.Background(), "q", 10); len(got) != 3 || c.Name() != "a" {
		t.Errorf("failed top-up should keep the first answer: %d, %q", len(got), c.Name())
	}
}

func TestChainHonorsCooldowns(t *testing.T) {
	hit := SearchResult{Title: "t", URL: "https://x"}
	cd := NewCooldown("", CooldownConfig{Enabled: true, Steps: []time.Duration{time.Hour}, ProbeInterval: time.Hour, QuotaStart: 1})
	var skips []string
	c := chainOf(&chainStub{name: "a", err: &APIError{Provider: "Exa", Status: 429}}, &chainStub{name: "b", results: []SearchResult{hit}})
	c.Cooldowns = cd
	c.OnSkip = func(name, reason string, announce bool) { skips = append(skips, fmt.Sprintf("%s:%v", name, announce)) }
	if _, err := c.Search(context.Background(), "q", 5); err != nil || c.Name() != "b" {
		t.Fatalf("first search: %v %q", err, c.Name())
	}
	if !cd.Active("a") {
		t.Fatal("429 should start a cooldown")
	}
	// Second search skips a without calling it and announces once.
	c = chainOf(&chainStub{name: "a", err: errors.New("must not be called")}, &chainStub{name: "b", results: []SearchResult{hit}})
	c.Cooldowns = cd
	c.OnSkip = func(name, reason string, announce bool) { skips = append(skips, fmt.Sprintf("%s:%v", name, announce)) }
	for i := 0; i < 2; i++ {
		if _, err := c.Search(context.Background(), "q", 5); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(skips, ",") != "a:true,a:false" {
		t.Errorf("skips = %v", skips)
	}
	// Everything cooling down is a clear error.
	cd.Note("b", &APIError{Provider: "Parallel", Status: 402})
	_, err := c.Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "skipped") {
		t.Errorf("err = %v", err)
	}
}

func TestChainSourcesWavesAndFusion(t *testing.T) {
	mk := func(prefix string, n int) []SearchResult {
		var out []SearchResult
		for i := 0; i < n; i++ {
			out = append(out, SearchResult{Title: prefix + " result " + string(rune('a'+i)), URL: "https://" + prefix + ".example/" + string(rune('a'+i))})
		}
		return out
	}
	shared := SearchResult{Title: "the shared page", URL: "https://shared.example/x"}
	c := chainOf(
		&chainStub{name: "a", results: append([]SearchResult{shared}, mk("a", 3)...)},
		&chainStub{name: "b", err: errors.New("down")},
		&chainStub{name: "c", results: append(mk("c", 2), shared)},
		&chainStub{name: "d"}, // empty
		&chainStub{name: "e", results: mk("e", 2)},
		&chainStub{name: "f", results: mk("f", 2)},
	)
	c.Sources = 3
	got, err := c.Search(context.Background(), "q", 20)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name() != "a+c+e" {
		t.Errorf("name = %q, want a+c+e (b failed, d empty, f not needed)", c.Name())
	}
	if got[0].URL != shared.URL || len(got) != 8 {
		t.Errorf("fused: first %s, %d results", got[0].URL, len(got))
	}
	if eng := c.Engines()[shared.URL]; strings.Join(eng, ",") != "a,c" {
		t.Errorf("engines for shared = %v", eng)
	}

	// Fewer available than requested is fine; one list is returned unfused.
	c = chainOf(&chainStub{name: "a", results: mk("a", 2)}, &chainStub{name: "b", err: errors.New("down")})
	c.Sources = 3
	if got, err := c.Search(context.Background(), "q", 20); err != nil || len(got) != 2 || c.Name() != "a" || c.Engines() != nil {
		t.Errorf("single list: %v %v %q", got, err, c.Name())
	}
	// Nothing available is an error.
	c = chainOf(&chainStub{name: "a", err: errors.New("one")}, &chainStub{name: "b", err: errors.New("two")})
	c.Sources = 2
	if _, err := c.Search(context.Background(), "q", 5); err == nil || !strings.Contains(err.Error(), "all 2 providers failed") {
		t.Errorf("err = %v", err)
	}
}
