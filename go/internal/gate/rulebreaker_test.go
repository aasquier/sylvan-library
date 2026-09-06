package gate_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The Rulebreaker clause, and the reason this file is a table of *printed
// sentences* against *synthetic cards*.
//
// The sentences are real and are not typed from memory (rule 1): every clause
// below was read out of the card pool on 2026-09-06 and pasted by machine, and
// either of these re-derives the set from scratch rather than trusting the
// table:
//
//	./mtglab cards show 'Seluma, Light of Aysen'
//	curl -s 'https://api.scryfall.com/cards/search?q=oracle%3Arulebreaker'
//
// The cards the clauses are tested *against* are invented, exactly as
// `companioncheck_test.go` invents its companions and for the same reason: what
// is being asserted is that the check reads the shape of a sentence and the
// shape of a type line, not that any particular Angel is red. The 21-card
// fixture pool carries no Mystery Booster commander and no off-identity Angel,
// so a golden-corpus case could not reach this code at all.
//
// What the table cannot cover is the ninth Rulebreaker card, whenever it is
// spoiled. That is what TestAnUnreadableRulebreakerIsReportedNotSwallowed is
// for, and it is the more important of the two.

// clause is one commander as the check sees it: a name, an identity, and the
// sentence printed on the card.
type clause struct{ commander, identity, text string }

var printed = []clause{
	{"Seluma, Light of Aysen", "W", "Rulebreaker — A deck with this commander can have Angel cards of any color identity and any basic land cards."},
	{"Maular, the Next Evolution", "G", "Rulebreaker — A deck with this commander can have creature cards with mana value 7 or greater of any color identity and any basic land cards."},
	{"The Everforger", "", "Rulebreaker — A deck with this commander can have artifact creature and Equipment cards of any color identity and any basic land cards."},
	{"The Unluckiest Planeswalker", "R", "Rulebreaker — A deck with this commander can have Aura cards of any color identity and any basic land cards."},
	{"Valko Indorian", "B", "Rulebreaker — A deck with this commander can have Phyrexian cards of any color identity and any basic land cards."},
	{"Grizzlegom, Hurloon Hero", "GR", "Rulebreaker — A deck with this commander can have any land cards."},
	{"Tolabow, Loch Rascal", "U", "Rulebreaker — If Tolabow, Loch Rascal is your commander, the color identity of instant and sorcery cards in your deck can include one color of your choice not in your commander's color identity, and your deck can have any basic land cards."},
	{"Whtz, the Bibliophile", "UW", "Rulebreaker — A deck with this commander has no maximum deck size."},
}

func clauseNamed(t *testing.T, name string) clause {
	t.Helper()
	for _, c := range printed {
		if c.commander == name {
			return c
		}
	}
	t.Fatalf("no clause recorded for %s", name)
	return clause{}
}

// commanderRec is a commander card carrying one printed clause.
func commanderRec(c clause) *pool.CardRecord {
	return &pool.CardRecord{
		Name: c.commander, CMC: 5, TypeLine: "Legendary Creature — Fixture",
		OracleText: c.text + "\nFlying", ColorIdentity: strings.Split(c.identity, ""),
		LegalCommander: true, Layout: "normal",
	}
}

// card is a synthetic deck card: a type line, a mana value and an identity are
// every field the clause reads.
func card(name, typeLine string, cmc float64, identity string) *pool.CardRecord {
	return &pool.CardRecord{
		Name: name, CMC: cmc, TypeLine: typeLine, ColorIdentity: strings.Split(identity, ""),
		LegalCommander: true, Layout: "normal",
	}
}

