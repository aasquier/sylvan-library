package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The route's payload parity lives in `internal/wheel`'s corpus; these pin
// the route's own edges -- the seed line, the degraded pool, ADR 5.

func wheelAPI(t *testing.T) *API {
	t.Helper()
	return New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t)})
}

// A seed the integer grammar refuses raises: an uncaught 500 -- the
// plain-text three words, measured on the live wire
// before this was written. The same wart, recorded rather than tidied.
func TestASeedTheGrammarRefusesIsTheRecordedUncaught500(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	for _, body := range []string{`{"seed":"abc"}`, `{"seed":[1]}`, `{"seed":{}}`} {
		status, _, raw := call(t, a, http.MethodPost,
			"/api/decks/local/mono-green-clean/wheel", body)
		if status != http.StatusInternalServerError {
			t.Fatalf("%s -> %d: %s", body, status, raw)
		}
		if string(raw) != "Internal Server Error" {
			t.Fatalf("%s -> body %q", body, raw)
		}
	}
}

// A float truncates and a fullwidth digit reads -- the recorded integer
// grammar, not `strconv`'s -- and the seed comes back as the number it
// became.
func TestTheSeedGoesThroughTheRecordedIntGrammar(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	for body, want := range map[string]string{
		`{"seed":3.9}`:   `"seed":3,`,
		`{"seed":"７"}`:   `"seed":7,`,
		`{"seed":"1_0"}`: `"seed":10,`,
	} {
		status, _, raw := call(t, a, http.MethodPost,
			"/api/decks/local/mono-green-clean/wheel", body)
		if status != 200 {
			t.Fatalf("%s -> %d: %s", body, status, raw)
		}
		if !strings.Contains(string(raw), want) {
			t.Fatalf("%s: %s not in %s", body, want, raw)
		}
	}
}

// No body at all is a fresh spin: the server rolls a seed and reports it.
func TestAnAbsentBodySpinsFresh(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	status, body, raw := call(t, a, http.MethodPost,
		"/api/decks/local/mono-green-clean/wheel", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool_available"] != true {
		t.Fatalf("pool_available: %s", raw)
	}
	if _, ok := body["seed"].(float64); !ok {
		if _, ok := body["seed"].(json.Number); !ok {
			t.Fatalf("no reported seed: %s", raw)
		}
	}
}

// No pool is the degraded shape, byte for byte, `pool_available` first.
func TestTheWheelWithNoPoolIsTheDegradedShape(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: decksDir(t)})
	status, _, raw := call(t, a, http.MethodPost,
		"/api/decks/local/mono-green-clean/wheel", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	// The shape is the assertion -- key order included, since it is the wire.
	// The sentence itself is read off the constant rather than copied here:
	// it is a sentence a *player* reads, so it changes when the copy changes,
	// and a literal in a test only ever rots into a second opinion about it.
	want := `{"pool_available":false,"card":null,"symbol":null,` +
		`"message":` + string(mustJSON(t, noPoolMessage)) + `}`
	if string(raw) != want {
		t.Fatalf("got %s\nwant %s", raw, want)
	}
}

// ADR 5: a deck this caller cannot see is 404 in the deck's own words.
func TestAnUnknownDeckIs404(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	status, body, raw := call(t, a, http.MethodPost,
		"/api/decks/local/nope/wheel", "")
	if status != http.StatusNotFound {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["detail"] != "no deck 'nope'" {
		t.Fatalf("detail: %s", raw)
	}
}
