// Package pytext is CPython's string behaviour where this port has to
// reproduce it rather than approximate it.
//
// The fifth reproduction of somebody else's work in this module, after
// `pyrand` (CPython's Mersenne Twister), `pyyaml` (PyYAML's emitter),
// `pyfloat` (`math.fsum` and both rounds) and `pycasefold` (full case
// folding). It exists for the same reason they do: a Go standard-library
// function that is *nearly* the Python one is a divergence that shows up as a
// wrong answer on somebody's screen rather than as a failure anywhere.
//
// Two behaviours live here, and both are places `strings` is not `str`:
//
//   - **`str.strip()` counts four characters as whitespace that Go does
//     not** — the information separators U+001C..U+001F. `strings.TrimSpace`
//     leaves them, so a string whose only padding is a file separator is
//     empty to Python and two characters to Go.
//   - **`str.splitlines()` splits on eight boundaries `strings.Split(s,
//     "\n")` does not** — U+000B, U+000C, U+001C..U+001E, U+0085, U+2028 and
//     U+2029. A Forge log line carrying one splits in two under Python and
//     stays whole under Go, which turns one game result into none.
//
// `internal/claude/text.go` was where these first landed, for the two
// searching modes; they moved here when `sim/tier3` needed the same set and
// the alternative was a second copy of the whitespace table. The rule the
// repo already learned from `pyfloat`: an arithmetic reproduction belongs in
// its own package the moment a second family needs it.
package textutil

import (
	"strings"
	"time"
	"unicode/utf8"
)

// IsSpace is `str.isspace()` for one character: Unicode whitespace as Python
// counts it, which is wider than Go's `unicode.IsSpace` by the four
// information separators U+001C..U+001F (bidirectional class B or S) and
// agrees with it everywhere else — the ASCII five, NEL, no-break space, and
// every category-Zs space. `str.split()` and `str.strip()` both use this set.
func IsSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r',
		0x1c, 0x1d, 0x1e, 0x1f,
		0x85, 0xa0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007,
		0x2008, 0x2009, 0x200a,
		0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return false
}

// Strip is `str.strip()` with no argument.
func Strip(s string) string { return strings.TrimFunc(s, IsSpace) }

// RStrip is `str.rstrip()` with no argument.
func RStrip(s string) string { return strings.TrimRightFunc(s, IsSpace) }

// SplitJoin is `" ".join(s.split())`: runs of whitespace collapsed to one
// space, none at either end.
func SplitJoin(s string) string {
	return strings.Join(strings.FieldsFunc(s, IsSpace), " ")
}

// Len is `len(s)` for a `str`: code points, not bytes.
func Len(s string) int { return utf8.RuneCountInString(s) }

// Head is `s[:n]` for a `str`: the first n code points.
func Head(s string, n int) string {
	if Len(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// lineBoundaries is every character `str.splitlines()` treats as a line
// break, minus `\r` and `\n`, which the loop handles together so that a CRLF
// counts once.
//
// Read off CPython's `unicodedata` rather than recalled: the set is the ASCII
// five (U+000A..U+000D plus U+001C..U+001E), U+0085, U+2028 and U+2029.
// U+001F is deliberately absent — it is whitespace to `strip` and *not* a line
// boundary, which is exactly the kind of near-miss this package exists for.
func lineBoundaries(r rune) bool {
	switch r {
	case '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// SplitLines is `str.splitlines()`: the string broken at every Unicode line
// boundary, with the terminators dropped and — unlike `strings.Split` — no
// empty final element for a string that ends in one.
func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '\r':
			out = append(out, s[start:i])
			i += size
			// CRLF is one boundary, not two.
			if i < len(s) && s[i] == '\n' {
				i++
			}
			start = i
		case r == '\n' || lineBoundaries(r):
			out = append(out, s[start:i])
			i += size
			start = i
		default:
			i += size
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// Isoformat is `datetime.now(UTC).isoformat()`:
// `2026-08-22T01:23:45.678901+00:00` — microseconds and a `+00:00` offset
// rather than a `Z`, and **no fraction at all** when the microsecond is zero,
// which Python elides and a fixed format does not.
//
// That elision is not decoration: the job corpus found it as a real
// divergence, and `created_at` is a sort key as *text* in more than one table
// here.
//
// It is a **three-way inconsistency in this module**, which is the honest way
// to say it. `internal/jobs` branches on the nanosecond and gets it right, and
// has a Python-driven corpus case for it. `internal/decklog` and
// `internal/claude/ledger` still spell a fixed six-digit layout and so differ
// from Python on one row in a million. The reason both survived is the reason
// worth carrying: **neither corpus holds a zero-microsecond row** — the same
// shape as a `TrimSpace`-for-`str.strip` mutant that lives through every
// corpus with no separator character in it. The corpus row is the work; the
// two lines are trivia.
func Isoformat(t time.Time) string {
	t = t.UTC().Truncate(time.Microsecond)
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}
