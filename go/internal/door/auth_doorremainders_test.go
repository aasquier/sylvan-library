package door

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The door's last branches: three wrappers and one memo.
//
// What is left in this package after the route sweeps is not about routing at
// all. It is the writers -- the three that wrap a response on its way out --
// and the ETag memo's degrade. Each one has the same shape: a case the
// ordinary request never takes, and a wrong answer that nobody would see until
// it mattered. A handler that writes a body without naming a status still has
// to get the hardening headers; a response somebody else encoded has to cross
// byte for byte; a handler that says nothing at all has to be counted as the
// 200 the wire carries rather than as a status of nothing; and a file that
// cannot be hashed has to serve exactly as it did before ETags existed.

// A door built with no logger takes the process's own rather than standing
// with a nil one, which the first line written would panic on. Every other
// test here hands one in, so the default had never run.
func TestADoorWithNoLoggerTakesTheProcessesOwn(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	d, err := New(Config{WebDist: web, TarotDir: tarot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if d.log == nil {
		t.Fatal("a door with no logger configured holds no logger")
	}

	// And it serves: a nil logger would panic on the first line any of this
	// writes, so one real request through the whole stack is the proof.
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("the shell answered %d", rec.Code)
	}
}

// The hardening headers land on a handler that writes a body without ever
// naming a status. `Write` has to open the response itself in that case --
// the headers are applied at WriteHeader time on purpose (a middleware that
// stamps them up front once put `nosniff, nosniff` on the wire), so a handler
// that never calls it would otherwise be served bare.
func TestABodyWrittenWithoutAStatusStillCarriesTheHardening(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	d, err := New(Config{WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	rec := httptest.NewRecorder()
	d.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No WriteHeader: the body is all this handler says.
		if _, err := w.Write([]byte("straight to the body")); err != nil {
			t.Error(err)
		}
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("a body with no status answered %d", rec.Code)
	}
	if got := rec.Body.String(); got != "straight to the body" {
		t.Errorf("the body came out as %q", got)
	}
	for name, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s is %q, want %q -- a body-only write was served bare",
				name, got, want)
		}
	}
}

// A response somebody else already encoded crosses byte for byte, however
// many writes it arrives in. The compressor is refused for it at the floor,
// and every byte after that has to go to the real writer untouched -- a
// doubly-encoded body is one a browser cannot read at all.
func TestAnAlreadyEncodedBodyCrossesUntouchedAcrossSeveralWrites(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	gz := &gzipWriter{ResponseWriter: rec, status: http.StatusOK}
	gz.Header().Set("Content-Encoding", "br")

	first := strings.Repeat("a", gzipFloor+1) // past the floor: it commits here
	second := "and the rest"
	if _, err := gz.Write([]byte(first)); err != nil {
		t.Fatal(err)
	}
	if _, err := gz.Write([]byte(second)); err != nil {
		t.Fatal(err)
	}
	gz.finish()

	if got := rec.Body.String(); got != first+second {
		t.Errorf("the body crossed as %d bytes, want the %d it was written as",
			len(got), len(first+second))
	}
	if got := rec.Header().Get("Content-Encoding"); got != "br" {
		t.Errorf("the Content-Encoding is now %q", got)
	}
	if rec.Header().Get("Vary") != "" {
		t.Error("a response that was never compressed carries a Vary")
	}
}

// A handler that answers with nothing at all is counted as the 200 the wire
// carries. Nothing calls WriteHeader on that path, so the status the ledger
// sees is zero -- and a request filed under a status that does not exist is a
// request missing from every reading of the day.
func TestAHandlerThatSaysNothingIsCountedAsTheTwoHundredItSends(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(dbPath); err != nil {
		t.Fatal(err)
	}
	web, tarot := site(t)
	d, err := New(Config{WebDist: web, TarotDir: tarot, AppDB: dbPath,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	// One route, and its handler returns without writing a byte. The served
	// table has nothing like it, which is why this branch had never run.
	table, err := newRouteTable([]api.Route{{Method: http.MethodGet,
		Pattern: "/api/silence", Handler: http.HandlerFunc(
			func(http.ResponseWriter, *http.Request) {})}})
	if err != nil {
		t.Fatal(err)
	}
	d.table = table

	srv := httptest.NewServer(d.Handler())
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/silence", nil)
	if err != nil {
		t.Fatal(err)
	}
	// No gzip offered: with it, the compression layer commits the response on
	// its way out and names the status itself.
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("a silent handler answered %d", resp.StatusCode)
	}

	// Close before the flush: the count is written by the handler's goroutine
	// after the client already has its answer.
	srv.Close()
	d.traffic.Flush()
	ledger, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ledger.Close() }()
	var class string
	if err := ledger.QueryRow(
		"SELECT status_class FROM request_log WHERE route = '/api/silence'").
		Scan(&class); err != nil {
		t.Fatalf("the silent request was not counted: %v", err)
	}
	if class != "2xx" {
		t.Errorf("the silent request was filed under %q, want the 2xx it sent", class)
	}
}

// A file that cannot be hashed serves exactly as it did before ETags existed:
// no tag, no refusal, and the miss counted so the degrade cannot slip past the
// register the admin stats read.
func TestAFileThatCannotBeHashedServesWithoutATag(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	s, err := newStaticSite(web, tarot, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	_, before := s.etagCounts()

	t.Run("a file that has gone since it was looked at", func(t *testing.T) {
		// Sequential on purpose: the two cases share one memo and the counter
		// below is read across both.
		vanished := filepath.Join(t.TempDir(), "gone.js")
		if err := os.WriteFile(vanished, []byte("console.log(1)"), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(vanished)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(vanished); err != nil {
			t.Fatal(err)
		}
		if tag := s.etagFor(vanished, info); tag != "" {
			t.Errorf("a file that is not there was tagged %s", tag)
		}
	})

	t.Run("a path that is a directory", func(t *testing.T) {
		dir := t.TempDir()
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		// It opens and refuses to be read, which is the other half of
		// unhashable -- and the one a serving path could reach by way of a
		// mount pointed at the wrong thing.
		if tag := s.etagFor(dir, info); tag != "" {
			t.Errorf("a directory was tagged %s", tag)
		}
	})

	if _, after := s.etagCounts(); after != before+2 {
		t.Errorf("the misses went %d -> %d, want both degrades counted",
			before, after)
	}
}

// A route pattern that is not canonical is refused as that, by name. The
// refusal used to be read as the empty-segment one, which could not fire:
// `NormalisePath` is `path.Clean` plus a trailing-slash trim, so its output
// never carries an empty segment, and `/api//decks` is turned away one line
// earlier for not being what normalising it would produce.
func TestANonCanonicalRoutePatternIsRefusedAsThat(t *testing.T) {
	t.Parallel()
	for _, pattern := range []string{"/api//decks", "/api/decks/", "/api/./decks"} {
		_, err := newRouteTable([]api.Route{{Method: http.MethodGet,
			Pattern: pattern, Handler: http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) {})}})
		if err == nil {
			t.Errorf("%q was served as a route", pattern)
			continue
		}
		if !strings.Contains(err.Error(), "canonical") {
			t.Errorf("%q was refused as %v, want the canonical check", pattern, err)
		}
	}
}
