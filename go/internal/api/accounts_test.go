package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The account routes' own tests. `internal/auth` already proves the *engine*
// against its recorded corpus -- the hashes byte for byte, the last-admin
// guard, the
// token's single use -- so what these prove is the layer above: that a refusal
// lands on the right status with the right sentence, that the cookie carries
// the recorded attributes, and that nothing here leaks what ADR 16 and
// ADR 17 say it must not.
//
// **No test in this file sends mail.** Every one that would passes a
// `recordedSender`, which is ADR 16's seam doing exactly the job it was built
// for.
//
// The sentences matter more here than anywhere else in the app. They
// are answered *verbatim* to a browser, the assertions below are the
// record, and `internal/auth`'s `failf` exists so
// a Go sentinel's own words never get in front of one.

const goodPassword = "correct-horse-battery-staple"

// recordedSender is the seam. It never opens a socket.
//
// The mutex is not decoration. `POST /api/auth/reset` sends from a background
// goroutine, so two resets in flight are two `Send` calls at once -- which the
// race detector proved by tearing this double apart when it had no lock, and
// which is now written into `auth.EmailSender`'s own contract.
type recordedSender struct {
	mu   sync.Mutex
	sent []auth.Message
}

func (r *recordedSender) Send(m auth.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, m)
	return nil
}

// messages is what has been sent so far, copied under the lock so a caller
// cannot read the slice while a background send is growing it.
func (r *recordedSender) messages() []auth.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]auth.Message{}, r.sent...)
}

type accountRig struct {
	api    *API
	db     *sql.DB
	sender *recordedSender
	close  func()
}

// newAccountRig builds an instance with a real `app.db` and three accounts:
// alice administers and can sign in, bob cannot administer, and `waiting`
// holds an unclaimed invite -- which is the account most of the interesting
// rules are about.
func newAccountRig(t *testing.T, requireAuth bool) *accountRig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := auth.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordedSender{}
	a := New(Config{DecksDir: t.TempDir(), AppDB: reader, AppWriteDB: db,
		AppDBPath: path, RequireAuth: requireAuth, EmailSender: sender})

	ctx := context.Background()
	for _, seed := range []struct {
		name, email  string
		admin, claim bool
	}{
		{"alice", "alice@example.com", true, true},
		{"bob", "bob@example.com", false, true},
		{"waiting", "waiting@example.com", false, false},
	} {
		user, err := auth.Create(ctx, db, seed.name, seed.email, seed.admin)
		if err != nil {
			t.Fatalf("seeding %s: %v", seed.name, err)
		}
		if seed.claim {
			if _, err := auth.SetPassword(ctx, db, user.ID, goodPassword); err != nil {
				t.Fatal(err)
			}
		} else if _, err := auth.IssueToken(ctx, db, user.ID, auth.PurposeInvite); err != nil {
			t.Fatal(err)
		}
	}
	return &accountRig{api: a, db: db, sender: sender,
		close: func() { _ = db.Close(); _ = reader.Close() }}
}

// call runs a request and hands back the whole recorder, because half of what
// these routes promise is in the headers.
func (r *accountRig) call(t *testing.T, scope auth.Scope, method, target, body string,
	cookie string) *httptest.ResponseRecorder {

	t.Helper()
	var found *Route
	values := map[string]string{}
	asked, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimPrefix(asked.Path, "/"), "/")
	for _, route := range r.api.Routes() {
		if route.Method != method {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(route.Pattern, "/"), "/")
		if len(segs) != len(parts) {
			continue
		}
		captured := map[string]string{}
		matched := true
		for i, seg := range segs {
			if strings.HasPrefix(seg, "{") {
				name, _, _ := strings.Cut(seg[1:], "}")
				captured[name] = parts[i]
				continue
			}
			if seg != parts[i] {
				matched = false
				break
			}
		}
		if matched {
			route := route
			found, values = &route, captured
			break
		}
	}
	if found == nil {
		t.Fatalf("no route for %s %s", method, target)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body)).
		WithContext(auth.WithScope(context.Background(), scope))
	if body != "" {
		// A body is parsed as JSON only when the content type says so.
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	}
	for name, value := range values {
		req.SetPathValue(name, value)
	}
	rec := httptest.NewRecorder()
	found.Handler(rec, req)
	return rec
}

