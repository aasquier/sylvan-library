package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The card held up, and the row it came out of.
//
// Aaron, 2026-09-27: *"I would like card hover previews to also have a border
// effect along the lines of what we have been doing. When a 99 is hovered over
// besides the card art preview it would be nice if the box for the card was
// highlighted subtly and also had a border with an effect."*
//
// Both wear the button family's gleam — the masked conic ring from
// `buttongleam_test.go`'s subject — and both are checked here rather than
// there because neither is a control and the promises are different ones:
// a preview is lit from birth and has no hover to wait for, a row has a
// `:focus-within` where a button has `:focus-visible`, and one of the two is
// drawn a hair's breadth from somebody else's painting.
//
// Read against the **committed bundle**, in `cardimagery_test.go`'s idiom and
// for its reason: what a browser parses is the artifact. The minifier writes
// `::before` as `:before` and emits a pre-`color-mix()` fallback copy of every
// rule that uses one, so a pattern that has to see a colour must expect to
// find the declaration twice.

// peekRingStandsOff is the one declaration that keeps the light off the card:
// the ring's box is pushed outside the printing rather than laid on it.
var peekRingStandsOff = regexp.MustCompile(`\.card-peek:before\{[^{}]*inset:\s*-3px`)

// peekRing is the ring existing at all, for the anti-vacuity guard every test
// below opens with.
var peekRing = regexp.MustCompile(`\.card-peek:before\{`)

// rowRingArrested and peekRingArrested are the reduced-motion rule naming each
// of the two new subjects. One pattern each rather than one naming both,
// because a single pattern lets either subject's guard excuse the other's
// deletion — the over-approximation this repository has now bought twice in
// two days, once in `boxesIn` and once in the arena's flame.
var rowRingArrested = regexp.MustCompile(
	`@media \(prefers-reduced-motion:\s*reduce\)\{[^{}]*\.deck-card-row:before[^{}]*\{[^{}]*animation-name:\s*none`)
var peekRingArrested = regexp.MustCompile(
	`@media \(prefers-reduced-motion:\s*reduce\)\{[^{}]*\.card-peek:before[^{}]*\{[^{}]*animation-name:\s*none`)

// TestTheHeldUpCardsRingIsDrawnOutsideThePrinting.
//
// **This is commandment 19 at the one place it is easiest to get wrong.** The
// ring is a layer rather than a `filter`, which `cardimagery_test.go` now
// holds (`card-peek` is in `artBearing`); this holds the other half, which no
// sweep would catch because it is not a violation of anything — it is a taste
// call that happens to also be the compliant one. At `inset: 0` the light sits
// **on** the card: two pixels of our gold over the cardstock's own black
// border, at the very edge of a painting that is lent to us. At `-3px` it
// stands a pixel clear and the printing is untouched by construction.
//
// The number is not the point and a different offset is not a failure of
// taste — what fails is a *zero or positive* inset, which is the light
// arriving on the card. The pattern is written against the declaration that
// exists so that the fix, if somebody ever reaches for the obvious
// `inset: 0`, is named rather than merely wrong.
func TestTheHeldUpCardsRingIsDrawnOutsideThePrinting(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !peekRing.MatchString(css) {
		t.Fatalf("the committed bundle has no `.card-peek:before` in it. Either " +
			"the held-up card's frame was removed (delete this file with it) or " +
			"web_dist/ is stale; rebuild it.")
	}
	if !peekRingStandsOff.MatchString(css) {
		t.Errorf("`.card-peek::before` no longer pushes its box outside the card " +
			"(`inset: -3px`). A ring at inset 0 or better is drawn ON the " +
			"printing — two pixels of our own light over the cardstock's black " +
			"border, at the edge of a painting that is lent to us. Commandment " +
			"19 and Scryfall's terms are about what reaches the card; the way " +
			"not to reach it is not to touch it. Then rebuild web_dist/.")
	}
}

// TestTheRowAndTheHeldUpCardBothStopWalkingUnderReducedMotion.
//
// Neither light may keep orbiting for somebody who asked the room to hold
// still, and neither may go out: `--btn-gleam` is registered with an initial
// value, so an arrested lap parks on the same shoulder every time rather than
// wherever it happened to be. A frozen mid-sweep is a smudge; an arc at rest
// is a highlight. The ruling is the button family's and these two inherit it
// by being in the same rule — which is exactly why it has to be checked, since
// being in a rule is not the same as being named in it.
func TestTheRowAndTheHeldUpCardBothStopWalkingUnderReducedMotion(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !peekRing.MatchString(css) {
		t.Fatal("no `.card-peek:before` in the committed bundle; nothing to arrest")
	}
	if !rowRingArrested.MatchString(css) {
		t.Error("a row in the 99 keeps walking its light under " +
			"`prefers-reduced-motion: reduce`. `.deck-card-row::before` has to be " +
			"named in the gleam's reduced-motion rule in `web/src/index.css` — " +
			"`animation-name: none`, the longhand, because the hover rule raises " +
			"duration and play-state at a higher specificity. Then rebuild " +
			"web_dist/.")
	}
	if !peekRingArrested.MatchString(css) {
		t.Error("the held-up card's frame keeps walking its light under " +
			"`prefers-reduced-motion: reduce`, and this is the louder of the two: " +
			"it is lit from birth rather than on hover, so it is the one a reader " +
			"who asked for stillness meets without reaching for anything. Name " +
			"`.card-peek::before` in the gleam's reduced-motion rule and rebuild " +
			"web_dist/.")
	}
}

