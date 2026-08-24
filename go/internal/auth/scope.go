package auth

import (
	"context"
	"database/sql"
)

// Scope is who a request is on behalf of.
type Scope struct {
	UserID        int64
	Username      string
	IsAdmin       bool
	Authenticated bool
	// ModelTier is which Claude answers this caller (`internal/tiers`), or
	// empty for the house model -- which is every account until a maintainer
	// says otherwise, and is also what an unauthenticated caller and the local
	// single-user app get.
	//
	// It belongs here for the same reason IsAdmin does: it is a fact about the
	// caller that a handler is allowed to know and that must be read fresh per
	// request rather than captured anywhere longer-lived. It arrived with the
	// first Claude route; the comment above this struct used to say the
	// door had no use for it, which was true of every route family before
	// that one.
	ModelTier string
}

// Anonymous is auth on, no valid session: reaches the public routes and
// nothing else.
var Anonymous = Scope{}

// Local is auth off: one person on their own machine, holding the file the
// app reads. An admin because there is nobody else for it to be true
// relative to.
var Local = Scope{IsAdmin: true}

// Resolve turns a token into a caller, or
// Anonymous if it resolves to nobody. Re-checks `disabled` even though
// disabling an account deletes its sessions, deliberately:
// the redundancy is cheap and the failure it covers is an account holder
// keeping access after somebody believed they had removed it.
func Resolve(ctx context.Context, db *sql.DB, token string) (Scope, error) {
	return scopeFor(ctx, db, token, Lookup)
}

// ResolveTouching is Resolve over `LookupTouching`: the serving process's
// resolver, which deletes an expired row on the way past and keeps
// `last_seen_at` fresh. It needs a write handle; a read-only one keeps
// `Resolve`.
func ResolveTouching(ctx context.Context, db *sql.DB, token string) (Scope, error) {
	return scopeFor(ctx, db, token, LookupTouching)
}

func scopeFor(ctx context.Context, db *sql.DB, token string,
	lookup func(context.Context, *sql.DB, string) (*Session, error)) (Scope, error) {
	session, err := lookup(ctx, db, token)
	if err != nil || session == nil {
		return Anonymous, err
	}
	user, err := GetByID(ctx, db, session.UserID)
	if err != nil || user == nil || user.Disabled {
		return Anonymous, err
	}
	return Scope{UserID: user.ID, Username: user.Username, IsAdmin: user.IsAdmin,
		Authenticated: true, ModelTier: user.ModelTier}, nil
}

// The request's caller, carried on the context by the door's middleware so
// any route can ask who is asking.

type scopeKey struct{}

// WithScope attaches the caller to a context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFrom is the caller the middleware resolved. Local -- auth off, one
// person, full access -- when nothing was attached, which is the permissive
// default an app assembled without the middleware keeps.
func ScopeFrom(ctx context.Context) Scope {
	if s, ok := ctx.Value(scopeKey{}).(Scope); ok {
		return s
	}
	return Local
}
