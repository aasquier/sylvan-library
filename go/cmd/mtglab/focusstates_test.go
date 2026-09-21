package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// A control answers the keyboard's hand as well as the pointer's
// (commandment 17).
//
// # The clause that gets skipped
//
// Hover, focus *and* press all visibly reply -- and focus is the one a
// keyboard user actually has. Tab lands on a control and nothing will ever
// hover it, so a class with a `:hover` face and no `:focus-visible` face has
// answered the mouse and left the keyboard whatever ring the browser draws by
// default: on this site's dark rooms, a thin blue line the design never chose.
//
// # Two mechanisms, each a real silence
//
// **An inline `outline` on a control silences focus entirely.** The browser's
// own focus ring *is* an outline, and so is `.btn:focus-visible`'s vine, and
// an inline style outranks both. The art picker's printing tiles carried
// `style={{ outline: on ? … : '1px solid var(--hairline)' }}` to draw their
// selection ring, and the price was that Tab through the printings changed
// nothing on screen: the chosen tile stayed ringed and the focused one looked
// exactly like its neighbours. That is `ghostink_test.go`'s lesson -- an
// inline `color` that a `:hover` could never reach -- for the other
// pseudo-class.
//
// **A class that answers `:hover` and not `:focus-visible` is two-thirds
// done.** Ten of them were on buttons and links when this was written --
// `.card-action`, `.menu-row`, `.nav-link`, `.reader-tile`, `.wheel-spin-btn`
// and friends -- every one a named place in `index.css` where the hover reply
// was designed and the focus reply forgotten.
//
// # How it decides
//
// Every `<button>`, `<a>`, `<Link>` and `<NavLink>` under `web/src` is in
// scope, read with `barebutton_test.go`'s reader (quote- and brace-aware,
// comments blanked). For each, the question is answered off `index.css`
// itself: does any class the control wears carry a `:hover` rule, and if so
// does any class it wears carry a `:focus-visible` (or `:focus`, or
// `:focus-within`) rule? `.btn-primary` passes because the element also wears
// `.btn`, and `.btn:focus-visible` is the ring. A Tailwind `hover:` utility
// counts as a hover face and a `focus:` / `focus-visible:` utility as the
// answer, since `auth.tsx`'s inputs are dressed that way. A control with no
// hover face at all is not this guard's business -- the finding is
// *asymmetry*, not plainness.
//
// It reads the stylesheet's source rather than the bundle because the source
// is where a person will add the rule, and because Tailwind's build leaves
// the hand-written rules byte-identical anyway.
//
// # What it cannot see
//
// A focus reply living in a class the element does not wear, a hover face set
// from a stylesheet other than `index.css`, and a `:focus` rule that exists
// but changes nothing visible. It can miss; it cannot invent -- every tag it
// names has a hover rule and no focus rule in the one stylesheet, which a
// person with the file open can confirm in thirty seconds.
func TestEveryControlThatAnswersHoverAnswersFocus(t *testing.T) {
	t.Parallel()

	sheet := readIndexCSS(t)

	var silent []string
	seen := 0
	forEachControlTag(t, func(rel string, tag buttonTag) {
		seen++
		if why := focusSilence(tag.text, sheet); why != "" {
			silent = append(silent, fmt.Sprintf("%s:%d  %s", rel, tag.line, why))
		}
	})

	// The anti-vacuity floor every reader in this package keeps: a sweep that
	// found almost no controls has stopped measuring.
	if seen < 100 {
		t.Fatalf("found only %d controls under web/src; this app has well over "+
			"a hundred and the parse has stopped working", seen)
	}

	sort.Strings(silent)
	if len(silent) > 0 {
		t.Errorf("%d control(s) answer the pointer and not the keyboard:\n  %s\n\n"+
			"Commandment 17: every control answers the hand that reaches for "+
			"it, and a keyboard is a hand. Give the class a `:focus-visible` "+
			"rule beside its `:hover` one in web/src/index.css -- "+
			"`.btn:focus-visible`'s vine ring is the house answer, and "+
			"`.arena-gate` shows hover and focus sharing one reply. Never an "+
			"inline `outline`: it silences the ring the browser would "+
			"otherwise draw.", len(silent), strings.Join(silent, "\n  "))
	}
}

// The reader's own test, on fixtures, so the guard above cannot go quiet by
// misreading: each shape that decided the real findings is pinned here.
func TestTheFocusReaderRecognisesEachShape(t *testing.T) {
	t.Parallel()

	sheet := ".dull:hover { color: red; }\n" +
		".lit:focus-visible { outline: 0; }\n" +
		".row:hover .row-art img { transform: none; }\n" +
		".slot .note:hover,\n.slot .note:focus-visible { top: 0; }\n" +
		".pressed[aria-pressed='true']:hover { color: blue; }\n" +
		".box:hover { color: red; }\n.box:focus { color: red; }\n"

	for _, tc := range []struct {
		name, tag string
		silent    bool
	}{
		{"hover with no focus", `<button className="dull">`, true},
		{"the focus reply on a sibling class", `<button className="dull lit">`, false},
		{"a class arriving in a template literal",
			"<button className={`dull${on ? ' lit' : ''}`}>", false},
		{"a class arriving through concatenation",
			`<a className={'dull' + (isActive ? ' lit' : '')}>`, false},
		{"a hover that styles a descendant still counts as this class's hover",
			`<button className="row">`, true},
		{"a focus reply inside a descendant selector still counts",
			`<button className="note">`, false},
		{"an attribute selector between the class and the pseudo-class",
			`<button className="pressed">`, true},
		{"`:focus` is an answer, not only `:focus-visible`",
			`<button className="box">`, false},
		{"a Tailwind hover utility with nothing for focus",
			`<a className="hover:underline">`, true},
		{"a Tailwind hover utility answered by a Tailwind focus utility",
			`<a className="hover:underline focus-visible:ring-2">`, false},
		{"an inline outline silences even a dressed control",
			`<button className="lit" style={{ outline: 'none' }}>`, true},
		{"an inline outlineOffset is the same silence",
			`<button className="lit" style={{ outlineOffset: '1px' }}>`, true},
		{"no hover face at all is not this guard's business",
			`<button className="quiet">`, false},
		{"no class at all", `<button onClick={go}>`, false},
	} {
		got := focusSilence(tc.tag, sheet) != ""
		if got != tc.silent {
			t.Errorf("%s: silent=%v, want %v (%q)", tc.name, got, tc.silent,
				focusSilence(tc.tag, sheet))
		}
	}
}

