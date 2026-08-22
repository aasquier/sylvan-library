// Package api is `src/mtglab/api`, one family at a time: the served
// application's routes as they move across the door (docs/go-migration/
// PLAN.md section 4; the port board in section 10 says which have). The door
// asks this package for its routes and answers them itself, ahead of the
// proxy; anything not listed here still goes to the Python server behind it.
//
// Three rules, each the plan's:
//
//   - A route family moves whole. A job-shaped feature's submit and poll
//     flip together because the registry is per-process; a read family
//     flips when every route in it is here and the contract suite is green
//     through the door.
//   - A route here answers exactly what Python answers: the same status,
//     the same envelope, the same shape. `tests/contract/golden/` is the
//     record and `wire` is how the bytes get written.
//   - Nothing here is a new route. `tests/test_isolation.py` requires every
//     classified path to exist in FastAPI's table, so a Go-only `/api` path
//     would be a ghost to it; a route arrives here *from* Python, and the
//     door test `TestEveryPortedRouteIsInTheSharedTable` holds the list to
//     `tests/contract/routes.json`.
package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"sync"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/shelves"
)

// Config is what the ported routes need. It grew with the families: the
// pool with the card reads, the deck library with the deck reads.
type Config struct {
	Logger *slog.Logger
	// Pool is the card pool, or nil for an instance that has none -- the
	// same degraded answers `service._connect()` returning None produces.
	Pool *pool.Pool
	// AppDB is the door's read-only `app.db` handle when auth is on; nil
	// otherwise, and then AppDBPath is opened lazily, read-only, only if the
	// file exists -- reading must not acquire a database.
	AppDB     *sql.DB
	AppDBPath string
	// DecksDir is the file tier's root.
	DecksDir string
	// AdminEmail is MTGLAB_ADMIN_EMAIL, resolved to the maintainer's handle
	// through app.db for a signed-in caller (ADR 17, ADR 22); never rendered.
	AdminEmail string
	// Shelves are the three runtime caches under the data directory; nil is
	// an instance with none, where every shelf route is a 404.
	Shelves *shelves.Shelves
	// AppWriteDB is the read-write `app.db` handle the SQL deck tier's writes
	// use, or nil. Separate from AppDB because that one is opened `mode=ro`,
	// and a write through it would fail at the driver rather than at the gate
	// that is supposed to answer.
	AppWriteDB *sql.DB
	// Recorder writes ADR 28's activity log, or nil on an instance with no
	// `app.db` -- where an edit is recorded as a warning and still succeeds,
	// because the deck write has already happened by then.
	Recorder *decklog.Recorder
}

// API holds the ported routes' dependencies.
type API struct {
	log        *slog.Logger
	pool       *pool.Pool
	db         *sql.DB
	writeDB    *sql.DB
	dbPath     string
	decksDir   string
	adminEmail string
	shelves    *shelves.Shelves
	log28      *decklog.Recorder

	lazy   sync.Mutex
	lazyDB *sql.DB
}

// New builds the ported routes.
func New(cfg Config) *API {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &API{log: cfg.Logger, pool: cfg.Pool, db: cfg.AppDB, writeDB: cfg.AppWriteDB,
		dbPath: cfg.AppDBPath, decksDir: cfg.DecksDir, adminEmail: cfg.AdminEmail,
		shelves: cfg.Shelves, log28: cfg.Recorder}
}

// recorder is the activity log's writer. Nil is a real state -- an instance
// with no `app.db` -- and `Record` on a nil Recorder warns and returns, which
// is what makes "the log never fails an edit" true at this level too.
func (a *API) recorder() *decklog.Recorder { return a.log28 }

// actor is who is asking, as the log records them: a username, or empty for
// whoever is at this machine (the CLI, and the app with auth off). **Never an
// email address**, which must not reach a log line at all (CLAUDE.md rule 5)
// -- and cannot, because the scope carries only the handle.
func (a *API) actor(ctx context.Context) string {
	return auth.ScopeFrom(ctx).Username
}

// Route is one ported route: a method, a path template in the syntax
// `tests/contract/routes.json` uses (`/api/decks/{owner}/{slug}`), and the
// handler. Path values are read with `r.PathValue(name)`.
type Route struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

