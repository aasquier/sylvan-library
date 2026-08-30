// Package deck is the deck file, parsed. `deck.yaml` is the source of
// truth, and since ADR 30 the app's own data rather than anything git
// tracks; the five deliverables are generated from it and the activity log
// is its history. The app reads a deck to validate it, to analyse it, to
// render it to the wire and to compare it against the last build's
// snapshot.
//
// **Parsed, and in one place written** -- `dump.go`, which arrived with Phase
// 4 and which this comment said would never exist. ADR 12's surgery is still
// text surgery and still lives in `deckedit`, so `Dump` is not a general way
// to write a deck: it serves the three callers that produce a *whole* file
// (a deck created, a deck imported, and the artifacts snapshot), and `Payload`
// beside it stays a projection to compare rather than a second dumper.
//
// `FromText` reads the file field for field, default for default: a missing
// `status` is theoretical and a missing `stage` is curated (opposite
// defaults on purpose -- an undeclared power claim should undersell, an
// undeclared maturity should not demote a finished deck), `shared` is true
// unless the file says no, themes are lowered and stripped, a bare string
// in `cards:` is a card filed under utility. A file that will not parse is
// an error, and the handler answers it as a 500 in the envelope rather than
// inventing a deck.
package deck

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// CardEntry is one card in the 99 (or the command zone, or the swap board,
// or the graveyard).
type CardEntry struct {
	Name       string
	Category   string
	Why        string
	Qty        int
	ScryfallID *string
	ManaCost   *string
	Tags       []string
	// Which printing's art this deck shows for the card, or "" for the
	// pool's default.
	Art string
	// WhyBy is who wrote the rationale, and it is only ever `claude` or empty
	// (ADR 41). Empty means a person did, which is true of every rationale
	// written before the intake existed and of every one typed since.
	//
	// It is provenance rather than a warning label, and it is load-bearing:
	// Aaron ruled that a drafted rationale satisfies promotion to `curated`,
	// so this mark is the ONLY thing left carrying the difference between a
	// sentence somebody formed and a sentence somebody accepted. It comes off
	// the moment the text changes -- editing a draft is adopting it.
	WhyBy string
}

// Combo is one machine the deck can assemble: the cards it is made of, what
// it produces, how it is turned, and what it costs to set up.
//
// **There is no `name` field, and that is a decision.** A combo's name is the
// cards it is made of -- `Cards` joined with " + " -- which is how anybody who
// plays the deck refers to it and the one heading that cannot go stale when a
// piece is swapped. A separate title would be a second thing to keep true.
//
// **Terse by construction.** Four fields say what the machine is, two more
// mark the one that is a card short, and there is nothing else: table manners,
// lines to hold up, and when to go for it are the pilot's business and live in
// the deck's notes. A block that grew a seventh field would be a primer.
type Combo struct {
	// The pieces this deck already sleeves, by exact pool name. The heading.
	Cards []string
	// What it makes, in the player's own words: "infinite colored mana".
	Produces string
	// The instructions, numbered, in the order the hands do them.
	How string
	// What it takes to get there -- mana, turns, a creature that has to be
	// free of summoning sickness.
	Setup string
	// Needs is the one card this deck does not have yet. Its presence is what
	// makes an entry a near-miss rather than a machine, and it is why `Cut`
	// exists: Aaron's rule is that a suggestion the deck cannot act on is not
	// a suggestion, so a near-miss always names the trade.
	Needs string
	// Cut is the card in the 99 the near-miss would come in for.
	Cut string
	// By is who assembled this entry, and it is only ever `claude` or empty --
	// `CardEntry.WhyBy`'s rule, one block over (ADR 41).
	//
	// **Nothing in this phase writes it.** The field exists now so that the
	// intake's combos action lands into a shape that already carries
	// provenance rather than one that has to grow it later; a deck file may
	// still hold the mark, and it survives a round trip. It comes off the
	// moment a person changes what the entry says -- editing a draft is
	// adopting it -- and `deckedit.SetCombos` is where that happens, because a
	// mark the client could send is a mark the client could forge.
	By string
}

