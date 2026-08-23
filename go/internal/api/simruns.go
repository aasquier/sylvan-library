package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// `api/simruns.py`: simulation jobs, shaped for the UI.
//
// These reuse the exact compilation path the CLI uses, so a number shown in
// the app is the same number `mtglab sim mana` prints. Every result carries
// its own caveat, because Tier 1 answers mana questions only and a chart with
// no caption invites over-reading.
//
// Three things about the cache show up here, all of them ADR 18's.
//
// *Every run is seeded.* An absent seed resolves to [DefaultSeed] and the seed
// is reported in the result, so which sample you are looking at is a fact on
// the page rather than an assumption.
//
// *Planning happens in the request, running happens in the job.* The key is a
// hash of the compiled deck, so knowing whether this is a hit means compiling
// first -- a parse and one indexed pool query, milliseconds against the
// seconds it saves.
//
// *A planning failure is not a new failure mode.* Compiling in the request
// means a missing deck or an absent pool fails earlier than it used to, and it
// must not fail *differently*: the error is carried into the job and returned
// from the worker, so the caller still gets a 200 and then a job in state
// `error`, which is the shape the UI already knows.

// Tier1Caveat and LandSweepCaveat are `simruns.TIER1_CAVEAT` and
// `LAND_SWEEP_CAVEAT`, rendered into every result.
const (
	Tier1Caveat = "Tier 1 shuffles, draws and pays costs. It does not model " +
		"opponents, interaction, tutors, cost reduction, or card text beyond " +
		"mana production."

	LandSweepCaveat = "Read 'spells through T8', not commander speed: " +
		"commander speed rises monotonically with land count, so optimising " +
		"it alone always recommends more lands. Resizing cycles the existing " +
		"land pool, preserving the colour mix but not specific utility lands."
)

// DefaultSeed is the seed a run gets when the caller does not choose one.
// Seven, because that is what the land sweep has always defaulted to and two
// different "unspecified" seeds in one module would be a trap.
const DefaultSeed = 7

// movesTheNumbers is `simruns.MOVES_THE_NUMBERS`: gate failures that change
// what a simulation computes, as opposed to failures that are real but do not
// touch these numbers. A missing rationale blocks a curated deck and has no
// effect whatever on mana; a banned card is sitting in the 99 being shuffled.
// The client says something different about each, so the split is decided here
// rather than guessed there.
var movesTheNumbers = map[string]bool{
	"deck-size":           true,
	"banned":              true,
	"color-identity":      true,
	"unknown-card":        true,
	"no-commander":        true,
	"not-a-commander":     true,
	"too-many-commanders": true,
}

// checkError is one gate failure, capped into `deck_check.errors`.
type checkError struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Card    *string `json:"card"`
}

// deckCheck is `simruns._check_payload`: the gate's answer, shaped for a
// client that must not re-derive it. Field order is Python's dict order.
type deckCheck struct {
	OK           bool         `json:"ok"`
	ErrorCount   int          `json:"error_count"`
	WarningCount int          `json:"warning_count"`
	Errors       []checkError `json:"errors"`
	// AffectsNumbers is the difference between "this deck is illegal, and the
	// figures below describe it as written" and "this deck is illegal in a way
	// that makes the figures below describe a different deck".
	AffectsNumbers      bool     `json:"affects_numbers"`
	Unresolved          []string `json:"unresolved"`
	UnresolvedCount     int      `json:"unresolved_count"`
	CommanderUnresolved bool     `json:"commander_unresolved"`
	DeclaredSize        int      `json:"declared_size"`
	SimulatedSize       int      `json:"simulated_size"`
}

// compiled is what `_compile_checked` hands back: the deck, the compiled
// cards, and the gate's verdict over the same deck and the same pool.
type compiled struct {
	deck   *deck.Deck
	report *compile.Report
	check  deckCheck
}

