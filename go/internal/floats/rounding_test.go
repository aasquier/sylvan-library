package floats_test

// The two edges of [floats.RoundTo]: the values it declines to touch, and the
// half-way case.
//
// Both are contract rather than taste. A rounded number here is a stored
// number -- the average mana value on a `sim shelf`, the four odds in the
// curve's advice -- compared later against an unrounded one, so a drift of
// one place in the last digit is a recommendation that changes without
// anybody editing anything. The half-way rule is banker's, to even, which is
// the rule the frozen corpus was recorded under.

import (
	"math"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

func TestRoundToHandsBackWhatItCannotRound(t *testing.T) {
	t.Parallel()
	// Infinity has no decimal places and NaN has no value; a negative number
	// of places is a caller's mistake. All three come back untouched rather
	// than becoming zero, which is the answer that would quietly look real.
	if got := floats.RoundTo(math.Inf(1), 2); !math.IsInf(got, 1) {
		t.Fatalf("+Inf rounded to %v", got)
	}
	if got := floats.RoundTo(math.Inf(-1), 2); !math.IsInf(got, -1) {
		t.Fatalf("-Inf rounded to %v", got)
	}
	if got := floats.RoundTo(math.NaN(), 2); !math.IsNaN(got) {
		t.Fatalf("NaN rounded to %v", got)
	}
	if got := floats.RoundTo(1.2345, -1); got != 1.2345 {
		t.Fatalf("a negative place count rounded to %v", got)
	}
}

func TestRoundToBreaksAHalfWayTieToTheEvenDigit(t *testing.T) {
	t.Parallel()
	// Each input is exact in binary, so the tie is a real tie rather than a
	// value that merely prints as one: 0.375 is 3/8 and 0.125 is 1/8.
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"an odd last digit rounds up", 0.375, 0.38},
		{"an even last digit stays", 0.125, 0.12},
		{"the same, negative", -0.375, -0.38},
		{"and the even one, negative", -0.125, -0.12},
		{"a whole half to even", 2.5, 2.0},
		{"and the odd whole half away", 3.5, 4.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			places := 2
			if tc.in == math.Trunc(tc.in)+0.5 || tc.in == math.Trunc(tc.in)-0.5 {
				places = 0
			}
			if got := floats.RoundTo(tc.in, places); got != tc.want {
				t.Fatalf("RoundTo(%v, %d) = %v, want %v", tc.in, places, got, tc.want)
			}
		})
	}
}
