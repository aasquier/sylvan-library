package deckedit

import (
	"strings"
	"testing"
)

// The oracle's diagnostics: what a failed verification actually says.
//
// Every surgical edit (ADR 12) checks its own text surgery against a parsed
// document that says what the result *ought* to have been, and refuses if
// they disagree. That refusal is a sentence somebody reads at the moment
// their deck file did not change the way they asked -- so the sentence has
// to name where the two disagreed and what it found there.
//
// **None of this code runs on a healthy edit**, which is why it was
// untested and why testing it matters: a bug in the diagnosis is invisible
// until the day something else is already wrong, and then it costs an hour
// of looking in the wrong place. Both halves are pinned here -- the walk that
// finds the first disagreement, and the recorded spelling values render in.

// The walk names the deepest path where the two documents part company, not
// the top of the tree -- "cards[3].name" rather than "the document".
func TestTheDiagnosisNamesWhereTheDocumentsPartCompany(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name             string
		expected, actual any
		wants            []string
	}{
		{
			name:     "a scalar at the top",
			expected: map[string]any{"name": "Gyome"},
			actual:   map[string]any{"name": "Trostani"},
			wants:    []string{".name", "'Trostani'", "'Gyome'"},
		},
		{
			name:     "a key that appeared",
			expected: map[string]any{"name": "Gyome"},
			actual:   map[string]any{"name": "Gyome", "stray": 1},
			wants:    []string{"stray", "appeared"},
		},
		{
			name:     "a key that disappeared",
			expected: map[string]any{"name": "Gyome", "status": "draft"},
			actual:   map[string]any{"name": "Gyome"},
			wants:    []string{"status", "disappeared"},
		},
		{
			name: "a card deep in the list",
			expected: map[string]any{"cards": []any{
				map[string]any{"name": "Sol Ring"},
				map[string]any{"name": "Forest"},
			}},
			actual: map[string]any{"cards": []any{
				map[string]any{"name": "Sol Ring"},
				map[string]any{"name": "Island"},
			}},
			wants: []string{"cards[1].name", "'Island'", "'Forest'"},
		},
		{
			name: "a list that lost an entry",
			expected: map[string]any{"cards": []any{
				map[string]any{"name": "Sol Ring"},
				map[string]any{"name": "Forest"},
			}},
			actual: map[string]any{"cards": []any{map[string]any{"name": "Sol Ring"}}},
			wants:  []string{"cards", "1 entries", "expected 2"},
		},
		{
			name:     "a type that changed under it",
			expected: map[string]any{"cards": []any{}},
			actual:   map[string]any{"cards": "not a list"},
			wants:    []string{".cards", "'not a list'"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := firstDifference(tc.expected, tc.actual, "")
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Errorf("the diagnosis is %q, want something containing %q", got, want)
				}
			}
		})
	}
}

// Two documents that agree have no first difference to name, and the walk
// must not invent one -- it falls through to a comparison of the whole
// document, which is the honest answer for "these are somehow different but
// every part matched".
func TestTheDiagnosisOfIdenticalDocumentsFallsThroughRatherThanLying(t *testing.T) {
	t.Parallel()
	same := map[string]any{"name": "Gyome", "cards": []any{
		map[string]any{"name": "Sol Ring"}}}
	got := firstDifference(same, copyDoc(same), "")
	// It says something about the document rather than pointing at a key
	// that is fine.
	if !strings.Contains(got, "the document") {
		t.Errorf("identical documents diagnosed as %q", got)
	}
}

// A bare list at the root has no key to name, so it says "the list" rather
// than leaving a hole where the path would go.
func TestAnUnnamedListSaysWhatItIs(t *testing.T) {
	t.Parallel()
	got := firstDifference([]any{1, 2}, []any{1}, "")
	if !strings.Contains(got, "the list") {
		t.Errorf("a bare list diagnosed as %q", got)
	}
	if strings.HasPrefix(got, " ") {
		t.Errorf("the diagnosis begins with the hole where the path was: %q", got)
	}
}

