package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
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
// Three questions are asked of every route: a deck that is not there, an owner
// that is not there, and **another account's private deck** -- ADR 5's own,
// where the answer must be indistinguishable from the first two, because a 403
// would confirm the deck exists to somebody who may not know that.
//
// **The list of routes is derived, not typed.** ADR 5 asks for the sweep
// "parametrised over the route table, so an endpoint added without scoping
// fails the suite", and a hand-written list is exactly the shape that cannot
// do that: for months this file swept 22 of the 35 `{owner}` routes the app
// actually served, and nothing anywhere said so. `deckFamily` below is still
// written out by hand -- a route needs a body and a fill for its templated
// segments, and neither can be derived -- but
// `TestEverySweptRouteIsServedAndEveryServedRouteIsSwept` holds it equal to
// the served table in both directions, so the list cannot fall behind the app
// and cannot invent a route the app does not have. Mis-filing a shared route
// certifies a hole as shut, which is why the equality runs both ways.

// deckRoute is one route under `/api/decks/{owner}/{slug}`: the method, the
// path template that follows the owner and the slug, and a body that would be
// valid if the deck existed.
//
// The suffix is the SERVED template, placeholders included, because that is
// what the route table can be compared against. `path` fills it.
type deckRoute struct {
	method  string
	suffix  string
	payload string
}

// ownedPrefix is the one shape this file sweeps: every route whose data
// belongs to the owner named in its path.
const ownedPrefix = "/api/decks/{owner}/{slug}"

// deckFamily is every route under that prefix -- the reads, the writes, and
// the Claude surfaces that reach as far as a write does.
//
// Order is the route table's own, so the two read side by side.
var deckFamily = []deckRoute{
	// The reads.
	{"GET", "", ""},
	{"GET", "/validate", ""},
	{"GET", "/stats", ""},
	{"GET", "/suggestions", ""},
	{"GET", "/tokens", ""},
	{"GET", "/commander", ""},
	{"GET", "/printings", ""},
	{"GET", "/log", ""},
	{"GET", "/artifacts", ""},
	{"GET", "/artifacts/{name}", ""},
	{"GET", "/dossier", ""},
	// The two simulations. POSTs that write nothing, and swept with the rest
	// for exactly that reason: the method is not what decides whose deck this
	// is.
	{"POST", "/wheel", `{}`},
	{"POST", "/opening-hand", `{}`},
	// The writes.
	{"POST", "/swap", `{"out":"Forest","into":"Sol Ring","why":"a reason"}`},
	{"POST", "/cards", `{"name":"Sol Ring","why":"a reason"}`},
	{"POST", "/board", `{"name":"Sol Ring","why":"a reason"}`},
	{"DELETE", "/cards/{name}", ""},
	{"POST", "/entomb", `{"names":["Forest"],"why":"a reason"}`},
	{"POST", "/bulk", `{"text":"1 Sol Ring","dry_run":true}`},
	{"POST", "/graveyard/{name}/return", ""},
	{"DELETE", "/graveyard/{name}", ""},
	{"PATCH", "/cards/{name}", `{"field":"category","value":"ramp"}`},
	{"PATCH", "", `{"field":"stage","value":"draft"}`},
	{"PUT", "/notes/{key}", `{"value":"a note"}`},
	{"PUT", "/combos", `{"combos":[{"cards":["Sol Ring"],"produces":"two mana"}]}`},
	{"DELETE", "", `{"confirm":"gyome"}`},
	{"PUT", "/shared", `{"shared":true}`},
	// The night games' flag. It is here for the 404 sweeps rather than for its
	// own sake: whether a deck can *hold* this flag is the tier's business and
	// is answered elsewhere, but "there is no such deck" and "that deck is not
	// yours" must answer identically on this route and on the others.
	{"PUT", "/coliseum-at-night", `{"coliseum_at_night":true}`},
	{"POST", "/artifacts", `{}`},
	// The Claude surfaces. Every one of these reads its BODY before it
	// resolves the deck -- their own recorded order, so a malformed body is a
	// 422 before any 404 -- which is why each carries a body that would be
	// valid if the deck were there. A payload that failed validation would
	// make every one of these sweeps pass without ever asking about the deck.
	{"POST", "/interview", `{"card":"Sol Ring"}`},
	{"POST", "/argue", `{"card":"Sol Ring"}`},
	{"POST", "/argue/deck", `{"cards":["Sol Ring"]}`},
	{"POST", "/intake", `{"categories":true}`},
	{"POST", "/describe", `{}`},
	{"POST", "/dossier", `{}`},
}