// TestEachPrintedClauseWidensExactlyWhatItNames is the whole set, each clause
// against a card it covers and a card it must not. The pairs matter more than
// the singles: a check that let everything through would pass the first column
// and fail the second, which is the way this could go wrong and still look
// fixed.
func TestEachPrintedClauseWidensExactlyWhatItNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		commander string
		covers    []*pool.CardRecord
		refuses   []*pool.CardRecord
	}{{
		commander: "Seluma, Light of Aysen",
		covers: []*pool.CardRecord{
			card("Red Angel", "Legendary Creature — Angel", 7, "RW"),
			card("Swamp", "Basic Land — Swamp", 0, "B"),
		},
		refuses: []*pool.CardRecord{
			card("Black Demon", "Creature — Demon", 5, "B"),
			// A nonbasic land is not "any basic land cards": Seluma's clause
			// names the basics, and Grizzlegom's is the one that does not.
			card("Dual Land", "Land — Swamp Island", 0, "BU"),
			// The letters of "Angel" inside a longer word are not the type.
			card("Not An Angel", "Creature — Angelkin", 4, "B"),
		},
	}, {
		commander: "Maular, the Next Evolution",
		covers: []*pool.CardRecord{
			card("Huge Blue Thing", "Creature — Kraken", 7, "U"),
			card("Huger Black Thing", "Creature — Demon", 9, "B"),
		},
		refuses: []*pool.CardRecord{
			// One under the floor, which is the boundary the clause names.
			card("Nearly Huge", "Creature — Kraken", 6, "U"),
			// Mana value alone is not enough; it has to be a creature.
			card("Huge Sorcery", "Sorcery", 8, "R"),
		},
	}, {
		commander: "The Everforger",
		covers: []*pool.CardRecord{
			card("Coloured Equipment", "Artifact — Equipment", 2, "W"),
			card("Coloured Artifact Creature", "Artifact Creature — Construct", 4, "U"),
			// Both halves of the clause at once, which is the card that would
			// break a parser that split the sentence on every "and".
			card("Living Weapon", "Artifact Creature — Equipment Cat", 2, "W"),
		},
		refuses: []*pool.CardRecord{
			// A creature that is not an artifact, and an artifact that is not
			// a creature or an Equipment: neither group matches.
			card("Plain Creature", "Creature — Cat", 2, "W"),
			card("Plain Artifact", "Artifact — Clue", 2, "U"),
		},
	}, {
		commander: "The Unluckiest Planeswalker",
		covers:    []*pool.CardRecord{card("White Aura", "Enchantment — Aura", 2, "W")},
		refuses:   []*pool.CardRecord{card("White Enchantment", "Enchantment", 4, "W")},
	}, {
		commander: "Valko Indorian",
		covers: []*pool.CardRecord{
			card("Blue Phyrexian", "Artifact Creature — Phyrexian Shapeshifter", 4, "U"),
		},
		refuses: []*pool.CardRecord{card("Blue Instant", "Instant", 2, "U")},
	}, {
		commander: "Grizzlegom, Hurloon Hero",
		covers: []*pool.CardRecord{
			// "Any land cards" subsumes the basics, which is why the clause
			// does not name them a second time.
			card("Shock Land", "Land — Island Swamp", 0, "BU"),
			card("Island", "Basic Land — Island", 0, "U"),
		},
		refuses: []*pool.CardRecord{card("Blue Instant", "Instant", 2, "U")},
	}, {
		commander: "Whtz, the Bibliophile",
		// Whtz changes how big the deck may be and nothing about its colours.
		refuses: []*pool.CardRecord{
			card("Black Demon", "Creature — Demon", 5, "B"),
			card("Swamp", "Basic Land — Swamp", 0, "B"),
		},
	}}
	for _, tc := range cases {
		t.Run(tc.commander, func(t *testing.T) {
			t.Parallel()
			rs := gate.ReadRulebreakers([]*pool.CardRecord{commanderRec(clauseNamed(t, tc.commander))})
			if unread := rs.Unread(); len(unread) > 0 {
				t.Fatalf("the printed clause did not parse: %s", unread[0].Unsupported)
			}
			for _, rec := range tc.covers {
				if !rs.AnyIdentity(rec) {
					t.Errorf("%s (%s) is not covered, and the clause names it", rec.Name, rec.TypeLine)
				}
			}
			for _, rec := range tc.refuses {
				if rs.AnyIdentity(rec) {
					t.Errorf("%s (%s) is covered, and the clause does not name it", rec.Name, rec.TypeLine)
				}
			}
		})
	}
}

// A double-faced card is its front face everywhere but the battlefield and the
// stack, so a card whose *back* is an Angel is not an Angel card in the ninety-
// nine. Reading the whole type line would have made this pass, quietly.
func TestARulebreakerReadsTheFrontFaceOfACard(t *testing.T) {
	t.Parallel()
	rs := gate.ReadRulebreakers([]*pool.CardRecord{commanderRec(clauseNamed(t, "Seluma, Light of Aysen"))})
	back := card("Two-Faced", "Creature — Human // Creature — Angel", 3, "BW")
	back.Layout = "transform"
	if rs.AnyIdentity(back) {
		t.Fatal("a card with an Angel on its back face was taken for an Angel card")
	}
}

