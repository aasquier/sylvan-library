package gate_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// What the report **says**, as against what it finds.
//
// The gate's findings are well covered against the pool; the sentences around
// them are not, and they are the half a person actually reads. Two shapes
// recur and both truncate at six with an "and N more" tail -- the draft's
// unjustified list and the companion's violation list -- and a truncation is
// the classic place for an off-by-one that nobody notices, because the list
// still looks about right. This file counts.
//
// The other half is the refusals nothing in the 21-card pool can produce: a
// stage that is not a stage, a legacy archetype that is not a class, three
// commanders, and the one companion that changes how big the deck must be.
// All four are things a hand-written file can say and the app never writes,
// which is exactly the audience commandment 2 names.

// aCard is one synthetic card entry, with a `why` unless one is not wanted.
func aCard(name, why string) deck.CardEntry {
	return deck.CardEntry{Name: name, Category: "utility", Qty: 1, Why: why, Tags: []string{}}
}

// fill is `n` synthetic cards, each with a rationale, so a deck reaches its
// expected size without any of them being the thing under test.
func fill(n int, prefix string) []deck.CardEntry {
	out := make([]deck.CardEntry, 0, n)
	for i := range n {
		out = append(out, aCard(fmt.Sprintf("%s %02d", prefix, i), "Filler."))
	}
	return out
}

// A stage the model does not know is an error naming both the value and the
// two that are allowed -- a person who wrote `stage: finished` needs to be
// told what to write instead, not merely that they were wrong.
func TestAStageOrStatusTheModelDoesNotKnowNamesWhatIsAllowed(t *testing.T) {
	t.Parallel()
	for _, row := range []struct{ status, stage, code string }{
		{"theoretical", "finished", "deck-stage"},
		{"theoretical", "", "deck-stage"},
		{"finished", "draft", "deck-status"},
		{"", "draft", "deck-status"},
	} {
		d := &deck.Deck{Slug: "d", Name: "D", Status: row.status, Stage: row.stage,
			Commander: []string{"Fixture Commander"}, Cards: fill(99, "Filler")}
		report := gate.Validate(d, map[string]*pool.CardRecord{}, 100)
		if !hasCode(report, row.code) {
			t.Errorf("status %q stage %q passed: %v", row.status, row.stage, codes(report))
			continue
		}
		issue := find(report, row.code)
		if issue.Level != "error" {
			t.Errorf("%s is a %s", row.code, issue.Level)
		}
		// The two legal values are both named, so the message is a fix.
		want := []string{"draft", "curated"}
		if row.code == "deck-status" {
			want = []string{"built", "theoretical"}
		}
		for _, w := range want {
			if !strings.Contains(issue.Message, w) {
				t.Errorf("%s does not offer %q: %q", row.code, w, issue.Message)
			}
		}
	}
}

// The legacy `archetype:` key is always a warning, and it says something
// **extra** when the word is not a class the boards know -- because a deck
// carrying `archetype: spicy` is not merely using an old key, it is using an
// old key to say nothing at all, and the two deserve different sentences.
func TestALegacyArchetypeSaysMoreWhenItIsNotAClass(t *testing.T) {
	t.Parallel()
	build := func(archetype string) gate.Issue {
		t.Helper()
		d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
			Commander: []string{"Fixture Commander"}, LegacyArchetype: archetype,
			Cards: fill(99, "Filler")}
		report := gate.Validate(d, map[string]*pool.CardRecord{}, 100)
		if !hasCode(report, "legacy-archetype") {
			t.Fatalf("archetype %q produced no notice: %v", archetype, codes(report))
		}
		return find(report, "legacy-archetype")
	}
	known := build("midrange")
	if known.Level != "warn" {
		t.Errorf("a known class is a %s -- the key is legacy, the deck is not wrong", known.Level)
	}
	if strings.Contains(known.Message, "counts for nothing") {
		t.Errorf("a known class was told it counts for nothing: %q", known.Message)
	}
	unknown := build("spicy")
	if !strings.Contains(unknown.Message, "counts for nothing") ||
		!strings.Contains(unknown.Message, "spicy") {
		t.Errorf("an unknown class was not told so: %q", unknown.Message)
	}
	// Both still point at the replacement, which is the actionable half.
	for _, issue := range []gate.Issue{known, unknown} {
		if !strings.Contains(issue.Message, "themes") {
			t.Errorf("the notice does not name the key that replaced it: %q", issue.Message)
		}
	}
}

