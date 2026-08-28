package cards

import (
	"context"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Suggest is the typeahead behind "add a card": a short, ranked shortlist of
// real card names for a few letters somebody is still typing.
//
// # Why this is not `SearchCards`
//
// `deckread.SearchCards` asks a research question -- identity, type line,
// mana value, price, rules text -- and answers it with a substring test on
// the name. That test has one failure mode and it is the one that matters
// here: **a misspelling returns nothing at all**, and nothing is
// indistinguishable from "this card does not exist". Somebody adding their
// first card types `Sol Rng`, gets an empty list, and has no way to tell
// which of the two happened. That is commandment 2's failure exactly.
//
// # Why this is not a second matcher either
//
// The scoring is [ByTitle]'s -- literally, via [titleScore]: one jaro-winkler
// expression, scored against the whole name and the front face, so a name
// typed by hand, a name read off a photograph (ADR 34) and a name pasted in a
// decklist (`deckimport.Respell`) are all judged by the same measure. What
// differs is what is *done* with the number, and that difference is argued at
// [NearFloor].
//
// # The four tiers
//
// A person typing into a box means different things by different queries, and
// one score cannot tell them apart. So the shortlist is tiered:
//
//  1. `exact` -- the name, or its front face, is what was typed. Never a
//     guess about intent, so nothing outranks it.
//  2. `holds` -- the name contains what was typed, anywhere, ordered by how
//     much the game plays it. `sol r` -> Sol Ring; `bolt` -> Lightning Bolt.
//  3. `words` -- the name contains every word of it, in any order.
//     `titan primeval` -> Primeval Titan, which scores 0.706 as one string
//     and would never survive the floor below; `sakura tribe elder` ->
//     Sakura-Tribe Elder, a dropped hyphen, which is the commonest miss of
//     all and never reaches the guess at all.
//  4. `near`  -- nothing matched literally, and a card is close enough to be
//     worth *offering*. `Sol Rng` -> Sol Ring; `craterhof behemoth` ->
//     Craterhoof Behemoth.
//
// Only the fourth is a guess, and it is the only one that carries a
// threshold. The first three are facts about the string.
//
// **Popularity beats position inside `holds`, and that was measured rather
// than assumed.** The first cut ranked a name that *begins* with the query
// above one that merely contains it, which reads right and is wrong: against
// the whole pool, `bolt` filled the list with Bolt Bend, Boltwave, Boltbender
// and Bolt Hound, and Lightning Bolt -- rank 160, and the only card anybody
// means by that word -- was nowhere. Somebody typing a fragment means the
// famous card. `starts_with` survives as a tiebreak below the rank, where it
// settles the cards the game plays equally.
//
// The cost of that is the one thing `edhrec_rank` cannot rank: a basic land
// has no rank at all, so `Forest` sinks under every card whose name holds the
// word. That is what tier one is for -- a name typed in full is an answer, not
// a fragment, and `Forest` typed out reaches it before popularity is ever
// consulted.
func Suggest(ctx context.Context, c *pool.Conn, text string, limit int) ([]Suggestion, error) {
	out := []Suggestion{}
	query := strings.ToLower(strings.TrimSpace(text))
	if runes := []rune(query); len(runes) > MaxTitle {
		query = string(runes[:MaxTitle])
	}
	if query == "" {
		return out, nil
	}
	if limit < 1 {
		limit = 1
	}
	if limit > MaxSuggestions {
		limit = MaxSuggestions
	}

	// The near tier is switched off by making its floor unreachable rather
	// than by branching the SQL: one query shape, whatever was typed, so the
	// thing that runs in production is the thing the tests run.
	floor := 2.0
	if len([]rune(query)) >= NearLength {
		floor = NearFloor
	}

	// `contains` per word, ANDed -- the `words` tier. A single-word query
	// makes this identical to `holds`, and the CASE reaches `holds` first, so
	// the tier collapses on its own with nothing to special-case.
	//
	// Capped, because the number of words is the one part of this query a
	// caller controls: 200 characters of single letters is a hundred
	// `contains` calls in one CASE, and that is a search box turned into a
	// lever. Past the cap the extra words are dropped rather than the query
	// refused -- a paste of something that is not a card name should answer
	// "nothing like that", which is what the remaining words will say.
	words := strings.Fields(query)
	if len(words) > MaxQueryWords {
		words = words[:MaxQueryWords]
	}
	holdsWord := make([]string, 0, len(words))
	// **In the order the placeholders appear in the SELECT list**, which is
	// the order they are read: the score's two, the exact tier's two, the
	// holds tier's one, then one per word, then the `opens` tiebreak -- which
	// is written after the CASE and therefore binds after the words inside
	// it. Putting it before them cost a debugging round: the words tier
	// simply stopped matching, because every word was being compared against
	// the whole query.
	params := []any{query, query, query, query, query}
	for _, w := range words {
		holdsWord = append(holdsWord, "contains(lower(name), ?)")
		params = append(params, w)
	}
	wordsTier := "FALSE"
	if len(holdsWord) > 0 {
		wordsTier = strings.Join(holdsWord, " AND ")
	}
	params = append(params, query, floor, limit)

	rows, err := c.DB().QueryContext(ctx,
		`SELECT name, score, tier FROM (
		   SELECT name, edhrec_rank, `+titleScore+` AS score,
		          CASE WHEN lower(name) = ?
		                 OR lower(split_part(name, ' // ', 1)) = ? THEN 0
		               WHEN contains(lower(name), ?) THEN 1
		               WHEN `+wordsTier+` THEN 2
		               ELSE 3 END AS tier,
		          starts_with(lower(name), ?) AS opens
		   FROM oracle_cards WHERE `+isCard+`
		 )
		 WHERE tier < 3 OR score >= ?
		 -- Likeness inside the guess, popularity inside the literal tiers,
		 -- and position only where the game plays two cards the same amount.
		 -- `+"`name`"+` last, so a tie is broken the same way twice: this is a
		 -- list somebody arrows through, and an order that moved between two
		 -- identical requests would move the selection under their finger.
		 ORDER BY tier,
		          CASE WHEN tier = 3 THEN -score ELSE 0 END,
		          edhrec_rank NULLS LAST,
		          opens DESC,
		          name
		 LIMIT ?`, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Suggestion
		var tier int
		if err := rows.Scan(&s.Name, &s.Score, &tier); err != nil {
			return nil, err
		}
		s.Via = tierNames[tier]
		out = append(out, s)
	}
	return out, rows.Err()
}

// Suggestion is one offered name, how alike it is to what was typed, and
// which tier put it in the list. `Via` travels because the interface says
// different things about a name it *found* and a name it is *guessing at*,
// and only the server knows which happened.
type Suggestion struct {
	Name  string
	Score float64
	Via   string
}

// tierNames indexes by the CASE above, so the two cannot drift.
var tierNames = [4]string{"exact", "holds", "words", "near"}

// MaxSuggestions bounds one request's work. A shortlist somebody reads on a
// phone is about eight long; past twenty it is a search results page wearing
// a dropdown's clothes.
const MaxSuggestions = 20

// MaxQueryWords bounds the `words` tier. Comfortably past the longest name a
// Commander deck can hold and far short of a pasted decklist.
const MaxQueryWords = 12

// NearFloor is where a near-miss is worth OFFERING, and it is deliberately
// far below `deckimport.nearFloor` (0.95), which is where a near-miss is
// worth *making*. Two different questions, and conflating them is what would
// make this feel broken.
//
// **The importer decides alone.** It substitutes a card into a deck file with
// nobody looking at the moment it happens, so it needs both a high floor and
// a clear lead over the runner-up: 0.95 and 0.04. Here a person is reading
// the list and choosing from it, so:
//
//   - **The lead requirement is dropped entirely.** It is a guard against
//     ambiguity, and ambiguity is what a list is *for*. Two cards at 0.96 are
//     a refusal for the importer and a choice of two for a human.
//   - **The floor comes down, because its job changed.** It no longer
//     separates "certainly this card" from "possibly this card"; it separates
//     "worth showing" from "noise".
//
// **0.86 is measured, and measured against the whole 35,393-card pool rather
// than a fixture.** Real whole-name typos land in a tight band well above it
// -- `smothring tithe` 0.988, `lightening bolt` 0.987, `arcane singet` 0.985,
// `avenger of zendicar` 0.979, `Sol Rng` 0.975, `birds of paridise` 0.964 --
// and the ones that matter are the low ones, because **they are below the
// importer's 0.95 and it would show a person nothing at all**:
// `craterhof behemoth` 0.945, `terastadon` 0.938, and `swrds to plowshares`
// 0.9165, each of which comes back as the *only* card offered.
//
// Underneath, keyboard mash finds nothing: `qwertyuiop` and `asdkjhasd`
// return an empty list against all 35,393 names, and the highest score either
// reaches is 0.71. So 0.86 sits well below the lowest real typo and well
// above the highest measured non-word, and the gap it leaves in the middle is
// on purpose: `gorclaw` scores 0.823 against Goreclaw, Terror of Qal Sisma
// and is not offered. A misspelled *fragment* is genuinely ambiguous, and in
// a room whose job is teaching somebody their first deck, a confidently wrong
// suggestion costs more than an empty list with a sentence under it.
const NearFloor = 0.86

// NearLength is the shortest query the near tier reads. Below it a query is a
// fragment, not a misspelling, and the score says so: two and three letters
// score 0.80-0.89 against half the pool (`so` -> Solitude 0.800, `cul` ->
// Cultivate 0.844) purely because jaro-winkler weights a common opening.
// Those are all `holds` hits anyway, so switching the guess off below four
// letters loses nothing and stops the list filling with alphabetical
// neighbours after two keystrokes.
const NearLength = 4
