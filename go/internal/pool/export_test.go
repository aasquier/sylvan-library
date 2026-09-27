package pool

import (
	"database/sql"
	"time"
)

// The knobs the external tests turn: a short lease, and a look at the lease
// count and the memo. Test-only, so the package's surface stays the app's.

// ConnOver is a Conn over a handle the test brought itself.
//
// **It exists because a Pool opens its own file.** [Pool.acquire] calls [Open]
// on `p.path`, so there is no way to hand a Pool a handle that fails on
// purpose without a seam in the app's own code — and a fixture that only
// reaches the *first* statement of a read is the gap
// `pooltest.OpenFaulty` was built to close. This is that door, and it is a test
// file rather than a production field for exactly that reason: nothing the app
// ships needed changing.
//
// The Pool it carries is a bare one, which is the same honesty `onTables`
// buys: a Conn only consults its pool for the memo, `New("", nil)` has never
// opened anything, and a pool that matches no handle sends every lookup to the
// database. So a read driven through here is a read of the pool file, never of
// a remembered answer.
func ConnOver(db *sql.DB) *Conn { return &Conn{db: db, pool: New("", nil)} }

func (p *Pool) SetIdle(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.idle = d
}

func (p *Pool) ForceLease(n int, lastUsed time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.leases = n
	p.lastUsed = lastUsed
}

func (p *Pool) CacheLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cards == nil {
		return 0
	}
	return p.cards.order.Len()
}

// Memo is one memo's hits and misses since the pool file last changed under
// this pool -- the hit rate that no correctness test can stand in for.
// Exported to the tests alone: nothing in the app reads a counter, and
// commandment 10 keeps it that way.
func (p *Pool) Memo(name string) (hits, misses int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.memos[name]
	if c == nil {
		return 0, 0
	}
	return c.hits, c.misses
}
