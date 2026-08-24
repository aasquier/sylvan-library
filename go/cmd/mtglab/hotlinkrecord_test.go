package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// **"The served app hotlinks nothing it could serve itself"** is one of this
// project's oldest standing claims, it is re-verified by hand every quarter,
// and until now it was enforced by nothing at all.
//
// The claim has two halves and they fail differently. The *licensing* half --
// Wizards' card art is hotlinked from `cards.scryfall.io` because the licence
// forbids rehosting it -- is argued in `NOTICE.md` and checked there. The half
// below is the efficiency and privacy one: **a CDN is a hotlink with better
// marketing**, it puts a third party between a reader and the page, it leaks
// every visitor's address to that third party, and it is one `npm install`
// away at all times. A dependency that pulls a webfont from Google, a
// component library that reaches for a stylesheet, a worker that fetches its
// own WebAssembly -- each of those ships silently, works perfectly, and is
// invisible to a source grep because it only appears in the *built* bundle.
//
// So this reads `web_dist`, which is the half a source grep cannot see, and it
// asks three questions of it. Each is about a *shape that fetches* rather than
// about a string that merely appears: a minified bundle is full of inert
// absolute URLs (documentation links, an SVG namespace, a funding page in
// vendored package metadata) and a test that failed on those would be deleted
// within a month.
//
// The 2026-08-24 measurement this was written from: one runtime external host
// (`cards.scryfall.io`), four self-hosted `.woff2` faces, no `@import`, and no
// absolute `url()` in any stylesheet.

// bundleFiles is every file the door would serve out of the built bundle,
// discovered by walking rather than listed, so a new chunk joins the check by
// existing.
func bundleFiles(t *testing.T) (root string, files []string) {
	t.Helper()
	root = repoRoot(t)
	dist := filepath.Join(root, "web_dist")
	if _, err := os.Stat(dist); err != nil {
		t.Skipf("no built bundle at web_dist: %v", err)
	}
	err := filepath.WalkDir(dist, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking web_dist: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("web_dist is empty; this guard would pass by having nothing to read")
	}
	return root, files
}

// The hosts that exist to serve somebody else's code, fonts or styles. Never
// this project's: everything it runs, it serves.
var cdnHosts = []string{
	"fonts.googleapis.com", "fonts.gstatic.com", "ajax.googleapis.com",
	"cdn.jsdelivr.net", "unpkg.com", "cdnjs.cloudflare.com", "esm.sh",
	"cdn.skypack.dev", "code.jquery.com", "stackpath.bootstrapcdn.com",
	"use.typekit.net", "kit.fontawesome.com",
}

// The one recorded appearance, and why it is allowed: `cdn.jsdelivr.net` is
// tesseract.js's *default* `workerPath`, and `lib/reader.ts` overrides it
// along with `corePath` and `langPath` so every OCR byte comes from
// `/api/ocr`, pinned by SHA-256 on the way in. The allowance is tied to those
// override paths being *in the bundle* rather than to a filename or to a
// chunk: the bundler splits the vendored library away from the call site that
// tames it (today the default is in `reader.js` and the overrides are in
// `Import.js`), so a same-file rule would fail on a build detail rather than
// on a hotlink. What must never happen is the default surviving with no
// override anywhere -- that fetches unpinned WebAssembly from a third party,
// leaks every visitor's address, and still works.
var ocrOverrides = []string{
	"/api/ocr/worker.min.js", "/api/ocr/tesseract-core-simd-lstm.wasm.js",
}

func TestTheBundleReachesForNobodyElsesCodeOrFonts(t *testing.T) {
	root, files := bundleFiles(t)
	bodies := map[string]string{}
	whole := strings.Builder{}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		bodies[rel] = string(body)
		whole.Write(body)
	}
	tamed := hasAll(whole.String(), ocrOverrides)
	for rel, text := range bodies {
		for _, host := range cdnHosts {
			if !strings.Contains(text, host) {
				continue
			}
			if host == "cdn.jsdelivr.net" && tamed {
				continue
			}
			t.Errorf("%s reaches for %s.\n"+
				"A CDN is a hotlink with better marketing: it leaks every "+
				"visitor's address to a third party and puts unpinned bytes "+
				"in the page. Serve it first-party instead -- and if this is "+
				"the OCR worker's default, the bundle has lost lib/reader.ts's "+
				"overrides (%v), which is the whole reason it was inert.",
				rel, host, ocrOverrides)
		}
	}
}

// An absolute URL inside a stylesheet is a fetch: `@import url(https://…)` and
// `src: url(https://…)` are how a webfont arrives without anybody deciding to
// add one. `data:` is not a fetch and is how this bundle carries its own
// noise textures.
var cssAbsoluteURL = regexp.MustCompile(`url\(\s*['"]?(?:https?:)?//`)

func TestNoStylesheetFetchesAnythingFromOffTheInstance(t *testing.T) {
	root, files := bundleFiles(t)
	for _, path := range files {
		if !strings.HasSuffix(path, ".css") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		if m := cssAbsoluteURL.FindString(string(body)); m != "" {
			t.Errorf("%s fetches from off the instance (%q). Every face this "+
				"project sets is a .woff2 it serves; an absolute url() in a "+
				"stylesheet is how that stops being true.", rel, m)
		}
	}
}

// The shell is where a script tag or a stylesheet link would land, and it is
// small enough to hold to the strict rule the chunks cannot be held to: every
// URL it names is relative, except a `preconnect`, which fetches nothing and
// is the right mitigation for the one hotlink the licence requires.
var htmlURLAttr = regexp.MustCompile(`(?s)<([a-zA-Z]+)([^>]*?)\s(?:href|src)="([^"]*)"`)

func TestTheShellNamesNoAbsoluteURLItDoesNotWarm(t *testing.T) {
	root, files := bundleFiles(t)
	shell := ""
	for _, path := range files {
		if filepath.Base(path) == "index.html" && filepath.Dir(path) == filepath.Join(root, "web_dist") {
			shell = path
		}
	}
	if shell == "" {
		t.Fatal("no web_dist/index.html; the shell is what this guard reads")
	}
	body, err := os.ReadFile(shell)
	if err != nil {
		t.Fatal(err)
	}
	warmed := 0
	for _, m := range htmlURLAttr.FindAllStringSubmatch(string(body), -1) {
		tag, attrs, url := m[1], m[2], m[3]
		switch {
		case strings.HasPrefix(url, "/"), strings.HasPrefix(url, "data:"),
			!strings.Contains(url, "//"):
			continue
		case tag == "link" && strings.Contains(attrs, `rel="preconnect"`):
			// A preconnect warms a handshake and downloads nothing. It is
			// allowed only for a host the app genuinely fetches from at
			// runtime, which today is Wizards' art and nothing else.
			if !strings.Contains(url, "cards.scryfall.io") {
				t.Errorf("the shell preconnects to %s. A preconnect is a "+
					"promise that the app fetches from there; if it does "+
					"not, delete it, and if it does, that is a new hotlink "+
					"and wants an argument in NOTICE.md.", url)
			}
			warmed++
		default:
			t.Errorf("the shell's <%s> names %s. Everything this app runs, "+
				"it serves.", tag, url)
		}
	}
	if warmed != 1 {
		t.Errorf("%d preconnects in the shell, want exactly 1 -- the card-art "+
			"host. A second one is either a new hotlink or a stale promise.",
			warmed)
	}
}

func hasAll(text string, want []string) bool {
	for _, w := range want {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}
