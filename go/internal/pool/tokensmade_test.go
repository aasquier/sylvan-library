package pool_test

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// What a deck makes, and the two ways this must decline.
//
// The fixture is built for this question. Gyome, Master Chef and Bag End
// Banquet both make Food and **point at different Food printings** — Commander
// 2021's and a Secret Lair's, which are the real ids Scryfall gives them — so
// a reading that grouped by the printing named would list Food twice. The
// picture both of them get is Throne of Eldraine's, from 2019, which neither
// card names: the earliest printing of that token, which is `tokens.go`'s
// ruling applied to an identity instead of a word.
//
// Ajani's Cat Warrior and Terastodon's Elephant are the other half of the
// case: real tokens whose printings this pool does not carry, which must still
// be named rather than dropped.

// sheetFor is the whole question, asked of a pool at `path`.
func sheetFor(t *testing.T, path string, names ...string) pool.TokenSheet {
	t.Helper()
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	var sheet pool.TokenSheet
	err := p.Use(context.Background(), func(c *pool.Conn) error {
		var err error
		sheet, err = c.TokensMade(context.Background(), names)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return sheet
}

func TestADeckIsToldWhatItMakes(t *testing.T) {
	t.Parallel()
	sheet := sheetFor(t, pooltest.Build(t), "Gyome, Master Chef",
		"Bag End Banquet", "Terastodon", "Sol Ring")
	if !sheet.Read {
		t.Fatal("a pool with the column filled reported that it could not read it")
	}
	if len(sheet.Tokens) != 2 {
		t.Fatalf("got %d tokens, want 2 (Food and Elephant): %+v",
			len(sheet.Tokens), sheet.Tokens)
	}

	// Food first: the token this deck makes most is the one somebody has to
	// find a pile of before they sit down.
	food := sheet.Tokens[0]
	if food.Name != "Food" {
		t.Fatalf("the first token is %q; two cards make Food and one makes an "+
			"Elephant, so Food leads", food.Name)
	}
	if food.TypeLine != "Token Artifact — Food" {
		t.Errorf("Food's type line is %q", food.TypeLine)
	}
	if len(food.MadeBy) != 2 || food.MadeBy[0] != "Bag End Banquet" ||
		food.MadeBy[1] != "Gyome, Master Chef" {
		t.Errorf("Food is made by %v, want both cards, sorted", food.MadeBy)
	}
	if food.Art == nil {
		t.Fatal("Food came back with no painting, and this pool has four of them")
	}
	if food.Art.Artist != "Randy Gallegos" || food.Art.Set != "TELD" {
		t.Errorf("Food was painted by %q for %q, want Randy Gallegos / TELD — "+
			"the earliest printing, which is the pie anybody would recognise, "+
			"and which is not the printing either card names",
			food.Art.Artist, food.Art.Set)
	}
	if food.Art.Image == "" {
		t.Error("Food resolved with no image")
	}

	elephant := sheet.Tokens[1]
	if elephant.Name != "Elephant" || elephant.TypeLine != "Token Creature — Elephant" {
		t.Errorf("the second token is %q / %q, want Terastodon's Elephant",
			elephant.Name, elephant.TypeLine)
	}
	if len(elephant.MadeBy) != 1 || elephant.MadeBy[0] != "Terastodon" {
		t.Errorf("the Elephant is made by %v", elephant.MadeBy)
	}
	// This pool has no Elephant printing, and that is a plate with no painting
	// rather than a token nobody is told about.
	if elephant.Art != nil {
		t.Errorf("the Elephant found art %+v in a pool that has none", elephant.Art)
	}
}

// A double-faced card is asked for however the deck file spells it, and the
// answer credits the card by the caller's spelling rather than the pool's.
func TestATransformingCardIsFoundByItsFrontFace(t *testing.T) {
	t.Parallel()
	sheet := sheetFor(t, pooltest.Build(t), "Ajani, Nacatl Pariah")
	if len(sheet.Tokens) != 1 {
		t.Fatalf("got %+v, want Ajani's one Cat Warrior", sheet.Tokens)
	}
	cat := sheet.Tokens[0]
	if cat.Name != "Cat Warrior" {
		t.Errorf("the token is %q, want Cat Warrior", cat.Name)
	}
	if len(cat.MadeBy) != 1 || cat.MadeBy[0] != "Ajani, Nacatl Pariah" {
		t.Errorf("credited to %v; the deck asked by the front face and that is "+
			"the name it should read back", cat.MadeBy)
	}
}

// The empty answer, which is a real answer: these cards were read, and they
// make nothing.
func TestADeckThatMakesNothingIsStillRead(t *testing.T) {
	t.Parallel()
	sheet := sheetFor(t, pooltest.Build(t), "Sol Ring", "Swamp")
	if !sheet.Read {
		t.Fatal("a filled pool reported that it could not read")
	}
	if len(sheet.Tokens) != 0 {
		t.Errorf("Sol Ring and a Swamp make %+v", sheet.Tokens)
	}
	empty := sheetFor(t, pooltest.Build(t))
	if !empty.Read || len(empty.Tokens) != 0 {
		t.Errorf("no names came back as %+v", empty)
	}
}

// **The deploy window.** Merging is deploying, so this code runs against a
// library built before the column existed until somebody refreshes it. The
// honest answer there is "nobody has looked yet", never "this deck makes
// nothing" — the first sends a reader to wait, the second tells them
// something false about their deck.
func TestAPoolWithoutTheColumnSaysSoRatherThanSayingNothingIsMade(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	// The name index has to go first: DuckDB refuses to alter a table an
	// index depends on, which is also why the pool cannot migrate itself.
	alter(t, path, "DROP INDEX idx_oracle_name")
	alter(t, path, "ALTER TABLE oracle_cards DROP COLUMN all_parts")
	sheet := sheetFor(t, path, "Gyome, Master Chef")
	if sheet.Read {
		t.Error("a pool with no `all_parts` column claimed it had read one")
	}
	if len(sheet.Tokens) != 0 {
		t.Errorf("it also answered with %+v", sheet.Tokens)
	}
}

// The second way to be missing, and the reason the column's presence is not
// enough on its own: `OpenWriter`'s ladder adds the column to an old pool
// before the loaders fill it, so a refresh that adds the column and loads
// nothing into it leaves a table that binds and answers NULL to everything.
func TestAColumnWithNothingInItIsNotAnAnswer(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	alter(t, path, "UPDATE oracle_cards SET all_parts = NULL")
	sheet := sheetFor(t, path, "Gyome, Master Chef")
	if sheet.Read {
		t.Error("an empty `all_parts` column read as an answer; every card " +
			"in that pool would look like a card that makes nothing")
	}
}

func alter(t *testing.T, path, statement string) {
	t.Helper()
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(context.Background(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}
