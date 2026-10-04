package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two `decks` faults nothing reached: a deck directory the process cannot
// write into, and a history the database holds and will not hand over.
//
// Both are the same question as everywhere else -- not "did it fail" but "did
// it lie". A build that printed `wrote .../primer-quick.md` over a file it
// never wrote sends somebody to read a stale primer; an empty history over a
// table that could not be read says "nothing has been done to this deck",
// which is the one sentence `decks log` exists to be trusted about.

// A deck on a shelf the process cannot write to is refused, and nothing is
// reported as written.
//
// `0o500` is the read-only shelf from docs/polish/COVERAGE.md's lever 21: the
// deck file is still readable, the gate still runs, the artifacts still render,
// and only the one step that puts bytes on the disk fails. It is what a volume
// mounted read-only after an incident looks like from inside the command.
func TestABuildOntoAShelfItCannotWriteToWritesNothingAndSaysSo(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	shelf := filepath.Join(d.DecksDir, "evergreen")
	if err := os.Chmod(shelf, 0o500); err != nil {
		t.Fatal(err)
	}
	// Put back before the temp directory is swept, or the sweep cannot either.
	t.Cleanup(func() { _ = os.Chmod(shelf, 0o700) })

	out, err := d.run(t, "decks", "build", "evergreen")
	if err == nil {
		t.Fatalf("a build onto a read-only shelf reported success:\n%s", out)
	}
	if strings.Contains(out, "wrote ") {
		t.Errorf("the build named files it could not have written:\n%s", out)
	}
	// And it really wrote nothing: no artifacts directory at all.
	if _, err := os.Stat(filepath.Join(shelf, "artifacts")); err == nil {
		t.Error("the build created an artifacts directory on a read-only shelf")
	}
}

// A deck the artifacts cannot be rendered from is refused before anything is
// written, and `--force` does not get past it.
//
// Lever 26's shape (docs/polish/COVERAGE.md): a file that **parses perfectly**
// and still cannot be used. `strategy` is prose in every deck anything here
// writes, and a hand-written file may hold a list -- which YAML is happy with,
// the deck model passes through, and the whole-file dump cannot write back,
// because there is no ordering it could give a mapping or a list. So the deck
// loads, the gate reports on it, and the render refuses.
//
// The two refusals are deliberately different and this is the one that is not
// about rationales: `--force` exists for the gate's errors and is *not* a way
// past a deck that cannot be written down. A build that forced past this would
// leave an artifacts directory holding four fresh files and a snapshot from the
// last build -- which is the one state `swaps.md` can never be trusted over
// again.
func TestADeckThatCannotBeWrittenBackIsRefusedEvenWithForce(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)
	// Valid YAML, a real deck, and a strategy that is a list rather than prose.
	writeDeck(t, d.DecksDir, "listy", strings.ReplaceAll(evergreen,
		"strategy: Forests forever.",
		"strategy:\n  - Forests forever.\n  - And then some.")+"")

	out, err := d.run(t, "decks", "build", "listy", "--force")
	if err == nil {
		t.Fatalf("a deck that cannot be written back was built anyway:\n%s", out)
	}
	if strings.Contains(out, "wrote ") {
		t.Errorf("the build named files it could not have written:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(d.DecksDir, "listy", "artifacts")); statErr == nil {
		t.Error("a refused render still left an artifacts directory behind")
	}
}

// A history the database will not hand over is a refusal rather than an empty
// one.
//
// The absent-file case is already driven and answers "nothing recorded yet",
// which is true there: a box with no `app.db` has no history. The same sentence
// over a database that *has* one and could not be read is false, and it is the
// more likely of the two on a deployed instance -- the file is always there.
func TestAHistoryTheDatabaseWillNotHandOverIsNotAnEmptyOne(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	writeDeck(t, d.DecksDir, "evergreen", evergreen)
	seedAppDB(t, d.DataDir,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)
		   VALUES ('2026-08-21T12:00:00+00:00', NULL, 'evergreen', NULL, 'add', 'added Sol Ring as ramp')`)

	// It reads, before the table goes: so the refusal below is about the
	// table rather than about the fixture.
	out, err := d.run(t, "decks", "log", "evergreen")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "added Sol Ring as ramp") {
		t.Fatalf("the seeded history did not read back:\n%s", out)
	}

	hollowed(t, d, "deck_log")
	out, err = d.run(t, "decks", "log", "evergreen")
	if err == nil {
		t.Fatalf("a history that could not be read was answered anyway:\n%s", out)
	}
	if strings.Contains(out, "nothing recorded yet") {
		t.Errorf("an unreadable history was reported as an empty one:\n%s", out)
	}
}
