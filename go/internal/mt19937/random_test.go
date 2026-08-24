package mt19937

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The corpus is CPython's own answers, written by `tests/go_fixtures.py` from
// a real interpreter. Nothing here is a expectation somebody typed: every
// number below came out of `random.Random`, and this file's whole job is to
// say that this package's numbers are the same ones.
//
// The sections are separate on purpose. `words` is the raw generator; every
// other section reaches it through a method. A run where `words` matches and
// a method does not has a bug in random.go; a run where `words` itself is
// wrong has one in the seeding or the twist, and the rest of the failures are
// only its echo. That is the whole reason the corpus records a layer the
// application never calls.

type corpus struct {
	Seeds     []seedCase     `json:"seeds"`
	BitsSweep []bitsCase     `json:"bits_sweep"`
	FloorDiv  []floorDivCase `json:"floor_div"`
	Tier1     tier1Corpus    `json:"tier1"`
}

type floorDivCase struct {
	A    int64 `json:"a"`
	B    int64 `json:"b"`
	Want int64 `json:"want"`
}

type seedCase struct {
	Seed       string        `json:"seed"`
	Words      []uint32      `json:"words"`
	Randoms    []uint64      `json:"randoms"`
	BitsMixed  []bitsDraw    `json:"bits_mixed"`
	Below      []belowDraw   `json:"below"`
	Ranges     []rangeDraw   `json:"ranges"`
	Shuffles   []shuffleCase `json:"shuffles"`
	Repeated99 [][]int       `json:"repeated_99"`
	Choices    []int         `json:"choices"`
}

type bitsCase struct {
	Seed   string   `json:"seed"`
	K      uint     `json:"k"`
	Values []uint64 `json:"values"`
}

type bitsDraw struct {
	K     uint   `json:"k"`
	Value uint64 `json:"value"`
}

type belowDraw struct {
	N     int64 `json:"n"`
	Value int64 `json:"value"`
}

type rangeDraw struct {
	Start int64  `json:"start"`
	Stop  *int64 `json:"stop"`
	Step  *int64 `json:"step"`
	Value int64  `json:"value"`
}

type shuffleCase struct {
	N     int   `json:"n"`
	Order []int `json:"order"`
}

type tier1Corpus struct {
	ReferenceDigest string           `json:"reference_digest"`
	Seed            string           `json:"seed"`
	Games           int              `json:"games"`
	Turns           int              `json:"turns"`
	SweepCounts     []int            `json:"sweep_counts"`
	Generators      []tier1Generator `json:"generators"`
}

type tier1Generator struct {
	Seed    string      `json:"seed"`
	Lengths []int       `json:"lengths"`
	Draws   int         `json:"draws"`
	First   []belowDraw `json:"first"`
	Last    []belowDraw `json:"last"`
	Digest  string      `json:"digest"`
}

func load(t *testing.T) corpus {
	t.Helper()
	body, err := os.ReadFile("testdata/draws.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var c corpus
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatalf("parsing the corpus: %v", err)
	}
	if len(c.Seeds) == 0 || len(c.BitsSweep) == 0 || len(c.Tier1.Generators) == 0 {
		t.Fatal("the corpus is missing a section; regenerate with " +
			"`python tests/go_fixtures.py`")
	}
	return c
}

// newFromString builds a generator the way the corpus names its seed.
//
// Seeds are strings because they outgrow a JSON number, and this is also the
// only place both constructors are exercised on the same value: where the
// seed fits an int64 the two doors must agree, and where it does not, only
// one door exists.
func newFromString(t *testing.T, seed string) *Random {
	t.Helper()
	value, ok := new(big.Int).SetString(seed, 10)
	if !ok {
		t.Fatalf("the corpus names a seed that is not an integer: %q", seed)
	}
	r := NewFromBig(value)
	if value.IsInt64() {
		small := New(value.Int64())
		if *small != *r {
			t.Fatalf("seed %s: New and NewFromBig disagree about the state", seed)
		}
	}
	return r
}

// ------------------------------------------------------------ the generator

