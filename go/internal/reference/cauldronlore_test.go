package reference

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/brew"
)

// The cauldron lore's three readers, and the one completeness claim the
// corpus makes. What the readers' tests cannot reach is the branches a real
// pot never produces: an offer with nothing left to give, an ingredient
// nobody wrote about, and an id spelled the way a model shouts it.

// TestEveryIngredientInTheCauldronHasItsFact is the completeness check, and
// the list it checks against is **discovered rather than written down**.
//
// Writing the twenty-four keys out here would produce a test that passes
// forever on the day somebody adds a twenty-fifth ingredient and forgets its
// fact -- which is exactly the failure the check exists for. So the catalogue
// is asked, and the corpus is held to whatever it answers.
//
// The import is test-only. Nothing this package serves depends on
// `internal/brew`, and the dependency runs this way round on purpose: the pot
// decides what can go in it, and the corpus owes a fact for each.
func TestEveryIngredientInTheCauldronHasItsFact(t *testing.T) {
	t.Parallel()
	for _, in := range brew.Catalogue {
		facts := CauldronFactsForIngredient(in.Key)
		if len(facts) == 0 {
			t.Errorf("%s can go in the pot and nothing in the corpus is about it",
				in.Key)
			continue
		}
		for _, f := range facts {
			if f.Text == "" || f.Source == "" {
				t.Errorf("%s: fact %q has no text or no source -- a fact that "+
					"cites nothing is an opinion", in.Key, f.ID)
			}
		}
	}
	// And the other direction: a fact about an ingredient that cannot be
	// picked is a fact nobody will ever be told.
	for _, f := range Cauldron().Facts {
		if f.Ingredient == "" {
			continue
		}
		if _, ok := brew.ByKey[f.Ingredient]; !ok {
			t.Errorf("fact %q is about %q, which is not in the catalogue",
				f.ID, f.Ingredient)
		}
	}
	// Ids are the contract, so they have to be distinct.
	seen := map[string]bool{}
	for _, f := range Cauldron().Facts {
		if seen[f.ID] {
			t.Errorf("%q is the id of two facts; a citation could not choose", f.ID)
		}
		seen[f.ID] = true
	}
}

// The pot tier comes first and always, which is why a brew of three quiet
// ingredients is not a brew with nothing to say.
func TestThePotTierLeadsEveryBrew(t *testing.T) {
	t.Parallel()
	potTier := 0
	for _, f := range Cauldron().Facts {
		if f.Ingredient == "" {
			potTier++
		}
	}
	if potTier == 0 {
		t.Fatal("no fact is true of the pot itself")
	}
	// Three real ingredients, one from each place, the way a pick produces
	// them -- taken from the catalogue rather than named, for the reason the
	// completeness test gives.
	keys := []string{}
	for _, place := range brew.Order {
		keys = append(keys, brew.ByCategory[place.Slot][0].Key)
	}
	if len(keys) != 3 {
		t.Fatalf("the pot has %d places", len(keys))
	}
	facts := CauldronFactsForBrew(keys)
	if len(facts) <= potTier {
		t.Fatalf("%d facts for three ingredients, and %d of them are the pot's "+
			"-- the ingredients contributed nothing", len(facts), potTier)
	}
	for i := 0; i < potTier; i++ {
		if facts[i].Ingredient != "" {
			t.Errorf("fact %d belongs to an ingredient; the pot tier does not lead", i)
		}
	}
	if facts[potTier].Ingredient != keys[0] {
		t.Errorf("the first ingredient fact is %q, want %q -- they follow in "+
			"the order they went in", facts[potTier].Ingredient, keys[0])
	}
	// An ingredient nobody wrote about contributes nothing rather than
	// failing.
	if got := CauldronFactsForIngredient("not-an-ingredient"); len(got) != 0 {
		t.Errorf("an ingredient nobody wrote about has %d facts", len(got))
	}
}

// **An offer with nothing left is the empty string, not a heading over
// nothing.** The caller appends it to the frame unconditionally, so a header
// with no facts under it would be an instruction to cite from an empty list.
// No real pot reaches this -- the pot tier alone is six facts -- so it is
// driven directly.
func TestACauldronOfferWithNothingLeftIsEmpty(t *testing.T) {
	t.Parallel()
	keys := []string{brew.Catalogue[0].Key}
	full := CauldronOffer(keys, nil)
	// The offer lists fact **ids**, never the ingredient key -- the id is
	// what the witch puts in `source`, and the lookup goes by that.
	if full == "" || !strings.Contains(full, "\n- pot-") {
		t.Fatalf("a full offer carries no pot-tier fact:\n%s", full)
	}
	if strings.Contains(full, "\n- "+keys[0]+":") {
		t.Error("the offer names the ingredient key, which is not what she cites")
	}

	told := []string{}
	for _, f := range CauldronFactsForBrew(keys) {
		told = append(told, f.Text)
	}
	if got := CauldronOffer(keys, told); got != "" {
		t.Errorf("everything has been told and the offer is still %d "+
			"characters:\n%s", len(got), got)
	}
	// Telling all but one leaves exactly that one, and the header with it.
	got := CauldronOffer(keys, told[:len(told)-1])
	if !strings.Contains(got, "True things you know") || strings.Count(got, "\n- ") != 1 {
		t.Errorf("one fact left and the offer reads:\n%s", got)
	}
	// A told fact is matched after stripping, the way the repeat check sees
	// it.
	padded := append([]string{}, told...)
	padded[0] = "   " + padded[0] + "  "
	if a, b := CauldronOffer(keys, padded), CauldronOffer(keys, told); a != b {
		t.Error("a told fact with whitespace round it was offered again")
	}
}

// An id is folded and stripped before it is looked up, which is not
// politeness: the `cauldron:` prefix is matched case-insensitively, so a turn
// that shouts `CAULDRON:POT-CERIDWEN` gets past the prefix check and would
// then miss here on the one difference nobody would ever debug from a
// dropped-fact counter.
func TestACauldronFactIsFoundHoweverItsIdIsSpelled(t *testing.T) {
	t.Parallel()
	want := Cauldron().Facts[0].ID
	for _, spelling := range []string{want, strings.ToUpper(want), "  " + want + " "} {
		got := CauldronFactByID(spelling)
		if got == nil || got.ID != want {
			t.Errorf("%q resolved to %v", spelling, got)
		}
	}
	// An id nobody wrote is nil rather than a raise: one bad reference costs
	// the fact, never the turn. The last two are a tarot id and a bare prefix
	// -- the corpora are addressed separately and neither answers for the
	// other.
	for _, missing := range []string{"", "   ", "not-a-fact", "pot", "pixie-fee"} {
		if got := CauldronFactByID(missing); got != nil {
			t.Errorf("%q resolved to %q", missing, got.ID)
		}
	}
	if got := TarotFactByID("pot-ceridwen"); got != nil {
		t.Errorf("the deck's corpus answered a cauldron id with %q", got.ID)
	}
}
