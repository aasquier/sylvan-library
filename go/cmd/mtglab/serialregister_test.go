package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The serial-test register.
//
// CLAUDE.md's Testing section makes a claim it calls "one a script can
// re-check in a second rather than a claim to trust": a new test calls
// `t.Parallel` unless it is holding something shared, and **every one that
// does not says why where it stands**. The script was never in the tree. Two
// polish runs a week apart each wrote their own, and they disagreed -- one
// counted 27 serial tests and the next 39 over a tree that had added none of
// the difference, because one read `t.Parallel()` anywhere in the function
// and the other only as a direct statement. A count that changes with the
// person asking cannot be re-checked, and a claim nothing enforces has either
// drifted already or will. So the definition lives here.
//
// **Serial** means the test's own body never calls `t.Parallel()` as a
// statement of its own. A `t.Parallel()` inside a `t.Run` closure does not
// count, and that is the honest reading of Go, not a conservatism: a parallel
// subtest waits for its parent to return and then runs beside its siblings,
// while the parent itself still takes its turn against every other top-level
// test in the package. `TestMain` and benchmarks are not tests and are not
// counted.
//
// **Says why** means the word is there to be found: the test's doc comment,
// or a comment in the body ahead of its first statement, contains "serial" or
// "parallel" in any case. That is a low bar on purpose. What the register
// holds is that the reason *exists where it stands* -- "**Serial**: it swaps
// the package-level `scryfallSets`", "**Not parallel**, and the reason is not
// shared state" -- so that a session asking "why is this one alone?" reads the
// answer on the spot instead of measuring for it; judging whether the reason
// is a good one is a reading, and CLAUDE.md's own recipe for that is to add
// `t.Parallel()` and run the test alone. It is not a ceiling: a new serial
// test with its reason written down passes, and the ledger keeps the count.
var arguesItself = regexp.MustCompile(`(?i)serial|parallel`)

func TestEverySerialTestSaysWhyWhereItStands(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "go")
	fset := token.NewFileSet()
	total, serial := 0, 0
	var unargued []string
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
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
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
			if callsParallelDirectly(fn) {
				continue
			}
			serial++
			if !arguesItself.MatchString(leadingComments(fn, file)) {
				unargued = append(unargued, rel+": "+fn.Name.Name)
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
	sort.Strings(unargued)
	for _, name := range unargued {
		t.Errorf("%s is serial and does not say why. Add t.Parallel() if nothing "+
			"shared stops it (CLAUDE.md's recipe: add it and run the test alone -- "+
			"Go panics on t.Setenv, and -race reports a package-level write), or "+
			"write the reason as its doc comment or first comment, using the "+
			"word 'serial' or 'parallel' so this register can find it.", name)
	}
	t.Logf("%d top-level tests under go/, %d serial, every one argued", total, serial)
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

// leadingComments is the text a reader finds before the test does anything:
// its doc comment, plus every comment between the opening brace and the first
// statement (or the closing brace of an empty body).
func leadingComments(fn *ast.FuncDecl, file *ast.File) string {
	var sb strings.Builder
	if fn.Doc != nil {
		sb.WriteString(fn.Doc.Text())
	}
	end := fn.Body.Rbrace
	if len(fn.Body.List) > 0 {
		end = fn.Body.List[0].Pos()
	}
	for _, group := range file.Comments {
		if group.Pos() > fn.Body.Lbrace && group.End() <= end {
			sb.WriteString(group.Text())
		}
	}
	return sb.String()
}
