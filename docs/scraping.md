# Scraping

`--scrape` fetches each kept result's page and reduces it to text. `--filter-chunks` keeps only the parts worth quoting.

## Fetching

Pages are fetched with a 10s timeout, 4 at a time, and converted from HTML to text (scripts, styles, navigation elements dropped; block structure kept as line breaks). Plain text and JSON pass through; PDFs are not parsed. A page that cannot be fetched (403 bot wall, PDF, timeout) falls back to the provider's own excerpt, marked in the output.

Reddit: `www.reddit.com` answers with a JavaScript challenge page (HTTP 200, no content). The challenge is solved, the resulting cookies are kept for the run, and the comment tree, which ships inside a `<template>` element, is read. Threads come back with post and comments.

## Chunk filtering

Each page is split into chunks of about 2,000 characters on paragraph boundaries. One Jev batch request per page asks whether each chunk is worth quoting for the query; chunks with P(yes) ≥ 0.5 are kept and rejoined. Typical outcome: 20 to 30 percent of text removed (navigation, footers, tangents), themes fully retained. `--max-chars` caps text per page before chunking (default 50,000).

## Output

Terminal: a `--- content (k/n chunks kept, c chars) ---` header per result. JSON: `content`, `scrape_error`, `chunks_total`, `chunks_kept`, `filter_error`.
