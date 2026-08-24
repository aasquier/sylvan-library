package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/curve"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
	"github.com/aasquier/sylvan-library/go/internal/sim/mulligan"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The closed form and the policy search.
//
// Two surfaces of very different weight, shaped differently on purpose --
// this is the one place where the sibling-duration rule came
// out **different for two routes in one file**.
//
// *The shelf is a plain route.* The closed form is arithmetic over an
// already-compiled deck, measured at 0.03-0.04s on every deck in the library.
// A job would add a submit, a poll and a registry row to a call that finishes
// before the response is serialised.
//
// *The policy search is a job.* Thirty-three seeded Tier 1 runs, about fifty
// seconds at the default sample, squarely in the territory `simruns` exists
// for. CPU lane, not NET: it is the same compute-bound work the Tier 1 runs
// it is made of are, and putting it in NET would starve the socket-bound work
// that pool protects.

const (
	ShelfCaveat = "The closed form asks whether the mana would be there, " +
		"assuming the card is in your hand. It does not ask whether you drew " +
		"it, and it cannot see ramp. Read it beside the simulation, not " +
		"instead of it."

	CurveCaveat = "These odds assume you keep your opening seven. " +
		"Mulliganing digs for lands, and against Tier 1 it is worth about six " +
		"points at turn four on these decks — so read a slot count as the " +
		"pessimistic end."

	PolicyCaveat = "Policies are judged on spells deployed through turn 8, " +
		"the same measure the land sweep uses: mulligan rate alone recommends " +
		"keeping everything, and hand quality alone recommends mulliganing " +
		"forever."
)

// values converts the compiled library to the value slice the closed forms
// take. `karsten.Read` and `curve.Curve` sit below `compile` and take plain
// records, which is the boundary that keeps them fast to test.
func values(library []*sim.Card) []sim.Card {
	out := make([]sim.Card, 0, len(library))
	for _, c := range library {
		out = append(out, *c)
	}
	return out
}

// ---------------------------------------------------------------- the shelf

type pipTierPayload struct {
	Pips      int     `json:"pips"`
	Turn      int     `json:"turn"`
	Need      int     `json:"need"`
	Have      int     `json:"have"`
	Met       bool    `json:"met"`
	Shortfall int     `json:"shortfall"`
	OddsNow   float64 `json:"odds_now"`
	// Cards is capped: a single-pip rung in a mono-coloured deck names thirty
	// cards, and a tooltip is not a decklist. The count rides alongside so the
	// client can say "and 27 more".
	Cards     []string `json:"cards"`
	CardCount int      `json:"card_count"`
}

type colorPayload struct {
	Color     string           `json:"color"`
	Have      int              `json:"have"`
	HaveLands int              `json:"have_lands"`
	Met       bool             `json:"met"`
	Shortfall int              `json:"shortfall"`
	Tiers     []pipTierPayload `json:"tiers"`
}

type oddsPayload struct {
	Name         string    `json:"name"`
	MV           int       `json:"mv"`
	OnCurve      *float64  `json:"on_curve"`
	ReliableTurn *int      `json:"reliable_turn"`
	Lag          *int      `json:"lag"`
	ByTurn       []float64 `json:"by_turn"`
}

type landEstimatePayload struct {
	LandsNow         int      `json:"lands_now"`
	Recommended      int      `json:"recommended"`
	Delta            int      `json:"delta"`
	AverageManaValue float64  `json:"average_mana_value"`
	CheapAccelerants int      `json:"cheap_accelerants"`
	Caveats          []string `json:"caveats"`
}

type turnPayload struct {
	Turn         int     `json:"turn"`
	FromLands    float64 `json:"from_lands"`
	FromRamp     float64 `json:"from_ramp"`
	ExpectedMana float64 `json:"expected_mana"`
	LandDropOdds float64 `json:"land_drop_odds"`
	Odds         float64 `json:"odds"`
}

type advicePayload struct {
	TargetTurn        int     `json:"target_turn"`
	TargetMana        int     `json:"target_mana"`
	Odds              float64 `json:"odds"`
	OddsPerLand       float64 `json:"odds_per_land"`
	OddsPerRamp       float64 `json:"odds_per_ramp"`
	Recommend         string  `json:"recommend"`
	Slots             *int    `json:"slots"`
	RampIsGeneric     bool    `json:"ramp_is_generic"`
	BeyondTheCurve    bool    `json:"beyond_the_curve"`
	LandsForEveryDrop *int    `json:"lands_for_every_drop"`
}

