# Duplicates

Several engines return the same page at different addresses, and the web mirrors, syndicates, and rewrites content. Two passes fold that without comparing every pair.

## Pass 1: exact (before Jev)

Results collapse when their normalized URL matches (lowercase host without `www.`, `m.`, `amp.`; no scheme, fragment, or `utm_*`/`fbclid`/`gclid`/`ref` parameters; no trailing slash) or when their normalized title matches (lowercase, punctuation removed, site suffix dropped, at least 15 characters). First occurrence wins. This costs nothing and spares Jev the copies.

## Pass 2: near-duplicates (after scoring)

1. Each result's text (excerpt, else snippet, with the title) is split into word 3-gram shingles. Results under 120 characters are skipped.
2. A MinHash sketch of 64 values is computed per result. Sketches are cut into 16 bands of 4; results sharing any band land in the same bucket. Pairs are proposed only from buckets, so the work is linear in the number of results.
3. A proposed pair must reach an estimated Jaccard similarity of 0.35 across the full sketch, and must not share a normalized URL.
4. The candidates (usually a handful per search) go to Jev in one batch request: is this pair the same content (mirror, abstract vs. PDF, syndicated copy, rewrite with nothing of its own)?
5. Confirmed pairs are unioned into groups. Each group keeps its best-scored member; the others are listed as `duplicates` in JSON and `Duplicate:` lines in the terminal, and their engine tags are merged into the keeper.

`--no-dedupe` skips pass 2. Without a Jev key (`--no-filter`) pass 2 does not run.
