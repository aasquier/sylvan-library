package opening

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
)

// The deal, and the counting beside it.
//
// The arithmetic is tested against hands built by hand rather than dealt: a
// dealt hand is a fact about a shuffle, and "a tapped land costs the turn it
// really costs" is a claim that has to be asked of a hand chosen to answer
// it. The deal itself is then held to the tiny pool, where the library is
// known and the seed is a promise.

func forest() *sim.Card {
	return &sim.Card{Name: "Forest", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}}
}

func tapped() *sim.Card {
	return &sim.Card{Name: "Blossoming Sands", IsLand: true, EntersTapped: true,
		Produces: []sim.Source{{Colors: []string{"G"}, Amount: 1}}}
}

func plains() *sim.Card {
	return &sim.Card{Name: "Plains", IsLand: true,
		Produces: []sim.Source{{Colors: []string{"W"}, Amount: 1}}}
}

// spell is a nonland costing `generic` plus one pip of each colour named.
func spell(name string, generic int, colors ...string) *sim.Card {
	pips := make([][]string, len(colors))
	for i, c := range colors {
		pips[i] = []string{c}
	}
	return &sim.Card{Name: name, Cost: sim.Cost{Generic: generic, Pips: pips}}
}

// turnOf reads one card's answer out of a whole hand's, which is the only way
// `earliestTurns` reports: positionally, alongside the hand it was given.
func turnOf(t *testing.T, hand []*sim.Card, index int) int {
	t.Helper()
	got := earliestTurns(hand)[index]
	if got == nil {
		t.Fatalf("%s: no turn at all, off %d cards", hand[index].Name, len(hand))
	}
	return *got
}

// Two untapped Forests cast a two-drop on turn two: the base case, and the
// number a beginner would count on their fingers.
func TestASpellLandsOnTheTurnItsLandsArrive(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{forest(), forest(), spell("Grizzly Bears", 1, "G")}
	if got := turnOf(t, hand, 2); got != 2 {
		t.Fatalf("turn %d, want 2", got)
	}
}

// The same two-drop off two lands that enter tapped lands on turn THREE, not
// turn two -- the second land is played on turn two and does nothing until
// turn three. This is the whole reason the subset enumeration tracks tapped
// at all: a beginner with two taplands who was told "turn two" would sit
// there on turn two unable to do the thing they were promised.
func TestALandThatEntersTappedCostsTheTurnItReallyCosts(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{tapped(), tapped(), spell("Grizzly Bears", 1, "G")}
	if got := turnOf(t, hand, 2); got != 3 {
		t.Fatalf("turn %d, want 3", got)
	}
}

// One untapped land among tapped ones buys the turn back: play the tapped
// one first, the untapped one last, and the mana is all there on turn two.
func TestOneUntappedLandIsPlayedLastAndBuysTheTurnBack(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{tapped(), forest(), spell("Grizzly Bears", 1, "G")}
	if got := turnOf(t, hand, 2); got != 2 {
		t.Fatalf("turn %d, want 2", got)
	}
}

// Colour is not a formality. Two Plains never cast a green two-drop however
// many turns pass, and the honest answer is no turn at all rather than a
// large one.
func TestASpellTheseLandsCannotColourHasNoTurn(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{plains(), plains(), spell("Grizzly Bears", 1, "G")}
	if got := earliestTurns(hand)[2]; got != nil {
		t.Fatalf("turn %d, want none -- two Plains do not cast a green spell", *got)
	}
}

// A spell that costs more than the hand holds lands for is also no turn: the
// counting is over THIS hand's lands, and a fourth land off the top is a
// draw, not a fact.
func TestASpellBeyondTheHandsLandsHasNoTurn(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{forest(), forest(), spell("Craterhoof Behemoth", 5, "G", "G", "G")}
	if got := earliestTurns(hand)[2]; got != nil {
		t.Fatalf("turn %d, want none -- eight mana off two Forests", *got)
	}
}

// A land is played, not cast, so it never carries a turn.
func TestALandCarriesNoTurn(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{forest(), spell("Llanowar Elves", 0, "G")}
	if got := earliestTurns(hand)[0]; got != nil {
		t.Fatalf("a Forest was given turn %d", *got)
	}
}

