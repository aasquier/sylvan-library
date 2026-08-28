package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Upkeep: the two things about this instance that go out of date on their
// own, and what can honestly be done about each from a browser.
//
// # The library refresh, and why it is here at all
//
// **ADR 6 wrote this endpoint's specification and then declined to build
// it**: *"If refreshing by hand turns out to be forgotten in practice, the
// next step is an authenticated admin endpoint that starts a refresh as a
// background job, called on a schedule from outside. Build that only after
// forgetting it twice."* Aaron asked for the button on 2026-08-28, which
// settles the forgetting question, so this is that clause cashed in -- the
// endpoint ADR 6 described, with the constraints it attached still load-
// bearing:
//
//   - **It cannot be synchronous.** ~500MB and several minutes. It is a job
//     on the registry, and the runner takes `context.Background()` rather
//     than the request's, because the request is over long before the work
//     is.
//   - **It holds the pool's exclusive write lock**, so card lookups are
//     unavailable while it runs (ADR 2). That is by design, it is said out
//     loud on the page before anybody commits to it, and
//     [pool.Pool.Seal] is what makes it true of this process rather than
//     merely of the file.
//   - **An accidental second run is made hard**, because Scryfall asks that
//     needless bulk traffic not be generated: one at a time by
//     construction ([refreshLatch]), the control disables itself while the
//     job runs, and the state route below is what lets a reloaded page find
//     the run already in progress instead of offering to start another.
//
// Nothing about how the bulk data is fetched changed: the same descriptive
// User-Agent, the same one bulk file rather than the per-card API, the same
// hot-linked images. Those are ADR 6's other three constraints and this
// endpoint is not the place they would be regressed -- it calls
// `pool.Refresh`, which is the same sequence `mtglab data refresh` runs.
//
// # The arena, and why there is no button for it
//
// Aaron asked the same question about the machine that plays the games:
// could a button refresh and re-cut it? The honest answer is no, and the
// reasons are worth having written down where the next person looks:
//
//   - **This process cannot cut an image.** The worker is built by the
//     delivery pipeline from `Dockerfile.forge`; there is no builder inside
//     the app, and giving the app a credential that could start one would be
//     handing a web page write access to the repository.
//   - **Which Forge the arena plays is a decision, not a refresh.** ADR 36
//     records `forge_version` on every match precisely because Forge's AI is
//     the instrument each recorded game was measured with, so an upgrade
//     changes the judge rather than a dependency. A watcher already checks
//     weekly and prepares the decision; a person takes it.
//   - **The part that genuinely must not go stale already does not.** The
//     arena is re-cut with every release, and its security layer is keyed on
//     the date, so it is at most a day behind with nobody in the loop.
//
// What was missing was not a lever but a *reading*: nowhere in the app could
// you see which Forge the arena is actually playing with. [API.upkeep]
// answers that from the match ledger -- the version recorded against the last
// game actually played here, which is a fact rather than a claim about a
// build.

// LibraryRefreshKind is the job kind, in the family spelling the other kinds
// use. A refresh belongs to the instance rather than to a person, but it is
// still owned by the admin who started it: the registry scopes lookups by
// owner (ADR 5), and an unowned job would be invisible to the tab that
// started it the moment auth is on.
const LibraryRefreshKind = "library.refresh"

// SealBudget is how long a refresh will wait for the app's own readers to
// finish before it gives up and says the library is busy.
//
// **The same minute `pool.WriterWait` allows a second process**, and for the
// same reason: a lease is ten seconds and every page load pushes it out
// another ten, so a minute of that is the honest point at which the answer is
// that somebody is reading. It is a *separate* budget from WriterWait rather
// than the same one because they guard different doors -- this one is this
// process's own readers, that one is anybody else's -- and a refresh may
// spend both.
const SealBudget = pool.WriterWait

