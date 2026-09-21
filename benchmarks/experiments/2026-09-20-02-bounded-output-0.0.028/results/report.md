# webctl benchmark

Started 2026-09-20 16:55, webctl 0.0.028, judge fireworks/accounts/fireworks/models/kimi-k3. 30 cases × 5 arms.

Tokens are everything the model attended to across all turns: uncached input + cache reads + cache writes + output. Cost is what the harness reported (Codex reports none). Quality is the judge's 0–10 score, blind to arm.

## Savings: webctl vs the harness's own search

| harness | metric | native | webctl | change |
|---|---|---|---|---|
| claude sonnet → webctl | wall clock (s, mean) | 20.6 | 14.6 | -29% |
| claude sonnet → webctl | tokens (mean per case) | 96762 | 69376 | -28% |
| claude sonnet → webctl | search payload tokens (mean, chars/4) | 1040 | 4637 | +346% |
| claude sonnet → webctl | output tokens (mean) | 821 | 719 | -12% |
| claude sonnet → webctl | cost USD (mean) | 0.138 | 0.112 | -19% |
| claude sonnet → webctl | quality (0–10, mean) | 8.67 | 9.33 | +8% |
| claude sonnet → webctl | sourced answers | 28/30 | 30/30 | |
| claude sonnet → webctl | failed runs | 0 | 0 | |
| codex gpt-5.6-terra → webctl | wall clock (s, mean) | 26.4 | 32.1 | +22% |
| codex gpt-5.6-terra → webctl | tokens (mean per case) | 62795 | 47359 | -25% |
| codex gpt-5.6-terra → webctl | search payload tokens (mean, chars/4) | hidden | 6798 | |
| codex gpt-5.6-terra → webctl | output tokens (mean) | 443 | 592 | +33% |
| codex gpt-5.6-terra → webctl | quality (0–10, mean) | 9.23 | 9.53 | +3% |
| codex gpt-5.6-terra → webctl | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra → webctl | failed runs | 0 | 0 | |

## Per arm

| arm | cases | failed | wall s (mean) | tokens (mean) | payload tok (mean) | input | cache read | output | cost USD | quality | sourced | tool calls | violations |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| claude-sonnet-webctl | 30 | 0 | 14.6 | 69376 | 4637 | 717522 | 1342203 | 21564 | 3.354 | 9.33 | 30/30 | 34 webctl | 0 |
| claude-sonnet-native | 30 | 0 | 20.6 | 96762 | 1040 | 574000 | 2304238 | 24636 | 4.151 | 8.67 | 28/30 | 53 search | 0 |
| codex-terra-webctl | 30 | 0 | 32.1 | 47359 | 6798 | 347009 | 1056000 | 17756 | 0.000 | 9.53 | 30/30 | 65 webctl | 0 |
| codex-terra-native | 30 | 0 | 26.4 | 62795 | 0 | 507081 | 1363456 | 13301 | 0.000 | 9.23 | 30/30 | 76 search | 0 |
| pi-kimi-k3-webctl | 30 | 0 | 31.7 | 14694 | 7393 | 259636 | 137860 | 43311 | 1.429 | 9.97 | 30/30 | 55 webctl | 0 |

## Per domain (quality mean / tokens mean)

| domain | cases | claude-sonnet-webctl | claude-sonnet-native | codex-terra-webctl | codex-terra-native | pi-kimi-k3-webctl |
|---|---|---|---|---|---|---|
| community | 5 | 9.6 / 71k | 6.0 / 126k | 9.8 / 87k | 9.6 / 78k | 10.0 / 28k |
| dev-docs | 10 | 9.5 / 67k | 9.6 / 84k | 9.5 / 40k | 9.3 / 52k | 9.9 / 14k |
| earnings | 3 | 9.7 / 68k | 8.3 / 101k | 9.7 / 42k | 9.7 / 91k | 10.0 / 17k |
| long-doc | 5 | 9.4 / 66k | 10.0 / 92k | 10.0 / 30k | 10.0 / 66k | 10.0 / 9k |
| news | 4 | 8.8 / 74k | 8.0 / 109k | 8.2 / 50k | 7.2 / 54k | 10.0 / 13k |
| policy | 1 | 9.0 / 64k | 7.0 / 81k | 10.0 / 42k | 9.0 / 71k | 10.0 / 5k |
| science | 2 | 8.5 / 82k | 10.0 / 80k | 10.0 / 32k | 9.5 / 46k | 10.0 / 6k |

## Per case (quality · seconds · tokens)

