// Package artifacts is `artifacts/generate.py`: the five deliverables, and
// the bytes are the product.
//
// Rule 3 of the non-negotiables -- five artifacts for every new deck and every
// refactor, never hand-written. `primer-quick.md` gets somebody playing,
// `primer-advanced.md` is the lines and the failure modes,
// `decklist-annotated.md` is the 99 with a reason beside every card,
// `moxfield.txt` is the bulk import, and `swaps.md` is a diff -- which is why
// this package sits over `pyyaml` rather than beside it. A deliverable is
// markdown a person reads on the train and pastes into Moxfield, so "close
// enough" is not a standard that exists here: `testdata/artifacts.json` holds
// what Python renders for every fixture deck and this package reproduces it
// byte for byte.
//
// Three shapes carried across unchanged, each of which is a decision rather
// than a default:
//
//   - **`RenderAll` returns text and `Store` writes it**, split in Python when
//     the API learned to build. A deck-facing endpoint holds a `DeckSource`,
//     which is a locator and may not be a disk at all, so generating and
//     storing had to stop being the same act. Everything above `RenderAll` is
//     a pure function of the deck; below it is somebody's storage.
//
//   - **`Deliverables` is the served set and the path-traversal guard in one,
//     and `Snapshot` is deliberately outside it.** A name that is not in that
//     list is refused before anything touches a filesystem, so there is no
//     `..` to sanitise. The snapshot is the build's own bookkeeping -- the
//     baseline the next `swaps.md` diffs against, because decks are not in git
//     (ADR 30) and there is no revision to diff -- and nobody asked for a copy
//     of the deck they already have.
//
//   - **A rebuild prunes the deliverables it did not produce.** A build with
//     no baseline writes no `swaps.md`, and the previous build's swap list
//     left sitting in the directory describes a diff that no longer exists:
//     stale in the one way that is indistinguishable from current. Found on
//     2026-08-21 by the parity test across the two deck sources, and fixed in
//     the one place they share, which is `Store`.
//
// And one refusal. **A draft is refused outright** (ADR 13): a primer that
// quietly omits the argument for a card reads exactly like one that had it,
// which is ADR 8's reasoning applied to the shareable surface. It is not
// forceable -- `force` overrides the gate's errors, which are things the deck
// got *wrong*, and a draft is not wrong but unfinished. The way out is to
// write the rationales and promote it.
package artifacts

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// Snapshot is `generate.SNAPSHOT`: the build's own baseline for the next
// `swaps.md`. A normalised dump rather than the hand-written file, because
// `SwapList` compares decks and not text, and a snapshot that round-trips
// through a parse is all the baseline it needs.
const Snapshot = "deck.last-built.yaml"

// Deliverables is `generate.DELIVERABLES`, in the order the module numbers
// them: the set a reader may ask for, and therefore the set a rebuild prunes.
var Deliverables = []string{"primer-quick.md", "primer-advanced.md",
	"decklist-annotated.md", "moxfield.txt", "swaps.md"}

// IsDeliverable answers `name in DELIVERABLES` -- membership first, and for
// the file tier it is the only check that matters, because a name that is not
// one of the five never becomes a path at all.
func IsDeliverable(name string) bool {
	for _, d := range Deliverables {
		if d == name {
			return true
		}
	}
	return false
}

// categoryTitles is `generate.CATEGORY_TITLES`: the heading a category is
// written under. A category with no entry here falls back to the raw word in
// the quick primer's table and to Python's `str.title()` in the annotated
// list -- two different fallbacks for the same missing key, which is a fact
// about the Python rather than a tidy rule, so both are reproduced.
var categoryTitles = map[string]string{
	"land":           "Lands",
	"ramp":           "Ramp & Mana Acceleration",
	"card-advantage": "Card Advantage",
	"tutor":          "Tutors",
	"interaction":    "Interaction & Removal",
	"protection":     "Protection",
	"threat":         "Threats",
	"engine":         "Engines",
	"sac-outlet":     "Sacrifice Outlets",
	"payoff":         "Payoffs",
	"recursion":      "Recursion",
	"win-con":        "Win Conditions",
	"utility":        "Utility",
}

