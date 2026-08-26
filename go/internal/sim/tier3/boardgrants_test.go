package tier3_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The four things Forge's bus says that the board could not repeat: what is in
// a mana pool, how a permanent left, which keywords a card *instance* has, and
// that an ability was used.
//
// `boardstate_test.go` holds the rulings about what is on a card;
// `boardedges_test.go` holds the ones about where it is. This file holds the
// facts that arrived when the scribe was given four more events to listen to
// (ADR 45). Driven the same way as both: synthetic lines through the public
// reader, never a reach into the assembler.
//
// Every line shape here was read off a **real recorded match** — Kaheera
// against itself, played through Forge on 2026-08-26 — rather than invented, so
// a test passing here is a test about the stream Forge actually writes.

// manaLine is one seat's pool, as the scribe renders it: the symbols a person
// would write, and the empty string for a pool that has drained.
func manaLine(seat int, pool string) string {
	return fmt.Sprintf(`{"t":"mana","game":1,"seat":%d,"who":"Probe","pool":%q}`,
		seat, pool)
}

// keywordLine is a card being mentioned with the keywords it has right now.
// Any line naming a card carries them; a `stats` line is the ordinary one.
func keywordLine(id int, name, keywords string) string {
	return fmt.Sprintf(`{"t":"stats","game":1,"id":%d,"card":%q,`+
		`"types":"Creature - Beast","power":4,"toughness":5,"keywords":%q}`,
		id, name, keywords)
}

// pools is every value each seat's pool took across the whole game, in order.
func pools(log tier3.EventLog) []string {
	var out []string
	for _, step := range log.Board.Steps {
		for _, at := range step.Floating {
			out = append(out, fmt.Sprintf("%d:%s", at.Seat, at.Pool))
		}
	}
	return out
}

// liveOf is every live keyword set published for one card, in order, each
// rendered as a single string so a test can compare it as one value.
func liveOf(log tier3.EventLog, id int) []string {
	var out []string
	for _, step := range log.Board.Steps {
		for _, change := range step.Changes {
			if change.ID == id && change.Live != nil {
				out = append(out, strings.Join(*change.Live, "+"))
			}
		}
	}
	return out
}

// A pool is drawn as it fills, not only as it ends.
//
// Aaron, 2026-08-26: *"It would be nice if we could show the mana pool as
// things tap into it before it is drained to cast things."* The board pays for
// that with the one sequence it carries: a pool fills and empties several times
// between two beats, so a step holding only the pool's *final* value holds an
// empty pool nearly every time. Measured on a real match before this was a
// sequence — ten pool changes reached the browser and nine were empty.
func TestAManaPoolIsDrawnAsItFillsAndNotOnlyAsItDrains(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		// One beat's worth of a real turn: a Plains taps and is spent, a
		// Forest taps and is spent, a Sol Ring makes two and one is spent.
		manaLine(1, "W"), manaLine(1, ""),
		manaLine(1, "G"), manaLine(1, ""),
		manaLine(1, "CC"), manaLine(1, "C"),
		// And the other seat floating mana of its own in the same step, which
		// is what makes a pool per seat rather than per board: two players can
		// hold mana at once and neither one's is the other's.
		manaLine(2, "G"),
		turnLine(2, 2),
		endGame)
	got := pools(logs[0])
	want := []string{"1:W", "1:", "1:G", "1:", "1:CC", "1:C", "2:G"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("the pool crossed as %v, want %v -- every value it took, in "+
			"order, or the room only ever sees an empty pool", got, want)
	}
}

// A pool that has never held anything says nothing.
//
// A seat's first mana event is usually the drain at the end of its first step,
// and "this empty pool is still empty" is not news. The same rule
// [tier3.BoardChange.Counters] follows for a card with no counters.
func TestAnEmptyPoolIsNotAnnouncedUntilItHasHeldSomething(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		manaLine(1, ""), manaLine(1, ""),
		turnLine(2, 2),
		endGame)
	if got := pools(logs[0]); len(got) != 0 {
		t.Fatalf("an empty pool announced itself as %v; a seat that has never "+
			"floated mana has nothing to say", got)
	}
}

// Draining is news once the pool has held something.
func TestAPoolThatEmptiesSaysSo(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		manaLine(1, "GG"), manaLine(1, ""),
		turnLine(2, 2),
		endGame)
	got := pools(logs[0])
	if len(got) != 2 || got[1] != "1:" {
		t.Fatalf("the pool crossed as %v; the drain is as much news as the "+
			"fill, or mana stays on the table all game", got)
	}
}

