package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckimport"
	"github.com/aasquier/sylvan-library/go/internal/decklist"
)

// The body coercions the collection routes share.
//
// These look like plumbing and are not: they are the recorded readings of
// what a browser sends, and each one differs from what a naive Go cast would
// do in a way that a caller depends on. `{"shared": 0}` is **false** and
// `{"shared": "no"}` is **true** -- the second is not what anybody would
// design, and it is what the routes have always done, so a "cleanup" that
// made it sane would silently unshare somebody's deck.
//
// The other reason they are worth pinning: every one of them has a fallback
// branch, and a fallback that quietly returned a zero where the caller meant
// something would be a create that succeeded with the wrong bracket rather
// than a refusal anybody sees.

// The bracket is optional, and every shape a browser can put in the field is
// either a number or a refusal that names it.
func TestTheBracketReadsEveryShapeABrowserSends(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body map[string]any
		want *int
	}{
		{"absent", map[string]any{}, nil},
		{"null", map[string]any{"bracket": nil}, nil},
		{"a cleared input", map[string]any{"bracket": ""}, nil},
		{"a number", map[string]any{"bracket": json.Number("3")}, intPtr(3)},
		{"a string from an input", map[string]any{"bracket": "3"}, intPtr(3)},
		// A fraction truncates rather than refusing -- the recorded
		// coercion for a JSON number arriving as a float.
		{"a fraction", map[string]any{"bracket": json.Number("3.9")}, intPtr(3)},
		// Nobody sends a boolean; leaving it to the default branch would
		// refuse where the recorded coercion accepts.
		{"true", map[string]any{"bracket": true}, intPtr(1)},
		{"false", map[string]any{"bracket": false}, intPtr(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := bodyBracket(tc.body)
			if err != nil {
				t.Fatalf("%v", err)
			}
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("read %d, want none", *got)
			case tc.want != nil && got == nil:
				t.Errorf("read none, want %d", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("read %d, want %d", *got, *tc.want)
			}
		})
	}
}

// A bracket that will not parse is refused rather than becoming zero, because
// bracket 0 is a real value and a create that silently took it would put the
// deck in the wrong bracket with nobody the wiser.
func TestABracketThatWillNotParseIsRefusedRatherThanZeroed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		body  map[string]any
		wants string
	}{
		{"words", map[string]any{"bracket": "three"}, "invalid literal for int()"},
		{"a list", map[string]any{"bracket": []any{3}}, "int() argument must be"},
		{"an object", map[string]any{"bracket": map[string]any{"n": 3}}, "int() argument must be"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := bodyBracket(tc.body)
			if err == nil {
				t.Fatalf("%v was accepted as %v", tc.body, got)
			}
			if got != nil {
				t.Errorf("a refusal also returned %d", *got)
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("the refusal said %q, want something containing %q", err, tc.wants)
			}
		})
	}
	// The refusal quotes the value, so the caller can see what it read.
	_, err := bodyBracket(map[string]any{"bracket": "three"})
	if !strings.Contains(err.Error(), "three") {
		t.Errorf("the refusal does not name the value: %q", err)
	}
}

