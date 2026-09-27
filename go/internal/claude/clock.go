package claude

import (
	"time"

	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// Clock is where a report's timestamp comes from, as a value.
//
// The stamps are `2026-08-22T01:23:45.678901+00:00`: microseconds and a
// `+00:00` offset rather than a `Z` -- and **no fraction at all** when the
// microsecond is zero, which [textutil.Isoformat] elides and a fixed format
// would not. They reach a dossier's `generated_at`, a research report's, and
// the cache row's `created_at`, all three of which the client renders.
//
// **A parameter rather than a package variable**, and the difference is the
// whole reason this type exists. The recorded corpora carry a frozen stamp,
// so every test that compares a report byte for byte has to hold the clock
// still; while the clock was a package-level `var now`, holding it still meant
// *writing* it, which `-race` reports and which kept four tests out of the
// parallel set. Nil is the system clock, so nothing that does not care has to
// say anything.
type Clock func() string

// stamp is this clock's reading, or the system's when nobody handed one down.
//
// A method on a nil-able func type, so the fallback lives in exactly one
// place and a caller never writes `if clock == nil` for itself.
func (c Clock) stamp() string {
	if c == nil {
		return textutil.Isoformat(time.Now())
	}
	return c()
}
