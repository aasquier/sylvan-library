package deckread

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The pure helpers every deck payload is assembled from.
//
// They look small enough not to need tests, and two of them decide something
// a reader would notice. `WithArt` is the rule that a chosen printing changes
// **the picture, the painter and the flavour text and never the card's own
// text** -- an override that leaked into `oracle_text` would be a card whose
// rules text came from the wrong printing, which is exactly the class of
// error CLAUDE.md's rule 1 exists to prevent. And `TypeParts` is what every
// tribal and type-matters read rests on, over a type line that may carry any
// of three dashes and a back face nobody asked about.

// A chosen printing changes what the card looks like, never what it does.
func TestAChosenPrintingChangesThePictureAndNotTheRules(t *testing.T) {
	t.Parallel()
	oracle := "Add {C}{C}."
	typeLine := "Artifact"
	crop := "https://example.invalid/original-crop.jpg"
	row := CardJSON{
		Name: "Sol Ring", Known: true, full: true,
		Image: strPtr("https://example.invalid/original.jpg"), ArtCrop: &crop,
		OracleText: &oracle, TypeLine: &typeLine,
		Artist: strPtr("The First Painter"), FlavorText: strPtr("The original flavour."),
	}

	newCrop := "https://example.invalid/chosen-crop.jpg"
	out := WithArt(row, map[string]ChosenArt{"Sol Ring": {
		Image: strPtr("https://example.invalid/chosen.jpg"), ArtCrop: &newCrop,
		Artist: strPtr("The Chosen Painter"), FlavorText: strPtr("The chosen flavour."),
	}})

	if out.Image == nil || *out.Image != "https://example.invalid/chosen.jpg" {
		t.Errorf("the picture is %v", out.Image)
	}
	if out.ArtCrop == nil || *out.ArtCrop != newCrop {
		t.Errorf("the crop is %v", out.ArtCrop)
	}
	if out.Artist == nil || *out.Artist != "The Chosen Painter" {
		t.Errorf("the painter is %v", out.Artist)
	}
	if out.FlavorText == nil || *out.FlavorText != "The chosen flavour." {
		t.Errorf("the flavour is %v", out.FlavorText)
	}

	// **Never the card's own text.** A printing does not change what a card
	// does, and a row that said otherwise would be a rules error on the page.
	if out.OracleText == nil || *out.OracleText != oracle {
		t.Errorf("the chosen printing changed the oracle text to %v", out.OracleText)
	}
	if out.TypeLine == nil || *out.TypeLine != typeLine {
		t.Errorf("the chosen printing changed the type line to %v", out.TypeLine)
	}
}

// A printing chosen for a card the pool does not know is ignored: there is
// nothing to dress, and dressing it would put a picture on a row whose text
// nobody could look up.
func TestAnOverrideOnAnUnknownCardIsIgnored(t *testing.T) {
	t.Parallel()
	unknown := CardJSON{Name: "Not A Real Card", Known: false}
	out := WithArt(unknown, map[string]ChosenArt{"Not A Real Card": {
		Image: strPtr("https://example.invalid/chosen.jpg")}})
	if out.Image != nil {
		t.Errorf("an unknown card was dressed with %q", *out.Image)
	}

	// And a card nobody chose a printing for keeps what it had.
	kept := CardJSON{Name: "Sol Ring", Known: true,
		Image: strPtr("https://example.invalid/default.jpg")}
	out = WithArt(kept, map[string]ChosenArt{"Something Else": {
		Image: strPtr("https://example.invalid/chosen.jpg")}})
	if out.Image == nil || *out.Image != "https://example.invalid/default.jpg" {
		t.Errorf("an unrelated override changed the picture to %v", out.Image)
	}
	// No overrides at all is the common case.
	if got := WithArt(kept, nil); got.Image != kept.Image {
		t.Errorf("no overrides changed the picture to %v", got.Image)
	}
}

// A short row -- the one a list renders -- does not carry the painter or the
// flavour, so an override must not add them: they would be fields the
// recorded shape does not have.
func TestAShortRowGainsNoPainterFromAnOverride(t *testing.T) {
	t.Parallel()
	short := CardJSON{Name: "Sol Ring", Known: true, full: false}
	out := WithArt(short, map[string]ChosenArt{"Sol Ring": {
		Image:  strPtr("https://example.invalid/chosen.jpg"),
		Artist: strPtr("The Chosen Painter"), FlavorText: strPtr("Flavour."),
	}})
	if out.Image == nil || *out.Image != "https://example.invalid/chosen.jpg" {
		t.Errorf("the picture is %v", out.Image)
	}
	if out.Artist != nil || out.FlavorText != nil {
		t.Errorf("a short row gained a painter (%v) and flavour (%v)", out.Artist, out.FlavorText)
	}
}

// An override with no crop of its own keeps the row's, rather than blanking
// the field -- a card page with no crop falls back to a blank hero.
func TestAnOverrideWithNoCropKeepsTheOneItHad(t *testing.T) {
	t.Parallel()
	crop := "https://example.invalid/original-crop.jpg"
	row := CardJSON{Name: "Sol Ring", Known: true, ArtCrop: &crop}
	out := WithArt(row, map[string]ChosenArt{"Sol Ring": {
		Image: strPtr("https://example.invalid/chosen.jpg")}})
	if out.ArtCrop == nil || *out.ArtCrop != crop {
		t.Errorf("the crop became %v", out.ArtCrop)
	}
}

