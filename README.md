# smart_search

Web search from the terminal, filtered by [Jev](https://typesafe.ai) so only relevant results reach your context window.

```
You:    "latest advances in mechanistic interpretability 2025"
         │
         ▼
    ┌─────────────┐
    │  Search API  │  (exa / parallel / sonar / youcom / ddg / searxng)
    │  50 results  │
    └──────┬──────┘
           │
           ▼
    ┌─────────────┐
    │     Jev      │  typed relevance scoring
    │  per result  │  (score 0–3: topic + source quality)
    └──────┬──────┘
           │
           ▼
    ┌─────────────┐
    │  8 results   │  ✂️  low scorers dropped
    │  (relevant)  │  ✅  high scorers kept
    └─────────────┘
```

## Install

```bash
go install github.com/dorkitude/smart_search/cmd/smart_search@latest
```

## Quick start

```bash
smart_search "latest advances in mechanistic interpretability"
```

Zero config: with no keys, Exa and Parallel are used over their keyless endpoints, with DuckDuckGo as a last resort. Add a Jev key to enable filtering, and search API keys to lift rate limits:

```bash
smart_search setup        # interactive; keys stored in ~/secrets/keys.json (0600)
```

| Setting | Env var | Needed for |
|---|---|---|
| Jev key | `JEV_API_KEY` | relevance filtering, `--filter-chunks` |
| Exa / Parallel / Sonar / You.com key | `EXA_API_KEY`, `PARALLEL_API_KEY`, `SONAR_API_KEY`, `YOUCOM_API_KEY` | keyed backends (Exa, Parallel, and You.com also work without one) |
| SearXNG URL | `SEARXNG_URL` | self-hosted metasearch |

The keys file may be shared with other tools; unknown fields are preserved. Point elsewhere with `--keys-file`, `SMART_SEARCH_KEYS_FILE`, or `keys_file` in `~/smart_search/config.yaml`.

Without a Jev key, pass `--no-filter` to get raw results.

## Backends

With no `--provider`, backends are tried in order until one succeeds: configured keyed providers (exa, parallel, sonar, youcom), then Exa, Parallel, and You.com over their keyless hosted MCP endpoints, then `ddg`, then `searxng`. A provider that answers 429 is skipped for 90s within the process. Each attempt is capped at 12s and the whole chain at 30s. When a provider answers with fewer than half the requested results (throttled free tiers do this instead of erroring), the next provider is queried too and the lists are fused by reciprocal rank. Exa and Parallel return page excerpts with each result; those stand in for pages that cannot be scraped.

```bash
smart_search -p exa "transformer circuits"   # pick one
smart_search --multi "transformer circuits"  # query all, fuse with RRF, tag engines
smart_search --random "transformer circuits" # one random backend, fall back on failure
```

Duplicate hits (same URL, or the same title from several hosts such as an arXiv abstract, its PDF, and a proceedings mirror) are collapsed before filtering.

### Your own SearXNG (no quotas)

The keyless tiers throttle after a few dozen searches. A local SearXNG has no quota and aggregates Google, Bing, and others:

```bash
docker run -d --name searxng -p 8899:8080 \
  -v "$PWD/docs/searxng/settings.yml:/etc/searxng/settings.yml:ro" searxng/searxng:latest
export SEARXNG_URL=http://localhost:8899          # or: smart_search keys set searxng --value http://localhost:8899
export SMART_SEARCH_PROVIDER=searxng               # optional: try it first
```

`docs/searxng/settings.yml` enables the JSON format the client needs and turns the rate limiter off for local use.

## Filtering

```bash
smart_search --min-score 2.5 "q"                      # stricter (default 1.8 on a 0–3 scale: "Useful" or better)
smart_search --noul "Is this a peer-reviewed paper?" "q"  # yes/no question instead of a score
smart_search --rubric "off-topic,related,on-point" "q"    # custom scale
smart_search --batch "q"                              # one Jev request for all results
smart_search --no-filter "q"                          # skip Jev
```

## Scraping

```bash
smart_search --scrape "q"                    # fetch page text for each kept result
smart_search --scrape --filter-chunks "q"    # keep only the chunks Jev says are relevant
smart_search --scrape --max-chars 20000 "q"  # cap content per page (default 50000)
```

`--filter-chunks` splits each page into ~2000-char chunks, asks Jev about all of them in one batch request, and reassembles the survivors. A 50K-char page often shrinks to a few K of signal.

Reddit: `www.reddit.com` answers a plain GET with a JavaScript challenge page (HTTP 200, no content). The scraper solves it (the script's answer is the challenge token doubled), refetches with the resulting cookies, and reuses those cookies for the rest of the run. Comments ship inside a `<template>` element, which is read like any other block. Threads come back with post body and comments; a hard bot wall (HTTP 403, "Prove your humanity") is reported as an error.

## Output

```bash
smart_search --json "q" | jq '.[].url'   # JSON array (adds content/chunks_total/chunks_kept with --scrape)
smart_search --urls-only "q"             # one URL per line
smart_search --verbose "q"               # show probabilities and dropped results
smart_search -n 20 "q"                   # results to request
```

## Other commands

```bash
smart_search keys list|set|unset|validate   # non-interactive key management
smart_search eval                           # run the eval suite (live calls; see evals/README.md)
smart_search eval report --cases            # Markdown tables from evals/results.db, per version
smart_search --version                      # behavior version; bumped when results would change
```

Eval outcomes, including filter vs. no-filter numbers, are in [docs/EVAL_REPORT.md](docs/EVAL_REPORT.md).

## License

MIT
