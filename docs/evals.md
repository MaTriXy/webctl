# Evals

`multi_search_web eval` runs the cases in `evals/cases/` through the real pipeline and stores every stage in `evals/results.db`, keyed by the behavior version (`multi_search_web --version`). The full method and current results are in `docs/EVAL_REPORT.md`; the case format is in `evals/README.md`.

## Stages

- `nofilter`: the raw fused top-k as it would reach a context window.
- `filter`: Jev scoring, duplicate folding, threshold.
- `scrape` (cases marked `scrape: true`): fetch kept pages, keep Jev-approved chunks, re-judge coverage, and check whether dropped chunks still covered anything.

Each stage records results, characters, hand-labelled junk-domain hits, audit-flagged pages, expected-domain hits, theme coverage, timing, and Jev tokens. A case passes when its filter stage passes.

## Running

```
multi_search_web eval                          # all cases, all stages, 2 in parallel
multi_search_web eval --fresh                  # ignore cached provider results (24h cache otherwise)
multi_search_web eval -p searxng --verbose reddit-quiet-switches
multi_search_web eval --modes filter --json
multi_search_web eval report --compare         # Markdown tables per version, latest run each
multi_search_web eval report --cases           # one row per case and stage
```

The suite needs a Jev key. Provider results are cached in the database for 24 hours so that prompt and threshold iterations judge identical inputs and spare the keyless tiers.
