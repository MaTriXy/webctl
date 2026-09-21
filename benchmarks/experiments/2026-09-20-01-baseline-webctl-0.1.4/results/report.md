# webctl benchmark

Started 2026-09-20 16:55, webctl 0.1.4, judge fireworks/accounts/fireworks/models/kimi-k3. 30 cases × 5 arms.

Tokens are everything the model attended to across all turns: uncached input + cache reads + cache writes + output. Cost is what the harness reported (Codex reports none). Quality is the judge's 0–10 score, blind to arm.

## Savings: webctl vs the harness's own search

| harness | metric | native | webctl | change |
|---|---|---|---|---|
| claude sonnet → webctl | wall clock (s, mean) | 20.6 | 18.7 | -9% |
| claude sonnet → webctl | tokens (mean per case) | 96762 | 88565 | -8% |
| claude sonnet → webctl | search payload tokens (mean, chars/4) | 1040 | 0 | -100% |
| claude sonnet → webctl | output tokens (mean) | 821 | 866 | +6% |
| claude sonnet → webctl | cost USD (mean) | 0.138 | 0.109 | -21% |
| claude sonnet → webctl | quality (0–10, mean) | 8.80 | 8.83 | +0% |
| claude sonnet → webctl | sourced answers | 28/30 | 30/30 | |
| claude sonnet → webctl | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl | wall clock (s, mean) | 26.4 | 33.3 | +26% |
| codex gpt-5.6-terra → webctl | tokens (mean per case) | 62795 | 52257 | -17% |
| codex gpt-5.6-terra → webctl | search payload tokens (mean, chars/4) | hidden | 0 | |
| codex gpt-5.6-terra → webctl | output tokens (mean) | 443 | 573 | +29% |
| codex gpt-5.6-terra → webctl | quality (0–10, mean) | 9.43 | 9.27 | -2% |
| codex gpt-5.6-terra → webctl | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl | failed runs | 0 | 0 | |

## Per arm

| arm | cases | failed | wall s (mean) | tokens (mean) | payload tok (mean) | input | cache read | output | cost USD | quality | sourced | tool calls | violations |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| claude-sonnet-webctl | 30 | 0 | 18.7 | 88565 | 0 | 651436 | 1979526 | 25992 | 3.261 | 8.83 | 30/30 | 52 webctl | 0 |
| claude-sonnet-native | 30 | 0 | 20.6 | 96762 | 1040 | 574000 | 2304238 | 24636 | 4.151 | 8.80 | 28/30 | 53 search | 0 |
| codex-terra-webctl | 30 | 0 | 33.3 | 52257 | 0 | 442820 | 1107712 | 17184 | 0.000 | 9.27 | 30/30 | 58 webctl | 0 |
| codex-terra-native | 30 | 0 | 26.4 | 62795 | 0 | 507081 | 1363456 | 13301 | 0.000 | 9.43 | 30/30 | 76 search | 0 |
| pi-kimi-k3-webctl | 30 | 0 | 41.2 | 27932 | 0 | 497044 | 286575 | 54330 | 2.306 | 9.87 | 30/30 | 53 webctl | 1 |

## Per domain (quality mean / tokens mean)

| domain | cases | claude-sonnet-webctl | claude-sonnet-native | codex-terra-webctl | codex-terra-native | pi-kimi-k3-webctl |
|---|---|---|---|---|---|---|
| community | 5 | 9.2 / 136k | 6.4 / 126k | 9.2 / 92k | 9.0 / 78k | 9.6 / 68k |
| dev-docs | 10 | 9.3 / 73k | 9.8 / 84k | 9.4 / 47k | 9.6 / 52k | 9.9 / 25k |
| earnings | 3 | 9.7 / 98k | 8.7 / 101k | 9.3 / 50k | 9.7 / 91k | 10.0 / 14k |
| long-doc | 5 | 8.8 / 70k | 10.0 / 92k | 9.8 / 25k | 9.8 / 66k | 10.0 / 13k |
| news | 4 | 5.8 / 100k | 8.0 / 109k | 8.0 / 65k | 8.8 / 54k | 10.0 / 23k |
| policy | 1 | 10.0 / 61k | 6.0 / 81k | 9.0 / 29k | 9.0 / 71k | 10.0 / 21k |
| science | 2 | 10.0 / 71k | 10.0 / 80k | 10.0 / 37k | 10.0 / 46k | 9.5 / 13k |

## Per case (quality · seconds · tokens)

