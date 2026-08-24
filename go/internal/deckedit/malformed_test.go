package deckedit

import (
	"strings"
	"testing"
)

// Every edit against a deck file it cannot read.
//
// `deck.yaml` is the source of truth (ADR 1) and these are the only functions
// that rewrite it, so the rule is absolute: **an edit that cannot understand
// the file changes nothing**. Not "changes what it can", not "reformats the
// rest" -- nothing. The alternative is an edit that half-lands on a file
// somebody hand-wrote, and since ADR 30 there is no revision to restore it
// from.
//
// The failures are swept over every operation rather than spot-checked
// because the parse happens at the top of each one independently. A new
// operation that forgot the check would be the one that ate somebody's deck,
// and no structural rule would see it.

// op is one edit, wrapped so the sweep can call them all the same way.
type op struct {
	name string
	run  func(text string) (string, error)
}

// everyOperation is all ten writes, each with arguments that would be valid
// against a readable file.
var everyOperation = []op{
	{"swap", func(t string) (string, error) {
		return ReplaceCard(t, "Sol Ring", "Arcane Signet", "a reason", nil)
	}},
	{"add", func(t string) (string, error) {
		return AddCard(t, "Sol Ring", "ramp", "a reason", 1, "cards")
	}},
	{"remove", func(t string) (string, error) { return RemoveCard(t, "Sol Ring") }},
	{"entomb", func(t string) (string, error) { return EntombCard(t, "Sol Ring") }},
	{"return", func(t string) (string, error) { return ReturnCard(t, "Sol Ring") }},
	{"exile", func(t string) (string, error) { return ExileCard(t, "Sol Ring") }},
	{"set-card", func(t string) (string, error) {
		return SetCardField(t, "Sol Ring", "category", "ramp")
	}},
	{"set-deck", func(t string) (string, error) { return SetDeckField(t, "stage", "draft") }},
	{"share", func(t string) (string, error) { return SetShared(t, false) }},
	{"note", func(t string) (string, error) { return SetNote(t, "plan", "a note") }},
}

// **An edit that cannot understand the file changes nothing.** Every
// operation, against every shape a file can be broken in.
func TestNoEditTouchesAFileItCannotRead(t *testing.T) {
	t.Parallel()
	for _, broken := range []struct{ name, text string }{
		{"unclosed bracket", "slug: gyome\ncards: [\n"},
		{"a tab where a space belongs", "slug: gyome\n\tname: Gyome\n"},
		{"a duplicate key", "slug: gyome\nslug: trostani\ncards: []\n"},
		{"not YAML at all", "\x00\x01\x02 this is not a deck"},
		{"a bare list", "- one\n- two\n"},
		{"a bare scalar", "just a string\n"},
		{"empty", ""},
		{"only whitespace", "   \n\n  \n"},
		{"unclosed quote", `slug: "gyome` + "\ncards: []\n"},
	} {
		t.Run(broken.name, func(t *testing.T) {
			t.Parallel()
			for _, o := range everyOperation {
				got, err := o.run(broken.text)
				if err == nil {
					t.Errorf("%s edited a file it could not read, producing:\n%s", o.name, got)
					continue
				}
				// Nothing partial comes back: a caller that ignored the
				// error would otherwise write the wreckage.
				if got != "" {
					t.Errorf("%s returned %d bytes alongside its error", o.name, len(got))
				}
			}
		})
	}
}

// A file that parses but is not a deck is refused too -- a mapping with no
// `cards` is a YAML document, not a decklist, and an edit that scaffolded one
// would turn somebody's notes file into a deck.
func TestAnEditRefusesADocumentThatIsNotADeck(t *testing.T) {
	t.Parallel()
	for _, o := range everyOperation {
		got, err := o.run("some_other_document: true\n")
		if err == nil {
			// A few operations legitimately create the section they write
			// to -- `note` and `set-deck` among them -- so what is asserted
			// is only that the result is still a document and nothing was
			// silently dropped.
			if !strings.Contains(got, "some_other_document") {
				t.Errorf("%s dropped the document it was handed:\n%s", o.name, got)
			}
			continue
		}
		if got != "" {
			t.Errorf("%s returned %d bytes alongside its error", o.name, len(got))
		}
	}
}

