---
name: relevance-batch
description: Score one result out of a batch shared in a single Jev request
type: score
model: jev-latest
batch: true
criteria:
  - "Completely irrelevant — different topic entirely"
  - "Tangentially related — mentions keywords but doesn't address the query"
  - "Relevant — directly addresses the query with substantive content"
  - "Highly relevant — exactly what the user is looking for"
---

The state contains the user's query and a list of search results, each with
an id. How relevant is the result with id "{{.ID}}" to the user's query?

Judge only that one result. Would opening it help the user with what they
asked for? Penalize pages that merely mention the query's keywords, content
farms, and results about a different subject. Reward pages whose substance
directly answers or addresses the query.

Query: {{.Query}}

Result {{.ID}}:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
