// Package analyze is `decks/analyze.py`: deterministic deck analysis, a pure
// function of a deck plus a card pool -- curve, pip demand against source
// supply, category coverage against conventional targets, the Game Changer
// count against the declared bracket, the opening-hand arithmetic. No
// database, no judgement that varies between runs; the same deck always
// produces the same report, and the API is a thin wrapper over it.
package analyze

import (
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/mana"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// Bucket is one rung of the curve.
type Bucket struct {
	MV    int      `json:"mv"`
	Count int      `json:"count"`
	Names []string `json:"names"`
}

// Curve is `analyze.curve`, in the key order `service.stats_for` writes.
type Curve struct {
	AverageMV    float64  `json:"average_mv"`
	NonlandCards int      `json:"nonland_cards"`
	Buckets      []Bucket `json:"buckets"`
}

// Category is one row of `category_report`.
type Category struct {
	Category   string  `json:"category"`
	Count      int     `json:"count"`
	TargetLow  *int    `json:"target_low"`
	TargetHigh *int    `json:"target_high"`
	Status     *string `json:"status"`
}

// GameChangers is `analyze.game_changers`.
type GameChangers struct {
	Cards   []string `json:"cards"`
	Count   int      `json:"count"`
	Allowed *int     `json:"allowed"`
	Bracket *int     `json:"bracket"`
	Verdict string   `json:"verdict"`
}

// LandRow is one row of the opening hand's land distribution.
type LandRow struct {
	Lands  int     `json:"lands"`
	Chance float64 `json:"chance"`
}

// Lands is the opening hand's land odds.
type Lands struct {
	Count        int       `json:"count"`
	Distribution []LandRow `json:"distribution"`
	Keepable     float64   `json:"keepable"`
}

// CategoryOdds is one category's seen-it-by odds.
type CategoryOdds struct {
	Category      string  `json:"category"`
	Count         int     `json:"count"`
	InOpeningHand float64 `json:"in_opening_hand"`
	ByTurnFour    float64 `json:"by_turn_four"`
}

// Singleton is the chance of having seen one particular card by a turn.
type Singleton struct {
	Turn      int     `json:"turn"`
	CardsSeen int     `json:"cards_seen"`
	Chance    float64 `json:"chance"`
}

// Opening is `analyze.opening_hand`.
type Opening struct {
	DeckSize   int            `json:"deck_size"`
	HandSize   int            `json:"hand_size"`
	Lands      Lands          `json:"lands"`
	Categories []CategoryOdds `json:"categories"`
	Singleton  []Singleton    `json:"singleton"`
}

// ColorNeed is demand for one colour, and the supply available to meet it.
type ColorNeed struct {
	Color         string  `json:"color"`
	Pips          int     `json:"pips"`
	Sources       int     `json:"sources"`
	Cards         int     `json:"cards"`
	SourcesPerPip float64 `json:"sources_per_pip"`
}

// Stats is `analyze.deck_stats`, as `service.stats_for` serialises it.
type Stats struct {
	Slug         string        `json:"slug"`
	Name         string        `json:"name"`
	Commander    []string      `json:"commander"`
	Bracket      *int          `json:"bracket"`
	TotalCards   int           `json:"total_cards"`
	LandCount    int           `json:"land_count"`
	Curve        Curve         `json:"curve"`
	Categories   []Category    `json:"categories"`
	GameChangers GameChangers  `json:"game_changers"`
	Opening      Opening       `json:"opening"`
	Colors       []ColorNeed   `json:"colors"`
	Types        orderedCounts `json:"types"`
}

// orderedCounts serialises as an object in its own order -- Python's dict
// keeps the order `type_breakdown` sorted it into, and a Go map would not.
type orderedCounts struct {
	Keys   []string
	Counts map[string]int
}

// MarshalJSON writes the counts as an object, highest first.
func (o orderedCounts) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range o.Keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(k))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(o.Counts[k]))
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// isLand is `analyze._is_land`: the whole type line, as Python reads it here.
func isLand(rec *pool.CardRecord) bool {
	return rec != nil && strings.Contains(rec.TypeLine, "Land")
}

