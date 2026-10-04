package gate

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The readers' own refusals: a clause the Rulebreaker parser cannot read, a
// pairing whose kind it does not know, a companion asked about a card with no
// Companion ability, and a list too long to print whole.
//
// **These are parser inputs rather than claims about Magic.** The printed
// clauses are a corpus and they live in `rulebreakerprinted_test.go`, read out of
// the real pool; what is here is the shape of a sentence the parser must refuse
// to guess at, which is a fact about the parser. ADR 51 is the whole reason that
// matters: the clause is read off the commander's card, so a clause this check
// cannot read has to be **named as unread** rather than quietly treated as
// granting nothing — a deck would then be validated against a rule nobody
// applied.

// A grant whose type list reads as nothing is refused by name rather than
// granting everything or granting nothing.
func TestAGrantWithNoTypesInItIsNamedAsUnread(t *testing.T) {
	t.Parallel()
	// A clause the pattern matches and the type list does not survive: the
	// separator is there and the words are not.
	a, why := readTypeGrant(", cards of any color identity")
	if why == "" {
		t.Fatalf("a clause naming no card types was read as the grant %+v", a)
	}
	if !strings.Contains(why, "cannot tell which cards") {
		t.Errorf("the refusal does not say what it could not tell: %q", why)
	}

	// The same shape one clause along, where a colour is granted rather than all
	// of them (Tolabow's sentence is the printed one).
	grants, why := readChoiceClause(
		"the color identity of , cards in your deck can include one color of " +
			"your choice not in your commander's color identity")
	if why == "" {
		t.Fatalf("a colour clause naming no card types was read as %+v", grants)
	}
	if grants != nil {
		t.Errorf("the refusal came back with %d allowances beside it", len(grants))
	}
}

// A mana value too big to be a number is refused rather than read as zero.
//
// The pattern captures digits, so the only way this fails is a run of them no
// integer can hold — and a clause read as "mana value 0 or greater" would let
// every card in the deck through a check that is supposed to be narrow. The
// comment that used to sit on this branch called it unreachable; a long enough
// number is what reaches it.
func TestAManaValueTooBigToBeANumberIsRefused(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("9", 40)
	a, why := readTypeGrant(
		"artifact cards with mana value " + huge + " or greater of any color identity")
	if why == "" {
		t.Fatalf("a clause whose mana value is %s digits long was read as the "+
			"grant %+v", huge, a)
	}
	if a.minMV != 0 || a.types != nil {
		t.Errorf("the refusal came back carrying a grant: %+v", a)
	}
	// The ordinary form still reads, so this is not passing because the pattern
	// stopped matching mana values at all.
	ok, why := readTypeGrant(
		"artifact cards with mana value 7 or greater of any color identity")
	if why != "" {
		t.Fatalf("an ordinary mana-value clause was refused: %q", why)
	}
	if ok.minMV != 7 {
		t.Errorf("the clause read a floor of %v, want 7", ok.minMV)
	}
}

// A grant with no type groups at all matches nothing, rather than matching
// everything.
//
// `matchesTypes` is the inner question of every type grant, and an empty group
// list is what a clause that could not be read leaves behind. Answering true
// there would turn an unreadable clause into a licence.
func TestAGrantWithNoTypeGroupsMatchesNothing(t *testing.T) {
	t.Parallel()
	rec := &pool.CardRecord{Name: "Fixture Angel", TypeLine: "Creature — Angel"}
	if matchesTypes(rec, nil) {
		t.Error("a grant naming no types matched a card")
	}
	if matchesTypes(rec, [][]string{}) {
		t.Error("a grant with an empty list of type groups matched a card")
	}
	// And the ordinary form still matches, so the guard above is a guard rather
	// than a broken matcher.
	if !matchesTypes(rec, [][]string{{"Angel"}}) {
		t.Error("a grant naming Angel did not match an Angel")
	}
}

// A pairing whose kind this check does not know pairs with nothing.
//
// Every kind the reader produces is one of the five the switch names, so this is
// the arm for a sixth that does not exist yet — and the answer has to be no. A
// default that said yes would let two commanders sit together because the check
// did not recognise what joined them, which is the one mistake a legality check
// may not make.
func TestAPairingOfAnUnknownKindPairsWithNothing(t *testing.T) {
	t.Parallel()
	other := card("Fixture Partner", "Legendary Creature — Human", "Partner")
	unknown := &Pairing{Kind: "a kind no printed card has"}
	if match(unknown, other, &Pairing{Kind: Partner}) {
		t.Error("a pairing of an unrecognised kind was allowed to pair")
	}
	// And a card with no pairing ability at all pairs with nothing either.
	if match(nil, other, &Pairing{Kind: Partner}) {
		t.Error("a card with no pairing ability was allowed to pair")
	}
}

// A companion asked about a card with no Companion ability says so, rather than
// reporting a restriction it never read.
//
// `Validate` guards this one call -- it checks the ability first -- so this is
// the exported function's own contract, and the Claude tools and the CLI both
// reach it directly. "I did not check" and "it passed" are different sentences
// and only one of them is true.
func TestACompanionCheckOnACardWithNoCompanionAbilitySaysSo(t *testing.T) {
	t.Parallel()
	plain := card("Fixture Chef", "Legendary Creature — Human Peasant",
		"When Fixture Chef enters, create a Food token.")
	got := CheckCompanion("Fixture Chef", nil,
		map[string]*pool.CardRecord{"Fixture Chef": plain})
	if got.Unsupported == "" {
		t.Fatalf("a card with no Companion ability was checked as a companion: %+v", got)
	}
	if got.Condition != "" {
		t.Errorf("the refusal came back carrying a condition: %q", got.Condition)
	}
	if len(got.Violations) != 0 {
		t.Errorf("the refusal came back carrying violations: %v", got.Violations)
	}
}

// A list longer than three names is shown as three and a count.
//
// It is the report's house habit and the reason is width: these messages sit
// beside two or three others on one line, and a companion restriction broken by
// twenty cards would otherwise push everything else off the page (commandment
// 2 -- a wall of names is not a diagnosis).
func TestALongListIsShownAsThreeNamesAndACount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "b", "c", "d"}, []string{"a", "b", "c", "and 1 more"}},
		{[]string{"a", "b", "c", "d", "e", "f"}, []string{"a", "b", "c", "and 3 more"}},
	} {
		got := shortList(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%v shortened to %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%v shortened to %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}
