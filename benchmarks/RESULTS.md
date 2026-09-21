# Rerun after bounding output: webctl 0.0.028

Same 30 cases and native cells; the three webctl arms re-run with `--scrape-top 3`, `--max-output 20000`, boilerplate stripping, and JSON-page rejection. Every case re-judged in one call, so native scores moved slightly too (judge noise: about 0.1 to 0.2 points).

| harness | metric | native | webctl 0.1.4 | webctl 0.0.028 |
|---|---|---|---|---|
| Claude Code, sonnet | wall clock (mean) | 20.6 s | 18.7 s | 14.6 s (-29%) |
| Claude Code, sonnet | tokens per case (mean) | 96.8k | 88.6k | 69.4k (-28%) |
| Claude Code, sonnet | cost per case (mean) | $0.138 | $0.109 | $0.112 (-19%) |
| Claude Code, sonnet | quality (0–10) | 8.67 | 8.83 | 9.33 (+0.66) |
| Codex, gpt-5.6-terra | wall clock (mean) | 26.4 s | 33.3 s | 32.1 s (+22%) |
| Codex, gpt-5.6-terra | tokens per case (mean) | 62.8k | 52.3k | 47.4k (-25%) |
| Codex, gpt-5.6-terra | quality (0–10) | 9.23 | 9.27 | 9.53 (+0.30) |
| pi, Kimi K3 | tokens per case (mean) | | 27.9k | 14.7k (-47%) |
| pi, Kimi K3 | quality (0–10) | | 9.87 | 9.97 |

**What changed underneath.** Before, 26 of 41 webctl outputs to Claude overflowed Claude Code's inline limit and reached the agent as a 2 KB preview. After, none did: median inline output is 15.8 KB. Claude also needed fewer calls (34 versus 52) and fewer turns, which is where the wall-clock and token savings come from. Codex, which never truncated, still cut fresh input from a median 11.4k to 10.0k tokens per case. pi's payload halved.

**Quality.** Claude with webctl now beats Claude's own search by 0.66 points and is the only Claude arm to cite a source on every case. The two worst cells from the first run (Kubernetes release at 2, PEP 8 at 6) are both 10 now: the fresh page and the exact sentence were in webctl's output all along, below the truncation line. Remaining sub-8 webctl cells: JWST orbit (7, one arm answered the L2 distance loosely), F1 latest race (6, both Claude arms), and the OpenAI announcement on Codex (4, both Codex arms agreed on a minority pick). Nothing in the rerun points at the chunk filter dropping needed text.

**Still true.** Codex is slower with webctl (shell round-trips versus server-side search). The harness baseline still dominates total tokens. Single run per cell.

# First run: webctl 0.1.4 (before output bounding)

webctl 0.1.4, 30 cases, 5 arms, 150 agent runs, no failures. Judge: Kimi K3 via pi, blind to arm. Run file: `~/webctl/benchmarks/2026-09-20T16-55-38.json`. Full generated report at the bottom.

## Headline

| harness | metric | native | webctl | change |
|---|---|---|---|---|
| Claude Code, sonnet | wall clock (mean) | 20.6 s | 18.7 s | -9% |
| Claude Code, sonnet | tokens per case (mean) | 96.8k | 88.6k | -8% |
| Claude Code, sonnet | cost per case (mean) | $0.138 | $0.109 | -21% |
| Claude Code, sonnet | quality (mean, 0–10) | 8.80 | 8.83 | flat |
| Codex, gpt-5.6-terra | wall clock (mean) | 26.4 s | 33.3 s | +26% |
| Codex, gpt-5.6-terra | tokens per case (mean) | 62.8k | 52.3k | -17% |
| Codex, gpt-5.6-terra | quality (mean, 0–10) | 9.43 | 9.27 | -2% |
| pi, Kimi K3 (webctl only) | tokens per case (mean) | | 27.9k | |
| pi, Kimi K3 (webctl only) | quality (mean, 0–10) | | 9.87 | |

Per case, Claude with webctl used fewer total tokens in 25 of 30 and was faster in 22 of 30. Codex with webctl used fewer tokens in 17 of 30 and was faster in 11 of 30.

## What the savings actually are

**Modest on tokens, real on cost, and not from smaller search results.** Median fresh input per case (uncached prompt tokens, which is where search results land) was 20.7k with webctl versus 18.8k native on Claude, and 11.4k versus 15.5k on Codex. Claude's built-in WebSearch returns compact snippets; webctl with `--scrape --filter-chunks` (which both agents used on every search, as instructed) returns more text than that. The token saving on Claude comes from fewer turns (median 2 versus 3), and the cost saving comes mostly from not paying Claude's server-side search fee. Codex's saving is in fresh input: webctl's filtered chunks are smaller than what Codex's native search pulls in.

**The harness baseline dominates.** Every turn re-reads about 20k cached tokens of system prompt and tool definitions on Claude and 16k on Codex. Search payloads are a minority of the total, so no search tool can move the total by more than a few tens of percent in these harnesses. pi, with a 500-token baseline, shows what the payload alone costs: 15k tokens median per case.

