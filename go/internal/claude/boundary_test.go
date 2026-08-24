package claude_test

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// The rule that does not bend: no model response reaches a card's `why`.
//
// Rule 4 says every card carries a rationale the user wrote. ADR 8 made that a
// gate rather than a convention. ADR 14 kept it intact against a language
// model — Claude may argue about a `why`, never author one — and ADR 15 named
// the assertion in code that has to keep passing:
//
//	no code path passes a model response into set_card_field(field="why")
//
// This file is that assertion, and it is deliberately
// written before the modes exist -- the same argument the stance makes
// for itself: retrofitting a gate around modes that already exist is how the
// gate ends up with holes in it. The failure it guards against is not a bug in
// code that exists — it is the code somebody adds later. "Tidy that rationale
// up" is one helpful-looking commit away at all times, and a test that only
// exercises today's call paths would pass right through it.
//
// # Stronger than a name check, in two specific ways
//
// The cheap version of this guard walks each file's syntax tree and matches
// identifiers by NAME. That is blunt on purpose,
// and it has two blind spots this pass does not:
//
//  1. **A name check cannot see an indirect path.** A file importing a helper
//     that itself writes a deck passes it completely. The first
//     test below bans the write engine across the whole TRANSITIVE import
//     graph, so an intermediary is not an escape.
//  2. **A name check matches strings, not objects.** A local variable called
//     `SetCardField` trips it, and an aliased import
//     can slip past it. The second test resolves
//     every identifier through the type checker, so it is the actual function
//     being named that matters and neither confusion is possible.
//
// No network, no key, no tokens. This must run everywhere, every time.

// claudeTree is the import-path fragment that marks a package as belonging to
// the Claude surfaces. It is a prefix match so that the check covers the
// packages this phase has yet to add — modes, tools, the schemas — without
// anybody remembering to list them.
const claudeTree = "/internal/claude"

// writeEngine is the package that exists only to change a deck. Banned
// outright and transitively: there is no legitimate reason for anything under
// the Claude surfaces to reach it, directly or through six intermediaries.
const writeEngine = "github.com/aasquier/sylvan-library/go/internal/deckedit"

// writeSurface is every function and method that changes a deck, in the
// packages that also hold reads and so cannot be banned wholesale.
//
// `SetCardField` is the one ADR 15 names — it is the only route to the `why`
// field — but the whole surface is listed, because a mode that can call
// `ReplaceCard` can launder a rationale through its `why` argument just as
// effectively.
var writeSurface = map[string][]string{
	writeEngine: {
		"ReplaceCard", "AddCard", "RemoveCard", "EntombCard", "ReturnCard",
		"ExileCard", "SetCardField", "SetDeckField", "SetShared", "SetNote",
	},
	"github.com/aasquier/sylvan-library/go/internal/library": {
		"WriteText", "Create", "Delete", "SetShared", "WriteArtifacts",
	},
	"github.com/aasquier/sylvan-library/go/internal/decklog": {
		"Record",
	},
}

func loadClaudePackages(t *testing.T) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Tests: false,
	}
	loaded, err := packages.Load(cfg, "github.com/aasquier/sylvan-library/go/internal/claude/...")
	if err != nil {
		t.Fatalf("loading the Claude packages: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("the Claude tree has no packages -- has it moved?")
	}
	for _, p := range loaded {
		if len(p.Errors) > 0 {
			t.Fatalf("%s did not typecheck (%v); this guard cannot run on a "+
				"broken build, and a skipped guard is a guard that is not there",
				p.PkgPath, p.Errors[0])
		}
	}
	return loaded
}

// TestNothingUnderTheClaudeSurfacesCanReachTheWriteEngine is the graph half,
// and the half a name check cannot express.
//
// `internal/deckedit` is the nine surgical operations and nothing else: every
// deck change goes through it. Banning it across the whole
// transitive import graph means an intermediary package is not a way around
// the rule — which is exactly how a determined-but-well-meaning commit would
// otherwise land it.
func TestNothingUnderTheClaudeSurfacesCanReachTheWriteEngine(t *testing.T) {
	t.Parallel()
	for _, root := range loadClaudePackages(t) {
		if path, reached := findImport(root, writeEngine, map[string]bool{}); reached {
			t.Errorf("%s reaches the deck editor: %s\n\nNothing under the "+
				"Claude surfaces may reach a deck write, directly or "+
				"indirectly -- see ADR 15. If this is deliberate, it needs a "+
				"new ADR superseding it, not a change here.",
				root.PkgPath, strings.Join(path, " -> "))
		}
	}
}

