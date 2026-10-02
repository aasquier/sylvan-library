package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// Every admin and account route against a database that answers for a while
// and then stops.
//
// `closeddb_test.go` asks the same question of a handle that has **gone**, and
// a gone handle fails at the *first* statement of a request. Almost every route
// in this file's reach is longer than one statement — the roster reads the
// users, then each account's password state, then each one's outstanding
// invite, then the session counts, then the usable-admin count — and the arms
// between the first statement and the last had no fixture at all. They are the
// arms where a request has **part** of its answer in hand and has to decide
// what to do with it, which is the shape of fault that produces a wrong answer
// rather than an error: a roster rendered from the three accounts it managed
// to read, with the fourth silently absent, looks exactly like a roster.
//
// `app.db` is on the instance's volume (ADR 23). A volume that detaches
// between two statements of one request is the fault this is, and
// `authtest.OpenFaulty` is that fault as a fixture: a real migrated database
// reached through a real driver, with a counter saying how many more statements
// this handle will answer.
//
// **The budget is swept rather than counted.** A hand-counted budget is a
// restatement of today's implementation — add a query to `accountBody` and the
// number that used to land on the commit lands somewhere else, and the test
// keeps passing while testing something else. Sweeping every budget from zero
// up asks the question the routes actually have to answer: *at no point in this
// request may a failure become a false answer.*

// faultyAdmin is an API over a real migrated `app.db` reached through a handle
// with a statement budget, seeded with the three accounts the admin routes are
// about: alice administers and can sign in, bob cannot administer, and
// `waiting` holds an unclaimed invite.
//
// One handle serves as both the read and the write side. The app opens two
// (`mode=ro` and `mode=rw`) and the difference matters to the gate that
// refuses a write; it does not matter to the question here, which is what a
// statement failure does once a request is under way.
func faultyAdmin(t *testing.T) (*API, *authtest.Fault) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, fault, err := authtest.OpenFaulty(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	for _, seed := range []struct {
		name, email string
		admin, pass bool
	}{
		{"alice", "alice@example.com", true, true},
		{"bob", "bob@example.com", false, true},
		{"waiting", "waiting@example.com", false, false},
	} {
		user, err := auth.Create(ctx, db, seed.name, seed.email, seed.admin)
		if err != nil {
			t.Fatalf("seeding %s: %v", seed.name, err)
		}
		if seed.pass {
			hash, err := seededHash()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx,
				"UPDATE users SET password_hash = ? WHERE id = ?",
				hash, user.ID); err != nil {
				t.Fatal(err)
			}
		} else if _, err := auth.IssueToken(ctx, db, user.ID,
			auth.PurposeInvite); err != nil {
			t.Fatal(err)
		}
	}
	// Two days of edits, so the activity view's result set is more than one
	// row and a read can be cut short in the middle of it rather than at its
	// end. Yesterday and the day before, never a written date: the view's
	// window is thirty days back from now, and two literal September days
	// fell out of it on 2026-10-02 and turned every run on `main` red with
	// the deploy skipped behind it.
	for _, back := range []int{1, 2} {
		day := time.Now().UTC().AddDate(0, 0, -back).Format(time.DateOnly)
		if _, err := db.ExecContext(ctx,
			"INSERT INTO deck_log (created_at, owner_id, slug, actor, action,"+
				" summary) VALUES (?, ?, ?, ?, ?, ?)",
			day+"T12:00:00+00:00", 1, "cats", "alice", "add", "one card"); err != nil {
			t.Fatalf("seeding the activity log: %v", err)
		}
	}

	a := New(Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AppDB:  db, AppWriteDB: db, AppDBPath: path,
		DecksDir: t.TempDir(), DataDir: t.TempDir(),
		EmailSender: &recordedSender{},
	})
	return a, fault
}

// everyAdminRoute is every admin and account route with a fillable pattern,
// discovered from the table the API hands the door rather than listed here —
// so the route added last, which is the one nobody has driven against a
// half-answering database, is in the sweep the day it lands.
func everyAdminRoute(t *testing.T, a *API) []struct{ Method, Target, Body string } {
	t.Helper()
	out := []struct{ Method, Target, Body string }{}
	for _, route := range a.Routes() {
		if !strings.HasPrefix(route.Pattern, "/api/admin") &&
			!strings.HasPrefix(route.Pattern, "/api/account") {
			continue
		}
		// `bob` rather than `alice`: the destructive routes are in this sweep,
		// and a request that succeeds must not be able to delete the admin the
		// next request signs in as.
		target := strings.ReplaceAll(route.Pattern, "{username}", "bob")
		if strings.Contains(target, "{") {
			continue
		}
		out = append(out, struct{ Method, Target, Body string }{
			route.Method, target, bodyFor(route.Pattern)})
	}
	if len(out) < 12 {
		t.Fatalf("only %d admin routes were found -- the filter has stopped "+
			"matching the route table", len(out))
	}
	return out
}

// answersHonestly is `closeddb_test.go`'s question, asked of one answer: the
// route may refuse, and it may succeed, and the one thing it may not do is
// report an emptiness it did not read.
func answersHonestly(t *testing.T, what string, status int, raw []byte) {
	t.Helper()
	if len(raw) == 0 && status != http.StatusNoContent {
		t.Errorf("%s answered %d with no body", what, status)
		return
	}
	if status >= 400 {
		if !json.Valid(raw) {
			t.Errorf("%s answered %d with non-JSON: %s", what, status, raw)
			return
		}
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
			t.Errorf("%s answered %d with no detail: %s", what, status, raw)
		}
		return
	}
	if isEmptyCollection(raw) {
		t.Errorf("%s answered %d with an empty collection over a database that "+
			"stopped answering partway through -- that reads as 'there is "+
			"nothing there': %s", what, status, raw)
	}
}

