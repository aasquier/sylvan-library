package deckread

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Two guards over a pool row that is not quite the shape the reader hopes for.
//
// Neither is a fault in the pool: one is a column a loader left empty and one is
// a card whose second face carries only half a picture. Both are states the
// served library can really be in, and both have a reader that answers
// differently because of them.

// A card whose keywords column is empty reaches the page as an empty list, not
// as a missing one.
//
// `keywords` is nullable, and the wire field is a list every caller ranges over
// — so a NULL that travelled as `null` would be a `.map` on nothing in the
// browser. The row is bent with a statement rather than invented: a real
// fixture card with its keyword list emptied, which is what a pool built before
// that column was filled actually looks like.
func TestACardWithNoKeywordColumnStillCarriesAList(t *testing.T) {
	t.Parallel()
	p := bentPool(t, "UPDATE oracle_cards SET keywords = NULL WHERE name = 'Sol Ring'")
	var rows NamedCards
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		out, err := CardsNamed(t.Context(), c, []string{"Sol Ring"})
		rows = out
		return err
	}); err != nil {
		t.Fatalf("reading a card whose keywords are empty: %v", err)
	}
	if len(rows.Cards) != 1 {
		t.Fatalf("the read came back with %d cards", len(rows.Cards))
	}
	if rows.Cards[0].Keywords == nil {
		t.Error("a card with no keyword column reached the page with no list at " +
			"all -- every caller ranges over that field")
	}
	if len(rows.Cards[0].Keywords) != 0 {
		t.Errorf("an emptied keyword column read back as %v", rows.Cards[0].Keywords)
	}
}

// A double-faced card whose second face has a crop and no card picture gets no
// face list at all, rather than a list the other half indexes into wrongly.
//
// The rule is `paintedTwice`'s own and it is written there: the row draws the
// crop and the hover draws the whole card, so a face with one and not the other
// is a turn that works in one place and blanks in the other. All 501 of these
// cards carried both when it was measured; this is the guard for the day one
// does not, and it is driven over a record built here rather than over a
// fixture row, because the shape being tested is a column that is half empty
// rather than a fact about any real card. The faces are named as the invented
// things they are.
func TestAFaceWithHalfAPictureCancelsTheWholeFaceList(t *testing.T) {
	t.Parallel()
	whole := "https://example.invalid/fixture-front.jpg"
	crop := "https://example.invalid/fixture-front-crop.jpg"

	both := &pool.CardRecord{
		Name: "Fixture Twin // Fixture Twin Reversed",
		Faces: []pool.CardFace{
			{Name: "Fixture Twin", ImageNormal: &whole, ImageArtCrop: &crop},
			{Name: "Fixture Twin Reversed", ImageNormal: &whole, ImageArtCrop: &crop},
		},
	}
	names, images, crops := paintedTwice(both)
	if len(names) != 2 || len(images) != 2 || len(crops) != 2 {
		t.Fatalf("a card whose faces both carry both pictures gave %d names, "+
			"%d pictures and %d crops", len(names), len(images), len(crops))
	}

	// Now take the second face's card picture away and leave its crop.
	half := &pool.CardRecord{
		Name: both.Name,
		Faces: []pool.CardFace{
			{Name: "Fixture Twin", ImageNormal: &whole, ImageArtCrop: &crop},
			{Name: "Fixture Twin Reversed", ImageArtCrop: &crop},
		},
	}
	names, images, crops = paintedTwice(half)
	if names != nil || images != nil || crops != nil {
		t.Errorf("a face with a crop and no card picture still produced %v / "+
			"%v / %v -- a list one half short is an index that answers "+
			"confidently for the wrong painting", names, images, crops)
	}

	// And a card with one face is not a two-faced card at all.
	if names, images, crops = paintedTwice(&pool.CardRecord{
		Name:  "Fixture Single",
		Faces: []pool.CardFace{{Name: "Fixture Single", ImageNormal: &whole, ImageArtCrop: &crop}},
	}); names != nil || images != nil || crops != nil {
		t.Errorf("a one-faced card produced %v / %v / %v", names, images, crops)
	}
}