// refreshLatch is the one-refresh-at-a-time guard.
//
// **The registry cannot be the guard here, and that is the whole reason this
// type exists.** `jobs.Options.Key` dedupes per *owner* (deliberately -- a
// job belongs to a person, and handing two accounts one id would 404 the
// second), so two admins pressing the button would start two five-hundred-
// megabyte downloads against a file only one of them can hold. This is per
// *process*, which is the scope the write lock actually has.
//
// It holds the job id as well as the flag, so a page reloaded mid-refresh
// re-attaches to the run in progress rather than being offered a second one.
type refreshLatch struct {
	mu    sync.Mutex
	held  bool
	jobID string
}

// claim takes the latch, or reports that somebody already has it. The id
// arrives later, from [refreshLatch.name], because it does not exist until
// the registry has minted it.
func (l *refreshLatch) claim() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return false
	}
	l.held = true
	l.jobID = ""
	return true
}

func (l *refreshLatch) name(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.jobID = id
}

// release hands the latch back. Called from the worker's defer, so a panic
// inside the refresh -- which the registry recovers into a failed job --
// still frees it. A latch that outlived its reason would make the button dead
// until the process restarted.
func (l *refreshLatch) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.held = false
	l.jobID = ""
}

// running is the id of the refresh in progress, if there is one.
func (l *refreshLatch) running() (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.jobID, l.held
}

// libraryTrouble is the room's own words for a refresh that could not be
// done, and it is the only place a refresh failure becomes a sentence
// somebody reads.
//
// **Shaped after `forgeTrouble` on purpose**, and for the same reason it
// exists: everything underneath this button is technology a user may never
// see (commandment 10), and an admin is still a user. The diagnosis -- the
// lock message, the status the other end sent, the path that would not open
// -- goes to the log, where the person who can act on it will look.
//
// Three answers, because three different things are true:
func libraryTrouble(err error) string {
	switch {
	case err == nil:
		return ""
	// Somebody was reading the library while we tried to rebuild it. **An
	// ordinary thing that happened rather than a fault**, and the likeliest
	// outcome on a busy instance: a reader's hold is renewed by every page
	// load, so a minute of steady traffic is enough. It says what to do about
	// it because waiting really is the answer.
	//
	// Deliberately **not** every failure of that phase. A directory that is
	// not there and a permission that is wrong also fail before the gathering
	// starts, and calling either of those "busy" would send somebody away to
	// wait for a thing that will never change.
	case errors.Is(err, pool.ErrStillReading), errors.Is(err, pool.ErrSealed),
		pool.Locked(err):
		return "the library was busy being read and would not come free, so " +
			"nothing was changed. Try again in a few minutes"
	// The cards did not arrive, or the gathering broke off partway. Nothing
	// has been shelved, so the shelves are as they were.
	case pool.PhaseOf(err) == pool.PhaseGather:
		return "the cards could not be gathered just now — nothing came " +
			"back, or it stopped partway. The shelves are as they were; " +
			"try again later"
	default:
		return "the gathering broke off before the shelves were finished, " +
			"and nothing here can say why. Try again; if it keeps stopping, " +
			"the library will have to be gathered by hand"
	}
}

