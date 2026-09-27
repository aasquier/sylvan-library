package suggest_test

// The shortlist's shape: what it does with a card that has nothing to say,
// and how it cuts.
//
// The order is the whole product here -- somebody is looking at five rows and
// reading the top one as the answer -- so the two things that decide it are
// worth pinning separately from the arithmetic the corpus next door records.
// A tie broken by anything but the name is a shortlist that reorders itself
// between two runs over the same pool, which reads as the tool changing its
// mind.

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
)

// card is a synthetic record: everything the scorer reads held equal but the
// name and the text, so a difference in the answer can only be one of those.
func card(name, text string) *pool.CardRecord {
	return &pool.CardRecord{Name: name, TypeLine: "Creature — Bear", CMC: 3, OracleText: text}
}

func TestACardWithNoTextToCompareScoresNothingForText(t *testing.T) {
	t.Parallel()
	// A vanilla creature -- or a card the pool holds with its oracle text
	// empty -- has no words to share, so the text component is zero rather
	// than a division by the size of an empty set.
	silent := card("Fixture Vanilla", "")
	wordy := card("Fixture Chatterbox",
		"When this creature enters, draw a card and gain two life.")

	quiet := suggest.Score(silent, wordy, "")
	for _, r := range quiet.Reasons {
		if len(r) > 5 && r[:5] == "text:" {
			t.Fatalf("a target with no text reported %q", r)
		}
	}
	if quiet.Score < 0 {
		t.Fatalf("score %v", quiet.Score)
	}
	// The other way round is the same question and the same answer: shared
	// words need words on both sides.
	if loud := suggest.Score(wordy, silent, ""); loud.Score < 0 {
		t.Fatalf("score %v", loud.Score)
	}
}

func TestTheShortlistBreaksTiesByNameAndCutsAtTheLimit(t *testing.T) {
	t.Parallel()
	// Four candidates identical in everything the scorer reads, handed in
	// out of alphabetical order. They tie, so the only thing left to order
	// them is the name -- and the cut takes the first two of that order,
	// not the first two as they arrived.
	target := card("Fixture Target", "Draw a card.")
	same := func(name string) *pool.CardRecord {
		return card(name, "Draw a card.")
	}
	candidates := []*pool.CardRecord{
		same("Fixture Delta"), same("Fixture Bravo"),
		same("Fixture Charlie"), same("Fixture Alpha"),
	}

	all := suggest.Rank(target, candidates, "", 10, nil)
	if len(all) != 4 {
		t.Fatalf("%d candidates ranked", len(all))
	}
	want := []string{"Fixture Alpha", "Fixture Bravo", "Fixture Charlie", "Fixture Delta"}
	for i, name := range want {
		if all[i].Name() != name {
			t.Fatalf("position %d is %q, want %q", i, all[i].Name(), name)
		}
		if all[i].Score != all[0].Score {
			t.Fatalf("%q scored %v against %v -- these were meant to tie",
				all[i].Name(), all[i].Score, all[0].Score)
		}
	}

	cut := suggest.Rank(target, candidates, "", 2, nil)
	if len(cut) != 2 || cut[0].Name() != "Fixture Alpha" || cut[1].Name() != "Fixture Bravo" {
		t.Fatalf("the cut is %+v", names(cut))
	}
	// The card being replaced is never its own replacement, and neither is
	// anything the caller excluded.
	none := suggest.Rank(target, []*pool.CardRecord{target}, "", 5, nil)
	if len(none) != 0 {
		t.Fatalf("the target suggested itself: %+v", names(none))
	}
	filtered := suggest.Rank(target, candidates, "", 5,
		map[string]bool{"fixture alpha": true, "Fixture Bravo": true})
	if len(filtered) != 2 || filtered[0].Name() != "Fixture Charlie" {
		t.Fatalf("the exclusions left %+v", names(filtered))
	}
}

func names(cs []suggest.Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name()
	}
	return out
}
