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
// # The rule, verbatim
//
// Scryfall's imagery guidelines, from https://scryfall.com/docs/api ("Use of
// Scryfall Data and Images"), fetched 2026-08-28. Quoted rather than
// paraphrased because the paraphrase was wrong for a year -- see "what this
// file used to claim" below:
//
//	Do not cover, crop, or clip off the copyright or artist name on card images.
//	Do not distort, skew, or stretch card images.
//	Do not blur, sharpen, desaturate, or color-shift card images.
//	Do not add your own watermarks, stamps, or logos to card images.
//	When using the art_crop, list the artist name and copyright elsewhere in
//	the same interface presenting the art crop, or use the full card image
//	elsewhere in the same interface. Users should be able to identify the
//	artist and source of the image somehow.
//
// ADR 32 bound this repo to the same line in its own words -- *"motion and
// parallax, never distortion, blur or colour-shift of the artwork"* -- and
// ADR 48 is the sweep that made the whole app obey it. Commandment 9 makes
// honouring it a hard boundary rather than a preference; commandment 19 is
// the rule itself, and points here.
//
// **What this file used to claim, and why the correction matters.** It said
// flatly that the guidelines "forbid cropping". They do not. The clause is
// about the *credit* -- covering or clipping the copyright line and the
// artist name -- and Scryfall publish `art_crop` and `art` endpoints that are
// nothing but a crop. That overstatement is why the `object-fit: cover`
// question sat here for months as an unanswerable "policy call". It has an
// answer: cropping is a violation exactly when it clips the artist or
// copyright off a **full card image**, and it is fine on an art crop whose
// artist is credited elsewhere in the same interface. Measured on
// 2026-08-28, every `object-fit: cover` in this app is on an art crop, or on
// a full card in a box within a quarter of a per cent of the printed
// 488x680 -- `.stage-face` is exactly it and `.field-card-turn` is 58/81.
// Nothing clips a credit.
//
// **`brightness` and `contrast` are not enumerated by name.** "color-shift"
// is the umbrella they sit under in any ordinary reading, and ADR 32 chose
// the strict line before anyone read the clause this carefully. It stays
// strict: stricter than required is free. But the citation is honest --
// "color-shift", never an invented "no brightness" rule.
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
// It reads the bundle's **script** as well as its stylesheet, which is the
// hole ADR 48 closed and the reason the graveyard carried a live
// `style={{ filter: 'grayscale(0.7)' }}` for as long as it did: an inline
// filter in JSX never reaches a stylesheet, so a stylesheet reader is blind
// to it *by construction* and says nothing is wrong. The alternative
// considered was a lint rule banning `filter` in a JSX `style` prop. Reading
// the artifact wins for the same reason it won for the CSS -- a lint rule
// polices one spelling in one language in one directory, and the bundle is
// what the browser is handed however it got written. It costs the check
// nothing: [bundleScripts] is a directory read, and both halves run against
// the same [alteringFilters].
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
// `.field-card-leaf::after` is the reference implementation, and index.css's
// "light lands ON a card" block is the same argument in the language the
// fixes are written in.
//
// # How the covered list is kept
//
// [artBearing] was the cards in play and the card on the stage, and the
// comment that stood here deferred everything else. ADR 48 settled the rest
// by **cross-referencing every altering filter in the stylesheet against the
// JSX that puts card art on the class it names** -- which is the sweep to
// re-run rather than a list to re-read, because it found four surfaces a
// careful reading of the audit had missed, one of them (`.scene-backdrop-art`)
// on every mastheaded route in the app.
//
// A name added here with nothing else done fails immediately, which is the
// right way round.

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

	// --- added by ADR 48's sweep, 2026-08-28 -------------------------------
	//
	// Every one of these carried an altering filter when the sweep began, and
	// the entry is what the fix has to keep true.

	"masthead-art": "components/ui.tsx: <img className='masthead-art' src={art}> " +
		"in PageMasthead -- the one image every mastheaded route shows",
	"masthead-plate": "components/ui.tsx: the span the masthead art sits in, " +
		"which the lift is inset:0 of",
	"hero-art": "routes/Library.tsx: <img className='hero-art' " +
		"src={SYLVAN_LIBRARY_ART}> behind FirstRun's copy",
	"deck-hero-band": "routes/DeckDetail.tsx: the div holding the commander's " +
		"still, its motion derivative and the scrim",
	"deck-hero-frame": "routes/DeckDetail.tsx: CardArt's ratio class on the " +
		"still, so the <img> is inside it",
	"deck-hero-motion": "components/cardmotion.tsx: the box ADR 32's derivative " +
		"renders its <video>/<canvas> into",
	"deck-hero-video": "components/cardmotion.tsx: the <video> itself -- a loop " +
		"derived from the painting is still the painting",
	// `.scene-backdrop-art` is deliberately NOT here: it no longer exists. It
	// was `<img src={art}>` at `blur(11px)` across every mastheaded route, and
	// the fix was to stop showing a card image at all rather than to hold one
	// -- `lib/artwash.ts` samples nine colours out of the painting and the room
	// is lit by those. There is nothing left on that element for a filter to
	// reach, so an entry naming it would be a check on a class the anti-vacuity
	// test would correctly report as gone. `scene-lane` below is what still
	// holds `SceneBackdrop`, and it is two `<img src={art}>` of the same
	// painting.
	"scene-lane": "components/forest.tsx: two <img src={art}> as the walls " +
		"of the room at >=1440px",
	"card-sheet-art": "components/ui.tsx: <img className='card-sheet-art' " +
		"src={c.image}> -- a whole card face, frame and printed credit included",
	"card-sheet-slide": "components/ui.tsx: the span one card of the carousel " +
		"sits in, and the element that turns",
	"seance-vision": "components/tarot.tsx: <img className='seance-vision' " +
		"src={vision.image}> in the crystal ball -- commandment 15's room",
	"arena-art": "routes/Coliseum.tsx: <img className='arena-art' " +
		"src={arena.art.url}> -- the chosen arena's painting",
	"coliseum-hero-art": "routes/Coliseum.tsx: the banner, <img> or <video>, " +
		"Grand Coliseum by Carl Critchlow",
	"arena-gate-card": "routes/Coliseum.tsx: <img className='arena-gate-card'> -- " +
		"a whole printed card AS the button; index.css's own rule is 'nothing is " +
		"done to it'",
	"field-fan-card": "components/gearfan.tsx: the span each card of the gear " +
		"fan sits in, holding <img src={c.image}> -- a whole printed card",
	"field-pile-ground": "components/board.tsx: <img className='field-pile-ground' " +
		"src={dressed.art.url}> -- the zone's painting",
	"field-pile-art": "components/board.tsx: <img className='field-pile-art' " +
		"src={top.image}> -- the pile's top card",
	"field-pile-ghost": "components/board.tsx: the span the rising ghost's <img> " +
		"is inside",
	"field-pile-ghost-lit": "components/board.tsx: the wrapper the ghost's pallor " +
		"is screened onto, because an <img> has no pseudo-element",
	"entombing": "routes/DeckDetail.tsx: the <li> a card being entombed wears, " +
		"and it contains that card's CardArt",
	"art-dimmed": "index.css: the shared wrapper that puts a shade over one " +
		"piece of card art (DeckDetail's graveyard rows use it)",

	// --- the token shelf, 2026-08-28 ---------------------------------------
	//
	// Not from ADR 48's sweep: these two carried no filter and still do not.
	// They are here because the shelf is the newest surface in the app that
	// puts a whole printed card face on the page, and the sweep's own lesson
	// was that the list is kept by walking the JSX rather than by waiting for
	// a violation. A plate lifts and turns on hover, which is exactly the
	// gesture that reaches for `filter` next time.

	"token-face": "components/tokens.tsx: <img className='token-face' " +
		"src={token.image}> in TokenPlateCard -- a whole printed token face, " +
		"artist and copyright line included",
	"token-plate": "components/tokens.tsx: the <li> that face sits in, and " +
		"the element whose :hover lifts and turns it",

	// --- the 32 colour pages, 2026-08-28 -----------------------------------
	//
	// Twenty of the 32 wear a faction's heraldry -- Ravnica's Signets,
	// Alara's Obelisks, Tarkir's Banners -- as the art crop of one pinned
	// printing, with the artist and printing credited in the same room
	// (`.combo-label-hand`). Added by walking the JSX rather than in response
	// to a violation: these two carry no filter and never have. The plate is
	// here as well as the image because the plate is what a `:hover` or a
	// theme rule would land on, and it is what `.art-lift` is `inset: 0` of.

	"combo-art": "routes/ColorPage.tsx: <img className='combo-art' " +
		"src={sigil.art}> in Nameplate -- the faction's own painted device",
	"combo-plate": "routes/ColorPage.tsx: the span that painting sits in, " +
		"which the dark theme's lift is inset:0 of",
}

