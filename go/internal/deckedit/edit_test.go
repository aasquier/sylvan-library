package deckedit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The gate Phase 4 sets for this family, in the plan's own words: "every
// operation applied by Go over fixture decks yields byte-output Python's
// operation also yields". `tests/go_fixtures.py` runs the nine operations over
// the eight fixture decks the gate uses and records both halves of what an
// edit operation is -- the text when it applies, the sentence when it refuses
// -- and this reproduces both.
//
// Steps chain, each applying to the previous one's output, which is the only
// way the round trips are reachable: a return needs a burial, a second burial
// needs a graveyard, and an emptied list needs a list somebody emptied.

type editFixture struct {
	Decks map[string]string `json:"decks"`
	Cases []editCase        `json:"cases"`
}

type editCase struct {
	Deck  string     `json:"deck"`
	Chain int        `json:"chain"`
	Steps []editStep `json:"steps"`
}

type editStep struct {
	Op    string         `json:"op"`
	Args  map[string]any `json:"args"`
	OK    bool           `json:"ok"`
	Want  string         `json:"want"`
	Error string         `json:"error"`
}

func loadEdits(t *testing.T) editFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/edits.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var fixture editFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(fixture.Cases) == 0 || len(fixture.Decks) == 0 {
		t.Fatal("the oracle is empty; run `python tests/go_fixtures.py`")
	}
	return fixture
}

// apply dispatches one recorded step to the operation it names.
func apply(step editStep, text string) (string, error) {
	arg := func(key string) string { s, _ := step.Args[key].(string); return s }
	number := func(key string, fallback int) int {
		if v, ok := step.Args[key].(float64); ok {
			return int(v)
		}
		return fallback
	}
	switch step.Op {
	case "replace_card":
		var category *string
		if raw, ok := step.Args["category"]; ok {
			value, _ := raw.(string)
			category = &value
		}
		return ReplaceCard(text, arg("old_name"), arg("new_name"), arg("why"), category)
	case "add_card":
		list := "cards"
		if raw, ok := step.Args["list_key"]; ok {
			list, _ = raw.(string)
		}
		return AddCard(text, arg("name"), arg("category"), arg("why"), number("qty", 1), list)
	case "remove_card":
		return RemoveCard(text, arg("name"))
	case "entomb_card":
		return EntombCard(text, arg("name"))
	case "return_card":
		return ReturnCard(text, arg("name"))
	case "exile_card":
		return ExileCard(text, arg("name"))
	case "set_card_field":
		return SetCardField(text, arg("name"), arg("field"), step.Args["value"])
	case "set_deck_field":
		return SetDeckField(text, arg("field"), step.Args["value"])
	case "set_note":
		return SetNote(text, arg("key"), arg("value"))
	default:
		return "", fmt.Errorf("unknown operation %q", step.Op)
	}
}

// TestEveryOperationWritesWhatPythonWrites is the family's whole gate.
func TestEveryOperationWritesWhatPythonWrites(t *testing.T) {
	fixture := loadEdits(t)
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%d", c.Deck, c.Chain), func(t *testing.T) {
			text, ok := fixture.Decks[c.Deck]
			if !ok {
				t.Fatalf("the oracle has no deck named %q", c.Deck)
			}
			for i, step := range c.Steps {
				got, err := apply(step, text)
				switch {
				case step.OK && err != nil:
					t.Fatalf("step %d (%s %v): Python applied it, Go refused: %v",
						i, step.Op, step.Args, err)
				case step.OK:
					if diff := cmp.Diff(splitKeepingLines(step.Want), splitKeepingLines(got)); diff != "" {
						t.Fatalf("step %d (%s %v): different bytes\n%s", i, step.Op, step.Args, diff)
					}
					// A refused step leaves the text where it was, exactly as
					// the generator's chain does, so the next step in this
					// chain sees what Python's next step saw.
					text = got
				case err == nil:
					t.Fatalf("step %d (%s %v): Python refused with %q, Go applied it",
						i, step.Op, step.Args, step.Error)
				default:
					if !IsFailed(err) {
						t.Fatalf("step %d (%s %v): refused, but not as an edit failure: %v",
							i, step.Op, step.Args, err)
					}
					if !sameRefusal(step.Error, err.Error()) {
						t.Fatalf("step %d (%s %v): different refusal\n  Python: %s\n      Go: %s",
							i, step.Op, step.Args, step.Error, err.Error())
					}
				}
			}
		})
	}
}