// refreshLibrary is `POST /api/admin/library/refresh`: start the gathering as
// a background job.
//
// 202, because the work has been accepted and not done -- the same claim
// `POST /api/admin/users/{username}/reset` makes about a message handed to a
// provider. 409 when one is already running, which is a conflict with the
// state of the world rather than a malformed request, and it carries the id
// of the run already going so the page can attach to it.
func (a *API) refreshLibrary(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	// Two refusals about how this deployment is put together rather than about
	// the request, and both are said in plain words for the same reason the
	// failures below are: an admin is still a user, and neither of these
	// sentences may name what is missing. Nothing serving can reach either --
	// a real process always has both -- which is exactly why they are easy to
	// get wrong and worth writing carefully once.
	if a.jobs == nil {
		wire.Detail(w, http.StatusServiceUnavailable,
			"nothing here can take on slow work just now")
		return
	}
	if a.poolPath == "" {
		wire.Detail(w, http.StatusServiceUnavailable,
			"there is nowhere here to keep a library")
		return
	}
	if !a.gathering.claim() {
		id, _ := a.gathering.running()
		wire.JSON(w, http.StatusConflict, wire.OrderedMap{
			{Key: "detail", Value: "a gathering is already under way"},
			{Key: "job_id", Value: nilIfEmpty(id)},
		})
		return
	}

	owner := auth.ScopeFrom(r.Context()).UserID
	job, err := a.jobs.FromPlan(jobs.Plan{
		Kind:  LibraryRefreshKind,
		Label: "the card library",
		// The CPU lane, and the choice is between two poor fits rather than
		// one good one. NET is two wide and its width is about *money* -- a
		// Claude call costs per run -- so parking a five-minute download
		// there would halve a budget that has nothing to do with downloads.
		// CPU is one wide on the instance, which looks worse and is not:
		// everything else in that lane needs the card pool, and the card pool
		// is shut for the duration, so queueing behind the refresh is
		// strictly better for those jobs than running beside it and failing.
		Lane: jobs.CPU,
		Run:  a.gatherTheLibrary,
	}, owner)
	if err != nil {
		a.gathering.release()
		a.log.Error("the library refresh could not be queued", "error", err)
		wire.Detail(w, http.StatusInternalServerError,
			"the gathering could not be set going")
		return
	}
	a.gathering.name(job.ID)
	a.log.Warn("a library refresh was started", "by", a.actor(r.Context()),
		"job", job.ID)
	wire.JSON(w, http.StatusAccepted, job.Payload())
}

// gatherTheLibrary is the worker.
//
// **It takes `context.Background()` and never the request's**, which is the
// one thing that would silently break this whole feature: the response goes
// out in milliseconds and the request's context is cancelled with it, so a
// runner that reached for it would cancel a download it had barely started.
//
// The seal comes first and is undone in a defer, so a refresh that fails, or
// panics into the registry's recover, still gives the library back.
func (a *API) gatherTheLibrary(rep jobs.Progress) (any, error) {
	defer a.gathering.release()
	ctx := context.Background()

	rep.ReportPartial(0, gatherSteps, gatherPartial(sayClearing))
	if a.pool != nil {
		reopen, err := a.pool.Seal(ctx, SealBudget)
		if err != nil {
			a.log.Error("the library would not come free for a refresh", "error", err)
			return nil, errors.New(libraryTrouble(err))
		}
		defer reopen()
	}

	step := 0
	advance := func(saying string) {
		step++
		rep.ReportPartial(step, gatherSteps, gatherPartial(saying))
	}

	counts, err := pool.Refresh(ctx, pool.RefreshOptions{
		DBPath:      a.poolPath,
		ScryfallDir: a.scryfallDir,
	}, pool.RefreshWatcher{
		Gathering: func(kind string) {
			if kind == pool.OracleBulk {
				advance(sayGatheringCards)
			} else {
				advance(sayGatheringPrints)
			}
		},
		Shelved: func(kind string, _ int64) {
			if kind == pool.OracleBulk {
				advance(sayShelvingCards)
			} else {
				advance(sayShelvingPrints)
			}
		},
	})
	if err != nil {
		// The whole diagnosis, once, where the person who can act on it will
		// look. What comes back is the room's words.
		a.log.Error("the library refresh did not finish",
			"phase", string(pool.PhaseOf(err)), "error", err)
		return nil, errors.New(libraryTrouble(err))
	}
	a.log.Warn("the library was gathered again",
		"cards", counts.Oracle, "printings", counts.Printings)
	return wire.OrderedMap{
		{Key: "cards", Value: counts.Oracle},
		{Key: "printings", Value: counts.Printings},
	}, nil
}

// The beats a gathering has, in order, in the words a person reads while they
// wait.
//
// Named and listed rather than written inline at the four call sites, so the
// vocabulary sweep in the tests is over **this** list and not over a second
// copy of it that would drift the first time one was reworded. (A grep-built
// "all of them" list loses the odd spelling; a discovered one cannot.)
const (
	sayClearing        = "clearing the shelves"
	sayGatheringCards  = "gathering the cards"
	sayShelvingCards   = "shelving the cards"
	sayGatheringPrints = "gathering every printing"
	sayShelvingPrints  = "shelving the printings"
)

