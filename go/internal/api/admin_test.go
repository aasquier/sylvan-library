package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

// The admin routes. ADR 17's surface, and the one place in the whole port
// where an **email address is deliberately serialised** -- so the tests carry
// that rule from both sides: present here, absent everywhere else.
//
// The door refuses `/api/admin` to a non-admin before routing, so these
// handlers are the *second* check. That is the one that matters when a route
// is mounted in the wrong place, which is precisely the case no structural
// rule can see, so it is swept over every route rather than spot-checked.

// adminScope and plainScope are the two callers every route is asked about.
var (
	adminScope = auth.Scope{UserID: 1, Username: "alice", IsAdmin: true, Authenticated: true}
	plainScope = auth.Scope{UserID: 2, Username: "bob", Authenticated: true}
)

// adminRoutes is every admin route this side answers, with a body that would
// be valid if the caller were entitled to send it.
var adminRoutes = []struct{ method, target, payload string }{
	{"GET", "/api/admin/users", ""},
	{"POST", "/api/admin/users", `{"email":"new@example.com"}`},
	{"PATCH", "/api/admin/users/bob", `{"is_admin":true}`},
	{"POST", "/api/admin/users/bob/reset", ""},
	{"DELETE", "/api/admin/users/bob/sessions", ""},
}

// The second check, on every route. 403 and **not** ADR 5's 404, which is the
// deliberate exception ADR 17 argues: that rule protects resources whose
// existence is the secret, and an admin route's existence is published in a
// public repository.
func TestEveryAdminRouteRefusesANonAdminItself(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()

	for _, route := range adminRoutes {
		rec := rig.call(t, plainScope, route.method, route.target, route.payload, "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s answered %d to a caller who is not an admin",
				route.method, route.target, rec.Code)
		}
		if detail(t, rec) != "admin only" {
			t.Errorf("%s %s said %q", route.method, route.target, detail(t, rec))
		}
	}
	// Nothing happened while it was refusing: bob is still not an admin and
	// still has whatever sessions he had.
	bob, _ := auth.Get(context.Background(), rig.db, "bob")
	if bob.IsAdmin {
		t.Error("a refused PATCH granted admin anyway")
	}
}

// **The one route that is deliberately not here.** `DELETE
// /api/admin/users/{username}` calls `jobs.forget_owner` on a registry held in
// the uvicorn process's memory, and `users.id` is re-issued by SQLite, so jobs
// left keyed on a freed id would be handed to whoever is created next. A DELETE
// on that path must therefore match no route on this side and fall through to
// the proxy as it arrived.
//
// Asserted rather than left implicit, because the failure mode of somebody
// "finishing the family" later is silent: the route would answer, the account
// would go, and the jobs would stay.
func TestDeletingAnAccountIsStillPythons(t *testing.T) {
	for _, route := range New(Config{}).Routes() {
		if route.Method == http.MethodDelete && route.Pattern == "/api/admin/users/{username}" {
			t.Fatal("DELETE /api/admin/users/{username} has been ported, but " +
				"`jobs.forget_owner` runs in the Python process and `users.id` " +
				"is re-issued -- see internal/api/admin.go and PLAN section 10")
		}
	}
}

// ---- the account list ------------------------------------------------------