// compileChecked is `simruns._compile_checked`.
//
// Both halves happen here because they need the same open connection, and
// because **every number this module reports has to be able to say whether the
// deck it describes is legal.** Refusing to simulate an invalid deck is the
// wrong call: decks in this library deliberately fail the gate on a banned
// card, a deck mid-import fails it by construction, and the simulator is the
// tool somebody reaches for to *fix* a deck. Refusing takes the diagnosis away
// at exactly the moment it is wanted, which is commandment 2 with the sign
// flipped. So nothing is blocked except the one state with no answer in it --
// a deck that compiles to no cards, which `compile.Deck` refuses.
func (a *API) compileChecked(ctx context.Context, src library.Source, slug string) (*compiled, error) {
	d, err := src.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	var cards map[string]*pool.CardRecord
	err = a.withPool(ctx, func(c *pool.Conn) error {
		if c == nil {
			return nil
		}
		found, err := deckread.PoolFor(ctx, c, d)
		if err != nil {
			return err
		}
		cards = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		// `service._connect()` answering None, or a pool that has never heard
		// of any of these names. Python raises a bare RuntimeError with this
		// text and the route turns it into a job error.
		return nil, fmt.Errorf("simulation needs the card pool -- run `mtglab data refresh`")
	}
	report, err := compile.Compile(d, cards)
	if err != nil {
		return nil, err
	}
	verdict := gate.Validate(d, cards, gate.DefaultSize)
	return &compiled{deck: d, report: report, check: checkPayload(report, verdict)}, nil
}

func checkPayload(report *compile.Report, verdict *gate.Report) deckCheck {
	errs := verdict.Errors()
	material := false
	for _, item := range errs {
		if movesTheNumbers[item.Code] {
			material = true
			break
		}
	}
	// Capped: a deck failing on ninety-nine rationales would otherwise send
	// ninety-nine strings to render a sentence that says "99".
	shown := make([]checkError, 0, 6)
	for _, item := range errs {
		if len(shown) == 6 {
			break
		}
		shown = append(shown, checkError{Code: item.Code, Message: item.Message, Card: item.Card})
	}
	unresolved := report.Unresolved
	if len(unresolved) > 6 {
		unresolved = unresolved[:6]
	}
	if unresolved == nil {
		unresolved = []string{}
	}
	return deckCheck{
		OK:                  verdict.OK(),
		ErrorCount:          len(errs),
		WarningCount:        len(verdict.Warnings()),
		Errors:              shown,
		AffectsNumbers:      material || !report.Complete(),
		Unresolved:          append([]string{}, unresolved...),
		UnresolvedCount:     len(report.Unresolved),
		CommanderUnresolved: report.CommanderUnresolved,
		DeclaredSize:        report.DeclaredSize,
		SimulatedSize:       report.SimulatedSize(),
	}
}

// ---------------------------------------------------------------- parameters

func keepRuleFrom(body map[string]any) tier1.KeepRule {
	k := tier1.DefaultKeepRule()
	k.MinLands = simInt(body, "min_lands", 2)
	k.MaxLands = simInt(body, "max_lands", 5)
	k.MinManaPieces = simInt(body, "min_pieces", 3)
	return k
}

// seedFrom is `simruns._seed`: always a real number. Clamped to nothing and
// validated barely -- any integer is a valid seed.
func seedFrom(body map[string]any) int {
	raw, ok := body["seed"]
	if !ok || raw == nil || raw == "" {
		return DefaultSeed
	}
	return simInt(body, "seed", DefaultSeed)
}

type manaParams struct {
	games, turns int
	keep         tier1.KeepRule
	seed         int
}

// manaParamsFrom clamps *before* the key is computed, and that ordering
// matters: `games=10**9` and `games=200000` run the identical simulation, and
// keying on the raw request would store that result twice under two names.
func manaParamsFrom(body map[string]any) manaParams {
	games := clampInt(simInt(body, "games", 20000), 100, 200000)
	turns := clampInt(simInt(body, "turns", 12), 4, 20)
	return manaParams{games: games, turns: turns, keep: keepRuleFrom(body), seed: seedFrom(body)}
}

// simInt is Python's `int(payload.get(key, fallback))` over a decoded JSON
// value.
//
// Every coercion Python makes is made here: a JSON number truncates toward
// zero (`int(4.9)` is 4), a numeric string parses, and `True` is 1. An absent
// key takes the fallback.
//
// **An unparseable value takes the fallback too, and that is a deliberate,
// named divergence rather than an oversight.** Python raises `ValueError` out
// of `_mana_params`, which sits *before* `plan_mana`'s try, so it escapes the
// route as an unhandled exception and FastAPI answers 500. Reproducing a 500
// for `{"games": "abc"}` would mean building a path whose only job is to crash
// in the same place, for input no client sends and no golden pins. The clamp
// that follows every call makes the fallback safe in the only way that
// matters -- the run is a legal run -- and this note is here so the choice is
// found rather than discovered.
func simInt(body map[string]any, key string, fallback int) int {
	raw, given := body[key]
	if !given || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return fallback
		}
		return int(f)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			// `int("4.0")` is a ValueError in Python, so this is the fallback
			// branch too -- but a float that arrived as a string is worth
			// truncating rather than discarding, and the clamp bounds it.
			return int(f)
		}
		return fallback
	case bool:
		if v {
			return 1
		}
		return 0
	default:
		return fallback
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---------------------------------------------------------------------- mana

