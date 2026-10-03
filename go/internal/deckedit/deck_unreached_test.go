package deckedit

import (
	"errors"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
)

// The refusals the engine carries and nothing had ever asked it for.
//
// Three different kinds live here and they are worth telling apart, because
// each one is a different answer to "why had this never run".
//
//   - **The last net.** `verified` is what makes every operation in this
//     package safe to offer at all, and the arm that fires when an edit means
//     something *else* had never been entered -- because no operation has a bug
//     that would enter it. A net nothing has ever tested is a net nobody knows
//     the state of.
//   - **The poison line.** A value the recorded style has no spelling for
//     cannot be written, and this package answers that with a line no parser
//     accepts rather than with an error threaded back through twelve
//     operations. That answer is only safe because the verification refuses it,
//     which is a fact about two functions agreeing and so a fact to check.
//   - **The file the planner skipped.** A deck whose `cards:` is a flow
//     sequence has no `cards:` block for the text scanner to find, so the bulk
//     planner's own pre-check -- which is written `if err == nil` -- does not
//     run, and the fold's refusals are reached by a plan the planner itself
//     produced. That is the half `docs/polish/COVERAGE.md` had ruled
//     unreachable "without a hand-built plan", and the ruling was reading the
//     guard rather than the file.
const (
	// A deck whose 99 is written as a flow sequence: valid YAML, a real card,
	// and no `cards:` line for `blockSpan` to match. Nothing in the library
	// looks like this, which is exactly why it is the shape that walks past a
	// check keyed on the block being findable.
	flowDeck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards: [{name: Sol Ring, why: ramp}]\n"
	// The same, with a bare string where a card entry belongs.
	namelessFlowDeck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards: [Sol Ring]\n"
)

// The value with no spelling: refused by the verification, never written.
//
// `render` cannot answer an error -- every value this package renders is a
// string, an int, a bool or a list of strings chosen by the operation itself --
// so a type it has no spelling for comes back as a line that cannot parse. The
// two halves are asserted separately because the safety is the *pair*: the line
// really is unparseable, and the verification really does refuse it.
func TestAValueTheStyleCannotWriteBecomesARefusalRatherThanAFile(t *testing.T) {
	t.Parallel()

	got := render("why", 3.5, 4)
	if len(got) != 1 || strings.TrimSpace(got[0]) != "why: [" {
		t.Fatalf("a value with no spelling rendered as %q", got)
	}
	if indentOf(got[0]) != 4 {
		t.Errorf("the poison line sits at column %d rather than at the key indent", indentOf(got[0]))
	}

	// It is poison because no parser takes it, not because it looks odd.
	if _, err := deckyaml.Parse([]byte("name: Sol Ring\n" + got[0])); err == nil {
		t.Fatal("the line a refused rendering produces parses, so nothing would refuse it")
	}

	// And the verification is what turns that into a refused edit with
	// nothing written -- which is the promise the whole choice rests on.
	if _, err := verified("cards:\n  - name: Sol Ring\n"+got[0]+"\n",
		map[string]any{"cards": []any{map[string]any{"name": "Sol Ring"}}}); err == nil {
		t.Fatal("the verification accepted an edit carrying a line that cannot parse")
	}

	// The fold's own door answers the same way, so a note is no different from
	// a rationale.
	if lines := emitted(nil, errors.New("no spelling"), "combos", 0); len(lines) != 1 ||
		lines[0] != "combos: [" {
		t.Errorf("a refused whole block came back as %q", lines)
	}
	// A rendering that worked comes through untouched, so the test above is
	// not passing on a door that poisons everything. A note is a folded block,
	// which is the style the hand-written notes in the deck files are in.
	if lines := renderProse("plan", "Keep a counterspell up.", 2); len(lines) != 2 ||
		lines[0] != "  plan: >-" || lines[1] != "    Keep a counterspell up." {
		t.Errorf("a prose note rendered as %q", lines)
	}
}

