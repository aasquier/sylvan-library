// Package deckimport turns a pasted decklist into a draft `deck.yaml`.
//
// The parser (`internal/decklist`) reads lines. This resolves what those lines
// *mean* against the pool and writes a deck file. Split that way because
// resolution is the half with an opinion, and the opinion is short:
//
// **Nothing is guessed.** Rule 1 says never evaluate a card from memory, and
// the same discipline applies to a name: a line that does not resolve is
// reported with what was written, kept in the deck verbatim so the list stays
// the size the user pasted, and left for the gate to flag as `unknown-card`.
// Dropping it would quietly hand back a 96-card deck.
//
// **Nothing is invented.** Every card arrives with an empty `why` and the deck
// is written as `stage: draft`, so the gate reports those as warnings and
// counts them (ADR 13). A rationale written by the tool is precisely the empty
// justification rule 4 exists to prevent, and doing it 99 times at once is
// worse than doing it once (ADR 11).
//
// **One thing is inferred, and only because it is a card pool fact.** A card
// is filed under `land` when `CardRecord.IsLand` says so -- which is right
// about the double-faced cards a type line is wrong about -- and everything
// else takes the model's `utility` default for a human to file.
package deckimport

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Header is the comment block every imported file opens with. `{today}` is
// filled at build time.
const Header = `# Imported %s from a pasted decklist, and NOT yet reasoned about.
#
# ` + "`stage: draft`" + ` means the gate reports every missing ` + "`why`" + ` as a warning
# instead of an error, so the deck's *facts* -- legality, colour identity,
# singleton, size -- are checked from day one while the thinking is still
# owed. Artifacts stay blocked until that work is done.
#
# To promote it: write a ` + "`why`" + ` on every card, then set ` + "`stage: curated`" + `. The
# gate refuses the promotion while any card is still blank, and nothing here
# will fill one in for you -- a rationale written by the tool is exactly the
# empty justification the rule exists to prevent.
`

// Refused is `ImportRefused`: the list could not be turned into a deck, and
// nothing was written.
type Refused struct{ Reason string }

func (e *Refused) Error() string { return e.Reason }

func refusef(format string, args ...any) error {
	return &Refused{Reason: fmt.Sprintf(format, args...)}
}

// IsRefused reports whether err is the import's own refusal.
func IsRefused(err error) bool {
	var refused *Refused
	return errors.As(err, &refused)
}

// Report is what the import did, and what it could not do.
//
// Deliberately verbose. An import that half-worked and said nothing is how a
// deck ends up with three cards spelled wrong and a commander in the 99.
type Report struct {
	Deck *deck.Deck
	YAML string
	// Names the pool does not have. Kept in the deck; reported here and by the
	// gate as `unknown-card`.
	Unknown []string
	// Lines the parser could not read at all.
	Unreadable []decklist.Line
	// Lines under a section that is not part of the deck, e.g. Tokens.
	Skipped []decklist.Line
	// Things that were changed rather than rejected, each worth saying aloud.
	Notes []string
}

// NeedsRationale is how many cards are still waiting for one.
func (r *Report) NeedsRationale() int { return len(r.Deck.Unjustified()) }

// NamesIn is every name that needs a card pool lookup, so the caller can fetch
// once.
func NamesIn(parsed decklist.List, commander []string, companion string) []string {
	seen := map[string]bool{}
	names := []string{}
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, c := range parsed.Cards {
		add(c.Name)
	}
	for _, c := range commander {
		add(c)
	}
	add(companion)
	slices.Sort(names)
	return names
}

// Options are `build_deck`'s keyword arguments.
type Options struct {
	Slug      string
	Name      string
	Commander []string
	Companion string
	Bracket   *int
	Status    string
}

