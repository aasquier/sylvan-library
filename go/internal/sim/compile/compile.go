// Package compile is `sim/compile.py`: a deck file plus pool records become
// the `sim.Card`s every tier is handed, and a report saying what the pool
// could not resolve.
//
// It is the piece `go/internal/sim` was written around -- that package's
// comment says "`sim/compile.py` is what builds these, and when it is ported
// it lands here" -- and it is the only place in the simulator where a card's
// *text* is read. Everything above it reasons about `sim.Card`; everything
// below it is arithmetic. So a wrong answer here is wrong in every tier at
// once, which is why Python's comments about the three "confidently wrong for
// every deck" bugs travelled across with the code and are kept below.
//
// # The two behaviours that are contract, not detail
//
// **A dropped card shrinks the deck silently unless the report says so.**
// `compileOne` returns nil for a name the pool does not know, and for most of
// this module's life the caller dropped it on the floor: a 99-card deck whose
// pool was missing six cards simulated as a 93-card deck with no signal at
// all. For Tier 1.5 that is not a rounding error -- the library size is the
// population every hypergeometric is computed over -- so `Report.Unresolved`
// is the field this type exists for, and `Deck` is the shape that does not
// carry it, kept only because most callers really do just want the cards.
//
// **`ManaProduced` reads the amount off the ORACLE TEXT.** Scryfall's
// `produced_mana` names colours and never amounts, so until 2026-08-21 every
// mana source in this project compiled to exactly one mana -- Sol Ring
// produced one, Mana Vault one, Gilded Lotus one -- and both tiers understated
// every deck's acceleration, the fast-mana decks most.
//
// # What is deliberately absent
//
// Nothing. `PoolRequired` and `NothingToSimulate` both cross, because both are
// refusals a served route has to be able to make.
package compile

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/mana"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim"
)

// NothingToSimulate is `compile.NothingToSimulate`: the deck compiled to no
// cards at all, so there is nothing to measure.
//
// Not the same failure as `PoolRequired`, which is "this machine has no card
// pool". This is the one deck state where a simulation must be refused rather
// than reported, and it exists because an empty deck simulated perfectly
// happily and answered with confident nonsense: a 100% mulligan rate, zero
// spells through turn 8, and a shelf demanding coloured sources for a
// commander's two colours against a library of nought. Every one of those
// numbers is arithmetically correct and none of them is about anything.
// `adrix-and-nev-twincasters` was in exactly that state on the deployed
// instance.
type NothingToSimulate struct {
	Slug string
	// Declared is what the deck file says it holds, counting `qty` -- the
	// number that makes "it compiled to nothing" diagnosable rather than
	// merely true.
	Declared int
}

func (e *NothingToSimulate) Error() string {
	return fmt.Sprintf("%s compiles to no cards, so there is nothing to "+
		"simulate (the deck file declares %d)", e.Slug, e.Declared)
}

// PoolRequired is `compile.PoolRequired`: a simulation was asked for without
// the card pool.
//
// Mana production cannot be inferred from a deck file alone, and guessing
// would produce numbers that look authoritative and are not. In Python this
// was once `sys.exit` -- fine for the CLI, and a confusing way to fail from
// the worker thread that also reached it.
type PoolRequired struct{}

func (e *PoolRequired) Error() string {
	return "simulation needs the card pool -- run `mtglab data refresh` first"
}

// EntersTapped is `compile.enters_tapped`: whether a land unconditionally
// enters tapped.
//
// Scryfall retemplated this. Current oracle text reads "This land enters
// tapped", not "enters the battlefield tapped", and matching only the old
// wording silently treated every modern tapland as untapped -- which
// overstates early mana for every deck, and is the retemplating ADR 18 cites
// as the reason a cache may not be keyed on the deck file.
//
// Conditional lands are deliberately treated as untapped. Tier 1 cannot
// evaluate "unless you control a Forest" or a shock land's "you may pay 2
// life", and in practice those resolve untapped in most real games; calling
// them tapped would systematically slow every deck instead.
func EntersTapped(oracleText string) bool {
	text := strings.ToLower(oracleText)
	if !strings.Contains(text, "enters tapped") &&
		!strings.Contains(text, "enters the battlefield tapped") {
		return false
	}
	return !strings.Contains(text, "unless") && !strings.Contains(text, "you may pay")
}

