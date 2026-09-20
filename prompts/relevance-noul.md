---
name: relevance-noul
description: Ask a yes/no (noul) question about a single search result
type: noul
model: jev-latest
---

Answer the following yes/no question about the search result below, in the
context of the user's query. Base your answer only on the title, URL, and
snippet shown.

Question: {{.Question}}

Query: {{.Query}}
{{- if .Goal}}
Goal: {{.Goal}}
{{- end}}

Result:
  Title: {{.Title}}
  URL: {{.URL}}
  Snippet: {{.Snippet}}
