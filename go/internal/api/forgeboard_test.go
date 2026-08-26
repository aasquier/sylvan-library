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
		map[int]string{1: "gyome", 2: "trostani"}, nil, nil, 3)
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
	board := newForgeBoard(aReel(2), map[int]string{}, nil, nil, 2)
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
	board := newForgeBoard(aReel(900), map[int]string{1: "gyome"}, nil, nil,
		ForgeBeatsMax)
	if len(board.Steps) != ForgeBeatsMax {
		t.Errorf("%d steps crossed against a ceiling of %d beats; the picture "+
			"and the account would drift apart by the difference",
			len(board.Steps), ForgeBeatsMax)
	}

	// And a short game keeps every step it had.
	whole := newForgeBoard(aReel(9), map[int]string{1: "gyome"}, nil, nil, 9)
	if len(whole.Steps) != 9 {
		t.Errorf("a nine-step game came out as %d", len(whole.Steps))
	}
}

func TestAMatchWithNoBoardShapesToNothing(t *testing.T) {
	t.Parallel()
	// A worker without the scribe plays the match and reports no board. The
	// room draws the account alone — ADR 42's fourth decision, at this layer.
	if board := newForgeBoard(nil, map[int]string{1: "gyome"}, nil, nil, 5); board != nil {
		t.Errorf("a boardless match shaped to %+v", board)
	}
	shaped := newForgeBeats(tier3.EventLog{Game: 1, Events: theBeats()},
		map[int]string{1: "gyome"}, nil, nil)
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
	//
	// Compared field by field rather than with `!=`, because boardArt now
	// carries the card's keywords and a struct holding a slice is not
	// comparable. `reflect.DeepEqual` would say the same thing in one line and
	// would also say it about two *different* slices that happen to match —
	// which is the opposite of what this asks. The question is whether the
	// second call left the entry alone, so the slice is checked by identity:
	// same backing array means nothing re-resolved it.
	again := known["Gyome, Master Chef"]
	if again.Image != first.Image || again.Art != first.Art ||
		again.Artist != first.Artist || again.Mana != first.Mana ||
		len(again.Keywords) != len(first.Keywords) {
		t.Error("a second game re-resolved a card the match had already asked about")
	}
	if len(first.Keywords) > 0 && &again.Keywords[0] != &first.Keywords[0] {
		t.Error("the keywords were rebuilt, so the pool was asked again")
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

	board := newForgeBoard(aReel(2), map[int]string{1: "gyome"}, nil, known, 2)
	if board == nil || len(board.Cards) != 2 {
		t.Fatalf("a board without a pool came back as %+v", board)
	}
	for _, card := range board.Cards {
		if card.Name == "" {
			t.Error("a card lost its name along with its painting")
		}
	}
}

// The command zone, resolved from the deck.
//
// Everything that begins in a command zone looks alike from the stream — a
// commander, a partner and a companion all arrive as a card in `command` on
// step zero — so the split is made here against `deck.yaml`, and these are the
// three things that can go wrong with it.

// aPairing is a board where seat 1 ran a partner pair and a companion, and
// seat 2 ran a double-faced commander under the face Forge knows it by.
func aPairing() *tier3.BoardReel {
	return &tier3.BoardReel{
		Seats: []tier3.BoardSeat{
			{Seat: 1, Name: "Partners", Life: 40},
			{Seat: 2, Name: "Two-faced", Life: 40},
		},
		Cards: []tier3.BoardCard{
			{ID: 11, Name: "Kaheera, the Orphanguard", Seat: 1,
				Types: "Legendary Creature - Cat Beast"},
			{ID: 12, Name: "Thrasios, Triton Hero", Seat: 1,
				Types: "Legendary Creature - Merfolk Wizard"},
			{ID: 13, Name: "Tymna the Weaver", Seat: 1,
				Types: "Legendary Creature - Human Cleric"},
			// Forge's index has no `A // B`, so a modal card is on the board
			// under its front face.
			{ID: 21, Name: "Brutal Cathar", Seat: 2,
				Types: "Creature - Human Soldier"},
			// The same name in the other seat: a board id belongs to one
			// player, and a lookup that ignored the seat would hand seat 2's
			// throne to seat 1's card.
			{ID: 22, Name: "Thrasios, Triton Hero", Seat: 2,
				Types: "Legendary Creature - Merfolk Wizard"},
		},
		Steps: []tier3.BoardStep{{Turn: 1, Seat: 1}},
	}
}

func TestAPairingGetsTwoThronesInTheDecksOwnOrder(t *testing.T) {
	t.Parallel()
	board := newForgeBoard(aPairing(), map[int]string{1: "pair", 2: "faces"},
		map[int]forgeCommandZone{
			1: {Commanders: []string{"Thrasios, Triton Hero",
				"Tymna the Weaver"},
				Companion: "Kaheera, the Orphanguard"},
			2: {Commanders: []string{"Brutal Cathar // Moonrage Brute"}},
		}, nil, 1)
	if board == nil || len(board.Seats) != 2 {
		t.Fatalf("a pairing shaped to %+v", board)
	}
	// **Order is the deck's**, not the board's: Tymna is the lower id and
	// arrives second in `deck.yaml`, so a shaper reading the reel would put
	// her first and the same commander would change sides between games.
	if got := board.Seats[0].Commanders; len(got) != 2 ||
		got[0] != 12 || got[1] != 13 {
		t.Errorf("seat 1's thrones came out %v, want [12 13] — the order "+
			"`deck.yaml` lists them in", got)
	}
	// The companion is named as itself and is **not** one of the commanders.
	// It sits in the command zone like they do, and it has never had a tax.
	if got := board.Seats[0].Companion; got != 11 {
		t.Errorf("the companion resolved to %d, want 11", got)
	}
	for _, id := range board.Seats[0].Commanders {
		if id == 11 {
			t.Error("the companion was counted as a commander, which is what " +
				"charges it commander tax it does not owe")
		}
	}
	// A `A // B` name never appears in Forge's index, so the throne is found
	// through the same resolution the exporter used to write the `.dck`.
	if got := board.Seats[1].Commanders; len(got) != 1 || got[0] != 21 {
		t.Errorf("a double-faced commander resolved to %v, want [21]", got)
	}
	if board.Seats[1].Companion != 0 {
		t.Errorf("a deck with no companion reported %d",
			board.Seats[1].Companion)
	}
}

func TestAThroneIsFoundInItsOwnSeat(t *testing.T) {
	t.Parallel()
	// Two decks can run the same commander. Seat 2's Thrasios is id 22, and a
	// lookup that indexed the whole board by name would have handed it 12 —
	// which draws one player's commander on the other player's rail.
	board := newForgeBoard(aPairing(), nil, map[int]forgeCommandZone{
		2: {Commanders: []string{"Thrasios, Triton Hero"}},
	}, nil, 1)
	if got := board.Seats[1].Commanders; len(got) != 1 || got[0] != 22 {
		t.Errorf("seat 2's own Thrasios resolved to %v, want [22]", got)
	}
}

func TestACommanderForgeNeverGotIsNoThroneAtAll(t *testing.T) {
	t.Parallel()
	// The pre-flight drops a card Forge does not implement, so a match can run
	// without a commander in it. Zero is a real answer here: a throne that is
	// not drawn says "this card is not in this game", where a throne standing
	// empty says "it is out on the sand", and those are different facts.
	board := newForgeBoard(aPairing(), nil, map[int]forgeCommandZone{
		1: {Commanders: []string{"Someone Forge Has Never Heard Of"},
			Companion: "Nor This One"},
	}, nil, 1)
	if got := board.Seats[0].Commanders; len(got) != 0 {
		t.Errorf("an absent commander drew a throne anyway: %v", got)
	}
	if got := board.Seats[0].Companion; got != 0 {
		t.Errorf("an absent companion resolved to %d, want 0", got)
	}
	// And it stays off the wire rather than crossing as a zero, so a browser
	// tells "no companion" from "companion id nought".
	raw, err := json.Marshal(board.Seats[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "companion") ||
		strings.Contains(string(raw), "commanders") {
		t.Errorf("an empty command zone crossed as %s", raw)
	}
}