// BuildDeck resolves a parsed list into a draft deck.
//
// `cards` is a name -> record mapping, as `pool.GetCards` returns over
// `NamesIn`. Passing an empty mapping is legal and means every name is
// reported as unknown, which is what a fresh clone with no card pool gets --
// honestly useless rather than silently wrong.
func BuildDeck(parsed decklist.List, cards map[string]*pool.CardRecord,
	opts Options) (*Report, error) {

	notes := []string{}
	unknown := []string{}

	resolve := func(written string) (string, *pool.CardRecord) {
		canonical, rec := canonicalName(written, cards)
		if rec == nil && !slices.Contains(unknown, canonical) {
			unknown = append(unknown, canonical)
		}
		return canonical, rec
	}

	// ---- the command zone ------------------------------------------------
	// An explicit commander wins over whatever the list claimed: the caller
	// knows which deck this is and the exporter only knows what it wrote.
	wanted := []string{}
	for _, c := range opts.Commander {
		if strings.TrimSpace(c) != "" {
			wanted = append(wanted, c)
		}
	}
	if len(wanted) == 0 {
		wanted = parsed.Commander()
	}
	if len(wanted) == 0 {
		return nil, &Refused{Reason: noCommanderMessage(parsed)}
	}
	if len(wanted) > 2 {
		return nil, refusef("%d commanders listed (%s); Commander allows at most "+
			"two, and only with a pairing ability",
			len(wanted), strings.Join(wanted, ", "))
	}

	commanders := make([]string, 0, len(wanted))
	for _, c := range wanted {
		name, _ := resolve(c)
		commanders = append(commanders, name)
	}
	companion := ""
	if strings.TrimSpace(opts.Companion) != "" {
		companion, _ = resolve(strings.TrimSpace(opts.Companion))
	} else if parsed.Companion() != "" {
		companion, _ = resolve(parsed.Companion())
	}

	outside := map[string]bool{}
	for _, c := range commanders {
		outside[strings.ToLower(c)] = true
	}
	if companion != "" {
		outside[strings.ToLower(companion)] = true
	}

	// ---- the 99 ----------------------------------------------------------
	// A list can nominate a commander or companion that the caller then
	// overrides. Those cards are still cards, and dropping them because of a
	// section header they were filed under would quietly hand back a 98-card
	// deck -- so anything the command zone did not take falls into the 99.
	nominated := append(parsed.Section("commander"), parsed.Section("companion")...)
	demoted := []string{}
	demotedLines := []decklist.Card{}
	for _, line := range nominated {
		canonical, _ := canonicalName(line.Name, cards)
		if outside[strings.ToLower(canonical)] {
			continue
		}
		demotedLines = append(demotedLines, line)
		if !slices.Contains(demoted, line.Name) {
			demoted = append(demoted, line.Name)
		}
	}
	if len(demotedLines) > 0 {
		slices.Sort(demoted)
		notes = append(notes, fmt.Sprintf(
			"%d card(s) the list nominated for the command zone were not chosen, "+
				"and went into the 99: %s", len(demotedLines), strings.Join(demoted, ", ")))
	}

	entries, moved := buildEntries(append(parsed.Section("deck"), demotedLines...),
		resolve, outside, &notes)
	swaps, alsoMoved := buildEntries(parsed.Section("swap_board"), resolve, outside, &notes)

	// Lists that mark the commander inline really do have 100 lines, and a
	// commander given with `--commander` is usually still sitting in the
	// sideboard section -- that is where our own moxfield.txt artifact puts it.
	lifted := slices.Clone(moved)
	for _, n := range alsoMoved {
		if !slices.Contains(lifted, n) {
			lifted = append(lifted, n)
		}
	}
	if len(lifted) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d card(s) sit outside the 99 and were removed from it: %s",
			len(lifted), strings.Join(lifted, ", ")))
	}

	if companion == "" {
		notes = append(notes, companionHints(swaps, cards)...)
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = opts.Slug
	}
	var companionPtr *string
	if companion != "" {
		value := companion
		companionPtr = &value
	}
	built := &deck.Deck{
		Slug:      opts.Slug,
		Name:      name,
		Status:    opts.Status,
		Stage:     "draft",
		Shared:    true,
		Commander: commanders,
		Companion: companionPtr,
		Bracket:   opts.Bracket,
		Cards:     entries,
		SwapBoard: swaps,
	}
	body, err := built.Dump()
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf(Header, time.Now().Format("2006-01-02")) + "\n" + body

	return &Report{Deck: built, YAML: text, Unknown: unknown,
		Unreadable: parsed.Unreadable, Skipped: parsed.Skipped, Notes: notes}, nil
}