// The card operations refuse a card that is not there rather than creating
// one -- an edit to a card the deck does not hold is a typo, and inventing
// the card would bury it.
func TestTheCardOperationsRefuseACardThatIsNotThere(t *testing.T) {
	t.Parallel()
	const deck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"

	for _, o := range []op{
		{"swap", func(t string) (string, error) {
			return ReplaceCard(t, "Nonexistent Card", "Arcane Signet", "a reason", nil)
		}},
		{"remove", func(t string) (string, error) { return RemoveCard(t, "Nonexistent Card") }},
		{"entomb", func(t string) (string, error) { return EntombCard(t, "Nonexistent Card") }},
		{"set-card", func(t string) (string, error) {
			return SetCardField(t, "Nonexistent Card", "category", "ramp")
		}},
	} {
		got, err := o.run(deck)
		if err == nil {
			t.Errorf("%s edited a card that is not in the deck:\n%s", o.name, got)
			continue
		}
		if !strings.Contains(err.Error(), "Nonexistent Card") {
			t.Errorf("%s said %q without naming the card", o.name, err)
		}
		if got != "" {
			t.Errorf("%s returned %d bytes alongside its error", o.name, len(got))
		}
	}

	// The graveyard's own two, likewise: a card that was never entombed
	// cannot be returned or exiled.
	for _, o := range []op{
		{"return", func(t string) (string, error) { return ReturnCard(t, "Sol Ring") }},
		{"exile", func(t string) (string, error) { return ExileCard(t, "Sol Ring") }},
	} {
		if got, err := o.run(deck); err == nil {
			t.Errorf("%s acted on an empty graveyard:\n%s", o.name, got)
		}
	}
}

// **The editor is the mechanism, not the policy.** It writes what it is told,
// and rule 4's refusal -- "no surface ever writes a rationale on the user's
// behalf" -- lives one layer up, at every route and command that calls it.
//
// This test exists to pin that boundary rather than to approve of it: if the
// refusal ever moves down here, this fails and somebody decides deliberately.
// The backstop underneath is the gate, which fails a deck whose cards have no
// rationale however the file came to be that way.
func TestTheEditorWritesWhatItIsToldAndTheRouteHoldsRuleFour(t *testing.T) {
	t.Parallel()
	const deck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"

	// The editor writes an empty rationale when asked directly...
	out, err := AddCard(deck, "Arcane Signet", "ramp", "", 1, "cards")
	if err != nil {
		t.Fatalf("the editor refused: %v -- if that is deliberate, rule 4 "+
			"now lives here and the routes' checks are the redundant copy", err)
	}
	if !strings.Contains(out, "Arcane Signet") {
		t.Fatalf("the card was not added:\n%s", out)
	}
	// ...and the blank is written as a blank rather than as something
	// invented, which is the half of rule 4 that IS this layer's.
	if !strings.Contains(out, "why: ''") {
		t.Errorf("the editor invented a rationale:\n%s", out)
	}

	// The category is the same shape: a value the fixed set does not hold
	// is written, and refused by `checkCategory` at the route.
	out, err = AddCard(deck, "Arcane Signet", "rampp", "a reason", 1, "cards")
	if err != nil {
		t.Fatalf("the editor refused a category: %v", err)
	}
	if !strings.Contains(out, "category: rampp") {
		t.Errorf("the editor changed the category it was given:\n%s", out)
	}
}

// Adding a card that is already there is refused rather than doubled --
// singleton is the format, and a silent duplicate is a deck that fails the
// gate for a reason nobody typed.
func TestAddingACardTwiceIsRefused(t *testing.T) {
	t.Parallel()
	const deck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"

	got, err := AddCard(deck, "Sol Ring", "ramp", "a reason", 1, "cards")
	if err == nil {
		t.Fatalf("a card was added twice:\n%s", got)
	}
	if !strings.Contains(err.Error(), "Sol Ring") {
		t.Errorf("the refusal said %q", err)
	}

	// Case and surrounding space do not make it a different card, which is
	// the same folding the lookups use.
	for _, name := range []string{"sol ring", "  Sol Ring  ", "SOL RING"} {
		if got, err := AddCard(deck, name, "ramp", "a reason", 1, "cards"); err == nil {
			t.Errorf("%q was added beside Sol Ring:\n%s", name, got)
		}
	}
}

// A successful edit is still a whole document: the operations that do work
// return something that parses back, which is what makes the oracle's
// verification meaningful rather than circular.
func TestASuccessfulEditStillParses(t *testing.T) {
	t.Parallel()
	const deck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"

	for _, o := range []op{
		{"add", func(t string) (string, error) {
			return AddCard(t, "Arcane Signet", "ramp", "a reason", 1, "cards")
		}},
		{"remove", func(t string) (string, error) { return RemoveCard(t, "Sol Ring") }},
		{"entomb", func(t string) (string, error) { return EntombCard(t, "Sol Ring") }},
		{"set-card", func(t string) (string, error) {
			return SetCardField(t, "Sol Ring", "category", "ramp")
		}},
		{"set-deck", func(t string) (string, error) { return SetDeckField(t, "stage", "curated") }},
		{"share", func(t string) (string, error) { return SetShared(t, false) }},
		{"note", func(t string) (string, error) { return SetNote(t, "plan", "a note") }},
	} {
		out, err := o.run(deck)
		if err != nil {
			t.Errorf("%s: %v", o.name, err)
			continue
		}
		if out == "" {
			t.Errorf("%s produced nothing", o.name)
			continue
		}
		// It parses back, and it is still the same deck.
		if !strings.Contains(out, "slug: gyome") {
			t.Errorf("%s lost the slug:\n%s", o.name, out)
		}
		// Every one of these is surgical: the deck's other fields survive.
		if !strings.Contains(out, "name: Gyome") {
			t.Errorf("%s lost the name:\n%s", o.name, out)
		}
	}
}
