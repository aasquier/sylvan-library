package api

import (
	"io"
	"log/slog"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/night"
)

// The night runner reads the house's deck off the file tier once a bout, and
// it reads through the same memory the shelf does -- the memo's cautionary
// tale is a cache that one caller forgets to be handed, and this is the
// caller the shelf memo left as a bare tier. Two reads of one house seat:
// the first parses, the second is remembered, and the API's own counters say
// which.
func TestTheHouseSeatIsReadFromTheSameMemoryAsTheShelf(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DecksDir: decksDir(t)})
	seat := night.Seat{Slug: "kaheera"}

	first, address, owner, err := a.nightDeck(t.Context(), seat)
	if err != nil {
		t.Fatal(err)
	}
	if owner != nil || address != "the house's kaheera" {
		t.Fatalf("the house seat resolved as %q owned by %v", address, owner)
	}
	if hits, misses := a.deckMemo.Counts(); hits != 0 || misses != 1 {
		t.Fatalf("the first read hits=%d misses=%d, want one parse", hits, misses)
	}

	second, _, _, err := a.nightDeck(t.Context(), seat)
	if err != nil {
		t.Fatal(err)
	}
	if hits, misses := a.deckMemo.Counts(); hits != 1 || misses != 1 {
		t.Fatalf("the second read hits=%d misses=%d, want it from memory", hits, misses)
	}
	if second != first {
		t.Fatal("the remembered read is a different parse from the first")
	}
	if first.Name != second.Name || len(first.Cards) != len(second.Cards) {
		t.Fatalf("the two reads disagree: %q/%d vs %q/%d", first.Name, len(first.Cards), second.Name, len(second.Cards))
	}
}
