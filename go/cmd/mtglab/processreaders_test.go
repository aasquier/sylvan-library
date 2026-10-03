package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Where the process environment is allowed to be read, made checkable.
//
// **This is a doctrine the tree applies everywhere and enforces nowhere.**
// ADR 39 made the configuration a value and ADR 40 did the same for the
// Claude endpoint; `CLAUDE.md`'s testing section states the general form —
// *"a reader of the process became a lookup handed in"* — and lists the
// conversions by name. The reason it was worth doing is recorded next to the
// first one: eighty-eight tests in this package were serial because the only
// way to say "this deployment keeps its decks over there" was to write the
// process environment through [testing.T.Setenv], which Go panics on inside
// a parallel test. Every one of those tests was about something else.
//
// Nothing holds the doctrine. A new package that reads [os.Getenv] where it
// wants the value — which is the natural thing to write, and what every one
// of these packages used to do — compiles, passes, and makes the next test
// that needs to describe a deployment serial again. The failure is not in the
// diff that adds the read; it arrives weeks later as a `t.Setenv` somebody
// could not avoid, and by then the fix is a refactor rather than a review
// comment. This is `addressreach_test.go`'s move on a different claim: a rule
// re-checked by whoever remembers to re-check it has already drifted or is
// about to.
//
// **The register is deliberately small, and a new entry is the finding.**
// Each one carries the reason it is allowed to be the exception, so the day
// somebody adds one they are answering an argument rather than editing a
// list. The count is not written here — the test logs the number it walked,
// and a figure in prose beside a register is the one claim the register
// cannot hold. Three shapes are in it: the *seam's own default*, which is the
// one line the deployed binary runs and no test does (`LoadSettings` sat at
// 0% coverage for exactly that reason until the test beside it was written);
// a flag's documented environment fallback, where the value lands in a flag
// `--help` names and an argument always outranks; and a field's fallback on
// a type whose caller may hand the value in instead.
//
// Tests are outside the rule, and have to be: `configrecord_test.go` reads
// `.env.example` and the fixtures describe deployments with maps, which is
// the whole point.
var processEnvReaders = map[string]string{
	// The composition root's own defaults: each is `XFrom(os.Getenv)` beside
	// an injected seam, called once from `main` (or from one command's
	// constructor) and from nowhere else in the tree.
	"internal/config:Load":            "ADR 39: the one reader of the deployment, handed down as a value",
	"internal/claude:EndpointFromEnv": "ADR 40: the one reader of the Claude endpoint",
	"internal/claude:SettingsFromEnv": "ADR 40: the one reader of the stance ceiling and the model dials",
	"internal/sim/tier3:LoadSettings": "the one reader of the Forge and Fly variables",

	// A flag's environment fallback, which is a different thing from a
	// package reading the process: the value lands in a Cobra flag whose help
	// text names the variable, so `--web-dist` always wins and the default is
	// visible in `mtglab ui --help`.
	"cmd/mtglab:uiCommand": "two flag defaults (MTGLAB_WEB_DIST, MTGLAB_TAROT_DIR), named in --help and overridable",

	// Handed-in first, process second. Both of these prefer a value the
	// caller chose; the environment is the fallback, which is what lets a
	// test describe a panel with a literal.
	"internal/flymetrics:Panel.token": "the fallback under Panel.Token, which a test hands in instead",
	"internal/flymetrics:valueOr":     "the middle of three sources, under what the panel was given",
}

// processEnvCalls are the ways a Go program asks the process what it was
// told. [os.Getenv] is the one the tree uses; the others are here so that
// reaching for a different spelling is not a way around the register.
var processEnvCalls = map[string]bool{
	"Getenv": true, "LookupEnv": true, "Environ": true, "ExpandEnv": true,
}

// The register: the tree reads the process environment in exactly these
// functions, for exactly these reasons.
func TestOnlyTheArguedFunctionsReadTheProcessEnvironment(t *testing.T) {
	t.Parallel()

	found := map[string][]string{} // key -> the lines it reads on
	root := filepath.Join(repoRoot(t), "go")
	fset := token.NewFileSet()

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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		pkgDir, _ := filepath.Rel(root, filepath.Dir(path))
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			for _, at := range processEnvTouches(fn) {
				key := pkgDir + ":" + funcKey(fn)
				found[key] = append(found[key],
					filepath.Base(path)+":"+strconv.Itoa(fset.Position(at).Line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	// Anti-vacuity. A sweep that matched nothing reads exactly like a tree
	// that never consults its environment, which is not a thing a deployed
	// web application can be: the four composition-root readers are always
	// there, because the binary has to find its data directory somehow.
	if len(found) < 4 {
		t.Fatalf("only %d readers of the process environment found in the whole "+
			"tree; the sweep is looking at the wrong thing", len(found))
	}

	for key, where := range found {
		if _, argued := processEnvReaders[key]; !argued {
			t.Errorf("%s reads the process environment (%s) and no argument says why.\n"+
				"The tree's rule is that a reader of the process becomes a lookup "+
				"handed in — `func(string) string` as a parameter or a field, with "+
				"the composition root passing os.Getenv exactly once (ADR 39, ADR "+
				"40). A package that reads it where the value is wanted makes every "+
				"test about that package's behaviour serial, because describing a "+
				"deployment then needs t.Setenv and Go refuses that beside "+
				"t.Parallel. If this one is genuinely the composition root's, put "+
				"the reason in `processEnvReaders` above; otherwise take the lookup "+
				"as an argument.", key, strings.Join(where, ", "))
		}
	}
	for key, why := range processEnvReaders {
		if _, still := found[key]; !still {
			t.Errorf("%s is argued as a reader of the process environment (%q) and "+
				"reads none any more. Drop the entry — a register naming functions "+
				"that stopped mattering reads wider than the tree actually is.",
				key, why)
		}
	}
	if !t.Failed() {
		keys := make([]string, 0, len(found))
		for key := range found {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		t.Logf("%d readers of the process environment, every one argued: %s",
			len(found), strings.Join(keys, ", "))
	}
}

// processEnvTouches is every position inside fn where the process environment
// is reached for.
//
// It matches the **selector** rather than the call, because one of the
// registered readers passes `os.Getenv` as a value — `envOr(os.Getenv, …)` in
// `ui.go` — and a sweep keyed on `CallExpr` would miss exactly the shape the
// tree is full of. Handing the function along is the same act as calling it
// as far as this rule is concerned.
//
// `os` is matched by name rather than resolved, so a local variable called
// `os` would produce a false positive. That is the safe direction for a
// register — it over-reports and somebody answers it — and a package in this
// tree shadowing `os` is a finding either way.
func processEnvTouches(fn *ast.FuncDecl) []token.Pos {
	var at []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "os" || !processEnvCalls[sel.Sel.Name] {
			return true
		}
		at = append(at, sel.Pos())
		return true
	})
	return at
}
