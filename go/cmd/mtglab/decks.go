package main

// `mtglab decks` -- the deck-facing quarter of the runbook surface (Phase 8:
// the binary carries the runbook). The CLI is the LOCAL user at a terminal,
// so it reads the file tier rooted at `settings().DecksDir` -- always the
// config-resolved paths, never a hard-wired directory -- and the activity
// log's file-tier rows (`owner_id IS NULL`), which on a deployed instance
// are the maintainer's own decks too.
//
// The table, the refusal sentences, and the exit codes are recorded
// contract, kept to the byte. Three conventions worth stating. A refusal
// returned as an error is printed by the root as `mtglab: <sentence>`, so
// no command prints its own prefix. A fault of the environment (an
// unreadable pool, an unparseable file) comes back as an error sentence,
// never a stack. And `decks log` never CREATES `app.db`: the ladder belongs
// to the serving command alone (`auth.Migrate`, Phase 8) -- an absent
// `app.db` is read as an empty history, the same words on the screen with
// no side effect on the disk.

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// osExit ends the process for the one command that exits nonzero with no
// sentence to print: `decks validate` on a failing deck. A variable so a test
// can observe the code instead of dying with the process.
var osExit = os.Exit

// decksCommand is the `mtglab decks` family: the library on disk.
func decksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decks",
		Short: "The library: list decks, run the gate, build artifacts, read the history",
	}
	cmd.AddCommand(decksListCommand())
	cmd.AddCommand(decksValidateCommand())
	cmd.AddCommand(decksBuildCommand())
	cmd.AddCommand(decksLogCommand())
	return cmd
}

// deckAt reads the deck at `<decks>/<slug>/deck.yaml`, or refuses with the
// recorded clean sentence when there is nothing there.
func deckAt(slug string) (*deck.Deck, error) {
	path := filepath.Join(settings().DecksDir, slug, "deck.yaml")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no deck at %s", path)
	}
	return deckFile(path)
}

// deckFile parses a deck file wherever it sits, the parent directory's name
// standing in for a slug the file does not declare -- the `--against`
// baseline included, which is exactly a deck file outside the library.
func deckFile(path string) (*deck.Deck, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return deck.FromText(string(raw), filepath.Base(filepath.Dir(path)))
}

// poolCards looks up every card in the deck. A nil map with no error is an
// absent pool, so callers degrade to structural checks with the gate's own
// visible warning -- never a silent pass.
//
// The names are the CLI's own list -- commander, the 99, the swap board, the
// companion -- and deliberately not `deckread.PoolFor`'s, which adds the
// graveyard for the deck page. The gate reads none of the extra entries, so
// the two are indistinguishable to every caller here, but the narrower list
// is this command's recorded behaviour and stays its own.
func poolCards(cmd *cobra.Command, d *deck.Deck) (map[string]*pool.CardRecord, error) {
	dbPath := settings().DBPath()
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil //nolint:nilnil // no pool, no error: absence is a degraded mode, not a fault
	}
	p := pool.New(dbPath, nil)
	defer p.Close()
	var cards map[string]*pool.CardRecord
	err := p.Use(cmd.Context(), func(c *pool.Conn) error {
		names := append([]string{}, d.Commander...)
		for _, card := range d.Cards {
			names = append(names, card.Name)
		}
		for _, card := range d.SwapBoard {
			names = append(names, card.Name)
		}
		if d.Companion != nil && *d.Companion != "" {
			names = append(names, *d.Companion)
		}
		found, err := c.GetCards(cmd.Context(), names)
		if err != nil {
			return err
		}
		cards = found
		return nil
	})
	return cards, err
}

// renderReport is `ValidationReport.render` plus `Issue.__str__`: every issue
// in the order the checks ran, errors and warnings interleaved, one line
// each -- `LEVEL code [card]: message`, the level padded to five columns so
// the codes line up under ERROR and WARN alike.
func renderReport(rep *gate.Report) string {
	if len(rep.Issues) == 0 {
		return "OK -- no issues."
	}
	lines := make([]string, 0, len(rep.Issues))
	for _, issue := range rep.Issues {
		where := ""
		if issue.Card != nil {
			where = " [" + *issue.Card + "]"
		}
		lines = append(lines, fmt.Sprintf("%-5s %s%s: %s",
			strings.ToUpper(issue.Level), issue.Code, where, issue.Message))
	}
	return strings.Join(lines, "\n")
}