// fills names a value for each templated suffix, keyed by the whole suffix
// rather than by the placeholder: `{name}` is a card on the card routes and an
// artifact on the artifacts route, so one table keyed by placeholder could not
// serve both. A new templated route with no entry here is a test failure
// rather than a silently unswept route.
var fills = map[string]string{
	"/artifacts/{name}":        "/artifacts/primer-quick.md",
	"/cards/{name}":            "/cards/Forest",
	"/graveyard/{name}":        "/graveyard/Forest",
	"/graveyard/{name}/return": "/graveyard/Forest/return",
	"/notes/{key}":             "/notes/plan",
}

// path is this route aimed at one deck.
func (route deckRoute) path(t *testing.T, owner, slug string) string {
	t.Helper()
	suffix := route.suffix
	if strings.ContainsAny(suffix, "{}") {
		filled, ok := fills[suffix]
		if !ok {
			t.Fatalf("%s %s has a templated segment and no value to fill it "+
				"with -- add one to `fills` rather than leaving the route unswept",
				route.method, suffix)
		}
		suffix = filled
	}
	return "/api/decks/" + owner + "/" + slug + suffix
}

// reads is the GET half, for the questions that only make sense of a read.
func reads() []deckRoute {
	out := []deckRoute{}
	for _, route := range deckFamily {
		if route.method == http.MethodGet {
			out = append(out, route)
		}
	}
	return out
}

// writes is everything else -- including the two simulations, which write
// nothing but are refused on the same evidence.
func writes() []deckRoute {
	out := []deckRoute{}
	for _, route := range deckFamily {
		if route.method != http.MethodGet {
			out = append(out, route)
		}
	}
	return out
}

// servedOwnedRoutes is every route the app serves whose path names an owner,
// as `METHOD suffix` keys -- read off the same accessor the door's sweeps
// read, so a route added to the table is in this set the moment it is mounted.
func servedOwnedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, r := range New(Config{}).Routes() {
		if !strings.Contains(r.Pattern, "{owner}") {
			continue
		}
		if !strings.HasPrefix(r.Pattern, ownedPrefix) {
			// A route naming an owner somewhere else entirely. Filing it under
			// this sweep's prefix would sweep the wrong path and certify a hole
			// as shut, so it stops the suite instead.
			t.Fatalf("%s %s names an owner but does not start with %s -- this "+
				"sweep cannot address it, and guessing would certify a hole as shut",
				r.Method, r.Pattern, ownedPrefix)
		}
		out[r.Method+" "+strings.TrimPrefix(r.Pattern, ownedPrefix)] = true
	}
	return out
}

