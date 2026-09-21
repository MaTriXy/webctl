package benchmarks

import (
	"fmt"
	"strings"
)

// BuildPrompt is the task given to an agent. The question is identical
// across arms; only the tool instructions differ, and each arm is told
// exactly which tools it may use so the comparison is webctl versus the
// harness's own search, not one harness's habits versus another's.
func BuildPrompt(c Case, mode Mode) string {
	var b strings.Builder
	b.WriteString("Research task. Answer the question below using the web, then reply with a concise, specific answer ")
	b.WriteString("(facts, numbers, names, versions as appropriate) and list the source URLs you relied on. ")
	b.WriteString("Do not pad. Do not ask follow-up questions. If you cannot find something, say so.\n\n")
	switch mode {
	case ModeWebctl:
		b.WriteString(`Tools: you have the command-line tool webctl on PATH, and it is the ONLY way you may reach the web. Do not use any built-in web search or fetch tool, and do not run curl, wget, or any command other than webctl.

Usage:
  webctl search "<search-engine style query>" --goal "<what you actually need, in plain words>"
  webctl search "<query>" --goal "<goal>" --scrape --filter-chunks   # also returns the relevant parts of each page

Always pass --goal. The snippets are usually enough; add --scrape --filter-chunks whenever you would otherwise read a whole page, since it returns only the chunks that matter. Run at most 4 webctl commands.

`)
	case ModeWebctlLite:
		b.WriteString(`Tools: you have the command-line tool webctl on PATH, and it is the ONLY way you may reach the web. Do not use any built-in web search or fetch tool, and do not run curl, wget, or any command other than webctl.

Usage:
  webctl search "<search-engine style query>" --goal "<what you actually need, in plain words>"

Always pass --goal. Do not pass --scrape or --filter-chunks: work from the titles, URLs, and snippets webctl returns. Run at most 4 webctl commands.

`)
	case ModeWebctlSummarize:
		b.WriteString(`Tools: you have the command-line tool webctl on PATH, and it is the ONLY way you may reach the web. Do not use any built-in web search or fetch tool, and do not run curl, wget, or any command other than webctl.

Usage:
  webctl search "<search-engine style query>" --goal "<what you actually need, in plain words>"
  webctl search "<query>" --goal "<goal>" --scrape --filter-chunks --summarize   # also returns a short summary of each relevant page

Always pass --goal. The snippets are usually enough; add --scrape --filter-chunks --summarize whenever you would otherwise read a whole page, since it returns a short summary of what each page says about the goal. Run at most 4 webctl commands.

`)
	case ModeNative:
		b.WriteString(`Tools: use your built-in web search and web fetch tools, and nothing else. Do not run shell commands. Run at most 4 searches/fetches.

`)
	}
	fmt.Fprintf(&b, "Question: %s\n", strings.TrimSpace(c.Question))
	return b.String()
}
