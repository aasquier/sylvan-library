// Package deckedit is `decks/edit.py`: surgical edits to a deck.yaml,
// preserving every byte they do not change.
//
// `deck.yaml` is the source of truth (ADR 1) and `swaps.md` is literally a
// diff of it -- against the last build's own snapshot, since ADR 30 took decks
// out of git and left no revision to diff. That makes the *size* of an edit
// part of its correctness: a one-card swap has to be a one-card diff, or the
// swap record it produces is unreadable.
//
// Load-and-dump cannot do that, which the Python module measured rather than
// assumed (829 changed lines on Goreclaw, and all eight comments gone). So
// this edits the *text*: it finds the lines belonging to one card entry and
// rewrites only those. ADR 12 has the full argument and the five rules every
// operation here obeys.
//
// **How an edit proves itself.** Each operation computes the document it
// *ought* to produce by mutating the parsed deck -- an ordinary map, no text
// involved -- and then refuses to return its text unless that text parses to
// exactly that document. The naive parse-mutate-dump is used as the oracle it
// is good at being, while the text surgery does the writing it is good at
// doing. An operation cannot quietly damage a neighbouring card, drop a note,
// or reorder the 99: any of those show up as a document mismatch and the edit
// is refused with nothing written.
//
// The failure this package must not have is silently corrupting the one file
// the whole project is built on, so it is checked rather than argued -- and
// the port carries a second check the original did not need: the lines it
// writes come from `internal/pyyaml`, which reproduces PyYAML's emitter byte
// for byte against a corpus Python generated. Same edit, same bytes, either
// runtime.
package deckedit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
	"github.com/aasquier/sylvan-library/go/internal/pyyaml"
)

// Failed is `EditFailed`: the edit could not be made safely, so nothing was
// changed. Every operation returns text rather than writing it, which is what
// makes that sentence true.
type Failed struct{ Reason string }

func (e *Failed) Error() string { return e.Reason }

func failf(format string, args ...any) error {
	return &Failed{Reason: fmt.Sprintf(format, args...)}
}

// IsFailed reports whether an error is a refusal to edit rather than a bug.
func IsFailed(err error) bool {
	var f *Failed
	return errors.As(err, &f)
}

// `  - name: Primeval Titan` -- the first line of a card entry. The name may
// be quoted, and a card name can contain almost anything, so the value is
// taken whole and compared after parsing rather than matched precisely here.
var (
	entryStart = regexp.MustCompile(`^(\s*)-\s+name:\s*(.*?)\s*$`)
	keyLine    = regexp.MustCompile(`^(\s*)([A-Za-z_][\p{L}\p{N}_-]*):(.*)$`)

	// `status: built  # the cards are sleeved up` -- a scalar with a trailing
	// comment. The comment is the author's, not the value's, so changing one
	// must not take the other.
	scalarLine = regexp.MustCompile(`^([A-Za-z_][\p{L}\p{N}_-]*):\s*([^#]*?)\s*(#.*)?$`)

	// A Scryfall printing id: a plain UUID. Checked by shape rather than
	// against the pool, because this package is pure text surgery over YAML
	// and reaching for DuckDB here would give the editor a database
	// dependency it has never had. The pool check happens one layer up.
	printingID = regexp.MustCompile(`^[0-9a-fA-F]{8}-(?:[0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$`)
)

// CardLists are the two lists a card can live in. `swap_board` is the bubble:
// cards kept just outside the 99 with the reason they did not make it.
var CardLists = []string{"cards", "swap_board"}

// Graveyard is where an entombed card waits (ADR 27). Deliberately not in
// CardLists: `locateCard` serves the editing operations, and a card in the
// graveyard is frozen -- it cannot have its `why` edited or its quantity
// changed until it is returned to the 99. The only operations that reach it
// are ReturnCard and ExileCard.
const Graveyard = "graveyard"

// SettableFields is what SetCardField will write. Deliberately short. `name`
// belongs to ReplaceCard, which also drops the overrides identifying the
// outgoing card; `scryfall_id` and `mana_cost` are overrides for cards the
// pool does not yet know, and hand-editing them through this path would mask
// a stale pool. `art` is the card's own `commander_art`: which printing's
// picture this deck shows for the slot, blank for the pool's default.
var SettableFields = []string{"category", "qty", "why", "art"}

