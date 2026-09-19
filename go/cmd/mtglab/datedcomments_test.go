package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The dated-comment ratchet.
//
// A comment that carries a date is usually the right thing to write: it says
// *when* something was true, which is the honest way to record a measurement
// or a ruling. The trouble is that it is only ever added. The polish pass
// retires roughly twenty of these a cycle by reading them and asking whether
// the date still earns its place, and the tree adds them faster than that, so
// the sweep was losing by arithmetic and becoming ritual.
//
// This is the coverage-floor pattern applied to prose residue: the count may
// not rise. It is deliberately NOT a ban. A session that has a real reason to
// write a dated comment raises the ceiling in the same diff, which turns an
// invisible drift into one line a reviewer can see and argue with.
//
// **Deciding whether to raise it.** Raise the ceiling when the date is the
// fact -- "measured 2026-09-13", "Aaron ruled on 2026-08-28", "lapsed
// 2026-09-12" -- because a reader who cannot see when it was measured cannot
// tell a fresh number from a rotted one. Do NOT raise it for a date that is
// merely when somebody happened to be typing; that belongs in the commit,
// which already records it, and `git blame` answers it better than a comment
// ever will. If you are raising it by more than two or three in one branch,
// that is the signal to retire some instead.
//
// **Why the counts are pinned here rather than measured by a sweep.** Before
// this test, every session that counted wrote its own regex, and no two
// agreed: one morning's ledger said 183 dated lines in `go/` and 368 in
// `web/src`, while three defensible sweeps of the same tree the same day
// returned 114, 154 and 479 for the first and 340, 410 and 563 for the
// second. A number that changes with the person asking cannot ratchet. So the
// definition lives in code: for Go it is the language's own comment scanner,
// which cannot mistake a URL in a string for a comment; for the frontend it
// is a comment-led line, which is the conservatism described at
// [webCommentLine].

// datedComment is an ISO date anywhere in the comment text. Deliberately not
// anchored: "// superseded 2026-09-06 by ADR 46" and "/* 2026-08-24: */" are
// both the thing being counted.
var datedComment = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// webCommentLine matches a line whose first non-space characters open or
// continue a comment. This is the frontend half's one deliberate
// conservatism, and it runs the safe way: a dated comment sitting at the end
// of a line of code is missed, while a date inside a string literal -- a
// Scryfall image URL, an ISO timestamp in a fixture -- can never be counted
// as prose. A guard that miscounts upward gets deleted the first time it
// fails for the wrong reason.
var webCommentLine = regexp.MustCompile(`^\s*(//|/\*|\*)`)

// ceilings is the ratchet itself. Measured 2026-09-13 on the branch that
// walked the volume restore drill. Lower these freely; raising one is a
// decision that belongs in the diff, argued by the doc comment above.
var ceilings = map[string]int{
	"go":      goDatedCommentCeiling,
	"web/src": webDatedCommentCeiling,
}

const (
	// 114 → 115 on the branch that landed the pool rebuild. The one added
	// comment dates the measurement that argues for the whole change ("On
	// 2026-09-13 the served pool was 261,894,144 bytes"), and a reader who
	// cannot see when that was measured cannot tell a live number from a
	// rotted one -- which is the doc comment's own test for keeping a date.
	// This is what raising the ceiling is supposed to look like: one line, in
	// the diff, with the reason beside it.
	// 114 → 117 over 2026-09-13/14, one at a time across three branches: the
	// pool-bloat measurement, the two-days-seventeen-days-apart measurement,
	// and the count of `transform` cards the analyzer was mis-reading. All
	// three are counts taken against a pool that keeps growing, so a reader
	// who cannot see when they were taken cannot tell a live number from a
	// rotted one — which is this file's own test for keeping a date.
	//
	// **Worth saying plainly, though: that day added three and retired none.**
	// The ratchet is doing its job — every one of those was a visible, argued
	// line rather than drift — but a ceiling that only ever rises is the
	// sweep's arithmetic problem in a new costume. The next Colorless run
	// should spend its slice retiring dated comments rather than counting
	// them, and lower this by more than it raises.
	goDatedCommentCeiling = 117
	// 292 → 259 on the branch that swept the Coliseum board family
	// (`web/src/components/board.tsx`): every date there sat on one of
	// Aaron's rulings, whose argument is the sentence and not the day, so all
	// of them went and no ceiling was raised beside them. This is what the
	// fall side is for -- the test failed on the sweep before this line moved.
	webDatedCommentCeiling = 259
)

// slack is the ratchet's give, and it is the difference between a gate and a
// tripwire. A ceiling that must be re-typed every time a branch happens to
// delete a dated comment fails for the wrong reason constantly, and a guard
// that cries wolf gets deleted -- so a tree that has drifted a little under
// its ceiling is simply passing. Only a *material* gain is banked, because
// only a material gain was somebody actually doing the sweep. The rise side
// has no slack at all: one comment over is one comment over.
const slack = 10

func TestDatedCommentsDoNotOutgrowTheirCeiling(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	counts := map[string]int{
		"go":      countGoDatedComments(t, filepath.Join(root, "go")),
		"web/src": countWebDatedComments(t, filepath.Join(root, "web", "src")),
	}

	for tree, ceiling := range ceilings {
		got := counts[tree]
		switch {
		case got > ceiling:
			t.Errorf("%s carries %d dated comments outside tests, ceiling is %d.\n"+
				"Either retire %d of them, or -- if the date is the fact rather than "+
				"the day you typed it -- raise the ceiling in this file's const block "+
				"and say why in the commit. The doc comment above argues which.",
				tree, got, ceiling, got-ceiling)
		case got < ceiling-slack:
			t.Errorf("%s carries %d dated comments outside tests, ceiling is %d -- "+
				"lower the ceiling to %d in this file's const block. A ratchet that "+
				"is not tightened when the tree improves has given back the ground "+
				"the sweep just won.", tree, got, ceiling, got)
		case got < ceiling:
			t.Logf("%s is %d under its ceiling of %d; within the slack, so this is "+
				"passing rather than owed. Tighten it when the sweep next banks a "+
				"material gain.", tree, ceiling-got, ceiling)
		}
	}
}

// countGoDatedComments uses the language's own scanner, so a date inside a
// string or an identifier cannot be mistaken for commentary. Test files are
// out of scope: a date in a test is usually naming the day a bug was caught,
// which is evidence rather than residue.
func countGoDatedComments(t *testing.T, dir string) int {
	t.Helper()

	total := 0
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, group := range file.Comments {
			for _, comment := range group.List {
				for _, line := range strings.Split(comment.Text, "\n") {
					if datedComment.MatchString(line) {
						total++
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return total
}

// countWebDatedComments reads lines rather than parsing, because there is no
// TypeScript parser in this module and pulling one in to count comments would
// cost more than the residue does. [webCommentLine] carries the conservatism.
func countWebDatedComments(t *testing.T, dir string) int {
	t.Helper()

	total := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !webSource(path) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(body), "\n") {
			if webCommentLine.MatchString(line) && datedComment.MatchString(line) {
				total++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return total
}

func skipDir(name string) bool {
	switch name {
	case "testdata", "node_modules", "vendor", ".git":
		return true
	}
	return false
}

// webSource is the frontend's half of "outside tests" -- the same exclusion
// the Go half makes with `_test.go`.
func webSource(path string) bool {
	base := filepath.Base(path)
	switch filepath.Ext(base) {
	case ".ts", ".tsx", ".css":
	default:
		return false
	}
	return !strings.Contains(base, ".test.") && !strings.Contains(base, ".spec.")
}