// buildEntries resolves parsed lines into card entries, merging repeated
// names.
func buildEntries(lines []decklist.Card,
	resolve func(string) (string, *pool.CardRecord),
	outside map[string]bool, notes *[]string) ([]deck.CardEntry, []string) {

	order := []string{}
	byKey := map[string]*deck.CardEntry{}
	removed := []string{}

	for _, line := range lines {
		canonical, rec := resolve(line.Name)
		key := strings.ToLower(canonical)
		if outside[key] {
			// The card is in the command zone, so it is not in the 99. Lists
			// that mark the commander inline really do have 100 lines.
			if !slices.Contains(removed, canonical) {
				removed = append(removed, canonical)
			}
			continue
		}
		if existing, ok := byKey[key]; ok {
			existing.Qty += line.Qty
			*notes = append(*notes, fmt.Sprintf(
				"%s appeared on more than one line; merged to qty %d",
				canonical, existing.Qty))
			continue
		}
		// The only inference, and only because `IsLand` is a card pool fact.
		category := "utility"
		if rec != nil && rec.IsLand() {
			category = "land"
		}
		byKey[key] = &deck.CardEntry{Name: canonical, Category: category, Qty: line.Qty}
		order = append(order, key)
	}

	entries := make([]deck.CardEntry, 0, len(order))
	for _, key := range order {
		entries = append(entries, *byKey[key])
	}
	return entries, removed
}

// canonicalName is the name as the pool spells it, and the record behind it.
//
// Casing is corrected, but a double-faced card written by its front face stays
// written that way. `GetCards` resolves both, the curated decks all use face
// names ("Branchloft Pathway"), and expanding one to "Branchloft Pathway //
// Boulderloft Pathway" on import would make the library inconsistent for no
// gain.
func canonicalName(written string, cards map[string]*pool.CardRecord) (string, *pool.CardRecord) {
	rec, ok := cards[written]
	if !ok || rec == nil {
		return written, nil
	}
	for _, face := range strings.Split(rec.Name, " // ") {
		if strings.EqualFold(face, strings.TrimSpace(written)) {
			return face, rec
		}
	}
	return rec.Name, rec
}

// companionHints points out a companion sitting on the swap board, without
// assuming one.
//
// Our own moxfield.txt puts the commander *and* the companion under
// `SIDEBOARD:`, so re-importing an exported deck strands the companion there.
// Having a Companion ability is a card pool fact and worth reporting;
// concluding that this deck runs it as its companion is a judgement, and the
// card is perfectly capable of being an ordinary creature in the 99.
func companionHints(swaps []deck.CardEntry, cards map[string]*pool.CardRecord) []string {
	found := []string{}
	for _, e := range swaps {
		if rec, ok := cards[e.Name]; ok && rec != nil && gate.IsCompanion(rec) {
			found = append(found, e.Name)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return []string{strings.Join(found, ", ") + " has a Companion ability and is " +
		"on the swap board. Pass a companion explicitly if this deck runs one -- " +
		"it changes what the gate checks, so it is not assumed."}
}

// noCommanderMessage says what to do next, with the candidates the list
// actually contains.
func noCommanderMessage(parsed decklist.List) string {
	message := "no commander in this list, and none was given. Add a " +
		"`Commander` section to the list, or name one explicitly"
	// Moxfield's Commander import reads the commander out of `SIDEBOARD:`, and
	// our own moxfield.txt artifact writes it there -- so the swap board is
	// where a re-imported deck's commander actually is. Say so rather than
	// picking one, which would be a guess between two legendary creatures.
	board := []string{}
	for _, c := range parsed.Section("swap_board") {
		board = append(board, c.Name)
	}
	if len(board) > 0 {
		message += ". This list has a sideboard section containing " +
			strings.Join(board, ", ") + " -- Moxfield uses `SIDEBOARD:` to carry " +
			"the commander, so that may be what you want"
	}
	return message + "."
}