// The reading: lands and spells partition the seven, the commander's colours
// split into covered and missing by what the lands in hand can actually make,
// the first spell is the earliest of them, and the horizon count is a reach
// rather than a turn.
func TestTheReadingCountsWhatIsInFrontOfYou(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{
		forest(), forest(), forest(),
		spell("Grizzly Bears", 1, "G"),
		spell("Llanowar Elves", 0, "G"),
		spell("Craterhoof Behemoth", 5, "G", "G", "G"),
		spell("Swords to Plowshares", 0, "W"),
	}
	got := read(hand, earliestTurns(hand), []string{"G", "W"})
	if got.Lands != 3 || got.Spells != 4 {
		t.Fatalf("%d lands / %d spells, want 3/4", got.Lands, got.Spells)
	}
	if len(got.ColorsCovered) != 1 || got.ColorsCovered[0] != "G" {
		t.Fatalf("covered %v, want [G] -- three Forests make green and nothing else", got.ColorsCovered)
	}
	if len(got.ColorsMissing) != 1 || got.ColorsMissing[0] != "W" {
		t.Fatalf("missing %v, want [W]", got.ColorsMissing)
	}
	if got.FirstSpellTurn == nil || *got.FirstSpellTurn != 1 {
		t.Fatalf("first spell %v, want turn 1 -- the Elves cost one green", got.FirstSpellTurn)
	}
	// Elves on one and Bears on two. The Behemoth wants eight mana and the
	// Swords wants white, and neither is reachable off three Forests.
	if got.CastableByHorizon != 2 {
		t.Fatalf("castable by turn %d: %d, want 2", got.Horizon, got.CastableByHorizon)
	}
}

// A deck with no commander has no colours to be measured against, and says so
// by reporting neither -- empty rather than nil, so the wire carries `[]`
// and a client never has to tell an absent list from an empty one.
func TestNoCommanderIsNoColoursRatherThanNoField(t *testing.T) {
	t.Parallel()
	hand := []*sim.Card{forest(), spell("Grizzly Bears", 1, "G")}
	got := read(hand, earliestTurns(hand), nil)
	if got.ColorsCovered == nil || got.ColorsMissing == nil {
		t.Fatalf("nil colour lists: %+v", got)
	}
	if len(got.ColorsCovered) != 0 || len(got.ColorsMissing) != 0 {
		t.Fatalf("colours out of nowhere: %+v", got)
	}
}

// ---------------------------------------------------------------- the deal

// tinyDeck is a real deck over the 21-card pool: a green commander, four
// spells, and enough Forests to make a library worth shuffling.
func tinyDeck() *deck.Deck {
	return &deck.Deck{
		Slug:      "opening-fixture",
		Commander: []string{"Goreclaw, Terror of Qal Sisma"},
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Category: "ramp", Why: "fixture", Qty: 1},
			{Name: "Craterhoof Behemoth", Category: "threat", Why: "fixture", Qty: 1},
			{Name: "Regal Behemoth", Category: "threat", Why: "fixture", Qty: 1},
			{Name: "Primeval Titan", Category: "threat", Why: "fixture", Qty: 1},
			{Name: "Forest", Category: "land", Why: "fixture", Qty: 40},
		},
	}
}