// The last net, entered on purpose.
//
// Every operation hands its text to `verified` with the document it is supposed
// to mean, and an edit that damaged a neighbouring card, dropped a note or
// reordered the 99 fails here with nothing written. No operation has that bug,
// so this arm had never been entered -- which left the one check the package's
// whole safety argument rests on untested. Called directly, with a text that
// says something other than what was asked for.
func TestTheVerificationRefusesAnEditThatMeansSomethingElse(t *testing.T) {
	t.Parallel()

	got, err := verified("name: Arcane Signet\n", map[string]any{"name": "Sol Ring"})
	if err == nil {
		t.Fatal("the verification accepted a document it was not asked for")
	}
	if got != "" {
		t.Errorf("it returned %d bytes alongside its refusal", len(got))
	}
	if !IsFailed(err) {
		t.Errorf("a mismatch came back as %T rather than as a refusal to edit", err)
	}
	// The sentence names the difference, because "it changed more than it was
	// asked to" with nothing after it is a dead end.
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("the refusal %q does not say which key disagreed", err)
	}

	// And a text that does mean exactly that comes straight back, so the
	// assertion above is not standing on a check that refuses everything.
	if out, err := verified("name: Sol Ring\n",
		map[string]any{"name": "Sol Ring"}); err != nil || out != "name: Sol Ring\n" {
		t.Errorf("a faithful edit was refused: %q %v", out, err)
	}
}

// An empty line does not begin with a space, and the answer matters: it is what
// decides where a top-level block ends, so a `""` treated as indented would put
// the next key inside the block above it.
func TestAnEmptyLineIsNotAnIndentedOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"", false},
		{"cards:", false},
		{"  - name: Sol Ring", true},
		{"\tname: Sol Ring", true},
	} {
		if got := startsWithSpace(tc.line); got != tc.want {
			t.Errorf("startsWithSpace(%q) = %v", tc.line, got)
		}
	}
}

// The lookup passes over a sequence item that is not a card at all.
//
// The counts agreeing is what the lookup trusts, and this is the file where
// they agree and one item is still not a mapping: the `- name:` lines the
// scanner found are nested *inside* the first entry, so two spans stand beside
// two items of which the second is a bare string. The answer has to be "no such
// card" rather than a panic or a match on the wrong lines.
func TestTheLookupPassesOverAnItemThatIsNotACardEntry(t *testing.T) {
	t.Parallel()
	const odd = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - stuff:\n      - name: Sol Ring\n      - name: Arcane Signet\n" +
		"  - Mana Crypt\n"

	got, err := RemoveCard(odd, "Mana Crypt")
	if err == nil {
		t.Fatalf("an item that is not a card entry was edited as one:\n%s", got)
	}
	if got != "" {
		t.Errorf("it returned %d bytes alongside its refusal", len(got))
	}
	if !strings.Contains(err.Error(), "Mana Crypt") {
		t.Errorf("the refusal %q does not name the card", err)
	}
}

// The graveyard's three refusals, each about placing the card rather than
// finding it.
//
// A return finds its card in the graveyard and then has to put it back in the
// 99, and all three of these are the 99 refusing to take it: it is the
// commander, there is no `cards:` block at all, or the block is one the scanner
// cannot read. Nothing is written in any of them -- a half-returned card would
// be a card in two places with one rationale.
func TestAReturnRefusesWhenTheNinetyNineCannotTakeTheCard(t *testing.T) {
	t.Parallel()
	const head = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n"
	const grave = "graveyard:\n  - name: Sol Ring\n    why: ramp\n"

	for _, tc := range []struct{ name, text, says string }{
		{
			"the card in the coffin is the commander",
			head + "commander:\n  - Sol Ring\ncards:\n  - name: Arcane Signet\n    why: ramp\n" + grave,
			"commander",
		},
		{
			"the deck has no cards block",
			head + grave,
			"cards",
		},
		{
			"the cards block is one the editor cannot scan",
			head + "cards:\n  - {name: Arcane Signet, why: ramp}\n" + grave,
			"cannot edit safely",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The premise: the card really is in the graveyard, so a refusal
			// below is about the 99 rather than about the lookup.
			if !strings.Contains(tc.text, grave) {
				t.Fatalf("the fixture does not hold the buried card")
			}
			got, err := ReturnCard(tc.text, "Sol Ring")
			if err == nil {
				t.Fatalf("the card was returned anyway:\n%s", got)
			}
			if got != "" {
				t.Errorf("it returned %d bytes alongside its refusal", len(got))
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("the refusal %q does not say %q", err, tc.says)
			}
		})
	}
}

