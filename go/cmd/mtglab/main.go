// Command mtglab is the binary that carries the whole deployed surface and
// its runbook: `ui` serves the app, `forge-shim` is the worker's door, and
// `users`, `decks`, `sim`, `data` and `claude check` are what a
// `fly ssh console` reaches for. The dev bench -- animist, cardmotion,
// bench, mutate -- lives in `tools/` under its own name and never ships.
//
// cobra throughout, by Aaron's explicit requirement (ADR 38 decision 5).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
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
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mtglab:", err)
		os.Exit(1)
	}
}
