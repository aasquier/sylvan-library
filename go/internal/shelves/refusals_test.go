package shelves

// Every way a shelf says "not today".
//
// The shelves' whole contract is that a miss is quiet: "" comes back, the
// caller answers 404, and the client falls back to what it drew itself. That
// makes the refusals the part worth testing, because a refusal that silently
// became a *cache write* would put a captive portal's login page, a truncated
// download or bytes that failed their pin on the volume and serve them for
// ever after. Each test below asks the same two questions of one failure:
// nothing was served, and nothing was stored.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// ownTable is one test's own copy of the committed shelf configuration. The
// struct copies by value but its asset map does not, so the map is rebuilt by
// hand: a test that edits a pin must not be editing the table every other
// test in the suite is reading at the same moment.
func ownTable(t *testing.T) reference.RuntimeShelves {
	t.Helper()
	conf := *reference.Runtime()
	assets := make(map[string]reference.OCRAsset, len(conf.OCR.Assets))
	for name, a := range conf.OCR.Assets {
		assets[name] = a
	}
	conf.OCR.Assets = assets
	return conf
}

// quiet is a logger whose warnings go nowhere: these tests drive the paths
// that warn, and the warnings are not the assertion.
func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// errNoRoute stands in for the machine with no way out.
var errNoRoute = errors.New("no route to host")

// sha256Hex is the pin's spelling: the digest a shelf compares against.
func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func TestAHostThatAnswersAnythingButOKIsNeitherServedNorRemembered(t *testing.T) {
	t.Parallel()
	var asks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asks.Add(1)
		http.Error(w, "the shop is shut", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	s := NewWith(ownTable(t), t.TempDir(), redirecting(srv), quiet())
	ctx := context.Background()

	if got := s.Symbol(ctx, "W"); got != "" {
		t.Fatalf("a 500 was served as a symbol: %q", got)
	}
	// A 500 is trouble, not absence: unlike a 404 it is not remembered, so
	// the next ask goes back to the network and can succeed.
	if got := s.Symbol(ctx, "W"); got != "" || asks.Load() != 2 {
		t.Fatalf("a 500 was remembered as absence (%d asks, %q)", asks.Load(), got)
	}
	if got := s.OCR(ctx, "worker.min.js"); got != "" {
		t.Fatalf("a 500 was served as a reading-engine asset: %q", got)
	}
	if entries, err := os.ReadDir(s.SymbolsDir()); err == nil && len(entries) > 0 {
		t.Fatalf("the shelf holds %d files after three refusals", len(entries))
	}
}

func TestAnAssetOverTheCapIsRefusedBeforeItsDigestIsRead(t *testing.T) {
	t.Parallel()
	// The committed cap is sixteen megabytes, which is a real bound and a
	// silly thing to allocate in a test; the shelves take their cap as a
	// value so the same refusal can be driven with nine bytes.
	conf := ownTable(t)
	conf.OCR.MaxBytes = 8
	body := []byte("nine byte")
	// The pin is made to match these bytes, so the only thing that can refuse
	// them is the cap.
	asset := conf.OCR.Assets["worker.min.js"]
	asset.Digest = sha256Hex(body)
	conf.OCR.Assets["worker.min.js"] = asset

	srv := cdn(t, map[string][]byte{
		"/npm/tesseract.js@7.0.0/dist/worker.min.js": body,
	}, new(int32))
	s := NewWith(conf, t.TempDir(), redirecting(srv), quiet())
	if got := s.OCR(context.Background(), "worker.min.js"); got != "" {
		t.Fatalf("an asset over the cap was served: %q", got)
	}
	if _, err := os.Stat(filepath.Join(s.OCRDir(), "worker.min.js")); err == nil {
		t.Fatal("an asset over the cap reached the shelf")
	}
}

func TestAShelfThatCannotBeWrittenServesNothingRatherThanHalfAFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A regular file standing where the cache directory belongs: the
	// download succeeds, the write cannot, and the ask has to come back
	// empty rather than pretending.
	if err := os.MkdirAll(filepath.Join(dir, "cache"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cache", "symbols"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf := ownTable(t)
	stamp := conf.OCR.CacheStamp
	if err := os.MkdirAll(filepath.Join(dir, "cache", "ocr"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cache", "ocr", stamp), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	srv := cdn(t, map[string][]byte{
		"/card-symbols/W.svg":                        body,
		"/npm/tesseract.js@7.0.0/dist/worker.min.js": body,
	}, new(int32))
	// The pin is made to match, so the only thing left to fail is the write.
	asset := conf.OCR.Assets["worker.min.js"]
	asset.Digest = sha256Hex(body)
	conf.OCR.Assets["worker.min.js"] = asset

	s := NewWith(conf, dir, redirecting(srv), quiet())
	ctx := context.Background()
	if got := s.Symbol(ctx, "W"); got != "" {
		t.Fatalf("a symbol that could not be cached was served: %q", got)
	}
	if got := s.OCR(ctx, "worker.min.js"); got != "" {
		t.Fatalf("an asset that could not be cached was served: %q", got)
	}
}

func TestANetworkThatIsNotThereIsNotAnAnswer(t *testing.T) {
	t.Parallel()
	// A cold cache on a machine with no way out: every shelf answers "" and
	// nothing is remembered, because a transport failure is transient and
	// must not be written down as absence.
	dead := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errNoRoute
	})}
	s := NewWith(ownTable(t), t.TempDir(), dead, quiet())
	ctx := context.Background()
	if got := s.Symbol(ctx, "W"); got != "" {
		t.Fatalf("a symbol came back with no network: %q", got)
	}
	if got := s.OCR(ctx, "worker.min.js"); got != "" {
		t.Fatalf("an asset came back with no network: %q", got)
	}
	s.mu.Lock()
	missing, refused := len(s.missing), len(s.refused)
	s.mu.Unlock()
	if missing != 0 || refused != 0 {
		t.Fatalf("a failed transport was remembered (%d missing, %d refused)", missing, refused)
	}
}