// The bulk fold's four refusals, driven by a plan the planner itself made.
//
// `ApplyBulk` is a fold of three operations over the text, and each step can
// refuse. `docs/polish/COVERAGE.md` ruled those four arms closed on the grounds
// that `PlanBulk` reads the deck through the same lookup, so a file that would
// make a step fail is refused while the plan is still on screen. That is true of
// every file whose `cards:` block can be *found* -- and the pre-check is
// written `if err == nil`, so a deck whose 99 is a flow sequence walks straight
// past it with a perfectly good plan and fails one step at a time.
//
// Four pastes, one per step, because the fold stops at the first refusal: a
// reason to rewrite, a quantity to change, a card to add, and a list that names
// nothing so everything is buried. Each asserts the pair -- the step refused,
// and nothing came back to be written.
func TestTheBulkFoldRefusesAtEveryStepWhenTheDeckCannotBeEdited(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		step    string
		paste   []BulkCard
		touches func(p *BulkPlan) bool
		says    string
	}{
		{
			"a reason rewritten",
			[]BulkCard{{Name: "Sol Ring", Qty: 1, Why: "it is the best ramp there is"}},
			func(p *BulkPlan) bool { return len(p.Rewrite) == 1 },
			"Sol Ring",
		},
		{
			"a quantity changed",
			[]BulkCard{{Name: "Sol Ring", Qty: 2}},
			func(p *BulkPlan) bool { return len(p.Requantify) == 1 },
			"Sol Ring",
		},
		{
			"a card added",
			[]BulkCard{
				{Name: "Sol Ring", Qty: 1},
				{Name: "Arcane Signet", Qty: 1, Category: "ramp", Why: "two mana, any colour"},
			},
			func(p *BulkPlan) bool { return len(p.Add) == 1 },
			"cards",
		},
		{
			"a card buried",
			nil,
			func(p *BulkPlan) bool { return len(p.Entomb) == 1 },
			"Sol Ring",
		},
	} {
		t.Run(tc.step, func(t *testing.T) {
			t.Parallel()
			plan, err := PlanBulk(flowDeck, tc.paste)
			if err != nil {
				t.Fatalf("the planner refused the file instead: %v", err)
			}
			// The premise, and the whole reason this reaches the fold: the
			// plan is the planner's own, it is about this exact deck, and the
			// step under test is the only one in it.
			if plan.Basis != Fingerprint(flowDeck) {
				t.Fatalf("the plan is not about this deck")
			}
			if !tc.touches(plan) {
				t.Fatalf("the plan does not hold the step this case is about: %+v", plan)
			}
			if len(plan.Blocked) != 0 {
				t.Fatalf("the plan was blocked before the fold: %+v", plan.Blocked)
			}

			got, err := ApplyBulk(flowDeck, plan)
			if err == nil {
				t.Fatalf("the fold edited a deck the scanner cannot read:\n%s", got)
			}
			if got != "" {
				t.Errorf("it returned %d bytes alongside its refusal", len(got))
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("the refusal %q does not say %q", err, tc.says)
			}
		})
	}
}

// The planner passes over an item in the 99 that is not a card entry.
//
// Same file shape one step earlier: a flow sequence holding a bare string. The
// planner reads the 99 to decide what is held, and an item with no mapping in it
// is not a card -- so it is neither counted as held nor named for burial, and
// the paste's own card is still reported as an addition.
func TestThePlannerPassesOverWhatIsNotACardEntry(t *testing.T) {
	t.Parallel()
	plan, err := PlanBulk(namelessFlowDeck,
		[]BulkCard{{Name: "Arcane Signet", Qty: 1, Category: "ramp", Why: "two mana, any colour"}})
	if err != nil {
		t.Fatalf("the planner refused a file it can read: %v", err)
	}
	if len(plan.Add) != 1 || plan.Add[0].Name != "Arcane Signet" {
		t.Errorf("the paste's own card was not planned as an addition: %+v", plan.Add)
	}
	if len(plan.Entomb) != 0 {
		t.Errorf("something that is not a card was named for burial: %+v", plan.Entomb)
	}
	if len(plan.Unchanged) != 0 {
		t.Errorf("something that is not a card was reported as unchanged: %+v", plan.Unchanged)
	}
}
