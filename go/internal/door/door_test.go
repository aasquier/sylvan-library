package door

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/routes"
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

// upstream is a stand-in Python server that echoes what it was asked.
func upstream(t *testing.T) *url.URL {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
		body, _ := io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path": r.URL.Path, "raw": r.URL.RawPath, "query": r.URL.RawQuery,
			"host": r.Host, "method": r.Method, "body": string(body),
			"xff": r.Header.Get("X-Forwarded-For"), "xfh": r.Header.Get("X-Forwarded-Host"),
			"xfp": r.Header.Get("X-Forwarded-Proto"), "cookie": r.Header.Get("Cookie"),
			"accept_encoding": r.Header.Get("Accept-Encoding"),
		})
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u
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
	up := upstream(t)
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: requireAuth, SecureCookies: false,
		WebDist: web, TarotDir: tarot, Upstream: up,
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

// ---------------------------------------------------------------- the table

func TestTheCodeMatchesTheSharedTable(t *testing.T) {
	table, err := routes.Load(routes.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	var code []string
	for p := range PublicPaths {
		code = append(code, p)
	}
	sort.Strings(code)
	file := append([]string(nil), table.Public...)
	sort.Strings(file)
	if strings.Join(code, ",") != strings.Join(file, ",") {
		t.Fatalf("door.PublicPaths %v != routes.json public %v; change both or neither", code, file)
	}
	if AdminPrefix != table.AdminPrefix {
		t.Fatalf("door.AdminPrefix %q != routes.json %q", AdminPrefix, table.AdminPrefix)
	}
}

func TestEveryProtectedRouteRefusesWithoutASession(t *testing.T) {
	table, _ := routes.Load(routes.DefaultPath())
	srv := build(t, true, fakeResolver{})
	for _, route := range table.Protected() {
		resp := get(t, srv, "GET", table.Concrete(route), "")
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
	table, _ := routes.Load(routes.DefaultPath())
	srv := build(t, true, fakeResolver{})
	for _, route := range table.Public {
		resp := get(t, srv, "GET", route, "")
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			t.Errorf("%s is public but the door answered %d", route, resp.StatusCode)
		}
	}
}

func TestAdminRoutesRefuseASignedInNonAdmin(t *testing.T) {
	table, _ := routes.Load(routes.DefaultPath())
	srv := build(t, true, fakeResolver{})
	for route := range table.Admin {
		resp := get(t, srv, "GET", table.Concrete(route), "bob")
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
	// The control: alice reaches it (and the upstream answers).
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
	srv := build(t, true, fakeResolver{fail: true})
	if resp := get(t, srv, "GET", "/api/decks", "alice"); resp.StatusCode != 401 {
		t.Fatalf("a failing lookup let a request through: %d", resp.StatusCode)
	}
	if resp := get(t, srv, "GET", "/api/health", "alice"); resp.StatusCode != 200 {
		t.Fatalf("a failing lookup refused a public route: %d", resp.StatusCode)
	}
}

func TestWithAuthOffNothingIsRefused(t *testing.T) {
	srv := build(t, false, fakeResolver{fail: true})
	for _, p := range []string{"/api/decks", "/api/admin/users", "/api/jobs"} {
		if resp := get(t, srv, "GET", p, ""); resp.StatusCode != 200 {
			t.Errorf("%s answered %d with auth off", p, resp.StatusCode)
		}
	}
}

// ---------------------------------------------------------------- the proxy

func TestTheProxyPassesTheRequestThroughFaithfully(t *testing.T) {
	srv := build(t, true, fakeResolver{})
	req, _ := http.NewRequest("POST", srv.URL+"/api/decks/local/mono-green/swap?dry=1&q=a%20b", strings.NewReader(`{"x":1}`))
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "alice"})
	req.Header.Set("X-Forwarded-For", "1.2.3.4") // a client picking its own bucket
	req.Header.Set("Accept-Encoding", "gzip")
	req.Host = "sylvan-libraries.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var echo map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&echo); err != nil {
		t.Fatal(err)
	}
	if echo["path"] != "/api/decks/local/mono-green/swap" || echo["query"] != "dry=1&q=a%20b" ||
		echo["method"] != "POST" || echo["body"] != `{"x":1}` {
		t.Fatalf("the upstream saw %v", echo)
	}
	if echo["host"] != "sylvan-libraries.com" {
		t.Fatalf("Host was rewritten to %v", echo["host"])
	}
	if xff, _ := echo["xff"].(string); strings.Contains(xff, "1.2.3.4") || xff == "" {
		t.Fatalf("X-Forwarded-For reached the upstream as %q; the client's value must be dropped and the door's peer set", xff)
	}
	if echo["xfp"] != "http" || echo["xfh"] != "sylvan-libraries.com" {
		t.Fatalf("forwarded host/proto were %v / %v", echo["xfh"], echo["xfp"])
	}
	if echo["accept_encoding"] != "gzip" {
		t.Fatalf("the client's Accept-Encoding was changed to %v", echo["accept_encoding"])
	}
	if !strings.Contains(echo["cookie"].(string), "sid=alice") {
		t.Fatalf("the cookie did not reach the upstream: %v", echo["cookie"])
	}
}