// symbolRe is a mana symbol inside an activation cost or an "Add" clause.
// `{T}` and `{Q}` are not mana and must never be charged as though they were
// -- a Signet's `{1}, {T}:` costs one mana, not two.
var symbolRe = regexp.MustCompile(`\{([^}]+)\}`)

// amountWords is how many mana a written-out amount means. Magic spells these
// out whenever the colour is chosen rather than fixed ("Add three mana of any
// one color"), so a symbol count alone reads every one of them as zero.
var amountWords = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
}

// manaSymbols is mana in a run of symbols. `{2}` is two, `{G}` is one, `{T}`
// is none.
//
// Two Python details are reproduced rather than tidied, because tidying either
// changes an answer.
//
// **The colour test is a SUBSTRING test, not a set membership.** Python reads
// `any(ch in "WUBRGC" for ch in sym.split("/"))`, where `ch` walks the *parts*
// of the split rather than characters -- so a half of "wu" counts (it is a
// substring of "WUBRGC") and a half of "uw" does not, and an empty half counts
// because "" is a substring of everything. No real symbol reaches those
// branches, but the port is held to Python by a corpus that includes them, and
// a "cleaner" set test would answer differently on the ones that do.
//
// **`isdigit` is narrowed to ASCII on purpose.** Python's `str.isdigit()` is
// true for superscripts and other Unicode digits that `int()` then refuses, so
// the only inputs where the two implementations differ are inputs where Python
// raises `ValueError`. Answering rather than crashing is the safer of the two.
func manaSymbols(text string) int {
	total := 0
	for _, m := range symbolRe.FindAllStringSubmatch(text, -1) {
		sym := strings.TrimSpace(strings.ToUpper(m[1]))
		if isASCIIDigits(sym) {
			n, err := strconv.Atoi(sym)
			if err != nil {
				continue
			}
			total += n
			continue
		}
		for _, part := range strings.Split(sym, "/") {
			if strings.Contains("WUBRGC", part) {
				total++
				break
			}
		}
	}
	return total
}

func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// clauseSplit is Python's `re.split(r",| or ", clause)`: alternatives are
// separated by commas *and* by the word "or".
var clauseSplit = regexp.MustCompile(`,| or `)

