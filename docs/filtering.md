# Filtering

Jev (TypeSafe's System One model) scores every result. The score is an expected value over four levels, so 1.9 means Jev is nearly sure of level 2.

## Rubric

| score | meaning |
|---|---|
| 0 | off-topic, or a different meaning of the query's words |
| 1 | low value: SEO, affiliate, or content-farm page; listing, aggregator, or vendor pitch; thin listicle; keyword overlap only |
| 2 | useful: substantive, accurate treatment from an established source (reference work, established publication or institution, vendor docs, practitioner write-up, real Q&A) |
| 3 | best available: primary source, official docs, original research, first-hand practitioner experience |

Two rules are spelled out to Jev: an established source is not marked down for being general, and on opinion, experience, recommendation, or comparison queries a community thread (Reddit, HN, Stack Exchange, forums) on the exact question counts as level 3 even when only the opening post is visible.

## Threshold

`min_score` defaults to 1.8: results Jev leans toward calling useful or better. Raise it (`-m 2.5`) for primary sources only; lower it (`-m 1.5`) to let more mainstream explainers through. The eval report (`docs/EVAL_REPORT.md`) has the sweeps behind the default.

## A floor on the count

`--min-results N` (setting `min_results`, default 0 = off) keeps the score cut but insists on at least N results: if fewer pass, the best-scoring dropped results are promoted, highest first, until N are kept. Promoted results are marked `backfilled: true` in JSON and `Kept (below the 1.8 cut; backfilled ...)` in verbose terminal output, so a confident keep and a floor keep are distinguishable. Nothing scoring under 1.0 (the rubric's off-topic boundary) is ever promoted, and the summary line says when the floor could not be reached.

## Modes

- **Per result** (default): one Jev request per result, 8 in flight. About 0.4s per search.
- **`--batch`**: every result in one request. Fewer tokens; large batches can exceed Jev's request size.
- **`--noul "question?"`**: a yes/no question per result instead of a score; `min_score` becomes the minimum P(yes), default 0.5. Example: `--noul "Is this a research paper or preprint?"`.
- **`--rubric "a,b,c,d"`**: your own levels, lowest to highest. The default cut is 0.2 below the second-highest level.

## Output fields

`score`, `max_score`, `confidence` (how far Jev is from a coin flip), `probabilities` per level. `--verbose` prints dropped results with the reason.
