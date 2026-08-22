package shelves

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// cdn stands in for Scryfall and jsDelivr: every request lands on one
// handler that answers by path.
func cdn(t *testing.T, routes map[string][]byte, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// redirecting sends every request for a host the test does not run to the
// fake CDN, so the real URLs in shelves.json are exercised as written.
func redirecting(srv *httptest.Server) *http.Client {
	u := srv.URL
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme = "http"
		r2.URL.Host = strings.TrimPrefix(u, "http://")
		return http.DefaultTransport.RoundTrip(r2)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSymbolsAreFetchedOnceAndRefusedWhenMalformed(t *testing.T) {
	var hits int32
	srv := cdn(t, map[string][]byte{
		"/card-symbols/W.svg":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"/card-symbols/HTML.svg": []byte(`<html>captive portal</html>`),
		"/card-symbols/BIG.svg":  []byte("<svg>" + strings.Repeat("x", 70000)),
	}, &hits)
	s := New(t.TempDir(), redirecting(srv), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx := context.Background()
	path := s.Symbol(ctx, "W")
	if path == "" || !strings.HasSuffix(path, filepath.Join("cache", "symbols", "W.svg")) {
		t.Fatalf("W -> %q", path)
	}
	if again := s.Symbol(ctx, "W"); again != path || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("a second ask went to the network (%d hits)", hits)
	}
	// A code Scryfall has never heard of is remembered as missing: the
	// second ask does not reach the network.
	if s.Symbol(ctx, "NOPE") != "" {
		t.Fatal("a missing symbol was served")
	}
	if s.Symbol(ctx, "NOPE") != "" || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("missing was re-asked (%d hits)", hits)
	}
	// Not an SVG, over the cap, malformed: nothing cached, nothing served.
	if s.Symbol(ctx, "HTML") != "" || s.Symbol(ctx, "BIG") != "" || s.Symbol(ctx, "w/u") != "" ||
		s.Symbol(ctx, "../etc") != "" || s.Symbol(ctx, "") != "" {
		t.Fatal("a bad symbol was served")
	}
	if entries, _ := os.ReadDir(s.SymbolsDir()); len(entries) != 1 {
		t.Fatalf("cache holds %d files", len(entries))
	}
}

func TestOCRAssetsArePinnedByDigest(t *testing.T) {
	good := []byte("/*! worker */ console.log('hi')")
	sum := sha256.Sum256(good)
	// The real table, with its pins, is what the code reads; the fake CDN
	// answers the real URLs. One asset's bytes are made to match a fake pin
	// by pointing the table at... no: the pins are the real ones, so the
	// only honest test of a *match* is a file whose digest we can set. The
	// shelves read the table from reference; this test therefore checks
	// the mismatch path on a real asset and the match path through a table
	// entry whose digest is this body's.
	srv := cdn(t, map[string][]byte{
		"/npm/tesseract.js@7.0.0/dist/worker.min.js":             good,
		"/npm/tesseract.js@7.0.0/dist/worker.min.js.LICENSE.txt": []byte("MIT"),
	}, new(int32))
	s := New(t.TempDir(), redirecting(srv), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx := context.Background()
	// Bytes that do not match the pin are refused, and stay refused.
	if s.OCR(ctx, "worker.min.js") != "" {
		t.Fatal("a digest mismatch was cached")
	}
	if _, err := os.Stat(filepath.Join(s.OCRDir(), "worker.min.js")); err == nil {
		t.Fatal("the refused bytes are on the shelf")
	}
	if !s.refused["worker.min.js"] {
		t.Fatal("the refusal was not remembered")
	}
	// An unknown name never touches the network or the filesystem.
	if s.OCR(ctx, "../../etc/passwd") != "" || s.OCR(ctx, "nope") != "" {
		t.Fatal("an unknown asset was answered")
	}
	// The match path: pretend the pin is this body's digest.
	asset := reference.Runtime().OCR.Assets["worker.min.js"]
	asset.Digest = hex.EncodeToString(sum[:])
	reference.Runtime().OCR.Assets["worker.min.js"] = asset
	t.Cleanup(func() {
		asset.Digest = "576b7df7e3393e137e51849357c9adb53fe7ac1bb69bfa06cf3d61520f182c6d"
		reference.Runtime().OCR.Assets["worker.min.js"] = asset
	})
	s2 := New(t.TempDir(), redirecting(srv), nil)
	path := s2.OCR(ctx, "worker.min.js")
	if path == "" || !strings.Contains(path, reference.Runtime().OCR.CacheStamp) {
		t.Fatalf("a matching asset was not cached under the versioned stamp: %q", path)
	}
	if body, _ := os.ReadFile(path); string(body) != string(good) {
		t.Fatal("cached bytes differ")
	}
}

func TestFindReadyMatchesTheOracleTheEffectAndThePainting(t *testing.T) {
	s := New(t.TempDir(), nil, nil)
	fp := reference.Runtime().Cardmotion.Effects["depth-drift"].Fingerprint
	write := func(dir string, meta map[string]any, files ...string) {
		full := filepath.Join(s.CardmotionDir(), dir)
		_ = os.MkdirAll(full, 0o755)
		for _, f := range files {
			_ = os.WriteFile(filepath.Join(full, f), []byte("bytes"), 0o644)
		}
		if meta != nil {
			raw, _ := json.Marshal(meta)
			_ = os.WriteFile(filepath.Join(full, "attribution.json"), raw, 0o644)
		}
	}
	write("aaa", map[string]any{"oracle_id": "o1", "fingerprint": fp, "art_url": "https://x/art.jpg?1"}, "loop.webm", "poster.webp")
	write("bbb", map[string]any{"oracle_id": "o1", "fingerprint": "other"}, "loop.webm")
	write("ccc", nil, "loop.webm") // died mid-encode: no attribution
	write("ddd", map[string]any{"oracle_id": "o1", "fingerprint": fp, "art_url": "https://x/other.jpg?2"}, "loop.mp4")
	// Any art: the first ready match in sorted order.
	hit, ok := s.FindReady("o1", fp, nil)
	if !ok || filepath.Base(hit.Dir) != "aaa" || !hit.Has("loop.webm") || hit.Has("loop.mp4") || hit.Fingerprint() != fp {
		t.Fatalf("%+v %v", hit, ok)
	}
	// The painting the page shows, query string and all: a match on the stem.
	art := "https://x/other.jpg?99"
	if hit, ok := s.FindReady("o1", fp, &art); !ok || filepath.Base(hit.Dir) != "ddd" {
		t.Fatalf("art match %+v %v", hit, ok)
	}
	third := "https://x/third.jpg"
	if _, ok := s.FindReady("o1", fp, &third); ok {
		t.Fatal("a different painting matched")
	}
	if _, ok := s.FindReady("o2", fp, nil); ok {
		t.Fatal("another oracle matched")
	}
	if _, ok := New(filepath.Join(t.TempDir(), "absent"), nil, nil).FindReady("o1", fp, nil); ok {
		t.Fatal("a missing cache matched")
	}
	if ArtStem("https://a/b.jpg?x=1") != "https://a/b.jpg" || ArtStem("plain") != "plain" {
		t.Fatal("art stem")
	}
}
