package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

func TestTheProseRoutesAnswerTheEmbeddedPayloads(t *testing.T) {
	a := New(Config{})
	want := map[string][]byte{
		"/api/colors":   reference.ColorsJSON(),
		"/api/glossary": reference.GlossaryJSON(),
		"/api/themes":   reference.ThemesJSON(),
	}
	for _, route := range a.Routes() {
		body, known := want[route.Pattern]
		if !known {
			continue
		}
		rec := httptest.NewRecorder()
		route.Handler(rec, httptest.NewRequest(route.Method, route.Pattern, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d", route.Pattern, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: content-type %q", route.Pattern, ct)
		}
		if !bytes.Equal(rec.Body.Bytes(), body) {
			t.Errorf("%s: body is not the embedded payload", route.Pattern)
		}
		if !json.Valid(rec.Body.Bytes()) {
			t.Errorf("%s: not JSON", route.Pattern)
		}
	}
}

// The two generic job routes -- `GET /api/jobs` and `GET /api/jobs/{job_id}`
// -- are deliberately still Python's, and not because they are hard. They are
// *reads*, and they looked like the obvious next flip the day `internal/jobs`
// landed. The obstacle is that neither route owns any state: they are the
// **view** over a registry whose contents are written by eight other families
// (simruns, shelfruns, dossierruns, themeruns, researchruns, argueruns,
// scanruns, forgeruns), every one of which still submits into `api/jobs.py`'s
// module-level registry, in the uvicorn process's memory, which the door
// cannot reach.
//
// A registry is per-process, so a Go handler here would answer from a
// registry the app never writes to: `/api/jobs` would be `[]` forever and
// `/api/jobs/{job_id}` a 404 for every id the app hands out. That is not a
// quiet wrong answer either -- `followJob` in `web/src/lib/api.ts` reads a
// 404 as "the server restarted and the run died with it", so all seven of
// its call sites (review, dossier, both theme halves, camera, research, the
// simulator) would report every long job lost the instant it was submitted.
//
// Answering-when-Go-owns-the-id-and-proxying-otherwise was considered and
// refused **for today**: it is a handler whose every live branch is the
// proxy, since Go owns no ids at all, and for the *list* it is not even
// expressible -- a list must be the union of both registries, re-sorted on
// `created_at` as text, for a half that is always empty.
//
// It is refused today and not forever, because of how the families must
// move. Every job, whichever family submitted it, is polled through this one
// route (`followJob` -> `api.job(id)`; there is no per-family poll), so it
// couples all eight together: flip `simruns` alone and its Go-submitted ids
// are invisible to Python's poll route, flip this route alone and Python's
// ids are invisible to Go's. No partial order works. So either all eight
// families and both routes move in one change -- and these routes stay as
// simple as they read -- or that hybrid is built *with the first family
// flip*, which is the moment both of its branches are finally live. PLAN
// section 10 carries the choice.
//
// The general rule, and the reason this is a test rather than a line in a
// commit message: **a route can only flip when the state it reads has
// flipped**, and a view flips *last*, not first. The engine-then-routes
// rhythm reads backwards here; "it is only a read, so it is easy" is exactly
// wrong.
//
// This is meant to fail when somebody flips them on purpose, which is the
// moment to check that the eight families went across with them. See PLAN
// section 10.
// TestTheGenericJobRoutesAreTheHybrid replaces
// `TestTheGenericJobRoutesAreStillPythons`, which was the tripwire guarding
// these two routes until the sim family flipped.
//
// It is not simply deleted, because the property that made the tripwire
// necessary is still true and still load-bearing: a job lives in the registry
// of the process that submitted it, and five of the eight job-submitting
// families are still Python's. What changed is that Go now owns some of them,
// so the routes must answer from **both** -- which is exactly what a plain
// port would not do, and what a future edit could quietly undo by dropping the
// upstream branch.
//
// So this asserts the two routes exist *and* that neither answers without
// consulting the upstream when there is one.
func TestTheGenericJobRoutesAreTheHybrid(t *testing.T) {
	var list, one bool
	for _, route := range New(Config{}).Routes() {
		if route.Method == http.MethodGet && route.Pattern == "/api/jobs" {
			list = true
		}
		if route.Method == http.MethodGet && route.Pattern == "/api/jobs/{job_id}" {
			one = true
		}
	}
	if !list || !one {
		t.Fatalf("the generic job routes are not registered (list=%v one=%v); "+
			"they flipped with the sim family and a registry-less door still "+
			"has to answer them", list, one)
	}

	// Both must reach the upstream when Go's registry cannot answer. A handler
	// that skipped it would report every Python-submitted job as lost, which is
	// the failure PLAN section 10 spent a page on.
	for _, tc := range []struct {
		name   string
		method string
		target string
	}{
		{"the list", http.MethodGet, "/api/jobs"},
		{"one by id", http.MethodGet, "/api/jobs/whatever"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := false
			upstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				asked = true
				w.Header().Set("content-type", "application/json")
				_, _ = w.Write([]byte("[]"))
			})
			a := New(Config{Upstream: upstream})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.target, nil)
			for _, route := range a.Routes() {
				if route.Method == tc.method && route.Pattern == tc.target {
					route.Handler(rec, req)
				}
			}
			if tc.target == "/api/jobs" {
				a.listJobs(rec, req)
			} else {
				a.getJob(rec, req)
			}
			if !asked {
				t.Fatal("the handler answered without asking the upstream; " +
					"every job Python holds would read as lost")
			}
		})
	}
}

func TestEveryRouteHasAMethodAPatternAndAHandler(t *testing.T) {
	seen := map[string]bool{}
	for _, route := range New(Config{}).Routes() {
		if route.Method == "" || route.Pattern == "" || route.Handler == nil {
			t.Fatalf("incomplete route %+v", route)
		}
		key := route.Method + " " + route.Pattern
		if seen[key] {
			t.Fatalf("%s is listed twice", key)
		}
		seen[key] = true
	}
}
