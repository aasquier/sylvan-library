package deck

import (
	"fmt"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
	"github.com/aasquier/sylvan-library/go/internal/yamlemit"
)

// DumpWidth is the whole-file wrap width, 100. The surgical editor
// (`deckedit`) wraps the lines it rewrites at 96; the two differ because
// they always have, and a deck written at one width and edited at the other
// is the ordinary state of every file in the library.
const DumpWidth = 100

// Dump renders the whole file, in the recorded emitter shape (`yamlemit`).
//
// The write path is surgical everywhere else (ADR 12), so this is reached by
// exactly two lifecycle callers and both of them are writing a file that
// does not exist yet: a deck created and a deck imported. That is the whole
// justification for a second way to produce deck YAML -- a deck being born
// has no comments to destroy, no folded blocks to reflow and no diff to keep
// small.
//
// **There is a third caller now, and it is the reason the notes are ordered.**
// This function used to refuse a deck carrying notes outright: the emitter
// never sorts, so the payload's order *is* the file's order, the package
// held notes in a bare map, and neither lifecycle caller could reach the
// case -- a created deck has no notes and an imported one carries none
// across. The refusal named its own expiry ("the day something dumps a
// parsed deck -- the artifacts snapshot is the candidate -- ordering the
// notes is that flip's first job"), and the artifacts flip is that day.
// `deckyaml.ParseOrdered` keeps the order and `Deck.Notes` is a
// `deckyaml.Map`, so a note mapping now survives parse -> mutate -> dump
// in the order the author wrote.
func (d *Deck) Dump() (string, error) {
	if _, ok := d.Strategy.(string); !ok && d.Strategy != nil {
		// Same argument one field over: `strategy` is a string in every deck
		// the model describes, but a hand-written file may hold anything and
		// `FromText` passes it through. A mapping or a list would need the
		// same ordering this cannot give.
		return "", fmt.Errorf("deck %q has a strategy that is not prose, "+
			"which a whole-file dump cannot write", d.Slug)
	}

	payload := yamlemit.Map{
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
		payload = append(payload, yamlemit.Pair{Key: "shared", Value: false})
	}
	if d.Pilot != "" {
		payload = append(payload, yamlemit.Pair{Key: "pilot", Value: d.Pilot})
	}
	if d.CommanderArt != "" {
		payload = append(payload, yamlemit.Pair{Key: "commander_art", Value: d.CommanderArt})
	}
	if d.Companion != nil && *d.Companion != "" {
		payload = append(payload, yamlemit.Pair{Key: "companion", Value: *d.Companion})
	}
	if d.Bracket != nil {
		payload = append(payload, yamlemit.Pair{Key: "bracket", Value: *d.Bracket})
	}
	// The pre-ADR-37 declared class, written back only while it is
	// load-bearing: once the themes name a class word the key is shadowed and
	// the next write drops it. The value is preserved even when it is not a
	// class the boards know -- a round trip must not eat a line `validate` is
	// still warning about.
	if d.LegacyArchetype != "" && !d.HasClassWord() {
		payload = append(payload, yamlemit.Pair{Key: "archetype", Value: d.LegacyArchetype})
	}
	if len(d.Themes) > 0 {
		payload = append(payload, yamlemit.Pair{Key: "themes", Value: stringList(d.Themes)})
	}
	if text, _ := d.Strategy.(string); text != "" {
		payload = append(payload, yamlemit.Pair{Key: "strategy", Value: text})
	}
	if len(d.Notes) > 0 {
		notes, err := yamlNode(d.Notes)
		if err != nil {
			return "", fmt.Errorf("deck %q: notes: %w", d.Slug, err)
		}
		payload = append(payload, yamlemit.Pair{Key: "notes", Value: notes})
	}
	// The draft rule reaches the 99 and nothing else. `swap_board` and
	// `graveyard` are outside the deck -- the board is a list of cards under
	// consideration and the graveyard keeps the words a card already had -- so
	// neither gets the blank `why:` a draft writes as its own to-do list.
	payload = append(payload, yamlemit.Pair{Key: "cards", Value: cardList(d.Cards, d.Stage == "draft")})
	if len(d.SwapBoard) > 0 {
		payload = append(payload, yamlemit.Pair{
			Key: "swap_board", Value: cardList(d.SwapBoard, false)})
	}
	// Written only when occupied, like the swap board: an empty graveyard is
	// the normal state and six curated decks should not each grow a
	// `graveyard: []` line asserting it.
	if len(d.Graveyard) > 0 {
		payload = append(payload, yamlemit.Pair{
			Key: "graveyard", Value: cardList(d.Graveyard, false)})
	}
	return yamlemit.Dump(payload, DumpWidth)
}

