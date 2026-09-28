package main

import (
	"regexp"
	"strings"
	"testing"
)

// The fire on the commander whose turn it is, and the light that rises off a
// player who was mended, held to the promises a page cannot show you.
//
// Both landed on 2026-09-27, out of one ask: *"I want the border around the
// active commander in the coliseum to pop more like our buttons, it should
// look like a flame on top, and the whole border should be lit up... I would
// also like an effect for lifegain similar to the blood dripping for damage."*
// The mechanisms and their arguments live in `web/src/index.css`, beside the
// crown they are modelled on and beside the blood they are the sibling of;
// this file pins the three parts that are invisible to anybody looking at the
// board in front of them.
//
// **Why a file of its own, beside `reducedmotion_test.go`'s sweep.** The same
// reason `buttongleam_test.go` exists: that sweep is a tripwire rather than a
// rendering engine, and this animation has *already* slipped through it once.
// A flame on `.field-card::before` with its own guard deleted read as covered
// by the crown's guard on `.field-card-turn::before`, because the sweep was
// stapling the pseudo-element to every class in a selector rather than to its
// subject. That is fixed there, and this is the belt to that braces — and it
// says the two things the sweep structurally cannot: that the arrested flame
// **stays lit**, and that the burning ring is lit **all the way round**.
//
// All three read the **committed bundle**, in `cardimagery_test.go`'s idiom
// and for its reason: what a browser parses is the artifact. The minifier
// writes `::before` as `:before` and emits a pre-`color-mix()` fallback copy
// of every rule that uses one, so the patterns below are matched against what
// the bundle actually says.

// burningFlame is the flame's own rule: the two pseudo-elements of a
// commander standing on the active seat's sand.
var burningFlame = regexp.MustCompile(
	`\.field-side\.is-active \.field-card\.is-commander:before\{[^{}]*animation:[^{}]*field-burn`)

// flameArrested is the reduced-motion rule for those two boxes. It has to name
// the pseudo-element, which is the whole of what the sweep next door learned.
var flameArrested = regexp.MustCompile(
	`@media \(prefers-reduced-motion:\s*reduce\)\{[^@]*?` +
		`\.field-side\.is-active \.field-card\.is-commander:(?:before|after)` +
		`\{[^{}]*animation:\s*none`)

// TestTheFireOnACommanderOnTurnHoldsStillAndDoesNotGoOut.
//
// **Stillness may not answer with less than everybody else gets.** Which
// commander is on turn is *information* — three of four seats are waiting at
// any moment, and a person who asked the room to stop moving still has to be
// able to find the one that is not. So the flicker goes and the flame stays,
// which is the crown's own ruling one block up in the stylesheet and the turn
// rim's below it.
//
// It is also not the ambience ruling, and the difference is worth keeping
// straight: weather stilled is a smudge because a firefly holding perfectly
// still is a spot. A flame holding still is a flame in a painting.
func TestTheFireOnACommanderOnTurnHoldsStillAndDoesNotGoOut(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !burningFlame.MatchString(css) {
		t.Fatalf("the committed bundle has no flame on the commander on turn " +
			"— nothing matching `.field-side.is-active .field-card.is-commander" +
			":before{…animation:…field-burn`. Either it was removed (delete " +
			"this file with it) or web_dist/ is stale; rebuild it.")
	}
	if !flameArrested.MatchString(css) {
		t.Errorf("the flame on the commander on turn never stops. A " +
			"`prefers-reduced-motion: reduce` block has to name " +
			"`.field-side.is-active .field-card.is-commander::before` (and " +
			"::after) and set `animation: none` on it. Naming an ancestor, or " +
			"naming the class without the pseudo-element, is a guard on a " +
			"different box — see boxesIn in reducedmotion_test.go, which was " +
			"taught that by this very rule.")
	}
	// ...and it is still drawn. A guard that removed the flame would arrest
	// the animation and pass the check above while taking the answer away.
	for _, guard := range guardBodiesNaming(css,
		".field-side.is-active .field-card.is-commander:") {
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

// litFloor is the first colour stop of a conic gradient, as the bundle writes
// it: `conic-gradient(from var(--field-crown-sweep) at 50% 50%, <stop> 0deg`.
//
// **Anchored on the `}` that ends the rule before it**, because a selector
// pattern that is allowed to start anywhere starts in the middle: the first
// cut of this began at `.field-card.is-commander` and so read the *burning*
// rule's selector as if the `.field-side.is-active` in front of it were not
// there, then failed the plain-crown branch on the fire's own gradient. The
// reading room bought this lesson too, one file over.
var litFloor = regexp.MustCompile(
	`(?:^|\})([^{}]*\.field-card\.is-commander[^{}]*)\{[^{}]*` +
		`conic-gradient\(from var\(--field-crown-sweep\) at 50% 50%,\s*` +
		`([^,]+?)\s+0deg`)

// TestTheWholeBorderBurnsRatherThanOneArcOfIt.
//
// Aaron's clause, kept as a gate: *"the whole border should be lit up"*.
//
// **The crown and the fire are the same mechanism saying two different
// things, and the floor of the gradient is the whole difference.** A
// commander's crown may go almost out between passes, because the card is
// still a commander when the light is elsewhere and the crown glyph says so —
// its conic starts at `transparent`. Whose turn it is has nothing else saying
// it on the card at all, so the fire's floor carries colour the whole way
// round and the walk is a brightening across a lit ring rather than an arc
// appearing on a dark one. `.field-quad.is-on-turn` made exactly this move at
// the scale of a seat and its comment argues it.
//
// Both halves are checked, because the fault this catches is not "somebody
// deleted the fire" — it is a future session tidying the two gradients into
// one shape and taking the distinction with it.
func TestTheWholeBorderBurnsRatherThanOneArcOfIt(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	var burning, plain int
	for _, m := range litFloor.FindAllStringSubmatch(css, -1) {
		selector, floor := m[1], strings.TrimSpace(m[2])
		lit := strings.Contains(selector, ".field-side.is-active")
		dark := floor == "transparent"
		switch {
		case lit && dark:
			t.Errorf("the commander on turn's border starts at %q, so three "+
				"quarters of its lap is unlit — which is the crown's answer "+
				"to *is a commander*, not an answer to *and it is their "+
				"turn*. Selector: %s", floor, selector)
		case lit:
			burning++
		case dark:
			plain++
		default:
			t.Errorf("an ordinary commander's crown starts at %q rather than "+
				"transparent. The crown is a travelling highlight and the "+
				"fire is a lit ring; making them the same deletes the only "+
				"thing on the card that says whose turn it is. Selector: %s",
				floor, selector)
		}
	}
	if burning == 0 || plain == 0 {
		t.Fatalf("expected both rings in the committed bundle and found "+
			"%d burning / %d plain. Either one was removed or the bundle is "+
			"stale; rebuild web_dist/.", burning, plain)
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