| case | domain | claude-sonnet-webctl | claude-sonnet-native | codex-terra-webctl | codex-terra-native | pi-kimi-k3-webctl |
|---|---|---|---|---|---|---|
| attention-paper-dimensions | long-doc | 10 · 20s · 70k | 10 · 14s · 79k | 10 · 16s · 25k | 10 · 17s · 44k | 10 · 20s · 11k |
| clickhouse-final-modifier | dev-docs | 9 · 23s · 98k | 10 · 27s · 111k | 9 · 28s · 55k | 9 · 23s · 45k | 10 · 59s · 42k |
| community-bun-vs-node-production | community | 9 · 41s · 139k | 8 · 37s · 146k | 9 · 56s · 94k | 9 · 31s · 65k | 10 · 58s · 66k |
| community-kafka-to-redpanda | community | 9 · 38s · 106k | 6 · 38s · 114k | 9 · 51s · 131k | 8 · 34s · 80k | 9 · 102s · 110k |
| community-macos-operation-not-permitted | community | 10 · 12s · 64k | 8 · 19s · 79k | 9 · 37s · 57k | 10 · 25s · 46k | 10 · 31s · 6k |
| community-proxmox-vs-vmware | community | 8 · 33s · 94k | 5 · 34s · 115k | 9 · 65s · 91k | 8 · 46s · 79k | 9 · 102s · 121k |
| community-wsus-alternatives | community | 10 · 58s · 275k | 5 · 50s · 173k | 10 · 66s · 84k | 10 · 51s · 116k | 10 · 64s · 36k |
| go-context-cancellation | dev-docs | 10 · 12s · 60k | 10 · 18s · 79k | 10 · 29s · 42k | 10 · 17s · 46k | 10 · 29s · 29k |
| jwst-orbit | science | 10 · 13s · 70k | 10 · 13s · 79k | 10 · 20s · 25k | 10 · 20s · 45k | 9 · 32s · 14k |
| k8s-api-deprecation-ingress | long-doc | 10 · 13s · 60k | 10 · 15s · 79k | 10 · 19s · 27k | 10 · 18s · 45k | 10 · 36s · 10k |
| k8s-oomkilled-under-limit | dev-docs | 7 · 24s · 69k | 9 · 32s · 83k | 10 · 84s · 118k | 10 · 57s · 79k | 10 · 133s · 71k |
| kafka-kraft-quorum-voters | dev-docs | 9 · 11s · 60k | 10 · 15s · 79k | 9 · 27s · 45k | 9 · 20s · 45k | 9 · 22s · 8k |
| kafka-min-insync-replicas | dev-docs | 8 · 15s · 68k | 9 · 18s · 79k | 9 · 35s · 54k | 9 · 18s · 45k | 10 · 29s · 9k |
| news-f1-latest-race | news | 5 · 13s · 95k | 6 · 21s · 109k | 10 · 25s · 60k | 10 · 17s · 45k | 10 · 26s · 12k |
| news-fomc-latest-rate | news | 7 · 16s · 95k | 10 · 23s · 109k | 10 · 19s · 27k | 10 · 18s · 45k | 10 · 35s · 20k |
| news-kubernetes-latest-release | news | 2 · 13s · 102k | 9 · 18s · 136k | 9 · 34s · 48k | 9 · 17s · 45k | 10 · 42s · 39k |
| news-microsoft-azure-growth | earnings | 9 · 11s · 60k | 9 · 22s · 110k | 9 · 26s · 45k | 10 · 34s · 101k | 10 · 18s · 14k |
| news-nvidia-latest-quarter | earnings | 10 · 14s · 60k | 10 · 27s · 111k | 10 · 17s · 27k | 10 · 27s · 93k | 10 · 20s · 14k |
| news-openai-latest-announcement | news | 9 · 22s · 105k | 7 · 14s · 80k | 3 · 81s · 124k | 6 · 32s · 78k | 10 · 60s · 19k |
| news-tesla-latest-call | earnings | 10 · 28s · 173k | 7 · 17s · 81k | 9 · 48s · 77k | 9 · 30s · 77k | 10 · 29s · 14k |
| pep8-line-length | long-doc | 6 · 11s · 60k | 10 · 9s · 78k | 10 · 16s · 23k | 10 · 17s · 46k | 10 · 19s · 12k |
| postgres-autovacuum-threshold | dev-docs | 10 · 11s · 60k | 10 · 13s · 79k | 7 · 16s · 25k | 10 · 26s · 45k | 10 · 20s · 15k |
| redis-xautoclaim | dev-docs | 10 · 11s · 60k | 10 · 13s · 80k | 10 · 20s · 26k | 10 · 22s · 45k | 10 · 17s · 7k |
| rfc9110-retry-after | long-doc | 9 · 8s · 62k | 10 · 16s · 109k | 9 · 17s · 23k | 9 · 23s · 80k | 10 · 57s · 15k |
| rust-pin-future-poll | dev-docs | 10 · 20s · 96k | 10 · 22s · 81k | 10 · 22s · 31k | 9 · 26s · 47k | 10 · 51s · 41k |
| sodium-daily-limits | science | 10 · 10s · 70k | 10 · 12s · 80k | 10 · 33s · 48k | 10 · 22s · 45k | 10 · 21s · 12k |
| sqlite-wal-autocheckpoint | dev-docs | 10 · 9s · 60k | 10 · 16s · 80k | 10 · 18s · 25k | 10 · 17s · 45k | 10 · 19s · 8k |
| terraform-moved-block | dev-docs | 10 · 18s · 93k | 10 · 10s · 78k | 10 · 27s · 43k | 10 · 27s · 71k | 10 · 25s · 15k |
| tls13-key-share | long-doc | 9 · 19s · 96k | 10 · 21s · 110k | 10 · 20s · 26k | 10 · 32s · 111k | 10 · 29s · 14k |
| us-ev-tax-credit-end | policy | 10 · 15s · 60k | 6 · 13s · 81k | 9 · 27s · 29k | 9 · 28s · 71k | 10 · 31s · 20k |

