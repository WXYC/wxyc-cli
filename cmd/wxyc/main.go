// Command wxyc is a read-only-by-default CLI for the WXYC backend API,
// designed to be driven by both humans and agents.
package main

import (
	"os"

	"github.com/WXYC/wxyc-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
