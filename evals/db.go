package evals

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/dorkitude/smart_search/internal/provider"
)

// DB stores eval runs in SQLite. Every row carries the smart_search
// behavior version so results from different versions never mix.
type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS runs (
	id          INTEGER PRIMARY KEY,
	version     TEXT NOT NULL,
	git_sha     TEXT,
	started_at  TEXT NOT NULL,
	finished_at TEXT,
	provider    TEXT,
	modes       TEXT,
	notes       TEXT,
	cases       INTEGER,
	passed      INTEGER
);
CREATE TABLE IF NOT EXISTS results (
	id            INTEGER PRIMARY KEY,
	run_id        INTEGER NOT NULL REFERENCES runs(id),
	version       TEXT NOT NULL DEFAULT '',
	"case"        TEXT NOT NULL DEFAULT '',
	tags          TEXT DEFAULT '',
	mode          TEXT NOT NULL DEFAULT '',
	provider      TEXT DEFAULT '',
	passed        INTEGER NOT NULL DEFAULT 0,
	results       INTEGER DEFAULT 0,
	chars         INTEGER DEFAULT 0,
	junk          INTEGER DEFAULT 0,
	flagged       INTEGER DEFAULT 0,
	expected_hits INTEGER DEFAULT 0,
	themes_total  INTEGER DEFAULT 0,
	themes_covered INTEGER DEFAULT 0,
	pages_ok      INTEGER DEFAULT 0,
	pages_failed  INTEGER DEFAULT 0,
	chunks_total  INTEGER DEFAULT 0,
	chunks_kept   INTEGER DEFAULT 0,
	chars_raw     INTEGER DEFAULT 0,
	dropped_covered INTEGER DEFAULT 0,
	duration_ms   INTEGER DEFAULT 0,
	search_ms     INTEGER DEFAULT 0,
	tokens_in     INTEGER DEFAULT 0,
	tokens_out    INTEGER DEFAULT 0,
	error         TEXT DEFAULT '',
	failures      TEXT DEFAULT '',
	urls          TEXT DEFAULT '',
	judged        TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS results_version_case ON results(version, "case", mode);
CREATE TABLE IF NOT EXISTS search_cache (
	query      TEXT NOT NULL,
	num        INTEGER NOT NULL,
	provider   TEXT NOT NULL DEFAULT '',
	fetched_at TEXT NOT NULL,
	results    TEXT NOT NULL,
	PRIMARY KEY (query, num)
);
`

// OpenDB opens (creating if needed) the results database at path.
func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init %s: %w", path, err)
	}
	return &DB{sql: db}, nil
}

// Close releases the database.
func (d *DB) Close() error { return d.sql.Close() }

// RunInfo describes one invocation of the suite.
type RunInfo struct {
	Version  string
	GitSHA   string
	Provider string
	Modes    []Mode
	Notes    string
}

// StartRun inserts a run row and returns its id.
func (d *DB) StartRun(ctx context.Context, info RunInfo) (int64, error) {
	modes := make([]string, len(info.Modes))
	for i, m := range info.Modes {
		modes[i] = string(m)
	}
	res, err := d.sql.ExecContext(ctx, `INSERT INTO runs (version, git_sha, started_at, provider, modes, notes) VALUES (?, ?, ?, ?, ?, ?)`,
		info.Version, info.GitSHA, time.Now().UTC().Format(time.RFC3339), info.Provider, strings.Join(modes, ","), info.Notes)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}
	return res.LastInsertId()
}

// FinishRun records the tally on a run.
func (d *DB) FinishRun(ctx context.Context, runID int64, s Summary) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE runs SET finished_at = ?, cases = ?, passed = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), s.Total, s.Passed, runID)
	return err
}

// RecordReport writes one row per stage of rep (or one error row when the
// case could not run).
func (d *DB) RecordReport(ctx context.Context, runID int64, rep *Report) error {
	tags := strings.Join(rep.Tags, ",")
	if len(rep.Stages) == 0 {
		_, err := d.sql.ExecContext(ctx, `INSERT INTO results (run_id, version, "case", tags, mode, provider, passed, duration_ms, search_ms, error)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
			runID, rep.Version, rep.Case, tags, "", rep.Provider, rep.Duration.Milliseconds(), rep.SearchDuration.Milliseconds(), rep.Error)
		return err
	}
	judged := ""
	if len(rep.Judged) > 0 {
		b, _ := json.Marshal(rep.Judged)
		judged = string(b)
	}
	for _, st := range rep.Stages {
		urls, _ := json.Marshal(st.URLs)
		stJudged := ""
		if st.Mode == ModeFilter {
			stJudged = judged
		}
		stErr := st.Error
		if stErr == "" && rep.Error != "" {
			stErr = rep.Error
		}
		_, err := d.sql.ExecContext(ctx, `INSERT INTO results (run_id, version, "case", tags, mode, provider, passed, results, chars, junk, flagged, expected_hits,
			themes_total, themes_covered, pages_ok, pages_failed, chunks_total, chunks_kept, chars_raw, dropped_covered, duration_ms, search_ms, tokens_in, tokens_out, error, failures, urls, judged)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, rep.Version, rep.Case, tags, string(st.Mode), rep.Provider, boolInt(st.Passed && stErr == ""), st.Results, st.Chars, st.Junk, st.Flagged, st.ExpectedHits,
			len(st.Themes), st.Covered, st.PagesOK, st.PagesFailed, st.ChunksTotal, st.ChunksKept, st.CharsRaw, st.DroppedCovered,
			st.Duration.Milliseconds(), rep.SearchDuration.Milliseconds(), st.Usage.InputTokens, st.Usage.OutputTokens,
			stErr, strings.Join(st.Failures, "\n"), string(urls), stJudged)
		if err != nil {
			return fmt.Errorf("insert result: %w", err)
		}
	}
	return nil
}

// CachedSearch returns the stored results for (query, num) if they were
// fetched within maxAge, with the provider that produced them.
func (d *DB) CachedSearch(ctx context.Context, query string, num int, maxAge time.Duration) ([]provider.SearchResult, string, bool, error) {
	var raw, prov, fetched string
	err := d.sql.QueryRowContext(ctx, `SELECT results, provider, fetched_at FROM search_cache WHERE query = ? AND num = ?`, query, num).Scan(&raw, &prov, &fetched)
	if err == sql.ErrNoRows {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	at, err := time.Parse(time.RFC3339, fetched)
	if err != nil || time.Since(at) > maxAge {
		return nil, "", false, nil
	}
	var results []provider.SearchResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, "", false, err
	}
	return results, prov, true, nil
}

// StoreSearch records results for (query, num), replacing any older entry.
func (d *DB) StoreSearch(ctx context.Context, query string, num int, prov string, results []provider.SearchResult) error {
	raw, err := json.Marshal(results)
	if err != nil {
		return err
	}
	_, err = d.sql.ExecContext(ctx, `INSERT OR REPLACE INTO search_cache (query, num, provider, fetched_at, results) VALUES (?, ?, ?, ?, ?)`,
		query, num, prov, time.Now().UTC().Format(time.RFC3339), string(raw))
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ModeSummary aggregates one mode across the latest run of a version.
type ModeSummary struct {
	Version        string
	Mode           Mode
	Cases          int
	Passed         int
	Results        int
	Chars          int
	Junk           int
	Flagged        int
	ExpectedHits   int
	ThemesTotal    int
	ThemesCovered  int
	PagesOK        int
	PagesFailed    int
	ChunksTotal    int
	ChunksKept     int
	CharsRaw       int
	DroppedCovered int
	DurationMs     int64
	SearchMs       int64
	TokensIn       int
	TokensOut      int
}

// LatestRunID returns the newest run id for version (0 if none).
func (d *DB) LatestRunID(ctx context.Context, version string) (int64, error) {
	var id sql.NullInt64
	err := d.sql.QueryRowContext(ctx, `SELECT MAX(id) FROM runs WHERE version = ?`, version).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id.Int64, nil
}

// Versions lists every version with at least one run, oldest first.
func (d *DB) Versions(ctx context.Context) ([]string, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT DISTINCT version FROM runs ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SummarizeRun aggregates a run's rows per mode, in AllModes order.
func (d *DB) SummarizeRun(ctx context.Context, runID int64) ([]ModeSummary, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT version, mode, COUNT(*), SUM(passed), SUM(results), SUM(chars), SUM(junk), SUM(flagged), SUM(expected_hits),
		SUM(themes_total), SUM(themes_covered), SUM(pages_ok), SUM(pages_failed), SUM(chunks_total), SUM(chunks_kept), SUM(chars_raw), SUM(dropped_covered),
		SUM(duration_ms), SUM(search_ms), SUM(tokens_in), SUM(tokens_out)
		FROM results WHERE run_id = ? AND mode != '' GROUP BY version, mode`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byMode := map[Mode]ModeSummary{}
	for rows.Next() {
		var s ModeSummary
		var mode string
		if err := rows.Scan(&s.Version, &mode, &s.Cases, &s.Passed, &s.Results, &s.Chars, &s.Junk, &s.Flagged, &s.ExpectedHits,
			&s.ThemesTotal, &s.ThemesCovered, &s.PagesOK, &s.PagesFailed, &s.ChunksTotal, &s.ChunksKept, &s.CharsRaw, &s.DroppedCovered,
			&s.DurationMs, &s.SearchMs, &s.TokensIn, &s.TokensOut); err != nil {
			return nil, err
		}
		s.Mode = Mode(mode)
		byMode[s.Mode] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []ModeSummary
	for _, m := range AllModes {
		if s, ok := byMode[m]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// CaseRow is one stored stage result.
type CaseRow struct {
	Case           string
	Tags           string
	Mode           Mode
	Passed         bool
	Results        int
	Chars          int
	Junk           int
	Flagged        int
	ExpectedHits   int
	ThemesTotal    int
	ThemesCovered  int
	PagesOK        int
	PagesFailed    int
	ChunksTotal    int
	ChunksKept     int
	CharsRaw       int
	DroppedCovered int
	DurationMs     int64
	SearchMs       int64
	TokensIn       int
	Error          string
	Failures       string
}

// RunRows returns every stage row of a run, ordered by case then mode.
func (d *DB) RunRows(ctx context.Context, runID int64) ([]CaseRow, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT "case", tags, mode, passed, results, chars, junk, flagged, expected_hits, themes_total, themes_covered,
		pages_ok, pages_failed, chunks_total, chunks_kept, chars_raw, dropped_covered, duration_ms, search_ms, tokens_in, error, failures
		FROM results WHERE run_id = ? ORDER BY "case", CASE mode WHEN 'nofilter' THEN 0 WHEN 'filter' THEN 1 ELSE 2 END`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CaseRow
	for rows.Next() {
		var r CaseRow
		var mode string
		var passed int
		if err := rows.Scan(&r.Case, &r.Tags, &mode, &passed, &r.Results, &r.Chars, &r.Junk, &r.Flagged, &r.ExpectedHits, &r.ThemesTotal, &r.ThemesCovered,
			&r.PagesOK, &r.PagesFailed, &r.ChunksTotal, &r.ChunksKept, &r.CharsRaw, &r.DroppedCovered, &r.DurationMs, &r.SearchMs, &r.TokensIn, &r.Error, &r.Failures); err != nil {
			return nil, err
		}
		r.Mode, r.Passed = Mode(mode), passed == 1
		out = append(out, r)
	}
	return out, rows.Err()
}
