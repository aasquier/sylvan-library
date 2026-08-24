package mt19937

import "math/bits"

// The consumer half: everything that only consumes the stream mt19937.go
// produces. The split matters because these are the methods with *choices* in
// them -- a rejection loop, a direction to iterate, an arithmetic on the
// bounds -- and each choice is a place two implementations can both be
// reasonable and disagree.
//
// Nothing in this file touches the generator's state directly; randomness is
// reached only through `Float64`, `GetRandBits` and `RandBelow`. That is why
// a corpus failure here with the word stream intact points at this file.
//
// The refusals are panics rather than errors, the way `math/rand` panics on
// a non-positive bound rather than returning one it cannot honour. Every
// argument reaching these is a deck size, a card count or a literal; a caller
// that can produce an empty range has a bug upstream of here, and an error
// return would invite it to be ignored.

// RandBelow is a random int in [0, n), by rejection over `GetRandBits`.
//
// The rejection loop is the whole of it, and it is not an optimisation --
// where it rejects is part of the stream. `k` is `bitLen(n)`, not
// `bitLen(n-1)`, so for n a power of two the loop draws one extra bit
// and rejects half the time; for n one *past* a power of two it almost never
// rejects. Both are the recorded discipline and both are visible in the
// corpus.
//
// n == 1 is the case worth naming: it always returns 0, and it is never free.
// `GetRandBits(1)` is drawn and rejected until it comes up 0, so a
// `RandRange(1)` consumes an unbounded number of words to tell you the only
// answer there was. `Shuffle` never reaches it -- its smallest bound is 2 --
// but `RandRange` does.
//
// It panics for n <= 0. The bare loop, handed a zero, would hang instead:
// `GetRandBits(0)` is 0 and `0 >= 0` never stops being true. A hang is a
// worse answer than a panic.
func (r *Random) RandBelow(n int64) int64 {
	if n <= 0 {
		panic("mt19937: RandBelow needs a positive bound")
	}
	// Compared in the unsigned domain, which is the one `GetRandBits` answers
	// in; the two conversions are exact because n is positive and every value
	// that leaves the loop is below it.
	bound := uint64(n) //nolint:gosec // positive, by the guard above
	k := uint(bits.Len64(bound))
	v := r.GetRandBits(k)
	for v >= bound {
		v = r.GetRandBits(k)
	}
	return int64(v) //nolint:gosec // v < bound <= math.MaxInt64
}

// RandRange is a random int in [0, stop).
//
// The one-argument form is the only one with a caller in the served package
// -- the tarot deal's `RandRange(2**31)` when it has to invent a seed, and
// the Wheel's three -- and it answers by handing `stop` straight to
// `RandBelow`, with no arithmetic in between.
func (r *Random) RandRange(stop int64) int64 {
	if stop <= 0 {
		panic("mt19937: RandRange needs a positive stop")
	}
	return r.RandBelow(stop)
}

// RandRangeStep is a random element of the sequence start, start+step, ...
// short of stop.
//
// Two things are reproduced rather than rederived. The unit-step case is a
// separate branch and reaches `RandBelow(width)` directly, which matters
// because the general branch would compute the same n by a different
// route and there is no reason to trust that the rounding agrees. And the
// count uses **floor** division, not Go's truncation -- see
// floorDiv. The two disagree only for arguments that describe an empty range,
// where both then refuse, but "they only differ where it does not matter" is
// an argument that stops being true the day somebody widens the bounds.
func (r *Random) RandRangeStep(start, stop, step int64) int64 {
	width := stop - start

	if step == 1 {
		if width > 0 {
			return start + r.RandBelow(width)
		}
		panic("mt19937: empty range in RandRangeStep")
	}

	var count int64
	switch {
	case step > 0:
		count = floorDiv(width+step-1, step)
	case step < 0:
		count = floorDiv(width+step+1, step)
	default:
		panic("mt19937: zero step in RandRangeStep")
	}
	if count <= 0 {
		panic("mt19937: empty range in RandRangeStep")
	}
	return start + step*r.RandBelow(count)
}

// floorDiv is division rounded towards negative infinity.
//
// Go's `/` truncates towards zero, so the two differ for a negative quotient
// that does not divide evenly -- which `RandRangeStep`'s count is, for
// exactly the arguments that describe a backwards range.
func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// Shuffle is Fisher-Yates, in the recorded direction.
//
// The direction is the thing. The recorded shuffle walks **downwards** -- i
// from length-1 to 1 -- swapping x[i] with a randomly chosen x[j] where
// j <= i. The upward form is the same algorithm and the
// same quality of shuffle, and produces a completely different permutation
// from the same seed. So does stopping at 0 instead of 1, which draws one
// pointless `RandBelow(1)` and shifts every later draw.
//
// Shaped like `math/rand.Shuffle` so it can shuffle something that is not a
// slice; ShuffleSlice is the common case.
func (r *Random) Shuffle(length int, swap func(i, j int)) {
	for i := length - 1; i >= 1; i-- {
		j := r.RandBelow(int64(i) + 1)
		swap(i, int(j))
	}
}

// ShuffleSlice shuffles a slice in place.
//
// This is the call Tier 1 makes, and -- measured against the engine, not
// assumed -- the only way Tier 1 consumes randomness at all: one
// generator seeded per run, one shuffle per mulligan, and nothing else
// draws. So the whole entropy budget of a simulation is this function, which
// is what lets a recorded run's stream be checked on its own, without
// running the engine.
func ShuffleSlice[T any](r *Random, x []T) {
	r.Shuffle(len(x), func(i, j int) { x[i], x[j] = x[j], x[i] })
}

// ChoiceIndex is the index Choice picks from a sequence of the given
// length: `RandBelow(length)`, no more.
//
// It panics on an empty sequence, which has no index to answer with.
func (r *Random) ChoiceIndex(length int) int {
	if length <= 0 {
		panic("mt19937: ChoiceIndex on an empty sequence")
	}
	return int(r.RandBelow(int64(length)))
}

// Choice is one element of seq, chosen uniformly.
func Choice[T any](r *Random, seq []T) T {
	return seq[r.ChoiceIndex(len(seq))]
}