// GatherSayings is every line a gathering can show while it runs.
var GatherSayings = []string{sayClearing, sayGatheringCards, sayShelvingCards,
	sayGatheringPrints, sayShelvingPrints}

// gatherSteps is how many beats the progress bar has, which is however many
// there are to say.
var gatherSteps = len(GatherSayings)

// gatherPartial is what the page renders while the job runs. A `saying`
// rather than a percentage on its own, because five beats over several
// minutes means the bar barely moves and the words are the only thing that
// tells somebody it is alive.
func gatherPartial(saying string) wire.OrderedMap {
	return wire.OrderedMap{{Key: "saying", Value: saying}}
}

// upkeep is `GET /api/admin/upkeep`: what the two perishable things about
// this instance look like right now.
//
// The library half is what the page needs to decide whether to offer the
// button at all: a refresh already running is re-attached to rather than
// duplicated. The arena half is a reading and not a lever -- see this file's
// comment for why there is no button beside it.
func (a *API) upkeep(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	id, running := a.gathering.running()

	var cards, printings int64
	// **A probe, not a visitor**: this route is polled by an open admin tab,
	// and an ordinary lease would hold the pool file open for ten seconds
	// after every poll -- which is exactly the thing that made a refresh
	// unable to get in (`pool.Pool.UseWithoutHolding`). It also answers
	// honestly while a refresh is running: the pool is sealed, so this reads
	// as no counts rather than as stale ones.
	poolThere := a.poolForAProbe(r.Context(), func(c *pool.Conn) error {
		var err error
		if cards, err = pool.Count(r.Context(), c.DB(), "oracle_cards"); err != nil {
			return err
		}
		printings, err = pool.Count(r.Context(), c.DB(), "printings")
		return err
	}) == nil

	// The same answer `GET /api/forge` gives, through the same function: a
	// second spelling of "is there an arena here" is a second thing to keep in
	// step, and this one would drift the first time a way of having Forge was
	// added.
	arenaHere, _ := a.forgeStatus()

	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "library", Value: wire.OrderedMap{
			{Key: "present", Value: poolThere},
			{Key: "cards", Value: cards},
			{Key: "printings", Value: printings},
			{Key: "gathered", Value: nilIfEmpty(a.libraryGathered())},
			{Key: "refreshing", Value: running},
			{Key: "job_id", Value: nilIfEmpty(id)},
		}},
		{Key: "arena", Value: wire.OrderedMap{
			{Key: "here", Value: arenaHere},
			{Key: "playing_with", Value: nilIfEmpty(a.arenaVersion(r.Context()))},
		}},
	})
}

// libraryGathered is the day the pool file was last written, as `2026-08-24`,
// or empty when there is no pool.
//
// The file's own timestamp rather than a stored one: a refresh rewrites it,
// `mtglab data refresh` on the machine rewrites it, and a volume restored
// from a copy carries the copy's -- all three of which are the honest answer
// to "when was this last gathered" and none of which a column we wrote would
// have caught.
func (a *API) libraryGathered() string {
	if a.poolPath == "" {
		return ""
	}
	info, err := os.Stat(a.poolPath)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format("2006-01-02")
}

// arenaVersion is the Forge the last recorded match was played with, or empty
// when no game has been played here.
//
// From the ledger rather than from the worker, because the worker has no
// endpoint that answers the question and the ledger has the answer for the
// right reason: ADR 36 records `forge_version` on every match *because* the
// version is the instrument. What the last game was judged by is a stronger
// fact than what some image claims to hold.
func (a *API) arenaVersion(ctx context.Context) string {
	if a.matchLedgerOf == nil {
		return ""
	}
	recent, err := a.matchLedgerOf.Recent(ctx, 1)
	if err != nil {
		a.log.Warn("the match ledger would not say what the arena plays with", "error", err)
		return ""
	}
	for _, match := range recent {
		if match.ForgeVersion != nil && *match.ForgeVersion != "" {
			return *match.ForgeVersion
		}
	}
	return ""
}

// nilIfEmpty renders an absent string as JSON null rather than as `""`, so a
// client's `?? fallback` reaches its fallback.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
