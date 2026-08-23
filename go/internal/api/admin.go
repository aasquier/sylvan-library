package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/tiers"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The admin surface: `mtglab users` as routes. ADR 17.
//
// Everything here lives under `/api/admin`, and that is not a naming
// convention -- it is the enforcement. The door's middleware refuses the whole
// prefix to a non-admin **before routing** (`internal/door/auth.go`), so a
// route added here is protected by being here. `requireAdmin` below is the
// second check, on the handlers themselves, and it is what would catch an
// admin route mounted somewhere else by mistake -- the case the structural
// rule by construction cannot see. It costs an attribute read.
//
// **403 and not ADR 5's 404**, which is a deliberate exception and worth the
// sentence: that rule protects resources whose *existence* is the secret, and
// an admin route's existence is published in a public repository.
//
// What is deliberately **not** here, both from `api/admin.py`:
//
//   - **Setting somebody's password.** ADR 16 is unconditional: no password is
//     ever chosen by one person for another, because an admin-set password is a
//     password that has existed in plaintext in a chat window. `POST
//     .../reset` mails a link instead, which is the same intent routed through
//     the account holder.
//   - **Any leak-shaped difference between accounts.** ADR 5's 404-not-403 rule
//     is about resources that belong to one person; an admin listing accounts
//     is the one caller for whom every account is in scope, so a 404 here means
//     what it says -- no such username.
//
// **Addresses are returned from these routes**, which ADR 17 decided
// explicitly and which is why `accountBody` asks `AsDict(true)`. The rule that
// replaced "one caller" is narrow and still checkable: an address may be
// serialised only into a response an admin authenticated for. The door's
// prefix rule and the check below are two of the three mechanisms that
// guarantee it of this file; `tests/test_isolation.py` pins the third -- that
// no *other* module acquires the habit.
//
// **One route of this family is missing, and its absence is a decision.**
// `DELETE /api/admin/users/{username}` stays Python's until the jobs registry
// crosses, because deleting an account also calls `jobs.forget_owner` and that
// registry lives in memory in the uvicorn process. It cannot be skipped:
// `users.id` is `INTEGER PRIMARY KEY` without `AUTOINCREMENT`, so SQLite
// re-issues a deleted account's rowid, and jobs left keyed on that integer
// would be handed to whoever is created next -- the isolation `jobs.get`
// enforces, defeated by arithmetic. The response even reports `jobs_dropped`,
// so a handler here could not give an honest number. docs/go-migration/
// PLAN.md section 10 carries it.

// requireAdmin is `deps.admin`, the second of two checks. It answers 403 and
// reports whether it did.
func (a *API) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if auth.ScopeFrom(r.Context()).IsAdmin {
		return false
	}
	wire.Detail(w, http.StatusForbidden, "admin only")
	return true
}

// adminDB is `accountsDB` with the admin surface's answer to an absent
// database: there is nothing to administer, and nothing here creates a file
// the boot ladder did not. Reports whether it answered.
func (a *API) adminDB(w http.ResponseWriter) (*sql.DB, bool) {
	db, present := a.accountsDB()
	if present {
		return db, true
	}
	// An empty database has no accounts in it, so every per-account route is a
	// 404 and the list is empty. `listAccounts` handles its own case; this is
	// the answer for the rest.
	wire.Detail(w, http.StatusNotFound, "no account")
	return nil, false
}

// accountState is `_state`: the one word `mtglab users list` prints, computed
// the same way.
//
// Four states, and they are genuinely different things: `disabled` is revoked,
// `active` can log in, `invited` has a link outstanding, and `no password` is
// an account whose invite expired or was never sent -- which is the one that
// needs somebody to do something.
func accountState(ctx context.Context, db *sql.DB, user *auth.User) (string, error) {
	if user.Disabled {
		return "disabled", nil
	}
	has, err := auth.HasPassword(ctx, db, user.ID)
	if err != nil {
		return "", err
	}
	if has {
		return "active", nil
	}
	invited, err := auth.TokenOutstanding(ctx, db, user.ID, auth.PurposeInvite)
	if err != nil {
		return "", err
	}
	if invited {
		return "invited", nil
	}
	return "no password", nil
}

