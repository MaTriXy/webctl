# webctl benchmark

Started 2026-09-20 16:55, webctl 0.0.028, judge fireworks/accounts/fireworks/models/kimi-k3. 30 cases × 11 arms.

Tokens are everything the model attended to across all turns: uncached input + cache reads + cache writes + output. Cost is what the harness reported (Codex reports none). Quality is the judge's 0–10 score, blind to arm.

## Savings: webctl vs the harness's own search

| harness | metric | native | webctl | change |
|---|---|---|---|---|
| claude sonnet → webctl | wall clock (s, mean) | 20.6 | 14.6 | -29% |
| claude sonnet → webctl | tokens (mean per case) | 96762 | 69376 | -28% |
| claude sonnet → webctl | search payload tokens (mean, chars/4) | 1040 | 4637 | +346% |
| claude sonnet → webctl | output tokens (mean) | 821 | 719 | -12% |
| claude sonnet → webctl | cost USD (mean) | 0.138 | 0.112 | -19% |
| claude sonnet → webctl | quality (0–10, mean) | 8.50 | 9.17 | +8% |
| claude sonnet → webctl | sourced answers | 29/30 | 30/30 | |
| claude sonnet → webctl | failed runs | 0 | 0 | |
| claude sonnet → webctl-lite | wall clock (s, mean) | 20.6 | 16.7 | -19% |
| claude sonnet → webctl-lite | tokens (mean per case) | 96762 | 80590 | -17% |
| claude sonnet → webctl-lite | search payload tokens (mean, chars/4) | 1040 | 1548 | +49% |
| claude sonnet → webctl-lite | output tokens (mean) | 821 | 847 | +3% |
| claude sonnet → webctl-lite | cost USD (mean) | 0.138 | 0.097 | -30% |
| claude sonnet → webctl-lite | quality (0–10, mean) | 8.50 | 9.30 | +9% |
| claude sonnet → webctl-lite | sourced answers | 29/30 | 30/30 | |
| claude sonnet → webctl-lite | failed runs | 0 | 0 | |
| claude sonnet → webctl-summarize | wall clock (s, mean) | 20.6 | 22.7 | +10% |
| claude sonnet → webctl-summarize | tokens (mean per case) | 96762 | 74342 | -23% |
| claude sonnet → webctl-summarize | search payload tokens (mean, chars/4) | 1040 | 2076 | +100% |
| claude sonnet → webctl-summarize | output tokens (mean) | 821 | 788 | -4% |
| claude sonnet → webctl-summarize | cost USD (mean) | 0.138 | 0.100 | -28% |
| claude sonnet → webctl-summarize | quality (0–10, mean) | 8.50 | 9.30 | +9% |
| claude sonnet → webctl-summarize | sourced answers | 29/30 | 30/30 | |
| claude sonnet → webctl-summarize | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl | wall clock (s, mean) | 26.4 | 32.1 | +22% |
| codex gpt-5.6-terra → webctl | tokens (mean per case) | 62795 | 47359 | -25% |
| codex gpt-5.6-terra → webctl | search payload tokens (mean, chars/4) | hidden | 6798 | |
| codex gpt-5.6-terra → webctl | output tokens (mean) | 443 | 592 | +33% |
| codex gpt-5.6-terra → webctl | quality (0–10, mean) | 9.33 | 9.40 | +1% |
| codex gpt-5.6-terra → webctl | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl-lite | wall clock (s, mean) | 26.4 | 33.2 | +26% |
| codex gpt-5.6-terra → webctl-lite | tokens (mean per case) | 62795 | 42844 | -32% |
| codex gpt-5.6-terra → webctl-lite | search payload tokens (mean, chars/4) | hidden | 1974 | |
| codex gpt-5.6-terra → webctl-lite | output tokens (mean) | 443 | 614 | +39% |
| codex gpt-5.6-terra → webctl-lite | quality (0–10, mean) | 9.33 | 9.13 | -2% |
| codex gpt-5.6-terra → webctl-lite | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl-lite | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl-summarize | wall clock (s, mean) | 26.4 | 24.7 | -6% |
| codex gpt-5.6-terra → webctl-summarize | tokens (mean per case) | 62795 | 55958 | -11% |
| codex gpt-5.6-terra → webctl-summarize | search payload tokens (mean, chars/4) | hidden | 6994 | |
| codex gpt-5.6-terra → webctl-summarize | output tokens (mean) | 443 | 701 | +58% |
| codex gpt-5.6-terra → webctl-summarize | quality (0–10, mean) | 9.33 | 9.23 | -1% |
| codex gpt-5.6-terra → webctl-summarize | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl-summarize | failed runs | 0 | 0 | |

