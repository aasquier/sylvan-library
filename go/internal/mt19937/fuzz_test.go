package mt19937

import (
	"math"
	"math/big"
	"testing"
)

// What fuzzing is good for here: the corpus next door proves this package
// reproduces tens of thousands of *recorded* draws, but it can only ever
// cover the seeds and bounds somebody chose. The properties below hold for
// every seed and every bound there is, so a fuzzer looking for a
// counter-example is looking somewhere the corpus cannot.
//
// They are deliberately properties this package could satisfy while still
// being wrong -- a well-made generator that agrees with the corpus on
// nothing satisfies all of them. That division is the point: exactness is
// the corpus's job, and internal consistency is this one's. What it catches
// is the class of bug the corpus is blind to by construction -- an argument
// the recorded cases happen not to reach, an overflow at a bound nobody
// recorded, a panic where the contract answers.

func FuzzTheGeneratorAgreesWithItself(f *testing.F) {
	// Seeds worth starting from: the probe's, the boundaries where the key
	// grows a word, a negative, and the two-word edge cases.
	f.Add(int64(20260810), int64(99), int64(32), int64(60))
	f.Add(int64(0), int64(1), int64(1), int64(0))
	f.Add(int64(-7), int64(2), int64(64), int64(1))
	f.Add(int64(math.MaxInt64), int64(1<<62), int64(63), int64(2))
	f.Add(int64(math.MinInt64), int64(3), int64(33), int64(250))
	f.Add(int64(4294967296), int64(257), int64(32), int64(99))

	f.Fuzz(func(t *testing.T, seed, bound, width, length int64) {
		// The fuzzer supplies whole int64s; fold them into the ranges these
		// methods are defined on rather than rejecting, so no draw is wasted.
		bound = clamp(bound, 1, 1<<62)
		k := uint(clamp(width, 0, MaxBits))
		n := int(clamp(length, 0, 512))

		// --- the same seed is the same generator, always.
		first, second := New(seed), New(seed)
		for i := range 64 {
			if a, b := first.Uint32(), second.Uint32(); a != b {
				t.Fatalf("word %d: two generators on seed %d gave %d and %d",
					i, seed, a, b)
			}
		}

		// --- the int64 constructor and the big.Int constructor agree.
		if *New(seed) != *NewFromBig(big.NewInt(seed)) {
			t.Fatalf("seed %d: New and NewFromBig built different states", seed)
		}

		// --- a seed is used by absolute value, so the sign cannot matter.
		// Written through big.Int because negating math.MinInt64 overflows,
		// which is the one seed where a hand-rolled abs() goes wrong.
		negated := new(big.Int).Neg(big.NewInt(seed))
		if *New(seed) != *NewFromBig(negated) {
			t.Fatalf("seed %d and its negation built different states", seed)
		}

		// --- GetRandBits(32) is exactly one word off the stream. This is the
		// hinge the whole corpus hangs on: it is why a recorded 32-bit draw
		// records the raw generator itself.
		if a, b := New(seed).GetRandBits(32), New(seed).Uint32(); a != uint64(b) {
			t.Fatalf("seed %d: GetRandBits(32) gave %d, Uint32 gave %d",
				seed, a, b)
		}

		// --- k bits fit in k bits, and cost the words they should.
		r := New(seed)
		for range 8 {
			v := r.GetRandBits(k)
			if k < 64 && v >= 1<<k {
				t.Fatalf("GetRandBits(%d) gave %d, which does not fit", k, v)
			}
			if k == 0 && v != 0 {
				t.Fatalf("GetRandBits(0) gave %d", v)
			}
		}

		// --- RandBelow stays below, and RandBelow(1) is the answer it has to
		// be. It is not free -- it draws until a 1-bit comes up 0 -- so it is
		// worth a property of its own: an implementation that shortcut it
		// would agree here and desynchronise every draw after it.
		r = New(seed)
		for range 16 {
			if v := r.RandBelow(bound); v < 0 || v >= bound {
				t.Fatalf("RandBelow(%d) gave %d", bound, v)
			}
		}
		if v := New(seed).RandBelow(1); v != 0 {
			t.Fatalf("RandBelow(1) gave %d", v)
		}

		// --- a stepped range stays inside itself and lands on the step.
		r = New(seed)
		step := clamp(bound%7+1, 1, 7)
		start := seed % 1000
		stop := start + clamp(bound%97+1, 1, 97)
		for range 8 {
			v := r.RandRangeStep(start, stop, step)
			if v < start || v >= stop {
				t.Fatalf("RandRangeStep(%d, %d, %d) gave %d outside the range",
					start, stop, step, v)
			}
			if (v-start)%step != 0 {
				t.Fatalf("RandRangeStep(%d, %d, %d) gave %d, which is not on "+
					"the step", start, stop, step, v)
			}
		}

		// --- a shuffle is a permutation: every card still there, once.
		deck := identity(n)
		ShuffleSlice(New(seed), deck)
		seen := make([]bool, n)
		for _, card := range deck {
			if card < 0 || card >= n {
				t.Fatalf("shuffling %d cards produced %d", n, card)
			}
			if seen[card] {
				t.Fatalf("shuffling %d cards dealt %d twice", n, card)
			}
			seen[card] = true
		}

		// --- a double from 53 bits is in [0, 1) and is one of the 2**53
		// values that can be built from them.
		r = New(seed)
		for range 16 {
			v := r.Float64()
			if !(v >= 0 && v < 1) {
				t.Fatalf("Float64 gave %v", v)
			}
			if scaled := v * 9007199254740992.0; scaled != math.Trunc(scaled) {
				t.Fatalf("Float64 gave %v, which is not a 53-bit fraction", v)
			}
		}
	})
}

// clamp folds an arbitrary int64 into [low, high] rather than rejecting it,
// so a fuzz input is never spent on a value the method under test is not
// defined for. Folded with a modulo rather than negated, because negating
// math.MinInt64 overflows back to itself -- which would quietly hand a
// negative bound to a method documented to refuse one.
func clamp(v, low, high int64) int64 {
	span := high - low + 1
	m := v % span
	if m < 0 {
		m += span
	}
	return low + m
}
