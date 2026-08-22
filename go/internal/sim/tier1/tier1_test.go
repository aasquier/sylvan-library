package tier1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pyrand"
	"github.com/aasquier/sylvan-library/go/internal/sim"
)

// ReferenceDigest is `tests/test_determinism.py`'s REFERENCE_DIGEST, written
// out here as a literal on purpose.
//
// It is the phase gate, and a gate read out of the corpus beside it would be
// a gate that moves when the corpus is regenerated -- which is exactly the
// failure the Python side guards against by pinning it in source. So the
// number is typed twice, in two languages, and a test below holds the
// corpus's copy equal to this one: regenerating the fixture cannot quietly
// re-pin the digest, and neither can this file.
const ReferenceDigest = "c3e278e3e09ae7766b145886bddf7e07314533c292b6c5aeb9340c73b3ee22d4"

// ---------------------------------------------------------------- the corpus

// Cards, costs and sources decode straight into `sim.Card` -- the corpus is
// written by the same `_card_json` the closed forms' corpora use, so there is
// one encoding for one Go type rather than one per tier.
//
// It omits fields at their defaults, and one consequence is worth stating:
// a card whose Python category is "utility" arrives with `Category` empty.
// Tier 1 never reads the category, and `Card.Equal` still separates exactly
// the cards Python separates, because every "utility" card is empty here and
// every other category is written out.

type keepJSON struct {
	MinLands      int `json:"min_lands"`
	MaxLands      int `json:"max_lands"`
	MinManaPieces int `json:"min_mana_pieces"`
	CheapRampMV   int `json:"cheap_ramp_mv"`
	MaxMulligans  int `json:"max_mulligans"`
}

type deckJSON struct {
	Library   []sim.Card `json:"library"`
	Commander *sim.Card  `json:"commander"`
}

type corpus struct {
	Reference struct {
		Digest      string   `json:"digest"`
		Seed        string   `json:"seed"`
		Games       int      `json:"games"`
		Turns       int      `json:"turns"`
		SweepCounts []int    `json:"sweep_counts"`
		KeepRule    keepJSON `json:"keep_rule"`
		Outputs     []string `json:"outputs"`
	} `json:"reference"`
	Decks map[string]deckJSON `json:"decks"`
	Games []struct {
		Deck      string   `json:"deck"`
		Seed      string   `json:"seed"`
		Turns     int      `json:"turns"`
		KeepRule  keepJSON `json:"keep_rule"`
		OnThePlay bool     `json:"on_the_play"`
		Repr      string   `json:"repr"`
	} `json:"games"`
	Runs []struct {
		Deck    string `json:"deck"`
		Through []struct {
			Turn   int    `json:"turn"`
			Spells string `json:"spells"`
			Wasted string `json:"wasted"`
		} `json:"through"`
		Games    int      `json:"games"`
		Turns    int      `json:"turns"`
		Seed     string   `json:"seed"`
		KeepRule keepJSON `json:"keep_rule"`
		Repr     string   `json:"repr"`
	} `json:"runs"`
	Consume []struct {
		CostText  string       `json:"cost_text"`
		Cost      sim.Cost     `json:"cost"`
		Pool      string       `json:"pool"`
		Sources   []sim.Source `json:"sources"`
		CanPay    bool         `json:"can_pay"`
		Remaining []sim.Source `json:"remaining"`
	} `json:"consume"`
	Floats []struct {
		Bits uint64 `json:"bits"`
		Repr string `json:"repr"`
	} `json:"floats"`
	Strings []struct {
		Value string `json:"value"`
		Repr  string `json:"repr"`
	} `json:"strings"`
}

func load(t *testing.T) *corpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/tier1.json")
	if err != nil {
		t.Fatalf("reading the Tier 1 corpus: %v", err)
	}
	var c corpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decoding the Tier 1 corpus: %v", err)
	}
	return &c
}

