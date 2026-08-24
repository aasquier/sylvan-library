package tier1

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

// The canonical rendering of the three result records the determinism gate
// hashes.
//
// ReferenceDigest is pinned as a sha256 over this rendering
// of one game, one run and a three-point sweep. So reproducing the
// simulation is only half of reproducing the digest: the *text* is what is
// hashed, and the number formatting is part of it. `100.0` is not
// `100`, `1e-05` is not `0.00001`, and a float renders as the shortest
// decimal that reads back as the same double.
//
// Nothing serves these strings. They exist so a run can be compared with the
// recorded reference as bytes rather than field by field, which is what
// turns "the
// engine looks right" into "the engine is the same simulator".
//
// The float renderer is held to a corpus in testdata: every float
// the reference run produces, plus the boundaries where the rendering
// switches
// between fixed and exponential notation -- all of it a frozen golden.

// Repr is the canonical rendering of a GameResult.
func (g GameResult) Repr() string {
	var b strings.Builder
	b.WriteString("GameResult(commander_turn=")
	b.WriteString(reprOptInt(g.CommanderTurn))
	b.WriteString(", lands_by_turn=" + reprInts(g.LandsByTurn))
	b.WriteString(", mana_by_turn=" + reprInts(g.ManaByTurn))
	b.WriteString(", spells_by_turn=" + reprInts(g.SpellsByTurn))
	b.WriteString(", unused_by_turn=" + reprInts(g.UnusedByTurn))
	b.WriteString(", mulligans=" + strconv.Itoa(g.Mulligans))
	b.WriteString(", color_screwed_turns=" + strconv.Itoa(g.ColorScrewedTurns))
	b.WriteString(", first_cast=" + reprFirstCast(g.FirstCast))
	b.WriteString(", missed_drop_by_turn=" + reprBools(g.MissedDropByTurn))
	b.WriteString(", first_spell_turn=" + reprOptInt(g.FirstSpellTurn))
	b.WriteString(", stalled_turns=" + strconv.Itoa(g.StalledTurns))
	b.WriteString(")")
	return b.String()
}

// Repr is the canonical rendering of a CardTiming.
func (c CardTiming) Repr() string {
	return "CardTiming(name=" + ReprString(c.Name) +
		", mv=" + strconv.Itoa(c.MV) +
		", cast_rate=" + ReprFloat(c.CastRate) +
		", median_turn=" + reprOptFloat(c.MedianTurn) +
		", by_t8=" + ReprFloat(c.ByT8) + ")"
}

// Repr is the canonical rendering of a SimSummary.
func (s SimSummary) Repr() string {
	var b strings.Builder
	b.WriteString("SimSummary(games=" + strconv.Itoa(s.Games))
	b.WriteString(", turns=" + strconv.Itoa(s.Turns))
	b.WriteString(", keep_rule=" + ReprString(s.KeepRule))
	b.WriteString(", mulligan_rate=" + ReprFloat(s.MulliganRate))
	b.WriteString(", avg_mulligans=" + ReprFloat(s.AvgMulligans))
	b.WriteString(", commander_by_turn=" + s.reprCommanderByTurn())
	b.WriteString(", median_commander_turn=" + reprOptNumber(s.MedianCommanderTurn))
	b.WriteString(", never_cast_commander=" + ReprFloat(s.NeverCastCommander))
	b.WriteString(", avg_lands_by_turn=" + reprFloats(s.AvgLandsByTurn))
	b.WriteString(", avg_mana_by_turn=" + reprFloats(s.AvgManaByTurn))
	b.WriteString(", avg_unused_by_turn=" + reprFloats(s.AvgUnusedByTurn))
	b.WriteString(", avg_spells_by_turn=" + reprFloats(s.AvgSpellsByTurn))
	b.WriteString(", color_screw_rate=" + ReprFloat(s.ColorScrewRate))
	parts := make([]string, 0, len(s.CardTimings))
	for _, t := range s.CardTimings {
		parts = append(parts, t.Repr())
	}
	b.WriteString(", card_timings=[" + strings.Join(parts, ", ") + "]")
	b.WriteString(", missed_drop_by_turn=" + reprFloats(s.MissedDropByTurn))
	b.WriteString(", median_first_spell_turn=" + reprOptFloat(s.MedianFirstSpellTurn))
	b.WriteString(", avg_stalled_turns=" + ReprFloat(s.AvgStalledTurns))
	b.WriteString(")")
	return b.String()
}

// reprCommanderByTurn renders the by-turn table in the order it was built,
// which is 1..turns -- the loop that fills it counts up, and the recorded
// rendering keeps that insertion order.
func (s SimSummary) reprCommanderByTurn() string {
	parts := make([]string, 0, s.Turns)
	for t := 1; t <= s.Turns; t++ {
		parts = append(parts, strconv.Itoa(t)+": "+ReprFloat(s.CommanderByTurn[t]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func reprFirstCast(f FirstCast) string {
	parts := make([]string, 0, f.Len())
	for _, name := range f.Names() {
		turn, _ := f.Get(name)
		parts = append(parts, ReprString(name)+": "+strconv.Itoa(turn))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func reprInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func reprFloats(xs []float64) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = ReprFloat(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func reprBools(xs []bool) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = "False"
		if x {
			parts[i] = "True"
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func reprOptInt(v *int) string {
	if v == nil {
		return "None"
	}
	return strconv.Itoa(*v)
}

func reprOptFloat(v *float64) string {
	if v == nil {
		return "None"
	}
	return ReprFloat(*v)
}

func reprOptNumber(v *Number) string {
	switch {
	case v == nil:
		return "None"
	case v.IsFloat:
		return ReprFloat(v.Float)
	default:
		return strconv.Itoa(v.Int)
	}
}

// ReprFloat is the canonical float rendering, now `floats.Repr`.
//
// It left this package when the Forge result needed the same renderer: a
// payload's float fields must read `4.0` where the recorded wire says
// `4.0`, and a
// second copy of the exponent boundaries is how two halves of a reproduction
// drift. The corpus that pins those boundaries did not move.
func ReprFloat(v float64) string { return floats.Repr(v) }

// ReprString is the canonical string rendering.
//
// Single quotes, unless the string holds one and no double quote. Backslash,
// the chosen quote and the three whitespace escapes go out as escapes;
// anything unprintable goes out as `\xNN`, `\uXXXX` or `\UXXXXXXXX`.
//
// One narrowing, stated because it is a narrowing: printability is Go's
// `unicode.IsPrint`, one reading of the Unicode categories the recorded
// rendering read. The two agree on
// everything a card name or a keep-rule sentence has ever held, and the
// corpus in testdata checks the strings this package actually renders rather
// than asserting the general claim.
func ReprString(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x80 || unicode.IsPrint(r):
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x10000:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}
