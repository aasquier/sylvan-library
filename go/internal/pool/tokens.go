package pool

import (
	"context"
	"fmt"
	"strings"
)

// Token art.
//
// **Tokens are in the pool and always have been**, which is the opposite of
// what a session handoff once asserted. `SkipOracleLayout` drops the `token`
// and `double_faced_token` layouts from `oracle_cards` — correctly, because a
// token is not a card anybody puts in a deck — but `printings` filters on
// `digital` alone, so every token printing Scryfall ships is sitting there
// with its artist named. Measured: 63 Food, 98 Treasure, 103 Spirit.
//
// So [Conn.GetCards] cannot answer a token and this can, and the two are
// separate methods rather than one clever one because they read different
// tables and mean different things.
//
// **The earliest printing, never the newest**, and that is the whole design of
// this file. `GetCards` answers a name with its *newest* printing, which is
// how Teenage Mutant Ninja Turtles art arrived on the Grand Coliseum and Marvel
// art on Valor's Reach — crossover paintings on a room that is an homage to
// Magic's own arenas. For a token the newest printing is a lottery among
// ninety-eight of them; the earliest is the *original*, which is the picture
// anybody who has played with a Food knows: Throne of Eldraine's pie for Food,
// Ixalan's for Treasure, Shadows over Innistrad's for Clue. Recognisable beats
// recent, and deterministic beats both.
//
// **A name is not an identity, which is why there are two ways in here.** A
// generic token name — Spirit, Soldier, Zombie — names a dozen different
// bodies, and `printings` carries no power or toughness to tell them apart
// (those live in `oracle_cards`, which is exactly where tokens are not). So
// [Conn.TokenArtFor], which has only a name to go on, may draw a 1/1 white
// flying Spirit with some other Spirit's painting. It is the right method
// anyway for a board mid-match, where Forge has said a word and a plate has to
// be painted now.
//
// [Conn.TokensMade] is the other way, and the answer to that limit: it asks
// the *cards* what they make. Scryfall's `all_parts` names a particular
// printing for every token a card creates, so Elspeth's Soldier is that
// Soldier and no other. This is the chosen printing per token that the earlier
// argument here promised, and the deck page's token section is what it was
// promised for.

// TokenArt is one token's painting.
type TokenArt struct {
	Name string
	// Image is the whole card face. `printings` carries no `art_crop`, and for
	// a battlefield the whole face is the better picture anyway — it is what a
	// token looks like lying on a table.
	Image  string
	Artist string
	// Set and Printing name where the painting came from, because somebody
	// painted it (rule 9, and every other surface here credits them).
	Set      string
	Printing string
}

// ForgeTokenSuffix is what Forge adds to a token's name.
//
// Forge says "Food Token" where Scryfall says "Food". One suffix, stripped in
// one place, rather than a `strings.TrimSuffix` sprinkled through whatever
// happens to need it next.
const ForgeTokenSuffix = " Token"

// TokenName is a Forge token name as Scryfall spells it.
func TokenName(forge string) string {
	return strings.TrimSuffix(forge, ForgeTokenSuffix)
}

// TokenArtFor looks up many tokens at once, by the name Scryfall uses.
//
// Missing names are simply absent from the result — a board draws a plate with
// no painting on it rather than refusing to draw, which is the same contract
// [Conn.GetCards] keeps and the same reason: a picture is decoration and a
// missing one must never cost anybody a match they are watching.
func (c *Conn) TokenArtFor(ctx context.Context, names []string) (map[string]TokenArt, error) {
	out := map[string]TokenArt{}
	if len(names) == 0 {
		return out, nil
	}
	lowered := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		low := strings.ToLower(strings.TrimSpace(name))
		if low == "" || seen[low] {
			continue
		}
		seen[low] = true
		lowered = append(lowered, low)
	}
	if len(lowered) == 0 {
		return out, nil
	}

	// One row per name: the earliest printing that actually has a picture and
	// a painter. `released_at` then `id` because a set can print several of a
	// token on one day (Throne of Eldraine shipped three Foods) and a tie
	// broken by nothing at all is a tie broken differently on every query.
	rows, err := c.db.QueryContext(ctx,
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[])),
		      ranked AS (
		        SELECT name, image_normal, artist, set_code, set_name,
		               row_number() OVER (
		                 PARTITION BY lower(name)
		                 ORDER BY released_at ASC, id ASC) AS rank
		        FROM printings
		        WHERE lower(name) IN (SELECT w FROM wanted)
		          AND image_normal IS NOT NULL AND artist IS NOT NULL)
		 SELECT name, image_normal, artist, set_code, set_name
		 FROM ranked WHERE rank = 1`, lowered)
	if err != nil {
		return nil, fmt.Errorf("token_art: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, image, artist, set, setName any
		if err := rows.Scan(&name, &image, &artist, &set, &setName); err != nil {
			return nil, fmt.Errorf("token_art: %w", err)
		}
		art := TokenArt{Name: AsString(name), Image: AsString(image),
			Artist: AsString(artist), Set: strings.ToUpper(AsString(set)),
			Printing: AsString(setName)}
		out[strings.ToLower(art.Name)] = art
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("token_art: %w", err)
	}
	return out, nil
}
