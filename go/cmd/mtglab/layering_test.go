package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The layering register.
//
// Three absolute claims about this tree's shape are written down in three
// places -- CLAUDE.md's architecture block, the polish pass's Blue reference
// ("the layering is checkable ... grep, don't trust"), and every package
// comment that rests on one of them:
//
//  1. DuckDB stays behind `internal/pool`. Nothing else names the driver.
//  2. `internal/door` owns the HTTP concerns and sits ABOVE `internal/api`;
//     nothing in `internal/api` reaches back around it.
//  3. The determinism kernels import nothing above them.
//
// Until this file they were held by a person re-running three greps. Three
// consecutive Blue runs did exactly that and wrote the answers into the
// ledger, which is the tell: a claim whose enforcement is a habit is a claim
// with a date on it. The greps are here now, and they are stronger than the
// hand version in one way worth naming.
//
// # Why this parses rather than asking the toolchain
//
// `go list` and `packages.Load` answer for the platform they run on, so a
// `_linux.go` file's imports are invisible to every grep and every build on
// this project's darwin laptop -- the same blind spot blue.md warns about for
// the linter. A `go/parser` walk in [parser.ImportsOnly] mode reads every
// file in the tree whatever its build tags say, so a driver import smuggled
// into a platform-tagged file is caught here and not first in CI. It is also
// hermetic: no subprocess, no module cache, no network.
//
// Test files are deliberately outside all three claims. A test may import
// anything -- `internal/night`'s suite reaches `internal/api` on purpose to
// drive a real route -- and layering is a statement about what the binary
// links, never about what a fixture borrows.

// modulePathOf reads the module path off `go/go.mod` rather than restating it.
// A module rename then moves every import path in this file at once, and a
// typo cannot quietly reduce the in-module graph to nothing (the anti-vacuity
// checks below would catch that too, which is the point of having both).
func modulePathOf(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "go", "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	for line := range strings.Lines(string(body)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("go.mod names no module")
	return ""
}

// importGraph maps every package in this module to the import paths its
// non-test files name -- all of them, in-module and out.
func importGraph(t *testing.T) map[string][]string {
	t.Helper()
	module := modulePathOf(t)
	root := filepath.Join(repoRoot(t), "go")
	fset := token.NewFileSet()
	graph := map[string][]string{}
	err := filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(file))
		if err != nil {
			return err
		}
		pkg := module
		if rel != "." {
			pkg = module + "/" + filepath.ToSlash(rel)
		}
		for _, spec := range parsed.Imports {
			graph[pkg] = append(graph[pkg], strings.Trim(spec.Path.Value, `"`))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if len(graph) < 30 {
		t.Fatalf("only %d packages under go/ -- this walk is reading the wrong "+
			"tree, and an empty graph satisfies every claim below", len(graph))
	}
	return graph
}

// reaches is the transitive closure of one package's in-module imports.
// Everything outside the module is dropped: these claims are about this
// tree's own layers, and the standard library is under all of them.
func reaches(graph map[string][]string, module, from string) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(pkg string) {
		for _, dep := range graph[pkg] {
			if dep != module && !strings.HasPrefix(dep, module+"/") {
				continue
			}
			if seen[dep] {
				continue
			}
			seen[dep] = true
			walk(dep)
		}
	}
	walk(from)
	return seen
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// duckdb matches any import path naming the database, driver and bindings
// alike. A pattern rather than the pinned path from go.mod, because the
// bindings arrive as half a dozen per-platform modules and the claim is about
// the word, not about one release's spelling of it.
var duckdb = regexp.MustCompile(`(?i)duckdb`)

// TestOnlyTheCardPoolNamesTheDatabase is claim 1.
//
// `internal/pool` is the card pool's whole surface: DuckDB's dialect, its
// Appender, its lease discipline and its file all stop there, so every caller
// above it asks in Go and is free to be tested against a fixture rather than
// a database. A second package that names the driver would not announce
// itself -- it would look like one convenient query -- and from then on
// "behind `internal/pool`" would be prose.
func TestOnlyTheCardPoolNamesTheDatabase(t *testing.T) {
	t.Parallel()

	module := modulePathOf(t)
	graph := importGraph(t)
	pool := module + "/internal/pool"

	var named []string
	for pkg, imports := range graph {
		if slices.ContainsFunc(imports, duckdb.MatchString) {
			named = append(named, pkg)
		}
	}
	sort.Strings(named)

	if !slices.Contains(named, pool) {
		t.Fatalf("%s names no DuckDB import, so this guard is matching nothing "+
			"and would pass over a driver anywhere else", pool)
	}
	for _, pkg := range named {
		if pkg == pool {
			continue
		}
		t.Errorf("%s names a DuckDB import. The database stays behind %s: "+
			"every caller above it asks in Go, which is what lets the callers "+
			"be tested against a fixture. Add the query to the pool's own "+
			"surface instead.", pkg, pool)
	}
}