// accountBody is one account as the admin page sees it. **It carries the
// address.**
func accountBody(ctx context.Context, db *sql.DB, user *auth.User) (map[string]any, error) {
	state, err := accountState(ctx, db, user)
	if err != nil {
		return nil, err
	}
	sessions, err := auth.CountSessionsForUser(ctx, db, user.ID)
	if err != nil {
		return nil, err
	}
	body := user.AsDict(true)
	body["state"] = state
	body["sessions"] = sessions
	return body, nil
}

// findAccount is `_find`: the account, or the 404 that says there is none.
func (a *API) findAccount(w http.ResponseWriter, r *http.Request, db *sql.DB) (*auth.User, bool) {
	username := r.PathValue("username")
	user, err := auth.Get(r.Context(), db, username)
	if err != nil {
		a.refuse(w, "account", err)
		return nil, false
	}
	if user == nil {
		wire.Detail(w, http.StatusNotFound, "no account "+wire.PyRepr(username))
		return nil, false
	}
	return user, true
}

// emailSender is the ADR 16 seam reaching the edge. A nil configured sender
// means "decide from the environment, when a message is actually being sent"
// -- which is what a real process wants, and which is also why no test in this
// module sends mail: the tests pass a recorder instead.
func (a *API) emailSender() (auth.EmailSender, error) {
	if a.email != nil {
		return a.email, nil
	}
	return auth.SenderFromEnv(nil)
}

// senderOr503 resolves the sender, answering 503 when nothing is configured.
//
// **503 rather than 500**: nothing is broken, something is unset, and the
// maintainer reading this is the person who can set it.
func (a *API) senderOr503(w http.ResponseWriter) (auth.EmailSender, bool) {
	sender, err := a.emailSender()
	if err == nil {
		return sender, true
	}
	if errors.Is(err, auth.ErrEmailNotConfigured) {
		wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		return nil, false
	}
	a.refuse(w, "mail", err)
	return nil, false
}

// ---- the five routes -------------------------------------------------------

// listAccounts is `GET /api/admin/users`: every account, with the address, the
// state and the session count.
//
// `admins` is the count of admins who can actually sign in, and it is here so
// the page can grey out the last demote button rather than offering a click
// that returns 409. It is the same predicate the refusal uses --
// `auth.UsableAdminIDs` -- because a second spelling of it is a second thing
// to keep in step.
//
// `tiers` is the roster this build knows, so the page offers exactly what the
// server will accept. One list, serialised: a second one written in TypeScript
// would drift the day a tier is added, and the drift would present as a
// control that 422s.
func (a *API) listAccounts(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	db, present := a.accountsDB()
	if !present {
		// An instance with no `app.db` has no accounts, which is what an empty
		// list says. The roster is a fact about the build and answers anyway.
		wire.JSON(w, http.StatusOK, map[string]any{
			"users": []any{}, "admins": 0, "tiers": tiers.Roster()})
		return
	}
	everyone, err := auth.AllUsers(r.Context(), db)
	if err != nil {
		a.refuse(w, "accounts", err)
		return
	}
	accounts := make([]map[string]any, 0, len(everyone))
	for _, user := range everyone {
		body, err := accountBody(r.Context(), db, user)
		if err != nil {
			a.refuse(w, "accounts", err)
			return
		}
		accounts = append(accounts, body)
	}
	admins, err := auth.UsableAdminIDs(r.Context(), db)
	if err != nil {
		a.refuse(w, "accounts", err)
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{
		"users": accounts, "admins": len(admins), "tiers": tiers.Roster()})
}

