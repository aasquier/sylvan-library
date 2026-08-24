package door

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The content types the door answers, by extension, named outright rather
// than asked of the host: Go's `mime` package reads the host's
// `/etc/mime.types` on top of its built-in table, so the answer would
// otherwise depend on where the binary ran. These are the recorded serving
// types -- the deployed container's table, with `; charset=utf-8` on every
// `text/*` -- captured on 2026-08-21 and pinned by
// `TestContentTypesMatchTheContainer`.
var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".htm":   "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".json":  "application/json",
	".md":    "text/markdown; charset=utf-8",
	".txt":   "text/plain; charset=utf-8",
	".svg":   "image/svg+xml",
	".webp":  "image/webp",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".ico":   "image/vnd.microsoft.icon",
	".woff2": "font/woff2",
	".woff":  "font/woff",
	".mp4":   "video/mp4",
	".webm":  "video/webm",
	".wasm":  "application/wasm",
	".xml":   "text/xml; charset=utf-8",
	".pdf":   "application/pdf",
}

// The recorded fallback for an extension the table does not name.
const fallbackContentType = "text/plain; charset=utf-8"

func init() {
	// Teach the process's own table too, so anything else in this binary that
	// asks `mime` gets the same answer the door gives.
	for ext, typ := range contentTypes {
		_ = mime.AddExtensionType(ext, typ)
	}
}

// ContentType is the type the door serves a file as.
func ContentType(name string) string {
	if typ, ok := contentTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return typ
	}
	return fallbackContentType
}

// staticSite is the shell, its assets, and the tarot pictures -- the
// `/assets` and `/tarot` mounts and the catch-all. A missing
// web_dist means no shell at all rather than a broken one, and a missing
// tarot directory means no `/tarot` mount.
type staticSite struct {
	webDist   string
	index     string            // path of index.html, or ""
	rootFiles map[string]string // name -> path, listed once from the trusted directory
	mounts    map[string]string // "/assets/" -> directory, "/tarot/" -> directory
}

func newStaticSite(webDist, tarotDir string, log *slog.Logger) (*staticSite, error) {
	s := &staticSite{rootFiles: map[string]string{}, mounts: map[string]string{}}
	if webDist != "" {
		if info, err := os.Stat(webDist); err == nil && info.IsDir() {
			s.webDist = webDist
			entries, err := os.ReadDir(webDist)
			if err != nil {
				return nil, fmt.Errorf("web_dist: %w", err)
			}
			for _, e := range entries {
				if e.Type().IsRegular() {
					s.rootFiles[e.Name()] = filepath.Join(webDist, e.Name())
				}
			}
			if p, ok := s.rootFiles["index.html"]; ok {
				s.index = p
			}
			assets := filepath.Join(webDist, "assets")
			if info, err := os.Stat(assets); err == nil && info.IsDir() {
				s.mounts["/assets/"] = assets
			}
		} else {
			log.Warn("no built frontend; the door will serve no shell", "web_dist", webDist)
		}
	}
	if tarotDir != "" {
		if info, err := os.Stat(tarotDir); err == nil && info.IsDir() {
			s.mounts["/tarot/"] = tarotDir
		} else {
			log.Warn("no tarot art directory; /tarot will not be served", "tarot", tarotDir)
		}
	}
	return s, nil
}

// ServeHTTP is the static half of the routing, in its recorded order: the
// mounts first (matched on the raw path, deliberately -- `dispatch` says
// why the API half normalises and this half does not), then
// the catch-all -- a root file by exact name, or the shell.
func (s *staticSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Path
	for prefix, dir := range s.mounts {
		if strings.HasPrefix(raw, prefix) {
			s.serveMounted(w, r, dir, strings.TrimPrefix(raw, prefix))
			return
		}
	}
	s.serveShell(w, r)
}

// serveMounted is a mount's file server: GET and HEAD only (405 otherwise),
// a missing or non-file path is a JSON 404, and a path that would walk out of
// the directory is the same 404 -- `Rel` is the containment check.
func (s *staticSite) serveMounted(w http.ResponseWriter, r *http.Request, dir, rel string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"detail": "Method Not Allowed"})
		return
	}
	if rel == "" || strings.Contains(rel, "\x00") {
		notFound(w)
		return
	}
	// Clean as a rooted path so `..` cannot climb, then re-root under dir.
	cleaned := path.Clean("/" + rel)
	full := filepath.Join(dir, filepath.FromSlash(cleaned))
	if within, err := filepath.Rel(dir, full); err != nil || within == "." ||
		strings.HasPrefix(within, "..") {
		notFound(w)
		return
	}
	// A path that named a directory (or ended in a slash) is not a file.
	if strings.HasSuffix(rel, "/") {
		notFound(w)
		return
	}
	serveFile(w, r, full)
}

// serveShell is the catch-all: GET only (a HEAD of the shell is a 405 --
// the recorded contract, and the mounts differ on exactly this),
// a bundle root file by *exact* name -- the lookup key is the raw path, the
// served path comes from the trusted listing -- else index.html.
func (s *staticSite) serveShell(w http.ResponseWriter, r *http.Request) {
	if s.index == "" {
		// No frontend built: the plain 404 for a path no route claims.
		notFound(w)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"detail": "Method Not Allowed"})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if target, ok := s.rootFiles[name]; ok {
		serveFile(w, r, target)
		return
	}
	serveFile(w, r, s.index)
}

// serveFile answers one regular file with the door's content type and
// `Cache-Control: no-cache` -- revalidate before reuse, every time, which is
// what a committed bundle with stable filenames needs, and a lesson that
// was paid for once already. `http.ServeContent` supplies Last-Modified,
// the conditional 304 and Range handling.
func serveFile(w http.ResponseWriter, r *http.Request, full string) {
	f, err := os.Open(full)
	if err != nil {
		notFound(w)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		notFound(w)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ContentType(full))
	setDefault(h, "Cache-Control", "no-cache")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// notFound is the static tiers' refusal: `{"detail": "Not Found"}`, the
// recorded shape for the mounts and the bare router alike.
func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"detail": "Not Found"})
}
