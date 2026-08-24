package gate

import (
	"regexp"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The companion deckbuilding restrictions. Two rules shape this file, and
// both are load-bearing: the condition text comes from the pool, never from
// memory (Kaheera's allowed creature types are parsed out of her own oracle
// text), and an unknown condition is reported, never passed -- a loud "not checked" rather than a silent "no violations" for a
// rule that was never evaluated.

// Entry is one card of the starting deck -- the 99 and the commander, never
// the companion itself -- as (name, record).
type Entry struct {
	Name string
	Rec  *pool.CardRecord
}

// CompanionCheck is the result of checking one companion's restriction.
type CompanionCheck struct {
	Condition  string
	Violations []string
	// Exact is false when the check is a heuristic rather than an exact
	// reading of the card data; callers report those as warnings.
	Exact bool
	// Unsupported is set when the condition cannot be evaluated from oracle
	// data at all.
	Unsupported string
}

// PermanentTypes are the card types that make a card a permanent card.
var PermanentTypes = []string{"Artifact", "Creature", "Enchantment", "Land", "Planeswalker", "Battle"}

// activatedKeywords are keyword abilities that ARE activated abilities but
// are printed without a colon when Scryfall omits reminder text.
var activatedKeywords = []string{
	"equip", "cycling", "level up", "crew", "unearth", "channel", "forecast",
	"fortify", "reconfigure", "outlast", "monstrosity", "adapt", "scavenge",
	"transfigure", "transmute", "morph", "megamorph", "disguise", "prototype",
	"boast", "exhaust",
}

var (
	companionLine = regexp.MustCompile(`(?i)^(old )?companion\s*—`)
	reminderTail  = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
	reminderAny   = regexp.MustCompile(`\([^)]*\)`)
	manaSymbols   = regexp.MustCompile(`\{([^}]+)\}`)
	creatureTypes = regexp.MustCompile(`is an? (.+?) card`)
	typeListSplit = regexp.MustCompile(`,\s*|\s+or\s+`)
)

func isPermanent(rec *pool.CardRecord) bool {
	front := Front(rec.TypeLine)
	for _, t := range PermanentTypes {
		if strings.Contains(front, t) {
			return true
		}
	}
	return false
}

// isLandFace is the card's own front face, deliberately not CardRecord.IsLand:
// a modal DFC with a land back is a nonland *card*.
func isLandFace(rec *pool.CardRecord) bool {
	return strings.Contains(Front(rec.TypeLine), "Land")
}

// manaSymbolsOf is `_mana_symbols`: colour, hybrid and colourless symbols;
// generic and {X} excluded.
func manaSymbolsOf(cost *string) []string {
	out := []string{}
	if cost == nil {
		return out
	}
	for _, m := range manaSymbols.FindAllStringSubmatch(*cost, -1) {
		sym := m[1]
		if isDigits(sym) || strings.EqualFold(sym, "X") {
			continue
		}
		out = append(out, strings.ToUpper(sym))
	}
	return out
}

func isDigits(s string) bool {
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

// hasActivatedAbility is the heuristic `_has_activated_ability`.
func hasActivatedAbility(rec *pool.CardRecord) bool {
	for _, line := range strings.Split(rec.OracleText, "\n") {
		bare := reminderAny.ReplaceAllString(line, "")
		if strings.Contains(bare, ":") {
			return true
		}
		lowered := strings.ToLower(strings.TrimSpace(bare))
		for _, kw := range activatedKeywords {
			if strings.HasPrefix(lowered, kw) {
				return true
			}
		}
	}
	return strings.Contains(Front(rec.TypeLine), "Planeswalker")
}

// ---- the conditions ---------------------------------------------------------

func evenManaValues(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	out := []string{}
	for _, e := range entries {
		if int(e.Rec.CMC)%2 != 0 {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

func oddManaValuesOrLand(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	out := []string{}
	for _, e := range entries {
		if !isLandFace(e.Rec) && int(e.Rec.CMC)%2 == 0 {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

func mvThreeOrGreaterOrLand(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	out := []string{}
	for _, e := range entries {
		if !isLandFace(e.Rec) && e.Rec.CMC < 3 {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

func permanentMvTwoOrLess(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	out := []string{}
	for _, e := range entries {
		if isPermanent(e.Rec) && e.Rec.CMC > 2 {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

// creatureTypesOf is Kaheera: the allowed types are read out of her own text.
func creatureTypesOf(entries []Entry, rec *pool.CardRecord) ([]string, error) {
	m := creatureTypes.FindStringSubmatch(rec.OracleText)
	if m == nil {
		return nil, errParse("could not parse the allowed creature types")
	}
	allowed := []string{}
	for _, t := range typeListSplit.Split(m[1], -1) {
		if t = strings.TrimSpace(t); t != "" {
			allowed = append(allowed, t)
		}
	}
	out := []string{}
	for _, e := range entries {
		front := Front(e.Rec.TypeLine)
		if !strings.Contains(front, "Creature") {
			continue
		}
		ok := false
		for _, t := range allowed {
			if strings.Contains(front, t) {
				ok = true
				break
			}
		}
		if !ok {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

type errParse string

func (e errParse) Error() string { return string(e) }

func noRepeatedManaSymbol(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	out := []string{}
	for _, e := range entries {
		syms := manaSymbolsOf(e.Rec.ManaCost)
		seen := map[string]bool{}
		for _, s := range syms {
			seen[s] = true
		}
		if len(syms) != len(seen) {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

func distinctNonlandNames(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	seen := map[string]int{}
	for _, e := range entries {
		if !isLandFace(e.Rec) {
			seen[e.Name]++
		}
	}
	out := []string{}
	for n, c := range seen {
		if c > 1 {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

func nonlandSharesAType(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	nonland := []Entry{}
	for _, e := range entries {
		if !isLandFace(e.Rec) {
			nonland = append(nonland, e)
		}
	}
	if len(nonland) == 0 {
		return []string{}, nil
	}
	candidates := append(append([]string{}, PermanentTypes...), "Instant", "Sorcery")
	shared := map[string]bool{}
	for _, t := range candidates {
		shared[t] = true
	}
	for _, e := range nonland {
		front := Front(e.Rec.TypeLine)
		for t := range shared {
			if !strings.Contains(front, t) {
				delete(shared, t)
			}
		}
	}
	if len(shared) > 0 {
		return []string{}, nil
	}
	// No single type is common to all of them. Report the minority so the
	// message is actionable: the type most of them share, and the rest.
	counts := map[string]int{}
	for _, e := range nonland {
		front := Front(e.Rec.TypeLine)
		for _, t := range candidates {
			if strings.Contains(front, t) {
				counts[t]++
			}
		}
	}
	out := []string{}
	if len(counts) == 0 {
		for _, e := range nonland {
			out = append(out, e.Name)
		}
		return out, nil
	}
	best, bestN := "", -1
	for _, t := range candidates { // a fixed order breaks ties the same way every run
		if counts[t] > bestN {
			best, bestN = t, counts[t]
		}
	}
	for _, e := range nonland {
		if !strings.Contains(Front(e.Rec.TypeLine), best) {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

func permanentsHaveActivatedAbilities(entries []Entry, _ *pool.CardRecord) ([]string, error) {
	out := []string{}
	for _, e := range entries {
		if isPermanent(e.Rec) && !hasActivatedAbility(e.Rec) {
			out = append(out, e.Name)
		}
	}
	return out, nil
}

type checker struct {
	fn    func([]Entry, *pool.CardRecord) ([]string, error)
	exact bool
}

// checks is `_CHECKS`: name -> (checker, exact). Yorion restricts deck SIZE
// and is handled by the size check; its entry scans nothing.
var checks = map[string]checker{
	"gyruda, doom of depths":   {evenManaValues, true},
	"obosh, the preypiercer":   {oddManaValuesOrLand, true},
	"keruga, the macrosage":    {mvThreeOrGreaterOrLand, true},
	"lurrus of the dream-den":  {permanentMvTwoOrLess, true},
	"kaheera, the orphanguard": {creatureTypesOf, true},
	"jegantha, the wellspring": {noRepeatedManaSymbol, true},
	"lutri, the spellchaser":   {distinctNonlandNames, true},
	"umori, the collector":     {nonlandSharesAType, true},
	"zirda, the dawnwaker":     {permanentsHaveActivatedAbilities, false},
	"yorion, sky nomad":        {func([]Entry, *pool.CardRecord) ([]string, error) { return []string{}, nil }, true},
}

// DeckSizeBonus is `DECK_SIZE_BONUS`: Yorion is the one companion that
// changes how big the deck must be.
var DeckSizeBonus = map[string]int{"yorion, sky nomad": 20}

// uncheckable is `_UNCHECKABLE`: conditions that reference something an
// oracle card does not carry.
var uncheckable = map[string]string{
	"lutri, pauper otter": "the condition is about expansion symbols, which are " +
		"a property of a printing rather than an oracle card",
	"treizeci, sun of serra": "the condition is about retro frames and other " +
		"'nostalgic' treatments, which are per-printing",
	"the companion of the wilds": "the condition names specific sets, which " +
		"oracle data does not carry",
}

// ConditionText is the Companion sentence from a card's oracle text,
// reminder stripped.
func ConditionText(rec *pool.CardRecord) string {
	for _, line := range strings.Split(rec.OracleText, "\n") {
		if companionLine.MatchString(strings.TrimSpace(line)) {
			return strings.TrimSpace(reminderTail.ReplaceAllString(line, ""))
		}
	}
	return ""
}

// IsCompanion: does this card actually have a Companion ability?
func IsCompanion(rec *pool.CardRecord) bool { return ConditionText(rec) != "" }

// CheckCompanion is `companion.check`: a companion's restriction against the
// starting deck.
func CheckCompanion(name string, entries []Entry, cards map[string]*pool.CardRecord) CompanionCheck {
	rec := cards[name]
	if rec == nil {
		return CompanionCheck{Unsupported: "card not in the pool"}
	}
	condition := ConditionText(rec)
	if condition == "" {
		return CompanionCheck{Unsupported: "card has no Companion ability"}
	}
	key := strings.ToLower(name)
	if why, ok := uncheckable[key]; ok {
		return CompanionCheck{Condition: condition, Unsupported: why}
	}
	c, ok := checks[key]
	if !ok {
		return CompanionCheck{Condition: condition, Unsupported: "no checker is implemented for this companion"}
	}
	violations, err := c.fn(entries, rec)
	if err != nil {
		return CompanionCheck{Condition: condition, Unsupported: err.Error()}
	}
	return CompanionCheck{Condition: condition, Violations: violations, Exact: c.exact}
}
