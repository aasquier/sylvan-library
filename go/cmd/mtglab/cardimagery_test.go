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

// Nothing may reach through a card and change Wizards' painting.
//
// Scryfall's imagery guidelines forbid cropping, distorting, desaturating and
// watermarking card images, and ADR 32 wrote that boundary into this repo in
// so many words: the effect vocabulary is *"motion and parallax, never
// distortion, blur or colour-shift of the artwork."* Commandment 9 makes
// honouring it a hard boundary rather than a preference.
//
// **The rule was already written down and the stylesheet drifted off it
// anyway.** On 2026-08-26 a dying creature in the Coliseum was drawn with
// `filter: grayscale(0.85) brightness(0.62)` on `.field-card-turn` -- the
// art's own container -- and the card on the centre stage with
// `grayscale(0.8)` on `.stage-face`, which *is* the painting. Neither was
// sloppy; both were somebody reaching for the obvious CSS for "this thing
// died" eleven days after the ADR said not to. A sentence in a document does
// not stop that. This does.
//
// # What it reads, and why it reads the bundle
//
// The committed bundle rather than the source, for `reducedmotion_test.go`'s
// reason one facet over: **what a browser applies is the artifact.** A rule
// that arrives through a utility class or a dependency is invisible in
// `index.css` and perfectly visible here.
//
// # The distinction the whole check turns on
//
// A `filter` on a *pseudo-element* is a filter on a layer somebody drew. A
// `filter` on the element itself reaches the artwork under it. That is exactly
// the line the token materials found first (`web/src/lib/tokens.ts`, and the
// "what a token is made of" block beside them): light is laid on a card
// through a layer of its own, never as a filter on the art. So
// `.stage-frame::before { filter: blur(7px) }` is the spell's ring of light
// and is fine; `.stage-frame { animation: ... }` reaching a keyframe that
// blurs is the card going soft and is not.
//
// # What it deliberately does not cover
//
// **The cards in play and the card on the stage, and nothing else yet.** The
// same sweep that found the two above found colour-shifts on the deck heroes,
// the mastheads, the Wheel's own copies of an Alpha painting, the seance
// vision and the card-sheet carousel, plus a much larger question about
// `object-fit: cover` reframing a printed crop. Those are separate surfaces
// with their own arguments and at least one of them is a policy call rather
// than a bug; widening [artBearing] is how they get held once they are
// settled, and a name added here with nothing else done fails immediately,
// which is the right way round.

// artBearing is a class whose element either *is* a Scryfall image or is an
// ancestor of one, so a filter landing on it lands on the painting.
//
// Every entry is a fact about the DOM rather than about the stylesheet, and
// names the component it was read out of so the next session can re-read it
// rather than trust this line -- the same contract `coveredBy` keeps in
// reducedmotion_test.go.
var artBearing = map[string]string{
	"field-card-art": "components/board.tsx: <img className='field-card-art' " +
		"src={card.image}> -- the painting itself",
	"field-card-turn": "components/board.tsx: the div the art <img> is inside, " +
		"and the element that turns when a permanent taps",
	"field-card-leaf": "components/board.tsx: style={{'--leaf-art': " +
		"`url(${card.image})`}}, drawn by index.css as background-image",
	"stage-face": "components/stage.tsx: <img className='stage-face' " +
		"src={item.image}> -- the whole card face, big",
	"stage-frame": "components/stage.tsx: the span the stage face is inside",
	"stage-card":  "components/stage.tsx: the group holding frame, veil and plate",
}

// Anything that re-renders the pixels. `drop-shadow` is deliberately absent:
// it draws a shadow of the element's own alpha silhouette and changes nothing
// inside the outline, which is a `box-shadow` that understands transparency
// rather than a change to the image. `blur(0)`, `brightness(1)` and their
// kind are no-ops and are allowed, because pinning a property at its identity
// in the first keyframe is how you stop the browser interpolating it across
// the whole animation -- a trap `stage-doom` paid for once and records.
var (
	shifting = regexp.MustCompile(
		`(grayscale|sepia|saturate|hue-rotate|invert|blur|brightness|contrast|opacity)\(([^)]*)\)`)
	noop = regexp.MustCompile(
		`^(0|0px|0deg|0%|0turn|0rad|1|1\.0+|100%|none)$`)
	filterDecl  = regexp.MustCompile(`(^|[^-\w])(-webkit-)?(backdrop-)?filter\s*:\s*([^;}]*)`)
	animName    = regexp.MustCompile(`(^|[^-\w])animation(-name)?\s*:\s*([^;}]*)`)
	keyframesAt = regexp.MustCompile(
		`@(-webkit-)?keyframes\s+([A-Za-z0-9_-]+)\s*\{`)
)

