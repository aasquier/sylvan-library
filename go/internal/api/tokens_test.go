package api

import (
	"net/http"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The route's own edges. What a deck makes, and how a token's painting is
// chosen, belong to `internal/pool` and are held there; these pin the answers
// this route can give and the one it must never give.

func tokensAPI(t *testing.T) *API {
	t.Helper()
	return New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t)})
}

func plates(t *testing.T, body map[string]any, raw []byte) []map[string]any {
	t.Helper()
	listed, _ := body["tokens"].([]any)
	out := make([]map[string]any, 0, len(listed))
	for _, item := range listed {
		plate, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("a token came back as %T: %s", item, raw)
		}
		out = append(out, plate)
	}
	return out
}

// **The commanders are read with the 99.** `pair` declares Gyome, Master Chef
// and Ajani, Nacatl Pariah as its commanders and nothing else that makes
// anything, so a route that read only the 99 would answer this deck with an
// empty section -- and Gyome's Food is the whole reason anybody asked.
func TestACommandersTokensAreTheDecksTokens(t *testing.T) {
	t.Parallel()
	a := tokensAPI(t)
	status, body, raw := call(t, a, http.MethodGet, "/api/decks/local/pair/tokens", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool_available"] != true || body["read"] != true {
		t.Fatalf("a pool that can answer said it could not: %s", raw)
	}
	made := map[string]bool{}
	for _, plate := range plates(t, body, raw) {
		name, _ := plate["name"].(string)
		made[name] = true
	}
	if !made["Food"] || !made["Cat Warrior"] {
		t.Fatalf("got %v, want Gyome's Food and Ajani's Cat Warrior: %s", made, raw)
	}
}

// The plate: a name, what the token *is*, who painted it, and which of this
// deck's cards make it. The painting's fields are absent together or present
// together -- a picture credited to nobody would be rule 9 broken in the one
// place it is easiest to break.
func TestAPlateNamesItsTokenAndCreditsItsPainter(t *testing.T) {
	t.Parallel()
	a := tokensAPI(t)
	status, body, raw := call(t, a, http.MethodGet,
		"/api/decks/local/last-bit/tokens", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	var food map[string]any
	for _, plate := range plates(t, body, raw) {
		if plate["name"] == "Food" {
			food = plate
		}
	}
	if food == nil {
		t.Fatalf("Bag End Banquet makes Food and the deck was told otherwise: %s", raw)
	}
	if food["type_line"] != "Token Artifact — Food" {
		t.Errorf("Food's type line is %v", food["type_line"])
	}
	if food["artist"] != "Randy Gallegos" || food["set_code"] != "TELD" {
		t.Errorf("Food is credited to %v of %v, want the original printing",
			food["artist"], food["set_code"])
	}
	if food["image"] == nil || food["art_crop"] == nil {
		t.Errorf("Food has no picture: %v", food)
	}
	madeBy, _ := food["made_by"].([]any)
	if len(madeBy) != 1 || madeBy[0] != "Bag End Banquet" {
		t.Errorf("Food is credited to %v", madeBy)
	}
	for _, key := range []string{"name", "type_line", "image", "art_crop",
		"artist", "set_code", "set_name", "made_by"} {
		if _, ok := food[key]; !ok {
			t.Errorf("the plate has no %q: %s", key, raw)
		}
	}
}

// A token this pool has no printing of is named anyway, with no painting and
// nobody credited -- the plate with nothing on it, which is the contract
// `internal/pool` keeps for exactly this.
func TestATokenWithNoPrintingHereIsStillNamed(t *testing.T) {
	t.Parallel()
	a := tokensAPI(t)
	status, body, raw := call(t, a, http.MethodGet,
		"/api/decks/local/last-bit/tokens", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	var elephant map[string]any
	for _, plate := range plates(t, body, raw) {
		if plate["name"] == "Elephant" {
			elephant = plate
		}
	}
	if elephant == nil {
		t.Fatalf("Terastodon's Elephant was dropped: %s", raw)
	}
	if elephant["image"] != nil || elephant["artist"] != nil {
		t.Errorf("the Elephant found a painting this pool does not have: %v", elephant)
	}
	if elephant["type_line"] != "Token Creature — Elephant" {
		t.Errorf("the Elephant is a %v", elephant["type_line"])
	}
}

// A deck whose cards make nothing is read and told so. This is the answer the
// **deploy window** must never be confused with: `read` stays true here,
// because the library did look.
func TestADeckThatMakesNothingIsReadAndSaysSo(t *testing.T) {
	t.Parallel()
	a := tokensAPI(t)
	status, body, raw := call(t, a, http.MethodGet,
		"/api/decks/local/mono-green-clean/tokens", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["read"] != true {
		t.Fatalf("a filled library reported that it had not read: %s", raw)
	}
	if len(plates(t, body, raw)) != 0 {
		t.Fatalf("that deck makes %s", raw)
	}
}

// No pool at all is the degraded 200 every deck route answers with, so the
// page renders and says why it is empty rather than erroring. `read` is false
// there too: nothing was looked at.
func TestNoPoolIsADegradedTwoHundred(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: decksDir(t)})
	status, body, raw := call(t, a, http.MethodGet,
		"/api/decks/local/pair/tokens", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool_available"] != false || body["read"] != false {
		t.Fatalf("a machine with no library claimed one: %s", raw)
	}
	if len(plates(t, body, raw)) != 0 {
		t.Fatalf("it answered with tokens anyway: %s", raw)
	}
}

// A deck that is not there is a 404, not an empty section.
func TestAnUnknownDeckHasNoTokenSection(t *testing.T) {
	t.Parallel()
	a := tokensAPI(t)
	status, _, raw := call(t, a, http.MethodGet,
		"/api/decks/local/no-such-deck/tokens", "")
	if status != 404 {
		t.Fatalf("%d: %s", status, raw)
	}
}