// SettableDeckFields is what SetDeckField will write: the deck's own scalars.
// `strategy` and `notes` are prose and belong to SetNote; `commander` and
// `companion` change what the whole deck is legal to contain and are a
// rebuild, not a field edit. `themes` is the one non-scalar -- a list, but a
// list of vocabulary keys, so it edits like an enum with a plural rather than
// like the card blocks. `archetype` is deliberately absent: since ADR 37 it is
// a reading of the themes, and the way to change a reading is to change what
// it reads.
var SettableDeckFields = []string{"stage", "status", "bracket", "commander_art", "pilot", "themes"}

// PilotMax is the most anybody's name needs. A pilot is a person at a table,
// not a bio.
const PilotMax = 40

// deckKeyOrder is the order `Deck.dump` writes top-level keys in. Used to
// place a key the file does not have yet -- `stage` is absent from every deck
// written before ADR 13, and appending it to the bottom of the file would be
// legal YAML and unlike every deck in the repository.
var deckKeyOrder = []string{
	"slug", "name", "status", "stage", "commander", "shared", "pilot",
	"commander_art", "companion", "bracket", "archetype", "themes",
	"strategy", "notes", "cards", "swap_board", "graveyard",
}

// entry is one card's place in the file, in both encodings at once.
type entry struct {
	start      int // index of the `- name:` line
	end        int // index one past the entry's last line
	dashIndent int // columns before the `-`
	keyIndent  int // columns before `category:`, `why:` and friends
}

// ------------------------------------------------------------------ reading

// open parses the deck and splits its lines, the way every operation starts.
func open(text string) (map[string]any, []string, error) {
	doc, err := deckyaml.Parse([]byte(text))
	if err != nil {
		if strings.Contains(err.Error(), "not a mapping") {
			return nil, nil, failf("deck.yaml is not a mapping")
		}
		return nil, nil, failf("deck.yaml does not parse: %v", err)
	}
	return doc, strings.Split(text, "\n"), nil
}

// requiresRationale answers whether this deck's cards must each carry a `why`
// (rule 4, ADR 13). A curated deck must; a draft is honestly incomplete and
// owes them. Absent means curated, matching `Deck.from_text`, so an edit to
// one of the six existing decks is never quietly held to the looser standard.
func requiresRationale(doc map[string]any) bool {
	stage, _ := doc["stage"].(string)
	if strings.TrimSpace(stage) == "" {
		return true
	}
	return strings.ToLower(strings.TrimSpace(stage)) != "draft"
}

// blockHeader finds the index of a top-level key's own line.
//
// Matches an emptied list -- `swap_board: []` -- as well as a block, because a
// list this package emptied has to be one it can fill again.
func blockHeader(lines []string, key string) (int, error) {
	header := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*(\[\s*\])?\s*(#.*)?$`)
	for i, line := range lines {
		if header.MatchString(line) {
			return i, nil
		}
	}
	return 0, failf("no `%s:` block in this deck file", key)
}

// blockSpan is the line range of a top-level block's body, as [start, end).
//
// Ends at the next line in column zero, so the following key -- or a comment
// introducing it -- stays outside the block and out of reach of any edit.
func blockSpan(lines []string, key string) (int, int, error) {
	i, err := blockHeader(lines, key)
	if err != nil {
		return 0, 0, err
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) != "" && !startsWithSpace(lines[j]) {
			return i + 1, j, nil
		}
	}
	return i + 1, len(lines), nil
}

