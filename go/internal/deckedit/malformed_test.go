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

// cardOperations is the subset of the writes above that has to find a card
// entry in the text before it can rewrite one. `set-deck`, `share` and `note`
// are deliberately absent: they edit top-level keys and never look inside the
// card list, so a card list this package cannot scan is none of their business.
var cardOperations = []op{
	{"swap", func(t string) (string, error) {
		return ReplaceCard(t, "Sol Ring", "Arcane Signet", "a reason", nil)
	}},
	{"add", func(t string) (string, error) {
		return AddCard(t, "Arcane Signet", "ramp", "a reason", 1, "cards")
	}},
	{"remove", func(t string) (string, error) { return RemoveCard(t, "Sol Ring") }},
	{"entomb", func(t string) (string, error) { return EntombCard(t, "Sol Ring") }},
	{"set-card", func(t string) (string, error) {
		return SetCardField(t, "Sol Ring", "category", "ramp")
	}},
	{"draft", func(t string) (string, error) { return DraftRationale(t, "Sol Ring", "a reason") }},
	// The bulk *plan* belongs here rather than beside `ApplyBulk`, and finding
	// that out is the useful part: the plan is read through the same lookup, so
	// a file like this is refused while it is still a screen full of proposed
	// changes -- before anybody presses the button, not after.
	{"plan-bulk", func(t string) (string, error) {
		_, err := PlanBulk(t, []BulkCard{{Name: "Sol Ring", Why: "a new reason", Qty: 1}})
		return "", err
	}},
}

// The file that parses perfectly and still cannot be edited -- ADR 12's case,
// which every shape above misses.
//
// The nine broken shapes in the sweep above all fail at the parse, so an
// operation refuses them before it has looked at anything. This class is the
// opposite and much more dangerous: **the document is valid YAML and a real
// deck**, so the parse hands back a card list with the right cards in it, and
// only the *text* scanner comes up empty -- because it recognises a card entry
// by the literal line `- name: ...` and these files do not have one.
//
// `locateCard` notices that the two counts disagree and refuses, and the
// refusal is the whole point: with N parsed entries and no spans, any edit that
// guessed would rewrite the wrong lines of somebody's hand-written file, and
// since ADR 30 there is no revision to restore it from. A `continue` where that
// refusal is would do exactly that.
//
// The five shapes are the ones the code's own comment names plus two more that
// arrive by hand rather than by theory: reordering the keys so `why` comes
// first, and quoting the `name` key. Neither is unusual in a file somebody
// edited in an editor that reformats YAML, and neither is something the writer
// would expect to matter.
func TestAnEditRefusesADeckWhoseCardListItCannotScan(t *testing.T) {
	t.Parallel()
	const head = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n"

	for _, unreadable := range []struct{ name, text, block string }{
		{"a flow mapping", head + "cards:\n  - {name: Sol Ring, why: ramp}\n",
			"  - {name: Sol Ring, why: ramp}"},
		{"a bare string entry", head + "cards:\n  - Sol Ring\n", "  - Sol Ring"},
		{"why before name", head + "cards:\n  - why: ramp\n    name: Sol Ring\n",
			"  - why: ramp"},
		{"a quoted name key", head + "cards:\n  - \"name\": Sol Ring\n    why: ramp\n",
			"  - \"name\": Sol Ring"},
		{"an anchor and its alias",
			head + "cards:\n  - &r\n    name: Sol Ring\n    why: ramp\n  - *r\n", "  - &r"},
	} {
		t.Run(unreadable.name, func(t *testing.T) {
			t.Parallel()

			// The premise: this really is a deck, and the card really is in it.
			// Without checking that, a refusal below could be "there is no such
			// card" wearing the same clothes.
			if !strings.Contains(unreadable.text, "Sol Ring") {
				t.Fatalf("the fixture does not hold the card the sweep asks for")
			}

			for _, o := range cardOperations {
				got, err := o.run(unreadable.text)
				if err == nil {
					t.Errorf("%s edited a card list it cannot scan, producing:\n%s", o.name, got)
					continue
				}
				// The refusal says which list and that the file is beyond this
				// editor -- the two things somebody needs to fix it by hand.
				if !strings.Contains(err.Error(), "cards") ||
					!strings.Contains(err.Error(), "cannot edit safely") {
					t.Errorf("%s refused with %q, which does not say the file is "+
						"beyond the editor", o.name, err)
				}
				if got != "" {
					t.Errorf("%s returned %d bytes alongside its error", o.name, len(got))
				}
			}

			// And the one write that legitimately succeeds does not touch the
			// block it could not read. `AddToBoard` writes to `swap_board`, a
			// list of its own, so refusing it would refuse a safe edit -- but
			// the unreadable lines have to come through byte-identical, or the
			// exemption is a hole rather than a distinction.
			out, err := AddToBoard(unreadable.text, "Arcane Signet", "ramp", "a reason", 1)
			if err != nil {
				t.Fatalf("the swap board refused a list it never reads: %v", err)
			}
			if !strings.Contains(out, unreadable.block) {
				t.Errorf("the swap board write rewrote the card list it cannot read.\n"+
					"wanted these bytes intact: %q\ngot:\n%s", unreadable.block, out)
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
