package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The account routes: the five doors and the `me` that says who is
// standing in them.
//
// The **middleware** half of the auth story lives in
// `internal/door`; this is the other half. All six are on
// `door.PublicPaths`,
// which is the load-bearing fact about them: every one is reachable with no
// session, so each is rate limited and each is written so that a refusal tells
// the caller nothing it did not already know.
//
// Four properties are worth finding here rather than in a review, and each
// is a standing decision:
//
//   - **`login` answers one thing for every refusal.** Unknown account, wrong
//     password, unclaimed invite, disabled account: 401, one sentence, and one
//     Argon2 verification spent either way (`auth.Authenticate`). It also never
//     reports *which* of the two budgets a 429 came from, for the same reason.
//   - **`reset` answers the same thing whether or not the address exists**, and
//     the lookup and the send both happen *after* the response has gone -- so
//     "no such address" is not merely un-said, it is un-timeable. `SendReset`
//     cannot report which case it was, which is what stops a later edit from
//     branching on it.
//   - **`claim` resolves the token before it hashes anything.** It is
//     unauthenticated and Argon2 costs 19 MiB a call, so an endpoint that
//     hashed first would be a denial of service with a password field on it.
//   - **`claim` does not log you in.** A cookie there would make an emailed
//     link a session-minting endpoint, and what it would save is one trip
//     through a login form that has to work anyway.
//
// With auth *off* these routes still exist and still work, deliberately:
// `login` opens `app.db`, verifies a password and hands back a
// cookie that nothing then checks. That is how the flow gets exercised against
// a real browser on a laptop. What it must never become is a route that
// *grants* something locally, and it does not -- with auth off every caller
// already has full access, so a session confers nothing.

// cookieName is the session cookie, and the same
// name `internal/door` reads to resolve a caller.
const cookieName = "sid"

// resetAnswer is the only thing `POST /api/auth/reset` ever says.
//
// A constant rather than a literal in the handler, because the one way this
// endpoint leaks is by answering two different things, and a single name is
// harder to fork than a string typed twice.
const resetAnswer = "if that address has an account, a link is on its way -- " +
	"check your mail, and the spam folder"

// accountsDB is the read-write `app.db` handle the account routes use, and
// whether there is one.
//
// It prefers the handle the door opened at start and otherwise opens one
// lazily, which is not tidiness: on a fresh volume `app.db` can appear
// after boot -- minted by the CLI, or by a ladder run the door started
// without. The lazy open is what makes the first login after that work
// rather than answering out of a decision taken minutes earlier.
//
// `false` means there is no database, and every caller below then answers as
// though there were an **empty** one -- see `internal/auth/writes.go` for why
// that is the honest answer and not a shortcut, and why nothing here creates
// the file.
func (a *API) accountsDB() (*sql.DB, bool) {
	if a.writeDB != nil {
		return a.writeDB, true
	}
	a.lazy.Lock()
	defer a.lazy.Unlock()
	if a.lazyWriteDB != nil {
		return a.lazyWriteDB, true
	}
	if a.dbPath == "" {
		return nil, false
	}
	if _, err := os.Stat(a.dbPath); err != nil {
		return nil, false
	}
	db, err := auth.OpenReadWrite(a.dbPath)
	if err != nil {
		a.log.Warn("app.db exists but could not be opened for writing", "error", err)
		return nil, false
	}
	a.lazyWriteDB = db
	return db, true
}