// topLevelSpan is the line range of a top-level key including its header, or
// ok=false.
//
// Unlike blockSpan this matches a scalar as well as a block, and the range
// starts at the header rather than after it -- because setting a deck's own
// field replaces the header line, where a card edit only ever touches what is
// underneath one.
func topLevelSpan(lines []string, key string) (start, end int, ok bool) {
	header := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:(\s|$)`)
	for i, line := range lines {
		if !header.MatchString(line) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) != "" && !startsWithSpace(lines[j]) {
				return i, j, true
			}
		}
		return i, len(lines), true
	}
	return 0, 0, false
}

// entrySpans finds every item in a sequence's line range, in document order.
//
// An entry runs to the start of the next one, so anything between two cards --
// Goreclaw's `# ---- RAMP 14` banners, blank lines -- lands in the earlier
// entry's span and is trimmed back off by splitTail before anything is
// written. Nothing between two cards is ever treated as part of either.
func entrySpans(lines []string, spanStart, spanStop int) []entry {
	dashIndent := -1
	var starts []int
	for i := spanStart; i < min(spanStop, len(lines)); i++ {
		match := entryStart.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		indent := len(match[1])
		if dashIndent < 0 {
			dashIndent = indent
		}
		if indent == dashIndent {
			starts = append(starts, i)
		}
	}
	if dashIndent < 0 {
		return nil
	}

	out := make([]entry, 0, len(starts))
	for n, i := range starts {
		end := spanStop
		if n+1 < len(starts) {
			end = starts[n+1]
		}
		// Read the key indent off the entry rather than assuming two spaces.
		keyIndent := dashIndent + 2
		for _, follower := range lines[i+1 : end] {
			if strings.TrimSpace(follower) != "" {
				keyIndent = indentOf(follower)
				break
			}
		}
		out = append(out, entry{start: i, end: end, dashIndent: dashIndent, keyIndent: keyIndent})
	}
	return out
}

// splitTail splits an entry's own lines from the gap that follows it.
//
// The gap is blank lines and comments sitting at or left of the key indent --
// a section banner introducing the *next* group of cards, and the blank line
// before it. They are inside the entry's line span only as an artefact of
// where the next entry starts, and no edit may touch them.
func splitTail(body []string, keyIndent int) (content, tail []string) {
	cut := len(body)
	for cut > 0 {
		line := body[cut-1]
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			cut--
			continue
		}
		if strings.HasPrefix(stripped, "#") && indentOf(line) <= keyIndent {
			cut--
			continue
		}
		break
	}
	return body[:cut], body[cut:]
}

// locateCard finds a card, agreeing on both its parsed position and its lines.
//
// Returning the two from a single lookup is what makes verification mean
// anything. If the document index and the text span could ever disagree -- a
// name appearing in both `cards` and `swap_board` is enough to do it -- an
// edit could check one entry and rewrite a different one, which is the exact
// failure the verification exists to catch.
//
// `lists` is which lists to search: the editing operations look through
// CardLists, the graveyard operations look only at the graveyard.
func locateCard(doc map[string]any, lines []string, name string, lists []string) (string, int, entry, error) {
	wanted := strings.ToLower(strings.TrimSpace(name))
	for _, key := range lists {
		items, ok := doc[key].([]any)
		if !ok {
			continue
		}
		start, stop, err := blockSpan(lines, key)
		if err != nil {
			continue
		}
		spans := entrySpans(lines, start, stop)
		if len(spans) != len(items) {
			// The parse and the text disagree about how many cards there are,
			// so no span can be trusted. ADR 12 anticipated this: the file
			// uses YAML this package does not handle -- an anchor, a flow
			// mapping, a bare string entry -- and the answer is to refuse,
			// not to guess.
			return "", 0, entry{}, countMismatch(key, len(items), len(spans))
		}
		for i, item := range items {
			card, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if strings.ToLower(strings.TrimSpace(asString(card["name"]))) == wanted {
				return key, i, spans[i], nil
			}
		}
	}
	return "", 0, entry{}, failf("no card entry named %s", pyRepr(name))
}

func countMismatch(key string, items, spans int) error {
	return failf("`%s` parses to %d entries but %d were found in the text; "+
		"this file uses YAML the editor cannot edit safely", key, items, spans)
}

// ------------------------------------------------------------------ writing

// render is `_render` at the deck files' own width.
func render(key string, value any, indent int) ([]string, error) {
	return pyyaml.Render(key, value, indent, pyyaml.RenderWidth, false)
}

// cardLines writes a whole card entry, in the key order the deck files use.
func cardLines(shape entry, name, category, why string, qty int) ([]string, error) {
	rendered, err := render("name", name, shape.keyIndent)
	if err != nil {
		return nil, err
	}
	out := []string{spaces(shape.dashIndent) + "- " + strings.TrimLeft(rendered[0], " ")}
	out = append(out, rendered[1:]...)

	block, err := render("category", category, shape.keyIndent)
	if err != nil {
		return nil, err
	}
	out = append(out, block...)

	if qty != 1 {
		block, err = render("qty", qty, shape.keyIndent)
		if err != nil {
			return nil, err
		}
		out = append(out, block...)
	}
	// Always written, even blank. In a draft the empty `why:` is the to-do
	// list recorded in the file itself, which is how `decks import` writes one
	// and where ADR 13 wants the outstanding work to be visible.
	block, err = render("why", why, shape.keyIndent)
	if err != nil {
		return nil, err
	}
	return append(out, block...), nil
}

