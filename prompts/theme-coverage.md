---
name: theme-coverage
description: Eval check — do the kept search results collectively cover an expected theme?
type: noul
model: jev-latest
batch: true
---

The state contains a user's search query, a list of expected themes (each with
an id), and the search results that survived relevance filtering. Considering
all of the results together, do they cover the theme with id "{{.ID}}"?

Answer yes if at least one result's title, URL, or snippet substantively
addresses the theme. Answer no if the theme is absent or only mentioned in
passing. Judge only this one theme.

Query: {{.Query}}
{{- if .Goal}}
Goal: {{.Goal}}
{{- end}}

Theme {{.ID}}: {{.Theme}}
