// Package mt19937 is the app's seeded generator: MT19937 with a fixed
// seeding path and draw discipline, reproduced bit for bit against a
// recorded corpus.
//
// It exists because three things this app shows a person are seeded, and a
// seed is a promise. Tier 1's every simulation runs from one
// (`simruns.DEFAULT_SEED`; an unseeded sample is not reproducible and is
// not shown). The tarot deal returns its seed and the browser holds it, so
// a reload lays out the same three cards -- the reading is of one person on
// one evening, and a spread that changed under them would be a different
// reading of the same person. The Wheel of Fortune spins from one too.
// None of that survives a generator that merely shuffles *well*.
//
// So what breaks if this drifts is not a test. A client-held tarot seed
// deals a different spread, which is the one surface commandment 15 says
// gets the most care. Every quoted Tier 1 number moves, and every generated
// primer with it. And ADR 18's cache stops being *coherent*: its keys carry
// an engine-source fingerprint, so cached rows re-key honestly and nothing
// serves a stale number -- but a user who re-runs an old seed to see the
// same game again would not see it, which is precisely what "seeded"
// promised them. None of those three failures is loud. That is why this is
// a reproduction held to a corpus rather than a generator held to a test.
//
// # The shape of the package
//
// Two files, split where a bug localises:
//
//   - this file is the generator -- the state, the two seeding routines
//     (`initGenrand`, `seedWords`), the raw word stream, `Float64` and
//     `GetRandBits`.
//   - random.go is everything that only *consumes* the stream --
//     `RandBelow`, `RandRange`, `Shuffle`, `Choice`.
//
// Held to `testdata/draws.json`, a frozen golden that records the raw word
// stream separately from every method that consumes it, so a failure says
// which half is wrong. The corpus is never regenerated: the recorded
// stream is the promise every stored seed rests on.
//
// # The three details a reimplementation gets wrong
//
// **`New(n)` does not seed like `initGenrand`.** It takes the absolute
// value, splits it into little-endian 32-bit words, and runs `seedWords`
// over them -- so seed 7 and seed -7 are the same stream, and the stream
// changes shape at 2**32 and again at 2**64 as the key grows a word.
// Seeding with `initGenrand(7)` directly is a different generator that
// looks equally random and agrees with the corpus on nothing.
//
// **`Float64` is two words, in that order**: `(a>>5) * 67108864.0 + (b>>6)`
// scaled by `1.0/9007199254740992.0`. Every value it can produce is exactly
// representable, so the corpus compares float64 *bits* -- a tolerance here
// would be hiding a bug rather than allowing for one.
//
// **`RandBelow` rejects, and where it rejects is part of the stream.**
// `k = bitLen(n)` and it draws `GetRandBits(k)` until the value is below n,
// so `RandBelow(2**20)` never rejects and `RandBelow(2**20 + 1)` rejects
// almost half the time. Get the rejection wrong and the first draw agrees,
// the tenth does not, and every shuffle after it is a different deck.
//
// # What is deliberately absent
//
// Only what the served surfaces call is here: `Float64`, `RandRange`,
// `Shuffle`, `Choice`, and the two primitives under them. Weighted
// sampling, the distributions, random bytes and state get/set have no
// caller -- the tarot deal cannot use weights, and says so where it deals
// -- and a path nothing exercises is a liability rather than a feature.
// `GetRandBits` stops at 64 bits for the same reason: `RandBelow` asks for
// `bitLen(n)`, and no caller passes an n beyond int64.
//
// Nothing here is safe for concurrent use; a run owns its generator.
package mt19937

import (
	"encoding/binary"
	"math/big"
	"math/bits"
)

// MT19937's constants, from the published reference implementation.
const (
	n = 624
	m = 397

	matrixA   uint32 = 0x9908b0df
	upperMask uint32 = 0x80000000 // most significant w-r bits
	lowerMask uint32 = 0x7fffffff // least significant r bits
)

// Random is one Mersenne Twister, in the state the recorded stream starts
// from.
//
// `index == n` means the state is spent and the next word triggers a twist,
// which is the condition `init_genrand` leaves behind and therefore the state
// a freshly seeded generator is in.
type Random struct {
	state [n]uint32
	index int
}