func TestTheRawWordStreamIsTheOneCPythonProduces(t *testing.T) {
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			got := make([]uint32, len(c.Words))
			for i := range got {
				got[i] = r.Uint32()
			}
			// Reported by index rather than with a whole-slice diff: the
			// first divergence is the diagnosis, and a 700-line diff buries
			// it. An index below 624 is a seeding fault, at or past it a
			// twist fault -- which is the distinction this length buys.
			for i := range got {
				if got[i] != c.Words[i] {
					t.Fatalf("word %d of %d: got %d, CPython says %d "+
						"(%s the first twist)",
						i, len(got), got[i], c.Words[i],
						map[bool]string{true: "before", false: "at or after"}[i < 624])
				}
			}
		})
	}
}

func TestRandomIsTheSameDoubleDownToTheBit(t *testing.T) {
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			for i, want := range c.Randoms {
				value := r.Float64()
				if got := math.Float64bits(value); got != want {
					t.Fatalf("draw %d: got bits %#016x (%v), CPython says "+
						"%#016x (%v)", i, got, value, want,
						math.Float64frombits(want))
				}
				if value < 0 || value >= 1 {
					t.Fatalf("draw %d: %v is outside [0, 1)", i, value)
				}
			}
		})
	}
}

func TestGetRandBitsAtEveryWidthFromOneToSixtyFour(t *testing.T) {
	for _, c := range load(t).BitsSweep {
		t.Run(fmt.Sprintf("%s/k=%d", c.Seed, c.K), func(t *testing.T) {
			r := newFromString(t, c.Seed)
			for i, want := range c.Values {
				got := r.GetRandBits(c.K)
				if got != want {
					t.Fatalf("draw %d at k=%d: got %d, CPython says %d",
						i, c.K, got, want)
				}
				if c.K < 64 && got >= 1<<c.K {
					t.Fatalf("draw %d at k=%d: %d does not fit", i, c.K, got)
				}
			}
		})
	}
}

func TestGetRandBitsThreadsItsStateBetweenWidths(t *testing.T) {
	// The sweep above asks one width of a fresh generator, so it cannot see a
	// fault in how many words a width *consumes*. This asks a single
	// generator for widths in a changing order, which can.
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			for i, want := range c.BitsMixed {
				if got := r.GetRandBits(want.K); got != want.Value {
					t.Fatalf("draw %d at k=%d: got %d, CPython says %d",
						i, want.K, got, want.Value)
				}
			}
		})
	}
}

// --------------------------------------------------------- the consumers

func TestRandBelowRejectsWhereCPythonRejects(t *testing.T) {
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			for i, want := range c.Below {
				got := r.RandRange(want.N)
				if got != want.Value {
					t.Fatalf("draw %d with n=%d: got %d, CPython says %d "+
						"(a mismatch here with the word stream intact is a "+
						"rejection-loop fault)", i, want.N, got, want.Value)
				}
			}
		})
	}
}

func TestRandRangeInEveryFormItIsCalledIn(t *testing.T) {
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			for i, want := range c.Ranges {
				var got int64
				switch {
				case want.Stop == nil:
					got = r.RandRange(want.Start)
				case want.Step == nil:
					got = r.RandRangeStep(want.Start, *want.Stop, 1)
				default:
					got = r.RandRangeStep(want.Start, *want.Stop, *want.Step)
				}
				if got != want.Value {
					t.Fatalf("draw %d of randrange(%d, %v, %v): got %d, "+
						"CPython says %d", i, want.Start,
						deref(want.Stop), deref(want.Step), got, want.Value)
				}
			}
		})
	}
}

// TestFloorDivisionIsPythons pins the one thing in this package that no
// caller can reach.
//
// `randrange`'s step count uses Python's `//`; Go's `/` truncates. Worked
// through, the two differ only where the range is empty and both then refuse
// -- so every case in the corpus above passes with either, and a truncating
// port would look correct. That reasoning is why this test exists rather than
// why it does not: "equivalent for every input that matters" is an argument,
// and an argument is checked here rather than believed.
func TestFloorDivisionIsPythons(t *testing.T) {
	cases := load(t).FloorDiv
	if len(cases) == 0 {
		t.Fatal("the corpus carries no division cases")
	}
	differ := 0
	for _, c := range cases {
		if got := floorDiv(c.A, c.B); got != c.Want {
			t.Errorf("%d // %d: got %d, Python says %d", c.A, c.B, got, c.Want)
		}
		if c.A/c.B != c.Want {
			differ++
		}
	}
	// And the corpus actually covers the disagreement. A table of cases that
	// truncation also satisfies would pass this test while pinning nothing.
	if differ == 0 {
		t.Fatal("no case in the corpus distinguishes floor division from " +
			"truncation, so this test proves nothing")
	}
}