// ManaProduced is `compile.mana_produced`: how many mana one activation of
// this permanent actually nets.
//
// Scryfall's `produced_mana` says *which colours* a card can make and never
// **how many**, so every mana source in this project compiled to exactly one
// mana until 2026-08-21. The amount is read off the oracle text,
// conservatively: anything this cannot parse stays at one. Five things it has
// to get right, each of which is a real card:
//
//   - **an alternative is not a sum.** Talisman of Progress reads "Add {W} or
//     {U}" and makes one mana, not two.
//   - **alternatives are separated by commas as well as by "or".** A triome
//     reads "Add {R}, {G}, or {W}" and Wooded Bastion "Add {G}{G}, {G}{W}, or
//     {W}{W}"; splitting on "or" alone reads those as two and four. A run is
//     never comma-separated -- it is "{C}{C}", never "{C}, {C}" -- so the comma
//     is unambiguous.
//   - **the cost comes off the same ability only.** Arcane Signet's "{1}, {T}:
//     Add {W}{U}" nets one; Grim Monolith's "{4}: Untap this artifact" sits on
//     its own line and must not be charged against its "Add {C}{C}{C}".
//   - **amounts are often words.** Gilded Lotus adds "three mana of any one
//     color" and contains no mana symbol at all. The first number word after
//     "add" is the amount -- "any *one* color" comes later in that same
//     sentence and is not it.
//   - **"add" is not always mana.** A card that adds a +1/+1 counter says so,
//     which is why the clause must mention mana or carry a mana symbol.
//
// Nykthos is the shape that correctly falls through to one: "Add an amount of
// mana of that color equal to your devotion" is not a number this can know,
// and guessing would be worse than the floor.
//
// **Non-mana costs are not priced**, which is this module's existing position
// rather than a new one: `EntersTapped` already reads "you may pay 2 life" as
// untapped. So Ashnod's Altar counts as two and Phyrexian Tower's sacrifice
// ability as two, because those abilities really do add that much -- what Tier
// 1 cannot model is the supply of creatures to feed them. Read a
// sacrifice-driven deck's acceleration as an upper bound.
func ManaProduced(oracleText string) int {
	best := 0
	for _, line := range strings.Split(strings.ToLower(oracleText), "\n") {
		if !strings.Contains(line, "add") {
			continue
		}
		// `str.partition`: a line with no colon, or one ending in a colon,
		// has no activation cost and its whole text is the clause.
		head, tail, _ := strings.Cut(line, ":")
		costText, body := "", line
		if tail != "" {
			costText, body = head, tail
		}
		start := strings.Index(body, "add")
		if start < 0 {
			continue
		}
		clause := body[start:]
		if stop := strings.Index(clause, "."); stop > 0 {
			clause = clause[:stop]
		}
		if !strings.Contains(clause, "mana") && !symbolRe.MatchString(clause) {
			continue
		}

		// Alternatives are maxed; a run is summed.
		added := 0
		for _, part := range clauseSplit.Split(clause, -1) {
			if n := manaSymbols(part); n > added {
				added = n
			}
		}
		if added == 0 {
			// `clause[len("add"):].split()` -- whitespace split, empties
			// dropped, first token only.
			after := strings.Fields(clause[len("add"):])
			if len(after) > 0 {
				added = amountWords[after[0]]
			}
		}
		if added == 0 {
			continue
		}
		if net := added - manaSymbols(costText); net > best {
			best = net
		}
	}
	// One is the floor, not a default to fall back on lightly: a source that
	// nets nothing is still a source for *colour*, and Tier 1 has always
	// treated it as one mana.
	if best < 1 {
		return 1
	}
	return best
}

// fetchWords are the land words a fetch effect has to name. Python spells the
// five basics out beside "land" rather than relying on a type line, because
// the sentence being read is oracle text.
var fetchWords = []string{"land", "forest", "swamp", "island", "mountain", "plains"}

// FetchesLands is `compile.fetches_lands`: how many lands a spell puts onto
// the battlefield from the library.
//
// Nature's Lore, Three Visits, Skyshroud Claim and Sakura-Tribe Elder are ramp
// that produces no mana of its own. Without this they compile to blank cards,
// which understates the deck's acceleration and skews the land-count
// recommendation.
func FetchesLands(oracleText string) int {
	text := strings.ToLower(oracleText)
	if !strings.Contains(text, "search your library") ||
		!strings.Contains(text, "onto the battlefield") {
		return 0
	}
	named := false
	for _, w := range fetchWords {
		if strings.Contains(text, w) {
			named = true
			break
		}
	}
	if !named {
		return 0
	}
	if strings.Contains(text, "two") || strings.Contains(text, "up to two") {
		return 2
	}
	return 1
}

// Report is `compile.CompileReport`: what compiling produced, and what it
// could not.
//
// `Unresolved` is the field this exists for -- see the package comment. The
// library is `[]*sim.Card` rather than `[]sim.Card` for the reason Python's is
// a list of shared objects: `qty` repeats put **the same card** in the list
// that many times, and Tier 1's commander check is identity rather than
// equality. `Values` is the copy the closed forms take.
type Report struct {
	Library   []*sim.Card
	Commander *sim.Card
	// Unresolved is the names in the deck the pool did not know, in deck
	// order. Counted once per entry, not once per copy.
	Unresolved []string
	// DeclaredSize is what the deck file says it holds, counting `qty`.
	DeclaredSize int
	// CommanderUnresolved is true when the commander itself did not resolve.
	// Its own field because "the deck lost six cards" and "every
	// commander-speed figure is about nothing" are different sentences.
	CommanderUnresolved bool
}

