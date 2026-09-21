# webctl benchmark

Started 2026-09-20 16:55, webctl 0.0.028, judge fireworks/accounts/fireworks/models/kimi-k3. 30 cases × 8 arms.

Tokens are everything the model attended to across all turns: uncached input + cache reads + cache writes + output. Cost is what the harness reported (Codex reports none). Quality is the judge's 0–10 score, blind to arm.

## Savings: webctl vs the harness's own search

| harness | metric | native | webctl | change |
|---|---|---|---|---|
| claude sonnet → webctl | wall clock (s, mean) | 20.6 | 14.6 | -29% |
| claude sonnet → webctl | tokens (mean per case) | 96762 | 69376 | -28% |
| claude sonnet → webctl | search payload tokens (mean, chars/4) | 1040 | 4637 | +346% |
| claude sonnet → webctl | output tokens (mean) | 821 | 719 | -12% |
| claude sonnet → webctl | cost USD (mean) | 0.138 | 0.112 | -19% |
| claude sonnet → webctl | quality (0–10, mean) | 8.53 | 9.47 | +11% |
| claude sonnet → webctl | sourced answers | 27/30 | 30/30 | |
| claude sonnet → webctl | failed runs | 0 | 0 | |
| claude sonnet → webctl-lite | wall clock (s, mean) | 20.6 | 16.7 | -19% |
| claude sonnet → webctl-lite | tokens (mean per case) | 96762 | 80590 | -17% |
| claude sonnet → webctl-lite | search payload tokens (mean, chars/4) | 1040 | 1548 | +49% |
| claude sonnet → webctl-lite | output tokens (mean) | 821 | 847 | +3% |
| claude sonnet → webctl-lite | cost USD (mean) | 0.138 | 0.097 | -30% |
| claude sonnet → webctl-lite | quality (0–10, mean) | 8.53 | 9.40 | +10% |
| claude sonnet → webctl-lite | sourced answers | 27/30 | 30/30 | |
| claude sonnet → webctl-lite | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl | wall clock (s, mean) | 26.4 | 32.1 | +22% |
| codex gpt-5.6-terra → webctl | tokens (mean per case) | 62795 | 47359 | -25% |
| codex gpt-5.6-terra → webctl | search payload tokens (mean, chars/4) | hidden | 6798 | |
| codex gpt-5.6-terra → webctl | output tokens (mean) | 443 | 592 | +33% |
| codex gpt-5.6-terra → webctl | quality (0–10, mean) | 9.37 | 9.37 | +0% |
| codex gpt-5.6-terra → webctl | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl-lite | wall clock (s, mean) | 26.4 | 33.2 | +26% |
| codex gpt-5.6-terra → webctl-lite | tokens (mean per case) | 62795 | 42844 | -32% |
| codex gpt-5.6-terra → webctl-lite | search payload tokens (mean, chars/4) | hidden | 1974 | |
| codex gpt-5.6-terra → webctl-lite | output tokens (mean) | 443 | 614 | +39% |
| codex gpt-5.6-terra → webctl-lite | quality (0–10, mean) | 9.37 | 9.20 | -2% |
| codex gpt-5.6-terra → webctl-lite | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl-lite | failed runs | 0 | 0 | |

## Per arm

| arm | cases | failed | wall s (mean) | tokens (mean) | payload tok (mean) | input | cache read | output | cost USD | quality | sourced | tool calls | violations |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| claude-sonnet-webctl | 30 | 0 | 14.6 | 69376 | 4637 | 717522 | 1342203 | 21564 | 3.354 | 9.47 | 30/30 | 34 webctl | 0 |
| claude-sonnet-webctl-lite | 30 | 0 | 16.7 | 80590 | 1548 | 570241 | 1822071 | 25396 | 2.899 | 9.40 | 30/30 | 51 webctl | 0 |
| claude-sonnet-native | 30 | 0 | 20.6 | 96762 | 1040 | 574000 | 2304238 | 24636 | 4.151 | 8.53 | 27/30 | 53 search | 0 |
| codex-terra-webctl | 30 | 0 | 32.1 | 47359 | 6798 | 347009 | 1056000 | 17756 | 0.000 | 9.37 | 30/30 | 65 webctl | 0 |
| codex-terra-webctl-lite | 30 | 0 | 33.2 | 42844 | 1974 | 204995 | 1061888 | 18429 | 0.000 | 9.20 | 30/30 | 74 webctl | 0 |
| codex-terra-native | 30 | 0 | 26.4 | 62795 | 0 | 507081 | 1363456 | 13301 | 0.000 | 9.37 | 30/30 | 76 search | 0 |
| pi-kimi-k3-webctl | 30 | 0 | 31.7 | 14694 | 7393 | 259636 | 137860 | 43311 | 1.429 | 9.77 | 30/30 | 55 webctl | 0 |
| pi-kimi-k3-webctl-lite | 30 | 0 | 29.3 | 7726 | 2093 | 106114 | 84163 | 41495 | 0.941 | 9.60 | 30/30 | 67 webctl | 0 |