// rigFor is the fixture for one step of a sweep.
//
// A read-only route gets one instance for the whole sweep — healed between
// budgets, so each step starts from the same rows — because building a scratch
// `app.db` per budget is the expensive half of these tests and a GET has
// nothing to change. A route that writes gets a fresh one every time: a
// request that got far enough to insert an account would otherwise change what
// the next budget is measuring.
func rigFor(t *testing.T, method string, shared *API, fault *authtest.Fault) (
	*API, *authtest.Fault) {

	t.Helper()
	if method == http.MethodGet && shared != nil {
		fault.Heal()
		return shared, fault
	}
	return faultyAdmin(t)
}

// The sweep. Every admin and account route, at every budget from "the very
// next statement fails" up past the longest request any of them makes.
func TestNoAdminRouteLiesWhenTheDatabaseStopsAnsweringPartwayThrough(t *testing.T) {
	t.Parallel()
	// The ceiling is above the longest of these requests: the roster reads
	// three accounts at four statements each plus the admin count. A budget
	// past the end is not wasted — it is the healthy case, and it has to
	// answer too.
	const widest = 20

	probe, _ := faultyAdmin(t)
	for _, route := range everyAdminRoute(t, probe) {
		t.Run(route.Method+" "+route.Target, func(t *testing.T) {
			t.Parallel()
			shared, sharedFault := faultyAdmin(t)
			for budget := 0; budget <= widest; budget++ {
				a, fault := rigFor(t, route.Method, shared, sharedFault)
				fault.After(budget)
				status, _, raw := callAs(t, a, alice, route.Method,
					route.Target, route.Body)
				answersHonestly(t, route.Method+" "+route.Target, status, raw)
				// Healed on the way out so the shared instance starts the
				// next budget from the rows it started this one from.
				fault.Heal()
			}
		})
	}
}

// The control, and it is load-bearing: with the budget lifted every route in
// the sweep answers without a server fault. Without this, a fixture that was
// simply broken — a schema the seeding never applied, a scope the routes
// refuse — would make every assertion above pass for the wrong reason.
func TestEveryRouteInTheSweepAnswersOnAHealthyDatabase(t *testing.T) {
	t.Parallel()
	probe, _ := faultyAdmin(t)
	for _, route := range everyAdminRoute(t, probe) {
		a, fault := faultyAdmin(t)
		fault.Heal()
		status, _, raw := callAs(t, a, alice, route.Method, route.Target, route.Body)
		if status >= 500 && status != http.StatusServiceUnavailable {
			t.Errorf("%s %s answered %d on a healthy database: %s",
				route.Method, route.Target, status, raw)
		}
	}
}

// A read cut short partway down its result set.
//
// The statement budget above fails a whole statement; this fails the
// *iteration* of one that already succeeded, which is the only fault that
// produces a short list rather than an error. Every loop over a result set in
// this package ends by asking `rows.Err()` whether the walk failed or merely
// finished, and the difference is a page of history with rows missing and
// nothing saying so.
func TestNoAdminRouteLiesWhenAReadIsCutShortMidIteration(t *testing.T) {
	t.Parallel()
	probe, _ := faultyAdmin(t)
	for _, route := range everyAdminRoute(t, probe) {
		t.Run(route.Method+" "+route.Target, func(t *testing.T) {
			t.Parallel()
			shared, sharedFault := faultyAdmin(t)
			// Wider than it looks necessary, and the reason is the activity
			// view: its result-set walk is the *last* read of a handler that
			// has already made five single-row queries, and a row budget is
			// spent across every one of them. A sweep that stopped at four
			// would never reach the loop.
			for rows := 0; rows <= 24; rows++ {
				a, fault := rigFor(t, route.Method, shared, sharedFault)
				fault.RowsAfter(rows)
				status, _, raw := callAs(t, a, alice, route.Method,
					route.Target, route.Body)
				answersHonestly(t, route.Method+" "+route.Target, status, raw)
				fault.Heal()
			}
		})
	}
}

// The activity view named on its own, because it is the one route in the sweep
// whose answer is a *list read out of a result set* and whose emptiness is a
// sentence: "nobody has edited anything for a month" is what an admin reads
// off an empty `edits` array, and it is the answer a walk that failed halfway
// would give.
func TestTheActivityViewRefusesRatherThanReportingADayItCouldNotRead(t *testing.T) {
	t.Parallel()
	a, fault := faultyAdmin(t)

	// Healthy first: both seeded days come back, so the assertion below is
	// about the cut-short read and not about an empty log.
	status, _, raw := callAs(t, a, alice, http.MethodGet, "/api/admin/stats/activity", "")
	if status != http.StatusOK {
		t.Fatalf("the healthy activity view answered %d: %s", status, raw)
	}
	var whole struct {
		Edits []struct {
			Day   string `json:"day"`
			Edits int    `json:"edits"`
		} `json:"deck_edits_by_day"`
	}
	if err := json.Unmarshal(raw, &whole); err != nil {
		t.Fatalf("decoding the activity view: %v", err)
	}
	if len(whole.Edits) != 2 {
		t.Fatalf("the fixture's two days of edits read back as %d: %s",
			len(whole.Edits), raw)
	}

	// One day handed over, and the read of the second fails rather than ending
	// the set.
	a2, fault2 := faultyAdmin(t)
	fault2.RowsAfter(1)
	status, _, raw = callAs(t, a2, alice, http.MethodGet, "/api/admin/stats/activity", "")
	fault2.Heal()
	fault.Heal()
	if status == http.StatusOK {
		t.Errorf("a read cut short after one day answered 200: %s", raw)
	}
	answersHonestly(t, "GET /api/admin/stats/activity", status, raw)
}
