package mt19937

import "testing"

// Benchmarks for the seeded generator.
//
// This is the floor everything seeded stands on: a Tier 1 run draws from it
// once per card per hand per trial, so a regression here is a regression in
// every simulation the site can run. The numbers are only worth reading as a
// comparison against another run of the same benchmark on the same machine --
// `benchstat old.txt new.txt` -- never as an absolute.
//
// The results are assigned to package-level sinks because the compiler is
// entitled to delete a pure call whose result nobody reads, and a benchmark
// that measured nothing would report a very good number for it.

var (
	sinkU32   uint32
	sinkF64   float64
	sinkBits  uint64
	sinkSlice []float64
)

func BenchmarkSeed(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		r := New(int64(i))
		sinkU32 = r.Uint32()
	}
}

func BenchmarkUint32(b *testing.B) {
	r := New(1)
	for b.Loop() {
		sinkU32 = r.Uint32()
	}
}

// BenchmarkUint32AcrossATwist spans more than one 624-word block, so the cost
// of `twist` is inside the measurement rather than amortised away by a loop
// that never reaches it.
func BenchmarkUint32AcrossATwist(b *testing.B) {
	r := New(1)
	for b.Loop() {
		for range 624 {
			sinkU32 = r.Uint32()
		}
	}
}

func BenchmarkFloat64(b *testing.B) {
	r := New(1)
	for b.Loop() {
		sinkF64 = r.Float64()
	}
}

func BenchmarkGetRandBits(b *testing.B) {
	for _, k := range []uint{1, 16, 32, 53} {
		b.Run(bitLabel(k), func(b *testing.B) {
			r := New(1)
			for b.Loop() {
				sinkBits = r.GetRandBits(k)
			}
		})
	}
}

// BenchmarkShuffleSizedLikeALibrary is the shape a Tier 1 trial actually
// asks for: ninety-nine draws off one seeded generator.
func BenchmarkShuffleSizedLikeALibrary(b *testing.B) {
	const library = 99
	out := make([]float64, library)
	for b.Loop() {
		r := New(7)
		for i := range out {
			out[i] = r.Float64()
		}
		sinkSlice = out
	}
}

func bitLabel(k uint) string {
	switch k {
	case 1:
		return "1bit"
	case 16:
		return "16bits"
	case 32:
		return "32bits"
	default:
		return "53bits"
	}
}