// Heading is what this combo is called: its pieces, joined. The one name it
// has, derived rather than stored, so it cannot disagree with the cards.
func (c Combo) Heading() string { return strings.Join(c.Cards, " + ") }

// NearMiss reports whether this entry is a card short of being a machine.
func (c Combo) NearMiss() bool { return strings.TrimSpace(c.Needs) != "" }

// Deck is the parsed file.
type Deck struct {
	Slug   string
	Name   string
	Status string
	Stage  string
	Shared bool
	// ColiseumAtNight is the owner's standing consent to send this deck down
	// to the arena after dark, once the night games begin.
	//
	// **Read from the row and never from the file, which is the one way it
	// differs from `Shared` above.** Aaron ruled it, and the ruling has a
	// consequence worth stating where the field is: `FromText` never sets
	// this, `Dump` and `Payload` never write it, so a deck that round-trips
	// through YAML loses it -- correctly, because the YAML was never where it
	// lived. Only `SQLSource` fills it in, from `user_decks.coliseum_at_night`
	// (rung 13). The file tier has no row to read, so its decks answer `false`
	// and its write verb refuses; `SetColiseumAtNight` argues that.
	ColiseumAtNight bool
	Pilot           string
	Commander       []string
	CommanderArt    string
	Companion       *string
	Bracket         *int
	LegacyArchetype string
	Themes          []string
	// Strategy and Notes pass through as the file holds them: a string (or
	// null, if a hand-edited file says `strategy:` and nothing), a mapping
	// of note keys to values. Neither is read by the gate.
	//
	// Notes keeps the file's order, which is the one thing about a deck that
	// is nobody's to reorder: these keys are the author's prose, and both
	// `Dump` and the wire write them back in the order they were read.
	Strategy  any
	Notes     deckyaml.Map
	Cards     []CardEntry
	SwapBoard []CardEntry
	Graveyard []CardEntry
	// The machines this deck can assemble, in the order the file lists them.
	// Order is the author's -- the first entry is the one they lead with -- so
	// nothing here sorts.
	Combos []Combo
}

