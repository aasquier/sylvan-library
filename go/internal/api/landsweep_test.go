package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
)

// The land sweep: the one Tier 1 read CLAUDE.md names outright, because
// **spells deployed through T8** is what a land count is decided on and
// commander speed is not (it rises forever). So the row's measure, the
// argmax rule and the flat verdict are pinned here rather than inferred from
// a green run.
//
// The sweep is also the heaviest user of ADR 18's cache -- one key per land
// count, eleven by default -- and the cache is consulted twice on purpose.
// Both halves are tested, because the failure mode of the second read going
// missing is invisible: the sweep still answers, it just pays twice.

// sweepRig is an API with a real sim cache, since the sweep's whole shape is
// about what it stores and re-reads.
type sweepRig struct {
	api  *API
	jobs *jobs.Registry
}

func newSweepRig(t *testing.T) *sweepRig {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := cache.Open(dbPath, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t),
		AdminEmail: "alice@example.com", AppDB: db, Jobs: reg,
		SimCache: store, Logger: quiet})
	return &sweepRig{api: a, jobs: reg}
}

// run submits a sweep and waits for it, returning the decoded result.
func (r *sweepRig) run(t *testing.T, body string) map[string]any {
	t.Helper()
	status, submitted, raw := callAs(t, r.api, alice, "POST", "/api/sim/lands", body)
	if status != 200 {
		t.Fatalf("the sweep answered %d: %s", status, raw)
	}
	// A cache hit is born finished and carries its result inline; a miss
	// carries an id to wait on.
	if result, done := submitted["result"].(map[string]any); done && submitted["status"] == "done" {
		return result
	}
	id, _ := submitted["id"].(string)
	if id == "" {
		t.Fatalf("no job id and no result: %v", submitted)
	}
	r.jobs.Wait()
	job := r.jobs.Get(id, alice.UserID)
	if job == nil {
		t.Fatal("the registry lost the sweep")
	}
	if job.Status() != "done" {
		t.Fatalf("the sweep ended %q: %v", job.Status(), job.Payload().Error)
	}
	encoded, err := json.Marshal(job.Result())
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The sweep's rows are a curve, and every row carries the measure the caveat
// names. A row ordering that followed the map's iteration rather than the
// counts would be a chart that lies, so the ordering is asserted.
func TestTheLandSweepReturnsAnOrderedCurveOfDeployment(t *testing.T) {
	t.Parallel()
	rig := newSweepRig(t)
	out := rig.run(t, `{"slug":"kaheera","low":33,"high":36,"games":120,"turns":8,"seed":5}`)

	if out["caveat"] != LandSweepCaveat {
		t.Errorf("the sweep's caveat is %q", out["caveat"])
	}
	if out["seed"] != float64(5) {
		t.Errorf("the seed came back as %v -- a seed is a promise", out["seed"])
	}
	rows, ok := out["rows"].([]any)
	if !ok || len(rows) != 4 {
		t.Fatalf("the sweep returned %v rows, want 33..36", out["rows"])
	}
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("row %d is %T", i, raw)
		}
		if got := row["lands"]; got != float64(33+i) {
			t.Fatalf("row %d is %v lands -- the rows are out of order", i, got)
		}
		// The measure a land count is decided on, and the two that ride
		// beside it.
		for _, key := range []string{"spells_through_t8", "wasted_through_t8",
			"commander_by_t5", "mulligan_rate"} {
			if _, present := row[key]; !present {
				t.Errorf("row %d carries no %q", i, key)
			}
		}
	}
	// The spread makes a flat curve visible as flat rather than read as a
	// peak, and the verdict is decided here rather than in TypeScript.
	if _, ok := out["deployment_spread"].(float64); !ok {
		t.Errorf("deployment_spread is %T", out["deployment_spread"])
	}
	if _, ok := out["flat"].(bool); !ok {
		t.Errorf("flat is %T, not a verdict", out["flat"])
	}
	argmax, ok := out["argmax_lands"].(float64)
	if !ok || argmax < 33 || argmax > 36 {
		t.Errorf("argmax_lands is %v, outside the swept band", out["argmax_lands"])
	}
	if _, present := out["deck_check"]; !present {
		t.Error("the sweep carries no deck_check")
	}
}

