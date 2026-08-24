package library_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The two tiers are held to each other over the artifacts, which is the one
// place they have already disagreed: until 2026-08-21 a rebuild that produced
// no `swaps.md` left the previous build's sitting in the file tier's directory
// while the memory tier replaced the set, and only one of them could be right.
// So the promises are parametrised over the sources: the same assertions run
// against the two tiers actually served.
//
// Every assertion here is deliberately about a *promise* rather than an
// implementation: what a rebuild leaves behind, what order a reader sees, and
// what the snapshot is and is not. A tier is free to keep artifacts in a
// directory or in a table; it is not free to answer differently.

const parityDeck = `slug: mini
name: Mini
status: built
stage: curated
commander:
  - Gyome, Master Chef
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana.
`

func fileTier(t *testing.T) library.Source {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "mini")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(parityDeck), 0o644); err != nil {
		t.Fatal(err)
	}
	return library.NewFileSource(root, true)
}

func sqlTier(t *testing.T) library.Source {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ddl := `
CREATE TABLE user_decks (id INTEGER PRIMARY KEY AUTOINCREMENT, owner_id INTEGER NOT NULL,
  slug TEXT NOT NULL, name TEXT NOT NULL, yaml TEXT NOT NULL, shared INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE user_deck_artifacts (id INTEGER PRIMARY KEY AUTOINCREMENT, deck_id INTEGER NOT NULL,
  name TEXT NOT NULL, body TEXT NOT NULL, built_at TEXT NOT NULL, UNIQUE(deck_id, name));`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO user_decks (id, owner_id, slug, name, yaml, created_at, updated_at)"+
			" VALUES (1, 7, 'mini', 'Mini', ?, '2026-08-22T00:00:00+00:00', '2026-08-22T00:00:00+00:00')",
		parityDeck); err != nil {
		t.Fatal(err)
	}
	return library.NewSQLSource(db, db, 7, true, false)
}

func tiers(t *testing.T) map[string]library.Source {
	t.Helper()
	return map[string]library.Source{"file": fileTier(t), "sql": sqlTier(t)}
}

func writer(t *testing.T, src library.Source) library.Writer {
	t.Helper()
	w, err := library.WriterFor(src, "mini")
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func held(t *testing.T, src library.Source) []string {
	t.Helper()
	list, err := src.Artifacts(context.Background(), "mini")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.Name)
	}
	return out
}

// A deck nobody has built has no artifacts and no baseline, and neither is an
// error: that is the ordinary state of a deck on the day it is created.
func TestAnUnbuiltDeckHasNothingAndThatIsNotAnError(t *testing.T) {
	t.Parallel()
	for name, src := range tiers(t) {
		t.Run(name, func(t *testing.T) {
			if got := held(t, src); len(got) != 0 {
				t.Errorf("an unbuilt deck holds %v", got)
			}
			text, present, err := src.ReadBaseline(context.Background(), "mini")
			if err != nil || present || text != "" {
				t.Errorf("baseline: %q %v %v", text, present, err)
			}
		})
	}
}

// A reader sees `Deliverables` order, whatever order the build wrote in --
// the primers first and `swaps.md` last, because that is the order a person
// reads them and not the order a map or a table happens to yield.
func TestBothTiersListInDeliverablesOrderNotWriteOrder(t *testing.T) {
	t.Parallel()
	backwards := artifacts.Files{}
	for i := len(artifacts.Deliverables) - 1; i >= 0; i-- {
		backwards = append(backwards, artifacts.File{Name: artifacts.Deliverables[i], Text: "x"})
	}
	backwards = append(backwards, artifacts.File{Name: artifacts.Snapshot, Text: parityDeck})

	for name, src := range tiers(t) {
		t.Run(name, func(t *testing.T) {
			if _, err := writer(t, src).WriteArtifacts(context.Background(), "mini", backwards); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(held(t, src), ","); got != strings.Join(artifacts.Deliverables, ",") {
				t.Errorf("listed %s", got)
			}
		})
	}
}

