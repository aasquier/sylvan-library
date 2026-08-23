package claude

import (
	"strings"
	"time"
	"unicode/utf8"
)

// The handful of Python string and time behaviours the two searching modes
// lean on, reproduced rather than approximated -- because each one reaches
// something a person or a store sees.

// now is `datetime.now(UTC).isoformat()`: `2026-08-22T01:23:45.678901+00:00`,
// microseconds and a `+00:00` offset rather than a `Z` -- and **no fraction
// at all** when the microsecond is zero, which Python elides and a fixed
// format would not. It stamps a dossier's `generated_at` and the cache row's
// `created_at`, both of which the client renders, and it is a variable so a
// test can freeze it.
var now = func() string { return pyIsoformat(time.Now()) }

func pyIsoformat(t time.Time) string {
	t = t.UTC().Truncate(time.Microsecond)
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}

// pyIsSpace is `str.isspace()` for one character: Unicode whitespace as
// Python counts it, which is **wider than Go's `unicode.IsSpace`** by the
// four information separators U+001C..U+001F (bidirectional class B or S)
// and agrees with it everywhere else -- the ASCII five, NEL, no-break
// space, and every category-Zs space. `str.split()` and `str.strip()` both
// use this set, so a question whose only "whitespace" is a file separator is
// empty to Python and two characters to `strings.TrimSpace`.
func pyIsSpace(r rune) bool {
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

// pyStrip is `str.strip()` with no argument.
func pyStrip(s string) string { return strings.TrimFunc(s, pyIsSpace) }

// PyRStrip is `str.rstrip()` with no argument.
func PyRStrip(s string) string { return strings.TrimRightFunc(s, pyIsSpace) }

// pySplitJoin is `" ".join(s.split())`: runs of whitespace collapsed to one
// space, none at either end.
func pySplitJoin(s string) string {
	return strings.Join(strings.FieldsFunc(s, pyIsSpace), " ")
}

// PyLen is `len(s)` for a `str`: code points, not bytes. A question of 2,000
// accented characters is 2,000 to Python and well over the ceiling in bytes.
func PyLen(s string) int { return utf8.RuneCountInString(s) }

// PyHead is `s[:n]` for a `str`: the first n code points.
func PyHead(s string, n int) string {
	if PyLen(s) <= n {
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