## Per domain (quality mean / tokens mean)

| domain | cases | claude-sonnet-webctl | claude-sonnet-webctl-lite | claude-sonnet-native | codex-terra-webctl | codex-terra-webctl-lite | codex-terra-native | pi-kimi-k3-webctl | pi-kimi-k3-webctl-lite |
|---|---|---|---|---|---|---|---|---|---|
| community | 5 | 9.6 / 71k | 9.0 / 79k | 6.4 / 126k | 9.4 / 87k | 9.8 / 65k | 9.6 / 78k | 9.8 / 28k | 9.6 / 14k |
| dev-docs | 10 | 9.5 / 67k | 9.5 / 79k | 9.4 / 84k | 9.2 / 40k | 9.1 / 45k | 9.4 / 52k | 9.8 / 14k | 9.7 / 7k |
| earnings | 3 | 10.0 / 68k | 9.0 / 85k | 8.0 / 101k | 9.7 / 42k | 9.3 / 45k | 9.7 / 91k | 10.0 / 17k | 9.7 / 10k |
| long-doc | 5 | 10.0 / 66k | 10.0 / 75k | 9.4 / 92k | 10.0 / 30k | 9.8 / 25k | 10.0 / 66k | 9.4 / 9k | 9.4 / 4k |
| news | 4 | 8.0 / 74k | 9.0 / 95k | 8.0 / 109k | 8.2 / 50k | 7.8 / 39k | 7.8 / 54k | 10.0 / 13k | 9.5 / 7k |
| policy | 1 | 10.0 / 64k | 9.0 / 62k | 7.0 / 81k | 10.0 / 42k | 9.0 / 36k | 9.0 / 71k | 10.0 / 5k | 10.0 / 6k |
| science | 2 | 9.5 / 82k | 10.0 / 81k | 10.0 / 80k | 10.0 / 32k | 9.5 / 30k | 10.0 / 46k | 9.5 / 6k | 9.5 / 4k |

## Per case (quality · seconds · tokens)

