package mana

import (
	"strings"
	"testing"
)

// The generative half of the solver's net, and it is not decoration.
//
// The fuzzer plays the solver against the brute-force and Hall's-theorem
// oracles, written once from the definitions. `castability_test.go` proves
// this package answers the recorded 13,944 *enumerated* cases; this one looks
// where that enumeration structurally cannot.
//
// **And it had to, on the first day it was written.** The enumerated set
// draws
// its pips as non-decreasing index
// tuples -- and the hybrid {W,U} is the *last* entry in
// its pip alphabet. So no cost in all 13,944 cases ever presents a **wide pip
// before a narrow one**: every sequence of pip widths it can produce is
// non-decreasing. That is not a small omission. Deleting the `seen` reset in
// [maxMatching] -- the classic Kuhn's mistake, and one line -- passes the
// entire case set, both oracles across the entire case set, and every
// hand-pinned trap. It fails on `{W/U}{W} <- [W U]`, which is two pips and two
// lands and which the enumeration cannot express in that order.
//
// The moral is the one this project keeps buying: an enumeration covers what
// its generator can say, and its generator's shape is a claim nobody restates.
// That case is now pinned by name in `solver_test.go` as well, because a trap
// found is a trap that gets a permanent test -- but the fuzzer is what found
// it, and the fuzzer is what will find the next one.
//
// Two kinds of check run here. The three implementations are played against
// each other, which is the exactness argument; and a handful of properties are
// asserted that must hold of *any* correct solver, stated without reference to
// how one works. The second kind is what caught the case above without needing
// an oracle at all: a solver whose answer depends on the order the pips arrive
// in is wrong, whatever it answers.

// fuzzColors is the domain: the five colours and colourless. A source's
// colours and a pip's are both subsets of it, packed as a six-bit mask.
var fuzzColors = []string{"W", "U", "B", "R", "G", "C"}

func maskColors(mask uint8) []string {
	var out []string
	for i, color := range fuzzColors {
		if mask&(1<<i) != 0 {
			out = append(out, color)
		}
	}
	return out
}

// A generated case, and the bytes it travels as.
//
// The encoding is a fixed six-byte header naming the sizes, then the bodies.
// It exists so a seed can be written as a described shape rather than as a
// magic string -- `caseBytes` below is how every seed in this file is built,
// and reading one back is [decodeCase].
type fuzzCase struct {
	generic   int
	hasX      bool
	xValue    int
	pips      []uint8
	phyrexian []uint8
	sources   []uint8 // pairs: colour mask, then amount
}

func caseBytes(c fuzzCase) []byte {
	flags := byte(0)
	if c.hasX {
		flags = 1
	}
	out := []byte{
		byte(c.generic), flags, byte(c.xValue),
		byte(len(c.pips)), byte(len(c.phyrexian)), byte(len(c.sources) / 2),
	}
	out = append(out, c.pips...)
	out = append(out, c.phyrexian...)
	return append(out, c.sources...)
}

// decodeCase folds arbitrary bytes into a case the oracles can afford.
//
// Sizes are clamped rather than rejected, so no fuzz input is spent being
// thrown away: the brute force is factorial in the pip count and Hall's is
// exponential in it, and the interesting failures live at three or four pips
// against four sources, not at ten. A read past the end of the input reads
// zero, which is what makes every byte string a valid case.
func decodeCase(data []byte) (Cost, []Source, int) {
	at := 0
	next := func() byte {
		if at >= len(data) {
			at++
			return 0
		}
		b := data[at]
		at++
		return b
	}

	generic := int(next() % 4)
	hasX := next()&1 == 1
	xValue := int(next() % 4)
	pipCount := int(next() % 5)
	phyrexianCount := int(next() % 3)
	sourceCount := int(next() % 5)

	cost := Cost{Generic: generic, HasX: hasX}
	for range pipCount {
		cost.Pips = append(cost.Pips, maskColors(next()%64))
	}
	for range phyrexianCount {
		cost.Phyrexian = append(cost.Phyrexian, maskColors(next()%64))
	}
	sources := make([]Source, 0, sourceCount)
	for range sourceCount {
		sources = append(sources, Source{
			Colors: maskColors(next() % 64),
			Amount: int(next() % 3),
		})
	}
	return cost, sources, xValue
}

