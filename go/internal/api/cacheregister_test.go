package api

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// The cache register: the three hit counters read out of a running instance.
//
// The rule they serve is the one a correctness test cannot: a cache can be
// right, green, and never once consulted. Counting is half of it; this route
// is the other half, because a counter nobody reads off a live box proves
// nothing about the box.
//
// What is pinned here is the *shape and the sourcing*, and in particular the
// difference between a row of zeros and a row of null. Zeros say "this cache
// exists in this process and has never answered", which is a bug to chase;
// null says "there is no such cache here", which is an ordinary state -- an
// API with no door above it, a laptop with no app.db -- and reading one as
// the other is how the register would lie in the only direction that matters.
func TestTheSystemViewReportsTheThreeCacheRegisters(t *testing.T) {
	t.Parallel()
	rig := newStatsRig(t)

	// The rig is an API with no door and no simulation cache: two of three
	// sources are genuinely absent, and the third is present and unused.
	payload := rig.get(t, "/api/admin/stats/system")
	for _, key := range []string{"tier1", "etag"} {
		if got := nested(t, payload, "caches", key); got != nil {
			t.Errorf("caches.%s is %v on an instance that has no such cache, want null", key, got)
		}
	}
	for _, key := range []string{"hits", "misses"} {
		if got := nested(t, payload, "caches", "shelf", key); got != float64(0) {
			t.Errorf("caches.shelf.%s is %v on an untouched memo, want 0", key, got)
		}
	}

	// Now give the same API the two missing sources. The ETag memo arrives as
	// a reader because the door cannot be imported here and would not exist
	// yet if it could -- `door.New` builds its static site before this API,
	// and `TestTheDoorHandsItsETagRegisterToTheStatsRoute` holds the real
	// hand-off; what this asserts is that whatever is handed in is what is
	// reported, rather than a zero the route invented.
	rig.api.etagCounts = func() (int, int) { return 7, 3 }

	store, err := cache.Open(rig.api.dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	rig.api.simCache = store

	// A real miss and a real hit, through the store's own verbs: the numbers
	// below are the cache's own count of work it did and work it skipped, not
	// a value set by the test.
	ctx := context.Background()
	if hit := store.Get(ctx, "a key nothing ever stored"); hit != nil {
		t.Fatal("an empty cache answered a key it never stored")
	}
	store.Put(ctx, "tier1-register-probe", "tier1", struct{ N int }{1})
	if hit := store.Get(ctx, "tier1-register-probe"); hit == nil {
		t.Fatal("a stored result was not found again")
	}

	payload = rig.get(t, "/api/admin/stats/system")
	for _, row := range []struct {
		name         string
		hits, misses float64
	}{
		{"tier1", 1, 1},
		{"etag", 7, 3},
	} {
		if got := nested(t, payload, "caches", row.name, "hits"); got != row.hits {
			t.Errorf("caches.%s.hits is %v, want %v", row.name, got, row.hits)
		}
		if got := nested(t, payload, "caches", row.name, "misses"); got != row.misses {
			t.Errorf("caches.%s.misses is %v, want %v", row.name, got, row.misses)
		}
	}
	// And the shelf row has not moved: nothing visited the shelf, so a row
	// that now reads 1/1 would be a register wired to the wrong counter --
	// three rows naming three sources is the only thing making it a register
	// rather than one number printed three times.
	for _, key := range []string{"hits", "misses"} {
		if got := nested(t, payload, "caches", "shelf", key); got != float64(0) {
			t.Errorf("caches.shelf.%s is %v after only the sim cache was used, want 0", key, got)
		}
	}
}
