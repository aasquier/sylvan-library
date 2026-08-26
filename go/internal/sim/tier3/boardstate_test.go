package tier3_test

import (
	"fmt"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The board's state beyond where the cards are: what is on them, what they are
// doing, and what they cost.
//
// `boardedges_test.go` holds the rulings about zones; this file holds the four
// facts a picture needs that the zone alone cannot give it — counters that go
// away when a card does, the account of how they arrived, the fight, and the
// commander tax. Driven the same way: synthetic lines through the public
// reader, never a reach into the assembler.

// zoneLine is one Forge zone event.
func zoneLine(id int, name, types, zone, mode string, seat int) string {
	return fmt.Sprintf(`{"t":"zone","game":1,"zone":%q,"mode":%q,"seat":%d,`+
		`"id":%d,"card":%q,"types":%q,"power":2,"toughness":2}`,
		zone, mode, seat, id, name, types)
}

// counterLine is one Forge counter event, both totals, exactly as the scribe
// writes them.
func counterLine(id int, kind string, was, now int) string {
	return fmt.Sprintf(`{"t":"counters","game":1,"counter":%q,"was":%d,`+
		`"now":%d,"id":%d,"card":"A card"}`, kind, was, now, id)
}

// turnLine begins a turn, which is also where the last combat ends.
func turnLine(turn, seat int) string {
	return fmt.Sprintf(`{"t":"turn","game":1,"turn":%d,"seat":%d,`+
		`"who":"Gyome — Food","life":40}`, turn, seat)
}

// standing is one card's state after every step, folded the way a browser
// folds it: the last value each field was given.
type standing struct {
	zone      string
	counters  []tier3.BoardCounter
	moves     []tier3.BoardCounterMove
	combat    string
	attacking int
	blocking  int
	casts     int
}

func folded(log tier3.EventLog, id int) standing {
	out := standing{}
	for _, step := range log.Board.Steps {
		for _, change := range step.Changes {
			if change.ID != id {
				continue
			}
			if change.Zone != "" {
				out.zone = change.Zone
			}
			if change.Counters != nil {
				out.counters = *change.Counters
			}
			out.moves = append(out.moves, change.CounterMoves...)
			if change.Combat != nil {
				out.combat = *change.Combat
			}
			if change.Attacking != nil {
				out.attacking = *change.Attacking
			}
			if change.Blocking != nil {
				out.blocking = *change.Blocking
			}
			if change.Casts != nil {
				out.casts = *change.Casts
			}
		}
	}
	return out
}

// Ruling 5: a card that changes zones arrives with nothing on it.
//
// Aaron, 2026-08-26: *"currently counters are following things into exile, the
// graveyard, and the command zone, they fall off a creature when they move to
// any of those zones"* — which is rule 400.7 and is what a player sees at a
// table. Forge announces no counter event for it, because from Forge's side
// nothing was removed: the object carrying them stopped existing.
//
// All three destinations, because all three were named and because they take
// different paths through `drawnZone` — and a token is a fourth, since a token
// leaving the battlefield is rewritten to [tier3.ZoneGone] on arrival and the
// shedding must survive that rewrite.
func TestCountersDoNotFollowACardOutOfPlay(t *testing.T) {
	t.Parallel()
	for _, path := range []struct {
		name  string
		forge string
		drawn string
	}{
		{"the graveyard", "Graveyard", tier3.ZoneGraveyard},
		{"exile", "Exile", tier3.ZoneExile},
		{"the command zone", "Command", tier3.ZoneCommand},
	} {
		t.Run(path.name, func(t *testing.T) {
			t.Parallel()
			const creature = "Creature - Squirrel Warlock"
			logs := played(t, openGame, seatOne, seatTwo,
				zoneLine(7, "Hazel's Brewmaster", creature,
					"Battlefield", "in", 1),
				counterLine(7, "+1/+1", 0, 2),
				turnLine(2, 2),
				zoneLine(7, "Hazel's Brewmaster", creature,
					"Battlefield", "out", 1),
				zoneLine(7, "Hazel's Brewmaster", creature,
					path.forge, "in", 1),
				turnLine(3, 1),
				endGame)
			got := folded(logs[0], 7)
			if got.zone != path.drawn {
				t.Fatalf("the card ended in %q, want %q", got.zone, path.drawn)
			}
			if len(got.counters) != 0 {
				t.Errorf("a creature that left the battlefield for %s still "+
					"carries %v; a card that changes zones is a new object "+
					"and arrives with nothing on it", path.name, got.counters)
			}
		})
	}
}

// The same rule for a token, whose arrival is rewritten before it lands.
//
// A token put into a graveyard ceases to exist, so the board sends it to
// [tier3.ZoneGone] rather than to the graveyard (ruling 5's neighbour, argued
// in `board.go`). The rewrite happens inside the same call that sheds, and a
// shed keyed to the *rewritten* zone rather than to the change itself would
// have quietly skipped every token in the game — which for a Trostani board is
// most of it.
func TestATokenShedsItsCountersOnTheWayOut(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,`+
			`"id":8,"card":"Saproling Token","token":true,`+
			`"types":"Creature - Saproling","power":1,"toughness":1}`,
		counterLine(8, "+1/+1", 0, 1),
		turnLine(2, 2),
		zoneLine(8, "Saproling Token", "Creature - Saproling",
			"Battlefield", "out", 1),
		zoneLine(8, "Saproling Token", "Creature - Saproling",
			"Graveyard", "in", 1),
		turnLine(3, 1),
		endGame)
	got := folded(logs[0], 8)
	if got.zone != tier3.ZoneGone {
		t.Fatalf("the token ended in %q, want %q", got.zone, tier3.ZoneGone)
	}
	if len(got.counters) != 0 {
		t.Errorf("a token that ceased to exist still carries %v", got.counters)
	}
}

// And the other half of the rule: a permanent that has **not** changed zones
// keeps everything.
//
// A creature changing controller is a seat change with no zone change — the
// same object, on the same battlefield, in somebody else's row — so a Threaten
// effect must not strip the counters off what it steals. This is the case
// [tier3.BoardChange.Counters]' shedding could most easily have got wrong,
// because a controller change goes down exactly the same path as a move.
func TestAStolenPermanentKeepsItsCounters(t *testing.T) {
	t.Parallel()
	const creature = "Creature - Squirrel Warlock"
	logs := played(t, openGame, seatOne, seatTwo,
		zoneLine(9, "Hazel's Brewmaster", creature, "Battlefield", "in", 1),
		counterLine(9, "+1/+1", 0, 3),
		turnLine(2, 2),
		// Seat two takes it. No zone line for the leaving, because it never
		// left one.
		zoneLine(9, "Hazel's Brewmaster", creature, "Battlefield", "in", 2),
		turnLine(3, 1),
		endGame)
	got := folded(logs[0], 9)
	if len(got.counters) != 1 || got.counters[0].N != 3 {
		t.Errorf("a stolen creature carries %v; it never changed zones and "+
			"should still be a 2/2 with three +1/+1 counters", got.counters)
	}
}

// A card losing its last counter says so.
//
// The set is sent whole whenever any of it moves, and an empty set is the
// answer for a card that has none. Under a plain slice that answer was
// unsendable — `omitempty` renders an empty slice and an absent field as the
// same bytes — so the browser kept drawing the counter that had just been
// removed. The pointer is what makes "none" a thing this wire can say.
func TestACardWithNoCountersLeftSaysSo(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		zoneLine(11, "Marchesa", "Legendary Creature - Human",
			"Battlefield", "in", 1),
		counterLine(11, "+1/+1", 0, 1),
		turnLine(2, 2),
		counterLine(11, "+1/+1", 1, 0),
		turnLine(3, 1),
		endGame)
	said := false
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 11 && change.Counters != nil &&
				len(*change.Counters) == 0 {
				said = true
			}
		}
	}
	if !said {
		t.Error("the last counter came off and no step said the card was " +
			"empty; a browser holding the old set has no way to find out")
	}
}

// The account of how a card got its counters — item 2, the honest half.
//
// Aaron, 2026-08-26: *"could we keep a history of why a creature has all of
// the counters it does and show it as help text on a hover"*. Forge sends both
// totals on every counter event and this reader dropped the first one, so the
// board could say a creature had three and never say when the three arrived.
//
// **What it cannot say is who put them there.** Forge's
// `GameEventCardCounters` carries no source (ADR 42 records the gap), so the
// history is *what happened*, never *who did it*.
func TestACounterMoveCarriesWhereItCameFrom(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		zoneLine(12, "Hazel's Brewmaster", "Creature - Squirrel Warlock",
			"Battlefield", "in", 1),
		counterLine(12, "+1/+1", 0, 2),
		turnLine(2, 2),
		counterLine(12, "+1/+1", 2, 3),
		counterLine(12, "shield", 0, 1),
		turnLine(3, 1),
		endGame)
	got := folded(logs[0], 12)
	want := []tier3.BoardCounterMove{
		{Kind: "+1/+1", Was: 0, Now: 2},
		{Kind: "+1/+1", Was: 2, Now: 3},
		{Kind: "shield", Was: 0, Now: 1},
	}
	if len(got.moves) != len(want) {
		t.Fatalf("the card's counters moved %d times, want %d: %v",
			len(got.moves), len(want), got.moves)
	}
	for i, move := range want {
		if got.moves[i] != move {
			t.Errorf("move %d is %v, want %v", i, got.moves[i], move)
		}
	}
}

// Combat, which the board could not draw at all.
//
// `attack` and `block` were beats and nothing else, so the account said who
// was swinging and the picture never did — a row of tokens looked identical
// whether it was attacking, blocking or asleep (Aaron, 2026-08-26, asking for
// attacking and blocking token piles to be told apart).
//
// The blocker's attacker is kept **by id**. Forge sends `target_id` on every
// block line and this reader used to read only `target`, a name, so two Egg
// Tokens blocking two identical attackers were an unanswerable question.
func TestTheBoardKnowsWhoIsFighting(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		zoneLine(20, "Gyome, Master Chef", "Legendary Creature - Troll Warlock",
			"Battlefield", "in", 1),
		zoneLine(21, "Egg Token", "Creature - Egg", "Battlefield", "in", 2),
		turnLine(2, 1),
		`{"t":"attack","game":1,"seat":1,"who":"Gyome — Food",`+
			`"against":"Atla — Eggs","against_seat":2,"id":20,`+
			`"card":"Gyome, Master Chef"}`,
		`{"t":"block","game":1,"seat":2,"who":"Atla — Eggs","target_id":20,`+
			`"target":"Gyome, Master Chef","id":21,"card":"Egg Token"}`,
		endGame)

	attacker := folded(logs[0], 20)
	if attacker.combat != tier3.CombatAttacking {
		t.Errorf("the attacker's combat is %q, want %q",
			attacker.combat, tier3.CombatAttacking)
	}
	if attacker.attacking != 2 {
		t.Errorf("the attacker is attacking seat %d, want seat 2 — `seat` is "+
			"the attacker's own side and `against_seat` is the one under "+
			"attack", attacker.attacking)
	}
	blocker := folded(logs[0], 21)
	if blocker.combat != tier3.CombatBlocking {
		t.Errorf("the blocker's combat is %q, want %q",
			blocker.combat, tier3.CombatBlocking)
	}
	if blocker.blocking != 20 {
		t.Errorf("the blocker is facing card %d, want the attacker's own id "+
			"20 — a name cannot tell two Egg Tokens apart", blocker.blocking)
	}
}

// Combat ends when the turn does, and a creature that dies leaves it at once.
//
// Forge's bus has no end-of-combat event the scribe listens for, so a turn
// beginning is the boundary — argued at `board.endCombat`. Without it a sword
// mark stays on a creature for the rest of the game, which is worse than the
// phase of lateness the turn boundary costs.
func TestCombatEndsWhenTheTurnDoes(t *testing.T) {
	t.Parallel()
	const troll = "Legendary Creature - Troll Warlock"
	logs := played(t, openGame, seatOne, seatTwo,
		zoneLine(30, "Gyome, Master Chef", troll, "Battlefield", "in", 1),
		zoneLine(31, "Egg Token", "Creature - Egg", "Battlefield", "in", 2),
		turnLine(2, 1),
		`{"t":"attack","game":1,"seat":1,"who":"Gyome — Food",`+
			`"against":"Atla — Eggs","against_seat":2,"id":30,`+
			`"card":"Gyome, Master Chef"}`,
		`{"t":"block","game":1,"seat":2,"who":"Atla — Eggs","target_id":30,`+
			`"target":"Gyome, Master Chef","id":31,"card":"Egg Token"}`,
		// The blocker dies in the fight; the attacker survives to the next
		// turn.
		zoneLine(31, "Egg Token", "Creature - Egg", "Battlefield", "out", 2),
		zoneLine(31, "Egg Token", "Creature - Egg", "Graveyard", "in", 2),
		turnLine(3, 2),
		endGame)

	if got := folded(logs[0], 30); got.combat != "" || got.attacking != 0 {
		t.Errorf("the attacker is still %q against seat %d after the turn "+
			"turned over", got.combat, got.attacking)
	}
	if got := folded(logs[0], 31); got.combat != "" || got.blocking != 0 {
		t.Errorf("a blocker that left the battlefield is still %q against "+
			"card %d; a creature that changes zones is out of combat at once",
			got.combat, got.blocking)
	}
}

// Commander tax, counted where the game is read rather than in the browser.
//
// Forge reports no tax and `CardView.isCommander()` is deliberately not on the
// wire, so the count of times a card has left the command zone is the only
// answer available. The browser used to count the same transitions itself,
// which put a reading of the game in the one file that is meant to decide none
// (ADR 14).
func TestLeavingTheCommandZoneIsCounted(t *testing.T) {
	t.Parallel()
	const troll = "Legendary Creature - Troll Warlock"
	logs := played(t, openGame, seatOne, seatTwo,
		zoneLine(40, "Gyome, Master Chef", troll, "Command", "in", 1),
		turnLine(2, 1),
		// Cast the first time.
		zoneLine(40, "Gyome, Master Chef", troll, "Command", "out", 1),
		zoneLine(40, "Gyome, Master Chef", troll, "Battlefield", "in", 1),
		turnLine(3, 2),
		// Killed, and put back in the zone.
		zoneLine(40, "Gyome, Master Chef", troll, "Battlefield", "out", 1),
		zoneLine(40, "Gyome, Master Chef", troll, "Command", "in", 1),
		turnLine(4, 1),
		// And cast again, which is where the tax becomes four.
		zoneLine(40, "Gyome, Master Chef", troll, "Command", "out", 1),
		zoneLine(40, "Gyome, Master Chef", troll, "Battlefield", "in", 1),
		turnLine(5, 2),
		endGame)
	if got := folded(logs[0], 40); got.casts != 2 {
		t.Errorf("the commander has left its zone %d times, want 2 — a "+
			"commander sitting at home and one going back to it are not "+
			"casts, and the two that are must not be counted twice",
			got.casts)
	}
	// A card that has never been in a command zone owes nothing, and must not
	// carry the field at all: a land with `casts: 0` on every change would be
	// most of a payload.
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 40 {
				continue
			}
			if change.Casts != nil {
				t.Errorf("card %d carries a cast count it never earned",
					change.ID)
			}
		}
	}
}

// The fight, against the recording rather than against a fixture.
//
// The tests above drive one line at a time, which proves the fold and proves
// nothing about the stream: a reading that expected a field Forge does not
// send would pass every one of them. This asks the frozen match — two real
// games of Gyome against Atla Palani — whether the combat it recorded reaches
// the board at all.
//
// **The blocker's attacker is checked against the dictionary**, because that
// is the assertion a name could never have made and the one most likely to be
// quietly wrong: an id that is really the blocker's own, or a target the
// reader took from the wrong field, still looks like a number.
func TestTheRecordedMatchDrawsItsCombat(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)
	attackers, blockers, stoodDown := 0, 0, 0
	for _, log := range logs {
		named := map[int]bool{}
		seats := map[int]bool{}
		for _, card := range log.Board.Cards {
			named[card.ID] = true
		}
		for _, seat := range log.Board.Seats {
			seats[seat.Seat] = true
		}
		for _, step := range log.Board.Steps {
			for _, change := range step.Changes {
				if change.Combat == nil {
					continue
				}
				switch *change.Combat {
				case tier3.CombatAttacking:
					attackers++
				case tier3.CombatBlocking:
					blockers++
				case "":
					stoodDown++
				}
				if change.Blocking != nil && *change.Blocking != 0 {
					if !named[*change.Blocking] {
						t.Errorf("game %d: a blocker faces card %d, which is "+
							"in no dictionary — the attacker's id is what "+
							"pairs them, and a wrong field looks like this",
							log.Game, *change.Blocking)
					}
					if *change.Blocking == change.ID {
						t.Errorf("game %d: card %d is blocking itself, which "+
							"is Forge's encoding for *unblocked* and not a "+
							"block at all", log.Game, change.ID)
					}
				}
				if change.Attacking != nil && *change.Attacking != 0 &&
					!seats[*change.Attacking] {
					t.Errorf("game %d: an attacker is attacking seat %d, "+
						"which is not at this table", log.Game,
						*change.Attacking)
				}
			}
		}
	}
	if attackers == 0 || blockers == 0 {
		t.Errorf("the recorded match drew %d attackers and %d blockers; both "+
			"are in the stream, so both should be on the board",
			attackers, blockers)
	}
	if stoodDown == 0 {
		t.Error("no creature was ever taken out of combat, so a sword mark " +
			"put on in the recording stays on for the rest of the game")
	}
}

// Ruling 3, widened: an effect Forge built from an ability is dropped wherever
// it turns up.
//
// Aaron, 2026-08-26: *"hovering on the command zone pops up some things I
// don't understand, like 'Olinda the Oblivious (99)'s Effect'"*. The old rule
// was an **and** — untyped *and* in the command zone — and either half failing
// leaves a blank card in somebody's picture. `forgeEffectName` needs neither:
// it matches the shape `Card.toString()` gives a host card, which no real
// Magic card name can wear, because no card name has ever held a parenthesis.
func TestAnAbilitysEffectIsDroppedWhateverItWears(t *testing.T) {
	t.Parallel()
	for _, phantom := range []struct {
		name  string
		line  string
		id    int
		drawn bool
	}{
		{"in the command zone, untyped — the rule as it stood",
			zoneLine(50, "Rogue's Passage (123)'s Effect", "",
				"Command", "in", 1), 50, false},
		{"in the command zone, carrying a type line",
			zoneLine(51, "Olinda the Oblivious (99)'s Effect",
				"Legendary Creature - Human Advisor", "Command", "in", 1),
			51, false},
		{"somewhere else entirely",
			zoneLine(52, "Olinda the Oblivious (99)'s Effect", "",
				"Exile", "in", 1), 52, false},
		// The two things that look like one and are not.
		{"an emblem, which a player can see and wants to",
			zoneLine(53, "Emblem - Elspeth, Knight-Errant", "",
				"Command", "in", 1), 53, true},
		{"a card whose name simply ends in the word",
			zoneLine(54, "Somebody's Effect", "Creature - Human",
				"Battlefield", "in", 1), 54, true},
	} {
		t.Run(phantom.name, func(t *testing.T) {
			t.Parallel()
			logs := played(t, openGame, seatOne, seatTwo,
				phantom.line, turnLine(1, 1), endGame)
			drawn := last(logs[0], phantom.id) != ""
			if drawn != phantom.drawn {
				t.Errorf("card %d drawn=%v, want %v", phantom.id, drawn,
					phantom.drawn)
			}
			named := false
			for _, card := range logs[0].Board.Cards {
				if card.ID == phantom.id {
					named = true
				}
			}
			if named != phantom.drawn {
				t.Errorf("card %d is in the dictionary=%v, want %v — a "+
					"phantom nothing draws is still a card the payload "+
					"carried", phantom.id, named, phantom.drawn)
			}
		})
	}
}

// inTheDictionary is whether the payload named a card at all.
func inTheDictionary(log tier3.EventLog, id int) bool {
	for _, card := range log.Board.Cards {
		if card.ID == id {
			return true
		}
	}
	return false
}

// touched is whether any step carried a change against a card id.
//
// Deliberately not "did it move" — an empty change is still a change, and a
// change against a card nothing named is exactly the thing that reaches a
// browser as a card with no name.
func touched(log tier3.EventLog, id int) bool {
	for _, step := range log.Board.Steps {
		for _, change := range step.Changes {
			if change.ID == id {
				return true
			}
		}
	}
	return false
}

// Ruling 3, widened again: a phantom refused once is refused on every line.
//
// The rule reads two things off a card — whether it is in a command zone and
// whether it has a type line — and **only a `zone` line carries a zone**. So
// asking it again on each line meant a `Commander Effect` refused on the line
// that created it walked back in through the next `stats`, `tapped` or
// `attach` line about it, was named into the dictionary, and folded a change
// against. Widening the rule to every card-shaped line had moved the
// name-shape half of it and could not move this half.
//
// Each line below is one Forge writes about a card and carries no zone on, so
// each is a line that cannot answer the question and must not be allowed to
// try.
func TestAPhantomRefusedOnceIsRefusedOnEveryLine(t *testing.T) {
	t.Parallel()
	// The arrival Forge announces when it builds the thing: untyped, in a
	// command zone, and named nothing like a host card — so the name shape has
	// nothing to catch and the zone is the only fact that refuses it.
	arrival := zoneLine(50, "Commander Effect", "", "Command", "in", 1)
	for _, later := range []struct {
		name string
		line string
	}{
		{"a stats line", `{"t":"stats","game":1,"id":50,` +
			`"card":"Commander Effect","power":0,"toughness":0,"types":""}`},
		{"a tapped line", `{"t":"tapped","game":1,"tapped":true,"id":50,` +
			`"card":"Commander Effect","power":0,"toughness":0,"types":""}`},
		{"a counters line", `{"t":"counters","game":1,"counter":"+1/+1",` +
			`"was":0,"now":1,"id":50,"card":"Commander Effect","power":0,` +
			`"toughness":0,"types":""}`},
		{"an attach line", `{"t":"attach","game":1,"seat":1,"id":50,` +
			`"card":"Commander Effect","target_id":10,` +
			`"target":"Grizzly Bears","power":0,"toughness":0,"types":""}`},
		{"a sacrificed line", `{"t":"sacrificed","game":1,"seat":1,"id":50,` +
			`"card":"Commander Effect","power":0,"toughness":0,"types":""}`},
	} {
		t.Run(later.name, func(t *testing.T) {
			t.Parallel()
			logs := played(t, openGame, seatOne, seatTwo, arrival, later.line,
				turnLine(1, 1), endGame)
			if len(logs) != 1 {
				t.Fatalf("%d games closed, want 1", len(logs))
			}
			if inTheDictionary(logs[0], 50) {
				t.Errorf("%s put Forge's `Commander Effect` back in the card "+
					"dictionary after its arrival was refused — the same "+
					"phantom, one line later", later.name)
			}
			if touched(logs[0], 50) {
				t.Errorf("%s folded a change against a card the dictionary "+
					"does not hold, which a browser draws as a card with no "+
					"name", later.name)
			}
		})
	}
}

// A change is never folded against a card the dictionary does not hold.
//
// The board and the card dictionary were deciding separately which cards
// exist, and two deciders is one disagreement away from a blank card: an
// `attach` line naming an id nothing had named folded an attachment with no
// name onto a real creature, which `FieldGeared` tucks under it like any other
// sword. The browser survives one now — `stackRow` counts attachments as well
// as naming them, so the equipped token stopped merging into the bare pile
// beside it — but it should never have been sent one.
//
// Both ways the two can disagree are driven here: a card the dictionary
// refused because the line carried no name — `board.name` takes nothing
// without one — and a card it never heard of at all. **Which Forge event
// opens the gap is not measured and does not need to be**: the board's job is
// to survive the disagreement, not to know which line caused it, and guessing
// at a Forge behaviour to justify a test would be the guess ADR 44 forbids.
func TestAnAttachmentTheDictionaryNeverNamedIsNotDrawn(t *testing.T) {
	t.Parallel()
	bear := zoneLine(10, "Grizzly Bears", "Creature - Bear", "Battlefield",
		"in", 1)
	for _, ghost := range []struct {
		name  string
		lines []string
		id    int
	}{
		{"a card whose name the dictionary refused", []string{
			zoneLine(77, "", "", "Battlefield", "in", 1),
			`{"t":"attach","game":1,"seat":1,"id":77,"card":"",` +
				`"target_id":10,"target":"Grizzly Bears"}`,
		}, 77},
		{"a card nothing ever named", []string{
			`{"t":"attach","game":1,"seat":1,"id":88,"card":"",` +
				`"target_id":10,"target":"Grizzly Bears"}`,
		}, 88},
	} {
		t.Run(ghost.name, func(t *testing.T) {
			t.Parallel()
			lines := append([]string{openGame, seatOne, seatTwo, bear},
				ghost.lines...)
			lines = append(lines, turnLine(1, 1), endGame)
			logs := played(t, lines...)
			if len(logs) != 1 {
				t.Fatalf("%d games closed, want 1", len(logs))
			}
			if touched(logs[0], ghost.id) {
				t.Errorf("card %d reached the board with no name in the "+
					"dictionary — a blank card tucked under a creature",
					ghost.id)
			}
			// The bear is untouched by any of this: refusing a phantom must
			// not cost the real card standing beside it.
			if !inTheDictionary(logs[0], 10) || last(logs[0], 10) == "" {
				t.Error("the creature the ghost reached for stopped being " +
					"drawn, which is a wider cut than the one being made")
			}
		})
	}
}
