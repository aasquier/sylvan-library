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
	t.Parallel()
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

// The two generic job routes answer from this process's registry: the list
// is `[]` when it is empty (never `null` -- the frontend iterates it), and
// an id nobody holds is a 404 whose detail the frontend renders.
func TestTheGenericJobRoutesAnswerTheRegistry(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	var list, one bool
	for _, route := range a.Routes() {
		if route.Method == http.MethodGet && route.Pattern == "/api/jobs" {
			list = true
		}
		if route.Method == http.MethodGet && route.Pattern == "/api/jobs/{job_id}" {
			one = true
		}
	}
	if !list || !one {
		t.Fatalf("the generic job routes are not registered (list=%v one=%v)", list, one)
	}

	rec := httptest.NewRecorder()
	a.listJobs(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "[]" {
		t.Fatalf("an empty registry listed %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	a.getJob(rec, httptest.NewRequest(http.MethodGet, "/api/jobs/whatever", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown id answered %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no such job") {
		t.Fatalf("the 404 carries %q", rec.Body.String())
	}
}

func TestEveryRouteHasAMethodAPatternAndAHandler(t *testing.T) {
	t.Parallel()
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
