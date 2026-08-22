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
			wire.PyRepr(d.Status), strings.Join(model.DeckStatuses, ", ")), "")
	}
	if !contains(model.DeckStages, d.Stage) {
		rep.add("error", "deck-stage", fmt.Sprintf("stage %s is not one of %s",
			wire.PyRepr(d.Stage), strings.Join(model.DeckStages, ", ")), "")
	}
	drafting := d.Stage == "draft"

	archetypes := reference.Themes().Archetypes
	if d.LegacyArchetype != "" {
		detail := ""
		if reference.ArchetypeIndex(d.LegacyArchetype) < 0 {
			detail = fmt.Sprintf("archetype %s is not a class the boards know, so it counts for nothing; ",
				wire.PyRepr(d.LegacyArchetype))
		}
		rep.add("warn", "legacy-archetype", fmt.Sprintf("%s`archetype:` is a legacy key (ADR 37): declare a "+
			"strategy word in `themes` instead -- %s -- and the edit that does will drop this key itself",
			detail, strings.Join(archetypes, ", ")), "")
	}
	for _, theme := range d.Themes {
		if !reference.IsTheme(theme) {
			rep.add("warn", "unknown-theme", fmt.Sprintf("theme %s is not in the vocabulary; the list "+
				"grows by editing THEMES in decks/model.py", wire.PyRepr(theme)), "")
		}
	}

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
	if total := d.TotalCards(); total != expectedSize {
		because := ""
		if len(reasons) > 0 {
			because = " (" + strings.Join(reasons, ", ") + ")"
		}
		rep.add("error", "deck-size", fmt.Sprintf("deck has %d cards in the 99, expected %d%s",
			total, expectedSize, because), "")
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

	for _, card := range d.Cards {
		if !reference.IsCategory(card.Category) {
			rep.add("warn", "unknown-category", fmt.Sprintf("category %s is not one of %s",
				wire.PyRepr(card.Category), strings.Join(model.Categories, ", ")), card.Name)
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

	cmdRecords := []*pool.CardRecord{}
	for _, c := range d.Commander {
		if rec := cards[c]; rec != nil {
			cmdRecords = append(cmdRecords, rec)
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
					"it can be your commander%s", wire.PyRepr(rec.TypeLine), extra), rec.Name)
			}
		}
		if len(cmdRecords) == 2 {
			if problem := CheckPair(cmdRecords[0], cmdRecords[1]); problem != "" {
				rep.add("error", "illegal-pairing", problem, "")
			}
		}
		for _, card := range d.Cards {
			rec := cards[card.Name]
			if rec == nil {
				continue
			}
			if illegal := minus(setOf(rec.ColorIdentity), identity); len(illegal) > 0 {
				rep.add("error", "color-identity", fmt.Sprintf("identity %s includes %s, outside the commander's %s",
					braces(setOf(rec.ColorIdentity)), braces(illegal), bracesOrC(identity)), card.Name)
			}
			if !rec.LegalCommander {
				rep.add("error", "banned", "not legal in Commander", card.Name)
			}
			if card.Category == "land" && !rec.IsLand() {
				rep.add("warn", "category-mismatch", fmt.Sprintf("filed under 'land' but type line is %s",
					wire.PyRepr(rec.TypeLine)), card.Name)
			}
			if card.Category != "land" && rec.IsLand() {
				rep.add("warn", "category-mismatch", fmt.Sprintf("is a land but filed under %s",
					wire.PyRepr(card.Category)), card.Name)
			}
		}
	}

	// ---- companion --------------------------------------------------------
	if companion != "" {
		checkCompanion(d, companion, cards, identity, rep)
	}
	return rep
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

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
