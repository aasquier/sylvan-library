package tier3_test

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// What a line does *not* say, and what the board is allowed to do about it.
//
// Two holes, both measured on the same real match — Arahbo/Cats against
// Gyome/Food, seed 11, played through Forge 2.0.14 on this laptop, 2026-08-27.
// **Every line in question is one of that match's own, copied verbatim**; only
// the scaffolding around it — the turns, the roster, an ordinary arrival — uses
// this package's helpers, because that half is not what is on trial. The two
// holes are not the same hole:
//
//   - **A sacrifice had no seat.** `GameEventCardSacrificed` is the only
//     card-shaped event on Forge's bus with no player component at all, so
//     every sacrifice reached the room unattributed and the plate could only
//     say "Sacrificed" where the rest of the account says who. `Scribe.java`
//     now reads the controller off the card; [tier3.ScribeParser] falls back to
//     the board for a worker that predates that.
//   - **A card view came back blank.** Forge freezes its own view layer for
//     the whole of `GameAction.checkStateEffects`, and `TrackableProperty.Name`
//     both respects that freeze and defaults to the empty string — so every
//     death by lethal damage, and every commander going home, announced itself
//     with a card carrying no name, no type line and 0/0. Thirteen of that
//     game's twenty-one graveyard arrivals. `blankView` in `scribe.go` holds
//     the bytecode and the measurement.
//
// The blank half is the one worth reading the tests for, because believing a
// blank line broke three different things at once and only the first of them
// was visible.

// The seat on a sacrifice is the player who paid the cost.
//
// One of the forty-two lines a real Food deck produced across two games, after
// the scribe learned to name the seat — and all forty-two now name the seat
// that was sacrificing, where before not one of them did. The room said
// "Sacrificed" with nobody's name on it, which on a two-player board is a fact
// nobody can attribute, and in a Food deck sacrificing is most of what happens.
//
// **This one is the gate on the Java, not on the Go**, and it passes against
// the reader as it stood: this side always read `l.Seat` and there was never a
// seat on the line to read. It pins the *shape* the scribe now writes, so a
// change to `Scribe.java` that dropped the seat again would fail here rather
// than reaching a browser. The test below is the Go half.
func TestASacrificeNamesTheSeatThatPaidIt(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 2),
		zoneLine(263, "Squirrel Token", "Creature - Squirrel",
			"Battlefield", "in", 2),
		`{"t":"sacrificed","game":1,"seat":2,"who":"Gyome, Master Chef — Food",`+
			`"id":263,"card":"Squirrel Token","token":true,"power":1,`+
			`"toughness":1,"types":"Creature - Squirrel"}`,
		turnLine(2, 1),
		endGame)

	found := false
	for _, event := range logs[0].Events {
		if event.Kind != tier3.EventSacrificed {
			continue
		}
		found = true
		if event.Seat != 2 {
			t.Errorf("the sacrifice belongs to seat %d, want 2 -- a plate that "+
				"cannot name a seat can only say %q", event.Seat, "Sacrificed")
		}
		if event.Card != "Squirrel Token" {
			t.Errorf("the sacrifice named %q, want %q", event.Card, "Squirrel Token")
		}
	}
	if !found {
		t.Fatal("nothing in the account said anything was sacrificed")
	}
}

// A sacrifice with no seat on it takes the board's answer.
//
// The shape every worker built before the scribe carried a controller still
// writes, verbatim from the same match before the fix. ADR 42's fourth decision
// is that an older worker degrades rather than failing, and this is that
// decision one field down: the board has been holding this card's seat since it
// arrived on the battlefield, so the beat can still name a player.
func TestASacrificeWithNoSeatTakesTheBoardsAnswer(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 2),
		zoneLine(211, "Food Token", "Artifact - Food", "Battlefield", "in", 2),
		`{"t":"sacrificed","game":1,"id":211,"card":"Food Token","token":true,`+
			`"power":0,"toughness":0,"types":"Artifact - Food"}`,
		turnLine(2, 1),
		endGame)

	found := false
	for _, event := range logs[0].Events {
		if event.Kind != tier3.EventSacrificed {
			continue
		}
		found = true
		if event.Seat != 2 {
			t.Errorf("a seatless sacrifice landed on seat %d, want 2 from the "+
				"board -- the Food was seat two's from the moment it arrived",
				event.Seat)
		}
	}
	if !found {
		t.Fatal("nothing in the account said the Food was sacrificed")
	}
}

