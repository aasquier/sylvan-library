package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The board, shaped for the room.
//
// Two things happen at this layer and nothing else does: a seat becomes a
// deck, and the paintings are looked up. Everything about the *game* was
// decided in `tier3/board.go` against a recorded match, and everything about
// drawing it happens in a browser.
//
// The shaping carries one property worth a test of its own. **A step and a
// beat are the same moment seen twice**, so the ceiling on beats has to cut
// both or the picture drifts away from the account by however many beats were
// dropped — silently, and only on the long games nobody watches to the end.

// aReel is a board with `steps` steps and two named seats.
func aReel(steps int) *tier3.BoardReel {
	reel := &tier3.BoardReel{
		Seats: []tier3.BoardSeat{
			{Seat: 1, Name: "Gyome, Master Chef — Food", Life: 40},
			{Seat: 2, Name: "Trostani — Tokens", Life: 40},
		},
		Cards: []tier3.BoardCard{
			{ID: 1, Name: "Gyome, Master Chef", Seat: 1,
				Types: "Legendary Creature - Troll Warlock"},
			{ID: 2, Name: "Food Token", Token: true, Seat: 1,
				Types: "Artifact - Food"},
		},
	}
	for i := 0; i < steps; i++ {
		reel.Steps = append(reel.Steps, tier3.BoardStep{Turn: i + 1, Seat: 1})
	}
	return reel
}

func TestTheBoardsSeatsBecomeDecksAndNothingElseDoes(t *testing.T) {
	t.Parallel()
	board := newForgeBoard(aReel(3),
		map[int]string{1: "gyome", 2: "trostani"}, nil, 3)
	if board == nil {
		t.Fatal("a board with seats and steps came back nil")
	}
	if len(board.Seats) != 2 {
		t.Fatalf("%d seats crossed, want 2", len(board.Seats))
	}
	for i, want := range []string{"gyome", "trostani"} {
		if board.Seats[i].Slug == nil || *board.Seats[i].Slug != want {
			t.Errorf("seat %d resolved to %v, want %q",
				i+1, board.Seats[i].Slug, want)
		}
	}
	// Forge's own name rides along as the fallback for a seat the shelf
	// cannot answer, which is what a browser shows before the decks load.
	if !strings.HasPrefix(board.Seats[0].Name, "Gyome") {
		t.Errorf("the seat lost Forge's own name: %q", board.Seats[0].Name)
	}
}

func TestASeatTheShelfCannotNameStillGetsARail(t *testing.T) {
	t.Parallel()
	// An empty seat map is what a pre-theater worker or a mid-deploy skew
	// produces. A board with no rails would read as a bug; a board with rails
	// and Forge's own titles on them reads as a match.
	board := newForgeBoard(aReel(2), map[int]string{}, nil, 2)
	if board == nil || len(board.Seats) != 2 {
		t.Fatalf("an unnamed board came back as %+v", board)
	}
	for _, seat := range board.Seats {
		if seat.Slug != nil {
			t.Errorf("seat %d invented a slug: %v", seat.Seat, *seat.Slug)
		}
		if seat.Name == "" {
			t.Errorf("seat %d has neither slug nor name", seat.Seat)
		}
	}
}

func TestTheStepsAreCutWhereTheBeatsAre(t *testing.T) {
	t.Parallel()
	// The property this file exists for. A game that outran the beat ceiling
	// must lose exactly as many steps, because the room advances the board by
	// counting the beats it has told.
	board := newForgeBoard(aReel(900), map[int]string{1: "gyome"}, nil,
		ForgeBeatsMax)
	if len(board.Steps) != ForgeBeatsMax {
		t.Errorf("%d steps crossed against a ceiling of %d beats; the picture "+
			"and the account would drift apart by the difference",
			len(board.Steps), ForgeBeatsMax)
	}

	// And a short game keeps every step it had.
	whole := newForgeBoard(aReel(9), map[int]string{1: "gyome"}, nil, 9)
	if len(whole.Steps) != 9 {
		t.Errorf("a nine-step game came out as %d", len(whole.Steps))
	}
}

