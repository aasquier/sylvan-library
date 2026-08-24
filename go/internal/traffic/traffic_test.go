package traffic

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
	_ "modernc.org/sqlite"
)

func scratch(t *testing.T) *sql.DB {
	t.Helper()
	path := t.TempDir() + "/app.db"
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type summaryFile []struct {
	Name string
	Now  string
	Rows []struct {
		Day         string
		Route       string
		StatusClass string `json:"status_class"`
		Count       int
	}
	Rendered string
}

// TestTheRollUpMatchesTheGolden is the corpus: seeded `request_log` rows
// through `Summary` with the clock frozen, compared as the bytes the stats
// route wraps — the seeded 2xx/4xx/5xx keys on every day row, a 3xx
// appended in encounter order, the cutoff, and the top-twelve order.
func TestTheRollUpMatchesTheGolden(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/summary.json")
	if err != nil {
		t.Fatalf("summary.json: %v (a frozen golden; never regenerated)", err)
	}
	var cases summaryFile
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 3 {
		t.Fatalf("only %d cases; the corpus has thinned", len(cases))
	}
	for _, tc := range cases {
		db := scratch(t)
		for _, row := range tc.Rows {
			if _, err := db.Exec(
				"INSERT INTO request_log (day, route, status_class, count)"+
					" VALUES (?, ?, ?, ?)"+
					" ON CONFLICT(day, route, status_class)"+
					" DO UPDATE SET count = count + excluded.count",
				row.Day, row.Route, row.StatusClass, row.Count); err != nil {
				t.Fatal(err)
			}
		}
		now, err := time.Parse(time.RFC3339, tc.Now)
		if err != nil {
			t.Fatal(err)
		}
		r := New(db, nil)
		r.now = func() time.Time { return now }
		summary, err := r.Summary(context.Background(), 30)
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		got, err := wire.MarshalOrdered(summary)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.Rendered {
			t.Errorf("%s diverged:\n got %s\nwant %s", tc.Name, got, tc.Rendered)
		}
	}
}

// A recorder buffers: the first request writes nothing, the flush lands it,
// and a second flush has nothing to say.
func TestCountsBufferAndFlush(t *testing.T) {
	t.Parallel()
	db := scratch(t)
	r := New(db, nil)
	r.Record("/api/health", 200)
	r.Record("/api/health", 200)
	r.Record("(unrouted)", 404)
	var n int
	if err := db.QueryRow("SELECT count(*) FROM request_log").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("counts hit the database before a flush: %d rows", n)
	}
	r.Flush()
	var health int
	if err := db.QueryRow("SELECT count FROM request_log WHERE route = '/api/health'").Scan(&health); err != nil {
		t.Fatal(err)
	}
	if health != 2 {
		t.Fatalf("flushed count %d, want 2", health)
	}
	// The upsert accumulates across flushes rather than replacing.
	r.Record("/api/health", 200)
	r.Flush()
	if err := db.QueryRow("SELECT count FROM request_log WHERE route = '/api/health'").Scan(&health); err != nil {
		t.Fatal(err)
	}
	if health != 3 {
		t.Fatalf("re-flushed count %d, want 3", health)
	}
}

// A nil recorder records nothing and never fails a request — the instance
// with no app.db.
func TestANilRecorderIsSafe(t *testing.T) {
	t.Parallel()
	var r *Recorder
	r.Record("/api/health", 200)
	r.Flush()
	summary, err := r.Summary(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := wire.MarshalOrdered(summary)
	if string(got) != `{"days":[],"top_routes":[]}` {
		t.Fatalf("empty summary: %s", got)
	}
}

// A broken database loses a minute of counts, never a request.
func TestAFailedWriteIsAWarningNotAPanic(t *testing.T) {
	t.Parallel()
	db := scratch(t)
	if _, err := db.Exec("DROP TABLE request_log"); err != nil {
		t.Fatal(err)
	}
	r := New(db, nil)
	r.Record("/api/health", 200)
	r.Flush() // must not panic or error the caller
}
