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
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/shelves"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
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
	// ClaudeLedger is where a Claude conversation's accounting lands, or nil
	// on an instance with no `app.db` -- where a row is dropped with a warning
	// and the conversation still answers, because the call has already been paid
	// for by the time there is anything to record.
	ClaudeLedger *ledger.Recorder
	// RequireAuth mirrors MTGLAB_REQUIRE_AUTH. Two account routes read it and
	// nothing else does: `me` reports it, and `logout` deletes the session row
	// only when it is on -- both exactly as `auth.install(require=…)` does.
	RequireAuth bool
	// SecureCookies mirrors MTGLAB_SECURE_COOKIES: the `Secure` attribute on
	// the session cookie, on once TLS fronts the app.
	SecureCookies bool
	// EmailSender is ADR 16's seam reaching the edge. Nil means "decide from
	// the environment, when a message is actually being sent", which is what a
	// real process wants -- and which is also why no test here sends mail: the
	// tests pass a recorder instead.
	EmailSender auth.EmailSender
	// Jobs is this process's job registry, or nil before any job family has
	// flipped. Nil is what makes the two generic poll routes pure fall-through
	// again, which is the state PLAN section 10 describes and the state every
	// test that does not care about jobs runs in.
	Jobs *jobs.Registry
	// SimCache is ADR 18's `sim_cache` table, or nil for an instance with no
	// `app.db`. A nil `*cache.Store` is a working store that caches nothing,
	// so no caller branches on it.
	SimCache *cache.Store
	// Upstream is the door's proxy, and it exists for exactly one reason: the
	// **hybrid poll handler**.
	//
	// PLAN section 10 named this as the awkward part -- "it inverts the door's
	// dependency (the proxy is chosen in `door` when `match` misses; `door`
	// imports `api`, not the reverse)". The inversion is avoided by passing a
	// plain `http.Handler` rather than importing anything: the door builds its
	// proxy before it builds this package, and hands it over. `api` never
	// learns what an upstream is.
	//
	// Nil means "no upstream", and the poll routes then answer from Go's
	// registry alone -- correct for a test, and correct after Phase 8 when
	// there is no Python left to ask.
	Upstream http.Handler
}

// API holds the ported routes' dependencies.
type API struct {
	log           *slog.Logger
	pool          *pool.Pool
	db            *sql.DB
	writeDB       *sql.DB
	dbPath        string
	decksDir      string
	adminEmail    string
	shelves       *shelves.Shelves
	log28         *decklog.Recorder
	claudeLedger  *ledger.Recorder
	requireAuth   bool
	secureCookies bool
	email         auth.EmailSender
	jobs          *jobs.Registry
	simCache      *cache.Store
	upstream      http.Handler

	lazy        sync.Mutex
	lazyDB      *sql.DB
	lazyWriteDB *sql.DB

	// bg tracks the work started after a response has gone -- Starlette's
	// `BackgroundTasks`, which one route needs (`POST /api/auth/reset`, whose
	// whole timing argument is that the lookup happens where nobody is
	// waiting). Only tests wait on it.
	bg sync.WaitGroup
}

// New builds the ported routes.
func New(cfg Config) *API {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &API{log: cfg.Logger, pool: cfg.Pool, db: cfg.AppDB, writeDB: cfg.AppWriteDB,
		dbPath: cfg.AppDBPath, decksDir: cfg.DecksDir, adminEmail: cfg.AdminEmail,
		shelves: cfg.Shelves, log28: cfg.Recorder, requireAuth: cfg.RequireAuth,
		secureCookies: cfg.SecureCookies, email: cfg.EmailSender,
		jobs: cfg.Jobs, simCache: cfg.SimCache, upstream: cfg.Upstream,
		claudeLedger: cfg.ClaudeLedger}
}

// background runs fn after the response has gone, which is Starlette's
// `BackgroundTasks` and, for the one route that uses it, the whole design:
// `POST /api/auth/reset` must cost the same whether or not the address
// resolves, and it can only do that if the lookup happens where nobody is
// timing it.
//
// The context is detached from the request's, because the request is over --
// a cancelled one must not abort a message that is already owed. It carries a
// ceiling so a hung mail provider cannot leak a goroutine per attempt.
func (a *API) background(fn func(context.Context)) {
	a.bg.Add(1)
	go func() {
		defer a.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), backgroundBudget)
		defer cancel()
		fn(ctx)
	}()
}