func TestTheListCarriesTheAddressTheStateAndTheRoster(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()

	rec := rig.call(t, adminScope, "GET", "/api/admin/users", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := body(t, rec)

	// `admins` is the count of admins who can actually sign in -- the same
	// predicate the refusal uses, so the page can grey out the last demote
	// button rather than offering a click that returns 409.
	if got["admins"] != float64(1) {
		t.Errorf("admins = %v, want 1", got["admins"])
	}

	// The roster the page offers is the roster the server accepts, and it
	// carries **no model id** (commandment 10).
	raw, _ := json.Marshal(got["tiers"])
	var roster []map[string]any
	_ = json.Unmarshal(raw, &roster)
	if len(roster) != len(tiers.All) {
		t.Fatalf("the roster has %d entries, the build knows %d", len(roster), len(tiers.All))
	}
	for _, entry := range roster {
		if entry["key"] == nil || entry["label"] == nil || entry["blurb"] == nil {
			t.Errorf("a roster entry is incomplete: %v", entry)
		}
		if len(entry) != 3 {
			t.Errorf("a roster entry carries more than key/label/blurb: %v", entry)
		}
		for _, tier := range tiers.All {
			if entry["key"] == tier.Key && strings.Contains(
				strings.Join([]string{entry["label"].(string), entry["blurb"].(string)}, " "),
				tier.Model) {
				t.Errorf("the roster names a model id: %v", entry)
			}
		}
	}

	states := map[string]string{}
	for _, item := range got["users"].([]any) {
		account := item.(map[string]any)
		// **The address is here**, which ADR 17 decided explicitly and which
		// is the whole reason this route exists rather than reusing the shelf.
		if _, present := account["email"]; !present {
			t.Errorf("an admin's account list has no address: %v", account)
		}
		for _, key := range []string{"id", "username", "is_admin", "disabled",
			"created_at", "model_tier", "state", "sessions"} {
			if _, present := account[key]; !present {
				t.Errorf("the account has no %q: %v", key, account)
			}
		}
		states[account["username"].(string)] = account["state"].(string)
	}
	// The four states are genuinely different things, and `invited` is the one
	// that says a link is outstanding rather than that somebody must act.
	if states["alice"] != "active" || states["bob"] != "active" ||
		states["waiting"] != "invited" {
		t.Errorf("the states read %v", states)
	}

	// An account whose invite expired or was never sent is the state that
	// needs somebody to do something, and it is not the same as `invited`.
	ctx := context.Background()
	orphan, err := auth.Create(ctx, rig.db, "orphan", "orphan@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = orphan
	rec = rig.call(t, adminScope, "GET", "/api/admin/users", "", "")
	for _, item := range body(t, rec)["users"].([]any) {
		account := item.(map[string]any)
		if account["username"] == "orphan" && account["state"] != "no password" {
			t.Errorf("an account with no invite reads %v", account["state"])
		}
	}
}

// ---- invites ---------------------------------------------------------------

func TestAnInviteCreatesAnUnclaimedAccountAndMailsIt(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()

	rec := rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"New@Example.COM","username":"newcomer"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := body(t, rec)
	if got["username"] != "newcomer" || got["email"] != "new@example.com" {
		t.Errorf("the invite answered %s", rec.Body)
	}
	// **Unclaimed, never disabled.** `disabled_at` is the maintainer's
	// revocation lever and redeeming a link must not undo it.
	if got["disabled"] != false || got["state"] != "invited" {
		t.Errorf("a fresh invite reads disabled=%v state=%v", got["disabled"], got["state"])
	}
	if len(rig.sender.messages()) != 1 {
		t.Fatalf("%d messages went out", len(rig.sender.messages()))
	}
	if rig.sender.messages()[0].To != "new@example.com" {
		t.Errorf("the invite went to %q", rig.sender.messages()[0].To)
	}
	// Sent **synchronously**, unlike the public reset: this caller is an
	// authorised admin who is owed the truth about whether the message went,
	// so nothing is waiting on a background task.
	if !strings.Contains(rig.sender.messages()[0].Body, "/auth/claim#token=") {
		t.Error("the invite carries no claim link")
	}

	// Re-inviting an *unclaimed* account re-issues the link. That is the
	// resend path, and it is why this is not a 409.
	rec = rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"new@example.com"}`, "")
	if rec.Code != http.StatusCreated {
		t.Errorf("a resend answered %d %s", rec.Code, rec.Body)
	}
	if len(rig.sender.messages()) != 2 {
		t.Errorf("the resend sent %d messages in total", len(rig.sender.messages()))
	}
	// A username derived from the address when none is given.
	rec = rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"derived@example.com"}`, "")
	if rec.Code != 201 || body(t, rec)["username"] != "derived" {
		t.Errorf("the derived handle is %s", rec.Body)
	}
}

