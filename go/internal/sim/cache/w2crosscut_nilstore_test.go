package cache_test

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// A store that was never attached says so, and the two sentences `enabled`
// exists to tell apart are the whole of this file.
//
// **The bug this holds shut was a fallback reading as a fact.** `Stats` built
// its `Enabled` field off the engine fingerprint -- a fact about the embedded
// code, true on any machine that can read itself -- on the line *before* it
// looked at whether there was a handle at all. So a nil store, which is how
// `mtglab sim cache` represents "there is no `app.db` here, or I could not
// open it", answered `enabled: yes` under a row count of zero. A reader of
// that pairing has exactly two readings available -- the cache is on and
// empty, or the cache is off -- and the command printed the first while
// meaning the second.
//
// Three states, because the fix is only honest if the other two keep their
// answers: a store with a live handle reports the fingerprint's yes
// (`TestStatsAndClear`), a store whose handle has been closed *also* reports
// yes because the cache is configured and merely unreachable
// (`closedstore_test.go` argues that at length), and a store that is not
// there reports no. Only the third moved.
func TestAStoreThatWasNeverAttachedReportsTheCacheOff(t *testing.T) {
	t.Parallel()

	var absent *cache.Store
	stats := absent.Stats(t.Context())
	if stats.Enabled {
		t.Error("a nil store reported the cache enabled -- there is nothing " +
			"there to cache into, and `enabled: yes` beside `rows: 0` reads " +
			"as an empty cache that is working")
	}
	if stats.Rows != 0 || stats.Bytes != 0 || len(stats.ByKind) != 0 {
		t.Errorf("a nil store reported %+v", stats)
	}
	if stats.Oldest != nil || stats.Newest != nil {
		t.Errorf("a nil store dated its contents: %+v", stats)
	}

	// The map is built rather than left nil, because the command ranges over
	// it and a caller serialising this answers `{}` rather than `null`.
	if stats.ByKind == nil {
		t.Error("a nil store answered a nil by-kind map")
	}

	// And the fingerprint really is non-empty in this binary, so the
	// assertion above is about the nil check rather than about a tree that
	// could not read its own sources.
	if cache.Fingerprint() == "" {
		t.Fatal("the engine fingerprint is empty in this binary, so " +
			"`enabled: no` above proves nothing about the nil store")
	}
	if live := scratch(t).Stats(t.Context()); !live.Enabled {
		t.Error("an attached store reported the cache disabled, so the nil " +
			"check above could be reading a hard-coded no")
	}
}
