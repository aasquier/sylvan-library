package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// A simulation job must store its result AFTER the request that submitted it
// has gone.
//
// Every other route test here drives a handler through a recorder, whose
// context is never cancelled. A real server cancels the request's context the
// moment the handler returns -- and the job closures in `planMana`,
// `planLands` and `planPolicy` captured `r.Context()` to write the cache
// with, found it dead by the time Tier 1 finished, failed the write with
// `context canceled`, warned, and never stored a row. **From v183 (2026-08-22)
// until this test existed, the Go sim cache stored nothing**: every run paid
// full price and every second ask recomputed, which is the failure that looks
// exactly like a cache that simply missed. So this drives the route through a
// real `httptest.Server` and asks the only question that matters: is the
// second ask a job born finished?
//
// Found by the dossier lane, which had to decide what context a Claude job
// runs under and asked the same question of its siblings -- and it is the
// lesson to carry: **a job's Run never touches the request's context.**
func TestASimulationJobStoresItsResultAfterTheRequestHasGone(t *testing.T) {
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := cache.Open(dbPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reg := jobs.New(jobs.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t), AdminEmail: "alice@example.com",
		AppDB: db, Jobs: reg, SimCache: store})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.simMana(w, r.WithContext(auth.WithScope(r.Context(), alice)))
	}))
	defer srv.Close()

	post := func() map[string]any {
		t.Helper()
		resp, err := http.Post(srv.URL+"/api/sim/mana", "application/json",
			strings.NewReader(`{"slug":"kaheera","games":40}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%d %s", resp.StatusCode, body)
		}
		payload := map[string]any{}
		_ = json.Unmarshal(body, &payload)
		return payload
	}
	first := post()
	reg.Wait()
	if got := reg.Get(first["id"].(string), alice.UserID); got == nil || got.Status() != "done" {
		t.Fatalf("the first run did not finish: %v", got)
	}
	second := post()
	if second["status"] != "done" {
		t.Fatalf("the second ask was not served from the cache: status %v -- the job's cache write "+
			"used the request's context, which was cancelled when the handler returned", second["status"])
	}
	result, _ := second["result"].(map[string]any)
	if result["cached"] != true {
		t.Errorf("the second run reports cached=%v", result["cached"])
	}
}

// TestAMissingDeckCarriesTheBareSlugIntoTheJob is the divergence the pair
// diff found: a job's `error` becomes a JS `Error` in `lib/api.ts` and the
// screen shows it, so the two runtimes must say the same sentence.
//
// Python's `DeckNotFound(slug)` has no `__str__` of its own, so `str(exc)` is
// the slug alone; the 404's "no deck 'x'" is built by the route's exception
// handler and belongs only there. Go's `library.ErrNotFound` renders the
// sentence — right for the handler, wrong for the job — and the door carried
// it into every deferred failure from Phase 5 until 2026-08-23.
func TestAMissingDeckCarriesTheBareSlugIntoTheJob(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Logger: quiet, Pool: pooltest.Open(t), DecksDir: decksDir(t),
		AdminEmail: "alice@example.com", AppDB: db, Jobs: reg})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.simMana(w, r.WithContext(auth.WithScope(r.Context(), alice)))
	}))
	defer srv.Close()

	for _, c := range []struct{ note, body, want string }{
		{"a slug nobody has", `{"slug":"nope"}`, "nope"},
		// `str(["x"])` is `"['x']"`, quotes and all — the other half of the
		// same finding, in the coercion rather than in the error.
		{"a list where a slug belongs", `{"slug":["x"]}`, "['x']"},
	} {
		t.Run(c.note, func(t *testing.T) {
			resp, err := http.Post(srv.URL+"/api/sim/mana", "application/json",
				strings.NewReader(c.body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			id, ok := payload["id"].(string)
			if !ok {
				t.Fatalf("the route answered %d %v rather than a job", resp.StatusCode, payload)
			}
			reg.Wait()
			job := reg.Get(id, alice.UserID)
			if job == nil || job.Status() != "error" {
				t.Fatalf("the job did not fail: %v", job)
			}
			got := job.Payload().Error
			if got == nil || *got != c.want {
				t.Errorf("the job's error is %v, want %q — Python says the "+
					"bare slug, and the browser renders this verbatim", got, c.want)
			}
		})
	}
}
