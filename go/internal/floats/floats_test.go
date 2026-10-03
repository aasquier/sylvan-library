package floats_test

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

// The floor the two closed forms stand on, held to `testdata/corpus.json` --
// a frozen recorded golden, never regenerated.
//
// # The epsilon, pinned per function
//
// Every comparison here is pinned at an epsilon, and all three are pinned at
// **zero -- bit equality** -- with the same justification in three parts:
// each of these functions is a *reproduction* of a recorded algorithm rather
// than an approximation of a mathematical value, so there is no error term
// to allow for. `Fsum` runs Shewchuk's partials in a fixed order; `Round`
// answers an **integer**, so it has no epsilon to pin at all; `RoundTo` does
// in exact rationals what the recorded contract defines by decimal
// formatting and re-parsing, and both are correctly rounded to the same
// definition. A non-zero epsilon here would not absorb a rounding difference,
// it would hide a transcription error -- which is the only way any of them can
// be wrong.
const (
	epsilonFsum    = 0.0
	epsilonRoundTo = 0.0
)

type floatsCorpus struct {
	Note string `json:"note"`
	Fsum []struct {
		Values []float64 `json:"values"`
		Value  float64   `json:"value"`
	} `json:"fsum"`
	Round []struct {
		X     float64 `json:"x"`
		Value int     `json:"value"`
	} `json:"round"`
	RoundTo []struct {
		X       float64 `json:"x"`
		Ndigits int     `json:"ndigits"`
		Value   float64 `json:"value"`
	} `json:"round_to"`
}

func loadCorpus(t *testing.T) floatsCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/corpus.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var corpus floatsCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the corpus: %v", err)
	}
	return corpus
}

// same compares to a pinned epsilon, where zero means the bits must match.
// Bits rather than `==` so that a positive and a negative zero are told apart:
// they compare equal and they are not the same answer.
func same(got, want, epsilon float64) bool {
	if epsilon == 0 {
		return math.Float64bits(got) == math.Float64bits(want)
	}
	return math.Abs(got-want) <= epsilon
}

func TestFsumMatchesTheRecordedSums(t *testing.T) {
	t.Parallel()
	corpus := loadCorpus(t)
	if len(corpus.Fsum) < 20 {
		t.Fatalf("the corpus has shrunk to %d sequences", len(corpus.Fsum))
	}
	for i, c := range corpus.Fsum {
		got := floats.Fsum(c.Values)
		if !same(got, c.Value, epsilonFsum) {
			t.Errorf("case %d (%d terms): Fsum = %v (%#016x), the corpus says %v (%#016x)",
				i, len(c.Values), got, math.Float64bits(got),
				c.Value, math.Float64bits(c.Value))
		}
	}
}

func TestFsumBeatsANaiveSumOnAtLeastOneCase(t *testing.T) {
	t.Parallel()
	// A corpus that a running total would also pass proves nothing about the
	// algorithm, only about the arithmetic. This asserts the corpus is
	// discriminating -- the lesson recorded as "a probe that cannot fail
	// differently is not a probe".
	corpus := loadCorpus(t)
	differs := 0
	for _, c := range corpus.Fsum {
		naive := 0.0
		for _, v := range c.Values {
			naive += v
		}
		if math.Float64bits(naive) != math.Float64bits(c.Value) {
			differs++
		}
	}
	if differs == 0 {
		t.Fatal("no sequence in the corpus separates fsum from a running total")
	}
	t.Logf("%d of %d sequences separate fsum from a running total", differs, len(corpus.Fsum))
}

// The corpus pins what `Fsum` answers; it does not pin *how it finishes*, and
// mutation testing is how that came out. `gremlins unleash ./internal/floats/`
// read 76.32% efficacy, and three of the eighteen survivors were in the five
// lines that turn the partials back into one float: `hi = x + y` in the
// top-down loop can be written `hi = x - y`, `lo = y - (hi - x)` can be written
// `y + (hi - x)`, and every recorded sequence still passes. The lines are
// *covered* — they run on nearly every call. They are not pinned, because in
// each recorded sequence the smaller partial sits far below the larger's
// half-ulp, where `x + y` and `x - y` both round to `x`.
//
// The corpus cannot be asked for the missing cases: it is frozen and never
// regenerated. An independent oracle can be. A sum of float64s is a rational,
// `big.Float` at a precision no such sum can exceed accumulates it exactly, and
// `Float64` rounds that once to the nearest float64 with ties to even — which
// is the contract `Fsum`'s own doc claims ("the answer is the nearest float64
// to the true sum"). So every expectation below is derived from the definition
// rather than recorded from a run, and `testdata/corpus.json` is untouched.

