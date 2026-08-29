// Package deckimport turns a pasted decklist into a draft `deck.yaml`.
//
// The parser (`internal/decklist`) reads lines. This resolves what those lines
// *mean* against the pool and writes a deck file. Split that way because
// resolution is the half with an opinion, and the opinion is short:
//
// **A name is READ against the pool, and every reading is stated.** This is
// the rule that changed on 2026-08-24, and it is worth writing down carefully
// because the sentence it replaces said the opposite.
//
// It used to be "nothing is guessed": a line that did not resolve was kept
// verbatim and reported as `unknown-card`, full stop. The reasoning was rule
// 1 -- never evaluate a card from memory -- and the reasoning was sound but
// the conclusion was too wide. Rule 1 forbids *recall*. It does not forbid
// asking the pool, and asking the pool which of its 35,393 names is nearest
// to `Sol Rng` is a measurement, not a memory: the same jaro-winkler the
// camera door has scored titles with since ADR 34.
//
// So a name the pool does not know is looked up, and when one card is clearly
// what was meant -- `nearFloor` and `nearLead` below, both measured -- the
// deck gets that card, and the import SAYS SO, by name, in `Report.Read`.
// Aaron's ruling (2026-08-24): "we should do the fuzzy matching on the
// backend, not allow misspelled things in." A deck of real cards that reports
// four corrections is worth more than a deck of four broken strings that
// reports four errors, and the person still sees exactly what was decided.
//
// What has NOT changed is the floor under it. A reading that is not clear is
// not made: the name is kept in the deck exactly as written so the list stays
// the size the user pasted, and it is reported for the gate to flag as
// `unknown-card`. Dropping it would quietly hand back a 96-card deck, and
// silently *substituting* for it would be worse -- which is why the bar is
// high enough that no measured typo has ever come near failing it and no
// measured non-word has ever come near passing it.
//
// **Nothing is invented, and the rationale column is not an exception.** A
// pasted line may carry its own reason -- `1 Acidic Slime (ZNC) 59 "deathtouch
// body that also eats artifacts"` -- and that reason is written into the
// deck's `why` verbatim. This is not the tool filling a field in: it is a
// person writing their own rationale in the only place the exports left free,
// which is the same act as typing it into the deck editor and is what rule 4
// asks for rather than something it forbids. What would break the rule is
// composing one, and nothing here composes anything. A card that arrived
// without a quoted reason still arrives with an empty `why`.
//
// The deck is written as `stage: draft` either way, including when every line
// carried a reason. Draft is not only a statement about empty fields -- it is
// the deck saying nobody has looked at it in one piece yet -- and the gate
// reports a draft's problems as warnings instead of errors (ADR 13), which is
// the diagnosis a newcomer wants on the first screen rather than a refusal.
// Promotion is one control away on the deck page, and the import says so when
// there is nothing left owed.
//
// **The reading of the column takes a card pool, so it happens here.** Twelve
// card names contain a quote and five end in one, so `decklist` hands up both
// readings of a line like `1 Kongming, "Sleeping Dragon"` and `ReadRationales`
// asks which one the pool has. Measured against this pool, not one of the five
// peels to a name the pool also knows, so the question has a single answer
// every time it is asked.
//
// **One thing is inferred, and only because it is a card pool fact.** A card
// is filed under `land` when `CardRecord.IsLand` says so -- which is right
// about the double-faced cards a type line is wrong about -- and everything
// else takes the model's `utility` default for a human to file.
package deckimport

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The comment block every imported file opens with, in its two forms. The
// deck file is the truth, so the truth is what the header states: a list
// pasted with its reasons already written did not arrive unreasoned about,
// and telling its owner that nothing here will fill a `why` in for them would
// be answering a question they did not ask.
const (
	headerBare = `# Imported %s from a pasted decklist, and NOT yet reasoned about.
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

	headerReasoned = `# Imported %s from a pasted decklist, with %d of its reasons written
# by the person who pasted it. Those are their words, carried across verbatim
# from the quoted column; nothing here composed one.
#
# ` + "`stage: draft`" + ` means the gate reports what is still owed as a warning
# instead of an error, so the deck's *facts* -- legality, colour identity,
# singleton, size -- are checked from day one.
#
# %s
`
)

// header is the block for a deck that carried `carried` reasons of its own and
// still owes `owed`.
func header(date string, carried, owed int) string {
	if carried == 0 {
		return fmt.Sprintf(headerBare, date)
	}
	tail := fmt.Sprintf("%d card(s) are still waiting for a reason. The gate\n"+
		"# refuses promotion to `curated` until every one of them has one.", owed)
	if owed == 0 {
		tail = "Nothing is owed: every card has a reason, so this deck can be\n" +
			"# promoted to `curated` whenever you are happy with it."
	}
	return fmt.Sprintf(headerReasoned, date, carried, tail)
}

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
	// Names the pool does not have, and could not be read as anything either.
	// Kept in the deck; reported here and by the gate as `unknown-card`.
	Unknown []string
	// What was misspelled and what it was read as. Never silent: this is the
	// other half of being allowed to correct at all.
	Read []Correction
	// Lines the parser could not read at all.
	Unreadable []decklist.Line
	// Lines under a section that is not part of the deck, e.g. Tokens.
	Skipped []decklist.Line
	// Things that were changed rather than rejected, each worth saying aloud.
	Notes []string
	// Rationales is how many cards arrived with a reason of their own, from
	// the quoted column. Counted rather than derived from the deck so the
	// number is about what was PASTED: a card whose reason was written twice
	// on two merged lines is one card with a reason, not two.
	Rationales int
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
		// The other reading of a trailing quoted run, so `ReadRationales` can
		// choose by lookup rather than by guessing -- the same trick, and for
		// the same reason, as the comma below. After it has chosen there is
		// one reading per line and this adds nothing.
		add(c.Unpeeled)
	}
	for _, c := range commander {
		add(c)
		// Both readings of a comma, so `commanderReading` can decide between
		// them by lookup rather than by guessing. Fetching a few extra names
		// costs one entry in a list that is already ~100 long.
		for _, parts := range pairParts(c) {
			for _, part := range parts {
				add(part)
			}
		}
	}
	add(companion)
	slices.Sort(names)
	return names
}

// ---- reading a misspelling -------------------------------------------------

// Where a reading is clear enough to make, and both numbers are measured
// against this pool rather than chosen.
//
// Asked for real typos, jaro-winkler puts the intended card at 0.975 or
// better every time -- `Sol Rng` -> Sol Ring 0.975, `Cultivater` -> Cultivate
// 0.980, `Path to Exil` -> Path to Exile 0.985, `Swords to Plowshars` ->
// Swords to Plowshares 0.990, a doubled letter in Rhystic Study 0.986 --
// while the runner-up in each of those sits between 0.87 and 0.92, and a line
// of keyboard mash tops out at 0.71 against the whole pool.
//
// So `nearFloor` is well above every non-word measured and well below every
// typo measured, and `nearLead` is the second half of the same guard: a
// reading is made only when one card is CLEARLY what was meant, never when it
// merely wins a close field. `Cultivater` beats its runner-up by 0.075;
// two similarly-named cards a genuine new printing might sit between would
// not clear 0.04 between them.
const (
	nearFloor = 0.95
	nearLead  = 0.04
)

// Correction is a name the pool did not know, and the card it was read as.
type Correction struct {
	Written string  `json:"written"`
	Read    string  `json:"read"`
	Score   float64 `json:"score"`
}

// Candidate is one scored name, as a Speller returns it.
type Candidate struct {
	Name  string
	Score float64
}

// Reader is what Respell needs from the card pool: a way to score names
// against a written one, and a way to fetch the records it settles on.
//
// An interface rather than a `*pool.Conn` so this package stays testable
// without a database -- and so the scoring stays in one place. The production
// implementation wraps `cards.ByTitle`, which is the same function the camera
// door resolves photographed titles through (ADR 34): one scorer, so a name
// typed by hand and a name read off a photograph are judged by one measure.
type Reader interface {
	// Nearest is the pool names closest to `written`, best first.
	Nearest(ctx context.Context, written string, limit int) ([]Candidate, error)
	// Cards is `pool.Conn.GetCards`: whole records, by name.
	Cards(ctx context.Context, names []string) (map[string]*pool.CardRecord, error)
}

// ReadRationales decides, for every line that ended with a quoted run, whether
// that run was the card's reason or part of the card's name.
//
// `decklist` cannot tell: `1 Sol Ring "fast mana"` and `1 Kongming, "Sleeping
// Dragon"` are the same shape, and only a card pool separates them. So the
// grammar hands up both readings and this one asks -- `cards` being the same
// finished lookup `BuildDeck` will use, fetched over both readings by
// `NamesIn`.
//
// The peeled reading wins unless the pool knows the whole one and does not
// know the peeled one. Measured over this pool that condition is exact rather
// than merely likely: five card names end in a quoted run, and not one of them
// peels to a name the pool also has, so no line is ever decided by precedence
// alone. When the pool knows neither, the peeled reading is kept so `Respell`
// scores the card's name instead of the card's name with somebody's sentence
// glued to it.
//
// Returns the list with one reading per line -- `Unpeeled` cleared, so a
// second `NamesIn` asks about exactly what was chosen -- and a note for each
// line where the quoted run turned out to be a name, because a reason the
// person thought they wrote and did not get is precisely the kind of silence
// this package refuses.
func ReadRationales(parsed decklist.List,
	cards map[string]*pool.CardRecord) (decklist.List, []string) {

	notes := []string{}
	out := parsed
	out.Cards = make([]decklist.Card, len(parsed.Cards))
	for i, c := range parsed.Cards {
		out.Cards[i] = c
		out.Cards[i].Unpeeled = ""
		if c.Unpeeled == "" {
			continue
		}
		_, peeled := CanonicalName(c.Name, cards)
		_, whole := CanonicalName(c.Unpeeled, cards)
		if peeled != nil || whole == nil {
			continue
		}
		out.Cards[i].Name, out.Cards[i].Why = c.Unpeeled, ""
		notes = append(notes, fmt.Sprintf(
			"line %d: the quoted words are part of the card's name, so %s was "+
				"read as one card and not as a card with a reason", c.LineNo, whole.Name))
	}
	return out, notes
}

// Respell reads the names the pool could not resolve.
//
// `cards` is the lookup `BuildDeck` will use, and this ADDS to it: a reading
// is installed under the name that was WRITTEN, so `CanonicalName` finds the
// record and hands back the card's real name, and every count, category and
// colour downstream is the real card's. Nothing else in the pipeline needs to
// know a correction happened -- which is exactly the point, because a
// corrected card must be as real as one that was typed correctly.
//
// Only names that failed are read. A name the pool knows is never
// second-guessed, so a correctly spelled card can never be swapped for a
// better-scoring neighbour.
//
// Two passes, because a per-name record fetch would be one query per typo:
// score everything first, then fetch every winner at once.
func Respell(ctx context.Context, reader Reader, names []string,
	cards map[string]*pool.CardRecord) ([]Correction, error) {

	if reader == nil {
		return nil, nil
	}
	type pick struct {
		written string
		name    string
		score   float64
	}
	picks := []pick{}
	seen := map[string]bool{}
	for _, written := range names {
		if strings.TrimSpace(written) == "" || seen[written] {
			continue
		}
		seen[written] = true
		if _, rec := CanonicalName(written, cards); rec != nil {
			continue
		}
		found, err := reader.Nearest(ctx, written, 2)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 || found[0].Score < nearFloor {
			continue
		}
		if len(found) > 1 && found[0].Score-found[1].Score < nearLead {
			continue
		}
		picks = append(picks, pick{written: written, name: found[0].Name,
			score: found[0].Score})
	}
	if len(picks) == 0 {
		return nil, nil
	}
	wanted := make([]string, 0, len(picks))
	for _, p := range picks {
		wanted = append(wanted, p.name)
	}
	records, err := reader.Cards(ctx, wanted)
	if err != nil {
		return nil, err
	}
	out := []Correction{}
	for _, p := range picks {
		rec := records[p.name]
		if rec == nil {
			// The pool scored a name it will not hand over. Nothing is
			// invented on the way past: the miss stays a miss.
			continue
		}
		cards[p.written] = rec
		out = append(out, Correction{Written: p.written, Read: rec.Name,
			Score: math.Round(p.score*10000) / 10000})
	}
	return out, nil
}

// Options are `build_deck`'s keyword arguments.
type Options struct {
	Slug      string
	Name      string
	Commander []string
	Companion string
	Bracket   *int
	Status    string
	// Read is what `Respell` decided before this was called, so the report
	// carries the corrections and the notes say them out loud. Passed in
	// rather than done here because reading a name needs the pool and this
	// function is handed a finished lookup.
	Read []Correction
	// Notes is what the steps before this one decided, for the same reason:
	// `ReadRationales` needs the pool and runs earlier, and what it chose has
	// to reach the person who pasted the list.
	Notes []string
}

// pairSeparators is how one field might be holding two commanders, best
// first. `+` is what players write between partners and appears in no card
// name; a comma is what somebody types anyway, and is tried last because it
// is also punctuation inside most legendary names.
var pairSeparators = []string{" + ", "+", ","}

// pairParts is every way this string might be two or more names, in the order
// worth trying. Its own function so `NamesIn` and `commanderReading` cannot
// disagree about what the parts of a name are -- one fetches them, the other
// chooses between them, and a fetch that missed a reading would make that
// reading permanently unavailable.
func pairParts(name string) [][]string {
	out := [][]string{}
	for _, sep := range pairSeparators {
		if !strings.Contains(name, sep) {
			continue
		}
		parts := []string{}
		for _, p := range strings.Split(name, sep) {
			if t := strings.TrimSpace(p); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) >= 2 {
			out = append(out, parts)
		}
	}
	return out
}

// commanderReading decides between "one card whose name contains a comma" and
// "two partners written with a comma between them".
//
// **A comma is part of a legendary creature's name far more often than it
// separates two of them.** Arahbo, Roar of the World. Atla Palani, Nest
// Tender. Gyome, Master Chef. Tivit, Seller of Secrets. Every deck in this
// library is led by a name with a comma in it, and the import page used to
// split its commander field on commas before it ever reached the wire -- so
// typing the commander in by hand produced *two* commanders, neither of them
// a card, and a deck reporting `unknown-card` twice for the two halves of one
// legend. It was found on 2026-08-24 by typing the name of the deck the whole
// library is built around.
//
// The choice is made by asking the pool, which is why this is not a guess and
// not a violation of the package's first rule: **if the whole string is a
// card, it is one commander.** Only when it is not, and every comma-separated
// part is, does the pair reading win. When neither reading resolves, nothing
// is invented -- the original string stays exactly as written and is reported
// as unknown, with the shortlist beside it.
func commanderReading(wanted []string, cards map[string]*pool.CardRecord) ([]string, string) {
	if len(wanted) != 1 {
		return wanted, ""
	}
	whole := wanted[0]
	if _, rec := CanonicalName(whole, cards); rec != nil {
		return wanted, ""
	}
	for _, parts := range pairParts(whole) {
		resolved := make([]string, 0, len(parts))
		for _, part := range parts {
			name, rec := CanonicalName(part, cards)
			if rec == nil {
				break
			}
			resolved = append(resolved, name)
		}
		// Every part had to be a card. A reading where one half resolves and
		// the other does not is not a pairing with a typo in it -- it is the
		// wrong reading, and the next separator may be the right one.
		if len(resolved) != len(parts) {
			continue
		}
		return resolved, fmt.Sprintf(
			"%q is not a card, but the %d names inside it are, so it was read "+
				"as a pairing: %s. Write one commander per entry if that is "+
				"wrong.", whole, len(resolved), strings.Join(resolved, " + "))
	}
	return wanted, ""
}

// BuildDeck resolves a parsed list into a draft deck.
//
// `cards` is a name -> record mapping, as `pool.GetCards` returns over
// `NamesIn`. Passing an empty mapping is legal and means every name is
// reported as unknown, which is what a fresh clone with no card pool gets --
// honestly useless rather than silently wrong.
func BuildDeck(parsed decklist.List, cards map[string]*pool.CardRecord,
	opts Options) (*Report, error) {

	notes := slices.Clone(opts.Notes)
	unknown := []string{}

	resolve := func(written string) (string, *pool.CardRecord) {
		canonical, rec := CanonicalName(written, cards)
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
	// Before the count is checked, because the whole point is that one name
	// with a comma in it is one commander and not two.
	wanted, pairing := commanderReading(wanted, cards)
	if pairing != "" {
		notes = append(notes, pairing)
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
		canonical, _ := CanonicalName(line.Name, cards)
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

	// A reason written on the commander's own line has nowhere to go in the
	// 99, because the command zone is a list of names and not of entries. It
	// is still the person's writing, so it is kept where the deck file keeps
	// an author's prose -- `notes` -- rather than dropped, and the report says
	// where it went. Nothing is composed on the way: the sentence is theirs,
	// and the only thing added is which card it was about, and only when there
	// is more than one card it could have been about.
	zoneWhy, zoneCount := commandZoneReasons(parsed, cards, outside)

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

	// Deliberately NOT a note. `Report.Read` is the saying-so, and it is a
	// typed list of pairs rather than a sentence -- so a caller renders it
	// where it belongs (the import page puts it above the errors, because a
	// correction changed what the deck contains) instead of parsing prose out
	// of `Notes`. A note as well was two copies of one fact on one screen.

	rationales := zoneCount
	for _, c := range append(slices.Clone(entries), swaps...) {
		if strings.TrimSpace(c.Why) != "" {
			rationales++
		}
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
	if zoneWhy != "" {
		notes = append(notes, "a reason was written on a line that turned out to "+
			"be in the command zone, which holds names and not reasons; it was "+
			"kept verbatim as the deck's `command_zone` note")
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
	if zoneWhy != "" {
		built.Notes = deckyaml.Map{{Key: "command_zone", Value: zoneWhy}}
	}
	body, err := built.Dump()
	if err != nil {
		return nil, err
	}
	text := header(time.Now().Format("2006-01-02"), rationales,
		len(built.Unjustified())) + "\n" + body

	return &Report{Deck: built, YAML: text, Unknown: unknown, Read: opts.Read,
		Unreadable: parsed.Unreadable, Skipped: parsed.Skipped, Notes: notes,
		Rationales: rationales}, nil
}

// commandZoneReasons gathers the quoted reasons from lines whose card ended up
// in the command zone, in the order they were pasted.
//
// One card's reason is kept exactly as it was written. Two cards' -- a partner
// pair, or a commander and a companion -- are labelled with the card each was
// about, because two unattributed sentences in one note is a worse record of
// what somebody said than two attributed ones.
func commandZoneReasons(parsed decklist.List, cards map[string]*pool.CardRecord,
	outside map[string]bool) (string, int) {

	type reason struct{ name, why string }
	found := []reason{}
	for _, line := range parsed.Cards {
		if strings.TrimSpace(line.Why) == "" {
			continue
		}
		canonical, _ := CanonicalName(line.Name, cards)
		if !outside[strings.ToLower(canonical)] {
			continue
		}
		found = append(found, reason{name: canonical, why: line.Why})
	}
	switch len(found) {
	case 0:
		return "", 0
	case 1:
		return found[0].why, 1
	}
	lines := make([]string, 0, len(found))
	for _, f := range found {
		lines = append(lines, fmt.Sprintf("%s: %s", f.name, f.why))
	}
	return strings.Join(lines, "\n"), len(found)
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
			// The merged line's reason is kept only when the first line had
			// none. Two lines of one card with two different reasons is a
			// person disagreeing with themselves, and choosing between them
			// here would be composing a rationale by picking one.
			if existing.Why == "" {
				existing.Why = line.Why
			}
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
		byKey[key] = &deck.CardEntry{Name: canonical, Category: category,
			Qty: line.Qty, Why: line.Why}
		order = append(order, key)
	}

	entries := make([]deck.CardEntry, 0, len(order))
	for _, key := range order {
		entries = append(entries, *byKey[key])
	}
	return entries, removed
}

// CanonicalName is the name as the pool spells it, and the record behind it.
//
// Casing is corrected, but a double-faced card written by its front face stays
// written that way. `GetCards` resolves both, the curated decks all use face
// names ("Branchloft Pathway"), and expanding one to "Branchloft Pathway //
// Boulderloft Pathway" on import would make the library inconsistent for no
// gain.
//
// **Exported on 2026-08-29 for the deck page's bulk edit**, which resolves a
// pasted list against a deck that already exists rather than into a new one.
// It has to spell a name the way this does or it would find no match for a
// card the deck holds under its front face -- and would then bury that card
// and add the same card back under a different spelling. One resolver, so the
// two paths cannot disagree about what a name is.
func CanonicalName(written string, cards map[string]*pool.CardRecord) (string, *pool.CardRecord) {
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
