package pool

import (
	"context"
	"errors"
	"time"
)

// The seal: how this process lets go of the library so that this process can
// rewrite it.
//
// **A refresh started from inside the serving process cannot simply open the
// writer, and the reason is measured rather than reasoned.** DuckDB caches
// its database instance per file *inside* one process, so a second open of a
// file this process already holds read-only does not take the cross-process
// lock and wait -- it fails outright, with `Can't open a connection to same
// database file with a different configuration`. That is not a message
// [Locked] recognises, so [OpenWriterWaiting]'s patience would never even be
// spent: the refresh would fail in the first millisecond with a sentence
// about configuration. (`writerlock_test.go` records the same fact from the
// other side -- it copies the fixture to a path the parent has never opened,
// precisely so its conflict is a real cross-process one.)
//
// So the door has to hand the file back first, and stay away from it until
// the rewrite is done. That is what a seal is. It is deliberately **not** a
// lock a reader waits on: a card lookup during a refresh answers
// [ErrNoPool] and degrades, which is the shape ADR 6 chose ("card lookups
// are briefly unavailable *during* a refresh ... by design") and the shape
// every read path in the app already handles.

// SealPoll is how often a seal asks whether the last reader has gone. Short
// enough that an idle instance is sealed in the time a click takes, long
// enough not to spin.
const SealPoll = 50 * time.Millisecond

// ErrSealed is a second seal asked for while the first is still on. One
// rewrite at a time is the whole point of the thing.
var ErrSealed = errors.New("the library is already sealed for a rewrite")

// ErrStillReading is a seal that waited out its budget with somebody still
// holding the pool.
//
// **A budget rather than patience without end**, because the lease is not the
// only thing that holds this file: a Tier 1 sweep holds one for as long as it
// runs. Waiting forever would turn "refresh the library" into a control that
// hangs; saying the library is busy is the honest answer and the one the
// operator can act on.
var ErrStillReading = errors.New("the library is being read and did not come free")

// Seal shuts the pool and keeps it shut until the returned reopen is called.
//
// It waits for readers already inside to finish -- a lease is held for the
// length of one query, or of one simulation -- and then closes the file, so
// nothing is ever closed out from under a running read. New readers are
// refused from the moment Seal is *called*, not from the moment it succeeds,
// which is what makes the wait terminate: a busy instance drains rather than
// being continuously topped up.
//
// The returned reopen is idempotent and never nil on success. On failure
// nothing is sealed and there is nothing to undo.
func (p *Pool) Seal(ctx context.Context, budget time.Duration) (func(), error) {
	p.mu.Lock()
	if p.sealed {
		p.mu.Unlock()
		return nil, ErrSealed
	}
	p.sealed = true
	p.mu.Unlock()

	var once bool
	reopen := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if once {
			return
		}
		once = true
		p.sealed = false
	}

	deadline := time.Now().Add(budget)
	for {
		p.mu.Lock()
		if p.leases == 0 {
			// Closed under the same lock that refuses new readers, so there is
			// no window where the file is open and unsealed.
			p.closeLocked()
			p.mu.Unlock()
			return reopen, nil
		}
		p.mu.Unlock()

		if !time.Now().Before(deadline) {
			reopen()
			return nil, ErrStillReading
		}
		select {
		case <-ctx.Done():
			reopen()
			return nil, ctx.Err()
		case <-time.After(SealPoll):
		}
	}
}

// Sealed reports whether the library is shut for a rewrite right now.
func (p *Pool) Sealed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sealed
}
