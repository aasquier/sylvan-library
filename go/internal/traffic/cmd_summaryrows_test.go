package traffic

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The row budget, swept rather than counted (docs/polish/COVERAGE.md, levers 29
// and 34).
//
// `ledgerfaults_test.go` arms `RowsAfter` at two hand-picked numbers and calls
// the second one "the second result set, partway through". It is not: the
// budget is spent across **every** query on the handle, and with that fixture's
// two counts the days query alone takes three steps of it — so both numbers
// land inside the *first* loop and the top-routes iteration was never
// interrupted at all. That is the exact failure lever 34 warns about: a counted
// budget is a restatement of today's statement order, and this one had already
// drifted off the statement it was named for.
//
// Swept from zero, the question stops being "which number" and becomes the one
// the code has to answer: **at no point in this read may a failure become a
// quiet day.** The sweep carries both floors — at least one budget refused and
// at least one answered — and asserts the pair every time, so it reaches the
// arms in between without naming one.
func TestNoRowBudgetTurnsAHalfReadSummaryIntoAQuietDay(t *testing.T) {
	t.Parallel()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rec := New(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	// Two routes and two status classes, so both of the summary's result sets
	// have more than one row in them and a budget can land inside either.
	rec.Record("/api/decks", 200)
	rec.Record("/api/decks", 503)
	rec.Record("/api/cards", 200)
	rec.Flush()
	if got := counted(t, db); got != 3 {
		t.Fatalf("the fixture wrote %d counts", got)
	}

	// The whole answer, so the sweep below has something definite to compare
	// a success against.
	whole, err := rec.Summary(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	wantRoutes := len(routesOf(t, whole))
	if wantRoutes != 2 {
		t.Fatalf("the ledger answered %d routes, want 2", wantRoutes)
	}

	refused, answered := 0, 0
	for budget := range 10 {
		fault.Heal()
		fault.RowsAfter(budget)
		got, err := rec.Summary(ctx, 7)
		fault.Heal()
		switch {
		case err != nil:
			if answered > 0 {
				// A bigger budget refusing after a smaller one answered means
				// the sweep below it proved nothing (lever 34).
				t.Fatalf("budget %d refused after a smaller one answered: "+
					"the budget is not monotonic and this sweep is measuring "+
					"something else", budget)
			}
			refused++
			if got != nil {
				t.Errorf("budget %d: a failed summary handed back a payload "+
					"anyway: %+v", budget, got)
			}
		default:
			answered++
			if n := len(routesOf(t, got)); n != wantRoutes {
				t.Errorf("budget %d: the summary answered with %d routes "+
					"where the whole ledger has %d -- a partial read reported "+
					"as the day's traffic", budget, n, wantRoutes)
			}
		}
	}
	if refused == 0 {
		t.Error("no budget in the sweep refused, so nothing about a failing " +
			"read was proved")
	}
	if answered == 0 {
		t.Error("no budget in the sweep answered, so the refusals above are " +
			"about the fixture rather than about the failure")
	}
}

// routesOf digs the top-routes list out of a summary, so the sweep can compare
// a partial answer against a whole one.
func routesOf(t *testing.T, summary wire.OrderedMap) []any {
	t.Helper()
	for _, kv := range summary {
		if kv.Key != "top_routes" {
			continue
		}
		list, ok := kv.Value.([]any)
		if !ok {
			t.Fatalf("top_routes is %T", kv.Value)
		}
		return list
	}
	t.Fatalf("the summary has no top_routes at all: %+v", summary)
	return nil
}