// New returns a generator seeded by the package's fixed seeding path:
// absolute value, little-endian 32-bit words, `seedWords`.
//
// Negative seeds seed by absolute value -- New(-7) and New(7) are the same
// generator. That is not a convenience added here; it is the recorded
// contract, and the corpus proves the two streams identical rather than
// trusting this sentence.
func New(seed int64) *Random {
	r := &Random{}
	r.Seed(seed)
	return r
}

// Seed re-seeds in place, running the same fixed path New runs.
func (r *Random) Seed(seed int64) {
	// abs(seed), as a uint64 because the answer does not always fit an int64.
	// Written the long way for math.MinInt64, whose absolute value is 2**63:
	// the plausible `if seed < 0 { seed = -seed }` overflows back to
	// math.MinInt64 there and seeds from a negative number, silently. No
	// caller passes it; the fuzz target does.
	var u uint64
	if seed < 0 {
		//nolint:gosec // ^x + 1 is two's-complement negation, exact for every
		// int64 including the one whose negation is not an int64.
		u = uint64(^seed) + 1
	} else {
		u = uint64(seed) //nolint:gosec // non-negative, by the branch
	}

	// `keyused = bits == 0 ? 1 : (bits - 1) / 32 + 1`, then the magnitude's
	// 32-bit words, least significant first. Both truncations below are the
	// decomposition itself rather than a loss.
	if bits.Len64(u) <= 32 {
		r.seedWords([]uint32{uint32(u)}) //nolint:gosec // u fits 32 bits here
		return
	}
	//nolint:gosec // the low and high words of u, least significant first
	r.seedWords([]uint32{uint32(u), uint32(u >> 32)})
}

// NewFromBig seeds from an integer of any size, which the seeding contract
// allows.
//
// A seed has no ceiling, and the key built from one grows a 32-bit word
// every 32 bits -- so a seed past 2**64 is a *different shape* of key, not
// merely a bigger number, and the stream it produces
// cannot be reached from any int64. Nothing in this application passes one;
// this is here so the corpus can prove the decomposition is right where it is
// hardest to get right, and so a future caller is not tempted to truncate.
func NewFromBig(seed *big.Int) *Random {
	r := &Random{}
	r.SeedBig(seed)
	return r
}

// SeedBig re-seeds in place from an integer of any size.
func (r *Random) SeedBig(seed *big.Int) {
	magnitude := new(big.Int).Abs(seed)

	keyused := 1
	if b := magnitude.BitLen(); b > 0 {
		keyused = (b-1)/32 + 1
	}

	// The magnitude's bytes, read into `keyused` little-endian 32-bit words.
	// FillBytes writes big-endian into the whole buffer, zero-padded on the
	// left, so word i -- the i-th least significant -- is the four bytes that
	// far from the right. Done through bytes rather than through `Bits()`
	// because a `big.Word` is 32 bits on some machines and 64 on others, and
	// a seeding path that changes with the host word size is exactly the kind
	// of bug this package exists to not have.
	buf := make([]byte, keyused*4)
	magnitude.FillBytes(buf)
	key := make([]uint32, keyused)
	for i := range key {
		off := len(buf) - 4*(i+1)
		key[i] = binary.BigEndian.Uint32(buf[off : off+4])
	}
	r.seedWords(key)
}

// seedWords is `init_by_array` over an already-decomposed key.
func (r *Random) seedWords(key []uint32) {
	r.initGenrand(19650218)

	i, j := 1, 0
	k := n
	if len(key) > k {
		k = len(key)
	}
	for ; k > 0; k-- {
		r.state[i] = (r.state[i] ^ ((r.state[i-1] ^ (r.state[i-1] >> 30)) * 1664525)) +
			key[j] + uint32(j) // non linear
		i++
		j++
		if i >= n {
			r.state[0] = r.state[n-1]
			i = 1
		}
		if j >= len(key) {
			j = 0
		}
	}
	for k = n - 1; k > 0; k-- {
		r.state[i] = (r.state[i] ^ ((r.state[i-1] ^ (r.state[i-1] >> 30)) * 1566083941)) -
			uint32(i) // non linear
		i++
		if i >= n {
			r.state[0] = r.state[n-1]
			i = 1
		}
	}
	// MSB is 1; assuring a non-zero initial array.
	r.state[0] = 0x80000000
}

