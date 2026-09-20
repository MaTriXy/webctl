package provider

import (
	"strings"
	"testing"
)

func TestExcerptSkipsChrome(t *testing.T) {
	text := "Skip to main content\n* Features\n* Pricing\nLog in\nPublished 6 August 2026 · Team · 8 min read\n" +
		"Research says 1.6 to 2.2 grams of protein per kilogram of bodyweight maximizes muscle protein synthesis for most lifters.\n" +
		"A second sentence with more detail follows here."
	got := excerpt(text)
	if !strings.HasPrefix(got, "Research says 1.6") || !strings.Contains(got, "second sentence") {
		t.Errorf("excerpt = %q", got)
	}
	// No prose line at all: fall back to the start.
	if got := excerpt("Home\nBlog\nAbout"); got != "Home Blog About" {
		t.Errorf("fallback = %q", got)
	}
	// Long text is cut to the snippet limit.
	long := strings.Repeat("word ", 300)
	if n := len([]rune(excerpt("This is a leading sentence of prose that is long enough to count.\n" + long))); n > snippetChars+1 {
		t.Errorf("excerpt length = %d", n)
	}
	if substantive("| Product | Best For | Price | Rating | Link | Notes | More | Columns |") {
		t.Error("table rows are not prose")
	}
}
