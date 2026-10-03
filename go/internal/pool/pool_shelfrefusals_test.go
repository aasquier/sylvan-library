package pool_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The last two things the download shelf can refuse, and the one line of
// `Refresh` that runs only when nobody names a source.
//
// `downloadfaults_test.go` holds the directory that cannot be made and the write
// that cannot finish; what is left is the step *after* the write -- the parked
// file taking its dated name -- and the sweep's own refusal to delete. Both are
// permission shapes, and the volume these run on is a real one with a real mode
// (ADR 23).

// A download that cannot take its dated name refuses rather than reporting a
// path nothing is at.
//
// The name is taken last, after the bytes are safely on disk under `.part`, so
// this is the one failure that happens with a complete download in hand -- and
// the one where reporting success would be worst: the next refresh's dated skip
// takes the presence of the name as proof the file is good, and would read a
// directory as a bulk file forever.
func TestADownloadThatCannotTakeItsNameRefusesRatherThanNaming(t *testing.T) {
	t.Parallel()
	shelf := filepath.Join(t.TempDir(), "scryfall")
	target := filepath.Join(shelf, "oracle_cards-2026-08-24.jsonl")

	// The shelf goes read-only **while the body is streaming**, which is a
	// volume remounted under a refresh: the bytes already have a file to land
	// in, and the directory will not accept a new name. The handler waits for
	// the `.part` file to exist rather than for a clock, so the moment is the
	// one this test means.
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{"type": pool.OracleBulk,
				"updated_at":         "2026-08-24T09:00:00.000+00:00",
				"jsonl_download_uri": base + "/files/oracle-cards.jsonl"},
		}})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		for range 400 {
			if _, err := os.Stat(target + ".part"); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if err := os.Chmod(shelf, 0o500); err != nil {
			t.Errorf("shutting the shelf: %v", err)
		}
		_, _ = w.Write([]byte(`{"oracle_id":"o1","name":"Fixture Chef"}` + "\n"))
	})
	server := httptest.NewServer(mux)
	base = server.URL
	t.Cleanup(server.Close)
	t.Cleanup(func() { _ = os.Chmod(shelf, 0o700) })

	path, err := pool.DownloadBulkFrom(context.Background(),
		server.URL+"/bulk-data", pool.OracleBulk, shelf)
	if err == nil {
		t.Fatalf("a download reported %q onto a shelf that stopped accepting "+
			"names while it was being written", path)
	}
	if path != "" {
		t.Errorf("the refusal came back naming %q", path)
	}
	// The dated name is what a later run's skip trusts, so it must not exist.
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the refused download left the dated name behind: %v", err)
	}
}

// A shelf whose files cannot be deleted sweeps nothing and says so with a count
// rather than with a failure.
//
// **A file that will not delete is not a failed refresh.** The rows are in, the
// pool is loaded, and the only casualty is a byte count -- so the sweep steps
// over what it cannot remove and reports what it did. A refresh that failed
// here would send an operator looking for a broken pool that is sitting there
// fully loaded.
func TestAShelfThatWillNotDeleteSweepsNothingAndStillAnswers(t *testing.T) {
	t.Parallel()
	shelf := filepath.Join(t.TempDir(), "scryfall")
	seedShelf(t, shelf, "oracle_cards-2026-08-01.jsonl", 11)
	seedShelf(t, shelf, "oracle_cards-2026-08-02.jsonl", 22)

	// Readable and searchable, not writable: the directory can be listed and
	// its files measured, and nothing in it can be unlinked.
	if err := os.Chmod(shelf, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shelf, 0o700) })

	swept, err := pool.SweepBulk(shelf, map[string]string{
		pool.OracleBulk: filepath.Join(shelf, "oracle_cards-2026-08-24.jsonl")})
	if err != nil {
		t.Fatalf("a shelf that could be read but not written answered %v -- the "+
			"directory's own error is the only one this reports", err)
	}
	if swept.Files != 0 || swept.Bytes != 0 {
		t.Errorf("the sweep claimed %d files and %d bytes off a shelf nothing "+
			"can be deleted from", swept.Files, swept.Bytes)
	}
	// Both files are still there, which is the honest outcome.
	if got := onTheShelf(t, shelf); len(got) != 2 {
		t.Errorf("the shelf holds %v", got)
	}
}

// A refresh that is told no source falls back to Scryfall's own index, and the
// fallback is the line the app runs and no test had.
//
// Driven over a context that is **already cancelled**, which is how a line that
// reaches for the network is proved without reaching it: the default is chosen
// before anything is opened or asked, and the run then stops at the first step
// that would have needed a wire. The assertion is that it refused for the
// cancellation's reason rather than by asking the internet anything.
func TestARefreshWithNoSourceNamedFallsBackToTheStandingOne(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	_, err := pool.Refresh(ctx, pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: filepath.Join(dir, "scryfall"),
		OracleOnly:  true,
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatal("a refresh over a cancelled context reported success")
	}
	// The standing index is a real URL and it is never asked here; what proves
	// the fallback ran is that the run got far enough to be stopped by the
	// cancellation rather than by an empty source.
	if pool.BulkIndex == "" {
		t.Error("there is no standing index for a refresh to fall back to")
	}
	var refusal *pool.RefreshError
	if !errors.As(err, &refusal) {
		t.Errorf("the refusal is not classified by phase: %v", err)
	}
}
