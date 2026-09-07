package convoke_test

import (
	"sync/atomic"
	"testing"

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
		if s, ok := p.(string); !ok || s != "tier1: Run needs at least one game" {
			t.Fatalf("re-raised %v, want the original value", p)
		}
		// The stop flag halts hand-out; with 2 workers over 1000 pieces a
		// panic at index 3 must leave most of the grid untouched.
		if n := ran.Load(); n >= 1000 {
			t.Fatalf("all %d pieces ran despite the panic", n)
		}
	}()
	convoke.Indexed(1000, 2, func(i int) {
		ran.Add(1)
		if i == 3 {
			panic("tier1: Run needs at least one game")
		}
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
