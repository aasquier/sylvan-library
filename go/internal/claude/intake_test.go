package claude

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The chunking, which decides how much one bad answer costs.
func TestAnIntakeIsChunkedSoOneBadAnswerCostsItsOwnCards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   []string
		size int
		want [][]string
		why  string
	}{
		{
			name: "nothing to do makes no calls", in: nil, size: 3, want: [][]string{},
			why: "a deck whose cards all have rationales must not spend a call saying so",
		},
		{
			name: "an exact multiple does not leave an empty chunk",
			in:   []string{"a", "b", "c", "d"}, size: 2,
			want: [][]string{{"a", "b"}, {"c", "d"}},
			why:  "an empty final chunk is a call with no cards in it",
		},
		{
			name: "a remainder rides in a short chunk",
			in:   []string{"a", "b", "c"}, size: 2,
			want: [][]string{{"a", "b"}, {"c"}},
			why:  "the tail is asked about, not dropped",
		},
		{
			name: "fewer cards than the chunk is one call",
			in:   []string{"a"}, size: 20,
			want: [][]string{{"a"}},
			why:  "the common case for a deck that arrived with its own reasons",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := chunked(tc.in, tc.size); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("chunked(%v, %d) = %v, wanted %v (%s)",
					tc.in, tc.size, got, tc.want, tc.why)
			}
		})
	}
}

// A name that came back is matched against the ask under the fold, and handed
// on in the DECK's spelling.
//
// Both halves matter and they fail differently. A card nobody asked about is
// a write to a card the person did not put in front of the model, and it must
// never happen. A card whose casing came back changed is a write that simply
// misses -- `deckedit` locates by name -- so the answer is silently lost, and
// a silently lost draft is the kind of thing that gets reported as "it only
// did some of them".
func TestAnAnswerIsMatchedToTheAskAndKeepsTheDecksSpelling(t *testing.T) {
	t.Parallel()
	asked := foldedSet([]string{"Sol Ring", "Arcane Signet", "Kongming, \"Sleeping Dragon\""})

	if _, held := asked[Casefold("sol ring")]; !held {
		t.Error("a name that came back lowercased did not match the ask")
	}
	if proper := asked[Casefold("SOL RING")]; proper != "Sol Ring" {
		t.Errorf("matched to %q; the deck's own spelling is what the write looks for", proper)
	}
	if _, held := asked[Casefold("Rhystic Study")]; held {
		t.Error("a card nobody asked about matched the ask, so a model volunteering " +
			"an extra card would have it written into somebody's deck")
	}
	if proper := asked[Casefold("kongming, \"sleeping dragon\"")]; proper == "" {
		t.Error("a card name with quotes in it did not survive the fold")
	}
}

// The facts the opening carries are the deck's identity, and the rationales it
// quotes back are the OWNER's -- never ones a previous intake drafted.
//
// Quoting a draft back as an example of how the owner writes is a model
// learning its own register from itself, which is how a second run drifts
// further from the person than the first one did.
func TestTheOpeningQuotesTheOwnersRationalesAndNotItsOwn(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{
		Slug: "gyome-food", Name: "Gyome — Food", Stage: "draft", Status: "built",
		Commander: []string{"Gyome, Master Chef"},
		Themes:    []string{"food", "aristocrats"},
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Category: "ramp", Why: "the fastest rock there is"},
			{Name: "Cultivate", Category: "ramp", Why: "drafted earlier", WhyBy: "claude"},
			{Name: "Blank", Category: "utility"},
		},
	}
	opening := draftOpening(d, []string{"Blank"})

	for _, want := range []string{"Gyome — Food", "Gyome, Master Chef", "food, aristocrats",
		"the fastest rock there is", "- Blank"} {
		if !strings.Contains(opening, want) {
			t.Errorf("the opening does not carry %q:\n%s", want, opening)
		}
	}
	if strings.Contains(opening, "drafted earlier") {
		t.Error("the opening quotes a rationale a previous intake drafted back as an " +
			"example of how the owner writes, so each run learns its register from " +
			"itself and drifts further from the person")
	}
}

// The dial must not answer `off` for a surface that is about to run.
//
// This is the bug `dialSurfaces` exists for and the third time it has been
// found the same way -- by loading the page, not by reading the code. The
// import screen has no deck by construction, so a default derived from a deck
// is a default derived from nothing, and `off` stands the whole sheet down on
// the one page it belongs to.
func TestTheIntakeSurfaceDoesNotAnswerOffWithNoDeck(t *testing.T) {
	t.Parallel()
	got, err := IntakeStanceFor(nil, nil)
	if err != nil {
		t.Fatalf("the intake surface has no default: %v", err)
	}
	if !got.AllowsCalls() {
		t.Errorf("the intake's own default is %+v, which makes no call -- so the "+
			"sheet stands down for everybody on the import screen", got)
	}
	// And it does NOT come with a write: ADR 41's second gate is a decision
	// the user makes on the stance dial, and a surface default that handed it
	// over would be the gate opening itself.
	if got.MayWrite() {
		t.Errorf("the intake's default may write (%+v), so ADR 41's second gate "+
			"is satisfied by the surface rather than by the user", got)
	}
	// A stance the caller asked for still wins, clamped.
	asked, err := IntakeStanceFor("off", nil)
	if err != nil {
		t.Fatalf("an explicit stance was refused: %v", err)
	}
	if asked.AllowsCalls() {
		t.Errorf("an explicit `off` was overridden by the surface default: %+v", asked)
	}
}
