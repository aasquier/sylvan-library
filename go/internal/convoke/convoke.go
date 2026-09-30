// Package convoke runs one job's independent pieces on several hands at
// once -- convoking a spell, in this house's vocabulary: many small
// contributions, one cast.
//
// It exists for the sweeps. A land sweep or a keep-rule grid is N calls of
// tier1.Run, and every call seeds its own generator from the same recorded
// seed, so the pieces share nothing and the answer cannot depend on the
// order they finish. [Indexed] keeps that true structurally: work is handed
// out by index and results land by index, so a caller writing `rows[i]`
// reproduces the serial loop's output byte for byte at any width.
//
// What must NOT come here is anything whose pieces share a stream -- the
// games *inside* one tier1.Run draw from one generator in sequence, so that
// loop is serial because its arithmetic is, and no worker count changes
// that. The seed promise and the frozen corpora both rest on the
// distinction.
package convoke

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// Indexed calls fn(i) for every i in [0, n), running at most `workers`
// calls at once, and returns when every call has.
//
// Zero or negative workers means one fewer than GOMAXPROCS, floored at one
// -- so a sweep on the serving instance always leaves a core answering the
// door, and a machine with two cores runs exactly the serial loop it always
// ran. One worker runs on the caller's goroutine in index order, so the
// serial case is not merely equivalent but identical.
//
// A panic inside fn stops the hand-out -- calls already in flight finish --
// and the first panic is re-raised on the caller once every worker has
// returned, so a panicking sweep still fails on the goroutine that owns the
// job. It crosses as a [*Panic] carrying the worker's own stack, because the
// crossing loses that stack otherwise and the value alone cannot name a
// frame: a nil dereference that came this way once reached a CI log as one
// line naming the test and nothing under it.
func Indexed(n, workers int, fn func(int)) {
	if n <= 0 {
		return
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0) - 1
	}
	workers = min(max(workers, 1), n)
	if workers == 1 {
		for i := range n {
			fn(i)
		}
		return
	}

	var (
		next    atomic.Int64
		stop    atomic.Bool
		wg      sync.WaitGroup
		panicMu sync.Mutex
		caught  any
	)
	call := func(i int) (ok bool) {
		defer func() {
			if p := recover(); p != nil {
				// The flag goes up first. The stack read below is slow --
				// milliseconds under coverage on CI's arm64 leg -- and while
				// it ran the other worker drained a thousand trivially cheap
				// pieces, which is "stops the hand-out" failing in public
				// (`TestAPanicReRaisesOnTheCaller`, 2026-09-29). The stack is
				// still read here, on the worker, while the frames that
				// raised it are below this one.
				stop.Store(true)
				stack := debug.Stack()
				panicMu.Lock()
				if caught == nil {
					caught = &Panic{Value: p, Stack: stack}
				}
				panicMu.Unlock()
			}
		}()
		fn(i)
		return true
	}
	for range workers {
		wg.Go(func() {
			for !stop.Load() {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				if !call(i) {
					return
				}
			}
		})
	}
	wg.Wait()
	if caught != nil {
		panic(caught)
	}
}

// Panic is a worker's panic as the caller receives it: the value fn raised
// and the stack of the goroutine that raised it, read at the recover. It
// prints as the value followed by that stack, so the `panic:` line in a log
// is followed by the frame that matters rather than by Indexed's own wait.
type Panic struct {
	Value any
	Stack []byte
}

func (p *Panic) Error() string {
	return fmt.Sprintf("%v\n\nraised on a convoke worker:\n%s", p.Value, p.Stack)
}

// Unwrap hands back the value when it was an error, so errors.Is and
// errors.As read through the crossing.
func (p *Panic) Unwrap() error {
	err, _ := p.Value.(error)
	return err
}
