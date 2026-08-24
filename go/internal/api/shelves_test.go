package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/shelves"
)

// A shelves set over a temp data dir with no network: the fake client
// refuses every request, so only what is already on the shelf serves.
func offlineShelves(t *testing.T) *shelves.Shelves {
	t.Helper()
	client := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return nil, os.ErrDeadlineExceeded
	})}
	return shelves.New(t.TempDir(), client, nil)
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTheShelvesRefuseWhatTheyDoNotHold(t *testing.T) {
	t.Parallel()
	sh := offlineShelves(t)
	a := New(Config{Shelves: sh})
	for target, detail := range map[string]string{
		"/api/symbols/NOT-A-SYMBOL.svg":                             "no such symbol",
		"/api/symbols/W.svg":                                        "no such symbol", // cold cache, no network
		"/api/ocr/no-such-file":                                     "no such reader file",
		"/api/ocr/worker.min.js":                                    "no such reader file", // cold cache, no network
		"/api/art/motion/0aae2e33-0000-4000-8000-000000000000/nope": "no effect 'nope'",
		"/api/art/motion/0aae2e33-0000-4000-8000-000000000000/depth-drift/loop.webm": "no such derivative",
		"/api/art/motion/0aae2e33-0000-4000-8000-000000000000/depth-drift/evil.sh":   "no such derivative",
	} {
		status, body, raw := call(t, a, "GET", target, "")
		if status != 404 || body["detail"] != detail {
			t.Errorf("%s: %d %s", target, status, raw)
		}
	}
	// Not ready is a complete, correct answer.
	status, body, raw := call(t, a, "GET", "/api/art/motion/0aae2e33-0000-4000-8000-000000000000/depth-drift", "")
	if status != 200 || string(raw) != `{"ready":false,"effect":"depth-drift"}` {
		t.Fatalf("%d %s %v", status, raw, body)
	}
	// No shelves at all: the same refusals.
	none := New(Config{})
	if status, _, _ := call(t, none, "GET", "/api/symbols/W.svg", ""); status != 404 {
		t.Fatal("no shelves should be a 404")
	}
	if _, _, raw := call(t, none, "GET", "/api/art/motion/x/breath", ""); string(raw) != `{"ready":false,"effect":"breath"}` {
		t.Fatalf("%s", raw)
	}
}