// Three commanders is its own error rather than only a deck-size complaint --
// the size arithmetic silently accommodates any number of them, so without
// this a four-commander deck would validate clean at 96 cards.
func TestThreeCommandersIsItsOwnRefusalAndNotJustASizeComplaint(t *testing.T) {
	t.Parallel()
	// Three commanders take two off the expected size, so 98 cards satisfies
	// the size check and the only thing left to notice is the count itself.
	d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture One", "Fixture Two", "Fixture Three"},
		Cards:     fill(98, "Filler")}
	report := gate.Validate(d, map[string]*pool.CardRecord{}, 100)
	if hasCode(report, "deck-size") {
		t.Fatalf("the fixture is the wrong size, so this proves nothing: %q",
			find(report, "deck-size").Message)
	}
	if !hasCode(report, "too-many-commanders") {
		t.Fatalf("three commanders passed: %v", codes(report))
	}
	issue := find(report, "too-many-commanders")
	if issue.Level != "error" {
		t.Errorf("three commanders is a %s", issue.Level)
	}
	if !strings.Contains(issue.Message, "3 commanders") || !strings.Contains(issue.Message, "two") {
		t.Errorf("the message does not say how many or how many are allowed: %q", issue.Message)
	}
	// Two is fine -- so this is a rule rather than a blanket refusal of the
	// pairing that makes two legal in the first place.
	two := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture One", "Fixture Two"}, Cards: fill(99, "Filler")}
	if hasCode(gate.Validate(two, map[string]*pool.CardRecord{}, 100), "too-many-commanders") {
		t.Error("two commanders was refused")
	}
}

// **The one companion that changes the deck's size**, and the sentence that
// explains the number -- because "expected 120" with no reason is a person
// counting their cards twice and finding a hundred each time.
//
// The name is the code's own key, read out of `gate.DeckSizeBonus` rather
// than remembered, and nothing here asserts anything about the card beyond
// what that table says.
func TestTheSizeBonusCompanionExplainsTheNumberItAskedFor(t *testing.T) {
	t.Parallel()
	var name string
	var bonus int
	for key, n := range gate.DeckSizeBonus {
		name, bonus = key, n
	}
	if name == "" {
		t.Skip("no companion changes the deck size any more")
	}
	// Title-cased back, because a deck file writes the printed name and the
	// lookup is what lowers it -- which is itself the thing under test.
	printed := strings.ToUpper(name[:1]) + name[1:]
	companion := printed
	d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
		Cards: fill(99, "Filler")}
	cards := map[string]*pool.CardRecord{
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", nil, true),
		printed:             rec(printed, "Legendary Creature — Bird", beastCompanion, nil, true),
	}
	report := gate.Validate(d, cards, 100)
	if !hasCode(report, "deck-size") {
		t.Fatalf("a 99-card deck with the size companion passed: %v", codes(report))
	}
	issue := find(report, "deck-size")
	if !strings.Contains(issue.Message, fmt.Sprintf("expected %d", 100+bonus)) {
		t.Errorf("the expected size does not include the bonus: %q", issue.Message)
	}
	if !strings.Contains(issue.Message, fmt.Sprintf("+%d for %s", bonus, printed)) {
		t.Errorf("the message does not explain where the number came from: %q", issue.Message)
	}
	// And at the bigger size it is quiet, so the bonus is applied rather than
	// merely mentioned.
	d.Cards = fill(100+bonus, "Filler")
	if report := gate.Validate(d, cards, 100); hasCode(report, "deck-size") {
		t.Errorf("the bonus was named but not applied: %q", find(report, "deck-size").Message)
	}
}

