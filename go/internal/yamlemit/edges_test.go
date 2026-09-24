package yamlemit

// The document dump's edges, and the round-trip check's four ways of saying
// no.
//
// This is a reproduction rather than a YAML writer, so every assertion here
// is a spelling the emitter already produces -- written down, never changed.
// The two that matter most are at opposite ends of the same problem: a `>`
// block that keeps its trailing newlines leaves the document *open*, and a
// reader cannot tell where the scalar stopped without the explicit `...`;
// and a value with no spelling at all has to come back as an error rather
// than as a document missing a key.

import (
	"strings"
	"testing"
)

func TestADumpOfAScalarIsJustTheScalar(t *testing.T) {
	t.Parallel()
	// The root reaches `increaseIndent` before any indent has been decided,
	// which is the one place the flow branch is taken. A whole-document dump
	// of a bare scalar is what `set_shared` would produce if a deck file were
	// one value, and the recorded answer is the value and a newline.
	cases := []struct {
		value any
		want  string
	}{
		{"hello", "hello\n"},
		{12, "12\n"},
		{true, "true\n"},
		{List(nil), "[]\n"},
		{Map(nil), "{}\n"},
		{List{}, "[]\n"},
		{Map{}, "{}\n"},
		{folded("hi"), ">-\n  hi\n"},
	}
	for _, tc := range cases {
		got, err := Dump(tc.value, 80)
		if err != nil {
			t.Fatalf("Dump(%#v): %v", tc.value, err)
		}
		if got != tc.want {
			t.Fatalf("Dump(%#v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestAFoldedBlockThatKeepsItsBlankLineClosesTheDocument(t *testing.T) {
	t.Parallel()
	// A rationale that ends in a blank line takes the `>+` hint, which keeps
	// every trailing newline -- so the stream has to be closed with the
	// explicit `...` or a reader cannot tell where the scalar stopped. It
	// reaches a deck file rarely and is exactly why it would otherwise never
	// be noticed.
	got, err := Dump(folded("line one\n\n"), 80)
	if err != nil {
		t.Fatal(err)
	}
	if got != ">+\n  line one\n\n...\n" {
		t.Fatalf("Dump = %q", got)
	}
	// A block that does *not* keep them takes `>-` and needs no closing
	// marker, which is what says the `...` above is the `+` hint's doing.
	plain, err := Dump(folded("one\ntwo"), 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "...") {
		t.Fatalf("a stripped block closed the document: %q", plain)
	}
	if plain != ">-\n  one\n\n  two\n" {
		t.Fatalf("Dump = %q", plain)
	}
	// And inside a mapping, where a deck file's `why` actually sits.
	nested, err := Dump(Map{{Key: "why", Value: folded("line one\n\n")}}, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(nested, "...\n") {
		t.Fatalf("a nested open block did not close the document: %q", nested)
	}
}

func TestAValueTheStyleHasNoSpellingForIsAnErrorAtEveryDepth(t *testing.T) {
	t.Parallel()
	// The deck file's one style has no float in it. A value carrying one has
	// to come back as an error from wherever it sits -- a sequence item, a
	// mapping value, the root -- because the alternative is a document that
	// parses and is missing a key.
	cases := map[string]any{
		"at the root":        1.5,
		"in a sequence":      List{"fine", 1.5},
		"in a mapping":       Map{{Key: "why", Value: 1.5}},
		"under a long key":   Map{{Key: strings.Repeat("k", 200), Value: 1.5}},
		"nested two deep":    Map{{Key: "cards", Value: List{Map{{Key: "qty", Value: 1.5}}}}},
		"as a mapping's key": Map{{Key: "ok", Value: List{1.5}}},
	}
	for where, value := range cases {
		out, err := Dump(value, 80)
		if err == nil {
			t.Fatalf("a float %s dumped as %q", where, out)
		}
		if !strings.Contains(err.Error(), "cannot render float64") {
			t.Fatalf("a float %s refused with %v", where, err)
		}
		if out != "" {
			t.Fatalf("a refused dump handed back %q", out)
		}
	}
	// `Render` carries the same refusal out of `dump`, and refuses a fold of
	// anything that is not a string before it even gets there.
	if lines, err := Render("qty", 1.5, 0, 96, false); err == nil {
		t.Fatalf("Render wrote %q for a float", lines)
	}
	if lines, err := Render("qty", 12, 0, 96, true); err == nil {
		t.Fatalf("Render folded a number into %q", lines)
	}
}

func TestACharacterBeyondTheBasicPlaneIsEscapedWholeRatherThanCut(t *testing.T) {
	t.Parallel()
	// Double quoting is the fallback the style always permits, and it is
	// where an astral character lands. The escape is the eight-digit form --
	// cutting it to four would write half a code point and the file would no
	// longer round-trip.
	//
	// A space before a line break is what forces the double quotes: the
	// analysis forbids every other style for it, which is the one
	// arrangement a rationale pasted from a word processor reliably has.
	lines, err := Render("why", "a \nb\U0001F600", 0, 96, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("wrote %d lines: %q", len(lines), lines)
	}
	if lines[0] != `why: "a \nb\U0001F600"` {
		t.Fatalf("wrote %q", lines[0])
	}
}

func TestTheRoundTripCheckSaysNoFourSeparateWays(t *testing.T) {
	t.Parallel()
	// The fold is a request, not an instruction: folding collapses single
	// newlines and adjusts the trailing one, so for some strings it is not
	// value-preserving. This is the check that catches it, and each of its
	// four "no"s is a different way the folded text failed to read back as
	// what was handed in.
	cases := []struct {
		name  string
		text  string
		key   string
		value any
	}{
		{"the fold produced more than one key", "a: 1\nb: 2\n", "a", "1"},
		{"the key is not in what came back", "a: 1\n", "b", "1"},
		{"the value came back as another type", "a: 1\n", "a", "1"},
		{"nothing parsed at all", "\tnot: yaml\n", "a", "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, err := roundTrips(tc.text, tc.key, tc.value)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if ok {
				t.Fatal("the check accepted it")
			}
		})
	}
	// A value the style cannot render at all is a different answer from "the
	// fold was not faithful": the check reports the error rather than
	// quietly asking the caller to try again unfolded, which would loop.
	if _, err := roundTrips("a: x\n", "a", 1.5); err == nil {
		t.Fatal("a value with no spelling was answered as a failed fold")
	}
	// And a fold that really does round-trip is accepted, so the sweep above
	// is not passing on a check that refuses everything.
	ok, err := roundTrips("why: Green.\n", "why", "Green.")
	if err != nil || !ok {
		t.Fatalf("a faithful fold was refused: %v %v", ok, err)
	}
}

func TestThePlainWriterKeepsABreakRunAsABlankLine(t *testing.T) {
	t.Parallel()
	// A direct test of the writer rather than of a spelling, because the
	// style's analysis never offers the plain form to a multi-line scalar --
	// a line break forbids it. The writer's break handling is still one of
	// the three decisions the package comment names as carrying the style,
	// and this is what pins it against the day the analysis changes.
	e := newEmitter(80)
	e.increaseIndent(false)
	e.writePlain("one\n\ntwo", true)
	if got := e.out.String(); got != "one\n\n\ntwo" {
		t.Fatalf("a break run wrote %q", got)
	}
	// Nothing at all writes nothing -- not a space, which is what the
	// whitespace bookkeeping above it would otherwise leave behind.
	empty := newEmitter(80)
	empty.increaseIndent(false)
	empty.writePlain("", true)
	if got := empty.out.String(); got != "" {
		t.Fatalf("an empty scalar wrote %q", got)
	}
	// And a block with no text in it asks for no hints, which is the same
	// question at the other end of the writer.
	if got := determineBlockHints(""); got != "" {
		t.Fatalf("an empty block asked for the hints %q", got)
	}
}
