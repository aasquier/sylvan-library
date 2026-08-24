package api

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/tarot"
)

func claudeRoute(t *testing.T, pattern string) Route {
	t.Helper()
	for _, route := range New(Config{}).Routes() {
		if route.Pattern == pattern {
			return route
		}
	}
	t.Fatalf("no route registered for %s", pattern)
	return Route{}
}

// TestThePersonaRouteServesTheRosterAndNoPrompt is the wire half of the
// structural guard in internal/claude: the served payload has no `voice`.
//
// Asserted at the route as well as at the type, because these fail
// differently — the type test catches a RosterEntry that grows a field, and
// this catches a handler that stops using RosterEntry.
func TestThePersonaRouteServesTheRosterAndNoPrompt(t *testing.T) {
	t.Parallel()
	route := claudeRoute(t, "/api/claude/personas")
	rec := httptest.NewRecorder()
	route.Handler(rec, httptest.NewRequest(http.MethodGet, route.Pattern, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "voice") {
		t.Errorf("the route leaked a prompt:\n%s", body)
	}
	var got struct {
		Personas []claude.RosterEntry `json:"personas"`
		Default  string               `json:"default"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got.Personas) != len(claude.PersonaKeys) || got.Default != claude.DefaultPersona {
		t.Errorf("roster is %d entries default %q, want %d / %q",
			len(got.Personas), got.Default, len(claude.PersonaKeys), claude.DefaultPersona)
	}
	// The recorded body is `personas` first, then `default`; a struct in the
	// wrong field order marshals to different bytes and the goldens hold it.
	if !strings.HasPrefix(body, `{"personas":`) || !strings.Contains(body, `"default":`) {
		t.Errorf("field order is not the recorded one:\n%s", body[:60])
	}
}

// TestTheTarotRouteDealsFromTheSeedItIsGiven covers the query-string half,
// which is where this route's real divergences live: the recorded reading
// takes the LAST
// repeated value where Go's Query().Get takes the first, and the recorded
// integer grammar accepts three spellings strconv refuses.
func TestTheTarotRouteDealsFromTheSeedItIsGiven(t *testing.T) {
	t.Parallel()
	route := claudeRoute(t, "/api/tarot/reading")
	deal := func(target string) (int, string) {
		rec := httptest.NewRecorder()
		route.Handler(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec.Code, rec.Body.String()
	}

	// A seeded deal is the seeded deal, and the route adds nothing to it.
	code, body := deal("/api/tarot/reading?seed=7")
	if code != http.StatusOK {
		t.Fatalf("seed=7: status %d", code)
	}
	want, err := json.Marshal(tarot.Deal(big.NewInt(7)))
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if body != string(want) {
		t.Errorf("seed=7:\n route %s\n deal  %s", body, want)
	}

	// Last value wins. `?seed=7&seed=9` is nine, and a handler using
	// Query().Get() answers seven — silently, and only for a client that
	// appends rather than replaces.
	if _, nine := deal("/api/tarot/reading?seed=7&seed=9"); !strings.HasPrefix(nine, `{"seed":9,`) {
		t.Errorf("repeated seed: took the first, not the last: %s", nine[:40])
	}

	// The three spellings the recorded grammar accepts and strconv.ParseInt
	// does not.
	for target, wantSeed := range map[string]string{
		"/api/tarot/reading?seed=1_0":     `{"seed":10,`,
		"/api/tarot/reading?seed=+7":      `{"seed":7,`,
		"/api/tarot/reading?seed=%20%207": `{"seed":7,`,
		"/api/tarot/reading?seed=0007":    `{"seed":7,`,
	} {
		code, body := deal(target)
		if code != http.StatusOK {
			t.Errorf("%s: %d, want 200 — the record answers this one", target, code)
			continue
		}
		if !strings.HasPrefix(body, wantSeed) {
			t.Errorf("%s: %s, want prefix %s", target, body[:24], wantSeed)
		}
	}

	// And what it refuses, as the validation 422. An EMPTY value is a
	// refusal,
	// not a fresh deal — measured against the real route, because the opposite
	// is the natural thing to write.
	for _, target := range []string{
		"/api/tarot/reading?seed=abc", "/api/tarot/reading?seed=7.5",
		"/api/tarot/reading?seed=", "/api/tarot/reading?seed=_7",
		"/api/tarot/reading?seed=%EF%BC%97", // fullwidth 7: the body grammar takes it, the query grammar does not
	} {
		code, body := deal(target)
		if code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", target, code)
			continue
		}
		if !strings.Contains(body, `"int_parsing"`) || !strings.Contains(body, `"query"`) {
			t.Errorf("%s: not the recorded int_parsing shape: %s", target, body)
		}
	}

	// No parameter at all is a fresh deal, which is the one case that must not
	// be a 422.
	code, body = deal("/api/tarot/reading")
	if code != http.StatusOK || !strings.Contains(body, `"cards":`) {
		t.Errorf("an absent seed must deal: %d %s", code, body[:60])
	}
}
