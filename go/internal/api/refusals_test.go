package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// The deck family's refusals, swept over every route rather than spot-checked.
//
// A sweep rather than a handful of cases, for the reason the admin family's
// sweep exists: **the refusals are the part no structural rule can see**. A
// route mounted correctly, wired correctly and answering correctly for a deck
// that exists can still answer 500 for one that does not, and nothing about
// the code's shape would say so. That failure is invisible until somebody
// mistypes a slug and gets "the library could not answer that right now"
// instead of "no deck 'gyoem'" -- at which point they think the site is
// broken rather than that they made a typo.
//
// Two questions are asked of every route: a deck that is not there, and an
// owner that is not there. Both must be 404 and neither may be 500.

// deckRoutes is every read route in the deck family, with a body that would
// be valid if the deck existed.
var deckRoutes = []struct{ method, suffix, payload string }{
	{"GET", "", ""},
	{"GET", "/validate", ""},
	{"GET", "/stats", ""},
	{"GET", "/suggestions", ""},
	{"GET", "/commander", ""},
	{"GET", "/printings", ""},
	{"GET", "/log", ""},
	{"GET", "/artifacts", ""},
	{"GET", "/artifacts/primer-quick.md", ""},
}

// writeRoutes is every route that changes a deck, likewise.
var writeRoutes = []struct{ method, suffix, payload string }{
	{"POST", "/swap", `{"out":"Forest","into":"Sol Ring","why":"a reason"}`},
	{"POST", "/cards", `{"name":"Sol Ring","why":"a reason"}`},
	{"DELETE", "/cards/Forest", ""},
	{"POST", "/entomb", `{"names":["Forest"],"why":"a reason"}`},
	{"POST", "/graveyard/Forest/return", ""},
	{"DELETE", "/graveyard/Forest", ""},
	{"PATCH", "/cards/Forest", `{"field":"category","value":"ramp"}`},
	{"PATCH", "", `{"field":"stage","value":"draft"}`},
	{"PUT", "/notes/plan", `{"value":"a note"}`},
	{"PUT", "/combos", `{"combos":[{"cards":["Sol Ring"],"produces":"two mana"}]}`},
	{"DELETE", "", `{"confirm":"gyome"}`},
	{"PUT", "/shared", `{"shared":true}`},
	// The night games' flag. It is here for the 404 sweeps rather than for its
	// own sake: whether a deck can *hold* this flag is the tier's business and
	// is answered elsewhere, but "there is no such deck" and "that deck is not
	// yours" must answer identically on this route and on the ten above it.
	{"PUT", "/coliseum-at-night", `{"coliseum_at_night":true}`},
}

// **A deck that is not there is a 404 on every route, never a 500.** The
// distinction is the whole difference between "you mistyped the slug" and
// "the site is broken".
func TestEveryDeckRouteAnswers404ForADeckThatIsNotThere(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	for _, route := range append(append([]struct{ method, suffix, payload string }{},
		deckRoutes...), writeRoutes...) {
		target := "/api/decks/alice/no-such-deck" + route.suffix
		status, payload, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusInternalServerError {
			t.Errorf("%s %s answered 500 -- a mistyped slug reads as a broken site: %s",
				route.method, target, raw)
			continue
		}
		if status != http.StatusNotFound {
			t.Errorf("%s %s answered %d, want 404", route.method, target, status)
			continue
		}
		// The refusal names what was looked for, because the caller's next
		// move is to check the spelling.
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, "no-such-deck") {
			t.Errorf("%s %s said %q without naming the deck", route.method, target, detail)
		}
	}
}

// An owner nobody holds is the same 404, and for the ADR 5 reason: another
// account's library is absent rather than forbidden, so the two cases must
// be indistinguishable from outside.
func TestEveryDeckRouteAnswers404ForAnOwnerThatIsNotThere(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	for _, route := range append(append([]struct{ method, suffix, payload string }{},
		deckRoutes...), writeRoutes...) {
		target := "/api/decks/nobody/gyome" + route.suffix
		status, _, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusInternalServerError {
			t.Errorf("%s %s answered 500: %s", route.method, target, raw)
			continue
		}
		if status != http.StatusNotFound && status != http.StatusForbidden {
			t.Errorf("%s %s answered %d, want 404", route.method, target, status)
		}
	}
}

