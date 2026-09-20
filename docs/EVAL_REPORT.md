# Eval report: filter vs. no-filter

webctl 0.0.013, 2026-09-20. Scores in this report are on the 0–3 rubric-index scale used at the time; the product now reports the same judgment scaled to 0–10 (multiply by 3.33; the 1.8 cut is today's 6). The numbers come from eval runs made while the suite was being built (then stored in a SQLite file, since replaced by JSON run files under `~/webctl/evals/`); regenerate current tables with `webctl eval report --compare`.

## Summary

Jev filtering removes almost all of the noise in a web search's top-k and keeps the results an expert would want, at a cost of about half a second per query.

On the reference run (29 cases, local SearXNG aggregating Google and others, version 0.0.013):

| | raw top-k | after filter |
|---|---|---|
| results delivered | 439 | 162 |
| characters delivered | 76,955 | 29,011 |
| hand-labelled junk domains delivered | 4 | 0 |
| pages the source-quality audit flags as SEO/affiliate/content-farm | 157 | 1 |
| results on the domains where the best answers live (Reddit, HN, GitHub, Stack Overflow) | 14 | 14 |
| expected themes covered | 55/57 | 51/57 |
| cases passing | | 22/29 |

On the best provider (Exa's neural index, version 0.0.007, run 4) the same shape holds with more to work with: 424 raw results and 113 labelled junk became 215 results and 7 junk, with every one of 57 expected themes still covered and 23 of 24 expected-domain hits retained.

The filter's median cost is 0.44s of wall clock per query (Jev per-result scoring, 8 concurrent requests) on top of the provider's own latency. A filtered search end to end is 1.5 to 3.5 seconds.

## What was evaluated

29 cases in `evals/cases/`, 24 of them new for this report:

| group | cases | what makes them hard |
|---|---|---|
| Reddit is where the best answers live | quiet keyboard switches, espresso grinders, SF-to-NYC neighborhoods, Kindle Scribe | opinion queries; affiliate "best of" listicles outrank the threads |
| Hacker News | SQLite in production, monolith returns, Tailscale vs WireGuard | AI-written tech blogs restate the discussion |
| GitHub | Rust TUI library, pgvector, uv, LLM router | aggregator and tutorial sites around the repos |
| SEO-dominated topics | VPN at home, protein intake, desk back pain, closing a credit card | 40–70% of the top 20 are affiliate or content-farm pages |
| Ambiguous terms | python (snake), mercury (planet), java (island), go (board game), crane (bird), jaguar (cat) | the other meaning, plus the usual farms |
| Technical research | DPO vs RLHF, Postgres bloat, JWT refresh rotation, Kubernetes OOMKilled, mechanistic interpretability, the Transformer paper | mostly clean top-k; tests that the filter does not over-prune |

Each case runs one search and then three stages on the same results:

- **nofilter**: the raw top-k, as it would reach a context window without Jev.
- **filter**: Jev scores each result 0–3 on topic and source quality; results at 1.8 or above are kept (the product default).
- **scrape** (9 cases): the kept pages are fetched, split into ~2,000-character chunks, and only the chunks Jev says are worth quoting are kept.

For each stage the runner records what would be delivered (results, characters, hits on hand-labelled junk domains, hits on the domains where the best answers live) and asks Jev, in one batch request, whether the delivered text covers the case's expected themes. Two independent noise measures are used: `junk_domains`, hand-labelled per case from observed raw results (21 cases, 113 labelled hits on Exa), and a separate Jev audit that flags SEO, affiliate, and content-farm pages with a differently worded yes/no question. Thresholds and prompts were tuned with the audit and pass/fail; the junk labels are the ground truth the report leans on.

A run can re-judge an earlier run's provider results (`--reuse-searches`), so two filter versions can be compared on identical inputs. Filter-stage timing includes only Jev.

## Filter vs. no-filter, per case

Reference run 13 (0.0.013, SearXNG). "flagged" is the audit; "junk" is the hand labels; "expected-domain" counts results on the case's `expected_domains`.

| case | tags | raw results | kept | raw chars | kept chars | junk raw→kept | flagged raw→kept | expected-domain raw→kept | themes raw | themes kept | filter ms | pass |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| amb-crane-migration | ambiguous,noise | 14 | 8 | 3439 | 1990 | 0→0 | 6→0 | 0→0 | 2/2 | 2/2 | 482 | ✓ |
| amb-go-board-game | ambiguous,noise | 13 | 3 | 3199 | 744 | 1→0 | 3→0 | 0→0 | 2/2 | 2/2 | 384 | ✓ |
| amb-java-island | ambiguous,noise,travel | 15 | 1 | 4136 | 584 | 1→0 | 12→0 | 0→0 | 2/2 | 2/2 | 508 | ✓ |
| amb-mercury-planet | ambiguous,noise | 15 | 14 | 3657 | 3527 | 0→0 | 1→0 | 0→0 | 2/2 | 2/2 | 471 | ✗ |
| amb-python-snake | ambiguous,noise | 15 | 6 | 3531 | 1508 | 0→0 | 6→0 | 0→0 | 2/2 | 2/2 | 446 | ✓ |
| ambiguous-jaguar |  | 15 | 3 | 3009 | 551 | 1→0 | 3→0 | 0→0 | 2/2 | 2/2 | 381 | ✓ |
| attention-paper |  | 5 | 3 | 800 | 480 | 0→0 | 0→0 | 0→0 | 2/2 | 2/2 | 255 | ✓ |
| github-llm-router | github,code,noise | 15 | 5 | 2424 | 807 | 0→0 | 5→0 | 1→1 | 2/2 | 2/2 | 514 | ✓ |
| github-pgvector | github,code,noise | 15 | 9 | 2249 | 1326 | 0→0 | 1→0 | 1→1 | 2/2 | 1/2 | 457 | ✗ |
| github-rust-tui | github,code | 15 | 8 | 2305 | 1181 | 0→0 | 4→0 | 2→2 | 2/2 | 2/2 | 482 | ✓ |
| github-uv | github,code | 15 | 3 | 2365 | 464 | 0→0 | 5→0 | 1→1 | 2/2 | 2/2 | 395 | ✓ |
| go-generics |  | 15 | 4 | 2388 | 655 | 0→0 | 5→0 | 0→0 | 2/2 | 2/2 | 420 | ✓ |
| hn-monolith-return | hackernews,engineering,noise | 14 | 2 | 2206 | 323 | 0→0 | 10→0 | 0→0 | 2/2 | 2/2 | 365 | ✗ |
| hn-sqlite-production | hackernews,engineering | 15 | 5 | 2433 | 816 | 0→0 | 3→0 | 1→1 | 2/2 | 1/2 | 389 | ✗ |
| hn-tailscale-wireguard | hackernews,engineering | 15 | 2 | 2428 | 323 | 0→0 | 8→0 | 2→2 | 2/2 | 2/2 | 474 | ✓ |
| mech-interp |  | 20 | 15 | 3164 | 2356 | 0→0 | 2→0 | 0→0 | 2/2 | 2/2 | 546 | ✓ |
| noul-research-only |  | 14 | 7 | 2213 | 1118 | 0→0 | 0→0 | 0→0 | 1/1 | 1/1 | 417 | ✓ |
| reddit-espresso-grinder | reddit,opinion,noise | 15 | 3 | 2412 | 488 | 0→0 | 7→0 | 1→1 | 2/2 | 2/2 | 423 | ✓ |
| reddit-kindle-scribe | reddit,opinion,noise | 15 | 7 | 2378 | 1095 | 0→0 | 3→0 | 1→1 | 2/2 | 2/2 | 449 | ✓ |
| reddit-nyc-neighborhood | reddit,opinion,noise | 15 | 2 | 2389 | 320 | 0→0 | 12→0 | 1→1 | 1/2 | 0/2 | 380 | ✗ |
| reddit-quiet-switches | reddit,opinion,noise | 15 | 2 | 2404 | 326 | 0→0 | 13→0 | 1→1 | 2/2 | 2/2 | 508 | ✓ |
| seo-back-pain-desk | seo,noise,health | 20 | 9 | 3221 | 1452 | 0→0 | 8→0 | 0→0 | 2/2 | 2/2 | 583 | ✓ |
| seo-close-credit-card | seo,noise,finance | 16 | 12 | 2577 | 1927 | 0→0 | 3→1 | 0→0 | 2/2 | 2/2 | 416 | ✓ |
| seo-protein-intake | seo,noise,health | 18 | 9 | 2880 | 1452 | 0→0 | 5→0 | 0→0 | 2/2 | 2/2 | 517 | ✓ |
| seo-vpn-home-wifi | seo,noise,security | 20 | 1 | 3209 | 160 | 0→0 | 11→0 | 0→0 | 1/2 | 1/2 | 482 | ✗ |
| tech-dpo-vs-rlhf | research,ml | 15 | 4 | 2383 | 632 | 0→0 | 6→0 | 0→0 | 2/2 | 1/2 | 404 | ✗ |
| tech-jwt-refresh-rotation | engineering,security,noise | 15 | 3 | 2406 | 472 | 0→0 | 4→0 | 0→0 | 2/2 | 2/2 | 466 | ✓ |
| tech-k8s-oomkilled | engineering,kubernetes | 15 | 5 | 2352 | 800 | 0→0 | 8→0 | 2→2 | 2/2 | 2/2 | 411 | ✓ |
| tech-postgres-bloat | engineering,databases | 15 | 7 | 2398 | 1134 | 1→0 | 3→0 | 0→0 | 2/2 | 2/2 | 409 | ✓ |

What the table shows, by group:

- **Reddit and HN cases.** Every expected-domain hit survives the filter (14 of 14 on this run; 23 of 24 on Exa). On the keyboard-switch query the filter dropped 13 audit-flagged listicles and kept the two community threads. The two failing Reddit/HN cases (`reddit-nyc-neighborhood`, `hn-sqlite-production`) fail on theme coverage, not on what was kept: Google-style snippets for a Reddit thread are one or two lines, and the judge sees only that text.
- **SEO cases.** The filter delivers 1 audit-flagged page out of 27 flagged in the raw top-20s. `seo-vpn-home-wifi` keeps a single result because the honest answer to that query (a Reddit thread) was the only non-marketing page in the top 20; the case's second theme is then uncovered. The remaining pass, with 9 to 12 kept out of 16 to 20, and the kept set on the credit-card query is the bureaus, the CFPB, and the banks.
- **Ambiguous terms.** Modern engines rarely return the other meaning for a phrased query, so the noise here is farms, not the other sense. The filter keeps 1 to 8 of 15 and covers every theme; `amb-mercury-planet` fails only because 14 NASA, ESA, and museum pages were all legitimately kept against a bound of 10 (raised to 15 after this run).
- **Research and code.** Where the whole top-k is papers or repositories the filter keeps most of it (mech-interp 15 of 20, rust-tui 8 of 15); `github-pgvector` and `tech-dpo-vs-rlhf` fail on one theme each from snippet-only judging.

## Scrape-to-chunk

Nine cases fetch the kept pages and keep only chunks Jev judges worth quoting. The measures are page fetch success, how much text the chunk filter removes, whether the kept text still covers the themes, and, as a recall check, whether the text that was thrown away covered any theme on its own.

| run | provider | pages ok / failed | chunks kept / total | chars raw → kept | themes covered by kept text | themes still present in dropped text |
|---|---|---|---|---|---|---|
| 4 (0.0.007) | Exa | 44 / 11 | 350 / 466 | 780,638 → 608,246 | 18/18 | not measured |
| 13 (0.0.013) | SearXNG | 34 / 10 | 249 / 369 | 627,344 → 444,383 | 17/18 | 10/18 |

The chunk filter removes 22 to 30 percent of fetched text and the kept text covers the themes as well as the whole page did. The dropped text still "covers" themes in 10 of 18 judgments because long pages repeat themselves; that is redundancy removed, not signal lost. Chunk probabilities are well separated on a Reddit thread (navigation 0.08, weak comment 0.44, substantive comments 0.81 to 0.94), so the 0.5 cut is not a sensitive knob.

Fetch failures are bot walls and PDFs: DOI resolvers and publishers answer 403, and academic PDFs are not parsed. Since 0.0.003 a page that cannot be fetched falls back to the provider's own excerpt, so those results still contribute text (the `seo-protein-intake` scrape stage passed with 13 of 16 fetches failing). Reddit specifically is handled: the JavaScript challenge is solved and the comment tree, which ships inside a `<template>` element, is read.

## Scrape vs. no scrape

Two runs on 2026-09-20 with the scrape stage forced on all 29 cases, comparing what the filter stage delivered (title, URL, and the provider's snippet or excerpt) against the scraped, chunk-filtered pages of the same kept results. Jev judged theme coverage on both.

| provider | themes covered: filter → scrape | chars to context: filter → scrape (raw pages) | Jev input tokens: filter → scrape | avg stage time | pages fetched / failed |
|---|---|---|---|---|---|
| Parallel (2.8K-char excerpts per result) | 55/57 → 51/57 (better on 0 cases, worse on 4) | 86,796 → 1,303,936 (1,971,553) | 572K → 1,454K | 0.46s → 1.9s | 124 / 31 (20%) |
| SearXNG (170-char Google snippets) | 51/57 → 54/57 (better on 4, worse on 2) | 31,068 → 1,262,669 (2,068,892) | 463K → 1,533K | 0.54s → 1.6s | 140 / 22 (14%) |

Reading: when the provider already returns excerpts, scraping adds nothing Jev can detect and costs about 15× the context, 2.5× the Jev tokens, and 4× the time. When the provider returns snippets only, scraping recovers coverage the snippets could not show (`reddit-nyc-neighborhood` went from 0 to 2 themes). The chunk filter removes 34 to 39 percent of fetched text. Fetch failures are bot walls (arXiv, Britannica, Stack Overflow, Forbes, IEEE, NASA answer 403), PDFs, and pages with no readable text (Facebook); a failed fetch falls back to the provider excerpt. Caveat: the theme judge reads at most 2,500 characters per result in both stages, so a long page whose relevant passage sits past that cap can judge worse than its own snippet.

## Speed

| stage | avg ms per case (run 13) | avg ms per case (run 4, Exa) |
|---|---|---|
| provider search | 1,529 (SearXNG, cold) | 611 |
| audit + no-filter judge | 146 | 164 |
| filter (Jev scoring + judge) | 443 | 600 |
| scrape + chunk filter + judge (9 cases) | 1,458 | 1,512 |
| whole case, wall clock | 2,569 avg, 5,051 max | 1,844 avg, 4,437 max |

The full 29-case suite runs in 40 seconds at two cases in parallel. A filtered search from the CLI is 1.5 to 3.5 seconds; adding `--scrape --filter-chunks` for 7 pages adds about 1.5 seconds because fetches and chunk requests run concurrently.

Jev cost: filtering 439 results took 456K input tokens in per-result mode (one request per result, each carrying the rubric). Batch mode (all results in one request) cut that by about 18 percent per case on a 15-case comparison with a comparable pass rate, but is not the default: the eval judge already hit Jev's request-size limit at 20 results, so batches would need splitting.

## Changes made during this work, with evidence

Every change bumped `internal/version.Version`; each run below is a full 29-case pass unless noted.

| version | change | effect |
|---|---|---|
| 0.0.002 | Keyless Exa and Parallel via their hosted MCP servers; budgeted fallback chain (12s per attempt, 30s total) | Searches work with no keys after DuckDuckGo started refusing this IP |
| 0.0.003 | Scrape falls back to the provider's excerpt when a page cannot be fetched | Walled and PDF results keep contributing text |
| 0.0.004 | Rubric v2: score topic and source quality together; default threshold 1.0 → 2.0 | On Exa, with the eval still cutting at 1.0, the new rubric kept 400 of 450 and delivered 91 audit-flagged pages; at the product's 2.0 cut (run 2) it kept 227 of 445 and delivered 1 |
| 0.0.006 | Rubric v3: a community thread on an opinion or experience question is a best-available source | Reddit threads moved from 1.7–1.96 (just under the cut) to 2.3–2.6; `reddit-nyc-neighborhood` went from dropping all six threads to keeping four; 24/29 pass |
| 0.0.007 | Collapse results with the same URL or title | `attention-paper`: ten copies of the same paper became three results |
| 0.0.008 | Snippets start at the first prose line, not page chrome | Exa's degraded keyword mode had been feeding "Skip to main content… Subscribe…" to the judge |
| 0.0.009 | Chain tops up a short answer (under half the requested count) from the next provider, fused by reciprocal rank | Throttled tiers answer with 10 truncated results instead of an error |
| 0.0.010 | Rubric v4: level 1 is reserved for SEO, affiliate, farm, listing, and pitch pages; level 2 names reference works and established publications | Britannica, LiveScience, agency pages had been scoring 1.8–1.97 and being cut on general-reference queries |
| 0.0.011 | You.com keyless backend; a provider answering 429 or 402 is skipped for 90s per process; 3s dial timeout | A dead DuckDuckGo had cost 12s per search; a throttled tier now costs one request per run |
| 0.0.013 | Default threshold 2.0 → 1.8 | Between 1.75 and 2.0 sat Wikipedia, museums, zoos, Reddit threads, Auth0 docs and no labelled junk; 151 kept became 184 on the same inputs with junk still 0 |

Threshold sweep on the v3-rubric Exa inputs (run 3), count bounds and junk labels only:

| threshold | count-bound passes | kept | junk kept / labelled | expected-domain kept / present |
|---|---|---|---|---|
| 1.9 | 24/29 | 229 | 9/113 | 23/24 |
| 2.0 | 25/29 | 215 | 7/113 | 23/24 |
| 2.2 | 26/29 | 188 | 4/113 | 23/24 |
| 2.4 | 26/29 | 174 | 2/113 | 22/24 |

The same sweep on v4-rubric You.com inputs (run 10) kept junk at 0/16 from 1.7 through 2.0 while kept results went from 199 to 151, which is what moved the default to 1.8.

## Caveats

- **The provider decides what the filter has to work with.** Exa's neural index returns papers and repositories for research queries and long excerpts (2–8K characters per result); SearXNG and You.com return keyword results with 150–500 character snippets. Jev scores on title, URL, and snippet, and the theme judge reads the same text, so short snippets both lower scores and undercount coverage. The scrape stage is the remedy when it matters.
- **Free tiers throttle.** Exa's and Parallel's keyless endpoints rate-limited after a few dozen searches and stayed limited for over an hour, and You.com's free profile answered 402 after about 70; DuckDuckGo refused TCP connections from this address all day. Runs 8, 9, and 11 are partial or single-provider for that reason and are not in the headline. The local SearXNG documented in the README has no such limit and is what the reference run used.
- **Results drift between runs.** Engines return different pages for the same query hours apart, so per-case pass/fail moves by one or two cases run to run. `--reuse-searches` makes any two runs of the same inputs comparable; the version history above is not one input set.
- **Junk labels are judgment calls.** Four hosts first labelled junk on the SQLite case (raxxo.shop, ultrathink.art, prodsens.live, 0x.run) were removed after reading their snippets: first-hand engineering posts with specific numbers, whatever the domain looks like. The remaining borderline survivors (a realtor's guide with actual price data, a keyboard guide claiming six months of hands-on use) are counted against the filter.
- **Case bounds were corrected during the work** where a whole top-k was legitimately top-tier (12 meta-analyses for "what the studies say"). Those corrections are in the commit history; none loosened a junk or expected-domain check.

## Reproduce

```bash
go build -o webctl ./cmd/webctl
docker run -d --name searxng -p 8899:8080 -v "$PWD/docs/searxng/settings.yml:/etc/searxng/settings.yml:ro" searxng/searxng:latest
SEARXNG_URL=http://localhost:8899 WEBCTL_PROVIDER=searxng ./webctl eval
./webctl eval report --compare          # tables per version, latest run each
./webctl eval --verbose reddit-quiet-switches   # every judged score, keep/drop
```