// ErrDraft is `generate.DraftDeck`: artifacts were asked for a deck nobody has
// finished reasoning about. The message names the cards still owed a `why`,
// and it reaches a caller verbatim -- `service.build_artifacts` re-raises it
// as the 422's `detail` -- so it is part of the wire and not just a log line.
var ErrDraft = errors.New("the deck is a draft")

// Stat is one entry of `render_all`'s `stats` mapping, which both primers
// render as `- **key:** value`.
//
// A slice rather than a map because Python's is a `dict` and dicts are
// ordered; strings rather than `any` because Python renders each value with
// `str()`, and reproducing `str()` over arbitrary objects is not a thing Go
// can promise. No caller supplies stats today -- neither `mtglab decks build`
// nor `service.build_artifacts` passes any -- so the shape is carried across
// rather than the stringifying, and whoever wires one up brings their own.
type Stat struct {
	Key   string
	Value string
}

// Options are `render_all`'s keyword arguments.
type Options struct {
	// Cards is the pool's reading of the deck, for the mana costs the
	// annotated list prints. Absent is ordinary: the costs simply do not
	// render, exactly as when Python is handed `cards=None`.
	Cards map[string]*pool.CardRecord
	// Previous is the deck as of its last build, parsed from the snapshot.
	// `swaps.md` is present only when this is, which is why a build's output
	// is a list rather than a fixed five: a first build has nothing to diff.
	Previous *deck.Deck
	// Prices turns the swap list's "In" column into a shopping list. Also
	// unsupplied by either caller today.
	Prices map[string]float64
	Stats  []Stat
	// Today is `date.today()`, injectable so the oracle is not a fixture that
	// expires at midnight. Zero means ask the clock -- the *local* clock,
	// because `date.today()` is local and the instance runs in UTC.
	Today time.Time
}

func (o Options) today() string {
	t := o.Today
	if t.IsZero() {
		t = time.Now()
	}
	return t.Format("2006-01-02")
}

// File is one rendered deliverable.
type File struct {
	Name string
	Text string
}

// Files is a build's output, in the order `render_all` builds it -- which is
// the order `Store` writes it in, snapshot last.
type Files []File

// Has answers whether this build produced a named file, which is the question
// the pruning asks of every deliverable.
func (f Files) Has(name string) bool {
	for _, file := range f {
		if file.Name == name {
			return true
		}
	}
	return false
}

// Text is one file's contents, and whether it was built at all.
func (f Files) Text(name string) (string, bool) {
	for _, file := range f {
		if file.Name == name {
			return file.Text, true
		}
	}
	return "", false
}

// RenderAll is `generate.render_all`: the deliverables as text, written
// nowhere. A draft comes back as ErrDraft and nothing is rendered.
//
// The snapshot is placed last and `Store` relies on that order. In Python
// that ordering used to be the whole guard -- a refusal partway through left
// the old baseline in place -- and it is belt and braces now, because
// rendering refuses before a single byte is stored.
func RenderAll(d *deck.Deck, o Options) (Files, error) {
	if d.Stage == "draft" {
		return nil, refuseDraft(d)
	}
	quick, err := QuickPrimer(d, o)
	if err != nil {
		return nil, err
	}
	advanced, err := AdvancedPrimer(d, o)
	if err != nil {
		return nil, err
	}
	annotated, err := AnnotatedDecklist(d, o)
	if err != nil {
		return nil, err
	}
	files := Files{
		{Name: "primer-quick.md", Text: quick},
		{Name: "primer-advanced.md", Text: advanced},
		{Name: "decklist-annotated.md", Text: annotated},
		{Name: "moxfield.txt", Text: MoxfieldText(d)},
	}
	if o.Previous != nil {
		files = append(files, File{Name: "swaps.md", Text: SwapList(d, o.Previous, o)})
	}
	snapshot, err := d.Dump()
	if err != nil {
		return nil, err
	}
	return append(files, File{Name: Snapshot, Text: snapshot}), nil
}