// The sweep is the heaviest user of ADR 18's cache. The second ask must be
// born finished and must say so: an uncached result quoted as cached, or a
// cached one quoted as fresh, is the thing CLAUDE.md forbids.
func TestASweptCountIsCachedAndQuotedAsCached(t *testing.T) {
	t.Parallel()
	rig := newSweepRig(t)
	body := `{"slug":"kaheera","low":34,"high":35,"games":100,"turns":8,"seed":11}`

	first := rig.run(t, body)
	if first["cached"] != false {
		t.Errorf("the first sweep reported cached=%v", first["cached"])
	}
	if first["computed_at"] != nil {
		t.Errorf("a fresh sweep carries computed_at=%v", first["computed_at"])
	}

	second := rig.run(t, body)
	if second["cached"] != true {
		t.Fatalf("the second sweep reported cached=%v -- the per-count rows were not stored",
			second["cached"])
	}
	if second["computed_at"] == nil {
		t.Error("a cached sweep carries no computed_at")
	}
	// Same seed, same input: the same curve, bit for bit.
	firstRows, _ := json.Marshal(first["rows"])
	secondRows, _ := json.Marshal(second["rows"])
	if string(firstRows) != string(secondRows) {
		t.Errorf("the cached curve differs from the computed one:\n%s\n%s", firstRows, secondRows)
	}
}

// A sweep that is partly cached simulates only what is missing. This is the
// second cache read -- the one in the worker rather than the plan -- and its
// absence is invisible: the sweep still answers, it just pays twice.
func TestAPartlyCachedSweepReusesTheRowsItAlreadyHas(t *testing.T) {
	t.Parallel()
	rig := newSweepRig(t)
	// Warm two counts.
	rig.run(t, `{"slug":"kaheera","low":34,"high":35,"games":100,"turns":8,"seed":13}`)
	// Ask for four, two of which are already there.
	out := rig.run(t, `{"slug":"kaheera","low":34,"high":37,"games":100,"turns":8,"seed":13}`)

	rows, ok := out["rows"].([]any)
	if !ok || len(rows) != 4 {
		t.Fatalf("the widened sweep returned %v", out["rows"])
	}
	if out["cached"] != false {
		t.Errorf("a partly-cached sweep reported cached=%v -- it did fresh work", out["cached"])
	}
	for i, raw := range rows {
		row, _ := raw.(map[string]any)
		if got := row["lands"]; got != float64(34+i) {
			t.Fatalf("the reused rows landed out of order at %d: %v", i, got)
		}
	}
}

// The band is clamped at both ends and repaired when it arrives inverted,
// because a sweep of 20-60 lands is the widest question worth asking and a
// backwards one is a typo rather than a request.
func TestTheLandBandIsClampedAndRepaired(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		body      map[string]any
		low, high int
		games     int
		turns     int
	}{
		{"the defaults", map[string]any{}, 30, 40, 5000, 10},
		{"below the floor", map[string]any{"low": json.Number("3")}, 20, 40, 5000, 10},
		{"above the ceiling", map[string]any{"high": json.Number("99")}, 30, 60, 5000, 10},
		{"inverted", map[string]any{
			"low": json.Number("40"), "high": json.Number("30")}, 30, 40, 5000, 10},
		{"games and turns clamped low", map[string]any{
			"games": json.Number("1"), "turns": json.Number("1")}, 30, 40, 100, 8},
		{"games and turns clamped high", map[string]any{
			"games": json.Number("9999999"), "turns": json.Number("99")}, 30, 40, 100000, 20},
	} {
		p := landParamsFrom(tc.body)
		if len(p.counts) == 0 {
			t.Fatalf("%s: swept nothing", tc.name)
		}
		if p.counts[0] != tc.low || p.counts[len(p.counts)-1] != tc.high {
			t.Errorf("%s: swept %d-%d, want %d-%d",
				tc.name, p.counts[0], p.counts[len(p.counts)-1], tc.low, tc.high)
		}
		if p.games != tc.games || p.turns != tc.turns {
			t.Errorf("%s: games=%d turns=%d, want %d and %d",
				tc.name, p.games, p.turns, tc.games, tc.turns)
		}
		// The band is contiguous -- the rows are a curve.
		for i, n := range p.counts {
			if n != tc.low+i {
				t.Fatalf("%s: the band has a hole at %d: %v", tc.name, i, p.counts)
			}
		}
	}
}

