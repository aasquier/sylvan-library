package cache

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// TestTheTimestampIsTheRecordedFormat covers the two details `internal/jobs`
// had to find the hard way, in the module where they are the **sort key**:
// `last_used_at` is what eviction orders by, as text.
//
// The fraction vanishes entirely at a zero microsecond -- the format writes
// `...:20+00:00`, never `...:20.000000+00:00` -- and the offset is spelled
// `+00:00`, never `Z`. Both happen roughly once in a million, which is exactly
// why neither shows up in a store test that stamps rows from the clock.
func TestTheTimestampIsTheRecordedFormat(t *testing.T) {
	t.Parallel()
	whole := time.Date(2026, 8, 22, 6, 10, 20, 0, time.UTC)
	if got := stamp(whole); got != "2026-08-22T06:10:20+00:00" {
		t.Errorf("a whole second stamps as %q", got)
	}
	frac := time.Date(2026, 8, 22, 6, 10, 20, 123456000, time.UTC)
	if got := stamp(frac); got != "2026-08-22T06:10:20.123456+00:00" {
		t.Errorf("a fractional second stamps as %q", got)
	}
	// Nanoseconds are truncated, not rounded: the format has no unit finer
	// than the microsecond, and a clock reading below one is simply lost.
	nanos := time.Date(2026, 8, 22, 6, 10, 20, 123456999, time.UTC)
	if got := stamp(nanos); got != "2026-08-22T06:10:20.123456+00:00" {
		t.Errorf("sub-microsecond time stamps as %q", got)
	}
	// And the elided form sorts BELOW every fractional one in the same second,
	// which is what makes storing the text rather than the instant sound:
	// `+` is 0x2B and every digit is above it.
	if stamp(whole) >= stamp(frac) {
		t.Error("the microsecond-zero stamp does not sort first within its second")
	}
	// A non-UTC instant is carried to UTC rather than stamped with its own
	// offset: every recorded stamp is UTC, and the sort depends on it.
	east := time.FixedZone("east", 3600)
	if got := stamp(whole.In(east)); got != stamp(whole) {
		t.Errorf("a non-UTC time stamps as %q", got)
	}
}

// The fingerprint's two claims, neither of which the differential corpus can
// make: that every package named is actually mixed in, and that each of those
// packages embeds the whole of itself.
//
// In package rather than beside it, because both need `engineSources` -- and
// exporting that list so a test could reach it would publish the one thing
// this package's callers must not depend on.

// TestEveryEngineSourceMovesTheFingerprint proves the list is live.
//
// A package can be named in `engineSources`, embedded, and contribute nothing
// -- a `continue` in the wrong place, a nil `fs.FS`, a glob that matches no
// file -- and the fingerprint would still be a plausible 64 characters. So
// each one is dropped in turn and the digest must move. This is the mutation
// test for the mechanism ADR 18's consequence 2 rests on.
func TestEveryEngineSourceMovesTheFingerprint(t *testing.T) {
	// **Serial, and Go will not say so.** `digestOver` swaps three
	// package-level values -- `engineSources`, `fingerprintVal` and
	// `fingerprintOnce` -- and puts them back on the way out. Adding
	// `t.Parallel()` panics at nothing and passes when this test is run alone;
	// what it would do is let this test and its neighbour swap the same three
	// while the other is reading them, and let any later test in the package
	// read a fingerprint computed over a list this one was holding.
	//
	// This is the second of the three things CLAUDE.md names, and the one
	// nothing but a reading catches: `-race` sees it only when the two
	// actually overlap, which is a coin toss rather than a gate. The reason it
	// cannot simply be fixed is in this file's own package comment -- both
	// tests need `engineSources`, and exporting it would publish the one thing
	// this package's callers must not depend on.
	//
	// Snapshotted below, because `digestOver` swaps the list while it works. Ranging over the global and slicing it inside the loop reads the
	// swapped one, which is a panic on the second pass and would have been a
	// silently narrower test if the lengths had happened to match.
	real := append([]engineSource{}, engineSources...)
	full := digestOver(t, real)
	if full == "" {
		t.Fatal("the fingerprint is empty over the real source list")
	}
	for i, pkg := range real {
		without := append([]engineSource{}, real[:i]...)
		without = append(without, real[i+1:]...)
		if got := digestOver(t, without); got == full {
			t.Errorf("dropping %s does not change the fingerprint, so its "+
				"source is not in the key at all", pkg.name)
		}
	}
	// And order is part of it: two packages swapped must give a different
	// digest, or the name prefix is not doing its job either.
	if len(real) >= 2 {
		swapped := append([]engineSource{}, real...)
		swapped[0], swapped[1] = swapped[1], swapped[0]
		if digestOver(t, swapped) == full {
			t.Error("reordering the source list does not change the fingerprint")
		}
	}
}

// digestOver runs `Fingerprint`'s body over an arbitrary list, which is why
// that body is worth keeping in one loop.
//
// It swaps the package's own state and puts it back **before returning**,
// rather than through `t.Cleanup`: a cleanup runs at the end of the test, so
// the global would stay swapped for the rest of the body. These subtests
// therefore cannot run in parallel with anything that reads the real
// fingerprint -- none of them do, and none of them may.
func digestOver(t *testing.T, sources []engineSource) string {
	t.Helper()
	saved, savedVal := engineSources, fingerprintVal
	defer func() {
		engineSources, fingerprintVal = saved, savedVal
		fingerprintOnce = sync.Once{}
		// Re-arm the memo on the real list, so a later test in this package
		// sees the fingerprint this binary really has.
		Fingerprint()
	}()
	engineSources = sources
	fingerprintVal = ""
	fingerprintOnce = sync.Once{}
	return Fingerprint()
}

