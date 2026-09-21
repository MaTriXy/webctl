package scrape

import (
	"strings"
	"unicode/utf8"
)

// Boilerplate stripping tunables.
const (
	// menuLineChars is the longest a line can be and still count as menu-like.
	menuLineChars = 40
	// menuRunMin is how many consecutive menu-like lines make a run worth stripping.
	menuRunMin = 4
	// stripKeepFraction: if stripping would leave less than this share of the
	// text, it is a page of short lines, not a page with menus, and is kept whole.
	stripKeepFraction = 0.2
)

// menuSeparators join link-like words in a navigation line.
var menuSeparators = []string{"|", "·", "•"}

// StripBoilerplate removes a leading and a trailing run of menu-like lines
// (site navigation, tickers, footers) from page text. A run is menuRunMin or
// more consecutive non-blank lines that are each short (≤ menuLineChars
// runes) or link-like (short words joined by |, ·, or •). Runs in the middle
// of the text are left alone, so lists and headings survive; a short line at
// the end of the leading run that precedes real prose is kept as its heading;
// a trailing run must include a link-like row, or it is a closing list, not
// a footer. If stripping would remove more than 1-stripKeepFraction of the
// text, the original is returned.
func StripBoilerplate(text string) string {
	lines := strings.Split(text, "\n")
	lo := leadingRun(lines)
	hi := trailingRun(lines)
	if lo == 0 && hi == len(lines) {
		return text
	}
	if lo >= hi {
		return text
	}
	out := strings.TrimSpace(strings.Join(lines[lo:hi], "\n"))
	if float64(utf8.RuneCountInString(out)) < stripKeepFraction*float64(utf8.RuneCountInString(text)) {
		return text
	}
	return out
}

// leadingRun returns the index of the first line to keep: 0 when the text
// does not start with a menu run.
func leadingRun(lines []string) int {
	n, last, end := 0, -1, 0
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !menuLike(l) {
			break
		}
		n, last, end = n+1, i, i+1
	}
	if n < menuRunMin || end >= len(lines) {
		return 0
	}
	// A heading is a short line, not a link row, right before the prose.
	if !linkLike(lines[last]) {
		return last
	}
	return end
}

// trailingRun returns the index one past the last line to keep: len(lines)
// when the text does not end with a menu run.
func trailingRun(lines []string) int {
	n, start, links := 0, len(lines), 0
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !menuLike(l) {
			break
		}
		n, start = n+1, i
		if linkLike(l) {
			links++
		}
	}
	if n < menuRunMin || links == 0 {
		return len(lines)
	}
	return start
}

func menuLike(line string) bool {
	line = strings.TrimSpace(line)
	return utf8.RuneCountInString(line) <= menuLineChars || linkLike(line)
}

// linkLike reports whether line is two or more short segments joined by menu
// separators, like "new | past | comments | ask".
func linkLike(line string) bool {
	line = strings.TrimSpace(line)
	for _, sep := range menuSeparators {
		if !strings.Contains(line, sep) {
			continue
		}
		parts := strings.Split(line, sep)
		segments := 0
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if utf8.RuneCountInString(p) > menuLineChars {
				return false
			}
			segments++
		}
		return segments >= 2
	}
	return false
}
