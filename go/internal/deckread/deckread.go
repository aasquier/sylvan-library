// Package deckread is the deck routes' read side: the payload builders they
// answer with.
//
// It exists because two callers need them and neither is the other's
// parent: the routes call these builders AND the Claude tools call them, so
// a tool result and a route payload are the same bytes by construction.
// This logic once lived inside the HTTP handlers, which was fine while the
// routes were the only caller and stopped being fine the moment the Claude
// surfaces needed the same facts.
//
// **The layering was not a preference; the boundary guard chose it.**
// `internal/api` imports `internal/deckedit`, because the same package holds
// the write handlers. Had the Claude tools reached these builders through
// `internal/api`, `claude/boundary_test.go` would have failed on the
// transitive import ban -- correctly, since it would mean anything under the
// Claude surfaces could reach a deck write through one hop. So the shared code
// lives BELOW both: this package has no writes in it and imports nothing that
// does, which is what lets the tools call it and the guard stay green.
//
// Every function here takes a leased pool that may be nil. That is the shape
// every deck route already had and it is deliberate: **degrade, never fail,
// when the pool is missing.** A nil pool is how the gate is told it was never
// consulted; an empty map means a pool that has never heard of these cards.
package deckread

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/analyze"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// PoolFor is `service._pool_for`: the deck's names, looked up.
//
// It is only ever called with a live connection, so it has no "no pool" answer
// of its own -- the caller holds one, and which one it holds matters. A caller
// that hands the result to `gate.Validate` must leave the map **nil** when
// there is no pool, because nil is how the gate is told it was never consulted;
// an empty map means a pool that has never heard of any of these cards. A
// caller that only looks names up may use either, and every one of them uses an
// empty map so the lookups read straight through.
func PoolFor(ctx context.Context, c *pool.Conn, d *deck.Deck) (map[string]*pool.CardRecord, error) {
	names := append([]string{}, d.Commander...)
	for _, card := range d.Cards {
		names = append(names, card.Name)
	}
	for _, card := range d.SwapBoard {
		names = append(names, card.Name)
	}
	for _, card := range d.Graveyard {
		names = append(names, card.Name)
	}
	if d.Companion != nil {
		names = append(names, *d.Companion)
	}
	return c.GetCards(ctx, names)
}

// Tile is one row of `GET /api/decks`, in `service._tiles`' key order, with
// `showcase` appended by `list_library`.
type Tile struct {
	Slug           string   `json:"slug"`
	Owner          string   `json:"owner"`
	Name           string   `json:"name"`
	Writable       bool     `json:"writable"`
	Shared         bool     `json:"shared"`
	Pilot          string   `json:"pilot"`
	Status         string   `json:"status"`
	Stage          string   `json:"stage"`
	NeedsRationale int      `json:"needs_rationale"`
	Commander      []string `json:"commander"`
	Companion      *string  `json:"companion"`
	Bracket        *int     `json:"bracket"`
	Archetype      string   `json:"archetype"`
	Themes         []string `json:"themes"`
	TotalCards     int      `json:"total_cards"`
	LandCount      int      `json:"land_count"`
	Strategy       any      `json:"strategy"`
	ArtCrop        *string  `json:"art_crop"`
	ColorIdentity  []string `json:"color_identity"`
	Errors         *int     `json:"errors"`
	Warnings       *int     `json:"warnings"`
	Showcase       bool     `json:"showcase"`
}

// ChosenArt is one row of `service._chosen_arts`: the printing behind a
// chosen art id.
type ChosenArt struct {
	Image      *string
	ArtCrop    *string
	SetName    *string
	SetCode    string
	Artist     *string
	FlavorText *string
}

