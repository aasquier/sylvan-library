package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The parallel register.
//
// Every top-level test under `go/` calls `t.Parallel()` as a statement of its
// own, and this is where that stops being a claim. It used to be a register
// of exceptions: CLAUDE.md's Testing section said a test was parallel "unless
// it is holding something shared", and this file held that every serial test
// said why where it stood -- 39 of them on 2026-09-19, each with an argued
// paragraph. Reading those paragraphs together was the useful part, because
// they were not thirty-nine reasons. They were ten pieces of shared state,
// and every one of them was a fact about the code rather than about the
// tests: a ceiling read off the process at call time, a Fly token read the
// same way, a Scryfall URL in a package variable so a test could swap it, a
// coverage index every machine shared, a `PATH` search, three environment
// readers with no way to describe an environment but installing one, a
// fingerprint computed over a package-level list, a slow-request floor in a
// package variable, and a stop that was a signal to the whole process. On
// 2026-09-24 each became a value -- handed in, described in a struct literal,
// built once at the composition root -- and the count went to zero without a
// single test being weakened to get there.
//
// So the definition is now the whole rule. **Parallel** means the test's own
// body calls `t.Parallel()` directly. A `t.Parallel()` inside a `t.Run`
// closure does not count, and that is the honest reading of Go, not a
// conservatism: a parallel subtest waits for its parent to return and then
// runs beside its siblings, while the parent itself still takes its turn
// against every other top-level test in the package. `TestMain` and
// benchmarks are not tests and are not counted.
//
// A new test that Go refuses to run in parallel -- it panics on `t.Setenv`,
// or `-race` reports a package-level write -- is not a reason to add a
// serial exception here. It is the next piece of shared state, and the fix
// is the same one that cleared the other ten: make the thing a value and
// hand it in. CLAUDE.md's Testing section carries the recipe.
func TestEveryTestRunsBesideItsNeighbours(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "go")
	fset := token.NewFileSet()
	total := 0
	var serial []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isTopLevelTest(fn) {
				continue
			}
			total++
			if !callsParallelDirectly(fn) {
				serial = append(serial, rel+": "+fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if total == 0 {
		t.Fatal("found no tests under go/, so this register is reading the wrong tree")
	}
	sort.Strings(serial)
	for _, name := range serial {
		t.Errorf("%s does not call t.Parallel() as a statement of its own. Every "+
			"test under go/ runs beside its neighbours. If Go refuses (it panics "+
			"on t.Setenv, or -race reports a package-level write), the test has "+
			"found shared state the code still reads off the process: make it a "+
			"value the test can describe -- a lookup handed in, a field on the "+
			"struct, a fixture built per test -- rather than a serial exception.",
			name)
	}
	t.Logf("%d top-level tests under go/, every one parallel", total)
}

// isTopLevelTest is a `func TestX(t *testing.T)`: no receiver, one parameter
// of that type. The type check is what keeps `TestMain(m *testing.M)` out.
func isTopLevelTest(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 || len(params[0].Names) != 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

// callsParallelDirectly is a `t.Parallel()` statement in the function's own
// body -- not inside a subtest closure, and not a `defer`.
func callsParallelDirectly(fn *ast.FuncDecl) bool {
	for _, stmt := range fn.Body.List {
		expr, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Parallel" {
			return true
		}
	}
	return false
}
