package curve_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/curve"
)

// `sim/curve.py`, held to Python by `testdata/curve.json` (written by
// `python tests/go_fixtures.py`) and to its own documented findings by the
// traps ported from `tests/test_curve.py`.
//
// # The epsilon, pinned per function
//
// As in the sibling package, each is pinned at **zero -- bit equality** rather
// than at a tolerance, and each says why separately so that a future drift
// names the function that drifted. What is different here is *where the risk
// lives*: `karsten` divides exact integers and this module accumulates floats,
// so every one of these is exact because the accumulation order was matched
// and the fused multiply-add was guarded, not because there is nothing to get
// wrong.
const (
	// expectedLandsInPlay: a running total of `capped * Exactly(...)`, summed
	// in ascending k exactly as Python's generator yields it. Exact, and the
	// one place `sim.Rounded` is doing visible work -- without it arm64 fuses
	// the multiply into the add and answers one ulp differently.
	epsilonExpectedLands = 0.0

	// expectedRamp: a running total over the accelerants in deck order. Exact
	// for the same two reasons, plus one more: `min(1.0, seen/deck)` is
	// Python's `min`, which returns its second operand only when it is
	// strictly smaller.
	epsilonExpectedRamp = 0.0

	// landDistribution / rampDistribution: the distributions the answer is
	// convolved from. The ramp one is a dynamic program whose two writes per
	// cell arrive in a fixed order -- the `p` term from a lower index first,
	// then the `1-p` term -- and reordering them is a last-bit difference in
	// a probability that then gets summed a hundred times.
	epsilonLandDistribution = 0.0
	epsilonRampDistribution = 0.0

	// onCurveOdds: the headline. Exact, and wanted exact because
	// `slotsToTarget` scans it against `>= target` and `Curve`'s advice
	// branches on `abs(perLand - perRamp) < TooClose` -- so one ulp is a
	// different slot count or a different recommendation, which is the whole
	// output of the feature.
	epsilonOnCurveOdds = 0.0

	// curve: the assembled answer, including the four odds rounded to four
	// places for the wire. Pinned separately because the rounding is applied
	// to the *reported* figures while the decisions are made on the unrounded
	// ones, and a port that rounded before deciding would agree on every
	// number in `turns` and disagree about what to do.
	epsilonCurve = 0.0
)

// -------------------------------------------------------------- the corpus

type deckCase struct {
	Name      string     `json:"name"`
	Why       string     `json:"why"`
	Library   []sim.Card `json:"library"`
	Commander *sim.Card  `json:"commander"`
}

type turnCase struct {
	Turn         int     `json:"turn"`
	FromLands    float64 `json:"from_lands"`
	FromRamp     float64 `json:"from_ramp"`
	ExpectedMana float64 `json:"expected_mana"`
	LandDropOdds float64 `json:"land_drop_odds"`
	Odds         float64 `json:"odds"`
}

type adviceCase struct {
	TargetTurn        int     `json:"target_turn"`
	TargetMana        int     `json:"target_mana"`
	Target            float64 `json:"target"`
	Odds              float64 `json:"odds"`
	OddsPerLand       float64 `json:"odds_per_land"`
	OddsPerRamp       float64 `json:"odds_per_ramp"`
	Recommend         string  `json:"recommend"`
	Slots             *int    `json:"slots"`
	RampIsGeneric     bool    `json:"ramp_is_generic"`
	BeyondTheCurve    bool    `json:"beyond_the_curve"`
	LandsForEveryDrop *int    `json:"lands_for_every_drop"`
}

type manaCurveCase struct {
	DeckSize    int        `json:"deck_size"`
	Lands       int        `json:"lands"`
	Accelerants int        `json:"accelerants"`
	TargetTurn  int        `json:"target_turn"`
	TargetMana  int        `json:"target_mana"`
	Target      float64    `json:"target"`
	OnThePlay   bool       `json:"on_the_play"`
	Turns       []turnCase `json:"turns"`
	Advice      adviceCase `json:"advice"`
}

