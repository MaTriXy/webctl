# multi_search_web

Web search from the terminal, filtered by [Jev](https://typesafe.ai) so only relevant results reach your context window.

- [Quick start](#quick-start)
- [How it works](#how-it-works)
- [Backends](#backends)
- [Filtering](#filtering)
- [Scraping](#scraping)
- [Output](#output)
- [Docs](#docs)
- [Other commands](#other-commands)

## Quick start

```bash
go install github.com/dorkitude/multi_search_web/cmd/multi_search_web@latest
multi_search_web setup        # asks for your Jev key; stored in ~/secrets/keys.json (0600)
multi_search_web "latest advances in mechanistic interpretability"
```

Required: a Jev key from [typesafe.ai](https://typesafe.ai). Search runs on the keyless Exa, Parallel, and You.com endpoints, with DuckDuckGo as a last resort. Search API keys and a local SearXNG are optional.

| Setting | Env var | Needed for |
|---|---|---|
| Jev key | `JEV_API_KEY` | relevance filtering, `--filter-chunks` |
| Exa / Parallel / Sonar / You.com key | `EXA_API_KEY`, `PARALLEL_API_KEY`, `SONAR_API_KEY`, `YOUCOM_API_KEY` | keyed backends (Exa, Parallel, and You.com also work without one) |
| SearXNG URL | `SEARXNG_URL` | self-hosted metasearch |

The keys file may be shared with other tools; unknown fields are preserved. Point elsewhere with `--keys-file`, `MULTI_SEARCH_WEB_KEYS_FILE`, or `keys_file` in `~/multi_search_web/config.yaml`.

`--no-filter` returns raw fused results and is the only mode that works without a Jev key.

## How it works

### Schematics

Filter search engine results to save tokens:

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
```

Filter chunks of scraped webpages to save even more tokens (`--scrape --filter-chunks`):

```
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

Deduplicate results:

```
      ┌──────────────┐
      │  3 engines   │
      │  45 results  │
      └───────┬──────┘
              │  pass 1: same normalized URL or title → collapsed  ✂️
              ▼
      ┌──────────────┐
      │  25 results  │  ──►  Jev scores them (diagram 1)
      └───────┬──────┘
              │  pass 2: MinHash LSH over each excerpt
              ▼
      ┌───────────────────────────────────────────────────────┐
      │  candidate pairs (a few)                              │  A~B  C~D  E~F
      └───────┬───────────────┬───────────────┬───────────────┘
              ▼               ▼               ▼
      ┌───────────────────────────────────────────────────────┐
      │                          Jev                          │  one batch request
      │           "are these two the same content?"           │
      └───────┬───────────────┬───────────────────────────────┘
              │ yes           │ yes            ✂️  no: both stay
              ▼               ▼
      ┌───────────────────────────────────────────────────────┐
      │  best-scored copy kept, engine tags merged            │  ✅ others listed as duplicates
      └───────────────────────────────────────────────────────┘
```

Respect rate limits with automatic cooldowns:

```
      search ──► exa ──► HTTP 429 (rate limited) or 402 (quota spent)
                                    │
                                    ▼
      ┌───────────────────────────────────────────────────────┐
      │  ~/multi_search_web/cooldown.json                     │  shared by every process
      │  exa: strike 1, skip until +15m                       │
      └───────────────────────────────────────────────────────┘
                                    │
                                    ▼
      next search ──► exa (skipped) ──► parallel ──► youcom ──► ddg ──► searxng

      strike     1       2       3        4        5        6
      window    15m ──► 1h ──► 4h ──► 12h ──► 24h ──► 72h ──► parked: one probe per 24h
                                ▲
                                └── a 402 starts here
      any success ──► strikes reset to 0
```

## Backends

Every search gathers results from three providers (`sources`, configurable) and fuses their rankings by reciprocal rank. Providers are taken in this order, skipping any that are cooling down: configured keyed providers (exa, parallel, sonar, youcom), then Exa, Parallel, and You.com over their keyless hosted MCP endpoints, then `ddg`, then `searxng`. A provider that fails or answers empty is replaced by the next one; if fewer than three are available, fewer are used. Each attempt is capped at 12s and the whole search at 30s. Results carry the engines that returned them.

```bash
multi_search_web --sources 1 "transformer circuits"   # plain fallback chain: first provider that answers
multi_search_web -p exa "transformer circuits"        # exactly one named provider
multi_search_web --multi "transformer circuits"       # every available provider
multi_search_web --random "transformer circuits"      # same, in random order
multi_search_web config set sources 2                 # persist a default
```

### Duplicates

Two passes:

1. Before Jev sees anything, results with the same normalized URL (no `www.`/`m.`/`amp.`, no tracking parameters) or the same title are collapsed.
2. After scoring, near-duplicate pairs are proposed by MinHash locality-sensitive hashing over each result's excerpt and confirmed by Jev in one batch request. Confirmed groups keep the best-scored copy, merge the engine tags, and list the others under `duplicates` in `--json` and `Duplicate:` lines in the terminal.

`--no-dedupe` skips the second pass.

### Cooldowns for rate-limiting

When a provider answers 429 (rate limited) or 402 (quota spent), it is skipped for a window that grows with each consecutive failure:

```
15m → 1h → 4h → 12h → 24h → 72h
```

A 402 starts at the third step (4h). At the top of the ladder the provider stays parked, and one probe request is allowed every 24h; a failed probe re-arms the 72h, a success resets everything. State lives in `~/multi_search_web/cooldown.json`, so every process on the machine honors it. Keyed and keyless use of a provider are tracked separately: adding a key clears that provider's cooldown.

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

A local SearXNG has no quota and aggregates Google, Bing, and others:

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

`--filter-chunks` splits each page into ~2000-char chunks, asks Jev about all of them in one batch request, and reassembles the survivors.

Reddit threads scrape with post body and comments; a hard bot wall (HTTP 403, "Prove your humanity") is reported as an error.

## Output

```bash
multi_search_web --json "q" | jq '.[].url'   # JSON array (adds content/chunks_total/chunks_kept with --scrape)
multi_search_web --urls-only "q"             # one URL per line
multi_search_web --verbose "q"               # show probabilities and dropped results
multi_search_web -n 20 "q"                   # results to request
```

## Docs

`--help` is short. The full reference is compiled into the binary and mirrors `docs/`:

```bash
multi_search_web docs            # topics
multi_search_web docs providers  # one page
multi_search_web docs all        # everything, in reading order
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