// A creature that dies inside the state-based sweep is still named.
//
// Verbatim: a Cat Token arrives, grows, attacks, and is killed in combat. The
// `Battlefield out` names it in full and the `Graveyard in` one line later
// names nobody, because Forge's tracker is frozen for the whole sweep. The beat
// used to carry that emptiness straight through, and a beat with no card name
// draws *nothing at all* on the centre stage — so a creature died and the room
// was silent. Eleven of the twelve deaths in the match's first game, and
// thirteen of fifteen across both.
func TestADeathInsideTheStateSweepStillNamesTheCard(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":238,"card":"Cat Token",`+
			`"token":true,"power":1,"toughness":1,"types":"Creature - Cat"}`,
		`{"t":"stats","game":1,"id":238,"card":"Cat Token","token":true,`+
			`"power":3,"toughness":3,"types":"Creature - Cat","keywords":"Lifelink"}`,
		turnLine(2, 2),
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"out","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":238,"card":"Cat Token",`+
			`"token":true,"power":3,"toughness":3,"types":"Creature - Cat",`+
			`"keywords":"Lifelink"}`,
		`{"t":"zone","game":1,"zone":"Graveyard","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":238,"card":"",`+
			`"power":0,"toughness":0,"types":""}`,
		turnLine(3, 1),
		endGame)

	deaths := 0
	for _, event := range logs[0].Events {
		if event.Kind != tier3.EventDies {
			continue
		}
		deaths++
		if event.Card != "Cat Token" {
			t.Errorf("the death named %q, want %q -- a death with no name "+
				"draws nothing in the middle of the arena", event.Card, "Cat Token")
		}
		if event.Seat != 1 {
			t.Errorf("the death landed on seat %d, want 1", event.Seat)
		}
	}
	if deaths != 1 {
		t.Fatalf("the account holds %d deaths, want 1", deaths)
	}
}

// A card the board has never met stays nameless, rather than being invented.
//
// The other end of [tier3.ScribeParser]'s fallback, and the line it must not
// cross: the dictionary is a second source for a name this reader was told
// earlier in this same game, never a guess. A blank line about an id nothing
// has ever described is a line with nothing in it, and the honest answer is
// that nothing happened — the board cannot draw the card either.
func TestABlankLineAboutAnUnknownCardSaysNothing(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		`{"t":"zone","game":1,"zone":"Graveyard","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":9001,"card":"",`+
			`"power":0,"toughness":0,"types":""}`,
		turnLine(2, 2),
		endGame)

	for _, event := range logs[0].Events {
		if event.Kind == tier3.EventDies || event.Kind == tier3.EventEnters {
			t.Errorf("a line about an unknown card raised %q for %q",
				event.Kind, event.Card)
		}
	}
	for _, card := range logs[0].Board.Cards {
		if card.ID == 9001 {
			t.Errorf("the board drew a card called %q for an id nothing "+
				"ever named", card.Name)
		}
	}
}

// A blank line does not restamp the card it could not describe.
//
// The same sequence, read for the picture rather than for the sentence. Every
// number on a blank line is a trackable property sitting at its default, so
// folding one told the browser that a 3/3 Cat Token had become a 0/0 on the
// way to the graveyard — the last thing ever said about it, and the size it
// would be drawn at while the skull is on it.
func TestABlankLineDoesNotRestampTheCard(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":238,"card":"Cat Token",`+
			`"token":true,"power":3,"toughness":3,"types":"Creature - Cat"}`,
		turnLine(2, 2),
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"out","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":238,"card":"Cat Token",`+
			`"token":true,"power":3,"toughness":3,"types":"Creature - Cat"}`,
		`{"t":"zone","game":1,"zone":"Graveyard","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":238,"card":"",`+
			`"power":0,"toughness":0,"types":""}`,
		// And the `stats` line Forge sends inside the same frozen window, which
		// is the same defaults arriving by another road.
		`{"t":"stats","game":1,"id":238,"card":"","power":0,"toughness":0,"types":""}`,
		turnLine(3, 1),
		endGame)

	power, toughness := sized(logs[0], 238)
	if power != 3 || toughness != 3 {
		t.Errorf("the Cat Token ended as a %d/%d, want 3/3 -- a blank view is "+
			"Forge not having filled the card in yet, not a card that shrank",
			power, toughness)
	}
}

// A commander going home is not one of Forge's bookkeeping cards.
//
// The worst of the three, and the one nothing was watching for. A commander
// that dies goes back to the command zone, and that arrival is blank like every
// other move inside the sweep — which reads as `Command` with an empty type
// line, and that is exactly [tier3] `isForgeEffect`'s second rule. So the
// commander was convicted of being one of Forge's own invisible effects,
// remembered as one for the rest of the game, and every later line about it
// dropped: recast from the command zone, it never came back to the board.
//
// Verbatim, and it is the whole sequence rather than a sample, because the
// order is the argument: the graveyard arrival is blank, the graveyard
// *departure* is named again, and the command-zone arrival is blank once more.
func TestACommanderGoingHomeIsNotForgesBookkeeping(t *testing.T) {
	t.Parallel()
	const arahbo = "Arahbo, Roar of the World"
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(102, arahbo, "Legendary Creature - Cat Avatar", "Command", "in", 1),
		zoneLine(102, arahbo, "Legendary Creature - Cat Avatar", "Command", "out", 1),
		zoneLine(102, arahbo, "Legendary Creature - Cat Avatar",
			"Battlefield", "in", 1),
		turnLine(2, 2),
		// It dies.
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"out","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":102,`+
			`"card":"Arahbo, Roar of the World","power":7,"toughness":7,`+
			`"types":"Legendary Artifact Creature - Cat Avatar Food"}`,
		`{"t":"zone","game":1,"zone":"Graveyard","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":102,"card":"",`+
			`"power":0,"toughness":0,"types":""}`,
		`{"t":"zone","game":1,"zone":"Graveyard","mode":"out","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":102,`+
			`"card":"Arahbo, Roar of the World","power":5,"toughness":5,`+
			`"types":"Legendary Creature - Cat Avatar"}`,
		`{"t":"zone","game":1,"zone":"Command","mode":"in","seat":1,`+
			`"who":"Arahbo, Roar of the World — Cats","id":102,"card":"",`+
			`"power":0,"toughness":0,"types":""}`,
		turnLine(3, 1),
		// And is cast again, which is what never reached the board.
		zoneLine(102, arahbo, "Legendary Creature - Cat Avatar", "Command", "out", 1),
		zoneLine(102, arahbo, "Legendary Creature - Cat Avatar",
			"Battlefield", "in", 1),
		turnLine(4, 2),
		endGame)

	if got := last(logs[0], 102); got != tier3.ZoneBattlefield {
		t.Fatalf("the commander ended in %q, want %q -- a blank command-zone "+
			"arrival is not evidence that a card is Forge's own bookkeeping",
			got, tier3.ZoneBattlefield)
	}
	// It is drawn as itself and not as a nameless card, which is the other half
	// of what a refusal costs.
	named := false
	for _, card := range logs[0].Board.Cards {
		if card.ID == 102 {
			named = card.Name == arahbo
		}
	}
	if !named {
		t.Errorf("the board's dictionary does not hold %q for the commander", arahbo)
	}
}

// sized is a card's power and toughness after every step, folded the way a
// browser folds them: the last value each was given.
func sized(log tier3.EventLog, id int) (int, int) {
	power, toughness := 0, 0
	for _, step := range log.Board.Steps {
		for _, change := range step.Changes {
			if change.ID != id {
				continue
			}
			if change.Power != nil {
				power = *change.Power
			}
			if change.Toughness != nil {
				toughness = *change.Toughness
			}
		}
	}
	return power, toughness
}