// backgroundBudget bounds one background task. Comfortably past `MailTimeout`
// plus a database read, and far short of anything that would pile up.
const backgroundBudget = 30 * time.Second

// WaitBackground blocks until every background task has finished. **For tests
// only** -- production never calls it, so the code path under test is the one
// that ships. A test that ran the task inline instead would be measuring
// something else: the asynchrony is the property, not an implementation
// detail.
func (a *API) WaitBackground() { a.bg.Wait() }

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
		// The Claude surface's two free corners (Phase 6, the pure half):
		// a checked-in roster of voices, and a seeded deal. Neither needs a
		// key, a pool or a network, so both answer on a base install --
		// and the deal is internal/pyrand's first served caller, where a
		// seed minted before the cutover must still deal its own spread.
		{Method: http.MethodGet, Pattern: "/api/claude/personas", Handler: a.personaRoster},
		// The dial itself, which crosses LAST of the free corners rather than
		// first: it reports which modes are built and what each surface
		// defaults to, so it could only answer honestly once the modes and
		// their stance owners had crossed. Free, reaching no network and no
		// pool -- but it does resolve the caller's library, because Python
		// passes `lib.source_for(...)` as an argument and `?owner=nobody`
		// alone is therefore a 404.
		{Method: http.MethodGet, Pattern: "/api/claude", Handler: a.claudeStatus},
		{Method: http.MethodGet, Pattern: "/api/tarot/reading", Handler: a.tarotReading},
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
		// The deck lifecycle (Phase 4's second flip): the moments a deck
		// begins and ends. None of these goes through `commit` -- creation
		// and deletion are outside `service._commit` in Python and therefore
		// outside ADR 28's log, which is a decision to keep rather than one
		// to drift out of. The two collection routes write into the caller's
		// **own** library and never into an owner named in a path.
		{Method: http.MethodPost, Pattern: "/api/decks", Handler: a.createDeck},
		{Method: http.MethodPost, Pattern: "/api/decks/import", Handler: a.importDeck},
		{Method: http.MethodDelete, Pattern: "/api/decks/{owner}/{slug}", Handler: a.deleteDeck},
		{Method: http.MethodPut, Pattern: "/api/decks/{owner}/{slug}/shared", Handler: a.setDeckShared},
		// The artifacts rebuild, and with it every route under `/api/decks` is
		// the door's: the five deliverables, derived from the deck rather than
		// edited into it. A **plain route**
		// and not a job, which was measured rather than assumed -- 70-83ms
		// warm across four real decks on the instance. Like the lifecycle
		// above it does not go through `commit`, and for a sharper reason:
		// this changes no deck field at all, so ADR 28 has nothing to record.
		// The two GETs on the same path flipped with the deck reads.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/artifacts", Handler: a.buildArtifacts},
		// The rationale interview (Phase 6's first flipped Claude surface):
		// one plain route, because the mode is handed its facts rather than
		// sent shopping for them and answers in the seconds class. It reaches
		// exactly as far as the deck does -- `Library` resolves the owner, and
		// the same source goes to the tools the conversation may reach for.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/interview", Handler: a.rationaleInterview},
		// The slot argument (ADR 25), the interview's twin: same shape, same
		// status codes, opposite direction. `.../argue/deck` is the same mode
		// swept across a selection and is a JOB, so it stays Python's until
		// that lane crosses -- a prefix is not a family.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/argue", Handler: a.argueSlot},
		// The commander dossier (ADR 19), both halves: the free GET that reads
		// the store and never calls, and the POST that writes one as a JOB on
		// the NET lane -- the first Claude job family to answer from the door,
		// landing in the registry the hybrid poll routes below already read.
		// A stored dossier and a stance of off are jobs born finished; no
		// commander the pool knows is a 422 in the request, never four minutes
		// later.
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/dossier", Handler: a.claudeDossierCached},
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/dossier", Handler: a.claudeDossier},
		// Research (ADR 26): a job, outside /api/decks on purpose, taking no
		// owner and no deck -- the absence is the contract. Deduplicated in
		// flight on the normalised question, cached never.
		{Method: http.MethodPost, Pattern: "/api/claude/research", Handler: a.claudeResearch},
		// The theme interview (ADR 20), both halves, both JOBS on the NET
		// lane and both keyed on nothing -- two turns in flight are two
		// different conversations, which is the opposite call from research's
		// dedupe one line up. Outside /api/decks like research and for a
		// sharper reason: this surface runs BEFORE a deck exists, and neither
		// handler takes a deck source. A floor not yet reached is the 409 no
		// other Claude route has.
		{Method: http.MethodPost, Pattern: "/api/claude/theme", Handler: a.claudeTheme},
		{Method: http.MethodPost, Pattern: "/api/claude/theme/proposal", Handler: a.claudeThemeProposal},
		// The accounts (Phase 4's last flip): the five public doors of
		// `api/auth.py` and the `me` that says who is standing in them, then
		// the admin surface of `api/admin.py`. The middleware half of
		// `api/auth.py` crossed in Phase 2 and lives in `internal/door`;
		// these are the routes it lets through.
		//
		// **Every one of the first six is in `PUBLIC_PATHS`**, which is the
		// load-bearing fact about them: reachable with no session, so each is
		// rate limited and each is written so a refusal tells the caller
		// nothing it did not already know.
		{Method: http.MethodPost, Pattern: "/api/auth/login", Handler: a.login},
		{Method: http.MethodPost, Pattern: "/api/auth/logout", Handler: a.logout},
		{Method: http.MethodPost, Pattern: "/api/auth/reset", Handler: a.requestReset},
		{Method: http.MethodPost, Pattern: "/api/auth/claim", Handler: a.claim},
		{Method: http.MethodPost, Pattern: "/api/auth/claim/preview", Handler: a.claimPreview},
		{Method: http.MethodGet, Pattern: "/api/auth/me", Handler: a.me},
		// The admin surface. The door refuses this prefix to a non-admin
		// before routing (ADR 17, and 403 rather than ADR 5's 404 because an
		// admin route's existence is published in a public repository);
		// `requireAdmin` on each handler is the second check, and the only
		// one an admin route mounted somewhere else by mistake would have.
		//
		// `DELETE /api/admin/users/{username}` is deliberately absent and
		// still Python's: it calls `jobs.forget_owner` on a registry held in
		// the uvicorn process's memory, and `users.id` is re-issued by
		// SQLite, so jobs left keyed on a freed id would be handed to the
		// next account created. `internal/api/admin.go` argues it; it flips
		// with the jobs family. A DELETE on that path simply matches no route
		// here and goes to the proxy as it arrived.
		{Method: http.MethodGet, Pattern: "/api/admin/users", Handler: a.listAccounts},
		{Method: http.MethodPost, Pattern: "/api/admin/users", Handler: a.inviteAccount},
		{Method: http.MethodPatch, Pattern: "/api/admin/users/{username}", Handler: a.updateAccount},
		{Method: http.MethodPost, Pattern: "/api/admin/users/{username}/reset", Handler: a.sendAccountReset},
		{Method: http.MethodDelete, Pattern: "/api/admin/users/{username}/sessions", Handler: a.revokeSessions},

		// The sim family, and with it the two generic job routes -- Phase 5's
		// flip. `/api/sim/forge` is deliberately absent: it needs `sim/tier3`,
		// which is Phase 7's, so it stays Python's and keeps the hybrid poll
		// handler's proxy branch live alongside the five Claude families.
		{Method: http.MethodPost, Pattern: "/api/sim/mana", Handler: a.simMana},
		{Method: http.MethodPost, Pattern: "/api/sim/lands", Handler: a.simLands},
		{Method: http.MethodPost, Pattern: "/api/sim/shelf", Handler: a.simShelf},
		{Method: http.MethodPost, Pattern: "/api/sim/policy", Handler: a.simPolicy},
		{Method: http.MethodGet, Pattern: "/api/jobs", Handler: a.listJobs},
		{Method: http.MethodGet, Pattern: "/api/jobs/{job_id}", Handler: a.getJob},
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
