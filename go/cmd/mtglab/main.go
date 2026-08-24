// Command mtglab is the binary that carries the whole deployed surface and
// its runbook: `ui` serves the app, `forge-shim` is the worker's door, and
// `users`, `decks`, `sim`, `data` and `claude check` are what a
// `fly ssh console` reaches for. The dev bench -- `animist` and
// `cardmotion`, and nothing else -- lives in `tools/` under its own name and
// never ships. (`bench` and `mutate` were Python-era subcommands of this
// binary and did not cross: mutation sampling is `gremlins`, installed on
// demand, and there is no benchmark harness beyond `go test -bench`.)
//
// cobra throughout, by Aaron's explicit requirement (ADR 38 decision 5).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newRoot is the whole command tree, assembled. Separate from [main] so a
// test can ask the binary what verbs it actually has rather than restate a
// list beside it -- which is how prose in this repository came to name
// `mtglab animist`, a family that moved to `tools/` and left the sentences
// behind.
func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "mtglab",
		Short:         "The sylvan library: a Commander toolkit",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(uiCommand())
	root.AddCommand(shimCommand())
	root.AddCommand(dataCommand())
	root.AddCommand(usersCommand())
	root.AddCommand(decksCommand())
	root.AddCommand(simCommand())
	root.AddCommand(cardsCommand())
	root.AddCommand(claudeCommand())
	root.AddCommand(probeCommand())
	return root
}

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mtglab:", err)
		os.Exit(1)
	}
}
