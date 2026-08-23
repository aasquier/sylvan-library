// Command mtglab is the Go `mtglab` -- the binary that, at the end of the
// migration (docs/go-migration/PLAN.md), carries the whole runbook surface
// under the same name the Python CLI has always had. In Phase 2 it has one
// command: `ui`, the front door.
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
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mtglab:", err)
		os.Exit(1)
	}
}
