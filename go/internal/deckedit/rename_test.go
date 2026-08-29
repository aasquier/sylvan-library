package deckedit

import (
	"strings"
	"testing"
)

// Renaming a deck, and the one thing a rename must never do.
//
// A deck carries two identities and they are not the same identity. `name` is
// what a person reads -- the shelf card, the page's heading, the primer's
// title. The slug is the deck's *address*: ADR 22 makes a reference an owner
// plus a slug, so it is in every link already shared, every artifact on disk,
// and every row of the deck's own history.
//
// Until 2026-08-29 neither was writable after an import: `name` was not in
// `SettableDeckFields`, so a deck wore whatever the import form was given
// forever. The fix settles only the readable half, deliberately. The
// precedent is `FileSource.Restore`, which refuses to rename a deck coming
// back out of the crypt rather than hand back "a deck whose every artifact,
// link and log entry named something else" -- a rename that dragged the
// address along would be that same wrong, done on purpose.

// theDeck is a file that disagrees with itself on purpose, which is the state
// this whole family has to survive: the repository's own `mono-green-clean`
// fixture lives in a folder of that name and says `slug: mono-green` inside.
// The folder is the address; the key is not.
const theDeck = "slug: mono-green\nname: Mono-Green Fixture\n" +
	"status: theoretical\nstage: curated\n" +
	"cards:\n  - name: Sol Ring\n    category: ramp\n    why: two mana on turn one\n"

func TestADeckCanBeRenamed(t *testing.T) {
	t.Parallel()
	out, err := SetDeckField(theDeck, "name", "Goreclaw, Terror of Qal Sisma — Stompy")
	if err != nil {
		t.Fatalf("renaming a deck was refused: %v", err)
	}
	if !strings.Contains(out, "Goreclaw, Terror of Qal Sisma — Stompy") {
		t.Fatalf("the new name did not land:\n%s", out)
	}
	if strings.Contains(out, "Mono-Green Fixture") {
		t.Errorf("the old name survived under the new one:\n%s", out)
	}
	// One `name:` at column zero, not two. The cards each have one of their
	// own, indented, and none of them may be touched.
	if n := strings.Count(out, "\nname:"); n != 1 {
		t.Errorf("the deck has %d top-level names:\n%s", n, out)
	}
	if !strings.Contains(out, "  - name: Sol Ring") {
		t.Errorf("a card entry was disturbed by a deck-level rename:\n%s", out)
	}
}

// The ruling, as a test. A rename changes what the deck is called and never
// where it lives.
func TestARenameLeavesTheAddressAlone(t *testing.T) {
	t.Parallel()
	out, err := SetDeckField(theDeck, "name", "Something Else Entirely")
	if err != nil {
		t.Fatalf("renaming a deck was refused: %v", err)
	}
	if !strings.HasPrefix(out, "slug: mono-green\n") {
		t.Errorf("the rename reached the deck's address; every link anybody "+
			"has already shared now points at nothing:\n%s", out)
	}
}

// A deck imported without a `name:` key reads back as its slug (`Deck.from_text`
// falls back), which is the state most in need of the pen -- and the key has to
// land where the dumper would have put it rather than at the end of the file.
func TestARenameWritesTheKeyADeckDoesNotHaveYet(t *testing.T) {
	t.Parallel()
	const nameless = "slug: untitled-deck\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"
	out, err := SetDeckField(nameless, "name", "Gyome, Master Chef")
	if err != nil {
		t.Fatalf("naming an unnamed deck was refused: %v", err)
	}
	if !strings.HasPrefix(out, "slug: untitled-deck\nname: Gyome, Master Chef\n") {
		t.Errorf("the key was not written where the dumper puts it:\n%s", out)
	}
}

// Blank is refused, and this is the refusal that matters most: `Deck.from_text`
// falls back to the slug when the name is empty, so a blanked name would not
// look like an error -- the shelf would go on showing a name nobody chose, as
// though somebody had chosen it.
func TestABlankNameIsRefused(t *testing.T) {
	t.Parallel()
	for _, empty := range []string{"", "   ", "\n\t ", " "} {
		out, err := SetDeckField(theDeck, "name", empty)
		if err == nil {
			t.Errorf("a blank name (%q) was written:\n%s", empty, out)
			continue
		}
		if !IsFailed(err) {
			t.Errorf("a blank name (%q) failed as a bug rather than a refusal: %v", empty, err)
		}
	}
}

// The cap is a courtesy fence around a heading, so it is counted in characters
// a reader sees rather than in bytes -- an em dash is one of them, not three.
func TestANameLongerThanAHeadingIsRefused(t *testing.T) {
	t.Parallel()
	// Exactly at the cap, built out of multibyte runes so a byte count would
	// refuse it and a rune count must not.
	atTheCap := strings.Repeat("é", DeckNameMax)
	if _, err := SetDeckField(theDeck, "name", atTheCap); err != nil {
		t.Errorf("a name of exactly %d characters was refused, so the cap is "+
			"being counted in bytes: %v", DeckNameMax, err)
	}
	_, err := SetDeckField(theDeck, "name", strings.Repeat("é", DeckNameMax+1))
	if err == nil {
		t.Fatalf("a name of %d characters was written", DeckNameMax+1)
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("the refusal does not say where the long text belongs: %q", err)
	}
}

// Whitespace is tidied rather than refused. A name arrives pasted as often as
// typed, and a doubled space is a slip; a name of the wrong length is a
// decision, and gets an answer instead of a correction.
func TestARenameTidiesWhitespaceRatherThanRefusingIt(t *testing.T) {
	t.Parallel()
	out, err := SetDeckField(theDeck, "name", "  Gyome,   Master\tChef \n")
	if err != nil {
		t.Fatalf("a pasted name was refused: %v", err)
	}
	if !strings.Contains(out, "name: Gyome, Master Chef\n") {
		t.Errorf("the name was not tidied:\n%s", out)
	}
}

// A name is free text, so it is the settable deck field most likely to contain
// something YAML reads as syntax. The emitter quotes what it has to; this is
// the check that the whole round trip survives it, since a name that broke the
// file would take the deck with it.
func TestANameMadeOfPunctuationSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"# 1 in the pod", "yes", "no", "* Stompy *", "Gyome: Master Chef",
		"[Fixture]", "Ærathi — 100% Food", "'Squire'", `"Panache"`,
	} {
		out, err := SetDeckField(theDeck, "name", name)
		if err != nil {
			t.Errorf("%q was refused: %v", name, err)
			continue
		}
		doc, _, err := open(out)
		if err != nil {
			t.Errorf("%q produced a file that no longer parses: %v", name, err)
			continue
		}
		if got := asString(doc["name"]); got != name {
			t.Errorf("%q read back as %q", name, got)
		}
	}
}

// The trailing-comment rule, which every scalar edit obeys: the comment is the
// author's note about the field, not about the value (ADR 12 rule 1).
func TestARenameKeepsTheAuthorsCommentOnTheLine(t *testing.T) {
	t.Parallel()
	const commented = "slug: gyome\nname: Gyome  # named for the troll, not the food\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"
	out, err := SetDeckField(commented, "name", "Gyome, Master Chef")
	if err != nil {
		t.Fatalf("renaming was refused: %v", err)
	}
	if !strings.Contains(out, "# named for the troll, not the food") {
		t.Errorf("the author's comment went with the value:\n%s", out)
	}
}