// resize is what makes a per-count cache key sound: a pure function of the
// compiled library and the count. It cycles the existing lands so the colour
// mix -- the thing being measured -- survives, and holds the deck at 99 so
// mulligan rates stay comparable across counts.
func TestResizeHoldsTheDeckAt99AndKeepsTheColourMix(t *testing.T) {
	t.Parallel()
	forest := &sim.Card{Name: "Forest", IsLand: true}
	plains := &sim.Card{Name: "Plains", IsLand: true}
	spells := make([]*sim.Card, 0, 90)
	for i := 0; i < 90; i++ {
		spells = append(spells, &sim.Card{Name: "Spell"})
	}
	library := append([]*sim.Card{forest, plains}, spells...)

	for _, count := range []int{20, 35, 60} {
		out, err := resize(library, count)
		if err != nil {
			t.Fatalf("%d lands: %v", count, err)
		}
		if len(out) != 99 {
			t.Errorf("%d lands produced a %d-card deck, want 99", count, len(out))
		}
		lands := 0
		for _, c := range out {
			if c.IsLand {
				lands++
			}
		}
		if lands != count {
			t.Errorf("asked for %d lands, got %d", count, lands)
		}
		// Cycling means both lands appear, so the colour mix survives.
		seen := map[string]bool{}
		for _, c := range out[:count] {
			seen[c.Name] = true
		}
		if !seen["Forest"] || !seen["Plains"] {
			t.Errorf("%d lands dropped a colour: %v", count, seen)
		}
	}

	// More lands than the deck holds: the spells are gone and the deck is
	// all lands rather than over 99.
	out, err := resize(library, 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 99 {
		t.Errorf("a 99-land sweep produced %d cards", len(out))
	}

	// A deck with nothing to sweep is refused, and the refusal has to reach
	// the caller the way a missing deck does.
	if _, err := resize(spells, 35); err == nil {
		t.Error("a deck with no lands was swept anyway")
	} else if !strings.Contains(err.Error(), "no lands") {
		t.Errorf("the refusal said %q", err)
	}
}

// A landless deck fails as a job in state `error` rather than as a 500 --
// the same 200-then-error shape a missing deck gets, because the UI knows
// how to show one.
func TestALandlessDeckFailsAsAJobRatherThanAsAServerError(t *testing.T) {
	t.Parallel()
	rig := newSweepRig(t)
	// `draft` is the fixture with no mana base to sweep.
	status, submitted, raw := callAs(t, rig.api, alice, "POST", "/api/sim/lands",
		`{"slug":"no-such-deck","low":34,"high":35,"games":100}`)
	if status != 200 {
		t.Fatalf("a missing deck answered %d rather than a failing job: %s", status, raw)
	}
	id, _ := submitted["id"].(string)
	if id == "" {
		t.Fatalf("no job came back: %v", submitted)
	}
	rig.jobs.Wait()
	job := rig.jobs.Get(id, alice.UserID)
	if job == nil || job.Status() != "error" {
		t.Fatalf("the job ended %v", job)
	}
	// The bare slug, not the sentence: a job's error becomes a JS Error and
	// the screen shows it, so an unstripped one said "no deck '['x']'".
	if got := job.Payload().Error; got == nil || *got != "no-such-deck" {
		t.Errorf("the job's error is %v, want the bare slug", got)
	}
}

// The argmax keeps the FIRST maximum, so a tie names the lower land count --
// the recorded rule, and the one that matters because a flat curve is full
// of ties.
func TestTheArgmaxKeepsTheFirstMaximumAndTheSpreadDecidesFlat(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{Name: "Test Deck"}

	tied := []landRow{
		{Lands: 33, SpellsThroughT8: 7.00},
		{Lands: 34, SpellsThroughT8: 7.00},
		{Lands: 35, SpellsThroughT8: 6.90},
	}
	out := landSummaryFrom("t", d, tied, 100, 3)
	if out.ArgmaxLands != 33 {
		t.Errorf("a tie named %d lands, want the lower 33", out.ArgmaxLands)
	}
	if !out.Flat {
		t.Errorf("a 0.10 spread was called a peak (spread %v)", out.DeploymentSpread)
	}

	peaked := []landRow{
		{Lands: 33, SpellsThroughT8: 6.0},
		{Lands: 34, SpellsThroughT8: 8.5},
		{Lands: 35, SpellsThroughT8: 7.0},
	}
	out = landSummaryFrom("t", d, peaked, 100, 3)
	if out.ArgmaxLands != 34 {
		t.Errorf("the peak is at %d lands, want 34", out.ArgmaxLands)
	}
	if out.Flat {
		t.Error("a 2.5 spread was called flat")
	}
	if out.DeploymentSpread != 2.5 {
		t.Errorf("the spread is %v, want 2.5", out.DeploymentSpread)
	}
	if out.Caveat != LandSweepCaveat {
		t.Errorf("the summary's caveat is %q", out.Caveat)
	}
	if out.Games != 100 || out.Seed != 3 || out.DeckName != "Test Deck" {
		t.Errorf("the summary lost its provenance: %+v", out)
	}

	// A single row is its own argmax and has no spread.
	one := landSummaryFrom("t", d, []landRow{{Lands: 34, SpellsThroughT8: 7}}, 100, 3)
	if one.ArgmaxLands != 34 || one.DeploymentSpread != 0 || !one.Flat {
		t.Errorf("a one-row sweep summarised as %+v", one)
	}
}

// The row carries the two turn-8 numbers rounded the way the recorded
// goldens hold them, and the commander read at turn 5 unrounded beside them.
func TestTheLandRowRoundsTheTurnEightNumbers(t *testing.T) {
	t.Parallel()
	summary := tier1.SimSummary{MulliganRate: 0.125}
	summary.CommanderByTurn = map[int]float64{5: 0.5}
	row := landRowFrom(37, summary)
	if row.Lands != 37 {
		t.Errorf("the row is labelled %d", row.Lands)
	}
	if row.CommanderByT5 != 0.5 {
		t.Errorf("commander_by_t5 is %v", row.CommanderByT5)
	}
	if row.MulliganRate != 0.125 {
		t.Errorf("the mulligan rate is %v", row.MulliganRate)
	}
}

// The cache key is a pure function of the compiled input plus the knobs, so
// two different land counts must never collide -- a collision would serve
// one count's curve for another's.
func TestEachLandCountGetsItsOwnCacheKey(t *testing.T) {
	t.Parallel()
	commander := &sim.Card{Name: "Kaheera"}
	library := []*sim.Card{{Name: "Forest", IsLand: true}, {Name: "Spell"}}
	p := landParams{games: 100, turns: 8, keep: tier1.DefaultKeepRule(), seed: 4}

	thirty, err := resize(library, 30)
	if err != nil {
		t.Fatal(err)
	}
	forty, err := resize(library, 40)
	if err != nil {
		t.Fatal(err)
	}
	if landKey(thirty, commander, p) == landKey(forty, commander, p) {
		t.Fatal("two land counts share a cache key -- one curve would be served for the other")
	}
	// The same input twice is the same key: that is what makes the second
	// ask free. The two sides are resized separately so this is a question
	// about the key rather than about one value compared with itself.
	againThirty, err := resize(library, 30)
	if err != nil {
		t.Fatal(err)
	}
	if landKey(thirty, commander, p) != landKey(againThirty, commander, p) {
		t.Fatal("the same input hashed to two keys -- the second ask would never be free")
	}
	// A different seed is a different question.
	other := p
	other.seed = 5
	if landKey(thirty, commander, p) == landKey(thirty, commander, other) {
		t.Fatal("the seed is not in the key -- a seed is a promise")
	}
}

// simInt reads the shapes a browser sends and refuses to guess at the rest,
// because a garbled knob that silently became zero would be a different run
// reported as the one that was asked for.
func TestSimIntReadsTheWireShapesAndFallsBack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"absent", map[string]any{}, 7},
		{"null", map[string]any{"games": nil}, 7},
		{"a JSON number", map[string]any{"games": json.Number("500")}, 500},
		{"a float that is whole", map[string]any{"games": json.Number("500.0")}, 500},
		{"a string from an input", map[string]any{"games": "250"}, 250},
		{"an empty string", map[string]any{"games": ""}, 7},
		{"nonsense", map[string]any{"games": "many"}, 7},
		// A bool coerces the way the recorded Python did: int(True) is 1.
		{"true", map[string]any{"games": true}, 1},
		{"false", map[string]any{"games": false}, 0},
		{"a list", map[string]any{"games": []any{1}}, 7},
	} {
		if got := simInt(tc.body, "games", 7); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}

