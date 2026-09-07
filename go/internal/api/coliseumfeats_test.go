package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
)

// The paintings the three feat boards are drawn with, and the credit that has
// to travel with every one of them.
//
// **This test exists because the query it exercises was written wrong and
// nothing else could have said so.** The first draft selected
// `image_art_crop` out of `printings` — a column that lives on `oracle_cards`
// and has never existed on `printings`. The failure would have been silent all
// the way to the browser: [API.featArt] swallows a pool error on purpose, so
// every feat would have arrived with no painting and no complaint, and the
// board would have looked like a room where nothing had been played yet.
// A read against a real pool is the only thing that catches a column name.
func TestEveryFeatPaintingArrivesWithItsPainterAndItsPrinting(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	art := a.featArt(context.Background(), &ledger.Standings{
		Blows:  []ledger.KillRecord{{Card: "Craterhoof Behemoth"}},
		Giants: []ledger.GiantRecord{{Card: "Primeval Titan"}},
		Stacks: []ledger.StackRecord{{Card: "Food Token"}},
	})

	for _, name := range []string{"Craterhoof Behemoth", "Primeval Titan",
		"Food Token"} {
		one, ok := art[name]
		if !ok {
			t.Errorf("%q came back with no painting at all", name)
			continue
		}
		if one.Image == "" {
			t.Errorf("%q has a credit and no picture", name)
		}
		// Commandment 19, as a gate rather than a promise: a picture this app
		// draws is a picture this app credits, and the credit is the artist and
		// the printing in words.
		if one.Artist == "" {
			t.Errorf("%q is drawn with nobody credited for painting it", name)
		}
		if one.Printing == "" {
			t.Errorf("%q is credited to %q with no printing named", name,
				one.Artist)
		}
	}
}

// A feat may name a token, and Forge does not spell tokens the way Scryfall
// does. The stack board always names one; the biggest creature often does,
// because a 15/15 is as likely to be a token as a card somebody hard cast.
func TestAFeatTokenIsFoundUnderForgesOwnSpelling(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	art := a.featArt(context.Background(), &ledger.Standings{
		Stacks: []ledger.StackRecord{{Card: "Food Token"}},
	})

	// Filed under Forge's spelling, which is the only name the record has: a
	// board looking up `stack.card` must find it without knowing that the pool
	// calls it something else.
	food, ok := art["Food Token"]
	if !ok {
		t.Fatal(`the Food stack was filed under Scryfall's "Food" rather than ` +
			`the record's own "Food Token", so the board would look it up and ` +
			`find nothing`)
	}
	// The earliest printing, never the newest. Four Food printings are in the
	// fixture and the newest is not the pie anybody recognises.
	if food.Artist != "Randy Gallegos" {
		t.Errorf("the Food token was painted by %q, want Randy Gallegos — the "+
			"earliest printing is the original, and the newest is a lottery",
			food.Artist)
	}
	if food.Printing != "Throne of Eldraine Tokens" {
		t.Errorf("the Food token is credited to %q", food.Printing)
	}
}

// The rule that keeps the crossover paintings off this board, on a *real card*
// rather than on a token. It is the same trap that put Teenage Mutant Ninja
// Turtles art on the Grand Coliseum, and a hall of fame is exactly the surface
// it would look worst on.
func TestAFeatIsPaintedAsTheCardWasFirstPrinted(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	art := a.featArt(context.Background(), &ledger.Standings{
		Blows: []ledger.KillRecord{{Card: "Sol Ring"}},
	})

	ring := art["Sol Ring"]
	if ring.Printing != "Limited Edition Alpha" {
		t.Errorf("Sol Ring is credited to %q, want Limited Edition Alpha — the "+
			"pool holds a later printing too, and answering with it means the "+
			"picture changes every time the pool is refreshed", ring.Printing)
	}
	if ring.Artist != "Mark Tedin" {
		t.Errorf("Sol Ring was painted by %q, want Mark Tedin", ring.Artist)
	}
	if ring.Set != "LEA" {
		t.Errorf("Sol Ring's set reads %q, want the code upper-cased", ring.Set)
	}
}

