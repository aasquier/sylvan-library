package main

import (
	"regexp"
	"strings"
	"testing"
)

// The reading table's three promises about a card lying on it.
//
// Commandment 15 makes this room the one that gets the best of everything,
// and on 2026-09-26 it got the room's own light: the séance table is a
// photograph in which forty candles stand in banks to the left, the right and
// the back, and the cards lie at the near edge, which is the furthest point
// from every one of them. Until this branch the cards were flat scans lit from
// nowhere and were therefore the brightest objects in a candlelit room --
// pasted onto the photograph rather than lying on the table in it.
//
// Three things were done about it, and all three are the kind of promise that
// is invisible from the page you are looking at:
//
//  1. **The light is a layer, never a filter.** Half the cards in the deck are
//     Scryfall paintings -- a Magic crossover wears the 1909 frame with
//     `<img className='tarot-rws-art' src={card.image}>` inside it, and
//     `card.image` is a `cards.scryfall.io/art_crop/...` URL. Commandment 19
//     and `cardimagery_test.go` therefore both apply here, and that file's
//     `artBearing` grew by seven tarot classes on the same branch. This file
//     holds the other half of the claim: that the lamp **exists** and is drawn
//     on a pseudo-element. A missing layer is not a violation, so the sweep
//     next door would say nothing at all about it, and the obvious way to
//     "fix" a card that is too bright on a dark table is the exact `filter`
//     that ADR 48 spent a day removing from fourteen other surfaces.
//
//  2. **The glint is an event and the lamp is not.** A turning card throws a
//     specular as it passes the candles; that rides the flip's 760ms and is
//     *removed* under reduced motion, because a band of light parked halfway
//     across a picture is a smudge. The lamp underneath it stays, because
//     "this object is lying on that table" is information about the room
//     rather than ceremony. `reducedmotion_test.go` can see the first half of
//     that (it was taught `class::pseudo` the day before this landed) and
//     cannot see the second: nothing in a sweep for unarrested animations
//     notices a guard that turns off too much.
//
//  3. **A place with nothing in it keeps its printed name.** The three
//     positions are printed on the cloth and the cards land in them, and the
//     stylesheet has said so in prose since 2026-08-18 while the rendered
//     table said it to nobody on a pointer device: the frame gave up its own
//     name because the caption said it, then the caption moved onto hover.
//     Two correct changes, one hole, and no test between them.
//
// All three read the **committed bundle** rather than `web/src`, in
// `cardimagery_test.go`'s idiom and for its reason: what a browser parses is
// the artifact. The minifier writes `::after` as `:after` and folds every
// `rgba()` to a hex, so every pattern below is matched against what the
// bundle actually says.

var (
	// The lamp: the `::after` on either face of a dealt card. Anchored on the
	// pseudo-element and the layer's own two gradients rather than on exact
	// colours, which are a tuning decision and are meant to move.
	//
	// The leading `}` is load-bearing and was bought by a mutation: without it
	// the pattern's first hit is
	// `.tarot-hinge:has(.is-face-up):hover .tarot-face:after{opacity:.4}` --
	// the hover reply, a rule whose body is one declaration -- so deleting the
	// lamp outright reported "draws 0 gradients" against the wrong rule
	// instead of "there is no lamp". In a minified sheet a rule begins at the
	// `}` before it, so that is where the selector has to start.
	tarotLampRule = regexp.MustCompile(
		`\}(\.tarot-face:after)\{([^{}]*)\}`)
	// A `linear-gradient` in the bundle's spelling: the minifier drops
	// `to bottom` and folds `rgba(...)` into `#rrggbbaa`.
	tarotLampGradient = regexp.MustCompile(`linear-gradient\([^()]*#[0-9a-f]{8}`)
	// The glint's own keyframe, and the rule that runs it on a card that has
	// been turned.
	tarotGlintRun = regexp.MustCompile(
		`\.tarot-card\.is-face-up \.tarot-face:before\{[^{}]*animation:[^{}]*tarot-turn-glint`)
	tarotGlintFrames = regexp.MustCompile(`@keyframes tarot-turn-glint\{`)
	// The slip: any rule that lifts `.tarot-legend` out of flow. Its subject
	// is the legend and its body says `position:absolute`.
	legendRule = regexp.MustCompile(`([^{}]*\.tarot-legend)\{([^{}]*)\}`)
	// The turned card's tab stop, as the bundle spells it.
	turnedHingeTabStop = regexp.MustCompile(
		"`tarot-hinge`,role:`img`,tabIndex:0")
)

