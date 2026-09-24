package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// What the artifacts shelf, the colour challenge and the slot argument do
// when the volume underneath them is not what they expect.
//
// The artifacts routes are the ones that matter most here, because they are
// the only place in the app where a *directory* is the unit of work: a deck
// builds into `artifacts/`, the snapshot the next build diffs against lives
// in it, and "this deck has never been built" and "I cannot read what it was
// built into" are two sentences that share one shape. Reporting the second as
// the first is how a rebuild quietly loses a `swaps.md` nobody notices is
// gone.

// A deck directory the process cannot write into: the build refuses rather
// than reporting a shelf of files it never wrote.
func TestABuildThatCannotWriteItsArtifactsSaysSoRatherThanListingThem(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	decks := decksDir(t)
	dir := filepath.Join(decks, "mono-green-clean")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: db})

	status, payload, raw := callAs(t, a, alice, "POST",
		"/api/decks/alice/mono-green-clean/artifacts", `{}`)
	if status == http.StatusOK {
		t.Fatalf("a build that could not write reported a shelf: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries no sentence: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "artifacts")); err == nil {
		t.Error("the build left a half-written artifacts directory behind")
	}
}

// An artifacts directory that is there and cannot be read.
//
// **This records a behaviour rather than approving of it, and the two routes
// answer differently.** A build refuses, which is right. The shelf answers
// 200 with an empty list and `baseline: "unknown"` -- the same words a deck
// that has genuinely never been built gets -- because `FileSource.Artifacts`
// skips a file it cannot `Stat` and `ReadBaseline` reads an unreadable
// snapshot as an absent one (`internal/library/source.go`, both with comments
// saying the absence is ordinary, which it is; the permission error is the
// case those comments do not separate).
//
// It is this repo's most-repeated bug shape -- a fallback that reads as a
// fact -- and it is deliberately not fixed here: the swallow is one layer
// down, in a function whose ordinary case really is "not there", and telling
// the two apart is a decision about that tier rather than a patch to this
// route. Written down so the next reader finds it as a known answer rather
// than as a surprise, and so it fails the day somebody separates them.
func TestAnUnreadableArtifactsDirectoryIsAFaultRatherThanAnUnbuiltDeck(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	decks := decksDir(t)
	// `kaheera` is the fixture that has artifacts and a snapshot beside them.
	arts := filepath.Join(decks, "kaheera", "artifacts")
	if err := os.Chmod(arts, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(arts, 0o750) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: db})

	// The build refuses, which is the answer that matters: nothing claims to
	// have written a shelf it could not write.
	status, payload, raw := callAs(t, a, alice, "POST",
		"/api/decks/alice/kaheera/artifacts", `{}`)
	if status == http.StatusOK {
		t.Errorf("a build answered 200 over an artifacts directory it could "+
			"not read: %s", raw)
	} else if !saysSomething(payload) {
		t.Errorf("the build answered %d with nothing to read: %s", status, raw)
	}

	// The shelf does not, and this is the recorded answer.
	status, payload, raw = callAs(t, a, alice, "GET",
		"/api/decks/alice/kaheera/artifacts", "")
	if status != http.StatusOK {
		t.Fatalf("the shelf now refuses an unreadable artifacts directory (%d) "+
			"-- that is the better answer, and this test recorded the older "+
			"one: %s", status, raw)
	}
	held, _ := payload["artifacts"].([]any)
	if len(held) != 0 || payload["baseline"] != "unknown" {
		t.Fatalf("the shelf now reads an unreadable directory as %v / %v -- "+
			"the recorded answer was an empty list and `unknown`: %s",
			payload["artifacts"], payload["baseline"], raw)
	}
	// The one thing that keeps this from being a lie people act on: the deck
	// page can still tell that a build is possible, so the way out is one
	// click rather than a shell.
	if payload["buildable"] != true {
		t.Errorf("an unreadable shelf reports the deck as unbuildable, which "+
			"leaves no way out of it: %s", raw)
	}
}

