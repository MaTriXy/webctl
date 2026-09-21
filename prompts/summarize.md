---
name: summarize
description: Summarize the relevant parts of one scraped page for the user's query and goal. Sent to a small general model, not Jev.
type: text
---
You are condensing one web page for someone who is researching a question. Below are the parts of the page that a relevance filter kept, in page order. Write what this page contributes to the goal, and nothing else.

Rules:
- Facts only: numbers, names, versions, dates, steps, quoted claims. Keep exact figures and units. Attribute opinions to who said them (an author, a commenter, the vendor).
- Say what the page does NOT answer if the goal asks for something the kept text lacks. Do not guess or fill gaps from memory.
- No preamble, no restating the question, no "the page discusses". Plain prose or short bullets. Under 200 words unless the goal needs a list of specifics.
- If the kept text is boilerplate or off topic, reply with exactly: NOTHING RELEVANT

Query: {{.Query}}
{{- if .Goal}}
Goal: {{.Goal}}
{{- end}}

Page: {{.Title}}
URL: {{.URL}}

Kept text:
{{.Chunk}}
