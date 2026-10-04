package door

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The door's end of the cache register.
//
// Three caches live in a serving process and only two of them are the API's:
// the static tiers' ETag memo belongs here, and `internal/door` imports
// `internal/api` rather than the reverse, so the API can only read it through
// a reader the composition root hands down. That hand-off is one line in
// [New] and nothing else in the tree would notice its absence -- the route
// would answer a well-formed payload with `"etag": null` and read as an
// instance that serves no static files. This test is the only thing that
// would, which is why it drives the real serving path for its numbers rather
// than asserting the field is non-nil.
func TestTheDoorHandsItsETagRegisterToTheStatsRoute(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: false, WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)

	// Two serves of one asset: the first can only miss, the second must come
	// from memory. The numbers are the mechanism's, not a sample -- every
	// serve consults the memo exactly once.
	get(t, srv, "GET", "/assets/app.js", "")
	get(t, srv, "GET", "/assets/app.js", "")
	if hits, misses := d.static.etagCounts(); hits != 1 || misses != 1 {
		t.Fatalf("after two serves the memo read %d hits / %d misses, want 1 / 1",
			hits, misses)
	}

	resp := get(t, srv, "GET", "/api/admin/stats/system", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the stats route answered %d", resp.StatusCode)
	}
	var payload struct {
		Caches struct {
			ETag *struct {
				Hits   int `json:"hits"`
				Misses int `json:"misses"`
			} `json:"etag"`
		} `json:"caches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Caches.ETag == nil {
		t.Fatal("the stats route reported no etag register: the door never handed its reader down")
	}
	if payload.Caches.ETag.Hits != 1 || payload.Caches.ETag.Misses != 1 {
		t.Fatalf("the route read %d hits / %d misses off the door's memo, want 1 / 1",
			payload.Caches.ETag.Hits, payload.Caches.ETag.Misses)
	}
}
