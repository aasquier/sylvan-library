package deck

import (
	"fmt"
	"strings"
	"testing"
)

// The coercions a hand-written deck file goes through, and the refusals it
// meets when a value is not one.
//
// **These are the shapes the app never writes.** `mtglab decks build` emits
// `qty: 3` and `bracket: 3` and nothing else, so every branch below exists for
// a file a person typed -- an import pasted out of a spreadsheet, a `qty:
// "2"` left quoted by an editor, a `bracket: casual` written as the word.
// Which is exactly why they need holding: the paths the app exercises are the
// ones its own round trip already proves, and the paths a newcomer's first
// hand-edited file takes are the ones nothing was watching.
//
// The rule the whole family rests on is `truthy`, and it is one rule over
// every type goccy can hand back rather than a string comparison -- so
// `stringOr(0, "x")` is "x" and `stringOr("0", "x")` is "0", which is a
// distinction a deck file can express and a careless reader cannot.

// deckWith is one card's YAML, in a deck with nothing else in it.
func deckWith(card string) string {
	return "slug: coerce\nname: Coerce\ncards:\n  - " + card + "\n"
}

// TestAQuantityIsReadFromEveryShapeAFileCanWriteIt covers the four types the
// `qty:` switch accepts and the two it refuses.
//
// The bool arm is the one worth naming. `qty: false` meaning **zero** is what
// a careless `if q { 1 } else { 1 }` would get wrong -- a card nobody wanted,
// silently in the deck, with nothing failing.
//
// **`yes` and `no` are not bools here**, measured: this reader is YAML 1.2, so
// only the words `true` and `false` are, and `qty: yes` is the string "yes"
// and refused by name. That is the better answer of the two -- a file that
// says `yes` where a count belongs is a file with a mistake in it -- but it is
// the opposite of YAML 1.1, which is what most people have in their heads.
func TestAQuantityIsReadFromEveryShapeAFileCanWriteIt(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		yaml string
		want int
	}{
		{"{name: Forest, qty: 12}", 12},
		{"{name: Forest, qty: 12.0}", 12},   // float64
		{`{name: Forest, qty: "12"}`, 12},   // a number an editor left quoted
		{`{name: Forest, qty: " 12 "}`, 12}, // ...with the spaces a paste leaves
		{"{name: Forest, qty: true}", 1},    // YAML's bool, meaning one
		{"{name: Forest, qty: false}", 0},   // and its other half, meaning none
		{"{name: Forest, qty: null}", 1},    // an explicit null takes the default
		{"{name: Forest}", 1},               // as does an absent key
	} {
		d, err := FromText(deckWith(row.yaml), "coerce")
		if err != nil {
			t.Errorf("%s: %v", row.yaml, err)
			continue
		}
		if len(d.Cards) != 1 {
			t.Errorf("%s: %d cards", row.yaml, len(d.Cards))
			continue
		}
		if got := d.Cards[0].Qty; got != row.want {
			t.Errorf("%s: qty %d, want %d", row.yaml, got, row.want)
		}
	}
}

// A quantity that is not a number is refused **by the card's name**, because
// the person fixing it is looking at a file of ninety-nine lines and the
// index alone would send them counting.
func TestAQuantityThatIsNotANumberIsRefusedByTheCardsName(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		yaml string
		want string
	}{
		{`{name: Forest, qty: "three"}`, `Forest: qty "three" is not a number`},
		{`{name: Forest, qty: ""}`, `Forest: qty "" is not a number`},
		{"{name: Forest, qty: [2]}", "Forest: qty"},      // the default arm
		{"{name: Forest, qty: {a: 1}}", "Forest: qty"},   // ...and its other shape
		{"{name: Sol Ring, qty: nope}", "Sol Ring: qty"}, // a bare word is a string
		{"{name: Sol Ring, qty: yes}", "Sol Ring: qty"},  // ...and so is `yes` (YAML 1.2)
	} {
		_, err := FromText(deckWith(row.yaml), "coerce")
		if err == nil {
			t.Errorf("%s was accepted", row.yaml)
			continue
		}
		if !strings.Contains(err.Error(), row.want) {
			t.Errorf("%s refused with %q, want it to contain %q", row.yaml, err, row.want)
		}
		// And the refusal says which section and which row, so the file is
		// searchable even when two cards share a name across sections.
		if !strings.Contains(err.Error(), "cards[0]") {
			t.Errorf("%s refused without saying where: %q", row.yaml, err)
		}
	}
}

// A card that is neither a name nor a mapping is refused by what it *is*,
// which is the only useful thing to say about `- 7` on a line by itself.
func TestACardThatIsNeitherANameNorAMappingIsRefusedByItsType(t *testing.T) {
	t.Parallel()
	for _, row := range []struct{ yaml, want string }{
		{"7", "not int"},
		{"[Forest]", "not []interface {}"},
		{"true", "not bool"},
	} {
		_, err := FromText(deckWith(row.yaml), "coerce")
		if err == nil {
			t.Errorf("a card written %q was accepted", row.yaml)
			continue
		}
		if !strings.Contains(err.Error(), "a card is a name or a mapping") ||
			!strings.Contains(err.Error(), row.want) {
			t.Errorf("a card written %q refused with %q, want the type named (%q)",
				row.yaml, err, row.want)
		}
	}
	// The two sections behind `cards:` refuse the same way and say their own
	// name -- a swap board is edited by hand more often than the 99 is.
	for _, section := range []string{"swap_board", "graveyard"} {
		_, err := FromText("slug: coerce\nname: Coerce\n"+section+":\n  - 7\n", "coerce")
		if err == nil || !strings.Contains(err.Error(), section+"[0]") {
			t.Errorf("%s did not refuse a bare number by name: %v", section, err)
		}
	}
	// A card with no name at all is its own refusal, and does not read as a
	// type problem.
	_, err := FromText(deckWith("{category: land}"), "coerce")
	if err == nil || !strings.Contains(err.Error(), "a card has no name") {
		t.Errorf("a nameless card refused with %v", err)
	}
}