type byTurnRow struct {
	Turn          int     `json:"turn"`
	Lands         float64 `json:"lands"`
	Mana          float64 `json:"mana"`
	Unused        float64 `json:"unused"`
	Spells        float64 `json:"spells"`
	CommanderDown float64 `json:"commander_down"`
	MissedDrop    float64 `json:"missed_drop"`
}

type cardTimingRow struct {
	Name       string   `json:"name"`
	MV         int      `json:"mv"`
	CastRate   float64  `json:"cast_rate"`
	MedianTurn *float64 `json:"median_turn"`
	ByT8       float64  `json:"by_t8"`
}

// manaResult is `_mana_result`, and its field order is Python's dict order --
// a ported payload is a struct precisely so `encoding/json` does not
// alphabetise it.
//
// `Cached` and `ComputedAt` sit at the end because `_stamp` appends them to an
// already-built dict, and `DeckCheck` after those because it is assigned last.
type manaResult struct {
	Slug                string          `json:"slug"`
	DeckName            string          `json:"deck_name"`
	Games               int             `json:"games"`
	Turns               int             `json:"turns"`
	Seed                int             `json:"seed"`
	MulliganRate        float64         `json:"mulligan_rate"`
	AvgMulligans        float64         `json:"avg_mulligans"`
	MedianCommanderTurn *tier1.Number   `json:"median_commander_turn"`
	NeverCastCommander  float64         `json:"never_cast_commander"`
	ColorScrewRate      float64         `json:"color_screw_rate"`
	ByTurn              []byTurnRow     `json:"by_turn"`
	MedianFirstSpell    *float64        `json:"median_first_spell_turn"`
	StalledTurns        float64         `json:"stalled_turns"`
	CardTimings         []cardTimingRow `json:"card_timings"`
	Caveat              string          `json:"caveat"`

	Cached     bool       `json:"cached"`
	ComputedAt *string    `json:"computed_at"`
	DeckCheck  *deckCheck `json:"deck_check,omitempty"`
}

func manaResultFrom(slug string, d *deck.Deck, summary tier1.SimSummary, p manaParams) manaResult {
	byTurn := make([]byTurnRow, 0, p.turns)
	for t := 0; t < p.turns; t++ {
		byTurn = append(byTurn, byTurnRow{
			Turn:          t + 1,
			Lands:         pyfloat.RoundTo(summary.AvgLandsByTurn[t], 2),
			Mana:          pyfloat.RoundTo(summary.AvgManaByTurn[t], 2),
			Unused:        pyfloat.RoundTo(summary.AvgUnusedByTurn[t], 2),
			Spells:        pyfloat.RoundTo(summary.AvgSpellsByTurn[t], 2),
			CommanderDown: summary.CommanderByTurn[t+1],
			// P(no land to play this turn) -- the drop that could not be made,
			// never the one held back.
			MissedDrop: pyfloat.RoundTo(summary.MissedDropByTurn[t], 4),
		})
	}
	timings := make([]cardTimingRow, 0, len(summary.CardTimings))
	for _, ct := range summary.CardTimings {
		timings = append(timings, cardTimingRow{
			Name: ct.Name, MV: ct.MV,
			CastRate:   pyfloat.RoundTo(ct.CastRate, 4),
			MedianTurn: ct.MedianTurn,
			ByT8:       pyfloat.RoundTo(ct.ByT8, 4),
		})
	}
	return manaResult{
		Slug: slug, DeckName: d.Name,
		Games: p.games, Turns: p.turns, Seed: p.seed,
		MulliganRate:        summary.MulliganRate,
		AvgMulligans:        summary.AvgMulligans,
		MedianCommanderTurn: summary.MedianCommanderTurn,
		NeverCastCommander:  summary.NeverCastCommander,
		ColorScrewRate:      summary.ColorScrewRate,
		ByTurn:              byTurn,
		MedianFirstSpell:    summary.MedianFirstSpellTurn,
		StalledTurns:        pyfloat.RoundTo(summary.AvgStalledTurns, 2),
		CardTimings:         timings,
		Caveat:              Tier1Caveat,
	}
}