func TestAnInviteRefusesOnTheStatusEachReasonEarns(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()

	for _, c := range []struct {
		name, payload string
		want          int
		says          string
	}{
		{"no address at all", `{}`, 422, "an invite needs an email address"},
		{"not shaped like an address", `{"email":"nope"}`, 422, "does not look like an email"},
		// **Not a string at all**, which answered 500 in Python until
		// 2026-08-22: `payload.get("email")` went to `normalise_email` raw, and
		// `.strip()` on an int raises `AttributeError`, which is not the
		// `InvalidEmail` the route catches. The door has always coerced here and
		// so has always answered 422; both runtimes now say the same sentence,
		// which is why the quoted forms are pinned rather than just the status.
		// `0` and `false` are the interesting pair -- they are what a
		// `str(... or "")` coercion would fold into "absent", reporting a
		// missing address for a body that plainly supplied one.
		{"a number", `{"email":123}`, 422, "'123' does not look like an email"},
		{"a zero", `{"email":0}`, 422, "'0' does not look like an email"},
		{"a true", `{"email":true}`, 422, "'True' does not look like an email"},
		{"a false", `{"email":false}`, 422, "'False' does not look like an email"},
		{"an explicit null", `{"email":null}`, 422, "an invite needs an email address"},
		{"an address already claimed", `{"email":"alice@example.com"}`, 409,
			"already claimed that address"},
		{"a handle nobody could hold", `{"email":"x@example.com","username":"a b"}`, 422,
			"choose a username"},
		{"a handle already taken", `{"email":"y@example.com","username":"bob"}`, 409,
			"already registered"},
	} {
		rec := rig.call(t, adminScope, "POST", "/api/admin/users", c.payload, "")
		if rec.Code != c.want {
			t.Errorf("%s answered %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
		if !strings.Contains(detail(t, rec), c.says) {
			t.Errorf("%s said %q, which does not mention %q", c.name, detail(t, rec), c.says)
		}
	}
	// A claimed address is pointed at the reset flow, because that is what
	// somebody who has forgotten their password actually needs.
	rec := rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"alice@example.com"}`, "")
	if !strings.Contains(detail(t, rec), "send them a reset link instead") {
		t.Errorf("the refusal does not point anywhere: %q", detail(t, rec))
	}
	if len(rig.sender.messages()) != 0 {
		t.Errorf("%d messages went out for a table of nothing but refusals",
			len(rig.sender.messages()))
	}
}

// **503 rather than 500**: nothing is broken, something is unset, and the
// maintainer reading it is the person who can set it. `golden/admin.json`
// records both cases, because the harness runs with no key on purpose.
func TestWithNoMailConfiguredBothSendersAnswer503(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	rig := newAccountRig(t, true)
	defer rig.close()
	rig.api.email = nil // decide from the environment, as a real process does

	for _, c := range []struct{ name, method, target, payload string }{
		{"an invite", "POST", "/api/admin/users", `{"email":"x@example.com"}`},
		{"a reset link", "POST", "/api/admin/users/bob/reset", ""},
	} {
		rec := rig.call(t, adminScope, c.method, c.target, c.payload, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s answered %d, want 503: %s", c.name, rec.Code, rec.Body)
		}
		if !strings.Contains(detail(t, rec), "RESEND_API_KEY") {
			t.Errorf("%s said %q, which does not name what is unset", c.name, detail(t, rec))
		}
	}
	// The sender is resolved *before* any database work, so a 503 leaves no
	// account behind whose invite can never be sent.
	if account, _ := auth.Get(context.Background(), rig.db, "x"); account != nil {
		t.Error("a 503 still created the account")
	}
}

// ---- patch -----------------------------------------------------------------

func TestPatchGrantsRevokesDisablesAndSetsATier(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()

	rec := rig.call(t, adminScope, "PATCH", "/api/admin/users/bob", `{"is_admin":true}`, "")
	if rec.Code != http.StatusOK || body(t, rec)["is_admin"] != true {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// The answer is a re-read of the account, not an echo of the request.
	if body(t, rec)["state"] != "active" {
		t.Errorf("the answer is not a fresh account: %s", rec.Body)
	}

	// Disabling revokes sessions too -- an account that can no longer log in
	// but whose cookie still works has not been disabled in any sense.
	bob, _ := auth.Get(ctx, rig.db, "bob")
	if _, err := auth.CreateSession(ctx, rig.db, bob.ID); err != nil {
		t.Fatal(err)
	}
	rec = rig.call(t, adminScope, "PATCH", "/api/admin/users/bob",
		`{"disabled":true,"is_admin":false}`, "")
	if rec.Code != 200 || body(t, rec)["disabled"] != true {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if body(t, rec)["sessions"] != float64(0) {
		t.Errorf("disabling left %v sessions", body(t, rec)["sessions"])
	}
	if body(t, rec)["state"] != "disabled" {
		t.Errorf("a disabled account reads %v", body(t, rec)["state"])
	}

	rec = rig.call(t, adminScope, "PATCH", "/api/admin/users/bob",
		`{"model_tier":"opus"}`, "")
	if rec.Code != 200 || body(t, rec)["model_tier"] != "opus" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// `null` restores the house default, which is stored as NULL rather than
	// as the default's key.
	rec = rig.call(t, adminScope, "PATCH", "/api/admin/users/bob",
		`{"model_tier":null}`, "")
	if rec.Code != 200 || body(t, rec)["model_tier"] != tiers.DefaultKey {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestPatchRefusesOnTheStatusEachReasonEarns(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()

	for _, c := range []struct {
		name, target, payload string
		want                  int
		says                  string
	}{
		// A name nobody holds. An admin is the one caller for whom every
		// account is in scope, so 404 here means what it says.
		{"no such account", "/api/admin/users/nobody-here", `{"is_admin":true}`, 404,
			"no account 'nobody-here'"},
		// The request is well formed and understood, and conflicts with the
		// state of the world. ADR 17.
		{"the last admin who can sign in", "/api/admin/users/alice", `{"is_admin":false}`,
			409, "only admin who can sign in"},
		{"disabling the last admin", "/api/admin/users/alice", `{"disabled":true}`,
			409, "only admin who can sign in"},
		// A body that changes nothing is a request that means nothing.
		{"nothing to change", "/api/admin/users/bob", `{}`, 422, "nothing to change"},
		{"a change that is not one", "/api/admin/users/bob", `{"is_admin":false}`, 422,
			"nothing to change"},
		// **An unknown tier against an account at the default is "nothing to
		// change", not "no such tier"** -- measured against Python
		// 2026-08-22, and it surprised this test before it surprised anybody
		// else. Both sides are compared *through the roster*, where an
		// unknown key resolves to the default, so the write is never reached
		// and `UnknownTier` never fires. That is the same tolerance a stale
		// column needs, seen from the request's end.
		{"an unknown tier that changes nothing", "/api/admin/users/bob",
			`{"model_tier":"nope"}`, 422, "nothing to change"},
	} {
		rec := rig.call(t, adminScope, "PATCH", c.target, c.payload, "")
		if rec.Code != c.want {
			t.Errorf("%s answered %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
		if !strings.Contains(detail(t, rec), c.says) {
			t.Errorf("%s said %q, which does not mention %q", c.name, detail(t, rec), c.says)
		}
	}

	// ...and once the account is on a tier the unknown key *would* move it
	// off, the write is reached and the core refuses it: 422, because that is
	// a malformed request rather than a conflicting one.
	if rec := rig.call(t, adminScope, "PATCH", "/api/admin/users/bob",
		`{"model_tier":"opus"}`, ""); rec.Code != 200 {
		t.Fatalf("granting opus answered %d", rec.Code)
	}
	for _, c := range []struct {
		name, target, payload string
		want                  int
		says                  string
	}{
		{"an unknown tier that would change something", "/api/admin/users/bob",
			`{"model_tier":"nope"}`, 422, "no such tier: nope"},
	} {
		rec := rig.call(t, adminScope, "PATCH", c.target, c.payload, "")
		if rec.Code != c.want {
			t.Errorf("%s answered %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
		if !strings.Contains(detail(t, rec), c.says) {
			t.Errorf("%s said %q, which does not mention %q", c.name, detail(t, rec), c.says)
		}
	}
	// Alice survived every refusal above.
	alice, _ := auth.Get(context.Background(), rig.db, "alice")
	if !alice.IsAdmin || alice.Disabled {
		t.Fatal("the instance lost its only admin to a refused request")
	}
}

// FastAPI validates a declared body before the handler runs, so a malformed
// body against a name nobody holds is a 422 there -- not the 404 the account
// would earn. The order is the wire's, not a preference.
func TestAMalformedBodyBeatsAMissingAccount(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()

	rec := rig.call(t, adminScope, "PATCH", "/api/admin/users/nobody-here", `[1,2]`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a list body against a missing account answered %d", rec.Code)
	}
	if _, isList := body(t, rec)["detail"].([]any); !isList {
		t.Errorf("that 422 is the handler's, not FastAPI's: %s", rec.Body)
	}
}

// ---- the admin's reset link ------------------------------------------------

func TestTheAdminsResetMailsALinkAndRefusesTheTwoCasesItCannot(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()

	rec := rig.call(t, adminScope, "POST", "/api/admin/users/bob/reset", "", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(detail(t, rec), "on its way to bob") {
		t.Errorf("the answer reads %q", detail(t, rec))
	}
	if len(rig.sender.messages()) != 1 || rig.sender.messages()[0].To != "bob@example.com" {
		t.Fatalf("%d messages went out", len(rig.sender.messages()))
	}

	if rec := rig.call(t, adminScope, "POST", "/api/admin/users/nobody/reset", "", ""); rec.Code != 404 {
		t.Errorf("a missing account answered %d", rec.Code)
	}

	// An account with no address cannot be mailed a link, and the answer names
	// the break-glass path rather than stopping at "no".
	noAddress, err := auth.Create(ctx, rig.db, "silent", "", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = noAddress
	rec = rig.call(t, adminScope, "POST", "/api/admin/users/silent/reset", "", "")
	if rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(detail(t, rec), "mtglab users passwd") {
		t.Errorf("an addressless account answered %d %s", rec.Code, rec.Body)
	}

	// **A disabled account is refused out loud.** The public endpoint declines
	// this silently and correctly; an admin who pressed a button is owed the
	// reason -- a lever the disabled party can undo from their own inbox is
	// not a lever.
	bob, _ := auth.Get(ctx, rig.db, "bob")
	if _, err := auth.SetDisabled(ctx, rig.db, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	rec = rig.call(t, adminScope, "POST", "/api/admin/users/bob/reset", "", "")
	if rec.Code != http.StatusConflict ||
		!strings.Contains(detail(t, rec), "enable the account first") {
		t.Errorf("a disabled account answered %d %s", rec.Code, rec.Body)
	}
	if len(rig.sender.messages()) != 1 {
		t.Errorf("a refusal still sent a message (%d in total)", len(rig.sender.messages()))
	}
}

// ---- revoking sessions -----------------------------------------------------

// The lighter of the two revocations, and the one wanted for a lost laptop:
// the account is still good, the cookies on that machine are not.
func TestRevokingSessionsLeavesTheAccountUsable(t *testing.T) {
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()
	bob, _ := auth.Get(ctx, rig.db, "bob")
	for i := 0; i < 3; i++ {
		if _, err := auth.CreateSession(ctx, rig.db, bob.ID); err != nil {
			t.Fatal(err)
		}
	}

	rec := rig.call(t, adminScope, "DELETE", "/api/admin/users/bob/sessions", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := body(t, rec)
	if got["username"] != "bob" || got["revoked"] != float64(3) {
		t.Errorf("the answer reads %s", rec.Body)
	}
	if n, _ := auth.CountSessionsForUser(ctx, rig.db, bob.ID); n != 0 {
		t.Errorf("%d sessions survived", n)
	}
	// Still good: bob can sign in again immediately.
	if rec := rig.call(t, anonymous, "POST", "/api/auth/login",
		`{"username":"bob","password":"`+goodPassword+`"}`, ""); rec.Code != 200 {
		t.Errorf("revoking sessions disabled the account: %d", rec.Code)
	}
	if rec := rig.call(t, adminScope, "DELETE", "/api/admin/users/nobody/sessions", "", ""); rec.Code != 404 {
		t.Errorf("a missing account answered %d", rec.Code)
	}
}

// ---- an absent database ----------------------------------------------------

// The same rule the public routes keep: an absent `app.db` is read as an empty
// one and nothing here creates it.
func TestTheAdminSurfaceWithNoDatabase(t *testing.T) {
	a := New(Config{AppDBPath: "", DecksDir: t.TempDir(), EmailSender: &recordedSender{}})
	rig := &accountRig{api: a, close: func() {}}

	rec := rig.call(t, adminScope, "GET", "/api/admin/users", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("the list answered %d", rec.Code)
	}
	got := body(t, rec)
	if items, ok := got["users"].([]any); !ok || len(items) != 0 {
		t.Errorf("an instance with no accounts listed %v", got["users"])
	}
	if got["admins"] != float64(0) {
		t.Errorf("admins = %v", got["admins"])
	}
	// The roster is a fact about the *build*, so it answers regardless.
	if roster, ok := got["tiers"].([]any); !ok || len(roster) != len(tiers.All) {
		t.Errorf("the roster is %v", got["tiers"])
	}
	// Every per-account route is a 404, because there is no such account.
	for _, route := range adminRoutes[2:] {
		rec := rig.call(t, adminScope, route.method, route.target, route.payload, "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s answered %d with no database", route.method, route.target, rec.Code)
		}
	}
}
