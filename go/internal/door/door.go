// Package door is the front door: the one binary that takes the listening
// port, and in front of *both* runtimes for the whole of the migration
// (docs/go-migration/PLAN.md section 4, ADR 38 decision 2).
//
// What it does, in request order:
//
//  1. Resolves the caller from the `sid` cookie and refuses, before anything
//     is routed, what `api/auth.py`'s middleware refuses: 401 outside the
//     public list without a session, 403 under the admin prefix without an
//     admin. Same sentences, same JSON envelope, same normalisation of the
//     path -- the contract suite (`tests/contract/`) holds the two doors to
//     one table, `tests/contract/routes.json`.
//  2. Serves the built frontend and the tarot pictures itself -- the shell,
//     `/assets/*`, `/tarot/*` -- with the same content types, the same
//     `Cache-Control: no-cache`, and the same refusals Python's mounts give.
//  3. Answers the routes that have moved across (`internal/api`, the port
//     board in docs/go-migration/PLAN.md section 10) itself, and proxies
//     everything else under `/api` to the Python server on loopback -- a
//     shrinking set as the families move; `routes.go` is the rule for which
//     request is whose.
//
// Every response the door writes itself carries the hardening headers
// `api/app.py:security_headers` sets, so the middleware's own refusals look
// the same from either runtime.
//
// Python's middleware stays on behind the door for the duration -- defence
// in depth, costing microseconds -- and is the one that writes the session
// touch; this door never writes `app.db` (see `internal/auth`).
package door

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/shelves"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Config is what a door needs to stand.
type Config struct {
	// RequireAuth mirrors MTGLAB_REQUIRE_AUTH: off, every caller is LOCAL and
	// nothing is refused; on, the middleware in auth.go runs on every request.
	RequireAuth bool
	// SecureCookies mirrors MTGLAB_SECURE_COOKIES and decides HSTS, exactly as
	// it does in Python: TLS fronts the app, so the header is safe to send.
	SecureCookies bool
	// AppDB is the path to `app.db`; read-only, and only when RequireAuth.
	AppDB string
	// WebDist is the built frontend directory; empty or missing means the door
	// serves no shell, as `api/app.py` registers no SPA route without one.
	WebDist string
	// TarotDir is the packaged tarot art; same rule.
	TarotDir string
	// Upstream is where everything under /api not yet served here goes.
	Upstream *url.URL
	// Pool is the card pool the ported routes read, leased (`internal/pool`);
	// nil is an instance with no pool, answered in the degraded shapes
	// `service._connect()` returning None produces.
	Pool *pool.Pool
	// DecksDir is the file tier's root (MTGLAB_DECKS_DIR).
	DecksDir string
	// DataDir is MTGLAB_DATA_DIR: the three runtime shelves live under its
	// `cache/`. Empty means no shelves, and every shelf route a 404.
	DataDir string
	// AdminEmail is MTGLAB_ADMIN_EMAIL, resolved to the maintainer's handle
	// through app.db when a caller is signed in (ADR 17, ADR 22); never
	// rendered.
	AdminEmail string
	// EmailSender is ADR 16's seam, handed to the account routes. Nil is what
	// a real process wants -- the sender is decided from the environment when
	// a message is actually being sent -- and a test passes a recorder, which
	// is how no test in this module sends mail.
	EmailSender auth.EmailSender
	// Logger, or slog.Default().
	Logger *slog.Logger
}

// Door is a built handler and the things it holds open.
type Door struct {
	cfg      Config
	log      *slog.Logger
	resolver Resolver
	db       *sql.DB
	writeDB  *sql.DB
	recorder *decklog.Recorder
	static   *staticSite
	proxy    http.Handler
	table    *routeTable
}

