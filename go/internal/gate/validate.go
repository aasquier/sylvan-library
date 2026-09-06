package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Issue is one finding: "error" blocks generation, "warn" does not.
type Issue struct {
	Level   string  `json:"-"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Card    *string `json:"card"`
}

// Report is `validate.ValidationReport`.
type Report struct {
	Issues []Issue
}

// Errors is every blocking issue.
func (r *Report) Errors() []Issue {
	out := []Issue{}
	for _, i := range r.Issues {
		if i.Level == "error" {
			out = append(out, i)
		}
	}
	return out
}

// Warnings is every non-blocking issue.
func (r *Report) Warnings() []Issue {
	out := []Issue{}
	for _, i := range r.Issues {
		if i.Level == "warn" {
			out = append(out, i)
		}
	}
	return out
}

// OK is `ValidationReport.ok`: no errors.
func (r *Report) OK() bool { return len(r.Errors()) == 0 }

func (r *Report) add(level, code, message string, card string) {
	issue := Issue{Level: level, Code: code, Message: message}
	if card != "" {
		c := card
		issue.Card = &c
	}
	r.Issues = append(r.Issues, issue)
}

// DefaultSize is the single-commander 99.
const DefaultSize = 99

func braces(colors map[string]bool) string {
	keys := make([]string, 0, len(colors))
	for c := range colors {
		keys = append(keys, c)
	}
	sort.Strings(keys)
	return "{" + strings.Join(keys, "") + "}"
}

func bracesOrC(colors map[string]bool) string {
	if len(colors) == 0 {
		return "{C}"
	}
	return braces(colors)
}

func setOf(list []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range list {
		out[s] = true
	}
	return out
}

func minus(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		if !b[k] {
			out[k] = true
		}
	}
	return out
}

// Validate is `validate.validate`: the gate. `cards` maps name -> record
// from the pool; nil runs only the structural checks and warns that the
// card-level checks were skipped -- it never silently claims the deck is
// fine. `expectedSize` is the single-commander default, adjusted here for a
// second commander and for Yorion.
func Validate(d *deck.Deck, cards map[string]*pool.CardRecord, expectedSize int) *Report {
	rep := &Report{Issues: []Issue{}}
	model := reference.Deck()

	// ---- structure --------------------------------------------------------
	if len(d.Commander) == 0 {
		rep.add("error", "no-commander", "deck has no commander", "")
	}
	if !contains(model.DeckStatuses, d.Status) {
		rep.add("error", "deck-status", fmt.Sprintf("status %s is not one of %s",
			wire.Quote(d.Status), strings.Join(model.DeckStatuses, ", ")), "")
	}
	if !contains(model.DeckStages, d.Stage) {
		rep.add("error", "deck-stage", fmt.Sprintf("stage %s is not one of %s",
			wire.Quote(d.Stage), strings.Join(model.DeckStages, ", ")), "")
	}
	drafting := d.Stage == "draft"

	archetypes := reference.Themes().Archetypes
	if d.LegacyArchetype != "" {
		detail := ""
		if reference.ArchetypeIndex(d.LegacyArchetype) < 0 {
			detail = fmt.Sprintf("archetype %s is not a class the boards know, so it counts for nothing; ",
				wire.Quote(d.LegacyArchetype))
		}
		rep.add("warn", "legacy-archetype", fmt.Sprintf("%s`archetype:` is a legacy key (ADR 37): declare a "+
			"strategy word in `themes` instead -- %s -- and the edit that does will drop this key itself",
			detail, strings.Join(archetypes, ", ")), "")
	}
	for _, theme := range d.Themes {
		if !reference.IsTheme(theme) {
			rep.add("warn", "unknown-theme", fmt.Sprintf("theme %s is not in the vocabulary, so no filter "+
				"will find this deck by it; the deck page's labels editor sets themes from the "+
				"curated list", wire.Quote(theme)), "")
		}
	}

	// The commanders' own records, read here rather than with the rest of the
	// card-level checks because one Rulebreaker clause changes how big the
	// deck is allowed to be, and the size check is above. `cards` may be nil;
	// indexing a nil map is the same "not found" every missing name gets, so
	// no pool means no clauses and the size check below is untouched.
	cmdRecords := []*pool.CardRecord{}
	for _, c := range d.Commander {
		if rec := cards[c]; rec != nil {
			cmdRecords = append(cmdRecords, rec)
		}
	}
	breakers := ReadRulebreakers(cmdRecords)

	reasons := []string{}
	if len(d.Commander) > 1 {
		expectedSize -= len(d.Commander) - 1
		reasons = append(reasons, fmt.Sprintf("%d commanders", len(d.Commander)))
	}
	companion := ""
	if d.Companion != nil {
		companion = *d.Companion
	}
	if bonus := DeckSizeBonus[strings.ToLower(companion)]; bonus != 0 {
		expectedSize += bonus
		reasons = append(reasons, fmt.Sprintf("+%d for %s", bonus, companion))
	}
	if len(d.Commander) > 2 {
		rep.add("error", "too-many-commanders", fmt.Sprintf("%d commanders listed; Commander allows at "+
			"most two, and only with a pairing ability", len(d.Commander)), "")
	}
	// A Rulebreaker clause can lift the ceiling off the deck (Whtz), which
	// turns the number into a floor: too few cards is still an error, too many
	// is the card doing what it says. Nothing here touches the singleton rule,
	// which is a separate rule and still applies to all 200 of them.
	noCeiling := breakers.NoMaximumDeckSize()
	total := d.TotalCards()
	tooFew, tooMany := total < expectedSize, total > expectedSize && !noCeiling
	if tooFew || tooMany {
		because := ""
		if len(reasons) > 0 {
			because = " (" + strings.Join(reasons, ", ") + ")"
		}
		if noCeiling {
			rep.add("error", "deck-size", fmt.Sprintf("deck has %d cards in the 99, expected at least %d%s "+
				"-- the commander's Rulebreaker lifts the maximum, not the minimum",
				total, expectedSize, because), "")
		} else {
			rep.add("error", "deck-size", fmt.Sprintf("deck has %d cards in the 99, expected %d%s",
				total, expectedSize, because), "")
		}
	}

	seen := map[string]int{}
	seenOrder := []string{}
	for _, card := range d.Cards {
		key := strings.ToLower(card.Name)
		if _, there := seen[key]; !there {
			seenOrder = append(seenOrder, key)
		}
		seen[key] += card.Qty
	}
	for _, name := range seenOrder {
		if count := seen[name]; count > 1 && !reference.IsSingletonExempt(name) {
			rep.add("error", "singleton", fmt.Sprintf("appears %d times", count), name)
		}
	}
	for _, cmd := range d.Commander {
		if _, there := seen[strings.ToLower(cmd)]; there {
			rep.add("error", "commander-in-99", "commander is also listed in the 99", cmd)
		}
	}

	pending := d.Unjustified()
	if drafting && len(pending) > 0 {
		shown := []string{}
		for i, c := range pending {
			if i == 6 {
				break
			}
			shown = append(shown, c.Name)
		}
		more := ""
		if len(pending) > 6 {
			more = fmt.Sprintf(", and %d more", len(pending)-6)
		}
		rep.add("warn", "draft-incomplete", fmt.Sprintf("%d of %d cards still need a `why` (%s%s). Write them, "+
			"then set `stage: curated` -- the gate refuses the promotion while any card is still blank",
			len(pending), len(d.Cards), strings.Join(shown, ", "), more), "")
	}

	checkCombos(d, rep)

	for _, card := range d.Cards {
		if !reference.IsCategory(card.Category) {
			rep.add("warn", "unknown-category", fmt.Sprintf("category %s is not one of %s",
				wire.Quote(card.Category), strings.Join(model.Categories, ", ")), card.Name)
		}
		if strings.TrimSpace(card.Why) == "" && !drafting {
			rep.add("error", "missing-rationale", "no `why` -- every inclusion must justify itself", card.Name)
		}
	}

	// ---- card-level -------------------------------------------------------
	if cards == nil {
		rep.add("warn", "unverified", "no card pool supplied; identity, legality and text were NOT checked", "")
		return rep
	}

	allNames := append([]string{}, d.Commander...)
	for _, c := range d.Cards {
		allNames = append(allNames, c.Name)
	}
	for _, c := range d.SwapBoard {
		allNames = append(allNames, c.Name)
	}
	if companion != "" {
		allNames = append(allNames, companion)
	}
	for _, name := range allNames {
		if cards[name] == nil {
			rep.add("error", "unknown-card", "not found in the local pool -- check spelling, or refresh "+
				"the Scryfall data if this is a new card", name)
		}
	}

	// The same question one block over, answered a softer way. A name in the
	// 99 that nobody can look up makes the deck's legality a guess, which is an
	// error; a name in the combos block that nobody can look up costs the entry
	// its picture and nothing else. The deck is still a deck.
	for _, name := range d.ComboNames() {
		if cards[name] == nil {
			rep.add("warn", "combo-unknown-card", "named in the combos but not found in the local "+
				"pool, so it has no card to show -- check the spelling against the card's printed "+
				"name, or refresh the Scryfall data if this is a new card", name)
		}
	}

	var identity map[string]bool
	if len(cmdRecords) > 0 {
		identity = map[string]bool{}
		for _, rec := range cmdRecords {
			for _, c := range rec.ColorIdentity {
				identity[c] = true
			}
		}
		paired := len(cmdRecords) > 1
		for _, rec := range cmdRecords {
			if !CanBeCommander(rec, paired) {
				extra := ""
				if NonlegendaryPartner(rec) {
					extra = " -- a nonlegendary creature can't be your commander even with a 'partner with' ability"
				} else if !paired && IsBackground(rec) {
					extra = " -- a Background is only legal as a second commander, and this deck lists one"
				}
				rep.add("error", "not-a-commander", fmt.Sprintf("type line is %s and it does not say "+
					"it can be your commander%s", wire.Quote(rec.TypeLine), extra), rec.Name)
			}
		}
		if len(cmdRecords) == 2 {
			if problem := CheckPair(cmdRecords[0], cmdRecords[1]); problem != "" {
				rep.add("error", "illegal-pairing", problem, "")
			}
		}
		// A clause nobody could read is said out loud. The identity check
		// below then runs as though the clause were not printed, which will
		// call legal cards illegal -- so the deck is told that is what
		// happened, rather than left to argue with a report that is quietly
		// working from a rule it could not read.
		for _, rb := range breakers.Unread() {
			rep.add("warn", "rulebreaker-unread", fmt.Sprintf("its Rulebreaker was NOT applied -- %s. "+
				"Colour identity below was checked as though the card did not have one, so any card the "+
				"clause makes legal is reported as an error. Clause: %s",
				rb.Unsupported, rb.Text), rb.Commander)
		}
		// Tolabow's clause is a colour the deck picks by playing it; the loop
		// collects what the instants and sorceries actually reach, and the
		// count is judged once, after.
		chosen := []Entry{}
		for _, card := range d.Cards {
			rec := cards[card.Name]
			if rec == nil {
				continue
			}
			if illegal := minus(setOf(rec.ColorIdentity), identity); len(illegal) > 0 {
				switch {
				case breakers.AnyIdentity(rec):
					// The commander says so in as many words: not an error,
					// and not a warning either. A legal card is legal.
				case breakers.OneChosenColor(rec):
					chosen = append(chosen, Entry{Name: card.Name, Rec: rec})
				default:
					rep.add("error", "color-identity", fmt.Sprintf("identity %s includes %s, outside the commander's %s%s",
						braces(setOf(rec.ColorIdentity)), braces(illegal), bracesOrC(identity),
						breakers.Why()), card.Name)
				}
			}
			if !rec.LegalCommander {
				rep.add("error", "banned", "not legal in Commander", card.Name)
			}
			if card.Category == "land" && !rec.IsLand() {
				rep.add("warn", "category-mismatch", fmt.Sprintf("filed under 'land' but type line is %s",
					wire.Quote(rec.TypeLine)), card.Name)
			}
			if card.Category != "land" && rec.IsLand() {
				rep.add("warn", "category-mismatch", fmt.Sprintf("is a land but filed under %s",
					wire.Quote(card.Category)), card.Name)
			}
		}
		if colors, by := breakers.ColorChoice(chosen, identity); len(colors) > 1 {
			reach := []string{}
			for _, c := range colors {
				reach = append(reach, fmt.Sprintf("{%s} (%s)", c, strings.Join(shortList(by[c]), ", ")))
			}
			rep.add("error", "rulebreaker-color-choice", fmt.Sprintf("the commander's Rulebreaker lets these "+
				"cards reach ONE colour past the commander's %s, and this deck reaches %d: %s. Keep the "+
				"colour you want and cut the rest, or move them to the swap board",
				bracesOrC(identity), len(colors), strings.Join(reach, "; ")), "")
		}
	}

	// ---- companion --------------------------------------------------------
	if companion != "" {
		checkCompanion(d, companion, cards, identity, rep)
	}
	return rep
}

// checkCombos reads the combos block against the deck it describes.
//
// **Warnings, every one of them, and that is the decision rather than the
// default.** A combos block is a reading of the deck, not part of it: a deck
// whose catalogue has drifted is a deck with a stale note, and refusing to
// generate its artifacts over one would be the gate holding a primer hostage to
// a paragraph. It is also commandment 2 -- somebody who catalogues their first
// machine and gets four red errors for it has been told they did it wrong, when
// what happened is that they cut a card afterwards.
//
// Nothing here asks whether the Magic is right. Whether two cards actually go
// infinite together is not a question a deterministic check can answer (ADR
// 14), and pretending otherwise would be the gate having an opinion. What it
// checks is whether the entry still matches the deck sitting next to it.
func checkCombos(d *deck.Deck, rep *Report) {
	in99 := map[string]bool{}
	for _, card := range d.Cards {
		in99[strings.ToLower(strings.TrimSpace(card.Name))] = true
	}
	for _, name := range d.Commander {
		in99[strings.ToLower(strings.TrimSpace(name))] = true
	}
	has := func(name string) bool {
		return in99[strings.ToLower(strings.TrimSpace(name))]
	}

	for _, combo := range d.Combos {
		where := combo.Heading()
		for _, piece := range combo.Cards {
			if has(piece) {
				continue
			}
			// The commonest way a catalogue goes stale: the machine was real,
			// and then a piece of it was cut. Named as the piece rather than as
			// the entry, so the row points at the card to put back.
			rep.add("warn", "combo-piece-missing", fmt.Sprintf("is listed as a piece of "+
				"%s but is not in the 99 -- either it was cut and the combo no longer "+
				"assembles, or it belongs in the entry's `needs` as the card this deck "+
				"is still looking for", wire.Quote(where)), piece)
		}
		if !combo.NearMiss() {
			continue
		}
		if has(combo.Needs) {
			rep.add("warn", "combo-needs-in-99", fmt.Sprintf("is marked as the card %s is "+
				"waiting for, but it is already in the deck -- this machine is complete, "+
				"so move it up into the entry's `cards` and drop the trade",
				wire.Quote(where)), combo.Needs)
		}
		if strings.TrimSpace(combo.Cut) == "" {
			// Aaron's rule, checked rather than trusted: a suggestion the deck
			// cannot act on is not a suggestion.
			rep.add("warn", "combo-no-cut", fmt.Sprintf("%s is one card short and does not "+
				"say what would come out for it; a card to bring in is only a suggestion "+
				"once there is a slot for it", wire.Quote(where)), combo.Needs)
		} else if !has(combo.Cut) {
			rep.add("warn", "combo-cut-missing", fmt.Sprintf("is offered as the cut that "+
				"makes room for %s, but it is not in the 99 -- the slot it would free is "+
				"already free", wire.Quote(combo.Needs)), combo.Cut)
		}
	}
}

// checkCompanion is `validate._check_companion`: the companion itself and
// its deckbuilding restriction.
func checkCompanion(d *deck.Deck, name string, cards map[string]*pool.CardRecord,
	identity map[string]bool, rep *Report) {
	rec := cards[name]
	if rec == nil {
		return // already reported as unknown-card
	}
	if !IsCompanion(rec) {
		rep.add("error", "not-a-companion", "listed as companion but has no Companion ability", name)
		return
	}
	if !rec.LegalCommander {
		rep.add("error", "companion-banned", "not legal in Commander, so it cannot be your companion", name)
	}
	// **A Rulebreaker clause deliberately does not reach here.** Every printed
	// one says what "a deck with this commander can have", and a companion is
	// not in the deck -- it waits outside the hundred and is bought into the
	// hand. The reading is also moot in fact: no companion is an Angel, an
	// Aura, a Phyrexian, an Equipment, a land or a seven-drop, so none of the
	// clauses names one. It is written down because the next reader will
	// wonder, and a silence is not an answer.
	if identity != nil {
		if illegal := minus(setOf(rec.ColorIdentity), identity); len(illegal) > 0 {
			rep.add("error", "companion-color-identity", fmt.Sprintf("identity %s includes %s, outside the commander's %s",
				braces(setOf(rec.ColorIdentity)), braces(illegal), bracesOrC(identity)), name)
		}
	}
	for _, c := range d.Cards {
		if strings.EqualFold(c.Name, name) {
			rep.add("error", "companion-in-99", "the companion sits outside the 100, not in the deck", name)
			break
		}
	}
	entries := []Entry{}
	for _, c := range d.Cards {
		if r := cards[c.Name]; r != nil {
			entries = append(entries, Entry{Name: c.Name, Rec: r})
		}
	}
	for _, n := range d.Commander {
		if r := cards[n]; r != nil {
			entries = append(entries, Entry{Name: n, Rec: r})
		}
	}
	result := CheckCompanion(name, entries, cards)
	if result.Unsupported != "" {
		condition := result.Condition
		if condition == "" {
			condition = "unknown"
		}
		rep.add("warn", "companion-unchecked", fmt.Sprintf("deckbuilding restriction was NOT verified -- %s. Condition: %s",
			result.Unsupported, condition), name)
		return
	}
	if len(result.Violations) > 0 {
		sorted := append([]string{}, result.Violations...)
		sort.Strings(sorted)
		shown := sorted
		if len(shown) > 6 {
			shown = shown[:6]
		}
		text := strings.Join(shown, ", ")
		if more := len(sorted) - 6; more > 0 {
			text += fmt.Sprintf(", and %d more", more)
		}
		level, detail := "error", ""
		if !result.Exact {
			level, detail = "warn", " (heuristic check -- verify by hand)"
		}
		rep.add(level, "companion-restriction", fmt.Sprintf("%d card(s) break the companion "+
			"restriction%s: %s. Condition: %s", len(result.Violations), detail, text, result.Condition), name)
	}
}

// shortList is the report's house habit -- name a few, count the rest -- for a
// message that has to fit beside two or three others on one line.
func shortList(names []string) []string {
	if len(names) <= 3 {
		return names
	}
	return append(append([]string{}, names[:3]...), fmt.Sprintf("and %d more", len(names)-3))
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