func TestShuffleLaysTheDeckOutAsCPythonDoes(t *testing.T) {
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			for _, want := range c.Shuffles {
				r := newFromString(t, c.Seed)
				got := identity(want.N)
				ShuffleSlice(r, got)
				if diff := cmp.Diff(want.Order, got); diff != "" {
					t.Fatalf("shuffling %d cards (-CPython +go):\n%s",
						want.N, diff)
				}
			}
		})
	}
}

func TestOneGeneratorShufflesTheDeckOverAndOver(t *testing.T) {
	// Tier 1's exact shape: one generator, one shuffle per mulligan, every
	// game. A `shuffle` that consumed one draw too many would agree on the
	// first deck here and on none after it.
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			for n, want := range c.Repeated99 {
				got := identity(99)
				ShuffleSlice(r, got)
				if diff := cmp.Diff(want, got); diff != "" {
					t.Fatalf("shuffle %d of 10 (-CPython +go):\n%s", n+1, diff)
				}
			}
		})
	}
}

func TestChoicePicksWhatCPythonPicks(t *testing.T) {
	for _, c := range load(t).Seeds {
		t.Run(c.Seed, func(t *testing.T) {
			r := newFromString(t, c.Seed)
			seq := identity(7)
			for i, want := range c.Choices {
				if got := Choice(r, seq); got != want {
					t.Fatalf("choice %d: got %d, CPython says %d", i, got, want)
				}
			}
		})
	}
}

// TestANegativeSeedIsItsAbsoluteValue reads the claim off the corpus rather
// than off this package.
//
// `random_seed` runs the argument through `int.__abs__` before splitting it
// into words, so -7 and 7 are the same generator in CPython. That is asserted
// here against CPython's own two streams -- if the corpus ever showed them
// differing, this package would be wrong to make them agree.
func TestANegativeSeedIsItsAbsoluteValue(t *testing.T) {
	streams := map[string][]uint32{}
	for _, c := range load(t).Seeds {
		streams[c.Seed] = c.Words
	}
	for _, pair := range [][2]string{
		{"-1", "1"}, {"-7", "7"}, {"-20260810", "20260810"},
		{"-4294967296", "4294967296"},
	} {
		negative, ok := streams[pair[0]]
		positive, alsoOK := streams[pair[1]]
		if !ok || !alsoOK {
			t.Fatalf("the corpus no longer carries both %s and %s", pair[0], pair[1])
		}
		if diff := cmp.Diff(positive, negative); diff != "" {
			t.Fatalf("CPython gives %s and %s different streams; this package "+
				"gives them the same one (-%s +%s):\n%s",
				pair[0], pair[1], pair[1], pair[0], diff)
		}
		got := New(mustInt64(t, pair[0]))
		for i, want := range positive {
			if v := got.Uint32(); v != want {
				t.Fatalf("seed %s word %d: got %d, want %d", pair[0], i, v, want)
			}
		}
	}
}

// ------------------------------------------------------------ Tier 1

