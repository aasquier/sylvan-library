package claude

import (
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pytext"
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

func pyIsoformat(t time.Time) string { return pytext.Isoformat(t) }

// pyIsSpace is `str.isspace()` for one character. It lives in
// `internal/pytext` now, with `str.splitlines()` beside it: `sim/tier3`
// needed the same whitespace table for Forge's log, and a second copy of a
// table this exact is how the two halves of a reproduction drift apart.
func pyIsSpace(r rune) bool { return pytext.IsSpace(r) }

// PyStrip is `str.strip()`, exported for `internal/api`: the argue sweep
// strips names somebody typed, and Python's whitespace set is not Go's --
// U+001C-U+001F are whitespace to `str.strip()` and not to
// `strings.TrimSpace`.
func PyStrip(s string) string { return pyStrip(s) }

// pyStrip is `str.strip()` with no argument.
func pyStrip(s string) string { return pytext.Strip(s) }

// PyRStrip is `str.rstrip()` with no argument.
func PyRStrip(s string) string { return pytext.RStrip(s) }

// pySplitJoin is `" ".join(s.split())`: runs of whitespace collapsed to one
// space, none at either end.
func pySplitJoin(s string) string { return pytext.SplitJoin(s) }

// PyLen is `len(s)` for a `str`: code points, not bytes. A question of 2,000
// accented characters is 2,000 to Python and well over the ceiling in bytes.
func PyLen(s string) int { return pytext.Len(s) }

// PyHead is `s[:n]` for a `str`: the first n code points.
func PyHead(s string, n int) string { return pytext.Head(s, n) }
