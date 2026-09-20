# Scraping

`--scrape` fetches each kept result's page and reduces it to text. `--filter-chunks` keeps only the parts worth quoting.

Agents should use `--scrape --filter-chunks` instead of reading pages themselves most of the time: webctl does the fetch, and only the chunks Jev judges relevant to the query and goal are returned, so a long thread, PDF, or article costs a fraction of the tokens. Read a page directly only when you need it whole.

## Fetching

Pages are fetched with a 10s timeout, 4 at a time, and converted from HTML to text (scripts, styles, navigation elements dropped; block structure kept as line breaks). Plain text and JSON pass through. A page that cannot be fetched (403 bot wall, timeout) falls back to the provider's own excerpt, marked in the output.

## PDFs

A result that is a PDF (by content type, or by the `%PDF-` signature when a host serves it as octet-stream) has its text layer extracted with a pure-Go parser; a scanned PDF with no text layer is reported as a fetch error and falls back to the excerpt. Because a paper's text runs to tens of thousands of characters, a PDF is always chunk-filtered through Jev when a Jev key is present, even without `--filter-chunks`; only `--no-filter` leaves it whole. The terminal header reads `PDF text (k/n chunks kept, c chars)` and JSON carries `"pdf": true`.

Reddit: `www.reddit.com` answers with a JavaScript challenge page (HTTP 200, no content). The challenge is solved, the resulting cookies are kept for the run, and the comment tree, which ships inside a `<template>` element, is read. Threads come back with post and comments.

## Chunk filtering

Each page is split into chunks of about 2,000 characters on paragraph boundaries, and Jev is asked whether each chunk is worth quoting for the query; chunks with P(yes) ≥ 0.5 are kept and rejoined. Typical outcome: 20 to 30 percent of text removed (navigation, footers, tangents), themes fully retained. `--max-chars` caps text per page before chunking (default 50,000).

Chunks are packed into batches that stay under an estimated-token budget and sent in parallel, so a long page does not become one request too large for Jev to accept. The budget accounts for each chunk being sent twice (once in the request state, once in its rendered question) plus the prompt boilerplate; a batch is capped at 24 chunks regardless. A batch Jev still rejects for size is halved and retried down to a single chunk.

Failure is contained. If some batches fail, the chunks they covered are kept unfiltered rather than dropped — losing a verdict must never silently delete page content — and the run reports `k/n chunks kept, u unjudged` with `chunks_unjudged` in JSON. Only when every batch fails does the page fall back to its whole unfiltered text, reported as before.

## Output

Terminal: a `--- content (k/n chunks kept, c chars) ---` header per result. JSON: `content`, `scrape_error`, `chunks_total`, `chunks_kept`, `filter_error`.
