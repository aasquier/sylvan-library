package deckyaml

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The normaliser, called directly.
//
// `orderedValue` has five cases and `ParseOrdered` can only ever hand it two
// of them: goccy answers `UseOrderedMap` with `yaml.MapSlice`, a sequence with
// `[]any` and a non-negative integer with `uint64`. The two map cases are the
// decoder's *other* answers -- what a build without the option, or a future
// version that stopped honouring it, would produce -- and the function's own
// comment says why they are kept rather than deleted: a decoder that changed
// its mind would otherwise drop every key order silently instead of loudly.
//
// A comment is not a test, and that is what these are. Each case is driven
// with the value the decoder would hand it and held to the answer the file's
// own rules require: every depth becomes a `Map`, a mapping with no order of
// its own is **sorted** so that what it yields is at least the same twice, and
// a key that is not a string is spelled rather than refused. Nothing here
// calls past a guard -- there is no guard, only a type switch whose arms had
// never been entered.
func TestTheNormaliserAnswersEveryShapeADecoderCanHandIt(t *testing.T) {
	t.Parallel()

	t.Run("a plain Go map is ordered by sorting it", func(t *testing.T) {
		t.Parallel()
		// Deliberately not in alphabetical order, and nested, so the answer
		// proves both halves: the sort and the recursion.
		got := orderedValue(map[string]any{
			"why":  "it ramps",
			"name": "Sol Ring",
			"deep": map[string]any{"two": 2, "one": 1},
		})
		want := Map{
			{Key: "deep", Value: Map{{Key: "one", Value: int64(1)}, {Key: "two", Value: int64(2)}}},
			{Key: "name", Value: "Sol Ring"},
			{Key: "why", Value: "it ramps"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("a plain map normalised wrong (-want +got):\n%s", diff)
		}
	})

	t.Run("a map with any keys spells them and sorts them", func(t *testing.T) {
		t.Parallel()
		// `2` and `true` are legal YAML keys and neither is a string. The
		// deck files have none, which is exactly why this is the arm nothing
		// had ever entered.
		got := orderedValue(map[any]any{
			"name": "Sol Ring",
			2:      []any{"a", map[string]any{"k": "v"}},
			true:   "yes",
		})
		want := Map{
			{Key: "2", Value: []any{"a", Map{{Key: "k", Value: "v"}}}},
			{Key: "name", Value: "Sol Ring"},
			{Key: "true", Value: "yes"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("a map with non-string keys normalised wrong (-want +got):\n%s", diff)
		}
	})

	t.Run("an integer too large for int64 is left as it is", func(t *testing.T) {
		t.Parallel()
		// The signed shape is the recorded one, so every integer that fits
		// becomes an int64; one that does not is handed back untouched,
		// because a silent wrap would be worse than an unusual type.
		big := uint64(math.MaxInt64) + 1
		if got := orderedValue(big); got != any(big) {
			t.Errorf("an unrepresentable integer came back as %#v, not as itself", got)
		}
		if got := orderedValue(uint64(math.MaxInt64)); got != any(int64(math.MaxInt64)) {
			t.Errorf("the largest representable integer came back as %#v", got)
		}
	})

	t.Run("a Go int becomes the recorded signed shape", func(t *testing.T) {
		t.Parallel()
		if got := orderedValue(7); got != any(int64(7)) {
			t.Errorf("an int came back as %#v, not as an int64", got)
		}
	})

	t.Run("a Map survives a round trip through Plain and JSON", func(t *testing.T) {
		t.Parallel()
		// The two map arms are only worth keeping if what they produce is a
		// Map the rest of the package can use, so the answer is taken on
		// through the two things anything here is ever asked for: the lookup
		// and the wire.
		ordered, ok := orderedValue(map[any]any{"b": 1, "a": "first"}).(Map)
		if !ok {
			t.Fatalf("a mapping did not normalise to a Map")
		}
		if value, found := ordered.Get("a"); !found || value != "first" {
			t.Errorf("Get(\"a\") answered %#v, %v", value, found)
		}
		if diff := cmp.Diff(map[string]any{"a": "first", "b": int64(1)}, ordered.Plain()); diff != "" {
			t.Errorf("Plain lost something (-want +got):\n%s", diff)
		}
	})
}

// sortedKeys is only reachable from the two map arms above, so it had never
// run either. Held to the one property the arms rest on: the answer is every
// key, in one order, whatever order the map handed them over in.
func TestTheKeysOfAnUnorderedMappingComeBackSorted(t *testing.T) {
	t.Parallel()
	got := sortedKeys(map[string]any{"why": 1, "name": 2, "Art": 3, "qty": 4})
	want := []string{"Art", "name", "qty", "why"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the keys came back wrong (-want +got):\n%s", diff)
	}
}