// TestTheDoorIsAboveTheAPIAndNothingReachesBack is claim 2, and the two
// halves are guarding different things -- worth saying, because one of them
// would otherwise look like ceremony.
//
// The door decides who is allowed in **before routing** (ADR 5's 404-not-403,
// ADR 17's admin prefix) and then hands the survivors to the route table. So
// the door knows the API and the API must not know the door: a handler that
// could reach a door helper could answer a question the middleware has
// already refused, while the sweeps that derive deny-by-default from the
// served route table still read green.
//
// **While the door imports the API, Go itself refuses the reach-back** -- any
// path from `internal/api` home to `internal/door` closes an import cycle and
// the build stops. That is the strongest possible enforcement and this test
// does not improve on it. What the pair of assertions catches is the *other*
// way the layer can be lost: an inversion. A session that moves the route
// table above the middleware has a tree the compiler is perfectly happy with
// and a door whose refusals a handler can now outrank, so the direction is
// asserted as a fact and its loss is fatal here rather than invisible.
//
// The third assertion is the one that guards a live edge: the door is mounted
// once, by the composition root. Nothing stops a package below from importing
// it -- no cycle, no complaint -- and the day one does there are two places
// the middleware's order is decided.
func TestTheDoorIsAboveTheAPIAndNothingReachesBack(t *testing.T) {
	t.Parallel()

	module := modulePathOf(t)
	graph := importGraph(t)
	door := module + "/internal/door"
	api := module + "/internal/api"
	root := module + "/cmd/mtglab"

	if !reaches(graph, module, door)[api] {
		t.Fatalf("%s does not reach %s, so the layer this test describes is "+
			"not the tree's shape any more -- re-read the door's package "+
			"comment before deciding which half moved", door, api)
	}
	if reaches(graph, module, api)[door] {
		t.Errorf("%s reaches %s. The door refuses before it routes, so a route "+
			"that can reach back into it can answer past a refusal the door's "+
			"own sweeps still read as covered. Pass what the handler needs in "+
			"as a value instead.", api, door)
	}
	for pkg, imports := range graph {
		if pkg == root || !slices.Contains(imports, door) {
			continue
		}
		t.Errorf("%s imports %s. The door is mounted once, by the composition "+
			"root, so that there is exactly one place the middleware's order "+
			"is decided.", pkg, door)
	}
}

// kernels are the determinism kernels named in CLAUDE.md's architecture
// block: the seeded generator, the exact float arithmetic and its rendering,
// the recorded string semantics, and the deck file's one YAML style.
//
// They are the floor everything recorded rests on. A stored Tier 1 cache key,
// a frozen `testdata/` corpus and a deck file's byte-for-byte shape are all
// promises that these four packages answer the same way forever, which is
// only affordable while they depend on nothing that can change under them.
var kernels = []string{"mt19937", "floats", "textutil", "yamlemit"}

// kernelFloor is the one in-module package a kernel may reach, and the reason
// is `yamlemit/render.go`: the quoting rules for a `why` containing a colon or
// a trailing space are the emitter's, not hand-rolled, and `internal/deckyaml`
// is that emitter. It qualifies because it is itself a floor -- the assertion
// below holds it to zero in-module imports of its own -- so the kernels' own
// promise is not resting on anything that can move.
const kernelFloor = "deckyaml"

// TestTheDeterminismKernelsImportNothingAboveThem is claim 3.
func TestTheDeterminismKernelsImportNothingAboveThem(t *testing.T) {
	t.Parallel()

	module := modulePathOf(t)
	graph := importGraph(t)

	allowed := map[string]bool{}
	for _, name := range append(slices.Clone(kernels), kernelFloor) {
		pkg := module + "/internal/" + name
		if _, ok := graph[pkg]; !ok {
			t.Fatalf("%s is not a package in this tree. A kernel that has been "+
				"renamed or moved silently empties this guard, so the list is "+
				"held against the tree rather than read from it.", pkg)
		}
		allowed[pkg] = true
	}

	floor := module + "/internal/" + kernelFloor
	if got := sorted(reaches(graph, module, floor)); len(got) > 0 {
		t.Errorf("%s reaches %v. It is on the kernels' allowed list only "+
			"because it is itself a floor; give it an in-module dependency and "+
			"the four kernels below inherit whatever that dependency does.",
			floor, got)
	}

	for _, name := range kernels {
		pkg := module + "/internal/" + name
		for _, dep := range sorted(reaches(graph, module, pkg)) {
			if allowed[dep] {
				continue
			}
			t.Errorf("%s reaches %s. The determinism kernels are the floor a "+
				"stored cache key and a frozen corpus rest on; a dependency "+
				"above them is a way for a recorded answer to change without "+
				"the kernel being touched. Hand the value in instead.", pkg, dep)
		}
	}
}
