package traffic

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The two halves of the ledger nothing else drives: the minute passing, and
// the database going away underneath it.
//
// The write path never raises on purpose — a full disk loses a minute of
// counts, not a request — so the only way to tell a working flush from a
// broken one is to read the rows back. The read path is the opposite: it
// raises, because the caller asked a question and a wrong silent answer is
// worse than an error. Both rules are only worth having if something proves
// them, and both were unreached.

func ledger(t *testing.T) (*Recorder, *authtest.Fault, *sql.DB) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db, slog.New(slog.NewTextHandler(io.Discard, nil))), fault, db
}

func counted(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var total int64
	if err := db.QueryRowContext(context.Background(),
		"SELECT coalesce(sum(count), 0) FROM request_log").Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

// Counts buffer in memory and go to the disk when a request lands more than
// FlushEvery after the last flush. Both halves matter: a ledger that wrote
// every request would be a database write on every page view, and one that
// never noticed the minute would lose everything a restart did not flush.
func TestTheLedgerBuffersAMinuteAndThenWritesWhatItHeld(t *testing.T) {
	t.Parallel()
	rec, _, db := ledger(t)
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	rec.now = func() time.Time { return at }
	rec.lastFlush = at

	for range 3 {
		rec.Record("/api/decks", 200)
	}
	if got := counted(t, db); got != 0 {
		t.Fatalf("%d counts reached the disk inside the minute", got)
	}

	// The minute passes, and the next request carries the buffer with it.
	at = at.Add(FlushEvery + time.Second)
	rec.Record("/api/decks", 404)
	if got := counted(t, db); got != 4 {
		t.Fatalf("%d counts were written when the minute lapsed, want 4", got)
	}
	// And the buffer went with them rather than being written twice.
	rec.Flush()
	if got := counted(t, db); got != 4 {
		t.Fatalf("a flush after the lapse double-counted to %d", got)
	}

	// The day and the class are what the row is keyed on, never the caller.
	var day, route, class string
	if err := db.QueryRowContext(context.Background(),
		"SELECT day, route, status_class FROM request_log ORDER BY status_class"+
			" LIMIT 1").Scan(&day, &route, &class); err != nil {
		t.Fatal(err)
	}
	if day != "2026-09-06" || route != "/api/decks" || class != "2xx" {
		t.Fatalf("the row reads %s %s %s", day, route, class)
	}
}

// A write that fails loses a minute of counts and says so in the log — and
// does not re-buffer them, because a broken database plus a growing buffer is
// two problems. What must never happen is the request failing with it.
func TestAFlushThatCannotWriteLosesTheMinuteRatherThanTheRequest(t *testing.T) {
	t.Parallel()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	logged := &strings.Builder{}
	rec := New(db, slog.New(slog.NewTextHandler(logged, nil)))

	rec.Record("/api/decks", 200)
	fault.After(0)
	rec.Flush()
	fault.Heal()
	if !strings.Contains(logged.String(), "request counts were not written") {
		t.Fatalf("a lost minute went unmentioned:\n%s", logged.String())
	}
	if got := counted(t, db); got != 0 {
		t.Fatalf("%d counts landed on a database that had gone", got)
	}
	// Not re-buffered: the next flush has nothing to say.
	rec.Flush()
	if got := counted(t, db); got != 0 {
		t.Fatalf("the lost minute was re-buffered and written later (%d)", got)
	}
}

// The read raises. Every one of these is a shape where a nil error would put
// an empty chart in front of somebody and call it a quiet day.
func TestASummaryThatCannotReadIsARefusalRatherThanAQuietDay(t *testing.T) {
	t.Parallel()
	rec, fault, db := ledger(t)
	ctx := context.Background()
	rec.Record("/api/decks", 200)
	rec.Record("/api/decks", 503)
	rec.Flush()
	if got := counted(t, db); got != 2 {
		t.Fatalf("the fixture wrote %d counts", got)
	}

	for _, tc := range []struct {
		name   string
		arm    func()
		reason string
	}{
		{"the days query", func() { fault.After(0) },
			"the per-day read"},
		{"the top-routes query", func() { fault.After(1) },
			"the second query, after the first answered"},
		{"the days iteration", func() { fault.RowsAfter(0) },
			"the rows themselves, partway through"},
		{"the top-routes iteration", func() { fault.RowsAfter(2) },
			"the second result set, partway through"},
	} {
		fault.Heal()
		tc.arm()
		got, err := rec.Summary(ctx, 7)
		fault.Heal()
		if err == nil {
			t.Errorf("%s: a summary came back over a failure in %s: %v",
				tc.name, tc.reason, got)
		}
		if got != nil {
			t.Errorf("%s: a failed summary handed back a payload anyway", tc.name)
		}
	}

	// Healed, the same question answers — so the four refusals above are
	// about the failure rather than about the fixture.
	fault.Heal()
	summary, err := rec.Summary(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 2 || summary[0].Key != "days" || summary[1].Key != "top_routes" {
		t.Fatalf("the summary's shape is %+v", summary)
	}
	days, _ := summary[0].Value.([]any)
	if len(days) != 1 {
		t.Fatalf("%d days in a one-day ledger", len(days))
	}
}

// A ledger with no database records nothing and never fails a request — the
// honest shape for an instance with no app.db, and the one a nil check in
// four places is paying for.
func TestANilLedgerCountsNothingAndAnswersAnEmptyHistory(t *testing.T) {
	t.Parallel()
	if New(nil, nil) != nil {
		t.Fatal("a ledger was minted over no database at all")
	}
	var absent *Recorder
	absent.Record("/api/decks", 200) // must not panic
	absent.Flush()
	summary, err := absent.Summary(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 2 {
		t.Fatalf("an absent ledger answered %+v", summary)
	}
	for _, kv := range summary {
		if list, ok := kv.Value.([]any); !ok || len(list) != 0 {
			t.Fatalf("%s is %v, want an empty list", kv.Key, kv.Value)
		}
	}
}