| case | domain | claude-sonnet-webctl | claude-sonnet-native | codex-terra-webctl | codex-terra-native | pi-kimi-k3-webctl |
|---|---|---|---|---|---|---|
| attention-paper-dimensions | long-doc | 9 · 11s · 67k | 10 · 14s · 79k | 10 · 16s · 23k | 10 · 17s · 44k | 10 · 14s · 7k |
| clickhouse-final-modifier | dev-docs | 9 · 13s · 65k | 10 · 27s · 111k | 9 · 29s · 45k | 8 · 23s · 45k | 10 · 59s · 34k |
| community-bun-vs-node-production | community | 10 · 22s · 79k | 7 · 37s · 146k | 9 · 50s · 71k | 9 · 31s · 65k | 10 · 41s · 30k |
| community-kafka-to-redpanda | community | 9 · 28s · 74k | 7 · 38s · 114k | 10 · 54s · 84k | 9 · 34s · 80k | 10 · 81s · 34k |
| community-macos-operation-not-permitted | community | 10 · 15s · 64k | 9 · 19s · 79k | 10 · 43s · 72k | 10 · 25s · 46k | 10 · 34s · 12k |
| community-proxmox-vs-vmware | community | 9 · 25s · 71k | 4 · 34s · 115k | 10 · 63s · 115k | 10 · 46s · 79k | 10 · 52s · 27k |
| community-wsus-alternatives | community | 10 · 19s · 66k | 3 · 50s · 173k | 10 · 61s · 92k | 10 · 51s · 116k | 10 · 41s · 35k |
| go-context-cancellation | dev-docs | 10 · 10s · 67k | 10 · 18s · 79k | 10 · 15s · 26k | 10 · 17s · 46k | 10 · 22s · 13k |
| jwst-orbit | science | 7 · 10s · 63k | 10 · 13s · 79k | 10 · 14s · 24k | 10 · 20s · 45k | 10 · 13s · 5k |
| k8s-api-deprecation-ingress | long-doc | 10 · 14s · 66k | 10 · 15s · 79k | 10 · 15s · 25k | 10 · 18s · 45k | 10 · 17s · 7k |
| k8s-oomkilled-under-limit | dev-docs | 9 · 18s · 63k | 8 · 32s · 83k | 10 · 75s · 87k | 10 · 57s · 79k | 10 · 75s · 34k |
| kafka-kraft-quorum-voters | dev-docs | 9 · 11s · 67k | 10 · 15s · 79k | 8 · 17s · 26k | 9 · 20s · 45k | 10 · 23s · 7k |
| kafka-min-insync-replicas | dev-docs | 9 · 16s · 68k | 9 · 18s · 79k | 9 · 39s · 57k | 8 · 18s · 45k | 10 · 25s · 6k |
| news-f1-latest-race | news | 6 · 11s · 66k | 5 · 21s · 109k | 10 · 25s · 45k | 10 · 17s · 45k | 10 · 20s · 8k |
| news-fomc-latest-rate | news | 10 · 11s · 62k | 10 · 23s · 109k | 9 · 27s · 37k | 10 · 18s · 45k | 10 · 21s · 11k |
| news-kubernetes-latest-release | news | 10 · 16s · 105k | 9 · 18s · 136k | 10 · 23s · 43k | 5 · 17s · 45k | 10 · 30s · 17k |
| news-microsoft-azure-growth | earnings | 9 · 11s · 69k | 9 · 22s · 110k | 10 · 17s · 27k | 10 · 34s · 101k | 10 · 19s · 7k |
| news-nvidia-latest-quarter | earnings | 10 · 8s · 65k | 10 · 27s · 111k | 10 · 18s · 27k | 10 · 27s · 93k | 10 · 33s · 23k |
| news-openai-latest-announcement | news | 9 · 10s · 62k | 8 · 14s · 80k | 4 · 52s · 73k | 4 · 32s · 78k | 10 · 49s · 12k |
| news-tesla-latest-call | earnings | 10 · 22s · 68k | 6 · 17s · 81k | 9 · 49s · 71k | 9 · 30s · 77k | 10 · 43s · 18k |
| pep8-line-length | long-doc | 10 · 8s · 63k | 10 · 9s · 78k | 10 · 15s · 24k | 10 · 17s · 46k | 10 · 14s · 5k |
| postgres-autovacuum-threshold | dev-docs | 10 · 12s · 65k | 10 · 13s · 79k | 10 · 18s · 24k | 9 · 26s · 45k | 9 · 22s · 6k |
| redis-xautoclaim | dev-docs | 10 · 15s · 67k | 10 · 13s · 80k | 10 · 18s · 25k | 10 · 22s · 45k | 10 · 26s · 7k |
| rfc9110-retry-after | long-doc | 8 · 14s · 61k | 10 · 16s · 109k | 10 · 42s · 49k | 10 · 23s · 80k | 10 · 40s · 15k |
| rust-pin-future-poll | dev-docs | 10 · 16s · 68k | 9 · 22s · 81k | 9 · 31s · 44k | 9 · 26s · 47k | 10 · 40s · 15k |
| sodium-daily-limits | science | 10 · 13s · 100k | 10 · 12s · 80k | 10 · 25s · 39k | 9 · 22s · 45k | 10 · 14s · 6k |
| sqlite-wal-autocheckpoint | dev-docs | 9 · 12s · 65k | 10 · 16s · 80k | 10 · 18s · 25k | 10 · 17s · 45k | 10 · 15s · 6k |
| terraform-moved-block | dev-docs | 10 · 17s · 66k | 10 · 10s · 78k | 10 · 29s · 39k | 10 · 27s · 71k | 10 · 21s · 7k |
| tls13-key-share | long-doc | 10 · 16s · 69k | 10 · 21s · 110k | 10 · 23s · 26k | 10 · 32s · 111k | 10 · 23s · 8k |
| us-ev-tax-credit-end | policy | 9 · 16s · 63k | 7 · 13s · 81k | 10 · 41s · 42k | 9 · 28s · 71k | 10 · 27s · 4k |

