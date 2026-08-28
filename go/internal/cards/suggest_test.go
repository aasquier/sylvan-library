package cards_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// suggest runs one query against the fixture pool and returns the names in
// the order they were offered, alongside the tier each came from.
func suggest(t *testing.T, text string, limit int) ([]string, map[string]string) {
	t.Helper()
	ctx := context.Background()
	names := []string{}
	via := map[string]string{}
	p := pooltest.Open(t)
	if err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := cards.Suggest(ctx, c, text, limit)
		if err != nil {
			return err
		}
		for _, s := range found {
			names = append(names, s.Name)
			via[s.Name] = s.Via
		}
		return nil
	}); err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	return names, via
}

// first is the shortlist's head, or "" for an empty one.
func first(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// The four tiers, each doing the job it exists for. Every expectation here is
// a real thing somebody types into the box, not a synthetic string.
func TestEachTierOffersWhatItIsFor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ typed, want, via string }{
		// exact -- the name typed out in full, which nothing outranks. It is
		// what carries a basic land, whose `edhrec_rank` is null.
		{"Sol Ring", "Sol Ring", "exact"},
		{"forest", "Forest", "exact"},
		{"Etali, Primal Conqueror", "Etali, Primal Conqueror // Etali, Primal Sickness", "exact"},
		// holds -- a fragment, anywhere in the name.
		{"sol r", "Sol Ring", "holds"},
		{"behemoth", "Craterhoof Behemoth", "holds"},
		{"reborn", "Llanowar Reborn", "holds"},
		// words -- the same words, the wrong way round. As one string
		// `titan primeval` scores 0.706 and would never survive the floor.
		{"titan primeval", "Primeval Titan", "words"},
		{"study rhystic", "Rhystic Study", "words"},
		// near -- the whole point: a misspelling that literal matching
		// answers with nothing at all.
		{"Sol Rng", "Sol Ring", "near"},
		{"rystic study", "Rhystic Study", "near"},
		{"swords to plowshars", "Swords to Plowshares", "near"},
		{"terastadon", "Terastodon", "near"},
		// One word right, one word wrong -- the `words` tier needs every word
		// and gets neither this nor `behemoth` alone, so the guess answers.
		{"craterhof behemoth", "Craterhoof Behemoth", "near"},
		// And the tiers do not fight: `smothering tith` is *inside*
		// Smothering Tithe, so the literal tier answers and the guess is
		// never consulted.
		{"smothering tith", "Smothering Tithe", "holds"},
	} {
		names, via := suggest(t, tc.typed, 8)
		if got := first(names); got != tc.want {
			t.Errorf("%q offered %q first, wanted %q (all: %v)", tc.typed, got, tc.want, names)
			continue
		}
		if via[tc.want] != tc.via {
			t.Errorf("%q found %s via %q, wanted %q", tc.typed, tc.want, via[tc.want], tc.via)
		}
	}
}

// The near tier's floor, from both sides. These are the numbers
// [cards.NearFloor] argues, checked rather than quoted: three real typos that
// the importer's 0.95 would refuse and this must offer, and keyboard mash
// that must stay out of a beginner's list.
func TestTheNearFloorSitsBetweenARealTypoAndKeyboardMash(t *testing.T) {
	t.Parallel()
	// Below the importer's 0.95 and above ours. `sakura tribe elder` is the
	// shape of it: one dropped hyphen, which is the commonest miss of all.
	for _, typed := range []string{"terastadon", "craterhof behemoth", "primevil titan"} {
		names, _ := suggest(t, typed, 8)
		if len(names) == 0 {
			t.Errorf("%q was offered nothing -- the floor is too high", typed)
		}
	}
	// Nothing at all, rather than the alphabetically nearest card. An empty
	// list with a sentence under it beats a confident wrong answer.
	for _, typed := range []string{"qwertyuiop", "asdkjhasd", "zzzzzzzz", "xkcdxkcd"} {
		if names, _ := suggest(t, typed, 8); len(names) != 0 {
			t.Errorf("%q was offered %v -- the floor is too low", typed, names)
		}
	}
}

// Below `NearLength` a query is a prefix, not a misspelling. Two and three
// letters score 0.80-0.89 against half the pool purely because jaro-winkler
// weights a common opening, so guessing there would fill the list with
// alphabetical neighbours after two keystrokes.
func TestAShortQueryIsAPrefixAndNeverAGuess(t *testing.T) {
	t.Parallel()
	for _, typed := range []string{"so", "sol", "cul", "gyo"} {
		names, via := suggest(t, typed, 8)
		for _, n := range names {
			if via[n] == "near" {
				t.Errorf("%q guessed at %s after %d letters", typed, n, len([]rune(typed)))
			}
		}
	}
	// And the guess switches on at exactly four, which is the constant's
	// whole claim. `Swmp` scores 0.947 against Swamp and matches it no other
	// way; `Swm` is the same misspelling one letter shorter and is answered
	// with nothing at all.
	if names, via := suggest(t, "Swmp", 8); first(names) != "Swamp" || via["Swamp"] != "near" {
		t.Errorf("four letters did not reach the near tier: %v %v", names, via)
	}
	if names, _ := suggest(t, "Swm", 8); len(names) != 0 {
		t.Errorf("three letters guessed at %v", names)
	}
}

// A literal hit always outranks a guess, however close the guess scores.
// `cultivat` sits inside Cultivator Colossus and scores well against half a
// dozen other names; nothing may reorder those.
func TestALiteralHitOutranksAGuess(t *testing.T) {
	t.Parallel()
	names, via := suggest(t, "cultivat", 8)
	if first(names) != "Cultivator Colossus" || via["Cultivator Colossus"] != "holds" {
		t.Fatalf("%v %v", names, via)
	}
}

