# 02: bounded output, webctl 0.0.028

Same 30 cases and native cells as 01; the three webctl arms re-run after `--scrape-top 3`, `--max-output 20000`, boilerplate-run stripping, and JSON-page rejection. Every case re-judged in one call with all five arms.

Headline: Claude sonnet -28% tokens, -29% wall, -19% cost, quality 9.33 vs 8.67 native. Codex terra -25% tokens, +22% wall, quality 9.53 vs 9.23. No webctl output truncated by Claude Code.

Search payload alone (chars of search-tool results returned into context, /4): Claude native 1.0k tokens per case (links and fetch summaries; the snippet reading happens in hidden sub-calls), Claude webctl 4.6k, Codex webctl 6.7k, pi webctl 7.3k. Codex native payload is server-side and not visible.
