package yamlemit

import (
	"strings"
	"testing"
)

// The document writer's own edges: the shapes a deck file reaches that a
// happy-path dump never does.
//
// This package is one of the determinism kernels (CLAUDE.md): the deck file's
// one YAML style, which the recorded goldens rest on. What that means here is
// that every one of these shapes has exactly one right rendering, and a
// second one is not an alternative -- it is a diff on somebody's hand-written
// deck the next time an edit touches it.
//
// The long-form key is the case worth naming. No deck *field* reaches it --
// the keys are `slug`, `name`, `cards` and their kin -- but a note's key
// comes from a URL segment (`PUT .../notes/{key}`), so a key too long or too
// strange to sit on one line is reachable by a person rather than only by a
// fuzzer.

// An empty collection is written in flow form, because a block with nothing
// under it is not a thing YAML can say.
func TestAnEmptyCollectionIsWrittenInFlowForm(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"an empty mapping", Map{}, "{}\n"},
		{"an empty list", List{}, "[]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Dump(tc.in, 80)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("dumped %q, want %q", got, tc.want)
			}
		})
	}

	// And nested inside a mapping, which is what a deck with no cards and no
	// swap board looks like.
	got, err := Dump(Map{
		{Key: "cards", Value: List{}},
		{Key: "notes", Value: Map{}},
	}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "cards: []") {
		t.Errorf("an empty card list dumped as:\n%s", got)
	}
	if !strings.Contains(got, "notes: {}") {
		t.Errorf("an empty note mapping dumped as:\n%s", got)
	}
}

// A key too long or too strange for one line is written the long way, with
// its own `?` and `:` lines -- reachable through a note's key, which comes
// from a URL segment.
func TestAKeyThatCannotSitOnOneLineIsWrittenTheLongWay(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a-very-long-note-key-", 12)

	got, err := Dump(Map{{Key: long, Value: "the note"}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "?") {
		t.Errorf("a long key was written on one line anyway:\n%s", got)
	}
	// The value gets its own line under the `:`.
	if !strings.Contains(got, ":") {
		t.Errorf("the long form lost its value indicator:\n%s", got)
	}
	if !strings.Contains(got, "the note") {
		t.Errorf("the value went missing:\n%s", got)
	}

	// A key with a newline in it cannot be a simple key either.
	got, err = Dump(Map{{Key: "two\nlines", Value: "v"}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "?") {
		t.Errorf("a multi-line key was written as a simple one:\n%s", got)
	}
}

// A block sequence's dashes sit under their key rather than hard against the
// margin -- the indentless form is never taken, which is the deck file's own
// style and the one every golden holds.
func TestASequencesDashesSitUnderTheirKey(t *testing.T) {
	t.Parallel()
	got, err := Dump(Map{
		{Key: "commander", Value: List{"Gyome"}},
		{Key: "cards", Value: List{
			Map{{Key: "name", Value: "Sol Ring"}, {Key: "why", Value: "ramp"}},
		}},
	}, 80)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"commander:\n  - Gyome", "cards:\n  - name: Sol Ring"} {
		if !strings.Contains(got, want) {
			t.Errorf("dumped:\n%s\nwant it to contain %q", got, want)
		}
	}
	// Never at the margin, which is the other legal YAML and the wrong one
	// here.
	if strings.Contains(got, "\n- ") {
		t.Errorf("a dash landed at the margin:\n%s", got)
	}
}

// A folded scalar that leaves the document open gets the explicit `...` that
// says the scalar is over -- without it, a reader cannot tell where the
// stream ended.
func TestAFoldedScalarThatEndsTheDocumentIsClosedExplicitly(t *testing.T) {
	t.Parallel()
	lines, err := Render("why", strings.Repeat("a long rationale that wraps ", 8),
		0, 60, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 2 {
		t.Fatalf("a folded rationale rendered as %v", lines)
	}
	if !strings.HasPrefix(lines[0], "why:") {
		t.Errorf("the first line is %q", lines[0])
	}
	// Folded, so the text is wrapped rather than on one very long line.
	for _, line := range lines {
		if len(line) > 80 {
			t.Errorf("a folded line is %d columns: %q", len(line), line)
		}
	}
}

// Only a string folds: a fold request on anything else is a refusal rather
// than a silent stringification, because the folded form is what a rationale
// wears and nothing else in a deck file has one.
func TestOnlyAStringFolds(t *testing.T) {
	t.Parallel()
	for _, bad := range []any{7, true, nil, []any{"a"}, Map{}} {
		if _, err := Render("why", bad, 0, 80, true); err == nil {
			t.Errorf("%#v was folded", bad)
		} else if !strings.Contains(err.Error(), "only a string folds") {
			t.Errorf("the refusal said %q", err)
		}
	}
	// And it does fold a string.
	if _, err := Render("why", "a rationale", 0, 80, true); err != nil {
		t.Errorf("a string was refused: %v", err)
	}
}

// Every scalar the deck file holds has one rendering, and a type it does not
// hold is a refusal rather than a guess -- a guess here would be a deck file
// that no longer round-trips.
func TestEveryScalarTheDeckFileHoldsHasOneRendering(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"a string", "Sol Ring", "Sol Ring"},
		{"an int", 4, "4"},
		{"an int64", int64(4), "4"},
		{"true", true, "true"},
		{"false", false, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Dump(Map{{Key: "k", Value: tc.in}}, 80)
			if err != nil {
				t.Fatalf("%#v: %v", tc.in, err)
			}
			if !strings.Contains(got, "k: "+tc.want) {
				t.Errorf("%#v dumped as %q, want `k: %s`", tc.in, got, tc.want)
			}
		})
	}

	// A type a deck file cannot hold refuses rather than rendering something
	// that would not read back -- **including nil**. A deck file never holds
	// a null: an absent field is absent, and writing `k: null` would be a
	// key that reads back as a value nothing in the model expects.
	for _, bad := range []any{nil, make(chan int), 1.5, uint(4)} {
		if _, err := Dump(Map{{Key: "k", Value: bad}}, 80); err == nil {
			t.Errorf("%#v was rendered into a deck file", bad)
		}
	}
}

// A string that would read back as something else is quoted, which is the one
// rule that keeps a deck file round-tripping: a card named `Yes` or a note
// reading `1.0` must come back as text.
func TestAStringThatWouldReadBackAsSomethingElseIsQuoted(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"true", "false", "null", "yes", "no", "on", "off",
		"1", "1.5", "-3", "0x10", "~", "",
	} {
		got, err := Dump(Map{{Key: "k", Value: text}}, 80)
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(got), "k:"))
		if value == text && text != "" {
			t.Errorf("%q was written bare as %q -- it would read back as a "+
				"different type", text, got)
		}
	}
	// A plain name is not quoted, because quoting everything would be a diff
	// on every hand-written deck.
	got, err := Dump(Map{{Key: "k", Value: "Sol Ring"}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `"Sol Ring"`) || strings.Contains(got, `'Sol Ring'`) {
		t.Errorf("an ordinary name was quoted: %q", got)
	}
}
