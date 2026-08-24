package floats

import (
	"math"
	"testing"
)

// Benchmarks for the exact-arithmetic kernel.
//
// [Fsum] is the one every aggregate in the simulator goes through, and it is
// deliberately *not* a naive sum -- it carries partials so the answer does not
// depend on the order the trials happened to finish in. That correctness costs
// something, and this is where the cost is visible. A change that makes Fsum
// faster and stops matching the frozen goldens is not an optimisation; the
// goldens decide, and these numbers only say what the guarantee costs.
//
// Sinks, because a pure call whose result nobody reads is one the compiler may
// delete outright.

var (
	sinkF64 float64
	sinkStr string
	sinkInt int
)

// summable is a slice with the properties that make naive summation wrong:
// values many orders of magnitude apart, so the small ones vanish into the
// large ones unless the partials are carried.
func summable(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		switch i % 4 {
		case 0:
			out[i] = 1e16
		case 1:
			out[i] = 1.0
		case 2:
			out[i] = -1e16
		default:
			out[i] = math.Pi
		}
	}
	return out
}

func BenchmarkFsum(b *testing.B) {
	for _, n := range []int{8, 100, 10_000} {
		values := summable(n)
		b.Run(size(n), func(b *testing.B) {
			b.SetBytes(int64(n * 8))
			for b.Loop() {
				sinkF64 = Fsum(values)
			}
		})
	}
}

// BenchmarkFsumAlreadyOrdered is the easy input, for contrast: same length, no
// catastrophic cancellation. The gap between this and BenchmarkFsum is what
// the partial-carrying actually costs on hard input.
func BenchmarkFsumAlreadyOrdered(b *testing.B) {
	values := make([]float64, 10_000)
	for i := range values {
		values[i] = float64(i) / 3
	}
	for b.Loop() {
		sinkF64 = Fsum(values)
	}
}

// BenchmarkRepr is the rendering half: every number that reaches the wire goes
// through it, so it runs once per field per response.
func BenchmarkRepr(b *testing.B) {
	for _, v := range []float64{0, 1, 0.1, 1.0 / 3.0, 1e22, math.MaxFloat64} {
		b.Run(Repr(v), func(b *testing.B) {
			for b.Loop() {
				sinkStr = Repr(v)
			}
		})
	}
}

func BenchmarkRoundTo(b *testing.B) {
	for b.Loop() {
		sinkF64 = RoundTo(2.675, 2)
	}
}

func BenchmarkRound(b *testing.B) {
	for b.Loop() {
		sinkInt = Round(2.5)
	}
}

func size(n int) string {
	switch n {
	case 8:
		return "8"
	case 100:
		return "100"
	default:
		return "10000"
	}
}
