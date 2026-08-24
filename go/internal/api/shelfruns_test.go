package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/curve"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
	"github.com/aasquier/sylvan-library/go/internal/sim/mulligan"
)

// The closed form and the policy search -- the one file where the
// sibling-duration rule came out different for two routes, so the tests
// carry that difference: the shelf answers inline and the policy answers a
// job id.
//
// The shelf's whole reason for existing is Tier 1.5's caveat (CLAUDE.md:
// quote it as a question about the mana base, never as a chance of having
// the card), so the caveat's presence in the payload is asserted as hard as
// the arithmetic is. A number that travels without it is the failure mode.

// shelfPost runs a POST through the router as alice and decodes the payload.
func shelfPost(t *testing.T, a *API, target, body string) (int, map[string]any) {
	t.Helper()
	status, payload, raw := callAs(t, a, alice, "POST", target, body)
	if status != 200 && payload == nil {
		t.Fatalf("%s answered %d with an undecodable body: %s", target, status, raw)
	}
	return status, payload
}

// The shelf is a plain route: arithmetic over an already-compiled deck,
// answered in the response rather than through a job. Every field the page
// renders is asserted, because a closed form that quietly dropped its tiers
// still answers 200.
func TestTheShelfAnswersTheClosedFormInline(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	status, payload := shelfPost(t, a, "/api/sim/shelf", `{"slug":"kaheera"}`)
	if status != 200 {
		t.Fatalf("the shelf answered %d: %v", status, payload)
	}
	if payload["slug"] != "kaheera" {
		t.Errorf("the shelf named %v", payload["slug"])
	}
	// Tier 1.5 never travels without the sentence that says what it is.
	if payload["caveat"] != ShelfCaveat {
		t.Errorf("the shelf's caveat is %q", payload["caveat"])
	}
	colors, ok := payload["colors"].([]any)
	if !ok || len(colors) == 0 {
		t.Fatalf("the shelf found no colours: %v", payload["colors"])
	}
	first, ok := colors[0].(map[string]any)
	if !ok {
		t.Fatalf("a colour row is %T", colors[0])
	}
	for _, key := range []string{"color", "have", "have_lands", "met", "shortfall", "tiers"} {
		if _, present := first[key]; !present {
			t.Errorf("a colour row carries no %q", key)
		}
	}
	tiers, ok := first["tiers"].([]any)
	if !ok || len(tiers) == 0 {
		t.Fatalf("a colour carries no pip tiers: %v", first["tiers"])
	}
	tier, ok := tiers[0].(map[string]any)
	if !ok {
		t.Fatalf("a pip tier is %T", tiers[0])
	}
	// The card list is capped and the count rides alongside, so the client
	// can say "and 27 more" rather than rendering a decklist in a tooltip.
	named, _ := tier["cards"].([]any)
	count, _ := tier["card_count"].(float64)
	if int(count) < len(named) {
		t.Errorf("card_count %v is below the %d cards named", count, len(named))
	}

	// The curve rides on the shelf rather than on a route of its own.
	mana, ok := payload["mana_curve"].(map[string]any)
	if !ok {
		t.Fatalf("the shelf carries no mana curve: %v", payload["mana_curve"])
	}
	if mana["caveat"] != CurveCaveat {
		t.Errorf("the curve's caveat is %q", mana["caveat"])
	}
	// The deck check rides on every result, invalid or not.
	if _, present := payload["deck_check"]; !present {
		t.Error("the shelf carries no deck_check")
	}
}

// A missing deck is a 404 and everything else about the request's deck is a
// 422 -- a fact about the deck, never a broken server.
func TestTheShelfSeparatesAMissingDeckFromAnUnusableOne(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	status, payload := shelfPost(t, a, "/api/sim/shelf", `{"slug":"no-such-deck"}`)
	if status != 404 {
		t.Errorf("a missing deck answered %d: %v", status, payload)
	}

	status, payload = shelfPost(t, a, "/api/sim/shelf", `{}`)
	if status != 422 {
		t.Errorf("no slug answered %d", status)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "slug is required") {
		t.Errorf("the refusal said %q", detail)
	}
}

