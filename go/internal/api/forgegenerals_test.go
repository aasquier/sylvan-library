package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The commander clock survives the shaping and reaches the browser.
//
// **A guard on a seam that fails silently, and this one fails differently from
// [TestHowAPermanentArrivedReachesTheRoom]'s.** That seam is a hand-written
// projection where a forgotten field is simply never copied. This one is the
// opposite risk: [newForgeBoard] passes [tier3.BoardStep] through *whole*, and
// [grantKeywords] rebuilds the slice with a `copy` — so the field arrives today
// for free and would vanish the day either of them is rewritten field by field.
// Nothing would fail: the room would draw a plate with no dial on it, which is
// exactly what a match with no commander damage in it looks like.
//
// So this asserts on the **rendered JSON**, not on the struct. A field that
// reaches the browser is a field with a tag on it in a response somebody can
// read, and only the render can say that.
//
// The numbers are from a real match on this laptop — Goreclaw/Stompy against
// Gyome/Food, seed 11, 2026-08-27. Gyome hit seat 1 four times for five and the
// dial finished on twenty; Goreclaw hit seat 2 once in the same game.
func TestTheCommanderClockReachesTheRoom(t *testing.T) {
	t.Parallel()
	reel := &tier3.BoardReel{
		Seats: []tier3.BoardSeat{
			{Seat: 1, Name: "Goreclaw — Stompy", Life: 40},
			{Seat: 2, Name: "Gyome — Food", Life: 40},
		},
		Cards: []tier3.BoardCard{
			{ID: 100, Name: "Goreclaw, Terror of Qal Sisma", Seat: 1},
			{ID: 201, Name: "Gyome, Master Chef", Seat: 2},
		},
		Steps: []tier3.BoardStep{
			{Turn: 5, Seat: 2, Generals: []tier3.BoardCommanderDamage{
				{Seat: 1, From: []tier3.BoardGeneral{{ID: 201, Damage: 20}}}}},
			{Turn: 6, Seat: 1, Generals: []tier3.BoardCommanderDamage{
				{Seat: 2, From: []tier3.BoardGeneral{{ID: 100, Damage: 5}}}}},
		},
	}
	board := newForgeBoard(reel, map[int]string{1: "goreclaw-stompy",
		2: "gyome-food"}, nil, nil, -1)
	if board == nil {
		t.Fatal("no board came back at all")
	}
	rendered, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("rendering the board: %v", err)
	}
	wire := string(rendered)
	for _, want := range []string{
		`"generals":[{"seat":1,"from":[{"id":201,"damage":20}]}]`,
		`"generals":[{"seat":2,"from":[{"id":100,"damage":5}]}]`,
	} {
		if !strings.Contains(wire, want) {
			t.Errorf("the board renders as\n%s\nand does not carry\n%s\n"+
				"the dial is drawn from this and nothing else fails when it "+
				"is dropped", wire, want)
		}
	}
}

// A match nobody's commander connected in carries no such key at all.
//
// `omitempty` is doing this, and it is load-bearing rather than tidy: the
// browser reads an absent `generals` as "no commander damage known" and an
// empty one the same way, but a `"generals":[]` on every step of every match
// would be the wire claiming the question had been asked and answered on beats
// where Forge never said anything.
func TestAnUntouchedBoardCarriesNoCommanderClock(t *testing.T) {
	t.Parallel()
	board := newForgeBoard(&tier3.BoardReel{
		Seats: []tier3.BoardSeat{{Seat: 1, Name: "Goreclaw — Stompy", Life: 40}},
		Steps: []tier3.BoardStep{{Turn: 1, Seat: 1}},
	}, nil, nil, nil, -1)
	rendered, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("rendering the board: %v", err)
	}
	if strings.Contains(string(rendered), "generals") {
		t.Errorf("a board no commander ever hit renders as\n%s\nand mentions "+
			"the clock; an empty set is a claim that it was measured",
			rendered)
	}
}
