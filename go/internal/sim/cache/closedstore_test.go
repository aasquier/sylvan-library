package cache_test

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// The store when the database underneath it has gone.
//
// **A cache failure is never a failure.** Every method here is on the path of
// a simulation somebody is waiting for, and the whole point of ADR 18's cache
// is that it makes an answer faster, not that it makes one possible. So a
// store whose file has been closed, deleted or locked has exactly one correct
// behaviour: report a miss, log, and let the caller compute. A `Get` that
// returned an error would push that decision onto every call site; a `Put`
// that panicked would take down a request that had already succeeded.
//
// That is easy to write and easy to get wrong, because the failing paths are
// the ones nothing normally drives -- a working store never takes them. The
// lever is the one `internal/auth/errorpaths_test.go` uses: close the handle
// and call everything. The assertion is not the message, which is SQLite's
// wording rather than ours, but that nothing claims success and nothing dies.

// closed is a real store, opened over a real migrated database, whose handle
// has then been closed underneath it. Every query fails; nothing is nil.
func closed(t *testing.T) *cache.Store {
	t.Helper()
	store := scratch(t)
	if err := store.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}
	return store
}

func TestEveryStoreMethodSurvivesAClosedDatabase(t *testing.T) {
	t.Parallel()
	store := closed(t)
	ctx := context.Background()

	if hit := store.Get(ctx, "a-key"); hit != nil {
		t.Errorf("a closed store answered a hit: %+v", hit)
	}
	// Put has no return value at all, which is the design: a caller cannot
	// act on a cache write failing. What it must not do is panic.
	store.Put(ctx, "a-key", "tier1", result{Games: 1, Rate: 0.5})
	if hit := store.Get(ctx, "a-key"); hit != nil {
		t.Error("a write to a closed store came back on the next read")
	}

	stats := store.Stats(ctx)
	if stats.Rows != 0 || len(stats.ByKind) != 0 {
		t.Errorf("a closed store reported %+v", stats)
	}
	// `Enabled` is a fact about the engine fingerprint rather than about the
	// database, so it stays true -- the cache is configured, it just cannot
	// answer right now. Reporting it disabled would say the wrong thing in
	// `mtglab sim cache`.
	if !stats.Enabled {
		t.Error("a closed store reported the cache disabled -- that is a fact " +
			"about the fingerprint, not about this handle")
	}

	if n := store.Clear(ctx); n != 0 {
		t.Errorf("a closed store cleared %d rows", n)
	}
	// Closing twice is what a deferred close after an explicit one does.
	if err := store.Close(); err != nil {
		t.Errorf("closing a closed store: %v", err)
	}
}

// **A nil store is a working store that caches nothing.** `api.Config` says so
// in as many words -- an instance with no `app.db` passes nil and no caller
// branches on it -- so the nil receiver is load-bearing rather than defensive,
// and a nil-pointer panic here would take out every simulation on a fresh
// machine.
func TestANilStoreIsAWorkingStoreThatCachesNothing(t *testing.T) {
	t.Parallel()
	var store *cache.Store
	ctx := context.Background()

	if hit := store.Get(ctx, "a-key"); hit != nil {
		t.Errorf("a nil store answered a hit: %+v", hit)
	}
	store.Put(ctx, "a-key", "tier1", result{Games: 1})
	if db := store.DB(); db != nil {
		t.Error("a nil store handed out a handle")
	}
	if stats := store.Stats(ctx); stats.Rows != 0 {
		t.Errorf("a nil store reported %d rows", stats.Rows)
	}
	if n := store.Clear(ctx); n != 0 {
		t.Errorf("a nil store cleared %d rows", n)
	}
	if err := store.Close(); err != nil {
		t.Errorf("closing a nil store: %v", err)
	}
}

// A store over a path with no database on it: `Open` must refuse rather than
// mint one. The door's rule is that a reader never acquires a database, and
// the cache is opened on the read path.
func TestOpeningAStoreOverNothingIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := cache.Open(t.TempDir()+"/not-here.db", nil); err == nil {
		t.Fatal("a store was opened over a database that is not there")
	}
}
