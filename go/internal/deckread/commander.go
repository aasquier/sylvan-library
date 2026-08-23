package deckread

import (
	"context"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// CommanderDossier is `service.commander_dossier`: everything interesting the
// pool knows about a deck's commander, every part of it a query rather than a
// recollection.
//
// It lived inside the `.../commander` handler until the Claude dossier needed
// the same facts as its brief -- the same split `DeckPayload` made one lane
// earlier, for the same reason: a route payload and the facts a mode is handed
// are the same bytes by construction only when one function builds both. ADR
// 19 leans on exactly that -- the counted strip on the deck page and the prose
// underneath it come from one query, so they cannot disagree.
//
// Three kinds of fact, each saying something the card's own text does not:
// the subtypes and how rare they are (a Troll Warlock is a more interesting
// thing to be when eight legendary Trolls exist and eighty-one legendary
// Warlocks do), the other cards carrying the character's name (matched on
// the part before the comma, which is how Magic names a legend, and offered
// as related rather than asserted as the same character), and how often and
// how early it was printed.
//
// A nil conn, a deck with no commander, or a commander the pool cannot find
// all answer the empty shape -- `card: null` with empty lists -- rather than
// failing, because this is a decorative panel and a fresh clone should still
// show its decks. Callers that need a commander (the Claude dossier) read
// `card` and refuse on their own terms.
func CommanderDossier(ctx context.Context, c *pool.Conn, d *deck.Deck) (wire.OrderedMap, error) {
	empty := wire.OrderedMap{{Key: "slug", Value: d.Slug}, {Key: "card", Value: nil},
		{Key: "subtypes", Value: []any{}}, {Key: "other_cards", Value: []any{}},
		{Key: "printings", Value: nil}}
	if len(d.Commander) == 0 || c == nil {
		return empty, nil
	}
	name := d.Commander[0]
	found, err := c.GetCards(ctx, []string{name})
	if err != nil {
		return nil, err
	}
	rec := found[name]
	if rec == nil {
		return empty, nil
	}
	supertypes, subtypes := TypeParts(rec.TypeLine)
	subtypeRows := []wire.OrderedMap{}
	for _, sub := range subtypes {
		// One query per subtype, and there are at most a handful. `ilike`
		// with word boundaries would be better, but DuckDB's `ilike` has no
		// \b -- so the count is over type lines *containing* the word, and the
		// payload says "type lines" rather than claiming anything sharper.
		pattern := "%" + sub + "%"
		var total, legends int64
		if err := c.DB().QueryRowContext(ctx, "SELECT count(*) FROM oracle_cards WHERE type_line ILIKE ?", pattern).Scan(&total); err != nil {
			return nil, err
		}
		if err := c.DB().QueryRowContext(ctx, "SELECT count(*) FROM oracle_cards WHERE type_line ILIKE ? "+
			"AND type_line ILIKE '%Legendary%'", pattern).Scan(&legends); err != nil {
			return nil, err
		}
		subtypeRows = append(subtypeRows, wire.OrderedMap{{Key: "name", Value: sub},
			{Key: "total", Value: total}, {Key: "legendary", Value: legends}})
	}
	// The character's name: everything before the first comma. A mononym
	// like "Goreclaw, Terror of Qal Sisma" gives "Goreclaw"; one with no comma
	// gives the whole name and simply matches itself, which `name <> ?` drops.
	character := strings.TrimSpace(strings.SplitN(name, ",", 2)[0])
	others := []wire.OrderedMap{}
	rows, err := c.DB().QueryContext(ctx, "SELECT name, type_line, mana_cost, image_normal, image_art_crop "+
		"FROM oracle_cards WHERE name ILIKE ? AND name <> ? ORDER BY edhrec_rank NULLS LAST LIMIT 6",
		"%"+character+"%", name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v [5]any
		ptrs := []any{&v[0], &v[1], &v[2], &v[3], &v[4]}
		if err := rows.Scan(ptrs...); err != nil {
			_ = rows.Close()
			return nil, err
		}
		others = append(others, wire.OrderedMap{{Key: "name", Value: pool.AsStringPtr(v[0])},
			{Key: "type_line", Value: pool.AsStringPtr(v[1])}, {Key: "mana_cost", Value: pool.AsStringPtr(v[2])},
			{Key: "image", Value: pool.AsStringPtr(v[3])}, {Key: "art_crop", Value: pool.AsStringPtr(v[4])}})
	}
	_ = rows.Close()
	// Scryfall's stable id for the *card*, across every printing of it. Two
	// things need it and neither is decorative: the art picker lists a card's
	// printings by it, and ADR 19 keys a cached dossier on it -- a dossier is
	// about a character, so every deck that commander leads shares one.
	var oracleID any
	_ = c.DB().QueryRowContext(ctx, "SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1", name).Scan(&oracleID)
	var count int64
	var first any
	if err := c.DB().QueryRowContext(ctx, "SELECT count(*), min(released_at) FROM printings "+
		"WHERE oracle_id = (SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1)", name).Scan(&count, &first); err != nil {
		return nil, err
	}
	var firstReleased, firstSet *string
	if t, ok := first.(time.Time); ok {
		s := t.Format("2006-01-02")
		firstReleased = &s
		var setName any
		if err := c.DB().QueryRowContext(ctx, "SELECT set_name FROM printings WHERE oracle_id = "+
			"(SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1) AND released_at = ? LIMIT 1",
			name, t).Scan(&setName); err == nil {
			firstSet = pool.AsStringPtr(setName)
		}
	}
	// This panel is about a *deck's* commander, so the two printing facts
	// below follow the deck's chosen printing exactly as the hero panel does.
	artist, flavor := rec.Artist, rec.FlavorText
	if d.CommanderArt != "" {
		arts, err := ChosenArts(ctx, c, []string{d.CommanderArt})
		if err != nil {
			return nil, err
		}
		if chosen, ok := arts[d.CommanderArt]; ok {
			artist, flavor = chosen.Artist, chosen.FlavorText
		}
	}
	card := wire.OrderedMap{{Key: "name", Value: rec.Name}, {Key: "oracle_id", Value: pool.AsStringPtr(oracleID)},
		{Key: "mana_cost", Value: rec.ManaCost}, {Key: "type_line", Value: rec.TypeLine},
		{Key: "oracle_text", Value: rec.OracleText}, {Key: "flavor_text", Value: flavor}, {Key: "artist", Value: artist},
		{Key: "power", Value: rec.Power}, {Key: "toughness", Value: rec.Toughness}, {Key: "loyalty", Value: rec.Loyalty},
		{Key: "image", Value: rec.ImageNormal}, {Key: "art_crop", Value: rec.ImageArtCrop},
		{Key: "color_identity", Value: rec.ColorIdentity}, {Key: "edhrec_rank", Value: rec.EdhrecRank},
		{Key: "game_changer", Value: rec.GameChanger}}
	return wire.OrderedMap{{Key: "slug", Value: d.Slug}, {Key: "card", Value: card},
		{Key: "supertypes", Value: supertypes}, {Key: "subtypes", Value: subtypeRows},
		{Key: "other_cards", Value: others}, {Key: "printings", Value: wire.OrderedMap{
			{Key: "count", Value: count}, {Key: "first_released", Value: firstReleased},
			{Key: "first_set", Value: firstSet}}}}, nil
}
