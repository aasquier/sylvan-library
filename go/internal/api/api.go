// Package api is every /api route family: the served
// application's routes, which the door asks for and answers itself.
//
// Three rules, all standing:
//
//   - A route family lives whole, one file per family. A job-shaped
//     feature's submit and poll belong together because the registry is
//     per-process.
//   - A route answers exactly the recorded wire contract: the same status,
//     the same envelope, the same shape. The in-package tests and the
//     frozen goldens are the record, and `wire` is how the bytes get
//     written.
//   - No route is invented in passing. The table this package hands the
//     door is the whole served surface, and the door's auth sweeps derive
//     from it -- so a new path is a deliberate addition, deny-by-default
//     at the middleware until `PublicPaths` says otherwise.
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
	"github.com/aasquier/sylvan-library/go/internal/flymetrics"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/shelves"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	matchledger "github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
	"github.com/aasquier/sylvan-library/go/internal/traffic"
)

// Config is what the routes need. It grew with the families: the
// pool with the card reads, the deck library with the deck reads.
type Config struct {
	Logger *slog.Logger
	// Pool is the card pool, or nil for an instance that has none --
	// answered in the recorded degraded shapes, never refused.
	Pool *pool.Pool
	// AppDB is the door's read-only `app.db` handle when auth is on; nil
	// otherwise, and then AppDBPath is opened lazily, read-only, only if the
	// file exists -- reading must not acquire a database.
	AppDB     *sql.DB
	AppDBPath string
	// DecksDir is the file tier's root.
	DecksDir string
	// ScryfallDir is where `data refresh` parks the bulk downloads; `health`
	// lists them. Empty means an instance that has never refreshed.
	ScryfallDir string
	// DataDir is MTGLAB_DATA_DIR, for the storage and system views that size
	// and statfs the volume.
	DataDir string
	// PoolPath is the card pool file, sized by the storage view and never
	// opened by it.
	PoolPath string
	// Traffic is the visitor ledger's recorder, shared with the door so the
	// stats view's read flushes the same buffer the door fills. Nil records
	// nothing and summarises an empty ledger.
	Traffic *traffic.Recorder
	// Fly is the metrics panel, or nil for one built over the real
	// transport; a test injects a Panel with a stub.
	Fly *flymetrics.Panel
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
	// MatchLedger is where a finished Forge match lands (ADR 36), or nil on
	// an instance with no `app.db` -- where the row is dropped with a warning
	// and the match still answers, because the JVM minutes have already been
	// spent by the time there is anything to record.
	MatchLedger *matchledger.Recorder
	// ForgeWorker is the hosted Forge client, or nil for the default one. A
	// field so a test can point the pre-flight and the match at a stub shim
	// without an environment variable deciding where the real one lives.
	ForgeWorker *tier3.Worker
	// RequireAuth mirrors MTGLAB_REQUIRE_AUTH. Two account routes read it and
	// nothing else does: `me` reports it, and `logout` deletes the session row
	// only when it is on -- facts about this process, passed rather than
	// looked up.
	RequireAuth bool
	// SecureCookies mirrors MTGLAB_SECURE_COOKIES: the `Secure` attribute on
	// the session cookie, on once TLS fronts the app.
	SecureCookies bool
	// EmailSender is ADR 16's seam reaching the edge. Nil means "choose from
	// Mail, when a message is actually being sent", which is what a real
	// process wants -- and which is also why no test here sends mail: the
	// tests pass a recorder instead.
	EmailSender auth.EmailSender
	// Jobs is this process's job registry. Nil refuses every submit with a
	// 503 -- a state no serving process is in, and the state every test
	// that does not care about jobs runs in.
	Jobs *jobs.Registry
	// SimCache is ADR 18's `sim_cache` table, or nil for an instance with no
	// `app.db`. A nil `*cache.Store` is a working store that caches nothing,
	// so no caller branches on it.
	SimCache *cache.Store
	// Mail is what EmailSender falls back to when it is nil: the three
	// settings `auth.SenderFor` chooses between. Passed rather than looked up,
	// so a test describes an instance with no mail key instead of unsetting
	// one on the process it shares with every other test.
	Mail auth.MailSettings
	// ClientIPHeader is MTGLAB_CLIENT_IP_HEADER: the header a trusted proxy
	// sets to the real client IP, or empty to trust only the peer. Empty is
	// the safe default and the one every test wants.
	ClientIPHeader string
}

