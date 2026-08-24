// Package shelves is the three runtime shelves under `data/cache/`, filled
// once and served first-party ever after (ADR 32, ADR 33). A symbol is fetched
// from Scryfall's CDN the first time anybody asks; a reading-engine file is
// fetched once and **refused unless its SHA-256 matches the pin**; a
// card-art derivative is never fetched at all -- it sits in the cache
// because a dev-machine build put it there, or it does not exist. Nothing
// here reaches the pool, nothing renders from a third party's origin, and
// every answer a shelf cannot give is a 404 the client already knows how to
// take (the drawn glyphs, the still).
//
// The configuration -- the CDN, the shape a symbol code may take, the
// pinned files and their digests, the effects table with the toolbox's
// fingerprints -- is `reference/data/shelves.json`, so a pin is bumped in
// one committed place and served from it.
package shelves

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// UserAgent is the one identity every request this app makes carries --
// the string Scryfall has always seen from it.
const UserAgent = "mtg-lab/0.1 (local personal deckbuilding tool)"

// Shelves holds the three caches under one data directory.
type Shelves struct {
	DataDir string
	Client  *http.Client
	Log     *slog.Logger

	mu      sync.Mutex
	missing map[string]bool // symbol codes Scryfall answered 404 for
	refused map[string]bool // ocr assets whose bytes did not match their pin
}

