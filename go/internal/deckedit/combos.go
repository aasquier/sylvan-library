package deckedit

import (
	"fmt"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/yamlemit"
)

// The twelfth operation, and the only one that rewrites a whole block.
//
// Every other operation here is line surgery over one card or one key, because
// `swaps.md` is a diff and the *size* of an edit is part of its correctness
// (ADR 12). This one is different and the difference is honest rather than
// convenient: a combo is a paragraph about several cards at once, entries are
// reordered as often as they are reworded, and a person adding the second
// machine to a block of three is not making a one-line change to any of them.
// Per-entry surgery would buy a smaller diff for a block that runs two to eight
// entries and cost a whole second locator -- "which entry is this" has no
// `- name:` line to find it by, because a combo has no name (`deck.Combo`
// argues why).
//
// So the block is composed by the caller and written whole. What keeps that
// safe is the same thing that keeps the line surgery safe: the text is refused
// unless it parses back to exactly the document this said it would produce, and
// **only the `combos:` block may differ** -- an operation that touched a card,
// a note or the 99 fails `verified` and nothing is written.

// combosKey is the block's key. Spelled once.
const combosKey = "combos"

// MaxCombos is how many machines one deck may catalogue.
//
// Not a modelling claim -- a deck with forty real combos is a deck with a
// problem -- but a bound on a whole-block PUT, which is the one shape here that
// a caller could hand an unbounded list. Generous enough that nobody meets it
// by playing Magic.
const MaxCombos = 40

// SetCombos rewrites the deck's `combos:` block, and is the only way that block
// is written.
//
// An empty list removes the block outright rather than writing `combos: []`.
// Absent already means "nothing catalogued", so the emptied form would leave
// every deck that ever tried the feature asserting a shelf it does not keep --
// the same self-cleaning round trip `SetShared` and the shadowed `archetype:`
// take.
//
// **The `by` mark is never taken from the caller.** A drafted entry is marked
// `by: claude` (ADR 41's rule, one block over), and the mark comes off the
// moment a person changes what the entry says. That cannot be enforced by
// asking a client to be honest about it, so it is not asked: the incoming
// entries' own `By` is discarded, and the mark is carried forward from the
// deck file only onto an entry whose every other field is unchanged. Edit one
// word of a drafted combo and it becomes yours, which is what adopting means.
func SetCombos(text string, wanted []deck.Combo) (string, error) {
	if len(wanted) > MaxCombos {
		return "", failf("a deck catalogues at most %d combos; this list has %d",
			MaxCombos, len(wanted))
	}
	combos := make([]deck.Combo, 0, len(wanted))
	for i, combo := range wanted {
		checked, err := comboValue(combo)
		if err != nil {
			return "", failf("combo %d: %s", i+1, err.Error())
		}
		combos = append(combos, checked)
	}

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	carryMarks(doc, combos)

	expected := copyDoc(doc)
	if len(combos) == 0 {
		delete(expected, combosKey)
	} else {
		items := make([]any, 0, len(combos))
		for _, combo := range combos {
			items = append(items, comboDocument(combo))
		}
		expected[combosKey] = items
	}

	start, end, found := topLevelSpan(lines, combosKey)
	if !found {
		if len(combos) == 0 {
			// Nothing catalogued, nothing to catalogue: the file already says
			// exactly this. Returned rather than refused, so a client that
			// removed the last entry twice is not told off for it.
			return verified(text, expected)
		}
		at := placeAfter(lines, combosKey)
		return verified(joinAround(lines, at, at, append([]string{""}, comboBlock(combos)...)), expected)
	}

	_, tail := splitTail(lines[start:end], 0)
	if len(combos) == 0 {
		// The block goes, and the blank line that led to it goes with it --
		// otherwise a deck that has emptied its shelf keeps a widening gap at
		// the foot of the file, one line per emptying.
		gone := start
		for gone > 0 && strings.TrimSpace(lines[gone-1]) == "" {
			gone--
		}
		return verified(joinAround(lines, gone, end, tail), expected)
	}
	return verified(joinAround(lines, start, end, append(comboBlock(combos), tail...)), expected)
}

