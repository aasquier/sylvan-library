package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every animation the browser is handed, against the reduced-motion guards.
//
// Commandment 6 wants a page that moves. `prefers-reduced-motion: reduce` is a
// promise to the people for whom movement is nausea rather than delight, and
// the two are only compatible while every animation is actually reachable by a
// guard. On 2026-08-16 that was checked by hand -- 43 animation declarations
// resolved against nine guard blocks -- and the ledger recorded the result as
// complete. Three days later the stylesheet had 111 of them.
//
// **A hand count is not a guard, and the hand count also looked in the wrong
// place.** It swept `web/src/index.css`, and the two animations that were
// genuinely unguarded were never in that file: `animate-spin` on the shared
// spinner and `animate-pulse` on the lazy-chunk skeleton are Tailwind
// utilities, so they exist only in the built bundle. That is
// `browserfloor_test.go`'s lesson one facet over: **what a phone parses is the
// artifact, not the source.** So this file reads the artifact too.
//
// # What "covered" means here, and what this file can and cannot see
//
// A rule is covered when any class in its selector appears inside a
// `prefers-reduced-motion: reduce` block **wearing the same pseudo-element**.
// A bare `.btn` in a guard covers `.btn`; it does not cover `.btn::before`.
//
// **That second sentence was bought.** It used to be one class, no pseudo, and
// the hole was proven by mutation on 2026-09-26: a travelling light was added
// to `.btn::before`, its own guard was deleted, the bundle was rebuilt, and
// this sweep stayed **green**. `.btn` appears in a reduced-motion block several
// hundred lines away — where it turns off *transitions* — and that was enough
// to excuse an animation on a pseudo-element the guard has never touched. The
// two are different boxes: a rule that stops the plate moving says nothing
// about the ring drawn around it, and every voice in the `.btn` family draws
// its glint on one pseudo-element or the other. So the key is the pair, and
// `buttongleam_test.go` — which had to pin that one animation by name because
// this file could not see it — is now the belt to this file's braces rather
// than the only strap.
//
// One escape stays, and it is the honest one: a guard that sets `display:
// none` on a class removes the element, and an element that is not rendered
// draws no `::before`. So a removal covers the class **and** both of its
// pseudo-elements, which is the same reasoning `coveredBy`'s ancestor entries
// already rest on.
//
// Most guards are direct. Two mechanisms are not, and both are real:
//
//   - **A base class on the same element.** `.lab-bubble-2` only sets a delay;
//     the element is `class="lab-bubble lab-bubble-2"` and `.lab-bubble` is
//     guarded.
//   - **An ancestor that is removed outright.** `.wheel-spark` is inside
//     `.wheel-strike`, which the guard sets to `display: none` -- a sword-strike
//     is ceremony, and the honest reduced version is that it does not happen.
//
// Neither is visible in a stylesheet, so [coveredBy] records them. That table
// is an assertion about the DOM which this file cannot check -- what it *can*
// check, and does, is that the cover named is really guarded and that the thing
// being excused really still animates. A typo, a deleted guard or a dead entry
// all fail; only a wrong claim about containment survives, and that one is a
// line of TSX away from the entry.
//
// It is a tripwire, not a rendering engine. It proves a guard exists, not that
// the guard wins the cascade -- source order decides that, and the one time it
// mattered it was verified against the served sheet's rule indices in a real
// browser. The class of drift it catches is the one that happened: an animation
// added, or arriving through a dependency, with nobody to notice it never got a
// guard.
//
// **This is the second time this guard has been written.** The first was Python
// and the Go crossing deleted it; between the two the promise held by nothing at
// all, which is exactly the state the original was built to end.

// cover is what arrests an animating class that no guard names directly.
type cover struct {
	by  string // the class a reduced-motion block does name
	why string // read out of the component, so the next session can re-read it
}

