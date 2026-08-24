package floats_test

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

// `repr.go` is the half of this kernel its own package never tested. Measured
// 2026-08-24: `Repr`, `MarshalJSON` and `UnmarshalJSON` all read 0.0% under
// `go test -cover ./internal/floats/`, and `UnmarshalJSON` reads 0.0% across
// the **whole module** — no test anywhere called it. The other two are reached
// at 96.4% and 100% transitively, through `wire` and the simulator, which is
// why the gap never showed as a failure: a kernel can be exercised everywhere
// and pinned nowhere.
//
// `Repr`'s own doc says its boundaries are "held by the frozen corpus rather
// than trusted from this comment", and `testdata/corpus.json` carries `fsum`,
// `round` and `round_to` — and no `repr`. That corpus is frozen (CLAUDE.md:
// never regenerated), so the missing section is not this test's to write.
//
// What is written here instead are the two properties `repr.go` states about
// itself, each derived rather than restated: a rendering that round-trips is
// the definition of "the shortest decimal that round-trips", and a `Float`
// that survives a wire is the reason [floats.Float.UnmarshalJSON] exists at
// all. Neither can be satisfied by a table copied out of the comment, which
// is the failure mode a boundary table would have had.

// reprSample is spread deliberately across the two renderings and both sides
// of each switch `Repr` documents, so a change that breaks one form cannot
// hide behind the other.
func reprSample() []float64 {
	out := []float64{
		0, math.Copysign(0, -1), 1, -1, 0.5, 100, 1e15, 1e16, 1e-4, 1e-5,
		math.MaxFloat64, math.SmallestNonzeroFloat64, -math.MaxFloat64,
		math.Pi, 1.0 / 3.0, 2.0 / 3.0, 0.1, 0.2, 0.1 + 0.2,
	}
	// And a deterministic sweep of the exponent range, so the sample is not
	// only the values somebody thought of.
	for e := -300; e <= 300; e += 7 {
		out = append(out, math.Ldexp(1.0, e), -math.Ldexp(1.7, e))
	}
	return out
}

func TestEveryFiniteRenderingParsesBackToTheSameFloat(t *testing.T) {
	t.Parallel()
	for _, v := range reprSample() {
		s := floats.Repr(v)
		back, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Errorf("Repr(%v) = %q, which does not parse: %v", v, s, err)
			continue
		}
		// Bits, not `==`: negative zero is a different answer from zero and
		// they compare equal.
		if math.Float64bits(back) != math.Float64bits(v) {
			t.Errorf("Repr(%v (%#016x)) = %q, which parses back to %v (%#016x)",
				v, math.Float64bits(v), s, back, math.Float64bits(back))
		}
	}
}

func TestAFloatSurvivesTheWireItWasDeclaredFor(t *testing.T) {
	t.Parallel()
	for _, v := range reprSample() {
		raw, err := json.Marshal(floats.Float(v))
		if err != nil {
			t.Errorf("marshalling %v: %v", v, err)
			continue
		}
		// The declared type's whole purpose: the wire spelling is Repr's, not
		// encoding/json's, so `4.0` does not arrive as `4`.
		if string(raw) != floats.Repr(v) {
			t.Errorf("Float(%v) marshalled as %s, Repr says %s", v, raw, floats.Repr(v))
		}
		var back floats.Float
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Errorf("unmarshalling %s: %v", raw, err)
			continue
		}
		if math.Float64bits(float64(back)) != math.Float64bits(v) {
			t.Errorf("%v (%#016x) round-tripped to %v (%#016x)",
				v, math.Float64bits(v), float64(back), math.Float64bits(float64(back)))
		}
	}
}

// A wire carries whitespace, and the decoder trims it before parsing. Pinned
// because the trim is the one line of [floats.Float.UnmarshalJSON] that is not
// `strconv`, and because a malformed number must come back as an error rather
// than as zero — a silent zero in a payload of numbers is the worst answer
// available.
func TestTheDecoderTrimsAndRefuses(t *testing.T) {
	t.Parallel()
	var f floats.Float
	if err := f.UnmarshalJSON([]byte("  2.5\n")); err != nil {
		t.Errorf("padded number refused: %v", err)
	} else if f != 2.5 {
		t.Errorf("padded number read as %v", f)
	}
	f = 99
	if err := f.UnmarshalJSON([]byte(`"2.5"`)); err == nil {
		t.Error("a quoted number was accepted")
	} else if f != 99 {
		t.Errorf("a refused decode still wrote %v", f)
	}
}
