package pool

import (
	"context"
	"fmt"
	"strings"
)

// The painting a card was first given.
//
// **The earliest printing, never the newest**, and this file is where that
// rule stopped being a token's rule and became the pool's. [Conn.GetCards]
// answers a name with its *newest* printing, which is how Teenage Mutant Ninja
// Turtles art arrived on the Grand Coliseum, Marvel art on Valor's Reach, and
// a Secret Lair about Bilbo's second breakfast on a Food token. Every one of
// those was a room that asked for "the picture of this card" and was handed
// whichever crossover shipped most recently.
//
// The earliest is the *original*: the picture somebody who has played with the
// card knows, painted for the set it was printed in. Recognisable beats recent,
// and deterministic beats both — a newest-printing lookup silently repaints
// itself every time the pool is refreshed, so a screenshot taken today does not
// match the room tomorrow and nothing anywhere reports a change.
//
// **A name is not an identity**, and that limit travels with this: a generic
// token name — Spirit, Soldier, Zombie — names a dozen different bodies, and
// `printings` carries no power or toughness to tell them apart. See
// [Conn.TokenArtFor], which is this method wearing the token's word, for the
// longer version of that argument and for why it is the right compromise
// anyway.

// Art is one card's painting, and the credit that has to travel with it.
//
// **Artist and Printing are not decoration on this struct.** Scryfall's terms
// require the artist and the source to be identifiable in the same interface
// the picture is shown in, and commandment 19 makes that a hard boundary rather
// than a nicety. A caller that draws [Art.Image] and does not print [Art.Artist]
// and [Art.Printing] beside it has broken the licence this whole project runs
// on — so they are in the same struct, arriving in the same read, and there is
// no path that hands out the picture without them.
type Art struct {
	Name string
	// Image is the whole card face, and there is deliberately no art crop
	// beside it.
	//
	// **`printings` has no `image_art_crop` column; only `oracle_cards` does.**
	// So a crop could only come from the oracle row, which is Scryfall's
	// *representative* printing — a different painting from the one this method
	// just chose and credited. That is not a missing feature, it is a trap with
	// a name: the schema comment above `printings.artist` records a deck page
	// that read a set name off one printing and a painter off another and
	// credited the wrong person in one sentence. A picture and its credit come
	// out of the same row here or they do not come out at all.
	Image  string
	Artist string
	// Set is the set's code, upper-cased, and Printing its name in words.
	// The credit line wants the words; the code is for anything that needs to
	// say *which* set without a sentence.
	Set      string
	Printing string
}

// ArtFor looks up many cards at once and answers with each one's earliest
// painted printing, keyed by lower-cased name.
//
// **Missing names are simply absent from the result**, which is the contract
// [Conn.GetCards] and [Conn.TokenArtFor] both keep and for the same reason: a
// picture is decoration, and a missing one must never cost anybody the surface
// it was going to sit on. Every caller here draws a plate with no painting.
//
// One query for the whole batch. A room asking for thirty paintings one at a
// time is thirty round trips for one screen.
func (c *Conn) ArtFor(ctx context.Context, names []string) (map[string]Art, error) {
	out := map[string]Art{}
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

	// One row per name: the earliest printing that actually has a picture and a
	// painter. `released_at` then `id` because a set can print several of a card
	// on one day (Throne of Eldraine shipped three Foods) and a tie broken by
	// nothing at all is a tie broken differently on every query — which would
	// make the picture on a leaderboard change without the record changing.
	//
	// **`promo` outranks the date, and it has to.** A set's promo printings
	// carry the *same release date* as the set itself, so on date alone the tie
	// was broken by an id — stable, and meaningless. Measured: Ygra, Eater of
	// All came back credited to "Bloomburrow Promos" and Sovereign Okinec Ahau
	// to "The Lost Caverns of Ixalan Promos", which are true sentences that
	// name a printing nobody associates with either card. Asking for the
	// non-promo first answers "Bloomburrow" and "The Lost Caverns of Ixalan",
	// and still falls back to a promo for a card that has only ever been one.
	//
	// Note this is *not* the Secret Lair rule: Scryfall files a Secret Lair as
	// `promo = false`, so those are kept off this board by the date and not by
	// this clause.
	rows, err := c.db.QueryContext(ctx,
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[])),
		      ranked AS (
		        SELECT name, image_normal, artist, set_code, set_name,
		               row_number() OVER (
		                 PARTITION BY lower(name)
		                 ORDER BY promo ASC, released_at ASC, id ASC) AS rank
		        FROM printings
		        WHERE lower(name) IN (SELECT w FROM wanted)
		          AND image_normal IS NOT NULL AND artist IS NOT NULL)
		 SELECT name, image_normal, artist, set_code, set_name
		 FROM ranked WHERE rank = 1`, lowered)
	if err != nil {
		return nil, fmt.Errorf("card_art: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, image, artist, set, setName any
		if err := rows.Scan(&name, &image, &artist, &set, &setName); err != nil {
			return nil, fmt.Errorf("card_art: %w", err)
		}
		art := Art{Name: AsString(name), Image: AsString(image),
			Artist: AsString(artist), Set: strings.ToUpper(AsString(set)),
			Printing: AsString(setName)}
		out[strings.ToLower(art.Name)] = art
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("card_art: %w", err)
	}
	return out, nil
}
