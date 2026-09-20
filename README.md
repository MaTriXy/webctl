# multi_search_web

Web search from the terminal, filtered by [Jev](https://typesafe.ai) so only relevant results reach your context window.

```
You:    "latest advances in mechanistic interpretability 2025"
              │
              ▼
      ┌──────────────┐
      │  Search API  │  (exa / parallel / sonar / youcom / ddg / searxng)
      │  25 results  │
      └───────┬──────┘
              │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │
              ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼
      ┌───────────────────────────────────────────────────────┐
      │                          Jev                          │  typed relevance scoring
      │        "is this on-topic, from a good source?"        │
      └───────┬───────┬───────┬───────┬───────┬───────┬───────┘
              │       │       │       │       │       │   ✂️  the rest dropped
              ▼       ▼       ▼       ▼       ▼       ▼
      ┌────────────────────────────────────────────────────┐
      │              10–15 results (relevant)              │  ✅ kept
      └────────────────────────────────────────────────────┘

--scrape --filter-chunks gives each kept page the same treatment:

      ┌──────────────┐
      │ Scraped page │  (~2000 chars per chunk)
      │  25 chunks   │
      └───────┬──────┘
              │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │ │
              ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼ ▼
      ┌───────────────────────────────────────────────────────┐
      │                          Jev                          │  one batch request per page
      │         "is this chunk relevant to the query?"        │
      └───────┬───────┬───────┬───────┬───────────────────────┘
              │       │       │       │   ✂️  irrelevant chunks dropped
              ▼       ▼       ▼       ▼
      ┌──────────────────────────────────┐
      │      3–6 chunks (relevant)       │  ✅ reassembled as the page text
      └──────────────────────────────────┘
```

## Install

```bash
go install github.com/dorkitude/multi_search_web/cmd/multi_search_web@latest
```

## Quick start

```bash
multi_search_web "latest advances in mechanistic interpretability"
```

Zero config: with no keys, Exa and Parallel are used over their keyless endpoints, with DuckDuckGo as a last resort. Add a Jev key to enable filtering, and search API keys to lift rate limits:

```bash
multi_search_web setup        # interactive; keys stored in ~/secrets/keys.json (0600)
```

| Setting | Env var | Needed for |
|---|---|---|
| Jev key | `JEV_API_KEY` | relevance filtering, `--filter-chunks` |
| Exa / Parallel / Sonar / You.com key | `EXA_API_KEY`, `PARALLEL_API_KEY`, `SONAR_API_KEY`, `YOUCOM_API_KEY` | keyed backends (Exa, Parallel, and You.com also work without one) |
| SearXNG URL | `SEARXNG_URL` | self-hosted metasearch |

The keys file may be shared with other tools; unknown fields are preserved. Point elsewhere with `--keys-file`, `MULTI_SEARCH_WEB_KEYS_FILE`, or `keys_file` in `~/multi_search_web/config.yaml`.

Without a Jev key, pass `--no-filter` to get raw results.

## Backends

Every search gathers results from three providers (`sources`, configurable) and fuses their rankings by reciprocal rank, so one engine's blind spot or bad day is covered by the others and each free tier carries a third of the load. Providers are taken in this order, skipping any that are cooling down: configured keyed providers (exa, parallel, sonar, youcom), then Exa, Parallel, and You.com over their keyless hosted MCP endpoints, then `ddg`, then `searxng`. A provider that fails or answers empty is replaced by the next one; if fewer than three are available, fewer are used. Each attempt is capped at 12s and the whole search at 30s. Results carry the engines that returned them.

```bash
multi_search_web --sources 1 "transformer circuits"   # plain fallback chain: first provider that answers
multi_search_web -p exa "transformer circuits"        # exactly one named provider
multi_search_web --multi "transformer circuits"       # every available provider
multi_search_web --random "transformer circuits"      # same, in random order
multi_search_web config set sources 2                 # persist a default
```

Duplicate hits (same URL, or the same title from several hosts such as an arXiv abstract, its PDF, and a proceedings mirror) are collapsed before filtering.

### Cooldowns for rate-limiting

The keyless tiers meter by the day. When a provider answers 429 (rate limited) or 402 (quota spent), it is skipped for a window that grows with each consecutive failure:

```
15m → 1h → 4h → 12h → 24h → 72h
```

A 402 starts at the third step (4h) because a spent quota does not come back in fifteen minutes. At the top of the ladder the provider stays parked, and one probe request is allowed every 24h; a failed probe re-arms the 72h, a success resets everything. State lives in `~/multi_search_web/cooldown.json`, so every process on the machine honors it, and keyed and keyless use of a provider are tracked separately: adding a key clears that provider's cooldown.

A skipped provider is mentioned once an hour on stderr (every time with `--verbose`):

```
exa skipped: cooling down until Mon 14:20 (rate limited 12m ago, strike 2 of 6); retry now with `multi_search_web cooldown clear exa`, adjust with `multi_search_web config set cooldown.steps ...`
```

```bash
multi_search_web cooldown                            # who is parked, strike, window
multi_search_web cooldown clear [exa]                # forget it; next search retries
multi_search_web config set cooldown.steps 30m,2h,8h,24h,72h
multi_search_web config set cooldown.probe_interval 12h
multi_search_web config set cooldown.quota_start 4
multi_search_web config set cooldown.enabled false   # always try every provider
```

If every provider is cooling down and no SearXNG is configured, the search fails with exit 1 and names the earliest retry time.

### Your own SearXNG (no quotas)

The keyless tiers throttle after a few dozen searches. A local SearXNG has no quota and aggregates Google, Bing, and others:

```bash
docker run -d --name searxng -p 8899:8080 \
  -v "$PWD/docs/searxng/settings.yml:/etc/searxng/settings.yml:ro" searxng/searxng:latest
export SEARXNG_URL=http://localhost:8899          # or: multi_search_web keys set searxng --value http://localhost:8899
export MULTI_SEARCH_WEB_PROVIDER=searxng               # optional: try it first
```

`docs/searxng/settings.yml` enables the JSON format the client needs and turns the rate limiter off for local use.

## Filtering

```bash
multi_search_web --min-score 2.5 "q"                      # stricter (default 1.8 on a 0–3 scale: "Useful" or better)
multi_search_web --noul "Is this a peer-reviewed paper?" "q"  # yes/no question instead of a score
multi_search_web --rubric "off-topic,related,on-point" "q"    # custom scale
multi_search_web --batch "q"                              # one Jev request for all results
multi_search_web --no-filter "q"                          # skip Jev
```

## Scraping

```bash
multi_search_web --scrape "q"                    # fetch page text for each kept result
multi_search_web --scrape --filter-chunks "q"    # keep only the chunks Jev says are relevant
multi_search_web --scrape --max-chars 20000 "q"  # cap content per page (default 50000)
```

`--filter-chunks` splits each page into ~2000-char chunks, asks Jev about all of them in one batch request, and reassembles the survivors. A 50K-char page often shrinks to a few K of signal.

Reddit: `www.reddit.com` answers a plain GET with a JavaScript challenge page (HTTP 200, no content). The scraper solves it (the script's answer is the challenge token doubled), refetches with the resulting cookies, and reuses those cookies for the rest of the run. Comments ship inside a `<template>` element, which is read like any other block. Threads come back with post body and comments; a hard bot wall (HTTP 403, "Prove your humanity") is reported as an error.

## Output

```bash
multi_search_web --json "q" | jq '.[].url'   # JSON array (adds content/chunks_total/chunks_kept with --scrape)
multi_search_web --urls-only "q"             # one URL per line
multi_search_web --verbose "q"               # show probabilities and dropped results
multi_search_web -n 20 "q"                   # results to request
```

## Other commands

```bash
multi_search_web config show                    # every setting, its value, and where it came from
multi_search_web config set provider parallel   # persist a default in ~/multi_search_web/config.yaml
multi_search_web config set min_score 2.2       # also: num, jev.base_url, jev.model, searxng_url, keys_file
multi_search_web keys list|set|unset|validate   # non-interactive key management
multi_search_web eval                           # run the eval suite (live calls; see evals/README.md)
multi_search_web eval report --cases            # Markdown tables from evals/results.db, per version
multi_search_web --version                      # behavior version; bumped when results would change
```

Eval outcomes, including filter vs. no-filter numbers, are in [docs/EVAL_REPORT.md](docs/EVAL_REPORT.md).

## License

MIT