func body(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("the body is not a JSON object: %s", rec.Body.String())
	}
	return parsed
}

func detail(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	got, _ := body(t, rec)["detail"].(string)
	return got
}

// maskedCookie normalises a Set-Cookie header for comparison: the attributes
// lowercased and sorted, the value dropped. The recorded expectations below
// are written in this exact form, so the masker is what lets a test compare
// against them rather than against a guess.
func maskedCookie(header string) string {
	first, rest, _ := strings.Cut(header, ";")
	name, _, _ := strings.Cut(first, "=")
	var attrs []string
	for _, attr := range strings.Split(rest, ";") {
		attr = strings.ToLower(strings.TrimSpace(attr))
		if attr == "" {
			continue
		}
		if strings.HasPrefix(attr, "expires=") {
			attr = "expires=*"
		}
		attrs = append(attrs, attr)
	}
	sort.Strings(attrs)
	return name + "=*; " + strings.Join(attrs, "; ")
}

var anonymous = auth.Scope{}

// ---- the caller's address --------------------------------------------------

// `clientAddress` returns an address or nothing, and the "or nothing" is the
// part with a story: CodeQL flagged the header-echoing shape of this
// function the moment it was written -- "sensitive data returned by HTTP
// request headers flows to a logging call" -- and it was right twice over. The
// environment names a *header*, not a value, so a misconfiguration would put a
// credential in every auth log line; and the value was otherwise unbounded, so
// a client behind the proxy could put a kilobyte into the log and into a
// rate-limit key on every request.
func TestOnlyAnActualAddressIsTrustedFromAHeader(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, header, value, want string
	}{
		{"no header configured", "", "", "192.0.2.1"},
		{"a plain address", "Fly-Client-IP", "203.0.113.9", "203.0.113.9"},
		{"a chain, whose first entry is the client", "X-Forwarded-For",
			"203.0.113.9, 70.41.3.18, 150.172.238.178", "203.0.113.9"},
		{"an IPv6 address", "Fly-Client-IP", "2001:db8::1", "2001:db8::1"},
		{"whitespace around it", "Fly-Client-IP", "  203.0.113.9  ", "203.0.113.9"},
		// The three the guard exists for. Each falls back to the peer rather
		// than becoming a log line and a rate-limit key of its own.
		{"a header that is not an address", "Fly-Client-IP", "not-an-address", "192.0.2.1"},
		{"a header carrying a credential", "Fly-Client-IP",
			"Bearer sk-not-a-real-secret-but-shaped-like-one", "192.0.2.1"},
		{"a header carrying a kilobyte", "Fly-Client-IP",
			strings.Repeat("x", 1024), "192.0.2.1"},
		{"an empty header", "Fly-Client-IP", "", "192.0.2.1"},
	} {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		if c.header != "" {
			req.Header.Set(c.header, c.value)
		}
		if got := clientAddress(req, c.header); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// And a peer that is not an address either -- which `httptest` cannot
	// produce but a unix socket can -- is "unknown" rather than whatever
	// arrived.
	req := httptest.NewRequest("POST", "/api/auth/login", nil)
	req.RemoteAddr = "@"
	if got := clientAddress(req, ""); got != "unknown" {
		t.Errorf("a peer with no address is %q", got)
	}
}

// ---- login -----------------------------------------------------------------

