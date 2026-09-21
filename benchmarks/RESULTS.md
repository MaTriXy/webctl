# Benchmark results

webctl 0.1.6 (behavior version 0.0.029). 30 research questions, two agent harnesses, one run per cell, 2026-09-20 and 21. Judge: Kimi K3 via pi, blind to arm, all arms of a case graded in one call.

Durable evidence lives in `experiments/<name>/results/`: `cells.jsonl` (one line per case × arm: answer, tokens, cost, payload, tool calls, verdict), `meta.json`, and `report.md`. The tables below come from `experiments/2026-09-21-04-summarize`, which holds every arm; `webctl-bench report 2026-09-21-04-summarize` regenerates the full report. Earlier experiments are prior states of the same cases, kept for the record.

Arms: Claude Code was told to use only its own web search (native), only `webctl search` (no scrape), webctl with `--scrape --filter-chunks` (scrape), or webctl with `--scrape --filter-chunks --summarize` (scrape + summarize, DeepSeek V4 Flash on Fireworks as the summarizer). pi has no built-in search. Codex was run too (cells are in the data) but is left out here: its native search is server-side, so what it puts into context cannot be observed.

## Headline

Search payload is what the search tool returned into the agent's context, in tokens (chars ÷ 4). Total tokens include the harness's own per-turn baseline, about 20k per turn on Claude, which no search tool can change.

| harness | arm | payload tok/case | total tok/case | wall s | cost/case | quality 0–10 | sourced |
|---|---|---|---|---|---|---|---|
| Claude Code sonnet | native | 1.0k | 96.8k | 20.6 | $0.138 | 8.50 | 29/30 |
| Claude Code sonnet | webctl, no scrape | 1.5k | 80.6k | 16.7 | $0.097 | 9.30 | 30/30 |
| Claude Code sonnet | webctl, scrape | 4.6k | 69.4k | 14.6 | $0.112 | 9.17 | 30/30 |
| Claude Code sonnet | webctl, scrape + summarize | 2.1k | 74.3k | 22.7 | $0.100 | 9.30 | 30/30 |
| pi Kimi K3 | webctl, no scrape | 2.1k | 7.7k | 29.3 | $0.031 | 9.53 | 30/30 |
| pi Kimi K3 | webctl, scrape | 7.4k | 14.7k | 31.7 | $0.048 | 9.73 | 30/30 |
| pi Kimi K3 | webctl, scrape + summarize | 2.6k | 8.1k | 36.2 | $0.034 | 9.63 | 30/30 |

- **Claude Code:** every webctl arm beats the built-in search by 0.7 to 0.8 points of quality, cites a source on every case, and costs 19 to 30% less. Total tokens drop 17 to 28% because webctl needs fewer turns.
- **No scrape is the cheapest and matches the best quality.** Its payload is within 50% of native links.
- **Summarize halves scrape's payload with no quality loss** (2.1k versus 4.6k, 9.30 versus 9.17) at the cost of about 8 seconds per case for the extra model round-trips. It is the scrape mode to use; plain scrape is now the worst of the three on every measure but speed.
- **Claude's native payload is small because it is links plus a hidden summary.** The snippet reading happens in sub-calls that are on the bill (native is the most expensive Claude arm) but not in context.
- **Kimi K3 graded its own pi answers.** Treat the pi quality numbers as inflated.

## Scrape, summarize, or neither

Quality by domain for Claude Code, payload in parentheses:

| domain | cases | native | no scrape | scrape | scrape + summarize |
|---|---|---|---|---|---|
| community | 5 | 6.2 | 8.8 (2.5k) | 9.8 (7.1k) | 9.2 (3.1k) |
| dev-docs | 10 | 9.4 | 9.6 (1.4k) | 9.6 (4.6k) | 9.2 (1.9k) |
| earnings | 3 | 8.3 | 9.0 (1.3k) | 10.0 (5.1k) | 9.7 (1.5k) |
| long-doc | 5 | 10.0 | 9.8 (1.0k) | 9.2 (3.8k) | 10.0 (1.5k) |
| news | 4 | 7.2 | 8.5 (1.6k) | 7.0 (3.3k) | 8.0 (2.4k) |
| policy | 1 | 6.0 | 10.0 (0.9k) | 9.0 (2.3k) | 10.0 (2.2k) |
| science | 2 | 10.0 | 9.5 (2.2k) | 8.5 (3.6k) | 10.0 (2.1k) |

Summarize keeps scrape's gains on earnings transcripts and community threads at a third of the payload, and removes scrape's losses on long documents, news, and short facts, where a page's kept text was more than the agent needed. Hence the help text: work from snippets; when you would otherwise read a whole page, scrape with summarize.

## Caveats

- One run per cell. Differences under about 0.3 points or 20% are noise; re-judging the same answers across experiments moved arm means by 0.1 to 0.3.
- Recency cases (7 of 30) are graded by agreement between arms plus sourcing, since no fixed answer exists.
- webctl's own Jev calls, and the summarizer's calls (about 600 tokens in and 130 out per page on DeepSeek Flash), are billed separately and are in no token count.
- Every arm was limited to four searches by the prompt.
