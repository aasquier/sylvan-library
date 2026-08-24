package main

// The `mtglab decks` family, driven exactly as main wires it -- the same root
// with the same silences, `decks <cmd>` in argv -- against a scratch library:
// a [deployment] with its own directories, the 21-card pool copied in when a
// test wants card-level checks, `app.db` seeded through `authtest.Schema`
// when one wants history.
//
// Every command writes through Cobra's own writer (ADR 40), so a test reads a
// buffer rather than swapping [os.Stdout] for a pipe, and describes its
// library with a value rather than exporting one -- which is what lets all of
// these run at once. Expected strings are written as literals, not rebuilt
// with the same format verbs, so a wrong verb fails here instead of agreeing
// with itself.

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // registers "sqlite" for the seeded app.db

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

func writeDeck(t *testing.T, decksDir, slug, text string) {
	t.Helper()
	dir := filepath.Join(decksDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedAppDB writes an `app.db` in the ladder's recorded shape
// (`authtest.Schema`) and runs the given inserts.
func seedAppDB(t *testing.T, dataDir string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(authtest.Schema()); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
}

// evergreen is a curated 99 the gate passes clean against the tiny pool: one
// exempt basic at qty 99 under a tiny-pool commander.
const evergreen = `name: Evergreen
commander: Goreclaw, Terror of Qal Sisma
status: theoretical
stage: curated
bracket: 4
strategy: Forests forever.
cards:
  - name: Forest
    category: land
    qty: 99
    why: The forest fuels everything.
`

// seedling fails the gate on exactly one error: a single card where 99 are
// expected.
const seedling = `name: Seedling
commander: Goreclaw, Terror of Qal Sisma
status: theoretical
stage: curated
cards:
  - name: Forest
    category: land
    why: A start.
`

// sprouts is a draft still owing one rationale -- and two cards, so it also
// fails the gate on size.
const sprouts = `name: Sprouts
commander: Goreclaw, Terror of Qal Sisma
status: theoretical
stage: draft
cards:
  - name: Forest
    category: land
    why: Grown.
  - name: Llanowar Reborn
    category: land
`

// sproutwall is a draft that passes every gate ERROR (99 cards) and still owes
// its `why` -- the deck that reaches the renderer's own draft refusal.
const sproutwall = `name: Sproutwall
commander: Goreclaw, Terror of Qal Sisma
status: theoretical
stage: draft
cards:
  - name: Forest
    category: land
    qty: 99
`

func TestDecksListRefusesWithoutADecksDirectory(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	// A library that is not there: the directory the config names does not
	// exist, which is a fresh checkout before anything has made one.
	d.DecksDir = filepath.Join(d.DecksDir, "missing")
	out, err := d.run(t, "decks", "list")
	if err == nil || err.Error() != "no decks/ directory" {
		t.Fatalf("want the bare `no decks/ directory` refusal, got %v", err)
	}
	if out != "" {
		t.Fatalf("a refused list printed anyway: %q", out)
	}
}

func TestDecksListPrintsTheTable(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	writeDeck(t, d.DecksDir, "sprouts", sprouts)
	// A hand-started file with nothing in it: commander renders as `?`.
	writeDeck(t, d.DecksDir, "nameless", "cards: []\n")
	// Scaffolding and empty directories are not decks.
	writeDeck(t, d.DecksDir, "_template", "name: T\n")
	if err := os.MkdirAll(filepath.Join(d.DecksDir, "hollow"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := d.run(t, "decks", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"  evergreen              B4    99 cards   Goreclaw, Terror of Qal Sisma\n" +
		"  nameless               B?     0 cards   ?\n" +
		"  sprouts                B?     2 cards   Goreclaw, Terror of Qal Sisma  draft, 1 to justify\n"
	if out != want {
		t.Fatalf("the table diverged from the recorded layout:\nwant:\n%q\ngot:\n%q", want, out)
	}
}

func TestDecksListIsSilentOverAnEmptyLibrary(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	out, err := d.run(t, "decks", "list")
	if err != nil || out != "" {
		t.Fatalf("an empty library lists nothing and exits clean; got out=%q err=%v", out, err)
	}
}

func TestDecksValidateRefusesAMissingDeck(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	_, err := d.run(t, "decks", "validate", "nope")
	want := "no deck at " + filepath.Join(d.DecksDir, "nope", "deck.yaml")
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestDecksValidateDegradesWithoutThePool(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "seedling", seedling)

	// A failing gate is a nonzero exit with nothing extra printed: the
	// sentinel [errFailedGate], which `main` turns into a status and no
	// sentence. It used to be `osExit(1)` and a nil error.
	out, err := d.run(t, "decks", "validate", "seedling")
	if !errors.Is(err, errFailedGate) {
		t.Fatalf("a failing gate returned %v, want errFailedGate", err)
	}
	want := "ERROR deck-size: deck has 1 cards in the 99, expected 99\n" +
		"WARN  unverified: no card pool supplied; identity, legality and text were NOT checked\n" +
		"\n1 error(s), 1 warning(s)\n"
	if out != want {
		t.Fatalf("the report diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
}

func TestDecksValidatePassesAgainstThePool(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d = d.withPool(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)

	out, err := d.run(t, "decks", "validate", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	want := "OK -- no issues.\n\n0 error(s), 0 warning(s)\n"
	if out != want {
		t.Fatalf("a clean deck reads differently:\nwant:\n%q\ngot:\n%q", want, out)
	}
}

func TestDecksBuildWritesPrunesAndThenDiffs(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d = d.withPool(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	outdir := filepath.Join(d.DecksDir, "evergreen", "artifacts")

	// A stale swap list from a build that no longer exists: the first build
	// has no baseline, writes no swaps.md, and must prune this one.
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outdir, "swaps.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := d.run(t, "decks", "build", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	want := "" +
		"  wrote " + filepath.Join(outdir, "primer-quick.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "primer-advanced.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "decklist-annotated.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "moxfield.txt") + "\n" +
		"  wrote " + filepath.Join(outdir, "deck.last-built.yaml") + "\n"
	if out != want {
		t.Fatalf("first build diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
	if _, err := os.Stat(filepath.Join(outdir, "swaps.md")); !os.IsNotExist(err) {
		t.Fatal("the stale swaps.md survived the prune")
	}
	if _, err := os.Stat(filepath.Join(outdir, "deck.last-built.yaml")); err != nil {
		t.Fatal("the build did not stash its snapshot")
	}

	// Edit the deck (Sol Ring in for one Forest) and build again: the
	// snapshot is the baseline now, so swaps.md is written -- between
	// moxfield.txt and the snapshot, in Store's order.
	edited := strings.Replace(evergreen, "qty: 99", "qty: 98", 1) +
		"  - name: Sol Ring\n    category: ramp\n    why: Every deck wants the acceleration.\n"
	writeDeck(t, d.DecksDir, "evergreen", edited)

	out, err = d.run(t, "decks", "build", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	want = "" +
		"  wrote " + filepath.Join(outdir, "primer-quick.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "primer-advanced.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "decklist-annotated.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "moxfield.txt") + "\n" +
		"  wrote " + filepath.Join(outdir, "swaps.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "deck.last-built.yaml") + "\n"
	if out != want {
		t.Fatalf("second build diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
	swaps, err := os.ReadFile(filepath.Join(outdir, "swaps.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(swaps), "Sol Ring") {
		t.Fatalf("swaps.md does not mention the card that came in:\n%s", swaps)
	}
}

func TestDecksBuildHonoursAnExplicitBaseline(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d = d.withPool(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	outdir := filepath.Join(d.DecksDir, "evergreen", "artifacts")

	// The baseline lives wherever the operator says; only its parent
	// directory's name stands in for a slug, as `Deck.load` has it.
	baseline := filepath.Join(t.TempDir(), "was", "deck.yaml")
	if err := os.MkdirAll(filepath.Dir(baseline), 0o755); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(evergreen, "qty: 99", "qty: 98", 1) +
		"  - name: Sol Ring\n    category: ramp\n    why: Kept from the old list.\n"
	if err := os.WriteFile(baseline, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := d.run(t, "decks", "build", "evergreen", "--against", baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "  wrote "+filepath.Join(outdir, "swaps.md")+"\n") {
		t.Fatalf("--against did not produce a swaps.md:\n%s", out)
	}
	swaps, err := os.ReadFile(filepath.Join(outdir, "swaps.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Against that baseline, Sol Ring LEFT the deck.
	if !strings.Contains(string(swaps), "Sol Ring") {
		t.Fatalf("swaps.md does not mention the diffed card:\n%s", swaps)
	}
}

func TestDecksBuildRefusesGateErrorsUnlessForced(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "seedling", seedling)
	outdir := filepath.Join(d.DecksDir, "seedling", "artifacts")

	out, err := d.run(t, "decks", "build", "seedling")
	wantErr := "refusing to generate with 1 error(s). Fix them, or pass --force if you know better."
	if err == nil || err.Error() != wantErr {
		t.Fatalf("want %q, got %v", wantErr, err)
	}
	wantOut := "ERROR deck-size: deck has 1 cards in the 99, expected 99\n" +
		"WARN  unverified: no card pool supplied; identity, legality and text were NOT checked\n"
	if out != wantOut {
		t.Fatalf("the refusal prints the report first:\nwant:\n%q\ngot:\n%q", wantOut, out)
	}
	if _, err := os.Stat(outdir); !os.IsNotExist(err) {
		t.Fatal("a refused build must write nothing")
	}

	// --force overrides the gate (and only the gate). The report renders as
	// the warnings branch does -- `print(rep.render(), "\n")`, trailing space
	// and blank line included -- then the files are written.
	out, err = d.run(t, "decks", "build", "seedling", "--force")
	if err != nil {
		t.Fatal(err)
	}
	wantOut = "ERROR deck-size: deck has 1 cards in the 99, expected 99\n" +
		"WARN  unverified: no card pool supplied; identity, legality and text were NOT checked \n\n" +
		"  wrote " + filepath.Join(outdir, "primer-quick.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "primer-advanced.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "decklist-annotated.md") + "\n" +
		"  wrote " + filepath.Join(outdir, "moxfield.txt") + "\n" +
		"  wrote " + filepath.Join(outdir, "deck.last-built.yaml") + "\n"
	if out != wantOut {
		t.Fatalf("the forced build diverged:\nwant:\n%q\ngot:\n%q", wantOut, out)
	}
}

func TestDecksBuildRefusesADraftAndForceDoesNotReachIt(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "sproutwall", sproutwall)
	writeDeck(t, d.DecksDir, "sprouts", sprouts)

	// A draft that passes every gate ERROR still refuses at the renderer, in
	// the renderer's own words.
	_, err := d.run(t, "decks", "build", "sproutwall")
	wantErr := "refusing to generate: sproutwall is a draft, and the artifacts are the " +
		"shareable surface. 1 card(s) still need a `why`: Forest"
	if err == nil || err.Error() != wantErr {
		t.Fatalf("want %q, got %v", wantErr, err)
	}
	if _, err := os.Stat(filepath.Join(d.DecksDir, "sproutwall", "artifacts")); !os.IsNotExist(err) {
		t.Fatal("a refused draft build must write nothing")
	}

	// --force overrides gate errors and must NOT override the draft refusal:
	// the way out of a draft is to write the rationales, not to pass a flag.
	_, err = d.run(t, "decks", "build", "sprouts", "--force")
	wantErr = "refusing to generate: sprouts is a draft, and the artifacts are the " +
		"shareable surface. 1 card(s) still need a `why`: Llanowar Reborn"
	if err == nil || err.Error() != wantErr {
		t.Fatalf("want %q, got %v", wantErr, err)
	}
}

func TestDecksLogAnswersEmptyWithoutCreatingAppDB(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)

	out, err := d.run(t, "decks", "log", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	want := "\n  Evergreen: nothing recorded yet.\n" +
		"  Edits made before this log existed were never recorded.\n\n"
	if out != want {
		t.Fatalf("the empty answer diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
	// A reader must never create app.db on the way past -- the ladder
	// belongs to the serving command.
	if _, err := os.Stat(d.AppDBPath()); !os.IsNotExist(err) {
		t.Fatal("`decks log` acquired an app.db it had no business creating")
	}

	// And a slug that is not a deck says so, rather than printing an empty
	// history -- the same fact wearing a misleading face.
	_, err = d.run(t, "decks", "log", "nope")
	want = "no deck at " + filepath.Join(d.DecksDir, "nope", "deck.yaml")
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestDecksLogPrintsTheHistoryNewestFirst(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	seedAppDB(t, d.DataDir,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T12:00:00+00:00', NULL, 'evergreen', NULL, 'add', 'added Sol Ring as ramp')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-22T09:30:00.123456+00:00', NULL, 'evergreen', 'aaron', 'swap', 'swapped Forest out for Sol Ring')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('bad-stamp', NULL, 'evergreen', NULL, 'note', 'changed the mulligan note')`,
		// Another owner's deck under the same slug, and another deck of the
		// file tier's: both invisible to this history.
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-22T10:00:00+00:00', 2, 'evergreen', 'bob', 'add', 'added Swamp as land')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-22T10:00:00+00:00', NULL, 'other', NULL, 'add', 'added Swamp as land')`,
	)

	out, err := d.run(t, "decks", "log", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	// Newest first; an unnamed actor is `local`; a stamp that will not parse
	// prints raw rather than sinking the history.
	want := "\n  Evergreen -- 3 entries\n\n" +
		"  bad-stamp         local          changed the mulligan note\n" +
		"  2026-08-22 09:30  aaron          swapped Forest out for Sol Ring\n" +
		"  2026-08-21 12:00  local          added Sol Ring as ramp\n" +
		"\n"
	if out != want {
		t.Fatalf("the history diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
}

func TestDecksLogSaysWhenTheLimitCappedIt(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	seedAppDB(t, d.DataDir,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T12:00:00+00:00', NULL, 'evergreen', NULL, 'add', 'added Sol Ring as ramp')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T13:00:00+00:00', NULL, 'evergreen', NULL, 'note', 'changed the mulligan note')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T14:00:00+00:00', NULL, 'evergreen', NULL, 'swap', 'swapped Forest out for Sol Ring')`,
	)

	out, err := d.run(t, "decks", "log", "evergreen", "--limit", "2")
	if err != nil {
		t.Fatal(err)
	}
	want := "\n  Evergreen -- 2 entries (most recent; raise --limit for more)\n\n" +
		"  2026-08-21 14:00  local          swapped Forest out for Sol Ring\n" +
		"  2026-08-21 13:00  local          changed the mulligan note\n" +
		"\n"
	if out != want {
		t.Fatalf("the capped history diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
}
