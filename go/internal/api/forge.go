package api

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// `api/forgeruns.py` and `app.py`'s two Forge routes: Tier 3 matches, shaped
// for the UI (ADR 35).
//
// Tier 3 reaches the app the way every slow thing does — a job — with the
// division `themeruns` argued and this inherits deliberately: **everything
// refusable is refused in the request**, and only the JVM run is queued. A
// missing deck is a 404, a bad games count is a 422, an absent Forge is a 503,
// and a deck Forge cannot fully play is a 422 that names the cards — the
// pre-flight reads a zip on the request thread and was designed to.
//
// Unlike the sim family, planning failures here are *not* deferred into the
// job. That deferral exists over there for compatibility; this surface was new
// when it was written, so it got the honest shape from day one: distinct
// refusals with status codes, and a job that only ever fails for runtime
// reasons.
//
// **The lane is FORGE, and one worker is load-bearing.** The work waits on a
// Forge subprocess, so CPU (it would block Tier 1 for minutes) and NET (two
// JVMs at once race the shared `.dck` directory) are both wrong. Serialising
// the lane also makes the dedupe story simple: a second identical request
// joins the live job via the plan's key, and a *different* match queues
// honestly behind the first.
//
// Nothing is cached. Forge is seeded here the way Tier 1 is (same default,
// same doctrine — an unseeded sample is not reproducible), but a cache needs a
// key that names the engine's behaviour and the distribution can be upgraded
// under us; until someone measures that a repeat ask is common, in-flight
// dedupe is the whole memory.
//
// **This is the last job-submitting family to flip** (Phase 7). Until it did,
// it was the only thing keeping the hybrid poll handler's proxy branch alive;
// see `jobruns.go`, where that branch now has a test that plants a real job on
// the upstream rather than one that passes because nothing answered.

// ForgeKind is what `/api/jobs` calls one of these.
const ForgeKind = "sim.forge"

// ForgeCaveat rides with the numbers rather than near them, because CLAUDE.md
// requires it quoted with them.
const ForgeCaveat = "Forge's AI is best with aggro and midrange, poor with " +
	"control, and bad with most combo — read results per archetype, never as " +
	"a single ranking. Games that hit the clock are reported apart from " +
	"draws: a clock-out is the measurement giving up, not a game outcome."

// ForgeClock is Forge's `-c`: seconds before a game is called a draw. 300
// rather than Forge's 120, because CLAUDE.md says so and a measured Trostani
// game ran 134 seconds — a shorter clock turns real games into fake draws.
const ForgeClock = 300

const (
	// ForgeGamesDefault is what a request that does not say gets.
	ForgeGamesDefault = 10
	// ForgeGamesMax caps the ask. Twenty games of the slowest measured
	// heads-up pairing is ~10 minutes of wall clock; the lane is serial, so
	// the cap is what keeps one enthusiastic request from parking the Forge
	// for an hour.
	ForgeGamesMax = 20
)

// forgeStatus is `forgeruns.status`: is the Forge reachable from this process?
// A fact about the environment.
//
// Two environments, one contract. With the hosted worker configured (ADR 35's
// second half), the answer is yes on configuration alone — no network, no
// machine woken to ask, exactly as `/api/claude` answers on the presence of a
// key rather than by calling Anthropic. Otherwise this probes the two things a
// local run needs — the distribution's jar and a JVM new enough — without
// booting either. `why` is maintainer-facing prose (it names paths and version
// floors); the client renders its own words, which is commandment 10 doing its
// usual work.
func forgeStatus() (bool, *string) {
	if tier3.Configured() {
		return true, nil
	}
	if _, err := tier3.DesktopJar(""); err != nil {
		why := err.Error()
		return false, &why
	}
	if _, err := tier3.JavaBinary(); err != nil {
		why := err.Error()
		return false, &why
	}
	return true, nil
}

// forgeGate is `GET /api/forge` — the gate the Simulator asks before it offers
// real games.
func (a *API) forgeGate(w http.ResponseWriter, r *http.Request) {
	available, why := forgeStatus()
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "available", Value: available},
		{Key: "why", Value: why},
	})
}