// FromText parses deck YAML that is not necessarily a file. `slug` is the
// location's name -- the directory, or the row -- which wins over nothing
// and loses to the file's own `slug:`.
func FromText(text string, slug string) (*Deck, error) {
	// Ordered, and then flattened for everything except `notes:`. Every other
	// field is named here one at a time, so a map serves them; the notes are
	// the author's own keys and the snapshot `swaps.md` diffs against is a
	// dump of this parse, so their order has to survive the round trip.
	ordered, err := deckyaml.ParseOrdered([]byte(text))
	if err != nil {
		// An empty document is an empty deck by contract; goccy reports an
		// empty document as an error rather than a nil map, so the one
		// case is told apart here.
		if strings.TrimSpace(text) == "" {
			ordered = deckyaml.Map{}
		} else {
			return nil, err
		}
	}
	raw := ordered.Plain()
	d := &Deck{}
	d.Slug = firstString(raw["slug"], slug)
	d.Name = firstString(raw["name"], slug)
	d.Status = strings.ToLower(strings.TrimSpace(stringOr(raw["status"], "theoretical")))
	d.Stage = strings.ToLower(strings.TrimSpace(stringOr(raw["stage"], "curated")))
	if v, present := raw["shared"]; !present || v == nil {
		d.Shared = true
	} else {
		d.Shared = truthy(v)
	}
	d.Pilot = strings.TrimSpace(stringOr(raw["pilot"], ""))
	switch c := raw["commander"].(type) {
	case string:
		d.Commander = []string{c}
	case []any:
		d.Commander = make([]string, 0, len(c))
		for _, item := range c {
			d.Commander = append(d.Commander, fmt.Sprint(item))
		}
	default:
		d.Commander = []string{}
	}
	d.CommanderArt = strings.TrimSpace(stringOr(raw["commander_art"], ""))
	if v := raw["companion"]; v != nil {
		s := fmt.Sprint(v)
		d.Companion = &s
	}
	if v := raw["bracket"]; v != nil {
		switch b := v.(type) {
		case int64:
			n := int(b)
			d.Bracket = &n
		case float64:
			n := int(b)
			d.Bracket = &n
		case string:
			// A string bracket is read when it is a number in quotes and
			// dropped when it is not -- the typed field has nowhere to
			// keep prose, and the edge cannot matter: a deck file written
			// by the app never has one, and the gate reads `bracket` only
			// through analyze, which treats an unknown as unlimited.
			if n, err := strconv.Atoi(strings.TrimSpace(b)); err == nil {
				d.Bracket = &n
			}
		}
	}
	d.LegacyArchetype = strings.ToLower(strings.TrimSpace(stringOr(raw["archetype"], "")))
	d.Themes = []string{}
	if list, ok := raw["themes"].([]any); ok {
		for _, item := range list {
			t := strings.TrimSpace(fmt.Sprint(item))
			if t != "" {
				d.Themes = append(d.Themes, strings.ToLower(t))
			}
		}
	}
	if v, present := raw["strategy"]; present {
		d.Strategy = v
	} else {
		d.Strategy = ""
	}
	d.Notes = deckyaml.Map{}
	if notes, present := ordered.Get("notes"); present {
		if m, ok := notes.(deckyaml.Map); ok {
			d.Notes = append(d.Notes, m...)
		}
	}
	for _, section := range []struct {
		key  string
		into *[]CardEntry
	}{{"cards", &d.Cards}, {"swap_board", &d.SwapBoard}, {"graveyard", &d.Graveyard}} {
		*section.into = []CardEntry{}
		list, _ := raw[section.key].([]any)
		for i, item := range list {
			entry, err := cardFrom(item)
			if err != nil {
				return nil, fmt.Errorf("deck yaml: %s[%d]: %w", section.key, i, err)
			}
			*section.into = append(*section.into, entry)
		}
	}
	d.Combos = []Combo{}
	if list, ok := raw["combos"].([]any); ok {
		for i, item := range list {
			combo, err := comboFrom(item)
			if err != nil {
				return nil, fmt.Errorf("deck yaml: combos[%d]: %w", i, err)
			}
			d.Combos = append(d.Combos, combo)
		}
	}
	return d, nil
}

// comboFrom reads one entry of the `combos:` block.
//
// A mapping only. `cardFrom` next door accepts a bare string as a card filed
// under utility, because a hand-written deck list is a list of names and that
// shorthand is worth having; a combo has no such shorthand -- a bare string
// could be a card, a heading, or a sentence, and guessing which would file
// somebody's prose as a card name the gate then warns about.
func comboFrom(obj any) (Combo, error) {
	m, ok := obj.(map[string]any)
	if !ok {
		return Combo{}, fmt.Errorf("a combo is a mapping of cards, produces, how and setup, not %T", obj)
	}
	combo := Combo{Cards: []string{}}
	switch c := m["cards"].(type) {
	case string:
		// One card is still a list of pieces. Written by a person rather than
		// by the app, which is the only way this shape arrives.
		combo.Cards = []string{c}
	case []any:
		for _, item := range c {
			// A nil entry is a line somebody left blank -- `- ` with nothing
			// after it, which a hand-edited file grows the moment a piece is
			// deleted rather than removed. Skipped rather than rendered, because
			// `fmt.Sprint(nil)` is the string "<nil>" and that would become a
			// card name in a heading and a warning from the gate.
			if item == nil {
				continue
			}
			if name := strings.TrimSpace(fmt.Sprint(item)); name != "" {
				combo.Cards = append(combo.Cards, name)
			}
		}
	}
	combo.Produces = strings.TrimSpace(stringOr(m["produces"], ""))
	combo.How = strings.TrimSpace(stringOr(m["how"], ""))
	combo.Setup = strings.TrimSpace(stringOr(m["setup"], ""))
	combo.Needs = strings.TrimSpace(stringOr(m["needs"], ""))
	combo.Cut = strings.TrimSpace(stringOr(m["cut"], ""))
	combo.By = strings.TrimSpace(stringOr(m["by"], ""))
	return combo, nil
}