// decksListCommand is `cmd_decks_list`: one line per deck. Only drafts are
// flagged -- curated is the norm and labelling it would make the one thing
// worth noticing harder to see.
func decksListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Every deck in the library, one line each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := settings().DecksDir
			if _, err := os.Stat(root); err != nil {
				return errors.New("no decks/ directory")
			}
			decks, err := library.NewFileSource(root, false).All(cmd.Context())
			if err != nil {
				return err
			}
			for _, d := range decks {
				commander := strings.Join(d.Commander, ", ")
				if commander == "" {
					commander = "?"
				}
				// `if deck.bracket:` -- truthy, so a bracket of zero is no
				// bracket, exactly as the artifacts' header reads it.
				bracket := "B?"
				if d.Bracket != nil && *d.Bracket != 0 {
					bracket = fmt.Sprintf("B%d", *d.Bracket)
				}
				draft := ""
				if d.Stage == "draft" {
					draft = fmt.Sprintf("  draft, %d to justify", len(d.Unjustified()))
				}
				fmt.Printf("  %-22s %-4s %3d cards   %s%s\n",
					d.Slug, bracket, d.TotalCards(), commander, draft)
			}
			return nil
		},
	}
}

// decksValidateCommand is the gate, rendered as text, and the exit code is
// the verdict -- nonzero on any error, with nothing printed beyond the
// report and its counts.
func decksValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <slug>",
		Short: "The gate -- run before anything else",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deckAt(args[0])
			if err != nil {
				return err
			}
			cards, err := poolCards(cmd, d)
			if err != nil {
				return err
			}
			rep := gate.Validate(d, cards, gate.DefaultSize)
			fmt.Println(renderReport(rep))
			fmt.Printf("\n%d error(s), %d warning(s)\n", len(rep.Errors()), len(rep.Warnings()))
			if len(rep.Errors()) > 0 {
				osExit(1)
			}
			return nil
		},
	}
}