// An instance with no card pool cannot compile a deck, and says so as a fact
// about the request rather than as a 500.
func TestTheShelfRefusesWithoutACardPool(t *testing.T) {
	t.Parallel()
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Everything the working rig has except the pool, so the refusal is
	// about the pool rather than about resolving the library.
	a := New(Config{DecksDir: decksDir(t), AdminEmail: "alice@example.com", AppDB: db})
	status, payload := shelfPost(t, a, "/api/sim/shelf", `{"slug":"kaheera"}`)
	if status != 422 {
		t.Errorf("no pool answered %d: %v", status, payload)
	}
}

// The target is clamped at both ends: a caller asking for certainty gets
// 0.99 and a caller asking for a coin flip gets 0.5, because the closed form
// outside that band is arithmetic about nothing.
func TestTheShelfClampsTheTargetAtBothEnds(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	for _, tc := range []struct{ name, body string }{
		{"far below the floor", `{"slug":"kaheera","target":0.01}`},
		{"far above the ceiling", `{"slug":"kaheera","target":0.999999}`},
		{"exactly the default", `{"slug":"kaheera"}`},
	} {
		status, payload := shelfPost(t, a, "/api/sim/shelf", tc.body)
		if status != 200 {
			t.Errorf("%s answered %d: %v", tc.name, status, payload)
		}
	}
}

// On the play and on the draw are different questions, and the shelf must
// answer them differently -- an extra card is an extra land often enough
// that a shelf blind to it would recommend the same mana base for both.
func TestTheShelfAnswersThePlayAndTheDrawDifferently(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	_, onPlay := shelfPost(t, a, "/api/sim/shelf", `{"slug":"kaheera","on_the_play":true}`)
	_, onDraw := shelfPost(t, a, "/api/sim/shelf", `{"slug":"kaheera","on_the_play":false}`)

	play, _ := json.Marshal(onPlay["colors"])
	draw, _ := json.Marshal(onDraw["colors"])
	if string(play) == string(draw) {
		t.Error("the draw's extra card changed nothing about the requirements")
	}
}

// `target_mana` is optional and, when given, is what the curve aims at.
func TestTheShelfTakesAnExplicitManaTarget(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()

	status, withTarget := shelfPost(t, a, "/api/sim/shelf",
		`{"slug":"kaheera","target_mana":4,"target_turn":5}`)
	if status != 200 {
		t.Fatalf("an explicit target answered %d: %v", status, withTarget)
	}
	mana, ok := withTarget["mana_curve"].(map[string]any)
	if !ok {
		t.Fatalf("no curve came back: %v", withTarget)
	}
	if _, present := mana["caveat"]; !present {
		t.Error("the curve dropped its caveat when given a target")
	}

	// An empty string is "not given" rather than zero: the frontend sends
	// one for a cleared input, and a zero target would silently be a
	// different question.
	status, blank := shelfPost(t, a, "/api/sim/shelf",
		`{"slug":"kaheera","target_mana":""}`)
	if status != 200 {
		t.Fatalf("a cleared target answered %d: %v", status, blank)
	}
}

