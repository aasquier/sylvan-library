package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The **sign-in** routes against a database that stops answering mid-request.
//
// `halfansweringdb_test.go` sweeps the budget over `/api/admin` and
// `/api/account`; the four routes under `/api/auth` are outside that filter and
// outside every other fixture in the package, which left the rate limiter's
// wait-fallback, the session-fixation delete, the session mint and the logout's
// own delete standing on nothing. They are the routes most worth the sweep:
// every one of them is reached by somebody who is not signed in yet, and the
// thing they must never do is let a statement failure become a *successful*
// sign-in, or hand out a cookie over a session that was never written.
//
// The budget is swept rather than counted, for lever 29's reason: a counted
// budget restates today's statement order, and the next query added to `login`
// moves it without anything failing. **A fresh rig per budget**, because every
// one of these routes writes — a claim spends its token, a login mints a
// session — so a shared fixture would have the second budget asking a different
// question from the first.

// authRig is one instance whose `app.db` answers a fixed number of statements
// and then refuses, plus the two secrets a sign-in needs: an unclaimed invite
// token and a live session.
type authRig struct {
	api     *API
	fault   *authtest.Fault
	token   string
	session string
}

// faultyAuth seeds alice (claimable, with a password) and `waiting` (an
// unclaimed invite). Auth is **on**, which is what makes the logout path do any
// work at all — with it off, logout deliberately never opens the database.
func faultyAuth(t *testing.T) *authRig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, fault, err := authtest.OpenFaulty(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fault.Heal(); _ = db.Close() })

	ctx := context.Background()
	admin, err := auth.Create(ctx, db, "alice", "alice@example.com", true)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := seededHash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE users SET password_hash = ? WHERE id = ?", hash, admin.ID); err != nil {
		t.Fatal(err)
	}
	waiting, err := auth.Create(ctx, db, "waiting", "waiting@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(ctx, db, waiting.ID, auth.PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, db, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	return &authRig{
		api: New(Config{Logger: quietLogger(), AppDB: db, AppWriteDB: db,
			AppDBPath: path, DecksDir: t.TempDir(), RequireAuth: true,
			EmailSender: &recordedSender{}}),
		fault: fault, token: token, session: session,
	}
}

// signInRoutes is the `/api/auth` routes that read the database, discovered from
// the table the API hands the door so a fifth one joins the sweep the day it
// lands. `reset` is deliberately out: it answers the same sentence whatever
// happens and sends from a goroutine, which is ADR 16 working rather than
// something a statement budget can ask about.
func signInRoutes(t *testing.T, a *API) []string {
	t.Helper()
	wanted := map[string]bool{
		"/api/auth/login":         true,
		"/api/auth/logout":        true,
		"/api/auth/claim":         true,
		"/api/auth/claim/preview": true,
	}
	out := []string{}
	for _, route := range a.Routes() {
		if route.Method == http.MethodPost && wanted[route.Pattern] {
			out = append(out, route.Pattern)
		}
	}
	if len(out) != len(wanted) {
		t.Fatalf("%d of the %d sign-in routes were found -- the filter has "+
			"stopped matching the route table", len(out), len(wanted))
	}
	return out
}

func bodyForSignIn(route string, rig *authRig) string {
	switch route {
	case "/api/auth/login":
		return `{"username":"alice","password":"` + goodPassword + `"}`
	case "/api/auth/claim":
		return `{"token":"` + rig.token + `","password":"` + goodPassword + `"}`
	case "/api/auth/claim/preview":
		return `{"token":"` + rig.token + `"}`
	default:
		return `{}`
	}
}

