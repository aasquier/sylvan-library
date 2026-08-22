package tier1

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Python's `repr`, for the three dataclasses the determinism gate hashes.
//
// `tests/test_determinism.py` pins REFERENCE_DIGEST as a sha256 over
// `repr()` of one game, one run and a three-point sweep. So reproducing the
// simulation is only half of reproducing the digest: the *text* is what is
// hashed, and Python's number formatting is part of it. `100.0` is not
// `100`, `1e-05` is not `0.00001`, and a float renders as the shortest
// decimal that reads back as the same double.
//
// Nothing serves these strings. They exist so a Go run and a CPython run can
// be compared as bytes rather than field by field, which is what turns "the
// port looks right" into "the port is the same simulator".
//
// The float renderer is held to Python by a corpus in testdata: every float
// the reference run produces, plus the boundaries where Python switches
// between fixed and exponential notation, recorded from CPython itself.

// Repr is `repr(GameResult)`.
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

// Repr is `repr(CardTiming)`.
func (c CardTiming) Repr() string {
	return "CardTiming(name=" + ReprString(c.Name) +
		", mv=" + strconv.Itoa(c.MV) +
		", cast_rate=" + ReprFloat(c.CastRate) +
		", median_turn=" + reprOptFloat(c.MedianTurn) +
		", by_t8=" + ReprFloat(c.ByT8) + ")"
}

// Repr is `repr(SimSummary)`.
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

// reprCommanderByTurn renders `dict[int, float]` in the order Python built
// it, which is 1..turns -- the loop that fills it counts up, and a CPython
// dict keeps insertion order.
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

// ReprFloat is CPython's `repr(float)`.
//
// The shortest decimal that round-trips, rendered fixed when the decimal
// point sits within reach and exponential when it does not: Python switches
// at a decimal exponent below -3 or above 16, so `1e16` reads `1e+16` while
// `1e15` reads `1000000000000000.0`, and `0.0001` stays fixed while
// `0.00001` becomes `1e-05`. A fixed rendering always carries a decimal
// point (`100.0`, never `100`); an exponential one never gains a spurious
// `.0`, and its exponent is at least two digits.
//
// Go's shortest formatting supplies the digits; only the presentation is
// Python's. The boundaries above are pinned by a corpus taken from CPython
// rather than trusted from this comment.
func ReprFloat(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	}
	sign := ""
	if math.Signbit(v) {
		sign, v = "-", -v
	}
	if v == 0 {
		return sign + "0.0"
	}

	// Shortest round-trip digits, and where the decimal point falls.
	s := strconv.FormatFloat(v, 'e', -1, 64)
	epos := strings.IndexByte(s, 'e')
	digits := strings.Replace(s[:epos], ".", "", 1)
	exp, err := strconv.Atoi(s[epos+1:])
	if err != nil {
		panic("tier1: unreadable float exponent in " + s)
	}
	decpt := exp + 1

	if decpt < -3 || decpt > 16 {
		out := digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		e, esign := decpt-1, "+"
		if e < 0 {
			e, esign = -e, "-"
		}
		return fmt.Sprintf("%s%se%s%02d", sign, out, esign, e)
	}
	switch {
	case decpt <= 0:
		return sign + "0." + strings.Repeat("0", -decpt) + digits
	case decpt >= len(digits):
		return sign + digits + strings.Repeat("0", decpt-len(digits)) + ".0"
	default:
		return sign + digits[:decpt] + "." + digits[decpt:]
	}
}

// ReprString is CPython's `repr(str)`.
//
// Single quotes, unless the string holds one and no double quote. Backslash,
// the chosen quote and the three whitespace escapes go out as escapes;
// anything unprintable goes out as `\xNN`, `\uXXXX` or `\UXXXXXXXX`.
//
// One narrowing, stated because it is a narrowing: printability is Go's
// `unicode.IsPrint`, which is that language's reading of the same Unicode
// categories CPython's `Py_UNICODE_ISPRINTABLE` reads. They agree on
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
