package evals

import (
	"fmt"
	"io"
	"strings"
)

// WriteModeTable prints one row per mode: what reached the context window
// and how well it covered the expected themes.
func WriteModeTable(w io.Writer, sums []ModeSummary, markdown bool) {
	header := []string{"mode", "cases", "pass", "results", "chars", "junk", "flagged", "expected-domain hits", "themes covered", "pages ok/failed", "chunks kept/total", "chars raw→kept", "avg stage ms", "jev tokens in"}
	var rows [][]string
	for _, s := range sums {
		pages, chunks, raw := "", "", ""
		if s.Mode == ModeScrape {
			pages = fmt.Sprintf("%d/%d", s.PagesOK, s.PagesFailed)
			chunks = fmt.Sprintf("%d/%d", s.ChunksKept, s.ChunksTotal)
			raw = fmt.Sprintf("%d→%d", s.CharsRaw, s.Chars)
		}
		avgMs := int64(0)
		if s.Cases > 0 {
			avgMs = s.DurationMs / int64(s.Cases)
		}
		rows = append(rows, []string{
			string(s.Mode), fmt.Sprint(s.Cases), fmt.Sprintf("%d/%d", s.Passed, s.Cases), fmt.Sprint(s.Results), fmt.Sprint(s.Chars),
			fmt.Sprint(s.Junk), fmt.Sprint(s.Flagged), fmt.Sprint(s.ExpectedHits), fmt.Sprintf("%d/%d", s.ThemesCovered, s.ThemesTotal),
			pages, chunks, raw, fmt.Sprint(avgMs), fmt.Sprint(s.TokensIn),
		})
	}
	writeTable(w, header, rows, markdown)
}

// WriteCaseTable prints one row per stored stage row.
func WriteCaseTable(w io.Writer, rows []CaseRow, markdown bool) {
	header := []string{"case", "tags", "mode", "pass", "results", "chars", "junk", "flagged", "exp-hits", "themes", "pages ok/failed", "chunks kept/total", "ms", "note"}
	var out [][]string
	for _, r := range rows {
		pass := "✓"
		if !r.Passed {
			pass = "✗"
		}
		note := r.Error
		if note == "" {
			note = strings.ReplaceAll(r.Failures, "\n", "; ")
		}
		pages, chunks := "", ""
		if r.Mode == ModeScrape {
			pages = fmt.Sprintf("%d/%d", r.PagesOK, r.PagesFailed)
			chunks = fmt.Sprintf("%d/%d", r.ChunksKept, r.ChunksTotal)
		}
		out = append(out, []string{
			r.Case, r.Tags, string(r.Mode), pass, fmt.Sprint(r.Results), fmt.Sprint(r.Chars), fmt.Sprint(r.Junk), fmt.Sprint(r.Flagged), fmt.Sprint(r.ExpectedHits),
			fmt.Sprintf("%d/%d", r.ThemesCovered, r.ThemesTotal), pages, chunks, fmt.Sprint(r.DurationMs), note,
		})
	}
	writeTable(w, header, out, markdown)
}

func writeTable(w io.Writer, header []string, rows [][]string, markdown bool) {
	if markdown {
		fmt.Fprintf(w, "| %s |\n", strings.Join(header, " | "))
		fmt.Fprintf(w, "|%s\n", strings.Repeat("---|", len(header)))
		for _, r := range rows {
			for i := range r {
				r[i] = strings.ReplaceAll(r[i], "|", "\\|")
			}
			fmt.Fprintf(w, "| %s |\n", strings.Join(r, " | "))
		}
		return
	}
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	line := func(cells []string) {
		for i, c := range cells {
			fmt.Fprintf(w, "%-*s  ", widths[i], c)
		}
		fmt.Fprintln(w)
	}
	line(header)
	for _, r := range rows {
		line(r)
	}
}