type curvePayload struct {
	DeckSize    int           `json:"deck_size"`
	Lands       int           `json:"lands"`
	Accelerants int           `json:"accelerants"`
	TargetTurn  int           `json:"target_turn"`
	TargetMana  int           `json:"target_mana"`
	Target      float64       `json:"target"`
	Turns       []turnPayload `json:"turns"`
	Advice      advicePayload `json:"advice"`
	Caveat      string        `json:"caveat"`
}

type shelfPayload struct {
	Slug          string              `json:"slug"`
	DeckName      string              `json:"deck_name"`
	DeckSize      int                 `json:"deck_size"`
	Lands         int                 `json:"lands"`
	Target        float64             `json:"target"`
	OnThePlay     bool                `json:"on_the_play"`
	Horizon       int                 `json:"horizon"`
	Colors        []colorPayload      `json:"colors"`
	LandsEstimate landEstimatePayload `json:"lands_estimate"`
	Cards         []oddsPayload       `json:"cards"`
	Approximated  []string            `json:"approximated"`
	Caveat        string              `json:"caveat"`

	DeckCheck deckCheck    `json:"deck_check"`
	ManaCurve curvePayload `json:"mana_curve"`
}

func colorsPayload(cs []karsten.ColorRequirement) []colorPayload {
	out := make([]colorPayload, 0, len(cs))
	for _, c := range cs {
		tiers := make([]pipTierPayload, 0, len(c.Tiers))
		for _, t := range c.Tiers {
			cards := t.Cards
			if len(cards) > 6 {
				cards = cards[:6]
			}
			if cards == nil {
				cards = []string{}
			}
			tiers = append(tiers, pipTierPayload{
				Pips: t.Pips, Turn: t.Turn, Need: t.Need, Have: t.Have,
				Met: t.Met(), Shortfall: t.Shortfall(),
				OddsNow:   floats.RoundTo(t.OddsNow, 4),
				Cards:     append([]string{}, cards...),
				CardCount: len(t.Cards),
			})
		}
		out = append(out, colorPayload{
			Color: c.Color, Have: c.Have, HaveLands: c.HaveLands,
			Met: c.Met(), Shortfall: c.Shortfall(), Tiers: tiers,
		})
	}
	return out
}

func shelfPayloadFrom(slug string, d *deck.Deck, s karsten.Shelf) shelfPayload {
	cards := make([]oddsPayload, 0, len(s.Odds))
	for _, o := range s.Odds {
		byTurn := make([]float64, 0, len(o.ByTurn))
		for _, x := range o.ByTurn {
			byTurn = append(byTurn, floats.RoundTo(x, 4))
		}
		var onCurve *float64
		if v := o.OnCurve(); v != nil {
			r := floats.RoundTo(*v, 4)
			onCurve = &r
		}
		cards = append(cards, oddsPayload{
			Name: o.Name, MV: o.MV, OnCurve: onCurve,
			ReliableTurn: o.ReliableTurn(), Lag: o.Lag(), ByTurn: byTurn,
		})
	}
	e := s.LandEstimate
	approx := s.Approximated
	if approx == nil {
		approx = []string{}
	}
	return shelfPayload{
		Slug: slug, DeckName: d.Name,
		DeckSize: s.DeckSize, Lands: s.Lands, Target: s.Target,
		OnThePlay: s.OnThePlay, Horizon: karsten.Horizon,
		Colors: colorsPayload(s.Colors),
		LandsEstimate: landEstimatePayload{
			LandsNow: e.LandsNow, Recommended: e.Recommended, Delta: e.Delta(),
			AverageManaValue: e.AverageManaValue,
			CheapAccelerants: e.CheapAccelerants,
			Caveats:          append([]string{}, e.Caveats...),
		},
		Cards: cards, Approximated: append([]string{}, approx...),
		Caveat: ShelfCaveat,
	}
}

func curvePayloadFrom(mc curve.ManaCurve) curvePayload {
	turns := make([]turnPayload, 0, len(mc.Turns))
	for _, t := range mc.Turns {
		turns = append(turns, turnPayload{
			Turn:         t.Turn,
			FromLands:    floats.RoundTo(t.FromLands, 2),
			FromRamp:     floats.RoundTo(t.FromRamp, 2),
			ExpectedMana: floats.RoundTo(t.ExpectedMana(), 2),
			LandDropOdds: floats.RoundTo(t.LandDropOdds, 4),
			Odds:         floats.RoundTo(t.Odds, 4),
		})
	}
	a := mc.Advice
	return curvePayload{
		DeckSize: mc.DeckSize, Lands: mc.Lands, Accelerants: mc.Accelerants,
		TargetTurn: mc.TargetTurn, TargetMana: mc.TargetMana, Target: mc.Target,
		Turns: turns,
		Advice: advicePayload{
			TargetTurn: a.TargetTurn, TargetMana: a.TargetMana, Odds: a.Odds,
			OddsPerLand: a.OddsPerLand, OddsPerRamp: a.OddsPerRamp,
			Recommend: a.Recommend, Slots: a.Slots,
			RampIsGeneric: a.RampIsGeneric, BeyondTheCurve: a.BeyondTheCurve,
			LandsForEveryDrop: a.LandsForEveryDrop,
		},
		Caveat: CurveCaveat,
	}
}