func (a *API) planMana(ctx context.Context, src library.Source, slug string, body map[string]any) jobs.Plan {
	p := manaParamsFrom(body)
	label := fmt.Sprintf("%s: mana, %s games", slug, pyComma(p.games))

	c, err := a.compileChecked(ctx, src, slug)
	if err != nil {
		// Planning is an optimisation, and a deck that cannot compile has to
		// fail the way it failed before this existed: inside the job.
		return jobs.Plan{Kind: "sim.mana", Label: label, Run: deferredFailure(err)}
	}
	check := c.check

	key := cache.Key("sim.mana", cache.Input{
		Library: c.report.Library, Commander: c.report.Commander,
		Games: p.games, Turns: p.turns, KeepRule: p.keep, Seed: p.seed,
	})
	if hit := a.simCache.Get(ctx, key); hit != nil {
		var stored manaResult
		if err := json.Unmarshal(hit.Result, &stored); err == nil {
			// The verdict is attached *after* the cache, deliberately: the
			// cached numbers are keyed on the compiled deck, but whether that
			// deck passes the gate can change without the compiled cards
			// moving -- a rationale written, a `stage` promoted. A stale
			// verdict on fresh-looking numbers is exactly the failure this
			// field exists to prevent.
			stamp := hit.CreatedAt
			stored.Cached, stored.ComputedAt, stored.DeckCheck = true, &stamp, &check
			return jobs.Plan{Kind: "sim.mana", Label: label, Result: stored}
		}
		a.log.Warn("a cached mana result did not decode; recomputing", "slug", slug)
	}

	report, d := c.report, c.deck
	return jobs.Plan{Kind: "sim.mana", Label: label, Run: func(rep jobs.Progress) (any, error) {
		summary := tier1.Run(report.Library, report.Commander, tier1.Options{
			Games: p.games, Turns: p.turns, KeepRule: &p.keep,
			Seed: int64Ptr(p.seed), Progress: progressOf(rep),
		})
		result := manaResultFrom(slug, d, summary, p)
		// The job's own context, NOT the request's. net/http cancels the
		// request's context the moment the handler returns, and this runs
		// after that -- so a write through `ctx` failed with `context
		// canceled`, warned, and stored nothing. **From v183 to 2026-08-23
		// the Go sim cache never stored a row**: every run paid full price
		// and every second ask recomputed. Found by the dossier lane, which
		// had to decide what context a Claude job runs under and asked the
		// same question of its siblings; proved by a test that drives this
		// route through a real server rather than a recorder, whose context
		// is never cancelled and so had never seen it.
		a.simCache.Put(context.Background(), key, "sim.mana", result)
		result.Cached, result.ComputedAt, result.DeckCheck = false, nil, &check
		return result, nil
	}}
}

// --------------------------------------------------------------------- lands

type landParams struct {
	counts       []int
	games, turns int
	keep         tier1.KeepRule
	seed         int
}

func landParamsFrom(body map[string]any) landParams {
	low := simInt(body, "low", 30)
	if low < 20 {
		low = 20
	}
	high := simInt(body, "high", 40)
	if high > 60 {
		high = 60
	}
	if high < low {
		low, high = high, low
	}
	counts := make([]int, 0, high-low+1)
	for n := low; n <= high; n++ {
		counts = append(counts, n)
	}
	return landParams{
		counts: counts,
		games:  clampInt(simInt(body, "games", 5000), 100, 100000),
		turns:  clampInt(simInt(body, "turns", 10), 8, 20),
		keep:   keepRuleFrom(body),
		seed:   seedFrom(body),
	}
}