// API holds the routes' dependencies.
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
	// mail is what emailSender falls back to when no sender was injected:
	// the settings, resolved once at config.Load, rather than the environment
	// read at the moment of sending.
	mail           auth.MailSettings
	clientIPHeader string
	jobs           *jobs.Registry
	simCache       *cache.Store
	matchLedgerOf  *matchledger.Recorder
	forgeClient    *tier3.Worker

	lazy        sync.Mutex
	lazyDB      *sql.DB
	lazyWriteDB *sql.DB

	scryfallDir string
	dataDir     string
	poolPath    string
	traffic     *traffic.Recorder
	fly         *flymetrics.Panel

	// The upcoming-sets answer, held for the day it was fetched on -- a
	// process-lifetime cache, kept as marshalled
	// bytes so a replay is byte-identical.
	setsMu   sync.Mutex
	setsDay  string
	setsBody []byte

	// bg tracks the work started after a response has gone, which one
	// route needs (`POST /api/auth/reset`, whose
	// whole timing argument is that the lookup happens where nobody is
	// waiting). Only tests wait on it.
	bg sync.WaitGroup
}

// New builds the route family.
func New(cfg Config) *API {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Fly == nil {
		cfg.Fly = &flymetrics.Panel{Log: cfg.Logger}
	}
	return &API{log: cfg.Logger, pool: cfg.Pool, db: cfg.AppDB, writeDB: cfg.AppWriteDB,
		dbPath: cfg.AppDBPath, decksDir: cfg.DecksDir, scryfallDir: cfg.ScryfallDir,
		dataDir: cfg.DataDir, poolPath: cfg.PoolPath, traffic: cfg.Traffic,
		fly:        cfg.Fly,
		adminEmail: cfg.AdminEmail,
		shelves:    cfg.Shelves, log28: cfg.Recorder, requireAuth: cfg.RequireAuth,
		secureCookies: cfg.SecureCookies, email: cfg.EmailSender,
		jobs: cfg.Jobs, simCache: cfg.SimCache,
		claudeLedger: cfg.ClaudeLedger, matchLedgerOf: cfg.MatchLedger,
		forgeClient: cfg.ForgeWorker,
		mail:        cfg.Mail, clientIPHeader: cfg.ClientIPHeader}
}

// background runs fn after the response has gone, which is, for the one
// route that uses it, the whole design:
// `POST /api/auth/reset` must cost the same whether or not the address
// resolves, and it can only do that if the lookup happens where nobody is
// timing it.
//
// The context is detached from the request's, because the request is over --
// a cancelled one must not abort a message that is already owed. It carries a
// ceiling so a hung mail provider cannot leak a goroutine per attempt.
func (a *API) background(fn func(context.Context)) {
	a.bg.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), backgroundBudget)
		defer cancel()
		fn(ctx)
	})
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