// forgeGames is `_games`: clamped to the cap, and never below one.
//
// `int(payload.get("games", GAMES_DEFAULT))` reproduced through Python's own
// `int()` grammar, which is not `strconv.Atoi` — `"1_0"` is ten, a float
// truncates, and a bool is 0 or 1. A value `int()` refuses raises where Python
// raises, and that is a **wart, reproduced rather than tidied**: `plan_forge`
// runs in the request with no handler for a ValueError, so `{"games": "many"}`
// is an uncaught 500 rather than the 422 it should be. Pinned by
// `TestAGamesCountThatIsNotANumberIsTheFiveHundredPythonGives`.
func forgeGames(body map[string]any) (int, error) {
	raw, ok := body["games"]
	if !ok {
		raw = ForgeGamesDefault
	}
	n, err := claude.IntValue(raw)
	if err != nil {
		return 0, err
	}
	// `max(1, min(n, GAMES_MAX))` over an unbounded integer.
	if n.Cmp(big.NewInt(ForgeGamesMax)) > 0 {
		return ForgeGamesMax, nil
	}
	if n.Cmp(big.NewInt(1)) < 0 {
		return 1, nil
	}
	return int(n.Int64()), nil
}

// forgeSeed is `_seed`: the default when absent or empty, otherwise `int(raw)`.
//
// A `*big.Int` because Python's integers are unbounded and the seed is echoed
// into the result, the dedupe key and Forge's own command line as text. An
// int64 would silently answer a different number for a seed past 2**63, and a
// seed is a promise.
func forgeSeed(body map[string]any) (*big.Int, error) {
	raw, ok := body["seed"]
	if !ok || raw == nil {
		return big.NewInt(DefaultSeed), nil
	}
	if s, isString := raw.(string); isString && s == "" {
		return big.NewInt(DefaultSeed), nil
	}
	return claude.IntValue(raw)
}

// forgeRow is one game as the client renders it, whichever moment it arrives
// in.
//
// The same shape serves twice: inside the finished result's `rows`, and on the
// job's `partial` while the match is still playing (the match theater). One
// builder is what makes a streamed row and its final self identical — a
// theater that showed one shape live and another in the tale of the tape would
// be the drift the wire codec exists to prevent, one layer up.
type forgeRow struct {
	Game   int     `json:"game"`
	Winner *string `json:"winner"`
	// Seconds is a `floats.Float` and not a `float64`, which is the one
	// thing about this struct that has to be decided rather than typed:
	// `round(4000/1000, 1)` is `4.0`, Python writes `4.0`, and
	// `encoding/json` writes `4`. Same number to a client, different bytes
	// in DevTools and in anything that ever hashes this payload.
	Seconds  floats.Float `json:"seconds"`
	Turns    *int         `json:"turns"`
	Draw     bool         `json:"draw"`
	TimedOut bool         `json:"timed_out"`
}

func newForgeRow(g tier3.GameResult, slug *string) forgeRow {
	row := forgeRow{Game: g.Index,
		Seconds:  floats.Float(floats.RoundTo(float64(g.Milliseconds)/1000, 1)),
		Turns:    g.Turns,
		Draw:     g.Draw && !g.TimedOut,
		TimedOut: g.TimedOut}
	if !g.TimedOut {
		row.Winner = slug
	}
	return row
}

// forgeSeat is one deck's line in the result.
type forgeSeat struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Wins    int    `json:"wins"`
}

// forgeResult is the payload a match becomes. Medians and tails, never a mean
// alone.
type forgeResult struct {
	Decks          []forgeSeat   `json:"decks"`
	Games          int           `json:"games"`
	Played         int           `json:"played"`
	Draws          int           `json:"draws"`
	TimedOut       int           `json:"timed_out"`
	MedianSeconds  *floats.Float `json:"median_seconds"`
	MaxSeconds     *floats.Float `json:"max_seconds"`
	StartupSeconds floats.Float  `json:"startup_seconds"`
	WallSeconds    floats.Float  `json:"wall_seconds"`
	Clock          int           `json:"clock"`
	Seed           *big.Int      `json:"seed"`
	Rows           []forgeRow    `json:"rows"`
	Caveat         string        `json:"caveat"`
}