var (
	controlOpen = regexp.MustCompile(`<(button|a|Link|NavLink)\b`)
	// A class token: how the stylesheet spells one, and how Tailwind does not
	// (its utilities carry `:` and `/` and `[`, which this does not match).
	classToken = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	// Tailwind's own state prefixes, at the start of a token.
	twHover = regexp.MustCompile("(^|[\\s\"'`])hover:")
	twFocus = regexp.MustCompile("(^|[\\s\"'`])focus(-visible|-within)?:")
	// `outline`, `outlineOffset`, `outlineColor`... as a key in the style
	// object. Read inside `style={{…}}` only, so a prop named `outline` on some
	// other attribute is not counted.
	inlineOutline = regexp.MustCompile(`\boutline(Offset|Color|Width|Style)?\s*:`)
)

// focusSilence is why this control answers the pointer and not the keyboard,
// or "" when it has nothing to answer for.
func focusSilence(tag, sheet string) string {
	if inlineOutline.MatchString(styleObject(tag)) {
		return "sets `outline` inline, which outranks every focus ring there is"
	}
	var hover []string
	focus := false
	for _, c := range classesOf(tag) {
		if classAnswers(c, "hover", sheet) && !slices.Contains(hover, c) {
			hover = append(hover, c)
		}
		if classAnswers(c, "focus(-visible|-within)?", sheet) {
			focus = true
		}
	}
	hasHover := len(hover) > 0 || twHover.MatchString(tag)
	hasFocus := focus || twFocus.MatchString(tag)
	if hasHover && !hasFocus {
		through := strings.Join(hover, ", ")
		if through == "" {
			through = "hover:"
		}
		return "answers :hover through [" + through +
			"] and nothing it wears answers :focus-visible"
	}
	return ""
}

// classAnswers reports whether `index.css` gives the class its own face for
// the pseudo-class -- attached to *this* compound, so `.reader-tile:hover img`
// says yes for `reader-tile` and `.slot .note:focus-visible` says yes for
// `note`. The lookahead keeps `.btn` from claiming `.btn-primary`'s rules.
func classAnswers(class, pseudo, sheet string) bool {
	re := regexp.MustCompile(`\.` + regexp.QuoteMeta(class) +
		`([^a-z0-9_\s{,-][^\s{,]*)?:` + pseudo + `\b`)
	return re.MatchString(sheet)
}

// classesOf is the class tokens a tag names, whatever shape its `className`
// takes. A string, a template literal and a `'nav-link' + (isActive ? '
// is-active' : ”)` expression all contribute their words -- which is also
// what the browser ends up with -- so the tokens are read as words rather
// than parsed as an expression. Identifiers with an uppercase letter are
// dropped by the token shape, and the odd lowercase variable that survives
// (`open`, `hut`) names no class in the stylesheet.
func classesOf(tag string) []string {
	value := attributeValue(tag, "className=")
	var out []string
	for _, w := range strings.FieldsFunc(value, func(r rune) bool {
		inWord := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || strings.ContainsRune("-_:/.[]%", r)
		return !inWord
	}) {
		if classToken.MatchString(w) {
			out = append(out, w)
		}
	}
	return out
}

// styleObject is the `{{…}}` after `style=`, or "" when the tag sets none.
func styleObject(tag string) string {
	return attributeValue(tag, "style=")
}

// attributeValue is the text of one JSX attribute's value: the inside of its
// quotes, or the brace-balanced expression after `=`.
func attributeValue(tag, attr string) string {
	at := strings.Index(tag, attr)
	if at < 0 {
		return ""
	}
	start := at + len(attr)
	if start >= len(tag) {
		return ""
	}
	switch tag[start] {
	case '"', '\'':
		end := strings.IndexByte(tag[start+1:], tag[start])
		if end < 0 {
			return tag[start+1:]
		}
		return tag[start+1 : start+1+end]
	case '{':
		depth := 0
		for i := start; i < len(tag); i++ {
			switch tag[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return tag[start : i+1]
				}
			}
		}
	}
	return ""
}

// forEachControlTag is forEachButtonTag for every element a hand reaches for.
func forEachControlTag(t *testing.T, fn func(rel string, tag buttonTag)) {
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
		for _, tag := range openingTags(blankComments(string(body)), controlOpen) {
			fn(rel, tag)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking web/src: %v", err)
	}
}

// readIndexCSS is the stylesheet's source, with a canary: a guard deriving
// "which classes are dressed" from an empty or wrong file finds an empty
// wardrobe and either fails for the wrong reason or passes forever.
func readIndexCSS(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "web", "src", "index.css")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	sheet := string(body)
	if !strings.Contains(sheet, ".btn:focus-visible") {
		t.Fatalf("%s no longer contains `.btn:focus-visible`, the house ring "+
			"this guard measures against -- the file moved or the ring did", path)
	}
	return sheet
}