// deck rebuilds one library from the corpus.
//
// Each card decodes to its own pointer where Python repeats one object per
// `qty`. That is safe and was checked rather than assumed: the only identity
// comparison in the engine is `chosen is commander`, and the commander is a
// separate object in Python too. Everything else -- `list.remove` above all
// -- compares by value, which is why sim.Card.Equal exists.
func deck(t *testing.T, c *corpus, name string) ([]*sim.Card, *sim.Card) {
	t.Helper()
	d, ok := c.Decks[name]
	if !ok {
		t.Fatalf("the corpus has no deck %q", name)
	}
	library := make([]*sim.Card, 0, len(d.Library))
	for _, card := range d.Library {
		library = append(library, &card)
	}
	var commander *sim.Card
	if d.Commander != nil {
		one := *d.Commander
		commander = &one
	}
	return library, commander
}

// toKeepRule is a conversion rather than a field-by-field copy, which means
// the compiler holds keepJSON and KeepRule to the same fields in the same
// order: a field added to one and not the other stops building here.
func toKeepRule(k keepJSON) KeepRule { return KeepRule(k) }

func seedOf(t *testing.T, text string) int64 {
	t.Helper()
	seed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		t.Fatalf("unreadable seed %q: %v", text, err)
	}
	return seed
}

// ------------------------------------------------------------------ the gate

// referenceOutputs is `determinism_probe.reference_outputs()` in Go: the same
// three calls, in the same order, against the same decks.
func referenceOutputs(t *testing.T, c *corpus) []string {
	t.Helper()
	seed := seedOf(t, c.Reference.Seed)
	library, commander := deck(t, c, "golgari-34")

	single := SimulateGame(library, commander,
		GameOptions{Turns: c.Reference.Turns, RNG: pyrand.New(seed)})
	summary := Run(library, commander, Options{
		Games: c.Reference.Games, Turns: c.Reference.Turns, Seed: &seed})

	out := []string{single.Repr(), summary.Repr()}
	swept := SweepLandCounts(
		func(n int) ([]*sim.Card, *sim.Card) {
			return deck(t, c, fmt.Sprintf("golgari-%d", n))
		},
		c.Reference.SweepCounts,
		Options{Games: c.Reference.Games / 3, Turns: c.Reference.Turns,
			Seed: &seed},
	)
	for _, point := range swept {
		out = append(out, fmt.Sprintf("%d:%s", point.Lands, point.Summary.Repr()))
	}
	return out
}

