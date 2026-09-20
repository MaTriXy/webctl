// Command smart_search is a web search CLI that qualifies results with Jev.
package main

import (
	"os"

	"github.com/dorkitude/smart_search/cmd/smart_search/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
