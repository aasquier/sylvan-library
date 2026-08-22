package pyfloat_test

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
)

// The floor the two closed forms stand on, held to CPython by
// `testdata/pyfloat.json` (written by `python tests/go_fixtures.py`).
//
// # The epsilon, pinned per function
//
// PLAN section 5 item 4 asks for "an epsilon pinned per function". All three
// here are pinned at **zero -- bit equality** -- and the justification is the
// same one in three parts: each of these functions is a *reproduction* of a
// CPython algorithm rather than an approximation of a mathematical value, so
// there is no error term to allow for. `Fsum` runs Shewchuk's partials in the
// same order CPython runs them; `Round` answers an **integer**, so it has no
// epsilon to pin at all; `RoundTo` does in exact rationals what CPython does
// by formatting and re-parsing, and both are correctly rounded to the same
// definition. A non-zero epsilon here would not absorb a rounding difference,
// it would hide a transcription error -- which is the only way any of them can
// be wrong.
const (
	epsilonFsum    = 0.0
	epsilonRoundTo = 0.0
)

type pyfloatCorpus struct {
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

func loadPyfloat(t *testing.T) pyfloatCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/pyfloat.json")
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	var corpus pyfloatCorpus
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

func TestFsumIsCPythonsFsum(t *testing.T) {
	corpus := loadPyfloat(t)
	if len(corpus.Fsum) < 20 {
		t.Fatalf("the corpus has shrunk to %d sequences", len(corpus.Fsum))
	}
	for i, c := range corpus.Fsum {
		got := pyfloat.Fsum(c.Values)
		if !same(got, c.Value, epsilonFsum) {
			t.Errorf("case %d (%d terms): Fsum = %v (%#016x), CPython = %v (%#016x)",
				i, len(c.Values), got, math.Float64bits(got),
				c.Value, math.Float64bits(c.Value))
		}
	}
}

func TestFsumBeatsANaiveSumOnAtLeastOneCase(t *testing.T) {
	// A corpus that a running total would also pass proves nothing about the
	// algorithm, only about the arithmetic. This asserts the corpus is
	// discriminating -- the lesson recorded as "a probe that cannot fail
	// differently is not a probe".
	corpus := loadPyfloat(t)
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

func TestRoundBreaksTiesToEven(t *testing.T) {
	corpus := loadPyfloat(t)
	for _, row := range corpus.Round {
		if got := pyfloat.Round(row.X); got != row.Value {
			t.Errorf("Round(%v) = %d, CPython = %d", row.X, got, row.Value)
		}
	}
	// The one that matters, spelled out: Go's own rounding disagrees, and it
	// disagrees by a whole land in `RegressionLands`.
	if int(math.Round(34.5)) == pyfloat.Round(34.5) {
		t.Fatal("math.Round and CPython agree about 34.5, so this guard is vacuous")
	}
	if pyfloat.Round(34.5) != 34 {
		t.Errorf("Round(34.5) = %d, want 34 (ties to even)", pyfloat.Round(34.5))
	}
}

func TestRoundToIsCPythonsTwoArgumentRound(t *testing.T) {
	corpus := loadPyfloat(t)
	if len(corpus.RoundTo) < 50 {
		t.Fatalf("the corpus has shrunk to %d cases", len(corpus.RoundTo))
	}
	for _, row := range corpus.RoundTo {
		if got := pyfloat.RoundTo(row.X, row.Ndigits); !same(got, row.Value, epsilonRoundTo) {
			t.Errorf("RoundTo(%v, %d) = %v (%#016x), CPython = %v (%#016x)",
				row.X, row.Ndigits, got, math.Float64bits(got),
				row.Value, math.Float64bits(row.Value))
		}
	}
	// The textbook case, because it is the one everybody expects to be wrong:
	// 2.675 is really 2.67499999999999982..., so rounding it to two places
	// gives 2.67 rather than the 2.68 the decimal literal suggests.
	if got := pyfloat.RoundTo(2.675, 2); got != 2.67 {
		t.Errorf("RoundTo(2.675, 2) = %v, want 2.67", got)
	}
}

// fusedProbe has the exact shape of every accumulation in `karsten` and
// `curve`: a running total, a product, and the `pyfloat.Rounded` guard between
// them.
func fusedProbe(xs, ys []float64, start float64) float64 {
	total := start
	for i := range xs {
		total += pyfloat.Rounded(xs[i] * ys[i])
	}
	return total
}

// fusedSubProbe is the other shape, from `RegressionLands`: a running value
// with a product subtracted, which arm64 fuses to FMSUB.
func fusedSubProbe(a, b, start float64) float64 {
	return start - pyfloat.Rounded(a*b)
}

func TestNoFusedMultiplyAddSurvivesTheGuard(t *testing.T) {
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
			"pyfloat.Rounded is not stopping the compiler on this architecture",
			got, math.Float64bits(got))
	}
	if got := fusedSubProbe(a, b, 1.0); got != 0.0 {
		t.Errorf("the subtraction fused: got %v (%#016x), want 0", got, math.Float64bits(got))
	}
	t.Logf("FMA would have answered %v; the guarded accumulation answered 0", fused)
}
