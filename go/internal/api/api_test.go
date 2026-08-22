package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
// refused: today it is a handler whose every live branch is the proxy, it
// inverts the door's dependency (the proxy is chosen in `door` when `match`
// misses, and `door` imports `api`, not the reverse), and for the *list* it
// is not even expressible -- a list must be the union of both registries,
// re-sorted on `created_at` as text, for a half that is always empty.
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
func TestTheGenericJobRoutesAreStillPythons(t *testing.T) {
	for _, route := range New(Config{}).Routes() {
		if route.Pattern == "/api/jobs" || strings.HasPrefix(route.Pattern, "/api/jobs/") {
			t.Fatalf("%s %s has been ported, but a job lives in the registry "+
				"of the process that submitted it and every family that "+
				"submits one is still Python's -- this handler would answer "+
				"from a registry nothing writes to (see PLAN section 10)",
				route.Method, route.Pattern)
		}
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
