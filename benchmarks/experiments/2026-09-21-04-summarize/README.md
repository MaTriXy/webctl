# 04: scrape + summarize

Same 30 cases. Adds `webctl-summarize` arms: the agent is told to add `--scrape --filter-chunks --summarize` when it would otherwise read a page. Summarizer: DeepSeek V4 Flash on Fireworks (`accounts/fireworks/models/deepseek-v4p1-flash`, reasoning off). The eight arms from experiment 03 are carried over unchanged and all eleven arms of each case were judged together.

Claude Code, per case: payload 2.1k tokens (scrape 4.6k, no scrape 1.5k, native 1.0k); quality 9.30 (scrape 9.17, no scrape 9.30, native 8.50); wall 22.7 s (scrape 14.6, no scrape 16.7, native 20.6); cost $0.100 (scrape $0.112, no scrape $0.097, native $0.138).

Summarize is the scrape mode to use: half the payload of plain scrape, no quality loss, slower by the summarizer round-trips.