## Quality: where webctl wins and loses

Mean quality is flat, but the domains diverge.

| domain | Claude native | Claude webctl | Codex native | Codex webctl |
|---|---|---|---|---|
| community (Reddit/HN) | 6.4 | 9.2 | 9.0 | 9.2 |
| dev-docs | 9.8 | 9.3 | 9.6 | 9.4 |
| long-doc | 10.0 | 8.8 | 9.8 | 9.8 |
| earnings | 8.7 | 9.7 | 9.7 | 9.3 |
| news | 8.0 | 5.8 | 8.8 | 8.0 |

**webctl wins on community questions.** Claude's native search failed the "cite a Reddit thread" criterion on three of five community cases and scored 5 to 6 on them; with webctl it found and cited the threads and scored 8 to 10. Codex's native search already reaches Reddit, so the gap is small there.

**webctl loses on recency.** The worst cell in the run: Claude with webctl answered the latest Kubernetes release with v1.36 (April 2026) and asserted it was current, while every other arm found v1.37 (August 2026). The FOMC meeting date was off by a day. Both are search-freshness problems: Brave returned the older page higher and Jev, which judges relevance not recency, kept it. pi with webctl got both right, so the tool can surface the fresh page; the agent's query phrasing decides whether it does.

**Isolated misreads.** Claude with webctl misstated PEP 8's team limit as 100 (the page says 99), and omitted node-level OOM on the Kubernetes memory case. Codex with webctl picked a minority answer on the OpenAI news case and gave a vague autovacuum formula. Each is a single-run outcome and within the noise of one sample per cell.

## Caveats

- Kimi K3 graded its own answers in the pi arm. Its 9.87 is not comparable with the others.
- One run per cell. Differences under about 1 point or 20% are noise.
- webctl's own Jev token use is not counted. It is billed separately and does not enter the agent's context.
- Both agents used `--scrape --filter-chunks` on every search because the prompt told them to prefer it. A prompt that reserves scraping for when the snippet is not enough would cut webctl's fresh input further.
- Recency grades rest on agreement between arms.

## Full report (after rerun)

# webctl benchmark

Started 2026-09-20 16:55, webctl 0.0.028, judge fireworks/accounts/fireworks/models/kimi-k3. 30 cases × 5 arms.

Tokens are everything the model attended to across all turns: uncached input + cache reads + cache writes + output. Cost is what the harness reported (Codex reports none). Quality is the judge's 0–10 score, blind to arm.

## Savings: webctl vs the harness's own search

| harness | metric | native | webctl | change |
|---|---|---|---|---|
| claude sonnet | wall clock (s, mean) | 20.6 | 14.6 | -29% |
| claude sonnet | tokens (mean per case) | 96762 | 69376 | -28% |
| claude sonnet | output tokens (mean) | 821 | 719 | -12% |
| claude sonnet | cost USD (mean) | 0.138 | 0.112 | -19% |
| claude sonnet | quality (0–10, mean) | 8.67 | 9.33 | +8% |
| claude sonnet | sourced answers | 28/30 | 30/30 | |
| claude sonnet | failed runs | 0 | 0 | |
| codex gpt-5.6-terra | wall clock (s, mean) | 26.4 | 32.1 | +22% |
| codex gpt-5.6-terra | tokens (mean per case) | 62795 | 47359 | -25% |
| codex gpt-5.6-terra | output tokens (mean) | 443 | 592 | +33% |
| codex gpt-5.6-terra | quality (0–10, mean) | 9.23 | 9.53 | +3% |
| codex gpt-5.6-terra | sourced answers | 30/30 | 30/30 | |
| codex gpt-5.6-terra | failed runs | 0 | 0 | |

## Per arm

| arm | cases | failed | wall s (mean) | tokens (mean) | input | cache read | output | cost USD | quality | sourced | tool calls | violations |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| claude-sonnet-webctl | 30 | 0 | 14.6 | 69376 | 717522 | 1342203 | 21564 | 3.354 | 9.33 | 30/30 | 34 webctl | 0 |
| claude-sonnet-native | 30 | 0 | 20.6 | 96762 | 574000 | 2304238 | 24636 | 4.151 | 8.67 | 28/30 | 53 search | 0 |
| codex-terra-webctl | 30 | 0 | 32.1 | 47359 | 347009 | 1056000 | 17756 | 0.000 | 9.53 | 30/30 | 65 webctl | 0 |
| codex-terra-native | 30 | 0 | 26.4 | 62795 | 507081 | 1363456 | 13301 | 0.000 | 9.23 | 30/30 | 76 search | 0 |
| pi-kimi-k3-webctl | 30 | 0 | 31.7 | 14694 | 259636 | 137860 | 43311 | 1.429 | 9.97 | 30/30 | 55 webctl | 0 |

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