// The type line splits on any of three dashes and reads the front face only,
// because Scryfall's combined `A // B` line would otherwise put the back
// face's types into a tribal count.
func TestTheTypeLineSplitsOnEveryDashAndReadsTheFrontFace(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		in              string
		types, subtypes []string
	}{
		{"an em dash", "Legendary Creature — Cat Warrior",
			[]string{"Legendary", "Creature"}, []string{"Cat", "Warrior"}},
		{"an en dash", "Legendary Creature – Cat Warrior",
			[]string{"Legendary", "Creature"}, []string{"Cat", "Warrior"}},
		{"a spaced hyphen", "Legendary Creature - Cat Warrior",
			[]string{"Legendary", "Creature"}, []string{"Cat", "Warrior"}},
		{"no subtypes at all", "Artifact", []string{"Artifact"}, nil},
		{"a sorcery", "Sorcery", []string{"Sorcery"}, nil},
		// The front face only: a combined line's back face is a different
		// card as far as a decklist is concerned.
		{"a double-faced card", "Creature — Human // Creature — Werewolf",
			[]string{"Creature"}, []string{"Human"}},
		{"a modal DFC", "Land // Instant", []string{"Land"}, nil},
		{"surrounding space", "  Artifact — Equipment  ",
			[]string{"Artifact"}, []string{"Equipment"}},
		{"empty", "", nil, nil},
		// A hyphenated subtype is one word, not a split point: only a
		// SPACED hyphen separates.
		{"a hyphenated name", "Creature — Beast-Rider",
			[]string{"Creature"}, []string{"Beast-Rider"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			types, subtypes := TypeParts(tc.in)
			// Never nil: both are ranged over.
			if types == nil || subtypes == nil {
				t.Fatalf("%q split to %v / %v, and both must be lists", tc.in, types, subtypes)
			}
			if !sameStrings(types, tc.types) {
				t.Errorf("%q gave types %v, want %v", tc.in, types, tc.types)
			}
			if !sameStrings(subtypes, tc.subtypes) {
				t.Errorf("%q gave subtypes %v, want %v", tc.in, subtypes, tc.subtypes)
			}
		})
	}
}

// A price that is nothing is null rather than zero, because a card with no
// recorded price and a card that is free are different facts and a shopping
// list would total them the same way.
func TestAnAbsentPriceIsNullRatherThanZero(t *testing.T) {
	t.Parallel()
	if got := Deref(nil); got != nil {
		t.Errorf("no price rendered as %#v", got)
	}
	zero := 0.0
	if got := Deref(&zero); got != 0.0 {
		t.Errorf("a zero price rendered as %#v", got)
	}
	price := 1.25
	if got := Deref(&price); got != 1.25 {
		t.Errorf("a price rendered as %#v", got)
	}
}

// The driver hands numbers back in whichever width the column had, so the
// reading takes all four rather than the one this machine happened to see.
func TestAPriceReadsInEveryWidthTheDriverHandsBack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want float64
	}{
		{"a float64", float64(1.25), 1.25},
		{"a float32", float32(1.5), 1.5},
		{"an int32", int32(3), 3},
		{"an int64", int64(3), 3},
	} {
		got := AsFloatPtr(tc.in)
		if got == nil {
			t.Errorf("%s read as nothing", tc.name)
			continue
		}
		if *got != tc.want {
			t.Errorf("%s read as %v, want %v", tc.name, *got, tc.want)
		}
	}

	// Anything else is nothing rather than zero -- a NULL column and a zero
	// price are different facts.
	for _, bad := range []any{nil, "1.25", true, int(3), []byte("1.25")} {
		if got := AsFloatPtr(bad); got != nil {
			t.Errorf("%#v read as %v", bad, *got)
		}
	}
}

func strPtr(s string) *string { return &s }

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// **The mark travels to the page, and only when there is one.**
//
// `why_by` is written into `deck.yaml` by `dump.go` and was then dropped on
// the way out: `CardRow` never copied it and `CardJSON` had no field for it,
// so the file knew which sentences Claude drafted and the deck page could not.
// Since a drafted `why` satisfies `curated` (Aaron, 2026-08-28), this mark is
// the only thing carrying that difference.
//
// Both directions are asserted. A mark that never appears and a mark that
// appears on everything are the same bug to a reader, and `omitempty` means
// the second one is a wire question rather than a rendering one.
func TestARationaleSaysWhoDraftedIt(t *testing.T) {
	t.Parallel()

	drafted := CardRow(deck.CardEntry{
		Name: "Sol Ring", Category: "ramp", Why: "Two mana on turn one.",
		WhyBy: "claude", Qty: 1,
	}, nil, false)
	if drafted.WhyBy != "claude" {
		t.Errorf("a drafted rationale reached the page as %q, want %q",
			drafted.WhyBy, "claude")
	}

	written := CardRow(deck.CardEntry{
		Name: "Cultivate", Category: "ramp", Why: "Fixes and ramps.", Qty: 1,
	}, nil, false)
	if written.WhyBy != "" {
		t.Errorf("a rationale nobody marked carries %q", written.WhyBy)
	}

	// And the wire keeps the difference: a mark is a thing that is *there*,
	// so the unmarked row must not carry an empty one for a reader to weigh.
	body, err := json.Marshal(written)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(body, []byte("why_by")) {
		t.Errorf("an unmarked rationale still serialised why_by: %s", body)
	}
	marked, err := json.Marshal(drafted)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(marked, []byte(`"why_by":"claude"`)) {
		t.Errorf("a drafted rationale did not serialise its mark: %s", marked)
	}
}