// inviteAccount is `POST /api/admin/users` (201): create an unclaimed account
// and mail its owner a setup link.
//
// The same three rules `mtglab users invite` follows, for the same reasons:
// the account is created **unclaimed** rather than disabled (`disabled_at` is
// the revocation lever and redeeming a link must not undo it); re-inviting an
// unclaimed account re-issues the link, which is the resend path; and
// re-inviting a *claimed* one is refused and points at the reset flow, because
// that is what somebody who has forgotten their password actually needs.
//
// Sent **synchronously**, unlike `POST /api/auth/reset`. That endpoint hides
// whether the address resolves to an account and so cannot report a delivery
// failure; here the caller is an authorised admin who is owed the truth about
// whether the message went.
func (a *API) inviteAccount(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// **A divergence the port declined to reproduce, and it has since been
	// ruled on.** Python handed `payload.get("email")` to `normalise_email`
	// *raw*, so a body carrying `{"email": 0}` -- or `true`, or a list --
	// reached `.strip()` on something that has not got one and became a
	// **500**. This answered 422 with the shape sentence instead, on the
	// grounds that reproducing a crash is not equivalence, and reported it
	// rather than ruling on it, the share toggle being the precedent: the port
	// finds it, Aaron rules, both runtimes are fixed at once.
	//
	// `api/admin.py` now coerces the same way, and the sentences match, which
	// is what the case table beside this pins. `str` and not `field`: `field`
	// folds `0` and `false` to the empty string, which would report a *missing*
	// address for a body that supplied one.
	address, err := auth.NormaliseEmail(str(body, "email"))
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if address == "" {
		wire.Detail(w, http.StatusUnprocessableEntity,
			"an invite needs an email address")
		return
	}
	wanted := strings.TrimSpace(field(body, "username"))
	makeAdmin := !falsy(body["is_admin"])

	// Resolved before any database work, exactly as Python resolves it: an
	// instance with no mail configured should say so rather than create an
	// account whose invite can never be sent.
	sender, ok := a.senderOr503(w)
	if !ok {
		return
	}
	db, ok := a.adminDB(w)
	if !ok {
		return
	}

	user, err := auth.GetByEmail(r.Context(), db, address)
	if err != nil {
		a.refuse(w, "invite", err)
		return
	}
	if user != nil {
		has, err := auth.HasPassword(r.Context(), db, user.ID)
		if err != nil {
			a.refuse(w, "invite", err)
			return
		}
		if has {
			wire.Detail(w, http.StatusConflict, user.Username+
				" has already claimed that address -- send them a reset link instead")
			return
		}
	} else {
		name := wanted
		if name == "" {
			name, _, _ = strings.Cut(address, "@")
		}
		if _, err := auth.NormaliseUsername(name); err != nil {
			wire.Detail(w, http.StatusUnprocessableEntity,
				err.Error()+"; choose a username")
			return
		}
		user, err = auth.Create(r.Context(), db, name, address, makeAdmin)
		if err != nil {
			if errors.Is(err, auth.ErrUserExists) {
				wire.Detail(w, http.StatusConflict, err.Error())
				return
			}
			a.refuse(w, "invite", err)
			return
		}
	}

	if err := auth.SendInvite(r.Context(), db, user, sender, ""); err != nil {
		// The account exists and its link did not go out. Say so plainly: the
		// fix is to press invite again once mail works, which this endpoint
		// supports, rather than to wonder whether half of something happened.
		wire.Detail(w, http.StatusBadGateway,
			"the account exists but the invite could not be sent: "+err.Error())
		return
	}
	answer, err := accountBody(r.Context(), db, user)
	if err != nil {
		a.refuse(w, "invite", err)
		return
	}
	a.log.Info("invited an account", "by", a.actor(r.Context()), "username", user.Username)
	wire.JSON(w, http.StatusCreated, answer)
}

// updateAccount is `PATCH /api/admin/users/{username}`: grant or revoke admin,
// disable or re-enable, set a model tier.
//
// **Every refusal comes from `internal/auth` rather than from here**, which is
// ADR 17's point: the rule that an instance may not be left without an admin
// belongs in the core, so the CLI and this route cannot disagree about it. A
// `LastAdmin` is a 409 -- the request is well-formed and understood, and it
// conflicts with the state of the world. An `UnknownTier` is a 422: that one
// is a malformed request, not a conflicting one, and it comes from the same
// place for the same reason.
func (a *API) updateAccount(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	// The body first, then the account -- FastAPI validates a declared body
	// before the handler runs, so a malformed body against a name nobody holds
	// is a 422 there and must be one here.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	db, ok := a.adminDB(w)
	if !ok {
		return
	}
	user, ok := a.findAccount(w, r, db)
	if !ok {
		return
	}

	var changed []string
	if wanted, given := body["is_admin"]; given {
		if want := !falsy(wanted); want != user.IsAdmin {
			if err := auth.SetAdmin(r.Context(), db, user.ID, want); err != nil {
				a.refuseAdminWrite(w, err)
				return
			}
			changed = append(changed, map[bool]string{true: "admin", false: "not admin"}[want])
		}
	}
	if wanted, given := body["disabled"]; given {
		if want := !falsy(wanted); want != user.Disabled {
			if _, err := auth.SetDisabled(r.Context(), db, user.ID, want); err != nil {
				a.refuseAdminWrite(w, err)
				return
			}
			changed = append(changed, map[bool]string{true: "disabled", false: "enabled"}[want])
		}
	}
	if asked, given := body["model_tier"]; given {
		// Compared through the roster on both sides so that an account holding
		// a stale key and one holding NULL -- which resolve to the same tier
		// and are answered by the same model -- do not read as a change worth
		// logging or a 422 worth raising.
		tier := ""
		if asked != nil {
			tier = str(body, "model_tier")
		}
		if tiers.Get(tier).Key != tiers.Get(user.ModelTier).Key {
			if err := auth.SetModelTier(r.Context(), db, user.ID, tier); err != nil {
				a.refuseAdminWrite(w, err)
				return
			}
			changed = append(changed, "answered by "+tiers.Get(tier).Label)
		}
	}
	if len(changed) == 0 {
		wire.Detail(w, http.StatusUnprocessableEntity,
			"nothing to change -- send is_admin, disabled or model_tier")
		return
	}

	fetched, err := auth.Get(r.Context(), db, user.Username)
	if err != nil {
		a.refuse(w, "account", err)
		return
	}
	if fetched == nil {
		wire.Detail(w, http.StatusNotFound, "no account "+wire.PyRepr(user.Username))
		return
	}
	answer, err := accountBody(r.Context(), db, fetched)
	if err != nil {
		a.refuse(w, "account", err)
		return
	}
	a.log.Warn("an account changed", "username", user.Username,
		"changes", strings.Join(changed, " and "))
	wire.JSON(w, http.StatusOK, answer)
}