## Per arm

| arm | cases | failed | wall s (mean) | tokens (mean) | payload tok (mean) | input | cache read | output | cost USD | quality | sourced | tool calls | violations |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| claude-sonnet-webctl | 30 | 0 | 14.6 | 69376 | 4637 | 717522 | 1342203 | 21564 | 3.354 | 9.17 | 30/30 | 34 webctl | 0 |
| claude-sonnet-webctl-lite | 30 | 0 | 16.7 | 80590 | 1548 | 570241 | 1822071 | 25396 | 2.899 | 9.30 | 30/30 | 51 webctl | 0 |
| claude-sonnet-webctl-summarize | 30 | 0 | 22.7 | 74342 | 2076 | 610865 | 1595752 | 23639 | 2.999 | 9.30 | 30/30 | 39 webctl | 0 |
| claude-sonnet-native | 30 | 0 | 20.6 | 96762 | 1040 | 574000 | 2304238 | 24636 | 4.151 | 8.50 | 29/30 | 53 search | 0 |
| codex-terra-webctl | 30 | 0 | 32.1 | 47359 | 6798 | 347009 | 1056000 | 17756 | 0.000 | 9.40 | 30/30 | 65 webctl | 0 |
| codex-terra-webctl-lite | 30 | 0 | 33.2 | 42844 | 1974 | 204995 | 1061888 | 18429 | 0.000 | 9.13 | 30/30 | 74 webctl | 0 |
| codex-terra-webctl-summarize | 30 | 0 | 24.7 | 55958 | 6994 | 366968 | 1290752 | 21028 | 0.000 | 9.23 | 30/30 | 87 webctl | 0 |
| codex-terra-native | 30 | 0 | 26.4 | 62795 | 0 | 507081 | 1363456 | 13301 | 0.000 | 9.33 | 30/30 | 76 search | 0 |
| pi-kimi-k3-webctl | 30 | 0 | 31.7 | 14694 | 7393 | 259636 | 137860 | 43311 | 1.429 | 9.73 | 30/30 | 55 webctl | 0 |
| pi-kimi-k3-webctl-lite | 30 | 0 | 29.3 | 7726 | 2093 | 106114 | 84163 | 41495 | 0.941 | 9.53 | 30/30 | 67 webctl | 0 |
| pi-kimi-k3-webctl-summarize | 30 | 0 | 36.2 | 8089 | 2568 | 122174 | 76960 | 43546 | 1.020 | 9.63 | 30/30 | 57 webctl | 0 |

## Per domain (quality mean / tokens mean)

| domain | cases | claude-sonnet-webctl | claude-sonnet-webctl-lite | claude-sonnet-webctl-summarize | claude-sonnet-native | codex-terra-webctl | codex-terra-webctl-lite | codex-terra-webctl-summarize | codex-terra-native | pi-kimi-k3-webctl | pi-kimi-k3-webctl-lite | pi-kimi-k3-webctl-summarize |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| community | 5 | 9.8 / 71k | 8.8 / 79k | 9.2 / 74k | 6.2 / 126k | 9.6 / 87k | 9.4 / 65k | 9.4 / 83k | 9.4 / 78k | 9.8 / 28k | 9.4 / 14k | 10.0 / 14k |
| dev-docs | 10 | 9.6 / 67k | 9.6 / 79k | 9.2 / 67k | 9.4 / 84k | 9.6 / 40k | 9.4 / 45k | 9.7 / 55k | 9.7 / 52k | 9.5 / 14k | 9.6 / 7k | 9.3 / 6k |
| earnings | 3 | 10.0 / 68k | 9.0 / 85k | 9.7 / 63k | 8.3 / 101k | 9.0 / 42k | 8.7 / 45k | 9.3 / 50k | 9.3 / 91k | 10.0 / 17k | 9.7 / 10k | 9.7 / 9k |
| long-doc | 5 | 9.2 / 66k | 9.8 / 75k | 10.0 / 76k | 10.0 / 92k | 9.8 / 30k | 9.8 / 25k | 9.2 / 39k | 10.0 / 66k | 10.0 / 9k | 10.0 / 4k | 10.0 / 7k |
| news | 4 | 7.0 / 74k | 8.5 / 95k | 8.0 / 98k | 7.2 / 109k | 8.0 / 50k | 7.5 / 39k | 7.5 / 55k | 7.2 / 54k | 9.8 / 13k | 9.5 / 7k | 9.5 / 8k |
| policy | 1 | 9.0 / 64k | 10.0 / 62k | 10.0 / 65k | 6.0 / 81k | 10.0 / 42k | 9.0 / 36k | 10.0 / 56k | 9.0 / 71k | 10.0 / 5k | 9.0 / 6k | 10.0 / 5k |
| science | 2 | 8.5 / 82k | 9.5 / 81k | 10.0 / 80k | 10.0 / 80k | 10.0 / 32k | 9.5 / 30k | 9.5 / 48k | 10.0 / 46k | 9.5 / 6k | 8.5 / 4k | 9.5 / 4k |

