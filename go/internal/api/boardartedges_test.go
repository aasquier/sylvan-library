package api

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// [API.resolveBoardArt] at its edges: the dictionary it fills spans a whole
// match, so what it does with a name it has *already* seen decides whether a
// pod of four costs one round trip or forty.
//
// `forgeboard_test.go` drives the ordinary cases — a card, a token, a land's
// mana. These are the four it does not reach, and each of them is a promise
// the room depends on rather than a defensive line: nothing is asked twice,
// nothing is asked when there is nothing to ask, a two-faced card arrives with
// both faces, and a pool that answers an error leaves the board drawable.

func boardOf(t *testing.T, a *API, cards []tier3.BoardCard) map[string]boardArt {
	t.Helper()
	known := map[string]boardArt{}
	a.resolveBoardArt(t.Context(), cards, known)
	return known
}

// A name already in the dictionary is not asked about again, and a card with
// no name at all is not asked about at all.
//
// The marking is what makes the second game of a pairing cheap: the same
// hundred cards are named again, and a miss is remembered as a miss so the
// pool is not asked a second time for a name it has already declined.
func TestTheBoardAsksAboutANameOnceAcrossAWholeMatch(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	known := map[string]boardArt{}
	a.resolveBoardArt(t.Context(), []tier3.BoardCard{
		{Name: "Craterhoof Behemoth"},
		// A name the pool has never heard of, remembered as a miss.
		{Name: "Fixture Shape That Is Not A Card"},
		// No name at all: Forge sends a face-down permanent this way.
		{Name: ""},
	}, known)
	if len(known) != 2 {
		t.Fatalf("the dictionary holds %d entries: %v", len(known), known)
	}
	if _, nameless := known[""]; nameless {
		t.Error("a card with no name was filed under the empty string")
	}
	painted := known["Craterhoof Behemoth"]
	if painted.Image == "" {
		t.Fatal("a card the pool knows went unpainted")
	}
	if miss := known["Fixture Shape That Is Not A Card"]; miss.Image != "" {
		t.Errorf("a name the pool does not know was painted %q", miss.Image)
	}

	// The second game names them all again. Nothing changes, and -- the point
	// -- the miss stays a miss rather than becoming a second round trip.
	before := len(known)
	a.resolveBoardArt(t.Context(), []tier3.BoardCard{
		{Name: "Craterhoof Behemoth"},
		{Name: "Fixture Shape That Is Not A Card"},
	}, known)
	if len(known) != before {
		t.Errorf("the dictionary grew from %d to %d on a repeat", before, len(known))
	}
	if known["Craterhoof Behemoth"].Image != painted.Image {
		t.Error("a second look at one card painted it differently")
	}
}

