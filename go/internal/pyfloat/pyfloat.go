// Package pyfloat is CPython's float arithmetic, reproduced rather than
// approximated: `math.fsum`, both of `round`'s spellings, and the explicit
// conversion that stops this machine improving on either.
//
// The third of the `py*` packages, after `pyrand` (CPython's `random.Random`)
// and `pyyaml` (PyYAML's emitter), and here for the same reason both of those
// are: a value crosses the wire, so the port has to answer in Python's
// dialect rather than in a better one. It lived in `internal/sim` until
// 2026-08-22, when the float-sum sweep found the same trap in three packages
// that are not the simulator -- `artifacts` renders a money total, `analyze`
// sums three hypergeometrics into a served payload, `suggest` sums four
// weighted products into a ranking key. Importing `internal/sim` from the
// deliverables renderer would have said the renderer depends on the
// simulator, which is false; a second transcription of Shewchuk's algorithm
// would have been worse than either.
//
// The reason to reproduce these at all is not tidiness. Every integer this
// port has to agree with comes out of a `>=` against a float:
// `required_sources` scans until `hypergeometric_at_least(...) >= target`,
// `reliable_turn` scans until `castable_odds >= TARGET`, `_slots_to_target`
// scans until `on_curve_odds >= target`, and `curve`'s advice branches on
// `abs(per_land - per_ramp) < TOO_CLOSE`. A one-ulp disagreement in any of
// them is not a rounding difference on a screen -- it is a different land
// count, a different reliable turn, a different row order in the shelf, or a
// different recommendation. So the float arithmetic is matched exactly, and
// the epsilons the differential tests pin are consequences of that rather than
// allowances made for it.
//
// # Which Python, when there is more than one
//
// `Fsum` is the answer to a question that only has one, which is the point of
// reaching for it. **`sum()` is not**: CPython 3.12 gave `sum()` over floats
// compensated (Neumaier) accumulation where 3.11 adds them left to right, and
// this project supports both, tests both in CI, and ships 3.12 in the image.
// A Go `for ... { total += x }` reproduces **3.11**, so a port written the
// obvious way agrees with the interpreter the container is not running. Every
// Python site that summed floats was moved to `math.fsum` on 2026-08-22, and
// every Go site that mirrored one was moved to `Fsum`; a naive accumulation
// loop over floats in this module is now a bug wherever it appears beside a
// Python `fsum`.
package pyfloat

import (
	"math"
	"math/big"
)

// Fsum is CPython's `math.fsum`: the correctly-rounded sum of a sequence,
// Shewchuk's algorithm with the final half-even fix-up CPython applies.
//
// `karsten.castable_odds` sums a hundred small products of probabilities with
// `fsum` deliberately -- its own comment says so -- and a naive left-to-right
// sum here would differ in the last bits. That would be invisible at one
// decimal place and visible in `CardOdds.reliable_turn`, which compares the
// result against 0.90.
//
// Transcribed from `Modules/mathmodule.c:math_fsum_impl`, including the two
// details a plausible implementation gets wrong: a zero running total is
// *not* appended to the partials, and the final accumulation stops at the
// first inexact addition and then corrects for half-even rounding across
// partials.
//
// Inputs are expected finite; every caller here sums probabilities. CPython
// raises OverflowError on an intermediate overflow of finite inputs, which
// cannot arise from values in [0, 1], so no error is returned and a
// non-finite input simply propagates through the same arithmetic.
func Fsum(values []float64) float64 {
	partials := make([]float64, 0, 8)
	for _, x := range values {
		i := 0
		for _, y := range partials {
			if math.Abs(x) < math.Abs(y) {
				x, y = y, x
			}
			hi := x + y
			lo := y - (hi - x)
			if lo != 0.0 {
				partials[i] = lo
				i++
			}
			x = hi
		}
		partials = partials[:i]
		if x != 0.0 {
			partials = append(partials, x)
		}
	}

	n := len(partials)
	hi := 0.0
	lo := 0.0
	if n > 0 {
		n--
		hi = partials[n]
		// Sum from the top, stopping when the sum becomes inexact.
		for n > 0 {
			x := hi
			n--
			y := partials[n]
			hi = x + y
			lo = y - (hi - x)
			if lo != 0.0 {
				break
			}
		}
		// Make half-even rounding work across multiple partials, so that the
		// answer is the correctly-rounded one rather than one ulp under it.
		if n > 0 && ((lo < 0.0 && partials[n-1] < 0.0) || (lo > 0.0 && partials[n-1] > 0.0)) {
			y := lo * 2.0
			x := hi + y
			if y == x-hi {
				hi = x
			}
		}
	}
	return hi
}