type curveCorpus struct {
	Horizon           int        `json:"horizon"`
	DefaultTargetTurn int        `json:"default_target_turn"`
	DefaultTarget     float64    `json:"default_target"`
	TooClose          float64    `json:"too_close"`
	GenericRock       []int      `json:"generic_rock"`
	Decks             []deckCase `json:"decks"`
	Accelerants       []struct {
		Deck  string  `json:"deck"`
		Value [][]int `json:"value"`
	} `json:"accelerants"`
	ExpectedLands    [][]any `json:"expected_lands"`
	LandDistribution []struct {
		DeckSize  int       `json:"deck_size"`
		Lands     int       `json:"lands"`
		Turn      int       `json:"turn"`
		OnThePlay bool      `json:"on_the_play"`
		Value     []float64 `json:"value"`
	} `json:"land_distribution"`
	ExpectedRamp []struct {
		Deck      string    `json:"deck"`
		OnThePlay bool      `json:"on_the_play"`
		ByTurn    []float64 `json:"by_turn"`
	} `json:"expected_ramp"`
	RampDistribution []struct {
		Deck       string    `json:"deck"`
		Turn       int       `json:"turn"`
		OnThePlay  bool      `json:"on_the_play"`
		Extra      []int     `json:"extra"`
		ExtraCount int       `json:"extra_count"`
		Value      []float64 `json:"value"`
	} `json:"ramp_distribution"`
	OnCurveOdds       [][]any `json:"on_curve_odds"`
	LandsForEveryDrop [][]any `json:"lands_for_every_drop"`
	TypicalAccelerant []struct {
		Deck    string `json:"deck"`
		Turn    int    `json:"turn"`
		Piece   []int  `json:"piece"`
		Generic bool   `json:"generic"`
	} `json:"typical_accelerant"`
	Curves []struct {
		Deck       string        `json:"deck"`
		TargetTurn int           `json:"target_turn"`
		TargetMana *int          `json:"target_mana"`
		Target     float64       `json:"target"`
		OnThePlay  bool          `json:"on_the_play"`
		Value      manaCurveCase `json:"value"`
	} `json:"curves"`
}

func load(t *testing.T) (curveCorpus, map[string]deckCase) {
	t.Helper()
	raw, err := os.ReadFile("testdata/curve.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var corpus curveCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the corpus: %v", err)
	}
	byName := map[string]deckCase{}
	for _, d := range corpus.Decks {
		byName[d.Name] = d
	}
	return corpus, byName
}

func asInt(v any) int       { return int(v.(float64)) }
func asFloat(v any) float64 { return v.(float64) }
func asBool(v any) bool     { return v.(bool) }

func exact(got, want, epsilon float64) bool {
	if epsilon == 0 {
		return math.Float64bits(got) == math.Float64bits(want)
	}
	return math.Abs(got-want) <= epsilon
}

func exactFloats(epsilon float64) cmp.Option {
	return cmp.Comparer(func(a, b float64) bool { return exact(a, b, epsilon) })
}

func exactSlice(t *testing.T, label string, got, want []float64, epsilon float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d buckets, Python has %d", label, len(got), len(want))
		return
	}
	for i := range want {
		if !exact(got[i], want[i], epsilon) {
			t.Errorf("%s[%d] = %v (%#016x), Python = %v (%#016x)",
				label, i, got[i], math.Float64bits(got[i]),
				want[i], math.Float64bits(want[i]))
		}
	}
}

func pieceOf(row []int) *curve.Piece {
	if row == nil {
		return nil
	}
	return &curve.Piece{Cost: row[0], Output: row[1], Delay: row[2]}
}

// -------------------------------------------------- differentially, per function