// round2 is Python's `round(x, 2)`: half to even on the scaled value.
func round2(x float64) float64 {
	return math.RoundToEven(x*100) / 100
}

func costOf(entry deck.CardEntry, rec *pool.CardRecord) string {
	if entry.ManaCost != nil && *entry.ManaCost != "" {
		return *entry.ManaCost
	}
	if rec != nil && rec.ManaCost != nil {
		return *rec.ManaCost
	}
	return ""
}

// CurveOf is `analyze.curve`: the MV histogram over the nonland 99.
func CurveOf(d *deck.Deck, cards map[string]*pool.CardRecord) Curve {
	buckets := map[int]*Bucket{}
	totalMV, counted := 0, 0
	for _, entry := range d.Cards {
		rec := cards[entry.Name]
		if entry.Category == "land" || isLand(rec) {
			continue
		}
		mv := mana.Parse(costOf(entry, rec)).ManaValue()
		b, ok := buckets[mv]
		if !ok {
			b = &Bucket{MV: mv, Names: []string{}}
			buckets[mv] = b
		}
		b.Count += entry.Qty
		b.Names = append(b.Names, entry.Name)
		totalMV += mv * entry.Qty
		counted += entry.Qty
	}
	mvs := make([]int, 0, len(buckets))
	for mv := range buckets {
		mvs = append(mvs, mv)
	}
	sort.Ints(mvs)
	out := Curve{NonlandCards: counted, Buckets: []Bucket{}}
	for _, mv := range mvs {
		out.Buckets = append(out.Buckets, *buckets[mv])
	}
	if counted > 0 {
		out.AverageMV = round2(float64(totalMV) / float64(counted))
	}
	return out
}

// ColorSources is `analyze.color_sources`: how many permanents can produce
// each colour, from Scryfall's `produced_mana`.
func ColorSources(d *deck.Deck, cards map[string]*pool.CardRecord) map[string]int {
	counts := map[string]int{}
	for _, c := range mana.Colors {
		counts[c] = 0
	}
	for _, entry := range d.Cards {
		rec := cards[entry.Name]
		if rec == nil {
			continue
		}
		for _, color := range rec.ProducedMana {
			if _, ok := counts[color]; ok {
				counts[color] += entry.Qty
			}
		}
	}
	return counts
}

// CommanderIdentity is `analyze.commander_identity`: the deck's legal
// colours from the commander's Scryfall identity, never from mana costs.
func CommanderIdentity(d *deck.Deck, cards map[string]*pool.CardRecord) map[string]bool {
	identity := map[string]bool{}
	for _, name := range d.Commander {
		if rec := cards[name]; rec != nil {
			for _, c := range rec.ColorIdentity {
				identity[c] = true
			}
		}
	}
	return identity
}

// PipRequirements is `analyze.pip_requirements`: coloured pip demand against
// coloured source supply, per colour inside the commander's identity.
func PipRequirements(d *deck.Deck, cards map[string]*pool.CardRecord) []ColorNeed {
	pips := map[string]int{}
	carders := map[string]int{}
	for _, entry := range d.Cards {
		rec := cards[entry.Name]
		if entry.Category == "land" || isLand(rec) {
			continue
		}
		seen := map[string]bool{}
		for _, pip := range mana.Parse(costOf(entry, rec)).Pips {
			for _, color := range pip {
				if isColor(color) {
					pips[color] += entry.Qty
					seen[color] = true
				}
			}
		}
		for color := range seen {
			carders[color] += entry.Qty
		}
	}
	sources := ColorSources(d, cards)
	identity := CommanderIdentity(d, cards)
	relevant := identity
	if len(relevant) == 0 {
		relevant = map[string]bool{}
		for _, c := range mana.Colors {
			if pips[c] > 0 {
				relevant[c] = true
			}
		}
	}
	out := []ColorNeed{}
	for _, c := range mana.Colors {
		if !relevant[c] {
			continue
		}
		need := ColorNeed{Color: c, Pips: pips[c], Sources: sources[c], Cards: carders[c]}
		if need.Pips > 0 {
			need.SourcesPerPip = round2(float64(need.Sources) / float64(need.Pips))
		}
		out = append(out, need)
	}
	return out
}

