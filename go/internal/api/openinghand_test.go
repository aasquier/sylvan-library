package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/opening"
)

// The route's own edges. What a hand *is* -- the shuffle, the counting, the
// tapped land -- belongs to `internal/sim/opening` and is held there; these
// pin the three answers this route can give and the one thing it must never
// do.

func handAPI(t *testing.T) *API {
	t.Helper()
	return New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t)})
}

const handPath = "/api/decks/local/mono-green-clean/opening-hand"

// A press deals seven, with the counting and the sentence that says what kind
// of counting it is.
func TestAPressDealsSevenWithTheReadingBesideThem(t *testing.T) {
	t.Parallel()
	a := handAPI(t)
	status, body, raw := call(t, a, http.MethodPost, handPath, "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool_available"] != true {
		t.Fatalf("pool_available: %s", raw)
	}
	cards, _ := body["cards"].([]any)
	if len(cards) != opening.Size {
		t.Fatalf("dealt %d cards: %s", len(cards), raw)
	}
	reading, _ := body["reading"].(map[string]any)
	if reading == nil {
		t.Fatalf("no reading: %s", raw)
	}
	for _, key := range []string{"lands", "spells", "colors_covered",
		"colors_missing", "first_spell_turn", "castable_by_horizon", "horizon"} {
		if _, ok := reading[key]; !ok {
			t.Fatalf("reading has no %q: %s", key, raw)
		}
	}
	if body["caveat"] != opening.Caveat {
		t.Fatalf("caveat: %s", raw)
	}
}

// **The seed never crosses the wire, in either direction.** Aaron's ask was a
// player who presses deal; a seed is machinery, and machinery is not
// something a person who came here for cards is ever shown (commandment 10).
// A field named here would be a lever with nothing on the other end of it,
// so the guard is on the whole payload rather than on one key.
func TestNoSeedIsEverReported(t *testing.T) {
	t.Parallel()
	a := handAPI(t)
	status, body, raw := call(t, a, http.MethodPost, handPath, "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if _, ok := body["seed"]; ok {
		t.Fatalf("the deal reported a seed: %s", raw)
	}
	if strings.Contains(string(raw), "seed") {
		t.Fatalf("the word seed reached the wire: %s", raw)
	}
}

// A seed sent anyway is ignored rather than honoured: the route reads the
// body only far enough to refuse a malformed one, and no key in it reaches
// the shuffle. Two deals with the same "seed" are free to differ, and what is
// asserted is the weaker and truer thing -- that a body with a seed in it is
// still just a deal.
func TestASeedInTheBodyIsNotALever(t *testing.T) {
	t.Parallel()
	a := handAPI(t)
	status, body, raw := call(t, a, http.MethodPost, handPath, `{"seed":42}`)
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if cards, _ := body["cards"].([]any); len(cards) != opening.Size {
		t.Fatalf("dealt %d cards: %s", len(cards), raw)
	}
	if _, ok := body["seed"]; ok {
		t.Fatalf("the seed came back: %s", raw)
	}
}

// A body that is not JSON at all is the shared 422, not a 500: the route
// borrows `readOptionalBody`, which is the Wheel's contract too.
func TestAMalformedBodyIsTheSharedRefusal(t *testing.T) {
	t.Parallel()
	a := handAPI(t)
	status, _, raw := call(t, a, http.MethodPost, handPath, `{`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("%d: %s", status, raw)
	}
	if !json.Valid(raw) {
		t.Fatalf("non-JSON refusal: %s", raw)
	}
}

// No pool degrades rather than fails: the page renders, holds no cards, and
// says why. Byte for byte, because the client reads `pool_available` first.
func TestTheDealWithNoPoolIsTheDegradedShape(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: decksDir(t)})
	status, _, raw := call(t, a, http.MethodPost, handPath, "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	want := `{"pool_available":false,"cards":[],` +
		`"message":` + string(mustJSON(t, noPoolMessage)) + `}`
	if string(raw) != want {
		t.Fatalf("got %s\nwant %s", raw, want)
	}
}

// ADR 5: a deck this caller cannot see is 404 in the deck's own words, and
// the deal is no exception to it.
func TestAnUnknownDeckHasNoHandToDeal(t *testing.T) {
	t.Parallel()
	a := handAPI(t)
	status, body, raw := call(t, a, http.MethodPost,
		"/api/decks/local/nope/opening-hand", "")
	if status != http.StatusNotFound {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["detail"] != "no deck 'nope'" {
		t.Fatalf("detail: %s", raw)
	}
}
