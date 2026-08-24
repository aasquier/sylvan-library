// Package door is the HTTP server: the one process that takes the listening
// port and answers everything the app serves.
//
// What it does, in request order:
//
//  1. Resolves the caller from the `sid` cookie -- touching the session's
//     `last_seen_at` and deleting an expired row on the way past -- and
//     refuses, before anything is routed, everything auth refuses: 401
//     outside the public list without a session, 403 under the admin prefix
//     without an admin. The public list is code (`PublicPaths`), and the
//     door's own sweeps derive from the served route table, so a new route
//     is deny-by-default.
//  2. Serves the built frontend and the tarot pictures -- the shell,
//     `/assets/*`, `/tarot/*` -- and compresses what crosses the gzip floor.
//  3. Answers `/api` from `internal/api`'s route table; a path no route
//     claims is the catch-all's 404 and a matched path on the wrong method
//     is the router's 405, both written by `dispatch`.
//
// Every response carries the hardening headers `securityHeaders` sets, and
// every request lands in the visitor ledger (`internal/traffic`) exactly
// once, by route template.
package door

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/shelves"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
	matchledger "github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
	"github.com/aasquier/sylvan-library/go/internal/traffic"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Config is what a door needs to stand.
type Config struct {
	// RequireAuth mirrors MTGLAB_REQUIRE_AUTH: off, every caller is LOCAL and
	// nothing is refused; on, the middleware in auth.go runs on every request.
	RequireAuth bool
	// SecureCookies mirrors MTGLAB_SECURE_COOKIES and decides HSTS with the
	// same flag: TLS fronts the app, so the header is safe to send.
	SecureCookies bool
	// AppDB is the path to `app.db`; read-only, and only when RequireAuth.
	AppDB string
	// WebDist is the built frontend directory; empty or missing means the door
	// serves no shell at all rather than a broken one.
	WebDist string
	// TarotDir is the packaged tarot art; same rule.
	TarotDir string
	// Pool is the card pool the routes read, leased (`internal/pool`);
	// nil is an instance with no pool, answered in the recorded degraded
	// shapes rather than refused.
	Pool *pool.Pool
	// DecksDir is the file tier's root (MTGLAB_DECKS_DIR).
	DecksDir string
	// ScryfallDir is where the bulk downloads live; `/api/health` lists them.
	ScryfallDir string
	// PoolPath is the card pool file itself, for the storage view that sizes
	// it without ever opening it.
	PoolPath string
	// DataDir is MTGLAB_DATA_DIR: the three runtime shelves live under its
	// `cache/`. Empty means no shelves, and every shelf route a 404.
	DataDir string
	// AdminEmail is MTGLAB_ADMIN_EMAIL, resolved to the maintainer's handle
	// through app.db when a caller is signed in (ADR 17, ADR 22); never
	// rendered.
	AdminEmail string
	// EmailSender is ADR 16's seam, handed to the account routes. Nil is what
	// a real process wants -- the sender is chosen from Mail when a message is
	// actually being sent -- and a test passes a recorder, which is how no test
	// in this module sends mail.
	EmailSender auth.EmailSender
	// Mail is what that choice is made from when EmailSender is nil: settings
	// resolved once at config.Load and carried here, never read again from the
	// environment.
	Mail auth.MailSettings
	// ClientIPHeader is MTGLAB_CLIENT_IP_HEADER, handed to the rate limiter.
	// Empty trusts only the peer, which is the safe default.
	ClientIPHeader string
	// Logger, or slog.Default().
	Logger *slog.Logger
}

// Door is a built handler and the things it holds open.
type Door struct {
	// jobs is this process's job registry: every job a route submits.
	jobs *jobs.Registry
	// simCache is ADR 18's `sim_cache` table, or nil on an instance with no
	// `app.db`. A nil store caches nothing and no caller branches on it.
	simCache *cache.Store
	cfg      Config
	log      *slog.Logger
	resolver Resolver
	db       *sql.DB
	writeDB  *sql.DB
	recorder *decklog.Recorder
	static   *staticSite
	table    *routeTable
	// traffic is the visitor ledger's recorder (schema v9), counting every
	// request this process answers. Nil on an instance with no app.db.
	traffic *traffic.Recorder
}