// ComboNames is every card name a combo block refers to, in reading order and
// deduplicated: the pieces, the card a near-miss needs, and the card it would
// cut.
//
// One list because there is one question behind all three -- does the pool
// know this name -- and both the gate and the wire ask it. A name is a name
// wherever it stands in the entry.
func (d *Deck) ComboNames() []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[strings.ToLower(name)] {
			return
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	for _, combo := range d.Combos {
		for _, name := range combo.Cards {
			add(name)
		}
		add(combo.Needs)
		add(combo.Cut)
	}
	return out
}

// cardFrom is `CardEntry.from_obj`.
func cardFrom(obj any) (CardEntry, error) {
	if s, ok := obj.(string); ok {
		return CardEntry{Name: s, Category: "utility", Qty: 1, Tags: []string{}}, nil
	}
	m, ok := obj.(map[string]any)
	if !ok {
		return CardEntry{}, fmt.Errorf("a card is a name or a mapping, not %T", obj)
	}
	name, ok := m["name"]
	if !ok {
		return CardEntry{}, fmt.Errorf("a card has no name")
	}
	entry := CardEntry{Name: fmt.Sprint(name), Category: "utility", Qty: 1, Tags: []string{}}
	if v, present := m["category"]; present {
		entry.Category = stringOr(v, "")
	}
	if v, present := m["why"]; present {
		entry.Why = stringOr(v, "")
	}
	if v, present := m["why_by"]; present {
		entry.WhyBy = stringOr(v, "")
	}
	if v, present := m["qty"]; present && v != nil {
		switch q := v.(type) {
		case int64:
			entry.Qty = int(q)
		case float64:
			entry.Qty = int(q)
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(q))
			if err != nil {
				return CardEntry{}, fmt.Errorf("%s: qty %q is not a number", entry.Name, q)
			}
			entry.Qty = n
		case bool:
			if q {
				entry.Qty = 1
			} else {
				entry.Qty = 0
			}
		default:
			return CardEntry{}, fmt.Errorf("%s: qty %v is not a number", entry.Name, v)
		}
	}
	if v := m["scryfall_id"]; v != nil {
		s := fmt.Sprint(v)
		entry.ScryfallID = &s
	}
	if v := m["mana_cost"]; v != nil {
		s := fmt.Sprint(v)
		entry.ManaCost = &s
	}
	if tags, ok := m["tags"].([]any); ok {
		for _, t := range tags {
			entry.Tags = append(entry.Tags, fmt.Sprint(t))
		}
	}
	if v := m["art"]; v != nil && truthy(v) {
		entry.Art = fmt.Sprint(v)
	}
	return entry, nil
}

// firstString is `raw.get(key) or fallback or ""`.
func firstString(v any, fallback string) string {
	if v != nil && truthy(v) {
		return fmt.Sprint(v)
	}
	return fallback
}

// stringOr renders the value, or the fallback for a key absent, null, or
// otherwise empty -- emptiness by `truthy`, not by string comparison.
func stringOr(v any, fallback string) string {
	if v == nil || !truthy(v) {
		return fallback
	}
	return fmt.Sprint(v)
}

// truthy is one emptiness rule over every value goccy hands over: nil,
// false, zero, "", and an empty container are empty; everything else is not.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int64:
		return t != 0
	case uint64:
		return t != 0
	case float64:
		return t != 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return true
}

// ---- derived views, as `Deck`'s properties ---------------------------------

