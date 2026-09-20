// Package dedupe finds results that cover the same content. Exact matches
// are caught by a normalized URL or title; near-duplicates (mirrors,
// syndicated copies, abstract vs. PDF, rewrites) are proposed by MinHash
// locality-sensitive hashing over word shingles, so candidate pairs come
// from hash buckets rather than every pair, and are then confirmed by a
// judge one batch at a time.
package dedupe

import (
	"context"
	"hash/fnv"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/dorkitude/multi_search_web/internal/provider"
)

// MinHash parameters. 64 hashes in 16 bands of 4 rows makes a pair with
// Jaccard similarity 0.5 land in a shared bucket with probability ~0.65
// and one at 0.8 with ~0.99, while pairs at 0.2 collide about 1% of the
// time. Candidates are then re-checked against the full signature.
const (
	numHashes    = 64
	bands        = 16
	rowsPerBand  = numHashes / bands
	shingleWords = 3
	// minEstimated is the signature-estimated Jaccard a bucket collision
	// must reach to become a candidate pair.
	minEstimated = 0.35
	// minText is the shortest text worth hashing; shorter snippets collide
	// on boilerplate alone.
	minText = 120
)

// NormalizeURL reduces a URL to the form two engines are likely to share:
// lowercase host without www./m./amp. prefixes, no scheme, no fragment,
// tracking parameters removed, trailing slash dropped.
func NormalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.ToLower(strings.TrimSpace(raw))
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range []string{"www.", "m.", "amp.", "mobile."} {
		host = strings.TrimPrefix(host, p)
	}
	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || lk == "fbclid" || lk == "gclid" || lk == "ref" || lk == "source" || lk == "mc_cid" || lk == "mc_eid" {
			q.Del(k)
		}
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	path = strings.TrimSuffix(path, "/amp")
	out := host + path
	if enc := q.Encode(); enc != "" {
		out += "?" + enc
	}
	return out
}

// tokens lowercases text and splits it into alphanumeric words.
func tokens(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// shingles returns the set of word n-grams of s as 64-bit hashes.
func shingles(s string) map[uint64]struct{} {
	w := tokens(s)
	out := map[uint64]struct{}{}
	for i := 0; i+shingleWords <= len(w); i++ {
		h := fnv.New64a()
		for j := 0; j < shingleWords; j++ {
			h.Write([]byte(w[i+j]))
			h.Write([]byte{0})
		}
		out[h.Sum64()] = struct{}{}
	}
	return out
}

// signature is a MinHash sketch: for each of numHashes seeded hash
// functions, the minimum over the shingle set.
type signature [numHashes]uint64

// seeds are fixed odd multipliers for the universal-hash family
// h_i(x) = (a_i * x + b_i) mod 2^64.
var seeds = func() [numHashes][2]uint64 {
	var s [numHashes][2]uint64
	x := uint64(0x9E3779B97F4A7C15)
	for i := range s {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		s[i][0] = x | 1
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		s[i][1] = x
	}
	return s
}()

func sketch(sh map[uint64]struct{}) signature {
	var sig signature
	for i := range sig {
		sig[i] = ^uint64(0)
	}
	for x := range sh {
		for i, s := range seeds {
			if h := s[0]*x + s[1]; h < sig[i] {
				sig[i] = h
			}
		}
	}
	return sig
}

func estimate(a, b signature) float64 {
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	return float64(same) / numHashes
}

// text is what a result is hashed on: its content when present, else its
// snippet, with the title prepended.
func text(r provider.SearchResult) string {
	body := r.Content
	if strings.TrimSpace(body) == "" {
		body = r.Snippet
	}
	return r.Title + "\n" + body
}

// Pair is a candidate duplicate: indexes into the input slice, first < second.
type Pair struct {
	A, B      int
	Estimated float64
}

// Candidates proposes near-duplicate pairs among results by LSH. Results
// whose normalized URLs match are not proposed; those are exact duplicates
// for the caller to collapse directly.
func Candidates(results []provider.SearchResult) []Pair {
	sigs := make([]signature, len(results))
	ok := make([]bool, len(results))
	for i, r := range results {
		t := text(r)
		if len([]rune(t)) < minText {
			continue
		}
		sigs[i] = sketch(shingles(t))
		ok[i] = true
	}
	seen := map[[2]int]bool{}
	var pairs []Pair
	for b := 0; b < bands; b++ {
		buckets := map[[rowsPerBand]uint64][]int{}
		for i := range results {
			if !ok[i] {
				continue
			}
			var key [rowsPerBand]uint64
			copy(key[:], sigs[i][b*rowsPerBand:(b+1)*rowsPerBand])
			buckets[key] = append(buckets[key], i)
		}
		for _, members := range buckets {
			for x := 0; x < len(members); x++ {
				for y := x + 1; y < len(members); y++ {
					i, j := members[x], members[y]
					if seen[[2]int{i, j}] {
						continue
					}
					seen[[2]int{i, j}] = true
					if NormalizeURL(results[i].URL) == NormalizeURL(results[j].URL) {
						continue
					}
					if est := estimate(sigs[i], sigs[j]); est >= minEstimated {
						pairs = append(pairs, Pair{A: i, B: j, Estimated: est})
					}
				}
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Estimated != pairs[j].Estimated {
			return pairs[i].Estimated > pairs[j].Estimated
		}
		return pairs[i].A*len(results)+pairs[i].B < pairs[j].A*len(results)+pairs[j].B
	})
	return pairs
}

// Confirmer judges whether each candidate pair really is the same content.
// Answers are aligned with pairs; a nil entry means no judgment.
type Confirmer interface {
	ConfirmDuplicates(ctx context.Context, query string, results []provider.SearchResult, pairs []Pair) ([]bool, error)
}

// Groups unions confirmed pairs into duplicate groups: each entry lists
// the indexes of one group, sorted, only for groups of two or more.
func Groups(n int, pairs []Pair, confirmed []bool) [][]int {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	for i, p := range pairs {
		if i < len(confirmed) && confirmed[i] {
			parent[find(p.A)] = find(p.B)
		}
	}
	byRoot := map[int][]int{}
	for i := 0; i < n; i++ {
		r := find(i)
		byRoot[r] = append(byRoot[r], i)
	}
	var out [][]int
	for _, g := range byRoot {
		if len(g) > 1 {
			sort.Ints(g)
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// Run proposes candidates, asks the confirmer about them in one pass, and
// returns the duplicate groups. With no confirmer, only pairs the sketch
// is very sure about (estimated Jaccard ≥ 0.7) count.
func Run(ctx context.Context, c Confirmer, query string, results []provider.SearchResult) ([][]int, []Pair, error) {
	pairs := Candidates(results)
	if len(pairs) == 0 {
		return nil, nil, nil
	}
	confirmed := make([]bool, len(pairs))
	if c == nil {
		for i, p := range pairs {
			confirmed[i] = p.Estimated >= 0.7
		}
		return Groups(len(results), pairs, confirmed), pairs, nil
	}
	answers, err := c.ConfirmDuplicates(ctx, query, results, pairs)
	if err != nil {
		return nil, pairs, err
	}
	copy(confirmed, answers)
	return Groups(len(results), pairs, confirmed), pairs, nil
}
