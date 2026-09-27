package door

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

// Compression the app owns. Fly's edge compresses on its own, but the app's
// level-9 gzip measured smaller than the edge's (84.5 kB against 119.6 kB
// for the bundle, 2026-08-14), the behaviour is the app's own on any host
// rather than one proxy's undocumented habit, and `mtglab ui` on a laptop
// has no edge at all.
//
// The floor keeps small responses whole: a body under `gzipFloor` bytes is
// written as it was, which spares 304s, tiny JSON and the health checks a
// header that saves nothing. The layer sits innermost — see `Handler` — so
// it reads real responses, and the middleware refusals outside it go out
// uncompressed. A response that already carries a Content-Encoding is left
// alone, and HEAD never compresses: there is no body to shrink and the
// headers must describe what a GET would have sent.

const gzipFloor = 1024

// **A level-9 compressor is the largest object in the request path, so it is
// borrowed rather than built.** Measured on `gzip_bench_test.go` (2026-09-26,
// this Mac): one compressed response allocated **836,848 B in 31 allocations**,
// against 22,704 B in 10 for the same body sent uncompressed. Nearly the whole
// difference is one `flate.Writer` — level 9's window pair and its deflate hash
// tables — built for one response and thrown away. Every asset and every JSON
// body over `gzipFloor` paid it. Pooled: **23,435 B in 14**, which is the
// uncompressed path plus the framing.
//
// The pool's one hazard is the only thing worth being careful about here: a
// writer returned while something still holds it would serve one response's
// bytes inside another's. Three rules keep that impossible, and
// `TestBorrowedCompressorsNeverBleedOneResponseIntoAnother` holds the first two:
// `Reset` re-points the writer at this response's own sink before a byte is
// written, the writer goes back only in [gzipWriter.finish] — after `Close` has
// written the trailer — and the field is cleared in the same breath, so a second
// `finish` cannot hand the same writer back twice.
//
// A handler that panics simply never returns its writer, which is the safe
// direction: one fewer reusable compressor, never a shared one.
var gzipWriters = sync.Pool{New: func() any {
	zw, _ := gzip.NewWriterLevel(nil, gzip.BestCompression)
	return zw
}}

func gzipped(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(gw, r)
		gw.finish()
	})
}

// gzipWriter buffers the first `gzipFloor` bytes to decide, then commits one
// way: plain for a small or excluded response, compressed for the rest.
type gzipWriter struct {
	http.ResponseWriter
	status    int
	buf       []byte
	committed bool // headers sent; buf is owned by the chosen path
	compress  bool
	zw        *gzip.Writer
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.committed {
		return
	}
	g.status = code
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	if g.committed {
		return g.sink(b)
	}
	g.buf = append(g.buf, b...)
	if len(g.buf) >= gzipFloor {
		g.commit()
	}
	return len(b), nil
}

// commit chooses, sends the headers, and flushes the buffer down the chosen
// path. Compression is refused for statuses with no body to speak of and
// for a body somebody else already encoded.
func (g *gzipWriter) commit() {
	g.committed = true
	h := g.Header()
	excluded := g.status == http.StatusNoContent || g.status == http.StatusNotModified ||
		g.status < http.StatusOK || h.Get("Content-Encoding") != ""
	if !excluded && len(g.buf) >= gzipFloor {
		g.compress = true
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		g.ResponseWriter.WriteHeader(g.status)
		zw, _ := gzipWriters.Get().(*gzip.Writer)
		zw.Reset(g.ResponseWriter)
		g.zw = zw
		_, _ = g.zw.Write(g.buf)
		g.buf = nil
		return
	}
	g.ResponseWriter.WriteHeader(g.status)
	if len(g.buf) > 0 {
		_, _ = g.ResponseWriter.Write(g.buf)
	}
	g.buf = nil
}

func (g *gzipWriter) sink(b []byte) (int, error) {
	if g.compress {
		return g.zw.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

// finish commits a response that never reached the floor and closes the
// compressor, which writes the gzip trailer — then hands the compressor back.
// The clear is not tidiness: it is what makes a second call to finish unable to
// return the same writer to the pool twice.
func (g *gzipWriter) finish() {
	if !g.committed {
		g.commit()
	}
	if g.zw != nil {
		_ = g.zw.Close()
		zw := g.zw
		g.zw = nil
		gzipWriters.Put(zw)
	}
}

func (g *gzipWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }
