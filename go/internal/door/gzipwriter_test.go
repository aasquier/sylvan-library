package door

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/api"
)

// The compressing writer's own decisions, made directly rather than through a
// served page.
//
// `door_test.go` proves a large response comes back compressed. What this
// holds is the **commit boundary** -- the moment the writer stops buffering
// and picks a path -- and every way a handler can reach it. The writer
// buffers the first kilobyte to decide, and after that it is committed: a
// `WriteHeader` arriving late is dropped rather than panicking with
// "superfluous WriteHeader", and every subsequent `Write` goes down the path
// already chosen.
//
// That late-header case is not hypothetical. A handler that streams a body
// and then hits an error tries to set a status it has already spent, and
// what happens next is either a clean truncated response or a panic inside
// the middleware.

// A handler that writes past the floor and then tries to set a status gets
// the status it already committed, not a panic.
func TestAStatusSetAfterTheCommitIsDroppedRatherThanPanicking(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	handler := gzipped(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Past the floor: the writer commits here.
		_, _ = w.Write(bytes.Repeat([]byte("a"), gzipFloor+1))
		// Too late -- the headers are already gone.
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("more"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("the response went out as %d -- the late status won", rec.Code)
	}
	// And the late write still landed, down the path already chosen.
	body := decompress(t, rec)
	if !strings.HasSuffix(body, "more") {
		t.Errorf("the write after the commit was lost (%d bytes)", len(body))
	}
}

// Under the floor, nothing is compressed: a kilobyte of gzip framing on a
// two-hundred-byte JSON answer is more bytes, not fewer.
func TestAResponseUnderTheFloorIsSentPlain(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	handler := gzipped(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"detail":"no deck 'gyome'"}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("a short answer was compressed (%q)", got)
	}
	if rec.Body.String() != `{"detail":"no deck 'gyome'"}` {
		t.Errorf("the body came back as %q", rec.Body.String())
	}
}

// Statuses with no body to speak of are never compressed, and neither is a
// body somebody else already encoded -- compressing a second time would make
// it unreadable.
func TestTheExcludedResponsesAreNeverCompressed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		status   int
		encoding string
	}{
		{"no content", http.StatusNoContent, ""},
		{"not modified", http.StatusNotModified, ""},
		{"already encoded", http.StatusOK, "br"},
		{"already gzipped", http.StatusOK, "gzip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			handler := gzipped(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.encoding != "" {
					w.Header().Set("Content-Encoding", tc.encoding)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write(bytes.Repeat([]byte("a"), gzipFloor*2))
			}))
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Errorf("answered %d, want %d", rec.Code, tc.status)
			}
			if got := rec.Header().Get("Content-Encoding"); got != tc.encoding {
				t.Errorf("Content-Encoding is %q, want %q", got, tc.encoding)
			}
			// The body is whatever the handler wrote, uncompressed.
			if tc.status == http.StatusOK && rec.Body.Len() != gzipFloor*2 {
				t.Errorf("the body is %d bytes, want it untouched", rec.Body.Len())
			}
		})
	}
}

// A caller that did not ask for gzip never gets it, whatever the size --
// which is what keeps a `curl` transcript readable and a client that cannot
// decompress from being handed something it cannot read.
func TestACallerThatDidNotAskIsNeverCompressed(t *testing.T) {
	t.Parallel()
	for _, accept := range []string{"", "identity", "br", "deflate"} {
		rec := httptest.NewRecorder()
		handler := gzipped(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(bytes.Repeat([]byte("a"), gzipFloor*2))
		}))
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if accept != "" {
			req.Header.Set("Accept-Encoding", accept)
		}
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Accept-Encoding %q got %q back", accept, got)
		}
		if rec.Body.Len() != gzipFloor*2 {
			t.Errorf("Accept-Encoding %q got a %d-byte body", accept, rec.Body.Len())
		}
	}
}

// A response that reaches the floor across several writes is compressed just
// the same -- the floor is about the whole body, not about one call.
func TestTheFloorCountsTheWholeBodyRatherThanOneWrite(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	handler := gzipped(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for i := 0; i < 40; i++ {
			_, _ = w.Write(bytes.Repeat([]byte("a"), 64))
		}
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("a body assembled from many writes was not compressed")
	}
	if got := len(decompress(t, rec)); got != 40*64 {
		t.Errorf("the decompressed body is %d bytes, want %d", got, 40*64)
	}
	// The length header is dropped, because the compressed length is not the
	// one the handler counted.
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("a compressed response kept Content-Length: %q", got)
	}
	// And it varies, so a cache does not serve gzip to a client that cannot
	// read it.
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary is %q", rec.Header().Get("Vary"))
	}
}

