package api

import (
	"database/sql"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The faults the deck routes answer that no fixture library produces on its
// own: a directory where a deck file belongs, a shelf the process may not read,
// a snapshot that will not parse, a value no encoder will take.

// A value that cannot be rendered is a refusal in the library's own words, not
// a 200 carrying an empty body. `rawOrdered` is the one place that branch lives
// now (see its comment in `decks.go`), and this is the only way into it.
func TestAnOrderedBodyThatWillNotRenderIsRefusedRatherThanSentEmpty(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	rec := httptest.NewRecorder()
	// Not a value any route builds -- it is the one shape JSON has no spelling
	// for, which is what makes it the probe for this arm.
	a.rawOrdered(rec, "deck", []wire.KV{{Key: "odds", Value: math.NaN()}})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("a body that will not render answered %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "could not answer") {
		t.Errorf("the refusal reads %q", rec.Body.String())
	}

	// And the ordinary path still answers the bytes, in order.
	rec = httptest.NewRecorder()
	a.rawOrdered(rec, "deck", []wire.KV{{Key: "b", Value: 1}, {Key: "a", Value: 2}})
	if rec.Code != http.StatusOK || rec.Body.String() != `{"b":1,"a":2}` {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

// A directory where a deck file belongs.
//
// The shelf lists a slug by finding a **regular** `<slug>/deck.yaml`, so a
// directory of that name is invisible to it -- while the create's own write
// opens the same path with O_EXCL and is told it is already there. That is the
// gap between the look and the write that the create's collision branch exists
// for, and it is reachable without a race: one half-made directory on the
// volume, which is what a failed write or an interrupted restore leaves.
//
// Both routes that make a deck are asked, because they refuse in two different
// places -- the create at the route, the import from inside the pool lease --
// and the sentence a person reads has to be the same either way.
func TestCreatingOverAHalfMadeDirectoryIsRefusedByName(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, tc := range []struct {
		name, slug, target, body string
	}{
		{"create", "collided", "/api/decks",
			`{"slug":"collided","commander":"Goreclaw, Terror of Qal Sisma"}`},
		{"import", "collided-import", "/api/decks/import",
			`{"slug":"collided-import","commander":["Goreclaw, Terror of Qal Sisma"],"text":"1 Sol Ring"}`},
	} {
		// `deck.yaml` as a directory: not a deck to the shelf, and already
		// there to anything that tries to create one.
		if err := os.MkdirAll(filepath.Join(rig.decks, tc.slug, "deck.yaml"), 0o755); err != nil {
			t.Fatal(err)
		}
		status, payload, raw := rig.do(t, alice, "POST", tc.target, tc.body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s over a half-made directory answered %d: %s", tc.name, status, raw)
			continue
		}
		detail := fmtDetail(payload)
		if !strings.Contains(detail, "already exists") || !strings.Contains(detail, tc.slug) {
			t.Errorf("%s: the refusal reads %q -- it has to name the slug", tc.name, detail)
		}
	}
}

// The colour challenge, scored twice: once on a machine with no card pool, and
// once on a library that will not open.
//
// It is the one route that scores the *instance's* file tier rather than the
// caller's library, so neither fault arrives through a source the caller named.
// Without a pool there is no colour identity to score with and the honest answer
// is the empty board with `pool: false` -- never a board claiming thirty-two
// unfilled slots as a fact about the library.
func TestTheColourChallengeAnswersWithoutAPoolAndRefusesAnUnreadableShelf(t *testing.T) {
	t.Parallel()

	noPool := New(Config{DecksDir: decksDir(t), AdminEmail: "alice@example.com"})
	status, payload, raw := as(t, noPool, alice, "/api/colors/progress")
	if status != http.StatusOK {
		t.Fatalf("a machine with no pool answered %d: %s", status, raw)
	}
	if payload["pool"] != false {
		t.Errorf("it claims a pool: %s", raw)
	}
	if payload["filled"] != float64(0) {
		t.Errorf("it scored %v slots with nothing to score them from", payload["filled"])
	}
	if slots, _ := payload["slots"].([]any); len(slots) == 0 {
		t.Error("the board is empty of slots entirely")
	}

	// And the other way round: a pool that answers and a library that does not.
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable directory to build")
	}
	decks := filepath.Join(t.TempDir(), "decks")
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
	shut := New(Config{DecksDir: decks, Pool: pooltest.Open(t), AdminEmail: "alice@example.com"})
	status, payload, raw = as(t, shut, alice, "/api/colors/progress")
	if status != http.StatusInternalServerError {
		t.Fatalf("an unreadable library was scored anyway: %d %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
}

// A quantity that is not a number, on the route that starts a swap board.
//
// The board's own route reads the body itself rather than sharing `/cards`'s,
// so its coercion refusal is its own too -- and a 422 naming the field is what
// sends somebody to fix their request rather than to wonder about their deck.
func TestStartingABoardWithAQuantityThatIsNotANumberIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.text(t)
	status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/board",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"the finisher","qty":"a few"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a quantity spelled in words answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "qty must be a number") {
		t.Errorf("the refusal reads %q", detail)
	}
	if rig.text(t) != before {
		t.Error("the refused add wrote a board anyway")
	}
}

// A simulation aimed at an owner who is not there.
//
// Every simulation names its deck in the body rather than in the path, so the
// owner segment every other deck route resolves is resolved here too -- and a
// segment nobody answers to has to be the same 404 a deck that is not yours
// gets (ADR 5), never a 500 and never a run over somebody else's deck.
func TestASimulationAimedAtAnOwnerNobodyAnswersToIsAFourOhFour(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()

	for _, target := range []string{"/api/sim/mana", "/api/sim/lands", "/api/sim/shelf", "/api/sim/policy"} {
		status, payload, raw := callAs(t, a, alice, "POST", target,
			`{"slug":"kaheera","owner":"nobody-at-all","games":200}`)
		if status != http.StatusNotFound {
			t.Errorf("%s answered %d for an owner nobody answers to: %s", target, status, raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s answered 404 with nothing a person could read: %s", target, raw)
		}
	}
}

// The shelf's file server, asked for the two things that are not a file.
//
// Both callers stat for a regular file before they hand a path over, so neither
// fault arrives through a route -- which is exactly why the guard is worth
// driving directly: the function's contract is that it reports whether it
// served, and a cache directory where a `.svg` has become a directory, or where
// the file was swept between the look and the open, is a real state of a volume.
// A `true` over either would mean a 200 with no body.
func TestTheShelfFileServerReportsWhatItCouldNotServe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	absent := filepath.Join(dir, "never-written.svg")
	rec := httptest.NewRecorder()
	if serveShelfFile(rec, httptest.NewRequest("GET", "/api/symbols/W.svg", nil),
		absent, "image/svg+xml", "public") {
		t.Error("a path with no file behind it was reported as served")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("it wrote %q", rec.Body)
	}

	asDir := filepath.Join(dir, "loop.webm")
	if err := os.MkdirAll(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	if serveShelfFile(rec, httptest.NewRequest("GET", "/api/art/motion/x/y/loop.webm", nil),
		asDir, "video/webm", "public") {
		t.Error("a directory was reported as served")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("it wrote %q", rec.Body)
	}

	// And the file that is a file still serves, with the media type and the
	// cache policy it was handed.
	real := filepath.Join(dir, "W.svg")
	if err := os.WriteFile(real, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	if !serveShelfFile(rec, httptest.NewRequest("GET", "/api/symbols/W.svg", nil),
		real, "image/svg+xml", "public, max-age=604800") {
		t.Fatal("a real file was not served")
	}
	if rec.Header().Get("Content-Type") != "image/svg+xml" ||
		rec.Header().Get("Cache-Control") != "public, max-age=604800" {
		t.Errorf("the headers are %v", rec.Header())
	}
	if rec.Body.String() != "<svg/>" {
		t.Errorf("it served %q", rec.Body)
	}
}

// An artifacts shelf the process cannot see, and a snapshot that will not parse.
//
// `kaheera` is the fixture with a built shelf beside it, so a mode change on
// that one directory separates two answers that used to be the same one: "you
// have no artifacts", which is false, from "I cannot read your artifacts". The
// snapshot is the other half -- a `deck.last-built.yaml` nothing can parse is
// deliberately **no** baseline rather than an error, because the next build
// replaces the one file a rebuild is about.
func TestAnUnreadableArtifactShelfIsRefusedAndAnUnparseableSnapshotIsNoBaseline(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable directory to build")
	}
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	const built = "/api/decks/alice/kaheera"
	arts := filepath.Join(rig.decks, "kaheera", "artifacts")

	// First the honest answer over a readable shelf, so the refusal below is
	// known to be the mode rather than the fixture.
	if status, payload, raw := rig.do(t, alice, "GET", built+"/artifacts", ""); status != http.StatusOK {
		t.Fatalf("the shelf answered %d: %s", status, raw)
	} else if payload["baseline"] != "current" {
		t.Fatalf("the fixture's baseline reads %v", payload["baseline"])
	}

	if err := os.Chmod(arts, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(arts, 0o755) })
	for _, tc := range []struct{ name, method, body string }{
		{"the shelf", "GET", ""},
		{"a rebuild", "POST", `{"force":true}`},
	} {
		status, payload, raw := rig.do(t, alice, tc.method, built+"/artifacts", tc.body)
		if status != http.StatusInternalServerError {
			t.Errorf("%s over an unreadable shelf answered %d: %s", tc.name, status, raw)
			continue
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
			t.Errorf("%s: the refusal reads %q", tc.name, detail)
		}
	}
	if err := os.Chmod(arts, 0o755); err != nil {
		t.Fatal(err)
	}

	// The snapshot that parses as nothing: a shelf that still lists its files
	// and a baseline of `unknown` rather than a refusal.
	if err := os.WriteFile(filepath.Join(arts, "deck.last-built.yaml"),
		[]byte("cards: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, payload, raw := rig.do(t, alice, "GET", built+"/artifacts", "")
	if status != http.StatusOK {
		t.Fatalf("an unparseable snapshot answered %d: %s", status, raw)
	}
	if payload["baseline"] != "unknown" {
		t.Errorf("the baseline reads %v, want unknown", payload["baseline"])
	}
	if list, _ := payload["artifacts"].([]any); len(list) == 0 {
		t.Error("the shelf lost its files over a snapshot it could not read")
	}
}

// A deck whose history cannot be read is a refusal rather than an empty one.
//
// The activity log is a table in `app.db`, and a schema older than the binary
// is the deployed shape for this (ADR 23: merging deploys, the ladder applies on
// boot). An empty list over a table that is not there would read as "nothing has
// ever happened to this deck", which is the one answer that is false.
func TestADeckHistoryThatCannotBeReadIsRefusedRatherThanEmptied(t *testing.T) {
	t.Parallel()
	path := appDB(t)
	write, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.Exec(`DROP TABLE deck_log`); err != nil {
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := auth.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	a := New(Config{DecksDir: decksDir(t), Pool: pooltest.Open(t),
		AdminEmail: "alice@example.com", AppDB: db})

	status, payload, raw := as(t, a, alice, "/api/decks/alice/kaheera/log")
	if status != http.StatusInternalServerError {
		t.Fatalf("a history that cannot be read answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
	// The deck itself still reads: the fault is the history's, and saying so is
	// the difference between a broken page and a broken panel on a page.
	if status, _, raw := as(t, a, alice, "/api/decks/alice/kaheera"); status != http.StatusOK {
		t.Fatalf("the deck answered %d: %s", status, raw)
	}
}