// oraclePrec holds any sum of a handful of float64s exactly. A float64's set
// bits live between 2^-1074 and 2^1023, so 2151 bits span the widest pair with
// no rounding at all; the rest is room for the carries.
const oraclePrec = 4096

func fsumOracle(values []float64) float64 {
	acc := new(big.Float).SetPrec(oraclePrec)
	for _, v := range values {
		acc.Add(acc, new(big.Float).SetPrec(oraclePrec).SetFloat64(v))
	}
	out, _ := acc.Float64()
	return out
}

// dyadicGrid is ±{1, 1.5}·2^e over eight exponents chosen to straddle the
// places the accumulation turns: 2^53 is where consecutive integers stop being
// representable, 2^54 and 2^55 are where a 1 falls below half an ulp (so the
// correction at the end of `Fsum` decides the last bit), 2^41 and 2^51 let a
// triple leave a residue several binades down, and 2^-1 and 2^2 let the
// smallest term be a fraction. The halves matter as much as the powers: a
// mantissa of 1.5 is what makes a sum land exactly on a midpoint and so makes
// ties-to-even observable.
func dyadicGrid() []float64 {
	var out []float64
	for _, e := range []int{-1, 2, 10, 41, 51, 53, 54, 55} {
		for _, m := range []float64{1, 1.5} {
			for _, s := range []float64{1, -1} {
				out = append(out, s*m*math.Ldexp(1, e))
			}
		}
	}
	return out
}

// Exhaustive rather than sampled, because the cases that distinguish a
// correctly-rounded sum from a nearly-correct one are a thin scatter: of the
// 32,768 triples below, 350 tell `x + y` from `x - y` and 80 tell the sign test
// in the half-even correction from its negation. A seeded sample of a few
// hundred would miss every one and read green.
func TestFsumIsTheCorrectlyRoundedSumOfEveryDyadicTriple(t *testing.T) {
	t.Parallel()
	grid := dyadicGrid()
	bad := 0
	for _, a := range grid {
		for _, b := range grid {
			for _, c := range grid {
				values := []float64{a, b, c}
				got, want := floats.Fsum(values), fsumOracle(values)
				if math.Float64bits(got) != math.Float64bits(want) {
					bad++
					if bad <= 10 {
						t.Errorf("Fsum(%v) = %v (%#016x), exactly summed and rounded once it is %v (%#016x)",
							values, got, math.Float64bits(got), want, math.Float64bits(want))
					}
				}
			}
		}
	}
	t.Logf("%d triples over a grid of %d, %d disagreements", len(grid)*len(grid)*len(grid), len(grid), bad)
}

// The three sentences `Fsum`'s doc comment ends on, each as a sequence that
// fails if the sentence stops being true. Every expectation here is an exact
// float64 written as the arithmetic that defines it — no case needs a value
// nobody can derive by hand, which is the property that keeps this out of the
// frozen corpus's territory.
func TestFsumFinishesTheAccumulationTheWayItsDocDescribes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		values []float64
		want   float64
	}{
		{
			// "the final accumulation stops at the first inexact addition":
			// the top-down loop runs, adds exactly, and the sum of the two
			// partials is the answer. 1 - 2^54 + 1.5·2^53 is 1 - 2^52, which
			// is under 2^53 and so exactly a float64; a running total loses
			// the 1 when it meets 2^54 and answers -2^52.
			name:   "an exact addition inside the top-down loop",
			values: []float64{1, -math.Ldexp(1, 54), 1.5 * math.Ldexp(1, 53)},
			want:   1 - math.Ldexp(1, 52),
		},
		{
			// The same loop iterating twice, which is the only shape that
			// tells `lo = y - (hi - x)` from `y + (hi - x)`. The exact sum is
			// 0.375 - 2^50, whose set bits run from 2^49 down to 2^-3 —
			// exactly 53, so it is a float64 and not a rounding.
			name:   "two exact additions, so the residual is carried twice",
			values: []float64{0.375, -1.5 * math.Ldexp(1, 51), -math.Ldexp(1, 52), 1.5 * math.Ldexp(1, 52)},
			want:   0.375 - math.Ldexp(1, 50),
		},
		{
			// "then corrects for half-even rounding across partials", which is
			// the one case the correction exists for. 2^53 + 1 + 2^-53 is a
			// hair above the midpoint between 2^53 and 2^53+2, so it rounds
			// up; without the correction the answer is 2^53, one ulp under,
			// and that is also what a running total says (2^53 + 1 ties to
			// even, back to 2^53, and 2^-53 then vanishes).
			name:   "the half-even correction across partials",
			values: []float64{math.Ldexp(1, 53), 1, math.Ldexp(1, -53)},
			want:   math.Ldexp(1, 53) + 2,
		},
	} {
		if got := floats.Fsum(c.values); !same(got, c.want, epsilonFsum) {
			t.Errorf("%s: Fsum(%v) = %v (%#016x), the arithmetic says %v (%#016x)",
				c.name, c.values, got, math.Float64bits(got), c.want, math.Float64bits(c.want))
		}
		// Each case must also be worth having: a sequence a running total
		// gets right proves nothing about the algorithm, which is the lesson
		// TestFsumBeatsANaiveSumOnAtLeastOneCase records for the corpus.
		naive := 0.0
		for _, v := range c.values {
			naive += v
		}
		if math.Float64bits(naive) == math.Float64bits(c.want) {
			t.Errorf("%s: a running total also answers %v, so the case is vacuous", c.name, naive)
		}
		// And the oracle must agree with the hand derivation, so a typo in
		// either one is caught by the other.
		if oracle := fsumOracle(c.values); math.Float64bits(oracle) != math.Float64bits(c.want) {
			t.Errorf("%s: the exact sum rounds to %v (%#016x), the comment derives %v (%#016x)",
				c.name, oracle, math.Float64bits(oracle), c.want, math.Float64bits(c.want))
		}
	}
}