func isColor(c string) bool {
	for _, col := range mana.Colors {
		if col == c {
			return true
		}
	}
	return false
}

// CategoryReport is `analyze.category_report`: counts against conventional
// targets, reported with `target: None` where there is no target.
func CategoryReport(d *deck.Deck) []Category {
	counts, _ := d.CategoryCounts()
	model := reference.Deck()
	out := []Category{}
	for _, category := range model.Categories {
		row := Category{Category: category, Count: counts[category]}
		if target, ok := model.CategoryTargets[category]; ok && len(target) == 2 {
			lo, hi := target[0], target[1]
			row.TargetLow, row.TargetHigh = &lo, &hi
			status := "ok"
			if row.Count < lo {
				status = "low"
			} else if row.Count > hi {
				status = "high"
			}
			row.Status = &status
		}
		out = append(out, row)
	}
	return out
}

// TypeBreakdown is `analyze.type_breakdown`: counts by primary card type,
// taking the front face, highest first (ties in first-seen order).
func TypeBreakdown(d *deck.Deck, cards map[string]*pool.CardRecord) orderedCounts {
	counts := map[string]int{}
	order := []string{}
	bump := func(key string, n int) {
		if _, seen := counts[key]; !seen {
			order = append(order, key)
		}
		counts[key] += n
	}
	for _, entry := range d.Cards {
		rec := cards[entry.Name]
		if rec == nil {
			bump("Unknown", entry.Qty)
			continue
		}
		front, _, _ := strings.Cut(rec.TypeLine, " // ")
		left, _, _ := strings.Cut(front, "—")
		fields := strings.Fields(strings.TrimSpace(left))
		primary := "Unknown"
		if len(fields) > 0 {
			primary = fields[len(fields)-1]
		}
		bump(primary, entry.Qty)
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	return orderedCounts{Keys: order, Counts: counts}
}

// GameChangersOf is `analyze.game_changers`: which Game Changers the deck
// runs and what its bracket allows. `unknown` when the deck declares no
// bracket or the pool is missing -- an absent count is not a count of zero.
func GameChangersOf(d *deck.Deck, cards map[string]*pool.CardRecord) GameChangers {
	names := []string{}
	for _, c := range d.Cards {
		names = append(names, c.Name)
	}
	names = append(names, d.Commander...)
	if d.Companion != nil {
		names = append(names, *d.Companion)
	}
	found := []string{}
	for _, n := range names {
		if rec := cards[n]; rec != nil && rec.GameChanger {
			found = append(found, n)
		}
	}
	sort.Strings(found)
	out := GameChangers{Cards: found, Count: len(found), Bracket: d.Bracket}
	bracketKey := "0"
	if d.Bracket != nil {
		bracketKey = strconv.Itoa(*d.Bracket)
	}
	allowed, known := reference.Deck().GameChangerLimits[bracketKey]
	if known {
		out.Allowed = allowed
	}
	switch {
	case len(cards) == 0 || d.Bracket == nil:
		out.Verdict = "unknown"
	case out.Allowed == nil || len(found) <= *out.Allowed:
		out.Verdict = "ok"
	default:
		out.Verdict = "over"
	}
	return out
}

// comb is math.comb: 0 for k > n or k < 0.
func comb(n, k int) *big.Int {
	if k < 0 || k > n {
		return big.NewInt(0)
	}
	return new(big.Int).Binomial(int64(n), int64(k))
}

func ratio(a, b *big.Int) float64 {
	if b.Sign() == 0 {
		return 0
	}
	f, _ := new(big.Rat).SetFrac(a, b).Float64()
	return f
}

// atLeastOne is `analyze._at_least_one`: P(at least one of `copies` among
// `seen` draws from `deckSize`), hypergeometric complement.
func atLeastOne(deckSize, copies, seen int) float64 {
	if copies <= 0 || deckSize <= 0 {
		return 0
	}
	if seen > deckSize {
		seen = deckSize
	}
	total := comb(deckSize, seen)
	if total.Sign() == 0 {
		return 0
	}
	return 1 - ratio(comb(deckSize-copies, seen), total)
}

// OpeningHand is `analyze.opening_hand`: draw odds for the opening seven and
// the seen-it-by-turn table, on the draw (`7 + t` cards by the end of turn t).
//
// `keepable` goes through `pyfloat.Fsum` and not a `+=` loop, matching the
// `math.fsum` Python spells it with. The loop was the obvious transcription of
// Python's old `sum(...)` and it was wrong in a way that only fires on one of
// the two interpreters: `sum()` over floats is compensated from CPython 3.12
// and left to right before it, so a Go accumulation loop reproduces **3.11**
// while the container runs 3.12. Three terms is enough to see it -- swept over
// every deck size from 8 to 250, the two arithmetics disagree in 5,098 shapes,
// 33 of them ordinary Commander decks, and the difference reaches the JSON.
func OpeningHand(d *deck.Deck) Opening {
	n := d.TotalCards()
	hand := 7
	if n < hand {
		hand = n
	}
	lands := d.LandCount()
	total := comb(n, hand)
	dist := []LandRow{}
	keep := []float64{}
	for k := 0; k <= hand; k++ {
		chance := 0.0
		if total.Sign() != 0 {
			num := new(big.Int).Mul(comb(lands, k), comb(n-lands, hand-k))
			chance = ratio(num, total)
		}
		dist = append(dist, LandRow{Lands: k, Chance: chance})
		if k >= 2 && k <= 4 {
			keep = append(keep, chance)
		}
	}
	keepable := pyfloat.Fsum(keep)
	counts, order := d.CategoryCounts()
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	categories := []CategoryOdds{}
	for _, cat := range order {
		m := counts[cat]
		categories = append(categories, CategoryOdds{Category: cat, Count: m,
			InOpeningHand: atLeastOne(n, m, hand), ByTurnFour: atLeastOne(n, m, hand+4)})
	}
	singleton := []Singleton{}
	for _, t := range []int{1, 4, 7, 10} {
		seen := hand + t
		if seen > n {
			seen = n
		}
		chance := 0.0
		if n > 0 {
			chance = float64(seen) / float64(n)
		}
		singleton = append(singleton, Singleton{Turn: t, CardsSeen: seen, Chance: chance})
	}
	return Opening{DeckSize: n, HandSize: hand,
		Lands:      Lands{Count: lands, Distribution: dist, Keepable: keepable},
		Categories: categories, Singleton: singleton}
}

// DeckStats is `analyze.deck_stats`: the whole deterministic report.
func DeckStats(d *deck.Deck, cards map[string]*pool.CardRecord) Stats {
	if cards == nil {
		cards = map[string]*pool.CardRecord{}
	}
	return Stats{
		Slug: d.Slug, Name: d.Name, Commander: append([]string{}, d.Commander...), Bracket: d.Bracket,
		TotalCards: d.TotalCards(), LandCount: d.LandCount(),
		Curve: CurveOf(d, cards), Categories: CategoryReport(d), GameChangers: GameChangersOf(d, cards),
		Opening: OpeningHand(d), Colors: PipRequirements(d, cards), Types: TypeBreakdown(d, cards),
	}
}
