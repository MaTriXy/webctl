package evals

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dorkitude/webctl/internal/jev"
	"github.com/dorkitude/webctl/internal/provider"
)

func TestRunSaveLoadSummarizeRows(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs")
	run := NewRun("0.0.9", "abc", "exa", AllModes, "test")
	raw := []provider.SearchResult{{Title: "t", URL: "https://a"}}
	run.Reports = []*Report{
		{Case: "c1", Version: "0.0.9", Tags: []string{"reddit"}, Raw: raw, SearchDuration: 250 * time.Millisecond, Stages: []Stage{
			{Mode: ModeFilter, Results: 4, Chars: 2000, Themes: make([]ThemeResult, 2), Covered: 2, Passed: true, Usage: jev.Usage{InputTokens: 100}},
			{Mode: ModeNoFilter, Results: 10, Chars: 5000, Junk: 3, Themes: make([]ThemeResult, 2), Covered: 2, Passed: true, Duration: time.Second},
			{Mode: ModeScrape, Results: 4, Chars: 8000, CharsRaw: 40000, ChunksTotal: 20, ChunksKept: 4, PagesOK: 3, PagesFailed: 1, Failures: []string{"theme x"}},
		}},
		{Case: "broken", Version: "0.0.9", Error: "search: down"},
	}
	run.Summary = Summarize(run.Reports)
	run.FinishedAt = time.Now()
	if _, err := run.Save(dir); err != nil {
		t.Fatal(err)
	}
	runs, err := LoadRuns(dir)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("LoadRuns = %v, %v", runs, err)
	}
	if got, err := ResolveRun(dir, "latest:0.0.9"); err != nil || got.ID != run.ID {
		t.Errorf("ResolveRun latest:version = %v, %v", got, err)
	}
	if _, err := ResolveRun(dir, "nope"); err == nil {
		t.Error("unknown id should fail")
	}
	if vs := Versions(runs); len(vs) != 1 || vs[0] != "0.0.9" || Latest(runs, "0.0.9") == nil || Latest(runs, "x") != nil {
		t.Errorf("versions = %v", vs)
	}
	if rs := runs[0].RawSearches(); len(rs["c1"]) != 1 || rs["c1"][0].URL != "https://a" {
		t.Errorf("raw searches = %v", rs)
	}
	sums := runs[0].Summarize()
	if len(sums) != 3 || sums[0].Mode != ModeNoFilter || sums[0].Junk != 3 || sums[1].TokensIn != 100 || sums[2].CharsRaw != 40000 || sums[2].Passed != 0 {
		t.Errorf("summaries = %+v", sums)
	}
	rows := runs[0].Rows()
	if len(rows) != 4 || rows[0].Case != "broken" || rows[0].Error != "search: down" || rows[1].Mode != ModeNoFilter || rows[3].Failures != "theme x" {
		t.Errorf("rows = %+v", rows)
	}
	if empty, err := LoadRuns(filepath.Join(dir, "missing")); err != nil || len(empty) != 0 {
		t.Errorf("missing dir = %v, %v", empty, err)
	}
}
