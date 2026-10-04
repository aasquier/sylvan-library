package ledger

import (
	"context"
	"testing"
)

// A usage row whose counts are not counts is reported rather than rolled up.
//
// `Record` only ever writes integers, so this is the shape a ledger arrives in
// rather than one it is written into: an app.db restored from a backup that was
// taken mid-write, a row somebody fixed by hand, a column that changed type
// between two versions of the schema. The store is not strict about types, so
// the row sits there and `sum()` over it answers a number that is not a whole
// one.
//
// What must not happen is the quiet half-answer. `Summarise` runs because
// somebody asked what Claude has cost, and the rows come back most-expensive
// first — so a read that stopped at the row before the damaged one would answer
// a smaller bill, sorted and formatted and wrong, with nothing anywhere saying
// a row had been skipped. The function's own note says it: a wrong silent answer
// would be worse than a failure. This is the test that it is one.
func TestARollUpRefusesARowWhoseCountsAreNotCounts(t *testing.T) {
	t.Parallel()
	r := scratch(t)
	ctx := context.Background()
	r.Record(ctx, Row{"commander-dossier", "claude-opus-5", "end_turn", 3, 100, 20, 50})

	// The premise: the ledger reads before the damage.
	before, err := r.Summarise(ctx, "mode", "", "")
	if err != nil {
		t.Fatalf("rolling up a sound ledger: %v", err)
	}
	if len(before) != 1 || before[0].Requests != 3 {
		t.Fatalf("the sound roll-up is %+v", before)
	}

	// A request count of half a request. Written straight to the table, because
	// nothing in this package would ever write one.
	if _, err := r.db.ExecContext(ctx, `INSERT INTO claude_usage
		(created_at, mode, model, stop_reason, requests, input_tokens,
		 output_tokens, cache_read_tokens)
		VALUES ('2020-01-01T00:00:00+00:00', 'commander-dossier',
		        'claude-opus-5', 'end_turn', 1.5, 10, 2, 0)`); err != nil {
		t.Fatalf("writing a damaged row: %v", err)
	}

	got, err := r.Summarise(ctx, "mode", "", "")
	if err == nil {
		t.Fatalf("a ledger holding a row that is not a count answered %+v", got)
	}
	if got != nil {
		t.Errorf("the refusal came back with %d rows beside it", len(got))
	}
	// Both axes read the same table through the same scan, so both have to
	// refuse: a per-model question over a damaged ledger must not answer where
	// the per-mode one would not.
	if got, err := r.Summarise(ctx, "model", "", ""); err == nil {
		t.Errorf("the model axis answered %+v over the same damaged row", got)
	}
	// And a window that excludes the damaged row still answers, so the refusal
	// is about the row rather than about the ledger. The damaged one is stamped
	// in 2020 and the sound one is stamped now, so a lower bound separates them.
	if got, err := r.Summarise(ctx, "mode", "2021-01-01", ""); err != nil {
		t.Errorf("a window before the damaged row refused: %v", err)
	} else if len(got) != 1 || got[0].Requests != 3 {
		t.Errorf("the earlier window answered %+v", got)
	}
}