// change is one instruction for rewriteEntry: drop the key, or write a value.
// A key absent from the change set is copied through untouched, which is the
// third state and needs no representation.
//
// Nothing here folds. `_render`'s `fold` argument defaults to False and only
// `SetNote` passes it -- a card's `why` is written as PyYAML's own choice at
// width 96, which for a long rationale is a single-quoted scalar broken across
// lines rather than a `>` block. That is what the six decks are written in.
type change struct {
	drop  bool
	value any
}

func set(value any) change { return change{value: value} }
func drop() change         { return change{drop: true} }

// rewriteEntry rewrites one entry, applying `changes` key by key.
//
// A key mapped to drop() is deleted; a key in `changes` that the entry does
// not have yet is appended. Everything else is copied verbatim, including the
// continuation lines of folded scalars that were not touched.
//
// `order` is the changed keys in the order they must be appended when absent,
// because Go maps do not keep one and a deck file's key order is part of what
// an edit must not disturb.
func rewriteEntry(lines []string, e entry, changes map[string]change, order []string) ([]string, error) {
	body, tail := splitTail(lines[e.start:e.end], e.keyIndent)
	var rebuilt []string
	seen := map[string]bool{}
	// True while walking the continuation lines of a key that was rewritten or
	// dropped -- a folded `why: >` owns every line indented beneath it, and
	// keeping those would strand the old rationale under the new one.
	dropping := false

	for offset, line := range body {
		if offset == 0 {
			seen["name"] = true
			c, ok := changes["name"]
			if !ok {
				rebuilt = append(rebuilt, line)
				continue
			}
			rendered, err := render("name", c.value, e.keyIndent)
			if err != nil {
				return nil, err
			}
			// Re-attach the dash, which the renderer knows nothing about.
			rebuilt = append(rebuilt, spaces(e.dashIndent)+"- "+strings.TrimLeft(rendered[0], " "))
			rebuilt = append(rebuilt, rendered[1:]...)
			dropping = true
			continue
		}

		if match := keyLine.FindStringSubmatch(line); match != nil && len(match[1]) == e.keyIndent {
			key := match[2]
			seen[key] = true
			dropping = false
			c, ok := changes[key]
			switch {
			case !ok:
				rebuilt = append(rebuilt, line)
			case c.drop:
				dropping = true
			default:
				rendered, err := render(key, c.value, e.keyIndent)
				if err != nil {
					return nil, err
				}
				rebuilt = append(rebuilt, rendered...)
				dropping = true
			}
			continue
		}

		// Only lines indented past the key can belong to the value being
		// replaced. A comment at or left of the key indent is the file's own,
		// so it survives even in the middle of a rewritten entry.
		if dropping && (strings.TrimSpace(line) == "" || indentOf(line) > e.keyIndent) {
			continue
		}
		dropping = false
		rebuilt = append(rebuilt, line)
	}

	for _, key := range order {
		c := changes[key]
		if seen[key] || c.drop {
			continue
		}
		rendered, err := render(key, c.value, e.keyIndent)
		if err != nil {
			return nil, err
		}
		rebuilt = append(rebuilt, rendered...)
	}

	return append(rebuilt, tail...), nil
}

// ------------------------------------------------------------- verification

// verified hands back the edited text only if it means exactly what it should.
//
// `expected` is the deck as an ordinary map, mutated the obvious way. Any
// difference at all -- a neighbouring card damaged, a note lost, the 99
// reordered, a folded scalar's tail stranded -- fails here, and a refused edit
// has changed nothing because these functions return text rather than writing
// it.
func verified(updated string, expected map[string]any) (string, error) {
	after, err := deckyaml.Parse([]byte(updated))
	if err != nil {
		return "", failf("the edit produced YAML that no longer parses: %v", err)
	}
	if !equalDocs(after, expected) {
		return "", failf("the edit changed more than it was asked to: %s",
			firstDifference(expected, after, ""))
	}
	return updated, nil
}

func spaces(n int) string { return strings.Repeat(" ", n) }

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t\n\v\f\r"))
}

func startsWithSpace(line string) bool {
	if line == "" {
		return false
	}
	switch line[0] {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
