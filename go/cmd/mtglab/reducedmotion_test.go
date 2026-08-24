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
// `prefers-reduced-motion: reduce` block. Most guards are that direct. Two
// mechanisms are not, and both are real:
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
)

// animatingRule is one rule that moves, and the classes its selector names.
type animatingRule struct {
	selector string
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

// splitSheet is (the rules that animate, the classes any guard mentions).
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
		classes := classesIn(selector)
		if inside {
			for _, c := range classes {
				guarded[c] = true
			}
			continue
		}
		trimmed := strings.TrimSpace(body)
		if motionDeclares.MatchString(body) && !motionArrests.MatchString(trimmed) {
			animating = append(animating, animatingRule{selector, classes})
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
		for _, c := range rule.classes {
			if guarded[c] || covers[c] {
				reached = true
				break
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
			"coveredBy with the component it was read from. Reduced, not "+
			"necessarily removed: a status indicator that stops turning says "+
			"the wrong thing.", strings.Join(sortedSet(loose), "\n  "))
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
