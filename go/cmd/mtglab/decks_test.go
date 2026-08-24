package main

// The `mtglab decks` family, driven exactly as main wires it -- a root with
// the same silences, `decks <cmd>` in argv -- against a scratch library:
// MTGLAB_DECKS_DIR and MTGLAB_DATA_DIR into temp dirs, the 21-card pool
// copied to `<data>/mtg.duckdb` when a test wants card-level checks,
// `app.db` seeded through `authtest.Schema` when one wants history.
//
// The commands print with bare fmt.Printf, so stdout is captured with a
// pipe rather than through cobra's own writer -- which keeps the command
// code plain. Expected strings are written as literals, not rebuilt with
// the same format verbs, so a wrong verb fails here instead of agreeing
// with itself.

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite" // registers "sqlite" for the seeded app.db

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// runDecks executes `mtglab decks <args...>` and returns what it printed to
// stdout and the error the root would render as `mtglab: <err>`.
func runDecks(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "mtglab", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(decksCommand())
	root.SetArgs(append([]string{"decks"}, args...))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	captured := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		captured <- buf.String()
	}()
	execErr := root.Execute()
	os.Stdout = old
	_ = w.Close()
	out := <-captured
	_ = r.Close()
	return out, execErr
}

// trapExit swaps osExit for a recorder, so `validate` on a failing deck is a
// recorded code rather than a dead test process.
func trapExit(t *testing.T) *[]int {
	t.Helper()
	codes := &[]int{}
	old := osExit
	osExit = func(code int) { *codes = append(*codes, code) }
	t.Cleanup(func() { osExit = old })
	return codes
}

