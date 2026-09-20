// Package docs embeds the reference documentation so `webctl docs`
// can print it without a checkout.
package docs

import "embed"

// FS holds every documentation page in this directory.
//
//go:embed *.md
var FS embed.FS