// Route is one served route: a method, a path template in the app's
// template syntax (`/api/decks/{owner}/{slug}`), and the
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
		// The reference prose with no pool behind it: fixed taxonomy,
		// fixed vocabulary, fixed words.
		{Method: http.MethodGet, Pattern: "/api/health", Handler: a.health},
		{Method: http.MethodGet, Pattern: "/api/sets/upcoming", Handler: a.upcomingSets},
		{Method: http.MethodGet, Pattern: "/api/colors", Handler: a.colors},
		{Method: http.MethodGet, Pattern: "/api/glossary", Handler: a.glossary},
		{Method: http.MethodGet, Pattern: "/api/themes", Handler: a.themes},
		// The Claude surface's two free corners:
		// a checked-in roster of voices, and a seeded deal. Neither needs a
		// key, a pool or a network, so both answer on a base install --
		// and the deal is internal/mt19937's first served caller, where a
		// seed a browser has held for months must still deal its own spread.
		{Method: http.MethodGet, Pattern: "/api/claude/personas", Handler: a.personaRoster},
		// The dial itself: it reports which modes are built and what each
		// surface defaults to. Free, reaching no network and no
		// pool -- but it does resolve the caller's library, because the
		// owner is resolved as an argument before anything else is
		// consulted, and `?owner=nobody`
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
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/wheel", Handler: a.deckWheel},
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
		// The deck writes: the nine editing routes,
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
		// The deck lifecycle: the moments a deck
		// begins and ends. None of these goes through `commit` -- creation
		// and deletion are deliberately
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
		// The two GETs on the same path are listed with the deck reads.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/artifacts", Handler: a.buildArtifacts},
		// The rationale interview:
		// one plain route, because the mode is handed its facts rather than
		// sent shopping for them and answers in the seconds class. It reaches
		// exactly as far as the deck does -- `Library` resolves the owner, and
		// the same source goes to the tools the conversation may reach for.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/interview", Handler: a.rationaleInterview},
		// The slot argument (ADR 25), the interview's twin: same shape, same
		// status codes, opposite direction. The single card is a plain route
		// on its measured seconds...
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/argue", Handler: a.argueSlot},
		// ...and the same mode swept across a selection, which is a JOB: one
		// call per card, so a few dozen slots is minutes. One job for the
		// whole sweep and sequential inside it, because N jobs would occupy
		// the two-wide NET lane and starve every sibling surface.
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/argue/deck", Handler: a.argueSweep},
		// The commander dossier (ADR 19), both halves: the free GET that reads
		// the store and never calls, and the POST that writes one as a JOB on
		// the NET lane,
		// landing in the registry the poll routes below read.
		// A stored dossier and a stance of off are jobs born finished; no
		// commander the pool knows is a 422 in the request, never four minutes
		// later.
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}/dossier", Handler: a.claudeDossierCached},
		{Method: http.MethodPost, Pattern: "/api/decks/{owner}/{slug}/dossier", Handler: a.claudeDossier},
		// Research (ADR 26): a job, outside /api/decks on purpose, taking no
		// owner and no deck -- the absence is the contract. Deduplicated in
		// flight on the normalised question, cached never.
		{Method: http.MethodPost, Pattern: "/api/claude/research", Handler: a.claudeResearch},
		// The camera's fallback tier (ADR 34): the one route in the app that
		// receives a photograph, and a JOB whose duration is UNMEASURED --
		// which is the reason it is a job rather than an argument that it
		// needs to be one. Like research it takes no owner and no deck; unlike
		// research its dedupe key is the picture itself, so two presses on one
		// shot are one paid call. What comes back is not a card: `identify`
		// decides that, through the same function the camera's own route
		// calls.
		{Method: http.MethodPost, Pattern: "/api/claude/scan", Handler: a.claudeScan},
		// The theme interview (ADR 20), both halves, both JOBS on the NET
		// lane and both keyed on nothing -- two turns in flight are two
		// different conversations, which is the opposite call from research's
		// dedupe one line up. Outside /api/decks like research and for a
		// sharper reason: this surface runs BEFORE a deck exists, and neither
		// handler takes a deck source. A floor not yet reached is the 409 no
		// other Claude route has.
		{Method: http.MethodPost, Pattern: "/api/claude/theme", Handler: a.claudeTheme},
		{Method: http.MethodPost, Pattern: "/api/claude/theme/proposal", Handler: a.claudeThemeProposal},
		// The accounts: the five public doors and
		// the `me` that says who is standing in them, then
		// the admin surface. The middleware half
		// lives in `internal/door`;
		// these are the routes it lets through.
		//
		// **Every one of the first six is on `door.PublicPaths`**, which is
		// the load-bearing fact about them: reachable with no session, so
		// each is rate limited and each is written so a refusal tells the
		// caller nothing it did not already know.
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
		// `DELETE /api/admin/users/{username}` forgets the account's jobs in
		// this process's registry before the row goes, because `users.id` is
		// re-issued by SQLite and a job left keyed on a freed id would be
		// handed to the next account created. `admin.go` argues it.
		{Method: http.MethodGet, Pattern: "/api/admin/users", Handler: a.listAccounts},
		{Method: http.MethodPost, Pattern: "/api/admin/users", Handler: a.inviteAccount},
		{Method: http.MethodPatch, Pattern: "/api/admin/users/{username}", Handler: a.updateAccount},
		{Method: http.MethodPost, Pattern: "/api/admin/users/{username}/reset", Handler: a.sendAccountReset},
		{Method: http.MethodDelete, Pattern: "/api/admin/users/{username}/sessions", Handler: a.revokeSessions},
		// The account deletion, and the dashboard's six stats views.
		{Method: http.MethodDelete, Pattern: "/api/admin/users/{username}", Handler: a.deleteAccount},
		{Method: http.MethodGet, Pattern: "/api/admin/stats/system", Handler: a.statsSystem},
		{Method: http.MethodGet, Pattern: "/api/admin/stats/storage", Handler: a.statsStorage},
		{Method: http.MethodGet, Pattern: "/api/admin/stats/claude", Handler: a.statsClaude},
		{Method: http.MethodGet, Pattern: "/api/admin/stats/activity", Handler: a.statsActivity},
		{Method: http.MethodGet, Pattern: "/api/admin/stats/traffic", Handler: a.statsTraffic},
		{Method: http.MethodGet, Pattern: "/api/admin/stats/fly", Handler: a.statsFly},

		// The sim family, and with it the two generic job routes.
		{Method: http.MethodPost, Pattern: "/api/sim/mana", Handler: a.simMana},
		{Method: http.MethodPost, Pattern: "/api/sim/lands", Handler: a.simLands},
		{Method: http.MethodPost, Pattern: "/api/sim/shelf", Handler: a.simShelf},
		{Method: http.MethodPost, Pattern: "/api/sim/policy", Handler: a.simPolicy},

		// Tier 3 (ADR 35). `GET /api/forge` is the gate the Simulator asks
		// first; `POST /api/sim/forge` is the match.
		{Method: http.MethodGet, Pattern: "/api/forge", Handler: a.forgeGate},
		{Method: http.MethodPost, Pattern: "/api/sim/forge", Handler: a.simForge},
		{Method: http.MethodGet, Pattern: "/api/jobs", Handler: a.listJobs},
		{Method: http.MethodGet, Pattern: "/api/jobs/{job_id}", Handler: a.getJob},
	}
}

// usePool leases the pool and runs the work: fn runs against a
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
