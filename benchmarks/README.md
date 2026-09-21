# Benchmarks

Measures what webctl saves a real agent, and what it costs in answer quality.

## What runs

Each case in `cases/` is a research question. Every arm gets the same question with one difference in the prompt: which tools it may use.

| arm | harness | model | reaches the web through |
|---|---|---|---|
| `claude-sonnet-webctl` | Claude Code | sonnet | `webctl` only (Bash restricted to `webctl`, WebSearch/WebFetch disallowed) |
| `claude-sonnet-native` | Claude Code | sonnet | built-in WebSearch/WebFetch only (Bash disallowed) |
| `codex-terra-*` (not default) | Codex | gpt-5.6-terra | same three arms; `--arms all` adds them. Left out of the results because Codex's native search is server-side and what it puts into context cannot be observed |
| `pi-kimi-k3-webctl` | pi | Kimi K3 (Fireworks) | `webctl` only (pi has no built-in search) |

Per arm and case it records wall clock, tokens (uncached input, cache read, cache write, output), cost where the harness reports it, tool calls, and violations (a native search in a webctl arm, or a shell command in a native arm).

Kimi K3, via pi, grades all answers to a case in one call, blind to arm and in shuffled order, 0–10 against the case's expected facts or rubric. Recency cases have no fixed facts; the judge uses agreement between answers plus sourcing.

## Cases

30 cases across dev docs on complex products (Kafka, Postgres, Kubernetes, ClickHouse, Redis, Rust, Terraform, SQLite, Go), long documents (RFCs, PEP 8, a paper PDF), community-sourced answers (Reddit, Hacker News), science and policy facts, and recency (earnings calls, Fed decisions, F1, Kubernetes releases, OpenAI news).

A case file:

```yaml
name: redis-xautoclaim
domain: dev-docs          # dev-docs, long-doc, community, science, policy, news, earnings
question: In Redis Streams, what does XAUTOCLAIM do ... ?
expected:                 # facts the judge checks; or use rubric: for opinion questions
  - Introduced in Redis 6.2
recency: false            # true: graded by cross-answer agreement and sourcing
```

## Running

```bash
go build -o webctl-bench ./cmd/webctl-bench
./webctl-bench list
./webctl-bench run                                  # full matrix, ~2 h
./webctl-bench run --cases 'kafka-*' --arms claude-sonnet-webctl,claude-sonnet-native
./webctl-bench run --resume latest                  # fill in missing or failed cells
./webctl-bench report latest                        # Markdown to stdout
```

Runs are JSON under `~/webctl/benchmarks/`, one file per run, with raw harness and judge output in a directory beside it. The run file is rewritten after every cell.

Requirements: `claude`, `codex`, `pi`, and `webctl` on PATH, logged in; a Fireworks key in pi's `models.json` for Kimi K3.

## Findings (2026-09-20, experiments 01–03)

Search payload per case, chars of search results returned into the agent's context ÷ 4, with judged quality. Same 30 cases, one run per cell.

| arm | payload tokens | quality | cost/case |
|---|---|---|---|
| Claude native (WebSearch + WebFetch) | 1.0k | 8.53 | $0.138 |
| Claude webctl, no scrape | 1.5k | 9.40 | $0.097 |
| Claude webctl, `--scrape --filter-chunks` (top 3) | 4.6k | 9.47 | $0.112 |
| Codex native | hidden (server-side) | 9.37 | |
| Codex webctl, no scrape | 2.0k | 9.20 | |
| Codex webctl, scrape | 6.8k | 9.37 | |

- webctl without scraping is the like-for-like comparison: 50% more payload than Claude's native links, one point better quality, lowest cost. Claude's native payload is links plus a hidden summary; the reading happens in sub-calls that are billed but not shown.
- Scraping triples the payload for 0.1 points on average. It pays on earnings calls (Claude 9.0 → 10.0), community threads (9.0 → 9.6), and long documents; it does nothing on dev docs, news, or short facts.
- Before `--max-output` and `--scrape-top` (experiment 01), scrape output was a median 52 KB per call and Claude Code cut 26 of 41 of them to a 2 KB preview. Bounding it (experiment 02) took Claude from 8.83 to 9.33 and cut total tokens 28%.
- Total tokens per case are dominated by the harness's own per-turn baseline (about 20k on Claude, 16k on Codex); compare payload, not totals, when comparing search tools.

Details: `experiments/*/README.md`; raw cells in `experiments/*/results/cells.jsonl`.

## Reading the report

The first table is the point: per harness, native versus webctl on mean wall clock, mean tokens, mean cost, mean quality, and how many answers cited a source. Tokens count everything the model attended to across all turns, cached or not, since caching changes price but not context use.

Caveats:
- webctl's own Jev calls are not in the token counts. They are billed to the Jev key, not the agent's context; `webctl -v` prints them per search.
- Native arms use server-side search (Claude, Codex), whose result text is billed as input tokens but not itemised.
- Recency grades rest on agreement between arms, so a case where every arm is wrong the same way scores high.
- One run per cell. Agents are not deterministic; run twice before trusting a difference under about 1 point or 20%.
