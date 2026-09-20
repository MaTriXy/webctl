# smart_search evals

End-to-end checks of search quality. Each case runs the real pipeline —
provider search → Jev qualification → threshold filter — and then asks Jev,
in **one batch request**, whether the surviving results collectively cover a
set of expected themes. A case passes when the kept-result count is within
bounds and every theme is covered.

These hit live APIs and cost money. They are not run by `go test`.

## Running

You need a configured search provider key and a Jev key (`smart_search setup`).

```bash
# Build once
go build -o smart_search ./cmd/smart_search

# Run every embedded case with your default provider
./smart_search eval

# Pick cases, a provider, and force Jev batch mode for qualification
./smart_search eval --provider exa --batch mech-interp attention-paper

# Show kept URLs and token usage per case
./smart_search eval --verbose

# Machine-readable output (array of reports + summary)
./smart_search eval --json | jq '.summary'

# Run cases from a directory instead of the embedded set
./smart_search eval --cases ./my-cases
```

The command exits non-zero if any case fails or errors, so it works in CI.

## Output

```
=== smart_search eval: 5 cases, provider exa ===

✓ PASS  attention-paper                10 → 4   kept   themes 2/2   confidence 0.91   1.8s
    ✓ the original 2017 Transformer paper by Vaswani et al.  P(yes)=0.97
    ✓ self-attention or multi-head attention mechanism      P(yes)=0.94
✗ FAIL  mech-interp                    20 → 2   kept   themes 1/2   confidence 0.62   3.1s
    ✓ sparse autoencoders or dictionary learning ...        P(yes)=0.88
    ✗ transformer circuits or attention head analysis       P(yes)=0.31
    ✗ too few results: 2 kept, need ≥ 3

4/5 passed, 1 failed
```

`confidence` is the mean of Jev's confidence across the theme judgments
(|P(yes) − 0.5| × 2). For cases with no themes it is the mean confidence of
the relevance scores of kept results.

## Writing a case

Cases are YAML files in `evals/cases/` (embedded into the binary) or any
directory passed with `--cases`.

```yaml
name: mech-interp                 # defaults to the file name
query: "latest advances in mechanistic interpretability 2025"
provider: exa                     # optional; --provider overrides
num: 20                           # results to request (default 10)
min_score: 1.5                    # relevance cutoff (default 1.0)
rubric: [irrelevant, related, relevant, perfect]   # optional custom scale
# noul: "Is this a research paper?"  # yes/no mode instead of scoring;
                                    # min_score then means P(yes), default 0.5
batch: true                       # force Jev batch qualification
expected_themes:                  # every theme must be covered to pass
  - sparse autoencoders or dictionary learning
  - transformer circuits
min_results: 3                    # bounds on kept results (0 = unbounded)
max_results: 15
coverage_threshold: 0.5           # P(yes) needed to count a theme as covered
```

Tips:

- Phrase themes as things a result would *substantively address*, not
  keywords. Jev judges the whole kept set, so a theme covered by one strong
  result passes.
- Use `max_results` to catch a threshold that is too loose and
  `min_results` to catch one that is too strict.
- Noul cases are good for testing qualification questions other than
  relevance (e.g. "is this a primary source?").
- Search results drift. Prefer themes that are stable over time, or pin the
  query with a year.
