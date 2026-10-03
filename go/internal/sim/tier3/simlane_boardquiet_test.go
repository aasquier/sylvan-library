package tier3

import (
	"encoding/json"
	"strings"
	"testing"
)

// The rest of the board's quiet answers, in `boardguards_test.go`'s shape.
//
// That file sweeps the lines whose *identifier* is missing — a seat of zero, a
// card with no name. These are the other two halves of the same rule, and they
// had never run: the lines that name something real and carry **no news**, and
// the second line about a thing that already moved. Both have to be silent, and
// for the same reason in both cases: a beat is drawn as a change, so a change
// that says "this is still what it was" makes the picture flinch for nothing
// and a seat listed twice in one beat is a seat drawn twice.
//
// Driven directly, for `boardguards_test.go`'s reason: these are the floor
// under a stream this package does not control, and reaching them through the
// parser would be testing the parser's spelling.

func TestABoardLineCarryingNoNewsChangesNothing(t *testing.T) {
	t.Parallel()
	b := newBoard()
	b.sit(1, "alpha", 40)
	b.name(11, "Gilded Goose", "Creature", false, 1)
	b.beat()

	// A counter line with no kind in it is not a counter, and the same total
	// again is not a move: a set of three tells nobody how it got to three, so
	// the move is what crosses — and there is no move here.
	b.counter(11, "", 0, 3)
	b.counter(11, "+1/+1", 0, 2)
	b.counter(11, "+1/+1", 2, 2)
	moves := 0
	for _, c := range b.changing {
		moves += len(c.CounterMoves)
	}
	if moves != 1 {
		t.Errorf("three counter lines, one of them news, produced %d moves", moves)
	}

	// A fate with no reason is a card that left for a reason Forge did not
	// name, which says nothing rather than saying it left for nothing.
	b.fate(0, "Destroyed")
	b.fate(11, "")
	if c := b.changing[11]; c != nil && c.Fate != "" {
		t.Errorf("an unnamed fate recorded %q", c.Fate)
	}

	// A mana pool belonging to no seat is not a pool, and an ability used by no
	// card is not an ability.
	b.floating(0, "{G}{G}")
	b.floating(-1, "{G}")
	if len(b.poolMoved) != 0 {
		t.Errorf("a pool with no seat moved %v", b.poolMoved)
	}
	b.usedAbility(0, 1, ZoneBattlefield, false, "")
	if len(b.used) != 0 {
		t.Errorf("an ability with no card on it recorded %v", b.used)
	}

	// A copy is true from the instant the card exists and never changes after,
	// so a card the dictionary has not been told about yet records nothing —
	// the next line, which names it, settles that — and the same copy twice is
	// not a second fact.
	b.copiedBy(999, 11)
	b.copiedBy(11, 1)
	b.copiedBy(11, 1)
	for _, card := range b.cards {
		if card.ID == 999 {
			t.Error("a copy line filed a card the dictionary never saw")
		}
	}
}

// A seat whose life or counters move twice between two beats is listed once:
// the beat draws the seats that changed, and a seat drawn twice is a scoreboard
// flinching at its own news.
func TestASeatThatMovesTwiceBetweenBeatsIsListedOnce(t *testing.T) {
	t.Parallel()
	b := newBoard()
	b.sit(1, "alpha", 40)
	b.sit(2, "beta", 40)

	b.lives(0, 39) // no seat: not a life total
	b.lives(1, 40) // the total it already had: not news
	b.lives(1, 37) // news
	b.lives(1, 35) // news about a seat already listed
	if len(b.lifeMoved) != 1 {
		t.Errorf("one seat losing life twice listed %v", b.lifeMoved)
	}

	b.playerCounter(0, "poison", 3) // no seat
	b.playerCounter(1, "", 0)       // every counter cleared, and none held
	if len(b.heldMoved) != 0 {
		t.Errorf("a player with no counters to clear recorded %v", b.heldMoved)
	}
	b.playerCounter(1, "poison", 1)
	b.playerCounter(1, "poison", 1) // the same total again
	b.playerCounter(1, "energy", 2)
	if len(b.heldMoved) != 1 {
		t.Errorf("one seat's counters moving twice listed %v", b.heldMoved)
	}

	// And clearing them all is news when there was something to clear.
	b.beat()
	b.playerCounter(1, "", 0)
	if len(b.heldMoved) != 1 {
		t.Errorf("a player whose counters were all cleared recorded %v", b.heldMoved)
	}
	if len(b.held[1]) != 0 {
		t.Errorf("the cleared counters are still held: %v", b.held[1])
	}
}

// A creature whose power holds and whose toughness moves still sends a change:
// the two numbers are folded separately, and a `-0/-1` that only reached the
// second one would otherwise be silent.
func TestAToughnessThatMovesAloneStillCrosses(t *testing.T) {
	t.Parallel()
	b := newBoard()
	b.name(11, "Gilded Goose", "Creature", false, 1)
	b.stats(11, 2, 2, "Creature")
	b.beat()
	b.stats(11, 2, 1, "Creature")

	c := b.changing[11]
	if c == nil {
		t.Fatal("a toughness that moved on its own sent no change")
	}
	if c.Power != nil {
		t.Errorf("a power that held still crossed as %d", *c.Power)
	}
	if c.Toughness == nil || *c.Toughness != 1 {
		t.Errorf("the toughness crossed as %v", c.Toughness)
	}
}

// **A board with cards and nobody at the table still crosses as a board**, and
// every list on it crosses as an empty list rather than as nothing at all. A
// `null` where a browser expects an array is the one shape that turns a quiet
// game into a room that cannot draw itself.
func TestAReelWithNoSeatsAndNoBeatsStillCarriesEveryList(t *testing.T) {
	t.Parallel()
	b := newBoard()
	b.name(11, "Gilded Goose", "Creature", false, 1)

	reel := b.reel()
	if reel == nil {
		t.Fatal("a board holding a card came back as no board at all")
	}
	if reel.Seats == nil || reel.Steps == nil || reel.Cards == nil {
		t.Fatalf("a list came back as nothing: seats=%v steps=%v cards=%v",
			reel.Seats, reel.Steps, reel.Cards)
	}
	raw, err := json.Marshal(reel)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"seats":[]`, `"steps":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the reel crossed as %s, without %s", raw, want)
		}
	}

	// And a board nothing ever happened on is no board, which is how a match
	// with no scribe behind it stays off the wire entirely.
	if empty := newBoard().reel(); empty != nil {
		t.Errorf("an untouched board came back as %+v", empty)
	}
}