// Every sign-in route, at every budget from nought up: **no statement failure
// may become a sign-in.**
//
// Two floors, per lever 29: at least one budget must refuse and at least one
// must succeed, or the sweep is measuring one state many times. `logout` is the
// one route where the second floor is a different fact — it answers 200 whatever
// the database does, deliberately, because a browser that asked to be signed out
// must end up signed out — so what is asserted there is that the cookie goes
// anyway.
func TestNoHalfAnsweredSignInBecomesASession(t *testing.T) {
	t.Parallel()
	for _, route := range signInRoutes(t, faultyAuth(t).api) {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			refused, allowed, settled := 0, 0, 0
			for budget := 0; budget <= 40 && settled < 2; budget++ {
				// A fresh rig per budget: every one of these routes writes, so
				// a shared fixture would have each budget asking a different
				// question from the one before it.
				rig := faultyAuth(t)
				rig.fault.After(budget)
				status, _, raw, cookies := postSignIn(t, rig.api, route,
					bodyForSignIn(route, rig), rig.session)
				rig.fault.Heal()

				what := fmt.Sprintf("%s at budget %d", route, budget)
				answersHonestly(t, what, status, raw)
				if status == http.StatusOK {
					allowed++
					settled++
				} else {
					refused++
					settled = 0
					// A refusal must not hand out a session.
					for _, c := range cookies {
						if c.Name == cookieName && c.Value != "" {
							t.Errorf("%s refused and still set a session cookie",
								what)
						}
					}
				}
				if route == "/api/auth/logout" {
					// The one route that always answers, and the thing it owes
					// is the cookie being gone.
					cleared := false
					for _, c := range cookies {
						if c.Name == cookieName && c.Value == "" {
							cleared = true
						}
					}
					if !cleared {
						t.Errorf("%s left the session cookie in place", what)
					}
				}
			}
			if route != "/api/auth/logout" && refused == 0 {
				t.Errorf("%s answered 200 at every budget, so the sweep never "+
					"reached a failure", route)
			}
			if allowed == 0 {
				t.Errorf("%s refused at every budget, so the sweep is measuring "+
					"a broken fixture rather than the route", route)
			}
		})
	}
}

// **A rate limiter that cannot say how long to wait still says to wait.**
//
// The 429 carries `Retry-After` so a page can count down, and the number comes
// from a second query. A handle that answers the "is this spent" question and
// not the "for how long" one is the volume going away between the two, and the
// refusal must still be a refusal — a login let through because the limiter
// could not read a clock would be the guard failing open.
func TestAThrottleThatCannotReadTheClockStillRefuses(t *testing.T) {
	t.Parallel()
	seenThrottle, seenEarlier := false, false
	for budget := 0; budget <= 8; budget++ {
		rig := faultyAuth(t)
		db, ok := rig.api.accountsDB()
		if !ok {
			t.Fatal("the rig built no accounts database")
		}
		// Spend the per-account budget for real, so `Exhausted` answers yes.
		key := auth.AccountKey("alice")
		for i := 0; i <= auth.PerAccount.Failures; i++ {
			if _, err := auth.RecordFailure(context.Background(), db, key,
				auth.PerAccount); err != nil {
				t.Fatal(err)
			}
		}

		rig.fault.After(budget)
		status, _, raw, _ := postSignIn(t, rig.api, "/api/auth/login",
			`{"username":"alice","password":"`+goodPassword+`"}`, rig.session)
		rig.fault.Heal()

		if status == http.StatusOK {
			t.Fatalf("a login against an exhausted bucket succeeded at budget "+
				"%d: %s", budget, raw)
		}
		if status == http.StatusTooManyRequests {
			seenThrottle = true
		} else {
			seenEarlier = true
		}
	}
	if !seenThrottle {
		t.Error("no budget in the sweep reached the throttle at all, so the " +
			"bucket was never read as spent")
	}
	if !seenEarlier {
		t.Error("every budget answered 429, so the sweep never reached a " +
			"statement before the limiter")
	}
}

// postSignIn posts to one of the sign-in routes carrying a session cookie, which
// is what makes the logout path and the session-fixation delete do any work at
// all, and hands back the cookies the answer set.
func postSignIn(t *testing.T, a *API, target, body, session string) (
	int, map[string]any, []byte, []*http.Cookie) {

	t.Helper()
	var handler http.HandlerFunc
	for _, route := range a.Routes() {
		if route.Pattern == target && route.Method == http.MethodPost {
			handler = route.Handler
			break
		}
	}
	if handler == nil {
		t.Fatalf("no POST route for %s", target)
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: session})
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	raw := rec.Body.Bytes()
	payload := map[string]any{}
	_ = json.Unmarshal(raw, &payload)
	return rec.Code, payload, raw, rec.Result().Cookies()
}
