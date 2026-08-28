package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A card held up beside the cursor has to land on the window, and a transform
// two elements up is enough to take the window away from it.
//
// # The bug this was written for
//
// The token shelf's plates deal in (`@keyframes token-deal`, commandment 6),
// and the rule filled that animation `both`. A forwards fill leaves the last
// keyframe applied for ever -- and `token-deal` ends at `transform: none`,
// which a browser resolves to the **identity matrix** rather than to nothing
// at all. Per the CSS Transforms spec any computed transform other than
// `none` makes the element a containing block for its `position: fixed`
// descendants, so the preview a hovered token face raises was placed against
// the 340px plate instead of against the window. Measured in a real browser
// on 2026-08-28: the preview landed at (122, 1058) in an 814px tall window --
// entirely off the bottom of the screen -- and was squeezed from 230px wide to
// 170px on the way. The fix was one word, `both` -> `backwards`, which fills
// only the stagger's delay and is the half that was ever wanted.
//
// **Nothing in either suite could see it.** jsdom lays nothing out, so all
// 1,156 web tests were green over a preview drawn off-screen; and a Go test
// that only reads `filter` (`cardimagery_test.go`) has no opinion about
// `transform`. It took driving the page. This is the pin so the next session
// does not have to.
//
// # Why a curated list rather than a sweep
//
// Thirteen other rules in the bundle have the same shape -- a `both` fill onto
// keyframes that touch `transform` -- and **most of them are perfectly fine**,
// because the shape is only a bug when the element wraps a `position: fixed`
// overlay that was rendered in place rather than portalled. `.field-hint-panel`
// *is* such an overlay and is portalled to the body; the board's cards carry
// deliberate transforms all day and their sheet is portalled too. So the
// checkable fact is not "this rule fills forwards", it is "this element is the
// one standing between an in-place overlay and the window", and that is a fact
// about the JSX. Hence [liftsAnOverlay], kept the way `artBearing` is kept:
// every entry names the component it was read out of.
//
// **The standing fix, when somebody has the appetite for it**, is to portal
// `CardHover`'s cursor preview the way `CardSheet` beside it already is -- the
// same file, the same reason, written down in its own doc comment. That would
// retire this whole class of trap for all twenty-three call sites. Until then
// the containing block has to stay out of the way, and this says so.

// liftsAnOverlay is a class whose element contains a `position: fixed` overlay
// that is rendered *where it stands* rather than portalled to the body, so
// anything that makes the element a containing block moves that overlay off
// the window and out of the reader's reach.
//
// Every entry is a fact about the DOM rather than about the stylesheet, and
// names the component it was read out of, so the next session can re-read it
// rather than trust this line.
var liftsAnOverlay = map[string]string{
	"token-plate": "components/tokens.tsx: the <li> wrapping CardHover, whose " +
		"cursor preview is a position:fixed span rendered in place",
}

// Everything that makes an element a containing block for `position: fixed`
// descendants. `transform` is the one that actually happened; the rest are the
// same trap wearing other spellings, and cost nothing to hold.
var (
	fixedTrap = regexp.MustCompile(
		`(^|[^-\w])(transform|rotate|scale|translate|perspective|` +
			`(-webkit-)?(backdrop-)?filter|will-change|contain)\s*:\s*([^;}]*)`)
	// The shorthand and the longhand. A fill keyword is a bare word among the
	// shorthand's parts, so the value is read as a bag of words rather than
	// parsed -- which over-reports a rule listing two animations where only
	// one fills forwards, and under-reports nothing. Over-reporting on a
	// hand-kept list of one is the right way round.
	animDecl = regexp.MustCompile(
		`(^|[^-\w])animation(-name|-fill-mode)?\s*:\s*([^;}]*)`)
)

// holdsTheWindow answers whether one declaration takes the window away from a
// fixed child. `none` is the only spelling of "nothing here", for every
// property in the set -- `transform: none`, `filter: none`, `contain: none`,
// `will-change: auto`.
func holdsTheWindow(prop, value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	switch prop {
	case "will-change":
		return strings.Contains(v, "transform") ||
			strings.Contains(v, "filter") || strings.Contains(v, "perspective")
	case "contain":
		// Only the values that establish containment do it; `contain: size`
		// and `contain: inline-size` do not.
		return strings.Contains(v, "paint") || strings.Contains(v, "layout") ||
			strings.Contains(v, "strict") || strings.Contains(v, "content")
	default:
		return v != "" && v != "none" && v != "auto"
	}
}