// **A promo printing is not the printing anybody knows the card by**, and the
// date alone cannot keep it off this board.
//
// Found on the live board rather than reasoned out: the largest creature ever
// to stand on this sand was credited to "Bloomburrow Promos" and the third
// largest to "The Lost Caverns of Ixalan Promos" — both true, both naming a
// printing nobody associates with either card. The cause is that a set's promo
// printings carry the *same release date* as the set, so ranking on the date
// left the tie to an id, which is stable and meaningless.
//
// The fixture's Emrakul is the case exactly: Rise of the Eldrazi and Rise of
// the Eldrazi Promos, the same day, two different painters. Mark Tedin painted
// the card everybody has seen.
func TestAFeatIsCreditedToTheSetAndNotToItsPromo(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	art := a.featArt(context.Background(), &ledger.Standings{
		Giants: []ledger.GiantRecord{{Card: "Emrakul, the Aeons Torn"}},
	})

	emrakul := art["Emrakul, the Aeons Torn"]
	if emrakul.Printing != "Rise of the Eldrazi" {
		t.Errorf("Emrakul is credited to %q, want Rise of the Eldrazi — a "+
			"promo shares its set's release date, so ranking on the date "+
			"alone leaves the choice to an id", emrakul.Printing)
	}
	if emrakul.Artist != "Mark Tedin" {
		t.Errorf("Emrakul was painted by %q, want Mark Tedin", emrakul.Artist)
	}
}

// A name the pool has never heard of is absent, not an empty picture with a
// blank painter under it — the board draws a plate for it instead.
func TestAFeatNothingHasPrintedIsSimplyAbsent(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	art := a.featArt(context.Background(), &ledger.Standings{
		Giants: []ledger.GiantRecord{{Card: "Not A Real Card At All"}},
	})
	if _, ok := art["Not A Real Card At All"]; ok {
		t.Error("an unprintable name came back carrying an empty credit, which " +
			"a board cannot tell from a real one")
	}
}

// **No pool is not an error.** A record is a record without paintings, and
// refusing somebody their own bouts because the card pool has not been
// refreshed would be the worse answer by a mile.
func TestTheRecordAnswersWholeWithNoPoolBehindIt(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	art := a.featArt(context.Background(), &ledger.Standings{
		Blows: []ledger.KillRecord{{Card: "Craterhoof Behemoth"}},
	})
	if len(art) != 0 {
		t.Errorf("a poolless deployment invented %d paintings", len(art))
	}

	rec := httptest.NewRecorder()
	a.coliseumStandings(rec, httptest.NewRequest(http.MethodGet,
		"/api/coliseum/standings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("the record answered %d without a pool", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// The key is always there, so a browser reads an empty map rather than
	// `undefined` and one `?? {}` short of a crash.
	if _, ok := body["art"]; !ok {
		t.Error("`art` is missing from the answer rather than empty, so every " +
			"reader has to guard a key that should simply be there")
	}
}

// The record's own shape is untouched by the decoration: `art` is one more key
// beside every field the board already carried, never a wrapper around them.
func TestThePaintingsRideBesideTheRecordRatherThanAroundIt(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	rec := httptest.NewRecorder()
	a.coliseumStandings(rec, httptest.NewRequest(http.MethodGet,
		"/api/coliseum/standings", nil))

	body := rec.Body.String()
	for _, key := range []string{`"decks"`, `"archetypes"`, `"meetings"`,
		`"blows"`, `"giants"`, `"stacks"`, `"floor"`, `"art"`} {
		if !strings.Contains(body, key) {
			t.Errorf("%s is not on the wire:\n%s", key, body)
		}
	}
}
