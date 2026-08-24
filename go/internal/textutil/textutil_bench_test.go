package textutil

import (
	"strings"
	"testing"
)

// Benchmarks for the recorded string semantics.
//
// These are small functions called a great many times -- every rationale, every
// line of every artifact, every card name that gets folded or measured. The
// interesting inputs are the non-ASCII ones: `Len` counts runes rather than
// bytes and `Strip` trims on a Unicode-aware predicate, so a card name with a
// diacritic takes a different path from its ASCII neighbours, and it is the
// path nobody benchmarks by accident.

var (
	sinkStr   string
	sinkInt   int
	sinkLines []string
	sinkBool  bool
)

var (
	ascii    = "Squires throughout the realm aspire to her mix of panache and martial prowess."
	unicoded = "Jötun Grunt — Márton Stromgald · Séance · Lim-Dûl the Necromancer"
	padded   = "   \t\n  a rationale somebody typed with the shift key still down  \n\t  "
	document = strings.Repeat("A line of a generated primer, about as long as they get.\n", 200)
)

func BenchmarkStrip(b *testing.B) {
	for _, c := range []struct{ name, in string }{
		{"padded", padded},
		{"nothing to trim", ascii},
		{"unicode", unicoded},
	} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				sinkStr = Strip(c.in)
			}
		})
	}
}

func BenchmarkSplitJoin(b *testing.B) {
	for b.Loop() {
		sinkStr = SplitJoin(padded)
	}
}

func BenchmarkLen(b *testing.B) {
	for _, c := range []struct{ name, in string }{{"ascii", ascii}, {"unicode", unicoded}} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				sinkInt = Len(c.in)
			}
		})
	}
}

func BenchmarkHead(b *testing.B) {
	for b.Loop() {
		sinkStr = Head(unicoded, 20)
	}
}

// BenchmarkSplitLines runs over a primer-sized document, which is the size
// that actually shows up: the artifact writers split whole files, not lines.
func BenchmarkSplitLines(b *testing.B) {
	b.SetBytes(int64(len(document)))
	for b.Loop() {
		sinkLines = SplitLines(document)
	}
}

func BenchmarkIsSpace(b *testing.B) {
	for b.Loop() {
		for _, r := range unicoded {
			sinkBool = IsSpace(r)
		}
	}
}