// The policy search is a job, not a route: thirty-three seeded Tier 1 runs is
// squarely what the registry exists for.
func TestThePolicySearchIsAJobAndItsGridIsOrdered(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := jobs.New(jobs.Config{Logger: quiet})
	a, done := deckAPI(t, claude.Settings{}, true)
	defer done()
	a.jobs = reg
	a.log = quiet

	// The floor of the clamp, so the search is thirty-three short runs
	// rather than thirty-three long ones.
	status, submitted := shelfPost(t, a, "/api/sim/policy",
		`{"slug":"kaheera","games":200,"turns":8,"seed":7}`)
	if status != 200 {
		t.Fatalf("the policy search answered %d: %v", status, submitted)
	}
	id, _ := submitted["id"].(string)
	if id == "" {
		t.Fatalf("no job id came back: %v", submitted)
	}
	reg.Wait()
	job := reg.Get(id, alice.UserID)
	if job == nil {
		t.Fatal("the registry lost the job")
	}
	if job.Status() != "done" {
		t.Fatalf("the policy job ended %q", job.Status())
	}

	raw, err := json.Marshal(job.Result())
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out["caveat"] != PolicyCaveat {
		t.Errorf("the policy result's caveat is %q", out["caveat"])
	}
	if out["seed"] != float64(7) {
		t.Errorf("the seed came back as %v -- a seed is a promise", out["seed"])
	}
	rows, ok := out["rows"].([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("the search returned no rows: %v", out["rows"])
	}
	for _, key := range []string{"best", "baseline", "gentlest", "spread", "gain", "flat"} {
		if _, present := out[key]; !present {
			t.Errorf("the policy result carries no %q", key)
		}
	}
	// The verdict is decided here rather than in TypeScript, so it must be
	// a real boolean rather than a missing key the client reads as false.
	if _, ok := out["flat"].(bool); !ok {
		t.Errorf("flat is %T, not a verdict", out["flat"])
	}
	// Every row carries the measure the caveat names.
	first, _ := rows[0].(map[string]any)
	if _, present := first["spells_through_t8"]; !present {
		t.Error("a policy row carries no spells_through_t8 -- the measure the caveat names")
	}
}

// The policy's clamps are harder than Tier 1's own because the grid
// multiplies them: the ceiling here is thirty-three times what is asked.
func TestThePolicyParametersAreClampedHarderThanTierOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		body         map[string]any
		games, turns int
	}{
		{"the defaults", map[string]any{}, 2000, 10},
		{"below both floors", map[string]any{
			"games": json.Number("1"), "turns": json.Number("1")}, 200, 8},
		{"above both ceilings", map[string]any{
			"games": json.Number("999999"), "turns": json.Number("999")}, 10000, 16},
		{"inside the band", map[string]any{
			"games": json.Number("500"), "turns": json.Number("12")}, 500, 12},
	} {
		games, turns, _ := policyParams(tc.body)
		if games != tc.games || turns != tc.turns {
			t.Errorf("%s: games=%d turns=%d, want %d and %d",
				tc.name, games, turns, tc.games, tc.turns)
		}
	}
}

// The grid is sorted by an insertion sort over integer tuples, so its
// ordering is pinned directly -- a mis-ordered grid still renders, just
// wrongly.
func TestTheGridSortsByTupleAndHandlesRaggedRows(t *testing.T) {
	t.Parallel()
	grid := [][]int{{3, 5, 1}, {2, 4, 0}, {3, 4, 2}, {2, 4, 0}}
	sortGrid(grid)
	want := [][]int{{2, 4, 0}, {2, 4, 0}, {3, 4, 2}, {3, 5, 1}}
	for i := range want {
		for j := range want[i] {
			if grid[i][j] != want[i][j] {
				t.Fatalf("sorted to %v, want %v", grid, want)
			}
		}
	}

	// A shorter tuple sorts first when it is a prefix of a longer one, and
	// a longer one never claims to be less than its own prefix.
	if !tupleLess([]int{1, 2}, []int{1, 2, 3}) {
		t.Error("a prefix does not sort before what extends it")
	}
	if tupleLess([]int{1, 2, 3}, []int{1, 2}) {
		t.Error("an extension sorts before its own prefix")
	}
	if tupleLess([]int{1, 2}, []int{1, 2}) {
		t.Error("a tuple sorts before itself")
	}

	// Already sorted, and empty: the two inputs an insertion sort is most
	// likely to walk off the end of.
	sortGrid([][]int{})
	sortGrid([][]int{{1}})
}