// alteringFilters is every filter function in a declaration that actually
// changes the picture, or nil when the declaration is a no-op or a pure
// drop-shadow.
func alteringFilters(value string) []string {
	var out []string
	for _, m := range shifting.FindAllStringSubmatch(value, -1) {
		if noop.MatchString(strings.TrimSpace(m[2])) {
			continue
		}
		out = append(out, m[0])
	}
	return out
}

// keyframeBodies is every `@keyframes` block in the sheet, by name, brace
// counted for the same reason guardSpans is: the block holds nested rules and
// the first `}` is not the end of it.
func keyframeBodies(css string) map[string]string {
	out := map[string]string{}
	for _, at := range keyframesAt.FindAllStringSubmatchIndex(css, -1) {
		name := css[at[4]:at[5]]
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
		out[name] += css[at[1]:i]
	}
	return out
}

// **The bundle spells a pseudo-element with one colon.** The minifier rewrites
// `::before` to the legacy `:before`, which is the same element and a
// different string -- so a reader that only knows the modern spelling excuses
// nothing and reports the arena's own ring of light as a filter on a card. It
// was the first thing this check said when it was pointed at the artifact, and
// it is `reducedmotion_test.go`'s lesson again: what a browser parses is the
// bundle, and the bundle is not the source retyped.
//
// Pseudo-*elements* only. `:hover` and `:focus-visible` are pseudo-classes --
// they select the element itself, so a filter under one is still on the
// painting and must still be caught.
var pseudoElements = []string{":before", ":after", ":first-line", ":first-letter",
	":placeholder", ":backdrop", ":marker", ":selection"}

// subjectClasses is the classes of the *last* compound selector in each comma
// separated part -- the element the rule actually styles -- and only when that
// compound is not a pseudo-element. `.stage-card.is-dies .stage-face` yields
// `stage-face`; `.stage-frame::before` and its minified `:before` twin yield
// nothing at all, which is the point.
func subjectClasses(selector string) []string {
	var out []string
	for _, part := range strings.Split(selector, ",") {
		fields := strings.Fields(strings.TrimSpace(
			strings.NewReplacer(">", " ", "+", " ", "~", " ").Replace(part)))
		if len(fields) == 0 {
			continue
		}
		subject := strings.ToLower(fields[len(fields)-1])
		drawn := strings.Contains(subject, "::")
		for _, p := range pseudoElements {
			drawn = drawn || strings.Contains(subject, p)
		}
		if drawn {
			continue
		}
		out = append(out, classesIn(subject)...)
	}
	return out
}

// artRules walks the bundle and returns every complaint: a rule whose subject
// is an art-bearing element and which alters the picture, either in its own
// body or through a `@keyframes` it names.
func artRules(css string) []string {
	frames := keyframeBodies(css)
	var found []string
	for _, m := range cssRule.FindAllStringSubmatchIndex(css, -1) {
		selector := strings.TrimSpace(css[m[2]:m[3]])
		body := css[m[4]:m[5]]
		var subjects []string
		for _, c := range subjectClasses(selector) {
			if _, ok := artBearing[c]; ok {
				subjects = append(subjects, c)
			}
		}
		if len(subjects) == 0 {
			continue
		}
		say := func(where string, bad []string) {
			found = append(found, fmt.Sprintf("%s\n      %s: %s\n      (%s)",
				selector, where, strings.Join(bad, " "), artBearing[subjects[0]]))
		}
		for _, d := range filterDecl.FindAllStringSubmatch(body, -1) {
			if bad := alteringFilters(d[4]); len(bad) > 0 {
				say("declares", bad)
			}
		}
		// ...and the half that actually happened: a rule whose own body is
		// clean, naming a keyframe that is not.
		for _, a := range animName.FindAllStringSubmatch(body, -1) {
			for _, word := range strings.FieldsFunc(a[3], func(r rune) bool {
				return r == ' ' || r == ','
			}) {
				frame, ok := frames[word]
				if !ok {
					continue
				}
				for _, d := range filterDecl.FindAllStringSubmatch(frame, -1) {
					if bad := alteringFilters(d[4]); len(bad) > 0 {
						say("animates @keyframes "+word+", which applies", bad)
					}
				}
			}
		}
	}
	sort.Strings(found)
	return found
}

