---
name: relevance-score
description: Score a single search result's relevance and source quality for the user's query
type: score
model: jev-latest
criteria:
  - "Off-topic: about a different subject, or a different meaning of the query's words"
  - "Low value: on-topic but generic or derivative — an SEO/affiliate/content-farm rewrite, a product listing or aggregator, or a page that only mentions the topic"
  - "Useful: substantive treatment from a credible source, such as documentation, a reputable publication, a practitioner's write-up, or a Q&A with real answers"
  - "Best available: a primary source, official docs, original research, or first-hand practitioner experience — the page an expert would open first for this query"
---

The user is an expert researching the query below and wants only high-signal
sources in their context window. Score this search result.

Judge two things together: whether the page is about what the user asked, and
whether it is a source worth reading. Read the query for intent — "real user
experiences", "what the studies say", or a term with several meanings tells
you which pages count. Use the URL: community threads (Reddit, Hacker News,
forums), official documentation, source repositories, reputable publications,
and authors with first-hand experience outrank anonymous blogs, "best X in
2026" listicles, affiliate review sites, and content farms that restate common
knowledge. A page that merely matches the query's keywords is not enough.

Query: {{.Query}}

Result:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
