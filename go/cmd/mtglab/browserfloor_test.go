package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The browser the site still works in, pinned against what the bundle needs.
//
// **The floor is Safari 16.4, declared by Aaron on 2026-08-19.** Before that it
// was stated as Safari 15 on macOS 12 -- this machine's own browser, and the
// reason `web/src` was swept for regex lookbehind every polish run. That
// statement stopped being true and nothing said so.
//
// Two independent things hold it there, which is what settled the decision:
// either alone would have to be argued away, and neither is a line anybody
// wrote on purpose.
//
//  1. **The look of the site.** Tailwind v4 emits `@property` and
//     `color-mix(in lab, ...)`, both Safari 16.4, into the stylesheet. Below the
//     floor this is quiet rather than fatal (see below), which is why it went
//     unnoticed for a release.
//  2. **The camera door.** `web/src/lib/reader.ts` pins `corePath` to
//     `tesseract-core-simd-lstm.wasm.js` and the OCR shelf serves exactly that
//     file first-party with no non-SIMD sibling; **WebAssembly SIMD is Safari
//     16.4**. Below the floor the reader does not degrade, it fails to start.
//     [TestTheCameraDoorStillHoldsTheFloorIndependently] is that route made
//     checkable, because it is the one a refactor could remove without touching
//     a line of CSS -- and the day it goes, the floor rests on Tailwind alone
//     and the decision is worth re-asking.
//
// The sweep that missed it looked in the wrong place. Grepping `web/src` finds
// what *we* wrote, and every feature that actually raised the floor arrived
// through a dependency: Tailwind's two above, and React's `Object.hasOwn` and
// `structuredClone` (both Safari 15.4). None of it appears in a single file we
// author, so a hand grep of the source could never have caught it, and did not.
//
// What that costs below the floor is not a white page -- it is quieter, which
// is why it went unnoticed. `@property` registers the `--tw-*` custom
// properties Tailwind composes shadows, transforms, filters and rings out of;
// where the at-rule is ignored those variables are unregistered, `var()` yields
// the guaranteed-invalid value, and the whole `box-shadow` / `transform` /
// `backdrop-filter` declaration drops. The page renders, laid out correctly,
// with its depth quietly gone.
//
// So this file does two things a checklist cannot. It scans the artifact users
// actually download -- `web_dist/assets`, not `web/src` -- and it makes the
// floor a **declared value** rather than a remembered one, so the next
// dependency that raises it fails a test instead of a friend's phone.
//
// It is a tripwire, not a compiler. Matching `structuredClone` in a minified
// bundle proves the string is there, not that the call runs; the class of drift
// it catches is the one that happened -- a floor raised by an upgrade nobody
// priced -- and for that a string is enough.
//
// **This file is the second time that guard has been written.** The first was
// Python (`tests/test_browser_floor.py`), and the Go crossing deleted it with
// the interpreter; `web/README.md` spent the days after recording that the
// floor was "declared here and enforced nowhere", which is the state this file
// ends. A guard that only one language could hold was never a guard.

// safariFloor is the oldest Safari the site supports, as major and minor.
//
// **Raised from 15.0 to 16.4 by Tailwind v4 and declared by Aaron on
// 2026-08-19**, once the camera door turned out to reach the same number by a
// route of its own. Lowering it means removing what [floorSetting] names below
// *and* answering the reader's SIMD core -- the comment above has both.
var safariFloor = version{16, 4}

type version struct{ major, minor int }

func (v version) String() string { return fmt.Sprintf("%d.%d", v.major, v.minor) }
func (v version) above(f version) bool {
	return v.major > f.major || (v.major == f.major && v.minor > f.minor)
}

// browserFeature is one marker string and the Safari that shipped what it
// means.
type browserFeature struct {
	marker string
	needs  version
	what   string
}