func TestADoubledSlashReachesTheUpstreamUntouched(t *testing.T) {
	// Python's router will not match it and answers its JSON 404; the door's
	// job is to refuse it to anonymous callers and otherwise hand it over as
	// it arrived rather than quietly repairing it.
	srv := build(t, true, fakeResolver{})
	resp := get(t, srv, "GET", "/api//decks", "alice")
	var echo map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&echo)
	if echo["path"] != "/api//decks" {
		t.Fatalf("the upstream saw %v", echo["path"])
	}
}

func TestAnUpstreamThatIsDownIsA502InTheEnvelope(t *testing.T) {
	web, tarot := site(t)
	dead, _ := url.Parse("http://127.0.0.1:1") // nothing listens on port 1
	d, err := New(Config{RequireAuth: false, WebDist: web, TarotDir: tarot, Upstream: dead,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	resp := get(t, srv, "GET", "/api/health", "")
	if resp.StatusCode != 502 {
		t.Fatalf("a dead upstream answered %d", resp.StatusCode)
	}
	if d := detail(t, resp); d == "" {
		t.Fatal("the 502 carries no detail for the frontend to show")
	}
	// The door itself is still up and says so, and the shell still loads.
	if resp := get(t, srv, "GET", DoorHealthPath, ""); resp.StatusCode != 200 {
		t.Fatalf("door health answered %d with the upstream down", resp.StatusCode)
	}
	if resp := get(t, srv, "GET", "/", ""); resp.StatusCode != 200 {
		t.Fatalf("the shell answered %d with the upstream down", resp.StatusCode)
	}
}

// --------------------------------------------------------------- the static

func TestTheShellAndItsMountsAnswerAsPythonDoes(t *testing.T) {
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
	// CPython 3.12's built-in table plus api/app.py's three registrations,
	// with Starlette's charset on text/* -- what the deployed container
	// answers for every extension the bundle and the tarot directory hold.
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
	up := upstream(t)
	d, err := New(Config{Upstream: up, WebDist: filepath.Join(t.TempDir(), "absent"),
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
		t.Fatalf("the proxy stopped working without a web_dist: %d", resp.StatusCode)
	}
}

// -------------------------------------------------------------- the headers

func TestEveryDoorResponseCarriesTheSecurityHeaders(t *testing.T) {
	srv := build(t, true, fakeResolver{})
	want := map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
		"Referrer-Policy":    "same-origin",
		"Permissions-Policy": "camera=(self), microphone=(), geolocation=()",
	}
	for _, c := range []struct{ method, path, token string }{
		{"GET", "/api/decks", ""}, {"GET", "/api/admin/users", "bob"}, {"GET", "/", ""},
		{"GET", "/tarot/00-fool.webp", ""}, {"GET", "/assets/nope.js", ""},
		{"GET", DoorHealthPath, ""}, {"GET", "/api/health", ""}, {"POST", "/", ""},
	} {
		resp := get(t, srv, c.method, c.path, c.token)
		for name, value := range want {
			// Values, not Get: a proxied response that carried Python's copy
			// *and* the door's would read `nosniff` from Get and be wrong on
			// the wire -- the contract suite's httpx joins them and saw
			// `nosniff, nosniff` on the first run through the door.
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
	up := upstream(t)
	web, tarot := site(t)
	d, err := New(Config{SecureCookies: true, WebDist: web, TarotDir: tarot, Upstream: up,
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

// ----------------------------------------------------------------- the flag

func TestFlagReadsLikeConfigFlag(t *testing.T) {
	for raw, want := range map[string]bool{"1": true, "true": true, " YES ": true, "On": true,
		"0": false, "false": false, "no": false, "maybe": false} {
		t.Setenv("MTGLAB_TEST_FLAG", raw)
		if got := Flag("MTGLAB_TEST_FLAG", true); got != want {
			t.Errorf("Flag(%q) = %v, want %v", raw, got, want)
		}
	}
	t.Setenv("MTGLAB_TEST_FLAG", "")
	if !Flag("MTGLAB_TEST_FLAG", true) || Flag("MTGLAB_TEST_FLAG", false) {
		t.Fatal("a blank flag did not fall back to its default")
	}
	t.Setenv("MTGLAB_DATA_DIR", "")
	if got := AppDBPath(); got != filepath.Join("data", "app.db") {
		t.Fatalf("AppDBPath() = %q", got)
	}
	t.Setenv("MTGLAB_DATA_DIR", "/data")
	if got := AppDBPath(); got != filepath.Join("/data", "app.db") {
		t.Fatalf("AppDBPath() = %q", got)
	}
}

// --------------------------------------------------- the real resolver path

func TestTheRealResolverReadsAppDB(t *testing.T) {
	// An app.db in Python's shape (see internal/auth's tests for the DDL and
	// the vectors): alice's session admits her; a missing file is anonymous.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	writeFixtureDB(t, dbPath)
	up := upstream(t)
	web, tarot := site(t)
	d, err := New(Config{RequireAuth: true, AppDB: dbPath, WebDist: web, TarotDir: tarot,
		Upstream: up, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
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
}