// comboValue is the per-entry validation, and every refusal in it is about the
// entry being *readable* rather than about the Magic being right.
//
// Whether two cards actually go infinite together is not a question this can
// answer and not one it pretends to: the gate warns about names, and the person
// who catalogued the machine is the one who knows whether it turns. What is
// checked here is that the entry says something -- a combo with no pieces has
// no heading, and one that does not say what it produces is a note about cards
// rather than a combo.
func comboValue(c deck.Combo) (deck.Combo, error) {
	out := deck.Combo{
		Produces: strings.TrimSpace(c.Produces),
		How:      strings.TrimSpace(c.How),
		Setup:    strings.TrimSpace(c.Setup),
		Needs:    strings.TrimSpace(c.Needs),
		Cut:      strings.TrimSpace(c.Cut),
	}
	for _, name := range c.Cards {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, already := range out.Cards {
			if strings.EqualFold(already, name) {
				return deck.Combo{}, fmt.Errorf(
					"%s is listed twice among the pieces", quotedValue(name))
			}
		}
		out.Cards = append(out.Cards, name)
	}
	if len(out.Cards) == 0 {
		return deck.Combo{}, fmt.Errorf("a combo is the cards it is made of, " +
			"and those cards are its name here -- there is no separate title. " +
			"Name at least one piece")
	}
	if out.Produces == "" {
		return deck.Combo{}, fmt.Errorf("say what it produces; a machine nobody " +
			"described the output of is a list of cards")
	}
	if out.Cut != "" && out.Needs == "" {
		return deck.Combo{}, fmt.Errorf("%s is offered as the cut, but this combo "+
			"is not missing anything -- a cut only means something beside the card "+
			"it would come in for", quotedValue(out.Cut))
	}
	if out.Needs != "" {
		for _, name := range out.Cards {
			if strings.EqualFold(name, out.Needs) {
				return deck.Combo{}, fmt.Errorf("%s is named as both a piece this "+
					"deck has and the card it is missing", quotedValue(out.Needs))
			}
		}
		if strings.EqualFold(out.Needs, out.Cut) {
			return deck.Combo{}, fmt.Errorf("%s cannot come in for itself",
				quotedValue(out.Needs))
		}
	}
	return out, nil
}

// carryMarks re-attaches the `by` marks the deck file already holds.
//
// Content-matched rather than position-matched, and that is the whole of it: an
// entry is the same entry when it says the same thing, wherever it has moved to
// in the block. Reordering a list is not editing what is in it, and a person who
// drags a drafted combo above another one has not adopted either.
func carryMarks(doc map[string]any, combos []deck.Combo) {
	marked := []map[string]any{}
	for _, item := range listOf(doc, combosKey) {
		entry, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(asString(entry["by"])) == "" {
			continue
		}
		marked = append(marked, entry)
	}
	if len(marked) == 0 {
		return
	}
	for i := range combos {
		// The candidate as it would be written, with no mark on it: what is
		// compared is what the entry *says*, so a mark can only survive onto an
		// entry that says exactly what the marked one did.
		bare := comboDocument(combos[i])
		for _, entry := range marked {
			was := map[string]any{}
			for k, v := range entry {
				if k != "by" {
					was[k] = v
				}
			}
			if equalDocs(bare, was) {
				combos[i].By = strings.TrimSpace(asString(entry["by"]))
				break
			}
		}
	}
}

// comboBlock renders the whole `combos:` block, ready to splice in.
//
// **At the whole-file width rather than this package's own 96**, which is the
// one place an operation here departs from the surgical rule, deliberately.
// Everything else in this package rewrites the lines it changes and leaves the
// file's own wrapping alone; this rewrites the block entire, so there is no
// neighbouring line whose width it has to sit beside -- and matching
// `deck.Dump` means a combos block written by an import and one written by
// somebody pressing Save are the same bytes rather than the same block wrapped
// two ways.
func comboBlock(combos []deck.Combo) []string {
	text, err := yamlemit.Dump(
		yamlemit.Map{{Key: combosKey, Value: deck.ComboList(combos)}}, deck.DumpWidth)
	if err != nil {
		// Unreachable through `comboValue`, which admits only strings and
		// refuses the empty list. Returned as a line the verification will
		// refuse rather than as a panic, because this package's promise is that
		// a failed edit changes nothing.
		return []string{combosKey + ": !"}
	}
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}

// comboDocument is one entry as `deckyaml.Parse` will hand it back, which is
// what makes the expected document comparable to the re-read one.
func comboDocument(c deck.Combo) map[string]any {
	cards := make([]any, 0, len(c.Cards))
	for _, name := range c.Cards {
		cards = append(cards, name)
	}
	out := map[string]any{"cards": cards}
	for _, pair := range []struct{ key, value string }{
		{"needs", c.Needs}, {"produces", c.Produces}, {"how", c.How},
		{"setup", c.Setup}, {"cut", c.Cut}, {"by", c.By},
	} {
		if pair.value != "" {
			out[pair.key] = pair.value
		}
	}
	return out
}

// placeAfter is the line a top-level key belongs on when the file has none --
// `SetDeckField`'s rule for an absent `stage:`, lifted so the two cannot
// disagree about where `deckKeyOrder` puts things.
func placeAfter(lines []string, key string) int {
	at := 0
	for _, before := range deckKeyOrder {
		if before == key {
			break
		}
		s, e, ok := topLevelSpan(lines, before)
		if !ok {
			continue
		}
		content, _ := splitTail(lines[s:e], 0)
		at = max(at, s+len(content))
	}
	return at
}