// Anything that re-renders the pixels. `drop-shadow` is deliberately absent:
// it draws a shadow of the element's own alpha silhouette and changes nothing
// inside the outline, which is a `box-shadow` that understands transparency
// rather than a change to the image. A function pinned at its identity is a
// no-op and is allowed, because pinning a property at its identity in the
// first keyframe is how you stop the browser interpolating it across the whole
// animation -- a trap `stage-doom` paid for once and records.
var (
	shifting = regexp.MustCompile(
		`(grayscale|sepia|saturate|hue-rotate|invert|blur|brightness|contrast|opacity)\(([^)]*)\)`)
	zeroArg = regexp.MustCompile(`^(0|0\.0+|0px|0deg|0%|0turn|0rad)$`)
	oneArg  = regexp.MustCompile(`^(1|1\.0+|100%)$`)
	// An SVG filter reference. Whatever is on the other end of that id can do
	// anything at all -- the seance's was an feDisplacementMap at scale="15",
	// fifteen pixels of the painting physically moved -- so there is no
	// argument to inspect and no version of this that is a no-op.
	svgFilter   = regexp.MustCompile(`url\(\s*['"]?#`)
	filterDecl  = regexp.MustCompile(`(^|[^-\w])(-webkit-)?(backdrop-)?filter\s*:\s*([^;}]*)`)
	animName    = regexp.MustCompile(`(^|[^-\w])animation(-name)?\s*:\s*([^;}]*)`)
	keyframesAt = regexp.MustCompile(
		`@(-webkit-)?keyframes\s+([A-Za-z0-9_-]+)\s*\{`)
	// A `filter` (or its vendor and backdrop spellings) as an object-literal
	// KEY, which is what a JSX `style` prop is by the time the bundle exists.
	// `[{,]` before the name is what keeps `xs.filter(...)` out: an array
	// method is reached through a dot and followed by a paren, never preceded
	// by a brace and followed by a colon. The value runs to the next comma or
	// brace, so a conditional (`e?"grayscale(1)":"none"`) is read whole rather
	// than only its first branch.
	jsStyleFilter = regexp.MustCompile(
		`[{,]\s*["']?(?:filter|WebkitFilter|webkitFilter|backdropFilter)["']?\s*:\s*([^,}]*)`)
)

