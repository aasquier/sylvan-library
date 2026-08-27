package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// What a deck makes.
//
// `tokens.go` argues the painting; this argues the question. Its closing
// paragraph names the limit this file closes: a token's *name* is not its
// identity — Spirit is a dozen different bodies — so answering "what does this
// deck make" by looking names up is answering with a guess. Scryfall's
// `all_parts` names a particular **printing** for every token a card creates,
// and that is an identity. Elspeth's Soldier is that Soldier.
//
// The document is ingested whole into `oracle_cards.all_parts` (see
// `schema.sql`) and read here rather than flattened into a table of its own:
// it is a handful of entries on a small minority of cards, the deck asking is
// a hundred names, and a table would buy an index nobody would use.
//
// **Only `component == "token"`.** Scryfall files an emblem as a
// `combo_piece`, and a meld's halves and result as `meld_part` and
// `meld_result`; a card also lists itself. A section headed Tokens that
// listed an emblem would be teaching a beginner something untrue
// (commandment 3), so the filter is Scryfall's own word and nothing cleverer.

// TokenMade is one token a deck can put onto the battlefield, and the cards in
// that deck that make it.
type TokenMade struct {
	// Name and TypeLine are Scryfall's, off the related-card entry: "Food",
	// "Token Artifact — Food". The type line is the token's whole printed
	// classification, which is the honest way to tell one Spirit from another
	// in words; the picture tells it better.
	Name     string
	TypeLine string
	// Art is the painting, or nil when this pool has no printing to draw from
	// — a plate with no picture on it, exactly the contract [Conn.TokenArtFor]
	// keeps and for the same reason.
	Art *TokenArt
	// MadeBy is the deck's own cards, spelled as the deck spells them, sorted.
	MadeBy []string
}

// TokenSheet is the answer to "what does this deck make".
type TokenSheet struct {
	// Read is false when this pool cannot answer at all — it predates the
	// column, or carries it empty.
	//
	// **Not the same as an empty Tokens**, which means the cards were read and
	// none of them makes anything. The two must never be shown as one
	// sentence: one says "this deck makes nothing", the other says "nobody has
	// looked yet", and a reader who is told the first when the second is true
	// has been lied to.
	Read   bool
	Tokens []TokenMade
}

const tokenComponent = "token"

// TokensMade answers, for a run of card names, which tokens those cards make.
//
// Pass the commanders as well as the 99: a commander makes tokens like
// anything else, and Gyome, Master Chef is the obvious case.
//
// **The pool cannot migrate itself** (see [Conn.Columns]), and merging is
// deploying (ADR 23), so there is a live window in which this code runs
// against a library built before the column existed. Two guards, because
// there are two ways to be missing: the column may be absent, and it may have
// been added by the `ALTER` ladder that no load has since filled. Either way
// this reports Read false and no tokens, and the page says so — a wrong
// answer being the one outcome worse than no answer.
func (c *Conn) TokensMade(ctx context.Context, names []string) (TokenSheet, error) {
	empty := TokenSheet{Tokens: []TokenMade{}}
	if len(names) == 0 {
		return TokenSheet{Read: true, Tokens: []TokenMade{}}, nil
	}
	have, err := c.Columns(ctx, "oracle_cards")
	if err != nil {
		return empty, err
	}
	if !have["all_parts"] {
		return empty, nil
	}
	filled, err := probe(ctx, c,
		"SELECT 1 FROM oracle_cards WHERE all_parts IS NOT NULL LIMIT 1")
	if err != nil {
		return empty, err
	}
	if !filled {
		return empty, nil
	}

	// The deck's spelling of each name, kept so the answer credits a card the
	// way the file writes it rather than the way the pool does.
	asked := map[string]string{}
	lowered := make([]string, 0, len(names))
	for _, name := range names {
		low := strings.ToLower(strings.TrimSpace(name))
		if low == "" {
			continue
		}
		if _, seen := asked[low]; seen {
			continue
		}
		asked[low] = name
		lowered = append(lowered, low)
	}
	if len(lowered) == 0 {
		return TokenSheet{Read: true, Tokens: []TokenMade{}}, nil
	}

	parts, makers, err := c.tokenParts(ctx, lowered, asked)
	if err != nil {
		return empty, err
	}
	if len(parts) == 0 {
		return TokenSheet{Read: true, Tokens: []TokenMade{}}, nil
	}

	ids := make([]string, 0, len(parts))
	for id := range parts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	identities, err := c.tokenIdentities(ctx, ids)
	if err != nil {
		return empty, err
	}

	groups, order := groupTokens(ids, parts, makers, identities)
	oracleIDs, byName := []string{}, []string{}
	for _, key := range order {
		if g := groups[key]; g.oracle != "" {
			oracleIDs = append(oracleIDs, g.oracle)
		} else {
			byName = append(byName, g.name)
		}
	}
	art, err := c.tokenArtByOracle(ctx, oracleIDs)
	if err != nil {
		return empty, err
	}
	namedArt, err := c.TokenArtFor(ctx, byName)
	if err != nil {
		return empty, err
	}

	out := make([]TokenMade, 0, len(order))
	for _, key := range order {
		g := groups[key]
		made := TokenMade{Name: g.name, TypeLine: g.typeLine,
			MadeBy: make([]string, 0, len(g.makers))}
		for maker := range g.makers {
			made.MadeBy = append(made.MadeBy, maker)
		}
		sort.Strings(made.MadeBy)
		if found, ok := art[g.oracle]; ok && g.oracle != "" {
			made.Art = &found
		} else if found, ok := namedArt[strings.ToLower(g.name)]; ok {
			made.Art = &found
		}
		out = append(out, made)
	}
	// The token this deck makes most, first: that is the one somebody has to
	// find a pile of before they sit down. Ties by name and then by type line,
	// so a map walk never decides the order.
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].MadeBy) != len(out[j].MadeBy) {
			return len(out[i].MadeBy) > len(out[j].MadeBy)
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].TypeLine < out[j].TypeLine
	})
	return TokenSheet{Read: true, Tokens: out}, nil
}