| case | domain | claude-sonnet-webctl | claude-sonnet-webctl-lite | claude-sonnet-native | codex-terra-webctl | codex-terra-webctl-lite | codex-terra-native | pi-kimi-k3-webctl | pi-kimi-k3-webctl-lite |
|---|---|---|---|---|---|---|---|---|---|
| attention-paper-dimensions | long-doc | 10 · 11s · 67k | 10 · 7s · 61k | 10 · 14s · 79k | 10 · 16s · 23k | 10 · 25s · 33k | 10 · 17s · 44k | 10 · 14s · 7k | 10 · 17s · 3k |
| clickhouse-final-modifier | dev-docs | 9 · 13s · 65k | 9 · 15s · 95k | 8 · 27s · 111k | 9 · 29s · 45k | 9 · 38s · 60k | 9 · 23s · 45k | 10 · 59s · 34k | 10 · 40s · 10k |
| community-bun-vs-node-production | community | 9 · 22s · 79k | 7 · 22s · 66k | 7 · 37s · 146k | 9 · 50s · 71k | 9 · 62s · 71k | 9 · 31s · 65k | 10 · 41s · 30k | 9 · 51s · 14k |
| community-kafka-to-redpanda | community | 9 · 28s · 74k | 10 · 22s · 65k | 8 · 38s · 114k | 8 · 54s · 84k | 10 · 49s · 66k | 9 · 34s · 80k | 9 · 81s · 34k | 10 · 49s · 13k |
| community-macos-operation-not-permitted | community | 10 · 15s · 64k | 9 · 28s · 62k | 9 · 19s · 79k | 10 · 43s · 72k | 10 · 37s · 49k | 10 · 25s · 46k | 10 · 34s · 12k | 9 · 15s · 3k |
| community-proxmox-vs-vmware | community | 10 · 25s · 71k | 10 · 34s · 103k | 5 · 34s · 115k | 10 · 63s · 115k | 10 · 52s · 76k | 10 · 46s · 79k | 10 · 52s · 27k | 10 · 44s · 16k |
| community-wsus-alternatives | community | 10 · 19s · 66k | 9 · 31s · 98k | 3 · 50s · 173k | 10 · 61s · 92k | 10 · 46s · 63k | 10 · 51s · 116k | 10 · 41s · 35k | 10 · 67s · 21k |
| go-context-cancellation | dev-docs | 10 · 10s · 67k | 10 · 11s · 62k | 10 · 18s · 79k | 10 · 15s · 26k | 10 · 14s · 21k | 10 · 17s · 46k | 10 · 22s · 13k | 10 · 13s · 4k |
| jwst-orbit | science | 9 · 10s · 63k | 10 · 11s · 62k | 10 · 13s · 79k | 10 · 14s · 24k | 9 · 15s · 22k | 10 · 20s · 45k | 9 · 13s · 5k | 9 · 11s · 3k |
| k8s-api-deprecation-ingress | long-doc | 10 · 14s · 66k | 10 · 10s · 62k | 10 · 15s · 79k | 10 · 15s · 25k | 10 · 15s · 23k | 10 · 18s · 45k | 10 · 17s · 7k | 10 · 11s · 3k |
| k8s-oomkilled-under-limit | dev-docs | 9 · 18s · 63k | 7 · 32s · 127k | 9 · 32s · 83k | 9 · 75s · 87k | 8 · 83s · 75k | 9 · 57s · 79k | 10 · 75s · 34k | 8 · 53s · 10k |
| kafka-kraft-quorum-voters | dev-docs | 9 · 11s · 67k | 10 · 15s · 61k | 10 · 15s · 79k | 8 · 17s · 26k | 9 · 43s · 63k | 9 · 20s · 45k | 10 · 23s · 7k | 9 · 23s · 5k |
| kafka-min-insync-replicas | dev-docs | 10 · 16s · 68k | 9 · 11s · 63k | 8 · 18s · 79k | 8 · 39s · 57k | 8 · 43s · 62k | 9 · 18s · 45k | 10 · 25s · 6k | 10 · 42s · 8k |
| news-f1-latest-race | news | 5 · 11s · 66k | 10 · 20s · 96k | 6 · 21s · 109k | 10 · 25s · 45k | 10 · 26s · 35k | 10 · 17s · 45k | 10 · 20s · 8k | 10 · 30s · 7k |
| news-fomc-latest-rate | news | 10 · 11s · 62k | 9 · 17s · 94k | 10 · 23s · 109k | 10 · 27s · 37k | 10 · 16s · 22k | 10 · 18s · 45k | 10 · 21s · 11k | 10 · 16s · 4k |
| news-kubernetes-latest-release | news | 9 · 16s · 105k | 9 · 15s · 94k | 10 · 18s · 136k | 10 · 23s · 43k | 9 · 23s · 34k | 8 · 17s · 45k | 10 · 30s · 17k | 9 · 23s · 6k |
| news-microsoft-azure-growth | earnings | 10 · 11s · 69k | 10 · 14s · 62k | 8 · 22s · 110k | 10 · 17s · 27k | 10 · 20s · 22k | 10 · 34s · 101k | 10 · 19s · 7k | 10 · 24s · 9k |
| news-nvidia-latest-quarter | earnings | 10 · 8s · 65k | 10 · 9s · 61k | 10 · 27s · 111k | 10 · 18s · 27k | 10 · 33s · 48k | 10 · 27s · 93k | 10 · 33s · 23k | 10 · 23s · 8k |
| news-openai-latest-announcement | news | 8 · 10s · 62k | 8 · 13s · 93k | 6 · 14s · 80k | 3 · 52s · 73k | 2 · 74s · 64k | 3 · 32s · 78k | 10 · 49s · 12k | 9 · 49s · 10k |
| news-tesla-latest-call | earnings | 10 · 22s · 68k | 7 · 29s · 130k | 6 · 17s · 81k | 9 · 49s · 71k | 8 · 62s · 65k | 9 · 30s · 77k | 10 · 43s · 18k | 9 · 43s · 12k |
| pep8-line-length | long-doc | 10 · 8s · 63k | 10 · 11s · 62k | 10 · 9s · 78k | 10 · 15s · 24k | 10 · 21s · 22k | 10 · 17s · 46k | 10 · 14s · 5k | 10 · 19s · 3k |
| postgres-autovacuum-threshold | dev-docs | 10 · 12s · 65k | 10 · 11s · 63k | 10 · 13s · 79k | 10 · 18s · 24k | 10 · 24s · 34k | 10 · 26s · 45k | 9 · 22s · 6k | 10 · 20s · 4k |
| redis-xautoclaim | dev-docs | 9 · 15s · 67k | 10 · 13s · 94k | 10 · 13s · 80k | 10 · 18s · 25k | 9 · 17s · 21k | 10 · 22s · 45k | 9 · 26s · 7k | 10 · 20s · 5k |
| rfc9110-retry-after | long-doc | 10 · 14s · 61k | 10 · 16s · 92k | 7 · 16s · 109k | 10 · 42s · 49k | 10 · 17s · 21k | 10 · 23s · 80k | 7 · 40s · 15k | 7 · 28s · 5k |
| rust-pin-future-poll | dev-docs | 9 · 16s · 68k | 10 · 18s · 65k | 9 · 22s · 81k | 9 · 31s · 44k | 8 · 36s · 48k | 8 · 26s · 47k | 10 · 40s · 15k | 10 · 38s · 9k |
| sodium-daily-limits | science | 10 · 13s · 100k | 10 · 12s · 98k | 10 · 12s · 80k | 10 · 25s · 39k | 10 · 23s · 37k | 10 · 22s · 45k | 10 · 14s · 6k | 10 · 18s · 5k |
| sqlite-wal-autocheckpoint | dev-docs | 10 · 12s · 65k | 10 · 14s · 61k | 10 · 16s · 80k | 10 · 18s · 25k | 10 · 16s · 22k | 10 · 17s · 45k | 10 · 15s · 6k | 10 · 24s · 6k |
| terraform-moved-block | dev-docs | 10 · 17s · 66k | 10 · 14s · 93k | 10 · 10s · 78k | 9 · 29s · 39k | 10 · 22s · 33k | 10 · 27s · 71k | 10 · 21s · 7k | 10 · 21s · 3k |
| tls13-key-share | long-doc | 10 · 16s · 69k | 10 · 14s · 94k | 10 · 21s · 110k | 10 · 23s · 26k | 9 · 17s · 22k | 10 · 32s · 111k | 10 · 23s · 8k | 10 · 24s · 3k |
| us-ev-tax-credit-end | policy | 10 · 16s · 63k | 9 · 12s · 62k | 7 · 13s · 81k | 10 · 41s · 42k | 9 · 26s · 35k | 9 · 28s · 71k | 10 · 27s · 4k | 10 · 23s · 5k |