// New builds a door. It opens `app.db` read-only when auth is required (and
// proves it can read the users table, so a wrong data directory fails at
// start rather than as a 401 on every request), lists the bundle's root
// files once from the trusted directory, and wires the proxy.
func New(cfg Config) (*Door, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Upstream == nil {
		return nil, fmt.Errorf("door: no upstream configured")
	}
	d := &Door{cfg: cfg, log: cfg.Logger}
	if cfg.RequireAuth {
		db, err := auth.Open(cfg.AppDB)
		if err != nil {
			return nil, err
		}
		d.db = db
		d.resolver = dbResolver{db: db}
	}
	site, err := newStaticSite(cfg.WebDist, cfg.TarotDir, cfg.Logger)
	if err != nil {
		return nil, err
	}
	d.static = site
	d.proxy = newProxy(cfg.Upstream, cfg.Logger)
	var shelf *shelves.Shelves
	if cfg.DataDir != "" {
		shelf = shelves.New(cfg.DataDir, nil, cfg.Logger)
	}
	// The write side of `app.db`, opened once for the deck writes and the
	// activity log (Phase 4). Both are absent on an instance whose database
	// does not exist yet, and both say so rather than creating one: Python
	// owns the schema ladder until Phase 8. A failure to open a database that
	// *is* there is loud and at start, because a write discovering it at the
	// insert is a write that has already changed a deck file.
	if err := d.openWriteSide(cfg); err != nil {
		return nil, err
	}
	ported := api.New(api.Config{Logger: cfg.Logger, Pool: cfg.Pool, AppDB: d.db,
		AppDBPath: cfg.AppDB, DecksDir: cfg.DecksDir, AdminEmail: cfg.AdminEmail,
		Shelves: shelf, AppWriteDB: d.writeDB, Recorder: d.recorder,
		// The two switches the account routes read, passed rather than looked
		// up so the door and the routes it serves cannot disagree about what
		// this process is: `me` reports RequireAuth, `logout` deletes a
		// session row only when it is on, and the session cookie carries
		// `Secure` exactly when Python's would.
		RequireAuth: cfg.RequireAuth, SecureCookies: cfg.SecureCookies,
		// ADR 16's seam. Nil is the real process's answer -- decide from the
		// environment when a message is actually being sent -- and a test
		// passes a recorder, which is how no test here sends mail.
		EmailSender: cfg.EmailSender})
	table, err := newRouteTable(ported.Routes(), ported.Proxied())
	if err != nil {
		return nil, err
	}
	d.table = table
	return d, nil
}

// openWriteSide opens `app.db` read-write for the deck writes and the log,
// when there is one. An absent database leaves both nil, which the SQL deck
// tier reports as read-only and the log reports as a dropped entry -- the
// honest answers on a laptop that has never created one.
func (d *Door) openWriteSide(cfg Config) error {
	if cfg.AppDB == "" {
		return nil
	}
	if _, err := os.Stat(cfg.AppDB); err != nil {
		// Not a failure: an instance whose Python half has not created the
		// database yet is a real state, and both halves of the write side say
		// so -- the SQL deck tier reports itself read-only, the log drops the
		// entry with a warning. Creating one here would be Go owning the
		// schema ladder, which is Python's until Phase 8.
		d.log.Info("no app.db yet; deck writes stay on the file tier and the "+
			"activity log waits for Python to create one", "path", cfg.AppDB)
		return nil //nolint:nilerr // an absent database is a state, not a failure
	}
	recorder, err := decklog.NewRecorder(cfg.AppDB, cfg.Logger)
	if err != nil {
		return fmt.Errorf("door: %w", err)
	}
	d.recorder = recorder
	d.writeDB = recorder.DB()
	return nil
}

// Check proves the door can do its job before it takes the port: `app.db`
// readable when auth is on. Called by the command after New; separate so a
// test can build a door against a file that appears later.
func (d *Door) Check(ctx context.Context) error {
	if d.db == nil {
		return nil
	}
	return auth.Ping(ctx, d.db)
}

