# Filtering

Jev (TypeSafe's System One model) scores every result 0–10 against the query and, when given, the `--goal`. Under the hood the judgment is a four-level rubric; Jev returns a probability for each level, the expected level is scaled to 0–10, and that one number is what you see. So 6.0 means "Jev leans useful", 9.5 means "almost certainly the best available".

## Rubric

| level | score | meaning |
|---|---|---|
| 0 | 0 | off-topic, or a different meaning of the query's words |
| 1 | 3.3 | low value: SEO, affiliate, or content-farm page; listing, aggregator, or vendor pitch; thin listicle; keyword overlap only |
| 2 | 6.7 | useful: substantive, accurate treatment from an established source (reference work, established publication or institution, vendor docs, practitioner write-up, real Q&A) |
| 3 | 10 | best available: primary source, official docs, original research, first-hand practitioner experience |

Two rules are spelled out to Jev: an established source is not marked down for being general, and on opinion, experience, recommendation, or comparison queries a community thread (Reddit, HN, Stack Exchange, forums) on the exact question counts as best available even when only the opening post is visible.

## Threshold

`min_score` defaults to 6: results Jev leans toward calling useful or better. Raise it (`-m 8.5`) for primary sources only; lower it (`-m 5`) to let more mainstream explainers through. The eval report (`docs/EVAL_REPORT.md`) has the sweeps behind the default (on its older 0–3 index scale; multiply by 3.33).

## A floor on the count

`--min-results N` (setting `min_results`, default 0 = off) keeps the score cut but insists on at least N results: if fewer pass, the best-scoring dropped results are promoted, highest first, until N are kept. Promoted results are marked `backfilled: true` in JSON and `Kept (below the 6 cut; backfilled ...)` in verbose terminal output, so a confident keep and a floor keep are distinguishable. Nothing at the off-topic level (under 3.3) is ever promoted, and the summary line says when the floor could not be reached.

## Modes

- **Per result** (default): one Jev request per result, 8 in flight. About 0.4s per search.
- **`--batch`**: every result in one request. Fewer tokens; large batches can exceed Jev's request size.
- **`--noul "question?"`**: a yes/no question per result instead of a score; `min_score` becomes the minimum P(yes), default 0.5. Example: `--noul "Is this a research paper or preprint?"`.
- **`--rubric "a,b,c,d"`**: your own levels, lowest to highest; still reported 0–10. The default cut sits 1.2 levels below the top.

## Output fields

`score` (0–10) always; `confidence` (how far Jev is from a coin flip) and `probabilities` per rubric level only with `--verbose`, which also prints dropped results with the reason.
