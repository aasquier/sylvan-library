package brew

// The four ways the cauldron refuses to be filled.
//
// None of these is reachable by anything a visitor does: the file is
// committed, embedded and tested. They stand between a damaged embed and a
// pot that fails four screens into somebody's conversation -- an empty slot
// is `RandBelow(0)`, which panics on the visitor's own request rather than at
// start. Holding them here is what keeps that argument true; a guard nothing
// has ever fired is otherwise only a promise.

import (
	"strings"
	"testing"
)

// refusal runs the loader over a document and returns what it panicked with.
func refusal(t *testing.T, what, doc string) string {
	t.Helper()
	var said string
	func() {
		defer func() {
			if r := recover(); r != nil {
				said, _ = r.(string)
			}
		}()
		loadCauldron([]byte(doc))
	}()
	if said == "" {
		t.Fatalf("%s was loaded without complaint", what)
	}
	return said
}

func TestADamagedCauldronRefusesAtStartRatherThanMidConversation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		doc  string
		says string
	}{
		{
			name: "half a document",
			doc:  `{"order": [`,
			says: "the embedded cauldron is unreadable",
		},
		{
			name: "the same ingredient twice",
			doc: `{"order": [{"slot": "engine", "name": "The Engine"}],
			       "ingredients": [{"key": "newt", "category": "engine"},
			                       {"key": "newt", "category": "engine"}]}`,
			says: `names "newt" twice`,
		},
		{
			name: "a place with nothing to put in it",
			doc: `{"order": [{"slot": "engine", "name": "The Engine"},
			                 {"slot": "trouble", "name": "The Trouble"}],
			       "ingredients": [{"key": "newt", "category": "engine"}]}`,
			says: `nothing in the catalogue is filed under "trouble"`,
		},
		{
			name: "an ingredient with nowhere to go",
			doc: `{"order": [{"slot": "engine", "name": "The Engine"}],
			       "ingredients": [{"key": "newt", "category": "engine"},
			                       {"key": "frog", "category": "weather"}]}`,
			says: `"frog" is filed under "weather", which is not a place`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := refusal(t, tc.name, tc.doc)
			if !strings.Contains(got, tc.says) {
				t.Fatalf("refused with %q, which does not say %q", got, tc.says)
			}
		})
	}
}

func TestTheCommittedCauldronLoadsAndFilesEveryIngredient(t *testing.T) {
	t.Parallel()
	// The sweep above would pass just as happily against a loader that
	// refused everything, so the committed file is loaded here too -- and
	// the indexes are checked to be the catalogue seen two ways rather than
	// two lists that happen to be the same length.
	catalogue, order, byCategory, byKey := loadCauldron(cauldronJSON)
	if len(order) < 3 {
		t.Fatalf("the pot has %d places", len(order))
	}
	if len(catalogue) != len(byKey) {
		t.Fatalf("%d ingredients indexed as %d keys", len(catalogue), len(byKey))
	}
	filed := 0
	for _, place := range order {
		if len(byCategory[place.Slot]) == 0 {
			t.Fatalf("nothing is filed under %q", place.Slot)
		}
		filed += len(byCategory[place.Slot])
	}
	if filed != len(catalogue) {
		t.Fatalf("%d of %d ingredients are filed under a place", filed, len(catalogue))
	}
	for _, in := range catalogue {
		if byKey[in.Key].Name != in.Name {
			t.Fatalf("%q indexes to %q", in.Key, byKey[in.Key].Name)
		}
	}
}
