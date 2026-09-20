---
name: relevance-batch
description: Score one result out of a batch shared in a single Jev request
type: score
model: jev-latest
batch: true
criteria:
  - "Off-topic: about a different subject, or a different meaning of the query's words"
  - "Low value: on-topic but generic or derivative — an SEO/affiliate/content-farm rewrite, a product listing or aggregator, or a page that only mentions the topic"
  - "Useful: substantive treatment from a credible source, such as documentation, a reputable publication, a practitioner's write-up, or a Q&A with real answers"
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
page that merely matches the query's keywords is not enough.

Community threads (Reddit, Hacker News, Stack Exchange, specialist forums)
are a special case. When the query asks for opinions, experiences,
recommendations, comparisons, or what people actually think, a thread on
that exact question is a best-available source (3), even if the snippet only
shows the opening post: the value is in the replies. On other queries, judge
a thread by how directly it addresses the question.

Query: {{.Query}}

Result {{.ID}}:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
