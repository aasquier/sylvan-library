package pool

import "time"

// The knobs the external tests turn: a short lease, and a look at the lease
// count and the memo. Test-only, so the package's surface stays the app's.

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
