package api

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// Which of a card instance's keywords are its printing's, and which are
// something else's doing.
//
// **This is the half of the question only this layer can answer.** The scribe
// sends the live set off Forge's view — honest, complete, and silent about
// where any of it came from, because `KeywordView` carries no source and not
// even Forge's own `isIntrinsic` flag. Scryfall's list for the printing lives
// here, next to the paintings. Subtracting one from the other is the whole
// mechanism, and its one hard part is that the two lists speak different
// dialects of the same language.

func liveChange(id int, live ...string) tier3.BoardChange {
	set := append([]string(nil), live...)
	return tier3.BoardChange{ID: id, Live: &set}
}

// A keyword the printing does not carry is granted; one it does is not.
//
// Kaheera beside a Beast is the case that asked for this (Aaron, 2026-08-26).
// Kaheera prints vigilance and the Beast does not, so the same word is a
// granted mark on one card and plain card text on the other — which is exactly
// the distinction a badge has to get right or it is worse than no badge.
func TestAGrantedKeywordIsTheOneThePrintingDoesNotCarry(t *testing.T) {
	t.Parallel()
	steps := []tier3.BoardStep{{Changes: []tier3.BoardChange{
		liveChange(120, "Vigilance"), // Leatherback Baloth: granted
		liveChange(100, "Vigilance"), // Kaheera: printed
	}}}
	printed := map[int][]string{
		120: {},
		100: {"Companion", "Vigilance"},
	}
	got := grantKeywords(steps, printed)
	granted := map[int][]string{}
	for _, change := range got[0].Changes {
		granted[change.ID] = *change.Granted
	}
	if len(granted[120]) != 1 || granted[120][0] != "Vigilance" {
		t.Errorf("the Beast's granted keywords are %v, want [Vigilance] -- "+
			"nothing in its printing gives it vigilance", granted[120])
	}
	if len(granted[100]) != 0 {
		t.Errorf("Kaheera's granted keywords are %v, want none -- it prints "+
			"vigilance, and a badge saying something gave it to her is a lie "+
			"about a card the player can read", granted[100])
	}
}

// The two lists spell the same keyword differently, and a naive compare lies.
//
// Forge writes a keyword as its card scripts do — `Ward:2`, `Protection from
// red` — and Scryfall writes the bare keyword. Compared as plain strings, a
// card that *prints* Ward 2 has it reported as granted. The match is on the
// oracle name being a prefix at a **word boundary**, so `ward` accounts for
// `ward:2` and not for `warden`.
func TestAPrintedKeywordIsRecognisedThroughForgesOwnSpelling(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		name    string
		live    string
		oracle  []string
		granted bool
	}{
		{"a parameterised keyword", "Ward:2", []string{"Ward"}, false},
		{"a keyword with a qualifier", "Protection from red",
			[]string{"Protection"}, false},
		{"the plain case", "Flying", []string{"Flying"}, false},
		{"a difference of case only", "flying", []string{"Flying"}, false},
		{"a keyword nothing printed", "Vigilance", []string{"Flying"}, true},
		{"a longer word that merely starts the same", "Warden",
			[]string{"Ward"}, true},
		{"a card that prints nothing at all", "Trample", nil, true},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			got := beyondPrinting([]string{one.live}, one.oracle)
			if (len(got) > 0) != one.granted {
				t.Fatalf("%q against a printing of %v came back as %v; "+
					"granted should be %v", one.live, one.oracle, got,
					one.granted)
			}
		})
	}
}

// A card that has lost the last keyword something gave it says so.
//
// The empty set is a real answer, for the same reason
// [tier3.BoardChange.Counters] is a pointer: absent means nothing changed, and
// empty means there is nothing left. Sent as neither, a creature keeps wearing
// a vigilance no longer granting it.
func TestACardWithNothingGrantedStillPublishesTheEmptyAnswer(t *testing.T) {
	t.Parallel()
	steps := []tier3.BoardStep{{Changes: []tier3.BoardChange{
		liveChange(120), // every grant gone
	}}}
	got := grantKeywords(steps, map[int][]string{120: {}})
	granted := got[0].Changes[0].Granted
	if granted == nil {
		t.Fatal("a card with no granted keywords sent nothing at all; " +
			"absent and empty are different facts here")
	}
	if len(*granted) != 0 {
		t.Fatalf("granted is %v, want the empty set", *granted)
	}
}

// The reel is not written through.
//
// It belongs to the stored job, and a room re-reading a match must not find it
// edited underneath — the same board is shaped again on every poll.
func TestShapingTheBoardLeavesTheStoredReelAlone(t *testing.T) {
	t.Parallel()
	steps := []tier3.BoardStep{{Changes: []tier3.BoardChange{
		liveChange(120, "Vigilance"),
	}}}
	_ = grantKeywords(steps, map[int][]string{120: {}})
	if steps[0].Changes[0].Granted != nil {
		t.Fatal("shaping the board wrote a granted set back into the reel " +
			"it was handed")
	}
}

// A board nobody reported keywords for is handed back untouched.
//
// Every match played by a worker built before the scribe read keywords is this
// case, and it must cost nothing rather than rebuilding a hundred and forty
// steps to change none of them.
func TestABoardWithNoKeywordsIsNotRebuilt(t *testing.T) {
	t.Parallel()
	steps := []tier3.BoardStep{{Changes: []tier3.BoardChange{{ID: 120}}}}
	got := grantKeywords(steps, map[int][]string{})
	if &got[0] != &steps[0] {
		t.Error("a board carrying no live keywords was copied anyway")
	}
}