// Round is CPython's one-argument `round(float)`: to the nearest integer, ties
// to even. Go's `math.Round` breaks ties away from zero, so `round(34.5)` is
// 34 in Python and 35 in Go -- and `karsten.RegressionLands` rounds a land
// count with it, where the difference is one land in the recommendation.
//
// The correction is CPython's own, from `Objects/floatobject.c:float___round__`.
//
// It returns an **int**, which is what CPython's one-argument round returns
// and what every caller here wants. That is not a detail: a float64 return
// would have to answer `Round(-0.5)`, where CPython's integer zero has no sign
// and Go's `math.Round` produces a negative one. Returning an int deletes the
// question rather than answering it wrongly, and the corpus pins integers for
// the same reason.
func Round(x float64) int {
	r := math.Round(x)
	if math.Abs(x-r) == 0.5 {
		r = 2.0 * math.Round(x/2.0)
	}
	return int(r)
}

// RoundTo is CPython's two-argument `round(float, ndigits)` for ndigits >= 0:
// the value rounded half-to-even at `ndigits` decimal places, then taken back
// to the nearest float64.
//
// CPython does this by formatting with `_Py_dg_dtoa` in mode 3 and re-parsing
// with `_Py_dg_strtod`, both correctly rounded. Done here in exact rationals
// instead of through `strconv`, because the claim wanted is *what the
// arithmetic is* rather than *what two libraries happen to agree about*: a
// float64 is exactly a rational, `x * 10^n` is exact, rounding that to an
// integer half-to-even is exact, and `big.Rat.Float64` is documented to give
// the nearest float64 with ties to even -- which is the same value CPython's
// re-parse lands on, by the same definition.
//
// `regression_lands` reports an average mana value at two places and
// `curve`'s advice reports four odds at four; all six are stored rounded and
// compared unrounded, so this is a serialisation detail that the wire shows.
func RoundTo(x float64, ndigits int) float64 {
	if math.IsInf(x, 0) || math.IsNaN(x) || ndigits < 0 {
		return x
	}
	r := new(big.Rat).SetFloat64(x) // non-nil: Inf and NaN are refused above
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(ndigits)), nil))
	r.Mul(r, scale)

	// Round r half-to-even to an integer.
	num, den := r.Num(), r.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	// QuoRem truncates toward zero; rem carries the sign of num.
	twice := new(big.Int).Abs(rem)
	twice.Lsh(twice, 1)
	switch twice.Cmp(den) {
	case 1: // strictly past the halfway point: away from zero
		q.Add(q, big.NewInt(int64(sign(num))))
	case 0: // exactly halfway: to even
		if q.Bit(0) == 1 {
			q.Add(q, big.NewInt(int64(sign(num))))
		}
	}

	out := new(big.Rat).SetFrac(q, new(big.Int).Set(scale.Num()))
	f, _ := out.Float64()
	if f == 0 {
		// `big.Rat` has no signed zero and CPython's dtoa/strtod round trip
		// does: `round(-0.4)` is `-0.0` there, and the difference is a visible
		// minus sign on any surface that formats the result. Found by the
		// corpus, which compares bits rather than values.
		return math.Copysign(0, x)
	}
	return f
}

func sign(n *big.Int) int {
	if n.Sign() < 0 {
		return -1
	}
	return 1
}

// Rounded is the guard against a fused multiply-add, and it is the reason
// every `total += a * b` in these two packages is written `total +=
// pyfloat.Rounded(a * b)`.
//
// The Go specification permits an implementation to "combine multiple
// floating-point operations into a single fused operation, possibly across
// statements", and the arm64 backend takes that permission: `z + x*y` becomes
// one FMADD, which rounds *once* where CPython rounds twice. That is a
// one-ulp difference, on one of the two architectures CI builds and the
// image ships, in exactly the accumulations these closed forms are made of --
// and a one-ulp difference here is a different integer out of the `>=` scans
// described at the top of this file, not a cosmetic one.
//
// The specification names precisely one way to stop it: "an explicit
// floating-point type conversion rounds to the precision of the target type,
// preventing fusion that would discard that rounding." So the float64-to-
// float64 conversion below is the whole point of this function and is not the
// no-op it reads as. (`unconvert` leaves it alone, checked -- so there is no
// `nolint` here, and adding one would claim an objection nobody made.)
//
// Verified rather than assumed, twice over. Compiled for arm64, the bare
// `t += xs[i] * ys[i]` emits one `FMADDD` and the guarded form emits `FMULD`
// then `FADDD` -- the guard survives inlining, which is the half that was
// worth checking. And `TestNoFusedMultiplyAddSurvivesTheGuard` runs the same
// shape on whatever architecture the suite is on, over a pair whose exact
// product is one ulp from its rounded one, so the guard is proven by the
// answer rather than by the instruction listing.
func Rounded(x float64) float64 { return float64(x) }
