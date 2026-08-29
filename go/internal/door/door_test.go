package door

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// fakeResolver is the two-account arrangement every adversarial test in this
// repository uses: alice administers, bob does not.
type fakeResolver struct{ fail bool }

func (f fakeResolver) Resolve(_ context.Context, token string) (auth.Scope, error) {
	if f.fail {
		return auth.Anonymous, errors.New("app.db is on fire")
	}
	switch token {
	case "alice":
		return auth.Scope{UserID: 1, Username: "alice", IsAdmin: true, Authenticated: true}, nil
	case "bob":
		return auth.Scope{UserID: 2, Username: "bob", Authenticated: true}, nil
	}
	return auth.Anonymous, nil
}

// site writes a web_dist and a tarot directory like the real ones.
func site(t *testing.T) (string, string) {
	t.Helper()
	web := filepath.Join(t.TempDir(), "web_dist")
	assets := filepath.Join(web, "assets")
	tarot := filepath.Join(t.TempDir(), "tarot")
	for _, dir := range []string{assets, tarot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(web, "index.html"):       "<!doctype html><html><body>shell</body></html>",
		filepath.Join(web, "robots.txt"):       "User-agent: *\n",
		filepath.Join(assets, "app.js"):        "console.log('app')",
		filepath.Join(assets, "big.js"):        strings.Repeat("x", 4096),
		filepath.Join(assets, "index.css"):     "body{}",
		filepath.Join(assets, "font.woff2"):    "wOF2",
		filepath.Join(assets, "loop.webm"):     "webm",
		filepath.Join(tarot, "00-fool.webp"):   "RIFF",
		filepath.Join(tarot, "PROVENANCE.md"):  "# Where",
		filepath.Join(tarot, "secret.notakey"): "?",
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return web, tarot
}

func build(t *testing.T, requireAuth bool, resolver Resolver) *httptest.Server {
	t.Helper()
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: requireAuth, SecureCookies: false,
		WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if resolver != nil {
		d.resolver = resolver
	}
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, method, path, token string, headers ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func detail(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	s, _ := body["detail"].(string)
	return s
}

// ---------------------------------------------------------------- the sweeps

// The sweeps' inputs come from the code itself: every pattern the API
// serves, the public list beside it, and placeholder values for templated
// segments. The refusals under test all happen before any handler runs, so
// the values need not resolve to anything real.
var placeholders = map[string]string{
	"{slug}": "arahbo-cats", "{owner}": "local", "{name}": "Forest",
	"{key}": "mulligan", "{job_id}": "deadbeef", "{username}": "alice",
	"{oracle_id}": "0aae2e33-0000-4000-8000-000000000000",
	"{effect}":    "depth-drift", "{filename}": "loop.webm", "{code}": "W",
	// The crypt's opaque handle. Sixteen hex characters is what one looks
	// like, and it names nothing here on purpose: these sweeps ask what the
	// door does before a handler ever sees the request.
	"{id}": "0123456789abcdef",
}

func concrete(t *testing.T, pattern string) string {
	t.Helper()
	for placeholder, value := range placeholders {
		pattern = strings.ReplaceAll(pattern, placeholder, value)
	}
	if strings.ContainsAny(pattern, "{}") {
		t.Fatalf("pattern %s carries a placeholder this table does not fill", pattern)
	}
	return pattern
}

// servedPaths is every path pattern the API serves, deduplicated across
// methods, sorted.
func servedPaths(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, r := range api.New(api.Config{}).Routes() {
		seen[r.Pattern] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Deny by default, derived rather than declared: a new route is protected
// unless somebody puts it on PublicPaths, and an entry there that nothing
// serves is a typo with a hole's shape.
func TestEveryPublicPathIsServed(t *testing.T) {
	t.Parallel()
	served := map[string]bool{}
	for _, p := range servedPaths(t) {
		served[p] = true
	}
	for p := range PublicPaths {
		if !served[p] {
			t.Errorf("PublicPaths names %s, which no route serves", p)
		}
	}
}

func TestEveryProtectedRouteRefusesWithoutASession(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for _, route := range servedPaths(t) {
		if PublicPaths[route] {
			continue
		}
		resp := get(t, srv, "GET", concrete(t, route), "")
		if resp.StatusCode != 401 {
			t.Errorf("%s answered %d with no session, want 401", route, resp.StatusCode)
			continue
		}
		if d := detail(t, resp); d != noSession {
			t.Errorf("%s refused with %q", route, d)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: 401 is %q, not JSON", route, ct)
		}
	}
}

func TestPublicRoutesAreNotRefused(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for route := range PublicPaths {
		resp := get(t, srv, "GET", route, "")
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			t.Errorf("%s is public but the middleware answered %d", route, resp.StatusCode)
		}
	}
}

func TestAdminRoutesRefuseASignedInNonAdmin(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for _, route := range servedPaths(t) {
		if route != AdminPrefix && !strings.HasPrefix(route, AdminPrefix+"/") {
			continue
		}
		resp := get(t, srv, "GET", concrete(t, route), "bob")
		if resp.StatusCode != 403 {
			t.Errorf("%s answered %d to bob, want 403", route, resp.StatusCode)
			continue
		}
		if d := detail(t, resp); d != adminOnly {
			t.Errorf("%s refused bob with %q", route, d)
		}
	}
	// And the prefix covers paths no route claims, so a non-admin cannot
	// learn which admin routes exist by which ones answer differently.
	if resp := get(t, srv, "GET", "/api/admin/nothing-here", "bob"); resp.StatusCode != 403 {
		t.Errorf("/api/admin/nothing-here answered %d to bob", resp.StatusCode)
	}
	// The control: alice reaches it.
	if resp := get(t, srv, "GET", "/api/admin/users", "alice"); resp.StatusCode != 200 {
		t.Errorf("alice was refused the admin prefix: %d", resp.StatusCode)
	}
	// Anonymous is told it needs a session, not that it needs to be somebody
	// else: the 401 comes before the 403.
	if resp := get(t, srv, "GET", "/api/admin/users", ""); resp.StatusCode != 401 {
		t.Errorf("anonymous at the admin prefix answered %d, want 401", resp.StatusCode)
	}
}

func TestDottedAndDoubledPathsDoNotSlipPast(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for _, p := range []string{"/api/./admin/users", "/api//admin/users", "/api/x/../admin/users"} {
		if resp := get(t, srv, "GET", p, "bob"); resp.StatusCode != 403 {
			t.Errorf("%s answered %d to bob, want 403", p, resp.StatusCode)
		}
		if resp := get(t, srv, "GET", p, ""); resp.StatusCode != 401 {
			t.Errorf("%s answered %d anonymously, want 401", p, resp.StatusCode)
		}
	}
	for _, p := range []string{"/api", "/api/", "/apix", "/api/../api/decks", "/api/decks/"} {
		if resp := get(t, srv, "GET", p, ""); resp.StatusCode != 401 {
			t.Errorf("%s answered %d anonymously, want 401", p, resp.StatusCode)
		}
	}
}

func TestNormalisePathMatchesPosixpath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"/api/decks": "/api/decks", "/api/decks/": "/api/decks", "//api/decks": "/api/decks",
		"/api/./decks": "/api/decks", "/api/x/../decks": "/api/decks", "/": "/", "": "/",
		"api/decks": "/api/decks", "/api/../..": "/", "/a//b///c": "/a/b/c",
	}
	for in, want := range cases {
		if got := NormalisePath(in); got != want {
			t.Errorf("NormalisePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestALookupFailureIsAnonymousNotAPass(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{fail: true})
	if resp := get(t, srv, "GET", "/api/decks", "alice"); resp.StatusCode != 401 {
		t.Fatalf("a failing lookup let a request through: %d", resp.StatusCode)
	}
	if resp := get(t, srv, "GET", "/api/health", "alice"); resp.StatusCode != 200 {
		t.Fatalf("a failing lookup refused a public route: %d", resp.StatusCode)
	}
}

func TestWithAuthOffNothingIsRefused(t *testing.T) {
	t.Parallel()
	srv := build(t, false, fakeResolver{fail: true})
	for _, p := range []string{"/api/decks", "/api/admin/users", "/api/jobs"} {
		if resp := get(t, srv, "GET", p, ""); resp.StatusCode != 200 {
			t.Errorf("%s answered %d with auth off", p, resp.StatusCode)
		}
	}
}

// ---------------------------------------------------------------- the proxy

// The router's own refusals, in the shapes the app has always answered: a
// path no route claims is the catch-all's 404 -- `no such endpoint`, with
// the normalised path -- whatever non-canonical spelling it arrived in, and
// a matched path on the wrong method is the router's 405, `Allow` carrying
// the matching route's method. HEAD is deliberately among the 405s: GET
// routes here have never answered HEAD, and a router that quietly grew it
// would be more helpful than its own contract.
func TestAStrayAPIRequestIsTheCatchAllsRefusal(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for _, c := range []struct{ path, normalised string }{
		{"/api/nonexistent", "/api/nonexistent"},
		{"//api/glossary", "/api/glossary"},
		{"/api/glossary/", "/api/glossary"},
		{"/api/./glossary", "/api/glossary"},
		{"/api/x/../glossary", "/api/glossary"},
		{"/api/glossary%2F", "/api/glossary"},
	} {
		resp := get(t, srv, "GET", c.path, "alice")
		if resp.StatusCode != 404 {
			t.Errorf("GET %s answered %d, want 404", c.path, resp.StatusCode)
			continue
		}
		if d := detail(t, resp); d != "no such endpoint: "+c.normalised {
			t.Errorf("GET %s: detail %q", c.path, d)
		}
	}
}

func TestAWrongMethodIsTheRoutersOwn405(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for _, method := range []string{"POST", "DELETE", "HEAD", "PUT"} {
		resp := get(t, srv, method, "/api/glossary", "alice")
		if resp.StatusCode != 405 {
			t.Errorf("%s /api/glossary answered %d, want 405", method, resp.StatusCode)
			continue
		}
		if allow := resp.Header.Get("Allow"); allow != "GET" {
			t.Errorf("%s /api/glossary: Allow %q, want GET", method, allow)
		}
		if method != "HEAD" {
			if d := detail(t, resp); d != "Method Not Allowed" {
				t.Errorf("%s /api/glossary: detail %q", method, d)
			}
		}
	}
}

// Compression, at the layer the app owns: a body over the floor is gzipped
// when the client asks, a small one is left whole, and a revalidation's 304
// stays bare. The floor reads the real response because the layer sits
// innermost -- the same placement the app has always had.
func TestResponsesOverTheFloorAreCompressed(t *testing.T) {
	t.Parallel()
	srv := build(t, false, nil)
	req, _ := http.NewRequest("GET", srv.URL+"/assets/big.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("a %d-byte asset went out with Content-Encoding %q", 4096, enc)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 4096 || body[0] != 'x' {
		t.Fatalf("the compressed asset decoded to %d bytes", len(body))
	}

	// Small stays whole: the health body is a handful of bytes.
	small, _ := http.NewRequest("GET", srv.URL+"/api/health", nil)
	small.Header.Set("Accept-Encoding", "gzip")
	sresp, err := http.DefaultClient.Do(small)
	if err != nil {
		t.Fatal(err)
	}
	defer sresp.Body.Close()
	if enc := sresp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("a tiny body went out with Content-Encoding %q", enc)
	}
}

// --------------------------------------------------------------- the static

func TestTheShellAndItsMountsAnswerTheRecordedShapes(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	cases := []struct {
		method, path string
		status       int
		contentType  string
		cache        string
		detail       string
	}{
		{"GET", "/", 200, "text/html; charset=utf-8", "no-cache", ""},
		{"GET", "/decks/local/mono-green", 200, "text/html; charset=utf-8", "no-cache", ""},
		{"GET", "/index.html", 200, "text/html; charset=utf-8", "no-cache", ""},
		{"GET", "/robots.txt", 200, "text/plain; charset=utf-8", "no-cache", ""},
		{"GET", "/favicon.ico", 200, "text/html; charset=utf-8", "no-cache", ""}, // no such root file: the shell
		{"GET", "/assets", 200, "text/html; charset=utf-8", "no-cache", ""},      // no trailing slash: the shell
		{"GET", "/tarot", 200, "text/html; charset=utf-8", "no-cache", ""},
		{"GET", DoorHealthPath, 200, "application/json", "", ""},
		{"HEAD", "/", 405, "application/json", "", ""},
		{"POST", "/", 405, "application/json", "", "Method Not Allowed"},
		{"GET", "/assets/app.js", 200, "text/javascript; charset=utf-8", "no-cache", ""},
		{"HEAD", "/assets/app.js", 200, "text/javascript; charset=utf-8", "no-cache", ""},
		{"GET", "/assets/index.css", 200, "text/css; charset=utf-8", "no-cache", ""},
		{"GET", "/assets/font.woff2", 200, "font/woff2", "no-cache", ""},
		{"GET", "/assets/loop.webm", 200, "video/webm", "no-cache", ""},
		{"GET", "/tarot/00-fool.webp", 200, "image/webp", "no-cache", ""},
		{"GET", "/tarot/PROVENANCE.md", 200, "text/markdown; charset=utf-8", "no-cache", ""},
		{"GET", "/tarot/secret.notakey", 200, "text/plain; charset=utf-8", "no-cache", ""},
		{"POST", "/assets/app.js", 405, "application/json", "", "Method Not Allowed"},
		{"GET", "/assets/nope.js", 404, "application/json", "", "Not Found"},
		{"GET", "/assets/", 404, "application/json", "", "Not Found"},
		{"GET", "/tarot/", 404, "application/json", "", "Not Found"},
		{"GET", "/assets/app.js/", 404, "application/json", "", "Not Found"},
		{"GET", "/assets/../index.html", 404, "application/json", "", "Not Found"},
		{"GET", "/assets/%2e%2e/index.html", 404, "application/json", "", "Not Found"},
		{"GET", "/tarot/../../etc/passwd", 404, "application/json", "", "Not Found"},
	}
	for _, c := range cases {
		resp := get(t, srv, c.method, c.path, "")
		if resp.StatusCode != c.status {
			t.Errorf("%s %s: %d, want %d", c.method, c.path, resp.StatusCode, c.status)
			continue
		}
		if ct := resp.Header.Get("Content-Type"); ct != c.contentType {
			t.Errorf("%s %s: content-type %q, want %q", c.method, c.path, ct, c.contentType)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != c.cache {
			t.Errorf("%s %s: cache-control %q, want %q", c.method, c.path, cc, c.cache)
		}
		if c.detail != "" {
			if d := detail(t, resp); d != c.detail {
				t.Errorf("%s %s: detail %q, want %q", c.method, c.path, d, c.detail)
			}
		}
	}
}

func TestAnAssetRevalidatesWithA304(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	first := get(t, srv, "GET", "/assets/app.js", "")
	lm := first.Header.Get("Last-Modified")
	if lm == "" {
		t.Fatal("no Last-Modified on an asset; the browser has nothing to revalidate with")
	}
	again := get(t, srv, "GET", "/assets/app.js", "", "If-Modified-Since", lm)
	if again.StatusCode != 304 {
		t.Fatalf("a conditional request answered %d, want 304", again.StatusCode)
	}
}

func TestContentTypesMatchTheContainer(t *testing.T) {
	t.Parallel()
	// The deployed container's recorded serving table, charset included on
	// every text/* -- what the wire has always answered for every extension
	// the bundle and the tarot directory hold.
	want := map[string]string{
		"x.css": "text/css; charset=utf-8", "x.html": "text/html; charset=utf-8",
		"x.ico": "image/vnd.microsoft.icon", "x.js": "text/javascript; charset=utf-8",
		"x.json": "application/json", "x.md": "text/markdown; charset=utf-8",
		"x.mjs": "text/javascript; charset=utf-8", "x.mp4": "video/mp4",
		"x.svg": "image/svg+xml", "x.txt": "text/plain; charset=utf-8",
		"x.wasm": "application/wasm", "x.webm": "video/webm", "x.webp": "image/webp",
		"x.woff2": "font/woff2", "x.unknown": "text/plain; charset=utf-8",
		"X.WEBP": "image/webp",
	}
	for name, typ := range want {
		if got := ContentType(name); got != typ {
			t.Errorf("%s -> %q, want %q", name, got, typ)
		}
	}
}

func TestNoFrontendMeansNoShell(t *testing.T) {
	t.Parallel()
	d, err := New(Config{WebDist: filepath.Join(t.TempDir(), "absent"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	if resp := get(t, srv, "GET", "/", ""); resp.StatusCode != 404 {
		t.Fatalf("with no web_dist the shell answered %d", resp.StatusCode)
	}
	if resp := get(t, srv, "GET", "/api/health", ""); resp.StatusCode != 200 {
		t.Fatalf("the API stopped answering without a web_dist: %d", resp.StatusCode)
	}
}

// -------------------------------------------------------------- the headers

// "Every" is derived, not sampled -- and the distinction is load-bearing
// rather than tidy. Three dismissed high-severity `go/reflected-xss` alerts
// (door.go's 404 echo and the gzip layer's two writes) rest on one sentence:
// every body this door writes is JSON or a disk asset, and `nosniff` reaches
// all of them. A hand-written list of eight paths cannot hold that sentence,
// because the route it does not name is exactly the one that breaks it. So
// the API half comes from `servedPaths` -- the served route table -- and the
// literals below are only the paths no route table names: the SPA shell, the
// two static tiers and the door's own liveness answer.
//
// Anonymous, deliberately: a protected route is refused before its handler
// runs and a public one answers for real, so the sweep costs nothing and
// still crosses the middleware seam where the headers are stamped. The one
// signed-in case is the admin prefix, whose 403 is written by a different
// arm of the same middleware.
func TestEveryDoorResponseCarriesTheSecurityHeaders(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	want := map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
		"Referrer-Policy":    "same-origin",
		"Permissions-Policy": "camera=(self), microphone=(), geolocation=()",
	}
	cases := []struct{ method, path, token string }{
		{"GET", "/", ""}, {"POST", "/", ""},
		{"GET", "/tarot/00-fool.webp", ""}, {"GET", "/assets/nope.js", ""},
		{"GET", DoorHealthPath, ""},
		{"GET", "/api/admin/users", "bob"},
	}
	for _, route := range servedPaths(t) {
		cases = append(cases, struct{ method, path, token string }{
			"GET", concrete(t, route), ""})
	}
	for _, c := range cases {
		resp := get(t, srv, c.method, c.path, c.token)
		for name, value := range want {
			// Values, not Get: a response carrying the header twice would
			// read `nosniff` from Get and be wrong on
			// the wire -- a client that joins repeated headers saw
			// `nosniff, nosniff` on the door's first run.
			if got := resp.Header.Values(name); len(got) != 1 || got[0] != value {
				t.Errorf("%s %s as %q: %s = %q, want exactly [%q]", c.method, c.path, c.token, name, got, value)
			}
		}
		if resp.Header.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s %s: HSTS sent without secure cookies", c.method, c.path)
		}
	}
}

func TestHSTSRidesOnlyWhenTLSFrontsTheApp(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	d, err := New(Config{SecureCookies: true, WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	if got := get(t, srv, "GET", "/", "").Header.Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("HSTS = %q", got)
	}
}

// --------------------------------------------------- the real resolver path

func TestTheRealResolverReadsAppDB(t *testing.T) {
	t.Parallel()
	// A real app.db (see internal/auth's tests for the DDL and
	// the vectors): alice's session admits her; a missing file is anonymous.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	writeFixtureDB(t, dbPath)
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: true, AppDB: dbPath, WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	if resp := get(t, srv, "GET", "/api/decks", "alice-token"); resp.StatusCode != 200 {
		t.Fatalf("alice's session answered %d", resp.StatusCode)
	}
	if resp := get(t, srv, "GET", "/api/admin/users", "bob-token"); resp.StatusCode != 403 {
		t.Fatalf("bob at the admin prefix answered %d", resp.StatusCode)
	}
	if resp := get(t, srv, "GET", "/api/decks", "nobody"); resp.StatusCode != 401 {
		t.Fatalf("an unknown token answered %d", resp.StatusCode)
	}

	// The serving resolver pays the sessions table what a live server owes
	// it: alice's request above touched `last_seen_at` (the fixture writes
	// none), and an expired row is deleted by the request it refuses.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var seen sql.NullString
	if err := db.QueryRow("SELECT last_seen_at FROM sessions WHERE token_hash = ?",
		auth.HashToken("alice-token")).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if !seen.Valid || seen.String == "" {
		t.Error("alice's session was resolved without touching last_seen_at")
	}
	stale := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000000") + "+00:00"
	if _, err := db.Exec("INSERT INTO sessions (token_hash, user_id, created_at, expires_at)"+
		" VALUES (?, 2, ?, ?)", auth.HashToken("stale-token"), stale, stale); err != nil {
		t.Fatal(err)
	}
	if resp := get(t, srv, "GET", "/api/decks", "stale-token"); resp.StatusCode != 401 {
		t.Fatalf("an expired token answered %d", resp.StatusCode)
	}
	var left int
	if err := db.QueryRow("SELECT count(*) FROM sessions WHERE token_hash = ?",
		auth.HashToken("stale-token")).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Error("the refused request left the expired session row in place")
	}
}

// -------------------------------------------------------- the API routes

func TestARouteAnswersTheEmbeddedPayload(t *testing.T) {
	t.Parallel()
	srv := build(t, true, fakeResolver{})
	for path, want := range map[string][]byte{
		"/api/colors":   reference.ColorsJSON(),
		"/api/glossary": reference.GlossaryJSON(),
		"/api/themes":   reference.ThemesJSON(),
	} {
		resp := get(t, srv, "GET", path, "alice")
		if resp.StatusCode != 200 {
			t.Fatalf("%s answered %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("%s: content-type %q", path, ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != string(want) {
			t.Fatalf("%s: the door did not answer the embedded payload (got %d bytes, want %d)", path, len(body), len(want))
		}
		// The answer carries the hardening headers.
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: no security headers on a door-served route", path)
		}
		// And it is still behind the middleware: anonymous is refused.
		if anon := get(t, srv, "GET", path, ""); anon.StatusCode != 401 {
			t.Fatalf("%s answered %d to anonymous", path, anon.StatusCode)
		}
	}
}

func TestTheRouteTableChoosesTheMostSpecificPattern(t *testing.T) {
	t.Parallel()
	var hit string
	named := func(name string) http.HandlerFunc {
		return func(http.ResponseWriter, *http.Request) { hit = name }
	}
	// Listed template first on purpose: order must not decide.
	table, err := newRouteTable([]api.Route{
		{Method: "GET", Pattern: "/api/colors/{key}", Handler: named("template")},
		{Method: "GET", Pattern: "/api/colors/progress", Handler: named("literal")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/api/colors/progress": "literal", "/api/colors/G": "template"} {
		req := httptest.NewRequest("GET", path, nil)
		h, _, ok := table.match(req)
		if !ok {
			t.Fatalf("%s: no match", path)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if hit != want {
			t.Errorf("%s landed on %s, want %s", path, hit, want)
		}
	}
	// The same shape twice: two routes nothing could choose between.
	h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if _, err := newRouteTable([]api.Route{
		{Method: "GET", Pattern: "/api/decks/{owner}/{slug}", Handler: h},
		{Method: "GET", Pattern: "/api/decks/{a}/{b}", Handler: h},
	}); err == nil {
		t.Fatal("two templates of one shape were accepted")
	}
	for _, bad := range []api.Route{
		{Method: "GET", Pattern: "/not/api", Handler: h},
		{Method: "GET", Pattern: "/api/x/", Handler: h},
		{Method: "GET", Pattern: "/api/{}", Handler: h},
		{Method: "GET", Pattern: "/api/{half", Handler: h},
		{Method: "GET", Pattern: "/api/ok"},
	} {
		if _, err := newRouteTable([]api.Route{bad}); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	// Different methods on one shape are fine -- one path, several verbs.
	if _, err := newRouteTable([]api.Route{
		{Method: "GET", Pattern: "/api/decks/{owner}/{slug}", Handler: h},
		{Method: "PATCH", Pattern: "/api/decks/{owner}/{slug}", Handler: h},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPathValuesReachTheHandler(t *testing.T) {
	t.Parallel()
	var got string
	table, err := newRouteTable([]api.Route{{Method: "GET", Pattern: "/api/decks/{owner}/{slug}/validate",
		Handler: func(_ http.ResponseWriter, r *http.Request) {
			got = r.PathValue("owner") + "/" + r.PathValue("slug")
		}}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/decks/local/mono-green/validate", nil)
	h, _, ok := table.match(req)
	if !ok {
		t.Fatal("no match")
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "local/mono-green" {
		t.Fatalf("path values %q", got)
	}
	for _, miss := range []string{"/api/decks/local/mono-green", "/api/decks/local/mono-green/validate/",
		"/api/decks/local//validate", "/api/decks/local/mono-green/stats"} {
		if _, _, ok := table.match(httptest.NewRequest("GET", miss, nil)); ok {
			t.Errorf("%s matched", miss)
		}
	}
	// A parameter with a literal suffix: `/api/symbols/{code}.svg`.
	var code string
	suffixed, err := newRouteTable([]api.Route{{Method: "GET", Pattern: "/api/symbols/{code}.svg",
		Handler: func(_ http.ResponseWriter, r *http.Request) { code = r.PathValue("code") }}})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("GET", "/api/symbols/W.svg", nil)
	if h, _, ok := suffixed.match(req); !ok {
		t.Fatal("W.svg did not match")
	} else {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if code != "W" {
		t.Fatalf("code %q", code)
	}
	for _, miss := range []string{"/api/symbols/W", "/api/symbols/.svg", "/api/symbols/W.png", "/api/symbols/W.svg/x"} {
		if _, _, ok := suffixed.match(httptest.NewRequest("GET", miss, nil)); ok {
			t.Errorf("%s matched the suffixed parameter", miss)
		}
	}
	if _, err := newRouteTable([]api.Route{{Method: "GET", Pattern: "/api/x/{a}{b}",
		Handler: func(http.ResponseWriter, *http.Request) {}}}); err == nil {
		t.Error("two parameters in one segment were accepted")
	}
}

// The generic job routes answer from this process's registry: an empty
// registry is an empty list -- `[]`, never `null` -- and an id nobody holds
// is a 404 whose detail the frontend renders.
func TestTheJobListIsTheRegistrys(t *testing.T) {
	t.Parallel()
	srv := build(t, false, fakeResolver{})
	resp := get(t, srv, "GET", "/api/jobs", "")
	if resp.StatusCode != 200 {
		t.Fatalf("the job list answered %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "[]" {
		t.Fatalf("an empty registry listed %q", body)
	}
}

func TestAJobIdNobodyHoldsIsA404(t *testing.T) {
	t.Parallel()
	srv := build(t, false, fakeResolver{})
	resp := get(t, srv, "GET", "/api/jobs/some-old-id", "")
	if resp.StatusCode != 404 {
		t.Fatalf("an unknown job id answered %d", resp.StatusCode)
	}
	if d := detail(t, resp); d != "no such job" {
		t.Fatalf("the 404 carries %q", d)
	}
}

// The visitor ledger, end to end: every request lands in `request_log`
// exactly once, under the TEMPLATE (never the concrete path). Refusals that
// never reached routing -- the middleware's and the router's 405 -- share
// `(unrouted)`, the static tiers record their mount prefix, and the shell
// and the stray-API 404s record the catch-all's `/{full_path}`.
func TestTheDoorCountsWhatItAnswers(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(dbPath); err != nil {
		t.Fatal(err)
	}
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: true, WebDist: web, TarotDir: tarot,
		AppDB:  dbPath,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	d.resolver = fakeResolver{}
	srv := httptest.NewServer(d.Handler())

	get(t, srv, "GET", "/api/glossary", "alice")    // a route, by its pattern
	get(t, srv, "GET", "/api/health", "")           // public: the template, anonymously
	get(t, srv, "GET", "/api/decks", "")            // refused before routing
	get(t, srv, "PUT", "/api/glossary", "alice")    // the router's 405
	get(t, srv, "GET", "/assets/app.js", "")        // a mount
	get(t, srv, "GET", "/tarot/00-fool.webp", "")   // the other mount
	get(t, srv, "GET", "/", "")                     // the shell
	get(t, srv, "GET", "/api/nonexistent", "alice") // the catch-all's 404
	get(t, srv, "GET", "//api/glossary", "alice")   // non-canonical: the same 404

	// Close BEFORE the flush: a count is recorded by the handler goroutine
	// after the client already has its response, and Close is the barrier
	// that waits those goroutines out. Flushing first is a born-flaky read
	// of a buffer someone may still be writing -- arm64's race scheduler
	// found it on the first try.
	srv.Close()
	d.traffic.Flush()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	counts := map[string]int{}
	rows, err := db.Query("SELECT route, sum(count) FROM request_log GROUP BY route")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var route string
		var n int
		if err := rows.Scan(&route, &n); err != nil {
			t.Fatal(err)
		}
		counts[route] = n
	}
	want := map[string]int{
		"/api/glossary": 1, "/api/health": 1, "(unrouted)": 2, "/assets": 1,
		"/tarot": 1, "/{full_path}": 3,
	}
	for route, n := range want {
		if counts[route] != n {
			t.Errorf("%s counted %d, want %d (all: %v)", route, counts[route], n, counts)
		}
	}
	if len(counts) != len(want) {
		t.Errorf("extra templates recorded: %v (every request lands exactly once)", counts)
	}
}