// browserFeatures is the tripwire's whole vocabulary.
//
// Only markers unambiguous enough in minified output to be worth asserting on.
// A miss here is cheap (the tripwire stays silent); a false positive would cost
// somebody an afternoon, so borderline strings are left out on purpose.
var browserFeatures = []browserFeature{
	// CSS
	{"@property", version{16, 4}, "registered custom properties (Tailwind v4)"},
	{"color-mix(in lab", version{16, 4}, "lab colour interpolation (Tailwind v4)"},
	{"color-mix(in oklab", version{16, 2}, "oklab colour interpolation (Tailwind v4)"},
	{"color-mix(in srgb", version{16, 2}, "colour mixing, hand-written in index.css"},
	{"container-type", version{16, 0}, "container queries"},
	{"@container", version{16, 0}, "container query at-rule"},
	{"@layer", version{15, 4}, "cascade layers (Tailwind v4)"},
	{":focus-visible", version{15, 4}, "focus ring only for keyboard users"},
	{"oklch(", version{15, 4}, "oklch colours"},
	{"dvh", version{15, 4}, "dynamic viewport units"},
	{"svh", version{15, 4}, "small viewport units"},
	{":has(", version{15, 4}, "the parent selector"},
	{"text-wrap:", version{17, 4}, "text-wrap: balance / pretty"},
	{"@starting-style", version{17, 5}, "entry animations from a start state"},
	{"light-dark(", version{17, 5}, "light-dark() colour pairs"},
	{"field-sizing", version{18, 0}, "auto-sizing form controls"},
	{"anchor-name", version{18, 0}, "CSS anchor positioning"},
	// JS
	{"Object.hasOwn", version{15, 4}, "Object.hasOwn (React)"},
	{"structuredClone", version{15, 4}, "structuredClone (React)"},
	{"reportError", version{15, 4}, "reportError (React)"},
	{".toSorted(", version{16, 4}, "non-mutating array sort"},
	{".toReversed(", version{16, 4}, "non-mutating array reverse"},
	{".toSpliced(", version{16, 4}, "non-mutating array splice"},
	{"URL.canParse", version{17, 0}, "URL.canParse"},
	{"Object.groupBy", version{17, 4}, "Object.groupBy"},
	{"Map.groupBy", version{17, 4}, "Map.groupBy"},
	{"Promise.withResolvers", version{17, 4}, "Promise.withResolvers"},
	{"Array.fromAsync", version{18, 0}, "Array.fromAsync"},
}

// floorSetting is what holds the floor where it is. If one of these ever stops
// appearing the floor may be able to come down -- which is a decision to take
// deliberately, so the test says so rather than letting it drift the other way
// unnoticed. A floor is only ever checked when it fails upward.
var floorSetting = []string{"@property", "color-mix(in lab"}

// lookbehind is lookbehind, and only lookbehind. The old standing rule grepped
// `(?<`, which also matches a *named capture group* -- `(?<name>...)` has
// worked since Safari 11.3 and is not the hazard. These two are.
var lookbehind = []string{"(?<=", "(?<!"}