// Close releases what New opened.
func (d *Door) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Handler is the whole door as one http.Handler, outermost layer first:
// security headers, then the auth middleware, then dispatch.
func (d *Door) Handler() http.Handler {
	return d.securityHeaders(d.authenticate(http.HandlerFunc(d.dispatch)))
}

// dispatch is the router, and deliberately a small one: it decides which of
// three things a path is. The API check is made on the *normalised* path,
// exactly as the auth middleware and the SPA catch-all make it in Python, so
// `//api/decks` and `/api/./decks` go to the runtime that will refuse them as
// JSON rather than being served the shell. Under /api the ported routes are
// asked first (`routes.go` says on what terms) and the proxy takes the rest.
// The static mounts match the raw path, as Starlette's `Mount` does.
func (d *Door) dispatch(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Path
	if raw == DoorHealthPath {
		d.health(w, r)
		return
	}
	if isAPI(NormalisePath(raw)) {
		if h, ok := d.table.match(r); ok {
			h.ServeHTTP(w, r)
			return
		}
		d.proxy.ServeHTTP(w, r)
		return
	}
	d.static.ServeHTTP(w, r)
}

// DoorHealthPath is the door's own liveness answer -- outside `/api`, where
// both runtimes agree every path is public, and therefore not a route the
// shared classification has to carry. `/api/health` is the *pair's* health
// (the pool, the decks, the Python half answering) and stays proxied: the
// platform's check, the image's HEALTHCHECK and the deploy's smoke test all
// ask it, and a door that answered it alone would report a healthy instance
// with nothing behind it.
const DoorHealthPath = "/door/health"

func (d *Door) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"detail": "Method Not Allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// securityHeaders is `api/app.py:security_headers`, setdefault semantics
// included: an explicit header on a particular response wins over the
// blanket one, and a proxied response already carrying Python's is left as
// it is.
//
// Applied at WriteHeader time, not before the handler runs -- the contract
// suite caught the difference on the first run through the door: the
// reverse proxy copies the upstream's headers with Add, so a value set here
// beforehand met Python's copy and the wire said `nosniff, nosniff`.
func (d *Door) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hw := &headerWriter{ResponseWriter: w, apply: func(h http.Header) {
			setDefault(h, "X-Content-Type-Options", "nosniff")
			setDefault(h, "X-Frame-Options", "DENY")
			setDefault(h, "Referrer-Policy", "same-origin")
			setDefault(h, "Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
			if d.cfg.SecureCookies {
				setDefault(h, "Strict-Transport-Security", "max-age=31536000")
			}
		}}
		next.ServeHTTP(hw, r)
	})
}

// headerWriter runs `apply` over the headers once, at the moment they are
// committed, so whatever the handler (or the proxied upstream) set is already
// there to be deferred to. Unwrap keeps http.ResponseController -- and so the
// proxy's Flush and Hijack -- working through it.
type headerWriter struct {
	http.ResponseWriter
	apply func(http.Header)
	wrote bool
}

func (h *headerWriter) WriteHeader(code int) {
	if !h.wrote {
		h.wrote = true
		h.apply(h.Header())
	}
	h.ResponseWriter.WriteHeader(code)
}

func (h *headerWriter) Write(b []byte) (int, error) {
	if !h.wrote {
		h.WriteHeader(http.StatusOK)
	}
	return h.ResponseWriter.Write(b)
}

func (h *headerWriter) Unwrap() http.ResponseWriter { return h.ResponseWriter }

func setDefault(h http.Header, name, value string) {
	if h.Get(name) == "" {
		h.Set(name, value)
	}
}

// writeJSON answers in FastAPI's envelope: `application/json`, and for an
// error a body with `detail`, which `web/src/lib/api.ts` reads off every
// non-2xx response. `wire` is the one encoder for both the door's own
// refusals and the ported routes' answers.
func writeJSON(w http.ResponseWriter, status int, body any) {
	wire.JSON(w, status, body)
}
