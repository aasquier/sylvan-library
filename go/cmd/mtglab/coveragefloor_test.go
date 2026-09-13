package main

import (
	"go/build"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The coverage floor is computed on one CI leg (`ci.yml`, the `Coverage
// floor` step runs under `if: matrix.arch == 'arm64'`), and the whole
// argument for that is a property of the tree: every Go file compiles on
// both architectures, so the merged number cannot differ between them and
// running it twice measured nothing the first run had not. This test is that
// property made checkable — the day an arch-constrained file lands, the
// arm64 leg's floor stops measuring the whole tree's view, and nothing else
// anywhere would say so.
//
// **It evaluates the toolchain's own rules rather than grepping for them.**
// `build.Context.MatchFile` applies exactly what the compiler applies — the
// `_amd64.go`/`_arm64.go` filename suffixes *and* any `//go:build` line in
// the file's header — so a constraint spelled either way is caught, and a
// spelling this test's author never thought of is caught too. Test files are
// deliberately in scope: an arch-tagged test runs on one leg only, covers
// statements the other leg's run does not, and skews the number just as
// surely as an arch-tagged body would.
//
// When this fires, there are two honest ways out: make the file portable, or
// move the `Coverage floor` step back to both matrix legs and delete this
// test — never a third where the file stays and the floor quietly narrows.
func TestEveryGoFileCompilesOnBothCILegs(t *testing.T) {
	t.Parallel()

	// Both CI legs are Linux with CGO on (`CGO_ENABLED=1`, the DuckDB
	// driver); only GOARCH separates them. Matching the legs exactly is the
	// point — a darwin context here would drag `system_darwin.go` into the
	// question, and that file's exclusion on CI is shared by both legs.
	amd := build.Default
	amd.GOOS, amd.GOARCH, amd.CgoEnabled = "linux", "amd64", true
	arm := build.Default
	arm.GOOS, arm.GOARCH, arm.CgoEnabled = "linux", "arm64", true

	goRoot := filepath.Join(repoRoot(t), "go")
	checked := 0
	err := filepath.WalkDir(goRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// `testdata` is invisible to the toolchain, so its contents are
			// outside the property; a hidden directory is outside the module.
			if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		dir := filepath.Dir(path)
		onAmd, err := amd.MatchFile(dir, d.Name())
		if err != nil {
			return err
		}
		onArm, err := arm.MatchFile(dir, d.Name())
		if err != nil {
			return err
		}
		checked++
		if onAmd != onArm {
			in, out := "amd64", "arm64"
			if onArm {
				in, out = "arm64", "amd64"
			}
			rel, _ := filepath.Rel(goRoot, path)
			t.Errorf("go/%s compiles on %s and not on %s, so the coverage "+
				"floor — computed on the arm64 leg alone (ci.yml) — no longer "+
				"measures the whole tree's view. Make the file portable, or "+
				"move the Coverage floor step back to both legs and delete "+
				"this test.", rel, in, out)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Anti-vacuity: a walk that found almost nothing has walked the wrong
	// root, and would otherwise read exactly like a portable tree.
	if checked < 100 {
		t.Fatalf("only %d Go files under %s; the walk is broken", checked, goRoot)
	}
}