// TestNoFilterReachesACardPainting: the guideline, held by a check rather than
// by whoever last read the ADR.
func TestNoFilterReachesACardPainting(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if bad := artRules(css); len(bad) > 0 {
		t.Errorf("these rules re-render Wizards' artwork, which Scryfall's "+
			"imagery guidelines forbid and ADR 32 bounds this repo away from:"+
			"\n  - %s\n\nDraw the effect as a layer *over* the card instead -- a "+
			"pseudo-element or a sibling carrying the light, the way the token "+
			"materials do (web/src/lib/tokens.ts). A filter on a ::before is "+
			"a filter on something you drew and is fine; a filter on the card "+
			"is the card being altered. Then rebuild web_dist/.",
			strings.Join(bad, "\n  - "))
	}
}

// TestEveryArtBearingClassIsStillInTheBundle: a table naming classes that no
// longer exist is a check that passes by seeing nothing.
//
// This is the failure mode this file is most likely to have, because renaming
// a class in `board.tsx` is an ordinary thing to do and nothing else here
// would notice. It is `reducedmotion_test.go`'s anti-vacuity lesson: a parser
// that stopped matching reads as a clean sweep.
func TestEveryArtBearingClassIsStillInTheBundle(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	var gone []string
	for class, why := range artBearing {
		if !strings.Contains(css, "."+class) {
			gone = append(gone, class+" ("+why+")")
		}
	}
	sort.Strings(gone)
	if len(gone) > 0 {
		t.Errorf("artBearing names classes the bundle no longer has, so "+
			"nothing is being checked for them:\n  %s\n\nRe-read the component "+
			"named beside each and update the entry, or drop it.",
			strings.Join(gone, "\n  "))
	}
}

// TestTheFilterReaderStillSeesFilters: the parser, against the exact rules
// this file was written for.
//
// Without this the check could rot into always-passing and read as a clean
// stylesheet forever -- and the two shapes below are not hypothetical, they
// are what stood in `index.css` on the night this was written. The keyframe
// case is the one a simpler reader misses: `.stage-card.is-dies .stage-face`
// declared no filter at all, it declared `animation: stage-cold`.
func TestTheFilterReaderStillSeesFilters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		css   string
		wants bool
	}{
		{"the grey that was on a dying card",
			`.field-card.is-dies .field-card-turn{filter:grayscale(0.85) brightness(0.62);}`,
			true},
		{"the grey that reached through a keyframe",
			`@keyframes stage-cold{100%{filter:grayscale(0.8) brightness(0.72);}}` +
				`.stage-card.is-dies .stage-face{animation:stage-cold 2s ease-out both;}`,
			true},
		{"the blur that softened a leaving card",
			`@keyframes stage-rise{100%{filter:blur(4px);}}` +
				`.stage-frame{animation:stage-rise 1150ms ease both;}`,
			true},
		{"light on a layer somebody drew",
			`.stage-frame::before{filter:blur(7px);}`, false},
		// The same rule as the browser is actually handed it. This is not a
		// hypothetical spelling: it is what `npm run build` emits, and reading
		// only the modern one made this check's first run accuse the arena's
		// ring of light of being a filter on a card.
		{"the same layer, as the bundle spells it",
			`.stage-frame:before{filter:blur(7px);}`, false},
		{"a pseudo-CLASS is still the element itself",
			`.field-card-leaf:hover{filter:brightness(0.7);}`, true},
		{"a shadow of the card's own outline",
			`.stage-face{filter:drop-shadow(0 14px 34px rgba(0,0,0,0.72));}`, false},
		{"a property pinned at its identity so it cannot interpolate",
			`@keyframes stage-doom{0%{filter:blur(0);}100%{opacity:0;}}` +
				`.stage-frame{animation:stage-doom 2s ease both;}`, false},
		{"a filter on something that is not a card",
			`.wheel-moon{filter:blur(7px);}`, false},
	} {
		if got := len(artRules(tc.css)) > 0; got != tc.wants {
			t.Errorf("%s: reader says altered=%v, want %v", tc.name, got, tc.wants)
		}
	}
}

// bundleStylesheet is the committed bundle's stylesheet, with the same
// anti-vacuity floor servedStylesheet keeps: a file that has moved or emptied
// out must fail loudly rather than pass by having nothing in it.
func bundleStylesheet(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "web_dist", "assets", "index.css")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no committed stylesheet at %s: %v", path, err)
	}
	css := string(body)
	if !strings.Contains(css, ".field-card-art") {
		t.Fatalf("%s does not mention .field-card-art; the bundle's shape has "+
			"moved and this check is measuring nothing", path)
	}
	return css
}
