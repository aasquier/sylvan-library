package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
)

// A card pool that **opens and then cannot answer**, and what the runbook
// commands say to one.
//
// This is `internal/api`'s schema-less pool at the CLI, and the distinction it
// rests on is the one that is easy to lose. A machine with **no** pool is a
// fresh checkout before `mtglab data refresh`: `pool.ErrNoPool`, a documented
// degraded answer, and every command here has a sentence for it. A file that
// DuckDB opens happily and then fails a `SELECT` against is a half-written
// refresh, a truncated restore, or a schema older than the binary — a real
// error, carrying DuckDB's own words, on a path nothing in this package had
// ever driven. A *corrupt* file is neither: `pool.Pool.Use` cannot open it and
// answers `ErrNoPool`, which re-drives the degraded path.
//
// Every command below has to **say something** and must not answer as though
// the pool were merely absent — a `decks validate` that fell back to
// structural checks over a pool it could not read would pass a deck it had
// never looked a card up for.

// halfARefresh writes a real DuckDB file with none of the pool's tables in it,
// where [config.Config.DBPath] looks.
func halfARefresh(t *testing.T, d deployment) deployment {
	t.Helper()
	db, err := sql.Open("duckdb", d.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	// One table, so the file is a database rather than an empty stub -- and
	// deliberately not one of ours, so nothing the app asks for resolves.
	if _, err := db.Exec(`CREATE TABLE half_a_refresh (name VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return d
}

// The sanity check the rest of this file rests on: the fault really is a query
// error rather than the degraded answer. Without it, a change that folded the
// two together would leave every test below passing for the wrong reason.
//
// Both refusals end with the same advice — `run mtglab data refresh`, which is
// the right advice for a pool in either state — so what separates them is what
// comes before it: a query that failed, named.
func TestAPoolThatCannotAnswerIsAFaultRatherThanAnAbsence(t *testing.T) {
	t.Parallel()
	d := halfARefresh(t, scratchDeployment(t))
	_, err := d.run(t, "cards", "show", "Sol Ring")
	if err == nil {
		t.Fatal("a pool with none of its tables answered for a card")
	}
	if !strings.Contains(err.Error(), "get_cards") {
		t.Errorf("a half-written pool was reported as an absent one: %v", err)
	}
	// And an absent pool is not: the two states arrive at different sentences
	// even though they end with the same advice.
	absent := scratchDeployment(t)
	if _, err := absent.run(t, "cards", "show", "Sol Ring"); err == nil ||
		strings.Contains(err.Error(), "get_cards") {
		t.Errorf("an absent pool was reported as a broken one: %v", err)
	}
}

// Every command that looks a card up, over a pool that cannot answer.
func TestNoCommandQuotesAPoolThatCouldNotAnswer(t *testing.T) {
	t.Parallel()
	d := halfARefresh(t, scratchDeployment(t))
	writeDeck(t, d.DecksDir, "gyome", monoGreenText(t))

	for _, argv := range [][]string{
		{"cards", "show", "Sol Ring"},
		{"decks", "validate", "gyome"},
		{"decks", "build", "gyome"},
		{"sim", "mana", "gyome", "--games", "2"},
		{"sim", "shelf", "gyome"},
	} {
		name := strings.Join(argv, " ")
		out, err := d.run(t, argv...)
		if err == nil {
			t.Errorf("`mtglab %s` answered over a pool it could not read:\n%s", name, out)
			continue
		}
		if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("`mtglab %s` refused with an empty error", name)
		}
	}
}

// A deck's swap board and its companion are looked up too, and they are
// **this command's own list** rather than the deck page's — the graveyard is
// deliberately not in it. A name that only appears on the board is the one a
// narrowing would lose without anything failing.
func TestTheSwapBoardAndTheCompanionAreLookedUpWithTheRest(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)
	// Kaheera is the gate corpus's companion deck; its board and companion are
	// cards the fixture pool holds.
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "gate",
		"testdata", "kaheera.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "companion:") {
		t.Fatal("the fixture no longer declares a companion; this test measures nothing")
	}
	writeDeck(t, d.DecksDir, "kaheera", text)

	// The companion is a card the gate can only judge if it was looked up, so
	// a report that mentions it at all is a report built from a pool lookup.
	out, err := d.run(t, "decks", "validate", "kaheera")
	if err != nil && !strings.Contains(err.Error(), "error(s)") {
		t.Logf("decks validate kaheera:\n%s", out)
	}
	if !strings.Contains(out, "error(s)") {
		t.Errorf("the gate printed no counts:\n%s", out)
	}

	// And the same list serves the simulator, which reads the board so a
	// swapped-in card is compiled beside the 99.
	if _, err := d.run(t, "sim", "shelf", "kaheera"); err != nil {
		t.Errorf("sim shelf over a deck with a board: %v", err)
	}

	// The gate reads the board too, which is what lets it say a swap would
	// break the deck's identity **before** somebody makes it. A card on the
	// board that never reached the pool lookup would be judged against no card
	// at all, and the gate would pass it in silence.
	writeDeck(t, d.DecksDir, "boarded", "slug: boarded\nname: Boarded\n"+
		"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
		"  - name: Forest\n    qty: 99\n    why: the mana\n"+
		"swap_board:\n  - name: Cyclonic Rift\n    why: outside the identity\n")
	boarded, _ := d.run(t, "decks", "validate", "boarded")
	if !strings.Contains(boarded, "Cyclonic Rift") {
		t.Errorf("the gate never judged the card on the board:\n%s", boarded)
	}
}

// A deck file handed in by path — `--against`, the snapshot a build diffs
// against — is refused by name when it cannot be read, rather than treated as
// a first build with nothing to compare to.
//
// **Silence is the wrong answer here and it is the tempting one.** A build
// whose baseline would not open and carried on would emit `swaps.md` claiming
// the deck changed in every slot, which reads as a diff rather than as a
// failure.
func TestABaselineThatWillNotOpenIsRefusedRatherThanTreatedAsAFirstBuild(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)
	// The deck the rest of this package builds with, which passes the gate --
	// a build refused for the deck's own sake would never reach the baseline.
	writeDeck(t, d.DecksDir, "evergreen", evergreen)

	_, err := d.run(t, "decks", "build", "evergreen",
		"--against", filepath.Join(t.TempDir(), "never-written.yaml"))
	if err == nil {
		t.Fatal("a build diffed against a baseline it could not read")
	}

	// And the other door to the same helper: the previous build's own
	// snapshot, sitting where the next build looks for it and unreadable.
	if _, err := d.run(t, "decks", "build", "evergreen"); err != nil {
		t.Fatalf("the first build failed: %v", err)
	}
	snapshot := filepath.Join(d.DecksDir, "evergreen", "artifacts", artifacts.Snapshot)
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatalf("the build stashed no snapshot, so the next one has nothing "+
			"to diff against: %v", err)
	}
	if err := os.WriteFile(snapshot, []byte("\tthis is not YAML at all\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.run(t, "decks", "build", "evergreen"); err == nil {
		t.Error("a build read past a snapshot it could not parse")
	}
}

// A library directory the process cannot read is a refusal rather than an
// empty roster — the same sentence `unmounted_test.go` insists on, for the
// case where the volume mounted and the permissions did not.
func TestAnUnreadableLibraryIsNotAnEmptyOne(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable directory to make")
	}
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "gyome", monoGreenText(t))
	if err := os.Chmod(d.DecksDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(d.DecksDir, 0o750) })

	out, err := d.run(t, "decks", "list")
	if err == nil {
		t.Fatalf("a library nobody can read was listed anyway:\n%s", out)
	}
}

// The gate's report names the card an issue is about, in brackets, because a
// message about "a card" in a hundred-card deck is a message about nothing.
func TestAGateIssueAboutACardSaysWhichCard(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)
	// A deck whose commander is green and which runs a card outside that
	// identity: the gate's answer is per-card, and the card is the half the
	// reader needs.
	writeDeck(t, d.DecksDir, "offcolour", "slug: offcolour\nname: Off Colour\n"+
		"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
		"  - name: Cyclonic Rift\n    why: not in this identity\n")

	out, _ := d.run(t, "decks", "validate", "offcolour")
	if !strings.Contains(out, "[Cyclonic Rift]") {
		t.Errorf("the report never named the card it is about:\n%s", out)
	}
}

// `sim mana` over a deck file the process cannot read is a refusal that is not
// the missing-deck sentence: a deck that is there and unreadable is a
// different problem from a deck that is not there, and the operator's next
// move differs.
func TestADeckFileThatCannotBeReadIsNotReportedAsMissing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable file to make")
	}
	d := scratchDeployment(t).withPool(t)
	writeDeck(t, d.DecksDir, "gyome", monoGreenText(t))
	path := filepath.Join(d.DecksDir, "gyome", "deck.yaml")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := d.run(t, "sim", "mana", "gyome")
	if err == nil {
		t.Fatal("a deck nobody can read was simulated")
	}
	if strings.HasPrefix(err.Error(), "no deck at ") {
		t.Errorf("an unreadable deck was reported as an absent one: %v", err)
	}
}

// A pool whose absence is genuine still reads as absence, which is the
// sentence the whole degraded mode rests on — and the guard that keeps the
// fixture above from having quietly redefined it.
//
// **The expectation comes off the refusal's own type rather than being typed.**
// What stood here matched the fragment `"mtglab data refresh"` and then, after
// that assertion had already passed, skipped the test if `pool.ErrNoPool`'s
// wording had changed — a conditional with no assertion in it, whose only
// possible effect was to relabel a green test as one that never ran. It was
// also aimed at the wrong sentence: `sim mana` does not surface
// `pool.ErrNoPool` at all. The compiler refuses first, with
// [compile.PoolRequired]'s own words, because mana production cannot be
// inferred from a deck file and a guess would look authoritative. Taking the
// sentence off that type is what the dead half was reaching for: a rewording
// moves this test with it instead of leaving it matching a fragment that used
// to be in there.
func TestAnAbsentPoolStillSaysToRefreshIt(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "gyome", monoGreenText(t))
	_, err := d.run(t, "sim", "mana", "gyome")
	want := (&compile.PoolRequired{}).Error()
	if err == nil || err.Error() != want {
		t.Fatalf("an absent pool answered %v, want the simulator's own refusal (%q)",
			err, want)
	}
}