// A draft's unjustified list shows six and counts the rest. The tail is the
// half worth holding: `len(pending) - 6` is one subtraction away from being
// wrong, and a wrong one reads as perfectly plausible.
func TestTheDraftListShowsSixAndCountsTheRest(t *testing.T) {
	t.Parallel()
	for _, blank := range []int{1, 6, 7, 20} {
		cards := append(fill(99-blank, "Justified"), func() []deck.CardEntry {
			out := []deck.CardEntry{}
			for i := range blank {
				out = append(out, aCard(fmt.Sprintf("Blank %02d", i), ""))
			}
			return out
		}()...)
		d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "draft",
			Commander: []string{"Fixture Commander"}, Cards: cards}
		report := gate.Validate(d, map[string]*pool.CardRecord{}, 100)
		if !hasCode(report, "draft-incomplete") {
			t.Errorf("%d blank rationales passed: %v", blank, codes(report))
			continue
		}
		msg := find(report, "draft-incomplete").Message
		// The count is the real one, not the shown one.
		if !strings.Contains(msg, fmt.Sprintf("%d of 99 cards", blank)) {
			t.Errorf("%d blank: the count is wrong: %q", blank, msg)
		}
		// At most six are named...
		named := 0
		for i := range blank {
			if strings.Contains(msg, fmt.Sprintf("Blank %02d", i)) {
				named++
			}
		}
		if want := min(blank, 6); named != want {
			t.Errorf("%d blank: %d names shown, want %d: %q", blank, named, want, msg)
		}
		// ...and the tail counts exactly the ones that were not.
		if blank > 6 {
			if !strings.Contains(msg, fmt.Sprintf("and %d more", blank-6)) {
				t.Errorf("%d blank: the tail is wrong: %q", blank, msg)
			}
		} else if strings.Contains(msg, "more") {
			t.Errorf("%d blank: a tail appeared with nothing in it: %q", blank, msg)
		}
	}
	// A curated deck says nothing about it here at all -- the promotion gate
	// is what refuses a blank rationale once the deck is out of draft, and
	// two notices for one fault is one too many (ADR 13).
	d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture Commander"},
		Cards:     append(fill(98, "Justified"), aCard("Blank 00", ""))}
	if hasCode(gate.Validate(d, map[string]*pool.CardRecord{}, 100), "draft-incomplete") {
		t.Error("a curated deck was given the draft's notice")
	}
}

// The companion sits **outside** the hundred. A file that lists it in the 99
// as well has one card too many and a companion that is not legal, and the
// gate has to say the second -- the deck-size error alone would send somebody
// hunting for a card they already know about.
func TestACompanionAlsoListedInThe99IsRefusedAsThat(t *testing.T) {
	t.Parallel()
	companion := "Fixture Companion"
	d := &deck.Deck{
		Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture Commander"}, Companion: &companion,
		Cards: append(fill(98, "Filler"), aCard("Fixture Companion", "In the deck by mistake.")),
	}
	cards := map[string]*pool.CardRecord{
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", nil, true),
		"Fixture Companion": rec("Fixture Companion", "Legendary Creature — Cat Beast",
			beastCompanion, nil, true),
	}
	report := gate.Validate(d, cards, 100)
	if !hasCode(report, "companion-in-99") {
		t.Fatalf("a companion inside the 99 passed: %v", codes(report))
	}
	issue := find(report, "companion-in-99")
	if issue.Level != "error" {
		t.Errorf("the issue is a %s", issue.Level)
	}
	if issue.Card == nil || *issue.Card != companion {
		t.Errorf("the issue does not name the card: %v", issue.Card)
	}
	// Once only, however many copies are in there -- the loop breaks, and a
	// deck with four of them should not produce four identical findings.
	d.Cards = append(d.Cards, aCard("Fixture Companion", "And again."))
	twice := gate.Validate(d, cards, 100)
	count := 0
	for _, c := range codes(twice) {
		if c == "companion-in-99" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("two copies produced %d findings, want 1", count)
	}
}