// A sacrificed permanent says so, and it is not a death.
//
// Aaron, 2026-08-26, on the Treasure that taps and is then cracked: *"they must
// tap to sacrifice and they go into the ether."* A fetchland is the same shape.
// Rule 700.4 gives "dies" to creatures and planeswalkers, so `dies` was right
// to stay silent about a cracked artifact and the account simply had nothing
// else to say. The line order is Forge's own, measured: `sacrificed`, then
// `Battlefield out`, then `Graveyard in`.
func TestASacrificedPermanentSaysHowItLeft(t *testing.T) {
	t.Parallel()
	const land = "Land"
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(163, "Wooded Foothills", land, "Battlefield", "in", 1),
		`{"t":"sacrificed","game":1,"id":163,"card":"Wooded Foothills","types":"Land"}`,
		zoneLine(163, "Wooded Foothills", land, "Battlefield", "out", 1),
		zoneLine(163, "Wooded Foothills", land, "Graveyard", "in", 1),
		turnLine(2, 2),
		endGame)

	fate := ""
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 163 && change.Fate != "" {
				fate = change.Fate
			}
		}
	}
	if fate != tier3.FateSacrificed {
		t.Fatalf("the land left with fate %q, want %q -- a cracked fetchland "+
			"is a sacrifice and not a death", fate, tier3.FateSacrificed)
	}
	// And the account says it out loud, which it could not before.
	said := false
	for _, event := range logs[0].Events {
		if event.Kind == tier3.EventSacrificed && event.Card == "Wooded Foothills" {
			said = true
		}
		if event.Kind == tier3.EventDies {
			t.Errorf("a sacrificed land raised %q; rule 700.4 gives that word "+
				"to creatures and planeswalkers", tier3.EventDies)
		}
	}
	if !said {
		t.Error("nothing in the account said the land was sacrificed")
	}
}

// A creature's keywords are its own, not its printing's.
//
// Aaron, 2026-08-26: *"Some cards like Kaheera give vigilance or another effect
// to other cards, we currently are not representing that symbolically."* The
// board's keywords were Scryfall's, keyed by card *name*, so every copy of a
// card wore the same marks all game. These are the lines a real match produced
// with Kaheera on the battlefield beside a Beast.
func TestACardInstanceCarriesTheKeywordsItActuallyHas(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(120, "Leatherback Baloth", "Creature - Beast",
			"Battlefield", "in", 1),
		// Kaheera arrives and the Beast gains vigilance it does not print.
		keywordLine(120, "Leatherback Baloth", "Vigilance"),
		turnLine(2, 2),
		endGame)
	got := liveOf(logs[0], 120)
	if len(got) != 1 || got[0] != "Vigilance" {
		t.Fatalf("the Beast's live keywords crossed as %v, want [Vigilance] "+
			"-- a granted keyword is invisible without this", got)
	}
}

// A card that never had a keyword never mentions keywords.
//
// Most permanents in most games have none, and every line naming a card carries
// the set — so without this a match publishes one empty change per land, which
// a real match measured at thirty-odd carrying no information at all.
func TestACardWithNoKeywordsPublishesNone(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(31, "Forest", "Basic Land - Forest", "Battlefield", "in", 1),
		keywordLine(31, "Forest", ""),
		turnLine(2, 2),
		endGame)
	if got := liveOf(logs[0], 31); len(got) != 0 {
		t.Fatalf("a Forest published keyword sets %v; a card that has never "+
			"had one has nothing to say", got)
	}
}

// Losing the last granted keyword is published, because it is a change.
//
// The mirror of the test above, and the reason the field is a pointer: an empty
// set is a real answer here, exactly as it is for counters. A creature whose
// anthem left must stop wearing the mark.
func TestACreatureThatLosesItsLastKeywordSaysSo(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(120, "Leatherback Baloth", "Creature - Beast",
			"Battlefield", "in", 1),
		keywordLine(120, "Leatherback Baloth", "Vigilance"),
		// A beat between the two, or both land on one step and the browser is
		// handed only the later — which is [tier3.BoardStep]'s pacing contract
		// working as designed, not a keyword being lost.
		turnLine(2, 2),
		// Kaheera leaves; the grant goes with it.
		keywordLine(120, "Leatherback Baloth", ""),
		turnLine(3, 1),
		endGame)
	got := liveOf(logs[0], 120)
	if len(got) != 2 || got[0] != "Vigilance" || got[1] != "" {
		t.Fatalf("the Beast's keywords crossed as %v, want [Vigilance] then "+
			"the empty set -- otherwise it wears a vigilance nothing is "+
			"granting any more", got)
	}
}

// An ability going on the stack reaches the board, with where it came from.
//
// Aaron, 2026-08-26, on eminence: *"It can be used on the battlefield or from
// the command zone… It should just visually indicate that an ability is being
// used."* Abilities never reached this stream at all before — the scribe
// returned on anything that was not a spell — so a commander sitting in the
// command zone doing the one thing it is in the deck to do was invisible.
//
// **The zone is the eminence half**: a commander using an ability from the
// command zone never moves, so there is no other signal anywhere that it did
// anything.
func TestAnAbilityIsDrawnWhereverItsSourceIs(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(100, "Syr Gwyn, Hero of Ashvale",
			"Legendary Creature - Human Knight", "Command", "in", 1),
		`{"t":"ability","game":1,"seat":1,"who":"Probe","trigger":true,`+
			`"zone":"Command","id":100,"card":"Syr Gwyn, Hero of Ashvale",`+
			`"types":"Legendary Creature - Human Knight"}`,
		turnLine(2, 2),
		endGame)

	var used []tier3.BoardAbility
	for _, step := range logs[0].Board.Steps {
		used = append(used, step.Abilities...)
	}
	if len(used) != 1 {
		t.Fatalf("%d abilities reached the board, want 1", len(used))
	}
	if used[0].ID != 100 || used[0].Zone != "Command" || !used[0].Trigger {
		t.Fatalf("the ability crossed as %+v; a triggered ability used from "+
			"the command zone is what eminence is", used[0])
	}
}

