package pool_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The pool's memory is keyed on the file's stamp, not on the open (Aaron,
// 2026-09-05): the reaper handing an idle pool back and a refresh rewriting
// it are different events, and only the second makes a remembered answer
// wrong. Before this rule the lore shelf paid 84x — 75.6ms instead of 0.9ms
// — on the first request after every ten-second gap, re-asking the same ~120
// names of the same unchanged file. These two tests hold the rule's two
// halves: an answer survives a hand-back, and dies with the file it was
// learned from. The ten-second lease itself is deliberately untouched —
// `mtglab data refresh` must always get in — and `TestTheLeaseHandsThePoolBack`
// keeps holding that half.

// reap hands the pool back the way the reaper does — the lease lapsed and
// the decision ran — and fails the test if the pool somehow stayed open.
func reap(t *testing.T, p *pool.Pool) {
	t.Helper()
	p.ForceLease(0, time.Now().Add(-time.Minute))
	p.ReapOnce()
	if p.Held() {
		t.Fatal("the pool was not handed back")
	}
}

func TestTheMemorySurvivesAHandBackOfAnUnchangedFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pooltest.Open(t)

	var first *pool.CardRecord
	use(t, p, func(c *pool.Conn) {
		got, err := c.GetCards(ctx, []string{"Sol Ring"})
		if err != nil {
			t.Fatal(err)
		}
		first = got["Sol Ring"]
		if first == nil {
			t.Fatal("the fixture has no Sol Ring")
		}
		if _, err := pool.Stale(ctx, c); err != nil {
			t.Fatal(err)
		}
	})
	reap(t, p)

	// The same asks after the hand-back are hits: the memoised record comes
	// back by identity, and every memo's counter says it answered from
	// memory rather than from a fresh walk of a file that never changed.
	use(t, p, func(c *pool.Conn) {
		got, err := c.GetCards(ctx, []string{"Sol Ring"})
		if err != nil {
			t.Fatal(err)
		}
		if got["Sol Ring"] != first {
			t.Fatal("the hand-back forgot the card lookup; the file never changed")
		}
		if _, err := pool.Stale(ctx, c); err != nil {
			t.Fatal(err)
		}
	})
	if hits, misses := p.Memo(pool.MemoCards); hits != 1 || misses != 1 {
		t.Fatalf("cards memo %d/%d across the hand-back, want 1 hit / 1 miss", hits, misses)
	}
	if hits, _ := p.Memo(pool.MemoStale); hits != 1 {
		t.Fatalf("staleness memo %d hits across the hand-back, want 1", hits)
	}
}

// The memory outliving the open is what makes this next rule necessary, and
// it is the half of the change with no visible symptom.
//
// While the memory died with the handle, a `Conn` that outlived its open
// could not reach it -- there was nothing there. Now there is, and it may
// describe a *newer* file than the handle that Conn is still holding: the
// stamp moved, `acquire` emptied the memory and filled it from the new file,
// and the old Conn's own queries still answer from the old one. So a Conn
// reads and teaches the memory only while it is the open the memory serves.
// `cache` and `staleness` have always asked that question; `Columns` did not
// need to until now.
//
// A hand-back rather than a refresh is used to stage it because it is the
// case that would otherwise pass: the file never changed, so the memory is
// genuinely still true, and only the gate keeps a stale handle out of it.
func TestAConnThatOutlivedItsOpenIsOutOfTheMemory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := pooltest.Open(t)

	// A Conn kept past its Use, which is what a shutdown leaves behind.
	var stray *pool.Conn
	use(t, p, func(c *pool.Conn) {
		stray = c
		if _, err := c.Columns(ctx, "printings"); err != nil {
			t.Fatal(err)
		}
	})
	hits, misses := p.Memo(pool.MemoColumns)
	if hits != 0 || misses != 1 {
		t.Fatalf("columns memo %d/%d after one ask, want 0 hits / 1 miss", hits, misses)
	}

	// Hand back and re-open. The memory stands -- same file -- and a fresh
	// Conn answers from it, which is the previous test's rule holding here.
	reap(t, p)
	use(t, p, func(c *pool.Conn) {
		if _, err := c.Columns(ctx, "printings"); err != nil {
			t.Fatal(err)
		}
	})
	if hits, _ := p.Memo(pool.MemoColumns); hits != 1 {
		t.Fatalf("columns memo %d hits after the hand-back, want 1", hits)
	}

	// The stray is holding the database that was closed under it, so its own
	// ask must reach that database and fail there -- never be answered out of
	// a memory it is no longer entitled to read. The error is the assertion:
	// an answer here is the gate gone.
	if cols, err := stray.Columns(ctx, "printings"); err == nil {
		t.Fatalf("a Conn that outlived its open answered %d columns out of "+
			"the pool's memory", len(cols))
	}
	// And it did not count as a reader of that memory either: the counters
	// instrument the memo, and a Conn that may not use it has not used it.
	if hits, misses := p.Memo(pool.MemoColumns); hits != 1 || misses != 1 {
		t.Fatalf("columns memo %d/%d after the stray's ask, want it unmoved at 1/1",
			hits, misses)
	}
}

func TestTheMemoryDiesWithTheFileItWasLearnedFrom(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := pooltest.Build(t)
	p := pool.New(path, nil)
	t.Cleanup(p.Close)

	var first *pool.CardRecord
	use(t, p, func(c *pool.Conn) {
		got, err := c.GetCards(ctx, []string{"Sol Ring"})
		if err != nil {
			t.Fatal(err)
		}
		first = got["Sol Ring"]
	})
	reap(t, p)

	// A refresh rewrites the pool file in place, which moves its mtime — the
	// stamp is (mtime_ns, size), and the mtime half alone is the signal, so
	// moving it is a faithful stand-in for the rewrite without needing a
	// second writer. (`TestAMovedPoolIsReopened` drives the whole-file swap
	// and reads the new rows; this test asks about the memory.)
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	use(t, p, func(c *pool.Conn) {
		got, err := c.GetCards(ctx, []string{"Sol Ring"})
		if err != nil {
			t.Fatal(err)
		}
		if got["Sol Ring"] == first {
			t.Fatal("a remembered answer outlived a refreshed file")
		}
	})
	// The counters died with the answers: this memory has seen exactly one
	// ask, and it was a miss.
	if hits, misses := p.Memo(pool.MemoCards); hits != 0 || misses != 1 {
		t.Fatalf("cards memo %d/%d after the refresh, want 0 hits / 1 miss", hits, misses)
	}
	if p.CacheLen() != 1 {
		t.Fatalf("the refreshed file's memo holds %d entries, want the fresh ask alone", p.CacheLen())
	}
}
