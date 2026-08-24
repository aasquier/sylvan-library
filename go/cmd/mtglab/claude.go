package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// claudeCommand is `mtglab claude`: the pipe's own shell door. One
// subcommand, `check` — the runbook's answer to "is the key live", run after
// a rotation or in six weeks when something 401s and the question is whether
// the integration broke or the key simply lapsed (docs/HOSTING.md).
func claudeCommand(pipe claude.Endpoint) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "The Claude pipe",
	}
	cmd.AddCommand(claudeCheckCommand(pipe))
	return cmd
}

func claudeCheckCommand(pipe claude.Endpoint) *cobra.Command {
	var withTools bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "One real call, so \"is the pipe open\" is a command rather than a guess",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			report := claude.Check(cmd.Context(), pipe, "")
			fmt.Fprintf(out, "  model     %s\n", report.Model)
			if !report.OK {
				fmt.Fprintln(out, "  status    unavailable")
				fmt.Fprintf(out, "  reason    %s\n", report.Error)
				return errUnavailable
			}
			fmt.Fprintf(out, "  served by %s\n", report.ServedBy)
			fmt.Fprintf(out, "  reply     %s\n", wire.Quote(report.Text))
			fmt.Fprintf(out, "  tokens    %d in / %d out\n",
				report.InputTokens, report.OutputTokens)

			if withTools {
				names := append([]string(nil), tools.Names...)
				sort.Strings(names)
				fmt.Fprintf(out, "\n  %d tools, all read-only:\n", len(names))
				for _, name := range names {
					fmt.Fprintf(out, "    %s\n", name)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&withTools, "tools", false, "list the read-only tool roster")
	return cmd
}

// errUnavailable makes `claude check` exit 1 without repeating the reason
// the report already printed -- the verdict is the code, not a second
// sentence.
var errUnavailable = fmt.Errorf("the pipe is not open")
