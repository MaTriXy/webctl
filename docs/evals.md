# Evals

`multi_search_web eval` runs the cases in `evals/cases/` through the real pipeline. Each run is saved as one JSON file under `~/multi_search_web/evals/` (test output, kept out of the repository). The full method and the most recent results are in `docs/EVAL_REPORT.md`; the case format is in `evals/README.md`.

## Stages

- `nofilter`: the raw fused top-k as it would reach a context window.
- `filter`: Jev scoring, duplicate folding, threshold.
- `scrape` (cases marked `scrape: true`): fetch kept pages, keep Jev-approved chunks, re-judge coverage, and check whether dropped chunks still covered anything.

Each stage records results, characters, hand-labelled junk-domain hits, audit-flagged pages, folded duplicates, expected-domain hits, theme coverage, timing, and Jev tokens. A case passes when its filter stage passes.

## Running

```
multi_search_web eval                                   # all cases, all stages, 2 in parallel
multi_search_web eval -p searxng --verbose reddit-quiet-switches
multi_search_web eval --modes filter --json
multi_search_web eval --reuse-searches latest           # judge the last run's provider results again
multi_search_web eval --reuse-searches latest:0.0.016 --notes "prompt v5"
multi_search_web eval report                            # tables: latest run of every version
multi_search_web eval report --run latest --compare     # per-case raw vs. filter table
multi_search_web eval report --version 0.0.016 --cases
```

The suite needs a Jev key. Search results drift from hour to hour, so to compare two versions of the filter on identical inputs, run once, then run again with `--reuse-searches <that run>`: the second run skips the providers and judges the saved results.

## Run files

`<version>-<UTC time>.json`, e.g. `0.0.016-20260920T143000Z.json`, holding the settings, the tally, and every report with its stages, judged scores, and raw provider results. `--runs-dir` changes the directory for both `eval` and `eval report`; `--no-save` skips writing.