// forgePartial is what the job's `partial` carries while the match plays.
type forgePartial struct {
	Rows []forgeRow `json:"rows"`
}

// shapeForge is `_shape`.
//
// `wins` is counted per seat and reported per deck; real draws and clock-outs
// are separate columns because they are separate facts (the parser keeps them
// apart and this must not fold them back).
//
// **A deck played against itself reports the combined total on both lines**,
// and that is Python's, reproduced rather than fixed: `wins` is keyed on the
// slug, so `a_slug == b_slug` collapses two seats into one counter. Unreachable
// only by convention — nothing refuses the request — and pinned by
// `TestADeckPlayedAgainstItselfShowsTheCombinedWins`, because the guard beats
// the fix for a wart nobody has hit.
func shapeForge(decks []*deck.Deck, addresses []string, games int,
	seed *big.Int, run *tier3.SimRun) forgeResult {
	wins := map[string]int{}
	for _, d := range decks {
		wins[d.Slug] = 0
	}
	rows := make([]forgeRow, 0, len(run.Games()))
	for _, game := range run.Games() {
		slug := run.WinnerSlug(game)
		// A clocked-out game counts for nobody even when Forge printed a
		// winner line after the slow-match warning — the parser attaches the
		// pending timeout to whatever result line follows, and a "win" awarded
		// because the other AI ran out of thinking time is the measurement
		// giving up wearing a trophy.
		if slug != "" && !game.TimedOut {
			wins[slug]++
		}
		var winner *string
		if slug != "" {
			s := slug
			winner = &s
		}
		rows = append(rows, newForgeRow(game, winner))
	}

	seconds := make([]float64, 0, len(rows))
	for _, r := range rows {
		seconds = append(seconds, float64(r.Seconds))
	}
	sort.Float64s(seconds)

	out := forgeResult{
		Decks:          make([]forgeSeat, 0, len(decks)),
		Games:          games,
		Played:         len(rows),
		StartupSeconds: floats.Float(floats.RoundTo(run.StartupSeconds(), 1)),
		WallSeconds:    floats.Float(floats.RoundTo(run.WallSeconds, 1)),
		Clock:          ForgeClock,
		Seed:           seed,
		Rows:           rows,
		Caveat:         ForgeCaveat,
	}
	for i, d := range decks {
		out.Decks = append(out.Decks, forgeSeat{Slug: d.Slug, Name: d.Name,
			Address: addresses[i], Wins: wins[d.Slug]})
	}
	for _, r := range rows {
		if r.Draw {
			out.Draws++
		}
		if r.TimedOut {
			out.TimedOut++
		}
	}
	if len(seconds) > 0 {
		median := floats.Float(floats.RoundTo(pyMedian(seconds), 1))
		out.MedianSeconds = &median
		max := floats.Float(seconds[len(seconds)-1])
		out.MaxSeconds = &max
	}
	if out.Rows == nil {
		out.Rows = []forgeRow{}
	}
	return out
}

// pyMedian is `statistics.median` over an already-sorted slice: the middle
// value, or the mean of the two middle values.
//
// The two-term mean is a single correctly-rounded addition, so it is the same
// number under `fsum` and under `+` — the one float sum in this file that
// needs no argument about which interpreter it agrees with.
func pyMedian(sorted []float64) float64 {
	n := len(sorted)
	i := n / 2
	if n%2 == 1 {
		return sorted[i]
	}
	return (sorted[i-1] + sorted[i]) / 2
}

