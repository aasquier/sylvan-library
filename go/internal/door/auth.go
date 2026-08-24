package door

import (
	"context"
	"database/sql"
	"net/http"
	"path"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// CookieName is the session cookie's name.
const CookieName = "sid"

// PublicPaths is the allowlist: reachable with no session when
// auth is on. Everything else under /api is refused before routing. It is
// code, not a file read at request time -- and the door's sweeps derive
// from the served route table (`TestEveryPublicPathIsServed` and its
// siblings), so an entry here that nothing serves fails a test and a new
// route is deny-by-default.
var PublicPaths = map[string]bool{
	"/api/health":             true,
	"/api/auth/login":         true,
	"/api/auth/logout":        true,
	"/api/auth/me":            true,
	"/api/auth/reset":         true,
	"/api/auth/claim":         true,
	"/api/auth/claim/preview": true,
}

// AdminPrefix is refused, before routing, to a
// caller who is not an admin -- 403, not ADR 5's 404, and ADR 17 says why.
const AdminPrefix = "/api/admin"

// The two sentences the middleware answers with. The frontend does not match
// on them; the door's tests and the deployed smoke test do, and an
// operator greps for them.
const (
	noSession = "authentication required"
	adminOnly = "admin only"
)

// NormalisePath is the one form the allowlist
// is checked against, so the check is never *more permissive* than the
// router: repeated slashes collapse and `.` and `..` resolve --
// `path.Clean` does exactly that for a path
// that starts with one slash, and the guard below gives it one. A trailing
// slash is dropped, and an empty result is `/`.
func NormalisePath(p string) string {
	cleaned := path.Clean("/" + strings.TrimLeft(p, "/"))
	cleaned = strings.TrimRight(cleaned, "/")
	if cleaned == "" {
		return "/"
	}
	return cleaned
}

// isAPI is the test both the middleware and the SPA catch-all make:
// a prefix test, deliberately not
// a segment test -- `/apix` counts as API and is refused rather than handed
// the shell, which is the recorded rule and the stricter of the two
// readings.
func isAPI(normalised string) bool {
	return strings.HasPrefix(normalised, "/api")
}

// IsPublic: anything outside /api is the built
// frontend and has to load for a login form to exist; under /api only the
// allowlist passes.
func IsPublic(p string) bool {
	normalised := NormalisePath(p)
	if !isAPI(normalised) {
		return true
	}
	return PublicPaths[normalised]
}

// IsAdminPath carries the `/` in the second
// test so `/api/administrators` is not swept in.
func IsAdminPath(p string) bool {
	normalised := NormalisePath(p)
	return normalised == AdminPrefix || strings.HasPrefix(normalised, AdminPrefix+"/")
}

// Resolver turns a session token into a caller. The door's is backed by
// `app.db`; tests use a fake.
type Resolver interface {
	Resolve(ctx context.Context, token string) (auth.Scope, error)
}

// dbResolver resolves against app.db. With a write handle it also carries
// the two writes a live server owes the sessions table -- the expired-row
// delete and the `last_seen_at` touch (`auth.ResolveTouching`); a read-only
// handle resolves without them, the degraded shape a test or a broken disk
// gets.
type dbResolver struct {
	db    *sql.DB
	touch bool
}

func (r dbResolver) Resolve(ctx context.Context, token string) (auth.Scope, error) {
	if r.touch {
		return auth.ResolveTouching(ctx, r.db, token)
	}
	return auth.Resolve(ctx, r.db, token)
}

// authenticate is the middleware: resolve the
// caller, then refuse anything not on the allowlist. The 401 comes before the
// 403 so an anonymous request for an admin path is told it needs a session
// rather than that it needs to be somebody else.
//
// A lookup that *fails* (app.db unreadable, say) resolves to anonymous and is
// logged: the middleware must not pass a request through on an error, and
// must not 500 the whole site over one lookup either -- an anonymous caller
// is refused everything private, which is the safe answer.
func (d *Door) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !d.cfg.RequireAuth {
			// One person on their own machine: the local scope, an admin
			// because there is nobody else for it to be true relative to.
			next.ServeHTTP(w, r.WithContext(auth.WithScope(r.Context(), auth.Local)))
			return
		}
		caller := auth.Anonymous
		if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
			resolved, err := d.resolver.Resolve(r.Context(), c.Value)
			if err != nil {
				d.log.Warn("session lookup failed; treating the caller as anonymous",
					"error", err)
			} else {
				caller = resolved
			}
		}
		if !caller.Authenticated && !IsPublic(r.URL.Path) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"detail": noSession})
			return
		}
		if !caller.IsAdmin && IsAdminPath(r.URL.Path) {
			d.log.Warn("refused an admin path to a caller who is not an admin",
				"path", NormalisePath(r.URL.Path), "username", caller.Username)
			writeJSON(w, http.StatusForbidden, map[string]any{"detail": adminOnly})
			return
		}
		// The caller rides on the context for the routes.
		next.ServeHTTP(w, r.WithContext(auth.WithScope(r.Context(), caller)))
	})
}
