# Benchmark: webctl 0.1.5 (behavior 0.0.028)

30 research questions, three agent harnesses, one run per cell, 2026-09-20. Judge: Kimi K3 via pi, blind to arm, all arms of a case graded in one call. Raw data: `experiments/2026-09-20-03-no-scrape-lite/results/cells.jsonl`. Regenerate the full report with `webctl-bench report 2026-09-20-03-no-scrape-lite`.

Arms: each harness told to use only its own web search (native), only `webctl search` (no scrape), or only webctl with `--scrape --filter-chunks` (scrape). pi has no native search.

## Headline

Search payload is what the search tool returned into the agent's context (chars ÷ 4). Total tokens include the harness's own per-turn baseline (about 20k on Claude, 16k on Codex), which no search tool can change.

| harness | arm | payload tok/case | total tok/case | wall s | cost/case | quality 0–10 | sourced |
|---|---|---|---|---|---|---|---|
| Claude Code sonnet | native | 1.0k | 96.8k | 20.6 | $0.138 | 8.53 | 27/30 |
| Claude Code sonnet | webctl, no scrape | 1.5k | 80.6k | 16.7 | $0.097 | 9.40 | 30/30 |
| Claude Code sonnet | webctl, scrape | 4.6k | 69.4k | 14.6 | $0.112 | 9.47 | 30/30 |
| Codex gpt-5.6-terra | native | hidden | 62.8k | 26.4 | | 9.37 | 30/30 |
| Codex gpt-5.6-terra | webctl, no scrape | 2.0k | 42.8k | 33.2 | | 9.20 | 30/30 |
| Codex gpt-5.6-terra | webctl, scrape | 6.8k | 47.4k | 32.1 | | 9.37 | 30/30 |
| pi Kimi K3 | webctl, no scrape | 2.1k | 7.7k | 29.3 | $0.031 | 9.60 | 30/30 |
| pi Kimi K3 | webctl, scrape | 7.4k | 14.7k | 31.7 | $0.048 | 9.77 | 30/30 |

- **Claude Code:** webctl beats the built-in search on quality by about a point either way, cites a source every time, and costs 19 to 30% less. Without scraping the payload is within 50% of native links; with scraping it is 4.5×. Total tokens drop 17 to 28% because webctl needs fewer turns.
- **Codex:** quality is a wash. webctl cuts total tokens 25 to 32% but is 22 to 26% slower, since a shell round-trip loses to server-side search. Codex's native payload is server-side and not observable.
- **Claude's native payload is small because it is links plus a hidden summary.** The snippet reading happens in sub-calls that appear on the bill (native is the most expensive Claude arm) but not in context.
- **Kimi K3 graded its own pi answers.** Treat the pi quality numbers as inflated.

## Scrape or not

Scraping triples the payload for 0.07 points on Claude and 0.17 on Codex. Where it matters, quality native / no scrape / scrape, payload in parentheses:

| domain | cases | Claude native | Claude no scrape | Claude scrape | Codex native | Codex no scrape | Codex scrape |
|---|---|---|---|---|---|---|---|
| community | 5 | 6.4 | 9.0 (2.5k) | 9.6 (7.1k) | 9.6 | 9.8 (4.0k) | 9.4 (13.0k) |
| dev-docs | 10 | 9.4 | 9.5 (1.4k) | 9.5 (4.6k) | 9.4 | 9.1 (1.6k) | 9.2 (6.2k) |
| earnings | 3 | 8.0 | 9.0 (1.3k) | 10.0 (5.1k) | 9.7 | 9.3 (1.9k) | 9.7 (5.8k) |
| long-doc | 5 | 9.4 | 10.0 (1.0k) | 10.0 (3.8k) | 10.0 | 9.8 (0.9k) | 10.0 (4.0k) |
| news | 4 | 8.0 | 9.0 (1.6k) | 8.0 (3.3k) | 7.8 | 7.8 (1.8k) | 8.2 (6.6k) |
| policy | 1 | 7.0 | 9.0 (0.9k) | 10.0 (2.3k) | 9.0 | 9.0 (2.1k) | 10.0 (6.2k) |
| science | 2 | 10.0 | 10.0 (2.2k) | 9.5 (3.6k) | 10.0 | 9.5 (2.0k) | 10.0 (3.5k) |
Scrape pays on earnings transcripts, community threads, and long documents, where the answer is a paragraph deep inside a page. It adds nothing on dev docs, news, or short facts. Hence the guidance in the help text: work from snippets, scrape when you would otherwise read the whole page.

## Caveats

- One run per cell. Differences under about 0.3 points or 20% are noise; re-judging the same answers moved arm means by 0.1 to 0.2.
- Recency cases (7 of 30) are graded by agreement between arms plus sourcing, since no fixed answer exists.
- webctl's own Jev calls are billed to the Jev key and are not in any token count.
- Native arms ran the agent's built-in tools as configured by the harness; the prompt limited every arm to four searches.