// noopArg answers whether one filter function, with one argument, changes the
// picture -- and it has to be per function rather than one list of harmless
// strings, because **the identity is not the same number for all of them and
// an omitted argument does not mean the same thing.**
//
// Per the Filter Effects spec, an omitted argument defaults to 1 for
// `grayscale`, `sepia`, `invert`, `saturate`, `brightness`, `contrast` and
// `opacity`, and to 0 for `blur` and `hue-rotate`. For the first three of
// those, **1 is full strength**: a bare `grayscale()` is `grayscale(1)` and
// takes every colour out of the picture, while a bare `saturate()` is
// `saturate(1)` and does nothing whatsoever. To a parser they are the same
// shape, and the minifier writes both -- it drops an argument that equals the
// default. One list could not tell them apart and did not try.
//
// The old single `noop` accepted `1` for every function in the set, which
// meant **`grayscale(1)` read as harmless** -- the strongest possible
// desaturation, excused. It was not hypothetical: `@keyframes entomb-sink`
// carried `grayscale(1) brightness(0.85)` while this file was being written.
func noopArg(fn, arg string) bool {
	arg = strings.TrimSpace(arg)
	switch fn {
	case "grayscale", "sepia", "invert":
		// Nothing is only nothing when it says so.
		return zeroArg.MatchString(arg)
	case "blur", "hue-rotate":
		return arg == "" || zeroArg.MatchString(arg)
	default: // saturate, brightness, contrast, opacity
		return arg == "" || oneArg.MatchString(arg)
	}
}

