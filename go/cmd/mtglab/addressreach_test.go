package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Where an email address is allowed to be read, made checkable.
//
// CLAUDE.md's fifth non-negotiable says an address "may be serialised only
// into a response an admin authenticated for", and `internal/api/admin.go`
// says the same thing in its own words and then adds the sentence this file
// exists for: *"no other module may acquire the habit."* Until now the only
// thing holding either was a grep — `.claude/skills/polish/references/white.md`
// instructs every White run to "grep for every call site each run — a third
// one is the finding", and four consecutive runs duly reported one non-test
// caller. A rule re-checked by whoever remembers to re-check it is a rule that
// has already drifted or is about to, and the fix for that is never to reword
// the sentence.
//
// Two sweeps, because the claim has two halves and they need different proofs.
//
// **The register** is the narrow half: every place in the tree that reads an
// address at all. It is small on purpose — seven functions — and an eighth is
// the finding, exactly as the checklist says. The register carries the reason
// each one is allowed, so the day somebody adds an eighth they are answering
// an argument rather than editing a list.
//
// **The reachability rule** is the half a register cannot state. Inside
// `internal/api` an address is not read by a handler; it is read by
// `accountBody`, three routes deep, and the thing that makes those routes safe
// is `requireAdmin` at the top of each. A register would keep saying
// "accountBody" while a new non-admin route quietly called it. So the second
// sweep computes what can reach the address through the package's own call
// graph and requires every way *in* to check for an admin first. That is the
// mechanism admin.go names, held rather than described.

// addressReaders is every function in the tree that may read an account's
// address, and why. Keyed `<package dir>:<func>`; a method is
// `Receiver.Method`.
//
// The list came from the grep the checklist prescribes and the sweep then held
// it equal in both directions, which is the point: it is not a description of
// the tree any more, it is a constraint on it. Read it as seven answers to one
// question — who has a reason to see somebody's address?
var addressReaders = map[string]string{
	// The package that owns the field. `AsDict` is the withholding itself —
	// the address is serialised here or nowhere — and the scan is how the
	// column becomes the field in the first place.
	"internal/auth:User.AsDict": "the withholding: the one place an address becomes JSON, and only when asked",
	"internal/auth:scanUser":    "the read out of the column into the field",
	// The mailers. An address is what a letter is sent to; these two are the
	// reason the column exists at all.
	"internal/auth:SendInvite": "addressing the invitation",
	"internal/auth:SendReset":  "addressing the reset link",

	// Outside `internal/auth`, exactly two, and they are the two ADR 17 argues.
	"cmd/mtglab:usersListCommand": "an operator's own terminal, never a response (docs/HOSTING.md's `who can sign in`)",
	"internal/api:accountBody":    "ADR 17: the admin page sees addresses — held by the reachability sweep below",
	"internal/api:API.sendAccountReset": "ADR 16: an admin may cause the letter to be sent, " +
		"not choose the password — held by the reachability sweep below",
}