// Routes is every route the Go side answers today. Order is not
// significant: the door matches on method and segments, and two routes that
// could both match one path would be a bug the door's table refuses at
// start.
func (a *API) Routes() []Route {
	return []Route{
		// The reference prose with no pool behind it (Phase 3, the first
		// family to move): fixed taxonomy, fixed vocabulary, fixed words.
		{Method: http.MethodGet, Pattern: "/api/colors", Handler: a.colors},
		{Method: http.MethodGet, Pattern: "/api/glossary", Handler: a.glossary},
		{Method: http.MethodGet, Pattern: "/api/themes", Handler: a.themes},
		// The pool behind the prose, and the pool's own two doors (the
		// second family): a combination's champions and signature cards,
		// the shelves' named cards, the search box, and the camera's reader.
		{Method: http.MethodGet, Pattern: "/api/colors/{key}", Handler: a.combination},
		{Method: http.MethodGet, Pattern: "/api/lore", Handler: a.lore},
		{Method: http.MethodGet, Pattern: "/api/cards/search", Handler: a.search},
		{Method: http.MethodPost, Pattern: "/api/cards/identify", Handler: a.identify},
		// The deck reads (the third family): the shelf, the deck, the gate's
		// verdict, the analysis, the suggestions, the commander's panel, the
		// printings, the history, the artifacts shelf -- every one resolved
		// through `Library` (ADR 22) -- and the 32 Deck Challenge's score.
		{Method: http.MethodGet, Pattern: "/api/decks", Handler: a.listDecks},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}", Handler: a.getDeck},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/validate", Handler: a.validateDeck},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/stats", Handler: a.deckStats},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/suggestions", Handler: a.suggestions},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/commander", Handler: a.commanderDossier},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/printings", Handler: a.commanderPrintings},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/log", Handler: a.deckLog},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/artifacts", Handler: a.deckArtifacts},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/artifacts/{name}", Handler: a.deckArtifact},
		{Method: http.MethodGet, Pattern: "/api/colors/progress", Handler: a.challengeProgress},
		// The runtime shelves (the fourth family, the read spine's last): a
		// mana symbol, a reading-engine file, a card-art derivative's status
		// and one of its files.
		{Method: http.MethodGet, Pattern: "/api/symbols/{code}.svg", Handler: a.symbolSVG},
		{Method: http.MethodGet, Pattern: "/api/ocr/{name}", Handler: a.ocrAsset},
		{Method: http.MethodGet, Pattern: "/api/art/motion/{oracle_id}/{effect}", Handler: a.artMotionStatus},
		{Method: http.MethodGet, Pattern: "/api/art/motion/{oracle_id}/{effect}/{filename}", Handler: a.artMotionFile},
		// The deck writes (Phase 4's first flip): the nine editing routes,
		// every one of them going out through `commit` -- so the gate's
		// verdict and ADR 28's log entry are inherited rather than
		// remembered. A deck the caller cannot see is a 404 before
		// writability is asked; one they can see but not edit is a 403.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/swap", Handler: a.swapCard},
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/cards", Handler: a.addCard},
		{Method: http.MethodDelete, Pattern: "/api/decks/{owner}/{slug}/cards/{name}", Handler: a.removeCard},
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/entomb", Handler: a.entombCards},
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/graveyard/{name}/return", Handler: a.returnCard},
		{Method: http.MethodDelete, Pattern: "/api/decks/{owner}/{slug}/graveyard/{name}", Handler: a.exileCard},
		{Method: http.MethodPatch, Pattern: "/api/decks/{owner}/{slug}/cards/{name}", Handler: a.patchCard},
		{Method: http.MethodPatch, Pattern: "/api/decks/{owner}/{slug}", Handler: a.patchDeck},
		{Method: http.MethodPut, Pattern: "/api/decks/{owner}/{slug}/notes/{key}", Handler: a.setNote},
	}
}

// Proxied is every exact path that a pattern above would capture but that
// still belongs to Python: the door hands these to the proxy before any
// pattern is consulted. FastAPI resolves a literal declared before a
// template by order; here a literal Go has not ported is named, and each
// entry leaves this list the day its route arrives in Routes.
// `/api/colors/progress` was the first entry, and left with the deck reads.
func (a *API) Proxied() []string {
	return []string{}
}

// usePool is `service._connect()` followed by the work: fn runs against a
// leased pool, and ErrNoPool -- no file, an unreadable file, or an instance
// built with no pool at all -- is returned for the handler to answer in its
// degraded shape. Any other error is the query's own and is a 500.
func (a *API) usePool(ctx context.Context, fn func(*pool.Conn) error) error {
	if a.pool == nil {
		return pool.ErrNoPool
	}
	return a.pool.Use(ctx, fn)
}

// noPoolMessage is the sentence every degraded answer carries.
const noPoolMessage = "no card pool yet -- run `mtglab data refresh`"
