// Package textutil is the app's recorded string semantics where the
// standard library's are *nearly* right -- and "nearly" is the problem: a
// function that is nearly the recorded one is a divergence that shows up as
// a wrong answer on somebody's screen rather than as a failure anywhere.
//
// Two tables carry most of it, and both are places `strings` gives a
// different answer:
//
//   - **the whitespace set counts four characters `strings.TrimSpace` does
//     not** -- the information separators U+001C..U+001F. A string whose
//     only padding is a file separator strips empty here and keeps two
//     characters there.
//   - **`SplitLines` splits on eight boundaries `strings.Split(s, "\n")`
//     does not** -- U+000B, U+000C, U+001C..U+001E, U+0085, U+2028 and
//     U+2029. A Forge log line carrying one splits in two here and stays
//     whole there, which turns one game result into none.
//
// These first landed beside the two searching modes; they moved here when
// `sim/tier3` needed the same set and the alternative was a second copy of
// the whitespace table. The rule the repo already learned from `floats`: an
// exact-behaviour table belongs in its own package the moment a second
// family needs it. Held to the frozen corpus in `testdata/`, which is never
// regenerated: the recorded boundaries are the contract.
package textutil

import (
	"strings"
	"time"
	"unicode/utf8"
)

// IsSpace is the recorded whitespace predicate for one character: wider
// than `unicode.IsSpace` by the four information separators U+001C..U+001F
// (bidirectional class B or S) and agreeing with it everywhere else — the
// ASCII five, NEL, no-break space, and every category-Zs space. [Strip] and
// [SplitJoin] both use this set.
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

// Strip trims the recorded whitespace set from both ends.
func Strip(s string) string { return strings.TrimFunc(s, IsSpace) }

// RStrip trims the recorded whitespace set from the right end only.
func RStrip(s string) string { return strings.TrimRightFunc(s, IsSpace) }

// SplitJoin collapses every run of recorded whitespace to one space, with
// none at either end.
func SplitJoin(s string) string {
	return strings.Join(strings.FieldsFunc(s, IsSpace), " ")
}

// Len counts code points, not bytes.
func Len(s string) int { return utf8.RuneCountInString(s) }

// Head is the first n code points.
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

// lineBoundaries is every character [SplitLines] treats as a line break,
// minus `\r` and `\n`, which the loop handles together so that a CRLF
// counts once.
//
// Recorded from the Unicode data rather than recalled: the set is the ASCII
// five (U+000A..U+000D plus U+001C..U+001E), U+0085, U+2028 and U+2029.
// U+001F is deliberately absent — it is whitespace to [Strip] and *not* a
// line boundary, which is exactly the kind of near-miss this package exists
// for.
func lineBoundaries(r rune) bool {
	switch r {
	case '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// SplitLines breaks the string at every Unicode line boundary, with the
// terminators dropped and — unlike `strings.Split` — no empty final element
// for a string that ends in one.
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

// Isoformat is the app's timestamp:
// `2026-08-22T01:23:45.678901+00:00` — microseconds and a `+00:00` offset
// rather than a `Z`, and **no fraction at all** when the microsecond is
// zero, which a fixed format would not elide.
//
// That elision is not decoration: the job corpus found it as a real
// divergence, and `created_at` is a sort key as *text* in more than one
// table here.
//
// It is a **three-way inconsistency in this module**, which is the honest
// way to say it. `internal/jobs` branches on the nanosecond and gets it
// right, and has a corpus case for it. `internal/decklog` and
// `internal/claude/ledger` still spell a fixed six-digit layout and so
// write a different byte on one row in a million. The reason both survived
// is the reason worth carrying: **neither corpus holds a zero-microsecond
// row** — the same shape as a TrimSpace-for-[Strip] mutant that lives
// through every corpus with no separator character in it. The corpus row is
// the work; the two lines are trivia.
func Isoformat(t time.Time) string {
	t = t.UTC().Truncate(time.Microsecond)
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}
