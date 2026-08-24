package gate_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The companion check, driven with hand-built records rather than through the
// 21-card pool.
//
// The pool fixture holds no companion, so the gate's companion branch has
// only ever been reached far enough to return early. That is a real hole:
// a companion is **the** rule a newcomer is most likely to get wrong -- it
// sits outside the 99, it has a deckbuilding restriction the rest of the deck
// has to satisfy, and getting it wrong means an illegal deck that looks fine.
// CLAUDE.md's second commandment is that nothing ships that makes a newcomer
// feel stupid; a gate that stayed quiet about a broken companion would do
// exactly that at the table rather than at the keyboard.
//
// Records are built here rather than looked up because the check's input is
// already a `map[string]*pool.CardRecord` -- the seam exists, and using it
// keeps this test free of a pool without inventing card facts: every field
// below is either a shape the check reads or a value with no bearing on it.

// rec is a card record with the fields the gate reads.
func rec(name, typeLine, oracle string, identity []string, legal bool) *pool.CardRecord {
	cost := "{2}{G}"
	return &pool.CardRecord{
		Name: name, ManaCost: &cost, CMC: 3, TypeLine: typeLine,
		OracleText: oracle, ColorIdentity: identity,
		LegalCommander: legal, Layout: "normal",
	}
}

// **Every card below is synthetic.** The check reads the *shape* of a
// Companion sentence and the restriction it names, not any particular card's
// text -- and rule 1 forbids writing a real card's oracle text from memory,
// so nothing here claims to be one. `internal/gate`'s golden corpus is where
// real cards are asserted, against the pool.
const beastCompanion = "Companion — Each creature card in your starting deck is a " +
	"Beast card. (If this card is your chosen companion, you may put it into " +
	"your hand from outside the game for {3} as a sorcery.)\nVigilance"

// A card listed as a companion that has no Companion ability at all is the
// commonest way to get this wrong, and it is an error rather than a warning:
// the deck is illegal and no amount of play will reveal it.
func TestACardWithNoCompanionAbilityIsRefusedAsOne(t *testing.T) {
	t.Parallel()
	companion := "Fixture Plain Creature"
	d := &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
	}
	cards := map[string]*pool.CardRecord{
		"Fixture Commander":      rec("Fixture Commander", "Legendary Creature — Bear", "", []string{"G"}, true),
		"Fixture Plain Creature": rec("Fixture Plain Creature", "Creature — Elf Druid", "{T}: Add {G}.", []string{"G"}, true),
	}

	report := gate.Validate(d, cards, 100)
	if !hasCode(report, "not-a-companion") {
		t.Fatalf("a card with no Companion ability passed as one: %v", codes(report))
	}
	if issue := find(report, "not-a-companion"); issue.Level != "error" {
		t.Errorf("the issue is a %s -- an illegal deck is not a warning", issue.Level)
	}
	if issue := find(report, "not-a-companion"); issue.Card == nil || *issue.Card != companion {
		t.Errorf("the issue does not name the card: %v", issue.Card)
	}
}

// A companion outside the commander's colour identity is illegal for the same
// reason any card is, and it is easy to miss because the companion is not in
// the 99 where a colour check is expected.
func TestACompanionOutsideTheIdentityIsRefused(t *testing.T) {
	t.Parallel()
	companion := "Fixture Companion"
	d := &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
	}
	cards := map[string]*pool.CardRecord{
		// A mono-green commander.
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", []string{"G"}, true),
		// A companion that is also white.
		"Fixture Companion": rec("Fixture Companion",
			"Legendary Creature — Cat Beast", beastCompanion, []string{"G", "W"}, true),
	}

	report := gate.Validate(d, cards, 100)
	if !hasCode(report, "companion-color-identity") {
		t.Fatalf("a companion outside the identity passed: %v", codes(report))
	}
	issue := find(report, "companion-color-identity")
	if issue.Level != "error" {
		t.Errorf("the issue is a %s", issue.Level)
	}
	// The sentence names both identities, because the reader's question is
	// "which colour is the problem".
	if !strings.Contains(issue.Message, "W") {
		t.Errorf("the message does not name the offending colour: %q", issue.Message)
	}
}

// A companion that is not legal in Commander cannot be your companion, and
// that is a separate finding from the identity one -- a card can fail both.
func TestACompanionBannedInCommanderIsRefused(t *testing.T) {
	t.Parallel()
	companion := "Fixture Banned Companion"
	d := &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
	}
	cards := map[string]*pool.CardRecord{
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", []string{"G"}, true),
		"Fixture Banned Companion": rec("Fixture Banned Companion",
			"Legendary Creature — Otter Elemental",
			"Companion — Each nonland card in your starting deck has a different name.",
			[]string{"G"}, false),
	}

	report := gate.Validate(d, cards, 100)
	if !hasCode(report, "companion-banned") {
		t.Fatalf("a banned companion passed: %v", codes(report))
	}
	if issue := find(report, "companion-banned"); issue.Level != "error" {
		t.Errorf("the issue is a %s", issue.Level)
	}
}

// A companion the pool has never heard of is already reported as an unknown
// card, so the companion check says nothing further -- one card, one
// diagnosis, rather than two findings a reader has to reconcile.
func TestAnUnknownCompanionIsReportedOnceRatherThanTwice(t *testing.T) {
	t.Parallel()
	companion := "Nonexistent Card"
	d := &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
	}
	cards := map[string]*pool.CardRecord{
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", []string{"G"}, true),
	}

	report := gate.Validate(d, cards, 100)
	for _, code := range []string{"not-a-companion", "companion-banned", "companion-color-identity"} {
		if hasCode(report, code) {
			t.Errorf("an unknown card also produced %q -- one card, one diagnosis", code)
		}
	}
}