// **The ninth Rulebreaker card, whenever it is printed.** A clause this file
// has not been taught is the one case that could go wrong in silence, in either
// direction -- widening nothing while the deck reads as broken, or widening
// everything while an illegal deck reads as fine. Neither is acceptable, so an
// unreadable clause says so and the identity check runs unwidened: the old
// wrong answer, with a sentence attached saying that is what happened.
func TestAnUnreadableRulebreakerIsReportedNotSwallowed(t *testing.T) {
	t.Parallel()
	future := commanderRec(clause{"Fixture Rulebreaker", "W",
		"Rulebreaker — A deck with this commander can have cards printed before the Brothers' War."})
	rs := gate.ReadRulebreakers([]*pool.CardRecord{future})
	if len(rs.Unread()) != 1 {
		t.Fatalf("a clause nobody can read was not reported: %+v", rs)
	}
	if rs.AnyIdentity(card("Black Demon", "Creature — Demon", 5, "B")) {
		t.Fatal("an unreadable clause widened the identity anyway")
	}
	if rs.NoMaximumDeckSize() {
		t.Fatal("an unreadable clause lifted the deck-size ceiling")
	}

	cards := map[string]*pool.CardRecord{
		"Fixture Rulebreaker": future,
		"Black Demon":         card("Black Demon", "Creature — Demon", 5, "B"),
	}
	report := gate.Validate(deckOf("Fixture Rulebreaker", []string{"Black Demon"}, cards), cards, 1)
	warned, errored := false, false
	for _, i := range report.Issues {
		if i.Code == "rulebreaker-unread" && i.Level == "warn" {
			warned = true
			if !strings.Contains(i.Message, "NOT applied") || !strings.Contains(i.Message, "Brothers' War") {
				t.Errorf("the warning does not say what happened or quote the clause: %s", i.Message)
			}
		}
		if i.Code == "color-identity" {
			errored = true
		}
	}
	if !warned || !errored {
		t.Fatalf("warned=%v errored=%v -- an unread clause must say so AND still check the identity", warned, errored)
	}
}

// Every way the parse can go, on one table. The rows that *fail* are the point:
// each is a sentence the check must refuse to guess at, and each has to be
// refused with a reason that names what could not be read -- "unsupported" on
// its own would send the next reader to a debugger.
func TestTheParserReadsWhatItCanAndNamesWhatItCannot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		what, text  string
		unsupported string // a fragment the refusal must carry; "" means it parses
		covers      *pool.CardRecord
	}{{
		// The other opening Wizards already uses inside Tolabow's own
		// sentence, standing on its own. Not a guess about a future card: the
		// words "your deck can have" are printed today.
		what: "the If-you-are-my-commander opening with a plain grant",
		text: "Rulebreaker — If Fixture Legend is your commander, your deck can have " +
			"Angel cards of any color identity and any basic land cards.",
		covers: card("Red Angel", "Creature — Angel", 7, "RW"),
	}, {
		what:   "a grant of the basics alone",
		text:   "Rulebreaker — A deck with this commander can have any basic land cards.",
		covers: card("Swamp", "Basic Land — Swamp", 0, "B"),
	}, {
		what:        "a sentence that never says what the deck can have",
		text:        "Rulebreaker — A deck with this commander has three heads.",
		unsupported: "does not say what the deck can have",
	}, {
		what:        "a grant whose subject is not a kind of card",
		text:        "Rulebreaker — A deck with this commander can have whatever it likes.",
		unsupported: "whatever it likes",
	}, {
		what:        "a grant whose subject is a kind of card the sentence does not name",
		text:        "Rulebreaker — A deck with this commander can have cards of any color identity.",
		unsupported: "cards of any color identity",
	}, {
		what: "an If-clause granting something this check cannot read",
		text: "Rulebreaker — If Fixture Legend is your commander, your library is upside down, " +
			"and your deck can have any basic land cards.",
		unsupported: "your library is upside down",
	}, {
		what:        "an opening nobody has templated",
		text:        "Rulebreaker — Decks helmed by this creature may run extra Angels.",
		unsupported: "opens in a way this check has not been taught",
	}}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			rb := gate.ReadRulebreaker(commanderRec(clause{"Fixture Legend", "W", tc.text}))
			if !rb.Printed() {
				t.Fatalf("the clause was not seen at all: %+v", rb)
			}
			if tc.unsupported == "" {
				if rb.Unsupported != "" {
					t.Fatalf("a readable clause was refused: %s", rb.Unsupported)
				}
				rs := gate.Rulebreakers{rb}
				if !rs.AnyIdentity(tc.covers) {
					t.Errorf("%s is not covered, and the clause names it", tc.covers.Name)
				}
				if !rs.AnyIdentity(card("Island", "Basic Land — Island", 0, "U")) {
					t.Error("the basics the clause grants are not covered")
				}
				return
			}
			if !strings.Contains(rb.Unsupported, tc.unsupported) {
				t.Fatalf("the refusal does not name what could not be read: %q, want it to carry %q",
					rb.Unsupported, tc.unsupported)
			}
			// Whatever it could not read, it must not have widened anything.
			if rs := (gate.Rulebreakers{rb}); rs.AnyIdentity(card("Red Angel", "Creature — Angel", 7, "RW")) ||
				rs.NoMaximumDeckSize() {
				t.Error("a clause that failed to parse widened something anyway")
			}
		})
	}
}

