package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Commandment 20 in code, in two halves: a control that changes this page is
// a button that answers the hand, and a disclosure says which state it is in.
//
// **Why a test and not a note.** `.btn-ghost` is the family's link-shaped
// voice -- underlined by design -- and until 2026-08-29 its entire hover was
// `color: var(--text-primary)`. An inline `style={{ color }}` outranks a
// stylesheet, so four controls answered a pointer with *nothing*: a blue
// "Tell me more" on the library shelf, a blue **"Save"** in the deck header,
// the import's own show/hide, and a tab switch mid-sentence. Not one of them
// looks broken in a diff; every one was dead under the hand, and the two
// disclosures among them never said whether they were open.
//
// The hover now also lays a wash and thickens the underline -- neither of
// which an ink can override -- so the reply survives a forgotten inline
// colour. These tests stop the *cause* returning anyway, because ink that is
// inline is ink no theme can move either.
//
// Both reuse `barebutton_test.go`'s reader rather than carrying a second one:
// it is quote- and brace-aware and it blanks comments, which is what stopped
// a docstring that merely *mentions* `<button>` being counted as markup.
var ghostInlineInk = regexp.MustCompile(`style=\{\{[^}]*\bcolor\s*:`)

func TestNoGhostControlSetsItsInkInline(t *testing.T) {
	t.Parallel()
	var offenders []string
	forEachButtonTag(t, func(rel string, tag buttonTag) {
		if !strings.Contains(tag.text, "btn-ghost") {
			return
		}
		if ghostInlineInk.MatchString(tag.text) {
			offenders = append(offenders, fmt.Sprintf("%s:%d", rel, tag.line))
		}
	})
	if len(offenders) > 0 {
		t.Errorf("a `:hover` cannot reach an inline style, so these "+
			".btn-ghost controls answer the hand with nothing -- wear "+
			".btn-ghost-accent instead (commandment 20):\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// A disclosure declares itself. Asked only of the controls whose handler is
// visibly a flip, because "this reveals something" is not a fact a parser can
// settle: a one-way reveal (`+ Add a note`) and a command across many groups
// (`Fold all`) are neither of them disclosures, and demanding `aria-expanded`
// of those would be wrong in a way nobody could act on.
var flipsADisclosure = regexp.MustCompile(`set(Open|Expanded|Folded|Show[A-Z]\w*)\(\s*\(?\w*\)?\s*=>\s*!`)

func TestADisclosureSaysWhetherItIsOpen(t *testing.T) {
	t.Parallel()
	var mute []string
	forEachButtonTag(t, func(rel string, tag buttonTag) {
		if !flipsADisclosure.MatchString(tag.text) {
			return
		}
		if strings.Contains(tag.text, "aria-expanded") ||
			strings.Contains(tag.text, "aria-pressed") {
			return
		}
		mute = append(mute, fmt.Sprintf("%s:%d", rel, tag.line))
	})
	if len(mute) > 0 {
		t.Errorf("these controls flip a disclosure without saying so; a "+
			"screen reader is told a button, never whether it is open -- add "+
			"aria-expanded (commandment 20):\n  %s", strings.Join(mute, "\n  "))
	}
}

// forEachButtonTag hands every opening `<button>` tag under `web/src` to fn,
// comments already blanked, with a repo-relative path for the message.
func forEachButtonTag(t *testing.T, fn func(rel string, tag buttonTag)) {
	t.Helper()
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
			fn(rel, tag)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking web/src: %v", err)
	}
}
