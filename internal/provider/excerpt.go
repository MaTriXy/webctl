package provider

import (
	"strings"
	"unicode"
)

// Snippet limits.
const (
	snippetChars      = 600
	substantiveChars  = 60
	substantiveWords  = 8
	substantiveLetter = 0.6
)

// excerpt picks the first substantive passage of page text for a snippet.
// Providers that return page text start it with navigation, menus, cookie
// notices, and dates; a judge shown that sees chrome, not the page. Lines are
// skipped until one reads like prose, then text is joined from there and
// cut to snippetChars. Text with no prose line is used from the start.
func excerpt(text string) string {
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if substantive(line) {
			start = i
			break
		}
	}
	if start < 0 {
		return truncate(collapseWhitespace(text), snippetChars)
	}
	return truncate(collapseWhitespace(strings.Join(lines[start:], "\n")), snippetChars)
}

// substantive reports whether a line reads like a sentence of prose rather
// than a menu item, heading, byline, or fragment.
func substantive(line string) bool {
	line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*•>#|"))
	if len([]rune(line)) < substantiveChars || len(strings.Fields(line)) < substantiveWords {
		return false
	}
	if strings.Count(line, "|") >= 3 {
		return false // a table row
	}
	lower := strings.ToLower(line)
	for _, chrome := range []string{"skip to", "cookie", "sign in", "log in", "subscribe", "menu", "copied to clipboard"} {
		if strings.HasPrefix(lower, chrome) {
			return false
		}
	}
	var letters, total int
	for _, r := range line {
		if !unicode.IsSpace(r) {
			total++
			if unicode.IsLetter(r) {
				letters++
			}
		}
	}
	return total > 0 && float64(letters)/float64(total) >= substantiveLetter
}
