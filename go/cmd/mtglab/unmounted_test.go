package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// What every command does on a machine whose volume did not mount.
//
// This is the deployed fault, not a contrived one: `MTGLAB_DATA_DIR` names a
// path on a volume, and a volume that failed to attach leaves the process
// pointed at somewhere it cannot read and cannot create. Every `users`
// subcommand opens `app.db` through `connectUsers` before it does anything
// else; every `decks log`, `sim cache` and `sim matches` reaches for the same
// file. Each of those is an `if err != nil` on the first line of the command,
// and until this sweep not one of them had been driven.
//
// **Two things are asked, and the second is the one that matters.** A command
// must not report success — a `users list` that printed an empty roster over
// an unmountable volume says "you have no accounts", which is a different
// sentence from "I cannot read your accounts" and sends somebody to create
// them again. And a refusal must *say something*: an empty error renders as
// `mtglab: ` and tells the operator nothing.
//
// The commands are discovered from [newRoot] rather than listed here, so a
// subcommand added tomorrow is swept tomorrow. A hand-kept list loses the one
// added last, which is the one nobody has driven.

// unmounted is a deployment whose data directory is under a path that does not
// exist and cannot be created — `/nonexistent` is not writable by anybody who
// is not root, which is why the sweep skips when it is run as one.
func unmounted(t *testing.T) deployment {
	t.Helper()
	root := filepath.Join("/nonexistent", "never-mounted")
	return deployment{Config: config.Config{
		DataDir:   root,
		DecksDir:  filepath.Join(root, "decks"),
		BaseURL:   config.DefaultBaseURL,
		EmailFrom: config.DefaultEmailFrom,
	}}
}

// leafCommands walks the tree and returns the argv of every command that
// actually runs something, with a placeholder for each argument it requires.
func leafCommands(t *testing.T) [][]string {
	t.Helper()
	var out [][]string
	var walk func(prefix []string, c *cobra.Command)
	walk = func(prefix []string, c *cobra.Command) {
		here := append(append([]string{}, prefix...), c.Name())
		if len(c.Commands()) > 0 {
			for _, sub := range c.Commands() {
				walk(here, sub)
			}
			return
		}
		if c.RunE == nil && c.Run == nil {
			return
		}
		out = append(out, append(here, argsFor(c.Use)...))
	}
	for _, c := range newRoot(config.Config{}, tier3.Settings{}, claude.Endpoint{}).Commands() {
		walk(nil, c)
	}
	return out
}

// argsFor reads a command's `Use` line and supplies one plausible value per
// required argument. Cobra's own `Args` validator would otherwise refuse
// before the command body ran, and the body is what this sweep is about.
func argsFor(use string) []string {
	var args []string
	for _, field := range strings.Fields(use)[1:] {
		if !strings.HasPrefix(field, "<") {
			continue // an optional [arg] or a flag; leave it out
		}
		switch {
		case strings.Contains(field, "email"):
			args = append(args, "somebody@example.com")
		case strings.Contains(field, "slug"), strings.Contains(field, "deck"):
			args = append(args, "gyome")
		case strings.Contains(field, "path"), strings.Contains(field, "dest"):
			args = append(args, filepath.Join("/nonexistent", "backup"))
		default:
			args = append(args, "somebody")
		}
		if strings.HasSuffix(field, "...") {
			break
		}
	}
	return args
}

// Every command on a volume that did not mount.
func TestNoCommandReportsSuccessOnAVolumeThatDidNotMount(t *testing.T) {
	t.Parallel()
	d := unmounted(t)

	swept := 0
	for _, argv := range leafCommands(t) {
		switch argv[0] {
		case "probe", "ui", "forge-shim":
			// A listener, a health GET and a server: none of them read the
			// volume in a way this sweep can drive without binding a port.
			continue
		case "data":
			if len(argv) > 1 && argv[1] == "refresh" {
				continue // it downloads from Scryfall before it touches disk
			}
		case "claude":
			continue // no volume involved; the keyless report is its own test
		case "sim":
			if len(argv) > 1 && (argv[1] == "cache" || argv[1] == "matches") {
				// These two read and must never mint (`sim forge`'s recording
				// write is the deliberate exception), so an absent `app.db` is
				// an empty one by design. On an unmountable volume that rule
				// produces a green "0 rows", which is recorded below rather
				// than forced to refuse here -- changing it is a decision.
				continue
			}
		}
		swept++
		out, err := d.run(t, argv...)
		if err == nil {
			t.Errorf("`mtglab %s` succeeded on a volume that did not mount, "+
				"printing:\n%s", strings.Join(argv, " "), out)
			continue
		}
		if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("`mtglab %s` refused with an empty error -- that renders "+
				"as `mtglab: ` and says nothing", strings.Join(argv, " "))
		}
	}
	if swept < 15 {
		t.Errorf("only %d commands were swept -- the walker has stopped "+
			"matching the tree", swept)
	}
}

// The same volume, with the library rather than the database in question: a
// deck read must refuse rather than report an empty shelf.
func TestTheShelfIsNotReportedEmptyOnAVolumeThatDidNotMount(t *testing.T) {
	t.Parallel()
	d := unmounted(t)

	out, err := d.run(t, "decks", "list")
	if err == nil && strings.TrimSpace(out) == "" {
		t.Fatal("an unmountable library listed as an empty one -- that reads " +
			"as 'your decks are gone'")
	}
}

// **A reader on an unmountable volume reports emptiness, not a fault.**
//
// Recorded rather than approved of, the same way
// `TestASnapshotOnAFreshMachineMintsAPoolAndReportsZero` records `data
// snapshot`'s. The rule producing it is a good one: a read must never acquire
// a database, so an absent `app.db` is an empty history rather than an error,
// which is what a laptop before its first write wants.
//
// The operational risk is worth naming where somebody will read it. On an
// instance whose volume did not mount, `MTGLAB_DATA_DIR` points at a path with
// nothing on it, and these two answer "0 rows" and "no matches recorded yet"
// with a green exit -- while the real ledger sits untouched on the volume that
// is not there. "You have no matches" and "I cannot read your matches" are
// different sentences, and only one of them is true.
//
// Whether a reader should distinguish an absent directory from an absent file
// is Aaron's call rather than a patch. This pins today's behaviour and fails
// the day it changes.
func TestAReaderOnAnUnmountableVolumeReportsEmptinessRatherThanAFault(t *testing.T) {
	t.Parallel()
	d := unmounted(t)

	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"sim", "cache"}, "rows:"},
		{[]string{"sim", "matches"}, "no matches recorded yet"},
	} {
		out, err := d.run(t, tc.argv...)
		if err != nil {
			t.Errorf("`mtglab %s` now refuses (%v) -- if that is deliberate, "+
				"this recording is out of date and the sweep above should "+
				"stop skipping it", strings.Join(tc.argv, " "), err)
			continue
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("`mtglab %s` printed:\n%s\nwant something containing %q",
				strings.Join(tc.argv, " "), out, tc.want)
		}
		// The half worth knowing: nothing in the output says the volume was
		// empty rather than the ledger.
		if strings.Contains(strings.ToLower(out), "could not") ||
			strings.Contains(strings.ToLower(out), "unreadable") {
			t.Errorf("`mtglab %s` now names the fault -- the recording is out "+
				"of date:\n%s", strings.Join(tc.argv, " "), out)
		}
	}
}
