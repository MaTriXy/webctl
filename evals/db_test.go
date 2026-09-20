package evals

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dorkitude/multi_search_web/internal/jev"
	"github.com/dorkitude/multi_search_web/internal/provider"
)

func TestDBRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	runID, err := db.StartRun(ctx, RunInfo{Version: "0.0.9", Provider: "exa", Modes: AllModes, Notes: "test"})
	if err != nil || runID == 0 {
		t.Fatal(err)
	}
	rep := &Report{Case: "c1", Version: "0.0.9", Provider: "exa", Tags: []string{"reddit"}, SearchDuration: 250 * time.Millisecond, Stages: []Stage{
		{Mode: ModeNoFilter, Results: 10, Chars: 5000, Junk: 3, Themes: make([]ThemeResult, 2), Covered: 2, Passed: true, Duration: time.Second},
		{Mode: ModeFilter, Results: 4, Chars: 2000, Junk: 0, Themes: make([]ThemeResult, 2), Covered: 2, Passed: true, Usage: jev.Usage{InputTokens: 100, OutputTokens: 5}, URLs: []string{"https://a"}},
		{Mode: ModeScrape, Results: 4, Chars: 8000, CharsRaw: 40000, ChunksTotal: 20, ChunksKept: 4, PagesOK: 3, PagesFailed: 1, Passed: false, Failures: []string{"theme x"}},
	}}
	if err := db.RecordReport(ctx, runID, rep); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordReport(ctx, runID, &Report{Case: "broken", Version: "0.0.9", Error: "search: down"}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishRun(ctx, runID, Summary{Total: 2, Passed: 1}); err != nil {
		t.Fatal(err)
	}

	if id, err := db.LatestRunID(ctx, "0.0.9"); err != nil || id != runID {
		t.Errorf("LatestRunID = %d, %v", id, err)
	}
	if id, _ := db.LatestRunID(ctx, "nope"); id != 0 {
		t.Errorf("unknown version id = %d", id)
	}
	if vs, _ := db.Versions(ctx); len(vs) != 1 || vs[0] != "0.0.9" {
		t.Errorf("versions = %v", vs)
	}
	sums, err := db.SummarizeRun(ctx, runID)
	if err != nil || len(sums) != 3 {
		t.Fatalf("summaries = %+v, %v", sums, err)
	}
	if sums[0].Mode != ModeNoFilter || sums[0].Junk != 3 || sums[1].TokensIn != 100 || sums[2].CharsRaw != 40000 || sums[2].Passed != 0 {
		t.Errorf("summaries = %+v", sums)
	}
	rows, err := db.RunRows(ctx, runID)
	if err != nil || len(rows) != 4 {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if rows[0].Case != "broken" || rows[0].Error != "search: down" || rows[3].Mode != ModeScrape || rows[3].Failures != "theme x" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestSearchCacheRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDB(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, _, ok, err := db.CachedSearch(ctx, "q", 10, time.Hour); ok || err != nil {
		t.Fatalf("empty cache: %v %v", ok, err)
	}
	want := []provider.SearchResult{{Title: "t", URL: "https://x", Snippet: "s", Content: "c"}}
	if err := db.StoreSearch(ctx, "q", 10, "exa", want); err != nil {
		t.Fatal(err)
	}
	got, prov, ok, err := db.CachedSearch(ctx, "q", 10, time.Hour)
	if err != nil || !ok || prov != "exa" || len(got) != 1 || got[0].Content != "c" {
		t.Errorf("cached = %+v %q %v %v", got, prov, ok, err)
	}
	if _, _, ok, _ := db.CachedSearch(ctx, "q", 20, time.Hour); ok {
		t.Error("a different num is a different entry")
	}
	if _, _, ok, _ := db.CachedSearch(ctx, "q", 10, 0); ok {
		t.Error("a zero max age is always stale")
	}
}