// coveredBy maps an animating class to the class that actually arrests it.
//
// Every entry is a fact about the DOM rather than about the stylesheet: the
// element carries both classes, or it sits inside the covering one. Each was
// read out of the component that renders it, and the component is named so the
// next person can re-read it rather than trust this line.
var coveredBy = map[string]cover{
	// Same element, base class plus a modifier -- components/theme.tsx,
	// routes/Research.tsx, components/forest.tsx.
	"flask-bubble-2":   {"flask-bubble", "class='flask-bubble flask-bubble-2'"},
	"flask-bubble-3":   {"flask-bubble", "class='flask-bubble flask-bubble-3'"},
	"lab-bubble-2":     {"lab-bubble", "class='lab-bubble lab-bubble-2'"},
	"lab-bubble-3":     {"lab-bubble", "class='lab-bubble lab-bubble-3'"},
	"lab-bubble-4":     {"lab-bubble", "class='lab-bubble lab-bubble-4'"},
	"lab-steam-2":      {"lab-steam", "class='lab-steam lab-steam-2'"},
	"lab-flame-2":      {"lab-flame", "class='lab-flame lab-flame-2'"},
	"scene-lane-left":  {"scene-lane", "class='scene-lane scene-lane-left'"},
	"scene-lane-right": {"scene-lane", "class='scene-lane scene-lane-right'"},
	"scene-sunbeam-b":  {"scene-sunbeam", "class='scene-sunbeam scene-sunbeam-b'"},
	// Inside an ancestor the guard sets to `display: none` -- the whole fate
	// effect is ceremony, and components/wheel.tsx nests each of these under
	// the element named here.
	"wheel-coin3d":       {"wheel-coinspin", "inside the spinning coin"},
	"wheel-heart-bloom":  {"wheel-heartfx", "inside the heart"},
	"wheel-ghost":        {"wheel-ghosts", "inside the shades"},
	"wheel-offer-blade":  {"wheel-offer", "inside the offering"},
	"wheel-offer-gleam":  {"wheel-offer", "inside the offering"},
	"wheel-clash-blade":  {"wheel-strike", "inside the strike"},
	"wheel-strike-flash": {"wheel-strike", "inside the strike"},
	"wheel-sparkburst":   {"wheel-strike", "inside the strike"},
	"wheel-spark":        {"wheel-strike", "inside the strike"},
	"wheel-blood":        {"wheel-strike", "inside the strike"},
	// components/forest.tsx: the whole ambience layer is removed, because a
	// firefly holding perfectly still is not a firefly, it is a spot.
	"firefly":   {"forest-ambience", "inside the ambience layer"},
	"leaf-fall": {"forest-ambience", "inside the ambience layer"},
	"page-fall": {"forest-ambience", "inside the ambience layer"},
}

// `animation`, `animation-name`, `animation-duration`, `animation-delay`,
// `animation-play-state` -- and deliberately not `animation-timing-function`,
// which appears inside `@keyframes` steps and animates nothing by itself.
//
// The leading `(^|[^-\w])` is a lookbehind written the way RE2 allows: Go's
// regexp has no `(?<!...)`, and without the guard `-animation:` and
// `--tw-animation:` would both count as declarations.
var (
	motionDeclares = regexp.MustCompile(`(^|[^-\w])animation(-name|-duration|-delay|-play-state)?\s*:`)
	motionArrests  = regexp.MustCompile(`(^|[^-\w])animation\s*:\s*none\s*;?\s*$`)
	cssRule        = regexp.MustCompile(`([^{}@;]+)\{([^{}]*)\}`)
	cssClass       = regexp.MustCompile(`\.([A-Za-z0-9_-]+)`)
	reducedMotion  = regexp.MustCompile(`@media[^{]*prefers-reduced-motion[^{]*\{`)
	// One or two colons, because the minifier writes `::before` as `:before`
	// and both are the same box. No lookbehind is wanted and an early draft's
	// cost the whole fix: a class name cannot contain a colon (`cssClass` says
	// so), so `.fade-after` can never be read as a pseudo-element -- while
	// `.btn:before` is preceded by a perfectly ordinary word character and a
	// "not preceded by a word character" guard silently dropped every
	// pseudo-element in the sheet.
	cssPseudoEl = regexp.MustCompile(`::?(before|after)\b`)
	// A guard that removes the element removes everything it draws with it.
	cssRemoves = regexp.MustCompile(`(^|[;{\s])display\s*:\s*none`)
)