// `commander` arrives as one string or a list of them, and every non-value
// reads as an empty list rather than as a one-element list holding nothing --
// a partner pair and a single commander travel through the same field.
func TestCommanderReadsAsOneNameOrAList(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body map[string]any
		want []string
	}{
		{"absent", map[string]any{}, nil},
		{"null", map[string]any{"commander": nil}, nil},
		{"a cleared input", map[string]any{"commander": ""}, nil},
		{"one name", map[string]any{"commander": "Gyome, Master Chef"}, []string{"Gyome, Master Chef"}},
		{"a partner pair", map[string]any{"commander": []any{"Thrasios", "Tymna"}},
			[]string{"Thrasios", "Tymna"}},
		{"an empty list", map[string]any{"commander": []any{}}, nil},
		{"the wrong type entirely", map[string]any{"commander": 7}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := bodyStrings(tc.body, "commander")
			// Never nil: the frontend and the gate both iterate it.
			if got == nil {
				t.Fatal("read as nil rather than as an empty list")
			}
			if len(got) != len(tc.want) {
				t.Fatalf("read %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("read %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// **The recorded truthiness**, which is not Go's and not a cast's: an empty
// string, a zero, an empty container and null are false, and everything else
// is true. `{"shared": "no"}` is TRUE. A cleanup that made this sane would
// unshare decks whose owners typed the word.
func TestTruthinessIsTheRecordedOneRatherThanTheObviousOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want bool
	}{
		{"null", nil, false},
		{"false", false, false},
		{"true", true, true},
		{"the empty string", "", false},
		{"zero", json.Number("0"), false},
		{"a zero float", json.Number("0.0"), false},
		{"an empty list", []any{}, false},
		{"an empty object", map[string]any{}, false},

		{"a non-empty string", "yes", true},
		// The one that surprises people, and the one a "fix" would break.
		{"the word no", "no", true},
		{"the word false", "false", true},
		{"a number", json.Number("1"), true},
		{"a negative number", json.Number("-1"), true},
		{"a list", []any{0}, true},
		{"an object", map[string]any{"a": 1}, true},
		// Anything the switch does not name is present, therefore true.
		{"an unexpected type", 7, true},
		{"a malformed number", json.Number("not-a-number"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := truthy(tc.in); got != tc.want {
				t.Errorf("truthy(%#v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// A companion renders as null rather than as an empty string, which is what
// the deck model holds and what every other route serialising one writes --
// a `""` on the wire would render as a companion nobody chose.
func TestACompanionRendersAsNullRatherThanAnEmptyString(t *testing.T) {
	t.Parallel()
	if got := companionOrNil(nil); got != nil {
		t.Errorf("no companion rendered as %#v", got)
	}
	blank := ""
	if got := companionOrNil(&blank); got != nil {
		t.Errorf("an empty companion rendered as %#v", got)
	}
	name := "Kaheera, the Orphanguard"
	if got := companionOrNil(&name); got != name {
		t.Errorf("a companion rendered as %#v", got)
	}
}

// Lists the frontend iterates are never null, because `null.map` is a blank
// page rather than an empty one.
func TestListsTheFrontendIteratesAreNeverNull(t *testing.T) {
	t.Parallel()
	if got := orEmpty[string](nil); got == nil || len(got) != 0 {
		t.Errorf("nothing rendered as %#v", got)
	}
	// Generic on purpose: the null that reached the browser was a
	// `[]deckimport.Correction`, and a helper that only knew about strings is
	// exactly why that field was the one nobody put through it.
	if got := orEmpty[deckimport.Correction](nil); got == nil || len(got) != 0 {
		t.Errorf("no corrections rendered as %#v", got)
	}
	if got := orEmpty([]string{}); got == nil || len(got) != 0 {
		t.Errorf("an empty list rendered as %#v", got)
	}
	got := orEmpty([]string{"a", "b"})
	if len(got) != 2 || got[0] != "a" {
		t.Errorf("a real list rendered as %#v", got)
	}

	if got := reportedLines(nil); got == nil || len(got) != 0 {
		t.Errorf("no unreadable lines rendered as %#v", got)
	}
	lines := reportedLines([]decklist.Line{{LineNo: 4, Text: "1x Nonsense"}})
	if len(lines) != 1 {
		t.Fatalf("rendered %v", lines)
	}
	// The line number and the text both travel: the importer's whole job
	// here is to say which line it could not read.
	if lines[0]["line"] != 4 {
		t.Errorf("the line number is %v", lines[0]["line"])
	}
	if lines[0]["text"] != "1x Nonsense" {
		t.Errorf("the text is %v", lines[0]["text"])
	}
}

// The colour identity is sorted, because it is rendered as a row of pips and
// an order that came from a Go map would shuffle between requests.
func TestTheColourIdentityIsSorted(t *testing.T) {
	t.Parallel()
	identity := map[string]bool{"G": true, "B": true, "W": true}
	first := sortedColors(identity)
	if len(first) != 3 || first[0] != "B" || first[1] != "G" || first[2] != "W" {
		t.Fatalf("sorted to %v", first)
	}
	for i := 0; i < 10; i++ {
		again := sortedColors(identity)
		for j := range again {
			if again[j] != first[j] {
				t.Fatalf("run %d gave %v, first gave %v", i, again, first)
			}
		}
	}
	if got := sortedColors(nil); got == nil || len(got) != 0 {
		t.Errorf("colourless rendered as %#v", got)
	}
}

func intPtr(n int) *int { return &n }
