package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	t.Parallel()
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

	a := New(Config{SetsFeed: server.URL})
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
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	a := New(Config{SetsFeed: server.URL})
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
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer server.Close()

	a := New(Config{SetsFeed: server.URL,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
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

// The seam's own guard: an instance nobody told where the feed lives asks
// Scryfall, and the three tests above prove nothing at all if that stops
// being true. The zero value is the whole reason the URL could stop being a
// package variable -- every caller that used to get the constant for free
// still gets it -- so it is asserted rather than assumed.
func TestAnInstanceToldNothingAsksScryfall(t *testing.T) {
	t.Parallel()
	if got := New(Config{}).setsFeed; got != ScryfallSets {
		t.Fatalf("an unconfigured instance asks %q, not Scryfall", got)
	}
	if got := New(Config{SetsFeed: "https://elsewhere.example/sets"}).setsFeed; got == ScryfallSets {
		t.Fatal("a configured feed was overwritten by the default")
	}
}

// A feed that is not a URL fails before a single byte goes out, and it fails
// as the library's own 500 rather than as a transport sentence -- "could not
// reach Scryfall" would be a lie about a request that was never made.
func TestAFeedThatIsNotAURLFailsBeforeAnythingIsAsked(t *testing.T) {
	t.Parallel()
	a := New(Config{SetsFeed: "://not a url",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := httptest.NewRecorder()
	a.upcomingSets(rec, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("%d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	detail, _ := body["detail"].(string)
	if strings.Contains(detail, "Scryfall") {
		t.Fatalf("a request that was never sent was blamed on Scryfall: %q", detail)
	}
	if strings.TrimSpace(detail) == "" {
		t.Fatalf("the 500 carries nothing a person could read: %s", rec.Body)
	}
}

// A feed that answers and then goes away mid-sentence is the transport
// refusal, not the malformed-payload 500: the bytes never arrived, so there
// is nothing to have been malformed. The two branches are four lines apart in
// the handler and they answer differently on purpose.
func TestAFeedThatStopsMidBodyIsTheTransportRefusal(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A length promised and not delivered: the read fails rather than
		// completing short, which is what makes this the transport's fault.
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte(`{"data":[`))
		panic(http.ErrAbortHandler)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer server.Close()

	a := New(Config{SetsFeed: server.URL,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := httptest.NewRecorder()
	a.upcomingSets(rec, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if detail, _ := body["detail"].(string); !strings.HasPrefix(detail, "could not reach Scryfall: ") {
		t.Fatalf("detail %q", detail)
	}
}

// A day-old answer is not today's cache: the key is the date the answer was
// fetched on, so the first ask of a new day fetches again. Nothing else
// asserts the `==` in that condition, and a cache that never expired would
// serve a whole spoiler season out of yesterday.
func TestYesterdaysAnswerIsNotTodaysCache(t *testing.T) {
	t.Parallel()
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	a := New(Config{SetsFeed: server.URL})
	a.setsDay, a.setsBody = "2000-01-01", []byte(`{"sets":[],"as_of":"2000-01-01"}`)

	rec := httptest.NewRecorder()
	a.upcomingSets(rec, httptest.NewRequest(http.MethodGet, "/api/sets/upcoming", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d: %s", rec.Code, rec.Body)
	}
	if hits.Load() != 1 {
		t.Fatalf("%d fetches: yesterday's answer was served as today's", hits.Load())
	}
	if strings.Contains(rec.Body.String(), "2000-01-01") {
		t.Fatalf("the answer is still stamped with the stale day: %s", rec.Body)
	}
	if a.setsDay == "2000-01-01" {
		t.Fatal("the cache kept the stale day after a fresh fetch")
	}
}
