package scrape

import (
	"strings"
	"testing"
)

const hnHeader = "Hacker Newsnew | past | comments | ask | show | jobs | submit\n\nlogin\n\nShow HN: A tiny search CLI\n\n213 points by dorkitude 3 hours ago | hide | past | favorite | 87 comments\n\n"

const rustdocHeader = "Skip to main content\n\nModule pin\n\nstd1.98.1\n\nSettings\n\nHelp\n\nSummary\n\nSource\n\nModule pin\n\n"

var (
	shortLines  = strings.TrimSpace(strings.Repeat("menu item\n\n", 20))
	mostlyShort = shortLines + "\n\nOne real sentence that is longer than the menu limit.\n\n" + shortLines + "\n\nHome | About | Contact"
)

var prose = strings.TrimSpace(strings.Repeat("Types that pin data to a location in memory, so the pinned data cannot be moved elsewhere. ", 4))

func TestStripBoilerplate(t *testing.T) {
	footer := "\n\nGuidelines | FAQ | Lists | API\n\nSecurity\n\nLegal\n\nApply to YC\n\nContact"
	list := "Steps:\n\nOne\n\nTwo\n\nThree\n\nFour\n\nFive"
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hacker news header", hnHeader + prose, prose},
		{"rustdoc header keeps heading before prose", rustdocHeader + prose, "Module pin\n\n" + prose},
		{"footer run", prose + footer, prose},
		{"both ends", hnHeader + prose + footer, prose},
		{"short list in the middle survives", prose + "\n\n" + list + "\n\n" + prose, prose + "\n\n" + list + "\n\n" + prose},
		{"short list at the end has no link row and survives", prose + "\n\n" + list, prose + "\n\n" + list},
		{"run shorter than the minimum is kept", "Home | About\n\nlogin\n\nCart\n\n" + prose, "Home | About\n\nlogin\n\nCart\n\n" + prose},
		{"whole page of short lines is kept", list, list},
		{"stripping most of the page is refused", mostlyShort, mostlyShort},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripBoilerplate(c.in); got != c.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}
}

func TestLinkLike(t *testing.T) {
	for line, want := range map[string]bool{
		"new | past | comments | ask | show | jobs | submit":                                  true,
		"Home · Docs · Blog":                                                                  true,
		"AAPL 1.2% • MSFT 0.4% • GOOG -0.1%":                                                  true,
		"• A single bullet point":                                                             false,
		"We propose a new architecture, and it | works":                                       true,
		"A long sentence with one pipe in the middle of it that goes on | and on for a while": false,
		"no separators here":                                                                  false,
	} {
		if got := linkLike(line); got != want {
			t.Errorf("linkLike(%q) = %v, want %v", line, got, want)
		}
	}
}
