package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// `GET /api/coliseum` with a card pool behind it.
//
// The arena room's own tests are all about the *feats* board beside it
// (`coliseumfeats_test.go`); the six arenas themselves were only ever asked of
// an instance with no pool, where the whole lookup is skipped and the prose
// answers alone. So the half of the handler that resolves a few dozen card
// names — one query for the whole file rather than one per arena — had never
// run from a test, and neither had the count it keeps.
//
// **The counting is the thing worth pinning.** This route's contract is the
// reference-prose rule: the text is checked in and finite, the card facts are
// the pool's, and a name the pool has never heard of is **dropped and
// counted** rather than guessed at. A `dropped` that stopped counting would
// look exactly like a pool that answers everything, which is the one state
// nobody would go looking for.

func coliseumRoom(t *testing.T, a *API) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	a.coliseum(rec, httptest.NewRequest(http.MethodGet, "/api/coliseum", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("the arena room answered %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the room is not JSON: %v", err)
	}
	return body
}

// With a pool, the room says so and counts what it could not find.
//
// The fixture pool is twenty-one cards and none of them is an arena or a
// champion, so every name is dropped — which is the case the count exists for
// and the one a real instance sees whenever a new arena is written before the
// library is refreshed. The prose still answers whole, because the facts are
// the point and they are all text.
func TestTheArenaRoomCountsEveryNameThePoolCouldNotAnswer(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	body := coliseumRoom(t, a)

	if body["pool"] != true {
		t.Fatalf("a room with a pool behind it reports pool=%v", body["pool"])
	}
	arenas, _ := body["arenas"].([]any)
	if len(arenas) < 6 {
		t.Fatalf("the room holds %d arenas", len(arenas))
	}
	dropped, _ := body["dropped"].(float64)
	if dropped <= 0 {
		t.Fatalf("every name in the file resolved against a twenty-one card "+
			"pool, which cannot be true -- dropped reads %v", dropped)
	}
	// The count is the names asked for, not a constant: it is at least one
	// per arena (each names its own backdrop) plus every champion.
	champions := 0
	for _, raw := range arenas {
		arena, _ := raw.(map[string]any)
		list, _ := arena["champions"].([]any)
		champions += len(list)
		// A name the pool could not answer leaves the arena with no backdrop,
		// which the frontend renders as its palette alone -- a legible state,
		// and the reason this is a 200 rather than a refusal.
		if arena["backdrop"] != nil {
			t.Errorf("%v resolved a backdrop out of the fixture pool", arena["key"])
		}
		if arena["palette"] == nil || arena["motion"] == nil || arena["art"] == nil {
			t.Errorf("%v lost the prose it answers without a pool: %v",
				arena["key"], arena)
		}
		if facts, _ := arena["facts"].([]any); len(facts) == 0 {
			t.Errorf("%v has no facts to rotate", arena["key"])
		}
	}
	if champions != 0 {
		t.Errorf("%d champions resolved out of the fixture pool", champions)
	}
	if int(dropped) <= len(arenas) {
		t.Errorf("dropped reads %v for %d arenas that each name a backdrop and "+
			"champions besides -- the count has stopped counting champions",
			dropped, len(arenas))
	}

	// The zones are set before the pool is opened and need none at all, so
	// they are the same either way.
	if zones, _ := body["zones"].([]any); len(zones) == 0 {
		t.Error("the board's own zones went missing")
	}
}

// With no pool, the prose answers whole and nothing is reported as dropped:
// the paintings are missing, and a count of what was not looked for would
// read as a library that is failing rather than one that has not arrived.
func TestWithNoPoolTheArenaRoomDropsNothingBecauseItAsksNothing(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	body := coliseumRoom(t, a)

	if body["pool"] != false {
		t.Errorf("a room with no pool reports pool=%v", body["pool"])
	}
	if dropped, _ := body["dropped"].(float64); dropped != 0 {
		t.Errorf("a room that asked nothing reports %v dropped", dropped)
	}
	arenas, _ := body["arenas"].([]any)
	if len(arenas) < 6 {
		t.Fatalf("the prose thinned to %d arenas without a pool", len(arenas))
	}
	if zones, _ := body["zones"].([]any); len(zones) == 0 {
		t.Error("the zones need no pool and went missing anyway")
	}
}

// A pool that opens and then fails its query is not a pool that is absent, and
// the room must not fold the two together: an arena drawn with no paintings
// and `pool: true` would say the library answered when it did not.
func TestAPoolThatWillNotAnswerIsNotAnArenaRoomWithNoPool(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: schemalessPool(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := httptest.NewRecorder()
	a.coliseum(rec, httptest.NewRequest(http.MethodGet, "/api/coliseum", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("a pool that answered an error was served as a room: %s", rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal is not JSON: %s", rec.Body)
	}
	if detail, _ := body["detail"].(string); detail == "" {
		t.Fatalf("the refusal carries nothing a person could read: %s", rec.Body)
	}
}