func TestADerivativeWithUnreadableAttributionIsSkippedRatherThanServed(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir(), nil, quiet())
	fp := reference.Runtime().Cardmotion.Effects["depth-drift"].Fingerprint
	broken := filepath.Join(s.CardmotionDir(), "aaa")
	if err := os.MkdirAll(broken, 0o750); err != nil {
		t.Fatal(err)
	}
	// Half a JSON object: the encoder died, or the volume filled. Either way
	// the directory is not a derivative and the scan moves on.
	if err := os.WriteFile(filepath.Join(broken, "attribution.json"), []byte(`{"oracle_id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if hit, ok := s.FindReady("o1", fp, nil); ok {
		t.Fatalf("a derivative with unreadable attribution was served: %+v", hit)
	}
}

func TestADownloadRefusesAnAddressItCannotEvenAskFor(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir(), nil, quiet())
	// A control character is not a URL, so the request is never built and
	// the shelves see a transport error rather than a status.
	body, status, err := s.download(context.Background(), "http://example.invalid/\x7f", 64, 5*time.Second)
	if err == nil {
		t.Fatalf("a malformed address was requested: %q %d", body, status)
	}
	if body != nil || status != 0 {
		t.Fatalf("a refused request still answered %q %d", body, status)
	}
}

func TestABodyThatStopsShortIsAnErrorRatherThanAShortFile(t *testing.T) {
	t.Parallel()
	// A response that promises sixty-four bytes and closes after five. The
	// bytes that did arrive must never be treated as the file: a truncated
	// symbol cached is a broken glyph served for ever.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		out := bufio.NewWriter(conn)
		_, _ = out.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 64\r\n\r\nshort")
		_ = out.Flush()
	}))
	t.Cleanup(srv.Close)
	s := New(t.TempDir(), redirecting(srv), quiet())
	_, status, err := s.download(context.Background(), "http://example.invalid/x.svg", 4096, 5*time.Second)
	if err == nil {
		t.Fatal("a body that stopped short was read as whole")
	}
	if status != http.StatusOK {
		t.Fatalf("the status was lost: %d", status)
	}
	if got := s.Symbol(context.Background(), "W"); got != "" {
		t.Fatalf("a truncated symbol was cached: %q", got)
	}
}

func TestWriteAtomicReportsEveryWayThePathCanRefuse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// A file standing where a directory has to be made.
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(blocked, "W.svg"), []byte("x")); err == nil {
		t.Fatal("a directory was made inside a file")
	}

	// A directory that exists and will not take a new file.
	readonly := filepath.Join(root, "readonly")
	if err := os.Mkdir(readonly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })
	if err := writeAtomic(filepath.Join(readonly, "W.svg"), []byte("x")); err == nil {
		t.Fatal("a staged file was written into a read-only directory")
	}

	// A directory sitting on the target's name: the stage is written and the
	// rename cannot land, and the stage must not be left behind.
	occupied := filepath.Join(root, "occupied", "W.svg")
	if err := os.MkdirAll(filepath.Join(occupied, "inside"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(occupied, []byte("x")); err == nil {
		t.Fatal("a file was renamed over a directory")
	}
	entries, err := os.ReadDir(filepath.Dir(occupied))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("a stage was left behind: %s", e.Name())
		}
	}

	// And the ordinary case still lands, so the sweep above is not passing
	// because writeAtomic refuses everything.
	target := filepath.Join(root, "good", "W.svg")
	if err := writeAtomic(target, []byte("<svg/>")); err != nil {
		t.Fatalf("an ordinary write: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "<svg/>" {
		t.Fatalf("wrote %q", got)
	}
}