func FuzzCastability(f *testing.F) {
	const (
		w = 1 << iota
		u
		b
		r
		g
		c
	)
	const any = w | u | b | r | g

	// The seeds are the long-pinned trap shapes, plus the one the
	// enumeration cannot reach. A seed corpus is not a wish list: each of these
	// is a case somebody was once wrong about, so a fuzzer that wanders off
	// still starts from the places wrongness has actually lived.
	for _, seed := range []fuzzCase{
		// The case the enumerated set cannot express: a hybrid pip standing
		// BEFORE a mono pip whose only payer the hybrid would otherwise take.
		// Payable, and the classic Kuhn's bug says otherwise.
		{pips: []uint8{w | u, w}, sources: []uint8{w, 1, u, 1}},
		// The same trap the other way up, which the enumeration does reach.
		{pips: []uint8{w, w | u}, sources: []uint8{w, 1, u, 1}},
		// A lone dual against two different pips: not payable.
		{pips: []uint8{w, u}, sources: []uint8{w | u, 1, g, 1}},
		// Two duals covering a double pip: payable.
		{pips: []uint8{g, g}, sources: []uint8{g | w, 1, g | w, 1}},
		// Sol Ring: one source, two colourless mana, no coloured pip.
		{generic: 2, sources: []uint8{c, 2}},
		{pips: []uint8{g}, sources: []uint8{c, 2}},
		// "Any colour" is not colourless.
		{pips: []uint8{c}, sources: []uint8{any, 1}},
		// Five pips across five any-colour sources, and four of them.
		{pips: []uint8{w, u, b, r}, sources: []uint8{any, 1, any, 1, any, 1, any, 1}},
		{pips: []uint8{w, u, b, r}, sources: []uint8{any, 1, any, 1, any, 1}},
		// X eating the leftovers.
		{hasX: true, xValue: 2, pips: []uint8{g}, sources: []uint8{g, 1, g, 1, g, 1}},
		// Phyrexian, which must constrain nothing at all.
		{phyrexian: []uint8{g}},
		{generic: 1, phyrexian: []uint8{g | u}},
		// The degenerate ends: nothing to pay, and nothing to pay it with.
		{},
		{pips: []uint8{w}},
		// A pip no colour can pay, and a source that makes no mana.
		{pips: []uint8{0}, sources: []uint8{any, 2}},
		{pips: []uint8{w}, sources: []uint8{w, 0}},
	} {
		f.Add(caseBytes(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cost, sources, xValue := decodeCase(data)
		got := CanPay(cost, sources, xValue)
		name := caseID(cost, sources)

		// --- the exactness half: three implementations, two definitions.
		if want := bruteForceCanPay(cost, sources, xValue); got != want {
			t.Fatalf("%s (x=%d): solver says %v, an exhaustive search of every "+
				"assignment says %v", name, xValue, got, want)
		}
		if want := hallCanPay(cost, sources, xValue); got != want {
			t.Fatalf("%s (x=%d): solver says %v, Hall's condition says %v",
				name, xValue, got, want)
		}

		// --- the order the pips arrive in cannot change the answer. This is
		// the property that needs no oracle, and the one that catches a
		// matching which forgets to reset its search between pips.
		if flipped := reversePips(cost); CanPay(flipped, sources, xValue) != got {
			t.Fatalf("%s (x=%d): answered %v, but %s answered %v -- the pips "+
				"are a set, and their order is not information",
				name, xValue, got, flipped, !got)
		}

		// --- nor can the order the sources arrive in. A solver that reads it
		// makes a seeded simulation irreproducible, which is the bug this
		// property was originally written against.
		if CanPay(cost, reverseSources(sources), xValue) != got {
			t.Fatalf("%s (x=%d): answered %v, and %v with the same lands in "+
				"the other order", name, xValue, got, !got)
		}

		// --- more mana never hurts. An extra any-colour source cannot make a
		// payable cost unpayable.
		if got && !CanPay(cost, append(append([]Source(nil), sources...),
			NewSource(fuzzColors, 1)), xValue) {
			t.Fatalf("%s (x=%d): payable, and not payable with one more source",
				name, xValue)
		}

		// --- Phyrexian is paid with life and constrains no mana base, so
		// adding one may raise the mana value and must not move the answer.
		dearer := cost
		dearer.Phyrexian = append(append([][]string(nil), cost.Phyrexian...),
			[]string{"G"})
		if CanPay(dearer, sources, xValue) != got {
			t.Fatalf("%s (x=%d): adding a Phyrexian symbol changed the answer "+
				"from %v -- it is paid with two life", name, xValue, got)
		}

		// --- a cost built its own pool always pays: one exactly-matching
		// source per pip, plus an any-colour source per point of generic and
		// X. The one direction where the right answer is known outright.
		exact := make([]Source, 0, len(cost.Pips))
		for _, pip := range cost.Pips {
			if len(pip) == 0 {
				exact = nil // an empty pip is unpayable by anything
				break
			}
			exact = append(exact, NewSource(pip, 1))
		}
		if exact != nil || len(cost.Pips) == 0 {
			spare := cost.Generic
			if cost.HasX {
				spare += xValue
			}
			for range spare {
				exact = append(exact, NewSource(fuzzColors, 1))
			}
			if !CanPay(cost, exact, xValue) {
				t.Fatalf("%s refused a pool built out of its own cost", cost)
			}
		}
	})
}

func reversePips(cost Cost) Cost {
	out := cost
	out.Pips = make([][]string, len(cost.Pips))
	for i, pip := range cost.Pips {
		out.Pips[len(cost.Pips)-1-i] = pip
	}
	return out
}

func reverseSources(sources []Source) []Source {
	out := make([]Source, len(sources))
	for i, source := range sources {
		out[len(sources)-1-i] = source
	}
	return out
}

// A guard on the guard. Every seed above is meant to describe a real shape,
// and a seed that decodes to something other than what it says is a seed that
// tests nothing while looking like it tests something -- the failure mode a
// hand-rolled encoding invites. So the round trip is checked on the two seeds
// whose exact shape the argument above depends on.
func TestTheSeedEncodingSaysWhatItMeans(t *testing.T) {
	cost, sources, xValue := decodeCase(caseBytes(fuzzCase{
		pips: []uint8{1<<0 /*W*/ | 1<<1 /*U*/, 1 << 0}, sources: []uint8{1 << 0, 1, 1 << 1, 1},
	}))
	if got := caseID(cost, sources); got != "{U/W}{W} <- [W U]" {
		t.Errorf("the wide-pip-first seed decodes to %q", got)
	}
	if xValue != 0 {
		t.Errorf("xValue decoded as %d, want 0", xValue)
	}
	if !CanPay(cost, sources, xValue) {
		t.Error("the wide-pip-first seed should be payable")
	}

	cost, sources, xValue = decodeCase(caseBytes(fuzzCase{
		hasX: true, xValue: 2, pips: []uint8{1 << 4 /*G*/},
		sources: []uint8{1 << 4, 1, 1 << 4, 1, 1 << 4, 1},
	}))
	if got := caseID(cost, sources); got != "{X}{G} <- [G G G]" {
		t.Errorf("the X seed decodes to %q", got)
	}
	if xValue != 2 {
		t.Errorf("xValue decoded as %d, want 2", xValue)
	}

	// And a truncated input is a case rather than a panic: every byte string
	// the fuzzer can hand over has to decode into something answerable.
	for size := range 12 {
		cost, sources, xValue := decodeCase(make([]byte, size))
		if strings.Contains(caseID(cost, sources), "!") {
			t.Errorf("%d zero bytes decoded oddly", size)
		}
		CanPay(cost, sources, xValue)
	}
}