func TestLoginHandsBackTheCookieTheGoldenRecords(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	rec := rig.call(t, anonymous, "POST", "/api/auth/login",
		`{"username":"alice","password":"`+goodPassword+`"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// The recorded login cookie. The attributes are the
	// contract; the value never is. No `secure` here because this rig's
	// cookies are not secure, exactly as a laptop runs it.
	const want = "sid=*; httponly; max-age=1209600; path=/; samesite=lax"
	if got := maskedCookie(rec.Header().Get("Set-Cookie")); got != want {
		t.Errorf("Set-Cookie is\n  %s\nand the record says\n  %s", got, want)
	}

	// The body is `{"user": …}` and the user carries **no address** -- ADR 17
	// permits one only in a response an admin authenticated for, and this is
	// the account's own login.
	user, ok := body(t, rec)["user"].(map[string]any)
	if !ok {
		t.Fatalf("no user in %s", rec.Body)
	}
	if _, leaked := user["email"]; leaked {
		t.Error("login serialised the account's address")
	}
	for _, key := range []string{"id", "username", "is_admin", "disabled",
		"created_at", "model_tier"} {
		if _, present := user[key]; !present {
			t.Errorf("the user has no %q: %v", key, user)
		}
	}

	// And the session is real: the token in the cookie resolves to alice.
	token := ""
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			token = c.Value
		}
	}
	scope, err := auth.Resolve(context.Background(), rig.db, token)
	if err != nil || !scope.Authenticated || scope.Username != "alice" {
		t.Fatalf("the cookie does not resolve to alice: %+v (%v)", scope, err)
	}
}

func TestASecureInstanceMarksTheCookieSecure(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	rig.api.secureCookies = true

	rec := rig.call(t, anonymous, "POST", "/api/auth/login",
		`{"username":"alice","password":"`+goodPassword+`"}`, "")
	if !strings.Contains(maskedCookie(rec.Header().Get("Set-Cookie")), "secure") {
		t.Errorf("a deployed instance's cookie is not Secure: %s",
			rec.Header().Get("Set-Cookie"))
	}
}

// Every refusal is the same status and the same sentence. A login form that
// says "that account is disabled" is a login form that confirms the account
// exists to anybody who asks.
func TestEveryLoginRefusalIsIndistinguishable(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()
	gone, err := auth.Get(ctx, rig.db, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.SetDisabled(ctx, rig.db, gone.ID, true); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, c := range []struct{ name, payload string }{
		{"an unknown account", `{"username":"nobody","password":"` + goodPassword + `"}`},
		{"a wrong password", `{"username":"alice","password":"wrong-but-long-enough"}`},
		{"an unclaimed invite", `{"username":"waiting","password":"` + goodPassword + `"}`},
		{"a disabled account", `{"username":"bob","password":"` + goodPassword + `"}`},
	} {
		rec := rig.call(t, anonymous, "POST", "/api/auth/login", c.payload, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s answered %d", c.name, rec.Code)
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s set a cookie", c.name)
		}
		seen[detail(t, rec)] = true
	}
	if len(seen) != 1 {
		t.Errorf("four refusals said %d different things: %v", len(seen), seen)
	}
	for sentence := range seen {
		if sentence != "invalid username or password" {
			t.Errorf("the refusal reads %q", sentence)
		}
	}
}

func TestLoginRefusesAnIncompleteBodyBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	for _, payload := range []string{`{}`, `{"username":"alice"}`,
		`{"password":"x"}`, `{"username":"","password":"y"}`,
		// `str(x or "")`: the `or` runs before the `str`, so a falsy value is
		// the empty string rather than "0" or "False".
		`{"username":0,"password":false}`} {
		rec := rig.call(t, anonymous, "POST", "/api/auth/login", payload, "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d", payload, rec.Code)
		}
		if got := detail(t, rec); got != "username and password are required" {
			t.Errorf("%s said %q", payload, got)
		}
	}
	// A body that is not an object at all is the validation 422, whose
	// `detail` is a *list* rather than a sentence.
	rec := rig.call(t, anonymous, "POST", "/api/auth/login", `[1,2]`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a list body answered %d", rec.Code)
	}
	if _, isList := body(t, rec)["detail"].([]any); !isList {
		t.Errorf("a validation failure's detail is not a list: %s", rec.Body)
	}
}

func TestTheLoginBudgetAnswers429WithRetryAfter(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	wrong := `{"username":"alice","password":"wrong-but-long-enough"}`
	for i := 0; i < auth.PerAccount.Failures; i++ {
		if rec := rig.call(t, anonymous, "POST", "/api/auth/login", wrong, ""); rec.Code != 401 {
			t.Fatalf("attempt %d answered %d", i, rec.Code)
		}
	}
	rec := rig.call(t, anonymous, "POST", "/api/auth/login", wrong, "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("past the budget the answer is %d", rec.Code)
	}
	// The login form counts this down, so it has to be whole positive seconds.
	retry := rec.Header().Get("Retry-After")
	seconds, err := strconv.Atoi(retry)
	if err != nil || seconds <= 0 {
		t.Errorf("Retry-After is %q", retry)
	}
	if got := detail(t, rec); got != "too many attempts -- wait and try again" {
		t.Errorf("the throttle reads %q", got)
	}
	// Throttled means throttled: the *right* password is refused too, and it
	// is refused without saying which budget was hit.
	right := `{"username":"alice","password":"` + goodPassword + `"}`
	if rec := rig.call(t, anonymous, "POST", "/api/auth/login", right, ""); rec.Code != 429 {
		t.Errorf("the correct password past the budget answered %d", rec.Code)
	}

	// And a success clears it. Spend the budget on one account, then log in as
	// another from the same address -- the per-address budget is three times
	// the per-account one, so it is still open.
	other := `{"username":"bob","password":"` + goodPassword + `"}`
	if rec := rig.call(t, anonymous, "POST", "/api/auth/login", other, ""); rec.Code != 200 {
		t.Fatalf("bob could not sign in while alice was throttled: %d", rec.Code)
	}
}

// Session fixation: whatever token arrived is destroyed, and the one that
// leaves is new.
func TestLoginDestroysTheTokenThatArrived(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()
	alice, err := auth.Get(ctx, rig.db, "alice")
	if err != nil {
		t.Fatal(err)
	}
	planted, err := auth.CreateSession(ctx, rig.db, alice.ID)
	if err != nil {
		t.Fatal(err)
	}

	rec := rig.call(t, anonymous, "POST", "/api/auth/login",
		`{"username":"alice","password":"`+goodPassword+`"}`, planted)
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	scope, err := auth.Resolve(ctx, rig.db, planted)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Authenticated {
		t.Error("the token that arrived is still valid after the login")
	}
}

// ---- logout ----------------------------------------------------------------

func TestLogoutClearsTheCookieAndTheRow(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()
	alice, _ := auth.Get(ctx, rig.db, "alice")
	token, err := auth.CreateSession(ctx, rig.db, alice.ID)
	if err != nil {
		t.Fatal(err)
	}

	rec := rig.call(t, auth.Scope{UserID: alice.ID, Username: "alice", IsAdmin: true,
		Authenticated: true}, "POST", "/api/auth/logout", "", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if body(t, rec)["authenticated"] != false {
		t.Errorf("logout said %s", rec.Body)
	}
	// The recorded deletion cookie carries neither `httponly` nor
	// `secure` -- which is not the mirror of the one
	// login sets, and is what the record says.
	const want = "sid=*; expires=*; max-age=0; path=/; samesite=lax"
	if got := maskedCookie(rec.Header().Get("Set-Cookie")); got != want {
		t.Errorf("Set-Cookie is\n  %s\nand the record says\n  %s", got, want)
	}
	if scope, _ := auth.Resolve(ctx, rig.db, token); scope.Authenticated {
		t.Error("the session row outlived the logout")
	}
}

// Public, and a no-op when there is none: a 401 on logout is a confusing
// answer to "get me out of here".
func TestLogoutOfNothingSucceeds(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	rec := rig.call(t, anonymous, "POST", "/api/auth/logout", "", "")
	if rec.Code != http.StatusOK || body(t, rec)["authenticated"] != false {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

// With auth off the row is deliberately left alone -- nothing reads it, and
// the local app is one person who has not asked for a database to be touched.
// The require flag guards the delete; this is it holding.
func TestLogoutLeavesTheRowAloneWithAuthOff(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, false)
	defer rig.close()
	ctx := context.Background()
	alice, _ := auth.Get(ctx, rig.db, "alice")
	token, err := auth.CreateSession(ctx, rig.db, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec := rig.call(t, auth.Local, "POST", "/api/auth/logout", "", token); rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	if scope, _ := auth.Resolve(ctx, rig.db, token); !scope.Authenticated {
		t.Error("with auth off, logout deleted a session row it must leave alone")
	}
}

// ---- me --------------------------------------------------------------------

// The three flags are separate on purpose: a frontend needs to tell "you are
// logged out of an instance that wants a login" from "this instance has no
// login", and one collapsed boolean makes the local app render a sign-in form
// it has no server for.
func TestMeAnswersTheThreeStatesApart(t *testing.T) {
	t.Parallel()
	signedIn := auth.Scope{UserID: 1, Username: "alice", IsAdmin: true, Authenticated: true}
	for _, c := range []struct {
		name         string
		requireAuth  bool
		scope        auth.Scope
		wantRequired bool
		wantAuthed   bool
		wantAdmin    bool
		wantUser     bool
	}{
		{"auth off, nobody in particular", false, auth.Local, false, false, true, false},
		{"auth on, anonymous", true, anonymous, true, false, false, false},
		{"auth on, signed in", true, signedIn, true, true, true, true},
	} {
		rig := newAccountRig(t, c.requireAuth)
		rec := rig.call(t, c.scope, "GET", "/api/auth/me", "", "")
		rig.close()
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", c.name, rec.Code)
		}
		got := body(t, rec)
		if got["auth_required"] != c.wantRequired || got["authenticated"] != c.wantAuthed ||
			got["is_admin"] != c.wantAdmin {
			t.Errorf("%s: %s", c.name, rec.Body)
		}
		user, isObject := got["user"].(map[string]any)
		if isObject != c.wantUser {
			t.Errorf("%s: user is %v", c.name, got["user"])
			continue
		}
		if c.wantUser && (user["username"] != "alice" || user["is_admin"] != true) {
			t.Errorf("%s: user is %v", c.name, user)
		}
		// `me` is public and answers about the caller and nobody else. It has
		// never carried an address and must not start.
		if _, leaked := user["email"]; leaked {
			t.Errorf("%s: me serialised an address", c.name)
		}
	}
}

// ---- reset -----------------------------------------------------------------

// ADR 16: the same answer whether or not the address exists, and the lookup
// happens where nobody is timing it.
func TestResetAnswersIdenticallyAndSendsAfterwards(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	hit := rig.call(t, anonymous, "POST", "/api/auth/reset",
		`{"email":"alice@example.com"}`, "")
	miss := rig.call(t, anonymous, "POST", "/api/auth/reset",
		`{"email":"nobody@example.com"}`, "")
	rig.api.WaitBackground()

	for _, rec := range []*httptest.ResponseRecorder{hit, miss} {
		if rec.Code != http.StatusAccepted {
			t.Errorf("a reset answered %d, not 202", rec.Code)
		}
	}
	if hit.Body.String() != miss.Body.String() {
		t.Errorf("a hit and a miss answered differently:\n  %s\n  %s", hit.Body, miss.Body)
	}
	if got := detail(t, hit); !strings.HasPrefix(got, "if that address has an account") {
		t.Errorf("the reset answer reads %q", got)
	}
	// The message went out *after* the response, and only for the address that
	// resolves. The client cannot see either fact, which is the point.
	if len(rig.sender.messages()) != 1 {
		t.Fatalf("%d messages went out for one real address and one fiction",
			len(rig.sender.messages()))
	}
	if rig.sender.messages()[0].To != "alice@example.com" {
		t.Errorf("the message went to %q", rig.sender.messages()[0].To)
	}
	if !strings.Contains(rig.sender.messages()[0].Body, "/auth/claim#token=") {
		t.Error("the link is not the fragment form; a query string reaches every access log")
	}
}

func TestResetRefusesAnEmptyAddressAndCountsEveryRequest(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	rec := rig.call(t, anonymous, "POST", "/api/auth/reset", `{}`, "")
	if rec.Code != http.StatusUnprocessableEntity ||
		detail(t, rec) != "an email is required" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}

	// Every request is counted, hit or miss -- a reset has no success to clear
	// the budget with.
	for i := 0; i < auth.ResetPerMailbox.Failures; i++ {
		if rec := rig.call(t, anonymous, "POST", "/api/auth/reset",
			`{"email":"alice@example.com"}`, ""); rec.Code != 202 {
			t.Fatalf("request %d answered %d", i, rec.Code)
		}
	}
	rec = rig.call(t, anonymous, "POST", "/api/auth/reset",
		`{"email":"alice@example.com"}`, "")
	rig.api.WaitBackground()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("past the mailbox budget the answer is %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a 429 must carry Retry-After")
	}
}

// ---- claim -----------------------------------------------------------------

func TestClaimSetsAPasswordAndDoesNotSignYouIn(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	token := rig.inviteToken(t, "waiting")

	rec := rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`","username":"newname"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := body(t, rec)
	if got["username"] != "newname" || got["detail"] != "password set -- you can sign in now" {
		t.Errorf("claim answered %s", rec.Body)
	}
	// **It does not log you in.** A cookie here would make an emailed link a
	// session-minting endpoint, and what it would save is one trip through a
	// login form that has to work anyway.
	if rec.Header().Get("Set-Cookie") != "" {
		t.Error("claim set a session cookie")
	}
	// It also never answers with an address: the holder knows their own, and
	// ADR 16 does not put one in a response nobody authenticated for.
	if _, leaked := got["email"]; leaked {
		t.Error("claim serialised an address")
	}
	// And the account can now sign in under its new handle.
	rec = rig.call(t, anonymous, "POST", "/api/auth/login",
		`{"username":"newname","password":"`+goodPassword+`"}`, "")
	if rec.Code != 200 {
		t.Errorf("the claimed account cannot sign in: %d %s", rec.Code, rec.Body)
	}
}

func (r *accountRig) inviteToken(t *testing.T, username string) string {
	t.Helper()
	ctx := context.Background()
	user, err := auth.Get(ctx, r.db, username)
	if err != nil || user == nil {
		t.Fatalf("no fixture account %q (%v)", username, err)
	}
	token, err := auth.IssueToken(ctx, r.db, user.ID, auth.PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestClaimRefusesOnTheStatusEachReasonEarns(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	token := rig.inviteToken(t, "waiting")

	for _, c := range []struct {
		name    string
		payload string
		want    int
	}{
		{"no token", `{"password":"` + goodPassword + `"}`, 422},
		{"no password", `{"token":"x"}`, 422},
		// A bad link is a 400 and it is counted: a stream of them is somebody
		// guessing.
		{"an unknown token", `{"token":"nope","password":"` + goodPassword + `"}`, 400},
		// A password the floor refuses is **not** a refusal of the link, so
		// nothing is spent and the sensible next move is a longer one.
		{"a short password", `{"token":"` + token + `","password":"short"}`, 422},
		// Nor is a handle the rules refuse.
		{"an impossible handle", `{"token":"` + token + `","password":"` +
			goodPassword + `","username":"not a handle"}`, 422},
		// A taken handle is a 409 and the link survives it.
		{"a taken handle", `{"token":"` + token + `","password":"` +
			goodPassword + `","username":"alice"}`, 409},
	} {
		rec := rig.call(t, anonymous, "POST", "/api/auth/claim", c.payload, "")
		if rec.Code != c.want {
			t.Errorf("%s answered %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
		if detail(t, rec) == "" {
			t.Errorf("%s answered with no sentence: %s", c.name, rec.Body)
		}
	}
	// After all of that the invite is still live, which is the property the
	// transaction in `RedeemToken` was written to buy.
	rec := rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`"}`, "")
	if rec.Code != 200 {
		t.Fatalf("the invite did not survive five refusals: %d %s", rec.Code, rec.Body)
	}
	// Single use: the second click says so.
	rec = rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`"}`, "")
	if rec.Code != 400 || !strings.Contains(detail(t, rec), "already been used") {
		t.Errorf("the second click answered %d %s", rec.Code, rec.Body)
	}
}

// A reset link cannot rename an account -- otherwise "somebody got into my
// email" and "somebody took over my identity here" are the same incident.
func TestAResetLinkCannotRenameAndIsNotSpentTrying(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()
	bob, _ := auth.Get(ctx, rig.db, "bob")
	token, err := auth.IssueToken(ctx, rig.db, bob.ID, auth.PurposeReset)
	if err != nil {
		t.Fatal(err)
	}

	rec := rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`","username":"robert"}`, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("renaming from a reset answered %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(detail(t, rec), "cannot change your username") {
		t.Errorf("the refusal reads %q", detail(t, rec))
	}
	// Not spent: the holder is not punished for their client's bug.
	rec = rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`"}`, "")
	if rec.Code != 200 {
		t.Errorf("the reset link was spent by a refused rename: %d", rec.Code)
	}
}

// ---- claim/preview ---------------------------------------------------------

func TestPreviewSaysWhatKindOfLinkAndNothingMore(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	token := rig.inviteToken(t, "waiting")

	rec := rig.call(t, anonymous, "POST", "/api/auth/claim/preview",
		`{"token":"`+token+`"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := body(t, rec)
	if got["purpose"] != "invite" || got["username"] != "waiting" {
		t.Errorf("preview said %s", rec.Body)
	}
	// Deliberately not the address: the holder knows their own, and a preview
	// is a response nobody authenticated for.
	if _, leaked := got["email"]; leaked {
		t.Error("preview serialised an address")
	}
	// **It spends nothing.** A valid link previewed ten times is still valid.
	for i := 0; i < 10; i++ {
		if rec := rig.call(t, anonymous, "POST", "/api/auth/claim/preview",
			`{"token":"`+token+`"}`, ""); rec.Code != 200 {
			t.Fatalf("preview %d answered %d", i, rec.Code)
		}
	}
	if rec := rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`"}`, ""); rec.Code != 200 {
		t.Errorf("ten previews spent the link: %d", rec.Code)
	}
}

func TestPreviewRefusesABadLinkAndADisabledAccountAlike(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	ctx := context.Background()

	rec := rig.call(t, anonymous, "POST", "/api/auth/claim/preview", `{}`, "")
	if rec.Code != 422 || detail(t, rec) != "a token is required" {
		t.Errorf("an empty body answered %d %s", rec.Code, rec.Body)
	}
	rec = rig.call(t, anonymous, "POST", "/api/auth/claim/preview", `{"token":"nope"}`, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown token answered %d", rec.Code)
	}

	bob, _ := auth.Get(ctx, rig.db, "bob")
	token, err := auth.IssueToken(ctx, rig.db, bob.ID, auth.PurposeReset)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.SetDisabled(ctx, rig.db, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	rec = rig.call(t, anonymous, "POST", "/api/auth/claim/preview",
		`{"token":"`+token+`"}`, "")
	if rec.Code != http.StatusBadRequest || detail(t, rec) != "that link is not valid" {
		t.Errorf("a disabled account's link answered %d %s", rec.Code, rec.Body)
	}
}

// ---- an absent database ----------------------------------------------------

// An absent `app.db` is read as an **empty** one, never created -- the
// recorded contract: the first login on such an instance answers 401,
// because an empty `users` table has nobody in it. See
// `internal/auth/writes.go`.
func TestWithNoDatabaseTheAnswersAreTheEmptyOnes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	a := New(Config{AppDBPath: path, DecksDir: dir, RequireAuth: false,
		EmailSender: &recordedSender{}})
	rig := &accountRig{api: a, close: func() {}}
	defer rig.close()

	for _, c := range []struct {
		name, method, target, payload string
		want                          int
	}{
		{"login", "POST", "/api/auth/login",
			`{"username":"alice","password":"` + goodPassword + `"}`, 401},
		{"reset", "POST", "/api/auth/reset", `{"email":"a@example.com"}`, 202},
		{"claim", "POST", "/api/auth/claim",
			`{"token":"x","password":"` + goodPassword + `"}`, 400},
		{"preview", "POST", "/api/auth/claim/preview", `{"token":"x"}`, 400},
		{"me", "GET", "/api/auth/me", "", 200},
		{"logout", "POST", "/api/auth/logout", "", 200},
	} {
		rec := rig.call(t, auth.Local, c.method, c.target, c.payload, "")
		if rec.Code != c.want {
			t.Errorf("%s answered %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
	}
	a.WaitBackground()
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a route created app.db; only the boot ladder may make the file")
	}
}