// refuseAdminWrite maps the core's two refusals to the statuses ADR 17 argues
// for, and anything else to a 500.
func (a *API) refuseAdminWrite(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrLastAdmin):
		wire.Detail(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrUnknownTier):
		wire.Detail(w, http.StatusUnprocessableEntity, "no such tier: "+err.Error())
	default:
		a.refuse(w, "account", err)
	}
}

// sendAccountReset is `POST /api/admin/users/{username}/reset` (202): mail this
// account a link to choose a new password.
//
// The admin's answer to "they are locked out" -- and the whole of it. ADR 16
// forbids setting the password for them, so what an admin can do is cause the
// link to be sent, which is exactly what the account holder could have done
// from the sign-in page.
//
// 202 and not 200 for the reason `POST /api/auth/reset` uses it: the message
// has been handed to a provider, and delivery is not something this response
// knows about.
func (a *API) sendAccountReset(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	sender, ok := a.senderOr503(w)
	if !ok {
		return
	}
	db, ok := a.adminDB(w)
	if !ok {
		return
	}
	user, ok := a.findAccount(w, r, db)
	if !ok {
		return
	}
	if !user.HasEmail || user.Email == "" {
		wire.Detail(w, http.StatusUnprocessableEntity, user.Username+
			" has no email address -- set a password with `mtglab users passwd` instead")
		return
	}
	if user.Disabled {
		// `auth.SendReset` would silently decline this one, which is the
		// correct behaviour for the public endpoint and the wrong answer for
		// an admin who pressed a button. Same rule, said out loud: a lever the
		// disabled party can undo from their own inbox is not a lever.
		wire.Detail(w, http.StatusConflict, user.Username+
			" is disabled -- enable the account first if they should be able to sign in")
		return
	}
	if err := auth.SendReset(r.Context(), db, user.Email, sender, ""); err != nil {
		wire.Detail(w, http.StatusBadGateway, err.Error())
		return
	}
	a.log.Info("a reset link was sent", "username", user.Username)
	wire.JSON(w, http.StatusAccepted, map[string]any{
		"detail": "a reset link is on its way to " + user.Username})
}

// revokeSessions is `DELETE /api/admin/users/{username}/sessions`: sign an
// account out everywhere, without disabling it.
//
// The lighter of the two revocations, and the one wanted for a lost laptop:
// the account is still good, the cookies on that machine are not. Disabling is
// for revoking the *account*, and it does this as well.
func (a *API) revokeSessions(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	db, ok := a.adminDB(w)
	if !ok {
		return
	}
	user, ok := a.findAccount(w, r, db)
	if !ok {
		return
	}
	ended, err := auth.DeleteSessionsForUser(r.Context(), db, user.ID)
	if err != nil {
		a.refuse(w, "sessions", err)
		return
	}
	a.log.Warn("every session for an account was revoked",
		"username", user.Username, "count", ended)
	wire.JSON(w, http.StatusOK, map[string]any{
		"username": user.Username, "revoked": ended})
}
