# Documentation index

Full reference. `--help` is deliberately short; these pages hold the detail. One requirement: a Jev key. Everything else is optional.

| topic | what it covers |
|---|---|
| `search` | the search pipeline, every flag, output formats, exit codes |
| `providers` | the provider chain, `sources`, keyless vs. keyed, per-provider limits and cost |
| `cooldowns` | rate-limit backoff: the ladder, the probe, the state file, how to tune or clear |
| `filtering` | Jev scoring: the rubric, the threshold, `--noul`, custom rubrics, batch mode |
| `scraping` | `--scrape` and `--filter-chunks`, Reddit handling, fetch fallbacks |
| `dedupe` | the two duplicate passes and the MinHash/Jev design |
| `config` | every setting, precedence, environment names, the `config` and `keys` commands, file locations |
| `evals` | running and reading the eval suite, the results database |
| `searxng` | running a local SearXNG so searches never hit a quota |

Read one with `multi_search_web docs <topic>`, all of them with `multi_search_web docs all`.