// boxesIn is every (class, pseudo-element) pair a selector names -- the unit
// of coverage. A selector with no `::before`/`::after` names each class's own
// box; one with them names the pseudo-elements, because that is the box the
// declarations in it actually paint.
//
// Two pieces of selector grammar have to be honoured or the answer is wrong in
// the direction that excuses things, and the second one is what let a
// travelling light past this sweep on 2026-09-26:
//
//   - **A comma is a new selector.** `.a::before,.b` is two, and the boxes are
//     `a::before` and `b` -- not the four a cross-product would give.
//   - **A class inside `:not()` is not a target, it is an exclusion.** The
//     gleam's ring is `.btn:not(.arena-gate)::before`, which paints on every
//     button *except* the gate; `.arena-gate::before` is separately guarded
//     because the shut portcullis is its own animation, and reading the
//     negation as a target made that guard excuse the ring it explicitly
//     excludes. Only `:not()` is stripped: `:is()` and `:where()` really do
//     name their contents.
func boxesIn(selector string) []string {
	var out []string
	for _, one := range splitSelectorList(selector) {
		one = stripNegations(one)
		classes := classesIn(one)
		var pseudos []string
		for _, m := range cssPseudoEl.FindAllStringSubmatch(one, -1) {
			pseudos = append(pseudos, m[1])
		}
		for _, c := range classes {
			if len(pseudos) == 0 {
				out = append(out, c)
				continue
			}
			// A descendant selector may name a pseudo-element only at its end,
			// so every class in it is read against that one -- which over-covers
			// the ancestors and never under-covers the subject. Over-covering an
			// ancestor is the safe direction: the thing actually painted is
			// always in the list.
			for _, p := range pseudos {
				out = append(out, c+"::"+p)
			}
		}
	}
	return out
}

// splitSelectorList splits on top-level commas: a comma inside `:not(…)` or
// `:is(…)` belongs to that function, not to the list.
func splitSelectorList(selector string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range selector {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, selector[start:i])
				start = i + 1
			}
		}
	}
	return append(out, selector[start:])
}