func TestAMatchWithNoBoardShapesToNothing(t *testing.T) {
	t.Parallel()
	// A worker without the scribe plays the match and reports no board. The
	// room draws the account alone — ADR 42's fourth decision, at this layer.
	if board := newForgeBoard(nil, map[int]string{1: "gyome"}, nil, 5); board != nil {
		t.Errorf("a boardless match shaped to %+v", board)
	}
	shaped := newForgeBeats(tier3.EventLog{Game: 1, Events: theBeats()},
		map[int]string{1: "gyome"}, nil)
	if shaped.Board != nil {
		t.Error("beats with no board came back carrying one")
	}
	// And it says so on the wire, so a browser can tell "no board" from
	// "board not sent yet".
	raw, err := json.Marshal(shaped)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"board":null`) {
		t.Errorf("the absent board is not stated on the wire:\n%s", raw)
	}
}

// The paintings, through the real pool.
//
// **Two lookups, because a token is not a card.** A real card comes out of
// `oracle_cards`; a token is not in that table at all and comes out of
// `printings` with its *earliest* printing. Getting that wrong is how Teenage
// Mutant Ninja Turtles art arrived on the Grand Coliseum.
func TestTheBoardsPaintingsComeFromTwoTables(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	known := map[string]boardArt{}
	a.resolveBoardArt(context.Background(), []tier3.BoardCard{
		{ID: 1, Name: "Gyome, Master Chef"},
		{ID: 2, Name: "Food Token", Token: true},
		{ID: 3, Name: "Not A Real Card At All"},
	}, known)

	if art := known["Gyome, Master Chef"]; art.Image == "" {
		t.Error("a real card came back with no painting")
	}
	food := known["Food Token"]
	if food.Image == "" {
		t.Fatal("the Food token came back with no painting; tokens are in " +
			"`printings` and always have been")
	}
	if food.Artist != "Randy Gallegos" {
		t.Errorf("the Food token was painted by %q, want Randy Gallegos — the "+
			"newest Food printing is a Secret Lair, and answering with it is "+
			"the Ninja Turtles mistake", food.Artist)
	}
	// A name nothing has printed is *marked* rather than left absent, so a
	// twenty-game match asks about it once instead of once per game.
	if _, asked := known["Not A Real Card At All"]; !asked {
		t.Error("an unresolvable name was not remembered, so every game would " +
			"ask the pool about it again")
	}
}

func TestPaintingsAreLookedUpOncePerMatch(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	known := map[string]boardArt{}
	cards := []tier3.BoardCard{{ID: 1, Name: "Gyome, Master Chef"}}
	a.resolveBoardArt(context.Background(), cards, known)
	first := known["Gyome, Master Chef"]

	// The second game of the same pairing names the same hundred cards. If
	// the cache did not hold, that would be a pool round trip per game for
	// nothing.
	a.resolveBoardArt(context.Background(), cards, known)
	if known["Gyome, Master Chef"] != first {
		t.Error("a second game re-resolved a card the match had already asked about")
	}
}

func TestAMatchWithNoPoolStillDrawsItsBoard(t *testing.T) {
	t.Parallel()
	// A picture is decoration, and a missing one must never cost somebody a
	// match they are already watching. No pool is not an error here.
	a := New(Config{})
	known := map[string]boardArt{}
	a.resolveBoardArt(context.Background(),
		[]tier3.BoardCard{{ID: 1, Name: "Gyome, Master Chef"}}, known)

	board := newForgeBoard(aReel(2), map[int]string{1: "gyome"}, known, 2)
	if board == nil || len(board.Cards) != 2 {
		t.Fatalf("a board without a pool came back as %+v", board)
	}
	for _, card := range board.Cards {
		if card.Name == "" {
			t.Error("a card lost its name along with its painting")
		}
	}
}