// ChosenArts is `_chosen_arts`: the printings behind a run of art ids, one
// query, keyed by id; an id the pool no longer has simply does not come
// back. The painter and the flavour text ride with the picture because they
// are the printing's.
func ChosenArts(ctx context.Context, c *pool.Conn, ids []string) (map[string]ChosenArt, error) {
	out := map[string]ChosenArt{}
	if len(ids) == 0 || c == nil {
		return out, nil
	}
	have, err := c.Columns(ctx, "printings")
	if err != nil {
		return nil, err
	}
	extra := []string{}
	for _, col := range []string{"artist", "flavor_text"} {
		if have[col] {
			extra = append(extra, col)
		} else {
			extra = append(extra, "NULL AS "+col)
		}
	}
	holders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := c.DB().QueryContext(ctx, "SELECT id, image_normal, set_name, set_code, "+strings.Join(extra, ", ")+
		" FROM printings WHERE id IN ("+holders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v [6]any
		ptrs := make([]any, len(v))
		for i := range v {
			ptrs[i] = &v[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		image := pool.AsStringPtr(v[1])
		if image == nil {
			continue
		}
		out[pool.AsString(v[0])] = ChosenArt{Image: image, ArtCrop: pool.ArtCropFrom(image),
			SetName: pool.AsStringPtr(v[2]), SetCode: strings.ToUpper(pool.AsString(v[3])),
			Artist: pool.AsStringPtr(v[4]), FlavorText: pool.AsStringPtr(v[5])}
	}
	return out, rows.Err()
}

// Tiles is `service._tiles`: the shelf's payload for a run of decks sharing
// one owner and pool -- one lookup for every name on the shelf, one for
// every pinned printing, then the gate per deck.
func Tiles(ctx context.Context, c *pool.Conn, decks []*deck.Deck, writable bool, owner string) ([]Tile, error) {
	nameSet := map[string]bool{}
	for _, d := range decks {
		for _, n := range d.CardNames(true) {
			nameSet[n] = true
		}
		for _, card := range d.SwapBoard {
			nameSet[card.Name] = true
		}
		if d.Companion != nil {
			nameSet[*d.Companion] = true
		}
	}
	cards := map[string]*pool.CardRecord{}
	arts := map[string]ChosenArt{}
	if c != nil && len(nameSet) > 0 {
		names := make([]string, 0, len(nameSet))
		for n := range nameSet {
			names = append(names, n)
		}
		sort.Strings(names)
		var err error
		if cards, err = c.GetCards(ctx, names); err != nil {
			return nil, err
		}
		ids := []string{}
		for _, d := range decks {
			if d.CommanderArt != "" {
				ids = append(ids, d.CommanderArt)
			}
		}
		if arts, err = ChosenArts(ctx, c, ids); err != nil {
			return nil, err
		}
	}
	out := []Tile{}
	for _, d := range decks {
		row := Tile{Slug: d.Slug, Owner: owner, Name: d.Name, Writable: writable, Shared: d.Shared,
			Pilot: d.Pilot, Status: d.Status, Stage: d.Stage, NeedsRationale: len(d.Unjustified()),
			Commander: append([]string{}, d.Commander...), Companion: d.Companion, Bracket: d.Bracket,
			Archetype: d.Archetype(), Themes: append([]string{}, d.Themes...),
			TotalCards: d.TotalCards(), LandCount: d.LandCount(), Strategy: d.Strategy,
			ColorIdentity: []string{}}
		if c != nil {
			rep := gate.Validate(d, cards, gate.DefaultSize)
			errs, warns := len(rep.Errors()), len(rep.Warnings())
			row.Errors, row.Warnings = &errs, &warns
			if len(d.Commander) > 0 {
				if rec := cards[d.Commander[0]]; rec != nil {
					row.ArtCrop = rec.ImageArtCrop
					if row.ArtCrop == nil {
						row.ArtCrop = rec.ImageNormal
					}
					row.ColorIdentity = append([]string{}, rec.ColorIdentity...)
				}
			}
			if chosen, ok := arts[d.CommanderArt]; ok {
				if chosen.ArtCrop != nil {
					row.ArtCrop = chosen.ArtCrop
				} else if chosen.Image != nil {
					row.ArtCrop = chosen.Image
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// CardJSON is `service._card_json`: one row of the 99, merged with what the
// pool knows about it; `full` adds what only a hero panel wants.
type CardJSON struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Why      string `json:"why"`
	Qty      int    `json:"qty"`
	Art      string `json:"art"`
	Known    bool   `json:"known"`
	// The pool's facts, present only when known.
	ManaCost      *string  `json:"mana_cost,omitempty"`
	CMC           *float64 `json:"cmc,omitempty"`
	TypeLine      *string  `json:"type_line,omitempty"`
	OracleText    *string  `json:"oracle_text,omitempty"`
	ColorIdentity []string `json:"color_identity,omitempty"`
	Image         *string  `json:"image,omitempty"`
	ArtCrop       *string  `json:"art_crop,omitempty"`
	EdhrecRank    *int     `json:"edhrec_rank,omitempty"`
	Reserved      *bool    `json:"reserved,omitempty"`
	Power         *string  `json:"power,omitempty"`
	Toughness     *string  `json:"toughness,omitempty"`
	Loyalty       *string  `json:"loyalty,omitempty"`
	GameChanger   *bool    `json:"game_changer,omitempty"`
	FlavorText    *string  `json:"flavor_text,omitempty"`
	Artist        *string  `json:"artist,omitempty"`
	// full: the hero fields were asked for; extra: the trailing pairs
	// `get_deck` appends to the commander's row (oracle_id, printing).
	full  bool
	Extra []wire.KV
}

// MarshalJSON writes the row with the recorded key set exactly: the pool's
// keys appear only for a known card (every one of them, null included), and
// the two hero fields only when asked. `omitempty` alone would drop a known
// card's null `power`, which the payload writes.
func (c CardJSON) MarshalJSON() ([]byte, error) {
	out := []wire.KV{{Key: "name", Value: c.Name}, {Key: "category", Value: c.Category}, {Key: "why", Value: c.Why}, {Key: "qty", Value: c.Qty},
		{Key: "art", Value: c.Art}, {Key: "known", Value: c.Known}}
	if c.Known {
		out = append(out, wire.KV{Key: "mana_cost", Value: c.ManaCost}, wire.KV{Key: "cmc", Value: Deref(c.CMC)}, wire.KV{Key: "type_line", Value: c.TypeLine},
			wire.KV{Key: "oracle_text", Value: c.OracleText}, wire.KV{Key: "color_identity", Value: c.ColorIdentity}, wire.KV{Key: "image", Value: c.Image},
			wire.KV{Key: "art_crop", Value: c.ArtCrop}, wire.KV{Key: "edhrec_rank", Value: c.EdhrecRank}, wire.KV{Key: "reserved", Value: c.Reserved},
			wire.KV{Key: "power", Value: c.Power}, wire.KV{Key: "toughness", Value: c.Toughness}, wire.KV{Key: "loyalty", Value: c.Loyalty},
			wire.KV{Key: "game_changer", Value: c.GameChanger})
		if c.full {
			out = append(out, wire.KV{Key: "flavor_text", Value: c.FlavorText}, wire.KV{Key: "artist", Value: c.Artist})
		}
		out = append(out, c.Extra...)
	}
	return wire.MarshalOrdered(out)
}

func Deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func CardRow(entry deck.CardEntry, rec *pool.CardRecord, full bool) CardJSON {
	row := CardJSON{Name: entry.Name, Category: entry.Category, Why: entry.Why, Qty: entry.Qty,
		Art: entry.Art, Known: rec != nil, full: full}
	if rec != nil {
		cmc := rec.CMC
		reserved, gc := rec.Reserved, rec.GameChanger
		row.ManaCost, row.CMC, row.TypeLine, row.OracleText = rec.ManaCost, &cmc, &rec.TypeLine, &rec.OracleText
		row.ColorIdentity = append([]string{}, rec.ColorIdentity...)
		row.Image, row.ArtCrop, row.EdhrecRank, row.Reserved = rec.ImageNormal, rec.ImageArtCrop, rec.EdhrecRank, &reserved
		row.Power, row.Toughness, row.Loyalty, row.GameChanger = rec.Power, rec.Toughness, rec.Loyalty, &gc
		if full {
			row.FlavorText, row.Artist = rec.FlavorText, rec.Artist
		}
	}
	return row
}

// WithArt is `service._with_art`: one row, wearing its chosen printing --
// the picture, the painter and the flavour text, never the card's own text.
func WithArt(row CardJSON, overrides map[string]ChosenArt) CardJSON {
	chosen, ok := overrides[row.Name]
	if !ok || !row.Known {
		return row
	}
	row.Image = chosen.Image
	if chosen.ArtCrop != nil {
		row.ArtCrop = chosen.ArtCrop
	}
	if row.full {
		row.Artist, row.FlavorText = chosen.Artist, chosen.FlavorText
	}
	return row
}

// CardArtOverrides is `_card_art_overrides`: the chosen printings for every
// card that picked one, keyed by name, one query.
func CardArtOverrides(ctx context.Context, c *pool.Conn, d *deck.Deck) (map[string]ChosenArt, error) {
	chosen := map[string]string{}
	ids := []string{}
	// **First choice wins, for the id list and the name map alike.** They
	// used to disagree: the id list kept the first printing a name chose
	// while the map kept the last, so a card that appears in two sections
	// with two different printings -- the 99 and the swap board, which is
	// exactly what a swap in progress looks like -- had the wrong id queried
	// and lost its art entirely. The sections are walked 99-first, so the
	// deck proper's choice is the one that renders.
	for _, list := range [][]deck.CardEntry{d.Cards, d.SwapBoard, d.Graveyard} {
		for _, card := range list {
			if card.Art == "" {
				continue
			}
			if _, dup := chosen[card.Name]; dup {
				continue
			}
			ids = append(ids, card.Art)
			chosen[card.Name] = card.Art
		}
	}
	out := map[string]ChosenArt{}
	if len(chosen) == 0 || c == nil {
		return out, nil
	}
	byID, err := ChosenArts(ctx, c, ids)
	if err != nil {
		return nil, err
	}
	for name, id := range chosen {
		if art, ok := byID[id]; ok {
			out[name] = art
		}
	}
	return out, nil
}

// SuggestionCard is one candidate as `service.suggestions_for` renders it.
type SuggestionCard struct {
	Name          string   `json:"name"`
	ManaCost      *string  `json:"mana_cost"`
	CMC           float64  `json:"cmc"`
	TypeLine      string   `json:"type_line"`
	OracleText    string   `json:"oracle_text"`
	ColorIdentity []string `json:"color_identity"`
	Image         *string  `json:"image"`
	ArtCrop       *string  `json:"art_crop"`
	EdhrecRank    *int     `json:"edhrec_rank"`
	Score         float64  `json:"score"`
	Reasons       []string `json:"reasons"`
}

type SuggestionTarget struct {
	Card       string           `json:"card"`
	Code       string           `json:"code"`
	Why        string           `json:"why"`
	Candidates []SuggestionCard `json:"candidates"`
}

// TypeParts is `service._type_parts`: a type line split into the part before
// the dash and the part after, front face only.
func TypeParts(typeLine string) ([]string, []string) {
	front := strings.TrimSpace(strings.SplitN(typeLine, "//", 2)[0])
	for _, dash := range []string{"—", "–", " - "} {
		if before, after, ok := strings.Cut(front, dash); ok {
			return strings.Fields(before), strings.Fields(after)
		}
	}
	return strings.Fields(front), []string{}
}

// DeckPayload is one deck's whole contents, in the recorded key order.
//
// The ordered pairs are the payload, not a convenience: `encoding/json`
// sorts a map's keys, the recorded payload holds a deliberate insertion
// order, and that difference has shipped once already -- the deck page's
// Notes tab was alphabetical from v159 to v166, scrambling a deliberate
// reading order into nonsense.
//
// `c` may be nil, and the answer is still a deck: `pool_available` says which
// happened, and every card row degrades to name-and-rationale rather than
// failing. That is the shape every deck route already had.
//
// This left `internal/api`'s getDeck handler in Phase 6 so that the Claude
// tools could answer with the same bytes the route answers with -- one
// builder, called by both, so agreement is structural rather than tested.
// A second builder would drift, and the drift would be invisible: the
// model would simply be told slightly different facts than the screen shows.
func DeckPayload(ctx context.Context, c *pool.Conn, d *deck.Deck, writable bool, owner string) ([]wire.KV, error) {
	cards := map[string]*pool.CardRecord{}
	if c != nil {
		var err error
		if cards, err = PoolFor(ctx, c, d); err != nil {
			return nil, err
		}
	}
	var commanderRec *pool.CardRecord
	if len(d.Commander) > 0 {
		commanderRec = cards[d.Commander[0]]
	}
	var commanderCard any
	if commanderRec != nil {
		why := d.NoteText("commander_why")
		row := CardRow(deck.CardEntry{Name: d.Commander[0], Category: "commander", Why: why, Qty: 1},
			commanderRec, true)
		// The oracle id rides here: the motion tier is keyed on it.
		var oracleID *string
		if c != nil {
			var id any
			if err := c.DB().QueryRowContext(ctx,
				"SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1", d.Commander[0]).Scan(&id); err == nil {
				oracleID = pool.AsStringPtr(id)
			}
		}
		row.Extra = append(row.Extra, wire.KV{Key: "oracle_id", Value: oracleID})
		// The chosen printing replaces what is the *printing's* -- the
		// images, the painter, the flavour text -- and nothing else.
		if d.CommanderArt != "" && c != nil {
			arts, err := ChosenArts(ctx, c, []string{d.CommanderArt})
			if err != nil {
				return nil, err
			}
			if chosen, ok := arts[d.CommanderArt]; ok {
				row.Image = chosen.Image
				if chosen.ArtCrop != nil {
					row.ArtCrop = chosen.ArtCrop
				}
				row.Artist, row.FlavorText = chosen.Artist, chosen.FlavorText
				row.Extra = append(row.Extra, wire.KV{Key: "printing", Value: map[string]any{
					"set_name": chosen.SetName, "set_code": chosen.SetCode}})
			}
		}
		commanderCard = row
	}
	overrides, err := CardArtOverrides(ctx, c, d)
	if err != nil {
		return nil, err
	}
	rows := func(list []deck.CardEntry) []CardJSON {
		out := []CardJSON{}
		for _, e := range list {
			out = append(out, WithArt(CardRow(e, cards[e.Name], false), overrides))
		}
		return out
	}
	identity := []string{}
	if commanderRec != nil {
		identity = append(identity, commanderRec.ColorIdentity...)
	}
	body := []wire.KV{
		{Key: "commander_art", Value: d.CommanderArt}, {Key: "slug", Value: d.Slug}, {Key: "name", Value: d.Name},
		{Key: "writable", Value: writable}, {Key: "owner", Value: owner}, {Key: "shared", Value: d.Shared},
		{Key: "pilot", Value: d.Pilot}, {Key: "status", Value: d.Status}, {Key: "stage", Value: d.Stage},
		{Key: "needs_rationale", Value: len(d.Unjustified())}, {Key: "commander", Value: d.Commander},
		{Key: "companion", Value: d.Companion}, {Key: "bracket", Value: d.Bracket}, {Key: "archetype", Value: d.Archetype()},
		{Key: "themes", Value: d.Themes}, {Key: "strategy", Value: d.Strategy}, {Key: "notes", Value: d.Notes},
		{Key: "total_cards", Value: d.TotalCards()}, {Key: "land_count", Value: d.LandCount()},
		{Key: "color_identity", Value: identity}, {Key: "commander_card", Value: commanderCard},
		{Key: "cards", Value: rows(d.Cards)}, {Key: "swap_board", Value: rows(d.SwapBoard)}, {Key: "graveyard", Value: rows(d.Graveyard)},
		{Key: "pool_available", Value: c != nil},
	}
	return body, nil
}

// Suggestions is `service.suggestions_for`: for each card the gate flagged as
// banned or off-colour, a scored shortlist of legal replacements.
//
// Only errors a *different card* would fix are covered — a missing rationale
// is not one of them, and no shortlist could be. That is rule 4 showing
// through the API surface: the gate can say a slot is empty of justification,
// and nothing here can fill it.
//
// Requires a live pool, unlike the rest of this package: a shortlist with no
// candidates in it is not a degraded answer, it is a misleading one. The
// caller reports the absence instead.
func Suggestions(ctx context.Context, c *pool.Conn, d *deck.Deck, limit int) ([]SuggestionTarget, error) {
	targets := []SuggestionTarget{}
	cards, err := PoolFor(ctx, c, d)
	if err != nil {
		return nil, err
	}
	rep := gate.Validate(d, cards, gate.DefaultSize)
	for _, issue := range rep.Errors() {
		if issue.Card == nil || (issue.Code != "banned" && issue.Code != "color-identity") {
			continue
		}
		name := *issue.Card
		candidates, err := suggest.ReplacementsFor(ctx, c, d, cards, name, limit)
		if err != nil {
			return nil, err
		}
		why := ""
		for _, card := range d.Cards {
			if card.Name == name {
				why = card.Why
				break
			}
		}
		target := SuggestionTarget{Card: name, Code: issue.Code, Why: why, Candidates: []SuggestionCard{}}
		for _, cand := range candidates {
			rec := cand.Record
			target.Candidates = append(target.Candidates, SuggestionCard{Name: rec.Name, ManaCost: rec.ManaCost,
				CMC: rec.CMC, TypeLine: rec.TypeLine, OracleText: rec.OracleText,
				ColorIdentity: rec.ColorIdentity, Image: rec.ImageNormal, ArtCrop: rec.ImageArtCrop,
				EdhrecRank: rec.EdhrecRank, Score: cand.Score, Reasons: cand.Reasons})
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// Validate is `service.validate_deck`: the gate's verdict over one deck.
//
// Three lines, and it is here rather than at each call site for one reason:
// **the nil-versus-empty distinction**. `gate.Validate` is told the pool was
// never consulted by a NIL map, and told the pool has never heard of these
// cards by an EMPTY one — two different verdicts from the same deck. Every
// caller getting that right independently is a bug waiting for the second
// caller, which is exactly what the Claude tools now are.
func Validate(ctx context.Context, c *pool.Conn, d *deck.Deck) (*gate.Report, error) {
	var cards map[string]*pool.CardRecord
	if c != nil {
		var err error
		if cards, err = PoolFor(ctx, c, d); err != nil {
			return nil, err
		}
	}
	return gate.Validate(d, cards, gate.DefaultSize), nil
}

// Stats is `service.stats_for`: the deterministic counts for one deck — the
// curve, the macro categories against its bracket's targets, and the colour
// spread.
//
// Note the OPPOSITE nil-handling to Validate, and that it is deliberate rather
// than an inconsistency: this one starts from an empty map, because the
// analysis only ever looks names up and an empty map reads straight through.
// The gate is the caller that can tell the two apart.
func Stats(ctx context.Context, c *pool.Conn, d *deck.Deck) (analyze.Stats, error) {
	cards := map[string]*pool.CardRecord{}
	if c != nil {
		var err error
		if cards, err = PoolFor(ctx, c, d); err != nil {
			return analyze.Stats{}, err
		}
	}
	return analyze.DeckStats(d, cards), nil
}

// ---- the card search ----------------------------------------------------

// SearchCard is one row of `service.search_cards`' answer, in its key order.
type SearchCard struct {
	Name          string   `json:"name"`
	ManaCost      *string  `json:"mana_cost"`
	CMC           float64  `json:"cmc"`
	TypeLine      *string  `json:"type_line"`
	OracleText    *string  `json:"oracle_text"`
	ColorIdentity []string `json:"color_identity"`
	EdhrecRank    *int     `json:"edhrec_rank"`
	Image         *string  `json:"image"`
	ArtCrop       *string  `json:"art_crop"`
	Reserved      bool     `json:"reserved"`
	PriceUSD      *float64 `json:"price_usd"`
}

// SearchQuery is what the card search takes. Every field is optional, and
// that is contract rather than convenience: "which legends have exactly
// this colour identity" is a search with no text in it at all, and it is
// the question ADR 20's proposal is built around.
type SearchQuery struct {
	Text           string
	Identity       string
	IdentityExact  bool
	CommandersOnly bool
	TypeLine       string
	CMCMax         float64
	HaveCMC        bool
	PriceMax       float64
	HavePrice      bool
	Sort           string
	Limit          int
}

// SearchCards is `service.search_cards`: the discovery tool.
//
// **It returns only Commander-legal cards, so it is not a lookup.** A banned
// card is simply missing, and absence here is not evidence a card does not
// exist -- which is why `GetCards` exists beside it and why the Claude tool
// descriptions say so twice.
//
// Requires a live pool. Left `internal/api`'s handler in Phase 6 so the Claude
// tools search the same index the card-search page searches, with the same
// filters applied in the same order -- the price cut and the commander rule
// both run AFTER the query, and a second implementation would put them
// somewhere subtly different.
func SearchCards(ctx context.Context, c *pool.Conn, q SearchQuery) ([]SearchCard, error) {
	text, identity, typeLine := q.Text, q.Identity, q.TypeLine
	identityExact, commandersOnly := q.IdentityExact, q.CommandersOnly
	cmcMax, haveCMC := q.CMCMax, q.HaveCMC
	priceMax, havePrice := q.PriceMax, q.HavePrice
	sortBy, limit := q.Sort, q.Limit
	if sortBy == "" {
		sortBy = "edhrec"
	}
	if limit <= 0 {
		limit = 60
	}
	where := []string{"json_extract_string(legalities, 'commander') = 'legal'"}
	params := []any{}
	if identity != "" || identityExact {
		allowed := []string{}
		for _, ch := range strings.ToUpper(identity) {
			if strings.ContainsRune("WUBRG", ch) {
				allowed = append(allowed, string(ch))
			}
		}
		where = append(where, fmt.Sprintf("len(list_filter(color_identity, x -> x NOT IN (%s))) = 0",
			QuotedList(allowed)))
		if identityExact {
			// Subset plus the right size is set equality, and it lets the
			// colourless slot work: an empty identity with length 0.
			where = append(where, fmt.Sprintf("len(color_identity) = %d", len(allowed)))
		}
	}
	// `contains(lower(col), ?)` rather than ILIKE: the same question asked
	// cheaply, and `%` and `_` stop being wildcards.
	if text != "" {
		where = append(where, "(contains(lower(name), ?) OR contains(lower(oracle_text), ?))")
		params = append(params, strings.ToLower(text), strings.ToLower(text))
	}
	if typeLine != "" {
		where = append(where, "contains(lower(type_line), ?)")
		params = append(params, strings.ToLower(typeLine))
	}
	if commandersOnly {
		// A superset of CanBeCommander pushed into SQL so LIMIT counts
		// candidates; the authoritative check runs below.
		where = append(where, "(type_line ILIKE '%Legendary%Creature%'"+
			" OR contains(lower(oracle_text), 'can be your commander'))")
	}
	if haveCMC {
		where = append(where, "cmc <= ?")
		params = append(params, cmcMax)
	}
	order := map[string]string{
		"edhrec": "edhrec_rank NULLS LAST",
		"cmc":    "cmc, edhrec_rank NULLS LAST",
		"name":   "name",
		"newest": "released_at DESC NULLS LAST",
	}[sortBy]
	if order == "" {
		order = "edhrec_rank NULLS LAST"
	}
	// **The cheapest price is asked for separately, after the LIMIT.** It used
	// to ride along as a correlated subquery -- `(SELECT min(p.price_usd) …
	// WHERE p.oracle_id = o.oracle_id)` -- which reads as the tidier query and
	// is the expensive one, because DuckDB decorrelates it: the subquery
	// becomes an aggregate over all 107,355 printings joined against **every**
	// row that survived the WHERE, and the whole wide result (oracle text
	// included) has to be materialised before the top-N can cut it to sixty.
	// Measured on the full pool, this Mac: a text search 38.6ms -> 27.5ms, and
	// the search the card page fires on mount -- no text at all, which matches
	// ~28,000 cards -- **84.8ms -> 38.4ms**. Asking again for the sixty rows
	// that survived costs about 9ms.
	//
	// The rule underneath, because it generalises: a projection whose value is
	// only ever *read* belongs after the LIMIT, never inside the query the
	// LIMIT applies to.
	sql := `
            SELECT o.name, o.mana_cost, o.cmc, o.type_line, o.oracle_text,
                   o.color_identity, o.edhrec_rank, o.image_normal,
                   o.image_art_crop, o.reserved, o.oracle_id
            FROM oracle_cards o
            WHERE ` + strings.Join(where, " AND ") + `
            ORDER BY ` + order + `
            LIMIT ?`
	rows, err := c.DB().QueryContext(ctx, sql, append(params, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := []SearchCard{}
	oracleIDs := []string{}
	for rows.Next() {
		var v [11]any
		ptrs := make([]any, len(v))
		for i := range v {
			ptrs[i] = &v[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		card := SearchCard{
			Name: pool.AsString(v[0]), ManaCost: pool.AsStringPtr(v[1]), CMC: pool.AsFloat(v[2]),
			TypeLine: pool.AsStringPtr(v[3]), OracleText: pool.AsStringPtr(v[4]),
			ColorIdentity: pool.AsStrings(v[5]), EdhrecRank: pool.AsIntPtr(v[6]),
			Image: pool.AsStringPtr(v[7]), ArtCrop: pool.AsStringPtr(v[8]), Reserved: pool.AsBool(v[9]),
		}
		sort.Strings(card.ColorIdentity)
		found = append(found, card)
		oracleIDs = append(oracleIDs, pool.AsString(v[10]))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cheapest, err := cheapestPrintings(ctx, c, oracleIDs)
	if err != nil {
		return nil, err
	}
	for i := range found {
		if usd, ok := cheapest[oracleIDs[i]]; ok {
			found[i].PriceUSD = &usd
		}
	}
	if havePrice {
		kept := found[:0]
		for _, card := range found {
			if card.PriceUSD != nil && *card.PriceUSD <= priceMax {
				kept = append(kept, card)
			}
		}
		found = kept
	}
	if commandersOnly {
		// After the query rather than in SQL, because the rule reads
		// oracle text as well as the type line. One implementation.
		names := make([]string, 0, len(found))
		for _, card := range found {
			names = append(names, card.Name)
		}
		keep, err := c.GetCards(ctx, names)
		if err != nil {
			return nil, err
		}
		kept := found[:0]
		for _, card := range found {
			if rec := keep[card.Name]; rec != nil && gate.CanBeCommander(rec, false) {
				kept = append(kept, card)
			}
		}
		found = kept
	}
	return found, nil
}

// cheapestPrintings is the least paper price on record for each oracle id, and
// only for the ids asked about -- the second half of the split the search
// query's comment argues for.
//
// One statement whatever the count, through a list parameter rather than a run
// of `?` placeholders: the placeholder form builds a different SQL string for
// every result size, so no two searches could ever share a plan. `GetCards`
// settled on the same idiom for the same reason.
//
// An id with no priced printing is simply absent, which is how the caller
// keeps writing `null` for a card nobody has priced -- the correlated
// subquery's own answer, preserved.
func cheapestPrintings(ctx context.Context, c *pool.Conn, oracleIDs []string) (map[string]float64, error) {
	out := map[string]float64{}
	if len(oracleIDs) == 0 {
		return out, nil
	}
	rows, err := c.DB().QueryContext(ctx,
		`SELECT oracle_id, min(price_usd) FROM printings
                  WHERE price_usd IS NOT NULL
                    AND oracle_id IN (SELECT unnest(?::VARCHAR[]))
                  GROUP BY oracle_id`, oracleIDs)
	if err != nil {
		return nil, fmt.Errorf("cheapest printings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var usd any
		if err := rows.Scan(&id, &usd); err != nil {
			return nil, fmt.Errorf("cheapest printings: %w", err)
		}
		if price := AsFloatPtr(usd); price != nil {
			out[id] = *price
		}
	}
	return out, rows.Err()
}

func AsFloatPtr(v any) *float64 {
	switch t := v.(type) {
	case float64:
		return &t
	case float32:
		f := float64(t)
		return &f
	case int32:
		f := float64(t)
		return &f
	case int64:
		f := float64(t)
		return &f
	}
	return nil
}

func QuotedList(items []string) string {
	if len(items) == 0 {
		return "''"
	}
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "'" + s + "'"
	}
	return strings.Join(quoted, ", ")
}

// MaxNamedCards caps one lookup.
const MaxNamedCards = 100

// NamedCard is one row of the named-cards answer, in the recorded key order.
type NamedCard struct {
	// Name is the POOL's spelling, not the caller's: asked for "arahbo, roar
	// of the world" you get the real name back, which is what any follow-up
	// edit has to be keyed on.
	Name string `json:"name"`
	// AskedAs is what the caller wrote, when it differed. Null otherwise.
	AskedAs        *string  `json:"asked_as"`
	ManaCost       *string  `json:"mana_cost"`
	CMC            float64  `json:"cmc"`
	TypeLine       *string  `json:"type_line"`
	OracleText     *string  `json:"oracle_text"`
	ColorIdentity  []string `json:"color_identity"`
	Keywords       []string `json:"keywords"`
	Layout         *string  `json:"layout"`
	LegalCommander bool     `json:"legal_commander"`
	Reserved       bool     `json:"reserved"`
	EdhrecRank     *int     `json:"edhrec_rank"`
	Image          *string  `json:"image"`
	ArtCrop        *string  `json:"art_crop"`
	Power          *string  `json:"power"`
	Toughness      *string  `json:"toughness"`
	Loyalty        *string  `json:"loyalty"`
	Defense        *string  `json:"defense"`
	GameChanger    bool     `json:"game_changer"`
	FlavorText     *string  `json:"flavor_text"`
	Artist         *string  `json:"artist"`
}

// NamedCards is `service.cards_named`'s whole answer.
type NamedCards struct {
	Cards         []NamedCard `json:"cards"`
	NotFound      []string    `json:"not_found"`
	PoolAvailable bool        `json:"pool_available"`
	Message       string      `json:"message,omitempty"`
}

// CardsNamed is exact-name card lookup: the answer to "what does this card
// actually do".
//
// Distinct from SearchCards, and the distinction is the point. SearchCards
// filters to Commander-legal, which is right when the question is "what could
// I play" and wrong when the question is "what is this card" — **a banned card
// is invisible to it.** That was found by running a Claude turn rather than by
// reasoning about it: asked what the two cards failing the gate do, it could
// look up neither, said so, and answered from labelled recall. Honest, and
// still rule 1 failing.
//
// So this filters on nothing and reports LegalCommander per card instead,
// which is strictly more useful: a banned card comes back with its real oracle
// text *and* the fact that it is banned, rather than as a shrug.
//
// **Names that do not resolve are returned in NotFound, never omitted.** The
// pool drops misses silently; this is the loud handling every caller owes.
// A lookup that quietly returns four cards for five names is how a confident
// claim gets made about the fifth.
func CardsNamed(ctx context.Context, c *pool.Conn, names []string) (NamedCards, error) {
	wanted := make([]string, 0, len(names))
	for _, n := range names {
		trimmed := strings.TrimSpace(n)
		if trimmed == "" {
			continue
		}
		wanted = append(wanted, trimmed)
		if len(wanted) == MaxNamedCards {
			break
		}
	}
	if len(wanted) == 0 {
		return NamedCards{Cards: []NamedCard{}, NotFound: []string{}, PoolAvailable: true}, nil
	}
	if c == nil {
		return NamedCards{Cards: []NamedCard{}, NotFound: wanted, PoolAvailable: false,
			Message: "no card pool yet -- run `mtglab data refresh`"}, nil
	}
	found, err := c.GetCards(ctx, wanted)
	if err != nil {
		return NamedCards{}, err
	}
	out := NamedCards{Cards: []NamedCard{}, NotFound: []string{}, PoolAvailable: true}
	for _, asked := range wanted {
		rec := found[asked]
		if rec == nil {
			out.NotFound = append(out.NotFound, asked)
			continue
		}
		var askedAs *string
		if asked != rec.Name {
			a := asked
			askedAs = &a
		}
		identity := append([]string{}, rec.ColorIdentity...)
		sort.Strings(identity)
		keywords := rec.Keywords
		if keywords == nil {
			keywords = []string{}
		}
		// The three plain strings on the record become pointers here, as
		// the existing card rows do: the wire field is nullable, and an
		// empty oracle text is not the same as an absent one.
		typeLine, oracle, layout := rec.TypeLine, rec.OracleText, rec.Layout
		out.Cards = append(out.Cards, NamedCard{
			Name: rec.Name, AskedAs: askedAs, ManaCost: rec.ManaCost, CMC: rec.CMC,
			TypeLine: &typeLine, OracleText: &oracle,
			ColorIdentity: identity, Keywords: keywords, Layout: &layout,
			LegalCommander: rec.LegalCommander, Reserved: rec.Reserved,
			EdhrecRank: rec.EdhrecRank, Image: rec.ImageNormal, ArtCrop: rec.ImageArtCrop,
			Power: rec.Power, Toughness: rec.Toughness, Loyalty: rec.Loyalty,
			Defense: rec.Defense, GameChanger: rec.GameChanger,
			FlavorText: rec.FlavorText, Artist: rec.Artist,
		})
	}
	return out, nil
}

// Source is the read side of a deck library, and it is declared HERE rather
// than used from `internal/library` for a reason the boundary guard found.
//
// `internal/library` holds both the read `Source` and the write `Writer`, and
// imports `internal/deckedit` for the second — so anything naming
// `library.Source` reaches the deck editor transitively, and
// `internal/claude`'s boundary analysis fails it. Correctly: that is a Claude
// surface one hop from a deck write, whatever the intent.
//
// The cure is to keep the two apart: the reader's protocol declared on its
// own, never borrowed from the editor's package. Go's interfaces are
// structural, so `library.Source` satisfies this without being changed or
// even knowing it exists — the narrow interface is purely a statement about
// what the caller is allowed to want.
//
// Deliberately narrower than `library.Source`: only the methods a reader
// actually needs. A method added there does not appear here unless somebody
// decides a reader should have it.
type Source interface {
	// Slugs is every deck's slug, without parsing any of them.
	Slugs(ctx context.Context) ([]string, error)
	// Get is one deck, or an error the caller reports by name.
	Get(ctx context.Context, slug string) (*deck.Deck, error)
	// All is every deck, parsed, in a stable order.
	All(ctx context.Context) ([]*deck.Deck, error)
	// ReadText is the deck's YAML, verbatim.
	ReadText(ctx context.Context, slug string) (string, error)
	// Writable is REPORTED, not exercised: it says whether this caller could
	// edit these decks, so a payload can tell a UI to show a control. The
	// refusal itself belongs to the write path, which is nowhere near here.
	Writable() bool
}