func digestOf(lines []string) string {
	sum := sha256.New()
	for _, line := range lines {
		sum.Write([]byte(line))
		sum.Write([]byte("\n"))
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// TestTheReferenceRunReproducesThePinnedDigest is Phase 5's gate.
//
// Not "the numbers are close" and not "the shapes agree": the same sha256
// CPython computes, over the same text, from a simulator written in another
// language. If this passes, a seeded run means the same games after the port
// as before it -- which is what every quoted Tier 1 number, every generated
// primer and ADR 18's cache all rest on.
func TestTheReferenceRunReproducesThePinnedDigest(t *testing.T) {
	got := digestOf(referenceOutputs(t, load(t)))
	if got != ReferenceDigest {
		t.Fatalf("REFERENCE_DIGEST is %s; this engine computes %s.\n"+
			"The dice are not the suspect -- internal/pyrand replays the "+
			"reference run's whole draw stream. Read the per-line failures "+
			"from TestTheReferenceOutputsAreCPythonsOwnText, which say which "+
			"game and which field moved.", ReferenceDigest, got)
	}
}

// TestTheReferenceOutputsAreCPythonsOwnText is the same gate with a diff.
//
// A digest is opaque, so a mismatch above says only that something moved.
// This says what: the five strings CPython produced sit in the corpus, and a
// divergence reports the line and the first character that differs.
func TestTheReferenceOutputsAreCPythonsOwnText(t *testing.T) {
	c := load(t)
	got := referenceOutputs(t, c)
	if len(got) != len(c.Reference.Outputs) {
		t.Fatalf("the reference run produced %d lines, CPython produced %d",
			len(got), len(c.Reference.Outputs))
	}
	for i, want := range c.Reference.Outputs {
		if got[i] != want {
			t.Errorf("reference line %d diverges:\n%s", i, diff(want, got[i]))
		}
	}
}

// TestTheCorpusAgreesWithThePinnedDigest stops a regenerated fixture from
// re-pinning the gate. The Python side writes the digest it measured; this
// file holds a literal; they must be the same number.
func TestTheCorpusAgreesWithThePinnedDigest(t *testing.T) {
	if got := load(t).Reference.Digest; got != ReferenceDigest {
		t.Fatalf("the corpus records %s but the pinned digest is %s -- one of "+
			"them was changed without the other", got, ReferenceDigest)
	}
}

// diff renders the first divergence between two long strings with enough
// context to read.
func diff(want, got string) string {
	i := 0
	for i < len(want) && i < len(got) && want[i] == got[i] {
		i++
	}
	from := max(0, i-60)
	return fmt.Sprintf("  first differs at byte %d\n  want ...%s\n  got  ...%s",
		i, clip(want[from:], 160), clip(got[from:], 160))
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---------------------------------------------------- the differential cases

// TestEveryGameIsCPythonsGame widens the gate past the one deck and the one
// seed it covers.
//
// The reference run is `build_golgari`, whose 99 cards all have distinct
// names -- so it never exercises `list.remove` taking out a different card
// from the one it was handed, which is what happens in every compiled deck.
// The duplicates deck here does, and so do a headless deck, a zero-turn
// horizon, the play/draw split and both branches of the mulligan bottoming
// loop.
func TestEveryGameIsCPythonsGame(t *testing.T) {
	c := load(t)
	for _, tc := range c.Games {
		name := fmt.Sprintf("%s/seed=%s/turns=%d", tc.Deck, tc.Seed, tc.Turns)
		t.Run(name, func(t *testing.T) {
			library, commander := deck(t, c, tc.Deck)
			rule := toKeepRule(tc.KeepRule)
			got := SimulateGame(library, commander, GameOptions{
				Turns:     tc.Turns,
				KeepRule:  &rule,
				RNG:       pyrand.New(seedOf(t, tc.Seed)),
				OnThePlay: tc.OnThePlay,
			})
			if got.Repr() != tc.Repr {
				t.Errorf("game diverges:\n%s", diff(tc.Repr, got.Repr()))
			}
		})
	}
}

// TestEveryRunIsCPythonsRun is the same over `run`: the accumulators, the
// medians, the timing table and its sort.
func TestEveryRunIsCPythonsRun(t *testing.T) {
	c := load(t)
	for _, tc := range c.Runs {
		name := fmt.Sprintf("%s/games=%d/turns=%d", tc.Deck, tc.Games, tc.Turns)
		t.Run(name, func(t *testing.T) {
			library, commander := deck(t, c, tc.Deck)
			rule := toKeepRule(tc.KeepRule)
			seed := seedOf(t, tc.Seed)
			got := Run(library, commander, Options{
				Games: tc.Games, Turns: tc.Turns, KeepRule: &rule, Seed: &seed})
			if got.Repr() != tc.Repr {
				t.Errorf("run diverges:\n%s", diff(tc.Repr, got.Repr()))
			}
			// `spells_through` is how a land sweep is read -- the flood-aware
			// measure CLAUDE.md says to read instead of commander speed. It
			// is a Python slice, so its bounds behave like one, and it is an
			// `fsum`, so the answer is the same on every interpreter and can
			// be pinned here at all.
			for _, th := range tc.Through {
				if s := ReprFloat(got.SpellsThrough(th.Turn)); s != th.Spells {
					t.Errorf("spells_through(%d) is %s, CPython said %s",
						th.Turn, s, th.Spells)
				}
				if w := ReprFloat(got.WastedThrough(th.Turn)); w != th.Wasted {
					t.Errorf("wasted_through(%d) is %s, CPython said %s",
						th.Turn, w, th.Wasted)
				}
			}
		})
	}
}

// TestConsumeIsCPythonsConsume holds the solver to `engine._consume` -- what
// it refuses, and exactly which units it leaves behind, in order.
//
// The leftovers are the half a "does it fit" test would miss: `_consume`
// pays generic with the least flexible units first, so the pool it returns
// decides what the *next* spell this turn can be paid with.
func TestConsumeIsCPythonsConsume(t *testing.T) {
	c := load(t)
	for _, tc := range c.Consume {
		name := fmt.Sprintf("%s/%s", tc.CostText, tc.Pool)
		t.Run(name, func(t *testing.T) {
			got, ok := consume(tc.Cost, tc.Sources)
			if ok != (tc.Remaining != nil) {
				t.Fatalf("payable=%v, CPython said %v", ok, tc.Remaining != nil)
			}
			if !ok {
				return
			}
			want := tc.Remaining
			if len(got) != len(want) {
				t.Fatalf("%d units left, CPython left %d", len(got), len(want))
			}
			for i := range want {
				if got[i].Amount != want[i].Amount ||
					!sim.SameColors(got[i].Colors, want[i].Colors) {
					t.Fatalf("unit %d is %v x%d, CPython left %v x%d", i,
						got[i].Colors, got[i].Amount,
						want[i].Colors, want[i].Amount)
				}
			}
		})
	}
}

// TestCanPayIsCPythonsCanPay is the argument in tier1.go's `canPay` comment,
// checked instead of believed.
//
// Tier 1 asks `mana.can_pay` in `_pick_land` and `engine._consume`
// everywhere else, and this port answers both through `consume`. The claim is
// that `_consume(...) is not None` and `can_pay(...)` are the same predicate;
// the corpus records CPython's answer from *both* functions, so a case where
// they part company fails here rather than showing up as a different land on
// turn three.
func TestCanPayIsCPythonsCanPay(t *testing.T) {
	c := load(t)
	agreed := 0
	for _, tc := range c.Consume {
		if tc.CanPay != (tc.Remaining != nil) {
			t.Fatalf("CPython's own can_pay and _consume disagree on %s / %s "+
				"-- the equivalence this port rests on is false",
				tc.CostText, tc.Pool)
		}
		if got := canPay(tc.Cost, tc.Sources); got != tc.CanPay {
			t.Errorf("%s / %s: canPay=%v, CPython said %v",
				tc.CostText, tc.Pool, got, tc.CanPay)
			continue
		}
		agreed++
	}
	if agreed == 0 {
		t.Fatal("no castability cases in the corpus")
	}
}

// TestConsumeAgreesWithCanPay is `tests/test_mana_properties.py`'s
// `test_consume_agrees_with_can_pay`, in Go.
//
// It exists because of what changed on 2026-08-22: `canPay` now delegates to
// `mana.CanPay`, so this package and `internal/mana` hold **two independent
// solvers** -- `consume`, which has to return the leftovers and therefore
// keeps its own matching, and `CanPay`, which only answers yes or no. Each was
// checked against Python and, until this test, **neither against the other**.
// Python pins exactly that pair with the Hypothesis property above; the corpus
// cannot, because it records what CPython answered rather than what the two Go
// functions answer about the same input.
//
// Seeded and bounded rather than a fuzz target: `internal/mana` already fuzzes
// castability, and what is wanted here is a cheap invariant that runs on every
// `go test` -- the divergence this guards against is a refactor of one solver,
// which a deterministic sweep catches on the next run rather than on the next
// fuzz budget.
func TestConsumeAgreesWithCanPay(t *testing.T) {
	// Small alphabets on purpose: a divergence between two matchings shows up
	// on tight pools where the assignment is forced, not on generous ones
	// where everything is payable.
	colours := [][]string{
		{"W"}, {"U"}, {"B"}, {"R"}, {"G"}, {"C"},
		{"G", "W"}, {"U", "B"}, {"B", "R", "G"},
		{"B", "G", "R", "U", "W"},
	}
	rng := rand.New(rand.NewSource(20260822)) //nolint:gosec // a test's own dice
	both, payable := 0, 0
	for i := 0; i < 20000; i++ {
		cost := sim.Cost{Generic: rng.Intn(4), HasX: rng.Intn(8) == 0}
		for p := rng.Intn(4); p > 0; p-- {
			cost.Pips = append(cost.Pips, colours[rng.Intn(len(colours))])
		}
		for p := rng.Intn(2); p > 0; p-- {
			cost.Phyrexian = append(cost.Phyrexian, colours[rng.Intn(len(colours))])
		}
		var pool []sim.Source
		for s := rng.Intn(6); s > 0; s-- {
			pool = append(pool, sim.Source{
				Colors: colours[rng.Intn(len(colours))],
				Amount: rng.Intn(3),
			})
		}
		_, ok := consume(cost, pool)
		if got := canPay(cost, pool); got != ok {
			t.Fatalf("the two solvers disagree on %v / %v: consume=%v canPay=%v",
				cost, pool, ok, got)
		}
		both++
		if ok {
			payable++
		}
	}
	// A sweep where nothing is payable, or everything is, proves neither
	// direction. Both counts are asserted so a narrowed generator fails here
	// rather than passing smaller.
	if payable == 0 || payable == both {
		t.Fatalf("%d of %d cases were payable; the generator has stopped "+
			"producing both answers", payable, both)
	}
}

// TestReprFloatIsCPythonsRepr holds the renderer to CPython over every float
// the reference run produces, plus the boundaries where the notation changes
// shape.
func TestReprFloatIsCPythonsRepr(t *testing.T) {
	c := load(t)
	if len(c.Floats) < 100 {
		t.Fatalf("only %d float cases; the corpus lost its sweep", len(c.Floats))
	}
	exponential, fixed := 0, 0
	for _, tc := range c.Floats {
		value := math.Float64frombits(tc.Bits)
		if got := ReprFloat(value); got != tc.Repr {
			t.Errorf("bits %#016x: repr is %q, CPython said %q",
				tc.Bits, got, tc.Repr)
		}
		if strings.ContainsRune(tc.Repr, 'e') {
			exponential++
		} else {
			fixed++
		}
	}
	// Both notations must be in the corpus, or the switch between them is
	// untested and the boundary is free to move.
	if exponential == 0 || fixed == 0 {
		t.Fatalf("the corpus holds %d exponential and %d fixed renderings; it "+
			"needs both to pin the boundary", exponential, fixed)
	}
}

// TestReprStringIsCPythonsRepr holds the string renderer to CPython: the
// quote choice, the escapes, and the printable non-ASCII it passes through.
func TestReprStringIsCPythonsRepr(t *testing.T) {
	for _, tc := range load(t).Strings {
		if got := ReprString(tc.Value); got != tc.Repr {
			t.Errorf("repr(%q) is %s, CPython said %s", tc.Value, got, tc.Repr)
		}
	}
}

// ----------------------------------------------------- the engine's own laws
//
// `tests/test_determinism.py` asserts these of the Python engine in prose
// that says why each one matters. They are asserted of this one for the same
// reasons, and none of them is implied by the digest: a digest is one run.

func TestASeededRunRepeatsExactly(t *testing.T) {
	c := load(t)
	library, commander := deck(t, c, "golgari-34")
	seed := int64(99)
	opts := Options{Games: 40, Turns: 8, Seed: &seed}
	first := Run(library, commander, opts).Repr()
	second := Run(library, commander, opts).Repr()
	if first != second {
		t.Fatal("the same seed answered differently twice in one process")
	}
}

func TestDifferentSeedsProduceDifferentResults(t *testing.T) {
	// The mirror image, and the one that catches a seed accepted and then
	// ignored -- which would make every test above pass for the wrong reason.
	c := load(t)
	library, commander := deck(t, c, "golgari-34")
	one, two := int64(1), int64(2)
	first := Run(library, commander, Options{Games: 40, Turns: 8, Seed: &one})
	second := Run(library, commander, Options{Games: 40, Turns: 8, Seed: &two})
	if first.Repr() == second.Repr() {
		t.Fatal("seeds 1 and 2 produced the same run")
	}
}

func TestARunDoesNotMutateTheLibraryItWasGiven(t *testing.T) {
	// `simulate_game` shuffles, draws and resolves land fetches. If any of
	// that reached the caller's slice, the second point of a sweep would be
	// simulating a different deck than the first.
	c := load(t)
	library, commander := deck(t, c, "golgari-34")
	before := append([]*sim.Card(nil), library...)
	seed := int64(3)
	Run(library, commander, Options{Games: 25, Turns: 8, Seed: &seed})
	if len(library) != len(before) {
		t.Fatalf("the library is %d cards, was %d", len(library), len(before))
	}
	for i := range before {
		if library[i] != before[i] {
			t.Fatalf("card %d moved: %s is now %s",
				i, before[i].Name, library[i].Name)
		}
	}
}

func TestTheProgressCallbackDoesNotChangeTheResult(t *testing.T) {
	// Instrumentation must be inert: it consumes no randomness and touches no
	// state, and this is what keeps that true.
	c := load(t)
	library, commander := deck(t, c, "golgari-34")
	seed := int64(5)
	plain := Run(library, commander, Options{Games: 200, Turns: 8, Seed: &seed})
	var seen [][2]int
	watched := Run(library, commander, Options{Games: 200, Turns: 8, Seed: &seed,
		Progress: func(done, total int) { seen = append(seen, [2]int{done, total}) }})
	if plain.Repr() != watched.Repr() {
		t.Fatal("a watched run answered differently from an unwatched one")
	}
	if len(seen) == 0 || seen[len(seen)-1] != [2]int{200, 200} {
		t.Fatalf("progress ended at %v, not (200, 200)", seen[len(seen)-1:])
	}
}

func TestEverySweepPointStartsFromTheSameSeed(t *testing.T) {
	// What makes a land sweep a comparison rather than a collection of
	// unrelated samples: each point sees the same stream of shuffles, so a
	// difference in the output is a difference in the deck.
	c := load(t)
	build := func(n int) ([]*sim.Card, *sim.Card) {
		return deck(t, c, fmt.Sprintf("golgari-%d", n))
	}
	seed := int64(13)
	opts := Options{Games: 30, Turns: 8, Seed: &seed}
	for _, point := range SweepLandCounts(build, []int{30, 34}, opts) {
		library, commander := build(point.Lands)
		if point.Summary.Repr() != Run(library, commander, opts).Repr() {
			t.Fatalf("sweep point %d is not the run it claims to be", point.Lands)
		}
	}
}

func TestTheReferenceRunIsTheShapeTheDigestAssumes(t *testing.T) {
	// A digest is opaque. If the corpus silently stopped simulating anything,
	// it would still be stable and still be worthless.
	c := load(t)
	if c.Reference.Games < 100 || c.Reference.Turns < 2 {
		t.Fatalf("the reference run is %d games over %d turns",
			c.Reference.Games, c.Reference.Turns)
	}
	if len(c.Reference.SweepCounts) < 2 {
		t.Fatalf("the sweep has %d points", len(c.Reference.SweepCounts))
	}
	library, commander := deck(t, c, "golgari-34")
	seed := seedOf(t, c.Reference.Seed)
	summary := Run(library, commander, Options{
		Games: c.Reference.Games, Turns: c.Reference.Turns, Seed: &seed})
	if summary.Games != c.Reference.Games ||
		len(summary.AvgSpellsByTurn) != c.Reference.Turns ||
		summary.MedianCommanderTurn == nil {
		t.Fatal("the reference summary is not the shape the digest assumes")
	}
}

// --------------------------------------------------------- the sharp corners

func TestRemoveTakesTheFirstEqualCard(t *testing.T) {
	// `list.remove` removes the first EQUAL element, not the one it was
	// handed. With a compiled deck those are routinely different cards, and
	// which one leaves reorders everything after it.
	forest := &sim.Card{Name: "Forest", Category: "land", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}}
	other := &sim.Card{Name: "Forest", Category: "land", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}}
	swamp := &sim.Card{Name: "Swamp", Category: "land", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"B"}, Amount: 1}}}
	if !forest.Equal(*other) {
		t.Fatal("two identically built cards do not compare equal")
	}
	hand := []*sim.Card{forest, swamp, other}
	got := removeFirstEqual(hand, other)
	if len(got) != 2 || got[0] != swamp || got[1] != other {
		t.Fatalf("removing the last Forest took out the wrong card: %v", names(got))
	}
}