// initGenrand is the scalar seeding MT19937 was published with
// (`init_genrand`). Nothing seeds with it directly: `seedWords` runs it as
// its first step, and a user's seed only ever arrives through `seedWords`.
func (r *Random) initGenrand(s uint32) {
	r.state[0] = s
	for i := 1; i < n; i++ {
		r.state[i] = 1812433253*(r.state[i-1]^(r.state[i-1]>>30)) + uint32(i)
	}
	// Spent: the next word twists. `init_by_array` relies on this and does
	// not set it again.
	r.index = n
}

// Uint32 is `genrand_uint32`: one tempered word off the stream.
//
// Exported because it is the only place a seeding or twist fault can be seen
// *as* one. Every other method here consumes this, so a corpus mismatch with
// this matching says the fault is in the consumer.
func (r *Random) Uint32() uint32 {
	if r.index >= n {
		r.twist()
	}
	y := r.state[r.index]
	r.index++

	y ^= y >> 11
	y ^= (y << 7) & 0x9d2c5680
	y ^= (y << 15) & 0xefc60000
	y ^= y >> 18
	return y
}

// twist regenerates all n words at once.
func (r *Random) twist() {
	mag01 := [2]uint32{0, matrixA}

	var kk int
	for kk = 0; kk < n-m; kk++ {
		y := (r.state[kk] & upperMask) | (r.state[kk+1] & lowerMask)
		r.state[kk] = r.state[kk+m] ^ (y >> 1) ^ mag01[y&1]
	}
	for ; kk < n-1; kk++ {
		y := (r.state[kk] & upperMask) | (r.state[kk+1] & lowerMask)
		r.state[kk] = r.state[kk+(m-n)] ^ (y >> 1) ^ mag01[y&1]
	}
	y := (r.state[n-1] & upperMask) | (r.state[0] & lowerMask)
	r.state[n-1] = r.state[m-1] ^ (y >> 1) ^ mag01[y&1]

	r.index = 0
}

// Float64 is the generator's uniform double: a 53-bit value from two words.
//
// The order and the arithmetic are both load-bearing. `a` is drawn first and
// keeps 27 bits, `b` second and keeps 26; the multiply-and-add builds the
// 53-bit significand exactly, and the scale is a power of two, so the whole
// expression is exact and every result matches the recorded stream to the
// bit. Any rearrangement that looks equivalent -- dividing twice, or scaling
// before adding -- is not.
func (r *Random) Float64() float64 {
	a := r.Uint32() >> 5
	b := r.Uint32() >> 6
	return (float64(a)*67108864.0 + float64(b)) * (1.0 / 9007199254740992.0)
}

// MaxBits is the widest `GetRandBits` will answer.
//
// The draw discipline itself has no ceiling -- wider requests would build an
// arbitrary-precision integer a word at a time. Nothing in this application
// asks for one: `GetRandBits` has no direct caller at all, and the only
// indirect one is `RandBelow`, which asks for `bitLen(n)` of a deck size, a
// card count or a spin. 64 covers every one of those with the whole int64
// range to spare, and it means this package answers in a machine word rather
// than in `math/big`.
const MaxBits = 64

// GetRandBits returns k random bits as an unsigned integer.
//
// Two paths, and the second is where the reimplementations go wrong. Up to 32
// bits it is one word shifted down -- `Uint32() >> (32 - k)`, so the bits
// kept are the *high* ones. Beyond that the recorded discipline fills 32-bit
// words from **least** significant to most, drawing in that order, and
// shifts only the last one down by whatever the remainder leaves. Draw the
// words the other way round and every value is wrong while every value still
// looks random.
//
// It panics above MaxBits, and for k == 0 returns 0 without drawing -- the
// recorded behavior, and the reason `RandBelow` must never be handed a zero.
func (r *Random) GetRandBits(k uint) uint64 {
	switch {
	case k == 0:
		return 0
	case k <= 32:
		return uint64(r.Uint32() >> (32 - k))
	case k <= MaxBits:
		lo := uint64(r.Uint32())
		hi := uint64(r.Uint32())
		if rest := k - 32; rest < 32 {
			hi >>= 32 - rest
		}
		return lo | hi<<32
	default:
		panic("mt19937: GetRandBits asked for more than 64 bits")
	}
}
