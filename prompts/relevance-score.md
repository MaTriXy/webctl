---
name: relevance-score
description: Score a single search result's relevance to the user's query
type: score
model: jev-latest
criteria:
  - "Completely irrelevant — different topic entirely"
  - "Tangentially related — mentions keywords but doesn't address the query"
  - "Relevant — directly addresses the query with substantive content"
  - "Highly relevant — exactly what the user is looking for"
---

How relevant is this search result to the user's query?

Judge relevance from the user's perspective: would opening this page help them
with what they asked for? Weigh the title, URL, and snippet together. Penalize
pages that merely mention the query's keywords, content farms, and results
whose snippet is about a different subject than the query. Reward pages whose
substance directly answers or addresses the query.

Query: {{.Query}}

Result:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
