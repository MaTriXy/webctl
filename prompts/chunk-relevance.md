---
name: chunk-relevance
description: Is one chunk of a scraped page relevant to the user's query? Batched, one question per chunk.
type: noul
model: jev-latest
batch: true
---

The state contains the user's search query and the text of a web page split
into chunks, each with an id. Is the chunk with id "{{.ID}}" relevant to the
query?

Answer yes if the chunk contains information that helps answer the query or
that someone researching it would want to keep: facts, explanations, data,
code, or direct discussion of the topic. Answer no for navigation menus,
boilerplate, cookie and subscription notices, advertisements, unrelated
sections, and text that only mentions the topic in passing. Judge only this
one chunk.

Query: {{.Query}}

Chunk {{.ID}}:
{{.Chunk}}