// **A fragment means the famous card, not the alphabetically lucky one.**
//
// This is the regression pin for a real mistake: the first cut ranked a name
// that *begins* with the query above one that merely contains it, and against
// the whole card pool `bolt` came back Bolt Bend, Boltwave, Boltbender, Bolt
// Hound -- with Lightning Bolt nowhere. Here `ter` opens exactly one card and
// sits inside three, and the order must be the order the game plays them.
func TestAFragmentIsOrderedByHowMuchTheGameActuallyPlaysIt(t *testing.T) {
	t.Parallel()
	names, via := suggest(t, "ter", 8)
	want := []string{
		"Craterhoof Behemoth",           // 315, and does not start with it
		"Goreclaw, Terror of Qal Sisma", // 674, ditto
		"Terastodon",                    // 1375, and the only one that does
		"Gyome, Master Chef",            // 4740
	}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("ordered %v, wanted %v", names, want)
	}
	for _, n := range names {
		if via[n] != "holds" {
			t.Errorf("%s came via %q", n, via[n])
		}
	}
	// And the whole name still wins outright, which is what keeps a basic
	// land -- ranked by nobody, so null -- findable at all.
	if names, via := suggest(t, "Terastodon", 8); first(names) != "Terastodon" ||
		via["Terastodon"] != "exact" {
		t.Fatalf("%v %v", names, via)
	}
}

// Nothing that cannot be in a Commander deck is offered -- but a card that is
// *banned* is, because hiding it is indistinguishable from it not existing,
// and the person is left retyping a name that was right all along. The write
// path refuses it, by name, with a reason.
func TestBannedCardsAreOfferedAndNonCardsAreNot(t *testing.T) {
	t.Parallel()
	if names, _ := suggest(t, "Black Lotus", 8); first(names) != "Black Lotus" {
		t.Errorf("a banned card was hidden rather than offered: %v", names)
	}
	if names, _ := suggest(t, "Emrakul", 8); first(names) != "Emrakul, the Aeons Torn" {
		t.Errorf("a banned card was hidden rather than offered: %v", names)
	}
	// An emblem is not a card a deck could hold, at any tier.
	for _, typed := range []string{"Ajani Steadfast Emblem", "emblem", "Ajani Steadfast Emblim"} {
		names, _ := suggest(t, typed, 20)
		for _, n := range names {
			if strings.Contains(n, "Emblem") {
				t.Errorf("%q offered %q, which no deck can hold", typed, n)
			}
		}
	}
}

// The limit is honoured and bounded, and an empty question is answered with
// an empty list rather than the whole pool.
func TestTheShortlistIsBoundedAndAnEmptyQuestionAnswersNothing(t *testing.T) {
	t.Parallel()
	for _, typed := range []string{"", "   ", "\t\n"} {
		if names, _ := suggest(t, typed, 8); len(names) != 0 {
			t.Errorf("%q offered %v", typed, names)
		}
	}
	if names, _ := suggest(t, "e", 3); len(names) > 3 {
		t.Errorf("a limit of 3 offered %d", len(names))
	}
	// Zero and negative are floored at one rather than refused, and a limit
	// past the ceiling is clamped rather than run.
	if names, _ := suggest(t, "e", 0); len(names) != 1 {
		t.Errorf("a limit of 0 offered %d", len(names))
	}
	if names, _ := suggest(t, "e", 9999); len(names) > cards.MaxSuggestions {
		t.Errorf("a limit past the ceiling offered %d", len(names))
	}
	// The two bounds a caller controls -- how long the question is, and how
	// many words it holds -- are clamped rather than run. A whole decklist
	// pasted into the box must answer, quickly, without building a CASE with
	// a hundred `contains` calls in it.
	if names, _ := suggest(t, strings.Repeat("Sol Ring ", 400), 8); len(names) == 0 {
		t.Error("a long paste answered nothing at all")
	}
	deckish := strings.Repeat("swamp forest island mountain plains ", 40)
	if names, _ := suggest(t, deckish, 8); len(names) != 0 {
		t.Errorf("a pasted decklist offered %v", names)
	}
}

// Two identical requests answer in the same order. This is a list somebody
// arrows through: an order that moved between two keystrokes would move the
// selection under their finger.
func TestTheOrderIsTheSameTwice(t *testing.T) {
	t.Parallel()
	for _, typed := range []string{"e", "behemoth", "sol", "primeval titan"} {
		once, _ := suggest(t, typed, 20)
		twice, _ := suggest(t, typed, 20)
		if strings.Join(once, "|") != strings.Join(twice, "|") {
			t.Errorf("%q: %v then %v", typed, once, twice)
		}
	}
}

// The double-faced cards, which are the reason the scorer reads the front
// face as well as the whole name: a person writes the side they know.
func TestADoubleFacedCardIsFoundByEitherFace(t *testing.T) {
	t.Parallel()
	const dfc = "Etali, Primal Conqueror // Etali, Primal Sickness"
	for _, typed := range []string{"Etali", "Primal Sickness", "Etali, Primal Conqueror"} {
		if names, _ := suggest(t, typed, 8); first(names) != dfc {
			t.Errorf("%q offered %v", typed, names)
		}
	}
	// And misspelled, through the near tier, against the front face -- which
	// is a whole 25 characters shorter than the name the row would score.
	// The name below is misspelled on purpose: it is the input under test
	// rather than prose, so letting `misspell` correct it would quietly
	// delete the assertion. Hence the directive on the line itself.
	if names, via := suggest(t, "Etali, Primal Conquerer", 8); first(names) != dfc || //nolint:misspell // the misspelling is the test
		via[dfc] != "near" {
		t.Errorf("a misspelled front face offered %v %v", names, via)
	}
}