func TestTheConstantsAreStillPythons(t *testing.T) {
	corpus, _ := load(t)
	if corpus.Horizon != curve.Horizon || corpus.DefaultTargetTurn != curve.DefaultTargetTurn ||
		corpus.DefaultTarget != curve.DefaultTarget || corpus.TooClose != curve.TooClose {
		t.Errorf("constants drifted: Python has horizon %d, turn %d, target %v, too-close %v",
			corpus.Horizon, corpus.DefaultTargetTurn, corpus.DefaultTarget, corpus.TooClose)
	}
	rock := curve.GenericRock()
	if corpus.GenericRock[0] != rock.Cost || corpus.GenericRock[1] != rock.Output ||
		corpus.GenericRock[2] != rock.Delay {
		t.Errorf("the stand-in accelerant is %v, Python's is %v", rock, corpus.GenericRock)
	}
}

func TestAccelerantsMatchPython(t *testing.T) {
	corpus, decks := load(t)
	for _, row := range corpus.Accelerants {
		got := curve.Accelerants(decks[row.Deck].Library)
		if len(got) != len(row.Value) {
			t.Errorf("%s: %d accelerants, Python found %d", row.Deck, len(got), len(row.Value))
			continue
		}
		for i, want := range row.Value {
			if got[i].Cost != want[0] || got[i].Output != want[1] || got[i].Delay != want[2] {
				t.Errorf("%s[%d] = %v, Python = %v", row.Deck, i, got[i], want)
			}
		}
	}
}

func TestExpectedLandsInPlayMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	if len(corpus.ExpectedLands) < 300 {
		t.Fatalf("the grid has shrunk to %d rows", len(corpus.ExpectedLands))
	}
	for _, row := range corpus.ExpectedLands {
		deckSize, lands, turn := asInt(row[0]), asInt(row[1]), asInt(row[2])
		otp, want := asBool(row[3]), asFloat(row[4])
		got := curve.ExpectedLandsInPlay(deckSize, lands, turn, otp)
		if !exact(got, want, epsilonExpectedLands) {
			t.Errorf("ExpectedLandsInPlay(%d, %d, %d, %v) = %v (%#016x), Python = %v (%#016x)",
				deckSize, lands, turn, otp, got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}
}

func TestExpectedRampMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	for _, row := range corpus.ExpectedRamp {
		for i, want := range row.ByTurn {
			got := curve.ExpectedRamp(decks[row.Deck].Library, i+1, row.OnThePlay)
			if !exact(got, want, epsilonExpectedRamp) {
				t.Errorf("ExpectedRamp(%s, turn %d, on_the_play=%v) = %v, Python = %v",
					row.Deck, i+1, row.OnThePlay, got, want)
			}
		}
	}
}

func TestLandDistributionMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	for _, row := range corpus.LandDistribution {
		got := curve.LandDistribution(row.DeckSize, row.Lands, row.Turn, row.OnThePlay)
		exactSlice(t, "LandDistribution", got, row.Value, epsilonLandDistribution)
	}
}

func TestRampDistributionMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	for _, row := range corpus.RampDistribution {
		got := curve.RampDistribution(decks[row.Deck].Library, row.Turn, row.OnThePlay,
			pieceOf(row.Extra), row.ExtraCount)
		exactSlice(t, row.Deck, got, row.Value, epsilonRampDistribution)
	}
}

func TestOnCurveOddsMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	if len(corpus.OnCurveOdds) < 3000 {
		t.Fatalf("the grid has shrunk to %d rows", len(corpus.OnCurveOdds))
	}
	nullNeeds := 0
	for _, row := range corpus.OnCurveOdds {
		deck, turn := row[0].(string), asInt(row[1])
		var need *int
		if row[2] == nil {
			nullNeeds++
		} else {
			n := asInt(row[2])
			need = &n
		}
		otp, extraLands := asBool(row[3]), asInt(row[4])
		var ramp *curve.Piece
		if row[5] != nil {
			raw := row[5].([]any)
			ramp = &curve.Piece{Cost: asInt(raw[0]), Output: asInt(raw[1]), Delay: asInt(raw[2])}
		}
		extraCount, want := asInt(row[6]), asFloat(row[7])
		got := curve.OnCurveOdds(decks[deck].Library, turn, need, otp,
			&curve.Extra{Lands: extraLands, Ramp: ramp, RampCount: extraCount})
		if !exact(got, want, epsilonOnCurveOdds) {
			t.Errorf("OnCurveOdds(%s, turn %d, need %v, otp %v, +%d lands, ramp %v x%d) = "+
				"%v (%#016x), Python = %v (%#016x)",
				deck, turn, row[2], otp, extraLands, ramp, extraCount,
				got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}
	if nullNeeds == 0 {
		t.Fatal("no row asks the `need=None` question, which is the default the surface uses")
	}
}

func TestLandsForEveryDropMatchesPython(t *testing.T) {
	corpus, _ := load(t)
	nils := 0
	for _, row := range corpus.LandsForEveryDrop {
		deckSize, turn := asInt(row[0]), asInt(row[1])
		target, otp := asFloat(row[2]), asBool(row[3])
		got := curve.LandsForEveryDrop(deckSize, turn, target, otp)
		if row[4] == nil {
			nils++
			if got != nil {
				t.Errorf("LandsForEveryDrop(%d, %d, %v, %v) = %d, Python found none",
					deckSize, turn, target, otp, *got)
			}
			continue
		}
		want := asInt(row[4])
		if got == nil || *got != want {
			t.Errorf("LandsForEveryDrop(%d, %d, %v, %v) = %v, Python = %d",
				deckSize, turn, target, otp, got, want)
		}
	}
	if nils == 0 {
		t.Fatal("no row reaches the unreachable case, which is a real answer")
	}
}

func TestTypicalAccelerantMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	for _, row := range corpus.TypicalAccelerant {
		piece, generic := curve.TypicalAccelerant(decks[row.Deck].Library, row.Turn)
		if generic != row.Generic {
			t.Errorf("%s turn %d: generic = %v, Python = %v", row.Deck, row.Turn, generic, row.Generic)
		}
		if piece.Cost != row.Piece[0] || piece.Output != row.Piece[1] || piece.Delay != row.Piece[2] {
			t.Errorf("%s turn %d: piece = %v, Python = %v", row.Deck, row.Turn, piece, row.Piece)
		}
	}
}

func TestTheWholeCurveMatchesPython(t *testing.T) {
	corpus, decks := load(t)
	if len(corpus.Curves) < 50 {
		t.Fatalf("the curve set has shrunk to %d cases", len(corpus.Curves))
	}
	for _, row := range corpus.Curves {
		got := project(curve.Curve(decks[row.Deck].Library, curve.Options{
			TargetTurn: row.TargetTurn,
			TargetMana: row.TargetMana,
			Target:     row.Target,
			OnTheDraw:  !row.OnThePlay,
		}))
		if diff := cmp.Diff(row.Value, got, exactFloats(epsilonCurve)); diff != "" {
			t.Errorf("Curve(%s, turn %d, mana %v, target %v, otp %v) differs (-python +go):\n%s",
				row.Deck, row.TargetTurn, row.TargetMana, row.Target, row.OnThePlay, diff)
		}
	}
}

func project(mc curve.ManaCurve) manaCurveCase {
	out := manaCurveCase{
		DeckSize: mc.DeckSize, Lands: mc.Lands, Accelerants: mc.Accelerants,
		TargetTurn: mc.TargetTurn, TargetMana: mc.TargetMana, Target: mc.Target,
		OnThePlay: mc.OnThePlay, Turns: []turnCase{},
		Advice: adviceCase{
			TargetTurn: mc.Advice.TargetTurn, TargetMana: mc.Advice.TargetMana,
			Target: mc.Advice.Target, Odds: mc.Advice.Odds,
			OddsPerLand: mc.Advice.OddsPerLand, OddsPerRamp: mc.Advice.OddsPerRamp,
			Recommend: mc.Advice.Recommend, Slots: mc.Advice.Slots,
			RampIsGeneric:     mc.Advice.RampIsGeneric,
			BeyondTheCurve:    mc.Advice.BeyondTheCurve,
			LandsForEveryDrop: mc.Advice.LandsForEveryDrop,
		},
	}
	for _, turn := range mc.Turns {
		out.Turns = append(out.Turns, turnCase{
			Turn: turn.Turn, FromLands: turn.FromLands, FromRamp: turn.FromRamp,
			ExpectedMana: turn.ExpectedMana(), LandDropOdds: turn.LandDropOdds,
			Odds: turn.Odds,
		})
	}
	return out
}

