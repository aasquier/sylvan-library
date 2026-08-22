// Package suggest is `decks/suggest.py`: ranked replacement candidates for a
// card that has to leave a deck -- a measurement, not a recommendation. Of
// every card that is legal here, which most resemble the one being removed?
// A weighted sum (type 0.30, mana value 0.20, keywords 0.15, oracle text
// 0.35) and a popularity nudge of up to 0.10 from EDHREC rank, last and
// small on purpose: popular is not the same as correct. Every fact comes
// from the pool; nothing here decides anything.
package suggest

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// PrimaryTypes are the card types, most specific first: a "Legendary
// Artifact Creature" is a creature.
var PrimaryTypes = []string{"Creature", "Planeswalker", "Land", "Instant", "Sorcery", "Artifact", "Enchantment", "Battle"}

var permanentTypes = map[string]bool{"Creature": true, "Planeswalker": true, "Land": true,
	"Artifact": true, "Enchantment": true, "Battle": true}

// Stopwords are English function words plus the Magic templating that
// appears on almost every card and therefore distinguishes nothing.
var Stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
a an and as at be been but by can cant do does each end for from has have if in
into is it its may more must not of on onto or other others out over own put
same than that the their them then there these they this those to under until
up upon was were when whenever where which while with without you your
also another any are back both get gets much no only rather very
`) {
		Stopwords[w] = true
	}
}

var wordRe = regexp.MustCompile(`[a-z]+`)

// Candidate is one suggestion, with the arithmetic that produced it.
type Candidate struct {
	Record  *pool.CardRecord
	Score   float64
	Reasons []string
}

// Name is the candidate's.
func (c Candidate) Name() string { return c.Record.Name }

// PrimaryType is `suggest.primary_type`: the card's main type, from the
// front face.
func PrimaryType(typeLine string) string {
	front, _, _ := strings.Cut(typeLine, " // ")
	for _, t := range PrimaryTypes {
		if strings.Contains(front, t) {
			return t
		}
	}
	return ""
}

// Tokens is `_tokens`: significant words, lowercased, stopwords and short
// words removed.
func Tokens(texts ...string) map[string]bool {
	out := map[string]bool{}
	for _, text := range texts {
		for _, w := range wordRe.FindAllString(strings.ToLower(text), -1) {
			if len(w) > 2 && !Stopwords[w] {
				out[w] = true
			}
		}
	}
	return out
}

func typeScore(target, candidate *pool.CardRecord) float64 {
	left, right := PrimaryType(target.TypeLine), PrimaryType(candidate.TypeLine)
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 1
	}
	if permanentTypes[left] && permanentTypes[right] {
		return 0.4
	}
	return 0
}

func curveScore(target, candidate *pool.CardRecord) float64 {
	return math.Max(0, 1-math.Abs(target.CMC-candidate.CMC)/3)
}

func lowerSet(list []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range list {
		out[strings.ToLower(s)] = true
	}
	return out
}

func keywordScore(target, candidate *pool.CardRecord) float64 {
	left, right := lowerSet(target.Keywords), lowerSet(candidate.Keywords)
	if len(left) == 0 {
		return 0
	}
	both, either := 0, len(left)
	for k := range right {
		if left[k] {
			both++
		} else {
			either++
		}
	}
	return float64(both) / float64(either)
}

func textScore(targetTokens map[string]bool, candidate *pool.CardRecord) float64 {
	if len(targetTokens) == 0 {
		return 0
	}
	cand := Tokens(candidate.OracleText)
	shared := 0
	for w := range targetTokens {
		if cand[w] {
			shared++
		}
	}
	return float64(shared) / float64(len(targetTokens))
}

// popularity is `_popularity`: EDHREC rank, log-scaled into 0..1.
func popularity(rec *pool.CardRecord) float64 {
	if rec.EdhrecRank == nil || *rec.EdhrecRank < 1 {
		return 0
	}
	return math.Max(0, 1-math.Log10(float64(*rec.EdhrecRank))/5)
}

// round4 is Python's `round(x, 4)`.
func round4(x float64) float64 { return math.RoundToEven(x*10000) / 10000 }

// thousands is Python's `f"{n:,}"`.
func thousands(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// Score is `suggest.score`: one candidate against the card being replaced.
// `why` is the deck's own rationale for the slot, folded into the text
// comparison because it says what the card was *for*.
func Score(target, candidate *pool.CardRecord, why string) Candidate {
	targetTokens := Tokens(target.OracleText, why)
	similarity := 0.30*typeScore(target, candidate) + 0.20*curveScore(target, candidate) +
		0.15*keywordScore(target, candidate) + 0.35*textScore(targetTokens, candidate)
	total := similarity + 0.10*popularity(candidate)

	reasons := []string{}
	if typeScore(target, candidate) == 1 {
		reasons = append(reasons, fmt.Sprintf("same card type (%s)", PrimaryType(candidate.TypeLine)))
	}
	if candidate.CMC == target.CMC {
		reasons = append(reasons, fmt.Sprintf("same mana value (%d)", int(target.CMC)))
	} else if math.Abs(candidate.CMC-target.CMC) <= 1 {
		reasons = append(reasons, fmt.Sprintf("mana value %d vs %d", int(candidate.CMC), int(target.CMC)))
	}
	left, right := lowerSet(target.Keywords), lowerSet(candidate.Keywords)
	shared := []string{}
	for k := range left {
		if right[k] {
			shared = append(shared, k)
		}
	}
	if len(shared) > 0 {
		sort.Strings(shared)
		reasons = append(reasons, "shares "+strings.Join(shared, ", "))
	}
	cand := Tokens(candidate.OracleText)
	words := []string{}
	for w := range targetTokens {
		if cand[w] {
			words = append(words, w)
		}
	}
	if len(words) > 0 {
		sort.Strings(words)
		if len(words) > 6 {
			words = words[:6]
		}
		reasons = append(reasons, "text: "+strings.Join(words, ", "))
	}
	if candidate.EdhrecRank != nil && *candidate.EdhrecRank != 0 {
		reasons = append(reasons, "EDHREC rank "+thousands(*candidate.EdhrecRank))
	}
	return Candidate{Record: candidate, Score: round4(total), Reasons: reasons}
}

// Rank is `suggest.rank`: score every candidate and return the best `limit`,
// ties broken by name so the output is stable run to run.
func Rank(target *pool.CardRecord, candidates []*pool.CardRecord, why string, limit int, exclude map[string]bool) []Candidate {
	blocked := map[string]bool{strings.ToLower(target.Name): true}
	for n := range exclude {
		blocked[strings.ToLower(n)] = true
	}
	scored := []Candidate{}
	for _, rec := range candidates {
		if blocked[strings.ToLower(rec.Name)] {
			continue
		}
		scored = append(scored, Score(target, rec, why))
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Name() < scored[j].Name()
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}

// CandidatePool is `suggest.candidate_pool`: everything legal that could
// plausibly fill the slot -- a prefilter to a few hundred, inside the
// identity, roughly the right cost and sort of card.
func CandidatePool(ctx context.Context, c *pool.Conn, target *pool.CardRecord, identity map[string]bool, poolSize int) ([]*pool.CardRecord, error) {
	colors := []string{}
	for _, col := range []string{"W", "U", "B", "R", "G"} {
		if identity[col] {
			colors = append(colors, col)
		}
	}
	listed := "''"
	if len(colors) > 0 {
		quoted := make([]string, len(colors))
		for i, col := range colors {
			quoted[i] = "'" + col + "'"
		}
		listed = strings.Join(quoted, ", ")
	}
	where := []string{
		"json_extract_string(legalities, 'commander') = 'legal'",
		fmt.Sprintf("len(list_filter(color_identity, x -> x NOT IN (%s))) = 0", listed),
		"cmc BETWEEN ? AND ?",
	}
	params := []any{math.Max(0, target.CMC-2), target.CMC + 2}
	if kind := PrimaryType(target.TypeLine); kind != "" {
		where = append(where, "type_line LIKE ?")
		params = append(params, "%"+kind+"%")
	}
	return c.Search(ctx, strings.Join(where, " AND "), params, poolSize, "edhrec_rank NULLS LAST", 0)
}

// ReplacementsFor is `suggest.replacements_for`: suggestions for one card in
// one deck; an empty list when the pool does not know the card.
func ReplacementsFor(ctx context.Context, c *pool.Conn, d *deck.Deck, cards map[string]*pool.CardRecord, name string, limit int) ([]Candidate, error) {
	target := cards[name]
	if target == nil {
		return []Candidate{}, nil
	}
	identity := map[string]bool{}
	for _, commander := range d.Commander {
		if rec := cards[commander]; rec != nil {
			for _, col := range rec.ColorIdentity {
				identity[col] = true
			}
		}
	}
	why := ""
	for _, entry := range d.Cards {
		if entry.Name == name {
			why = entry.Why
			break
		}
	}
	already := map[string]bool{}
	for _, entry := range d.Cards {
		already[entry.Name] = true
	}
	for _, entry := range d.SwapBoard {
		already[entry.Name] = true
	}
	for _, commander := range d.Commander {
		already[commander] = true
	}
	candidates, err := CandidatePool(ctx, c, target, identity, 400)
	if err != nil {
		return nil, err
	}
	return Rank(target, candidates, why, limit, already), nil
}