## Quality losses and wins with webctl (score difference ≥ 2)

- **community-kafka-to-redpanda** (claude): native 6 → webctl 9. webctl judge note: Accurate, well-sourced, balanced pro/con coverage; misses only the licensing point.. native judge note: Core facts are right, but leans on vendor/likely-invented Medium sources and misses Reddit entirely.
- **community-macos-operation-not-permitted** (claude): native 8 → webctl 10. webctl judge note: All expected facts present and correct with real Reddit and Apple Stack Exchange sources.. native judge note: Core cause and fix correct with real URLs, but the shell-binary claim is misleading and sources miss the Reddit/SE angle.
- **community-proxmox-vs-vmware** (claude): native 5 → webctl 8. webctl judge note: Hits all four rubric items accurately with real Reddit sources, just with less breadth of community-reported detail than the top answers.. native judge note: Substance is mostly right but fails the Reddit-source rubric item outright and concedes it synthesized secondary coverage instead of the community threads asked about.
- **community-wsus-alternatives** (claude): native 5 → webctl 10. webctl judge note: All rubric items satisfied with thread-backed specifics and community sentiment (positive and negative).. native judge note: Honestly disclosed its access failure and gave plausible secondary-source content, but misses the Reddit-source rubric item entirely.
- **k8s-oomkilled-under-limit** (claude): native 9 → webctl 7. webctl judge note: Accurate on metric-vs-cgroup accounting gaps and sampling, but omits the node-level OOM/eviction mechanism entirely and garbles the cgroup v2 detail.. native judge note: Covers all four expected mechanisms with sound framing, but lacks emptyDir specificity and slightly oversimplifies cgroup v2 limit enforcement.
- **news-fomc-latest-rate** (claude): native 10 → webctl 7. webctl judge note: Core rate and decision right, but the meeting date is confidently off by a day versus all other answers.. native judge note: All key facts match consensus; citing the meeting by its Sept 16 statement date is acceptable.
- **news-kubernetes-latest-release** (claude): native 9 → webctl 2. webctl judge note: Confidently outdated: answers a different question (previous release) while asserting it is the latest.. native judge note: Correct and consistent with consensus on every figure; slightly less detailed than B.
- **news-openai-latest-announcement** (claude): native 7 → webctl 9. webctl judge note: Matches the consensus pick with correct date, description, and sources, and handles the window nuance well; docked slightly for unverifiable precise stats.. native judge note: Gets the consensus answer and date right with solid sources, but is thinner and wrongly overlooks the Sept 10 Agents API announcement.
- **news-tesla-latest-call** (claude): native 7 → webctl 10. webctl judge note: All consensus facts present and correct with multiple real sources including Tesla IR.. native judge note: Substance matches the best-supported answers but the explicitly requested call date is confidently wrong.
- **pep8-line-length** (claude): native 10 → webctl 6. webctl judge note: Gets the base limits right but confidently misstates the team-negotiated limit as 100 with an invented rationale; pep8.org mirror is a real source.. native judge note: All expected facts correct with a deep-linked official source.
- **us-ev-tax-credit-end** (claude): native 6 → webctl 10. webctl judge note: Accurate and complete, including the correct IRS acquisition interpretation and a plausible deficit-savings figure, with CRS and IRS sources.. native judge note: Core facts right, but the confidently wrong acquisition rule (possession required) directly contradicts IRS guidance and would mislead a reader acting on it.
- **news-openai-latest-announcement** (codex): native 6 → webctl 3. webctl judge note: Specific, dated, and sourced, but a lone claim that contradicts the well-supported consensus pick and cannot be verified against the other answers.. native judge note: Factually accurate about a real, corroborated in-window launch, but misses the flagship Astra announcement the majority identify as the most significant.
- **postgres-autovacuum-threshold** (codex): native 10 → webctl 7. webctl judge note: Defaults are right but the formula's tuple-count basis is vague and the single source link doesn't back the claims.. native judge note: All expected facts correct with a real PostgreSQL 17 docs URL that documents the parameters.

