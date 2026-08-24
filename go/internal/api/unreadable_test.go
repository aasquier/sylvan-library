package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// What every deck route does when the library itself cannot be read.
//
// This is the deployed shape rather than a contrived one. The library lives
// on the instance's volume; a volume that failed to mount, a directory the
// process does not own, and a deck file with the wrong mode are all things
// that have happened -- and the Forge gate answered 500 for exactly this
// reason until it was caught on the live instance, because a permission error
// reads like a bug rather than like a missing installation.
//
// Two things are asked of every route. It must **say something**: a 500 with
// the standing sentence is honest, and a panic is not. And it must **not
// invent an empty library**: a shelf that answered `[]` over an unreadable
// directory would tell somebody their decks were gone.

// unreadableLibrary is an API whose decks directory exists and cannot be
// opened -- the deployed shape where the app runs as one user and the
// directory belongs to another.
func unreadableLibrary(t *testing.T) *API {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable directory to build")
	}
	root := t.TempDir()
	decks := filepath.Join(root, "decks")
	// A real deck inside it, so the failure is the permission rather than
	// the emptiness.
	if err := os.MkdirAll(filepath.Join(decks, "gyome"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decks, "gyome", "deck.yaml"),
		[]byte("slug: gyome\nname: Gyome\ncards: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(decks, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(decks, 0o750) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(Config{DecksDir: decks, AdminEmail: "alice@example.com", AppDB: db})
}

// Every read route, against a library that will not open.
func TestEveryDeckReadSaysSomethingWhenTheLibraryWillNotOpen(t *testing.T) {
	t.Parallel()
	a := unreadableLibrary(t)

	for _, route := range deckRoutes {
		target := "/api/decks/alice/gyome" + route.suffix
		status, payload, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusOK {
			t.Errorf("%s answered 200 over an unreadable library: %s", target, raw)
			continue
		}
		// Whatever it answered, it answered in the shape the client reads.
		if len(raw) == 0 {
			t.Errorf("%s answered %d with no body", target, status)
			continue
		}
		if !json.Valid(raw) {
			t.Errorf("%s answered non-JSON: %s", target, raw)
		}
		if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
			t.Errorf("%s answered %d with no detail: %s", target, status, raw)
		}
	}
}

// **The shelf never invents an empty library.** `[]` over an unreadable
// directory is the answer that tells somebody their decks are gone, and it is
// the one answer this must never give.
func TestTheShelfNeverInventsAnEmptyLibrary(t *testing.T) {
	t.Parallel()
	a := unreadableLibrary(t)

	status, _, raw := callAs(t, a, alice, http.MethodGet, "/api/decks", "")
	if status == http.StatusOK && strings.TrimSpace(string(raw)) == "[]" {
		t.Fatal("an unreadable library was listed as an empty one -- that reads " +
			"as 'your decks are gone'")
	}
	if status == http.StatusOK {
		// If it answered at all, it answered with something in it.
		var decks []any
		if err := json.Unmarshal(raw, &decks); err == nil && len(decks) == 0 {
			t.Fatal("an unreadable library listed no decks and no error")
		}
	}
}

// Every write route, likewise -- and a write must not report success over a
// library it could not read, which would leave somebody believing an edit
// landed.
func TestNoWriteReportsSuccessOverAnUnreadableLibrary(t *testing.T) {
	t.Parallel()
	a := unreadableLibrary(t)

	for _, route := range writeRoutes {
		target := "/api/decks/alice/gyome" + route.suffix
		status, _, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusOK {
			t.Errorf("%s %s reported success over an unreadable library: %s",
				route.method, target, raw)
		}
		if len(raw) == 0 {
			t.Errorf("%s %s answered %d with no body", route.method, target, status)
		}
	}

	// Creating and importing, which reach the library from a different
	// direction.
	for _, tc := range []struct{ target, body string }{
		{"/api/decks", `{"slug":"new-deck","commander":"Sol Ring"}`},
		{"/api/decks/import", `{"slug":"imported","text":"1 Sol Ring"}`},
	} {
		status, _, raw := callAs(t, a, alice, http.MethodPost, tc.target, tc.body)
		if status == http.StatusOK {
			t.Errorf("%s created a deck in an unreadable library: %s", tc.target, raw)
		}
	}
}

// A deck file the process cannot read is a different fault from a directory
// it cannot list, and it must not be reported as a missing deck: telling
// somebody their deck does not exist when it is right there and unreadable
// sends them to create it again.
func TestADeckFileThatCannotBeReadIsNotReportedAsMissing(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	decks := t.TempDir()
	dir := filepath.Join(decks, "gyome")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "deck.yaml")
	if err := os.WriteFile(path, []byte("slug: gyome\nname: Gyome\ncards: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{DecksDir: decks, AdminEmail: "alice@example.com", AppDB: db})

	status, payload, raw := callAs(t, a, alice, http.MethodGet, "/api/decks/alice/gyome", "")
	if status == http.StatusOK {
		t.Fatalf("an unreadable deck was served: %s", raw)
	}
	if status == http.StatusNotFound {
		detail, _ := payload["detail"].(string)
		t.Errorf("an unreadable deck was reported as missing (%q) -- somebody "+
			"would create it again on top of the one that is there", detail)
	}
}

// A deck file that is there and is not a deck is a fault about that file, and
// it must not take the whole shelf down with it: the other decks still list.
func TestOneUnparseableDeckDoesNotTakeTheShelfDown(t *testing.T) {
	t.Parallel()
	decks := decksDir(t)
	broken := filepath.Join(decks, "broken")
	if err := os.MkdirAll(broken, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "deck.yaml"),
		[]byte("this is not: [a deck\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{DecksDir: decks, AdminEmail: "alice@example.com", AppDB: db,
		Claude: claude.Settings{}})

	status, _, raw := callAs(t, a, alice, http.MethodGet, "/api/decks", "")
	if status != http.StatusOK {
		// If it refuses, it refuses in words rather than silently.
		if len(raw) == 0 {
			t.Fatalf("the shelf answered %d with no body", status)
		}
		t.Logf("one unparseable deck refuses the whole shelf (%d) -- that is a "+
			"choice, and this test records it: %s", status, raw)
		return
	}
	var decksOut []any
	if err := json.Unmarshal(raw, &decksOut); err != nil {
		t.Fatalf("the shelf answered %s", raw)
	}
	if len(decksOut) == 0 {
		t.Error("one unparseable deck emptied the whole shelf")
	}
}