func TestRoundBreaksTiesToEven(t *testing.T) {
	t.Parallel()
	corpus := loadCorpus(t)
	for _, row := range corpus.Round {
		if got := floats.Round(row.X); got != row.Value {
			t.Errorf("Round(%v) = %d, the corpus says %d", row.X, got, row.Value)
		}
	}
	// The one that matters, spelled out: Go's own rounding disagrees, and it
	// disagrees by a whole land in `RegressionLands`.
	if int(math.Round(34.5)) == floats.Round(34.5) {
		t.Fatal("math.Round and floats.Round agree about 34.5, so this guard is vacuous")
	}
	if floats.Round(34.5) != 34 {
		t.Errorf("Round(34.5) = %d, want 34 (ties to even)", floats.Round(34.5))
	}
}

func TestRoundToAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadCorpus(t)
	if len(corpus.RoundTo) < 50 {
		t.Fatalf("the corpus has shrunk to %d cases", len(corpus.RoundTo))
	}
	for _, row := range corpus.RoundTo {
		if got := floats.RoundTo(row.X, row.Ndigits); !same(got, row.Value, epsilonRoundTo) {
			t.Errorf("RoundTo(%v, %d) = %v (%#016x), the corpus says %v (%#016x)",
				row.X, row.Ndigits, got, math.Float64bits(got),
				row.Value, math.Float64bits(row.Value))
		}
	}
	// The textbook case, because it is the one everybody expects to be wrong:
	// 2.675 is really 2.67499999999999982..., so rounding it to two places
	// gives 2.67 rather than the 2.68 the decimal literal suggests.
	if got := floats.RoundTo(2.675, 2); got != 2.67 {
		t.Errorf("RoundTo(2.675, 2) = %v, want 2.67", got)
	}
}

// fusedProbe has the exact shape of every accumulation in `karsten` and
// `curve`: a running total, a product, and the `floats.Rounded` guard between
// them.
func fusedProbe(xs, ys []float64, start float64) float64 {
	total := start
	for i := range xs {
		total += floats.Rounded(xs[i] * ys[i])
	}
	return total
}

// fusedSubProbe is the other shape, from `RegressionLands`: a running value
// with a product subtracted, which arm64 fuses to FMSUB.
func fusedSubProbe(a, b, start float64) float64 {
	return start - floats.Rounded(a*b)
}

func TestNoFusedMultiplyAddSurvivesTheGuard(t *testing.T) {
	t.Parallel()
	// (1 + 2^-27) * (1 - 2^-27) is exactly 1 - 2^-54, which is precisely
	// halfway between 1 - 2^-53 and 1.0 and so rounds, ties to even, to 1.0.
	// An FMA keeps the exact product and the low half survives into the sum;
	// a rounded multiply throws it away. So starting from -1.0 the two
	// answers are 0 and -2^-54, which is the sharpest a single term can be.
	a := 1.0 + math.Ldexp(1, -27)
	b := 1.0 - math.Ldexp(1, -27)

	fused := math.FMA(a, b, -1.0)
	if fused == 0.0 {
		t.Fatal("the probe cannot tell the two apart; pick a sharper pair")
	}
	if got := fusedProbe([]float64{a}, []float64{b}, -1.0); got != 0.0 {
		t.Errorf("the accumulation fused: got %v (%#016x), want 0 -- "+
			"floats.Rounded is not stopping the compiler on this architecture",
			got, math.Float64bits(got))
	}
	if got := fusedSubProbe(a, b, 1.0); got != 0.0 {
		t.Errorf("the subtraction fused: got %v (%#016x), want 0", got, math.Float64bits(got))
	}
	t.Logf("FMA would have answered %v; the guarded accumulation answered 0", fused)
}