// TestTheTier1StreamIsTheOneTheDigestIsComputedOver is this package's answer
// to PLAN section 5 item 3's gate, as far as it can honestly reach today.
//
// The gate is `REFERENCE_DIGEST` reproduced byte for byte, and reproducing it
// needs the Tier 1 engine, which is Phase 5's own work and is not ported
// here. What *is* provable now is the half that was the actual risk: that the
// randomness the digest is computed over is the randomness this package
// produces.
//
// It is provable because of a measured fact about the engine. Tier 1 draws
// through exactly one call -- `rng.shuffle(deck)` in `simulate_game` -- and
// through nothing else, so a run's entire entropy budget is a sequence of
// shuffles of a known length from one seeded generator. `tests/go_fixtures.py`
// reads that sequence off a real reference run by instrumentation that
// delegates to CPython and therefore changes nothing (it re-checks
// `REFERENCE_DIGEST` while instrumented, and refuses to write a corpus if it
// moved). This replays it: same seed, same lengths, through the real
// `Shuffle`, and the 99,274 draws it makes must be the same 99,274 draws,
// in order.
//
// So a failure here says the digest cannot be reproduced, before anybody has
// written the engine that would try.
func TestTheTier1StreamIsTheOneTheDigestIsComputedOver(t *testing.T) {
	tier1 := load(t).Tier1

	if tier1.ReferenceDigest == "" || len(tier1.Generators) == 0 {
		t.Fatal("the corpus carries no reference run")
	}

	for i, g := range tier1.Generators {
		t.Run(fmt.Sprintf("generator-%d", i), func(t *testing.T) {
			r := New(mustInt64(t, g.Seed))

			var draws []belowDraw
			sum := sha256.New()
			for _, length := range g.Lengths {
				// Driven through the real `Shuffle` rather than through a
				// copy of its loop: the swap callback is handed exactly
				// `(i, _randbelow(i+1))`, so the direction of the walk and
				// where it stops are under test here, not assumed.
				r.Shuffle(length, func(i, j int) {
					draw := belowDraw{N: int64(i) + 1, Value: int64(j)}
					draws = append(draws, draw)
					fmt.Fprintf(sum, "%d:%d\n", draw.N, draw.Value)
				})
			}

			if len(draws) != g.Draws {
				t.Fatalf("made %d draws, the reference run made %d",
					len(draws), g.Draws)
			}
			// The shape beside the digest. A digest is opaque, and this
			// repository has already been bitten once by a golden that stayed
			// stable while the thing under it stopped happening.
			if diff := cmp.Diff(g.First, draws[:len(g.First)]); diff != "" {
				t.Fatalf("the first draws differ (-CPython +go):\n%s", diff)
			}
			if diff := cmp.Diff(g.Last, draws[len(draws)-len(g.Last):]); diff != "" {
				t.Fatalf("the last draws differ (-CPython +go):\n%s", diff)
			}
			if got := hex.EncodeToString(sum.Sum(nil)); got != g.Digest {
				t.Fatalf("the draw sequence digests to %s, the reference "+
					"run's to %s -- the ends agree, so the divergence is "+
					"somewhere in the middle", got, g.Digest)
			}
		})
	}
}

// TestTheReferenceRunIsTheShapeItsStreamAssumes is the shape test beside the
// stream, for the same reason `test_determinism.py` has one beside the
// digest: a corpus that quietly stopped recording anything would still be
// self-consistent and still be worthless.
func TestTheReferenceRunIsTheShapeItsStreamAssumes(t *testing.T) {
	tier1 := load(t).Tier1

	const pinned = "c3e278e3e09ae7766b145886bddf7e07314533c292b6c5aeb9340c73b3ee22d4"
	if tier1.ReferenceDigest != pinned {
		t.Fatalf("the corpus was recorded from a run digesting to %s; "+
			"tests/test_determinism.py pins %s. One of them moved, and that "+
			"is a decision rather than a detail",
			tier1.ReferenceDigest, pinned)
	}
	if tier1.Games < 100 || tier1.Turns < 5 || len(tier1.SweepCounts) < 2 {
		t.Fatalf("the reference run shrank: %d games, %d turns, %d sweep "+
			"points", tier1.Games, tier1.Turns, len(tier1.SweepCounts))
	}

	total := 0
	for _, g := range tier1.Generators {
		if g.Draws == 0 || len(g.Lengths) == 0 {
			t.Fatal("a generator in the reference run drew nothing")
		}
		for _, length := range g.Lengths {
			// A Commander library. If this ever stopped being deck-sized the
			// stream would still replay and would no longer be Tier 1's.
			if length < 50 {
				t.Fatalf("the reference run shuffled %d cards", length)
			}
		}
		total += g.Draws
	}
	if total < 10000 {
		t.Fatalf("the whole reference run drew only %d times", total)
	}
}

// ----------------------------------------------------------------- helpers

func identity(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func mustInt64(t *testing.T, seed string) int64 {
	t.Helper()
	value, ok := new(big.Int).SetString(seed, 10)
	if !ok || !value.IsInt64() {
		t.Fatalf("seed %q does not fit an int64", seed)
	}
	return value.Int64()
}

func deref(v *int64) any {
	if v == nil {
		return "None"
	}
	return *v
}