// The union is sorted, so the diagnosis is the same sentence every time --
// a Go map's iteration order would otherwise name a different key per run
// and make a refusal impossible to reproduce.
func TestTheKeyUnionIsSortedAndComplete(t *testing.T) {
	t.Parallel()
	a := map[string]any{"beta": 1, "alpha": 2}
	b := map[string]any{"gamma": 3, "alpha": 4}

	got := unionOfKeys(a, b)
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("the union is %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the union is %v, want %v", got, want)
		}
	}

	// Ten runs, one answer: the sentence has to be reproducible.
	for i := 0; i < 10; i++ {
		again := unionOfKeys(a, b)
		for j := range again {
			if again[j] != want[j] {
				t.Fatalf("run %d gave %v", i, again)
			}
		}
	}

	if got := unionOfKeys(nil, nil); len(got) != 0 {
		t.Errorf("two empty maps unioned to %v", got)
	}
	if got := unionOfKeys(a, nil); len(got) != 2 {
		t.Errorf("a map unioned with nothing gave %v", got)
	}
}

// The recorded refusal spelling. A card name is the common case and the one
// that matters: `'Sol Ring'`, not `Sol Ring`, so the quotes show where the
// name begins and ends.
func TestValuesRenderInTheRecordedRefusalSpelling(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"nothing", nil, "None"},
		{"true", true, "True"},
		{"false", false, "False"},
		{"a card name", "Sol Ring", "'Sol Ring'"},
		{"the empty string", "", "''"},
		{"an int", 7, "7"},
		{"an int64", int64(7), "7"},
		// A name with an apostrophe switches quotes rather than escaping,
		// so `Gaea's Cradle` reads as a name rather than as an escape
		// sequence.
		{"an apostrophe", "Gaea's Cradle", `"Gaea's Cradle"`},
		// Unless it already has double quotes, in which case escaping is
		// the only option left.
		{"both quotes", `He said "Gaea's"`, `'He said "Gaea\'s"'`},
		{"a list", []any{"a", 1, nil}, "['a', 1, None]"},
		{"an empty list", []any{}, "[]"},
		{"a mapping", map[string]any{"b": 2, "a": 1}, "{'a': 1, 'b': 2}"},
		{"an empty mapping", map[string]any{}, "{}"},
		{"a nested shape", map[string]any{"cards": []any{
			map[string]any{"name": "Sol Ring"}}},
			"{'cards': [{'name': 'Sol Ring'}]}"},
		// Anything else falls through to Go's own rendering rather than
		// vanishing.
		{"a float", 1.5, "1.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := quotedValue(tc.in); got != tc.want {
				t.Errorf("quotedValue(%#v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A mapping's keys are sorted before rendering, for the same reason the
// union is: a refusal that names its keys in a different order every run
// cannot be reported, compared or grepped for.
func TestARenderedMappingIsInSortedKeyOrder(t *testing.T) {
	t.Parallel()
	m := map[string]any{"zebra": 1, "alpha": 2, "mango": 3}
	first := quotedValue(m)
	for i := 0; i < 10; i++ {
		if got := quotedValue(m); got != first {
			t.Fatalf("run %d rendered %q, first run gave %q", i, got, first)
		}
	}
	if first != "{'alpha': 2, 'mango': 3, 'zebra': 1}" {
		t.Errorf("rendered %q", first)
	}

	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "alpha" || keys[2] != "zebra" {
		t.Errorf("sortedKeys gave %v", keys)
	}
	if got := sortedKeys(map[string]any{}); len(got) != 0 {
		t.Errorf("an empty map has keys %v", got)
	}
}

// `reflect.DeepEqual` would nearly do and is deliberately not used: it
// separates `[]any(nil)` from `[]any{}`, which the two sides of a
// verification reach by different routes. A deck with its last swap-board
// card removed would otherwise fail verification for a difference no reader
// could see.
func TestAnEmptiedListEqualsAnAbsentOne(t *testing.T) {
	t.Parallel()
	fromParser := map[string]any{"swap_board": []any(nil)}
	fromAnEdit := map[string]any{"swap_board": []any{}}
	if !equalDocs(fromParser, fromAnEdit) {
		t.Error("an emptied swap board does not equal an absent one -- " +
			"removing the last card would fail verification")
	}

	// The comparison still says no to everything that really is different.
	for _, tc := range []struct {
		name string
		a, b any
	}{
		{"different lengths", []any{1}, []any{1, 2}},
		{"different values", []any{1}, []any{2}},
		{"different key counts", map[string]any{"a": 1}, map[string]any{"a": 1, "b": 2}},
		{"a missing key", map[string]any{"a": 1}, map[string]any{"b": 1}},
		{"different types", map[string]any{"a": 1}, []any{1}},
		{"a list against a scalar", []any{1}, 1},
		{"different scalars", "a", "b"},
	} {
		if equalDocs(tc.a, tc.b) {
			t.Errorf("%s: %#v equalled %#v", tc.name, tc.a, tc.b)
		}
	}

	// And yes to what is the same.
	if !equalDocs(nil, nil) || !equalDocs("a", "a") || !equalDocs(1, 1) {
		t.Error("equal scalars compared unequal")
	}
}

// The deep copy is what lets an operation mutate its expectation without
// touching the document it was made from.
func TestTheDeepCopyDoesNotAliasTheOriginal(t *testing.T) {
	t.Parallel()
	original := map[string]any{
		"name":  "Gyome",
		"cards": []any{map[string]any{"name": "Sol Ring", "tags": []any{"ramp"}}},
	}
	copied := copyDoc(original)
	if !equalDocs(original, copied) {
		t.Fatal("the copy does not equal the original")
	}

	cards, _ := copied["cards"].([]any)
	card, _ := cards[0].(map[string]any)
	card["name"] = "changed"
	tags, _ := card["tags"].([]any)
	tags[0] = "changed"
	copied["name"] = "changed"

	origCards, _ := original["cards"].([]any)
	origCard, _ := origCards[0].(map[string]any)
	origTags, _ := origCard["tags"].([]any)
	if original["name"] != "Gyome" || origCard["name"] != "Sol Ring" || origTags[0] != "ramp" {
		t.Errorf("the copy aliases the original: %#v", original)
	}
}

// cardAt is how every operation reads one entry, and it has to answer nil
// rather than panicking for an index nobody has and an entry that is not a
// mapping -- a hand-written deck can contain either.
func TestCardAtRefusesToPanicOnAHandWrittenFile(t *testing.T) {
	t.Parallel()
	items := []any{
		map[string]any{"name": "Sol Ring"},
		"a bare string where a card should be",
	}
	if got := cardAt(items, 0); got == nil || got["name"] != "Sol Ring" {
		t.Errorf("the first card read as %v", got)
	}
	for _, i := range []int{-1, 1, 2, 99} {
		if got := cardAt(items, i); got != nil {
			t.Errorf("index %d read as %v", i, got)
		}
	}
	if got := cardAt(nil, 0); got != nil {
		t.Errorf("an empty list read as %v", got)
	}
}

// The name comparison every duplicate check uses: case-folded and trimmed,
// exactly as the operations locate a card, so "find" and "refuse as a
// duplicate" can never disagree.
func TestNameMatchingFoldsAndTrimsTheSameWayLocatingDoes(t *testing.T) {
	t.Parallel()
	card := map[string]any{"name": " Sol Ring "}
	for _, name := range []string{"Sol Ring", "sol ring", "SOL RING", "  Sol Ring  "} {
		if !nameMatches(card, name) {
			t.Errorf("%q did not match", name)
		}
	}
	for _, name := range []string{"Sol", "Sol  Ring", "Solring", ""} {
		if nameMatches(card, name) {
			t.Errorf("%q matched", name)
		}
	}
	// A card with no name at all matches only the empty string, and never
	// a real one.
	if nameMatches(map[string]any{}, "Sol Ring") {
		t.Error("a nameless card matched a real name")
	}
}

// listOf treats absent and null as empty, which is what lets an operation
// append to a section a hand-written file never wrote.
func TestListOfTreatsAbsentAndNullAsEmpty(t *testing.T) {
	t.Parallel()
	for _, doc := range []map[string]any{
		{},
		{"cards": nil},
		{"cards": "not a list"},
	} {
		if got := listOf(doc, "cards"); len(got) != 0 {
			t.Errorf("%#v read as %v", doc, got)
		}
	}
	if got := listOf(map[string]any{"cards": []any{1, 2}}, "cards"); len(got) != 2 {
		t.Errorf("a real list read as %v", got)
	}
}
