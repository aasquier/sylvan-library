package main

import (
	"regexp"
	"strings"
	"testing"
)

// The fire on the seat whose turn it is, and the light that rises off a player
// who was mended, held to the promises a page cannot show you.
//
// Both landed on 2026-09-27, out of one ask: *"I want the border around the
// active seat in the coliseum to pop more like our buttons, it should look
// like a flame on top, and the whole border should be lit up... I would also
// like an effect for lifegain similar to the blood dripping for damage."* The
// first build of the fire put it on the active commander's **card**, and was
// corrected the same day: *"I didn't mean a fire effect on the commander card
// itself, I meant on their square part of the arena."*
//
// **That correction is the third test in this file, as a gate.** A card is one
// permanent among a dozen and it moves; the seat's square — a half of the sand
// in a duel, a quadrant at four — is a fixed place a watcher finds from across
// a room. Having been told once which box this belongs to, the tree should not
// be able to drift back, and the drift would be easy: `.field-card` is where
// the crown already lives and where the first draft already put the flame.
//
// The mechanisms and their arguments live in `web/src/index.css`, beside the
// sand light they are the loud half of and beside the blood the blessing is
// the sibling of; this file pins the four parts that are invisible to anybody
// looking at the board in front of them.
//
// **Why a file of its own, beside `reducedmotion_test.go`'s sweep.** The same
// reason `buttongleam_test.go` exists: that sweep is a tripwire rather than a
// rendering engine, and this animation has *already* slipped through it once.
// A flame with its own guard deleted read as covered by the crown's guard,
// because the sweep was stapling the pseudo-element to every class in a
// selector rather than to its subject. That is fixed there, and this is the
// belt to those braces — it says the things the sweep structurally cannot:
// that the arrested flame **stays lit**, that the burning ring is lit **all
// the way round**, and that the fire is on the **seat**.
//
// All four read the **committed bundle**, in `cardimagery_test.go`'s idiom and
// for its reason: what a browser parses is the artifact. The minifier writes
// `::before` as `:before` and emits a pre-`color-mix()` fallback copy of every
// rule that uses one, so the patterns below are matched against what the
// bundle actually says.

// seatFlame is the flame's own rule: the tongues standing along the top edge
// of whichever square is on turn, in either layout.
var seatFlame = regexp.MustCompile(
	`\.field-quad-head:before\{[^{}]*animation:[^{}]*seat-burn`)

// flameArrested is the reduced-motion rule for one of the flame's two boxes.
// It has to name the pseudo-element, which is the whole of what the sweep next
// door learned — and there is one of these **per layer**, which is the rest of
// that lesson: a first cut of this accepted `:(?:before|after)` and so let the
// outer layer's guard excuse the inner one's deletion. Proved by mutation on
// 2026-09-27: the ::before guard removed, the bundle rebuilt, and the check
// stayed green on the strength of ::after. Two patterns, two boxes, no
// alternation.
var flameArrested = []*regexp.Regexp{
	regexp.MustCompile(`@media \(prefers-reduced-motion:\s*reduce\)\{[^@]*?` +
		`\.field-quad-head:before\{[^{}]*animation:\s*none`),
	regexp.MustCompile(`@media \(prefers-reduced-motion:\s*reduce\)\{[^@]*?` +
		`\.field-quad-head:after\{[^{}]*animation:\s*none`),
}

