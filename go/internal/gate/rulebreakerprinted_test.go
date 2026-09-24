package gate_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Every Rulebreaker clause Wizards has printed, read as the parser reads it.
//
// `TestTheParserReadsWhatItCanAndNamesWhatItCannot` drives the refusals with
// clauses nobody has printed, which is right for a parser's edges and wrong
// as the only corpus: the check exists for the cards that exist. These are
// the twelve, and **every sentence below was read out of the real pool on
// 2026-09-24** (`SELECT name, oracle_text FROM oracle_cards WHERE
// oracle_text LIKE '%Rulebreaker%'`), never typed from memory -- rule 1
// applies to a fixture that names a real card. The reminder text after the
// clause is left off; the parser reads the first sentence.
//
// What is asked of each is small and the same: the clause is seen, and it
// either parses or says in a sentence what it could not read. Which of the
// two is recorded beside it, because a card moving from one column to the
// other is news -- a clause this check stops understanding is a deck the gate
// starts refusing, which is the worse half of ADR 51's bug.
func TestEveryPrintedRulebreakerIsReadOrNamed(t *testing.T) {
	t.Parallel()
	printed := []struct {
		commander, identity, text string
		unsupported               bool
	}{
		{"Arvad of the Weatherlight", "WB", "Rulebreaker — If Arvad of the Weatherlight is your Commander, you may include legendary permanents of any color in your deck regardless of color identity.", true},
		{"Daxiver, Izzet Electromancer", "UR", "Rulebreaker — If Daxiver, Izzet Electromancer is your Commander, you may include instant and sorcery cards of any color in your deck regardless of color identity.", true},
		{"Grizzlegom, Hurloon Hero", "RW", "Rulebreaker — A deck with this commander can have any land cards.", false},
		{"Hadran, Naya Sunseeder", "RGW", "Rulebreaker — If Hadran, Naya Sunseeder is your Commander, you may include creature cards of power 4 or greater of any color in your deck regardless of color identity.", true},
		{"Maular, the Next Evolution", "RG", "Rulebreaker — A deck with this commander can have creature cards with mana value 7 or greater of any color identity and any basic land cards.", false},
		{"Seluma, Light of Aysen", "W", "Rulebreaker — A deck with this commander can have Angel cards of any color identity and any basic land cards.", false},
		{"The Everforger", "", "Rulebreaker — A deck with this commander can have artifact creature and Equipment cards of any color identity and any basic land cards.", false},
		{"The Unluckiest Planeswalker", "R", "Rulebreaker — A deck with this commander can have Aura cards of any color identity and any basic land cards.", false},
		{"Tolabow, Loch Rascal", "U", "Rulebreaker — If Tolabow, Loch Rascal is your commander, the color identity of instant and sorcery cards in your deck can include one color of your choice not in your commander's color identity, and your deck can have any basic land cards.", false},
		{"Valko Indorian", "B", "Rulebreaker — A deck with this commander can have Phyrexian cards of any color identity and any basic land cards.", false},
		{"Valko Indorian, Researcher", "UB", "Rulebreaker — If Valko Indorian, Researcher is your Commander, you may include all artifacts and enchantments in your deck regardless of color identity.", true},
		{"Whtz, the Bibliophile", "U", "Rulebreaker — A deck with this commander has no maximum deck size.", false},
	}
	var all gate.Rulebreakers
	for _, p := range printed {
		rec := &pool.CardRecord{
			Name: p.commander, CMC: 4, TypeLine: "Legendary Creature — Fixture",
			OracleText: p.text + "\nFlying", ColorIdentity: strings.Split(p.identity, ""),
			LegalCommander: true, Layout: "normal",
		}
		rb := gate.ReadRulebreaker(rec)
		if !rb.Printed() {
			t.Errorf("%s: the clause was not seen at all", p.commander)
			continue
		}
		all = append(all, rb)
		switch {
		case p.unsupported && rb.Unsupported == "":
			t.Errorf("%s: the parser now reads a clause this corpus records as beyond "+
				"it -- if that is deliberate, move the card to the other column and "+
				"add the pair `TestEachPrintedClauseWidensExactlyWhatItNames` asks for",
				p.commander)
		case !p.unsupported && rb.Unsupported != "":
			t.Errorf("%s: a clause the gate used to read is refused now: %s",
				p.commander, rb.Unsupported)
		case p.unsupported && !strings.Contains(rb.Unsupported, "cannot") &&
			!strings.Contains(rb.Unsupported, "not been taught") &&
			!strings.Contains(rb.Unsupported, "does not say"):
			t.Errorf("%s: the refusal does not read as a sentence about what could "+
				"not be read: %q", p.commander, rb.Unsupported)
		}
	}
	if len(all) != len(printed) {
		t.Fatalf("%d of %d clauses were seen", len(all), len(printed))
	}
	// The search hints skip what could not be read, and carry the rest: a
	// card whose clause the check does not understand widens no query.
	hints := all.TypeHints()
	if len(hints) == 0 {
		t.Fatal("twelve printed clauses produced no search hint at all")
	}
	for _, h := range hints {
		if strings.Contains(strings.ToLower(h), "regardless") {
			t.Errorf("a hint was read off a clause the parser refused: %q", h)
		}
	}
}
