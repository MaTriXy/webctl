// Package prompts embeds the OKF-style prompt templates shipped with smart_search.
//
// Each .md file has a YAML frontmatter block (name, description, type, model,
// criteria, ...) followed by a Go text/template body that becomes the
// question's instructions. See internal/prompts for parsing and rendering.
package prompts

import "embed"

// FS holds every prompt template in this directory.
//
//go:embed *.md
var FS embed.FS