// TestTheFireOnTheSeatOnTurnHoldsStillAndDoesNotGoOut.
//
// **Stillness may not answer with less than everybody else gets.** Whose turn
// it is is *information* — three of four seats are waiting at any moment, and
// a person who asked the room to stop moving still has to be able to find the
// one that is not. So the flicker goes and the flame stays, which is the
// crown's own ruling, the turn rim's, and the sand light's.
//
// It is also not the ambience ruling, and the difference is worth keeping
// straight: weather stilled is a smudge because a firefly holding perfectly
// still is a spot. A flame holding still is a flame in a painting.
func TestTheFireOnTheSeatOnTurnHoldsStillAndDoesNotGoOut(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !seatFlame.MatchString(css) {
		t.Fatalf("the committed bundle has no flame on the seat on turn — " +
			"nothing matching `.field-quad-head:before{…animation:…seat-burn`. " +
			"Either it was removed (delete this file with it) or web_dist/ is " +
			"stale; rebuild it.")
	}
	for i, arrested := range flameArrested {
		if arrested.MatchString(css) {
			continue
		}
		t.Errorf("layer %d of the flame on the seat on turn never stops. A "+
			"`prefers-reduced-motion: reduce` block has to name **each** of "+
			"the flame's own pseudo-elements and set `animation: none` on it. "+
			"Naming an ancestor, naming the class without the pseudo-element, "+
			"or naming the other layer, is a guard on a different box — see "+
			"boxesIn in reducedmotion_test.go, which was taught that by this "+
			"very rule.", i+1)
	}
	// ...and it is still drawn. A guard that removed the flame would arrest
	// the animation and pass the check above while taking the answer away.
	for _, guard := range guardBodiesNaming(css, ".field-quad-head:") {
		for _, out := range []string{"display:none", "content:none",
			"opacity:0}", "opacity:0;", "background:none"} {
			if strings.Contains(guard, out) {
				t.Errorf("the reduced-motion guard puts the flame out (%q) "+
					"rather than stilling it: %s\n\nWhose turn it is is "+
					"information, and a reduced-motion reader may not be told "+
					"less than everybody else. Stop the animation; keep the "+
					"fire.", out, guard)
			}
		}
	}
}

// conicFloor is the first colour stop of one of the room's walking rings, as
// the bundle writes it: `conic-gradient(from var(--<name>-sweep) at 50% 50%,
// <stop> 0deg`.
//
// **Anchored on the `}` that ends the rule before it**, because a selector
// pattern that is allowed to start anywhere starts in the middle: an early cut
// of this began at a class in the middle of a descendant selector and so read
// a scoped rule as if the compound in front of it were not there. The reading
// room bought this lesson too, one file over.
var conicFloor = regexp.MustCompile(
	`(?:^|\})([^{}]*)\{[^{}]*` +
		`conic-gradient\(from var\(--(field-turn|field-crown)-sweep\) at 50% 50%,\s*` +
		`([^,]+?)\s+0deg`)

// TestTheSeatOnTurnBurnsAllTheWayRoundAndTheCardKeepsItsTravellingCrown.
//
// Aaron's clause, kept as a gate: *"the whole border should be lit up"* — and
// beside it the thing that clause is **not** about, kept as the other half of
// the same gate.
//
// **Two rings, one mechanism, opposite floors, and the floor is the whole
// difference.** A commander's crown may go almost out between passes, because
// the card is still a commander when the light is elsewhere and the crown
// glyph says so: its conic starts at `transparent` and a short arc walks a
// dark rim. Whose turn it is has nothing else saying it at the scale of a
// seat, so the seat's ring carries colour the whole way round and the walk is
// a brightening across a lit ring rather than an arc appearing on a dark one.
//
// Both halves are checked, because the fault this catches is not "somebody
// deleted the fire" — it is a future session tidying the two gradients into
// one shape and taking the distinction with it.
func TestTheSeatOnTurnBurnsAllTheWayRoundAndTheCardKeepsItsTravellingCrown(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	var half, quad, crown int
	for _, m := range conicFloor.FindAllStringSubmatch(css, -1) {
		selector, which, floor := m[1], m[2], strings.TrimSpace(m[3])
		dark := floor == "transparent"
		if which == "field-crown" {
			if !dark {
				t.Errorf("a commander's crown starts at %q rather than "+
					"transparent. The crown is a travelling highlight that says "+
					"*this card is the commander*; the lit ring is the seat's "+
					"and says *and it is their turn*. Making them the same "+
					"deletes the difference Aaron asked for by name. "+
					"Selector: %s", floor, selector)
			}
			crown++
			continue
		}
		if dark {
			t.Errorf("the ring on the square whose turn it is starts at %q, "+
				"so three quarters of its lap is unlit — which is the crown's "+
				"answer to *is a commander*, not an answer to *and it is their "+
				"turn*. Selector: %s", floor, selector)
		}
		switch {
		case strings.Contains(selector, ".field-side.is-active"):
			half++
		case strings.Contains(selector, ".field-quad.is-on-turn"):
			quad++
		}
	}
	if half == 0 || quad == 0 || crown == 0 {
		t.Fatalf("expected a lit ring on a duel's half and on a pod's "+
			"quadrant, and a travelling crown on the card, and found %d / %d "+
			"/ %d. A board is one layout or the other and both have to "+
			"answer; if one was removed, say so here. Otherwise the bundle is "+
			"stale — rebuild web_dist/.", half, quad, crown)
	}
}