// New is the shelves under dataDir. The client is the network, and a test
// hands over one pointed at its own server.
func New(dataDir string, client *http.Client, log *slog.Logger) *Shelves {
	if log == nil {
		log = slog.Default()
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Shelves{DataDir: dataDir, Client: client, Log: log,
		missing: map[string]bool{}, refused: map[string]bool{}}
}

// ---- the mana symbols ---------------------------------------------------

var symbolCode = regexp.MustCompile("^" + reference.Runtime().Symbols.Code + "$")

// SymbolsDir is `symbols.cache_dir`.
func (s *Shelves) SymbolsDir() string { return filepath.Join(s.DataDir, "cache", "symbols") }

// Symbol is `symbols.ensure`: the cached SVG for one code, downloaded on
// the first ask. "" means "no such symbol here today": a malformed code, a
// code Scryfall has never heard of, or a cold cache with no network -- the
// caller answers 404 and the client falls back to its drawn glyphs.
func (s *Shelves) Symbol(ctx context.Context, code string) string {
	if !symbolCode.MatchString(code) {
		return ""
	}
	target := filepath.Join(s.SymbolsDir(), code+".svg")
	if isFile(target) {
		return target
	}
	s.mu.Lock()
	known := s.missing[code]
	s.mu.Unlock()
	if known {
		return ""
	}
	cfg := reference.Runtime().Symbols
	body, status, err := s.download(ctx, cfg.CDN+"/"+code+".svg", cfg.MaxBytes, 10*time.Second)
	if err != nil {
		// Network trouble is transient and must not be remembered as absence.
		s.Log.Warn("symbol: download failed", "code", code, "error", err)
		return ""
	}
	if status == http.StatusNotFound {
		s.mu.Lock()
		s.missing[code] = true
		s.mu.Unlock()
		return ""
	}
	if status != http.StatusOK {
		s.Log.Warn("symbol: Scryfall answered", "code", code, "status", status)
		return ""
	}
	// A captive portal's login page is not a symbol: light on purpose, it
	// only has to keep non-SVG bytes out of the cache.
	head := body
	if len(head) > 1024 {
		head = head[:1024]
	}
	if int64(len(body)) > cfg.MaxBytes || !strings.Contains(string(head), "<svg") {
		s.Log.Warn("symbol: response did not look like an SVG", "code", code)
		return ""
	}
	if err := writeAtomic(target, body); err != nil {
		s.Log.Warn("symbol: could not cache", "code", code, "error", err)
		return ""
	}
	return target
}

// ---- the reading engine -------------------------------------------------

// OCRDir is `ocr.cache_dir`: versioned, so bumping a pin cannot serve
// yesterday's bytes off a volume nobody thought to clear.
func (s *Shelves) OCRDir() string {
	return filepath.Join(s.DataDir, "cache", "ocr", reference.Runtime().OCR.CacheStamp)
}

// OCRAsset is `ocr.ASSETS[name]`, or false: the name must be a key of the
// table, which is the whole path-traversal story.
func OCRAsset(name string) (reference.OCRAsset, bool) {
	a, ok := reference.Runtime().OCR.Assets[name]
	return a, ok
}

// OCR is `ocr.ensure`: the cached file for one asset name, fetching it on
// the first ask and **refusing, loudly and stickily, bytes that do not
// match their pinned digest**. "" means not available here today.
func (s *Shelves) OCR(ctx context.Context, name string) string {
	asset, ok := OCRAsset(name)
	if !ok {
		return ""
	}
	target := filepath.Join(s.OCRDir(), name)
	if isFile(target) {
		return target
	}
	s.mu.Lock()
	refused := s.refused[name]
	s.mu.Unlock()
	if refused {
		return ""
	}
	cfg := reference.Runtime().OCR
	body, status, err := s.download(ctx, asset.URL, cfg.MaxBytes, 30*time.Second)
	if err != nil {
		s.Log.Warn("ocr asset: download failed", "name", name, "error", err)
		return ""
	}
	if status != http.StatusOK {
		s.Log.Warn("ocr asset: host answered", "name", name, "status", status)
		return ""
	}
	if int64(len(body)) > cfg.MaxBytes {
		s.Log.Warn("ocr asset: over the size cap", "name", name)
		return ""
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != asset.Digest {
		s.Log.Error("ocr asset: digest mismatch, refusing to cache", "name", name,
			"expected", asset.Digest, "got", got)
		s.mu.Lock()
		s.refused[name] = true
		s.mu.Unlock()
		return ""
	}
	if err := writeAtomic(target, body); err != nil {
		s.Log.Warn("ocr asset: could not cache", "name", name, "error", err)
		return ""
	}
	return target
}

// ---- card-art motion ----------------------------------------------------

// CardmotionDir is `cardmotion.cache.root`.
func (s *Shelves) CardmotionDir() string { return filepath.Join(s.DataDir, "cache", "cardmotion") }

// Derivative is one derivative's directory, ready: its attribution exists,
// which is written last, so a build that died mid-encode reads as absent.
type Derivative struct {
	Dir         string
	Attribution map[string]any
}

// File is the path of one member.
func (d Derivative) File(name string) string { return filepath.Join(d.Dir, name) }

// Has reports whether a member is on disk.
func (d Derivative) Has(name string) bool { return isFile(d.File(name)) }

// Fingerprint is the attribution's.
func (d Derivative) Fingerprint() string {
	f, _ := d.Attribution["fingerprint"].(string)
	return f
}

// ArtStem is `cardmotion.cache.art_stem`: a Scryfall image URL sans query
// string -- the path names the painting, the query is a cache-buster.
func ArtStem(url string) string {
	stem, _, _ := strings.Cut(url, "?")
	return stem
}

// FindReady is `cardmotion.cache.find_ready`: the derivative for an
// oracle id and effect, and -- when the page says which painting it is
// showing -- for that painting exactly; no match is "not ready", never the
// nearest neighbour. Dozens of directories, scanned in order.
func (s *Shelves) FindReady(oracleID, fingerprint string, artURL *string) (*Derivative, bool) {
	base := s.CardmotionDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, false
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		dir := filepath.Join(base, name)
		raw, err := os.ReadFile(filepath.Join(dir, "attribution.json"))
		if err != nil {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		if meta["oracle_id"] != oracleID || meta["fingerprint"] != fingerprint {
			continue
		}
		if artURL != nil {
			have, _ := meta["art_url"].(string)
			if ArtStem(have) != ArtStem(*artURL) {
				continue
			}
		}
		return &Derivative{Dir: dir, Attribution: meta}, true
	}
	return nil, false
}

// ---- plumbing -----------------------------------------------------------

// download is one bounded GET: the body up to max+1 bytes (so the cap can be
// seen to be exceeded), the status, or a transport error.
func (s *Shelves) download(ctx context.Context, url string, maxBytes int64, timeout time.Duration) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// writeAtomic stages under a unique name and renames into place: two cold
// asks each write whole bytes and whichever lands second wins with an
// identical file, where a shared stage could leave a torn one forever.
func writeAtomic(target string, body []byte) error {
	// The cache is the app's own, read by the one account that writes it.
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	stage := filepath.Join(filepath.Dir(target), fmt.Sprintf(".%s.%s.tmp", filepath.Base(target), hex.EncodeToString(nonce[:])))
	if err := os.WriteFile(stage, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		_ = os.Remove(stage)
		return err
	}
	return nil
}

// ErrNotFound is the shelves' one refusal.
var ErrNotFound = errors.New("not on the shelf")