func TestKeepRuleCountsCheapRampAsAManaPiece(t *testing.T) {
	rule := DefaultKeepRule()
	land := &sim.Card{Name: "Forest", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}}
	rock := &sim.Card{Name: "Signet", Cost: sim.Cost{Generic: 2},
		Produces: []sim.Source{{Colors: []string{"B", "G"}, Amount: 1}}}
	dear := &sim.Card{Name: "Gilded Lotus", Cost: sim.Cost{Generic: 5},
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 3}}}
	spell := &sim.Card{Name: "Spell", Cost: sim.Cost{Generic: 3}}
	if !rule.Keeps([]*sim.Card{land, land, rock, spell, spell, spell, spell}) {
		t.Error("two lands and a two-mana rock is a keep")
	}
	if rule.Keeps([]*sim.Card{land, land, dear, spell, spell, spell, spell}) {
		t.Error("a five-mana rock is not cheap ramp")
	}
	if rule.Keeps([]*sim.Card{land, spell, spell, spell, spell, spell, spell}) {
		t.Error("one land is under the floor")
	}
	if want := "keep 2-5 lands AND lands + ramp(mv<=2) >= 3"; rule.Describe() != want {
		t.Errorf("describe() is %q, want %q", rule.Describe(), want)
	}
}

func TestMedianOfAnOddCountIsAnInt(t *testing.T) {
	// `statistics.median` returns `data[n//2]` for an odd count, so
	// `median_commander_turn` renders as `5` and not `5.0`. The annotation
	// says `float | None`; the annotation is wrong and the digest is not.
	odd := medianInt([]int{3, 9, 5})
	if odd.IsFloat || odd.Int != 5 {
		t.Errorf("median of three ints is %+v, want the int 5", odd)
	}
	even := medianInt([]int{3, 9, 5, 1})
	if !even.IsFloat || even.Float != 4 {
		t.Errorf("median of four ints is %+v, want the float 4.0", even)
	}
	if reprOptNumber(&odd) != "5" || reprOptNumber(&even) != "4.0" {
		t.Errorf("rendered as %s and %s", reprOptNumber(&odd), reprOptNumber(&even))
	}
}

