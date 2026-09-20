package provider

import (
	"strings"
	"unicode"
)

// dedupeMinTitle is the shortest normalized title that is specific enough
// to identify a duplicate; shorter titles are never collapsed.
const dedupeMinTitle = 15

// Dedupe collapses results that share a URL or a title, keeping the first
// of each. Engines often return the same paper or post from several hosts
// (arXiv abs, arXiv pdf, a proceedings mirror) and an expert wants one.
// Titles are compared case- and punctuation-insensitively.
func Dedupe(results []SearchResult) []SearchResult {
	out := make([]SearchResult, 0, len(results))
	seenURL := map[string]bool{}
	seenTitle := map[string]bool{}
	for _, r := range results {
		u := strings.TrimRight(strings.ToLower(strings.TrimSpace(r.URL)), "/")
		if u != "" && seenURL[u] {
			continue
		}
		t := normalizeTitle(r.Title)
		if len(t) >= dedupeMinTitle && seenTitle[t] {
			continue
		}
		if u != "" {
			seenURL[u] = true
		}
		if len(t) >= dedupeMinTitle {
			seenTitle[t] = true
		}
		out = append(out, r)
	}
	return out
}

// normalizeTitle lowercases and keeps only letters, digits, and single
// spaces, dropping a trailing " | Site" or " - Site" suffix.
func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.LastIndexAny(s, "|—–-"); i > len(s)/2 {
		s = s[:i]
	}
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}