## Per case (quality · seconds · tokens)

| case | domain | claude-sonnet-webctl | claude-sonnet-webctl-lite | claude-sonnet-webctl-summarize | claude-sonnet-native | codex-terra-webctl | codex-terra-webctl-lite | codex-terra-webctl-summarize | codex-terra-native | pi-kimi-k3-webctl | pi-kimi-k3-webctl-lite | pi-kimi-k3-webctl-summarize |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| attention-paper-dimensions | long-doc | 9 · 11s · 67k | 10 · 7s · 61k | 10 · 10s · 61k | 10 · 14s · 79k | 10 · 16s · 23k | 10 · 25s · 33k | 10 · 14s · 36k | 10 · 17s · 44k | 10 · 14s · 7k | 10 · 17s · 3k | 10 · 17s · 3k |
| clickhouse-final-modifier | dev-docs | 9 · 13s · 65k | 9 · 15s · 95k | 9 · 27s · 99k | 8 · 27s · 111k | 9 · 29s · 45k | 9 · 38s · 60k | 9 · 24s · 58k | 9 · 23s · 45k | 9 · 59s · 34k | 10 · 40s · 10k | 9 · 37s · 9k |
| community-bun-vs-node-production | community | 10 · 22s · 79k | 7 · 22s · 66k | 10 · 34s · 68k | 7 · 37s · 146k | 10 · 50s · 71k | 10 · 62s · 71k | 10 · 41s · 82k | 10 · 31s · 65k | 10 · 41s · 30k | 9 · 51s · 14k | 10 · 83s · 18k |
| community-kafka-to-redpanda | community | 9 · 28s · 74k | 9 · 22s · 65k | 9 · 47s · 102k | 8 · 38s · 114k | 9 · 54s · 84k | 8 · 49s · 66k | 8 · 44s · 104k | 8 · 34s · 80k | 10 · 81s · 34k | 9 · 49s · 13k | 10 · 72s · 17k |
| community-macos-operation-not-permitted | community | 10 · 15s · 64k | 9 · 28s · 62k | 8 · 21s · 63k | 10 · 19s · 79k | 10 · 43s · 72k | 10 · 37s · 49k | 10 · 36s · 49k | 10 · 25s · 46k | 10 · 34s · 12k | 10 · 15s · 3k | 10 · 21s · 5k |
| community-proxmox-vs-vmware | community | 10 · 25s · 71k | 10 · 34s · 103k | 9 · 38s · 67k | 3 · 34s · 115k | 10 · 63s · 115k | 10 · 52s · 76k | 10 · 39s · 93k | 10 · 46s · 79k | 9 · 52s · 27k | 10 · 44s · 16k | 10 · 66s · 13k |
| community-wsus-alternatives | community | 10 · 19s · 66k | 9 · 31s · 98k | 10 · 40s · 67k | 3 · 50s · 173k | 9 · 61s · 92k | 9 · 46s · 63k | 9 · 41s · 85k | 9 · 51s · 116k | 10 · 41s · 35k | 9 · 67s · 21k | 10 · 65s · 16k |
| go-context-cancellation | dev-docs | 10 · 10s · 67k | 10 · 11s · 62k | 10 · 16s · 63k | 10 · 18s · 79k | 10 · 15s · 26k | 10 · 14s · 21k | 10 · 17s · 36k | 10 · 17s · 46k | 10 · 22s · 13k | 10 · 13s · 4k | 10 · 18s · 4k |
| jwst-orbit | science | 7 · 10s · 63k | 9 · 11s · 62k | 10 · 8s · 61k | 10 · 13s · 79k | 10 · 14s · 24k | 9 · 15s · 22k | 9 · 15s · 34k | 10 · 20s · 45k | 9 · 13s · 5k | 7 · 11s · 3k | 9 · 10s · 3k |
| k8s-api-deprecation-ingress | long-doc | 10 · 14s · 66k | 10 · 10s · 62k | 10 · 8s · 62k | 10 · 15s · 79k | 10 · 15s · 25k | 10 · 15s · 23k | 10 · 17s · 37k | 10 · 18s · 45k | 10 · 17s · 7k | 10 · 11s · 3k | 10 · 22s · 4k |
| k8s-oomkilled-under-limit | dev-docs | 8 · 18s · 63k | 7 · 32s · 127k | 8 · 36s · 64k | 7 · 32s · 83k | 9 · 75s · 87k | 7 · 83s · 75k | 9 · 40s · 116k | 9 · 57s · 79k | 9 · 75s · 34k | 8 · 53s · 10k | 10 · 79s · 17k |
| kafka-kraft-quorum-voters | dev-docs | 10 · 11s · 67k | 10 · 15s · 61k | 8 · 17s · 64k | 10 · 15s · 79k | 9 · 17s · 26k | 10 · 43s · 63k | 10 · 17s · 38k | 10 · 20s · 45k | 9 · 23s · 7k | 8 · 23s · 5k | 10 · 19s · 5k |
| kafka-min-insync-replicas | dev-docs | 10 · 16s · 68k | 10 · 11s · 63k | 9 · 20s · 62k | 9 · 18s · 79k | 9 · 39s · 57k | 9 · 43s · 62k | 9 · 26s · 55k | 9 · 18s · 45k | 10 · 25s · 6k | 10 · 42s · 8k | 10 · 30s · 4k |
| news-f1-latest-race | news | 3 · 11s · 66k | 8 · 20s · 96k | 10 · 24s · 99k | 4 · 21s · 109k | 9 · 25s · 45k | 9 · 26s · 35k | 9 · 21s · 56k | 9 · 17s · 45k | 10 · 20s · 8k | 10 · 30s · 7k | 10 · 38s · 9k |
| news-fomc-latest-rate | news | 9 · 11s · 62k | 10 · 17s · 94k | 8 · 19s · 62k | 10 · 23s · 109k | 10 · 27s · 37k | 10 · 16s · 22k | 10 · 15s · 37k | 10 · 18s · 45k | 10 · 21s · 11k | 10 · 16s · 4k | 10 · 28s · 6k |
| news-kubernetes-latest-release | news | 9 · 16s · 105k | 8 · 15s · 94k | 9 · 28s · 98k | 9 · 18s · 136k | 9 · 23s · 43k | 9 · 23s · 34k | 9 · 16s · 37k | 6 · 17s · 45k | 10 · 30s · 17k | 9 · 23s · 6k | 10 · 35s · 8k |
| news-microsoft-azure-growth | earnings | 10 · 11s · 69k | 10 · 14s · 62k | 10 · 18s · 63k | 9 · 22s · 110k | 10 · 17s · 27k | 9 · 20s · 22k | 10 · 21s · 55k | 10 · 34s · 101k | 10 · 19s · 7k | 10 · 24s · 9k | 10 · 18s · 4k |
| news-nvidia-latest-quarter | earnings | 10 · 8s · 65k | 10 · 9s · 61k | 10 · 16s · 62k | 10 · 27s · 111k | 10 · 18s · 27k | 10 · 33s · 48k | 10 · 17s · 38k | 10 · 27s · 93k | 10 · 33s · 23k | 10 · 23s · 8k | 10 · 34s · 12k |
| news-openai-latest-announcement | news | 7 · 10s · 62k | 8 · 13s · 93k | 5 · 34s · 133k | 6 · 14s · 80k | 4 · 52s · 73k | 2 · 74s · 64k | 2 · 33s · 88k | 4 · 32s · 78k | 9 · 49s · 12k | 9 · 49s · 10k | 8 · 32s · 6k |
| news-tesla-latest-call | earnings | 10 · 22s · 68k | 7 · 29s · 130k | 9 · 21s · 63k | 6 · 17s · 81k | 7 · 49s · 71k | 7 · 62s · 65k | 8 · 23s · 55k | 8 · 30s · 77k | 10 · 43s · 18k | 9 · 43s · 12k | 9 · 49s · 9k |
| pep8-line-length | long-doc | 10 · 8s · 63k | 10 · 11s · 62k | 10 · 16s · 63k | 10 · 9s · 78k | 10 · 15s · 24k | 10 · 21s · 22k | 10 · 15s · 35k | 10 · 17s · 46k | 10 · 14s · 5k | 10 · 19s · 3k | 10 · 16s · 3k |
| postgres-autovacuum-threshold | dev-docs | 10 · 12s · 65k | 10 · 11s · 63k | 8 · 18s · 63k | 10 · 13s · 79k | 10 · 18s · 24k | 9 · 24s · 34k | 10 · 17s · 35k | 10 · 26s · 45k | 8 · 22s · 6k | 10 · 20s · 4k | 8 · 32s · 5k |
| redis-xautoclaim | dev-docs | 10 · 15s · 67k | 10 · 13s · 94k | 10 · 16s · 62k | 10 · 13s · 80k | 10 · 18s · 25k | 10 · 17s · 21k | 10 · 15s · 37k | 10 · 22s · 45k | 10 · 26s · 7k | 10 · 20s · 5k | 9 · 25s · 3k |
| rfc9110-retry-after | long-doc | 7 · 14s · 61k | 10 · 16s · 92k | 10 · 24s · 126k | 10 · 16s · 109k | 10 · 42s · 49k | 10 · 17s · 21k | 7 · 23s · 49k | 10 · 23s · 80k | 10 · 40s · 15k | 10 · 28s · 5k | 10 · 59s · 15k |
| rust-pin-future-poll | dev-docs | 10 · 16s · 68k | 10 · 18s · 65k | 10 · 23s · 64k | 10 · 22s · 81k | 10 · 31s · 44k | 10 · 36s · 48k | 10 · 28s · 84k | 10 · 26s · 47k | 10 · 40s · 15k | 10 · 38s · 9k | 9 · 34s · 5k |
| sodium-daily-limits | science | 10 · 13s · 100k | 10 · 12s · 98k | 10 · 18s · 99k | 10 · 12s · 80k | 10 · 25s · 39k | 10 · 23s · 37k | 10 · 52s · 60k | 10 · 22s · 45k | 10 · 14s · 6k | 10 · 18s · 5k | 10 · 15s · 5k |
| sqlite-wal-autocheckpoint | dev-docs | 9 · 12s · 65k | 10 · 14s · 61k | 10 · 22s · 63k | 10 · 16s · 80k | 10 · 18s · 25k | 10 · 16s · 22k | 10 · 15s · 36k | 10 · 17s · 45k | 10 · 15s · 6k | 10 · 24s · 6k | 9 · 23s · 4k |
| terraform-moved-block | dev-docs | 10 · 17s · 66k | 10 · 14s · 93k | 10 · 17s · 62k | 10 · 10s · 78k | 10 · 29s · 39k | 10 · 22s · 33k | 10 · 22s · 50k | 10 · 27s · 71k | 10 · 21s · 7k | 10 · 21s · 3k | 9 · 34s · 4k |
| tls13-key-share | long-doc | 10 · 16s · 69k | 9 · 14s · 94k | 10 · 21s · 65k | 10 · 21s · 110k | 9 · 23s · 26k | 9 · 17s · 22k | 9 · 16s · 36k | 10 · 32s · 111k | 10 · 23s · 8k | 10 · 24s · 3k | 10 · 41s · 7k |
| us-ev-tax-credit-end | policy | 9 · 16s · 63k | 10 · 12s · 62k | 10 · 24s · 64k | 6 · 13s · 81k | 10 · 41s · 42k | 9 · 26s · 35k | 10 · 23s · 55k | 9 · 28s · 71k | 10 · 27s · 4k | 9 · 23s · 5k | 10 · 30s · 4k |

