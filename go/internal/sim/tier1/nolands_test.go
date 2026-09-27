package tier1

// A goldfish with no lands to find, a run with no games in it, and the two
// small renderings around them.
//
// The engine is asked about decks that are being built, which is where the
// nonsense inputs come from. A deck of ramp with no lands in it yet is a real
// state on the way to a finished one, and the fetch effects in it have to
// come back empty-handed rather than reaching past the end of the library.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim"
)

func TestAFetchWithNoLandLeftToFindComesBackEmptyHanded(t *testing.T) {
	t.Parallel()
	// Thirty pieces of fast mana and thirty fetch spells, and not one land
	// in the deck. Every fetch resolves and finds nothing, which must leave
	// the battlefield as it was rather than putting a nonland onto it.
	library := make([]*sim.Card, 0, 60)
	for i := 0; i < 30; i++ {
		library = append(library, &sim.Card{Name: "Fixture Lotus",
			Produces: []sim.Source{{Colors: []string{"C"}, Amount: 3}}})
	}
	for i := 0; i < 30; i++ {
		library = append(library, &sim.Card{Name: "Fixture Harrow",
			Cost: sim.Cost{Generic: 1}, FetchesLands: 2})
	}
	// No RNG and no keep rule: the run layer above supplies both, and a
	// caller that supplies neither still gets a whole game -- just one that
	// cannot be replayed.
	got := SimulateGame(library, nil, GameOptions{Turns: 8})
	if len(got.LandsByTurn) != 8 {
		t.Fatalf("%d turns of lands for an eight-turn game", len(got.LandsByTurn))
	}
	for turn, lands := range got.LandsByTurn {
		if lands != 0 {
			t.Fatalf("turn %d found %d lands in a deck with none", turn+1, lands)
		}
	}
	// The fast mana is still mana, so the game is not simply dead: something
	// was cast.
	if got.FirstSpellTurn == nil {
		t.Fatal("a deck of thirty lotuses cast nothing in eight turns")
	}
}

func TestARunOfNoGamesIsRefusedRatherThanDividedBy(t *testing.T) {
	t.Parallel()
	// The summary divides by the number of games in a dozen places, so zero
	// games is NaN in every figure on the page. Refusing loudly is the
	// recorded choice.
	var said string
	func() {
		defer func() {
			if r := recover(); r != nil {
				said, _ = r.(string)
			}
		}()
		Run([]*sim.Card{{Name: "Fixture Waste", IsLand: true}}, nil, Options{Games: 0})
	}()
	if !strings.Contains(said, "needs at least one game") {
		t.Fatalf("a run of no games said %q", said)
	}
}

func TestRemovingACardThatIsNotInTheListIsRefused(t *testing.T) {
	t.Parallel()
	// The removal is by first *equal*, not by identity, because a compiled
	// deck repeats one pointer per copy. A card equal to nothing in the list
	// means the caller has lost track of the library, and silently returning
	// it unchanged would hide a card that has been played twice.
	list := []*sim.Card{{Name: "Fixture Waste", IsLand: true}}
	var said string
	func() {
		defer func() {
			if r := recover(); r != nil {
				said, _ = r.(string)
			}
		}()
		removeFirstEqual(list, &sim.Card{Name: "Fixture Stranger"})
	}()
	if !strings.Contains(said, "not in the list") {
		t.Fatalf("removing a stranger said %q", said)
	}
	// And an equal card leaves, whichever copy it was handed.
	shorter := removeFirstEqual(list, &sim.Card{Name: "Fixture Waste", IsLand: true})
	if len(shorter) != 0 {
		t.Fatalf("the list still holds %d cards", len(shorter))
	}
}

func TestANumberReadsBackAsTheKindItWasWrittenAs(t *testing.T) {
	t.Parallel()
	// A stored result is decoded into the struct it was encoded from, so the
	// round trip has to preserve which of the two a number was: the digest
	// hashes the rendering, and `4` and `4.0` are different text.
	cases := []struct {
		name  string
		raw   string
		want  Number
		fails bool
	}{
		{name: "an integer", raw: "4", want: Number{Int: 4}},
		{name: "a negative integer", raw: "-4", want: Number{Int: -4}},
		{name: "a float", raw: "4.5", want: Number{IsFloat: true, Float: 4.5}},
		{name: "a whole float", raw: "4.0", want: Number{IsFloat: true, Float: 4}},
		{name: "an exponent", raw: "1e2", want: Number{IsFloat: true, Float: 100}},
		{name: "nothing at all", raw: "null", want: Number{}},
		{name: "a string where a number belongs", raw: `"4"`, fails: true},
		{name: "a number with no digits in it", raw: "1.2.3", fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got Number
			err := got.UnmarshalJSON([]byte(tc.raw))
			if tc.fails {
				if err == nil {
					t.Fatalf("%q decoded to %+v", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("%q decoded to %+v, want %+v", tc.raw, got, tc.want)
			}
			// And back out again as the same token.
			if tc.raw != "null" {
				out, err := json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				exponent := tc.raw == "1e2" && string(out) == "100.0"
				if string(out) != tc.raw && !exponent {
					t.Fatalf("%q came back as %q", tc.raw, out)
				}
			}
		})
	}
}

func TestReprStringEscapesWhatAConsoleCannotShow(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Fixture Forest":  `'Fixture Forest'`,
		"one\ttwo":        `'one\ttwo'`,
		"\u00c6ther":      "'\u00c6ther'",
		"a\u200bb":        `'a\u200bb'`,
		"a\U000E0001b":    `'a\U000e0001b'`,
		"it's":            `"it's"`,
		"one\x1btwo":      `'one\x1btwo'`,
		"latin-1 \u00ad!": `'latin-1 \xad!'`,
	}
	for in, want := range cases {
		if got := ReprString(in); got != want {
			t.Fatalf("ReprString(%q) = %s, want %s", in, got, want)
		}
	}
}