// TestABracketIsReadWhenItIsANumberAndDroppedWhenItIsProse holds the recorded
// asymmetry, which is deliberate and not obvious: an unreadable `qty` is an
// error and an unreadable `bracket` is not.
//
// The argument is in `deck.go` -- the typed field has nowhere to keep prose,
// and `analyze` treats an absent bracket as unlimited -- so the failure mode
// of dropping it is a deck checked against no bracket rules, while the
// failure mode of refusing it is a deck that will not open. The second is
// worse for the person who typed the word.
func TestABracketIsReadWhenItIsANumberAndDroppedWhenItIsProse(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		yaml string
		want *int
	}{
		{"bracket: 3", intp(3)},
		{"bracket: 3.0", intp(3)},   // float64
		{`bracket: "3"`, intp(3)},   // quoted
		{`bracket: " 3 "`, intp(3)}, // and padded
		{"bracket: casual", nil},    // prose: dropped, not refused
		{`bracket: ""`, nil},
		{"bracket: null", nil},
	} {
		d, err := FromText("slug: coerce\nname: Coerce\n"+row.yaml+"\ncards: []\n", "coerce")
		if err != nil {
			t.Errorf("%s: %v", row.yaml, err)
			continue
		}
		switch {
		case row.want == nil && d.Bracket != nil:
			t.Errorf("%s: bracket read as %d, want it dropped", row.yaml, *d.Bracket)
		case row.want != nil && d.Bracket == nil:
			t.Errorf("%s: bracket dropped, want %d", row.yaml, *row.want)
		case row.want != nil && *d.Bracket != *row.want:
			t.Errorf("%s: bracket %d, want %d", row.yaml, *d.Bracket, *row.want)
		}
	}
}

func intp(n int) *int { return &n }

// TestTruthyIsOneEmptinessRuleOverEveryTypeYAMLProduces is the rule
// `firstString`, `stringOr` and `shared:` all rest on, asked of itself.
//
// Called directly, because a deck file cannot express the difference between
// `int64(0)` and `uint64(0)` and this rule has an arm for each -- and because
// the arms exist to answer a question a `== ""` test cannot: **zero is empty
// and `"0"` is not.** A `name: 0` therefore falls back to the slug, while a
// `name: "0"` is a deck called nought.
func TestTruthyIsOneEmptinessRuleOverEveryTypeYAMLProduces(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		value any
		want  bool
	}{
		{nil, false},
		{false, false}, {true, true},
		{"", false}, {"x", true}, {"0", true}, {"false", true},
		{int64(0), false}, {int64(1), true}, {int64(-1), true},
		{uint64(0), false}, {uint64(1), true},
		{float64(0), false}, {0.5, true},
		{[]any{}, false}, {[]any{1}, true},
		{map[string]any{}, false}, {map[string]any{"a": 1}, true},
		// Nothing else goccy hands back is empty. A time, a struct, a
		// pointer: present is present.
		{struct{}{}, true},
	} {
		if got := truthy(row.value); got != row.want {
			t.Errorf("truthy(%#v) = %v, want %v", row.value, got, row.want)
		}
	}
	// And the two readers built on it, so the rule is proved where it is
	// actually used rather than only in the abstract.
	if got := stringOr(int64(0), "fallback"); got != "fallback" {
		t.Errorf("stringOr(int64(0)) = %q, want the fallback", got)
	}
	if got := stringOr("0", "fallback"); got != "0" {
		t.Errorf(`stringOr("0") = %q, want "0" -- a zero written as text is text`, got)
	}
	if got := firstString(float64(0), "fallback"); got != "fallback" {
		t.Errorf("firstString(0.0) = %q, want the fallback", got)
	}
}

// A deck whose name is the number zero falls back, and one whose name is the
// *string* zero does not. The same distinction as above, through the file, so
// it is a fact about deck files rather than about a helper.
//
// **What an empty name falls back to is the location, not the file's own
// `slug:`** -- measured, and worth pinning because it is the surprising half:
// `d.Slug` prefers the file and `d.Name` never sees it, so a file carrying
// `slug: gyome` and no `name:` in a directory called `food` is a deck whose
// slug is `gyome` and whose name is `Food`. Both readings are defensible;
// only one is what the code does, and a change to the other would rename
// decks on the shelf without touching a single deck file.
func TestAnEmptyNameFallsBackToTheLocationAndAQuotedZeroDoesNot(t *testing.T) {
	t.Parallel()
	for _, row := range []struct{ yaml, want string }{
		{"name: 0", "the-directory"},
		{`name: "0"`, "0"},
		{"name: null", "the-directory"},
		{"name: false", "the-directory"}, // YAML's bool, empty by the same rule
		{"name: Gyome", "Gyome"},
	} {
		d, err := FromText(fmt.Sprintf("slug: coerce\n%s\ncards: []\n", row.yaml), "the-directory")
		if err != nil {
			t.Errorf("%s: %v", row.yaml, err)
			continue
		}
		if d.Name != row.want {
			t.Errorf("%s: name %q, want %q", row.yaml, d.Name, row.want)
		}
		// ...while the slug still prefers the file's own, which is the half
		// that makes the fallback asymmetric rather than merely odd.
		if d.Slug != "coerce" {
			t.Errorf("%s: slug %q, want the file's own", row.yaml, d.Slug)
		}
	}
}