## Quality losses and wins with webctl (score difference ≥ 2)

- **community-bun-vs-node-production** (claude): native 7 → webctl 10. webctl judge note: Thorough and well-sourced; covers stability, ecosystem, governance, and serverless angles beyond the rubric with nothing incorrect.. native judge note: Meets the rubric on benefits/problems/URLs but dilutes primary-source reporting with dubious SEO-blog statistics presented confidently.
- **community-kafka-to-redpanda** (claude): native 7 → webctl 9. webctl judge note: Comprehensive and well-sourced with accurate thread attribution, minus one garbled detail in the otherwise-correct Vanlightly/fsync durability debate.. native judge note: Rich on the benchmark-war nuance but contains a confidently wrong claim about absent independent verification and partly off-scope/possibly hallucinated sources.
- **community-proxmox-vs-vmware** (claude): native 4 → webctl 9. webctl judge note: Strong on trigger, pain points, and sourcing, but lighter than peers on the range of reasons users cite for choosing Proxmox.. native judge note: Facts are mostly right but sourced entirely from secondary coverage, failing the core requirement of Reddit-thread grounding.
- **community-wsus-alternatives** (claude): native 3 → webctl 10. webctl judge note: Comprehensive, accurate, and heavily sourced with real Reddit threads; all rubric items exceeded.. native judge note: Honest about its inability to access Reddit and its general claims are directionally correct, but it fails the core question and the source requirement.
- **jwst-orbit** (claude): native 10 → webctl 7. webctl judge note: Core facts are right, but the closing sentence incorrectly states Webb's distance from Earth ranges 250,000–830,000 km — those figures describe its distance from L2, making the claim confusing and partially wrong.. native judge note: Both expected facts correct, extra orbital detail accurate, and sources include real NASA and Space.com URLs.
- **news-tesla-latest-call** (claude): native 6 → webctl 10. webctl judge note: Covers all consensus facts plus corroborated specifics with a transcript source and the official IR deck.. native judge note: Core facts all correct and sourced, but the explicitly requested call date is confidently wrong.
- **rfc9110-retry-after** (claude): native 10 → webctl 8. webctl judge note: Every expected fact is correct, but the confidently wrong parenthetical asserting 429 is not in RFC 9110 costs points.. native judge note: Complete and correct; the 429 addition is consistent with RFC 9110 §15.5.21, so nothing wrong.
- **us-ev-tax-credit-end** (claude): native 7 → webctl 9. webctl judge note: All expected facts present, but the supplementary '4+ years early' claim is wrong; ~$190B CRS figure is plausible.. native judge note: Core facts right, but it confidently misstates the acquisition rule, directly contradicting IRS guidance on a material point.
- **news-kubernetes-latest-release** (codex): native 5 → webctl 10. webctl judge note: Matches the consensus version, date, and headline feature with proper sourcing.. native judge note: Version and date match consensus, but the headline feature diverges from the best-supported answer without stronger sourcing.

