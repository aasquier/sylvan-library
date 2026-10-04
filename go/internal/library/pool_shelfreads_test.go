package library_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The shelf read against things on the volume that are not the files it expects.
//
// A deck's `artifacts/` directory is a real directory on a real mount that
// nothing but this code is supposed to write — and "supposed to" is the whole
// subject. A hand copy over `fly ssh sftp`, a half-finished restore, an
// unpacked tarball: each can leave something wearing a deliverable's name that
// is not a file. The rule is the one `FileSource.Artifacts` argues in its own
// comment: **"you have no artifacts" and "I cannot read your artifacts" are
// different sentences and only one of them is ever true**, and a shelf that
// answered the first when the second held would send somebody to rebuild a deck
// that is already built.

// Something wearing a deliverable's name that is not a file is not an artifact.
//
// A directory called `primer-quick.md` is what an unpacked archive leaves, and
// listing it as a built artifact would put a size and a date on the shelf for
// something the download route then cannot read.
func TestSomethingThatIsNotAFileIsNotAnArtifact(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	shelf := filepath.Join(root, "gyome", "artifacts")
	if err := os.MkdirAll(filepath.Join(shelf, artifacts.Deliverables[0], "inside"),
		0o750); err != nil {
		t.Fatal(err)
	}
	// One real artifact beside it, so "nothing is listed" is not the answer
	// either way.
	if err := os.WriteFile(filepath.Join(shelf, artifacts.Deliverables[1]),
		[]byte("# a primer\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := src.Artifacts(t.Context(), "gyome")
	if err != nil {
		t.Fatalf("reading a shelf with a directory on it: %v", err)
	}
	if len(got) != 1 || got[0].Name != artifacts.Deliverables[1] {
		t.Fatalf("the shelf listed %+v, want only the one real file", got)
	}
}

// A snapshot that cannot be read is not a deck that was never built.
//
// `swaps.md` diffs against the last build's own snapshot (ADR 28's sibling
// rule), so "unknown" is the honest answer for a deck nobody has built and a
// **lie** for a deck whose snapshot is sitting right there and will not open.
// The failure is the same shape as the one above: something on the volume
// wearing the name of a file.
func TestASnapshotThatWillNotOpenIsNotADeckThatWasNeverBuilt(t *testing.T) {
	t.Parallel()
	src, root := writableTier(t, "gyome")
	shelf := filepath.Join(root, "gyome", "artifacts")
	if err := os.MkdirAll(filepath.Join(shelf, library.Snapshot, "inside"),
		0o750); err != nil {
		t.Fatal(err)
	}

	text, built, err := src.ReadBaseline(t.Context(), "gyome")
	if err == nil {
		t.Fatalf("a snapshot that cannot be opened read back as built=%v, %q",
			built, text)
	}
	if built {
		t.Error("the refusal came back claiming the deck had been built")
	}
	if !strings.Contains(err.Error(), "gyome") {
		t.Errorf("the refusal does not name the deck: %v", err)
	}

	// And the ordinary absence is still the ordinary answer, so the guard above
	// is a guard rather than a shelf that refuses everything.
	plain, _ := writableTier(t, "cats")
	if _, built, err := plain.ReadBaseline(t.Context(), "cats"); err != nil || built {
		t.Errorf("a deck built for the first time answered built=%v, %v", built, err)
	}
}

// The artifact shelf in the SQL tier, cut short partway down its own result set.
//
// `halfansweringdb_test.go` sweeps the row budget across three reads at once,
// which means the shelf's own walk is reached only when the two before it have
// not already spent the budget — and the deck row this read looks up first
// spends one. Asked on its own, with a budget wide enough to find the deck and
// not wide enough to list what is on its shelf, the walk's verdict is the only
// thing between a shelf of two files and a shelf reported as one.
func TestTheSQLArtifactShelfRefusesAWalkCutShort(t *testing.T) {
	t.Parallel()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := auth.Create(t.Context(), db, "alice", "alice@example.com", false); err != nil {
		t.Fatal(err)
	}
	src := library.NewSQLSource(db, db, 1, true, false)
	if err := src.Create(t.Context(), "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if _, err := src.WriteArtifacts(t.Context(), "gyome", artifacts.Files{
		{Name: artifacts.Deliverables[0], Text: "# a primer\n"},
		{Name: artifacts.Deliverables[1], Text: "# the long one\n"},
		{Name: artifacts.Deliverables[2], Text: "# swaps\n"},
	}); err != nil {
		t.Fatal(err)
	}
	fault.Heal()

	const whole = 3
	refused, full := 0, 0
	for rows := 0; rows <= 8; rows++ {
		fault.RowsAfter(rows)
		got, err := src.Artifacts(t.Context(), "gyome")
		fault.Heal()
		if err != nil {
			refused++
			continue
		}
		if len(got) != whole {
			t.Errorf("at row budget %d the shelf listed %d of %d files with no "+
				"error", rows, len(got), whole)
		}
		full++
	}
	if refused == 0 || full == 0 {
		t.Errorf("%d row budgets refused and %d answered in full -- the sweep "+
			"never crossed the walk", refused, full)
	}
}

// Two more walks cut short, in the reads a short answer is quietest in.
//
// **The browse tab** lists every shared deck in the SQL tier, and a walk that
// stopped halfway down it would show somebody a list of owners with a name
// missing and nothing anywhere saying so -- which is the same bug as a shelf
// read short, one level up. **The crypt** is the other: a deck is deleted by
// marking its row, and the crypt is the only place it can be found again, so a
// crypt listed short is a deck somebody is told is gone for good.
func TestTheSharedListAndTheCryptRefuseAWalkCutShort(t *testing.T) {
	t.Parallel()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, row := range []struct{ name, email string }{
		{"alice", "alice@example.com"}, {"bob", "bob@example.com"},
		{"carol", "carol@example.com"},
	} {
		if _, err := auth.Create(t.Context(), db, row.name, row.email, false); err != nil {
			t.Fatal(err)
		}
	}
	// Three shared decks under one owner, and three of another's in the crypt:
	// both lists are long enough to be cut in the middle.
	bob := library.NewSQLSource(db, db, 2, true, false)
	for _, slug := range []string{"cats", "dinos", "stompy"} {
		if err := bob.Create(t.Context(), slug,
			strings.ReplaceAll(sqlDeck, "gyome", slug)); err != nil {
			t.Fatal(err)
		}
		if err := bob.SetShared(t.Context(), slug, true); err != nil {
			t.Fatal(err)
		}
	}
	buried := library.NewSQLSource(db, db, 3, true, false)
	for _, slug := range []string{"old-cats", "old-dinos", "old-stompy"} {
		if err := buried.Create(t.Context(), slug,
			strings.ReplaceAll(sqlDeck, "gyome", slug)); err != nil {
			t.Fatal(err)
		}
		if _, err := buried.Delete(t.Context(), slug); err != nil {
			t.Fatal(err)
		}
	}
	fault.Heal()

	const shared, entombed = 3, 3
	refused, full := 0, 0
	for rows := 0; rows <= 10; rows++ {
		fault.RowsAfter(rows)
		list, listErr := library.SharedDecks(t.Context(), db)
		crypt, cryptErr := buried.Entombed(t.Context())
		fault.Heal()
		if listErr == nil && len(list) != shared {
			t.Errorf("at row budget %d the shared list named %d of %d decks with "+
				"no error", rows, len(list), shared)
		}
		if cryptErr == nil && len(crypt) != entombed {
			t.Errorf("at row budget %d the crypt held %d of %d decks with no "+
				"error", rows, len(crypt), entombed)
		}
		if listErr != nil || cryptErr != nil {
			refused++
			continue
		}
		full++
	}
	if refused == 0 || full == 0 {
		t.Errorf("%d row budgets refused and %d answered in full -- the sweep "+
			"never crossed the walk", refused, full)
	}
}