func TestTheShelvesServeWhatTheyHold(t *testing.T) {
	t.Parallel()
	sh := offlineShelves(t)
	a := New(Config{Shelves: sh})
	// A symbol already on the shelf, and a reader file under the stamp.
	_ = os.MkdirAll(sh.SymbolsDir(), 0o755)
	_ = os.WriteFile(filepath.Join(sh.SymbolsDir(), "W.svg"), []byte(`<svg/>`), 0o644)
	_ = os.MkdirAll(sh.OCRDir(), 0o755)
	_ = os.WriteFile(filepath.Join(sh.OCRDir(), "worker.min.js"), []byte(`worker`), 0o644)
	_ = os.WriteFile(filepath.Join(sh.OCRDir(), "eng.traineddata.gz"), []byte{0x1f, 0x8b}, 0o644)
	for target, want := range map[string][2]string{
		"/api/symbols/W.svg":          {"image/svg+xml", "public, max-age=604800"},
		"/api/symbols/w.svg":          {"image/svg+xml", "public, max-age=604800"}, // upper-cased
		"/api/ocr/worker.min.js":      {"text/javascript; charset=utf-8", "public, max-age=31536000, immutable"},
		"/api/ocr/eng.traineddata.gz": {"application/gzip", "public, max-age=31536000, immutable"},
	} {
		rec := serve(t, a, target)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != want[0] || rec.Header().Get("Cache-Control") != want[1] {
			t.Errorf("%s: %d %q %q", target, rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
		}
		if rec.Header().Get("Last-Modified") == "" {
			t.Errorf("%s: no Last-Modified", target)
		}
	}
	// A derivative: status names its files with the fingerprint stamped on,
	// and the file route serves them with the art riding along.
	fp := reference.Runtime().Cardmotion.Effects["breath"].Fingerprint
	ddir := filepath.Join(sh.CardmotionDir(), "abc")
	_ = os.MkdirAll(ddir, 0o755)
	_ = os.WriteFile(filepath.Join(ddir, "loop.webm"), []byte("webm"), 0o644)
	_ = os.WriteFile(filepath.Join(ddir, "poster.webp"), []byte("webp"), 0o644)
	meta, _ := json.Marshal(map[string]any{"oracle_id": "o1", "fingerprint": fp, "art_url": "https://x/a.jpg?1",
		"artist": "Someone", "card_name": "A Card", "effect": "breath"})
	_ = os.WriteFile(filepath.Join(ddir, "attribution.json"), meta, 0o644)
	status, body, raw := call(t, a, "GET", "/api/art/motion/o1/breath?art=https%3A%2F%2Fx%2Fa.jpg%3F2", "")
	if status != 200 || body["ready"] != true || body["fingerprint"] != fp {
		t.Fatalf("%d %s", status, raw)
	}
	urls := body["urls"].(map[string]any)
	if urls["webm"] != "/api/art/motion/o1/breath/loop.webm?v="+fp+"&art=https%3A%2F%2Fx%2Fa.jpg%3F2" || urls["poster"] == nil || urls["mp4"] != nil {
		t.Fatalf("urls %v", urls)
	}
	if body["attribution"].(map[string]any)["artist"] != "Someone" {
		t.Fatalf("attribution %v", body["attribution"])
	}
	if !strings.HasPrefix(string(raw), `{"ready":true,"effect":"breath","fingerprint":"`) {
		t.Fatalf("order %.60s", raw)
	}
	rec := serve(t, a, "/api/art/motion/o1/breath/loop.webm")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "video/webm" || rec.Body.String() != "webm" ||
		rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("%d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	// A Range request is honoured, as a video element needs.
	req := httptest.NewRequest("GET", "/api/art/motion/o1/breath/loop.webm", nil)
	req.Header.Set("Range", "bytes=1-2")
	req.SetPathValue("oracle_id", "o1")
	req.SetPathValue("effect", "breath")
	req.SetPathValue("filename", "loop.webm")
	rec = httptest.NewRecorder()
	a.artMotionFile(rec, req)
	if rec.Code != 206 || rec.Body.String() != "eb" {
		t.Fatalf("range: %d %q", rec.Code, rec.Body.String())
	}
	// The wrong painting: not ready, and not served.
	if _, body, _ := call(t, a, "GET", "/api/art/motion/o1/breath?art=https%3A%2F%2Fx%2Fother.jpg", ""); body["ready"] != false {
		t.Fatal("a different painting was offered")
	}
	if status, _, _ := call(t, a, "GET", "/api/art/motion/o1/breath/loop.webm?art=https%3A%2F%2Fx%2Fother.jpg", ""); status != 404 {
		t.Fatal("a different painting's file was served")
	}
}

// serve runs a GET through the route matcher into a recorder, for the
// routes that answer bytes rather than JSON.
func serve(t *testing.T, a *API, target string) *httptest.ResponseRecorder {
	t.Helper()
	path := strings.SplitN(target, "?", 2)[0]
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, r := range a.Routes() {
		segs := strings.Split(strings.TrimPrefix(r.Pattern, "/"), "/")
		if r.Method != "GET" || len(segs) != len(parts) {
			continue
		}
		req := httptest.NewRequest("GET", target, nil)
		matched := true
		for i, seg := range segs {
			if strings.HasPrefix(seg, "{") {
				name, suffix, _ := strings.Cut(seg[1:], "}")
				if suffix != "" && !strings.HasSuffix(parts[i], suffix) {
					matched = false
					break
				}
				req.SetPathValue(name, strings.TrimSuffix(parts[i], suffix))
				continue
			}
			if seg != parts[i] {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		rec := httptest.NewRecorder()
		r.Handler(rec, req)
		return rec
	}
	t.Fatalf("no route for %s", target)
	return nil
}
