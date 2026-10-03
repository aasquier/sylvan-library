package convoke_test

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/convoke"
)

// Every index is visited exactly once at every width worth suspecting:
// serial, narrower than the work, wider than the work, and the auto width.
func TestEveryIndexRunsExactlyOnce(t *testing.T) {
	t.Parallel()
	for _, workers := range []int{1, 2, 3, 7, 64, 0, -1} {
		const n = 41
		counts := make([]atomic.Int32, n)
		convoke.Indexed(n, workers, func(i int) {
			counts[i].Add(1)
		})
		for i := range counts {
			if got := counts[i].Load(); got != 1 {
				t.Fatalf("workers=%d: index %d ran %d times", workers, i, got)
			}
		}
	}
}

// The answer is the serial answer at any width: results land by index, so a
// computation assembled through Indexed matches the plain loop element for
// element.
func TestResultsLandByIndexNotByFinishOrder(t *testing.T) {
	t.Parallel()
	const n = 100
	want := make([]int, n)
	for i := range want {
		want[i] = i * i
	}
	got := make([]int, n)
	convoke.Indexed(n, 8, func(i int) {
		got[i] = i * i
	})
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

// Nothing to do is a return, not a hang and not a call.
func TestZeroWorkIsANoOp(t *testing.T) {
	t.Parallel()
	called := false
	convoke.Indexed(0, 4, func(int) { called = true })
	convoke.Indexed(-3, 4, func(int) { called = true })
	if called {
		t.Fatal("fn ran for n <= 0")
	}
}

// A panic in one piece surfaces on the caller -- the goroutine that owns
// the job -- with the original value, and the hand-out stops rather than
// grinding through the rest of the grid.
func TestAPanicReRaisesOnTheCaller(t *testing.T) {
	t.Parallel()
	var ran atomic.Int32
	defer func() {
		p := recover()
		if p == nil {
			t.Fatal("the panic did not cross back to the caller")
		}
		pp, ok := p.(*convoke.Panic)
		if !ok {
			t.Fatalf("re-raised %T, want a *convoke.Panic carrying the value", p)
		}
		if s, ok := pp.Value.(string); !ok || s != "tier1: Run needs at least one game" {
			t.Fatalf("re-raised %v, want the original value", pp.Value)
		}
		// The stack is the worker's, read where the panic happened: it names
		// the fn this test handed in, which the caller's own stack -- Indexed
		// waiting on its group -- never could.
		if !strings.Contains(string(pp.Stack), "TestAPanicReRaisesOnTheCaller") {
			t.Fatalf("the stack does not name the frame that panicked:\n%s", pp.Stack)
		}
		// And the printed form carries both, because `panic:` in a log prints
		// Error() and nothing else.
		if msg := pp.Error(); !strings.Contains(msg, "tier1: Run needs at least one game") ||
			!strings.Contains(msg, "TestAPanicReRaisesOnTheCaller") {
			t.Fatalf("Error() = %q, want the value and the stack", msg)
		}
		// The stop flag halts hand-out; with 2 workers over 1000 pieces a
		// panic at index 3 must leave most of the grid untouched.
		if n := ran.Load(); n >= 1000 {
			t.Fatalf("all %d pieces ran despite the panic", n)
		}
	}()
	// **Every piece but the one that panics costs a millisecond, and the
	// assertion above is unmeasurable without it.** The flag goes up inside
	// the worker's recover, which is a panic unwind away from the `panic`
	// statement -- microseconds, more under coverage instrumentation -- and
	// an atomic increment is nanoseconds, so the other worker could drain
	// all 996 remaining pieces inside that window and "stops the hand-out"
	// failed in public on CI's arm64 leg with the mechanism working exactly
	// as designed. A millisecond a piece turns that window from a race at
	// even odds into a second of margin against a flag that lands in
	// microseconds; the test still finishes in a few milliseconds, because
	// the stop is what keeps the second worker from ever reaching the sleep
	// a thousand times.
	convoke.Indexed(1000, 2, func(i int) {
		ran.Add(1)
		if i == 3 {
			panic("tier1: Run needs at least one game")
		}
		time.Sleep(time.Millisecond)
	})
}

// One worker means the caller's goroutine in index order -- the serial loop
// itself, not an equivalent of it.
func TestOneWorkerRunsInOrderOnTheCaller(t *testing.T) {
	t.Parallel()
	var order []int
	convoke.Indexed(5, 1, func(i int) {
		order = append(order, i) // no lock: single goroutine is the contract
	})
	for i, got := range order {
		if got != i {
			t.Fatalf("order %v, want ascending", order)
		}
	}
	if len(order) != 5 {
		t.Fatalf("ran %d of 5", len(order))
	}
}

// A worker that panics with an *error* crosses back as something
// `errors.Is` and `errors.As` can read, which is what [convoke.Panic.Unwrap]
// is for and what no test asked of it: every panic a test raised here was a
// string, so the crossing had never been asked whether it preserves an
// error's identity.
//
// Why it matters more than the coverage. A sweep is N calls of a kernel, and
// a kernel refuses by panicking on a programming error (`tier1.Run` on zero
// games is the real one). A caller that recovers and branches on
// `errors.Is(err, SomeSentinel)` reads false through an Unwrap that answers
// nil — so the refusal arrives as "a panic" rather than as the named fault,
// in the one place a stack trace was already going to be hard to read.
func TestAWorkersPanicUnwrapsToTheErrorItRaised(t *testing.T) {
	t.Parallel()
	defer func() {
		p := recover()
		pp, ok := p.(*convoke.Panic)
		if !ok {
			t.Fatalf("re-raised %T, want a *convoke.Panic", p)
		}
		if !errors.Is(pp, errRefused) {
			t.Errorf("errors.Is read false through the crossing; Unwrap() = %v", pp.Unwrap())
		}
		var named *refusal
		if !errors.As(pp, &named) || named.why != "no games" {
			t.Errorf("errors.As did not read the raised error back out: %v", pp.Unwrap())
		}
		// And the printed form is unchanged by any of that: a log's `panic:`
		// line still carries the value and then the worker's stack.
		if msg := pp.Error(); !strings.Contains(msg, "no games") ||
			!strings.Contains(msg, "TestAWorkersPanicUnwrapsToTheErrorItRaised") {
			t.Errorf("Error() = %q, want the value and the stack", msg)
		}
	}()
	convoke.Indexed(200, 2, func(i int) {
		if i == 1 {
			panic(&refusal{why: "no games"})
		}
	})
}

// A panic that is NOT an error unwraps to nil rather than to something
// invented, so `errors.Is` answers false instead of matching by accident.
func TestAWorkersPanicThatIsNotAnErrorUnwrapsToNothing(t *testing.T) {
	t.Parallel()
	defer func() {
		pp, ok := recover().(*convoke.Panic)
		if !ok {
			t.Fatal("the panic did not cross back as a *convoke.Panic")
		}
		if err := pp.Unwrap(); err != nil {
			t.Errorf("Unwrap() = %v over a string value, want nil", err)
		}
		if errors.Is(pp, errRefused) {
			t.Error("a string panic matched an unrelated sentinel")
		}
	}()
	convoke.Indexed(200, 2, func(i int) {
		if i == 1 {
			panic("a string, as every other test here raises")
		}
	})
}

// A sentinel and a typed error, so the two reads above are the two reads a
// caller actually performs.
var errRefused = errors.New("the kernel refused the piece")

type refusal struct{ why string }

func (r *refusal) Error() string { return "refused: " + r.why }
func (r *refusal) Unwrap() error { return errRefused }
