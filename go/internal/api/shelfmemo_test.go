package api

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// The cautionary tale the memo was designed against, held by a route: a
// cache that is correct, tested and never consulted. This drives `GET
// /api/decks` through a real API twice and reads the API's own counters, so
// the memo proven here is the one the route reaches -- not one a test built
// beside it.
func TestTheShelfIsAnsweredFromMemoryOnTheSecondVisit(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, false)
	defer done()

	status, _, first := as(t, a, auth.Local, "/api/decks")
	if status != 200 {
		t.Fatalf("%d %s", status, first)
	}
	hits, misses := a.deckMemo.Counts()
	if hits != 0 || misses < 7 {
		t.Fatalf("the first visit read hits=%d misses=%d; the fixture library has at least seven decks", hits, misses)
	}
	decks := misses

	_, _, second := as(t, a, auth.Local, "/api/decks")
	if !bytes.Equal(first, second) {
		t.Fatalf("the remembered shelf differs from the parsed one:\n%s\n%s", first, second)
	}
	hits, misses = a.deckMemo.Counts()
	if hits != decks || misses != decks {
		t.Fatalf("the second visit read hits=%d misses=%d, want every one of %d decks from memory", hits, misses, decks)
	}

	// The 32 Deck Challenge scores the same files and reads through the
	// same memory: a visit there costs no parse either.
	if status, _, raw := as(t, a, auth.Local, "/api/colors/progress"); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	hits, misses = a.deckMemo.Counts()
	if hits != 2*decks || misses != decks {
		t.Fatalf("after the challenge read hits=%d misses=%d", hits, misses)
	}

	// An edit on disk -- what every write verb amounts to -- is one miss on
	// the next visit, and the shelf says the new name.
	path := filepath.Join(a.decksDir, "kaheera", "deck.yaml")
	text, err := os.ReadFile(path) //nolint:gosec // the test's own fixture
	if err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(text, []byte("name: Kaheera Fixture"), []byte("name: Kaheera, Remembered"), 1)
	if bytes.Equal(edited, text) {
		t.Fatalf("the fixture's name line moved; the edit changed nothing:\n%s", text)
	}
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, third := as(t, a, auth.Local, "/api/decks")
	if !bytes.Contains(third, []byte(`"Kaheera, Remembered"`)) {
		t.Fatalf("the edited deck was answered from memory: %s", third)
	}
	hits, misses = a.deckMemo.Counts()
	if hits != 3*decks-1 || misses != decks+1 {
		t.Fatalf("after the edit hits=%d misses=%d, want one miss for the one moved file", hits, misses)
	}
}