// ------------------------------------------------- the pinned Python traps

func TestTheLandDistributionSumsToOneAndRespectsTheCap(t *testing.T) {
	// You may play one land a turn, so turn 4 tops out at four in play. The
	// cap living inside the distribution is what stops a nine-land hand on
	// turn four being counted as nine mana -- the flooding case the whole
	// formula exists to price honestly.
	dist := curve.LandDistribution(99, 36, 4, true)
	if len(dist) != 5 {
		t.Fatalf("buckets are 0..turn, got %d", len(dist))
	}
	total := 0.0
	for _, v := range dist {
		total += v
	}
	if math.Abs(total-1.0) > 1e-9 {
		t.Errorf("the distribution sums to %v", total)
	}
	if dist[4] <= 0.3 {
		t.Errorf("four-or-more is the fat bucket at 36 lands, got %v", dist[4])
	}
}

func TestADeckWithNoAccelerantsHasAllItsRampMassAtZero(t *testing.T) {
	_, decks := load(t)
	dist := curve.RampDistribution(decks["mono-green"].Library, 4, true, nil, 0)
	if math.Abs(dist[0]-1.0) > 1e-9 {
		t.Errorf("a deck of bears has ramp mass %v at zero", dist[0])
	}
}

func TestARockTooExpensiveForTheTurnIsNotCounted(t *testing.T) {
	// A six-mana rock is not ramp for turn four.
	_, decks := load(t)
	library := append([]sim.Card{}, decks["mono-green"].Library...)
	library = append(library, sim.Card{
		Name: "Big", Cost: sim.Cost{Generic: 6},
		Produces: []sim.Source{{Colors: []string{"C"}, Amount: 1}},
	})
	dist := curve.RampDistribution(library, 4, true, nil, 0)
	tail := 0.0
	for _, v := range dist[1:] {
		tail += v
	}
	if math.Abs(tail) > 1e-9 {
		t.Errorf("a six-drop contributed %v to turn four", tail)
	}
}

func TestSummoningSicknessDelaysAManaCreature(t *testing.T) {
	// A dork cast on turn one pays on turn two, so its odds at a given turn
	// are the odds of having drawn it a turn earlier.
	_, decks := load(t)
	library := append([]sim.Card{}, decks["mono-green"].Library...)
	library = append(library, sim.Card{
		Name: "Elf", Cost: sim.Cost{Pips: [][]string{{"G"}}},
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}, ProduceDelay: 1,
	})
	early := curve.RampDistribution(library, 1, true, nil, 0)
	tail := 0.0
	for _, v := range early[1:] {
		tail += v
	}
	if math.Abs(tail) > 1e-9 {
		t.Errorf("a dork is sick on the turn it lands, but contributed %v", tail)
	}
	later := curve.RampDistribution(library, 2, true, nil, 0)
	tail = 0.0
	for _, v := range later[1:] {
		tail += v
	}
	if tail <= 0 {
		t.Error("a dork cast on turn one pays on turn two")
	}
}

func TestALandFetchSpellIsNotCappedByTheLandDrop(t *testing.T) {
	// Cultivate has no `Produces` at all and is still acceleration. Omitting
	// it was a -0.54 mana bias across the whole formula, and the error only
	// showed up as a pattern: Esper decks accurate, every green deck low. It
	// puts lands onto the battlefield, so it adds on top of the cap -- turn 4
	// caps lands at 4, so anything above 4 must come from the fetch.
	_, decks := load(t)
	plain := decks["mono-green"].Library
	if got := curve.ExpectedRamp(plain, 5, true); got != 0.0 {
		t.Errorf("a deck of bears ramps by %v", got)
	}
	fetched := append([]sim.Card{}, plain[:98]...)
	fetched = append(fetched, sim.Card{
		Name: "Skyshroud Claim", Cost: sim.Cost{Generic: 3, Pips: [][]string{{"G"}}},
		FetchesLands: 2,
	})
	if curve.ExpectedRamp(fetched, 5, true) <= 0.0 {
		t.Error("a land fetcher is ramp")
	}
	need := 6
	if curve.OnCurveOdds(fetched, 4, &need, true, nil) <= 0.0 {
		t.Error("a fetcher can put a deck past the one-land-a-turn cap")
	}
}

