package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The command families -- `decks`, `users`, `sim`, `data`, `cards`, `claude`
// -- hold subcommands and run nothing of their own, and cobra's default for a
// command shaped like that is to answer *anything* it does not recognise with
// the family's help text and a green exit. `mtglab decks frobnicate` printed
// the list of deck verbs and reported success; so did every other family,
// while the root alone refused, through a legacy rule cobra applies only
// there. The case that matters is a misspelt verb in a runbook line or a cron
// entry: the operator reads success, and the thing never ran.
//
// [family] is the fix. These tests are derived from the tree rather than from
// a list of family names, so a seventh family built by hand at its own site --
// a bare `&cobra.Command{Use, Short}` -- is caught the day it lands rather
// than the day somebody mistypes under it.

// families walks the tree and returns the argv path of every command that
// holds subcommands, the root included as the empty path.
func families(t *testing.T) [][]string {
	t.Helper()
	var out [][]string
	var walk func(prefix []string, c *cobra.Command)
	walk = func(prefix []string, c *cobra.Command) {
		if len(c.Commands()) == 0 {
			return
		}
		out = append(out, prefix)
		for _, sub := range c.Commands() {
			walk(append(append([]string{}, prefix...), sub.Name()), sub)
		}
	}
	walk(nil, newRoot(config.Config{}, tier3.Settings{}, claude.Endpoint{}))
	return out
}

func TestAMistypedVerbIsRefusedUnderEveryFamily(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	fams := families(t)
	if len(fams) < 7 {
		t.Fatalf("walked only %d families -- the walker has stopped matching "+
			"the tree: %v", len(fams), fams)
	}
	for _, path := range fams {
		name := strings.TrimSpace("mtglab " + strings.Join(path, " "))
		argv := append(append([]string{}, path...), "frobnicate")
		out, err := d.run(t, argv...)
		if err == nil {
			t.Errorf("`%s frobnicate` reported success and printed:\n%s", name, out)
			continue
		}
		if !strings.Contains(err.Error(), `"frobnicate"`) {
			t.Errorf("`%s frobnicate` refused without naming the verb: %v", name, err)
		}
	}
}

// The other half of the bargain: a bare family still prints its help, so the
// refusal above was bought without losing the one thing a non-runnable
// command was good for.
func TestABareFamilyStillPrintsItsHelp(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	for _, path := range families(t) {
		if len(path) == 0 {
			continue // the root's bare help is cobra's own, not the helper's
		}
		name := "mtglab " + strings.Join(path, " ")
		out, err := d.run(t, path...)
		if err != nil {
			t.Errorf("`%s` refused: %v", name, err)
			continue
		}
		if !strings.Contains(out, "Available Commands:") {
			t.Errorf("`%s` ran but printed no help:\n%s", name, out)
		}
	}
}

// `RunE` throughout: an error returns to main.go's one [os.Exit] rather than
// ending the process from inside a command, which is what lets every command
// be driven in-process by a test and lets a deferred Close actually run on
// the way out (`decks.go` argues it where the recorder is built). Derived from
// the tree, so a command wired through `Run` fails here by name.
func TestNoCommandExitsFromInsideItsOwnBody(t *testing.T) {
	t.Parallel()
	walked := 0
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		walked++
		if c.Run != nil {
			t.Errorf("`%s` is wired through Run rather than RunE", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRoot(config.Config{}, tier3.Settings{}, claude.Endpoint{}))
	if walked < 30 {
		t.Errorf("walked only %d commands -- the walker has stopped matching "+
			"the tree", walked)
	}
}