// TotalCards is the 99 (commander and companion sit outside it).
func (d *Deck) TotalCards() int {
	n := 0
	for _, c := range d.Cards {
		n += c.Qty
	}
	return n
}

// Archetype is the rating boards' class, read from the declared themes
// (ADR 37): among the class words present, the worst-piloted wins; a deck
// declaring none falls back to the legacy key while its file still carries
// one, and otherwise has no board to sit on.
func (d *Deck) Archetype() string {
	best := -1
	for _, t := range d.Themes {
		if i := reference.ArchetypeIndex(t); i > best {
			best = i
		}
	}
	if best >= 0 {
		return reference.Themes().Archetypes[best]
	}
	if reference.ArchetypeIndex(d.LegacyArchetype) >= 0 {
		return d.LegacyArchetype
	}
	return ""
}

// CategoryCounts is `Deck.category_counts`, in first-seen order of category.
func (d *Deck) CategoryCounts() (map[string]int, []string) {
	counts := map[string]int{}
	order := []string{}
	for _, c := range d.Cards {
		if _, seen := counts[c.Category]; !seen {
			order = append(order, c.Category)
		}
		counts[c.Category] += c.Qty
	}
	return counts, order
}

// LandCount is `Deck.land_count`.
func (d *Deck) LandCount() int {
	counts, _ := d.CategoryCounts()
	return counts["land"]
}

// Unjustified is the cards with no `why` yet -- the work a draft still owes.
func (d *Deck) Unjustified() []CardEntry {
	out := []CardEntry{}
	for _, c := range d.Cards {
		if strings.TrimSpace(c.Why) == "" {
			out = append(out, c)
		}
	}
	return out
}

// CardNames is `Deck.card_names`: the commander(s) first when asked, then
// every card as many times as its quantity.
func (d *Deck) CardNames(includeCommander bool) []string {
	names := []string{}
	if includeCommander {
		names = append(names, d.Commander...)
	}
	for _, c := range d.Cards {
		for i := 0; i < c.Qty; i++ {
			names = append(names, c.Name)
		}
	}
	return names
}

// NoteText is `deck.notes.get(key)` for the values a note actually holds:
// prose, or nothing. A note that is present but is not a string reads as
// absent here, which is what the wire already did with
// `d.Notes["commander_why"].(string)` and its dropped second return.
//
// `artifacts` asks the same question a stricter way and refuses instead,
// because there the answer is a document rather than one field of a payload.
func (d *Deck) NoteText(key string) string {
	v, ok := d.Notes.Get(key)
	if !ok {
		return ""
	}
	text, _ := v.(string)
	return text
}

// HasClassWord reports whether any declared theme is one of the four class
// words -- the condition under which `dump` drops the legacy key.
func (d *Deck) HasClassWord() bool {
	for _, t := range d.Themes {
		if reference.ArchetypeIndex(t) >= 0 {
			return true
		}
	}
	return false
}

// ---- the dump projection -----------------------------------------------

// cardPayload is `CardEntry.to_obj`.
func cardPayload(c CardEntry, draft bool) map[string]any {
	out := map[string]any{"name": c.Name, "category": c.Category}
	if c.Why != "" {
		out["why"] = c.Why
	} else if draft {
		out["why"] = ""
	}
	// Only when there is one, so a deck nobody has run the intake over says
	// nothing about authorship rather than saying "not claude" ninety-nine
	// times.
	if c.WhyBy != "" {
		out["why_by"] = c.WhyBy
	}
	if c.Qty != 1 {
		out["qty"] = c.Qty
	}
	if c.ScryfallID != nil && *c.ScryfallID != "" {
		out["scryfall_id"] = *c.ScryfallID
	}
	if c.ManaCost != nil && *c.ManaCost != "" {
		out["mana_cost"] = *c.ManaCost
	}
	if len(c.Tags) > 0 {
		out["tags"] = append([]string{}, c.Tags...)
	}
	if c.Art != "" {
		out["art"] = c.Art
	}
	return out
}