// resize is `simruns._resize`: the deck at `count` lands, by cycling the
// existing pool. That preserves the colour mix -- which is what the sweep is
// measuring -- and holds the deck at 99 so mulligan rates stay comparable
// across counts. A pure function of the compiled library and the count, which
// is what makes a per-count cache key sound.
func resize(library []*sim.Card, count int) ([]*sim.Card, error) {
	var lands, spells []*sim.Card
	for _, c := range library {
		if c.IsLand {
			lands = append(lands, c)
		} else {
			spells = append(spells, c)
		}
	}
	if len(lands) == 0 {
		return nil, fmt.Errorf("deck has no lands to sweep")
	}
	out := make([]*sim.Card, 0, count+len(spells))
	for i := 0; i < count; i++ {
		out = append(out, lands[i%len(lands)])
	}
	keep := 99 - count
	if keep < 0 {
		keep = 0
	}
	if keep > len(spells) {
		keep = len(spells)
	}
	return append(out, spells[:keep]...), nil
}

type landRow struct {
	Lands           int     `json:"lands"`
	CommanderByT5   float64 `json:"commander_by_t5"`
	SpellsThroughT8 float64 `json:"spells_through_t8"`
	WastedThroughT8 float64 `json:"wasted_through_t8"`
	MulliganRate    float64 `json:"mulligan_rate"`
}

type landSummary struct {
	Slug             string    `json:"slug"`
	DeckName         string    `json:"deck_name"`
	Games            int       `json:"games"`
	Seed             int       `json:"seed"`
	Rows             []landRow `json:"rows"`
	DeploymentSpread float64   `json:"deployment_spread"`
	ArgmaxLands      int       `json:"argmax_lands"`
	Flat             bool      `json:"flat"`
	Caveat           string    `json:"caveat"`

	Cached     bool       `json:"cached"`
	ComputedAt *string    `json:"computed_at"`
	DeckCheck  *deckCheck `json:"deck_check,omitempty"`
}

func landRowFrom(count int, summary tier1.SimSummary) landRow {
	return landRow{
		Lands:           count,
		CommanderByT5:   summary.CommanderByTurn[5],
		SpellsThroughT8: pyfloat.RoundTo(summary.SpellsThrough(8), 2),
		WastedThroughT8: pyfloat.RoundTo(summary.WastedThrough(8), 2),
		MulliganRate:    summary.MulliganRate,
	}
}

// landSummaryFrom reports the spread so a flat curve is visible as flat rather
// than being read as a peak. `max` keeps the FIRST maximum, as Python's does,
// so a tie names the lower land count.
func landSummaryFrom(slug string, d *deck.Deck, rows []landRow, games, seed int) landSummary {
	lo, hi := rows[0].SpellsThroughT8, rows[0].SpellsThroughT8
	best := rows[0]
	for _, r := range rows[1:] {
		if r.SpellsThroughT8 < lo {
			lo = r.SpellsThroughT8
		}
		if r.SpellsThroughT8 > hi {
			hi = r.SpellsThroughT8
		}
		if r.SpellsThroughT8 > best.SpellsThroughT8 {
			best = r
		}
	}
	spread := hi - lo
	return landSummary{
		Slug: slug, DeckName: d.Name, Games: games, Seed: seed, Rows: rows,
		DeploymentSpread: pyfloat.RoundTo(spread, 2),
		ArgmaxLands:      best.Lands,
		Flat:             spread < 0.25,
		Caveat:           LandSweepCaveat,
	}
}

func landKey(cards []*sim.Card, commander *sim.Card, p landParams) string {
	return cache.Key("sim.lands.count", cache.Input{
		Library: cards, Commander: commander,
		Games: p.games, Turns: p.turns, KeepRule: p.keep, Seed: p.seed,
	})
}

