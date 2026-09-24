package tier3

import "testing"

// What the board does with a line that says nothing.
//
// **Every field here arrives from somebody else's JSON**, decoded into a
// struct with zero values for anything absent — so a seat of 0, a card with no
// name and a board id of 0 are not hypothetical inputs, they are what one
// missing key looks like by the time it reaches these methods. The rule is the
// same everywhere and it is the one `events.go` follows: a line that cannot be
// read changes nothing, rather than seating a player called nobody or filing a
// card under id zero where the next real card will land on top of it.
//
// Driven directly rather than through the parser because that is the point:
// the guards are the floor under a stream this package does not control, and a
// test that could only reach them by writing a malformed line would be testing
// the parser's spelling instead.

func TestALineWithNothingInItChangesNothingOnTheBoard(t *testing.T) {
	t.Parallel()
	b := newBoard()

	// A seat that is not a seat.
	b.sit(0, "nobody", 40)
	b.sit(-1, "nobody", 40)
	if len(b.seats) != 0 {
		t.Errorf("a roster line with no seat number seated %d players", len(b.seats))
	}

	// A card with no id, and a card with no name: neither enters the
	// dictionary, because a dictionary entry is what every later line looks a
	// card up by.
	b.name(0, "Gilded Goose", "Creature", false, 1)
	b.name(77, "", "Creature", false, 1)
	if len(b.known) != 0 {
		t.Errorf("a nameless card entered the dictionary: %v", b.known)
	}

	// A combat line with no attacker in it raises no combat.
	b.inCombat(0, "attacking", 2, 0)
	if len(b.fighting) != 0 {
		t.Errorf("a combat line with no creature in it started a fight: %v", b.fighting)
	}

	// And a commander-damage line missing any of its three numbers is not a
	// tally: zero damage from nobody's commander to nobody is not a fact.
	b.commanderDamage(0, 100, 7)
	b.commanderDamage(1, 0, 7)
	b.commanderDamage(1, 100, 0)
	if len(b.generals) != 0 {
		t.Errorf("an empty commander-damage line recorded %v", b.generals)
	}
}

// A seat named twice is the same seat, updated -- the scribe re-announces a
// roster as life totals move, and a second line about seat one must not put a
// second player at the table.
func TestASeatNamedTwiceIsTheSameSeat(t *testing.T) {
	t.Parallel()
	b := newBoard()
	b.sit(1, "Gyome, Master Chef", 40)
	b.sit(2, "Arahbo, Roar of the World", 40)
	b.sit(1, "Gyome, Master Chef", 33)

	if len(b.seats) != 2 {
		t.Fatalf("the table holds %d seats, want 2", len(b.seats))
	}
	if b.seats[0].Life != 33 {
		t.Errorf("seat one's life is %d, want the later line's 33", b.seats[0].Life)
	}
	if b.seats[1].Seat != 2 {
		t.Errorf("the second seat is numbered %d", b.seats[1].Seat)
	}
}

// Commander damage only ever climbs. The scribe reports a running total, so a
// line carrying a smaller number than one already recorded is a line that
// arrived out of order -- and lowering the tally on it would un-kill a player
// the board has already watched die.
func TestCommanderDamageNeverGoesBackwards(t *testing.T) {
	t.Parallel()
	b := newBoard()
	b.sit(1, "Gyome, Master Chef", 40)
	b.sit(2, "Arahbo, Roar of the World", 40)

	b.commanderDamage(2, 100, 14)
	b.commanderDamage(2, 100, 7)
	if got := b.generals[2][100]; got != 14 {
		t.Errorf("the tally fell to %d, want the 14 already recorded", got)
	}
	b.commanderDamage(2, 100, 21)
	if got := b.generals[2][100]; got != 21 {
		t.Errorf("a larger total was not recorded: %d", got)
	}
	// Two commanders can be hitting one seat, and each keeps its own tally --
	// twenty-one is per commander, which is the whole reason this is a map
	// rather than a number.
	b.commanderDamage(2, 200, 9)
	if len(b.generals[2]) != 2 {
		t.Errorf("seat two is being hit by %d commanders, want 2", len(b.generals[2]))
	}
}
