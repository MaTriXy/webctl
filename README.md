# multi_search_web

Web search from the terminal, filtered by [Jev](https://typesafe.ai) so only relevant results reach your context window.

- [Quick start](#quick-start)
- [Schematics](#schematics)
- [Usage](#usage)

## Quick start

```bash
go install github.com/dorkitude/multi_search_web/cmd/multi_search_web@latest
multi_search_web setup        # asks for your Jev key (from typesafe.ai)
multi_search_web "latest advances in mechanistic interpretability"
```

That is the whole setup. Search itself needs no keys: it uses the keyless Exa, Parallel, and You.com endpoints, with DuckDuckGo as a fallback. Search API keys and a local SearXNG are optional extras ([docs/config.md](docs/config.md)).

## Schematics

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

## Usage

Every subsection is a summary; the linked page is the full reference. The same pages are compiled into the binary: `multi_search_web docs <topic>`.

### Search

One pipeline: search providers → dedupe → Jev scores → threshold → optional scrape → print. [docs/search.md](docs/search.md)

```bash
multi_search_web "q"
multi_search_web -n 20 "q"                   # results to request per provider
```

### Providers

Three providers per search, rankings fused by reciprocal rank. Keyed providers first, then keyless endpoints, then DuckDuckGo, then SearXNG. [docs/providers.md](docs/providers.md)

```bash
multi_search_web -p exa "q"                  # exactly one provider
multi_search_web --sources 1 "q"             # first provider that answers
multi_search_web --multi "q"                 # every available provider
```

### Filtering

Jev scores each result 0–3; the default cut is 1.8 ("Useful" or better). [docs/filtering.md](docs/filtering.md)

```bash
multi_search_web --min-score 2.5 "q"                          # stricter
multi_search_web --noul "Is this a peer-reviewed paper?" "q"  # yes/no question instead of a score
multi_search_web --rubric "off-topic,related,on-point" "q"    # custom scale
multi_search_web --no-filter "q"                              # skip Jev (works without a key)
```

### Scraping

Fetch page text for each kept result; `--filter-chunks` keeps only the ~2000-char chunks Jev says are relevant. [docs/scraping.md](docs/scraping.md)

```bash
multi_search_web --scrape "q"
multi_search_web --scrape --filter-chunks "q"
multi_search_web --scrape --max-chars 20000 "q"  # default 50000 per page
```

### Dedupe

Exact duplicates (same normalized URL or title) collapse before scoring; near-duplicates are proposed by MinHash and confirmed by Jev after. [docs/dedupe.md](docs/dedupe.md)

```bash
multi_search_web --no-dedupe "q"             # skip the near-duplicate pass
```

### Cooldowns

A provider that answers 429 or 402 is skipped for a growing window (15m → 72h), shared by every process on the machine. [docs/cooldowns.md](docs/cooldowns.md)

```bash
multi_search_web cooldown                    # who is parked, strike, window
multi_search_web cooldown clear exa          # retry now
```

### Output

```bash
multi_search_web --json "q" | jq '.[].url'   # JSON array
multi_search_web --urls-only "q"             # one URL per line
multi_search_web --verbose "q"               # probabilities and dropped results
```

### Config and keys

Flag → `MULTI_SEARCH_WEB_*` env → `~/multi_search_web/config.yaml` → default. Keys live in `~/secrets/keys.json`. [docs/config.md](docs/config.md)

```bash
multi_search_web config show                 # every setting, its value, and where it came from
multi_search_web config set min_score 2.2
multi_search_web keys list|set|unset|validate
```

### SearXNG

A local SearXNG has no quota. [docs/searxng.md](docs/searxng.md)

```bash
docker run -d --name searxng -p 8899:8080 \
  -v "$PWD/docs/searxng/settings.yml:/etc/searxng/settings.yml:ro" searxng/searxng:latest
multi_search_web keys set searxng --value http://localhost:8899
```

### Evals

Runs the cases in `evals/cases/` through the real pipeline; results in [docs/EVAL_REPORT.md](docs/EVAL_REPORT.md). [docs/evals.md](docs/evals.md)

```bash
multi_search_web eval
multi_search_web eval report --cases
```

## Providers

Chain order: your `searxng` or `degoog` if set, then any provider you set a key for, then `ketch`, then `ddg`. Three are queried per search and fused. Full table with limits and cost: `multi_search_web docs providers`.

| keyless | keyed |
|---|---|
| `ketch` (own chain of free tiers), `ddg`, `searxng` and `degoog` (your instances), `exa`, `parallel`, `youcom`, `firecrawl`, `keenable` (name with `-p` to use keyless) | `exa`, `parallel`, `sonar`, `youcom`, `brave`, `tavily`, `firecrawl`, `keenable`, `serpbase`, `serply` |

```bash
multi_search_web keys set brave        # a key puts the provider in the chain
multi_search_web -p tavily "query"     # exactly one provider
```

## License

MIT