// overlayRules walks the bundle and returns every complaint: a rule whose
// subject is one of [liftsAnOverlay] and which either declares a containing
// block outright, or fills an animation forwards onto keyframes that move.
func overlayRules(css string) []string {
	frames := keyframeBodies(css)
	var found []string
	for _, m := range cssRule.FindAllStringSubmatchIndex(css, -1) {
		selector := strings.TrimSpace(css[m[2]:m[3]])
		body := css[m[4]:m[5]]
		var subject string
		for _, c := range subjectClasses(selector) {
			if _, ok := liftsAnOverlay[c]; ok {
				subject = c
				break
			}
		}
		if subject == "" {
			continue
		}
		say := func(what string) {
			found = append(found, fmt.Sprintf("%s\n      %s\n      (%s)",
				selector, what, liftsAnOverlay[subject]))
		}
		for _, d := range fixedTrap.FindAllStringSubmatch(body, -1) {
			prop := strings.ToLower(d[2])
			if holdsTheWindow(prop, d[5]) {
				say("declares " + prop + ": " + strings.TrimSpace(d[5]))
			}
		}
		// ...and the half that actually happened: a rule declaring no
		// transform of its own, filling one forwards out of a keyframe.
		for _, a := range animDecl.FindAllStringSubmatch(body, -1) {
			words := strings.FieldsFunc(a[3], func(r rune) bool {
				return r == ' ' || r == ',' || r == '\t' || r == '\n'
			})
			fills, moves := false, ""
			for _, w := range words {
				switch strings.ToLower(w) {
				case "both", "forwards":
					fills = true
					continue
				}
				if frame, ok := frames[w]; ok && strings.Contains(frame, "transform") {
					moves = w
				}
			}
			if fills && moves != "" {
				say("fills @keyframes " + moves + " forwards, and those " +
					"keyframes move -- which leaves an identity matrix on the " +
					"element for ever")
			}
		}
	}
	sort.Strings(found)
	return found
}

// TestNoContainingBlockStealsACardPreview: the measurement, held by a check.
func TestNoContainingBlockStealsACardPreview(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if bad := overlayRules(css); len(bad) > 0 {
		t.Errorf("these rules make an element a containing block for the "+
			"`position: fixed` card preview inside it, which puts the preview "+
			"somewhere other than on the window:\n  - %s\n\nFill the animation "+
			"`backwards` rather than `both` (the delay is the half a stagger "+
			"needs), or move the transform onto a child that holds no overlay. "+
			"Then rebuild web_dist/.",
			strings.Join(bad, "\n  - "))
	}
}

// TestEveryOverlayClassIsStillInTheBundle: a table naming a class that no
// longer exists is a check that passes by seeing nothing --
// `cardimagery_test.go`'s anti-vacuity lesson, one file over.
func TestEveryOverlayClassIsStillInTheBundle(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	scripts := bundleScripts(t)
	var gone []string
	for class, why := range liftsAnOverlay {
		if strings.Contains(css, "."+class) {
			continue
		}
		named := false
		for _, js := range scripts {
			if strings.Contains(js, class) {
				named = true
				break
			}
		}
		if !named {
			gone = append(gone, class+" ("+why+")")
		}
	}
	sort.Strings(gone)
	if len(gone) > 0 {
		t.Errorf("liftsAnOverlay names classes the bundle no longer has:\n  %s"+
			"\n\nRe-read the component named beside each and update the entry, "+
			"or drop it.", strings.Join(gone, "\n  "))
	}
}

// TestTheOverlayReaderStillSeesTheTrap: the parser, against the exact rule
// that shipped the bug and the exact rule that fixed it.
//
// Without this the check could rot into always-passing, and the two shapes
// below are not hypothetical -- they are what stood in `index.css` on either
// side of the fix.
func TestTheOverlayReaderStillSeesTheTrap(t *testing.T) {
	t.Parallel()
	const deal = `@keyframes token-deal{0%{opacity:0;transform:translateY(7px) ` +
		`rotate(-.6deg)}to{opacity:1;transform:none}}`
	for _, tc := range []struct {
		name  string
		css   string
		wants bool
	}{
		{"the fill that put the preview off the screen",
			deal + `.token-plate{animation:.32s cubic-bezier(.2,.7,.3,1) both token-deal}`,
			true},
		{"the fill that only covers the stagger's delay",
			deal + `.token-plate{animation:.32s cubic-bezier(.2,.7,.3,1) backwards token-deal}`,
			false},
		{"a transform said outright, which is the same theft",
			`.token-plate:hover{transform:translateY(-2px)}`, true},
		{"and the same property pinned at nothing",
			`.token-plate{transform:none}`, false},
		{"a class nobody listed is nobody's business here",
			deal + `.field-card-turn{animation:.32s ease both token-deal}`, false},
		{"the reduced-motion escape is not an animation at all",
			`.token-plate{animation:none}`, false},
		{"a filter is a containing block too, whatever else it is",
			`.token-plate{filter:blur(2px)}`, true},
		{"promising a transform is as good as having one",
			`.token-plate{will-change:transform}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := len(overlayRules(tc.css)) > 0
			if got != tc.wants {
				t.Errorf("overlayRules(%q) reported %v, want %v",
					tc.css, got, tc.wants)
			}
		})
	}
}