// SimulatedSize is `CompileReport.simulated_size`.
func (r *Report) SimulatedSize() int { return len(r.Library) }

// Complete is `CompileReport.complete`.
func (r *Report) Complete() bool {
	return len(r.Unresolved) == 0 && !r.CommanderUnresolved
}

// Values is the library as values, for `karsten` and `curve`, which read a
// compiled deck and never mutate or identify one.
//
// Tier 1 takes the pointers. That split is real: the engine removes cards from
// a hand by first-equal and compares the commander by identity, so it wants
// exactly the aliasing Python's `[card] * qty` produces; the closed forms
// count and multiply, and a copy is simpler to reason about than a slice of
// aliases.
func (r *Report) Values() []sim.Card {
	out := make([]sim.Card, len(r.Library))
	for i, c := range r.Library {
		out[i] = *c
	}
	return out
}

// Compile is `compile.compile_report`: compile a deck and say what happened,
// including what went missing.
//
// Returns `*NothingToSimulate` when the result is empty. That refusal lives
// here rather than in each caller because it is not a policy question -- there
// is no number to report about no cards, and a caller that forgot to check
// would publish the confident nonsense that type describes.
//
// `cards` empty or nil is `*PoolRequired`, and it is checked FIRST: an empty
// pool is a broken machine, not an empty deck, and the two want different
// answers on the screen.
//
// **There is a wart here, and it is reproduced rather than fixed.** `GetCards`
// returns only what it found, so a deck where not one single name resolves --
// not even the commander -- hands this an empty mapping, which `len(cards) ==
// 0` cannot tell from "this machine has no pool". Such a deck is refused as
// `*PoolRequired` rather than as `*NothingToSimulate`, which is the wrong word
// for it. The corpus carries both cases (`nothing-resolves` and
// `empty-lookup`) so the two runtimes agree; changing which refusal a deck
// gets is a decision for the surface that renders it, not for a port.
func Compile(d *deck.Deck, cards map[string]*pool.CardRecord) (*Report, error) {
	library, commander, err := compileCards(d, cards)
	if err != nil {
		return nil, err
	}
	unresolved := []string{}
	declared := 0
	for _, entry := range d.Cards {
		if _, ok := cards[entry.Name]; !ok {
			unresolved = append(unresolved, entry.Name)
		}
		declared += entry.Qty
	}
	if len(library) == 0 {
		return nil, &NothingToSimulate{Slug: d.Slug, Declared: declared}
	}
	return &Report{
		Library:             library,
		Commander:           commander,
		Unresolved:          unresolved,
		DeclaredSize:        declared,
		CommanderUnresolved: len(d.Commander) > 0 && commander == nil,
	}, nil
}

// Deck is `compile.compile_deck`: the library and the commander, and nothing
// about what went missing.
//
// The long-standing shape, kept because most callers only want the cards.
// Anything that will *report a number to somebody* should use `Compile`
// instead -- silently dropping a card changes every probability in the answer.
func Deck(d *deck.Deck, cards map[string]*pool.CardRecord) ([]*sim.Card, *sim.Card, error) {
	return compileCards(d, cards)
}

func compileCards(d *deck.Deck, cards map[string]*pool.CardRecord) ([]*sim.Card, *sim.Card, error) {
	if len(cards) == 0 {
		return nil, nil, &PoolRequired{}
	}
	// Expand by qty. Basics carry qty 8-16, so ignoring it simulated a deck of
	// ~83 cards with ~20 lands instead of 99 with 34 -- which made every
	// mulligan rate and land-count recommendation wrong. The SAME pointer goes
	// in qty times, which is `[compiled] * entry.qty` exactly.
	library := []*sim.Card{}
	for _, entry := range d.Cards {
		compiled := compileOne(entry.Name, cards)
		if compiled == nil {
			continue
		}
		for i := 0; i < entry.Qty; i++ {
			library = append(library, compiled)
		}
	}
	var commander *sim.Card
	if len(d.Commander) > 0 {
		// A fresh card even when the same name is in the library, because
		// Python calls `compile_one` again and Tier 1 asks `is commander`.
		commander = compileOne(d.Commander[0], cards)
	}
	return library, commander, nil
}

