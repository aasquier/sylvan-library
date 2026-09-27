package pool

import (
	"context"
	"testing"
)

// The reader's own edges: a row that is all NULLs, a faces document that is
// not one, and the memo's ceiling.
//
// These are the shapes a *degraded* pool hands back -- an old file whose newer
// columns select as NULL, a `card_faces` written by something that was not
// this loader -- and the reading of each of them has a wrong answer that is
// worse than a refusal. A layout that reads as empty makes every renderer
// treat a normal card as an unknown one; a faces list that is *short* is an
// index that silently answers for the wrong half of a double-faced card.

// A card a pool has almost nothing to say about still reads as a card. This is
// what every column the ladder has not filled looks like on the way out, and
// the one thing the reader may not do with it is hand back a record whose
// layout is the empty string -- "" is not a layout, and nothing downstream has
// a branch for it.
func TestARowOfNothingButNullsReadsAsAnOrdinaryCard(t *testing.T) {
	t.Parallel()
	rec := toRecord(make([]any, len(readColumns)))
	if rec.Layout != "normal" {
		t.Errorf("a row with no layout read as %q; every renderer asks this "+
			"field which side of a card it is looking at", rec.Layout)
	}
	if rec.Faces != nil {
		t.Errorf("a card with no faces document grew faces: %+v", rec.Faces)
	}
	if rec.ColorIdentity == nil || rec.Keywords == nil || rec.ProducedMana == nil {
		t.Errorf("a list column read as nil rather than as empty: %+v", rec)
	}
	if rec.LegalCommander {
		t.Error("a card with no legalities was declared Commander-legal")
	}
}

// The back of a modal card is asked; the back of a transforming one is not.
// The third answer is the one nothing had driven: a modal card whose other
// side is a spell, which is a land on neither face.
func TestAModalCardWithNoLandOnEitherFaceIsNotALand(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		what   string
		rec    CardRecord
		isLand bool
	}{
		{"a modal card with a land on the back",
			CardRecord{TypeLine: "Creature — Elf // Land", Layout: "modal_dfc"}, true},
		{"a modal card that is spells on both sides",
			CardRecord{TypeLine: "Creature — Elf // Instant", Layout: "modal_dfc"}, false},
		{"a transforming card whose back is a land",
			CardRecord{TypeLine: "Creature — Human // Land", Layout: "transform"}, false},
		{"a plain land",
			CardRecord{TypeLine: "Basic Land — Forest", Layout: "normal"}, true},
	} {
		if got := one.rec.IsLand(); got != one.isLand {
			t.Errorf("%s: IsLand() = %v, want %v", one.what, got, one.isLand)
		}
	}
}

// `card_faces` arrives decoded from a JSON column and as text from a pool that
// stored it as VARCHAR, and both have to read.
//
// **A face with no name drops them all, deliberately.** Every caller indexes
// into this list beside the names the card's own `A // B` splits into, so a
// list that is one short is not a smaller answer -- it is an index that
// answers confidently for the wrong side of the card.
func TestAFacesDocumentIsReadWholeOrNotAtAll(t *testing.T) {
	t.Parallel()
	const both = `[{"name":"Fixture Pariah","type_line":"Creature — Cat",
	  "image_uris":{"normal":"https://example.invalid/front.jpg"}},
	  {"name":"Fixture Ascendant","type_line":"Creature — Cat Warrior"}]`

	for _, one := range []struct {
		how   string
		given any
	}{
		{"a VARCHAR column, as text", both},
		{"a BLOB column, as bytes", []byte(both)},
	} {
		faces := cardFaces(one.given)
		if len(faces) != 2 {
			t.Errorf("%s: read %d faces, want two", one.how, len(faces))
			continue
		}
		if faces[0].Name != "Fixture Pariah" || faces[1].Name != "Fixture Ascendant" {
			t.Errorf("%s: read %+v", one.how, faces)
		}
		if faces[0].ImageNormal == nil {
			t.Errorf("%s: the front face lost its painting", one.how)
		}
		if faces[0].ImageArtCrop != nil {
			t.Errorf("%s: a crop was invented for a face that has none", one.how)
		}
	}

	for _, one := range []struct {
		how   string
		given any
	}{
		{"a NULL column", nil},
		{"a column of some other type", int64(3)},
		{"text that is not the document", "{ not json"},
		{"bytes that are not the document", []byte("{ not json")},
		{"an empty list", `[]`},
		{"a list whose second face has no name", `[{"name":"Fixture Pariah"},{"type_line":"Land"}]`},
	} {
		if faces := cardFaces(one.given); faces != nil {
			t.Errorf("%s: read %+v, want nothing -- a partial list is an index "+
				"that answers for the wrong face", one.how, faces)
		}
	}
}

// The memo has a ceiling, and reaching it evicts the least recently wanted
// answer rather than growing without end. A cache with no bound on a
// long-running process is a leak that looks exactly like a working cache.
func TestTheCardMemoEvictsTheLeastRecentlyWantedAnswer(t *testing.T) {
	t.Parallel()
	cache := newCardCache()
	keys := make([]string, cache.max+1)
	for i := range keys {
		keys[i] = string(rune('a' + i))
		cache.put(keys[i], map[string]*CardRecord{keys[i]: {Name: keys[i]}})
	}
	if got := cache.order.Len(); got != cache.max {
		t.Fatalf("the memo holds %d answers, want its ceiling of %d",
			got, cache.max)
	}
	if _, ok := cache.get(keys[0]); ok {
		t.Errorf("the oldest answer %q survived the ceiling", keys[0])
	}
	if _, ok := cache.get(keys[len(keys)-1]); !ok {
		t.Error("the newest answer was the one evicted")
	}
	if len(cache.items) != cache.max {
		t.Errorf("the index kept %d entries against %d in the queue -- an "+
			"eviction that forgot the map is the leak the ceiling exists to "+
			"prevent", len(cache.items), cache.order.Len())
	}
}

// Every read refuses a pool it cannot ask what columns there are, rather than
// building a SELECT out of what it hopes is there.
//
// The select clause is assembled from the pool's *own* column list precisely
// so an old file degrades to "we do not know" instead of failing to bind --
// which means a failure to read that list is a failure to know what degrading
// even means, and answering anything at all from there would be a guess.
func TestEveryRecordReadRefusesAPoolThatCannotListItsColumns(t *testing.T) {
	t.Parallel()
	c := onAClosedPool(t)
	ctx := context.Background()

	if got, err := c.GetCards(ctx, []string{"Fixture Chef"}); err == nil {
		t.Errorf("a closed pool answered GetCards with %+v", got)
	}
	if got, err := c.Search(ctx, "true", nil, 10, "", 0); err == nil {
		t.Errorf("a closed pool answered Search with %+v", got)
	}
}

// A search whose WHERE the pool cannot answer is a refusal, not an empty
// result — the two read identically to a caller and only one of them is true.
func TestASearchThePoolCannotAnswerIsRefusedRatherThanEmptied(t *testing.T) {
	t.Parallel()
	c := onTables(t, `CREATE TABLE oracle_cards (name VARCHAR)`)
	got, err := c.Search(context.Background(),
		"no_such_column = 'Fixture Chef'", nil, 10, "", 0)
	if err == nil {
		t.Fatalf("a search on a column nothing has answered %+v", got)
	}
	if got != nil {
		t.Errorf("the refusal came back carrying rows anyway: %+v", got)
	}
}