// clientAddress is the caller's address, for rate limiting and the auth log.
//
// It reads a header only when `MTGLAB_CLIENT_IP_HEADER` names one, because a
// header any client can set is a rate limit any client can opt out of.
//
// **And it returns an address or nothing**, one step stricter than
// echoing the header, and deliberately so. CodeQL flagged the echoing
// shape when this was first written -- "sensitive data returned by HTTP
// request headers flows to a logging call" -- and it is right on two counts
// that are not hypothetical. The variable names a header rather than a value, so a
// misconfiguration (`MTGLAB_CLIENT_IP_HEADER=Authorization`) would put a
// credential in every auth log line; and the value is otherwise unbounded, so
// a client behind the proxy could put a kilobyte into the log and into a
// rate-limit key on every request.
//
// `net.ParseIP` is the whole guard: a header that does not carry an address is
// not carrying what this function is for, and the peer is the honest fallback.
// The parsed form is re-serialised rather than the input echoed, so what is
// logged is an address this process constructed. The one visible cost of
// the strictness is that a garbage header never becomes a rate-limit key of
// its own -- which was never a key worth having.
func clientAddress(r *http.Request) string {
	if header := config.ClientIPHeader(); header != "" {
		// `X-Forwarded-For` is a chain; the client is the first entry.
		first, _, _ := strings.Cut(r.Header.Get(header), ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip.String()
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return "unknown"
}

// loggable is a failed login's principal, safe to write to a log line.
//
// Usernames cannot contain `@` (`auth.UsernamePattern`), so anything that does
// is somebody typing their email address into the username box -- and ADR 16
// is unconditional that an address must never reach a log line. The domain is
// kept because it is the part that helps and the part that is not personal.
//
// Redacting rather than dropping the field: "who is failing to log in" is the
// question these lines exist to answer, and `<redacted>@example.com` still
// answers "is this one person or a script working through a list".
func loggable(username string) string {
	_, domain, found := strings.Cut(username, "@")
	if !found {
		return username
	}
	return "<redacted>@" + domain
}

// falsy is the recorded truthiness, negated -- for the or-empty idiom every
// account handler reads its fields through: the falsiness check runs
// *before* the stringification, so `0` and `false` arrive as the empty
// string rather than as "0" and "False".
func falsy(v any) bool {
	switch value := v.(type) {
	case nil:
		return true
	case bool:
		return !value
	case string:
		return value == ""
	case json.Number:
		f, err := value.Float64()
		return err == nil && f == 0
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	default:
		return false
	}
}

// field is the or-empty read: a falsy value is "", anything else is `str`.
func field(body map[string]any, key string) string {
	if falsy(body[key]) {
		return ""
	}
	return str(body, key)
}

// setSessionCookie is the browser's half of a session.
//
// `HttpOnly` keeps it out of JavaScript, `Secure` follows
// `MTGLAB_SECURE_COOKIES` (on once deployed), and `SameSite=Lax` is the CSRF
// defence -- it stops the cross-site form post, and the only state-changing
// routes here take a JSON body, which a cross-origin caller cannot send
// without a preflight the app does not answer. **If that policy is ever
// relaxed to `None`, a double-submit token becomes required**; the note is
// here because that change would look innocuous.
//
// gosec wants `Secure` unconditionally, and it is right about deployments and
// wrong about this one: with auth off the app is `http://127.0.0.1`, where a
// Secure cookie is simply never sent back and the login appears to succeed and
// then not work. `config.SecureCookies` defaults to `RequireAuth` for exactly
// that reason, so the deployed instance gets the attribute and the laptop gets
// a session that functions.
func (a *API) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows MTGLAB_SECURE_COOKIES; see above
		Name:     cookieName,
		Value:    token,
		MaxAge:   int(auth.Lifetime.Seconds()),
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

// clearSessionCookie writes the recorded deletion cookie, attribute
// for attribute -- which is deliberately **not** the mirror of the one above:
// the recorded deletion carries neither `HttpOnly` nor `Secure`. `Expires`
// is the moment of the request rather than
// the epoch, the recorded rendering of an immediate expiry.
//
// gosec objects to both missing attributes, and the answer is that this cookie
// carries **no value**: it exists to unset one. `HttpOnly` protects a secret
// from JavaScript and `Secure` keeps it off a plaintext hop; an empty string
// with `Max-Age=0` has nothing to protect, and adding either would make the
// header differ from the recorded one without making
// anything safer.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // an empty deletion cookie; see above
		Name:     cookieName,
		Value:    "",
		MaxAge:   -1, // rendered as `Max-Age=0`
		Expires:  time.Now().UTC(),
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

// budget is one rate-limit key and the limit it is counted against.
type budget struct {
	key   string
	limit auth.Limit
}

// throttle answers 429 with `Retry-After` if any budget is spent, and reports
// whether it did. A read; it records nothing.
//
// Which budget was hit is never reported, for the same reason `login` never
// reports which half of the credentials was wrong.
func (a *API) throttle(w http.ResponseWriter, r *http.Request, db *sql.DB,
	detail string, budgets ...budget) bool {

	for _, b := range budgets {
		spent, err := auth.Exhausted(r.Context(), db, b.key, b.limit)
		if err != nil {
			a.log.Warn("the rate limiter could not be read", "error", err)
			return false
		}
		if !spent {
			continue
		}
		wait, err := auth.RetryAfter(r.Context(), db, b.key, b.limit)
		if err != nil {
			wait = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		wire.Detail(w, http.StatusTooManyRequests, detail)
		return true
	}
	return false
}

// spend counts one failure against every budget. A failure to record is a
// logged warning and never a refused request: the limiter is a guard, and one
// that could 500 a login would be worse than the attack it prevents.
func (a *API) spend(ctx context.Context, db *sql.DB, budgets ...budget) {
	for _, b := range budgets {
		if _, err := auth.RecordFailure(ctx, db, b.key, b.limit); err != nil {
			a.log.Warn("the rate limiter could not be written", "error", err)
		}
	}
}

// forgive clears every budget. Called on a success.
func (a *API) forgive(ctx context.Context, db *sql.DB, budgets ...budget) {
	for _, b := range budgets {
		if err := auth.ClearLimit(ctx, db, b.key); err != nil {
			a.log.Warn("the rate limiter could not be cleared", "error", err)
		}
	}
}

// ---- the six routes --------------------------------------------------------

// login is `POST /api/auth/login`: a username and a password for a session
// cookie.
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	username, password := field(body, "username"), field(body, "password")
	username = strings.TrimSpace(username)
	address := clientAddress(r)
	if username == "" || password == "" {
		wire.Detail(w, http.StatusUnprocessableEntity,
			"username and password are required")
		return
	}

	db, present := a.accountsDB()
	if !present {
		// An empty database has nobody in it, and one Argon2 verification is
		// still spent so that this answer costs what a real refusal costs.
		auth.VerifyDummy(password)
		a.log.Warn("failed login with no accounts database",
			"username", loggable(username), "address", address)
		wire.Detail(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	budgets := []budget{
		{auth.AccountKey(username), auth.PerAccount},
		{auth.AddressKey(address, "login"), auth.PerAddress},
	}
	if a.throttle(w, r, db, "too many attempts -- wait and try again", budgets...) {
		a.log.Warn("login throttled", "username", loggable(username), "address", address)
		return
	}

	user, err := auth.Authenticate(r.Context(), db, username, password)
	if err != nil {
		a.refuse(w, "login", err)
		return
	}
	if user == nil {
		a.spend(r.Context(), db, budgets...)
		// Username and address, never the password and never a full email
		// address -- see `loggable`.
		a.log.Warn("failed login", "username", loggable(username), "address", address)
		wire.Detail(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	a.forgive(r.Context(), db, budgets...)

	// Session fixation: whatever token arrived is destroyed, and the one that
	// leaves is new. A token an attacker planted before the login is not the
	// token that is valid after it.
	if stale, err := r.Cookie(cookieName); err == nil && stale.Value != "" {
		if err := auth.DeleteSession(r.Context(), db, stale.Value); err != nil {
			a.log.Warn("the stale session could not be ended", "error", err)
		}
	}
	token, err := auth.CreateSession(r.Context(), db, user.ID)
	if err != nil {
		a.refuse(w, "login", err)
		return
	}

	a.setSessionCookie(w, token)
	a.log.Info("login", "username", user.Username, "address", address)
	wire.JSON(w, http.StatusOK, map[string]any{"user": user.AsDict(false)})
}

// logout is `POST /api/auth/logout`: end this session.
//
// Public, and a no-op when there is none -- because a 401 on logout is a
// confusing answer to "get me out of here", and the honest response to logging
// out of nothing is that you are logged out.
//
// The session row is deleted only when auth is **on**, the recorded rule:
// with auth off nothing reads the row anyway, and the local app is
// one person who has not asked for a database to be touched.
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" && a.requireAuth {
		if db, present := a.accountsDB(); present {
			if err := auth.DeleteSession(r.Context(), db, cookie.Value); err != nil {
				a.log.Warn("the session could not be ended", "error", err)
			}
		}
	}
	clearSessionCookie(w)
	wire.JSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

// requestReset is `POST /api/auth/reset`: ask for a password-reset link.
// Always the same answer (ADR 16).
//
// 202 rather than 200, and the code is the honest one: the request has been
// accepted and nothing about the outcome is being reported. That is the whole
// design, not a limitation of it.
//
// The 429 is not an exception to "always the same answer". Every request is
// counted, hit or miss -- a reset has no success to clear the budget with --
// so being throttled says something about how often *this* client and *this*
// mailbox have been asked about, and nothing about whether an account is
// behind it.
func (a *API) requestReset(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	email := strings.TrimSpace(field(body, "email"))
	if email == "" {
		wire.Detail(w, http.StatusUnprocessableEntity, "an email is required")
		return
	}
	address := clientAddress(r)

	if db, present := a.accountsDB(); present {
		budgets := []budget{
			{auth.EmailKey(email), auth.ResetPerMailbox},
			{auth.AddressKey(address, "reset"), auth.ResetPerAddress},
		}
		if a.throttle(w, r, db, "too many requests -- wait and try again", budgets...) {
			a.log.Warn("password reset throttled", "address", address)
			return
		}
		a.spend(r.Context(), db, budgets...)
	}

	a.background(func(ctx context.Context) { a.deliverReset(ctx, email) })
	// No address, and no "for ada@example.com". ADR 16 keeps them out of logs;
	// this line is where the temptation to be helpful would put one.
	a.log.Info("password reset requested", "address", address)
	wire.JSON(w, http.StatusAccepted, map[string]any{"detail": resetAnswer})
}

// deliverReset looks the address up and sends, after the response has already
// gone.
//
// Everything about this is downstream of one requirement: the caller must not
// be able to tell a hit from a miss. Doing the lookup here rather than in the
// handler means the *response time* carries no signal either -- a hit costs a
// database read and an HTTPS round trip to the mail provider, a miss costs
// neither, and neither of them happens while anybody is waiting.
//
// It also means a mail outage cannot become a 500 that says "this address
// exists, and something went wrong sending to it". The failure is a log line
// for the maintainer, which is who can act on it.
func (a *API) deliverReset(ctx context.Context, email string) {
	db, present := a.accountsDB()
	if !present {
		return
	}
	sender, err := a.emailSender()
	if err == nil {
		err = auth.SendReset(ctx, db, email, sender, "")
	}
	if err != nil {
		// Each of these carries a status code, a configuration complaint or a
		// socket error -- and none of them carries a recipient, which is what
		// makes this line safe to write. ADR 16, and `describeMailFailure` is
		// how that is kept true.
		a.log.Error("password reset could not be delivered", "error", err)
	}
}

// claim is `POST /api/auth/claim`: redeem an invite or reset link by choosing
// a password.
//
// An **invite** may also carry a `username`, which is the holder naming
// themselves rather than living with the one derived from their address. A
// reset may not; `auth.RedeemToken` gates that on the token's own purpose,
// read from the database, so this handler never has to be the thing that
// remembers.
func (a *API) claim(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	token := strings.TrimSpace(field(body, "token"))
	password := field(body, "password")
	chosen := strings.TrimSpace(field(body, "username"))
	if token == "" || password == "" {
		wire.Detail(w, http.StatusUnprocessableEntity,
			"a token and a password are required")
		return
	}
	address := clientAddress(r)

	db, present := a.accountsDB()
	if !present {
		wire.Detail(w, http.StatusBadRequest, "that link is not valid")
		return
	}
	spendable := budget{auth.AddressKey(address, "claim"), auth.ClaimPerAddress}
	if a.throttle(w, r, db, "too many attempts -- wait and try again", spendable) {
		return
	}

	user, err := auth.RedeemToken(r.Context(), db, token, password, "", chosen)
	switch {
	case err == nil:
	case errors.Is(err, auth.ErrWeakPassword), errors.Is(err, auth.ErrInvalidUsername),
		errors.Is(err, auth.ErrWrongPurpose):
		// Not counted and not a refusal of the link: the token is intact and
		// the sensible next move is a longer password, or a different handle.
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, auth.ErrUserExists):
		// 409, and the link is still live -- `RedeemToken` rolls the token
		// spend back with the failed rename, so "that name is taken" is a
		// retryable answer rather than a dead invite.
		wire.Detail(w, http.StatusConflict, err.Error())
		return
	case auth.IsTokenError(err):
		a.spend(r.Context(), db, spendable)
		a.log.Warn("rejected a claim", "address", address, "reason", err)
		wire.Detail(w, http.StatusBadRequest, err.Error())
		return
	default:
		a.refuse(w, "claim", err)
		return
	}
	a.forgive(r.Context(), db, spendable)

	a.log.Info("password set", "username", user.Username, "address", address)
	wire.JSON(w, http.StatusOK, map[string]any{
		"detail":   "password set -- you can sign in now",
		"username": user.Username,
	})
}

// claimPreview is `POST /api/auth/claim/preview`: what kind of link this is,
// and the name it currently carries.
//
// **A POST, and the token is in the body.** A GET would put it in a query
// string, and the entire reason `ClaimLink` hides it in a URL *fragment* is
// that a fragment reaches no server's access log. Reading a token back over
// the query string would undo that at the first hop, for the convenience of a
// verb.
//
// **It spends nothing and changes nothing.** A valid link previewed ten times
// is still a valid link; only `claim` consumes one. What comes back is the
// purpose and the username, and deliberately not the email address -- the
// holder of the token knows their own address, but ADR 16 does not put one in
// a response nobody authenticated for, and a preview is exactly that.
func (a *API) claimPreview(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	token := strings.TrimSpace(field(body, "token"))
	if token == "" {
		wire.Detail(w, http.StatusUnprocessableEntity, "a token is required")
		return
	}
	address := clientAddress(r)

	db, present := a.accountsDB()
	if !present {
		wire.Detail(w, http.StatusBadRequest, "that link is not valid")
		return
	}
	// Its own bucket rather than `claim`'s. Sharing one would let a page that
	// previews on mount spend the budget a person needs to actually redeem,
	// and the two failures mean different things.
	spendable := budget{auth.AddressKey(address, "claim-preview"), auth.ClaimPerAddress}
	if a.throttle(w, r, db, "too many attempts -- wait and try again", spendable) {
		return
	}

	resolved, err := auth.LookupToken(r.Context(), db, token, "")
	if err != nil {
		if !auth.IsTokenError(err) {
			a.refuse(w, "claim preview", err)
			return
		}
		a.spend(r.Context(), db, spendable)
		wire.Detail(w, http.StatusBadRequest, err.Error())
		return
	}
	account, err := auth.GetByID(r.Context(), db, resolved.UserID)
	if err != nil {
		a.refuse(w, "claim preview", err)
		return
	}
	if account == nil || account.Disabled {
		// The same sentence `RedeemToken` gives, for the same reason: a
		// disabled account's link is not a link whose holder is owed an
		// explanation of why.
		wire.Detail(w, http.StatusBadRequest, "that link is not valid")
		return
	}
	a.forgive(r.Context(), db, spendable)

	wire.JSON(w, http.StatusOK, map[string]any{
		"purpose": string(resolved.Purpose), "username": account.Username})
}

// me is `GET /api/auth/me`: who the caller is, and whether this instance
// requires anyone to be.
//
// Public, and the three flags are separate on purpose: a frontend needs to
// tell "you are logged out of an instance that wants a login" from "this
// instance has no login", and one collapsed boolean makes the local app render
// a sign-in form it has no server for.
//
// `is_admin` is reported at the top level as well as inside `user`, and the
// difference is exactly the case `user` cannot express: with auth off the
// caller is LOCAL -- nobody in particular, and an admin, because there is
// nobody else for it to be true relative to. A client deriving that from
// `auth_required` would be reimplementing `auth.Local` in TypeScript, so the
// server answers it instead. It is never a grant: the middleware reads the
// scope, not this response.
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	caller := auth.ScopeFrom(r.Context())
	var user any
	if caller.Authenticated {
		user = map[string]any{"id": caller.UserID, "username": caller.Username,
			"is_admin": caller.IsAdmin}
	}
	wire.JSON(w, http.StatusOK, map[string]any{
		"auth_required": a.requireAuth,
		"authenticated": caller.Authenticated,
		"is_admin":      caller.IsAdmin,
		"user":          user,
	})
}
