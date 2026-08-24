package claude

import (
	"time"

	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// now stamps this package's timestamps: `2026-08-22T01:23:45.678901+00:00`,
// microseconds and a `+00:00` offset rather than a `Z` -- and **no fraction
// at all** when the microsecond is zero, which [textutil.Isoformat] elides and a
// fixed format would not. It stamps a dossier's `generated_at` and the cache
// row's `created_at`, both of which the client renders, and it is a variable
// so a test can freeze it.
var now = func() string { return textutil.Isoformat(time.Now()) }
