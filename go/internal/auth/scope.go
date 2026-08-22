package auth

import (
	"context"
	"database/sql"
)

// Scope is who a request is on behalf of -- `api/deps.py:UserScope`, minus
// the model tier, which the door has no use for.
type Scope struct {
	UserID        int64
	Username      string
	IsAdmin       bool
	Authenticated bool
}

// Anonymous is auth on, no valid session: reaches the public routes and
// nothing else.
var Anonymous = Scope{}

// Local is auth off: one person on their own machine, holding the file the
// app reads. An admin because there is nobody else for it to be true
// relative to (`api/deps.py:LOCAL`).
var Local = Scope{IsAdmin: true}

// Resolve is `api/auth.py:scope_for_token`: a token becomes a caller, or
// Anonymous if it resolves to nobody. Re-checks `disabled` even though
// disabling an account deletes its sessions, for the reason Python gives:
// the redundancy is cheap and the failure it covers is an account holder
// keeping access after somebody believed they had removed it.
func Resolve(ctx context.Context, db *sql.DB, token string) (Scope, error) {
	session, err := Lookup(ctx, db, token)
	if err != nil || session == nil {
		return Anonymous, err
	}
	user, err := GetByID(ctx, db, session.UserID)
	if err != nil || user == nil || user.Disabled {
		return Anonymous, err
	}
	return Scope{UserID: user.ID, Username: user.Username, IsAdmin: user.IsAdmin,
		Authenticated: true}, nil
}

// The request's caller, carried on the context by the door's middleware so
// the ported routes can ask who is asking -- `api/deps.py:scope` reading
// what `api/auth.py` left on `request.state`.

type scopeKey struct{}

// WithScope attaches the caller to a context.
func WithScope(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFrom is the caller the middleware resolved. Local -- auth off, one
// person, full access -- when nothing was attached, which is the permissive
// default `api/deps.py` keeps for an app assembled without the middleware.
func ScopeFrom(ctx context.Context) Scope {
	if s, ok := ctx.Value(scopeKey{}).(Scope); ok {
		return s
	}
	return Local
}
