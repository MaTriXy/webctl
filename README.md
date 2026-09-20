# webctl

Smart web search CLI for agents, backed by [Jev](https://typesafe.ai). Saves a lot of tokens.

```bash
webctl search "final score san francisco giants september 19th baseball" \
```

...or, more likely:

```bash
webctl "San Francisco giants MLB score recent games" \
    --goal "The user asked to know the final score of last night's (September 19th) San Francisco Giants game in Major League Baseball"
```

By default, webctl will try 3 web search backends.  Each result set is passed (with the original query and goal) to Jev for scoring;  the high-scoring subset is then deduped deterministically, then passed through a Jev judge.  This means your Claude (or whatever) doesn't have to read as much junk, which saves you $$ (sorry, Anthropic!).

Optionally, webctl can also scrape the result pages so your agent doesn't have to fetch them.  It then parses out the textual content, divides it into chunks, and sends batches of those chunks (plus the original query and goal context) to Jev for scoring.  High-scoring results are returned.  For some workloads (think long PDFs, long Reddit/StackOverflow comment threads, entire Wikipedia articles, etc), this can save an *enormous* number of tokens.

Feel free to submit a PR if I missed something!  And if I miss the PR, hit me up [@dorkitude](https://x.com/dorkitude) and I'll get to it ASAP.

- [Quick start](#quick-start)
- [Installation](#installation)
- [Schematics](#schematics)
- [Usage](#usage)

## Quick start

1. Install: [Homebrew](#homebrew-macos-and-linux), [apt](#apt-debian-and-ubuntu), [npm](#npm), or [go install](#go-install).
2. Get a Jev key from [typesafe.ai](https://typesafe.ai).
3. Run:

```bash
webctl setup        # asks for your Jev key
webctl search "mechanistic interpretability 2026" --goal "Recent papers on sparse autoencoders and circuit analysis"
```

Searching does not require keys for hobbyist-level usage: with none configured, webctl runs through [ketch](https://github.com/1broseidon/ketch) (its own chain of free tiers) and DuckDuckGo, and backs off from rate limits with a cooldown ladder.

The author prefers [Brave Search](https://brave.com/search/api/): 5,000 free searches a month, requires an API key. It is the first choice in `webctl setup`; once any search key is configured, only your keyed providers are used. ([docs/providers.md](docs/providers.md))

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
      │  ~/webctl/cooldown.json                     │  shared by every process
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

## Installation

Prebuilt binaries for macOS and Linux (amd64 and arm64) are attached to every [release](https://github.com/dorkitude/webctl/releases).

### Homebrew (macOS and Linux)

```bash
brew install dorkitude/webctl/webctl
```

### apt (Debian and Ubuntu)

```bash
curl -fsSL https://dorkitude.github.io/webctl-apt/key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/webctl.gpg
echo "deb [signed-by=/usr/share/keyrings/webctl.gpg] https://dorkitude.github.io/webctl-apt stable main" | sudo tee /etc/apt/sources.list.d/webctl.list
sudo apt update && sudo apt install webctl
```

### npm

```bash
npm install -g webctl
```

### go install

```bash
go install github.com/dorkitude/webctl/cmd/webctl@latest
```

## Usage

### Search

One pipeline: search providers → dedupe → Jev scores → threshold → optional scrape → print. [docs/search.md](docs/search.md)

```bash
webctl search "q" --goal "what you actually need"
webctl search "q" -n 20            # results to request per provider
```

### Providers

Up to three providers per search, rankings fused by reciprocal rank: your SearXNG or Degoog if set, then the providers you set a key for (Brave first); ketch and DuckDuckGo only when no key exists. [docs/providers.md](docs/providers.md)

```bash
webctl search "q" -p exa           # exactly one provider
webctl search "q" --sources 1      # first provider that answers
webctl search "q" --multi          # every available provider
```

### Filtering

Jev scores each result 0–10; the default cut is 6 ("useful" or better). [docs/filtering.md](docs/filtering.md)

```bash
webctl search "q" --min-score 8.5                          # stricter
webctl search "q" --min-results 5                          # never fewer than 5 (marked backfilled)
webctl search "q" --noul "Is this a peer-reviewed paper?"  # yes/no question instead of a score
webctl search "q" --rubric "off-topic,related,on-point"    # custom scale
webctl search "q" --no-filter                              # skip Jev (works without a key)
```

### Scraping

Fetch page text for each kept result; `--filter-chunks` keeps only the ~2000-char chunks Jev says are relevant. [docs/scraping.md](docs/scraping.md)

```bash
webctl search "q" --scrape
webctl search "q" --scrape --filter-chunks
webctl search "q" --scrape --max-chars 20000  # default 50000 per page
```

### Dedupe

Exact duplicates (same normalized URL or title) collapse before scoring; near-duplicates are proposed by MinHash and confirmed by Jev after. [docs/dedupe.md](docs/dedupe.md)

```bash
webctl search "q" --no-dedupe      # skip the near-duplicate pass
```

### Cooldowns

A provider that answers 429 or 402 is skipped for a growing window (15m → 72h), shared by every process on the machine. [docs/cooldowns.md](docs/cooldowns.md)

```bash
webctl cooldown                    # who is parked, strike, window
webctl cooldown clear exa          # retry now
```

### Output

```bash
webctl search "q" --json | jq '.[].url'   # JSON array
webctl search "q" --urls-only             # one URL per line
webctl search "q" --verbose               # confidence, probabilities, dropped results
```

### Config and keys

Flag → `WEBCTL_*` env → `~/webctl/config.yaml` → default. Keys live in `~/secrets/keys.json`. [docs/config.md](docs/config.md)

```bash
webctl config show                 # every setting, its value, and where it came from
webctl config set min_score 2.2
webctl keys list|set|unset|validate
```

### SearXNG

A local SearXNG has no quota. [docs/searxng.md](docs/searxng.md)

```bash
docker run -d --name searxng -p 8899:8080 \
  -v "$PWD/docs/searxng/settings.yml:/etc/searxng/settings.yml:ro" searxng/searxng:latest
webctl keys set searxng --value http://localhost:8899
```

### Evals

Runs the cases in `evals/cases/` through the real pipeline; results in [docs/EVAL_REPORT.md](docs/EVAL_REPORT.md). [docs/evals.md](docs/evals.md)

```bash
webctl eval
webctl eval report --cases
```

## Providers

Chain order: your `searxng` or `degoog` if set, then the providers you set a key for. `ketch` and `ddg` are used only when no key is set. Up to three are queried per search and fused. Full table with limits and cost: `webctl docs providers`.

| keyless | keyed |
|---|---|
| `ketch` (own chain of free tiers), `ddg`, `searxng` and `degoog` (your instances), `exa`, `parallel`, `youcom`, `firecrawl`, `keenable` (name with `-p` to use keyless) | `exa`, `parallel`, `sonar`, `youcom`, `brave`, `tavily`, `firecrawl`, `keenable`, `serpbase`, `serply` |

```bash
webctl keys set brave        # a key puts the provider in the chain
webctl search "query" -p tavily     # exactly one provider
```

## License

MIT