// floatDefault reads the two shapes a browser sends -- a JSON number and the
// string an input element produces -- and falls back rather than failing on
// anything else, because a garbled tuning knob must not lose the run.
func TestFloatDefaultReadsBothWireShapesAndFallsBack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body map[string]any
		want float64
	}{
		{"absent", map[string]any{}, 0.9},
		{"explicitly null", map[string]any{"target": nil}, 0.9},
		{"a JSON number", map[string]any{"target": json.Number("0.75")}, 0.75},
		{"a string from an input", map[string]any{"target": "0.6"}, 0.6},
		{"an unparseable string", map[string]any{"target": "nonsense"}, 0.9},
		{"the wrong type entirely", map[string]any{"target": true}, 0.9},
		{"a malformed number", map[string]any{"target": json.Number("1e")}, 0.9},
	} {
		if got := floatDefault(tc.body, "target", 0.9); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// values flattens the compiled library the closed forms take. The copy is
// the point: `karsten.Read` and `curve.Curve` sit below `compile` and take
// plain records, which is the boundary that keeps them fast to test.
func TestValuesCopiesTheCompiledLibrary(t *testing.T) {
	t.Parallel()
	one := &sim.Card{Name: "Forest"}
	two := &sim.Card{Name: "Llanowar Elves"}
	out := values([]*sim.Card{one, two})
	if len(out) != 2 || out[0].Name != "Forest" || out[1].Name != "Llanowar Elves" {
		t.Fatalf("flattened to %v", out)
	}
	out[0].Name = "changed"
	if one.Name != "Forest" {
		t.Error("the flattened slice aliases the compiled library")
	}
	if got := values(nil); got == nil || len(got) != 0 {
		t.Errorf("an empty library flattened to %v, want an empty slice", got)
	}
}

// The three payload converters are pure shape, and shape is the contract the
// frontend reads -- so each is asserted field by field.
func TestThePayloadConvertersCarryEveryField(t *testing.T) {
	t.Parallel()

	row := rowPayload(mulligan.Row{MinLands: 2, MaxLands: 5, MinPieces: 1,
		Describe: "keep 2-5", SpellsThroughT8: 7.5, MulliganRate: 0.2,
		AvgMulligans: 0.3, ColorScrewRate: 0.1, StalledTurns: 1.5})
	if row.MinLands != 2 || row.MaxLands != 5 || row.MinPieces != 1 {
		t.Errorf("the land band came through as %+v", row)
	}
	if row.Describe != "keep 2-5" || row.SpellsThroughT8 != 7.5 {
		t.Errorf("the measure came through as %+v", row)
	}
	if row.MulliganRate != 0.2 || row.AvgMulligans != 0.3 ||
		row.ColorScrewRate != 0.1 || row.StalledTurns != 1.5 {
		t.Errorf("a rate was dropped: %+v", row)
	}

	// An empty requirement list converts to an empty payload list rather
	// than to null: the page iterates it.
	if got := colorsPayload(nil); got == nil || len(got) != 0 {
		t.Errorf("no requirements became %v", got)
	}
	if got := curvePayloadFrom(curve.ManaCurve{}); got.Caveat != CurveCaveat {
		t.Errorf("an empty curve lost its caveat: %+v", got)
	}
}

// The closed form's own defaults are the ones the shelf hands it, so a
// change to either side that silently disagreed would show up here.
func TestTheShelfUsesTheClosedFormsOwnDefaults(t *testing.T) {
	t.Parallel()
	if karsten.Target <= 0.5 || karsten.Target >= 0.99 {
		t.Errorf("the default target %v is outside the band the shelf clamps to", karsten.Target)
	}
	if curve.DefaultTargetTurn < 1 {
		t.Errorf("the default target turn is %d", curve.DefaultTargetTurn)
	}
}
