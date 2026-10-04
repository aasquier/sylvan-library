package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The inline shorthand that silences a reply nobody wrote inline.
//
// # The fault
//
// An inline `style` beats every rule in the stylesheet, which is the mechanism
// commandment 20 names for how a control ends up answering the hand with
// nothing. `barebutton_test.go`, `focusstates_test.go` and
// `pressstates_test.go` all read the *stylesheet* and so none of them can see
// it: the class has its `:hover` face, the guard is satisfied, and the element
// wearing the class never shows it.
//
// This file reads the half of that fault that is invisible even to the person
// writing it. Setting `color` inline over a `:hover` that sets `color` is at
// least legible as a collision -- the same word appears twice. Setting
// `border` inline over a `:hover` that sets `border-color` is not: the
// shorthand resets every longhand it contains, so an inline border written to
// say "one pixel, hairline" silently also says "and never change colour for
// anybody". Nothing in the element names the property that died.
//
// The specimen it was written from, measured on the deck page with a pointer
// actually resting on the control: the "Tag a pilot" button wore
// `.card-action` and an inline `border: 1px solid var(--hairline)` that
// duplicated what the class already drew. `.card-action:hover` sets
// `background` and `border-color`; through the whole hover the ground arrived
// and the edge stayed at `rgba(255, 255, 255, 0.1)`. Half a reply, from a
// declaration that changed nothing on purpose.
//
// # Why this is a ban and its three siblings are registers
//
// Because the tree holds none of them. The broader net -- any inline key over
// the same longhand -- finds four, and at least two are deliberate; that
// question is in the ledger, not in this file. The shorthand net was one, it
// is now zero, and a guard born green over an empty set is the cheap half of
// this: it costs nothing until somebody reintroduces the shape, and then it
// names the file, the line, the class and the property that stopped answering.
//
// # What it cannot see
//
// A shorthand in a `style` built outside the tag (a variable, a helper), a
// class applied through a variable rather than a literal `className`, and a
// stylesheet rule whose interactive face lives in a nested or media block this
// reader's flat scan splits differently. Like its siblings it can miss and
// cannot invent: every pair it reports is a literal inline key and a literal
// rule in the one stylesheet.
func TestNoInlineShorthandSilencesAnInteractiveReply(t *testing.T) {
	t.Parallel()

	sheet := readIndexCSS(t)
	answers := interactiveLonghands(sheet)
	if len(answers) < 20 {
		t.Fatalf("read interactive faces for only %d class(es) out of "+
			"web/src/index.css; this sheet dresses far more than that and the "+
			"rule scan has stopped working", len(answers))
	}

	root := repoRoot(t)
	src := filepath.Join(root, "web", "src")
	var (
		found []string
		tags  int
	)
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
		for _, tag := range openingTags(blankComments(string(body)), anyTagOpen) {
			style := styleObject(tag.text)
			if style == "" {
				continue
			}
			tags++
			for _, class := range classesOf(tag.text) {
				for longhand := range answers[class] {
					for _, short := range swallowedBy[longhand] {
						if !inlineSets(style, short) {
							continue
						}
						found = append(found, fmt.Sprintf(
							"%s:%d  .%s's interactive face sets `%s`, and this "+
								"tag sets the shorthand `%s` inline, which "+
								"resets it", rel, tag.line, class, longhand, short))
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", src, err)
	}

	// The anti-vacuity floor every reader in this package keeps.
	if tags < 100 {
		t.Fatalf("found only %d tag(s) carrying an inline style under %s; this "+
			"tree holds hundreds and the parse has stopped working", tags, src)
	}
	sort.Strings(found)
	t.Logf("inline-shorthand sweep: %d tag(s) with an inline style read against "+
		"%d dressed class(es); %d collision(s)", tags, len(answers), len(found))
	if len(found) > 0 {
		t.Errorf("%d inline shorthand(s) reset a property the stylesheet uses "+
			"to answer the hand:\n  %s\n\nAn inline style outranks every rule "+
			"in index.css, and a shorthand resets every longhand it contains -- "+
			"so this declaration also turns off a hover, press or focus reply "+
			"that nothing here mentions. Drop the inline declaration when the "+
			"class already draws it, or move the value into the class.",
			len(found), strings.Join(found, "\n  "))
	}
}

// anyTagOpen matches any JSX opening tag -- an inline style is not the
// preserve of `<button>`, and neither is wearing a dressed class.
var anyTagOpen = regexp.MustCompile(`<[A-Za-z][A-Za-z0-9]*\b`)

// swallowedBy is the longhands this guard watches and the inline keys that
// reset them. JSX spells a style key in camelCase, which is why the values are
// not simply the CSS shorthand names. Deliberately short: these are the four
// properties `index.css` actually changes on `:hover`, `:active` and
// `:focus-visible`, and a table that listed every shorthand in CSS would be a
// claim about the sheet rather than a reading of it.
var swallowedBy = map[string][]string{
	"border-color":        {"border"},
	"border-width":        {"border"},
	"border-top-color":    {"border", "borderTop"},
	"border-bottom-color": {"border", "borderBottom"},
	"border-left-color":   {"border", "borderLeft"},
	"border-right-color":  {"border", "borderRight"},
	"background-color":    {"background"},
	"background-image":    {"background"},
	"outline-color":       {"outline"},
	"outline-width":       {"outline"},
}

// interactiveLonghands is, per class, the longhand properties the stylesheet
// declares for that class under `:hover`, `:active` or `:focus-visible` -- the
// three replies commandment 17 asks for. The scan is flat over `selector {
// body }` pairs because that is all this question needs: a property is
// interesting when it appears in a rule whose selector mentions one of the
// three, wherever that rule happens to sit.
func interactiveLonghands(sheet string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, m := range cssRule.FindAllStringSubmatch(sheet, -1) {
		selector, body := m[1], m[2]
		if !strings.Contains(selector, ":hover") &&
			!strings.Contains(selector, ":active") &&
			!strings.Contains(selector, ":focus-visible") {
			continue
		}
		props := cssProperty.FindAllStringSubmatch(body, -1)
		if len(props) == 0 {
			continue
		}
		for _, c := range cssClass.FindAllStringSubmatch(selector, -1) {
			for _, p := range props {
				if out[c[1]] == nil {
					out[c[1]] = map[string]bool{}
				}
				out[c[1]][p[1]] = true
			}
		}
	}
	return out
}

// inlineSets reports whether a JSX style object names this key at the start of
// a declaration. The leading boundary is what keeps `border` from matching
// inside `borderColor`.
func inlineSets(style, key string) bool {
	re := regexp.MustCompile(`(^|[\s,{])` + regexp.QuoteMeta(key) + `\s*:`)
	return re.MatchString(style)
}

// `cssRule` and `cssClass` are the reduced-motion sweep's, shared on purpose:
// two readers of the same stylesheet that disagree about what a rule is would
// be two different claims about the file.
var cssProperty = regexp.MustCompile(`(?m)(?:^|[;{])\s*([a-z-]+)\s*:`)

// The reader's own test, on fixtures, so the ban above cannot go quiet by
// misreading -- the failure mode of a guard with an empty finding set is that
// it has stopped finding rather than that there is nothing to find.
func TestTheInlineShorthandReaderRecognisesEachShape(t *testing.T) {
	t.Parallel()

	sheet := ".plate { border: 1px solid grey; }\n" +
		".plate:hover { border-color: green; background: black; }\n" +
		".tab:active { background-color: blue; }\n" +
		".ring:focus-visible { outline-color: red; }\n" +
		".quiet { color: grey; }\n"
	answers := interactiveLonghands(sheet)

	if !answers["plate"]["border-color"] || !answers["plate"]["background"] {
		t.Errorf("a :hover rule's properties did not reach its class: %v",
			answers["plate"])
	}
	if answers["quiet"] != nil {
		t.Errorf("a rule with no interactive pseudo-class contributed anyway: %v",
			answers["quiet"])
	}
	if !answers["tab"]["background-color"] || !answers["ring"]["outline-color"] {
		t.Errorf(":active and :focus-visible must count too: %v", answers)
	}

	for _, tc := range []struct {
		name  string
		style string
		key   string
		want  bool
	}{
		{"the shorthand itself", `{ border: '1px solid red' }`, "border", true},
		{"a longhand is not its shorthand",
			`{ borderColor: 'red' }`, "border", false},
		{"second in the object", `{ color: 'red', border: '0' }`, "border", true},
		{"across a line break", "{ color: 'red',\n  background: 'red' }",
			"background", true},
		{"a value mentioning the word",
			`{ boxShadow: '0 0 0 1px border' }`, "border", false},
	} {
		if got := inlineSets(tc.style, tc.key); got != tc.want {
			t.Errorf("%s: inlineSets(%q, %q) = %v, want %v",
				tc.name, tc.style, tc.key, got, tc.want)
		}
	}

	if _, ok := swallowedBy["border-color"]; !ok {
		t.Error("the shorthand table lost `border-color`, which is the " +
			"collision this guard was written from")
	}
}