// decksBuildCommand is `cmd_decks_build`: render the deliverables and write
// them, pruning what this build did not make (`artifacts.Store`, which also
// stashes the snapshot the next bare build diffs against -- ADR 30).
//
// Two refusals, and only one of them is forceable. Gate ERRORS are refused by
// default and `--force` overrides them, because they are things the deck got
// wrong. A DRAFT is refused by the renderer and no flag here reaches it: a
// draft is not wrong, it is unfinished, and the way out is to write the
// rationales and promote it (ADR 13), not to pass a flag.
func decksBuildCommand() *cobra.Command {
	var against string
	var force bool
	cmd := &cobra.Command{
		Use:   "build <slug>",
		Short: "Generate the artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			d, err := deckAt(slug)
			if err != nil {
				return err
			}
			cards, err := poolCards(cmd, d)
			if err != nil {
				return err
			}
			rep := gate.Validate(d, cards, gate.DefaultSize)
			if n := len(rep.Errors()); n > 0 && !force {
				fmt.Println(renderReport(rep))
				// The recorded refusal opens with a blank line to stand
				// apart from the report; the blank line is printed here and
				// the sentence rides the error, so the root's `mtglab: `
				// prefix lands on the sentence rather than on an empty line.
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
				return fmt.Errorf("refusing to generate with %d error(s). "+ //nolint:staticcheck // the recorded sentence, full stop included
					"Fix them, or pass --force if you know better.", n)
			}
			if len(rep.Warnings()) > 0 {
				// The report, a trailing space, and a blank line -- the
				// recorded bytes exactly, odd space included.
				fmt.Print(renderReport(rep) + " \n\n")
			}

			outdir := filepath.Join(settings().DecksDir, slug, "artifacts")

			// The baseline for swaps.md, resolved before `Store` moves it: an
			// explicit --against wins, and otherwise the previous build's
			// snapshot is the diff base (ADR 30 -- build before editing). A
			// first build has neither, and emits no swaps.md.
			var previous *deck.Deck
			if against != "" {
				previous, err = deckFile(against)
				if err != nil {
					return err
				}
			} else if snap := filepath.Join(outdir, artifacts.Snapshot); fileExists(snap) {
				previous, err = deckFile(snap)
				if err != nil {
					return err
				}
			}

			files, err := artifacts.RenderAll(d, artifacts.Options{Cards: cards, Previous: previous})
			if errors.Is(err, artifacts.ErrDraft) {
				// No --force here on purpose: see the note on `RenderAll`. The
				// way out of a draft is to write the rationales, not to pass a
				// flag.
				return fmt.Errorf("refusing to generate: %s", artifacts.Message(err))
			}
			if err != nil {
				return err
			}
			written, err := artifacts.Store(files, outdir)
			if err != nil {
				return err
			}
			for _, name := range written {
				fmt.Printf("  wrote %s\n", filepath.Join(outdir, name))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&against, "against", "",
		"path to a previous deck.yaml for swaps.md; defaults to the last build's own snapshot when one exists")
	cmd.Flags().BoolVar(&force, "force", false,
		"generate even when the gate reports errors")
	return cmd
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// decksLogCommand is `cmd_decks_log`: what has been done to this deck, newest
// first (ADR 28). It reads the file tier -- `owner_id IS NULL` -- because
// that is the only library the CLI ever edits; on a deployed instance the
// maintainer's own decks are that tier too, so
// `fly ssh console -C 'mtglab decks log gyome'` shows the same history the
// deck page does.
//
// The deck is loaded first, so a slug that is not a deck says so rather than
// printing an empty history, which is the same fact wearing a misleading
// face.
func decksLogCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "log <slug>",
		Short: "What has been done to this deck",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deckAt(args[0])
			if err != nil {
				return err
			}
			db, err := openAppDB()
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
			}
			rows, err := decklog.Entries(cmd.Context(), db, nil, args[0], limit)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Printf("\n  %s: nothing recorded yet.\n", d.Name)
				fmt.Print("  Edits made before this log existed were never recorded.\n\n")
				return nil
			}

			capped := ""
			if len(rows) == limit {
				capped = " (most recent; raise --limit for more)"
			}
			fmt.Printf("\n  %s -- %d entries%s\n\n", d.Name, len(rows), capped)
			for _, row := range rows {
				// An empty actor is whoever is at this machine: the CLI
				// itself, and the app with auth off. Not an unknown actor --
				// an unnamed one.
				who := "local"
				if row.Actor != nil && *row.Actor != "" {
					who = *row.Actor
				}
				fmt.Printf("  %-17s %-14s %s\n", logStamp(row.CreatedAt), who, row.Summary)
			}
			fmt.Println()
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "how many entries to show, newest first")
	return cmd
}

// openAppDB is the log's read handle: `app.db` opened read-only when the file
// exists, and nil when it does not -- a deck with no history. It must never
// create the file: the ladder belongs to the serving command, and a reader
// that acquires a database is the one thing this surface refuses to be.
func openAppDB() (*sql.DB, error) {
	path := settings().AppDBPath()
	if _, err := os.Stat(path); err != nil {
		return nil, nil //nolint:nilerr,nilnil // an absent app.db is an empty history, not a failure
	}
	return auth.Open(path)
}

// logStamp renders an ISO-8601 instant as something a terminal column can
// hold. Falls back to the raw string rather than failing: a row that
// cannot be parsed is still a row that says what happened, and a history that
// refuses to print because one timestamp is odd is worse than one ugly line.
func logStamp(stamp string) string {
	for _, layout := range []string{
		time.RFC3339Nano,                      // what the log's writer produces
		"2006-01-02T15:04:05.999999999",       // a naive instant, read rather than refused
		"2006-01-02 15:04:05.999999999Z07:00", // ... and a space separator
		"2006-01-02 15:04:05.999999999",
	} {
		if t, err := time.Parse(layout, stamp); err == nil {
			return t.Format("2006-01-02 15:04")
		}
	}
	return stamp
}
