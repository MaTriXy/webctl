// Command webctl is a web search CLI that qualifies results with Jev.
package main

import (
	"os"

	"github.com/dorkitude/webctl/cmd/webctl/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
