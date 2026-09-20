# Search

`webctl [flags] "<query>"` runs one pipeline: search providers → fold exact duplicates → Jev scores each result → fold near-duplicates → apply the threshold → optionally scrape and chunk-filter → print.

## Pipeline

1. **Search.** Up to `sources` providers (default 3) are queried in concurrent waves and their lists fused by reciprocal rank. See `providers`.
2. **Exact dedupe.** Results with the same normalized URL or title collapse to one. See `dedupe`.
3. **Score.** Jev scores each result 0–3 on topic and source quality. See `filtering`.
4. **Near-dedupe.** MinHash proposes look-alike pairs; Jev confirms them in one request; groups keep the best copy.
5. **Threshold.** Results below `min_score` (default 1.8) are dropped. `--verbose` shows them anyway, marked.
6. **Scrape** (optional). Kept pages are fetched; `--filter-chunks` keeps only relevant chunks. See `scraping`.
7. **Print.** Terminal text, `--json`, or `--urls-only`.

## Flags

| flag | effect |
|---|---|
| `-n, --num N` | results to request from each provider (default 10; setting `num`) |
| `--sources N` | providers to query and fuse (default 3; setting `sources`); `-p` forces 1 |
| `-p, --provider NAME` | exactly one provider: exa, parallel, sonar, youcom, ddg, searxng |
| `--multi` | every available provider |
| `--random` | every available provider, tried in random order |
| `-m, --min-score X` | keep results scoring ≥ X on the rubric (default 1.8); with `--noul`, minimum P(yes) (default 0.5) |
| `--min-results N` | if fewer than N pass the cut, promote the best of the rest (never below 1.0); promoted results are marked `backfilled` |
| `--rubric "a,b,c"` | custom score levels, lowest to highest; default cut is 0.2 below the second-highest level |
| `--noul "question?"` | ask a yes/no question per result instead of scoring |
| `--batch` | score every result in one Jev request instead of one request per result |
| `--no-filter` | skip Jev; print provider results as fused |
| `--no-dedupe` | skip the Jev near-duplicate pass (exact dedupe still runs) |
| `--scrape` | fetch each kept result's page text |
| `--filter-chunks` | with `--scrape`, keep only chunks Jev judges worth quoting |
| `--max-chars N` | with `--scrape`, cap text per page (default 50000) |
| `--json` | JSON array on stdout; diagnostics stay on stderr |
| `--urls-only` | one URL per line |
| `-v, --verbose` | show scores, probabilities, dropped results, and every cooldown notice |
| `--config-dir DIR` | config directory (default `~/webctl`) |
| `--keys-file FILE` | keys file (default `~/secrets/keys.json`) |

## Output

Terminal: one block per kept result with title, host, URL, score, confidence, engines, snippet, and `Duplicate:` lines for folded copies. A summary line on stderr: `exa+parallel: 15 results → 6 kept (min score 1.8)`, then `N duplicate(s) folded` and Jev token usage.

`--json`: an array of objects with `title`, `url`, `snippet`, `score`, `max_score`, `confidence`, `probabilities`, `kept`, `engines`, `duplicates`, and with `--scrape` `content`, `scrape_error`, `chunks_total`, `chunks_kept`, `filter_error`. With `--noul`: `yes` and `probability` instead of score fields. With `--no-filter`: `title`, `url`, `snippet`, `content`, `engines`.

## Exit codes

0 success (including zero kept results), 1 any failure: no provider answered, Jev unreachable or key rejected, bad flags. Fall-throughs and cooldown skips are not failures.

## Examples

```
webctl "postgres autovacuum tuning for high-update tables"
webctl -n 20 -m 2.5 --json "DPO vs RLHF" | jq '.[].url'
webctl --noul "Is this a peer-reviewed paper?" "sparse autoencoders"
webctl --scrape --filter-chunks "kubernetes OOMKilled below memory limit"
webctl --sources 1 -v "why is my Go http server leaking goroutines"
```
