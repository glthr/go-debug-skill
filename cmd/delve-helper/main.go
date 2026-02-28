// delve-helper is a CLI that connects to a headless Delve instance and exposes
// subcommands so an agent can dynamically add breakpoints, step, and inspect state.
package main

import (
	"fmt"
	"os"

	"github.com/glthr/go-debug-skill/internal/delvehelper"
)

func main() {
	if err := delvehelper.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "delve-helper: %v\n", err)
		os.Exit(1)
	}
}
