# smart_search

A web search CLI that queries **Exa**, **Parallel**, or **Sonar** (Perplexity) and passes the results through **[Jev](https://typesafe.ai)** — TypeSafe's System One model — for typed, probabilistic relevance qualification. Jev scores each result; low-scoring results are filtered out, saving you context tokens downstream.

## Why?

Web search APIs return a pile of results. Some are gold, most are noise. Instead of dumping everything into your LLM's context window, `smart_search` runs each result past Jev's typed judgment engine. You set the bar; Jev clears the field.

```
You:    "latest advances in mechanistic interpretability 2025"
         │
         ▼
    ┌─────────────┐
    │  Search API  │  (exa / parallel / sonar)
    │  10 results  │
    └──────┬──────┘
           │
           ▼
    ┌─────────────┐
    │     Jev      │  typed relevance scoring
    │  per result  │  (score 0–N, confidence 0–1)
    └──────┬──────┘
           │
           ▼
    ┌─────────────┐
    │  3 results   │  ✂️  low scorers dropped
    │  (relevant)  │  ✅  high scorers kept
    └─────────────┘
```

## Requirements

You need **one** search provider key **and** one Jev key:

| Key | Where to get it | Purpose |
|-----|----------------|---------|
| `EXA_API_KEY` | [exa.ai](https://exa.ai) | Web search via Exa |
| `PARALLEL_API_KEY` | [parallel.ai](https://parallel.ai) | Web search via Parallel |
| `SONAR_API_KEY` | [perplexity.ai](https://perplexity.ai) | Web search via Sonar (Perplexity) |
| `JEV_API_KEY` | [typesafe.ai](https://typesafe.ai) | Typed result qualification via Jev |

> **At least one** search provider key is required. All three are supported — pick your favorite, or configure multiple and switch with `--provider`.

## Install

```bash
go install github.com/dorkitude/smart_search@latest
```

Or build from source:

```bash
git clone https://github.com/dorkitude/smart_search.git
cd smart_search
go build -o smart_search ./cmd/smart_search
```

## Setup

Interactive setup walks you through adding keys. Input is masked, keys are validated with a lightweight hello-world call, and stored in `~/smart_search/keys.json` (mode `0600`).

```bash
smart_search setup
```

```
=== smart_search setup ===

Search providers (need at least one):
  1) Exa
  2) Parallel
  3) Sonar (Perplexity)
  s) Skip to Jev key

Choose a provider to configure: 1
  Exa API key: 🔒 ************************************************
  ✓ Key validated successfully

Choose a provider to configure: s

Jev (TypeSafe) configuration:
  Jev API key: 🔒 ************************************************
  ✓ Key validated successfully

Keys saved to ~/smart_search/keys.json
```

Re-run `smart_search setup` any time to add, rotate, or re-validate keys.

### Manual key management

Keys live in `~/smart_search/keys.json`:

```json
{
  "exa_api_key": "your-exa-key",
  "parallel_api_key": "",
  "sonar_api_key": "",
  "jev_api_key": "your-jev-key"
}
```

Empty strings mean "not configured." You can also set environment variables — they take precedence over the file:

```bash
export EXA_API_KEY="..."
export JEV_API_KEY="..."
```

## Usage

### Basic search

```bash
smart_search "latest advances in mechanistic interpretability"
```

This will:
1. Query your configured search provider
2. Send the query + each result's title, URL, and snippet to Jev as a **score question**
3. Print results that meet the relevance threshold, sorted by Jev score

### Choose a provider

```bash
smart_search --provider exa "transformer circuits"
smart_search --provider parallel "transformer circuits"
smart_search --provider sonar "transformer circuits"
```

### Control the relevance threshold

Results are scored on a 0–3 scale by default. The `--min-score` flag sets the cutoff:

```bash
# Only keep highly relevant results
smart_search --min-score 2.0 "transformer circuits"

# Accept everything (no filtering)
smart_search --min-score 0 "transformer circuits"
```

### Custom Jev rubric

Override the scoring criteria with `--rubric`:

```bash
smart_search --rubric "irrelevant,somewhat relevant,highly relevant,exactly what I need" \
  "sparse autoencoders for feature discovery"
```

Or pass a noul (yes/no) question instead:

```bash
smart_search --noul "Is this result about machine interpretability research?" \
  "sparse autoencoders"
```

### Output formats

```bash
# Default: pretty-printed with scores
smart_search "attention is all you need"

# JSON for piping
smart_search --json "attention is all you need" | jq '.[].url'

# Just URLs
smart_search --urls-only "attention is all you need"

# Raw search results (skip Jev entirely)
smart_search --no-filter "attention is all you need"
```

### Number of results

```bash
smart_search --num 20 "attention is all you need"
```

### Verbose mode

See Jev's reasoning for each result, including confidence and score probabilities:

```bash
smart_search --verbose "attention is all you need"
```

```
[1] Attention Is All You Need — arxiv.org
    Score: 2.87 / 3  (confidence: 0.94)
    Probabilities: {0: 0.01, 1: 0.02, 2: 0.15, 3: 0.82}
    ✓ Kept

[2] Some SEO blog — content-farm.com
    Score: 0.31 / 3  (confidence: 0.91)
    Probabilities: {0: 0.72, 1: 0.25, 2: 0.02, 3: 0.01}
    ✗ Filtered (below 1.0 threshold)
```

## How Jev qualification works

By default, `smart_search` sends a **score question** to Jev for each search result:

```json
{
  "model": "jev-latest",
  "state": {
    "query": "latest advances in mechanistic interpretability",
    "result": {
      "title": "Sparse Autoencoders Find Highly Interpretable Features",
      "url": "https://arxiv.org/abs/...",
      "snippet": "We demonstrate that sparse autoencoders can..."
    }
  },
  "questions": {
    "relevance": {
      "type": "score",
      "instructions": "How relevant is this search result to the user's query?",
      "criteria": [
        "Completely irrelevant — different topic entirely",
        "Tangentially related — mentions keywords but doesn't address the query",
        "Relevant — directly addresses the query with substantive content",
        "Highly relevant — exactly what the user is looking for"
      ]
    }
  }
}
```

Jev returns a typed score with calibrated probabilities. Results below `--min-score` are dropped. This is fast (~300ms per result) and cheap — Jev is designed for high-volume typed judgments.

### Batched mode

For speed, use `--batch` to send all results in one Jev request:

```bash
smart_search --batch "transformer circuits"
```

This uses a single Jev call with one question per result, reducing latency at the cost of less granular per-result state.

## Architecture

```
cmd/smart_search/
  main.go                  # entrypoint
  cli/
    root.go                # cobra root command
    search.go              # search command
    setup.go               # interactive setup wizard
    keys.go                # key management helpers
internal/
  provider/
    provider.go            # Provider interface
    exa.go                 # Exa implementation
    parallel.go            # Parallel implementation
    sonar.go               # Sonar (Perplexity) implementation
  jev/
    client.go              # Jev API client
    qualify.go             # Result qualification logic
    types.go               # Request/response types
  config/
    config.go              # Viper config + keys.json
```

### Provider interface

```go
type Provider interface {
    Name() string
    Search(ctx context.Context, query string, numResults int) ([]SearchResult, error)
}

type SearchResult struct {
    Title   string `json:"title"`
    URL     string `json:"url"`
    Snippet string `json:"snippet"`
}
```

### Jev client

```go
type Client struct {
    APIKey  string
    BaseURL string // default: https://api.typesafe.ai
    Model   string // default: jev-latest
}

// ScoreResult sends a single search result to Jev for relevance scoring.
func (c *Client) ScoreResult(ctx context.Context, query string, result SearchResult, rubric []string) (*ScoreAnswer, error)

// ScoreBatch sends multiple results in one request.
func (c *Client) ScoreBatch(ctx context.Context, query string, results []SearchResult, rubric []string) (map[string]*ScoreAnswer, error)
```

## Dependencies

- **[Cobra](https://github.com/spf13/cobra)** — CLI framework
- **[Viper](https://github.com/spf13/viper)** — Configuration management
- **[golang.org/x/term](https://pkg.go.dev/golang.org/x/term)** — Masked terminal input for API keys

No heavy SDK dependencies — the search and Jev APIs are simple REST endpoints called with `net/http`.

## License

MIT
