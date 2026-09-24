package mt19937

// What the generator does when it is asked for something that does not exist.
//
// Every one of these is a panic, and the panics are the design rather than an
// oversight: handed a zero bound the bare rejection loop would *hang* --
// `GetRandBits(0)` is 0 and `0 >= 0` never stops being true -- and a hang in
// a seeded deal is a page that never arrives with nothing in the log. A panic
// names the caller. This file is what says the panics are still there, and
// that the two boundaries next to them (zero bits, and a seed wider than the
// state) answer rather than refuse.

import (
	"math/big"
	"strings"
	"testing"
)

// refuses runs f and returns the panic's text, failing when it did not panic.
func refuses(t *testing.T, what string, f func()) string {
	t.Helper()
	var said string
	func() {
		defer func() {
			if r := recover(); r != nil {
				said = r.(string)
			}
		}()
		f()
	}()
	if said == "" {
		t.Fatalf("%s answered instead of refusing", what)
	}
	return said
}

func TestAnEmptyRangeIsRefusedRatherThanHungOver(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		call func(*Random)
		says string
	}{
		{"a zero bound", func(r *Random) { r.RandBelow(0) }, "RandBelow needs a positive bound"},
		{"a negative bound", func(r *Random) { r.RandBelow(-5) }, "RandBelow needs a positive bound"},
		{"a zero stop", func(r *Random) { r.RandRange(0) }, "RandRange needs a positive stop"},
		{"a range of one step that is empty", func(r *Random) { r.RandRangeStep(5, 5, 1) }, "empty range"},
		{"a range that runs backwards", func(r *Random) { r.RandRangeStep(5, 1, 1) }, "empty range"},
		{"a step of zero", func(r *Random) { r.RandRangeStep(0, 10, 0) }, "zero step"},
		{"a step that never reaches the stop", func(r *Random) { r.RandRangeStep(0, 10, -2) }, "empty range"},
		{"an empty sequence", func(r *Random) { r.ChoiceIndex(0) }, "ChoiceIndex on an empty sequence"},
		{"more bits than a word holds", func(r *Random) { r.GetRandBits(65) }, "more than 64 bits"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := New(7)
			got := refuses(t, tc.name, func() { tc.call(r) })
			if !strings.Contains(got, tc.says) {
				t.Fatalf("refused with %q, which does not say %q", got, tc.says)
			}
		})
	}
}

func TestZeroBitsAnswersZeroWithoutDrawingAWord(t *testing.T) {
	t.Parallel()
	// The recorded behaviour, and the reason RandBelow must never be handed a
	// zero: asking for no bits consumes no randomness, so the stream is
	// exactly where it was.
	r, untouched := New(7), New(7)
	if got := r.GetRandBits(0); got != 0 {
		t.Fatalf("GetRandBits(0) = %d", got)
	}
	if got, want := r.Uint32(), untouched.Uint32(); got != want {
		t.Fatalf("the stream moved: %d, want %d", got, want)
	}
}

func TestASeedWiderThanTheStateIsMixedInWhole(t *testing.T) {
	t.Parallel()
	// The state is 624 words; `init_by_array` runs for as many rounds as the
	// key is long when the key is longer than that. Nothing in the app seeds
	// this wide -- the tarot deal mints 31 bits -- but a seed arrives from a
	// URL, and a URL can carry any integer somebody types.
	wide := new(big.Int).Lsh(big.NewInt(1), 20_000)
	wide.Add(wide, big.NewInt(12345))
	a, b := NewFromBig(wide), NewFromBig(wide)
	if a.Uint32() != b.Uint32() {
		t.Fatal("the same wide seed dealt two different streams")
	}
	// And the last word of the key is not ignored: a seed differing only far
	// above the state's width deals a different stream.
	other := new(big.Int).Lsh(big.NewInt(1), 20_000)
	other.Add(other, big.NewInt(12346))
	if NewFromBig(other).Uint32() == NewFromBig(wide).Uint32() {
		t.Fatal("two wide seeds one apart dealt the same stream")
	}
}