// A commander the pool does not know is a nil record, and a deck whose
// commander is missing already has an `unknown-card` error saying so. Reading
// clauses must skip it rather than panic on the way to that error.
func TestACommanderThePoolLacksCarriesNoClause(t *testing.T) {
	t.Parallel()
	rs := gate.ReadRulebreakers([]*pool.CardRecord{nil,
		commanderRec(clauseNamed(t, "Seluma, Light of Aysen")), nil})
	if len(rs) != 1 {
		t.Fatalf("nil records were not skipped: %d clauses", len(rs))
	}
	if !rs.AnyIdentity(card("Red Angel", "Creature — Angel", 7, "RW")) {
		t.Error("the one real clause was lost among the missing ones")
	}
}

// A card with no Rulebreaker at all is not a card with an unreadable one, and
// the gate for every deck in the library depends on telling those apart.
func TestACardWithNoRulebreakerWidensNothingAndComplainsAboutNothing(t *testing.T) {
	t.Parallel()
	plain := card("Fixture Commander", "Legendary Creature — Bear", 3, "G")
	plain.OracleText = "Trample\nWhenever this creature attacks, draw a card."
	if rb := gate.ReadRulebreaker(plain); rb.Printed() || rb.Unsupported != "" {
		t.Fatalf("an ordinary commander read as a Rulebreaker: %+v", rb)
	}
	rs := gate.ReadRulebreakers([]*pool.CardRecord{plain})
	if len(rs) != 0 || rs.Why() != "" || rs.NoMaximumDeckSize() {
		t.Fatalf("an ordinary commander widened something: %+v", rs)
	}
}

// deckOf is a minimal curated deck: a commander and some names, each with a
// rationale and a category that matches its type line, so that nothing but the
// check under test can put a row in the report.
func deckOf(commander string, names []string, cards map[string]*pool.CardRecord) *deck.Deck {
	d := &deck.Deck{Slug: "fixture", Name: "Fixture", Status: "theoretical", Stage: "curated",
		Commander: []string{commander}}
	for _, n := range names {
		category := "threat"
		if rec := cards[n]; rec != nil && rec.IsLand() {
			category = "land"
		}
		d.Cards = append(d.Cards, deck.CardEntry{Name: n, Category: category, Qty: 1,
			Why: "A fixture entry."})
	}
	return d
}

// Whtz lifts the ceiling and leaves the floor. Both halves are asserted because
// a check that simply stopped counting would pass the first.
func TestNoMaximumDeckSizeLiftsTheCeilingAndKeepsTheFloor(t *testing.T) {
	t.Parallel()
	whtz := commanderRec(clauseNamed(t, "Whtz, the Bibliophile"))
	cards := map[string]*pool.CardRecord{"Whtz, the Bibliophile": whtz}
	names := []string{}
	for _, n := range []string{"One", "Two", "Three", "Four"} {
		cards[n] = card(n, "Creature — Fixture", 2, "U")
		names = append(names, n)
	}
	d := deckOf("Whtz, the Bibliophile", names, cards)

	// Four cards where two are expected: the clause says that is allowed.
	for _, i := range gate.Validate(d, cards, 2).Issues {
		if i.Code == "deck-size" {
			t.Fatalf("a deck over the size was refused: %s", i.Message)
		}
	}
	// Four where six are expected: the clause never said that was allowed.
	found := ""
	for _, i := range gate.Validate(d, cards, 6).Issues {
		if i.Code == "deck-size" {
			found = i.Message
		}
	}
	if !strings.Contains(found, "at least 6") {
		t.Fatalf("a deck under the size was not refused as a floor: %q", found)
	}
	// And a commander with no such clause still wants the number exactly.
	plain := card("Plain Commander", "Legendary Creature — Bear", 3, "U")
	plainDeck := deckOf("Plain Commander", names, cards)
	cards["Plain Commander"] = plain
	found = ""
	for _, i := range gate.Validate(plainDeck, cards, 2).Issues {
		if i.Code == "deck-size" {
			found = i.Message
		}
	}
	if !strings.Contains(found, "expected 2") || strings.Contains(found, "at least") {
		t.Fatalf("an ordinary deck's size stopped being exact: %q", found)
	}
}

