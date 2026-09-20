// Command multi_search_web is a web search CLI that qualifies results with Jev.
package main

import (
	"os"

	"github.com/dorkitude/multi_search_web/cmd/multi_search_web/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