// alteringFilters is every filter function in a declaration that actually
// changes the picture, or nil when the declaration is all no-ops or a pure
// drop-shadow.
func alteringFilters(value string) []string {
	var out []string
	if svgFilter.MatchString(value) {
		out = append(out, "url(#...) -- an SVG filter, contents unknown here")
	}
	for _, m := range shifting.FindAllStringSubmatch(value, -1) {
		if noopArg(m[1], m[2]) {
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
// **It reads the script as well as the stylesheet**, and that is not tidiness:
// a class can be perfectly real and have no rule of its own. `.deck-hero-band`
// became exactly that the moment its `brightness(1.45)` moved to a sibling
// layer -- still the element wrapping the commander's painting, still worth
// holding, and no longer named anywhere in `index.css`. Asking only the
// stylesheet would have reported a live, correct entry as dead and invited the
// next session to delete it.
func TestEveryArtBearingClassIsStillInTheBundle(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	scripts := bundleScripts(t)
	var gone []string
	for class, why := range artBearing {
		if strings.Contains(css, "."+class) {
			continue
		}
		named := false
		for _, js := range scripts {
			// Bare, because a class arrives in JSX as a word inside a string
			// (`"art-lift deck-hero-lift"`), never with its selector dot.
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
		t.Errorf("artBearing names classes the bundle no longer has, in either "+
			"its stylesheet or its script, so nothing is being checked for "+
			"them:\n  %s\n\nRe-read the component named beside each and update "+
			"the entry, or drop it.", strings.Join(gone, "\n  "))
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

// bundleScripts is every script the committed bundle serves, by file name --
// the app chunk and one per lazily routed page, which is why it is a
// directory read rather than a named file. A route that gained its own chunk
// is covered the day it appears, with nothing added here.
func bundleScripts(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "web_dist", "assets")
	names, err := filepath.Glob(filepath.Join(dir, "*.js"))
	if err != nil {
		t.Fatalf("cannot list %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, name := range names {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("cannot read %s: %v", name, err)
		}
		out[filepath.Base(name)] = string(body)
	}
	// The same anti-vacuity floor the stylesheet keeps, in both directions: no
	// files at all, or files that no longer look like this app's bundle, has
	// to fail rather than sweep clean. `createElement` is React's own and
	// survives minification as a property name.
	joined := 0
	for _, body := range out {
		if strings.Contains(body, "createElement") {
			joined++
		}
	}
	if len(out) < 5 || joined == 0 {
		t.Fatalf("found %d scripts in %s and %d that look like this app's; the "+
			"bundle's shape has moved and this check is measuring nothing",
			len(out), dir, joined)
	}
	return out
}

// inlineStyleFilters is every altering filter written as an object-literal
// `filter` key anywhere in the bundle's script -- which is what
// `style={{ filter: ... }}` in a component becomes.
//
// **It does not ask which element the style lands on, and that is deliberate.**
// A minified bundle has no components left in it to attribute a style object
// to; asking would mean answering "which class is this?" against machine-
// generated code, and answering it wrong in the direction that excuses things.
// So the rule here is flatter and harder than the stylesheet's: an inline
// filter is banned outright, whatever it is on. That costs nothing real,
// because an inline `style` is already the wrong place for a filter -- no
// `:hover` can reach it (commandment 17's lesson, one property over), no media
// query can arrest it, and the stylesheet is where every effect in this app
// belongs. If a non-card element genuinely needs one, it wants a class.
func inlineStyleFilters(js string) []string {
	var found []string
	for _, m := range jsStyleFilter.FindAllStringSubmatch(js, -1) {
		if bad := alteringFilters(m[1]); len(bad) > 0 {
			found = append(found, strings.TrimSpace(m[0]))
		}
	}
	sort.Strings(found)
	return found
}

// TestNoInlineStyleFilterShipsInTheBundle: the hole the graveyard fell through.
//
// `routes/DeckDetail.tsx` carried `style={{ filter: 'grayscale(0.7)' }}` on a
// span wrapping a dead card's art, and every check in this file read it as a
// clean stylesheet -- because it read the stylesheet, and an inline style is
// not in one. A guard that cannot see a whole class of the thing it guards
// against reports a clean sweep with the violation on screen.
func TestNoInlineStyleFilterShipsInTheBundle(t *testing.T) {
	t.Parallel()
	for name, js := range bundleScripts(t) {
		if bad := inlineStyleFilters(js); len(bad) > 0 {
			t.Errorf("web_dist/assets/%s ships inline style filters:\n  - %s\n\n"+
				"An inline `style` filter is banned outright: no :hover, no media "+
				"query and no check can reach it, and on card art it is Scryfall's "+
				"\"blur, sharpen, desaturate, or color-shift\" clause with nothing "+
				"in the way. Move the effect to a class in web/src/index.css, draw "+
				"it as a layer over the art the way `.art-dimmed` and `.art-lift` "+
				"do, and rebuild web_dist/.",
				name, strings.Join(bad, "\n  - "))
		}
	}
}

// TestTheScriptReaderTellsAStyleFromAnArrayMethod: the parser, against the
// shapes a real bundle is full of.
//
// `.filter(` appears hundreds of times in any React bundle and none of them is
// a style. A reader that could not tell the difference would be turned off
// within a day, which is the failure mode that matters here -- not a missed
// violation, a check nobody can keep.
func TestTheScriptReaderTellsAStyleFromAnArrayMethod(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		js    string
		wants bool
	}{
		{"the grey that was on a dead card's art",
			`x.createElement("span",{style:{filter:"grayscale(0.7)"}})`, true},
		{"the same, single quoted",
			`{style:{filter:'blur(3px)'}}`, true},
		// **The spelling the minifier actually chose**, measured by putting the
		// graveyard's violation back, rebuilding and reading the output: esbuild
		// rewrote `'grayscale(0.7)'` as a template literal. A reader that had
		// hard-coded the two ordinary quote characters would have passed a bundle
		// with the exact bug it was written for -- which is a failure this repo
		// has already had, in `configrecord`, for the same reason.
		{"a template literal, which is what the bundle came out as",
			"{filter:`grayscale(0.7)`}", true},
		{"a conditional, read whole rather than by its first branch",
			`{filter:e?"none":"grayscale(1)"}`, true},
		{"the vendor spelling",
			`{WebkitFilter:"saturate(0.4)"}`, true},
		{"a backdrop filter, which blurs whatever is behind it",
			`{backdropFilter:"blur(6px)"}`, true},
		{"an array method, which is not a style at all",
			`const seen=cards.filter((c)=>c.image).map(k)`, false},
		{"a filter pinned at its identity so it cannot interpolate",
			`{filter:"brightness(1)"}`, false},
		{"a drop-shadow, which is a shadow of the outline",
			`{filter:"drop-shadow(0 2px 6px #000)"}`, false},
		{"no filter at all",
			`{style:{opacity:0.85}}`, false},
	} {
		if got := len(inlineStyleFilters(tc.js)) > 0; got != tc.wants {
			t.Errorf("%s: reader says inline-filter=%v, want %v", tc.name, got, tc.wants)
		}
	}
}

// TestAnOmittedArgumentIsNotOneThing: the correction that mattered most.
//
// The minifier drops an argument that equals the function's default, so the
// bundle spells both `saturate(1)` and `grayscale(1)` with empty parentheses
// -- and those two mean opposite things. A single list of harmless arguments
// cannot hold both, and the one that stood here held neither correctly: it
// excused `grayscale(1)`, which is every colour gone.
func TestAnOmittedArgumentIsNotOneThing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value   string
		altered bool
	}{
		// Argument-less, and full strength: the default for these three is 1.
		{"grayscale()", true},
		{"sepia()", true},
		{"invert()", true},
		// Argument-less, and nothing: the default for these is the identity.
		{"saturate()", false},
		{"brightness()", false},
		{"contrast()", false},
		{"blur()", false},
		{"hue-rotate()", false},
		// Written out. `grayscale(1)` is the one the old reader excused.
		{"grayscale(1)", true},
		{"grayscale(0)", false},
		{"grayscale(0%)", false},
		{"saturate(1)", false},
		{"saturate(100%)", false},
		{"saturate(0.6)", true},
		{"brightness(1.0)", false},
		{"brightness(0.94)", true},
		{"blur(0)", false},
		{"blur(0.4px)", true},
		{"hue-rotate(0deg)", false},
		// An SVG filter reference, which has no argument to read.
		{"url(#seance-ripple)", true},
		{"url(#seance-ripple) saturate(1.05) brightness(1.12) blur(0.4px)", true},
	} {
		if got := len(alteringFilters(tc.value)) > 0; got != tc.altered {
			t.Errorf("%q: reader says altered=%v, want %v",
				tc.value, got, tc.altered)
		}
	}
}
