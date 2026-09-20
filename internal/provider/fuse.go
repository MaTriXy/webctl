package provider

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// RRFK is the standard Reciprocal Rank Fusion smoothing constant.
const RRFK = 60

// Ranked is one engine's ordered result list.
type Ranked struct {
	Engine  string
	Results []SearchResult
}

// Fused is a deduplicated result with its fusion score and contributing engines.
type Fused struct {
	SearchResult
	// Score is the RRF score: Σ 1/(k + rank_i) over the engines that returned it.
	Score float64
	// Engines lists the engines that returned this URL, in list order.
	Engines []string
}

// SearchAll queries every provider concurrently and returns the successful
// lists in providers order, plus the error for each provider that failed.
func SearchAll(ctx context.Context, providers []Provider, query string, num int) ([]Ranked, map[string]error) {
	lists := make([]Ranked, len(providers))
	errs := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, p := range providers {
		wg.Add(1)
		go func(i int, p Provider) {
			defer wg.Done()
			results, err := p.Search(ctx, query, num)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[p.Name()] = err
				return
			}
			lists[i] = Ranked{Engine: p.Name(), Results: results}
		}(i, p)
	}
	wg.Wait()
	ok := make([]Ranked, 0, len(lists))
	for _, l := range lists {
		if l.Engine != "" {
			ok = append(ok, l)
		}
	}
	return ok, errs
}

// Fuse merges ranked lists with Reciprocal Rank Fusion, deduplicating by
// canonical URL. score = Σ 1/(k + rank) with 1-based ranks; k ≤ 0 uses RRFK.
// Ties break toward the result seen by more engines, then by first
// appearance. limit ≤ 0 means no cap.
func Fuse(lists []Ranked, k, limit int) []Fused {
	if k <= 0 {
		k = RRFK
	}
	index := map[string]int{}
	var out []Fused
	for _, l := range lists {
		for rank, r := range l.Results {
			key := canonicalURL(r.URL)
			if key == "" {
				continue
			}
			i, seen := index[key]
			if !seen {
				i = len(out)
				index[key] = i
				out = append(out, Fused{SearchResult: r})
			}
			f := &out[i]
			f.Score += 1 / float64(k+rank+1)
			if !contains(f.Engines, l.Engine) {
				f.Engines = append(f.Engines, l.Engine)
			}
			// Fill gaps from later engines.
			if f.Title == "" {
				f.Title = r.Title
			}
			if f.Snippet == "" {
				f.Snippet = r.Snippet
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return len(out[a].Engines) > len(out[b].Engines)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// canonicalURL normalizes a URL for deduplication: scheme dropped (http and
// https are the same page), lowercase host without "www.", no fragment, no
// trailing slash.
func canonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimRight(raw, "/")
	}
	u.Scheme = ""
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimPrefix(u.String(), "//")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