func (a *API) planLands(ctx context.Context, src library.Source, slug string, body map[string]any) jobs.Plan {
	p := landParamsFrom(body)
	label := fmt.Sprintf("%s: land sweep %d-%d", slug, p.counts[0], p.counts[len(p.counts)-1])

	c, err := a.compileChecked(ctx, src, slug)
	if err != nil {
		return jobs.Plan{Kind: "sim.lands", Label: label, Run: deferredFailure(err)}
	}
	resized := make(map[int][]*sim.Card, len(p.counts))
	for _, count := range p.counts {
		cards, err := resize(c.report.Library, count)
		if err != nil {
			// `resize` refuses a deck with no lands to sweep, and that has to
			// reach the caller the same way a missing deck does.
			return jobs.Plan{Kind: "sim.lands", Label: label, Run: deferredFailure(err)}
		}
		resized[count] = cards
	}
	check := c.check
	commander, d := c.report.Commander, c.deck

	keys := make(map[int]string, len(p.counts))
	for count, cards := range resized {
		keys[count] = landKey(cards, commander, p)
	}
	hits := make(map[int]*cache.Hit, len(p.counts))
	all := true
	for _, count := range p.counts {
		hits[count] = a.simCache.Get(ctx, keys[count])
		if hits[count] == nil {
			all = false
		}
	}
	if all {
		// Ordered by `counts` rather than by whatever the lookups returned:
		// the rows are a curve, and a sweep whose rows are out of order is a
		// chart that lies.
		rows := make([]landRow, 0, len(p.counts))
		oldest := ""
		decoded := true
		for _, count := range p.counts {
			var row landRow
			if err := json.Unmarshal(hits[count].Result, &row); err != nil {
				decoded = false
				break
			}
			rows = append(rows, row)
			if oldest == "" || hits[count].CreatedAt < oldest {
				oldest = hits[count].CreatedAt
			}
		}
		if decoded {
			out := landSummaryFrom(slug, d, rows, p.games, p.seed)
			out.Cached, out.ComputedAt, out.DeckCheck = true, &oldest, &check
			return jobs.Plan{Kind: "sim.lands", Label: label, Result: out}
		}
		a.log.Warn("a cached land row did not decode; recomputing", "slug", slug)
	}

	return jobs.Plan{Kind: "sim.lands", Label: label, Run: func(rep jobs.Progress) (any, error) {
		// The job's own context -- see planMana. The sweep both reads and
		// writes the cache, and under the request's cancelled context it did
		// neither: every count was recomputed and none was kept.
		out := a.sweep(context.Background(), slug, d, commander, resized, p, rep)
		out.DeckCheck = &check
		return out, nil
	}}
}

// sweep runs the counts that are not cached and reuses the ones that are.
//
// The cache is consulted here as well as in `planLands`, which costs one extra
// SELECT per count. That is not redundancy: the plan runs in the request and
// this runs in the worker, minutes apart on a queued job, and in between
// another request can have filled in the counts this one was going to
// simulate. Re-reading means the work is skipped rather than repeated.
func (a *API) sweep(ctx context.Context, slug string, d *deck.Deck, commander *sim.Card,
	resized map[int][]*sim.Card, p landParams, rep jobs.Progress) landSummary {

	keys := make(map[int]string, len(resized))
	for count, cards := range resized {
		keys[count] = landKey(cards, commander, p)
	}
	hits := make(map[int]*cache.Hit, len(resized))
	missing := 0
	for _, count := range p.counts {
		hits[count] = a.simCache.Get(ctx, keys[count])
		if hits[count] == nil {
			missing++
		}
	}
	// Progress counts only the counts actually being simulated, so a sweep
	// that is nine-elevenths cached shows a bar finishing in the time the
	// remaining two take rather than sitting at 82% from the first tick.
	steps := missing
	if steps == 0 {
		steps = 1
	}
	total := steps * p.games

	rows := make([]landRow, 0, len(p.counts))
	simulated := 0
	for _, count := range p.counts {
		if hit := hits[count]; hit != nil {
			var row landRow
			if err := json.Unmarshal(hit.Result, &row); err == nil {
				rows = append(rows, row)
				continue
			}
		}
		base := simulated * p.games
		keep := p.keep
		summary := tier1.Run(resized[count], commander, tier1.Options{
			Games: p.games, Turns: p.turns, KeepRule: &keep, Seed: int64Ptr(p.seed),
			Progress: func(done, _ int) { rep.Report(base+done, total) },
		})
		row := landRowFrom(count, summary)
		a.simCache.Put(ctx, keys[count], "sim.lands.count", row)
		rows = append(rows, row)
		simulated++
	}
	rep.Report(total, total)

	out := landSummaryFrom(slug, d, rows, p.games, p.seed)
	out.Cached, out.ComputedAt = false, nil
	return out
}