// shelfResult is `shelfruns.shelf_result`: the whole closed form for one deck,
// computed in the request. No cache and no job -- a cache would be keyed on the
// compiled deck and cost a hash plus a SELECT to save forty milliseconds of
// arithmetic, which is a cache that loses.
func (a *API) shelfResult(ctx context.Context, src library.Source, slug string,
	body map[string]any) (*shelfPayload, error) {

	onThePlay := true
	if raw, given := body["on_the_play"]; given {
		onThePlay = truthy(raw)
	}
	target := floatDefault(body, "target", karsten.Target)
	if target < 0.5 {
		target = 0.5
	}
	if target > 0.99 {
		target = 0.99
	}
	targetTurn := simInt(body, "target_turn", curve.DefaultTargetTurn)
	var targetMana *int
	if raw, given := body["target_mana"]; given && raw != nil && raw != "" {
		n := simInt(body, "target_mana", 0)
		targetMana = &n
	}

	c, err := a.compileChecked(ctx, src, slug)
	if err != nil {
		return nil, err
	}
	lib := values(c.report.Library)
	computed := karsten.Read(lib, c.report.Commander, target, onThePlay)
	// The curve rides on the shelf rather than on a route of its own: it is
	// the same arithmetic over the same compiled deck, it costs about as much,
	// and a second round trip for a second closed form would be two spinners
	// where the page needs none.
	mana := curve.Curve(lib, curve.Options{
		TargetTurn: targetTurn, TargetMana: targetMana,
		Target: target, OnTheDraw: !onThePlay,
	})
	out := shelfPayloadFrom(slug, c.deck, computed)
	out.DeckCheck = c.check
	out.ManaCurve = curvePayloadFrom(mana)
	return &out, nil
}

// --------------------------------------------------------------- the policy

type policyRow struct {
	MinLands            int           `json:"min_lands"`
	MaxLands            int           `json:"max_lands"`
	MinPieces           int           `json:"min_pieces"`
	Describe            string        `json:"describe"`
	SpellsThroughT8     float64       `json:"spells_through_t8"`
	MulliganRate        float64       `json:"mulligan_rate"`
	AvgMulligans        float64       `json:"avg_mulligans"`
	MedianCommanderTurn *tier1.Number `json:"median_commander_turn"`
	ColorScrewRate      float64       `json:"color_screw_rate"`
	StalledTurns        float64       `json:"stalled_turns"`
}

type policyResult struct {
	Slug     string      `json:"slug"`
	DeckName string      `json:"deck_name"`
	Games    int         `json:"games"`
	Turns    int         `json:"turns"`
	Seed     int         `json:"seed"`
	Rows     []policyRow `json:"rows"`
	Best     policyRow   `json:"best"`
	Baseline policyRow   `json:"baseline"`
	Gentlest policyRow   `json:"gentlest"`
	Spread   float64     `json:"spread"`
	Gain     float64     `json:"gain"`
	// Flat is the verdict, and the client renders words off it rather than
	// deciding for itself: it is measured against the default, not against the
	// grid's range, and a second implementation of that rule in TypeScript
	// would be a second chance to get it wrong.
	Flat   bool   `json:"flat"`
	Caveat string `json:"caveat"`

	Cached     bool       `json:"cached"`
	ComputedAt *string    `json:"computed_at"`
	DeckCheck  *deckCheck `json:"deck_check,omitempty"`
}

func rowPayload(r mulligan.Row) policyRow {
	return policyRow{
		MinLands: r.MinLands, MaxLands: r.MaxLands, MinPieces: r.MinPieces,
		Describe: r.Describe, SpellsThroughT8: r.SpellsThroughT8,
		MulliganRate: r.MulliganRate, AvgMulligans: r.AvgMulligans,
		MedianCommanderTurn: r.MedianCommanderTurn,
		ColorScrewRate:      r.ColorScrewRate, StalledTurns: r.StalledTurns,
	}
}

// policyParams is clamped harder than Tier 1's own runs because this
// multiplies by the size of the grid: the ceiling here is thirty-three times
// the number requested.
func policyParams(body map[string]any) (games, turns, seed int) {
	games = clampInt(simInt(body, "games", 2000), 200, 10000)
	turns = clampInt(simInt(body, "turns", 10), 8, 16)
	return games, turns, seedFrom(body)
}