// A rebuild leaves exactly what it produced. The `swaps.md` of a build that
// had a baseline must not survive one that did not -- it describes a diff that
// no longer exists, which is stale in the one way indistinguishable from
// current, and it is where the two tiers disagreed before.
func TestBothTiersPruneWhatARebuildDidNotProduce(t *testing.T) {
	t.Parallel()
	full := artifacts.Files{}
	for _, n := range artifacts.Deliverables {
		full = append(full, artifacts.File{Name: n, Text: "from the last build"})
	}
	full = append(full, artifacts.File{Name: artifacts.Snapshot, Text: parityDeck})

	partial := artifacts.Files{
		{Name: "primer-quick.md", Text: "fresh"},
		{Name: artifacts.Snapshot, Text: parityDeck},
	}

	for name, src := range tiers(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			w := writer(t, src)
			if _, err := w.WriteArtifacts(ctx, "mini", full); err != nil {
				t.Fatal(err)
			}
			if got := held(t, src); len(got) != len(artifacts.Deliverables) {
				t.Fatalf("the first build left %v", got)
			}
			if _, err := w.WriteArtifacts(ctx, "mini", partial); err != nil {
				t.Fatal(err)
			}
			if got := held(t, src); strings.Join(got, ",") != "primer-quick.md" {
				t.Errorf("the rebuild left %v", got)
			}
			// The snapshot is not a deliverable, so it survives -- and it is
			// this build's, not the last one's.
			text, present, err := src.ReadBaseline(ctx, "mini")
			if err != nil || !present || text != parityDeck {
				t.Errorf("the baseline did not survive: %q %v %v", text, present, err)
			}
			body, err := src.ReadArtifact(ctx, "mini", "primer-quick.md")
			if err != nil || body != "fresh" {
				t.Errorf("the surviving deliverable is %q (%v)", body, err)
			}
		})
	}
}

// The snapshot is stored by every build and served by neither tier: it is the
// build's own bookkeeping, `Deliverables` is what a reader may ask for, and
// that same tuple is the path-traversal guard.
func TestNeitherTierServesTheSnapshot(t *testing.T) {
	t.Parallel()
	stored := artifacts.Files{
		{Name: "moxfield.txt", Text: "1 Sol Ring\n"},
		{Name: artifacts.Snapshot, Text: parityDeck},
	}
	for name, src := range tiers(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if _, err := writer(t, src).WriteArtifacts(ctx, "mini", stored); err != nil {
				t.Fatal(err)
			}
			if got := held(t, src); strings.Join(got, ",") != "moxfield.txt" {
				t.Errorf("the shelf shows %v; the snapshot is not a deliverable", got)
			}
			var refused library.ErrArtifactNotFound
			for _, name := range []string{artifacts.Snapshot, "../../deck.yaml", "swaps.md"} {
				if _, err := src.ReadArtifact(ctx, "mini", name); !errors.As(err, &refused) {
					t.Errorf("reading %q gave %v", name, err)
				}
			}
		})
	}
}

// A deck that is not there is ErrNotFound rather than an empty shelf: "never
// built" and "no such deck" are answers a caller has to be able to tell apart.
func TestBothTiersRefuseAnUnknownDeck(t *testing.T) {
	t.Parallel()
	for name, src := range tiers(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var missing library.ErrNotFound
			if _, err := src.Artifacts(ctx, "nope"); !errors.As(err, &missing) {
				t.Errorf("Artifacts: %v", err)
			}
			if _, _, err := src.ReadBaseline(ctx, "nope"); !errors.As(err, &missing) {
				t.Errorf("ReadBaseline: %v", err)
			}
			if _, err := writer(t, src).WriteArtifacts(ctx, "nope",
				artifacts.Files{{Name: "moxfield.txt", Text: "x"}}); !errors.As(err, &missing) {
				t.Errorf("WriteArtifacts: %v", err)
			}
		})
	}
}