// New builds a door. It opens `app.db` read-only when auth is required (and
// proves it can read the users table, so a wrong data directory fails at
// start rather than as a 401 on every request), and lists the bundle's root
// files once from the trusted directory.
func New(cfg Config) (*Door, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	d := &Door{cfg: cfg, log: cfg.Logger}
	if cfg.RequireAuth {
		db, err := auth.Open(cfg.AppDB)
		if err != nil {
			return nil, err
		}
		d.db = db
	}
	site, err := newStaticSite(cfg.WebDist, cfg.TarotDir, cfg.Logger)
	if err != nil {
		return nil, err
	}
	d.static = site
	var shelf *shelves.Shelves
	if cfg.DataDir != "" {
		shelf = shelves.New(cfg.DataDir, nil, cfg.Logger)
	}
	// The write side of `app.db`, opened once for the deck writes and the
	// activity log. The serving command runs the ladder before the
	// door stands (`auth.Migrate`), so an absent file here is a caller that
	// skipped it -- a test, a bare library use -- and both halves degrade
	// rather than minting one. A failure to open a database that *is* there
	// is loud and at start, because a write discovering it at the insert is a
	// write that has already changed a deck file.
	if err := d.openWriteSide(cfg); err != nil {
		return nil, err
	}
	// The resolver, preferring the write handle: a live server owes the
	// sessions table its expired-row deletes and its `last_seen_at` touches,
	// and only a write handle can pay them. Without one -- a test against a
	// read-only file, a broken disk -- sessions still resolve, untouched.
	if cfg.RequireAuth {
		if d.writeDB != nil {
			d.resolver = dbResolver{db: d.writeDB, touch: true}
		} else {
			d.resolver = dbResolver{db: d.db}
		}
	}
	// The job registry and ADR 18's cache.
	d.jobs = jobs.New(jobs.Config{Logger: cfg.Logger})
	d.openSimCache(cfg)
	d.traffic = traffic.New(d.writeDB, cfg.Logger)

	routes := api.New(api.Config{Logger: cfg.Logger, Pool: cfg.Pool, AppDB: d.db,
		Jobs: d.jobs, SimCache: d.simCache,
		AppDBPath: cfg.AppDB, DecksDir: cfg.DecksDir, ScryfallDir: cfg.ScryfallDir,
		DataDir: cfg.DataDir, PoolPath: cfg.PoolPath, Traffic: d.traffic,
		AdminEmail: cfg.AdminEmail,
		Shelves:    shelf, AppWriteDB: d.writeDB, Recorder: d.recorder,
		// The Claude ledger writes a different table in the same file the
		// activity log writes, so it shares that handle rather than opening a
		// second one. Nil handle, nil database, dropped row with a warning --
		// the same degradation the log makes on an instance with no app.db.
		ClaudeLedger: ledger.RecorderFrom(d.writeDB, cfg.Logger),
		// The match ledger (ADR 36) writes a third table in that same file
		// and shares the handle for the same reason. Nil handle, nil
		// database, a match recorded nowhere and a warning -- never a match
		// somebody watched play out and then lost.
		MatchLedger: matchledger.FromDB(d.writeDB, cfg.Logger),
		// The two switches the account routes read, passed rather than looked
		// up so the door and the routes it serves cannot disagree about what
		// this process is: `me` reports RequireAuth, `logout` deletes a
		// session row only when it is on, and the session cookie carries
		// `Secure` exactly when the flag says the wire is TLS.
		RequireAuth: cfg.RequireAuth, SecureCookies: cfg.SecureCookies,
		// ADR 16's seam. Nil is the real process's answer -- decide from the
		// environment when a message is actually being sent -- and a test
		// passes a recorder, which is how no test here sends mail.
		EmailSender: cfg.EmailSender, Mail: cfg.Mail,
		ClientIPHeader: cfg.ClientIPHeader})
	table, err := newRouteTable(routes.Routes())
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
		// Not a failure: a door built without the ladder having run -- a
		// test, a bare library use -- is a real state, and both halves of the
		// write side say so rather than creating a file the ladder did not:
		// the SQL deck tier reports itself read-only, the log drops the entry
		// with a warning.
		d.log.Info("no app.db; deck writes stay on the file tier and the "+
			"activity log drops its entries", "path", cfg.AppDB)
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

// Close releases what New opened — and flushes the visitor ledger, the
// shutdown flush: a stopped door loses nothing it counted.
func (d *Door) Close() error {
	d.traffic.Flush()
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Handler is the whole door as one http.Handler, outermost layer first: the
// visitor ledger's counter (so a refusal that never reached routing is still
// a request the instance answered — `(unrouted)`), then security headers,
// then the auth middleware, then compression, then dispatch. Compression
// sits innermost deliberately: the floor reads the real response, so 304s
// and small JSON stay whole, and the middleware refusals outside it go out
// uncompressed — the same layering the app has always had.
func (d *Door) Handler() http.Handler {
	base := d.securityHeaders(d.authenticate(gzipped(http.HandlerFunc(d.dispatch))))
	if d.traffic == nil {
		return base
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := &tally{template: traffic.Unrouted}
		r = r.WithContext(context.WithValue(r.Context(), tallyKey{}, t))
		sw := &statusWriter{ResponseWriter: w}
		base.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		d.traffic.Record(t.template, status)
	})
}

// tally carries the matched template from dispatch back out to the counting
// layer — never the concrete path: a path can carry a slug and a slug can
// carry a person.
type tally struct {
	template string
}

type tallyKey struct{}

func note(r *http.Request, template string) {
	if t, ok := r.Context().Value(tallyKey{}).(*tally); ok {
		t.template = template
	}
}

// statusWriter remembers the committed status for the counter. Unwrap keeps
// http.ResponseController — and so the proxy's Flush — working through it.
//
// Deliberately no Write override. Every write reaches this through
// `securityHeaders`' headerWriter, which commits WriteHeader before the first
// body byte, so a Write that defaulted the status here would be dead code —
// and the counting layer above still reads 0 as 200, for the response nobody
// wrote a byte or a header of.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// dispatch is the router, and deliberately a small one: it decides which of
// three things a path is. The API check is made on the *normalised* path, so
// `//api/decks` and `/api/./decks` are refused as JSON rather than being
// served the shell. Under /api a request either matches a route (`routes.go`
// says on what terms), matches a route's path on another method — the
// router's own 405, `Allow` carrying the first matching route's method — or
// is nothing at all, which is the catch-all's 404: `no such endpoint`, with
// the normalised path, exactly the sentence the SPA catch-all has always
// answered a stray /api request with. The static mounts match the raw path.
func (d *Door) dispatch(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Path
	if raw == DoorHealthPath {
		note(r, DoorHealthPath)
		d.health(w, r)
		return
	}
	if isAPI(NormalisePath(raw)) {
		if h, pattern, ok := d.table.match(r); ok {
			note(r, pattern)
			h.ServeHTTP(w, r)
			return
		}
		if allow, ok := d.table.allowed(r); ok {
			// The router refuses before any route runs, so no route template
			// lands in the ledger: `(unrouted)`, the same bucket every
			// before-routing refusal shares.
			w.Header().Set("Allow", allow)
			writeJSON(w, http.StatusMethodNotAllowed,
				map[string]any{"detail": "Method Not Allowed"})
			return
		}
		// The catch-all's refusal — a route template of its own, because the
		// catch-all is a route and the ledger has always counted it as one.
		note(r, "/{full_path}")
		writeJSON(w, http.StatusNotFound,
			map[string]any{"detail": "no such endpoint: " + NormalisePath(raw)})
		return
	}
	// The static tiers record their mount prefix and the shell records the
	// catch-all's template -- `/{full_path}`, read off the deployed ledger --
	// exactly the templates the ledger has always carried.
	switch {
	case raw == "/assets" || strings.HasPrefix(raw, "/assets/"):
		note(r, "/assets")
	case raw == "/tarot" || strings.HasPrefix(raw, "/tarot/"):
		note(r, "/tarot")
	default:
		note(r, "/{full_path}")
	}
	d.static.ServeHTTP(w, r)
}

// DoorHealthPath is the process's own liveness answer -- outside `/api`,
// where every path is public, and therefore not a route the shared
// classification has to carry. `/api/health` is the *instance's* health --
// the pool, the decks, the staleness flag; this path answers with nothing
// but "the process is up", which is what the platform's check, the image's
// HEALTHCHECK and the deploy's smoke test each want first.
const DoorHealthPath = "/door/health"

func (d *Door) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"detail": "Method Not Allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// securityHeaders sets the hardening headers with setdefault semantics:
// an explicit header on a particular response wins over the
// blanket one.
//
// Applied at WriteHeader time, not before the handler runs -- learned the
// hard way on the door's first run: a middleware that stamps headers up
// front meets whatever a handler adds later as a *second* copy, and the
// wire once said `nosniff, nosniff`.
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

// writeJSON answers in the app's envelope: `application/json`, and for an
// error a body with `detail`, which `web/src/lib/api.ts` reads off every
// non-2xx response. `wire` is the one encoder for both the door's own
// refusals and the routes' answers.
func writeJSON(w http.ResponseWriter, status int, body any) {
	wire.JSON(w, status, body)
}

// openSimCache attaches to `app.db` for ADR 18's simulation cache.
//
// Absent, unreadable or schema-less leaves it nil, which is a working store
// that caches nothing: a simulation still runs, it just pays full price every
// time. That is the trade `internal/sim/cache` makes in every one of its own
// error paths -- an optimisation that can turn a working simulation into a
// failed one is a bad trade -- so a failure here is a log line and not a
// refusal to start.
func (d *Door) openSimCache(cfg Config) {
	if cfg.AppDB == "" {
		return
	}
	store, err := cache.Open(cfg.AppDB, cfg.Logger)
	if err != nil {
		d.log.Info("no simulation cache; every run will be computed",
			"path", cfg.AppDB, "err", err)
		return
	}
	d.simCache = store
}
