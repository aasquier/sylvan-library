package api

import (
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The three runtime shelves (the read spine's last family): one official
// mana-symbol SVG, one file of the reading engine, and a card-art motion
// derivative's status and files. Each serves a file off `data/cache/` with
// an explicit media type -- the container has no /etc/mime.types, the tarot
// lesson -- and the caching policy `api/app.py` gives it; a 404 is a
// complete answer the client already knows how to take.

// serveShelfFile answers one regular file with an explicit media type and
// cache policy. `http.ServeContent` supplies Last-Modified, the conditional
// 304 and Range handling -- a video element seeks with Range requests, and
// Starlette's FileResponse answers them too.
func serveShelfFile(w http.ResponseWriter, r *http.Request, path, mediaType, cacheControl string) bool {
	// The path is the shelf's own -- a validated symbol code, a key of the
	// reader table, or a member of a derivative found by scan, each under
	// the cache directory -- and never the request's; the allowlists above
	// are the guard.
	f, err := os.Open(path) //nolint:gosec // see the comment above
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	h := w.Header()
	h.Set("Content-Type", mediaType)
	h.Set("Cache-Control", cacheControl)
	http.ServeContent(w, r, "", info.ModTime(), f)
	return true
}

// symbolSVG is `GET /api/symbols/{code}.svg` -- `api/app.py:symbol_svg`:
// one official mana-symbol SVG, the code upper-cased, the module's shape
// check the path-traversal guard; a week of caching, since there is no
// version stamp to bust with and the set moves a few times a year.
func (a *API) symbolSVG(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSuffix(r.PathValue("code"), ".svg"))
	if a.shelves == nil {
		wire.Detail(w, http.StatusNotFound, "no such symbol")
		return
	}
	path := a.shelves.Symbol(r.Context(), code)
	if path == "" || !serveShelfFile(w, r, path, "image/svg+xml", "public, max-age=604800") {
		wire.Detail(w, http.StatusNotFound, "no such symbol")
	}
}

// ocrAsset is `GET /api/ocr/{name}` -- `api/app.py:ocr_asset`: one file of
// the reading engine off the shelf `ocr.py` fills; immutable, because the
// cache path carries the pinned versions.
func (a *API) ocrAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	asset, ok := reference.Runtime().OCR.Assets[name]
	if !ok || a.shelves == nil {
		wire.Detail(w, http.StatusNotFound, "no such reader file")
		return
	}
	path := a.shelves.OCR(r.Context(), name)
	mediaType := asset.MediaType
	// Starlette adds the charset to every text/* it answers.
	if strings.HasPrefix(mediaType, "text/") && !strings.Contains(mediaType, "charset=") {
		mediaType += "; charset=utf-8"
	}
	if path == "" || !serveShelfFile(w, r, path, mediaType, "public, max-age=31536000, immutable") {
		wire.Detail(w, http.StatusNotFound, "no such reader file")
	}
}

// artMotionTypes is `api/app.py:_ART_MOTION_TYPES`: the allowlist is the
// path-traversal guard -- `filename` never reaches the filesystem unless it
// is one of four fixed names.
var artMotionTypes = map[string]string{
	"loop.webm": "video/webm", "loop.mp4": "video/mp4",
	"poster.webp": "image/webp", "depth.png": "image/png",
}

// artMotionStatus is `GET /api/art/motion/{oracle_id}/{effect}` -- is there a
// motion derivative for this painting? `ready: false` is a complete, correct
// answer. `art` is the crop the page is showing: a deck that picked a
// printing must not be handed a loop derived from a different painting.
func (a *API) artMotionStatus(w http.ResponseWriter, r *http.Request) {
	oracleID, effectKey := r.PathValue("oracle_id"), r.PathValue("effect")
	effect, ok := reference.Runtime().Cardmotion.Effects[effectKey]
	if !ok {
		wire.Detail(w, http.StatusNotFound, "no effect "+wire.PyRepr(effectKey))
		return
	}
	var art *string
	if vals := r.URL.Query()["art"]; len(vals) > 0 && vals[len(vals)-1] != "" {
		v := vals[len(vals)-1]
		art = &v
	}
	notReady := wire.OrderedMap([]wire.KV{{Key: "ready", Value: false}, {Key: "effect", Value: effectKey}})
	if a.shelves == nil {
		raw, _ := wire.MarshalOrdered(notReady)
		wire.Raw(w, http.StatusOK, raw)
		return
	}
	hit, ok := a.shelves.FindReady(oracleID, effect.Fingerprint, art)
	if !ok {
		raw, _ := wire.MarshalOrdered(notReady)
		wire.Raw(w, http.StatusOK, raw)
		return
	}
	meta := hit.Attribution
	stamp, _ := meta["fingerprint"].(string)
	base := "/api/art/motion/" + oracleID + "/" + effectKey
	suffix := ""
	if art != nil {
		suffix = "&art=" + url.QueryEscape(*art)
	}
	// The art rides on the file URLs too: two printings of one commander are
	// two derivatives under one oracle_id, and the file route must land on
	// the same one the status answer described.
	keys := map[string]string{"loop.webm": "webm", "loop.mp4": "mp4", "poster.webp": "poster", "depth.png": "depth"}
	servable := append([]string{}, reference.Runtime().Cardmotion.Servable...)
	sort.Strings(servable)
	urls := []wire.KV{}
	for _, name := range servable {
		if hit.Has(name) {
			urls = append(urls, wire.KV{Key: keys[name], Value: base + "/" + name + "?v=" + stamp + suffix})
		}
	}
	raw, err := wire.MarshalOrdered([]wire.KV{{Key: "ready", Value: true}, {Key: "effect", Value: effectKey}, {Key: "fingerprint", Value: stamp},
		{Key: "urls", Value: wire.OrderedMap(urls)}, {Key: "attribution", Value: meta}})
	if err != nil {
		a.fail(w, "art/motion", err)
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// artMotionFile is `GET /api/art/motion/{oracle_id}/{effect}/{filename}` --
// one derivative file, long-lived caching safe because the status payload's
// URLs carry the fingerprint as a version stamp.
func (a *API) artMotionFile(w http.ResponseWriter, r *http.Request) {
	effectKey, filename := r.PathValue("effect"), r.PathValue("filename")
	effect, effectOK := reference.Runtime().Cardmotion.Effects[effectKey]
	mediaType, nameOK := artMotionTypes[filename]
	if !effectOK || !nameOK || a.shelves == nil {
		wire.Detail(w, http.StatusNotFound, "no such derivative")
		return
	}
	var art *string
	if vals := r.URL.Query()["art"]; len(vals) > 0 && vals[len(vals)-1] != "" {
		v := vals[len(vals)-1]
		art = &v
	}
	hit, ok := a.shelves.FindReady(r.PathValue("oracle_id"), effect.Fingerprint, art)
	if !ok || !hit.Has(filename) || !serveShelfFile(w, r, hit.File(filename), mediaType, "public, max-age=31536000, immutable") {
		wire.Detail(w, http.StatusNotFound, "no such derivative")
	}
}