// Payload is the mapping `Deck.dump` serialises, without the serialising:
// the same keys under the same conditions (shared only when false, pilot and
// art only when set, the legacy archetype only while unshadowed, a blank
// `why` written into a draft's cards), so two decks whose dumps would be the
// same text have equal payloads. `service._baseline_state` asks exactly that
// question of a deck and the last build's snapshot.
func (d *Deck) Payload() map[string]any {
	p := map[string]any{
		"slug": d.Slug, "name": d.Name, "status": d.Status, "stage": d.Stage,
		"commander": append([]string{}, d.Commander...),
	}
	if !d.Shared {
		p["shared"] = false
	}
	if d.Pilot != "" {
		p["pilot"] = d.Pilot
	}
	if d.CommanderArt != "" {
		p["commander_art"] = d.CommanderArt
	}
	if d.Companion != nil && *d.Companion != "" {
		p["companion"] = *d.Companion
	}
	if d.Bracket != nil {
		p["bracket"] = *d.Bracket
	}
	if d.LegacyArchetype != "" && !d.HasClassWord() {
		p["archetype"] = d.LegacyArchetype
	}
	if len(d.Themes) > 0 {
		p["themes"] = append([]string{}, d.Themes...)
	}
	if truthy(d.Strategy) {
		p["strategy"] = d.Strategy
	}
	if len(d.Notes) > 0 {
		// The order is part of the comparison, because it is part of the
		// text: the baseline check asks whether two dumps are the same
		// string, and two note mappings holding the same pairs in a
		// different order dump to different files. A bare map here would
		// have called that `current` and quietly said the artifacts were
		// up to date.
		p["notes"] = append(deckyaml.Map{}, d.Notes...)
	}
	draft := d.Stage == "draft"
	cards := make([]map[string]any, 0, len(d.Cards))
	for _, c := range d.Cards {
		cards = append(cards, cardPayload(c, draft))
	}
	p["cards"] = cards
	if len(d.SwapBoard) > 0 {
		board := make([]map[string]any, 0, len(d.SwapBoard))
		for _, c := range d.SwapBoard {
			board = append(board, cardPayload(c, false))
		}
		p["swap_board"] = board
	}
	if len(d.Graveyard) > 0 {
		yard := make([]map[string]any, 0, len(d.Graveyard))
		for _, c := range d.Graveyard {
			yard = append(yard, cardPayload(c, false))
		}
		p["graveyard"] = yard
	}
	// **Here for the same reason every other block is**, and it is worth
	// spelling out because the omission would be silent: `swaps.md` diffs a
	// deck against the last build's snapshot by asking whether these two
	// payloads are equal, so a block missing from this projection is a block
	// whose changes report the deck as unchanged. Catalogue three combos, run
	// a build, and the swap record would say nothing happened.
	if len(d.Combos) > 0 {
		machines := make([]map[string]any, 0, len(d.Combos))
		for _, combo := range d.Combos {
			machines = append(machines, comboPayload(combo))
		}
		p["combos"] = machines
	}
	return p
}

// comboPayload is one combo as `Dump` will write it: only the keys that have
// something to say, in the order the file puts them.
func comboPayload(c Combo) map[string]any {
	out := map[string]any{"cards": append([]string{}, c.Cards...)}
	if c.Needs != "" {
		out["needs"] = c.Needs
	}
	if c.Produces != "" {
		out["produces"] = c.Produces
	}
	if c.How != "" {
		out["how"] = c.How
	}
	if c.Setup != "" {
		out["setup"] = c.Setup
	}
	if c.Cut != "" {
		out["cut"] = c.Cut
	}
	if c.By != "" {
		out["by"] = c.By
	}
	return out
}

// SameAs reports whether two decks would dump to the same text -- the
// baseline comparison.
func (d *Deck) SameAs(other *Deck) bool {
	return reflect.DeepEqual(d.Payload(), other.Payload())
}