// TestTheViolationListShowsSixSortedAndCountsTheRest is the companion
// restriction's own renderer, and the second of the two truncations.
//
// **The companion's name here is a key, not a card fact.** `gate.checks` maps
// a lowercased name to a checker, so selecting a checker means naming one --
// and the two selected below ignore the companion's own record entirely
// (their signatures take it as `_`), so every judgement made is about the
// synthetic ninety-nine. The oracle text is an obvious placeholder and is not
// any card's real Companion sentence; nothing here asserts one.
func TestTheViolationListShowsSixSortedAndCountsTheRest(t *testing.T) {
	t.Parallel()
	// An exact checker that reads only mana values: odd ones are violations.
	const exactChecker = "Gyruda, Doom of Depths"
	for _, breaking := range []int{1, 6, 9} {
		cards := map[string]*pool.CardRecord{
			"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", nil, true),
			exactChecker: rec(exactChecker, "Legendary Creature — Fixture",
				"Companion — (placeholder; the checker does not read this)", nil, true),
		}
		entries := []deck.CardEntry{}
		for i := range 99 {
			name := fmt.Sprintf("Fixture %02d", i)
			entries = append(entries, aCard(name, "Filler."))
			// `rec` builds mana value 3; an even one is legal here.
			r := rec(name, "Creature — Fixture", "", nil, true)
			if i >= breaking {
				r.CMC = 4
			}
			cards[name] = r
		}
		// The commander is judged too, and it is odd, so it is one of them.
		cards["Fixture Commander"].CMC = 4
		companion := exactChecker
		d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
			Commander: []string{"Fixture Commander"}, Companion: &companion, Cards: entries}

		report := gate.Validate(d, cards, 100)
		if !hasCode(report, "companion-restriction") {
			t.Errorf("%d breaking cards passed: %v", breaking, codes(report))
			continue
		}
		issue := find(report, "companion-restriction")
		if issue.Level != "error" {
			t.Errorf("%d breaking: an exact check is a %s, want error", breaking, issue.Level)
		}
		if !strings.Contains(issue.Message, fmt.Sprintf("%d card(s)", breaking)) {
			t.Errorf("%d breaking: the count is wrong: %q", breaking, issue.Message)
		}
		named := strings.Count(issue.Message, "Fixture ")
		if want := min(breaking, 6); named != want {
			t.Errorf("%d breaking: %d names shown, want %d: %q", breaking, named, want, issue.Message)
		}
		if breaking > 6 {
			if !strings.Contains(issue.Message, fmt.Sprintf("and %d more", breaking-6)) {
				t.Errorf("%d breaking: the tail is wrong: %q", breaking, issue.Message)
			}
			// Sorted, so which six are shown does not depend on deck order --
			// two people reading the same deck see the same six.
			if !strings.Contains(issue.Message, "Fixture 00, Fixture 01") {
				t.Errorf("%d breaking: the list is not sorted: %q", breaking, issue.Message)
			}
		} else if strings.Contains(issue.Message, "more") {
			t.Errorf("%d breaking: a tail appeared with nothing in it: %q", breaking, issue.Message)
		}
		// And it quotes the condition, so the reader can check it by hand.
		if !strings.Contains(issue.Message, "Condition:") {
			t.Errorf("%d breaking: no condition quoted: %q", breaking, issue.Message)
		}
	}
}

// A checker the code marks **inexact** reports as a warning and says so.
// The distinction is the whole reason `exact` is a field: a heuristic that
// failed a legal deck as an error would be the gate telling a newcomer their
// deck is broken when it is the gate that cannot be sure.
func TestAHeuristicCompanionCheckWarnsRatherThanRefuses(t *testing.T) {
	t.Parallel()
	const heuristic = "Zirda, the Dawnwaker" // inexact in `gate.checks`
	companion := heuristic
	cards := map[string]*pool.CardRecord{
		"Fixture Commander": rec("Fixture Commander", "Legendary Creature — Bear", "", nil, true),
		heuristic: rec(heuristic, "Legendary Creature — Fixture",
			"Companion — (placeholder; the checker does not read this)", nil, true),
	}
	entries := []deck.CardEntry{}
	for i := range 99 {
		name := fmt.Sprintf("Fixture %02d", i)
		entries = append(entries, aCard(name, "Filler."))
		// A permanent with no activated ability is what this checker counts.
		cards[name] = rec(name, "Creature — Fixture", "Flying", nil, true)
	}
	d := &deck.Deck{Slug: "d", Name: "D", Status: "theoretical", Stage: "curated",
		Commander: []string{"Fixture Commander"}, Companion: &companion, Cards: entries}

	report := gate.Validate(d, cards, 100)
	if !hasCode(report, "companion-restriction") {
		t.Fatalf("the heuristic check found nothing: %v", codes(report))
	}
	issue := find(report, "companion-restriction")
	if issue.Level != "warn" {
		t.Errorf("an inexact check is a %s, want warn", issue.Level)
	}
	if !strings.Contains(issue.Message, "heuristic") ||
		!strings.Contains(issue.Message, "verify by hand") {
		t.Errorf("the notice does not say it is a guess: %q", issue.Message)
	}
}
