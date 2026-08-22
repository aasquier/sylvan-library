package deck

import (
	"fmt"

	"github.com/aasquier/sylvan-library/go/internal/pyyaml"
)

// DumpWidth is `Deck.dump`'s `width=100`. `edit.py`'s `_render` uses 96 for
// the lines it surgically rewrites; the two differ because they always have,
// and a deck written at one width and edited at the other is the ordinary
// state of every file in the library.
const DumpWidth = 100

// Dump is `decks/model.py:Deck.dump`: the whole file, as PyYAML writes it.
//
// The write path is surgical everywhere else (ADR 12), so this is reached by
// exactly two callers and both of them are writing a file that does not exist
// yet: `service.create_deck` and `service.import_deck`. That is the whole
// justification for a second way to produce deck YAML -- a deck being born has
// no comments to destroy, no folded blocks to reflow and no diff to keep
// small.
//
// **A deck carrying notes is refused rather than written**, and the refusal is
// the honest answer rather than a missing feature. `sort_keys=False` means the
// payload's order *is* the file's order, and this package holds notes in a Go
// map, which has none -- so dumping one would write a file whose key order
// changed between two runs of the same code. Neither caller can reach it: a
// created deck has no notes and an imported one carries none across. The day
// something dumps a parsed deck -- the artifacts snapshot is the candidate --
// ordering the notes is that flip's first job.
func (d *Deck) Dump() (string, error) {
	if len(d.Notes) > 0 {
		return "", fmt.Errorf(
			"deck %q carries notes, and dumping a whole file cannot order them; "+
				"edit the file surgically instead", d.Slug)
	}
	if _, ok := d.Strategy.(string); !ok && d.Strategy != nil {
		// Same argument one field over: `strategy` is a string in every deck
		// the model describes, but a hand-written file may hold anything and
		// `FromText` passes it through. A mapping or a list would need the
		// same ordering this cannot give.
		return "", fmt.Errorf("deck %q has a strategy that is not prose, "+
			"which a whole-file dump cannot write", d.Slug)
	}

	payload := pyyaml.Map{
		{Key: "slug", Value: d.Slug},
		{Key: "name", Value: d.Name},
		{Key: "status", Value: d.Status},
		{Key: "stage", Value: d.Stage},
		{Key: "commander", Value: stringList(d.Commander)},
	}
	// Written only when it says no, for the same reason `commander_art` is
	// written only when set: absent already means shared, so emitting
	// `shared: true` would rewrite all six curated files to assert the
	// default they already had.
	if !d.Shared {
		payload = append(payload, pyyaml.Pair{Key: "shared", Value: false})
	}
	if d.Pilot != "" {
		payload = append(payload, pyyaml.Pair{Key: "pilot", Value: d.Pilot})
	}
	if d.CommanderArt != "" {
		payload = append(payload, pyyaml.Pair{Key: "commander_art", Value: d.CommanderArt})
	}
	if d.Companion != nil && *d.Companion != "" {
		payload = append(payload, pyyaml.Pair{Key: "companion", Value: *d.Companion})
	}
	if d.Bracket != nil {
		payload = append(payload, pyyaml.Pair{Key: "bracket", Value: *d.Bracket})
	}
	// The pre-ADR-37 declared class, written back only while it is
	// load-bearing: once the themes name a class word the key is shadowed and
	// the next write drops it. The value is preserved even when it is not a
	// class the boards know -- a round trip must not eat a line `validate` is
	// still warning about.
	if d.LegacyArchetype != "" && !d.HasClassWord() {
		payload = append(payload, pyyaml.Pair{Key: "archetype", Value: d.LegacyArchetype})
	}
	if len(d.Themes) > 0 {
		payload = append(payload, pyyaml.Pair{Key: "themes", Value: stringList(d.Themes)})
	}
	if text, _ := d.Strategy.(string); text != "" {
		payload = append(payload, pyyaml.Pair{Key: "strategy", Value: text})
	}
	// The draft rule reaches the 99 and nothing else. `swap_board` and
	// `graveyard` are outside the deck -- the board is a list of cards under
	// consideration and the graveyard keeps the words a card already had -- so
	// neither gets the blank `why:` a draft writes as its own to-do list.
	payload = append(payload, pyyaml.Pair{Key: "cards", Value: cardList(d.Cards, d.Stage == "draft")})
	if len(d.SwapBoard) > 0 {
		payload = append(payload, pyyaml.Pair{
			Key: "swap_board", Value: cardList(d.SwapBoard, false)})
	}
	// Written only when occupied, like the swap board: an empty graveyard is
	// the normal state and six curated decks should not each grow a
	// `graveyard: []` line asserting it.
	if len(d.Graveyard) > 0 {
		payload = append(payload, pyyaml.Pair{
			Key: "graveyard", Value: cardList(d.Graveyard, false)})
	}
	return pyyaml.Dump(payload, DumpWidth)
}

// cardList is `[c.to_obj() for c in cards]`, with the draft rule applied.
func cardList(cards []CardEntry, draft bool) pyyaml.List {
	out := make(pyyaml.List, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardObject(c, draft))
	}
	return out
}

// cardObject is `CardEntry.to_obj`, and the draft rule on top of it.
//
// A blank `why:` is the to-do list written into the file itself, so the work
// shows up where it has to be done rather than only in the gate's output;
// omitted for a curated deck, where an empty rationale is a blocking error and
// should not be pre-typed.
//
// It lands **last** on a draft card, after `qty` and `art`, and that is not a
// tidy-looking accident: `to_obj` writes `why` in second place only when there
// is one, and Python's `setdefault` on a key the mapping has not got appends.
// A draft card with a quantity therefore reads `qty` before `why: ”`, which
// is the sort of thing only a corpus catches.
func cardObject(c CardEntry, draft bool) pyyaml.Map {
	obj := pyyaml.Map{{Key: "name", Value: c.Name}, {Key: "category", Value: c.Category}}
	if c.Why != "" {
		obj = append(obj, pyyaml.Pair{Key: "why", Value: c.Why})
	}
	if c.Qty != 1 {
		obj = append(obj, pyyaml.Pair{Key: "qty", Value: c.Qty})
	}
	if c.ScryfallID != nil && *c.ScryfallID != "" {
		obj = append(obj, pyyaml.Pair{Key: "scryfall_id", Value: *c.ScryfallID})
	}
	if c.ManaCost != nil && *c.ManaCost != "" {
		obj = append(obj, pyyaml.Pair{Key: "mana_cost", Value: *c.ManaCost})
	}
	if len(c.Tags) > 0 {
		obj = append(obj, pyyaml.Pair{Key: "tags", Value: stringList(c.Tags)})
	}
	if c.Art != "" {
		obj = append(obj, pyyaml.Pair{Key: "art", Value: c.Art})
	}
	if draft && c.Why == "" {
		obj = append(obj, pyyaml.Pair{Key: "why", Value: ""})
	}
	return obj
}

func stringList(items []string) pyyaml.List {
	out := make(pyyaml.List, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
