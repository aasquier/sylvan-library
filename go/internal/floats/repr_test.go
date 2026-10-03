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
// all.
//
// **Those two properties cannot see the presentation at all**, which mutation
// testing is how anybody found out: `gremlins unleash ./internal/floats/` read
// six LIVED mutants in the four lines that choose between fixed and
// exponential, and four more in branches nothing in this package reached. The
// reason is structural rather than an oversight — every mutation of the
// switch at `repr.go:59` still produces a decimal that parses back to the same
// float64, because `1e+15` and `1000000000000000.0` *are* the same number.
// Round-tripping is a property of the digits; the boundaries are a property of
// the *spelling*, and only a spelling pins a spelling.
//
// So the table below is the spellings `Repr`'s own doc comment names, and the
// doc comment is the source of truth it is checked against — not the corpus,
// which has no `repr` section, and not a run of the code, which would record
// whatever it currently does. Five of the eight strings appear verbatim in
// that comment; the rest are the same four rules applied one case further.

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

func TestReprRendersTheSpellingsItsOwnContractNames(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		v    float64
		want string
		rule string
	}{
		// The three non-finite answers, which `reprSample` cannot hold
		// because every value in it has to round-trip through ParseFloat.
		{math.NaN(), "nan", "not a number"},
		{math.Inf(1), "inf", "positive infinity"},
		{math.Inf(-1), "-inf", "negative infinity"},

		// "the switch is at a decimal exponent below -3 or above 16, so
		// `1e16` reads `1e+16` while `1e15` reads `1000000000000000.0`, and
		// `0.0001` stays fixed while `0.00001` becomes `1e-05`" — all four
		// boundary cases, verbatim from the doc comment.
		{1e16, "1e+16", "above 16: exponential"},
		{1e15, "1000000000000000.0", "16 exactly: still fixed"},
		{1e-4, "0.0001", "-3 exactly: still fixed"},
		{1e-5, "1e-05", "below -3: exponential, exponent padded to two digits"},

		// "A fixed rendering always carries a decimal point (`100.0`, never
		// `100`)" — and the same rule where the point lands immediately after
		// the digits rather than after padding zeros.
		{100, "100.0", "fixed, padded with zeros"},
		{25, "25.0", "fixed, the point exactly at the end of the digits"},
		{0.5, "0.5", "fixed, the point exactly at the start of the digits"},
		{2.5, "2.5", "fixed, the point inside the digits"},

		// Zero carries its sign, which is the one thing `big.Rat` could not
		// represent in `RoundTo` either.
		{0, "0.0", "zero"},
		{math.Copysign(0, -1), "-0.0", "negative zero"},

		// "an exponential one never gains a spurious `.0`" — a single digit
		// stays single — and the exponent is at least two digits but not
		// trimmed to two.
		{1e100, "1e+100", "exponential, three exponent digits"},
		{-1.5e-7, "-1.5e-07", "exponential, signed, with a fraction"},
	} {
		if got := floats.Repr(c.v); got != c.want {
			t.Errorf("Repr(%v) = %q, the contract says %q (%s)", c.v, got, c.want, c.rule)
		}
	}
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