// ------------------------------------------------------------------- helpers

// deferredFailure re-raises a planning failure inside the job it would have
// been.
//
// Compilation moved into the request when the cache arrived, so a deck that is
// missing, has no card pool or has no lands now fails *earlier* than it used
// to. It must not fail differently: before the cache, that error reached the
// caller as a job in state `error`, and the UI knows how to show one. So the
// error is caught, carried, and returned from the worker -- same message, same
// 200-then-error shape.
//
// **A missing deck carries the BARE SLUG**, which is the one place the message
// and the route's `detail` part company. `decks.DeckNotFound(slug)` has no
// `__str__` of its own, so `str(exc)` is its single argument -- the slug --
// while the 404's sentence is built by the route's exception handler. Go's
// `library.ErrNotFound` renders the sentence, which is right for the handler
// and wrong here: a job's `error` becomes a JS `Error` in `lib/api.ts` and the
// screen shows it, so the door said "no deck '['x']'" where Python said
// "['x']".
//
// The same shape as `converse` handing the model `no deck 'x'` where Python
// hands it the slug -- fixed there in Phase 6, found here on 2026-08-23 by
// diffing the pair, live since Phase 5. It is unreachable from the app's own
// client (nothing offers a deck that is not on the shelf), which is exactly
// why nothing had noticed.
func deferredFailure(err error) jobs.Runner {
	var missing library.ErrNotFound
	if errors.As(err, &missing) {
		err = errors.New(missing.Slug)
	}
	return func(jobs.Progress) (any, error) { return nil, err }
}

func progressOf(rep jobs.Progress) func(int, int) {
	return func(done, total int) { rep.Report(done, total) }
}

func int64Ptr(v int) *int64 { n := int64(v); return &n }

// pyComma is Python's `f"{n:,}"`.
func pyComma(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return "-" + pyComma(-n)
	}
	out := ""
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out += ","
		}
		out += string(ch)
	}
	return out
}

// ---------------------------------------------------------------- the routes

// simBody reads the body and resolves the deck source, which the four sim
// routes share.
//
// **`str()` on the slug, for the reason `owner` has always had one.** `payload`
// is a bare JSON object, so `{"slug": ["a"]}` walked a list past a truthiness
// check into the deck source, where SQLite refused to bind it and the route
// answered 500. A slug that is not a slug is the 404 any other unknown slug
// gets.
func (a *API) simBody(w http.ResponseWriter, r *http.Request) (map[string]any, library.Source, string, bool) {
	body, ok := readBody(w, r)
	if !ok {
		return nil, nil, "", false
	}
	slug := str(body, "slug")
	if slug == "" {
		wire.Detail(w, http.StatusUnprocessableEntity, "slug is required")
		return nil, nil, "", false
	}
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return nil, nil, "", false
	}
	owner := str(body, "owner")
	if owner == "" {
		owner = lib.MyOwner()
	}
	src, err := lib.SourceFor(r.Context(), owner)
	if a.refuse(w, "source", err) {
		return nil, nil, "", false
	}
	return body, src, slug, true
}

func (a *API) simMana(w http.ResponseWriter, r *http.Request) {
	body, src, slug, ok := a.simBody(w, r)
	if !ok {
		return
	}
	a.submit(w, r, a.planMana(r.Context(), src, slug, body))
}

func (a *API) simLands(w http.ResponseWriter, r *http.Request) {
	body, src, slug, ok := a.simBody(w, r)
	if !ok {
		return
	}
	a.submit(w, r, a.planLands(r.Context(), src, slug, body))
}

// submit is `app._job_for`: a plan becomes a job, and the job's wire shape is
// the response.
func (a *API) submit(w http.ResponseWriter, r *http.Request, plan jobs.Plan) {
	if a.jobs == nil {
		wire.Detail(w, http.StatusServiceUnavailable, "no job registry")
		return
	}
	job, err := a.jobs.FromPlan(plan, auth.ScopeFrom(r.Context()).UserID)
	if err != nil {
		wire.Detail(w, http.StatusInternalServerError, err.Error())
		return
	}
	wire.JSON(w, http.StatusOK, job.Payload())
}
