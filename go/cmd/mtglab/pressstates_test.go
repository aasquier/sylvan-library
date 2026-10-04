package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The third clause of commandment 17: a button answers the press.
//
// # Which absolute this closes
//
// Commandment 17 names three replies -- "hover, focus and press all visibly
// reply" -- and until this file two of them were held by a guard and the third
// by nobody. `barebutton_test.go` refuses an undressed control;
// `focusstates_test.go` refuses a class whose `:hover` face has no
// `:focus-visible` face. Press had the same shape of fault available to it and
// nothing watching: a class with a designed hover and no `:active` takes the
// click in silence, and the hand that pressed it has to go looking at the rest
// of the page to find out whether anything happened.
//
// The sheet already argues this in its own voice, above `.chip-toggle:active`:
// there was no `:active` on that family at all, "so a chip took a click in
// complete silence -- on a toggle, where the only other feedback is a state
// change the eye has to go looking for." That argument was made for one family
// and never generalised. This is the generalisation.
//
// # Why a register and not a ban
//
// A ban would have been born red on fifteen classes, which is a design pass
// across the Coliseum's tab strips, the deck page's card actions and every
// disclosure in the app -- not a thing a guard can demand in the diff that
// introduces it. So this is `datedcomments_test.go`'s ratchet instead: the
// number may not rise, and a branch that lowers it re-types the constant in
// the same diff.
//
// The first lowering is the argument for the shape. Three classes took the
// house press reply -- `.card-action`, `.strip-tab`, `.disclosure-toggle` --
// and the register fell by *five*, because `.card-action-danger`, `.armed` and
// the rest ride on elements that now answer through a sibling. A ban on the
// fifteen would have asked for fifteen rules; the ratchet asked for three and
// got the other two free, which is the whole reason the unit below is a class
// and the verdict is per element.
//
// # Why the unit is a class and not a button
//
// Gating on button sites would turn "somebody added a tenth card action" into
// a red check about an omission somebody else made in the stylesheet, which is
// how a guard gets deleted in anger. The fault is one rule missing from one
// named place, so the count is of named places. The sites are printed anyway,
// because the fix wants to be looked at on a page.
//
// # What is left, and why it is not simply more of the same
//
// What the first lowering did not touch is the deliberately bespoke shelf --
// the art picker's tiles, the reader tiles, the wheel's folds, the tarot
// hinge. Those carry their own named classes on purpose, and whether a card
// tile should settle under a thumb the way a plate does is a design question
// rather than a missing rule. Read the ledger's Red section before taking one.
//
// # Why Go and not Vitest
//
// `focusstates_test.go` paid for this answer: its first draft was a Vitest
// test, and `import css from './index.css?raw'` hands Vitest an empty string,
// so every class read as an empty wardrobe and the guard reported almost
// nothing. The question is "what does this stylesheet say", and Go is where
// this package already reads it.
//
// # What it cannot see
//
// A press reply that is not `:active` -- a class toggled from JavaScript on
// pointerdown, or a child element's transform -- and an `:active` rule that
// exists and changes nothing visible. Like its two siblings it can miss and
// cannot invent: every class it names has a hover rule and no active rule in
// the one stylesheet, which a person with the file open can confirm.
//
// `pressSilentClassCeiling` is the register. **Lower it freely; raising it is
// a decision that belongs in the diff.**
const pressSilentClassCeiling = 10