// cardList is `[c.to_obj() for c in cards]`, with the draft rule applied.
func cardList(cards []CardEntry, draft bool) yamlemit.List {
	out := make(yamlemit.List, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardObject(c, draft))
	}
	return out
}

// cardObject renders one card entry, with the draft rule on top.
//
// A blank `why:` is the to-do list written into the file itself, so the work
// shows up where it has to be done rather than only in the gate's output;
// omitted for a curated deck, where an empty rationale is a blocking error and
// should not be pre-typed.
//
// It lands **last** on a draft card, after `qty` and `art`, and that is not a
// tidy-looking accident: `why` is written in second place only when there is
// one, and the draft's blank entry is appended after every present field.
// A draft card with a quantity therefore reads `qty` before `why: ”`, which
// is the sort of thing only a corpus catches.
func cardObject(c CardEntry, draft bool) yamlemit.Map {
	obj := yamlemit.Map{{Key: "name", Value: c.Name}, {Key: "category", Value: c.Category}}
	if c.Why != "" {
		obj = append(obj, yamlemit.Pair{Key: "why", Value: c.Why})
	}
	// Immediately after the sentence it is about, because provenance that
	// sits six keys away from its text is provenance nobody reads. Absent
	// unless something drafted the rationale, so no existing deck file moves
	// a byte.
	if c.WhyBy != "" {
		obj = append(obj, yamlemit.Pair{Key: "why_by", Value: c.WhyBy})
	}
	if c.Qty != 1 {
		obj = append(obj, yamlemit.Pair{Key: "qty", Value: c.Qty})
	}
	if c.ScryfallID != nil && *c.ScryfallID != "" {
		obj = append(obj, yamlemit.Pair{Key: "scryfall_id", Value: *c.ScryfallID})
	}
	if c.ManaCost != nil && *c.ManaCost != "" {
		obj = append(obj, yamlemit.Pair{Key: "mana_cost", Value: *c.ManaCost})
	}
	if len(c.Tags) > 0 {
		obj = append(obj, yamlemit.Pair{Key: "tags", Value: stringList(c.Tags)})
	}
	if c.Art != "" {
		obj = append(obj, yamlemit.Pair{Key: "art", Value: c.Art})
	}
	if draft && c.Why == "" {
		obj = append(obj, yamlemit.Pair{Key: "why", Value: ""})
	}
	return obj
}

// yamlNode carries a parsed value across into the emitter's node model: an
// ordered mapping becomes a `yamlemit.Map`, a sequence a `yamlemit.List`, and a
// scalar is left for `scalarOf` to judge.
//
// The two are separate types on purpose -- one is what a file said, the other
// is what a dumper is about to write -- and this is the seam. It exists for
// `notes:` alone, which is the only mapping in a deck whose *shape* is the
// author's rather than the model's: everything else `Dump` writes it builds
// itself, key by key, from a typed field.
//
// A value the emitter cannot write comes back as an error rather than as
// something plausible, the same answer `Dump` already gives a strategy that
// is not prose -- and the route renders it as a 500, which is the honest
// answer for a hand-edited file nobody meant to write.
func yamlNode(v any) (any, error) {
	switch t := v.(type) {
	case deckyaml.Map:
		out := make(yamlemit.Map, 0, len(t))
		for _, p := range t {
			value, err := yamlNode(p.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Key, err)
			}
			out = append(out, yamlemit.Pair{Key: p.Key, Value: value})
		}
		return out, nil
	case []any:
		out := make(yamlemit.List, 0, len(t))
		for i, item := range t {
			value, err := yamlNode(item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out = append(out, value)
		}
		return out, nil
	case string, int, int64, bool:
		return t, nil
	default:
		return nil, fmt.Errorf("a %T cannot be written back into a deck file", v)
	}
}

func stringList(items []string) yamlemit.List {
	out := make(yamlemit.List, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
