package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// What the account and admin surfaces do when `app.db` stops answering.
//
// This is not a contrived state. `app.db` lives on the instance's volume, and
// the volume has failed to mount, has filled up, and has been read by a
// process that could not open it -- three of the four deployment-only faults
// CLAUDE.md records. What each of these routes does at that moment decides
// whether a maintainer sees "something is wrong with the server" or sees a
// **wrong answer**, which is far worse: `Exhausted` answering false on a
// failed read would turn the rate limiter off exactly when the database is
// unwell, and `HasPassword` answering false would re-invite somebody who has
// already claimed their account.
//
// So the sweep asks two things of every route: it must not panic, and it must
// not answer 200. A 500 is the honest answer here and the only safe one.

// failedRig is the account rig with its write handle closed, so every
// statement fails at the driver the way a vanished volume does.
func failedRig(t *testing.T) *accountRig {
	t.Helper()
	rig := newAccountRig(t, true)
	// Seeded first, then broken: the routes are asked about accounts that
	// really existed a moment ago, which is the shape of a volume going away
	// underneath a running process.
	if err := rig.db.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rig.close)
	return rig
}

// Every admin route, against a database that has stopped answering.
func TestEveryAdminRouteFailsLoudlyWhenTheDatabaseStops(t *testing.T) {
	t.Parallel()
	rig := failedRig(t)

	for _, route := range adminRoutes {
		rec := rig.call(t, adminScope, route.method, route.target, route.payload, "")
		switch rec.Code {
		case http.StatusOK:
			// The stats views are answered from the box rather than from
			// the database, and answering them is correct: they degrade to
			// the shapes an empty database would give.
			if !strings.HasPrefix(route.target, "/api/admin/stats/") {
				t.Errorf("%s %s answered 200 over a dead database: %s",
					route.method, route.target, rec.Body.String())
			}
		case http.StatusInternalServerError, http.StatusServiceUnavailable,
			http.StatusNotFound, http.StatusUnprocessableEntity:
			// All honest: something is wrong and the route says so.
		default:
			t.Errorf("%s %s answered %d over a dead database", route.method, route.target, rec.Code)
		}
		// Whatever it answered, it answered -- rather than panicking, which
		// would take the whole process with it.
		if rec.Body.Len() == 0 && rec.Code != http.StatusNoContent {
			t.Errorf("%s %s answered %d with no body", route.method, route.target, rec.Code)
		}
	}
}

// The account routes, likewise -- and the sign-in one matters most: a login
// that answered 200 over a failed read would be an authentication bypass.
func TestSigningInOverADeadDatabaseFailsClosed(t *testing.T) {
	t.Parallel()
	rig := failedRig(t)

	rec := rig.call(t, auth.Scope{}, http.MethodPost, "/api/auth/login",
		`{"username":"alice","password":"`+goodPassword+`"}`, "")
	if rec.Code == http.StatusOK {
		t.Fatalf("a login succeeded over a dead database: %s", rec.Body.String())
	}
	// And no session cookie was handed out on the way past.
	for _, c := range rec.Result().Cookies() {
		if c.Value != "" {
			t.Errorf("a failed login set %s=%q", c.Name, c.Value)
		}
	}
}

// A reset request must cost the same whether or not the address resolves --
// which is why it answers before the lookup happens. A dead database must not
// change that: an error here would tell an attacker their address was the one
// that reached the database.
func TestAResetRequestCostsTheSameOverADeadDatabase(t *testing.T) {
	t.Parallel()
	rig := failedRig(t)

	known := rig.call(t, auth.Scope{}, http.MethodPost, "/api/auth/reset",
		`{"email":"alice@example.com"}`, "")
	unknown := rig.call(t, auth.Scope{}, http.MethodPost, "/api/auth/reset",
		`{"email":"stranger@example.com"}`, "")
	rig.api.WaitBackground()

	if known.Code != unknown.Code {
		t.Errorf("a known address answered %d and an unknown one %d -- the "+
			"difference is an oracle", known.Code, unknown.Code)
	}
	if known.Body.String() != unknown.Body.String() {
		t.Errorf("the two answers differ:\n%s\n%s", known.Body.String(), unknown.Body.String())
	}
}

// Claiming an invite over a dead database must not hand out a session, and
// must not report success -- a claim that "worked" against nothing would
// leave somebody believing they have an account.
func TestClaimingAnInviteOverADeadDatabaseDoesNotSucceed(t *testing.T) {
	t.Parallel()
	rig := failedRig(t)

	rec := rig.call(t, auth.Scope{}, http.MethodPost, "/api/auth/claim",
		`{"token":"whatever","password":"`+goodPassword+`"}`, "")
	if rec.Code == http.StatusOK {
		t.Fatalf("a claim succeeded over a dead database: %s", rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Value != "" {
			t.Errorf("a failed claim set %s=%q", c.Name, c.Value)
		}
	}
}

// Whatever any of these answered, **no address is in it**. A failure path is
// exactly where a redaction gets forgotten, because nobody looks at the body
// of a 500.
func TestNoFailurePathEverLeaksAnAddress(t *testing.T) {
	t.Parallel()
	rig := failedRig(t)

	for _, route := range adminRoutes {
		rec := rig.call(t, adminScope, route.method, route.target, route.payload, "")
		if strings.Contains(rec.Body.String(), "@example.com") &&
			!strings.HasPrefix(route.target, "/api/admin/users") {
			t.Errorf("%s %s leaked an address on a failure path: %s",
				route.method, route.target, rec.Body.String())
		}
	}

	// The auth routes are the ones a stranger can reach, so none of them may
	// carry an address at all -- failure or not.
	for _, tc := range []struct{ target, body string }{
		{"/api/auth/login", `{"username":"alice@example.com","password":"x"}`},
		{"/api/auth/reset", `{"email":"alice@example.com"}`},
		{"/api/auth/claim", `{"token":"x","password":"` + goodPassword + `"}`},
	} {
		rec := rig.call(t, auth.Scope{}, http.MethodPost, tc.target, tc.body, "")
		if strings.Contains(rec.Body.String(), "alice@example.com") {
			t.Errorf("%s echoed the address back: %s", tc.target, rec.Body.String())
		}
	}
}

// An instance with no `app.db` at all is a different state from one whose
// database has failed, and the account routes tell them apart: no database is
// an instance with auth off, which answers rather than failing.
func TestAnInstanceWithNoDatabaseIsNotAFailedOne(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: t.TempDir(), DataDir: t.TempDir()})

	// `me` reports what this process is, which needs no database at all.
	status, _, raw := callAs(t, a, auth.Scope{}, http.MethodGet, "/api/auth/me", "")
	if status != http.StatusOK {
		t.Errorf("/api/auth/me answered %d with no app.db: %s", status, raw)
	}
}