// findImport walks the transitive import graph, returning the chain that
// reaches target so the failure names the route rather than only the fact.
func findImport(p *packages.Package, target string, seen map[string]bool) ([]string, bool) {
	if p.PkgPath == target {
		return []string{p.PkgPath}, true
	}
	if seen[p.PkgPath] {
		return nil, false
	}
	seen[p.PkgPath] = true
	// Sorted so a package importing two banned routes reports the same one
	// every run; a guard whose message moves between runs is a guard people
	// learn to re-run rather than read.
	names := make([]string, 0, len(p.Imports))
	for name := range p.Imports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if chain, found := findImport(p.Imports[name], target, seen); found {
			return append([]string{p.PkgPath}, chain...), true
		}
	}
	return nil, false
}

// TestNoIdentifierUnderTheClaudeSurfacesResolvesToAWrite is the typed half.
//
// Every identifier is resolved through the type checker rather than matched as
// text, which is what makes this stronger than a name check: an aliased
// import resolves to the same object and is caught, while a local variable
// that merely shares a name resolves to a different one and is not. Comments
// and docstrings are not identifiers at all, which is what lets the packages
// here discuss `SetCardField` by name — as they must, to explain why it is
// absent.
func TestNoIdentifierUnderTheClaudeSurfacesResolvesToAWrite(t *testing.T) {
	t.Parallel()
	banned := map[string]map[string]bool{}
	for pkg, names := range writeSurface {
		banned[pkg] = map[string]bool{}
		for _, n := range names {
			banned[pkg][n] = true
		}
	}

	for _, p := range loadClaudePackages(t) {
		for _, file := range p.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj := p.TypesInfo.Uses[id]
				if obj == nil || obj.Pkg() == nil {
					return true
				}
				fn, isFunc := obj.(*types.Func)
				if !isFunc {
					return true
				}
				owner := obj.Pkg().Path()
				// A method's receiver package is where the interface or type
				// is declared, which is the package the ban names.
				if banned[owner][fn.Name()] {
					pos := p.Fset.Position(id.Pos())
					t.Errorf("%s:%d references %s.%s\n\nNothing under the "+
						"Claude surfaces may name a deck write -- see ADR 15. "+
						"An interview supplies questions; the user supplies "+
						"the words.", pos.Filename, pos.Line, owner, fn.Name())
				}
				return true
			})
		}
	}
}

// TestTheWriteSurfaceNamedHereStillExists guards against the guard rotting.
//
// If `SetCardField` is renamed, the tests above keep passing while checking for
// a function nobody can call any more — a green suite that guards nothing. So
// assert the names are real, and let a rename fail loudly here where the fix is
// obvious. This is the half of the guard
// that decays on its own, which is why it exists at all.
func TestTheWriteSurfaceNamedHereStillExists(t *testing.T) {
	t.Parallel()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes}
	paths := make([]string, 0, len(writeSurface))
	for pkg := range writeSurface {
		paths = append(paths, pkg)
	}
	sort.Strings(paths)
	loaded, err := packages.Load(cfg, paths...)
	if err != nil {
		t.Fatalf("loading the write surface: %v", err)
	}
	found := map[string]map[string]bool{}
	for _, p := range loaded {
		names := map[string]bool{}
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			switch o := obj.(type) {
			case *types.Func:
				names[name] = true
			case *types.TypeName:
				// Interface methods and methods on named types: Writer's
				// verbs live here, not in the package scope.
				if iface, ok := o.Type().Underlying().(*types.Interface); ok {
					for i := range iface.NumMethods() {
						names[iface.Method(i).Name()] = true
					}
				}
				if named, ok := o.Type().(*types.Named); ok {
					for i := range named.NumMethods() {
						names[named.Method(i).Name()] = true
					}
				}
			}
		}
		found[p.PkgPath] = names
	}
	for pkg, wanted := range writeSurface {
		for _, name := range wanted {
			if !found[pkg][name] {
				t.Errorf("%s.%s no longer exists. Update writeSurface to the "+
					"current name -- do not delete the entry.", pkg, name)
			}
		}
	}
}

// TestTheGuardCoversThePackagesThatExist is the last piece of anti-rot: a
// guard that loads zero files passes silently.
//
// The risk is sharp here, because the pattern is a prefix and a package moved
// out of the tree would simply stop being checked rather than fail.
func TestTheGuardCoversThePackagesThatExist(t *testing.T) {
	t.Parallel()
	loaded := loadClaudePackages(t)
	var covered []string
	for _, p := range loaded {
		if !strings.Contains(p.PkgPath, claudeTree) {
			t.Errorf("%s was loaded but is not under %s; the pattern and the "+
				"constant disagree", p.PkgPath, claudeTree)
		}
		if len(p.Syntax) == 0 {
			t.Errorf("%s has no parsed files -- the guard would pass "+
				"vacuously over it", p.PkgPath)
		}
		covered = append(covered, p.PkgPath)
	}
	t.Logf("boundary guard covers %d package(s): %s",
		len(covered), strings.Join(covered, ", "))
}