// simForge is `POST /api/sim/forge` — one heads-up Forge match (ADR 35).
// Returns a **job**.
//
// Everything refusable is refused here, not in the job (the `themeruns`
// division): decks that do not resolve are 404, an uninstalled Forge is 503,
// and a deck with cards Forge does not implement is a 422 that names them —
// because a Forge game *plays on* without them and reports a winner, which is
// the one failure this surface exists to never serve.
//
// Heads-up only, and that is ADR 35 rather than a limitation to lift casually:
// measured on this hardware, 40% of four-player games hit the clock, and a
// mode whose results are mostly clock is not honest enough to ship. The CLI
// still plays pods for whoever wants to watch one.
func (a *API) simForge(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return
	}

	type pair struct{ owner, slug string }
	var pairs []pair
	for _, side := range []string{"a", "b"} {
		raw := body[side+"_slug"]
		// `if not slug` — the raw value's truthiness, before `str()`. A `0`
		// or an empty list is falsy and refused here; a non-empty list is
		// truthy and becomes a slug that no deck has, which is a 404.
		if !pyTruthy(raw) {
			wire.Detail(w, http.StatusUnprocessableEntity, side+"_slug is required")
			return
		}
		owner := str(body, side+"_owner")
		if owner == "" {
			owner = lib.MyOwner()
		}
		pairs = append(pairs, pair{owner: owner, slug: str(body, side+"_slug")})
	}

	// The gate before the decks, exactly as Python orders it: an instance
	// with no Forge answers 503 without ever asking the library who these
	// people are.
	if available, why := forgeStatus(); !available {
		detail := ""
		if why != nil {
			detail = *why
		}
		wire.Detail(w, http.StatusServiceUnavailable, detail)
		return
	}

	decks := make([]*deck.Deck, 0, len(pairs))
	addresses := make([]string, 0, len(pairs))
	// For the match ledger: the same ownership key the activity log uses (an
	// owner id, NULL for the file tier — never the URL's owner segment, which
	// is not stable across configurations).
	ownerIDs := make([]*int64, 0, len(pairs))
	for _, p := range pairs {
		src, err := lib.SourceFor(r.Context(), p.owner)
		if a.refuse(w, "source", err) {
			return
		}
		d, err := src.Get(r.Context(), p.slug)
		if a.refuse(w, "forge", err) {
			return
		}
		decks = append(decks, d)
		addresses = append(addresses, p.owner+"/"+p.slug)
		ownerIDs = append(ownerIDs, src.OwnerID())
	}

	// The pre-flight runs where the card scripts live: against the local zip,
	// or on the worker machine (which this wakes — the one request-thread cost
	// the hosted shape adds, bounded by the worker's boot budget so a machine
	// that will not come up is a 503 rather than a hang).
	hosted := tier3.Configured()
	if err := a.preflight(r.Context(), hosted, decks); err != nil {
		switch {
		case errors.Is(err, tier3.ErrCoverageFailed):
			wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, tier3.ErrForgeNotInstalled):
			wire.Detail(w, http.StatusServiceUnavailable, err.Error())
		default:
			a.log.Error("the Forge pre-flight failed", "error", err)
			wire.Detail(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	plan, err := a.planForge(decks, addresses, ownerIDs, body, hosted)
	if err != nil {
		// `int()` refused the games count or the seed. **Python's 500**, and
		// reproduced exactly: `plan_forge` runs in the request with nothing
		// catching a ValueError there, so Starlette answers its plain-text
		// three words -- not a JSON detail, which is what this wrote until
		// Phase 8's wheel port measured the real bytes. See [forgeGames].
		uncaught500(w, a.log, "forge", err)
		return
	}
	a.submit(w, r, plan)
}

// preflight is coverage, computed on whichever machine holds the card scripts.
func (a *API) preflight(ctx context.Context, hosted bool, decks []*deck.Deck) error {
	if hosted {
		_, err := a.forgeWorker().CheckCoverage(ctx, decks)
		return err
	}
	_, err := tier3.CheckCoverage(decks, "")
	return err
}

// forgeWorker is the hosted client, a method so a test can give the API one
// pointed at a stub shim.
func (a *API) forgeWorker() *tier3.Worker {
	if a.forgeClient != nil {
		return a.forgeClient
	}
	return &tier3.Worker{}
}

// planForge is `forgeruns.plan_forge`: one heads-up match, planned. Refusals
// happened at the route already.
//
// `decks` arrive resolved because resolving them is the route's job — it holds
// the library and the 404-versus-422 vocabulary. What this decides is the
// work: coverage has passed, so the closure is exactly one match. `addresses`
// are the `owner/slug` pairs the client asked with, echoed back so the result
// can say whose decks played without the job inventing a second naming scheme.
func (a *API) planForge(decks []*deck.Deck, addresses []string,
	ownerIDs []*int64, body map[string]any, hosted bool) (jobs.Plan, error) {
	games, err := forgeGames(body)
	if err != nil {
		return jobs.Plan{}, err
	}
	seed, err := forgeSeed(body)
	if err != nil {
		return jobs.Plan{}, err
	}

	slugs := make([]string, 0, len(decks))
	for _, d := range decks {
		slugs = append(slugs, d.Slug)
	}
	plural := "s"
	if games == 1 {
		plural = ""
	}
	label := fmt.Sprintf("Forge: %s, %d game%s", strings.Join(slugs, " vs "), games, plural)
	key := "forge|" + strings.Join(addresses, "|") + fmt.Sprintf("|%d|%s", games, seed)

	worker := a.forgeWorker()
	recorder := a.matchLedger()
	return jobs.Plan{
		Kind: ForgeKind, Label: label, Lane: jobs.FORGE, Key: key,
		Run: func(rep jobs.Progress) (any, error) {
			// **Its own context.** The request is over by the time this runs,
			// and `r.Context()` is cancelled by net/http the moment the
			// handler returns — a job that took one would be a match killed
			// mid-JVM. The recorded lesson, and the one only a real server
			// test can see.
			ctx := context.Background()
			rep.Report(0, games)

			// Forge's output is streamed, so the job ticks once per finished
			// game — and each tick carries the game it just watched end,
			// shaped by the same builder the final tally uses and exposed on
			// the job's `partial` for the client to seat live. Clamped because
			// a tick is a progress report, not a result. Seat order is the
			// deck order (both run paths promise it), which is what lets a
			// slug be named before the run exists. A pre-theater shim streams
			// counts without rows; the bar still moves and `partial` simply
			// stays sparse.
			seats := map[int]string{}
			for i, d := range decks {
				seats[i+1] = d.Slug
			}
			var rowsSoFar []forgeRow
			tick := func(finished int, game *tier3.GameResult) {
				if game != nil {
					var slug *string
					if game.WinnerSeat != nil {
						if s, ok := seats[*game.WinnerSeat]; ok {
							slug = &s
						}
					}
					rowsSoFar = append(rowsSoFar, newForgeRow(*game, slug))
				}
				rep.ReportPartial(min(finished, games), games,
					forgePartial{Rows: append([]forgeRow{}, rowsSoFar...)})
			}

			// Same match, two places it can run (ADR 35): the worker when the
			// environment names one, a local subprocess otherwise. The worker
			// hands back a run rebuilt from the wire and relays the same
			// per-game ticks, so the shaping and the bar cannot tell the
			// difference — that is the wire's whole promise.
			var run *tier3.SimRun
			var runErr error
			if hosted {
				run, runErr = worker.RunMatch(ctx, decks, games, ForgeClock, seed, tick)
			} else {
				run, runErr = tier3.RunGames(decks, tier3.RunOptions{
					Games: games, Clock: ForgeClock, Seed: seed,
					OnGame: func(finished int, game tier3.GameResult) {
						tick(finished, &game)
					},
				})
			}
			if runErr != nil {
				return nil, runErr
			}
			rep.Report(games, games)

			// The match ledger (ADR 36). After the run and before the shaping,
			// because the shape is for this response and the ledger is for
			// every question after it — and Record never fails, so a ledger
			// problem cannot cost anybody a match they just watched finish.
			recorder.Record(ctx, ledger.Match{Run: run, Decks: decks,
				Seed: seed, Clock: ForgeClock, GamesRequested: games,
				Hosted: hosted, OwnerIDs: ownerIDs})
			return shapeForge(decks, addresses, games, seed, run), nil
		},
	}, nil
}

// matchLedger is the recorder, or a nil one — which records nothing and warns
// about nothing, the no-`app.db` case rather than an error.
func (a *API) matchLedger() *ledger.Recorder { return a.matchLedgerOf }
