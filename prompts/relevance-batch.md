---
name: relevance-batch
description: Score one result out of a batch shared in a single Jev request
type: score
model: jev-latest
batch: true
criteria:
  - "Off-topic: about a different subject, or a different meaning of the query's words"
  - "Low value: an SEO, affiliate, or content-farm page, a product listing, aggregator, or vendor pitch, a thin listicle, or a page that only mentions the topic"
  - "Useful: substantive, accurate treatment from an established source — a reference work or encyclopedia, an established publication or institution, a vendor's documentation, a practitioner's write-up, or a Q&A with real answers"
  - "Best available: a primary source, official docs, original research, or first-hand practitioner experience — the page an expert would open first for this query"
---

The state contains the user's query and a list of search results, each with
an id. The user is an expert researching the query and wants only high-signal
sources in their context window. Score the result with id "{{.ID}}" and judge
only that one result.

Judge two things together: whether the page is about what the user asked, and
whether it is a source worth reading. Read the query for intent — "real user
experiences", "what the studies say", or a term with several meanings tells
you which pages count. Use the URL: official documentation, source
repositories, original research, reputable publications, and authors with
first-hand experience outrank anonymous blogs, "best X in 2026" listicles,
affiliate review sites, and content farms that restate common knowledge. A
page that merely matches the query's keywords is not enough. Do not mark an
established source down for being general: an encyclopedia entry, a museum
or agency page, or a major publication's explainer that answers the query
accurately is Useful, not Low value. Reserve Low value for pages whose
reason to exist is ranking, selling, or padding.

Community threads (Reddit, Hacker News, Stack Exchange, specialist forums)
are a special case. When the query asks for opinions, experiences,
recommendations, comparisons, or what people actually think, a thread on
that exact question is a best-available source (3), even if the snippet only
shows the opening post: the value is in the replies. On other queries, judge
a thread by how directly it addresses the question.

The query is what was sent to the search engine; the goal, when given, is what
the user actually needs. Judge whether opening this page would move the user
toward the goal, not whether the snippet already contains the answer: a
thread on the exact question, the official docs, or a box score page counts
even when the excerpt shows only its opening lines.

Query: {{.Query}}
{{- if .Goal}}
Goal: {{.Goal}}
{{- end}}

Result {{.ID}}:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
