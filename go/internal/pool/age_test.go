package pool_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The day the library's rows were published, read off the download shelf.
//
// The arithmetic on top of this — whole days, a future stamp, the nil for a
// question nobody can answer — belongs to the route that reports it
// (`internal/api`'s `poolAgeDays`). What is asked here is the two things only
// the shelf can answer: that the reading obeys [pool.SweepBulk]'s own rules
// about what is a download, and that a shelf nobody can read says so rather
// than answering with a zero time that would render as "today".

func TestTheLibrarysPublishedDayIsTheOldestKindOnTheShelf(t *testing.T) {
	t.Parallel()
	shelf := t.TempDir()
	park(t, shelf, "oracle_cards-2026-09-26.jsonl.gz")
	park(t, shelf, "oracle_cards-2026-08-01.jsonl.gz") // a rollback, not the load
	park(t, shelf, "default_cards-2026-08-15.jsonl")   // a different suffix, still ours

	day, ok := pool.BulkDataDay(shelf)
	if !ok {
		t.Fatal("a shelf holding both kinds could not be dated")
	}
	if want := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC); !day.Equal(want) {
		t.Errorf("the shelf dates to %s, want the older printings copy at %s",
			day.Format(time.DateOnly), want.Format(time.DateOnly))
	}
}

// Everything on the shelf that is not a download, by the sweep's own rules. A
// **directory** wearing a bulk file's exact name is the one worth spelling out:
// `SweepBulk` refuses to delete it for this reason and dating the library from
// it would be the same mistake in a gentler form.
func TestNothingButADownloadDatesTheLibrary(t *testing.T) {
	t.Parallel()
	shelf := t.TempDir()
	park(t, shelf, "oracle_cards-2026-09-26.jsonl.gz.part") // still arriving
	park(t, shelf, "oracle-cards-2026-09-20.jsonl.gz")      // not the kind
	park(t, shelf, "rulings-2026-09-20.json")               // a kind we never fetch
	park(t, shelf, "notes.txt")
	if err := os.MkdirAll(filepath.Join(shelf,
		"oracle_cards-2026-09-20.jsonl.gz"), 0o750); err != nil {
		t.Fatal(err)
	}

	if day, ok := pool.BulkDataDay(shelf); ok {
		t.Errorf("the library was dated %s off a shelf holding no downloads",
			day.Format(time.DateOnly))
	}

	// And the positive half, so the five refusals above are about the names
	// rather than about a reader that never finds anything.
	park(t, shelf, "oracle_cards-2026-09-20.json")
	if _, ok := pool.BulkDataDay(shelf); !ok {
		t.Error("one real download on the same shelf still did not date it")
	}
}

// A shelf that cannot be read is not a shelf with nothing on it. The zero
// [time.Time] would render as the first day of year one, and a caller that
// trusted the boolean-less answer would report a library two thousand years
// behind — or, worse, clamp it and report one that is current.
func TestAShelfNobodyCanReadDatesNothing(t *testing.T) {
	t.Parallel()
	shelf := t.TempDir()
	park(t, shelf, "oracle_cards-2026-09-20.jsonl.gz")
	if _, ok := pool.BulkDataDay(shelf); !ok {
		t.Fatal("the shelf could not be dated before it was closed, so the " +
			"assertion below would be about nothing")
	}
	if err := os.Chmod(shelf, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shelf, 0o750) })
	if day, ok := pool.BulkDataDay(shelf); ok {
		t.Errorf("an unreadable shelf answered %s", day.Format(time.DateOnly))
	}
}

func park(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
