package library_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The shelf's per-request cost, measured rather than reasoned about.
//
// `GET /api/decks` builds a Library per request, so `FileSource.All` runs
// once per visit to the shelf: a ReadDir, a Stat and a ReadFile per deck,
// and a full YAML parse per deck. Nothing memoises any of it, which is a
// fact about the code and not a complaint -- what this file exists for is
// the number that says whether it is worth an owner for a cache.
//
// Sized at the deployed library: 25 decks of 100 cards each, each card
// carrying the `why` rule 4 requires. Names are `Fixture ...` for
// `pooltest`'s reason -- nothing here resolves a card, but a deck file in a
// benchmark should still not read as a claim about a real card.

// benchDeck is one deck file with cards cards, in the shape the emitter writes.
func benchDeck(slug string, cards int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "slug: %s\nname: Fixture Deck %s\nstatus: built\nstage: curated\n", slug, slug)
	b.WriteString("commander:\n  - Fixture Commander\n")
	b.WriteString("archetype: midrange\nbracket: 3\n")
	b.WriteString("themes:\n  - fixtures\n  - measurement\n")
	b.WriteString("cards:\n")
	for i := range cards {
		fmt.Fprintf(&b, "  - name: Fixture Card %03d\n    category: ramp\n"+
			"    why: 'It pays for the next thing, and it is the %03dth reason "+
			"this deck file exists.'\n", i, i)
	}
	return b.String()
}

// benchShelf writes decks deck files of cards cards each and answers a file
// tier over them.
func benchShelf(tb testing.TB, decks, cards int) library.Source {
	tb.Helper()
	root := tb.TempDir()
	for i := range decks {
		slug := fmt.Sprintf("fixture-%02d", i)
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "deck.yaml"),
			[]byte(benchDeck(slug, cards)), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return library.NewFileSource(root, false)
}

// BenchmarkShelfAll is one visit to the deck shelf, file tier.
func BenchmarkShelfAll(b *testing.B) {
	ctx := context.Background()
	src := benchShelf(b, 25, 100)
	for b.Loop() {
		all, err := src.All(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if len(all) != 25 {
			b.Fatalf("read %d decks", len(all))
		}
	}
}

// BenchmarkDeckFromText is the parse alone, one deck, with the file read and
// the directory walk taken out -- so the split between syscalls and parsing
// is a measurement rather than an inference.
func BenchmarkDeckFromText(b *testing.B) {
	text := benchDeck("fixture-00", 100)
	for b.Loop() {
		d, err := deck.FromText(text, "fixture-00")
		if err != nil {
			b.Fatal(err)
		}
		if len(d.Cards) != 100 {
			b.Fatalf("parsed %d cards", len(d.Cards))
		}
	}
}
