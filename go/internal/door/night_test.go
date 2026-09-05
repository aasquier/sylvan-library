package door

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The door's half of the Coliseum at Night (ADR 46): the knot it ties in New
// — store over its own write handle, runner around the routes' player — and
// the promise it makes in Close, that the app's first scheduler stops with
// the door and leaks nothing. The runner's behaviour lives in
// internal/night's tests; what is asserted here is the wiring and the
// lifetime.

func TestTheDoorTiesTheNightAndStopsItWithItself(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	writeFixtureDB(t, dbPath)
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: true, AppDB: dbPath, WebDist: web,
		TarotDir: tarot, DecksDir: t.TempDir(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	// The store is wired over the same app.db the door writes: before any
	// night, the watching read says so through the whole stack.
	resp := get(t, srv, "GET", "/api/admin/night", "alice-token")
	if resp.StatusCode != 404 {
		t.Fatalf("GET /api/admin/night answered %d, want 404 before any night", resp.StatusCode)
	}

	// A sample opens through the door — an empty library deals an empty
	// card, which keeps this test off the arena entirely; the fighting is
	// internal/api's and internal/night's to prove.
	req, err := http.NewRequest("POST", srv.URL+"/api/admin/night/sample",
		strings.NewReader(`{"minutes":5}`))
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "alice-token"})
	req.Header.Set("Content-Type", "application/json")
	answer, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != 201 {
		t.Fatalf("the sample answered %d", answer.StatusCode)
	}
	var opened map[string]any
	if err := json.NewDecoder(answer.Body).Decode(&opened); err != nil {
		t.Fatal(err)
	}
	if opened["run_id"] == nil || opened["closes_at"] == nil {
		t.Fatalf("the sample's answer is missing its fields: %v", opened)
	}

	resp = get(t, srv, "GET", "/api/admin/night", "alice-token")
	if resp.StatusCode != 200 {
		t.Fatalf("GET after the sample answered %d", resp.StatusCode)
	}

	// The lifetime half: Close stops the runner it started, and its return
	// is the leak assertion — Stop waits for every goroutine the runner
	// owns, so a Close that comes back is a scheduler that is gone.
	closed := make(chan error, 1)
	go func() { closed <- d.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close hung on the night's runner")
	}
}

func TestADoorWithNoAppDBHasNoNight(t *testing.T) {
	t.Parallel()
	// No write handle means no rows, and rows are the night's whole memory:
	// the runner is never built, and the admin surface says so in words.
	web, tarot := site(t)
	d, err := New(Config{WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.night != nil {
		t.Fatal("a door with no app.db built a night runner anyway")
	}
	if err := d.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
}
