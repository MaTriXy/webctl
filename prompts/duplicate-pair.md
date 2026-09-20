---
name: duplicate-pair
description: Do two search results cover the same content? Batched, one question per candidate pair.
type: noul
model: jev-latest
batch: true
---

The state contains the user's query, a list of search results with ids, and
candidate pairs of results that look alike. Do results "{{.ID}}" (listed in
the state as pair {{.ID}}) cover the same content?

Answer yes if they are the same document at two addresses (a mirror, an
abstract and its PDF, a syndicated or republished copy, an AMP or mobile
version), or if one is a rewrite of the other with no information of its
own. Answer no if they are different pages that merely share a topic,
phrasing, or boilerplate, or if either adds material the other lacks. Judge
only this one pair.

Query: {{.Query}}

Pair {{.ID}}: {{.Snippet}}
