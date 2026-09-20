---
name: chunk-relevance
description: Is one chunk of a scraped page worth keeping for the user's query? Batched, one question per chunk.
type: noul
model: jev-latest
batch: true
---

The state contains the user's search query and the text of a web page split
into chunks, each with an id. Would the chunk with id "{{.ID}}" be worth
quoting to someone researching the query?

Answer yes only if the chunk itself carries specific substance about the
query: concrete facts, numbers, recommendations with reasons, code, steps,
comparisons, or first-hand experience. Answer no for navigation and site
boilerplate, cookie or subscription notices, advertisements, author bios,
lists of links or related articles, generic background the reader already
knows, restated context without new information, and passages about a
different topic. When in doubt, answer no: the reader can open the page.
Judge only this one chunk.

The goal, when given, is what the user actually needs; keep a chunk if it
helps toward the goal.

Query: {{.Query}}
{{- if .Goal}}
Goal: {{.Goal}}
{{- end}}

Chunk {{.ID}}:
{{.Chunk}}