// A handler that writes nothing at all still produces a response rather than
// a hung connection -- the writer commits on the way out.
func TestAHandlerThatWritesNothingStillAnswers(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	handler := gzipped(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("an empty handler answered %d", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "" {
		t.Error("an empty body was compressed")
	}
}

// The wrapper hands the real writer back through `Unwrap`, which is what lets
// the standard library reach a Flusher or Hijacker underneath -- without it,
// a streamed response would buffer to the end.
func TestTheWrappersHandTheRealWriterBack(t *testing.T) {
	t.Parallel()
	underneath := httptest.NewRecorder()

	gz := &gzipWriter{ResponseWriter: underneath, status: http.StatusOK}
	if gz.Unwrap() != http.ResponseWriter(underneath) {
		t.Error("the gzip wrapper does not unwrap to the real writer")
	}

	status := &statusWriter{ResponseWriter: underneath}
	if status.Unwrap() != http.ResponseWriter(underneath) {
		t.Error("the status wrapper does not unwrap to the real writer")
	}

	header := &headerWriter{ResponseWriter: underneath}
	if header.Unwrap() != http.ResponseWriter(underneath) {
		t.Error("the header wrapper does not unwrap to the real writer")
	}
}

// The route table can name every route it serves, which is what the door's
// own sweeps derive their coverage from -- a sweep that built its list any
// other way would go stale the day somebody added a route.
func TestTheRouteTableCanNameEveryRouteItServes(t *testing.T) {
	t.Parallel()
	table, err := newRouteTable([]api.Route{
		{Method: http.MethodGet, Pattern: "/api/decks", Handler: nothing},
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}", Handler: nothing},
		{Method: http.MethodPost, Pattern: "/api/decks", Handler: nothing},
	})
	if err != nil {
		t.Fatal(err)
	}

	patterns := table.Patterns()
	if len(patterns) != 3 {
		t.Fatalf("the table named %v", patterns)
	}
	for _, want := range []string{
		"GET /api/decks",
		"GET /api/decks/{owner}/{slug}",
		"POST /api/decks",
	} {
		found := false
		for _, got := range patterns {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is not in %v", want, patterns)
		}
	}
}

// A path that matches a route on another method is a 405 naming that method,
// and the first matching route in declaration order owns the refusal -- the
// recorded rule.
func TestAPathMatchedOnAnotherMethodNamesIt(t *testing.T) {
	t.Parallel()
	table, err := newRouteTable([]api.Route{
		{Method: http.MethodGet, Pattern: "/api/decks/{owner}/{slug}", Handler: nothing},
		{Method: http.MethodDelete, Pattern: "/api/decks/{owner}/{slug}", Handler: nothing},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/decks/alice/gyome", nil)
	method, ok := table.allowed(req)
	if !ok {
		t.Fatal("a path served on another method was not recognised")
	}
	if method != http.MethodGet {
		t.Errorf("the Allow value is %q, want the first declared method", method)
	}

	// A path nothing serves is not a 405 at all -- it is a 404, and saying
	// otherwise would advertise routes that do not exist.
	req = httptest.NewRequest(http.MethodPost, "/api/nothing/here", nil)
	if _, ok := table.allowed(req); ok {
		t.Error("a path nothing serves was reported as a wrong method")
	}
	// A path with the wrong number of segments, likewise.
	req = httptest.NewRequest(http.MethodPost, "/api/decks/alice", nil)
	if _, ok := table.allowed(req); ok {
		t.Error("a shorter path matched a longer route")
	}
	// And an unnormalised path is refused rather than normalised here: the
	// door normalises once, at the top, and a second normalisation would be
	// a second chance to disagree.
	req = httptest.NewRequest(http.MethodPost, "/api/decks//alice/gyome/", nil)
	if _, ok := table.allowed(req); ok {
		t.Error("an unnormalised path matched")
	}
}

func nothing(http.ResponseWriter, *http.Request) {}

// decompress reads a gzip response body.
func decompress(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Header().Get("Content-Encoding") != "gzip" {
		return rec.Body.String()
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("the compressed body will not read: %v", err)
	}
	defer func() { _ = zr.Close() }()
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("the compressed body is truncated: %v", err)
	}
	return string(raw)
}
