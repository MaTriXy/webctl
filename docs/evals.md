# Evals

`webctl eval` runs the cases in `evals/cases/` through the real pipeline. Each run is saved as one JSON file under `~/webctl/evals/` (test output, kept out of the repository). The full method and the most recent results are in `docs/EVAL_REPORT.md`; the case format is in `evals/README.md`.

## Stages

- `nofilter`: the raw fused top-k as it would reach a context window.
- `filter`: Jev scoring, duplicate folding, threshold.
- `scrape` (cases marked `scrape: true`): fetch kept pages, keep Jev-approved chunks, re-judge coverage, and check whether dropped chunks still covered anything.

Each stage records results, characters, hand-labelled junk-domain hits, audit-flagged pages, folded duplicates, expected-domain hits, theme coverage, timing, and Jev tokens. A case passes when its filter stage passes.

## Running

```
webctl eval                                   # all cases, all stages, 2 in parallel
webctl eval -p searxng --verbose reddit-quiet-switches
webctl eval --modes filter --json
webctl eval --reuse-searches latest           # judge the last run's provider results again
webctl eval --reuse-searches latest:0.0.016 --notes "prompt v5"
webctl eval report                            # tables: latest run of every version
webctl eval report --run latest --compare     # per-case raw vs. filter table
webctl eval report --version 0.0.016 --cases
```

The suite needs a Jev key. Search results drift from hour to hour, so to compare two versions of the filter on identical inputs, run once, then run again with `--reuse-searches <that run>`: the second run skips the providers and judges the saved results.

## Run files

`<version>-<UTC time>.json`, e.g. `0.0.016-20260920T143000Z.json`, holding the settings, the tally, and every report with its stages, judged scores, and raw provider results. `--runs-dir` changes the directory for both `eval` and `eval report`; `--no-save` skips writing.
