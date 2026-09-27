package door

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/api"
)

// The route table refuses a pattern it could not route, and it refuses at
// build time — which is the whole point of it being built rather than
// consulted. `New` compiles the table before it takes the port, so a pattern
// nobody can match is a boot that fails on this machine rather than a 404
// somebody meets on the instance.
//
// These shapes cannot appear in the served table today, and that is exactly why
// the guard had never run: it is standing in front of a mistake the next
// route family will make, not one this one has.
func TestARouteTableRefusesAPatternItCouldNotRoute(t *testing.T) {
	t.Parallel()
	nothing := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for name, route := range map[string]api.Route{
		"no handler at all":    {Method: "GET", Pattern: "/api/decks"},
		"outside the api tree": {Method: "GET", Pattern: "/decks", Handler: nothing},
		"a trailing slash the door would have normalised away": {
			Method: "GET", Pattern: "/api/decks/", Handler: nothing},
		"an empty segment": {Method: "GET", Pattern: "/api//decks", Handler: nothing},
		"a parameter the matcher cannot read": {
			Method: "GET", Pattern: "/api/decks/{}", Handler: nothing},
		"a parameter with a slash in its suffix": {
			Method: "GET", Pattern: "/api/symbols/{code}.svg.{ext}", Handler: nothing},
	} {
		if _, err := newRouteTable([]api.Route{route}); err == nil {
			t.Errorf("%s: %s %s compiled", name, route.Method, route.Pattern)
		}
	}

	// And the table this app actually serves compiles, so the refusals above
	// are about the patterns rather than about the compiler.
	served := api.New(api.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := newRouteTable(served.Routes()); err != nil {
		t.Fatalf("the served route table no longer compiles: %v", err)
	}
}

// A parameter matches one character or more, never none. The suffix form is
// where that bites: `/api/symbols/{code}.svg` must not match `/api/symbols/.svg`,
// because the empty code would reach the handler as a real request for a
// symbol with no name.
func TestAParameterMatchesOneOrMoreCharactersAndNeverNone(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		seg, part string
		want      bool
		value     string
	}{
		"a plain parameter takes the whole segment": {"{slug}", "mono-green", true, "mono-green"},
		"a suffixed parameter takes what is before it": {
			"{code}.svg", "2G.svg", true, "2G"},
		"a suffix that is not there does not match":   {"{code}.svg", "2G.png", false, ""},
		"a value of nothing before the suffix":        {"{code}.svg", ".svg", false, ""},
		"a literal segment is not a parameter at all": {"decks", "decks", false, ""},
	} {
		_, value, ok := capture(tc.seg, tc.part)
		if ok != tc.want || value != tc.value {
			t.Errorf("%s: capture(%q, %q) = %q, %v", name, tc.seg, tc.part, value, ok)
		}
	}

	// The same rule through the door, where it decides between a 404 and a
	// 405: a path whose parameter cannot bind matches no route on any
	// method, so it is not there rather than being there on another verb.
	srv := build(t, false, nil)
	resp := get(t, srv, "POST", "/api/symbols/.svg", "")
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Fatalf("a parameter that binds nothing was offered another method: %s",
			resp.Header.Get("Allow"))
	}
	// And the same path with a code in it is a real route, refused on the
	// wrong method rather than missing.
	real := get(t, srv, "POST", "/api/symbols/2G.svg", "")
	if real.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("a real route on the wrong method answered %d", real.StatusCode)
	}
}

// The door's own health check is a GET, and anything else is a 405 rather
// than a cheerful `ok: true`. It is the endpoint a platform polls, so a
// misconfigured probe answering green on a POST would be a health check that
// cannot fail.
func TestTheDoorsHealthCheckAnswersOnlyTheMethodsAProbeUses(t *testing.T) {
	t.Parallel()
	srv := build(t, false, nil)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if resp := get(t, srv, method, DoorHealthPath, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("%s %s answered %d", method, DoorHealthPath, resp.StatusCode)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodDelete, http.MethodPut} {
		resp := get(t, srv, method, DoorHealthPath, "")
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s answered %d, want 405", method, DoorHealthPath, resp.StatusCode)
		}
	}
}

// A door with a read handle and no write handle still resolves sessions — it
// simply cannot pay the table what a live server owes it. That is the
// degraded shape a broken volume gets, and the branch that chooses it had
// never been taken: every other test in this package either has both handles
// or has neither.
func TestADoorWithNoWriteHandleStillResolvesWithoutTouching(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	// Named, and not there: `auth.Open` records the DSN without touching the
	// disk, so the read side stands, while the write side finds no file and
	// leaves itself nil rather than minting one.
	absent := filepath.Join(t.TempDir(), "app.db")
	logged := &lockedLog{}
	d, err := New(Config{RequireAuth: true, AppDB: absent,
		WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(logged, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if d.writeDB != nil {
		t.Fatal("a write handle was opened over a file that is not there")
	}
	resolver, ok := d.resolver.(dbResolver)
	if !ok {
		t.Fatalf("the resolver is %T, not the database one", d.resolver)
	}
	if resolver.touch {
		t.Fatal("a door with no write handle promised to touch last_seen_at")
	}

	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	// A cookie over a database that is not there is not a session, and the
	// caller is anonymous rather than signed in as somebody.
	resp := get(t, srv, "GET", "/api/decks", "a-token-with-no-database-behind-it")
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("an unresolvable cookie answered %d: %s", resp.StatusCode, body)
	}
	if strings.Contains(logged.String(), "a-token-with-no-database-behind-it") {
		t.Fatal("the session token reached the log")
	}
}
