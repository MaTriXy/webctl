# webctl evals

End-to-end checks of search quality. Each case runs one real provider search
(the auto chain by default) and then up to three stages on the same results:

| stage | what it measures |
|---|---|
| `nofilter` | the raw top-k as it would reach a context window without Jev |
| `filter` | Jev qualification and the score threshold (the default pipeline) |
| `scrape` | for cases marked `scrape: true`: fetch the kept pages, keep only Jev-approved chunks |

For every stage the runner records what would be delivered (result count,
characters, hits on hand-labelled junk domains, hits on the domains where the
best answers live, and pages the source-quality audit flagged as SEO,
affiliate, or content-farm) and asks Jev, in one batch request, whether that
delivery covers the case's expected themes. A case passes when its `filter`
stage passes.

These hit live APIs and cost money. They are not run by `go test`.

## Running

You need a Jev key (`webctl setup`). Search keys are optional: the
keyless Exa and Parallel endpoints are used without them.

```bash
go build -o webctl ./cmd/webctl

./webctl eval                       # every case, all stages, 2 in parallel
./webctl eval --modes filter        # one stage only
./webctl eval -p parallel           # pin one provider
./webctl eval --verbose reddit-espresso-grinder github-uv
./webctl eval --json | jq '.summary'
./webctl eval --cases ./my-cases
```

The command exits non-zero if any case fails or errors.

Search results drift between runs. To compare two filter versions on the
same inputs, run once and then run with `--reuse-searches <run id>`; the
second run judges the first run's provider results without searching.

## Run files

Every run is saved as one JSON file under `~/webctl/evals/`
(`--runs-dir` to change it, `--no-save` to skip). The file holds the settings,
the tally, and every report with its stages, judged scores, and raw provider
results. Runs are test output and stay out of the repository; the write-up in
`docs/EVAL_REPORT.md` is what gets committed.

```bash
./webctl eval report                    # latest run of every version
./webctl eval report --run latest --compare
./webctl eval --reuse-searches latest   # re-judge the same provider results
```

## Writing a case

Cases are YAML files in `evals/cases/` (embedded into the binary) or any
directory passed with `--cases`.

```yaml
name: reddit-espresso-grinder     # defaults to the file name
query: "espresso grinder under $300 that people actually recommend after owning it"
num: 15                           # results to request (default 10)
min_score: 8                      # relevance cutoff out of 10 (default: the product default, 6)
tags: [reddit, opinion, noise]    # free labels for reporting
scrape: true                      # also run the scrape-to-chunks stage
expected_domains: [reddit.com]    # where the best answers live; dropping every
                                  # raw hit on these fails the filter stage
junk_domains: [some-farm.example] # hand-labelled noise; keeping one fails the stage
expected_themes:                  # every theme must be covered to pass
  - specific grinder models in the sub-$300 range
  - owner experience rather than a spec sheet
min_results: 2                    # bounds on kept results (0 = unbounded)
max_results: 12
# provider: exa                   # pin a provider for this case
# rubric: [irrelevant, related, relevant, perfect]   # custom scale
# noul: "Is this a research paper?"  # yes/no mode; min_score is then P(yes)
# batch: true                     # force Jev batch qualification
# coverage_threshold: 0.5         # P(yes) needed to count a theme as covered
```

Tips:

- Phrase themes as things a result would *substantively address*, not
  keywords. Jev judges the whole delivered set, so a theme covered by one
  strong result passes.
- Use `max_results` to catch a threshold that is too loose and
  `min_results` to catch one that is too strict.
- Label `junk_domains` from what you actually see in a `nofilter` run; the
  audit's `flagged` count covers the rest.
- Search results drift. Prefer themes that are stable over time.