func compileOne(name string, cards map[string]*pool.CardRecord) *sim.Card {
	rec, ok := cards[name]
	if !ok || rec == nil {
		return nil
	}
	// Only permanents stay on the battlefield making mana. Scryfall reports
	// produced_mana for Treasure-makers like Deadly Dispute too, and without
	// this guard an instant compiles into a permanent mana source.
	front := rec.FrontTypeLine()
	isPermanent := !strings.Contains(front, "Instant") && !strings.Contains(front, "Sorcery")

	produced := []string{}
	seen := map[string]bool{}
	for _, p := range rec.ProducedMana {
		// `p in "WUBRGC"` is Python's substring test again, and the set is
		// deduplicated because Python's is a frozenset.
		if strings.Contains("WUBRGC", p) && !seen[p] {
			seen[p] = true
			produced = append(produced, p)
		}
	}
	// Sorted, because Python's is a frozenset with no order at all: every
	// place it is serialised -- the cache key, the differential corpus --
	// writes `sorted(...)`, so sorted is the only order that cannot differ.
	sort.Strings(produced)

	var produces []sim.Source
	if len(produced) > 0 && isPermanent {
		// The amount comes from the oracle text; `produced_mana` only ever
		// said which colours. See `ManaProduced` for why that mattered.
		produces = []sim.Source{{Colors: produced, Amount: ManaProduced(rec.OracleText)}}
	}
	// The FULL type line, not the front face -- Python reads `"Creature" in
	// rec.type_line` here and `front` only for the permanent test, so a
	// creature on the back of a DFC delays its front face's mana.
	isCreature := strings.Contains(rec.TypeLine, "Creature")
	isLand := rec.IsLand()
	// A fetchland sacrifices itself, so it is net-zero lands and must not
	// count here -- only spells that add a land to the board do.
	fetch := 0
	if !isLand {
		fetch = FetchesLands(rec.OracleText)
	}
	cost := ""
	if rec.ManaCost != nil {
		cost = *rec.ManaCost
	}
	delay := 0
	if len(produces) > 0 && isCreature && !isLand {
		delay = 1
	}
	return &sim.Card{
		Name: rec.Name,
		Cost: fromManaCost(mana.Parse(cost)),
		// Set explicitly, and it is not decoration -- see `Category`.
		Category:     Category,
		IsLand:       isLand,
		EntersTapped: isLand && EntersTapped(rec.OracleText),
		Produces:     produces,
		ProduceDelay: delay,
		FetchesLands: fetch,
	}
}

// fromManaCost carries `mana.Cost` into the shape a compiled deck holds.
//
// Two types for one thing is Python's arrangement too -- `mana.ManaCost` is
// the parser's own and `sim.Cost` is what the simulator carries -- and the
// conversion is a copy rather than a re-parse, so a disagreement about parsing
// stays a `mana` failure instead of becoming an arithmetic one here.
func fromManaCost(c mana.Cost) sim.Cost {
	return sim.Cost{
		Generic:   c.Generic,
		Pips:      c.Pips,
		Phyrexian: c.Phyrexian,
		HasX:      c.HasX,
	}
}

// Category is what `SimCard.category` holds on a compiled card, and it is a
// constant here because Python never assigns it: `compile_one` passes no
// category at all and the dataclass default supplies **"utility"**.
//
// A Go zero value is "", so leaving the field alone would have been the
// natural port and the wrong one. Nothing in any tier reads the field -- the
// deck file's own category is deliberately not carried, since a second copy of
// a deck's categorisation is a place for the two to disagree -- so the
// difference is invisible to every simulation and visible in exactly one
// place: **the ADR 18 cache key**, whose `_card_form` serialises `category`
// precisely because "the engine ignores this field" is a claim about engine
// behaviour that can go stale. An empty string there would make every Go key
// differ from Python's for a reason nobody chose, on top of the one reason
// this port does choose (`cache.Fingerprint`). A test pins it.
const Category = "utility"