## Quality losses and wins with webctl (score difference ≥ 2)

- **community-bun-vs-node-production** (claude): native 7 → webctl 10. webctl judge note: Extensive, accurate, and consistent with corroborated reports including the Anthropic acquisition; clean on all rubric items.. native judge note: Meets the rubric on facts and has HN URLs, but the confident Bun 2.0/99.4% stat and fabricated-looking blog citations cost points.
- **community-proxmox-vs-vmware** (claude): native 3 → webctl 10. webctl judge note: Every rubric item covered with faithful community-reported migration gotchas and threads from both subreddits.. native judge note: Directionally correct synthesis but fails the core Reddit-community requirement and includes a wrong migration-tooling claim, so it scores low while staying above a confidently wrong answer.
- **community-wsus-alternatives** (claude): native 3 → webctl 10. webctl judge note: Covers all rubric criteria including the often-missed bandwidth caching tradeoff; nothing incorrect found.. native judge note: Admits it couldn't reach Reddit and hedges throughout; scores low per rubric but above a wrong answer for its honesty.
- **jwst-orbit** (claude): native 10 → webctl 7. webctl judge note: Expected facts present but the closing distance statement is confusingly wrong as written.. native judge note: All facts correct with accurate supporting detail and real sources.
- **news-tesla-latest-call** (claude): native 6 → webctl 10. webctl judge note: Covers every consensus fact with the fullest robotaxi detail and layered sourcing including the official shareholder deck.. native judge note: Substance is strong, but it confidently misdates the call — one of the two explicitly requested facts — by mistaking a transcript's publication date for the event date.
- **rfc9110-retry-after** (claude): native 10 → webctl 7. webctl judge note: Core answer is complete, accurate, and well sourced, but the confidently asserted note denying 429's presence in RFC 9110 is factually wrong.. native judge note: All expected facts correct; including 429 is accurate to RFC 9110 §10.2.3.
- **us-ev-tax-credit-end** (claude): native 6 → webctl 9. webctl judge note: All expected facts present, but the '4+ years early' claim understates the acceleration by several years.. native judge note: Core facts right, but a confidently wrong claim about the possession/acquisition rule materially misleads on eligibility.
- **community-proxmox-vs-vmware** (claude): native 3 → webctl 10. webctl judge note: Thorough and accurate; leans partly on r/Proxmox and some pre-Broadcom threads but still cites plenty of r/homelab/r/sysadmin and misses no rubric item.. native judge note: Directionally correct synthesis but fails the core Reddit-community requirement and includes a wrong migration-tooling claim, so it scores low while staying above a confidently wrong answer.
- **community-wsus-alternatives** (claude): native 3 → webctl 9. webctl judge note: Comprehensive and accurate with rich sourcing; one questionable Tanium claim costs a point.. native judge note: Admits it couldn't reach Reddit and hedges throughout; scores low per rubric but above a wrong answer for its honesty.
- **news-f1-latest-race** (claude): native 4 → webctl 8. webctl judge note: All core facts correct but sourcing is generic (BBC live page, calendar guide) rather than a specific result/race report.. native judge note: Got winner and date but missed the circuit, leaned toward the wrong venue, and contradicted the best-supported win count.
- **news-openai-latest-announcement** (claude): native 6 → webctl 8. webctl judge note: Correct answer with good detail and sources, marginally less precise on dates than the top answers.. native judge note: Right product, but includes an unsupported motive and wrongly asserts nothing else shipped in the window.
- **us-ev-tax-credit-end** (claude): native 6 → webctl 10. webctl judge note: Fully correct, including the transition rule, with real IRS/CRS sources.. native judge note: Core facts right, but a confidently wrong claim about the possession/acquisition rule materially misleads on eligibility.
- **community-bun-vs-node-production** (claude): native 7 → webctl 10. webctl judge note: Accurate and well-sourced via many Reddit threads (Reddit satisfies the URL rule); leading process-chatter line ignored per style rules.. native judge note: Meets the rubric on facts and has HN URLs, but the confident Bun 2.0/99.4% stat and fabricated-looking blog citations cost points.
- **community-macos-operation-not-permitted** (claude): native 10 → webctl 8. webctl judge note: Cause and fix correct but never mentions restarting Terminal, which is part of the expected fix.. native judge note: Complete and correct; sources are real though they lean on blogs/Apple Discussions rather than Reddit/SE.
- **community-proxmox-vs-vmware** (claude): native 3 → webctl 9. webctl judge note: Hits every rubric item but includes a stale/wrong Veeam-beta claim and thinner canonical reasons than peers.. native judge note: Directionally correct synthesis but fails the core Reddit-community requirement and includes a wrong migration-tooling claim, so it scores low while staying above a confidently wrong answer.
- **community-wsus-alternatives** (claude): native 3 → webctl 10. webctl judge note: Hits every rubric item with correct, well-attributed detail and real thread URLs.. native judge note: Admits it couldn't reach Reddit and hedges throughout; scores low per rubric but above a wrong answer for its honesty.
- **kafka-kraft-quorum-voters** (claude): native 10 → webctl 8. webctl judge note: Thorough and mostly accurate, but the confidently stated 4.1 introduction version contradicts the 3.9+ fact.. native judge note: Concise and fully correct; the kraft.version=1 deprecation and partial-list discovery notes are accurate.
- **news-f1-latest-race** (claude): native 4 → webctl 10. webctl judge note: Every expected fact correct with deep-linked official results URLs and details that cross-agree with other strong answers.. native judge note: Got winner and date but missed the circuit, leaned toward the wrong venue, and contradicted the best-supported win count.
- **news-fomc-latest-rate** (claude): native 10 → webctl 8. webctl judge note: Core facts right, but includes specific uncorroborated extra claims that risk fabrication.. native judge note: Correct on all required elements with primary sourcing.
- **news-tesla-latest-call** (claude): native 6 → webctl 9. webctl judge note: Comprehensive and consistent with the best-supported account; a couple of fine-grained extras rest on single-answer support.. native judge note: Substance is strong, but it confidently misdates the call — one of the two explicitly requested facts — by mistaking a transcript's publication date for the event date.
- **postgres-autovacuum-threshold** (claude): native 10 → webctl 8. webctl judge note: Core facts right with real sources, but confidently wrong version attribution for the cap and vague on reltuples.. native judge note: All expected facts correct, accurate worked example, vaguely but correctly notes the newer max-threshold cap.
- **us-ev-tax-credit-end** (claude): native 6 → webctl 10. webctl judge note: Complete and accurate with real IRS sources.. native judge note: Core facts right, but a confidently wrong claim about the possession/acquisition rule materially misleads on eligibility.
- **news-kubernetes-latest-release** (codex): native 6 → webctl 9. webctl judge note: All requested facts present, correct, consensus-backed, and sourced.. native judge note: Version and date match consensus, but its chosen headline feature appears nowhere else and is likely wrong or not a headline item.
- **k8s-oomkilled-under-limit** (codex): native 9 → webctl 7. webctl judge note: Solid on accounting mismatch and node OOM with real sources, but omits the tmpfs/emptyDir mechanism entirely.. native judge note: Compact but covers all four expected mechanisms accurately with only first-party Kubernetes and kernel-doc sources.
- **news-kubernetes-latest-release** (codex): native 6 → webctl 9. webctl judge note: Accurate and consensus-backed headline feature with a specific follow-up blog source.. native judge note: Version and date match consensus, but its chosen headline feature appears nowhere else and is likely wrong or not a headline item.
- **news-openai-latest-announcement** (codex): native 4 → webctl 2. webctl judge note: Confidently names an uncorroborated announcement while missing the consensus headline — wrong answer despite plausible-looking sources.. native judge note: Accurate about a secondary announcement but misses the consensus answer entirely.
- **news-kubernetes-latest-release** (codex): native 6 → webctl 9. webctl judge note: Fully correct and specific; single source, slightly less cross-support than the top answers.. native judge note: Version and date match consensus, but its chosen headline feature appears nowhere else and is likely wrong or not a headline item.
- **news-openai-latest-announcement** (codex): native 4 → webctl 2. webctl judge note: A lone, uncorroborated claim that misses the consensus announcement — treated as unsupported/confabulated under the agreement rubric.. native judge note: Accurate about a secondary announcement but misses the consensus answer entirely.
- **rfc9110-retry-after** (codex): native 10 → webctl 7. webctl judge note: Core answer correct and well sourced, but the confident claim that RFC 9110 omits 429 is false.. native judge note: All three expected facts present and correct with a primary source.