## Quality losses and wins with webctl (score difference ≥ 2)

- **community-bun-vs-node-production** (claude): native 7 → webctl 9. webctl judge note: Very thorough and accurate; the Anthropic-acquisition sentiment is hedged and corroborated by Answer G, so not penalized.. native judge note: Core HN/Reddit-reported content is solid, but the confidently stated compatibility statistic and questionable blog sources are confidently-wrong risks.
- **community-proxmox-vs-vmware** (claude): native 5 → webctl 10. webctl judge note: All rubric items satisfied with correct facts and direct Reddit sourcing.. native judge note: Admits it couldn't reach Reddit and relies on secondary coverage, so the core community-sourcing requirement fails despite decent synthesis.
- **community-wsus-alternatives** (claude): native 3 → webctl 10. webctl judge note: Accurate and thread-grounded; pricing/licensing nuances and the granular-approval complaint align with Microsoft's docs and other answers' independent reads.. native judge note: Honest failure mode: names plausible tools and tradeoffs from secondary coverage but cites zero Reddit threads, so it misses the core of the question while staying above a fabricated answer.
- **kafka-min-insync-replicas** (claude): native 8 → webctl 10. webctl judge note: All expected facts plus accurate extra nuance; sources are plausible and relevant.. native judge note: Content is fully correct, but both cited netdata.cloud guide URLs appear non-authoritative and likely fabricated, so sourcing can't be credited.
- **news-microsoft-azure-growth** (claude): native 8 → webctl 10. webctl judge note: All required facts present, matching consensus, with three Microsoft IR sources.. native judge note: Core facts correct and sourced, but the hedged constant-currency claim is vague and slightly at odds with the best-supported 43% CC figure.
- **news-openai-latest-announcement** (claude): native 6 → webctl 8. webctl judge note: Accurate consensus core with solid press sourcing, but less depth and no window reconciliation.. native judge note: Gets the consensus answer right but adds a likely confabulated motive and falsely denies other corroborated in-window announcements.
- **news-tesla-latest-call** (claude): native 6 → webctl 10. webctl judge note: Fully agrees with consensus on every figure and adds well-sourced, mutually corroborated detail with no errors.. native judge note: Good substance but confidently wrong on the call date, an explicitly requested fact.
- **rfc9110-retry-after** (claude): native 7 → webctl 10. webctl judge note: Fully correct and even clarifies the common 429 misconception with accurate attribution.. native judge note: Expected facts present but confidently attributes 429 usage to RFC 9110.
- **us-ev-tax-credit-end** (claude): native 7 → webctl 10. webctl judge note: Fully accurate with real IRS/CRS sources; minor understatement ('4+ years' vs ~7 years to 2032) is technically true and not an expected fact.. native judge note: Core facts correct, but the flatly wrong transition-rule claim contradicts IRS guidance and would mislead a buyer who ordered before the deadline.
- **community-kafka-to-redpanda** (claude): native 8 → webctl 10. webctl judge note: Accurate, balanced, and the inline per-claim attribution makes it the most verifiable answer.. native judge note: Solid reasoning and rubric coverage, but sourcing skews off-platform and the independent-verification claim is sloppy.
- **community-proxmox-vs-vmware** (claude): native 5 → webctl 10. webctl judge note: Thorough and accurate across all rubric items with extensive Reddit sourcing.. native judge note: Admits it couldn't reach Reddit and relies on secondary coverage, so the core community-sourcing requirement fails despite decent synthesis.
- **community-wsus-alternatives** (claude): native 3 → webctl 9. webctl judge note: Excellent breadth and sourcing, but the Tanium/Windows-11 assertion is the one confidently stated 'user claim' that doesn't ring true and can't be corroborated by the other thread readers.. native judge note: Honest failure mode: names plausible tools and tradeoffs from secondary coverage but cites zero Reddit threads, so it misses the core of the question while staying above a fabricated answer.
- **k8s-oomkilled-under-limit** (claude): native 9 → webctl 7. webctl judge note: Good on cache accounting and sampling, but omits the tmpfs/emptyDir mechanism and misdates the Memory QoS feature.. native judge note: Covers all four expected mechanisms correctly with plausible sources, though with less technical depth than the top answers.
- **news-f1-latest-race** (claude): native 6 → webctl 10. webctl judge note: All asked facts correct and sourced, with sound reasoning about recency.. native judge note: Winner and date correct, but the asked circuit is missing (with a wrong hedged guess) and a minor win-count detail contradicts consensus.
- **news-microsoft-azure-growth** (claude): native 8 → webctl 10. webctl judge note: Matches consensus; extra Intelligent Cloud segment figures ($39.3B, up 32%) are plausible but independently unverified.. native judge note: Core facts correct and sourced, but the hedged constant-currency claim is vague and slightly at odds with the best-supported 43% CC figure.
- **news-openai-latest-announcement** (claude): native 6 → webctl 8. webctl judge note: Correct consensus answer with good sourcing, docked for an imprecise date.. native judge note: Gets the consensus answer right but adds a likely confabulated motive and falsely denies other corroborated in-window announcements.
- **rfc9110-retry-after** (claude): native 7 → webctl 10. webctl judge note: Complete and correct with real source URLs; slightly less formal on 'non-negative integer' but substantively right.. native judge note: Expected facts present but confidently attributes 429 usage to RFC 9110.
- **us-ev-tax-credit-end** (claude): native 7 → webctl 9. webctl judge note: Expected facts all present, but sloppily conflates the vehicle credits' end date with unrelated credits' termination dates.. native judge note: Core facts correct, but the flatly wrong transition-rule claim contradicts IRS guidance and would mislead a buyer who ordered before the deadline.
- **news-kubernetes-latest-release** (codex): native 8 → webctl 10. webctl judge note: Every expected fact present and correct; the feature is the best-supported one across answers and is described specifically (Beta, default-on, object/external metrics).. native judge note: Core facts correct and sourced, but its headline feature is a lone claim with no independent agreement among the seven other answers, making it the weakest-supported feature pick.

