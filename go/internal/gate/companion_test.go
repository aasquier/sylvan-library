package gate

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// `tiny_pool` holds no companion, so the restriction checkers are held here
// over synthetic records -- real oracle sentences, typed from the cards --
// rather than by the differential cases.

func rec(name, typeLine, cost, text string, cmc float64) *pool.CardRecord {
	var c *string
	if cost != "" {
		c = &cost
	}
	return &pool.CardRecord{Name: name, TypeLine: typeLine, ManaCost: c, OracleText: text, CMC: cmc, LegalCommander: true}
}

func TestConditionTextIsTheCompanionSentence(t *testing.T) {
	t.Parallel()
	kaheera := rec("Kaheera, the Orphanguard", "Legendary Creature — Cat Beast", "{1}{G/W}{G/W}",
		"Companion — Each creature card in your starting deck is a Cat, Elemental, Nightmare, Dinosaur, or Beast card. (If this card is your chosen companion, you may put it into your hand from outside the game for {3} any time you could cast a sorcery.)\nVigilance", 3)
	if got := ConditionText(kaheera); got != "Companion — Each creature card in your starting deck is a Cat, Elemental, Nightmare, Dinosaur, or Beast card." {
		t.Fatalf("%q", got)
	}
	if !IsCompanion(kaheera) || IsCompanion(rec("Sol Ring", "Artifact", "{1}", "{T}: Add {C}{C}.", 1)) {
		t.Fatal("is_companion")
	}
	// "companion" in flavour or reminder text is not the ability.
	if IsCompanion(rec("X", "Creature", "", "Whenever a companion dies, draw a card.", 1)) {
		t.Fatal("the word alone was taken for the ability")
	}
}

func TestTheCheckersReadTheStartingDeck(t *testing.T) {
	t.Parallel()
	kaheera := rec("Kaheera, the Orphanguard", "Legendary Creature — Cat Beast", "{1}{G/W}{G/W}",
		"Companion — Each creature card in your starting deck is a Cat, Elemental, Nightmare, Dinosaur, or Beast card.", 3)
	cat := rec("Arahbo, Roar of the World", "Legendary Creature — Cat Avatar", "{3}{G}{W}", "Eminence", 5)
	troll := rec("Gyome, Master Chef", "Legendary Creature — Troll Warlock", "{2}{B}{G}", "", 4)
	forest := rec("Forest", "Basic Land — Forest", "", "({T}: Add {G}.)", 0)
	cards := map[string]*pool.CardRecord{kaheera.Name: kaheera}
	entries := []Entry{{cat.Name, cat}, {troll.Name, troll}, {forest.Name, forest}}
	got := CheckCompanion(kaheera.Name, entries, cards)
	if got.Unsupported != "" || !got.Exact || strings.Join(got.Violations, ",") != "Gyome, Master Chef" {
		t.Fatalf("kaheera: %+v", got)
	}

	// The even/odd, three-or-greater, two-or-less, repeated-symbol and
	// shared-type readings.
	mk := func(name, text string) *pool.CardRecord {
		return rec(name, "Legendary Creature — Demon", "", "Companion — "+text, 6)
	}
	sol := rec("Sol Ring", "Artifact", "{1}", "{T}: Add {C}{C}.", 1)
	elves := rec("Llanowar Elves", "Creature — Elf Druid", "{G}", "{T}: Add {G}.", 1)
	bear := rec("Grizzly Bears", "Creature — Bear", "{1}{G}", "", 2)
	bolt := rec("Lightning Bolt", "Instant", "{R}", "Deals 3.", 1)
	doubled := rec("Cryptic Command", "Instant", "{1}{U}{U}{U}", "Choose two.", 4)
	deck := []Entry{{sol.Name, sol}, {elves.Name, elves}, {bear.Name, bear}, {bolt.Name, bolt}, {doubled.Name, doubled}, {forest.Name, forest}}
	table := map[string][]string{
		"gyruda, doom of depths":   {"Sol Ring", "Llanowar Elves", "Lightning Bolt"},
		"obosh, the preypiercer":   {"Grizzly Bears", "Cryptic Command"},
		"keruga, the macrosage":    {"Sol Ring", "Llanowar Elves", "Grizzly Bears", "Lightning Bolt"},
		"lurrus of the dream-den":  {},
		"jegantha, the wellspring": {"Cryptic Command"},
		"lutri, the spellchaser":   {},
		"umori, the collector":     {"Sol Ring", "Lightning Bolt", "Cryptic Command"},
	}
	for key, want := range table {
		companion := mk(key, key)
		cards := map[string]*pool.CardRecord{key: companion}
		got := CheckCompanion(key, deck, cards)
		if got.Unsupported != "" {
			t.Errorf("%s: unsupported %q", key, got.Unsupported)
			continue
		}
		if strings.Join(got.Violations, ",") != strings.Join(want, ",") {
			t.Errorf("%s: violations %v, want %v", key, got.Violations, want)
		}
	}
	// Zirda is a heuristic, and says so.
	zirda := mk("zirda, the dawnwaker", "Each permanent card in your starting deck has an activated ability.")
	got = CheckCompanion("zirda, the dawnwaker", []Entry{{sol.Name, sol}, {bear.Name, bear}}, map[string]*pool.CardRecord{"zirda, the dawnwaker": zirda})
	if got.Exact || strings.Join(got.Violations, ",") != "Grizzly Bears" {
		t.Fatalf("zirda: %+v", got)
	}
	// Unknown, uncheckable, and absent.
	if got := CheckCompanion("Lutri, Pauper Otter", nil, map[string]*pool.CardRecord{"Lutri, Pauper Otter": mk("Lutri, Pauper Otter", "...")}); !strings.Contains(got.Unsupported, "expansion symbols") {
		t.Fatalf("uncheckable: %+v", got)
	}
	if got := CheckCompanion("Some New Companion", nil, map[string]*pool.CardRecord{"Some New Companion": mk("Some New Companion", "...")}); got.Unsupported != "no checker is implemented for this companion" {
		t.Fatalf("unknown: %+v", got)
	}
	if got := CheckCompanion("Nobody", nil, map[string]*pool.CardRecord{}); got.Unsupported != "card not in the pool" {
		t.Fatalf("absent: %+v", got)
	}
	if DeckSizeBonus["yorion, sky nomad"] != 20 {
		t.Fatal("yorion")
	}
}