// refuseDraft is `generate._refuse_draft`: name the cards still owed a
// rationale, then refuse. The sentence is the 422's detail, so it is
// reproduced word for word, `--` and all.
func refuseDraft(d *deck.Deck) error {
	pending := d.Unjustified()
	shown := make([]string, 0, len(pending))
	for _, c := range pending {
		if len(shown) == 8 {
			break
		}
		shown = append(shown, c.Name)
	}
	more := ""
	if len(pending) > 8 {
		more = fmt.Sprintf(", and %d more", len(pending)-8)
	}
	detail := "every card is justified -- set `stage: curated` to promote it"
	if len(pending) > 0 {
		detail = fmt.Sprintf("%d card(s) still need a `why`: %s%s",
			len(pending), strings.Join(shown, ", "), more)
	}
	return fmt.Errorf("%w: %s is a draft, and the artifacts are the shareable "+
		"surface. %s", ErrDraft, d.Slug, detail)
}

// Message is the sentence a refusal carries, without the wrapped sentinel Go
// needs and Python does not. `service.build_artifacts` turns `str(exc)` into
// the 422's detail, and this is that string.
func Message(err error) string {
	return strings.TrimPrefix(err.Error(), ErrDraft.Error()+": ")
}

// ---- the five ------------------------------------------------------------

