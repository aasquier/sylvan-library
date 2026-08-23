package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The no-pool shape, as bytes: the degraded answer is the platform's normal
// state between deploy and seeding, and its key order is part of the wire.
func TestHealthWithNoPoolIsTheDegradedShapeExactly(t *testing.T) {
	a := New(Config{DecksDir: t.TempDir()})
	status, _, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	want := `{"pool":false,"oracle_cards":0,"printings":0,` +
		`"message":"no card pool yet -- run ` + "`mtglab data refresh`" + `"}`
	if string(raw) != want {
		t.Fatalf("got %s\nwant %s", raw, want)
	}
}

// A healthy pool reports its counts, the bulk files on the shelf, the deck
// count and a false staleness flag -- with the keys in Python's order.
func TestHealthReportsThePoolTheShelfAndTheDecks(t *testing.T) {
	scryfall := t.TempDir()
	for _, name := range []string{"oracle_cards-2026-08-20.jsonl.gz",
		"default_cards-2026-08-21.jsonl.gz"} {
		if err := os.WriteFile(filepath.Join(scryfall, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t),
		ScryfallDir: scryfall})
	status, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool"] != true || body["pool_stale"] != false {
		t.Fatalf("pool flags: %s", raw)
	}
	if body["oracle_cards"].(float64) < 20 || body["printings"].(float64) < 10 {
		t.Fatalf("counts: %s", raw)
	}
	files, _ := body["bulk_files"].([]any)
	if len(files) != 2 || files[0] != "default_cards-2026-08-21.jsonl.gz" {
		t.Fatalf("bulk_files should list the shelf sorted: %s", raw)
	}
	if body["decks"].(float64) < 1 {
		t.Fatalf("decks: %s", raw)
	}
	if _, present := body["message"]; present {
		t.Fatalf("a current pool carries no message: %s", raw)
	}
}

// A pool that predates the printed stats answers `pool_stale` and the
// re-ingest message -- `pool.Stale`'s verdict on the route.
func TestHealthReportsAStalePool(t *testing.T) {
	path := pooltest.Build(t)
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE oracle_cards SET power = NULL"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	a := New(Config{Pool: p, DecksDir: t.TempDir()})
	status, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool_stale"] != true {
		t.Fatalf("pool_stale: %s", raw)
	}
	msg, _ := body["message"].(string)
	if msg != "pool predates the printed stats or the painters -- "+
		"run `mtglab data refresh`" {
		t.Fatalf("message %q", msg)
	}
}
