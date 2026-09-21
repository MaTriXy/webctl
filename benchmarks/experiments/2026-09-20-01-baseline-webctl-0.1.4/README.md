# 01: baseline, webctl 0.1.4

30 cases × 5 arms. webctl as released in 0.1.4: `-n 20`, every kept result scraped when `--scrape` is passed, no output cap.

Headline: Claude sonnet -8% tokens, -21% cost, quality 8.83 vs 8.80 native. Codex terra -17% tokens, +26% wall, quality 9.27 vs 9.43.

What the logs showed afterwards: webctl output with `--scrape --filter-chunks` was a median 52 KB per call. 26 of 41 outputs to Claude overflowed Claude Code's inline tool-output limit and reached the agent as a 2 KB preview. `payload_chars` is 0 for the webctl arms here because their logs were overwritten by experiment 02's rerun; the 52 KB figure was measured before that.

Judge: Kimi K3 via pi, blind, all arms of a case in one call.
