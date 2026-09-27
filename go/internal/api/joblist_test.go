package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
)

// The generic job list with something in it.
//
// `api_test.go` asks the empty registry and the unknown id, which are the two
// answers a fresh process gives; the whole of the loop that puts rows in the
// array had never run from here. Three things live in it and all three are
// contracts somebody's screen depends on: the array is **assembled by hand**
// so each payload keeps its struct's own field order (a slice through a
// generic encoder is how the Notes tab once shipped alphabetised), the order
// is newest first, and a row that will not serialise is **dropped with a log
// line rather than taking the whole listing down** — one wedged job must not
// blank a person's jobs tab.

func jobListing(t *testing.T, a *API, scope auth.Scope) []map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	a.listJobs(rec, req.WithContext(auth.WithScope(req.Context(), scope)))
	if rec.Code != http.StatusOK {
		t.Fatalf("the job listing answered %d: %s", rec.Code, rec.Body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("the listing is not JSON: %v (%s)", err, rec.Body)
	}
	return rows
}

// Newest first, scoped to the asker, and every row keyed the way its payload
// declares itself.
func TestTheJobListingIsNewestFirstAndOnlyTheAskersOwn(t *testing.T) {
	t.Parallel()
	reg := jobs.New(jobs.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	a := New(Config{Jobs: reg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	reg.Completed("sim.tier1", map[string]any{"which": "first"}, "the first", alice.UserID)
	reg.Completed("sim.tier1", map[string]any{"which": "second"}, "the second", alice.UserID)
	reg.Completed("sim.tier1", map[string]any{"which": "bobs"}, "bob's own", bob.UserID)

	rows := jobListing(t, a, alice)
	if len(rows) != 2 {
		t.Fatalf("alice sees %d jobs, want her own two: %v", len(rows), rows)
	}
	// `created_at` descending, as text -- the registry's instants are
	// fixed-width ISO, so string order is chronological order.
	if rows[0]["label"] != "the second" || rows[1]["label"] != "the first" {
		t.Errorf("the listing reads %v then %v, want newest first",
			rows[0]["label"], rows[1]["label"])
	}
	for _, row := range rows {
		if row["label"] == "bob's own" {
			t.Error("bob's job is in alice's listing (ADR 5)")
		}
	}

	// A row is the same bytes the single-job route answers, keys and order
	// alike: two spellings of one payload is how a tab starts rendering
	// something the other route never said.
	one := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/{job_id}", nil)
	req.SetPathValue("job_id", rows[0]["id"].(string))
	a.getJob(one, req.WithContext(auth.WithScope(req.Context(), alice)))
	if one.Code != http.StatusOK {
		t.Fatalf("the single-job route answered %d: %s", one.Code, one.Body)
	}
	listed, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	var alone map[string]any
	if err := json.Unmarshal(one.Body.Bytes(), &alone); err != nil {
		t.Fatal(err)
	}
	again, err := json.Marshal(alone)
	if err != nil {
		t.Fatal(err)
	}
	if string(listed) != string(again) {
		t.Errorf("the listed row and the single row differ:\n list %s\n one  %s",
			listed, again)
	}

	// And bob sees his, which is the other half of the scope rule: absent is
	// not the same as "there are no jobs on this instance".
	if got := jobListing(t, a, bob); len(got) != 1 || got[0]["label"] != "bob's own" {
		t.Errorf("bob's listing reads %v", got)
	}
}

// A job whose result cannot be serialised is dropped from the listing rather
// than breaking it: the rest of somebody's jobs tab still renders, and the
// reason goes to the log where it can be acted on.
func TestAJobThatWillNotSerialiseIsDroppedNotFatal(t *testing.T) {
	t.Parallel()
	reg := jobs.New(jobs.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	a := New(Config{Jobs: reg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	// A channel is not JSON and never will be. No route produces one -- this
	// is the guard standing behind every route that ever will.
	reg.Completed("sim.tier1", make(chan int), "the unserialisable one", alice.UserID)
	reg.Completed("sim.tier1", map[string]any{"fine": true}, "the good one", alice.UserID)

	rows := jobListing(t, a, alice)
	if len(rows) != 1 {
		t.Fatalf("the listing carries %d rows, want the one that serialises: %v",
			len(rows), rows)
	}
	if rows[0]["label"] != "the good one" {
		t.Errorf("the surviving row is %v", rows[0]["label"])
	}
}
