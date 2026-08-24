package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The narrator is tested by reading what it wrote, which is the only way a
// renderer can be tested honestly: every field it silently drops still leaves
// a passing test if the assertion is about the value rather than the page.

func narratedLife(n int) *int { return &n }

// oneNarratedGame is the turn-six sequence from the recorded Arahbo-vs-Goreclaw
// game, which is a whole combat: an attacker declared, nobody blocking, damage,
// and the life total moving by exactly what was dealt.
func oneNarratedGame() tier3.EventLog {
	return tier3.EventLog{Game: 1, Events: []tier3.GameEvent{
		{Kind: tier3.EventMulligan, Seat: 1, Amount: 7},
		{Kind: tier3.EventTurn, Turn: 6, Seat: 2},
		{Kind: tier3.EventLand, Turn: 6, Seat: 2, Card: "Castle Garenbrig"},
		{Kind: tier3.EventCast, Turn: 6, Seat: 2, Card: "Fauna Shaman"},
		{Kind: tier3.EventResolve, Turn: 6, Card: "Fauna Shaman"},
		{Kind: tier3.EventAttack, Turn: 6, Seat: 2, Card: "Fauna Shaman", TargetSeat: 1},
		{Kind: tier3.EventUnblocked, Turn: 6, Seat: 1, Card: "Fauna Shaman"},
		{Kind: tier3.EventDamage, Turn: 6, Card: "Fauna Shaman", Amount: 2, TargetSeat: 1},
		{Kind: tier3.EventLife, Turn: 6, Seat: 1, Amount: -2, Life: narratedLife(38)},
		{Kind: tier3.EventDies, Turn: 6, Card: "Sakura-Tribe Elder"},
		{Kind: tier3.EventBlock, Turn: 6, Seat: 1, Card: "Pride Sovereign",
			Target: "Goreclaw, Terror of Qal Sisma"},
		{Kind: tier3.EventOutcome, Turn: 6, Seat: 2, Amount: 0,
			Note: "life total reached 0"},
	}}
}

func TestTheNarratorTellsEveryBeatItIsGiven(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	narrateGame(&buf, oneNarratedGame(),
		map[int]string{1: "arahbo-cats", 2: "goreclaw-stompy"})
	got := buf.String()

	// Each of these is a fact from a *different field*. A renderer that
	// dropped `Amount`, or read `Target` where it meant `TargetSeat`, still
	// prints something plausible — so the assertions name the numbers and the
	// direction, not the shape of the line.
	for _, want := range []string{
		"game 1",
		"arahbo-cats keeps 7",
		"turn 6",
		"goreclaw-stompy plays Castle Garenbrig",
		"goreclaw-stompy casts Fauna Shaman",
		"Fauna Shaman resolves",
		"goreclaw-stompy attacks arahbo-cats with Fauna Shaman",
		"Fauna Shaman is unblocked",
		"Fauna Shaman deals 2 to arahbo-cats",
		"arahbo-cats at 38",
		"Sakura-Tribe Elder dies",
		"Pride Sovereign blocks Goreclaw, Terror of Qal Sisma",
		"goreclaw-stompy loses -- life total reached 0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the account never says %q. It said:\n%s", want, got)
		}
	}
}

func TestTheNarratorNamesAWinnerAsAWinner(t *testing.T) {
	t.Parallel()

	log := tier3.EventLog{Game: 3, Events: []tier3.GameEvent{
		{Kind: tier3.EventOutcome, Seat: 1, Amount: 1, Note: "all opponents have lost"},
	}}
	var buf bytes.Buffer
	narrateGame(&buf, log, map[int]string{1: "arahbo-cats"})
	if got := buf.String(); !strings.Contains(got, "arahbo-cats WINS") {
		t.Errorf("a win read as %q", got)
	}
}

func TestTheNarratorNamesASeatItWasNotGiven(t *testing.T) {
	t.Parallel()

	// A seat with no deck behind it renders as the seat rather than as an
	// empty string: a line reading "  attacks  with Fauna Shaman" is worse
	// than one naming a number.
	var buf bytes.Buffer
	narrateGame(&buf, oneNarratedGame(), map[int]string{})
	got := buf.String()
	if !strings.Contains(got, "seat 2 casts Fauna Shaman") {
		t.Errorf("an unnamed seat rendered as %q", got)
	}
	if strings.Contains(got, "  casts") {
		t.Error("an unnamed seat rendered as nothing at all")
	}
}

func TestTheNarratorSaysWhenItWasCutShort(t *testing.T) {
	t.Parallel()

	// The truncation flag exists so a short list never reads as a complete
	// one; a renderer that ignored it would undo that at the last step.
	log := oneNarratedGame()
	log.Truncated = true
	var buf bytes.Buffer
	narrateGame(&buf, log, map[int]string{1: "a", 2: "b"})
	if !strings.Contains(buf.String(), "outran") {
		t.Error("a truncated game was told as if it were whole")
	}
}

func TestSeatsFollowTheOrderTheDecksWerePassed(t *testing.T) {
	t.Parallel()

	// Forge's own rule: a seat is the position in the `-d` argument list, and
	// it is one-based because the label is `Ai(1)` for the first deck.
	decks := []*deck.Deck{{Slug: "arahbo-cats"}, {Slug: "goreclaw-stompy"}}
	seats := seatsOf(decks)
	if len(seats) != 2 {
		t.Fatalf("seated %d decks, want 2", len(seats))
	}
	if seats[1] != "arahbo-cats" || seats[2] != "goreclaw-stompy" {
		t.Errorf("seats %v do not follow the order passed", seats)
	}
	if _, ok := seats[0]; ok {
		t.Error("there is a seat zero")
	}
}
