package gate

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The corners of the companion conditions: the empty deck, the deck that
// satisfies the restriction, the deck of cards the checker has no word for,
// and the sentence it cannot read at all.
//
// A companion is the rule a newcomer is most likely to get wrong -- it sits
// outside the 99, it carries a deckbuilding restriction the rest of the deck
// must satisfy, and getting it wrong is an illegal deck that looks fine. So
// the two failure modes here are opposite and both bad. A checker that says
// nothing over a deck it could not read tells somebody their deck is legal
// when nobody looked. A checker that names cards over a deck that is fine
// sends somebody rebuilding a legal deck.
//
// `tiny_pool` holds no companion, so these are driven through `CheckCompanion`
// with synthetic records, exactly as the differential cases beside them are.
// The companion **names** are the real ones because they are the checker's
// key; every card they are pointed at is invented, and the one real sentence
// -- Kaheera's, which the checker parses -- is already typed out in
// `companion_test.go` where the parse is the subject.

// companionNamed builds the companion record the checker is keyed on. Its
// text only has to *be* a Companion sentence; what the restriction actually
// says is the checker's business, not this record's, for every companion but
// Kaheera.
func companionNamed(key string) *pool.CardRecord {
	return rec(key, "Legendary Creature — Demon", "", "Companion — "+key, 6)
}

func checkWith(key string, entries []Entry) CompanionCheck {
	return CheckCompanion(key, entries, map[string]*pool.CardRecord{key: companionNamed(key)})
}

// Umori asks that every nonland card share a card type. Three decks it must
// not complain about or must complain about completely.
func TestTheSharedTypeCheckReadsTheDecksItCannotSimplyCount(t *testing.T) {
	t.Parallel()
	const key = "umori, the collector"
	forest := rec("Forest", "Basic Land — Forest", "", "({T}: Add {G}.)", 0)
	cave := rec("Fixture Cave", "Land", "", "", 0)

	// **A starting deck of nothing but lands satisfies it vacuously**, and
	// the checker has to say so rather than dividing by a count of zero.
	if got := checkWith(key, []Entry{{forest.Name, forest}, {cave.Name, cave}}); got.Unsupported != "" ||
		len(got.Violations) > 0 {
		t.Errorf("a deck of nothing but lands broke the shared-type restriction: %+v", got)
	}

	// A deck whose nonlands do all share a type is legal, and the checker
	// names nobody -- this is the half that sends a player rebuilding a
	// perfectly good deck when it goes wrong.
	bear := rec("Fixture Bear", "Creature — Bear", "{1}{G}", "", 2)
	elf := rec("Fixture Elf", "Creature — Elf Druid", "{G}", "", 1)
	artifactCreature := rec("Fixture Golem", "Artifact Creature — Golem", "{4}", "", 4)
	legal := []Entry{{forest.Name, forest}, {bear.Name, bear}, {elf.Name, elf},
		{artifactCreature.Name, artifactCreature}}
	if got := checkWith(key, legal); got.Unsupported != "" || len(got.Violations) > 0 {
		t.Errorf("a deck of creatures broke the shared-type restriction: %+v", got)
	}

	// And a deck of cards whose type the checker has no word for is reported
	// **whole**: there is no majority type to measure a minority against, so
	// naming a few of them would point at the wrong cards. Schemes are a
	// real card type and no Commander deck holds one, which is why they can
	// stand in for a type line this list does not cover.
	oddities := []Entry{
		{"Fixture Scheme", rec("Fixture Scheme", "Scheme", "", "", 0)},
		{"Fixture Phenomenon", rec("Fixture Phenomenon", "Phenomenon", "", "", 0)},
	}
	got := checkWith(key, oddities)
	if got.Unsupported != "" {
		t.Fatalf("the checker gave up: %q", got.Unsupported)
	}
	if len(got.Violations) != 2 {
		t.Errorf("a deck of cards with no shared type named %v -- all of them "+
			"are the answer when none of them is the majority", got.Violations)
	}
}

// Lurrus asks that every permanent card be mana value two or less, and the
// card that breaks it is a permanent -- an expensive instant does not.
func TestTheCheapPermanentCheckCountsPermanentsAndNotSpells(t *testing.T) {
	t.Parallel()
	const key = "lurrus of the dream-den"
	entries := []Entry{
		{"Fixture Signet", rec("Fixture Signet", "Artifact", "{2}", "", 2)},
		{"Fixture Colossus", rec("Fixture Colossus", "Creature — Golem", "{6}", "", 6)},
		{"Fixture Counterspell", rec("Fixture Counterspell", "Instant", "{1}{U}{U}", "", 3)},
	}
	got := checkWith(key, entries)
	if got.Unsupported != "" {
		t.Fatalf("the checker gave up: %q", got.Unsupported)
	}
	if strings.Join(got.Violations, ",") != "Fixture Colossus" {
		t.Errorf("the violations are %v -- the expensive instant is not a "+
			"permanent and the cheap artifact is legal", got.Violations)
	}
}