// reduceBlocks is every `@media (prefers-reduced-motion: reduce)` block in the
// bundle, brace-counted, because the block holds many rules and the first `}`
// is not the end of it. `guardSpans` in reducedmotion_test.go does the same
// job for that file's sweep; this is the one-liner version, kept local so the
// two cannot drift into each other.
func reduceBlocks(css string) []string {
	var out []string
	at := regexp.MustCompile(`@media \(prefers-reduced-motion:\s*reduce\)\s*\{`)
	for _, m := range at.FindAllStringIndex(css, -1) {
		i, depth := m[1], 1
		for i < len(css) && depth > 0 {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		out = append(out, css[m[1]:i])
	}
	return out
}

// TestTheRoomsLightOnACardIsALayerAndNotAFilter: promise 1.
//
// The lamp has to be there, it has to be drawn on a pseudo-element, and the
// rule that draws it may not itself reach for a filter -- which is the one
// shape `cardimagery_test.go` deliberately permits (a filter on a layer
// somebody drew is fine) and which would be wrong *here*, because this
// particular layer is `inset: 0` over a Scryfall painting and a filter on it
// would blur or shift every pixel of that painting just the same.
func TestTheRoomsLightOnACardIsALayerAndNotAFilter(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	// Every rule whose subject is exactly `.tarot-face::after`, because there
	// is more than one: the lamp, and the rule inside the reduced-motion
	// block that drops its transition. The lamp is the one carrying light.
	lamp, most := "", 0
	for _, m := range tarotLampRule.FindAllStringSubmatch(css, -1) {
		if n := len(tarotLampGradient.FindAllString(m[2], -1)); n > most {
			lamp, most = m[2], n
		}
	}
	if most < 2 {
		t.Fatalf("no `.tarot-face::after` rule in the committed bundle draws "+
			"two gradients (the most any of them draws is %d), so the cards on "+
			"the reading table take no light from the room they are lying in. "+
			"The lamp is two layers — the candles over the far edge and the "+
			"table's own shade at the near one — because a card lit from one "+
			"side only reads as a card with a wash on it. Either the lamp was "+
			"removed (and this file goes with it) or web_dist/ is stale; "+
			"rebuild it.", most)
	}
	if strings.Contains(lamp, "filter:") {
		t.Errorf("`.tarot-face::after` declares a filter, and this layer is "+
			"`inset: 0` over a card face — on a Magic crossover that face is "+
			"a Scryfall painting (`.tarot-rws-art`), so a filter here "+
			"re-renders it exactly as a filter on the <img> would. "+
			"Commandment 19: light goes ON a card as a layer of its own.\n"+
			"  body: %s", lamp)
	}
}

// TestTheTurnsGlintIsRemovedUnderReducedMotionAndTheLampIsNot: promise 2.
//
// Both halves, because each without the other is a different bug. An
// unarrested glint is a light sweeping a card for somebody who asked the room
// to hold still. A lamp turned off beside it is the card leaving the table.
func TestTheTurnsGlintIsRemovedUnderReducedMotionAndTheLampIsNot(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !tarotGlintFrames.MatchString(css) || !tarotGlintRun.MatchString(css) {
		t.Fatalf("the committed bundle has no turn glint to check — no " +
			"`@keyframes tarot-turn-glint` run from " +
			"`.tarot-card.is-face-up .tarot-face:before`. Either it was " +
			"removed (delete this test with it) or web_dist/ is stale.")
	}
	arrested, lampOff := false, ""
	for _, block := range reduceBlocks(css) {
		for _, r := range legendlessRules(block) {
			switch {
			case strings.Contains(r.sel, ".tarot-face:before") &&
				strings.Contains(r.body, "animation:none"):
				arrested = true
			case strings.Contains(r.sel, ".tarot-face:after") &&
				(strings.Contains(r.body, "display:none") ||
					strings.Contains(r.body, "background-image:none") ||
					strings.Contains(r.body, "opacity:0")):
				lampOff = r.sel + "{" + r.body + "}"
			}
		}
	}
	if !arrested {
		t.Errorf("no reduced-motion rule turns off the turn glint. It is a " +
			"760ms band of light crossing a card, and somebody who asked for " +
			"reduced motion gets it on every turn. The rule wanted is " +
			"`.tarot-card.is-face-up .tarot-face::before { animation: none; " +
			"opacity: 0 }` inside a `prefers-reduced-motion: reduce` block.")
	}
	if lampOff != "" {
		t.Errorf("a reduced-motion rule puts out the card's lamp: %s\n"+
			"The lamp is not motion. It is the claim that this card is lying "+
			"on that table, which is information about the room, and a reader "+
			"who asked for stillness asked for stillness rather than for a "+
			"flat scan floating over a photograph.", lampOff)
	}
}

// TestAPlaceWithNoPictureInItKeepsItsPrintedName: promise 3.
//
// Stated as the invariant rather than as a list of selectors: **every rule
// that lifts the legend out of flow is gated on the slot holding a face-up
// card.** That is the sentence the stylesheet's prose has been making since
// 2026-08-18, and writing it this way means a fourth or fifth slip rule added
// later cannot quietly opt out of it.
func TestAPlaceWithNoPictureInItKeepsItsPrintedName(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	var ungated []string
	seen := 0
	for _, m := range legendRule.FindAllStringSubmatch(css, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		if !strings.Contains(body, "position:absolute") {
			continue
		}
		seen++
		for _, part := range strings.Split(sel, ",") {
			if !strings.Contains(part, ":has(.is-face-up)") {
				ungated = append(ungated, strings.TrimSpace(part))
			}
		}
	}
	if seen == 0 {
		t.Fatalf("no rule in the committed bundle takes `.tarot-legend` out " +
			"of flow, so there is no hover slip left to gate. Either the slip " +
			"was removed (delete this test with it) or web_dist/ is stale.")
	}
	if len(ungated) > 0 {
		t.Errorf("these rules hide a card's legend behind a pointer without "+
			"asking whether there is a picture in that place yet:\n  - %s\n\n"+
			"A face-down card's legend is ONE line — the position printed on "+
			"the cloth — and it is the whole of what a newcomer has to go on "+
			"before anything is turned over (commandment 2). The sprawl Aaron "+
			"cut in 2026-08-18 was the three lines a FACE-UP card adds. Gate "+
			"the rule on `.tarot-slot:has(.is-face-up)`, and carry "+
			"`:not(.is-small)` with it or the reveal loses a specificity "+
			"point to its own base rule and computes to 0.",
			strings.Join(ungated, "\n  - "))
	}
}

// TestATurnedCardCanBeReachedByAKeyboard: the fourth promise, and the one
// that was a live fault rather than a new flourish.
//
// `index.css` carries `:focus-visible` rules for a turned card -- the zoom
// that is the only way to actually look at a 136px plate, and (through the
// slot's `:focus-within`) the slip carrying the card's name and, on a Magic
// crossover, the line crediting the painting's artist. Every one of them was
// **dead**: they are gated on `:has(.is-face-up)`, the only focusable hinge
// was the `<button>`, and that button exists only while the card is face
// DOWN. A phone was fine -- `(hover: hover)` is false there and every line
// stands in flow -- so the loss was desktop-and-keyboard exactly.
//
// Scryfall's guidelines ask that a reader be able to identify the artist and
// source of an art crop "somehow", which makes this a licence question as
// well as commandment 17's (`cardimagery_test.go` quotes the clause in full).
func TestATurnedCardCanBeReachedByAKeyboard(t *testing.T) {
	t.Parallel()
	found := false
	for _, js := range bundleScripts(t) {
		if turnedHingeTabStop.MatchString(js) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no turned tarot card in the bundle takes a tab stop — " +
			"nothing matching \"`tarot-hinge`,role:`img`,tabIndex:0\" in any " +
			"script. Without it every `:focus-visible` rule this element " +
			"carries in index.css is unreachable, and a keyboard at a desk " +
			"loses the zoom, the card's name and the crossover's artist " +
			"credit. (If the minifier's property order changed, re-read " +
			"components/tarot.tsx and fix the pattern rather than the page.)")
	}
	css := bundleStylesheet(t)
	if !strings.Contains(css, ".tarot-hinge:focus-visible{outline:") {
		t.Errorf("`.tarot-hinge:focus-visible` declares no outline, so a " +
			"keyboard stop on a card has whatever ring the browser draws on " +
			"a dark photographed table — which is the state this repo's own " +
			"`.reader-tile` two rooms over refused. Give it the house ring: " +
			"`outline: 2px solid var(--vine)` with an offset, OUTSIDE the " +
			"card, because nothing is drawn on top of a painting.")
	}
}

// rule is one flattened CSS rule: its selector list and its declarations.
type rule struct{ sel, body string }

// legendlessRules splits a block into rules. Named for what it is not: it does
// no nesting and no at-rules, because it is only ever handed the *inside* of a
// reduced-motion block, which is a flat list of rules in the bundle.
func legendlessRules(block string) []rule {
	var out []rule
	for _, m := range regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`).
		FindAllStringSubmatch(block, -1) {
		out = append(out, rule{strings.TrimSpace(m[1]), m[2]})
	}
	return out
}