// The register: the tree holds exactly these address readers.
func TestOnlyTheArguedFunctionsReadAnAddress(t *testing.T) {
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
		// Tests are outside the rule: a fixture may hold an address, and one
		// of them has to in order to prove the withholding works.
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
			for _, at := range addressTouches(fn) {
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
	// with no address in it, which is the one answer this test must never
	// give quietly.
	if len(found) < 4 {
		t.Fatalf("only %d address readers found in the whole tree; the sweep is "+
			"looking at the wrong thing (the withholding and both mailers are "+
			"always there)", len(found))
	}

	for key, where := range found {
		if _, argued := addressReaders[key]; !argued {
			t.Errorf("%s reads an account's address (%s) and no argument says why.\n"+
				"CLAUDE.md rule 5: an address may be serialised only into a response "+
				"an admin authenticated for, and internal/api/admin.go adds that no "+
				"other module may acquire the habit. If this one has a reason, put "+
				"it in `addressReaders` above; if it does not, the address does not "+
				"belong here.", key, strings.Join(where, ", "))
		}
	}
	for key, why := range addressReaders {
		if _, still := found[key]; !still {
			t.Errorf("%s is argued as an address reader (%q) and reads no address "+
				"any more. Drop the entry -- a register that names functions which "+
				"stopped mattering reads wider than the tree actually is.", key, why)
		}
	}
	if !t.Failed() {
		t.Logf("%d address readers, every one argued", len(found))
	}
}

// The reachability rule: inside `internal/api`, every way in to the address
// checks for an admin first.
//
// The call graph is the package's own, built by name. Two functions in one
// package with the same name would merge, which widens the closure rather than
// narrowing it -- the safe direction for a guard, and the reason this is
// allowed to be a name rather than a resolved symbol.
//
// An **entry point** is a member of the closure that nothing else in the
// package calls, so the only thing that can call it is the router (or a
// package outside). Those are the ones that must check, and the check is
// `requireAdmin` -- which is itself the second of two mechanisms, the first
// being the door refusing `/api/admin/` by prefix before routing (ADR 17).
// This test holds the inner one; `internal/door`'s own sweeps hold the outer.
//
// The one shape this cannot see, said plainly rather than engineered around: a
// **cycle** of unchecked functions that nothing outside the cycle calls would
// have every member "called from inside the package" and so escape. Handlers
// do not call each other in circles, and the day one does the router has no way
// to reach it either -- but if this ever needs strengthening, that is where.
func TestEveryRouteThatCanReachAnAddressChecksForAnAdmin(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot(t), "go", "internal", "api")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	calls := map[string]map[string]bool{} // caller -> callees
	touches := map[string]bool{}          // reads an address directly
	checksAdmin := map[string]bool{}      // calls requireAdmin
	calledBy := map[string]map[string]bool{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			self := fn.Name.Name
			if len(addressTouches(fn)) > 0 {
				touches[self] = true
			}
			if calls[self] == nil {
				calls[self] = map[string]bool{}
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := ""
				switch f := call.Fun.(type) {
				case *ast.Ident:
					callee = f.Name
				case *ast.SelectorExpr:
					callee = f.Sel.Name
				}
				if callee == "" {
					return true
				}
				if callee == "requireAdmin" {
					checksAdmin[self] = true
				}
				calls[self][callee] = true
				if calledBy[callee] == nil {
					calledBy[callee] = map[string]bool{}
				}
				calledBy[callee][self] = true
				return true
			})
		}
	}

	if !touches["accountBody"] {
		t.Fatal("accountBody does not read an address, so this sweep is not " +
			"reading internal/api -- or the admin page stopped showing addresses, " +
			"which is ADR 17 changing and wants a ruling rather than a green test")
	}

	// The closure: seeded with the direct readers, grown by callers until it
	// stops growing.
	closure := map[string]bool{}
	for fn := range touches {
		closure[fn] = true
	}
	for grew := true; grew; {
		grew = false
		for caller, callees := range calls {
			if closure[caller] {
				continue
			}
			for callee := range callees {
				// Only functions this package defines are in the graph; a
				// callee nothing here declares is somebody else's.
				if closure[callee] && calls[callee] != nil {
					closure[caller] = true
					grew = true
					break
				}
			}
		}
	}

	var unguarded []string
	for fn := range closure {
		if checksAdmin[fn] {
			continue
		}
		// Not an entry point: something inside the package calls it, and that
		// caller is in the closure too (by construction), so the check lives
		// further out.
		insideCaller := false
		for caller := range calledBy[fn] {
			if calls[caller] != nil && caller != fn {
				insideCaller = true
				break
			}
		}
		if !insideCaller {
			unguarded = append(unguarded, fn)
		}
	}
	sort.Strings(unguarded)
	for _, fn := range unguarded {
		t.Errorf("internal/api.%s can reach an account's email address and "+
			"nothing between the router and the address calls requireAdmin. "+
			"ADR 17 allows addresses out of the admin surface and nowhere else; "+
			"either this route is an admin route and wants the check, or it must "+
			"not reach accountBody.", fn)
	}
	if !t.Failed() {
		t.Logf("%d functions in internal/api can reach an address; every way in "+
			"checks for an admin", len(closure))
	}
}

// addressTouches is every position in this function that reads an account's
// address: a selector named `Email`, or an `AsDict(true)`.
//
// `Email` is matched by name rather than by resolved type, deliberately. A
// struct field on some unrelated type called `Email` would be caught too and
// would have to be argued in the register — which is the right cost for a
// guard about addresses, and cheaper than loading types for the whole tree
// (the three `packages.Load` tests in `internal/claude` cost 28 seconds each).
func addressTouches(fn *ast.FuncDecl) []token.Pos {
	var at []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if node.Sel.Name == "Email" {
				at = append(at, node.Sel.Pos())
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "AsDict" || len(node.Args) != 1 {
				return true
			}
			if id, ok := node.Args[0].(*ast.Ident); ok && id.Name == "true" {
				at = append(at, node.Lparen)
			}
		}
		return true
	})
	return at
}

// funcKey is `Method` for a plain function and `Receiver.Method` for a method,
// with the pointer star dropped: what a reader would write in prose.
func funcKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if id, ok := recv.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
