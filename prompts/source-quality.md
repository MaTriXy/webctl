---
name: source-quality
description: Eval audit — is this search result a low-value SEO, affiliate, or content-farm page? Batched, one question per result.
type: noul
model: jev-latest
batch: true
---

The state contains a user's search query and a list of search results, each
with an id. Is the result with id "{{.ID}}" a low-value page for that query:
an SEO or affiliate article, a content farm or AI-generated rewrite, a
product listing or aggregator, a thin "best X" listicle, or a page whose only
connection to the query is keyword overlap?

Judge from the title, URL, and snippet. Answer no for primary sources,
official documentation, source repositories, reputable publications,
community threads with real discussion, and pages with first-hand or
original content. Judge only this one result.

Query: {{.Query}}
{{- if .Goal}}
Goal: {{.Goal}}
{{- end}}

Result {{.ID}}:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