func TestALandIsWorthNothingPastTheTurnItIsPlayedOn(t *testing.T) {
	// The rule the whole feature turns on. You may play one land a turn, so on
	// turn four no number of lands gets you to five mana. This is why "T mana
	// on turn T" could never recommend ramp, and why the surface asks how much
	// mana rather than assuming.
	lands := make([]sim.Card, 0, 99)
	for i := 0; i < 60; i++ {
		lands = append(lands, sim.Card{
			Name: "Forest", IsLand: true,
			Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}},
		})
	}
	for i := 0; i < 39; i++ {
		lands = append(lands, sim.Card{Name: "Bear", Cost: sim.Cost{Generic: 1, Pips: [][]string{{"G"}}}})
	}
	four, five := 4, 5
	if got := curve.OnCurveOdds(lands, 4, &four, true, nil); got <= 0.5 {
		t.Errorf("sixty lands make four mana on turn four only %v of the time", got)
	}
	if got := curve.OnCurveOdds(lands, 4, &five, true, nil); math.Abs(got) > 1e-9 {
		t.Errorf("sixty lands and still no fifth mana on turn four, got %v", got)
	}
}

func TestAtTheCurveALandIsAtLeastAsGoodAsAnAccelerantAndPastItIsNot(t *testing.T) {
	// Measured, and it is the finding that reshaped this module: six decks by
	// five target turns, and a land was ahead or level in all thirty. Past the
	// curve the comparison reverses outright, which is why `recommend ==
	// "ramp"` is not dead code.
	_, decks := load(t)
	library := decks["esper-rocks"].Library
	piece, _ := curve.TypicalAccelerant(library, 4)

	at := 4
	perLand := curve.OnCurveOdds(library, 4, &at, true, &curve.Extra{Lands: 1})
	perRamp := curve.OnCurveOdds(library, 4, &at, true, &curve.Extra{Ramp: &piece, RampCount: 1})
	if perLand < perRamp-curve.TooClose {
		t.Errorf("at the curve a land must be ahead or level: %v vs %v", perLand, perRamp)
	}

	past := 6
	perLand = curve.OnCurveOdds(library, 4, &past, true, &curve.Extra{Lands: 1})
	perRamp = curve.OnCurveOdds(library, 4, &past, true, &curve.Extra{Ramp: &piece, RampCount: 1})
	if perRamp <= perLand {
		t.Errorf("past the curve an accelerant must win outright: %v vs %v", perRamp, perLand)
	}
}

func TestALandDropEveryTurnIsUnaffordableAndSaysSo(t *testing.T) {
	// The answer to the question as originally asked. Fifty-four lands to make
	// every drop through turn four at 90%. Pinned because it is the number the
	// feature exists to talk somebody *out* of, and a regression that made it
	// look reasonable would be the feature quietly reversing its own advice.
	four := curve.LandsForEveryDrop(99, 4, 0.90, true)
	three := curve.LandsForEveryDrop(99, 3, 0.90, true)
	five := curve.LandsForEveryDrop(99, 5, 0.90, true)
	if four == nil || *four != 54 {
		t.Errorf("through turn four = %v, want 54", four)
	}
	if three == nil || *three != 48 {
		t.Errorf("through turn three = %v, want 48", three)
	}
	if five == nil || four == nil || *five <= *four {
		t.Error("and it only gets worse, which is the shape that makes it hopeless")
	}
}

