package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/prices"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// claudeCommand is `mtglab claude`: the pipe's own shell door. Two
// subcommands — `check`, the runbook's answer to "is the key live", run after
// a rotation or in six weeks when something 401s and the question is whether
// the integration broke or the key simply lapsed (docs/HOSTING.md); and
// `usage`, what it has all cost.
func claudeCommand(cfg config.Config, pipe claude.Endpoint) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "The Claude pipe",
	}
	cmd.AddCommand(claudeCheckCommand(pipe), claudeUsageCommand(cfg))
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

// claudeUsageCommand is `mtglab claude usage`: what Claude has cost on this
// box, per mode and per model, in tokens and in money.
//
// **It exists so the number can be read from anywhere.** The accounting is
// written on every way out of a conversation and the Admin panel rolls it up,
// but a panel is a signed-in browser surface and Claude never signs in to
// anything -- so while this verb was missing, the only spend figure a night
// run could quote was this laptop's, which is nearly idle, while the deployed
// instance is where the money actually goes.
// `fly ssh console -C "mtglab claude usage"` is the whole point. The Admin
// panel already sends people here twice, by name, for the model ids it will
// not render itself (commandment 10: a terminal is not a screen, the same
// carve-out `mtglab users list` gets for printing an address).
//
// Read-only, through `auth.Open`'s `mode=ro` handle: a command whose job is to
// report cannot be the command that writes to the accounts database. It also
// never runs the ladder -- unlike `mtglab users`, whose first invocation is
// what mints app.db on a bare laptop, this one has nothing to add to an
// instance that has never spent anything, and a reporting verb that could
// create a database by being asked a question is a reporting verb nobody
// should trust.
func claudeUsageCommand(cfg config.Config) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "What Claude has cost here: per mode, per model, in dollars",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if err := checkSince(since); err != nil {
				return err
			}
			path := cfg.AppDBPath()
			if _, err := os.Stat(path); err != nil {
				fmt.Fprintf(out, "  no ledger at %s\n", path)
				fmt.Fprintln(out, "  nothing on this box has asked Claude anything yet")
				return nil
			}
			db, err := auth.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			roll, err := ledger.RecorderFrom(db, nil).
				Window(cmd.Context(), since, prices.Today())
			if err != nil {
				return err
			}
			renderUsage(out, roll, since)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "",
		"only conversations from this date on, as 2026-09-01 (default: everything recorded)")
	return cmd
}

// checkSince refuses a `--since` the ledger would compare as nonsense.
//
// The comparison is textual against an ISO-8601 stamp, which is what makes it
// cheap -- and also what makes a typo silent: "1 Sep" sorts after every
// timestamp ever written and would answer "nothing recorded" with a straight
// face.
//
// **A plain date and nothing else**, which is a narrowing on purpose rather
// than a shortcut. A date sorts exactly where its own midnight does, so
// `--since 2026-09-01` takes in everything from that instant on. A full
// RFC3339 stamp does not behave: the ledger writes its offset as `+00:00` and
// RFC3339's own spelling of the same moment is `Z`, which sorts AFTER the dot
// of the microseconds -- so `2026-09-01T00:00:00Z` would silently drop every
// conversation from the first moments of that day. One accepted shape has no
// such corner.
func checkSince(since string) error {
	if since == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, since); err != nil {
		return refused("--since wants a date like 2026-09-01, not %s", quoted(since))
	}
	return nil
}

// renderUsage writes a roll-up as a page of text.
//
// Both axes, because they answer different questions -- which surface spent it
// and which Claude spent it -- and **dollars only against the models**, which
// is not a formatting choice: a mode's row is spread across every model that
// served it, so its price is a question with no answer, and a number in that
// column would look like arithmetic. The panel makes the same call for the
// same reason.
func renderUsage(out io.Writer, roll ledger.Rollup, since string) {
	span := "everything recorded"
	if since != "" {
		span = "from " + since
	}
	fmt.Fprintf(out, "  Claude spend on this box -- %s\n\n", span)
	if len(roll.ByMode) == 0 {
		fmt.Fprintln(out, "  nothing recorded in that window")
		return
	}

	const head = "  %-26s %6s %9s %11s %11s %13s"
	const row = "  %-26s %6d %9d %11d %11d %13d"
	fmt.Fprintf(out, head+"\n", "mode", "conv", "requests", "in", "out", "cached")
	for _, r := range roll.ByMode {
		fmt.Fprintf(out, row+"\n", r.Mode, r.Conversations, r.Requests,
			r.InputTokens, r.OutputTokens, r.CacheReadTokens)
	}

	fmt.Fprintf(out, "\n"+head+" %12s\n", "model", "conv", "requests", "in",
		"out", "cached", "est. USD")
	for _, r := range roll.ByModel {
		money := "unpriced"
		if cost, ok := roll.CostOf[r.Model]; ok && cost.Unpriced == 0 {
			money = dollars(cost.USD)
		}
		fmt.Fprintf(out, row+" %12s\n", r.Model, r.Conversations, r.Requests,
			r.InputTokens, r.OutputTokens, r.CacheReadTokens, money)
	}

	fmt.Fprintf(out, "\n  total     %s, at the rates in force when it was spent\n",
		dollars(roll.Cost.USD))
	fmt.Fprintf(out, "  rates     list prices read by a person on %s, never an invoice\n",
		prices.Checked)
	fmt.Fprintln(out, "  floor     a token count is a floor on the bill -- cache writes")
	fmt.Fprintln(out, "            bill at 1.25x input and are recorded nowhere")
	if roll.Cost.Unpriced > 0 {
		// The ids, spelled out. This is the line the Admin panel sends people
		// here for: it counts the unpriced conversations and deliberately will
		// not name the model that answered them, because a model id is not
		// something the site renders.
		fmt.Fprintf(out, "  unpriced  %d %s on a model with no rate, left out "+
			"of the total:\n", roll.Cost.Unpriced,
			plural(roll.Cost.Unpriced, "conversation"))
		for _, model := range roll.Cost.UnpricedModels {
			fmt.Fprintf(out, "              %s\n", model)
		}
	}
}

// plural is the difference between "1 conversation" and "2 conversations",
// which is small and is the sort of thing a report gets wrong forever.
func plural(n int64, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// dollars renders money the way the panel serialises it -- half-to-even to
// four places -- so the two surfaces can be held to the same figure rather
// than to the same rounding of two different figures.
func dollars(usd float64) string {
	return "$" + strconv.FormatFloat(floats.RoundTo(usd, 4), 'f', 4, 64)
}