func (a *API) planPolicy(ctx context.Context, src library.Source, slug string, body map[string]any) jobs.Plan {
	games, turns, seed := policyParams(body)
	label := fmt.Sprintf("%s: mulligan policies, %s games each", slug, commaGrouped(games))

	c, err := a.compileChecked(ctx, src, slug)
	if err != nil {
		return jobs.Plan{Kind: "sim.policy", Label: label, Run: deferredFailure(err)}
	}
	check := c.check

	// The grid rides in `extra` and it rides in full, not as a count. Which
	// rules were tried decides the answer, and a count is a fingerprint that
	// does not change when somebody swaps a 6 for a 7 in the grid -- the exact
	// shape of stale-cache bug ADR 18 keys on compiled input to avoid.
	// `KeepRule` is the baseline the gain is measured against, which is a real
	// input to the verdict.
	grid := make([][]int, 0, 33)
	for _, r := range mulligan.Candidates() {
		grid = append(grid, []int{r.MinLands, r.MaxLands, r.MinManaPieces})
	}
	sortGrid(grid)
	key := cache.Key("sim.policy", cache.Input{
		Library: c.report.Library, Commander: c.report.Commander,
		Games: games, Turns: turns, KeepRule: tier1.DefaultKeepRule(), Seed: seed,
		Extra: map[string]any{
			"grid": grid, "through": mulligan.Through, "flat": mulligan.Flat,
		},
	})
	if hit := a.simCache.Get(ctx, key); hit != nil {
		var stored policyResult
		if err := json.Unmarshal(hit.Result, &stored); err == nil {
			stamp := hit.CreatedAt
			stored.Cached, stored.ComputedAt, stored.DeckCheck = true, &stamp, &check
			return jobs.Plan{Kind: "sim.policy", Label: label, Result: stored}
		}
		a.log.Warn("a cached policy result did not decode; recomputing", "slug", slug)
	}

	report, d := c.report, c.deck
	return jobs.Plan{Kind: "sim.policy", Label: label, Run: func(rep jobs.Progress) (any, error) {
		sweep, err := mulligan.Search(report.Library, report.Commander, mulligan.Options{
			Games: games, Turns: turns, Seed: seed,
			Progress: func(done, total int) { rep.Report(done, total) },
		})
		if err != nil {
			return nil, err
		}
		rows := make([]policyRow, 0, len(sweep.Rows))
		for _, r := range sweep.Rows {
			rows = append(rows, rowPayload(r))
		}
		result := policyResult{
			Slug: slug, DeckName: d.Name, Games: games, Turns: turns, Seed: seed,
			Rows: rows, Best: rowPayload(sweep.Best),
			Baseline: rowPayload(sweep.Baseline), Gentlest: rowPayload(sweep.Gentlest()),
			Spread: sweep.Spread, Gain: sweep.Gain(), Flat: sweep.IsFlat(),
			Caveat: PolicyCaveat,
		}
		// The job's own context, not the request's -- see planMana in
		// simruns.go for the bug this closes.
		a.simCache.Put(context.Background(), key, "sim.policy", result)
		result.Cached, result.ComputedAt, result.DeckCheck = false, nil, &check
		return result, nil
	}}
}

// sortGrid sorts rows as tuples: element-wise, ascending -- the recorded
// grid order.
func sortGrid(grid [][]int) {
	for i := 1; i < len(grid); i++ {
		cur := grid[i]
		j := i - 1
		for j >= 0 && tupleLess(cur, grid[j]) {
			grid[j+1] = grid[j]
			j--
		}
		grid[j+1] = cur
	}
}

func tupleLess(a, b []int) bool {
	for i := range a {
		if i >= len(b) {
			return false
		}
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// ---------------------------------------------------------------- the routes

func (a *API) simShelf(w http.ResponseWriter, r *http.Request) {
	body, src, slug, ok := a.simBody(w, r)
	if !ok {
		return
	}
	out, err := a.shelfResult(r.Context(), src, slug, body)
	if err != nil {
		// A missing deck is a 404; no pool, a deck that cannot compile, and a
		// deck with nothing in it are all 422 -- every one of those is a fact
		// about the request's deck, not a broken server.
		var missing library.ErrNotFound
		if errors.As(err, &missing) {
			wire.Detail(w, http.StatusNotFound, missing.Error())
			return
		}
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

func (a *API) simPolicy(w http.ResponseWriter, r *http.Request) {
	body, src, slug, ok := a.simBody(w, r)
	if !ok {
		return
	}
	a.submit(w, r, a.planPolicy(r.Context(), src, slug, body))
}

func floatDefault(body map[string]any, key string, fallback float64) float64 {
	raw, given := body[key]
	if !given || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%g", &f); err == nil {
			return f
		}
	}
	return fallback
}
