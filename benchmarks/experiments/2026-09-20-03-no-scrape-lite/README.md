# 03: webctl without scraping (webctl-lite)

Same 30 cases. Three new arms run `webctl search "<q>" --goal "<g>"` with no `--scrape`, so the agent works from titles, URLs, scores, and snippets, the same shape as a native search result. The five arms from experiment 02 are carried over unchanged and all eight arms of each case were judged together in one call.

Search payload per case (chars of search-tool results returned into context, /4):

| arm | payload tokens | quality |
|---|---|---|
| Claude native (WebSearch + WebFetch) | 1.0k | 8.53 |
| Claude webctl-lite | 1.5k | 9.40 |
| Claude webctl with scrape | 4.6k | 9.47 |
| Codex native | hidden (server-side) | 9.37 |
| Codex webctl-lite | 2.0k | 9.20 |
| Codex webctl with scrape | 6.8k | 9.37 |
| pi webctl-lite | 2.1k | 9.60 |
| pi webctl with scrape | 7.4k | 9.77 |

Headline: without scraping, webctl's payload is within 50% of Claude's native search on Claude, a third of the scrape mode's, and quality stays within 0.1 to 0.2 of scrape mode. Cost on Claude is the lowest of the three: $0.097 per case versus $0.112 scrape and $0.138 native. Total tokens fall less than payload does because lite mode takes more calls (51 versus 34 on Claude), and each call re-reads the harness baseline.