// scratch points the config env at fresh directories and returns them.
func scratch(t *testing.T) (decksDir, dataDir string) {
	t.Helper()
	decksDir = filepath.Join(t.TempDir(), "decks")
	dataDir = t.TempDir()
	if err := os.MkdirAll(decksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MTGLAB_DECKS_DIR", decksDir)
	t.Setenv("MTGLAB_DATA_DIR", dataDir)
	return decksDir, dataDir
}

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

// installPool copies the 21-card fixture pool to `<data>/mtg.duckdb`, which
// is where `config.DBPath()` will look.
func installPool(t *testing.T, dataDir string) {
	t.Helper()
	raw, err := os.ReadFile(pooltest.Build(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "mtg.duckdb"), raw, 0o644); err != nil {
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
	decksDir, _ := scratch(t)
	t.Setenv("MTGLAB_DECKS_DIR", filepath.Join(decksDir, "missing"))
	out, err := runDecks(t, "list")
	if err == nil || err.Error() != "no decks/ directory" {
		t.Fatalf("want the bare `no decks/ directory` refusal, got %v", err)
	}
	if out != "" {
		t.Fatalf("a refused list printed anyway: %q", out)
	}
}

func TestDecksListPrintsTheTable(t *testing.T) {
	decksDir, _ := scratch(t)
	writeDeck(t, decksDir, "evergreen", evergreen)
	writeDeck(t, decksDir, "sprouts", sprouts)
	// A hand-started file with nothing in it: commander renders as `?`.
	writeDeck(t, decksDir, "nameless", "cards: []\n")
	// Scaffolding and empty directories are not decks.
	writeDeck(t, decksDir, "_template", "name: T\n")
	if err := os.MkdirAll(filepath.Join(decksDir, "hollow"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runDecks(t, "list")
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
	scratch(t)
	out, err := runDecks(t, "list")
	if err != nil || out != "" {
		t.Fatalf("an empty library lists nothing and exits clean; got out=%q err=%v", out, err)
	}
}

func TestDecksValidateRefusesAMissingDeck(t *testing.T) {
	decksDir, _ := scratch(t)
	_, err := runDecks(t, "validate", "nope")
	want := "no deck at " + filepath.Join(decksDir, "nope", "deck.yaml")
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestDecksValidateDegradesWithoutThePool(t *testing.T) {
	decksDir, _ := scratch(t)
	writeDeck(t, decksDir, "seedling", seedling)
	codes := trapExit(t)

	out, err := runDecks(t, "validate", "seedling")
	if err != nil {
		t.Fatal(err)
	}
	want := "ERROR deck-size: deck has 1 cards in the 99, expected 99\n" +
		"WARN  unverified: no card pool supplied; identity, legality and text were NOT checked\n" +
		"\n1 error(s), 1 warning(s)\n"
	if out != want {
		t.Fatalf("the report diverged:\nwant:\n%q\ngot:\n%q", want, out)
	}
	if len(*codes) != 1 || (*codes)[0] != 1 {
		t.Fatalf("a failing deck exits 1, exactly once; got %v", *codes)
	}
}

func TestDecksValidatePassesAgainstThePool(t *testing.T) {
	decksDir, dataDir := scratch(t)
	installPool(t, dataDir)
	writeDeck(t, decksDir, "evergreen", evergreen)
	codes := trapExit(t)

	out, err := runDecks(t, "validate", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	want := "OK -- no issues.\n\n0 error(s), 0 warning(s)\n"
	if out != want {
		t.Fatalf("a clean deck reads differently:\nwant:\n%q\ngot:\n%q", want, out)
	}
	if len(*codes) != 0 {
		t.Fatalf("a passing deck must not exit nonzero; recorded %v", *codes)
	}
}

func TestDecksBuildWritesPrunesAndThenDiffs(t *testing.T) {
	decksDir, dataDir := scratch(t)
	installPool(t, dataDir)
	writeDeck(t, decksDir, "evergreen", evergreen)
	outdir := filepath.Join(decksDir, "evergreen", "artifacts")

	// A stale swap list from a build that no longer exists: the first build
	// has no baseline, writes no swaps.md, and must prune this one.
	if err := os.MkdirAll(outdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outdir, "swaps.md"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runDecks(t, "build", "evergreen")
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
	writeDeck(t, decksDir, "evergreen", edited)

	out, err = runDecks(t, "build", "evergreen")
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
	decksDir, dataDir := scratch(t)
	installPool(t, dataDir)
	writeDeck(t, decksDir, "evergreen", evergreen)
	outdir := filepath.Join(decksDir, "evergreen", "artifacts")

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

	out, err := runDecks(t, "build", "evergreen", "--against", baseline)
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
	decksDir, _ := scratch(t)
	writeDeck(t, decksDir, "seedling", seedling)
	outdir := filepath.Join(decksDir, "seedling", "artifacts")

	out, err := runDecks(t, "build", "seedling")
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
	out, err = runDecks(t, "build", "seedling", "--force")
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
	decksDir, _ := scratch(t)
	writeDeck(t, decksDir, "sproutwall", sproutwall)
	writeDeck(t, decksDir, "sprouts", sprouts)

	// A draft that passes every gate ERROR still refuses at the renderer, in
	// the renderer's own words.
	_, err := runDecks(t, "build", "sproutwall")
	wantErr := "refusing to generate: sproutwall is a draft, and the artifacts are the " +
		"shareable surface. 1 card(s) still need a `why`: Forest"
	if err == nil || err.Error() != wantErr {
		t.Fatalf("want %q, got %v", wantErr, err)
	}
	if _, err := os.Stat(filepath.Join(decksDir, "sproutwall", "artifacts")); !os.IsNotExist(err) {
		t.Fatal("a refused draft build must write nothing")
	}

	// --force overrides gate errors and must NOT override the draft refusal:
	// the way out of a draft is to write the rationales, not to pass a flag.
	_, err = runDecks(t, "build", "sprouts", "--force")
	wantErr = "refusing to generate: sprouts is a draft, and the artifacts are the " +
		"shareable surface. 1 card(s) still need a `why`: Llanowar Reborn"
	if err == nil || err.Error() != wantErr {
		t.Fatalf("want %q, got %v", wantErr, err)
	}
}

func TestDecksLogAnswersEmptyWithoutCreatingAppDB(t *testing.T) {
	decksDir, dataDir := scratch(t)
	writeDeck(t, decksDir, "evergreen", evergreen)

	out, err := runDecks(t, "log", "evergreen")
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
	if _, err := os.Stat(filepath.Join(dataDir, "app.db")); !os.IsNotExist(err) {
		t.Fatal("`decks log` acquired an app.db it had no business creating")
	}

	// And a slug that is not a deck says so, rather than printing an empty
	// history -- the same fact wearing a misleading face.
	_, err = runDecks(t, "log", "nope")
	want = "no deck at " + filepath.Join(decksDir, "nope", "deck.yaml")
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestDecksLogPrintsTheHistoryNewestFirst(t *testing.T) {
	decksDir, dataDir := scratch(t)
	writeDeck(t, decksDir, "evergreen", evergreen)
	seedAppDB(t, dataDir,
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

	out, err := runDecks(t, "log", "evergreen")
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
	decksDir, dataDir := scratch(t)
	writeDeck(t, decksDir, "evergreen", evergreen)
	seedAppDB(t, dataDir,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T12:00:00+00:00', NULL, 'evergreen', NULL, 'add', 'added Sol Ring as ramp')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T13:00:00+00:00', NULL, 'evergreen', NULL, 'note', 'changed the mulligan note')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T14:00:00+00:00', NULL, 'evergreen', NULL, 'swap', 'swapped Forest out for Sol Ring')`,
	)

	out, err := runDecks(t, "log", "evergreen", "--limit", "2")
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
