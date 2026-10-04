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

// The skip census.
//
// Skips are a budget: every `t.Skip` under `go/` is conditional on a real
// absence -- a Forge install, a live key, a root user who can read anything,
// a frozen fixture that happens not to hold the shape a test wants -- and
// never a way to switch a test off while leaving it green. The polish pass
// counted them by hand each run and read the new ones (15, then 40, then 59
// across successive runs; the ledger's White section carries the dates),
// which is how a dead one was found: a skip that sat after the real
// assertion inside a branch nothing took, relabelling a green test as one
// that never ran. A count moving is the signal, and a number written into
// prose is a claim that rots, so this register logs the count instead and
// refuses the one shape that is dead on arrival.
//
// **Unconditional** means a `t.Skip`/`t.Skipf` that is a statement of a
// top-level test's own body -- not inside an `if`, a `switch`, a `select` or a
// loop, and not in a helper. A skip reached that way runs every time, and a
// test that always skips is a test that never ran wearing green. The one
// honest shape that looks similar is the trailing skip in a *helper* that
// loops over a table and returns when it finds its case (two of those exist,
// for a price rate scheduled to move): the loop's `return` is the condition,
// the helper is not a test, and this register does not read helpers. A skip
// that still wants to stand at the top of a test is not an exception to add
// here: it is a test to delete or a condition to write.
//
// What this cannot see is a conditional skip whose condition is wrong -- the
// dead skip the census found was inside an `if` -- so the count it logs is
// still the thing to read: a run whose census moved reads the new sites, as
// the polish pass's White reference says.
func TestNoTestSkipsItselfUnconditionally(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "go")
	fset := token.NewFileSet()
	sites := 0
	var unconditional []string
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
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isSkipCall(call) {
				sites++
			}
			return true
		})
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isTopLevelTest(fn) {
				continue
			}
			for _, stmt := range fn.Body.List {
				expr, ok := stmt.(*ast.ExprStmt)
				if !ok {
					continue
				}
				if call, ok := expr.X.(*ast.CallExpr); ok && isSkipCall(call) {
					unconditional = append(unconditional,
						rel+":"+fset.Position(call.Pos()).String()[len(path)+1:]+" "+fn.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if sites == 0 {
		t.Fatal("found no t.Skip under go/, so this census is reading the wrong tree")
	}
	sort.Strings(unconditional)
	for _, site := range unconditional {
		t.Errorf("%s skips as a statement of the test's own body, so it never "+
			"runs and reads green. A skip is conditional on a real absence -- put "+
			"it inside the condition, or delete the test.", site)
	}
	t.Logf("%d t.Skip sites under go/", sites)
}

// isSkipCall is `x.Skip(...)` or `x.Skipf(...)` on any receiver -- `t`, `tb`,
// a helper's own parameter -- which is wider than `*testing.T` on purpose:
// the register reads the shape, not the type, and a false positive here
// would be a method named Skip in a test file, which the tree does not have.
func isSkipCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if _, ok := sel.X.(*ast.Ident); !ok {
		return false
	}
	return sel.Sel.Name == "Skip" || sel.Sel.Name == "Skipf"
}