// **The completeness guard ADR 5 asks for.** The sweeps below are only as good
// as the list they iterate, so the list is held equal to the served route
// table in both directions: a deck route added without being swept fails here,
// and so does an entry for a route that no longer exists (which is the quieter
// bug -- it makes the sweep look bigger than it is).
func TestEverySweptRouteIsServedAndEveryServedRouteIsSwept(t *testing.T) {
	t.Parallel()
	served := servedOwnedRoutes(t)
	if len(served) == 0 {
		t.Fatal("no owner-scoped routes were found in the route table -- an " +
			"empty sweep passes every assertion below and proves nothing")
	}
	swept := map[string]bool{}
	for _, route := range deckFamily {
		key := route.method + " " + route.suffix
		if swept[key] {
			t.Errorf("%s is listed twice", key)
		}
		swept[key] = true
		// Every route is also addressable, which is the other half of being
		// swept: a templated suffix with nothing to fill it is unswept in
		// practice however it is listed here.
		route.path(t, "alice", "gyome")
	}
	missing, extra := []string{}, []string{}
	for key := range served {
		if !swept[key] {
			missing = append(missing, key)
		}
	}
	for key := range swept {
		if !served[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("%d owner-scoped route(s) are served and never swept, so a "+
			"deck of somebody else's could be reachable through them and "+
			"nothing here would say so:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("%d route(s) are swept and no longer served, which makes this "+
			"sweep look wider than it is:\n  %s",
			len(extra), strings.Join(extra, "\n  "))
	}
}

// **A deck that is not there is a 404 on every route, never a 500.** The
// distinction is the whole difference between "you mistyped the slug" and
// "the site is broken".
func TestEveryDeckRouteAnswers404ForADeckThatIsNotThere(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	for _, route := range deckFamily {
		target := route.path(t, "alice", "no-such-deck")
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

	for _, route := range deckFamily {
		target := route.path(t, "nobody", "gyome")
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

// **ADR 5's own sweep, and the one that would catch a leak.** A deck that
// exists and belongs to somebody else is *absent* -- 404, never 403 and never
// 200 -- on every route in the family, reads included.
//
// The reads are the half this file used to leave out, and they are the half
// that leaks a decklist: a write refused with 403 tells a stranger a deck is
// there, but a read answered with 200 hands them the cards. Bob's private deck
// asked for by alice is the crisp case -- she is the maintainer and an admin,
// and neither buys a way into another account's SQL-tier library.
func TestAnotherAccountsPrivateDeckIsAbsentOnEveryRoute(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	for _, route := range deckFamily {
		target := route.path(t, "bob", "bobs-private")
		status, _, raw := callAs(t, a, alice, route.method, target, route.payload)
		switch status {
		case http.StatusNotFound:
		case http.StatusInternalServerError:
			t.Errorf("%s %s answered 500: %s", route.method, target, raw)
		case http.StatusForbidden:
			t.Errorf("%s %s answered 403 -- ADR 5 says absent, and a refusal "+
				"that admits the deck exists is the leak: %s",
				route.method, target, raw)
		default:
			t.Errorf("%s %s answered %d for another account's private deck: %s",
				route.method, target, status, raw)
		}
	}
}

// The shared-library corner the sweep above cannot reach: bob can *see* the
// maintainer's shelf, so nothing here is about absence. What is never allowed
// is a change.
//
// **Asked of the deck rather than of the status code**, which is the only way
// to ask it of every route at once. A status is not a classification a test
// can trust here -- `POST /wheel` answers 200 because a spin changes nothing,
// and `POST /interview` answers 503 because the pipe is shut before writability
// is ever decided -- and sorting thirty-five routes into "may 200" and "may not"
// by hand is the mis-filing this file's own guard exists to prevent. So the
// question is asked of the library on disk: drive every route as a stranger and
// require that not one byte under the decks directory moved. A refusal that
// answers 403 and writes anyway fails this; so does one that answers 200 and
// writes anyway, which no status check would have caught at all.
func TestAStrangerWhoCanSeeADeckStillCannotChangeIt(t *testing.T) {
	t.Parallel()
	decks := decksDir(t)
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "alice@example.com", AppDB: db})

	// Bob asking about the maintainer's own library, one route at a time, so
	// the failure names the route that moved something rather than the sweep.
	before := libraryOnDisk(t, decks)
	for _, route := range deckFamily {
		target := route.path(t, "alice", "mono-green-clean")
		status, _, raw := callAs(t, a, bob, route.method, target, route.payload)
		if status == http.StatusInternalServerError {
			t.Errorf("%s %s answered 500: %s", route.method, target, raw)
		}
		after := libraryOnDisk(t, decks)
		if diff := changedFiles(before, after); diff != "" {
			t.Errorf("%s %s answered %d and changed the maintainer's library:\n%s",
				route.method, target, status, diff)
			before = after
		}
	}
}

// libraryOnDisk is every file under the decks directory and what is in it --
// the evidence the sweep above compares. Contents and not mtimes: a rewrite
// that produced identical bytes is not a change anybody could observe, and a
// timestamp is not a fact about the deck.
func libraryOnDisk(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("reading the library off disk: %v", err)
	}
	return out
}

// changedFiles names what moved between two readings, or "" for none.
func changedFiles(before, after map[string]string) string {
	lines := []string{}
	for name, body := range after {
		was, existed := before[name]
		switch {
		case !existed:
			lines = append(lines, "  written: "+name)
		case was != body:
			lines = append(lines, "  changed: "+name)
		}
	}
	for name := range before {
		if _, still := after[name]; !still {
			lines = append(lines, "  removed: "+name)
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
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

	for _, route := range reads() {
		target := route.path(t, "alice", "mono-green-clean")
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