func TestAnUnseededRunIsStillARun(t *testing.T) {
	// `seed=None` is a legal call and must not panic looking for entropy.
	c := load(t)
	library, commander := deck(t, c, "duplicates")
	if got := Run(library, commander, Options{Games: 5, Turns: 4}); got.Games != 5 {
		t.Fatalf("an unseeded run reported %d games", got.Games)
	}
}

func names(cards []*sim.Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.Name
	}
	return out
}

// TestNumberSurvivesJSONBothWays pins the marshalling `Number` did not have
// until the sim family flipped.
//
// The type is held to CPython by `repr` text and by `Float64bits`, and neither
// route goes through `encoding/json` -- so for as long as nothing serialised
// one, the default marshaller applied and `median_commander_turn` would have
// reached the browser as `{"IsFloat":false,"Int":4,"Float":0}`. Every corpus
// was green. The contract suite caught it on the first run through the door.
//
// Both directions are pinned because the ADR 18 cache needs the round trip: a
// stored result is decoded into the struct it was encoded from, and a number
// that went in an int has to come back one -- `repr(4)` and `repr(4.0)` are
// different text, and that text is inside the determinism digest.
func TestNumberSurvivesJSONBothWays(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Number
		want string
	}{
		{"an odd count is an int", Number{Int: 4}, "4"},
		{"an even count is a float", Number{IsFloat: true, Float: 4.5}, "4.5"},
		{"a whole float keeps its point", Number{IsFloat: true, Float: 4.0}, "4.0"},
		{"zero", Number{Int: 0}, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(raw) != tc.want {
				t.Fatalf("marshalled %s, want %s", raw, tc.want)
			}
			var back Number
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back != tc.in {
				t.Fatalf("round trip gave %+v, want %+v", back, tc.in)
			}
		})
	}

	// And inside a struct, which is how it actually travels -- a pointer
	// field, nil when no game cast the commander.
	type holder struct {
		N *Number `json:"median_commander_turn"`
	}
	raw, err := json.Marshal(holder{})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"median_commander_turn":null}` {
		t.Fatalf("a nil Number rendered as %s", raw)
	}
}