func TestEveryButtonThatAnswersHoverAnswersThePress(t *testing.T) {
	t.Parallel()

	sheet := readIndexCSS(t)
	if !strings.Contains(sheet, ".chip-toggle:active") {
		t.Fatalf("web/src/index.css no longer contains `.chip-toggle:active`, " +
			"the house press reply this register measures against -- either the " +
			"rule went or the file did")
	}

	seen := 0
	var sites []string
	silent := map[string][]string{}
	root := repoRoot(t)
	src := filepath.Join(root, "web", "src")
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".tsx") || strings.HasSuffix(path, ".test.tsx") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for _, tag := range openingButtonTags(blankComments(string(body))) {
			seen++
			dull := pressSilence(tag.text, sheet)
			if len(dull) == 0 {
				continue
			}
			sites = append(sites, fmt.Sprintf("%s:%d  answers :hover through [%s] "+
				"and nothing it wears answers :active", rel, tag.line,
				strings.Join(dull, ", ")))
			for _, c := range dull {
				silent[c] = append(silent[c], rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", src, err)
	}

	// The anti-vacuity floor every reader in this package keeps: a sweep that
	// found almost no buttons has stopped measuring and would report a clean
	// bill for a tree it never opened.
	if seen < 100 {
		t.Fatalf("found only %d buttons under %s; this app has well over a "+
			"hundred and the parse has stopped working", seen, src)
	}

	classes := make([]string, 0, len(silent))
	for c := range silent {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	sort.Strings(sites)
	census := make([]string, 0, len(classes))
	for _, c := range classes {
		census = append(census, fmt.Sprintf("%-22s %d button(s)", c, len(silent[c])))
	}
	t.Logf("press census: %d buttons read, %d answering hover and not the "+
		"press, through %d class(es) (ceiling %d):\n  %s",
		seen, len(sites), len(classes), pressSilentClassCeiling,
		strings.Join(census, "\n  "))

	switch {
	case len(classes) > pressSilentClassCeiling:
		t.Errorf("%d class(es) answer :hover and not :active, ceiling is %d -- "+
			"%d more than the register allows:\n  %s\n\n"+
			"Commandment 17: hover, focus AND press. A control that changes on "+
			"hover and takes the click in silence has answered two thirds of "+
			"the hand. Give the class an `:active` rule beside its `:hover` one "+
			"in web/src/index.css -- `.chip-toggle:active`'s "+
			"`transform: translateY(1px)` is the house answer and the comment "+
			"above it is the argument -- and never an inline `style`, which a "+
			"`:active` can no more reach than a `:hover` can.",
			len(classes), pressSilentClassCeiling,
			len(classes)-pressSilentClassCeiling, strings.Join(sites, "\n  "))
	case len(classes) < pressSilentClassCeiling:
		t.Errorf("only %d class(es) now answer :hover without :active, and the "+
			"ceiling is still %d -- lower `pressSilentClassCeiling` to %d in "+
			"this file. A ratchet that is not re-typed when the sweep wins "+
			"stops being a ratchet: it leaves room for the next one to come "+
			"back unnoticed.", len(classes), pressSilentClassCeiling, len(classes))
	}
}

// pressSilence is the classes through which this tag answers the pointer's
// hover while nothing it wears answers the pointer's press, or nil when it has
// nothing to answer for. A button with no hover face at all is not this
// register's business -- the finding is *asymmetry*, as in
// [TestEveryControlThatAnswersHoverAnswersFocus], not plainness.
func pressSilence(tag, sheet string) []string {
	var hover []string
	for _, c := range classesOf(tag) {
		if classAnswers(c, "active", sheet) {
			return nil
		}
		if classAnswers(c, "hover", sheet) && !slices.Contains(hover, c) {
			hover = append(hover, c)
		}
	}
	return hover
}

// The reader's own test, on fixtures, so the register above cannot go quiet by
// misreading. `classesOf` and `classAnswers` are shared with the focus guard
// and pinned by its fixtures; what is pinned here is the press question asked
// over them -- in particular that a sibling class's `:active` answers for the
// whole element, which is how `.btn-primary` is covered by `.btn:active`.
func TestThePressReaderRecognisesEachShape(t *testing.T) {
	t.Parallel()

	sheet := ".dull:hover { color: red; }\n" +
		".lit:active { transform: translateY(1px); }\n" +
		".row:hover .row-art img { transform: none; }\n" +
		".note:active:not(:disabled) { top: 1px; }\n" +
		".pressed[aria-pressed='true']:hover { color: blue; }\n" +
		".quiet { color: red; }\n"

	for _, tc := range []struct {
		name, tag string
		want      []string
	}{
		{"hover with no press", `<button className="dull">`, []string{"dull"}},
		{"the press reply on a sibling class",
			`<button className="dull lit">`, nil},
		{"a class arriving in a template literal",
			"<button className={`dull${on ? ' lit' : ''}`}>", nil},
		{"a hover that styles a descendant still counts as this class's hover",
			`<button className="row">`, []string{"row"}},
		{"an attribute selector between the class and the pseudo-class",
			`<button className="pressed">`, []string{"pressed"}},
		{"a `:not()` after `:active` is still a press reply",
			`<button className="dull note">`, nil},
		{"no hover face at all is not this register's business",
			`<button className="quiet">`, nil},
		{"no class at all", `<button onClick={go}>`, nil},
		{"two dull classes are both named",
			`<button className="dull row">`, []string{"dull", "row"}},
	} {
		got := pressSilence(tc.tag, sheet)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: pressSilence = %v, want %v", tc.name, got, tc.want)
		}
	}
}
