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
