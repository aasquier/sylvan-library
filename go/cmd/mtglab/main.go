// Command mtglab is the Go `mtglab` -- the binary that carries the deployed
// runbook surface under the name the CLI has always had (docs/go-migration/
// PLAN.md, Phase 8): `ui` serves the app, `forge-shim` is the worker's door,
// and `users`, `decks`, `sim`, `data` and `claude check` are what a
// `fly ssh console` reaches for. The Python remnant keeps the dev bench --
// animist, cardmotion, bench, mutate -- under its own name.
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
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mtglab:", err)
		os.Exit(1)
	}
}