// A token made as a copy names the card whose ability made it.
//
// Aaron, 2026-08-26, on populate: *"It really is making a clone, or splitting
// one thing into two."* Its presence is the copy — a token minted fresh carries
// nothing here.
//
// **It is not what the token was copied from**, and the wrong reading looks
// identical: a Centaur Token populated by Growing Ranks names *Growing Ranks*,
// which is the line a real match produced. The permanent that was copied lives
// on Forge's model and does not cross this pipe.
func TestAPopulatedTokenNamesWhatMadeIt(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(158, "Growing Ranks", "Enchantment", "Battlefield", "in", 1),
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,`+
			`"id":212,"card":"Centaur Token","token":true,"copied_by":158,`+
			`"copied_by_card":"Growing Ranks","types":"Creature - Centaur"}`,
		// And a token nobody copied carries nothing.
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,`+
			`"id":210,"card":"Centaur Token","token":true,`+
			`"types":"Creature - Centaur"}`,
		turnLine(2, 2),
		endGame)

	by := map[int]int{}
	for _, card := range logs[0].Board.Cards {
		by[card.ID] = card.CopiedBy
	}
	if by[212] != 158 {
		t.Errorf("the populated token was made by %d, want 158", by[212])
	}
	if by[210] != 0 {
		t.Errorf("a token minted fresh claims to be a copy of %d; the field's "+
			"presence is the whole signal that a copy happened", by[210])
	}
}

// Forge says when combat is over, and the turn boundary stands in when it does
// not.
//
// **Two boundaries, one rule, and a stated precedence.** ADR 44 ended combat at
// the next turn because that was the only boundary the stream had, and named
// `GameEventCombatUpdate` as the fix. That event is raised only by Forge's
// *human* input handlers and never fires in a headless match;
// `GameEventCombatEnded` is the engine's own. But a worker image built before
// the scribe listened for it still sends no `combat_end` at all, and a board
// that dropped the fallback would leave those matches attacking forever.
func TestCombatEndsWhenForgeSaysSoAndAtTheTurnWhenItDoesNot(t *testing.T) {
	t.Parallel()
	const beast = "Creature - Beast"
	attack := `{"t":"attack","game":1,"seat":1,"who":"Probe",` +
		`"against":"Other","against_seat":2,"id":120,` +
		`"card":"Leatherback Baloth","types":"Creature - Beast"}`

	t.Run("Forge's own signal ends it inside the turn", func(t *testing.T) {
		t.Parallel()
		logs := played(t, openGame, seatOne, seatTwo,
			turnLine(1, 1),
			zoneLine(120, "Leatherback Baloth", beast, "Battlefield", "in", 1),
			attack,
			`{"t":"combat_end","game":1}`,
			// A beat after combat ended, still inside turn 1 -- a creature
			// arriving, because that is what raises one. A land does not: it
			// stands in the land row, which is not [tier3.ZoneBattlefield].
			zoneLine(9, "Ravenous Baloth", beast, "Battlefield", "in", 1),
			endGame)
		if got := folded(logs[0], 120).combat; got != "" {
			t.Fatalf("the attacker is still %q after Forge ended combat, and "+
				"the turn has not changed", got)
		}
	})

	t.Run("the turn boundary stands in when nothing said", func(t *testing.T) {
		t.Parallel()
		logs := played(t, openGame, seatOne, seatTwo,
			turnLine(1, 1),
			zoneLine(120, "Leatherback Baloth", beast, "Battlefield", "in", 1),
			attack,
			turnLine(2, 2),
			endGame)
		if got := folded(logs[0], 120).combat; got != "" {
			t.Fatalf("the attacker is still %q a turn later, from a stream "+
				"that never says when combat ended", got)
		}
	})

	t.Run("a stream that speaks keeps the mark until it does", func(t *testing.T) {
		t.Parallel()
		// Once Forge has spoken for this game, the turn boundary no longer
		// ends combat on its own -- which is what makes it one rule rather
		// than two that can disagree. The attacker declared after the last
		// `combat_end` is still attacking when the next turn begins.
		logs := played(t, openGame, seatOne, seatTwo,
			turnLine(1, 1),
			zoneLine(120, "Leatherback Baloth", beast, "Battlefield", "in", 1),
			attack,
			`{"t":"combat_end","game":1}`,
			turnLine(2, 2),
			attack,
			turnLine(3, 1),
			endGame)
		if got := folded(logs[0], 120).combat; got != tier3.CombatAttacking {
			t.Fatalf("the attacker is %q, want %q -- once Forge is answering "+
				"this question the turn must stop answering it too",
				got, tier3.CombatAttacking)
		}
	})
}