// Every job label spells a games count with separators, and the negative
// branch exists because the recursion would otherwise put a comma after the
// minus sign.
func TestCommaGroupedSpellsAGamesCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   int
		want string
	}{
		{0, "0"}, {7, "7"}, {999, "999"}, {1000, "1,000"},
		{20000, "20,000"}, {100000, "100,000"}, {1234567, "1,234,567"},
		{-1000, "-1,000"},
	} {
		if got := commaGrouped(tc.in); got != tc.want {
			t.Errorf("commaGrouped(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The land route shares `simBody` with its three siblings, so the refusals
// are the same ones -- asserted here because this route reaches them through
// a different handler.
func TestTheLandRouteRefusesTheSameThingsItsSiblingsDo(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()
	a.jobs = jobs.New(jobs.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	status, payload, _ := callAs(t, a, alice, "POST", "/api/sim/lands", `{}`)
	if status != 422 {
		t.Errorf("no slug answered %d", status)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "slug is required") {
		t.Errorf("the refusal said %q", detail)
	}

	status, _, _ = callAs(t, a, alice, "POST", "/api/sim/lands", `not json`)
	if status != 400 && status != 422 {
		t.Errorf("a malformed body answered %d", status)
	}
}

// int64Ptr and progressOf are one-liners the sweep leans on; a nil seed
// pointer would make the run unseeded, which is the determinism contract.
func TestTheSweepsSmallHelpers(t *testing.T) {
	t.Parallel()
	if got := int64Ptr(7); got == nil || *got != 7 {
		t.Errorf("int64Ptr(7) = %v", got)
	}
	if got := int64Ptr(0); got == nil || *got != 0 {
		t.Error("a zero seed became a nil pointer -- an unseeded run")
	}
	var done, total int
	progressOf(reportFunc(func(d, tt int) { done, total = d, tt }))(3, 10)
	if done != 3 || total != 10 {
		t.Errorf("progress reported %d/%d", done, total)
	}
}

// reportFunc adapts a closure to jobs.Progress.
type reportFunc func(done, total int)

func (f reportFunc) Report(done, total int)               { f(done, total) }
func (f reportFunc) ReportPartial(done, total int, _ any) { f(done, total) }