// TestTheFingerprintHashesNamesAsWellAsBytes proves the path prefix is doing
// work.
//
// Without it, two files swapping contents -- a rename, a file split in two --
// would hash identically, and the cache would keep serving numbers computed by
// code that has moved. Real embedded sources cannot demonstrate that; two
// synthetic filesystems can, which is why `engineSource.fs` is an `fs.FS`
// rather than an `embed.FS`.
func TestTheFingerprintHashesNamesAsWellAsBytes(t *testing.T) {
	// **Serial**, for `TestEveryEngineSourceMovesTheFingerprint`'s reason:
	// `digestOver` swaps the package-level source list and the memo behind
	// `Fingerprint`. Go accepts `t.Parallel()` here and the collision is
	// silent -- and a fingerprint that came out wrong would not fail this
	// test, it would fail whichever cache test ran next.
	first := digestOver(t, []engineSource{{"pkg", fstest.MapFS{
		"a.go": &fstest.MapFile{Data: []byte("alpha")},
		"b.go": &fstest.MapFile{Data: []byte("beta")},
	}}})
	swapped := digestOver(t, []engineSource{{"pkg", fstest.MapFS{
		"a.go": &fstest.MapFile{Data: []byte("beta")},
		"b.go": &fstest.MapFile{Data: []byte("alpha")},
	}}})
	if first == swapped {
		t.Error("two files swapping contents hash the same, so a rename is " +
			"invisible to the key")
	}
	// And the package name is in it too, so the same file under two packages
	// is two different engines.
	elsewhere := digestOver(t, []engineSource{{"other", fstest.MapFS{
		"a.go": &fstest.MapFile{Data: []byte("alpha")},
		"b.go": &fstest.MapFile{Data: []byte("beta")},
	}}})
	if first == elsewhere {
		t.Error("the package name is not in the fingerprint")
	}
	// The separator matters as well: without it, `a.go` + "lpha" would hash
	// the same as `a.gol` + "pha".
	run := digestOver(t, []engineSource{{"pkg", fstest.MapFS{
		"a.go": &fstest.MapFile{Data: []byte("b.goalphabeta")},
	}}})
	if run == first {
		t.Error("the fingerprint has no separator between a name and its bytes")
	}
}

// TestTheGlobIsSortedAlready records an EQUIVALENT mutation rather than
// hiding it.
//
// Deleting the `sort.Strings` in `Fingerprint` changes nothing: `fs.Glob`
// walks through `fs.ReadDir`, which is documented to return entries "sorted by
// filename", so the names arrive sorted for `embed.FS`, for `os.DirFS` and for
// the `fstest.MapFS` above. The sort stays because that guarantee lives in a
// doc comment rather than in a type, and a future `fs.FS` that implements
// `ReadDirFS` itself may return whatever order it likes -- at which point the
// deleted line would be a wrong key nobody could see. This test says the
// mutation is equivalent *today* and names the day it stops being.
func TestTheGlobIsSortedAlready(t *testing.T) {
	t.Parallel()
	for _, pkg := range engineSources {
		names, err := fs.Glob(pkg.fs, "*")
		if err != nil {
			t.Fatalf("%s: glob: %v", pkg.name, err)
		}
		if !sort.StringsAreSorted(names) {
			t.Errorf("%s globs as %v, which is NOT sorted -- `sort.Strings` "+
				"in Fingerprint has stopped being belt-and-braces and is now "+
				"load-bearing", pkg.name, names)
		}
	}
}

// TestEveryFingerprintedPackageEmbedsItsWholeSelf is the guard the explicit
// `//go:embed` lists need.
//
// `*.go` would have been shorter and is wrong twice over -- it matches
// `_test.go`, so every test edit would empty the deployed cache and the test
// files would ride into the image -- so each package lists its own files. A
// list is a thing somebody forgets to update, and forgetting here is silent:
// the fingerprint stays a plausible digest and simply stops covering the file
// that was added. This reads each directory and fails by name.
func TestEveryFingerprintedPackageEmbedsItsWholeSelf(t *testing.T) {
	t.Parallel()
	// From `go/internal/sim/cache` to the module's `internal`.
	root := filepath.Join("..", "..", "..", "internal")
	for _, pkg := range engineSources {
		rel := strings.TrimPrefix(pkg.name, "internal/")
		dir := filepath.Join(root, filepath.FromSlash(rel))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("%s: read %s: %v", pkg.name, dir, err)
		}
		onDisk := map[string]bool{}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") ||
				strings.HasSuffix(name, "_test.go") {
				continue
			}
			onDisk[name] = true
		}
		embedded := map[string]bool{}
		names, err := fs.Glob(pkg.fs, "*")
		if err != nil {
			t.Fatalf("%s: glob: %v", pkg.name, err)
		}
		for _, name := range names {
			embedded[name] = true
		}
		for name := range onDisk {
			if !embedded[name] {
				t.Errorf("%s/%s is not embedded: add it to that package's "+
					"`//go:embed` list, or the simulation cache stops noticing "+
					"changes to it", pkg.name, name)
			}
		}
		for name := range embedded {
			if !onDisk[name] {
				t.Errorf("%s embeds %s, which is not a non-test source file "+
					"there", pkg.name, name)
			}
		}
		if len(onDisk) == 0 {
			t.Errorf("%s has no source files at all, which cannot be right",
				pkg.name)
		}
	}
}