func tinyRecords(t *testing.T) map[string]*pool.CardRecord {
	t.Helper()
	p := pooltest.Open(t)
	var found map[string]*pool.CardRecord
	err := p.Use(context.Background(), func(c *pool.Conn) error {
		var err error
		found, err = poolFor(context.Background(), c, tinyDeck())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// A seed is a promise: the same seed deals the same seven cards in the same
// order, which is what lets the route roll a fresh seed and say nothing
// about it.
func TestTheSameSeedDealsTheSameHand(t *testing.T) {
	t.Parallel()
	cards := tinyRecords(t)
	first, err := Deal(tinyDeck(), cards, big.NewInt(20260827))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Deal(tinyDeck(), cards, big.NewInt(20260827))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Cards) != Size {
		t.Fatalf("dealt %d cards, want %d", len(first.Cards), Size)
	}
	for i := range first.Cards {
		if first.Cards[i].Name != again.Cards[i].Name {
			t.Fatalf("card %d: %q then %q", i, first.Cards[i].Name, again.Cards[i].Name)
		}
	}
}

// A nil seed deals anyway. The point is not that the hand differs -- over a
// library of forty Forests two fresh deals often read the same -- but that
// the route's own call path, which never supplies one, produces a full hand
// rather than a refusal.
func TestNoSeedStillDealsAWholeHand(t *testing.T) {
	t.Parallel()
	hand, err := Deal(tinyDeck(), tinyRecords(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hand.Cards) != Size {
		t.Fatalf("dealt %d cards, want %d", len(hand.Cards), Size)
	}
	if hand.Reading.Lands+hand.Reading.Spells != Size {
		t.Fatalf("%d lands + %d spells is not %d", hand.Reading.Lands, hand.Reading.Spells, Size)
	}
}

// The hand is dealt off the 99, and the commander travels beside it rather
// than in it -- the command zone is where a commander starts, and a toy that
// shuffled it into the library would teach the wrong game.
func TestTheCommanderIsBesideTheHandAndNotInIt(t *testing.T) {
	t.Parallel()
	hand, err := Deal(tinyDeck(), tinyRecords(t), big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}
	if hand.Commander == nil || hand.Commander.Name != "Goreclaw, Terror of Qal Sisma" {
		t.Fatalf("commander %+v", hand.Commander)
	}
	if hand.DeckSize != 44 {
		t.Fatalf("shuffled %d cards, want the 44 the deck declares without its commander", hand.DeckSize)
	}
	for _, c := range hand.Cards {
		if c.Name == "Goreclaw, Terror of Qal Sisma" {
			t.Fatal("the commander was shuffled into the library")
		}
	}
}

// Every dealt card is drawable: a name, a printed type line and the picture
// the table draws. A hand of blank frames is the failure this catches, and
// it is invisible to a count.
func TestEveryDealtCardCarriesEnoughToDrawIt(t *testing.T) {
	t.Parallel()
	hand, err := Deal(tinyDeck(), tinyRecords(t), big.NewInt(11))
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range hand.Cards {
		if c.Name == "" || c.TypeLine == "" || c.Image == nil || *c.Image == "" {
			t.Fatalf("card %d is not drawable: %+v", i, c)
		}
	}
}

// A hand shorter than seven is dealt rather than refused: a library of three
// hands over three cards. The refusal is reserved for a deck with nothing in
// it at all.
func TestAShortLibraryDealsWhatItHas(t *testing.T) {
	t.Parallel()
	small := tinyDeck()
	small.Cards = []deck.CardEntry{{Name: "Forest", Category: "land", Why: "fixture", Qty: 3}}
	hand, err := Deal(small, tinyRecords(t), big.NewInt(3))
	if err != nil {
		t.Fatal(err)
	}
	if len(hand.Cards) != 3 {
		t.Fatalf("dealt %d cards off a library of three", len(hand.Cards))
	}
}

// The one state that refuses, and it refuses with compile's own error so the
// route can tell it from "this machine has no pool" and say the right
// sentence to a person.
func TestADeckThatCompilesToNothingIsRefused(t *testing.T) {
	t.Parallel()
	empty := tinyDeck()
	empty.Cards = []deck.CardEntry{{Name: "A Card No Pool Has", Category: "land", Why: "x", Qty: 1}}
	_, err := Deal(empty, tinyRecords(t), big.NewInt(1))
	var nothing *compile.NothingToSimulate
	if !errors.As(err, &nothing) {
		t.Fatalf("err %v, want *compile.NothingToSimulate", err)
	}
}

// A pool that is not there is `*compile.PoolRequired`, which the route turns
// into the degraded 200 rather than an error -- the page renders and says
// why it is empty.
func TestNoPoolIsPoolRequired(t *testing.T) {
	t.Parallel()
	_, err := DealFromPool(context.Background(), nil, tinyDeck(), big.NewInt(1))
	var needed *compile.PoolRequired
	if !errors.As(err, &needed) {
		t.Fatalf("err %v, want *compile.PoolRequired", err)
	}
}