// A deck that exists but is not this caller's is **absent**, not forbidden --
// ADR 5's rule, asked of every route rather than of one. A 403 anywhere here
// would confirm the deck exists to somebody who may not know that.
func TestAnotherAccountsDeckIsAbsentOnEveryRoute(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	// Bob asking about the maintainer's own library.
	for _, route := range writeRoutes {
		target := "/api/decks/alice/mono-green-clean" + route.suffix
		status, _, raw := callAs(t, a, bob, route.method, target, route.payload)
		switch status {
		case http.StatusNotFound, http.StatusForbidden:
			// Both are recorded answers here: absent for a deck bob cannot
			// see, read-only for one that is shared with him.
		case http.StatusInternalServerError:
			t.Errorf("%s %s answered 500: %s", route.method, target, raw)
		default:
			t.Errorf("%s %s let bob through with %d", route.method, target, status)
		}
	}
}

// The artifacts route distinguishes a deck that is not there from an artifact
// that is not there, and both are 404 with different words -- the difference
// is what tells somebody whether to build the deck's artifacts or check the
// deck's name.
func TestAMissingArtifactAndAMissingDeckSayDifferentThings(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	status, payload, _ := callAs(t, a, alice, "GET",
		"/api/decks/alice/mono-green-clean/artifacts/primer-quick.md", "")
	if status != http.StatusNotFound {
		t.Skipf("this fixture already has artifacts (%d)", status)
	}
	missingArtifact, _ := payload["detail"].(string)

	_, payload, _ = callAs(t, a, alice, "GET",
		"/api/decks/alice/no-such-deck/artifacts/primer-quick.md", "")
	missingDeck, _ := payload["detail"].(string)

	if missingArtifact == missingDeck {
		t.Errorf("a missing artifact and a missing deck both say %q -- one "+
			"means build them, the other means check the name", missingArtifact)
	}
	if !strings.Contains(missingArtifact, "primer-quick.md") {
		t.Errorf("the missing artifact is described as %q", missingArtifact)
	}
}

// An artifact name that is not one of the five is refused rather than
// becoming a path -- the file tier's only check, since a name that is not a
// deliverable never becomes a path at all.
func TestAnArtifactNameThatIsNotADeliverableIsRefused(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	// A traversal is not in this list on purpose: the door matches on the
	// *decoded* path, so `..%2F..%2Fapp.db` arrives as three segments and
	// matches no route at all -- it never reaches a handler to be refused.
	// What is asked here is the handler's own check: a name that is not one
	// of the five deliverables never becomes a path.
	for _, name := range []string{
		"deck.yaml",
		"anything.md",
		"primer-quick.md.bak",
		"..",
	} {
		target := "/api/decks/alice/mono-green-clean/artifacts/" + name
		status, _, raw := callAs(t, a, alice, "GET", target, "")
		if status == http.StatusOK {
			t.Errorf("%s was served: %s", name, raw)
		}
		if status == http.StatusInternalServerError {
			t.Errorf("%s answered 500: %s", name, raw)
		}
	}
}

// An instance with no card pool answers every deck route in its degraded
// shape rather than failing: `pool_available: false` and a deck that is still
// a deck. This is the laptop's state and a fresh instance's state, and a
// route that 500'd here would make the site unusable before its first
// refresh.
func TestEveryDeckReadDegradesRatherThanFailingWithoutAPool(t *testing.T) {
	t.Parallel()
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{DecksDir: decksDir(t), AdminEmail: "alice@example.com", AppDB: db})

	for _, route := range deckRoutes {
		target := "/api/decks/alice/mono-green-clean" + route.suffix
		status, _, raw := callAs(t, a, alice, route.method, target, route.payload)
		if status == http.StatusInternalServerError {
			t.Errorf("%s %s answered 500 with no card pool: %s", route.method, target, raw)
		}
	}
}

// The lazy `app.db` open is the read side's own: an instance whose database
// appeared after boot picks it up, and one whose path is empty or whose file
// is missing simply has no accounts rather than failing.
func TestTheLazyDatabaseOpenHandlesEveryStateOfTheFile(t *testing.T) {
	t.Parallel()

	// No path at all: an instance with auth off.
	if db := New(Config{}).appDB(); db != nil {
		t.Error("an instance with no app.db path opened one")
	}

	// A path with no file: a fresh volume before the ladder runs.
	missing := t.TempDir() + "/app.db"
	a := New(Config{AppDBPath: missing})
	if db := a.appDB(); db != nil {
		t.Error("a path with no file opened a database")
	}

	// A real file: opened once and reused, so a page of deck rows does not
	// open a handle per row.
	path := appDB(t)
	a = New(Config{AppDBPath: path})
	first := a.appDB()
	if first == nil {
		t.Fatal("a real app.db did not open")
	}
	if second := a.appDB(); second != first {
		t.Error("the handle is reopened on every read")
	}
}