// A companion whose condition the gate has no checker for is a **warning that
// says it was not verified**, never silence.
//
// This is the honest answer and the important one: silence would read as
// "checked and fine", and a newcomer would take an illegal deck to a table on
// the strength of it. The gate would rather say "I could not check this" than
// be quietly wrong -- the same discipline as quoting a cached number as
// cached.
func TestAnUncheckableConditionSaysItWasNotVerified(t *testing.T) {
	t.Parallel()
	companion := "Fixture Companion"
	d := &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
	}
	cards := map[string]*pool.CardRecord{
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Beast", "", []string{"G", "W"}, true),
		"Fixture Companion": rec("Fixture Companion",
			"Legendary Creature — Cat Beast", beastCompanion, []string{"G", "W"}, true),
	}

	report := gate.Validate(d, cards, 100)

	// No error: the companion itself is legal and inside the identity.
	for _, code := range []string{"not-a-companion", "companion-banned",
		"companion-color-identity"} {
		if hasCode(report, code) {
			t.Errorf("a legal companion produced %q", code)
		}
	}
	// But the restriction is reported as unverified rather than passed.
	if !hasCode(report, "companion-unchecked") {
		t.Fatalf("an uncheckable condition passed silently: %v", codes(report))
	}
	issue := find(report, "companion-unchecked")
	if issue.Level != "warn" {
		t.Errorf("the unchecked notice is a %s -- it is not the deck's fault", issue.Level)
	}
	if !strings.Contains(issue.Message, "NOT verified") {
		t.Errorf("the notice does not say it could not check: %q", issue.Message)
	}
	// And it quotes the condition, so a person can check it by hand.
	if !strings.Contains(issue.Message, "Beast card") {
		t.Errorf("the notice does not quote the condition: %q", issue.Message)
	}
}

// The Companion sentence is read off the oracle text with its reminder
// stripped, because the reminder is a printing's furniture rather than the
// rule -- and a condition with the reminder still attached would be quoted
// back to a reader as if the parenthetical were part of the restriction.
func TestTheCompanionConditionIsReadWithoutItsReminder(t *testing.T) {
	t.Parallel()
	card := rec("Fixture Companion", "Legendary Creature — Cat Beast",
		beastCompanion, []string{"G", "W"}, true)

	condition := gate.ConditionText(card)
	if condition == "" {
		t.Fatal("no Companion sentence was found")
	}
	if strings.Contains(condition, "(If this card is your chosen companion") {
		t.Errorf("the reminder survived: %q", condition)
	}
	if !strings.Contains(condition, "Companion —") {
		t.Errorf("the condition is %q", condition)
	}
	// The line after it is not part of the condition.
	if strings.Contains(condition, "Vigilance") {
		t.Errorf("the next ability was swept in: %q", condition)
	}

	if !gate.IsCompanion(card) {
		t.Error("a card with a Companion sentence is not a companion")
	}
	for _, notOne := range []*pool.CardRecord{
		rec("Fixture Plain Creature", "Creature — Elf Druid", "{T}: Add {G}.", []string{"G"}, true),
		rec("Fixture Artifact", "Artifact", "{T}: Add {C}{C}.", nil, true),
		// The word alone is not the ability.
		rec("Something", "Creature", "Companion creatures get +1/+1.", nil, true),
	} {
		if gate.IsCompanion(notOne) {
			t.Errorf("%q read as a companion on %q", notOne.Name, notOne.OracleText)
		}
	}
}

// A pairing describes itself the way the report records it, and every kind
// has a sentence -- an unnamed one would render as a blank beside a card.
func TestEveryPairingKindDescribesItself(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pairing gate.Pairing
		wants   string
	}{
		{gate.Pairing{Kind: gate.Partner}, "Partner"},
		{gate.Pairing{Kind: gate.Labeled, Label: "Survivors"}, "Partner—Survivors"},
		{gate.Pairing{Kind: gate.PartnerWith, PartnerName: "Tymna"}, "Partner with Tymna"},
		{gate.Pairing{Kind: gate.BackgroundChooser}, "Choose a Background"},
		{gate.Pairing{Kind: gate.DoctorsCompanion}, "Doctor's companion"},
	} {
		got := tc.pairing.Describe()
		if got != tc.wants {
			t.Errorf("%+v described as %q, want %q", tc.pairing, got, tc.wants)
		}
	}
	// An unknown kind falls back to the kind itself rather than to nothing,
	// so a new ability shows up as its own name instead of a blank.
	if got := (gate.Pairing{Kind: "something-new"}).Describe(); got != "something-new" {
		t.Errorf("an unknown kind described as %q", got)
	}
	if got := (gate.Pairing{}).Describe(); got != "" {
		t.Errorf("no pairing described as %q", got)
	}
}

// hasCode, find and codes read a report the way a reader does.
func hasCode(r *gate.Report, code string) bool {
	for _, i := range r.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func find(r *gate.Report, code string) gate.Issue {
	for _, i := range r.Issues {
		if i.Code == code {
			return i
		}
	}
	return gate.Issue{}
}

func codes(r *gate.Report) []string {
	out := make([]string, 0, len(r.Issues))
	for _, i := range r.Issues {
		out = append(out, i.Code)
	}
	return out
}