func TestAnImpossibleDropRequirementReturnsNil(t *testing.T) {
	// Nil means no land count reaches it, not "a big number". Harder to
	// provoke than it looks: 86 lands really does make every drop through turn
	// ten at 99.9%, because you see 6+T cards for T drops. The genuinely
	// unreachable case is a deck with fewer cards than the turn asks for.
	if got := curve.LandsForEveryDrop(5, 10, 0.90, true); got != nil {
		t.Errorf("a five-card deck makes ten drops with %d lands", *got)
	}
}

func TestLandDropOddsFallAsTheTurnsGoOn(t *testing.T) {
	// More turns means more drops to have made, and the requirement outruns
	// the draws. A curve that rose would mean the cap was being ignored.
	_, decks := load(t)
	mc := curve.Curve(decks["mono-green"].Library, curve.DefaultOptions())
	for i := 1; i < len(mc.Turns); i++ {
		if mc.Turns[i].LandDropOdds > mc.Turns[i-1].LandDropOdds {
			t.Errorf("turn %d is easier than turn %d: %v > %v",
				i+1, i, mc.Turns[i].LandDropOdds, mc.Turns[i-1].LandDropOdds)
		}
	}
}

func TestTheTargetTurnIsClampedToTheHorizon(t *testing.T) {
	_, decks := load(t)
	library := decks["mono-green"].Library
	if got := curve.Curve(library, curve.Options{TargetTurn: 99, Target: 0.9}).TargetTurn; got != curve.Horizon {
		t.Errorf("target turn 99 came back as %d", got)
	}
	if got := curve.Curve(library, curve.Options{TargetTurn: 0, Target: 0.9}).TargetTurn; got != 1 {
		t.Errorf("target turn 0 came back as %d", got)
	}
}

func TestAnEmptyLibraryDoesNotPanic(t *testing.T) {
	mc := curve.Curve(nil, curve.DefaultOptions())
	if mc.DeckSize != 0 || mc.Advice.Odds != 0.0 {
		t.Errorf("an empty library gave %d cards and odds %v", mc.DeckSize, mc.Advice.Odds)
	}
}

func TestSlotsIsASearchNotADivision(t *testing.T) {
	// Odds are not linear in slots, so the count must be found by trying.
	// Dividing a shortfall by a marginal rate assumes the tenth land buys what
	// the first did.
	_, decks := load(t)
	library := decks["mono-green-poor"].Library
	mc := curve.Curve(library, curve.Options{TargetTurn: 4, Target: 0.85})
	if mc.Advice.Slots == nil {
		t.Skip("this deck does not reach the target by adding one kind of card")
	}
	slots := *mc.Advice.Slots
	kind := mc.Advice.Recommend
	if kind != "lands" {
		t.Skipf("the advice recommends %q here, so the land search is not what was run", kind)
	}
	need := 4
	reached := curve.OnCurveOdds(library, 4, &need, true, &curve.Extra{Lands: slots})
	if reached < 0.85 {
		t.Errorf("%d slots reach only %v", slots, reached)
	}
	if slots > 1 {
		oneFewer := curve.OnCurveOdds(library, 4, &need, true, &curve.Extra{Lands: slots - 1})
		if oneFewer >= 0.85 {
			t.Errorf("slots must be the smallest count that works: %d already reaches %v",
				slots-1, oneFewer)
		}
	}
}

func TestADeckWithNoRampSaysItsComparisonIsAStandIn(t *testing.T) {
	// Advice built on a hypothetical Signet has to admit that it is.
	_, decks := load(t)
	six := 6
	mc := curve.Curve(decks["mono-green"].Library, curve.Options{
		TargetTurn: 4, TargetMana: &six, Target: 0.90,
	})
	if !mc.Advice.RampIsGeneric {
		t.Error("a deck with no accelerants must say its comparison is a stand-in")
	}
	piece, generic := curve.TypicalAccelerant(decks["mono-green"].Library, 4)
	if !generic || piece != curve.GenericRock() {
		t.Errorf("the stand-in is %v (generic %v), want %v", piece, generic, curve.GenericRock())
	}
	if !mc.Advice.BeyondTheCurve {
		t.Error("six mana on turn four is past the curve")
	}
}