// fireToken is the fire's own material, and the flame's own animation, as the
// bundle writes them.
var fireToken = regexp.MustCompile(`--arena-fire|seat-burn`)

// TestTheFireBelongsToTheSeatRatherThanTheCommanderCard.
//
// **The correction, as a ratchet** (Aaron, 2026-09-27, on the first build of
// this: *"I didn't mean a fire effect on the commander card itself, I meant on
// their square part of the arena."*).
//
// The board answers two different questions with two different marks in two
// different places: *which card is the commander* is the crown, on the card,
// and *whose turn is it* is the fire, on the square. Putting the fire back on
// the card does not add an answer — it moves the loud one onto a box that
// already had a mark of its own, and buries the seat under it.
//
// **It is also the commandment 19 shape.** Every `.field-card` is a Wizards
// painting with a black border, and the more this room paints *on* one the
// closer it stands to the line ADR 48 exists to hold. The seat's square is
// sand: ours to set alight.
func TestTheFireBelongsToTheSeatRatherThanTheCommanderCard(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	rules := regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	var seen int
	for _, m := range rules.FindAllStringSubmatch(css, -1) {
		selector, body := m[1], m[2]
		if !fireToken.MatchString(body) {
			continue
		}
		seen++
		for _, one := range splitSelectorList(selector) {
			if !strings.Contains(stripNegations(one), "field-card") {
				continue
			}
			t.Errorf("a rule that paints in the seat's fire reaches a card:\n"+
				"  %s { %s }\n\nThe fire says *whose turn it is* and its "+
				"subject is the seat's square of the arena — the half in a "+
				"duel, the quadrant at four. The card already has the crown, "+
				"which says the other thing. This was built the wrong way "+
				"round once and corrected by name; see this file's head.",
				one, body)
		}
	}
	if seen == 0 {
		t.Fatal("the committed bundle has no fire at all — no rule naming " +
			"`--arena-fire` or `seat-burn`. Either it was removed (delete " +
			"this file with it) or web_dist/ is stale.")
	}
}

// bloodToken is any of the arena's three reds, as the bundle writes them.
var bloodToken = regexp.MustCompile(`--arena-blood`)

// TestTheBlessingIsNeverTheBloodsColour.
//
// **Up is up** (commandment 2). A life total going up and a life total going
// down are the two things on this plate a newcomer must be able to tell apart
// with nothing learned and no legend, and the separation rests on three
// things: the direction of travel, the word, and the colour. The component
// suite holds the first two — `is-blessed` never arrives with `is-bleeding`,
// the motes are never drips. Nothing but this holds the third, and the third
// is the one a stylesheet edit can lose in a single careless line: the blood
// is right above the blessing in `index.css`, in the same idiom, with its
// tokens spelled almost the same way.
func TestTheBlessingIsNeverTheBloodsColour(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	rules := regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	var seen int
	for _, m := range rules.FindAllStringSubmatch(css, -1) {
		selector, body := m[1], m[2]
		if !strings.Contains(selector, "field-grace") &&
			!strings.Contains(selector, "is-blessed") {
			continue
		}
		seen++
		if bloodToken.MatchString(body) {
			t.Errorf("a rule that dresses a life *gain* reaches for the "+
				"arena's blood:\n  %s { %s }\n\nA player who was healed may "+
				"never be painted the colour of a player who was hit. The "+
				"tokens for this are `--arena-grace-*`, declared beside the "+
				"blood with the argument for them.", selector, body)
		}
	}
	if seen == 0 {
		t.Fatal("the committed bundle dresses no life gain at all — no rule " +
			"naming `field-grace` or `is-blessed`. Either the blessing was " +
			"removed (delete this file with it) or web_dist/ is stale.")
	}
}

// guardBodiesNaming returns the body of every rule inside a
// `prefers-reduced-motion: reduce` block whose selector contains `want`.
func guardBodiesNaming(css, want string) []string {
	var out []string
	for _, span := range guardSpans(css) {
		block := css[span[0]:span[1]]
		for _, m := range regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`).
			FindAllStringSubmatch(block, -1) {
			if strings.Contains(m[1], want) {
				out = append(out, m[0])
			}
		}
	}
	return out
}
