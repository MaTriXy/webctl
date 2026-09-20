package scrape

import (
	"strings"
	"unicode/utf8"
)

// DefaultChunkChars is the target chunk size for relevance filtering.
const DefaultChunkChars = 2000

// Split divides text into chunks of at most maxChars runes. Paragraphs
// ("\n\n") are packed together until the limit; a paragraph that is itself
// too long is split on lines, then sentences, then words, then hard-cut.
// maxChars ≤ 0 uses DefaultChunkChars.
func Split(text string, maxChars int) []string {
	if maxChars <= 0 {
		maxChars = DefaultChunkChars
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return pack(strings.Split(text, "\n\n"), "\n\n", maxChars, splitParagraph)
}

func splitParagraph(p string, max int) []string {
	return pack(strings.Split(p, "\n"), "\n", max, splitLine)
}

func splitLine(line string, max int) []string {
	return pack(splitSentences(line), " ", max, splitWords)
}

func splitWords(s string, max int) []string {
	return pack(strings.Fields(s), " ", max, hardCut)
}

// hardCut slices a single oversized token on rune boundaries.
func hardCut(s string, max int) []string {
	var out []string
	r := []rune(s)
	for len(r) > max {
		out = append(out, string(r[:max]))
		r = r[max:]
	}
	if len(r) > 0 {
		out = append(out, string(r))
	}
	return out
}

// pack greedily joins parts with sep into chunks no longer than max runes.
// Parts that exceed max on their own are broken up with splitPart first.
func pack(parts []string, sep string, max int, splitPart func(string, int) []string) []string {
	var chunks []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			chunks = append(chunks, s)
		}
		cur.Reset()
		curLen = 0
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n := utf8.RuneCountInString(part)
		if n > max {
			flush()
			chunks = append(chunks, splitPart(part, max)...)
			continue
		}
		if curLen > 0 && curLen+utf8.RuneCountInString(sep)+n > max {
			flush()
		}
		if curLen > 0 {
			cur.WriteString(sep)
			curLen += utf8.RuneCountInString(sep)
		}
		cur.WriteString(part)
		curLen += n
	}
	flush()
	return chunks
}

// splitSentences breaks text after sentence-ending punctuation followed by a space.
func splitSentences(s string) []string {
	var out []string
	start := 0
	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		switch runes[i] {
		case '.', '!', '?':
			if runes[i+1] == ' ' {
				out = append(out, string(runes[start:i+1]))
				start = i + 2
				i++
			}
		}
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}
	return out
}

// Join reassembles kept chunks into a single text.
func Join(chunks []string) string {
	return strings.Join(chunks, "\n\n")
}
