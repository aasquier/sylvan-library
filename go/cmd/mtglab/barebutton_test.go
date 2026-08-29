package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Thou shalt not make a simple button (commandment 17).
//
// # Why a test and not a reading
//
// A `<button>` with no `className` gets the browser's own control: a grey
// bevelled box in the operating system's font that answers a hand with the
// operating system's idea of an answer, in the middle of a page built out of
// brass, felt and card stock. Commandment 17 names it directly -- *"a bare
// unstyled `<button>` in a route is a bug"* -- and the `.btn` family in
// `web/src/index.css` is the rule in code.
//
// **This exists because the reading is genuinely hard and was got wrong.** On
// 2026-08-29 a sweep for bare buttons across `web/src` reported exactly one:
// `components/hint.tsx:64`. Line 64 of that file is a *docstring* -- the
// sentence "The trigger is a real `<button>` rather than a `<span tabindex>`",
// with the tag in backticks -- and the component's actual button, sixty lines
// further down, has worn `field-hint` since the day it was written. The scan
// had counted a comment. Meanwhile a naive `grep '<button'` over the same tree
// reports a hundred and forty false positives in the other direction, because
// almost every button in this app opens its tag on one line and carries its
// `className` on the next.
//
// So: neither grep answers this, and the answer changes every time somebody
// adds a control. That is the shape of a thing a machine should be doing.
//
// # How it reads
//
// Comments are blanked first -- to spaces, so line numbers survive -- and then
// each `<button` is walked forward to the `>` that closes its opening tag,
// counting `{}` nesting and skipping quoted strings, because a `className`
// three lines down inside a template literal is still this button's class.
//
// A spread (`{...props}`) counts as dressed: the class arrives from the caller
// and the tag cannot know. That is deliberately generous -- this is a floor
// against the naked case, not an audit of whether every control is beautiful.
//
// `*.test.tsx` is exempt, and only that. A fixture button inside a test file
// is scaffolding for an assertion; nobody looks at it, and dressing it would
// be dressing a stage prop.
func TestNoRouteShipsABareButton(t *testing.T) {
	t.Parallel()

	var bare []string
	src := filepath.Join(repoRoot(t), "web", "src")
	seen := 0
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		if strings.HasSuffix(path, ".test.tsx") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repoRoot(t), path)
		if err != nil {
			rel = path
		}
		for _, tag := range openingButtonTags(blankComments(string(body))) {
			seen++
			if strings.Contains(tag.text, "className") ||
				strings.Contains(tag.text, "{...") {
				continue
			}
			bare = append(bare, rel+":"+strconv.Itoa(tag.line)+"  <button"+
				collapse(tag.text))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk %s: %v", src, err)
	}

	// The anti-vacuity floor every reader in this package keeps: a sweep that
	// found no buttons at all has stopped measuring, and would report a clean
	// bill for a tree it never opened.
	if seen < 100 {
		t.Fatalf("found only %d buttons under %s; this app has well over a "+
			"hundred and the parse has stopped working", seen, src)
	}

	sort.Strings(bare)
	if len(bare) > 0 {
		t.Errorf("%d button(s) ship with no class of their own:\n  %s\n\n"+
			"Commandment 17: a bare `<button>` wears the operating system's "+
			"grey box in a room built out of brass and card stock, and it "+
			"answers a hand with the operating system's idea of an answer. "+
			"Give it `.btn` (with `.btn-quiet`, `.btn-sm` and the rest) for an "+
			"action, or `.chip-toggle` / `.strip-tab` and their siblings for a "+
			"control that is a place rather than an action -- and an inline "+
			"`style` is not a substitute, because a `:hover` can never reach "+
			"one.", len(bare), strings.Join(bare, "\n  "))
	}
}

// The parse's own test. It is the half that can be silently wrong -- a reader
// that finds nothing passes -- so the cases that broke the hand-written sweeps
// are pinned here, in both directions.
func TestTheButtonReaderCanTellAControlFromAComment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		src  string
		bare bool
	}{
		{"a naked button", `<button onClick={go}>Go</button>`, true},
		{"dressed on the same line",
			`<button className="btn" onClick={go}>Go</button>`, false},
		{"dressed on a later line, which is how this app writes them",
			"<button type=\"button\"\n        onClick={go}\n" +
				"        className=\"btn btn-sm\">Go</button>", false},
		{"a block comment that mentions the tag -- the false positive that " +
			"put this file here",
			"/**\n * The trigger is a real `<button>` rather than a span.\n */\n" +
				`<button className="field-hint">x</button>`, false},
		{"a line comment that mentions the tag",
			"// a bare <button> would be a bug\n" +
				`<button className="btn">x</button>`, false},
		{"a class arriving through a spread",
			`<button type="button" {...rest}>Go</button>`, false},
		{"a `>` inside an expression does not end the tag early",
			"<button onClick={() => n > 3 && go()}\n" +
				"        className=\"btn\">Go</button>", false},
		{"an arrow function before the class, on one line",
			`<button onClick={() => go()} className="btn">Go</button>`, false},
		{"a `>` inside a string does not end the tag early",
			"<button aria-label=\"a > b\"\n        className=\"btn\">x</button>", false},
		{"self-closing, naked", `<button aria-label="x" />`, true},
		{"a component named Button is not a button",
			`<Button onClick={go}>Go</Button>`, false},
	} {
		tags := openingButtonTags(blankComments(tc.src))
		got := false
		for _, tag := range tags {
			if !strings.Contains(tag.text, "className") &&
				!strings.Contains(tag.text, "{...") {
				got = true
			}
		}
		if got != tc.bare {
			t.Errorf("%s: reader says bare=%v, want %v (tags: %d)",
				tc.name, got, tc.bare, len(tags))
		}
	}
}

type buttonTag struct {
	line int
	text string
}

// blankComments replaces every block and line comment with spaces, keeping
// newlines so reported line numbers still point at the right place. Spaces
// rather than deletion for the same reason.
func blankComments(src string) string {
	blank := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r == '\n' {
				b.WriteRune('\n')
			} else {
				b.WriteRune(' ')
			}
		}
		return b.String()
	}
	src = blockComment.ReplaceAllStringFunc(src, blank)
	return lineComment.ReplaceAllStringFunc(src, blank)
}

var (
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComment  = regexp.MustCompile(`//[^\n]*`)
	buttonOpen   = regexp.MustCompile(`<button\b`)
)

// openingButtonTags is every `<button ...>` opening tag, as the text between
// the tag name and the `>` that closes it. `{}` nesting and quoting are
// tracked because a JSX attribute is arbitrary TypeScript: `onClick={() => n >
// 3}` contains a `>` that does not end anything, and so does `aria-label="a >
// b"`.
func openingButtonTags(src string) []buttonTag {
	var out []buttonTag
	for _, loc := range buttonOpen.FindAllStringIndex(src, -1) {
		var (
			b     strings.Builder
			depth int
			quote byte
			prev  byte
		)
	tag:
		for i := loc[1]; i < len(src); i++ {
			c := src[i]
			b.WriteByte(c)
			if quote != 0 {
				if c == quote && prev != '\\' {
					quote = 0
				}
				prev = c
				continue
			}
			switch c {
			case '"', '\'', '`':
				quote = c
			case '{':
				depth++
			case '}':
				depth--
			case '>':
				if depth == 0 {
					break tag
				}
			}
			prev = c
		}
		out = append(out, buttonTag{
			line: 1 + strings.Count(src[:loc[0]], "\n"),
			text: b.String(),
		})
	}
	return out
}

func collapse(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}