// A board of nothing new asks the pool nothing. Proven the way the killers'
// early return is: against a pool that answers an error, where a read that
// happened anyway would leave the dictionary's entries blanked.
func TestABoardWithNothingNewOnItAsksThePoolNothing(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: schemalessPool(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	known := map[string]boardArt{"Fixture Shape A": {Image: "already-here"}}
	a.resolveBoardArt(t.Context(), []tier3.BoardCard{{Name: "Fixture Shape A"}}, known)
	if known["Fixture Shape A"].Image != "already-here" {
		t.Errorf("a board with nothing new on it went to the pool and lost what "+
			"it had: %v", known["Fixture Shape A"])
	}
	a.resolveBoardArt(t.Context(), nil, known)
	if len(known) != 1 {
		t.Errorf("an empty board changed the dictionary: %v", known)
	}
}

// A two-faced card arrives with both faces, their type lines and their
// pictures -- which is what lets the board draw the half that is actually on
// the battlefield rather than always the front.
func TestATwoFacedCardArrivesWithBothOfItsFaces(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	// A real transforming card out of the recorded fixture, named by its
	// combined name exactly as the pool spells it. Its two faces have
	// *different* type lines, which is the half the browser could already
	// answer -- the point here is that both halves cross at all.
	const both = "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"
	known := boardOf(t, a, []tier3.BoardCard{{Name: both}})
	art, found := known[both]
	if !found {
		t.Fatalf("the pool did not answer %q: %v", both, known)
	}
	if len(art.Faces) < 2 {
		t.Fatalf("a two-faced card arrived with %d faces: %+v", len(art.Faces), art)
	}
	if art.Layout == "" {
		t.Error("the card arrived with no layout, so the board cannot tell how it turns")
	}
	if len(art.FaceTypes) != len(art.Faces) {
		t.Errorf("%d faces and %d type lines", len(art.Faces), len(art.FaceTypes))
	}
	if len(art.FaceImages) != len(art.Faces) {
		t.Fatalf("%d faces and %d pictures", len(art.Faces), len(art.FaceImages))
	}

	// And the board can then ask for a face by name, which is the whole
	// reason both pictures travel: `image_normal` is always the *front*, so a
	// card played as its back half would otherwise arrive correctly named and
	// wearing the wrong painting.
	if got := faceUp(art, art.Faces[1]); got != art.FaceImages[1] {
		t.Errorf("the back face resolved to %q", got)
	}
	if art.FaceImages[0] == art.FaceImages[1] {
		t.Error("both faces carry the same painting")
	}
	// The combined name matches no single face and keeps the front, which is
	// the right answer for a board that has not been told which half it is
	// looking at.
	if got := faceUp(art, both); got != art.Image {
		t.Errorf("the combined name resolved to %q rather than the front", got)
	}
}

// A two-faced card whose back has no recorded picture arrives with its faces
// and **no** face pictures at all, rather than with a half-filled list the
// board would index into.
//
// The fixture's Etali is exactly that shape, and it is the shape the real pool
// has whenever Scryfall records a face without its own `image_uris`. All or
// none is the contract `facePicturesOf` keeps; a list shorter than the faces
// is how a board draws a blank where a creature is.
func TestAFaceWithNoPictureLeavesTheWholeListEmpty(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	const both = "Etali, Primal Conqueror // Etali, Primal Sickness"
	known := boardOf(t, a, []tier3.BoardCard{{Name: both}})
	art, found := known[both]
	if !found {
		t.Fatalf("the pool did not answer %q: %v", both, known)
	}
	if len(art.Faces) < 2 {
		t.Fatalf("a two-faced card arrived with %d faces: %+v", len(art.Faces), art)
	}
	if len(art.FaceImages) != 0 {
		t.Errorf("a card with one picture between two faces carries %d: %v",
			len(art.FaceImages), art.FaceImages)
	}
	// With no per-face pictures, every face falls back to the card's own --
	// which is the front, and is the best answer there is.
	if got := faceUp(art, art.Faces[1]); got != art.Image {
		t.Errorf("a face with no picture resolved to %q rather than the front", got)
	}
}

// A pool that opens and then fails its query leaves the board drawable: a
// match is worth watching without paintings, and refusing to shape one
// because the library will not answer would cost somebody a match they are
// already watching. Both halves of the read -- cards and tokens -- take that
// branch, so both are asked.
func TestAPoolThatWillNotAnswerStillLeavesABoardToDraw(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: schemalessPool(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	for _, card := range []tier3.BoardCard{
		{Name: "Craterhoof Behemoth"},
		{Name: "Beast Token", Token: true},
	} {
		known := boardOf(t, a, []tier3.BoardCard{card})
		art, found := known[card.Name]
		if !found {
			t.Fatalf("%q fell out of the dictionary entirely", card.Name)
		}
		if art.Image != "" || art.Art != "" || len(art.Faces) != 0 {
			t.Errorf("%q was painted from a pool that answered an error: %+v",
				card.Name, art)
		}
	}
}

// The token half against a working pool, named the way Forge names it: the
// dictionary's key is Forge's spelling, because that is the name every card
// on the board is filed under.
func TestATokenIsFiledUnderForgesOwnSpelling(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	const forgeName = "Ajani Steadfast Emblem Token"
	known := boardOf(t, a, []tier3.BoardCard{{Name: forgeName, Token: true}})
	if _, found := known[forgeName]; !found {
		t.Fatalf("the token is not filed under Forge's own spelling: %v", known)
	}
	for key := range known {
		if strings.TrimSuffix(key, " Token") == key {
			continue
		}
		if key != forgeName {
			t.Errorf("the token was also filed under %q", key)
		}
	}
}