// bundle is every byte of the committed frontend, as one string: the artifact a
// phone actually parses, which is the whole reason this file exists.
func bundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "web_dist", "assets")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("no committed bundle at %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case ".js", ".css":
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	// Anti-vacuity, and it is the failure this whole file is about: an empty
	// read reads exactly like a clean one, and every assertion below is a
	// `strings.Contains` that a missing haystack passes.
	if len(names) < 2 {
		t.Fatalf("only %d script/stylesheet files under %s; the bundle has "+
			"moved and every check here would pass on nothing", len(names), dir)
	}
	var b strings.Builder
	for _, n := range names {
		body, readErr := os.ReadFile(filepath.Join(dir, n))
		if readErr != nil {
			t.Fatal(readErr)
		}
		b.Write(body)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestTheBundleStaysWithinTheDeclaredFloor: nothing users download may need a
// browser newer than [safariFloor].
func TestTheBundleStaysWithinTheDeclaredFloor(t *testing.T) {
	t.Parallel()
	blob := bundle(t)
	var over []string
	for _, f := range browserFeatures {
		if f.needs.above(safariFloor) && strings.Contains(blob, f.marker) {
			over = append(over, fmt.Sprintf("  %q needs Safari %s - %s",
				f.marker, f.needs, f.what))
		}
	}
	if len(over) > 0 {
		sort.Strings(over)
		t.Errorf("the committed bundle needs a newer browser than the declared "+
			"floor of Safari %s:\n%s\n\nThis is the drift this file exists to "+
			"catch. Either stop using the feature, or raise safariFloor "+
			"deliberately and say so in the ledger -- raising it is a decision "+
			"about whose phone still works, not a formality.",
			safariFloor, strings.Join(over, "\n"))
	}
}

// TestNoRegexLookbehindReachesTheBrowser: lookbehind is the one JS feature this
// project has always refused.
//
// Checked against the bundle rather than `web/src`, because a dependency can
// ship one just as easily as we can and only one of those two places is what a
// phone actually parses.
func TestNoRegexLookbehindReachesTheBrowser(t *testing.T) {
	t.Parallel()
	blob := bundle(t)
	for _, pat := range lookbehind {
		if strings.Contains(blob, pat) {
			t.Errorf("regex lookbehind %q reached the committed bundle. Safari "+
				"did not support it until 16.4, and a lookbehind in a served "+
				"regex is a SyntaxError at parse time -- the whole module fails "+
				"to load, which is the white-page failure rather than a "+
				"degraded one.", pat)
		}
	}
}

// TestTheFloorSettingFeaturesAreStillWhatHoldsIt keeps the floor a setting
// rather than a ratchet.
func TestTheFloorSettingFeaturesAreStillWhatHoldsIt(t *testing.T) {
	t.Parallel()
	blob := bundle(t)
	for _, m := range floorSetting {
		if !strings.Contains(blob, m) {
			t.Errorf("%q no longer appears in the bundle. It is one of the two "+
				"things that put the floor at Safari %s; without it the floor "+
				"may be able to come down. Re-run the scan, lower safariFloor "+
				"if it can go, and record the new number in "+
				"docs/polish/LEDGER.md.", m, safariFloor)
		}
	}
}

// TestTheCameraDoorStillHoldsTheFloorIndependently is the second route to 16.4,
// and the one no CSS scan can see.
//
// The reader's core is fetched at run time and cached under the data directory,
// so git holds none of it and [bundle] cannot reach it. What is in the
// repository is the *name* the client asks for and the table the server will
// answer with -- and both say SIMD, which is Safari 16.4 and is a hard failure
// below it rather than a quiet one.
//
// Read off the served shelf rather than restated: a swap to the plain core
// would be a one-word edit in two files, and it would silently make Tailwind
// the only thing holding the floor.
func TestTheCameraDoorStillHoldsTheFloorIndependently(t *testing.T) {
	t.Parallel()
	var cores []string
	for name := range reference.Runtime().OCR.Assets {
		if strings.Contains(name, "core") {
			cores = append(cores, name)
		}
	}
	sort.Strings(cores)
	want := []string{"tesseract-core-simd-lstm.wasm.js"}
	if len(cores) != 1 || cores[0] != want[0] {
		t.Fatalf("the reading engine's core is now %v, not %v. If it is no "+
			"longer a SIMD build, the camera door no longer requires Safari "+
			"16.4 and the floor rests on Tailwind alone -- which is a decision "+
			"to re-take, not a line to update.", cores, want)
	}
	reader, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "src", "lib", "reader.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reader), "/api/ocr/"+cores[0]) {
		t.Errorf("lib/reader.ts asks for a core the server does not serve; it "+
			"and the OCR shelf moved without each other. The shelf serves %q.",
			cores[0])
	}
}
