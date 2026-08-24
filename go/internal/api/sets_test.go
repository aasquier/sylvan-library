package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

type setsFile []struct {
	Name     string         `json:"name"`
	Today    string         `json:"today"`
	Payload  map[string]any `json:"payload"`
	Rendered string         `json:"rendered"`
}

// TestTheSetFilterMatchesTheGolden is the corpus:
// the filter run with the clock frozen and the network stubbed,
// compared as the bytes the route answers -- the strict `>` against today,
// the digital drop, the six-key row and the stable tie order all come from
// the recorded run, not from a description of it.
func TestTheSetFilterMatchesTheGolden(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/sets.json")
	if err != nil {
		t.Fatalf("sets.json: %v (a frozen golden; never regenerated)", err)
	}
	var cases setsFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 3 {
		t.Fatalf("only %d set cases; the corpus has thinned", len(cases))
	}
	for _, tc := range cases {
		got, err := wire.MarshalOrdered(upcomingFrom(tc.Payload, tc.Today))
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		if string(got) != tc.Rendered {
			t.Errorf("%s diverged:\n got %s\nwant %s", tc.Name, got, tc.Rendered)
		}
	}
}

// The answer is cached for the day it was fetched on: a second ask is the
// same bytes and no second request -- the standing behaviour
// that keeps a dashboard from asking Scryfall once per render.
func TestASecondAskIsTheCacheNotAFetch(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if ua := r.Header.Get("User-Agent"); ua != setsUserAgent {
			t.Errorf("User-Agent %q", ua)
		}
		_, _ = w.Write([]byte(`{"data":[{"code":"fut","name":"Future",` +
			`"released_at":"2999-01-01","card_count":1,` +
			`"icon_svg_uri":"u","set_type":"expansion"}]}`))
	}))
	defer server.Close()
	old := scryfallSets
	scryfallSets = server.URL
	defer func() { scryfallSets = old }()

	a := New(Config{})
	first := httptest.NewRecorder()
	a.upcomingSets(first, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	second := httptest.NewRecorder()
	a.upcomingSets(second, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	if first.Code != 200 || second.Code != 200 {
		t.Fatalf("%d then %d: %s", first.Code, second.Code, first.Body)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("the cached answer differs from the fetched one")
	}
	if hits.Load() != 1 {
		t.Fatalf("%d fetches for two asks", hits.Load())
	}
}

// A transport failure is the one 503 that says so plainly, and an upstream
// error status is the same refusal -- the recorded contract folds both
// into "could not reach Scryfall".
func TestScryfallDownIsA503ThatSaysSo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	old := scryfallSets
	scryfallSets = server.URL
	defer func() { scryfallSets = old }()

	a := New(Config{})
	rec := httptest.NewRecorder()
	a.upcomingSets(rec, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	detail, _ := body["detail"].(string)
	if !bytes.HasPrefix([]byte(detail), []byte("could not reach Scryfall: ")) {
		t.Fatalf("detail %q", detail)
	}
}

// A payload that is not the JSON this expects raises outside the
// transport-failure branch, so it is the recorded uncaught 500:
// plain-text three words, nothing about the cause.
func TestAMalformedFeedIsTheRecordedUncaught500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer server.Close()
	old := scryfallSets
	scryfallSets = server.URL
	defer func() { scryfallSets = old }()

	a := New(Config{})
	rec := httptest.NewRecorder()
	a.upcomingSets(rec, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("%d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type %q", ct)
	}
	if rec.Body.String() != "Internal Server Error" {
		t.Fatalf("body %q", rec.Body.String())
	}
}
