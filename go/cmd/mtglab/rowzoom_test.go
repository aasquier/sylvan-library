package main

import (
	"regexp"
	"strings"
	"testing"
)

// The 99's second reply to a hand: weight, after light.
//
// # What was asked for
//
// A row in the 99 answered a pointer with a wash, a warmed hairline and a
// ring walking its edge (`cardpeek_test.go` holds that reply). Aaron liked it
// and asked for more (2026-10-06): *"I would also like a zoom or accordion
// like effect as you move over them."* Both words are one lever in the
// stylesheet, and it is a **width** -- `.deck-card-art` grows from 4rem to
// 6.5rem under the row's hover, the painting is drawn larger (the zoom), the
// row gets taller with it and the rows below slide down (the accordion).
//
// # The one thing the rule may never do, and why a guard holds it
//
// The obvious zoom is `transform: scale()` on the row, and it is a bug the
// moment it lands: the card `CardHover` holds up beside the cursor is
// `position: fixed` and rendered *inside* the row, and any computed transform
// on an ancestor makes that ancestor the containing block for a fixed
// descendant. The token shelf shipped exactly that and `fixedoverlay_test.go`
// carries the measurement -- a preview placed against a 340px plate instead
// of the window, off the bottom of the screen. That guard watches animation
// fills; this one watches the hover rule itself, because "make it zoom" is the
// request that reaches for `scale()` first, and the suite has no layout to
// notice the preview leaving the screen.
//
// # The two gates
//
// The growth sits behind `(hover: hover)` -- a phone's `:hover` lands on a tap
// and stays, and a row that stays grown after the thumb has gone is a list
// that has stopped lining up -- and behind `(prefers-reduced-motion:
// no-preference)`, because a row growing under a pointer is layout moving and
// the rows under it moving is more of it. The light stays for everyone; the
// motion is for the hand that moves and the reader who did not ask for
// stillness.
//
// Read off the committed bundle rather than the source, like its neighbours,
// so a stale `web_dist/` fails here rather than shipping the old row.

// rowHoverRule is every rule in the bundle whose selector lights a row in the
// 99 under a hand -- the same shape `cardpeek_test.go` sweeps -- with its body
// captured, because this file has an opinion about what is *in* the rule.
var rowHoverRule = regexp.MustCompile(
	`(?:^|\})([^{}]*\.deck-card-row[^{}]*:is\(:hover,:focus-within\)[^{}]*)\{([^{}]*)\}`)

// rowArtGrows is the growth itself: the painting's class, inside the row's
// hover selector, given a larger width than it rests at.
var rowArtGrows = regexp.MustCompile(
	`\.deck-card-row:not\(\.action-pick\):not\(\.entombing\):is\(:hover,:focus-within\) \.deck-card-art\{width:6\.5rem\}`)

// rowArtRests is the width the painting rests at, which has to be the 64px
// `CardFacePlate` used to be handed as `w-16` -- the class replaced a utility,
// and a row that rests at a different size than before is a different list.
var rowArtRests = regexp.MustCompile(`\.deck-card-art\{width:4rem\}`)

// handsArtClass is the deck page passing the class to the plate, in whichever
// quotes the minifier chose.
var handsArtClass = regexp.MustCompile("artClassName:[`\"']deck-card-art[`\"']")

func TestTheRowGrowsItsPaintingUnderAHandAndNeverMoves(t *testing.T) {
	t.Parallel()
	css := bundleStylesheet(t)

	if !rowArtRests.MatchString(css) {
		t.Fatal("no `.deck-card-art{width:4rem}` in the committed bundle. The 99's " +
			"painting rests at the 64px it has always been drawn at; either the " +
			"class moved, the resting width changed, or web_dist/ is stale.")
	}
	grow := rowArtGrows.FindStringIndex(css)
	if grow == nil {
		t.Fatal("the 99's painting no longer grows under the row's hover " +
			"(`.deck-card-row…:is(:hover,:focus-within) .deck-card-art{width:6.5rem}`). " +
			"The zoom and the accordion are both that one width; if it was " +
			"meant to go, delete this file with it, otherwise rebuild web_dist/.")
	}

	// The gate the growth sits behind is whichever @media encloses it. Found
	// by walking back from the rule to the nearest `@media` and checking that
	// its block has not closed before the rule begins -- a brace count, since
	// the bundle nests `@supports` inside it.
	media := strings.LastIndex(css[:grow[0]], "@media")
	if media < 0 {
		t.Fatal("the painting's growth is not inside any `@media` at all")
	}
	between := css[media:grow[0]]
	if strings.Count(between, "{")-strings.Count(between, "}") < 1 {
		t.Fatal("the painting's growth sits outside the `@media` that precedes it")
	}
	query := between[:strings.Index(between, "{")]
	for _, gate := range []string{"(hover:hover)", "(prefers-reduced-motion:no-preference)"} {
		if !strings.Contains(query, gate) {
			t.Errorf("the painting's growth is gated by `%s`, which does not name "+
				"`%s`. A phone's :hover is sticky and a reader who asked for "+
				"stillness asked for the rows not to move; both gates are the "+
				"rule's, not optional.", strings.TrimSpace(query), gate)
		}
	}

	hits := rowHoverRule.FindAllStringSubmatch(css, -1)
	if len(hits) == 0 {
		t.Fatal("no `.deck-card-row` hover/focus rule in the committed bundle")
	}
	for _, hit := range hits {
		body := hit[2]
		if strings.Contains(body, "transform:") {
			t.Errorf("`%s` carries a `transform` (`%s`). The card CardHover holds up "+
				"beside the cursor is position:fixed and rendered inside this row, "+
				"and a transform on the row becomes its containing block -- the "+
				"preview lands against the row instead of the window "+
				"(fixedoverlay_test.go has the measurement). The zoom is a width "+
				"on `.deck-card-art`; the lift is a box-shadow. Neither moves "+
				"the row.", strings.TrimSpace(hit[1]), strings.TrimSpace(body))
		}
	}
}

// TestThe99HandsItsPlateTheGrowingWidth.
//
// The stylesheet can only grow a class the markup wears. `CardFacePlate`
// defaults its painting to `w-16`, which is the same 64px and which no rule
// can reach; the deck page has to hand it `deck-card-art` by name, and a
// refactor that drops the prop back to the default leaves every rule above
// true and the row perfectly still. Read off the page's own chunk.
func TestThe99HandsItsPlateTheGrowingWidth(t *testing.T) {
	t.Parallel()
	scripts := bundleScripts(t)
	var page string
	for name, body := range scripts {
		if strings.HasPrefix(name, "DeckDetail") {
			page = body
		}
	}
	if page == "" {
		t.Fatal("no DeckDetail chunk in web_dist/assets; the route's chunk was renamed")
	}
	// The minifier picks the quote -- backticks today -- so the prop is
	// matched by name and value with whatever stands between them.
	if !handsArtClass.MatchString(page) {
		t.Error("the deck page no longer hands CardFacePlate `artClassName=\"deck-card-art\"` " +
			"for the rows of the 99. Without it the plate rests at its default " +
			"`w-16` and the zoom has nothing to grow. Then rebuild web_dist/.")
	}
}