// orderedCategories is `generate._ordered_categories`: the declared order
// first, then anything a deck invented, sorted -- and lands last, because a
// reader wants the spells first.
func orderedCategories(d *deck.Deck) []string {
	_, present := d.CategoryCounts()
	seen := map[string]bool{}
	for _, c := range present {
		seen[c] = true
	}
	ordered := []string{}
	known := map[string]bool{}
	for _, c := range reference.Deck().Categories {
		known[c] = true
		if seen[c] {
			ordered = append(ordered, c)
		}
	}
	extra := []string{}
	for _, c := range present {
		if !known[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(extra)
	ordered = append(ordered, extra...)
	for i, c := range ordered {
		if c == "land" {
			ordered = append(ordered[:i], ordered[i+1:]...)
			ordered = append(ordered, "land")
			break
		}
	}
	return ordered
}

// note is `generate._note`: the note under a key, the default when there is
// none, and stripped either way.
//
// The error has no Python counterpart, and that is the point: `_note` calls
// `.strip()` on whatever the mapping holds, so a note carrying a number or a
// nested mapping is an `AttributeError` out of the renderer and a 500 from the
// route. A refusal here is the same answer with a sentence attached.
func note(d *deck.Deck, key, fallback string) (string, error) {
	value, present := d.Notes.Get(key)
	if !present || value == nil {
		return strings.TrimSpace(fallback), nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("note %q is not prose, and a primer can only "+
			"render prose", key)
	}
	if text == "" {
		// Python's `or`: an empty note is no note.
		return strings.TrimSpace(fallback), nil
	}
	return strings.TrimSpace(text), nil
}

// header is `generate._header`: the two-space-then-newline block every
// deliverable opens with.
func header(d *deck.Deck) string {
	cmd := strings.Join(d.Commander, " + ")
	if cmd == "" {
		cmd = "(no commander set)"
	}
	bits := []string{"**Commander:** " + cmd}
	if d.Companion != nil && *d.Companion != "" {
		bits = append(bits, "**Companion:** "+*d.Companion+" *(outside the 100)*")
	}
	// `if deck.bracket:` -- truthy, so a bracket of zero is no bracket.
	if d.Bracket != nil && *d.Bracket != 0 {
		bits = append(bits, "**Bracket:** "+strconv.Itoa(*d.Bracket))
	}
	bits = append(bits, fmt.Sprintf("**Size:** %d + commander", d.TotalCards()))
	return strings.Join(bits, "  \n")
}

// QuickPrimer is `generate.quick_primer`: one page, get somebody playing.
func QuickPrimer(d *deck.Deck, o Options) (string, error) {
	counts, _ := d.CategoryCounts()
	strategy, _ := d.Strategy.(string)
	if strategy == "" {
		strategy = "_(set `strategy:` in deck.yaml)_"
	}
	notes, err := manyNotes(d, [][2]string{
		{"gameplan", "_(set `notes.gameplan` in deck.yaml)_"},
		{"mulligan", "_(set `notes.mulligan` — derive it from a Tier 1 sweep)_"},
		{"curve_plan", "_(set `notes.curve_plan`)_"},
		{"pitfalls", "_(set `notes.pitfalls`)_"},
	})
	if err != nil {
		return "", err
	}
	lines := []string{
		"# " + d.Name + " — Quick Start",
		"",
		header(d),
		"",
		"## What this deck does",
		"",
		strategy,
		"",
		"## The 30-second version",
		"",
		notes[0],
		"",
		"## Mulligan rule",
		"",
		notes[1],
		"",
		"## Turn-by-turn shape",
		"",
		notes[2],
		"",
		"## Three things that will kill you",
		"",
		notes[3],
		"",
		"## Deck at a glance",
		"",
		"| Category | Count |",
		"| --- | ---: |",
	}
	for _, cat := range orderedCategories(d) {
		title, ok := categoryTitles[cat]
		if !ok {
			// The raw word, not `title()`: the annotated list capitalises an
			// unknown category and this table does not.
			title = cat
		}
		lines = append(lines, fmt.Sprintf("| %s | %d |", title, counts[cat]))
	}
	if len(o.Stats) > 0 {
		lines = append(lines, "", "## Simulated consistency", "")
		for _, s := range o.Stats {
			lines = append(lines, fmt.Sprintf("- **%s:** %s", s.Key, s.Value))
		}
	}
	lines = append(lines, "", "---", "_Generated "+o.today()+
		" from `deck.yaml`. Edit the deck file, not this document._")
	return strings.Join(lines, "\n"), nil
}

// advancedSections is the advanced primer's headings and the note key under
// each, in order.
var advancedSections = [][2]string{
	{"Core engine", "engine_detail"},
	{"Lines and sequencing", "lines"},
	{"Win conditions", "wincons"},
	{"Mana base notes", "manabase"},
	{"Matchups", "matchups"},
	{"Politics and table talk", "politics"},
	{"Failure modes and how to recover", "failure_modes"},
	{"Sideboard / swap philosophy", "swap_philosophy"},
	{"Rules corners worth knowing", "rules_corners"},
}

// AdvancedPrimer is `generate.advanced_primer`: lines, sequencing, matchups,
// failure modes -- the prose only a person can supply, which is why it lives
// in `notes:` and survives regeneration instead of being retyped.
func AdvancedPrimer(d *deck.Deck, o Options) (string, error) {
	lines := []string{"# " + d.Name + " — Advanced Primer", "", header(d), ""}
	for _, section := range advancedSections {
		title, key := section[0], section[1]
		body, err := note(d, key, "_(set `notes."+key+"` in deck.yaml)_")
		if err != nil {
			return "", err
		}
		lines = append(lines, "## "+title, "", body, "")
	}
	if len(o.Stats) > 0 {
		lines = append(lines, "## Simulation results", "")
		for _, s := range o.Stats {
			lines = append(lines, fmt.Sprintf("- **%s:** %s", s.Key, s.Value))
		}
		lines = append(lines, "")
	}
	lines = append(lines, "---", "_Generated "+o.today()+" from `deck.yaml`._")
	return strings.Join(lines, "\n"), nil
}

// AnnotatedDecklist is `generate.annotated_decklist`: the 99 with a reason for
// every card, which is rule 4 rendered.
func AnnotatedDecklist(d *deck.Deck, o Options) (string, error) {
	lines := []string{"# " + d.Name + " — Annotated Decklist", "", header(d), ""}
	if strategy, _ := d.Strategy.(string); strategy != "" {
		lines = append(lines, strategy, "")
	}
	// `for cmd in deck.commander: ...; break` -- the first commander only, and
	// nothing at all when there is none.
	if len(d.Commander) > 0 {
		why, err := note(d, "commander_why", "_(set `notes.commander_why`)_")
		if err != nil {
			return "", err
		}
		lines = append(lines, "## Command Zone", "",
			"**"+d.Commander[0]+"** — "+why, "")
	}
	byCategory := map[string][]deck.CardEntry{}
	for _, c := range d.Cards {
		byCategory[c.Category] = append(byCategory[c.Category], c)
	}
	for _, cat := range orderedCategories(d) {
		entries := append([]deck.CardEntry{}, byCategory[cat]...)
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		title, ok := categoryTitles[cat]
		if !ok {
			title = pyTitle(cat)
		}
		total := 0
		for _, e := range entries {
			total += e.Qty
		}
		lines = append(lines, fmt.Sprintf("## %s (%d)", title, total), "")
		for _, entry := range entries {
			lines = append(lines, "- "+cardLine(entry, o.Cards[entry.Name]))
		}
		lines = append(lines, "")
	}
	if len(d.SwapBoard) > 0 {
		board := append([]deck.CardEntry{}, d.SwapBoard...)
		sort.SliceStable(board, func(i, j int) bool { return board[i].Name < board[j].Name })
		lines = append(lines, "## Swap Board (outside the 99)", "")
		for _, entry := range board {
			why := entry.Why
			if why == "" {
				why = "_(no note)_"
			}
			lines = append(lines, "- **"+entry.Name+"** — "+why)
		}
		lines = append(lines, "")
	}
	lines = append(lines, "---", "_Generated "+o.today()+" from `deck.yaml`._")
	return strings.Join(lines, "\n"), nil
}

func cardLine(entry deck.CardEntry, rec *pool.CardRecord) string {
	cost := ""
	if rec != nil && rec.ManaCost != nil && *rec.ManaCost != "" {
		cost = " `" + *rec.ManaCost + "`"
	}
	qty := ""
	if entry.Qty > 1 {
		qty = strconv.Itoa(entry.Qty) + "x "
	}
	why := entry.Why
	if why == "" {
		why = "_**no rationale recorded — this card should not ship**_"
	}
	return "**" + qty + entry.Name + "**" + cost + " — " + why
}

// MoxfieldText is `generate.moxfield_txt`. Moxfield has no public API, so
// plain text import is the supported path, and the `SIDEBOARD:` marker is
// where it reads a Commander deck's commander from.
func MoxfieldText(d *deck.Deck) string {
	cards := append([]deck.CardEntry{}, d.Cards...)
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Name < cards[j].Name })
	lines := []string{}
	for _, entry := range cards {
		lines = append(lines, strconv.Itoa(entry.Qty)+" "+entry.Name)
	}
	if len(d.Commander) > 0 || (d.Companion != nil && *d.Companion != "") {
		lines = append(lines, "", "SIDEBOARD:")
		for _, cmd := range d.Commander {
			lines = append(lines, "1 "+cmd)
		}
		if d.Companion != nil && *d.Companion != "" {
			lines = append(lines, "1 "+*d.Companion)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// SwapList is `generate.swap_list`: two versions of a deck diffed into an
// out/in list, plus a shopping list when prices are to hand.
//
// `previous` is the deck as of its last build, stashed at
// `artifacts/deck.last-built.yaml`. Decks are not in git (ADR 30), so the
// baseline is one the build keeps for itself -- and either way this document
// is a computed diff rather than a hand-kept changelog.
func SwapList(d, previous *deck.Deck, o Options) string {
	old := map[string]bool{}
	for _, c := range previous.Cards {
		old[c.Name] = true
	}
	fresh := map[string]bool{}
	for _, c := range d.Cards {
		fresh[c.Name] = true
	}
	cut, add := []string{}, []string{}
	for name := range old {
		if !fresh[name] {
			cut = append(cut, name)
		}
	}
	for name := range fresh {
		if !old[name] {
			add = append(add, name)
		}
	}
	sort.Strings(cut)
	sort.Strings(add)

	lines := []string{"# " + d.Name + " — Swap List", "",
		fmt.Sprintf("**%d out / %d in**", len(cut), len(add)), ""}
	lines = append(lines, "## Out", "")
	for _, name := range cut {
		lines = append(lines, "- **"+name+"** — "+firstWhy(previous.Cards, name, "_(cut)_"))
	}
	lines = append(lines, "", "## In", "")
	for _, name := range add {
		lines = append(lines, "- **"+name+"** — "+firstWhy(d.Cards, name, "_(added)_"))
	}

	if len(o.Prices) > 0 {
		type priced struct {
			name  string
			price float64
		}
		known := []priced{}
		unknown := []string{}
		for _, name := range add {
			if p, ok := o.Prices[name]; ok {
				known = append(known, priced{name, p})
			} else {
				unknown = append(unknown, name)
			}
		}
		// `floats.Fsum`, matching the `math.fsum` Python spells this with --
		// and both of them said `sum` until 2026-08-22. A `+=` loop here is
		// CPython 3.11's `sum()`, which is not CPython 3.12's: 3.12 gave
		// `sum()` over floats compensated accumulation, and 3.12 is what the
		// image runs. The bytes of these five files are the product, so a
		// total that depends on which interpreter rendered it is a defect
		// whether or not today's prices happen to expose it.
		amounts := make([]float64, 0, len(known))
		for _, k := range known {
			amounts = append(amounts, k.price)
		}
		total := floats.Fsum(amounts)
		lines = append(lines, "", "## Shopping list", "",
			"| Card | Cheapest non-foil (USD) |", "| --- | ---: |")
		// `sorted(known, key=lambda x: -x[1])` -- dearest first, and stable,
		// so cards at the same price keep the alphabetical order they arrived
		// in.
		ordered := append([]priced{}, known...)
		sort.SliceStable(ordered, func(i, j int) bool { return -ordered[i].price < -ordered[j].price })
		for _, k := range ordered {
			lines = append(lines, fmt.Sprintf("| %s | %s |", k.name, money(k.price)))
		}
		for _, name := range unknown {
			lines = append(lines, "| "+name+" | _no price data_ |")
		}
		lines = append(lines, fmt.Sprintf("| **Total (%d priced)** | **%s** |",
			len(known), money(total)))
		lines = append(lines, "", "### TCGplayer Mass Entry", "",
			"Paste into <https://www.tcgplayer.com/massentry>:", "", "```")
		for _, name := range add {
			lines = append(lines, "1 "+name)
		}
		lines = append(lines, "```")
	}

	lines = append(lines, "", "---", "_Generated "+o.today()+" by diffing deck.yaml._")
	return strings.Join(lines, "\n")
}

// firstWhy is `next((c for c in cards if c.name == name), None)` and the
// rationale off it, or the placeholder when the entry has none.
func firstWhy(cards []deck.CardEntry, name, fallback string) string {
	for _, c := range cards {
		if c.Name == name {
			if c.Why != "" {
				return c.Why
			}
			return fallback
		}
	}
	return fallback
}

// money is Python's `f"{price:.2f}"`. Both languages round the shortest
// decimal representation half to even, so the two agree on the ties as well
// as on everything else.
func money(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

// pyTitle is Python's `str.title()` for a category word: the first letter of
// each run of letters upper-cased and the rest lowered, so `sac-outlet`
// becomes `Sac-Outlet`. Only reachable for a category `CATEGORY_TITLES` has
// no heading for, which is a category the deck invented.
func pyTitle(s string) string {
	var b strings.Builder
	previousWasLetter := false
	for _, r := range s {
		switch {
		case !unicode.IsLetter(r):
			b.WriteRune(r)
			previousWasLetter = false
		case previousWasLetter:
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(unicode.ToTitle(r))
			previousWasLetter = true
		}
	}
	return b.String()
}

// manyNotes reads several notes in order, so the quick primer's four reads
// are one error check rather than four.
func manyNotes(d *deck.Deck, keys [][2]string) ([]string, error) {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		text, err := note(d, k[0], k[1])
		if err != nil {
			return nil, err
		}
		out = append(out, text)
	}
	return out, nil
}