// Lutri asks that no two nonland cards share a name, and a repeated land is
// not a repeat -- which is the whole reason the check exists rather than a
// plain duplicate scan.
func TestTheSingletonCheckIgnoresRepeatedLands(t *testing.T) {
	t.Parallel()
	const key = "lutri, the spellchaser"
	forest := rec("Forest", "Basic Land — Forest", "", "({T}: Add {G}.)", 0)
	bolt := rec("Fixture Bolt", "Instant", "{R}", "", 1)
	entries := []Entry{
		{forest.Name, forest}, {forest.Name, forest},
		{bolt.Name, bolt}, {bolt.Name, bolt},
	}
	got := checkWith(key, entries)
	if strings.Join(got.Violations, ",") != "Fixture Bolt" {
		t.Errorf("the violations are %v -- two Forests are legal and two of "+
			"the same spell are not", got.Violations)
	}
}

// A restriction the checker cannot parse is reported as unverified, with the
// sentence it could not read beside it -- never as a clean bill of health.
//
// This is the one that matters most when a new companion is printed: the card
// is a companion, the app knows which checker to run, and the sentence has
// changed shape. Saying nothing would be a legality claim nobody made.
func TestARestrictionThatCannotBeParsedIsReportedAsUnverified(t *testing.T) {
	t.Parallel()
	const key = "kaheera, the orphanguard"
	// A Companion sentence in the right shape whose restriction names no
	// creature types the reader can pick out.
	odd := rec(key, "Legendary Creature — Cat Beast", "{1}{G/W}{G/W}",
		"Companion — Your starting deck is a good one.", 3)
	bear := rec("Fixture Bear", "Creature — Bear", "{1}{G}", "", 2)

	got := CheckCompanion(key, []Entry{{bear.Name, bear}},
		map[string]*pool.CardRecord{key: odd})
	if got.Unsupported == "" {
		t.Fatalf("an unreadable restriction passed as checked: %+v", got)
	}
	if len(got.Violations) > 0 {
		t.Errorf("an unreadable restriction still named cards: %v", got.Violations)
	}
	// The condition it could not read travels with the refusal, because the
	// player has to be able to check it by hand.
	if !strings.Contains(got.Condition, "Companion —") {
		t.Errorf("the refusal carries no condition: %+v", got)
	}
}

// Zirda asks that every permanent have an activated ability, and Scryfall
// prints some of them as a keyword with no colon -- so a card whose only
// activated ability is `Cycling {2}` is not a violation.
func TestAKeywordWithNoColonStillReadsAsAnActivatedAbility(t *testing.T) {
	t.Parallel()
	const key = "zirda, the dawnwaker"
	entries := []Entry{
		{"Fixture Cycler", rec("Fixture Cycler", "Artifact", "{2}", "Cycling {2}", 2)},
		{"Fixture Blank", rec("Fixture Blank", "Artifact", "{2}", "", 2)},
	}
	got := checkWith(key, entries)
	if got.Unsupported != "" {
		t.Fatalf("the checker gave up: %q", got.Unsupported)
	}
	if strings.Join(got.Violations, ",") != "Fixture Blank" {
		t.Errorf("the violations are %v -- a keyword printed without its "+
			"colon is still an activated ability", got.Violations)
	}
	// And it is a heuristic, which the result says so the report can warn
	// rather than refuse.
	if got.Exact {
		t.Error("the activated-ability reading reports itself as exact")
	}
}

// The mana-symbol reader's own guard: an empty symbol is not a number.
//
// It cannot arrive from the regex -- `{}` does not match -- and it is here
// because the loop it guards answers "yes, all of these are digits" for a
// string with no digits in it, which would silently drop a symbol from
// Jegantha's count and call a deck legal.
func TestAnEmptySymbolIsNotANumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"2", true},
		{"10", true},
		{"G", false},
		{"2G", false},
	} {
		if got := isDigits(tc.in); got != tc.want {
			t.Errorf("isDigits(%q) is %v", tc.in, got)
		}
	}
}