// tokenPart is one related printing, kept by the id `all_parts` named — one
// particular Soldier rather than the word.
type tokenPart struct{ name, typeLine string }

// tokenParts reads the deck's cards and returns the token printings they name,
// with the deck cards that named each.
func (c *Conn) tokenParts(ctx context.Context, lowered []string,
	asked map[string]string) (map[string]tokenPart, map[string]map[string]bool, error) {

	// The same WHERE as [Conn.GetCards]: a double-faced card answers to its
	// combined name or to either face, because a deck file may spell it either
	// way and both are the same card.
	rows, err := c.db.QueryContext(ctx,
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[]))
		 SELECT name, all_parts FROM oracle_cards
		 WHERE all_parts IS NOT NULL
		   AND (lower(name) IN (SELECT w FROM wanted)
		     OR (contains(name, ' // ') AND (
		            lower(split_part(name, ' // ', 1)) IN (SELECT w FROM wanted)
		         OR lower(split_part(name, ' // ', 2)) IN (SELECT w FROM wanted))))`,
		lowered)
	if err != nil {
		return nil, nil, fmt.Errorf("tokens_made: %w", err)
	}
	defer rows.Close()
	parts := map[string]tokenPart{}
	makers := map[string]map[string]bool{}
	for rows.Next() {
		var name, doc any
		if err := rows.Scan(&name, &doc); err != nil {
			return nil, nil, fmt.Errorf("tokens_made: %w", err)
		}
		maker := deckSpelling(asked, AsString(name))
		if maker == "" {
			continue
		}
		for _, related := range relatedParts(doc) {
			if related.Component != tokenComponent || related.ID == "" {
				continue
			}
			parts[related.ID] = tokenPart{related.Name, related.TypeLine}
			if makers[related.ID] == nil {
				makers[related.ID] = map[string]bool{}
			}
			makers[related.ID][maker] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("tokens_made: %w", err)
	}
	return parts, makers, nil
}

type tokenGroup struct {
	name, typeLine, oracle string
	makers                 map[string]bool
}

// groupTokens folds the named printings into one entry per token.
//
// The key is the token's **oracle id** — which Spirit this is — and falls back
// to its name when the printing `all_parts` named is not in this pool.
// Scryfall prints online-only tokens this library filters out, and an old pool
// may simply not have that set yet; grouping by name there is the same
// compromise [Conn.TokenArtFor] makes, for the same reason. `order` is the
// insertion order over sorted ids, so the walk that follows is deterministic
// before the sort ever runs.
func groupTokens(ids []string, parts map[string]tokenPart,
	makers map[string]map[string]bool,
	identities map[string]string) (map[string]*tokenGroup, []string) {

	groups := map[string]*tokenGroup{}
	order := []string{}
	for _, id := range ids {
		p := parts[id]
		oracle := identities[id]
		key := "oracle:" + oracle
		if oracle == "" {
			key = "name:" + strings.ToLower(p.name)
		}
		g := groups[key]
		if g == nil {
			g = &tokenGroup{name: p.name, typeLine: p.typeLine, oracle: oracle,
				makers: map[string]bool{}}
			groups[key] = g
			order = append(order, key)
		}
		for maker := range makers[id] {
			g.makers[maker] = true
		}
	}
	return groups, order
}

// relatedPart is one entry of Scryfall's `all_parts`, narrowed to the four
// fields read here. The document is stored whole, so a later reader that wants
// `uri` has it without another rebuild.
type relatedPart struct {
	ID        string `json:"id"`
	Component string `json:"component"`
	Name      string `json:"name"`
	TypeLine  string `json:"type_line"`
}

// relatedParts reads the JSON column however the driver hands it over — the
// same three shapes `legalities` copes with, for the same reason: a pool that
// stored it as text still reads, and a JSON-typed column comes back **already
// decoded**, as `[]any` of `map[string]any`. That last one is not a detail: it
// is what this column actually does on a pool the loaders wrote, and reading
// only the text shapes silently answered every deck with "makes nothing".
func relatedParts(v any) []relatedPart {
	var raw []byte
	switch t := v.(type) {
	case string:
		raw = []byte(t)
	case []byte:
		raw = t
	case []any:
		encoded, err := json.Marshal(t)
		if err != nil {
			return nil
		}
		raw = encoded
	default:
		return nil
	}
	var out []relatedPart
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// deckSpelling maps a pool row's name back to the name the caller asked with,
// by the full name or by either face.
func deckSpelling(asked map[string]string, poolName string) string {
	low := strings.ToLower(poolName)
	if name, ok := asked[low]; ok {
		return name
	}
	for _, face := range strings.Split(low, " // ") {
		if name, ok := asked[face]; ok {
			return name
		}
	}
	return ""
}

// tokenIdentities is printing id -> the token's oracle id: which Spirit this
// is, rather than the word "Spirit". An id this pool does not have is simply
// absent, and its caller falls back to the name.
func (c *Conn) tokenIdentities(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := c.db.QueryContext(ctx,
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[]))
		 SELECT id, oracle_id FROM printings WHERE id IN (SELECT w FROM wanted)`,
		ids)
	if err != nil {
		return nil, fmt.Errorf("token_identities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, oracle any
		if err := rows.Scan(&id, &oracle); err != nil {
			return nil, fmt.Errorf("token_identities: %w", err)
		}
		out[AsString(id)] = AsString(oracle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("token_identities: %w", err)
	}
	return out, nil
}

// tokenArtByOracle is [Conn.TokenArtFor]'s ruling applied to an identity
// rather than a name: the **earliest** printing of this exact token, so a Food
// is Throne of Eldraine's pie however new the Food the card that makes it
// happens to point at. Recognisable beats recent, and deterministic beats
// both — the argument is `tokens.go`'s and it is inherited here whole.
func (c *Conn) tokenArtByOracle(ctx context.Context, oracleIDs []string) (map[string]TokenArt, error) {
	out := map[string]TokenArt{}
	if len(oracleIDs) == 0 {
		return out, nil
	}
	rows, err := c.db.QueryContext(ctx,
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[])),
		      ranked AS (
		        SELECT oracle_id, name, image_normal, artist, set_code, set_name,
		               row_number() OVER (
		                 PARTITION BY oracle_id
		                 ORDER BY released_at ASC, id ASC) AS rank
		        FROM printings
		        WHERE oracle_id IN (SELECT w FROM wanted)
		          AND image_normal IS NOT NULL AND artist IS NOT NULL)
		 SELECT oracle_id, name, image_normal, artist, set_code, set_name
		 FROM ranked WHERE rank = 1`, oracleIDs)
	if err != nil {
		return nil, fmt.Errorf("token_art_by_oracle: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var oracle, name, image, artist, set, setName any
		if err := rows.Scan(&oracle, &name, &image, &artist, &set, &setName); err != nil {
			return nil, fmt.Errorf("token_art_by_oracle: %w", err)
		}
		out[AsString(oracle)] = TokenArt{Name: AsString(name),
			Image: AsString(image), Artist: AsString(artist),
			Set: strings.ToUpper(AsString(set)), Printing: AsString(setName)}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("token_art_by_oracle: %w", err)
	}
	return out, nil
}
