package claude

import (
	"reflect"
	"testing"
)

// The index terms an answer comes back with, held to the vocabulary the app
// can actually file a deck by.
//
// **The mode is asked for words rather than for a list to choose from**, and
// that is the decision this test guards the consequence of. Handing a model 43
// options would have it reaching for the nearest one instead of saying it does
// not know; the cost is that a word can come back that nothing here can file,
// and a theme the vocabulary does not hold is not a label — it is an
// `unknown-theme` warning on the gate and a filter that finds nothing.
//
// The vocabulary itself is read from `internal/reference`, never listed here.
// A test carrying its own copy of the 43 words would pass on the day somebody
// adds the forty-fourth and be wrong about the thing it exists to check.
func TestTheThemesAnAnswerOffersAreHeldToTheVocabulary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		what          string
		offered       []string
		kept, dropped []string
	}{{
		what:    "words the vocabulary holds come through, lowercased",
		offered: []string{"Angels", "tokens"},
		kept:    []string{"angels", "tokens"},
		dropped: []string{},
	}, {
		what: "a word nobody can file is dropped and counted, never written",
		// `voltron` is a real thing to say about a deck and may or may not be
		// in the vocabulary on any given day; `sparklecrunch` is nobody's
		// theme, which is what makes it a safe fixture.
		offered: []string{"angels", "sparklecrunch"},
		kept:    []string{"angels"},
		dropped: []string{"sparklecrunch"},
	}, {
		what:    "the same word twice is one label",
		offered: []string{"angels", "Angels", " angels "},
		kept:    []string{"angels"},
		dropped: []string{},
	}, {
		what:    "blanks are neither kept nor counted as a loss",
		offered: []string{"", "   ", "angels"},
		kept:    []string{"angels"},
		dropped: []string{},
	}, {
		what:    "nothing offered is nothing kept, and both are lists",
		offered: nil,
		kept:    []string{},
		dropped: []string{},
	}}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			kept, dropped := keptThemes(tc.offered)
			if !reflect.DeepEqual(kept, tc.kept) {
				t.Errorf("kept %v, want %v", kept, tc.kept)
			}
			if !reflect.DeepEqual(dropped, tc.dropped) {
				t.Errorf("dropped %v, want %v", dropped, tc.dropped)
			}
		})
	}
}

// Both lists reach the wire as lists. A client maps them, and `null` where a
// list belongs is a crash on a label line -- which is exactly how the deck page
// went down over `themes` once before.
func TestTheDescriptionReportCarriesBothThemeListsAsLists(t *testing.T) {
	t.Parallel()
	got := DescriptionFor("d", Description{Strategy: "It attacks."},
		IntakeOutcome{Asked: true})
	if got.Themes == nil || got.ThemesDropped == nil {
		t.Fatalf("a theme list came back nil: %+v", got)
	}
	if got.AnsweredBy != "claude" {
		t.Errorf("answered_by is %q -- ADR 14's third boundary is a field", got.AnsweredBy)
	}
}