// stripNegations removes every `:not(…)`, parenthesis-counted so a nested one
// (`:not(:hover:not(.x))`) comes out whole.
func stripNegations(selector string) string {
	var b strings.Builder
	for i := 0; i < len(selector); {
		rest := selector[i:]
		if !strings.HasPrefix(rest, ":not(") {
			b.WriteByte(selector[i])
			i++
			continue
		}
		j, depth := i+len(":not("), 1
		for j < len(selector) && depth > 0 {
			switch selector[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			j++
		}
		i = j
	}
	return b.String()
}

// animatingRule is one rule that moves, the boxes its selector paints, and the
// bare classes it names. `boxes` is what has to be guarded; `classes` is what
// `coveredBy` is keyed on, because that table's claims are about elements
// (a base class on the same one, or an ancestor removed outright).
type animatingRule struct {
	selector string
	boxes    []string
	classes  []string
}

// guardSpans is the byte range of every `prefers-reduced-motion: reduce`
// at-rule. Brace-counted rather than regexed: the block holds nested rules, so
// the first `}` is not the end of it.
func guardSpans(css string) [][2]int {
	var spans [][2]int
	for _, at := range reducedMotion.FindAllStringIndex(css, -1) {
		i, depth := at[1], 1
		for i < len(css) && depth > 0 {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		spans = append(spans, [2]int{at[0], i})
	}
	return spans
}

// splitSheet is (the rules that animate, the boxes any guard reaches).
//
// The guarded set is keyed by box -- `btn` and `btn::before` are two entries,
// and a guard earns whichever ones its own selector names. The one exception
// is a guard that hides the element: `display: none` on `.x` earns `x`,
// `x::before` and `x::after` together, because an element that is not rendered
// draws no pseudo-elements.
func splitSheet(css string) (animating []animatingRule, guarded map[string]bool, guards int) {
	spans := guardSpans(css)
	guarded = map[string]bool{}
	for _, m := range cssRule.FindAllStringSubmatchIndex(css, -1) {
		selector := strings.TrimSpace(css[m[2]:m[3]])
		body := css[m[4]:m[5]]
		inside := false
		for _, s := range spans {
			if s[0] <= m[2] && m[2] < s[1] {
				inside = true
				break
			}
		}
		if inside {
			for _, b := range boxesIn(selector) {
				guarded[b] = true
			}
			if cssRemoves.MatchString(body) {
				for _, c := range targetClasses(selector) {
					guarded[c] = true
					guarded[c+"::before"] = true
					guarded[c+"::after"] = true
				}
			}
			continue
		}
		trimmed := strings.TrimSpace(body)
		if motionDeclares.MatchString(body) && !motionArrests.MatchString(trimmed) {
			animating = append(animating, animatingRule{
				selector: selector,
				boxes:    boxesIn(selector),
				classes:  targetClasses(selector),
			})
		}
	}
	return animating, guarded, len(spans)
}

func classesIn(selector string) []string {
	var out []string
	for _, m := range cssClass.FindAllStringSubmatch(selector, -1) {
		out = append(out, m[1])
	}
	return out
}

// targetClasses is classesIn with the negations taken out -- the classes a
// selector is *about*, rather than every class name that appears in it. See
// boxesIn for why the difference is load-bearing.
func targetClasses(selector string) []string {
	var out []string
	for _, one := range splitSelectorList(selector) {
		out = append(out, classesIn(stripNegations(one))...)
	}
	return out
}

// servedStylesheet reads the committed bundle's stylesheet and splits it.
func servedStylesheet(t *testing.T) (animating []animatingRule, guarded map[string]bool) {
	t.Helper()
	path := filepath.Join(repoRoot(t), "web_dist", "assets", "index.css")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no committed stylesheet at %s: %v", path, err)
	}
	animating, guarded, guards := splitSheet(string(body))
	// Anti-vacuity in both directions, because this file's whole history is a
	// check that passed while seeing nothing: a parser that stopped matching
	// finds no animations and reads as a clean sweep, and one that found no
	// guards would fail every rule instead of excusing them.
	if len(animating) == 0 || guards == 0 {
		t.Fatalf("parsed %d animating rules and %d guard blocks out of %s; the "+
			"stylesheet's shape has moved and this check is measuring nothing",
			len(animating), guards, path)
	}
	return animating, guarded
}

// TestEveryAnimationInTheBundleCanBeArrested: nothing the browser downloads may
// move with no way to stop it.
func TestEveryAnimationInTheBundleCanBeArrested(t *testing.T) {
	t.Parallel()
	animating, guarded := servedStylesheet(t)
	covers := map[string]bool{}
	for animated, c := range coveredBy {
		if guarded[c.by] {
			covers[animated] = true
		}
	}
	loose := map[string]bool{}
	for _, rule := range animating {
		reached := false
		// The box first: `.x::before` wants a guard that says `::before`.
		for _, b := range rule.boxes {
			if guarded[b] {
				reached = true
				break
			}
		}
		// Then the element, because `coveredBy`'s claims are about elements --
		// a guarded base class on the same one, or a guarded ancestor removed
		// outright. Either takes the pseudo-element with it.
		if !reached {
			for _, c := range rule.classes {
				if covers[c] {
					reached = true
					break
				}
			}
		}
		if !reached {
			loose[rule.selector] = true
		}
	}
	if len(loose) > 0 {
		t.Errorf("these rules animate and no `prefers-reduced-motion: reduce` "+
			"block reaches them:\n  %s\n\nAdd a guard in web/src/index.css and "+
			"rebuild the bundle, or -- if the element already carries a guarded "+
			"base class or sits inside a guarded ancestor -- record that in "+
			"coveredBy with the component it was read from. **A guard on a "+
			"`::before` or `::after` has to name that pseudo-element**: the "+
			"plate and the ring drawn around it are two boxes, and a rule that "+
			"stops one says nothing about the other. Reduced, not necessarily "+
			"removed: a status indicator that stops turning says the wrong "+
			"thing.", strings.Join(sortedSet(loose), "\n  "))
	}
}

// TestEveryCoverNamedIsItselfGuarded: the excuse table cannot excuse anything
// with a guard that is gone.
func TestEveryCoverNamedIsItselfGuarded(t *testing.T) {
	t.Parallel()
	_, guarded := servedStylesheet(t)
	broken := map[string]bool{}
	for animated, c := range coveredBy {
		if !guarded[c.by] {
			broken[animated+" -> "+c.by+" ("+c.why+")"] = true
		}
	}
	if len(broken) > 0 {
		t.Errorf("coveredBy names a cover that no reduced-motion block "+
			"mentions, so the thing it excuses is not arrested by anything:\n  %s",
			strings.Join(sortedSet(broken), "\n  "))
	}
}

// TestTheCoverTableHoldsNothingThatStoppedAnimating: an excuse for something
// deleted is a claim nobody will re-check.
//
// Without this the table only ever grows, and a stale entry could later excuse a
// *different* rule that happens to reuse the class name.
func TestTheCoverTableHoldsNothingThatStoppedAnimating(t *testing.T) {
	t.Parallel()
	animating, _ := servedStylesheet(t)
	live := map[string]bool{}
	for _, rule := range animating {
		for _, c := range rule.classes {
			live[c] = true
		}
	}
	dead := map[string]bool{}
	for animated := range coveredBy {
		if !live[animated] {
			dead[animated] = true
		}
	}
	if len(dead) > 0 {
		t.Errorf("coveredBy still excuses %v, which no longer animates in the "+
			"bundle. Drop the entries.", sortedSet(dead))
	}
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