// TestTheOracleCoversEveryOperation guards the corpus rather than the code: a
// generator that quietly stopped emitting one operation would leave this
// package proving less while reporting the same green.
func TestTheOracleCoversEveryOperation(t *testing.T) {
	fixture := loadEdits(t)
	applied := map[string]int{}
	refused := map[string]int{}
	for _, c := range fixture.Cases {
		for _, step := range c.Steps {
			if step.OK {
				applied[step.Op]++
			} else {
				refused[step.Op]++
			}
		}
	}
	// Both halves for every operation: an operation only ever exercised on its
	// happy path is an operation whose refusals are unproven, and the
	// refusals are where rule 4 and ADR 27 live.
	for _, op := range []string{
		"replace_card", "add_card", "remove_card", "entomb_card", "return_card",
		"exile_card", "set_card_field", "set_deck_field", "set_note",
	} {
		if applied[op] == 0 {
			t.Errorf("the oracle never applies %s", op)
		}
		if refused[op] == 0 {
			t.Errorf("the oracle never sees %s refuse", op)
		}
	}
	if len(fixture.Decks) < 11 {
		t.Errorf("the oracle covers %d decks; it wants the gate's eight plus "+
			"the three written by hand", len(fixture.Decks))
	}
	for _, name := range []string{"handwritten", "wide", "tight"} {
		if _, ok := fixture.Decks[name]; !ok {
			t.Errorf("the oracle lost the %q fixture; it is the only deck in the "+
				"corpus with the shape it has, and the rules it exercises pass "+
				"every dumped deck", name)
		}
	}
}

// brokenParse is the one refusal whose sentence this port cannot reproduce.
//
// `_verified` says "the edit produced YAML that no longer parses: " and then
// quotes the loader, which is PyYAML on one side and goccy on the other --
// different libraries, different prose, and neither is wrong. Everything that
// matters is the same: both refuse, both refuse for the same reason, and both
// leave the file untouched because these operations return text rather than
// writing it.
//
// It is reachable at all only from a deck whose card keys do not sit two
// columns right of the dash, since `cardLines` re-attaches the dash by hand;
// the `tight` fixture is that deck, and it is in the corpus so that this stays
// an exercised path rather than a paragraph.
const brokenParse = "the edit produced YAML that no longer parses:"

func sameRefusal(want, got string) bool {
	if strings.HasPrefix(want, brokenParse) {
		return strings.HasPrefix(got, brokenParse)
	}
	return want == got
}

// TestABrokenParseRefuses pins that path rather than leaving it to the corpus,
// because a prefix comparison is exactly the kind that would keep passing if
// the refusal stopped happening at all.
func TestABrokenParseRefuses(t *testing.T) {
	fixture := loadEdits(t)
	text, ok := fixture.Decks["tight"]
	if !ok {
		t.Fatal("the oracle has no tight fixture")
	}
	// Writing a card entry into this deck cannot line the keys up.
	_, err := AddCard(text, "Llanowar Reborn", "ramp", "Anywhere.", 1, "cards")
	if err == nil {
		t.Fatal("adding a card to a deck the editor cannot lay out was accepted")
	}
	if !IsFailed(err) || !strings.HasPrefix(err.Error(), brokenParse) {
		t.Fatalf("expected a verification refusal, got %v", err)
	}
	// ... and rewriting a field in the same deck is fine, which is what makes
	// the refusal above about the layout rather than about the file.
	updated, err := SetCardField(text, "Sol Ring", "category", "utility")
	if err != nil {
		t.Fatalf("a field rewrite should serve this deck: %v", err)
	}
	if !strings.Contains(updated, "     category: utility") {
		t.Errorf("the rewrite did not keep the deck's own indent:\n%s", updated)
	}
}

// TestAnEditIsTheSizeItClaimsToBe holds ADR 12's rule 1 -- an edit touches
// only what it changes -- as a number rather than as a sentence.
//
// The oracle proves Go writes Python's bytes; it cannot prove those bytes are
// a *small* diff, because a mutual regression would pass. This asks the
// separate question: a one-card swap has to be a one-card diff, or `swaps.md`
// is unreadable, which is the whole reason this package is text surgery.
func TestAnEditIsTheSizeItClaimsToBe(t *testing.T) {
	fixture := loadEdits(t)
	text, ok := fixture.Decks["rich"]
	if !ok {
		t.Fatal("the oracle has no rich fixture")
	}
	before := strings.Split(text, "\n")

	swapped, err := ReplaceCard(text, "Sol Ring", "Mana Crypt",
		"Faster, and the damage is a cost this deck can pay.", nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed := changedLines(before, strings.Split(swapped, "\n")); changed > 8 {
		t.Errorf("a one-card swap changed %d lines; it is meant to be the "+
			"card's own entry and nothing else", changed)
	}

	// The comments are the file's, not the editor's: Goreclaw's section
	// banners are why `_split_tail` exists at all.
	for _, line := range before {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !strings.Contains(swapped, line) {
			t.Errorf("the swap lost the comment %q", line)
		}
	}
}

func changedLines(before, after []string) int {
	kept := map[string]int{}
	for _, line := range before {
		kept[line]++
	}
	changed := 0
	for _, line := range after {
		if kept[line] > 0 {
			kept[line]--
			continue
		}
		changed++
	}
	for _, remaining := range kept {
		changed += remaining
	}
	return changed
}

// splitKeepingLines makes a byte difference readable as the line diff it
// almost always is.
func splitKeepingLines(s string) []string { return strings.Split(s, "\n") }
