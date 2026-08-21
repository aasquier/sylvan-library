package door

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// newProxy is the loopback hop to the Python server for everything not yet
// ported. Three decisions, each the faithful-passthrough one:
//
//   - The inbound `Host` is preserved, as `NewSingleHostReverseProxy` would,
//     so Python sees what the client sent -- the same as it does today.
//   - The inbound `X-Forwarded-*` and `Forwarded` headers are *dropped* and
//     the door's own set (`SetXForwarded`): For = the door's peer, Host, Proto.
//     uvicorn trusts forwarded headers from loopback by default, and the door
//     is now on loopback, so a client-supplied `X-Forwarded-For` would
//     otherwise become `request.client.host` in Python -- a rate-limit bucket
//     a client picks for itself. With the client's header dropped, Python's
//     idea of the client is the door's peer, which is exactly what it was
//     before the door existed. (Deployed, the real address rides in
//     `Fly-Client-IP`, which passes through untouched; `fly.toml` says why.)
//   - The transport neither adds `Accept-Encoding` nor decompresses, so a
//     client that asked for gzip gets Python's gzip and one that did not gets
//     bytes, unchanged.
//
// An upstream that is not answering is a 502 in FastAPI's envelope, so the
// frontend's `detail` reader has a sentence to show rather than a blank.
func newProxy(upstream *url.URL, log *slog.Logger) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.Host = pr.In.Host
			// Keep the query string byte-for-byte; the door interprets no
			// parameter, so there is no interpretation to disagree about.
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			pr.SetXForwarded()
		},
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   64,
			IdleConnTimeout:       90 * time.Second,
			DisableCompression:    true,
			ExpectContinueTimeout: time.Second,
			// No ResponseHeaderTimeout: the longest synchronous route today is
			// well under a second, but the plan's lesson about sibling
			// durations says not to pick a ceiling nobody measured.
		},
		// Flush as bytes arrive, so a streamed body is not held back.
		FlushInterval: 100 * time.Millisecond,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("the upstream did not answer", "path", r.URL.Path, "error", err)
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"detail": "the library is not answering right now -- try again in a moment"})
		},
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	return proxy
}