// Tolabow's one colour, derived from the deck rather than stored in it. The
// three cases are the whole rule: none, one, and two.
func TestOneChosenColourIsReadBackOffTheInstantsAndSorceries(t *testing.T) {
	t.Parallel()
	tolabow := commanderRec(clauseNamed(t, "Tolabow, Loch Rascal"))
	base := map[string]*pool.CardRecord{
		"Tolabow, Loch Rascal": tolabow,
		"Red Instant":          card("Red Instant", "Instant", 2, "R"),
		"Red Sorcery":          card("Red Sorcery", "Sorcery", 3, "R"),
		"Green Sorcery":        card("Green Sorcery", "Sorcery", 3, "G"),
		"Red Creature":         card("Red Creature", "Creature — Goblin", 2, "R"),
		"Swamp":                card("Swamp", "Basic Land — Swamp", 0, "B"),
	}
	choice := func(names ...string) []gate.Issue {
		return gate.Validate(deckOf("Tolabow, Loch Rascal", names, base), base, len(names)).Issues
	}
	codes := func(issues []gate.Issue) []string {
		out := []string{}
		for _, i := range issues {
			out = append(out, i.Code)
		}
		return out
	}

	// One colour across two spells, plus a basic: legal, and silently so.
	if got := codes(choice("Red Instant", "Red Sorcery", "Swamp")); len(got) != 0 {
		t.Fatalf("a legal Tolabow deck was not clean: %v", got)
	}
	// Two colours: no single choice covers both, and the report says which.
	issues := choice("Red Instant", "Green Sorcery")
	found := ""
	for _, i := range issues {
		if i.Code == "rulebreaker-color-choice" {
			found = i.Message
		}
	}
	if !strings.Contains(found, "{G} (Green Sorcery)") || !strings.Contains(found, "{R} (Red Instant)") {
		t.Fatalf("two colours were not reported with the cards that reach them: %q -- all of %v",
			found, codes(issues))
	}
	// The clause covers instants and sorceries and stops there: an off-colour
	// creature is an ordinary identity error, not a colour choice.
	if got := codes(choice("Red Creature")); len(got) != 1 || got[0] != "color-identity" {
		t.Fatalf("an off-colour creature under Tolabow reported %v", got)
	}
}

// The clause widens the cards it names; it does not widen the deck. A Seluma
// deck may hold a red Angel and a Swamp and still not hold a red instant, and
// the error it gets quotes the clause so the reason is on the page rather than
// in this file.
func TestARulebreakerWidensTheCardsItNamesAndNotTheDeck(t *testing.T) {
	t.Parallel()
	seluma := commanderRec(clauseNamed(t, "Seluma, Light of Aysen"))
	cards := map[string]*pool.CardRecord{
		"Seluma, Light of Aysen": seluma,
		"Red Angel":              card("Red Angel", "Creature — Angel", 7, "RW"),
		"Swamp":                  card("Swamp", "Basic Land — Swamp", 0, "B"),
		"Red Instant":            card("Red Instant", "Instant", 2, "R"),
	}
	report := gate.Validate(deckOf("Seluma, Light of Aysen",
		[]string{"Red Angel", "Swamp", "Red Instant"}, cards), cards, 3)
	blamed := []string{}
	for _, i := range report.Issues {
		if i.Code != "color-identity" {
			continue
		}
		blamed = append(blamed, *i.Card)
		if !strings.Contains(i.Message, `Seluma, Light of Aysen's Rulebreaker does not cover it: `+
			`"A deck with this commander can have Angel cards of any color identity and any basic land cards."`) {
			t.Errorf("the error does not name the clause it failed, and quote it: %s", i.Message)
		}
	}
	if len(blamed) != 1 || blamed[0] != "Red Instant" {
		t.Fatalf("identity errors on %v, want only the red instant", blamed)
	}
}