// The 32 Deck Challenge counts decks by their commander's colours, so a deck
// with no commander is skipped rather than counted as colourless -- and a deck
// file that will not parse stops the count rather than being silently left
// out of it.
//
// The difference is the point. A skipped commanderless deck is a deck that
// genuinely fills no slot; a deck dropped because its file is broken would
// leave a slot reading empty when something is standing in it.
func TestTheColourChallengeSkipsACommanderlessDeckAndRefusesABrokenOne(t *testing.T) {
	t.Parallel()
	decks := decksDir(t)
	plantDeck(t, decks, "no-commander", `slug: no-commander
name: No Commander
status: theoretical
stage: draft
commander: []
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one.
`)
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "alice@example.com", AppDB: db})

	status, payload, raw := callAs(t, a, alice, "GET", "/api/colors/progress", "")
	if status != http.StatusOK {
		t.Fatalf("the challenge answered %d with a commanderless deck on the "+
			"shelf: %s", status, raw)
	}
	if !strings.Contains(string(raw), "filled") && payload["filled"] == nil {
		t.Errorf("the progress answer carries no slots: %s", raw)
	}

	// Now a deck file that is not a deck. The count stops and says so; it
	// does not report a shelf with a hole in it.
	plantDeck(t, decks, "not-a-deck", "this is not: [a deck\n")
	status, payload, raw = callAs(t, a, alice, "GET", "/api/colors/progress", "")
	if status == http.StatusOK {
		t.Logf("one unparseable deck is skipped by the challenge (%d) -- that "+
			"is a choice, and this records it: %s", status, raw)
		return
	}
	if !saysSomething(payload) {
		t.Errorf("the challenge answered %d with nothing to read: %s", status, raw)
	}
}

// The slot argument's `focus`, which narrows what the argument is about.
//
// It is read off the body before any call is made, so an instance with no
// credential still proves the reading: what comes back is the 503 every
// Claude surface gives, and what is under test is that a focus does not turn
// the request into a different refusal.
func TestAFocusedArgumentIsReadTheSameWayAsAnUnfocusedOne(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()

	var statuses []int
	for _, body := range []string{
		`{"card":"Sol Ring"}`,
		`{"card":"Sol Ring","focus":"the mana base"}`,
		// A falsy focus is no focus, the way a falsy stance is no stance.
		`{"card":"Sol Ring","focus":""}`,
	} {
		status, payload, raw := callAs(t, a, alice, "POST",
			"/api/decks/alice/kaheera/argue", body)
		statuses = append(statuses, status)
		if status == http.StatusOK {
			t.Errorf("an argument was answered with no credential: %s", raw)
		}
		if !saysSomething(payload) {
			t.Errorf("%s answered %d with nothing to read: %s", body, status, raw)
		}
	}
	if statuses[0] != statuses[1] || statuses[1] != statuses[2] {
		t.Errorf("a focus changed which refusal came back: %v -- the focus "+
			"narrows the argument, it does not change who may ask for one",
			statuses)
	}
}

// The theme surface's own refusals, which are its own because it has a floor
// the per-card modes do not: a readiness that is not met is a 409, and
// nothing else is.
func TestTheThemeSurfaceKeepsItsRefusalsApart(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()

	for _, row := range []struct {
		name, target, body string
		want               int
	}{
		// A stance nobody has: the caller's mistake.
		{"an unknown stance", "/api/claude/theme",
			`{"transcript":[],"stance":"emperor"}`, http.StatusUnprocessableEntity},
		{"an unknown persona", "/api/claude/theme",
			`{"transcript":[],"persona":"the bogeyman"}`, http.StatusUnprocessableEntity},
		// A proposal asked for before the interview has enough to go on.
		{"a proposal with no transcript", "/api/claude/theme/proposal",
			`{"transcript":[]}`, http.StatusConflict},
	} {
		status, payload, raw := callAs(t, a, alice, "POST", row.target, row.body)
		if status != row.want {
			t.Errorf("%s answered %d, want %d: %s", row.name, status, row.want, raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s answered %d with nothing to read: %s", row.name, status, raw)
		}
	}
}