// TestEveryColourIdentityHasAVoiceInTheHeldUpCardsFrame.
//
// The frame's material is the card's own colour identity, because that is how
// the game itself frames a card: one colour in that colour, two or more in
// gold, none at all in the artifact frame's grey. A missing voice is not a
// crash and not a wrong colour — it is one identity quietly falling through to
// the vine while its four neighbours are dressed, which is the kind of gap
// nobody finds by looking at the page they happen to be on.
//
// **The list is discovered rather than retyped.** The five come from
// [reference.WUBRG], which is the server's own canonical order and the same
// constant the pips are drawn from; the two others are the classes
// `peekVoice` adds in `web/src/components/ui.tsx`, read out of that function
// so that a sixth voice invented there fails here rather than going unchecked.
func TestEveryColourIdentityHasAVoiceInTheHeldUpCardsFrame(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)
	if !peekRing.MatchString(css) {
		t.Fatal("no `.card-peek:before` in the committed bundle; nothing to dress")
	}

	want := map[string]string{}
	for _, colour := range strings.ToLower(reference.WUBRG) {
		want[string(colour)] = "the " + string(colour) + " identity, off --mtg-" +
			string(colour)
	}
	for _, extra := range peekExtraVoices(t) {
		want[extra] = "a class `peekVoice` in web/src/components/ui.tsx emits"
	}
	if len(want) != 7 {
		t.Fatalf("expected the five colours plus colourless and gold, got %d "+
			"voices to check: %v", len(want), want)
	}

	for voice, why := range want {
		rule := regexp.MustCompile(`\.card-peek\.is-` + voice + `\{[^{}]*--peek-ink:`)
		if !rule.MatchString(css) {
			t.Errorf("`.card-peek.is-%s` sets no `--peek-ink` in the committed "+
				"bundle (%s). That identity falls through to the vine while the "+
				"others are dressed in their own colours, which reads as the frame "+
				"being broken for exactly one kind of card.", voice, why)
		}
	}
}

// peekExtraVoices is the voices `peekVoice` emits that are not one of the five
// colours: the artifact frame's grey and multicolour's gold today. Read out of
// the function rather than written down here, so that a sixth one invented
// there is checked rather than silently unchecked.
func peekExtraVoices(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "src", "components", "ui.tsx"))
	if err != nil {
		t.Fatalf("reading web/src/components/ui.tsx: %v", err)
	}
	body := string(raw)
	start := strings.Index(body, "function peekVoice(")
	if start < 0 {
		t.Fatal("web/src/components/ui.tsx no longer declares `function " +
			"peekVoice(`. That function is where a card's colour identity becomes " +
			"a material, and without reading it this guard cannot know which " +
			"voices exist.")
	}
	end := strings.Index(body[start:], "\n}")
	if end < 0 {
		t.Fatal("could not find the end of `peekVoice` in web/src/components/ui.tsx")
	}
	seen := map[string]bool{}
	var out []string
	for _, hit := range peekVoiceLiteral.FindAllStringSubmatch(body[start:start+end], -1) {
		if len(hit[1]) != 1 || strings.ContainsAny(hit[1], "wubrg") {
			continue
		}
		if !seen[hit[1]] {
			seen[hit[1]] = true
			out = append(out, hit[1])
		}
	}
	return out
}

// peekVoiceLiteral is a voice class written out in `peekVoice`: `'is-c'`,
// `'is-m'`. The template form (“ `is-${one}` “) is the five colours and is
// covered by [reference.WUBRG] instead.
var peekVoiceLiteral = regexp.MustCompile(`'is-([a-z]+)'`)

// TestAPickableRowDoesNotAlsoWearTheQuietLight.
//
// Two lights on one box saying two different things is worse than one. In pick
// mode a row **is** a control — `.action-pick`, with its own nudge right and
// its own coloured edge, tuned when the action bar was built — and a row on
// its way to the graveyard is being put out rather than lit. Both stand the
// quiet ring down, and both negations have to be on the state rule *and* on
// the wash, because losing either one is how a row ends up half-answering.
func TestAPickableRowDoesNotAlsoWearTheQuietLight(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)

	rules := regexp.MustCompile(`(?:^|\})([^{}]*\.deck-card-row[^{}]*:is\(:hover,:focus-within\)[^{}]*)\{`)
	found := rules.FindAllStringSubmatch(css, -1)
	if len(found) == 0 {
		t.Fatal("no `.deck-card-row` hover/focus rule in the committed bundle — " +
			"the row's whole reply to a hand is gone, or web_dist/ is stale.")
	}
	for _, hit := range found {
		selector := strings.TrimSpace(hit[1])
		for _, stood := range []string{":not(.action-pick)", ":not(.entombing)"} {
			if !strings.Contains(selector, stood) {
				t.Errorf("`%s` answers a hand without standing down for `%s`. A row "+
					"in pick mode already has a louder answer of its own, and a row "+
					"being entombed is being put out rather than lit; a rule that "+
					"forgets one of them lights a box that is already saying "+
					"something else.", selector, stood)
			}
		}
	}
}
