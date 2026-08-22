package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/analyze"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The deck reads (Phase 3's third family): the library shelf and the
// per-deck GETs -- the deck, the gate's verdict, the analysis, the
// suggestions, the commander's panel, the printings, the history and the
// artifacts shelf -- plus `/api/colors/progress`, which scores the library
// against the 32. Every route resolves its source through `Library`
// (ADR 22), so a deck the caller may not see is ErrNotFound -> 404 before a
// row is read, and none of them writes anything.

// appDB is the read-only app.db: the door's handle when auth is on; opened
// lazily when auth is off and the file exists (a laptop that has run an
// edit, say), and left alone when it does not -- reading must not acquire
// a database (`test_the_local_app_touches_no_database`).
func (a *API) appDB() *sql.DB {
	if a.db != nil {
		return a.db
	}
	a.lazy.Lock()
	defer a.lazy.Unlock()
	if a.lazyDB != nil {
		return a.lazyDB
	}
	if a.dbPath == "" {
		return nil
	}
	if _, err := os.Stat(a.dbPath); err != nil {
		return nil
	}
	db, err := auth.Open(a.dbPath)
	if err != nil {
		a.log.Warn("app.db exists but could not be opened read-only", "error", err)
		return nil
	}
	a.lazyDB = db
	return db
}

// library is `api/deps.py:library`: every deck this caller may reach.
func (a *API) library(ctx context.Context) (*library.Library, error) {
	db := a.appDB()
	resolver := library.Resolver{DecksDir: a.decksDir, AppDB: db,
		Maintainer: func(ctx context.Context) (string, error) {
			return library.MaintainerUsername(ctx, db, a.adminEmail)
		}}
	return resolver.For(ctx, auth.ScopeFrom(ctx))
}

// refuse turns the source's errors into the route layer's answers:
// ErrNotFound -> 404 `no deck '<x>'`, ErrArtifactNotFound -> 404
// `no artifact '<x>'`, anything else a 500. Reports whether it answered.
func (a *API) refuse(w http.ResponseWriter, where string, err error) bool {
	if err == nil {
		return false
	}
	var missing library.ErrNotFound
	var noArtifact library.ErrArtifactNotFound
	switch {
	case errors.As(err, &missing):
		wire.Detail(w, http.StatusNotFound, missing.Error())
	case errors.As(err, &noArtifact):
		wire.Detail(w, http.StatusNotFound, noArtifact.Error())
	default:
		a.log.Error("the deck route failed", "route", where, "error", err)
		wire.Detail(w, http.StatusInternalServerError, "the library could not answer that right now")
	}
	return true
}

// sourceFor resolves the owner segment, answering the 404 itself.
func (a *API) sourceFor(w http.ResponseWriter, r *http.Request) (library.Source, bool) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return nil, false
	}
	src, err := lib.SourceFor(r.Context(), r.PathValue("owner"))
	if a.refuse(w, "source", err) {
		return nil, false
	}
	return src, true
}

// poolFor is `service._pool_for`: the deck's names, looked up; an empty map
// when there is no pool.
func poolFor(ctx context.Context, c *pool.Conn, d *deck.Deck) (map[string]*pool.CardRecord, error) {
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

// withPool runs fn with a leased pool, or with nil when there is none --
// the shape every deck route takes: degrade, never fail, when the pool is
// missing.
func (a *API) withPool(ctx context.Context, fn func(c *pool.Conn) error) error {
	err := a.usePool(ctx, fn)
	if errors.Is(err, pool.ErrNoPool) {
		return fn(nil)
	}
	return err
}

// ---- the shelf ----------------------------------------------------------

// tile is one row of `GET /api/decks`, in `service._tiles`' key order, with
// `showcase` appended by `list_library`.
type tile struct {
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

// chosenArt is one row of `service._chosen_arts`: the printing behind a
// chosen art id.
type chosenArt struct {
	Image      *string
	ArtCrop    *string
	SetName    *string
	SetCode    string
	Artist     *string
	FlavorText *string
}

// chosenArts is `_chosen_arts`: the printings behind a run of art ids, one
// query, keyed by id; an id the pool no longer has simply does not come
// back. The painter and the flavour text ride with the picture because they
// are the printing's.
func chosenArts(ctx context.Context, c *pool.Conn, ids []string) (map[string]chosenArt, error) {
	out := map[string]chosenArt{}
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
		out[pool.AsString(v[0])] = chosenArt{Image: image, ArtCrop: pool.ArtCropFrom(image),
			SetName: pool.AsStringPtr(v[2]), SetCode: strings.ToUpper(pool.AsString(v[3])),
			Artist: pool.AsStringPtr(v[4]), FlavorText: pool.AsStringPtr(v[5])}
	}
	return out, rows.Err()
}

// tiles is `service._tiles`: the shelf's payload for a run of decks sharing
// one owner and pool -- one lookup for every name on the shelf, one for
// every pinned printing, then the gate per deck.
func tiles(ctx context.Context, c *pool.Conn, decks []*deck.Deck, writable bool, owner string) ([]tile, error) {
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
	arts := map[string]chosenArt{}
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
		if arts, err = chosenArts(ctx, c, ids); err != nil {
			return nil, err
		}
	}
	out := []tile{}
	for _, d := range decks {
		row := tile{Slug: d.Slug, Owner: owner, Name: d.Name, Writable: writable, Shared: d.Shared,
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

// listDecks is `GET /api/decks` -- `service.list_library`: every deck this
// caller may see, across every owner, their own first; one pool connection
// for the whole page.
func (a *API) listDecks(w http.ResponseWriter, r *http.Request) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "decks", err) {
		return
	}
	visible, err := lib.Visible(r.Context())
	if a.refuse(w, "decks", err) {
		return
	}
	out := []tile{}
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		out = []tile{}
		showcase := strings.ToLower(lib.FileOwner())
		for _, owned := range visible {
			decks, err := owned.Source.All(r.Context())
			if err != nil {
				return err
			}
			rows, err := tiles(r.Context(), c, decks, owned.Source.Writable(), owned.Owner)
			if err != nil {
				return err
			}
			for i := range rows {
				rows[i].Showcase = strings.ToLower(owned.Owner) == showcase
			}
			out = append(out, rows...)
		}
		return nil
	})
	if a.refuse(w, "decks", err) {
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// ---- one deck -----------------------------------------------------------

// cardJSON is `service._card_json`: one row of the 99, merged with what the
// pool knows about it; `full` adds what only a hero panel wants.
type cardJSON struct {
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
	extra []kv
}

// MarshalJSON writes the row with Python's key set exactly: the pool's keys
// appear only for a known card (every one of them, null included), and the
// two hero fields only when asked. `omitempty` alone would drop a known
// card's null `power`, which Python writes.
func (c cardJSON) MarshalJSON() ([]byte, error) {
	out := []kv{{"name", c.Name}, {"category", c.Category}, {"why", c.Why}, {"qty", c.Qty},
		{"art", c.Art}, {"known", c.Known}}
	if c.Known {
		out = append(out, kv{"mana_cost", c.ManaCost}, kv{"cmc", deref(c.CMC)}, kv{"type_line", c.TypeLine},
			kv{"oracle_text", c.OracleText}, kv{"color_identity", c.ColorIdentity}, kv{"image", c.Image},
			kv{"art_crop", c.ArtCrop}, kv{"edhrec_rank", c.EdhrecRank}, kv{"reserved", c.Reserved},
			kv{"power", c.Power}, kv{"toughness", c.Toughness}, kv{"loyalty", c.Loyalty},
			kv{"game_changer", c.GameChanger})
		if c.full {
			out = append(out, kv{"flavor_text", c.FlavorText}, kv{"artist", c.Artist})
		}
		out = append(out, c.extra...)
	}
	return marshalOrdered(out)
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func cardRow(entry deck.CardEntry, rec *pool.CardRecord, full bool) cardJSON {
	row := cardJSON{Name: entry.Name, Category: entry.Category, Why: entry.Why, Qty: entry.Qty,
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

// withArt is `service._with_art`: one row, wearing its chosen printing --
// the picture, the painter and the flavour text, never the card's own text.
func withArt(row cardJSON, overrides map[string]chosenArt) cardJSON {
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

// cardArtOverrides is `_card_art_overrides`: the chosen printings for every
// card that picked one, keyed by name, one query.
func cardArtOverrides(ctx context.Context, c *pool.Conn, d *deck.Deck) (map[string]chosenArt, error) {
	chosen := map[string]string{}
	ids := []string{}
	for _, list := range [][]deck.CardEntry{d.Cards, d.SwapBoard, d.Graveyard} {
		for _, card := range list {
			if card.Art != "" {
				if _, dup := chosen[card.Name]; !dup {
					ids = append(ids, card.Art)
				}
				chosen[card.Name] = card.Art
			}
		}
	}
	out := map[string]chosenArt{}
	if len(chosen) == 0 || c == nil {
		return out, nil
	}
	byID, err := chosenArts(ctx, c, ids)
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

// getDeck is `GET /api/decks/{owner}/{slug}` -- `service.get_deck`.
func (a *API) getDeck(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "deck", err) {
		return
	}
	var body []kv
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		cards := map[string]*pool.CardRecord{}
		if c != nil {
			var err error
			if cards, err = poolFor(r.Context(), c, d); err != nil {
				return err
			}
		}
		var commanderRec *pool.CardRecord
		if len(d.Commander) > 0 {
			commanderRec = cards[d.Commander[0]]
		}
		var commanderCard any
		if commanderRec != nil {
			why, _ := d.Notes["commander_why"].(string)
			row := cardRow(deck.CardEntry{Name: d.Commander[0], Category: "commander", Why: why, Qty: 1},
				commanderRec, true)
			// The oracle id rides here: the motion tier is keyed on it.
			var oracleID *string
			if c != nil {
				var id any
				if err := c.DB().QueryRowContext(r.Context(),
					"SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1", d.Commander[0]).Scan(&id); err == nil {
					oracleID = pool.AsStringPtr(id)
				}
			}
			row.extra = append(row.extra, kv{"oracle_id", oracleID})
			// The chosen printing replaces what is the *printing's* -- the
			// images, the painter, the flavour text -- and nothing else.
			if d.CommanderArt != "" && c != nil {
				arts, err := chosenArts(r.Context(), c, []string{d.CommanderArt})
				if err != nil {
					return err
				}
				if chosen, ok := arts[d.CommanderArt]; ok {
					row.Image = chosen.Image
					if chosen.ArtCrop != nil {
						row.ArtCrop = chosen.ArtCrop
					}
					row.Artist, row.FlavorText = chosen.Artist, chosen.FlavorText
					row.extra = append(row.extra, kv{"printing", map[string]any{
						"set_name": chosen.SetName, "set_code": chosen.SetCode}})
				}
			}
			commanderCard = row
		}
		overrides, err := cardArtOverrides(r.Context(), c, d)
		if err != nil {
			return err
		}
		rows := func(list []deck.CardEntry) []cardJSON {
			out := []cardJSON{}
			for _, e := range list {
				out = append(out, withArt(cardRow(e, cards[e.Name], false), overrides))
			}
			return out
		}
		identity := []string{}
		if commanderRec != nil {
			identity = append(identity, commanderRec.ColorIdentity...)
		}
		body = []kv{
			{"commander_art", d.CommanderArt}, {"slug", d.Slug}, {"name", d.Name},
			{"writable", src.Writable()}, {"owner", r.PathValue("owner")}, {"shared", d.Shared},
			{"pilot", d.Pilot}, {"status", d.Status}, {"stage", d.Stage},
			{"needs_rationale", len(d.Unjustified())}, {"commander", d.Commander},
			{"companion", d.Companion}, {"bracket", d.Bracket}, {"archetype", d.Archetype()},
			{"themes", d.Themes}, {"strategy", d.Strategy}, {"notes", d.Notes},
			{"total_cards", d.TotalCards()}, {"land_count", d.LandCount()},
			{"color_identity", identity}, {"commander_card", commanderCard},
			{"cards", rows(d.Cards)}, {"swap_board", rows(d.SwapBoard)}, {"graveyard", rows(d.Graveyard)},
			{"pool_available", c != nil},
		}
		return nil
	})
	if a.refuse(w, "deck", err) {
		return
	}
	raw, err := marshalOrdered(body)
	if a.refuse(w, "deck", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// validateDeck is `GET .../validate` -- `service.validate_deck`.
func (a *API) validateDeck(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "validate", err) {
		return
	}
	var rep *gate.Report
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var cards map[string]*pool.CardRecord
		if c != nil {
			var err error
			if cards, err = poolFor(r.Context(), c, d); err != nil {
				return err
			}
		}
		rep = gate.Validate(d, cards, gate.DefaultSize)
		return nil
	})
	if a.refuse(w, "validate", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"ok": rep.OK(), "errors": rep.Errors(), "warnings": rep.Warnings()})
}

// deckStats is `GET .../stats` -- `service.stats_for`.
func (a *API) deckStats(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "stats", err) {
		return
	}
	var stats analyze.Stats
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		cards := map[string]*pool.CardRecord{}
		if c != nil {
			var err error
			if cards, err = poolFor(r.Context(), c, d); err != nil {
				return err
			}
		}
		stats = analyze.DeckStats(d, cards)
		return nil
	})
	if a.refuse(w, "stats", err) {
		return
	}
	wire.JSON(w, http.StatusOK, stats)
}

// suggestionCard is one candidate as `service.suggestions_for` renders it.
type suggestionCard struct {
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

type suggestionTarget struct {
	Card       string           `json:"card"`
	Code       string           `json:"code"`
	Why        string           `json:"why"`
	Candidates []suggestionCard `json:"candidates"`
}

// suggestions is `GET .../suggestions` -- `service.suggestions_for`:
// replacement shortlists for the cards the gate says have to go, only for
// errors a different card would fix. Reports rather than resolves (ADR 8).
func (a *API) suggestions(w http.ResponseWriter, r *http.Request) {
	var errs []wire.ValidationError
	limit := boundedInt(r.URL.Query(), "limit", 5, 1, 20, &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	d, err := src.Get(r.Context(), slug)
	if a.refuse(w, "suggestions", err) {
		return
	}
	targets := []suggestionTarget{}
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		cards, err := poolFor(r.Context(), c, d)
		if err != nil {
			return err
		}
		rep := gate.Validate(d, cards, gate.DefaultSize)
		for _, issue := range rep.Errors() {
			if issue.Card == nil || (issue.Code != "banned" && issue.Code != "color-identity") {
				continue
			}
			name := *issue.Card
			candidates, err := suggest.ReplacementsFor(r.Context(), c, d, cards, name, limit)
			if err != nil {
				return err
			}
			why := ""
			for _, card := range d.Cards {
				if card.Name == name {
					why = card.Why
					break
				}
			}
			target := suggestionTarget{Card: name, Code: issue.Code, Why: why, Candidates: []suggestionCard{}}
			for _, cand := range candidates {
				rec := cand.Record
				target.Candidates = append(target.Candidates, suggestionCard{Name: rec.Name, ManaCost: rec.ManaCost,
					CMC: rec.CMC, TypeLine: rec.TypeLine, OracleText: rec.OracleText,
					ColorIdentity: rec.ColorIdentity, Image: rec.ImageNormal, ArtCrop: rec.ImageArtCrop,
					EdhrecRank: rec.EdhrecRank, Score: cand.Score, Reasons: cand.Reasons})
			}
			targets = append(targets, target)
		}
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, map[string]any{"slug": slug, "pool_available": false, "targets": []any{}})
		return
	}
	if a.refuse(w, "suggestions", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"slug": slug, "pool_available": true, "targets": targets})
}

// typeParts is `service._type_parts`: a type line split into the part before
// the dash and the part after, front face only.
func typeParts(typeLine string) ([]string, []string) {
	front := strings.TrimSpace(strings.SplitN(typeLine, "//", 2)[0])
	for _, dash := range []string{"—", "–", " - "} {
		if before, after, ok := strings.Cut(front, dash); ok {
			return strings.Fields(before), strings.Fields(after)
		}
	}
	return strings.Fields(front), []string{}
}

// commanderDossier is `GET .../commander` -- `service.commander_dossier`:
// everything interesting the pool knows about a deck's commander, every
// part of it a query, not a recollection.
func (a *API) commanderDossier(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "commander", err) {
		return
	}
	empty := []kv{{"slug", d.Slug}, {"card", nil}, {"subtypes", []any{}}, {"other_cards", []any{}}, {"printings", nil}}
	if len(d.Commander) == 0 {
		raw, _ := marshalOrdered(empty)
		wire.Raw(w, http.StatusOK, raw)
		return
	}
	name := d.Commander[0]
	var body []kv
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		ctx := r.Context()
		found, err := c.GetCards(ctx, []string{name})
		if err != nil {
			return err
		}
		rec := found[name]
		if rec == nil {
			body = empty
			return nil
		}
		supertypes, subtypes := typeParts(rec.TypeLine)
		subtypeRows := []orderedMap{}
		for _, sub := range subtypes {
			pattern := "%" + sub + "%"
			var total, legends int64
			if err := c.DB().QueryRowContext(ctx, "SELECT count(*) FROM oracle_cards WHERE type_line ILIKE ?", pattern).Scan(&total); err != nil {
				return err
			}
			if err := c.DB().QueryRowContext(ctx, "SELECT count(*) FROM oracle_cards WHERE type_line ILIKE ? "+
				"AND type_line ILIKE '%Legendary%'", pattern).Scan(&legends); err != nil {
				return err
			}
			subtypeRows = append(subtypeRows, orderedMap([]kv{{"name", sub}, {"total", total}, {"legendary", legends}}))
		}
		character := strings.TrimSpace(strings.SplitN(name, ",", 2)[0])
		others := []orderedMap{}
		rows, err := c.DB().QueryContext(ctx, "SELECT name, type_line, mana_cost, image_normal, image_art_crop "+
			"FROM oracle_cards WHERE name ILIKE ? AND name <> ? ORDER BY edhrec_rank NULLS LAST LIMIT 6",
			"%"+character+"%", name)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v [5]any
			ptrs := []any{&v[0], &v[1], &v[2], &v[3], &v[4]}
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return err
			}
			others = append(others, orderedMap([]kv{{"name", pool.AsStringPtr(v[0])}, {"type_line", pool.AsStringPtr(v[1])},
				{"mana_cost", pool.AsStringPtr(v[2])}, {"image", pool.AsStringPtr(v[3])}, {"art_crop", pool.AsStringPtr(v[4])}}))
		}
		_ = rows.Close()
		var oracleID any
		_ = c.DB().QueryRowContext(ctx, "SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1", name).Scan(&oracleID)
		var count int64
		var first any
		if err := c.DB().QueryRowContext(ctx, "SELECT count(*), min(released_at) FROM printings "+
			"WHERE oracle_id = (SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1)", name).Scan(&count, &first); err != nil {
			return err
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
		artist, flavor := rec.Artist, rec.FlavorText
		if d.CommanderArt != "" {
			arts, err := chosenArts(ctx, c, []string{d.CommanderArt})
			if err != nil {
				return err
			}
			if chosen, ok := arts[d.CommanderArt]; ok {
				artist, flavor = chosen.Artist, chosen.FlavorText
			}
		}
		card := []kv{{"name", rec.Name}, {"oracle_id", pool.AsStringPtr(oracleID)}, {"mana_cost", rec.ManaCost},
			{"type_line", rec.TypeLine}, {"oracle_text", rec.OracleText}, {"flavor_text", flavor}, {"artist", artist},
			{"power", rec.Power}, {"toughness", rec.Toughness}, {"loyalty", rec.Loyalty}, {"image", rec.ImageNormal},
			{"art_crop", rec.ImageArtCrop}, {"color_identity", rec.ColorIdentity}, {"edhrec_rank", rec.EdhrecRank},
			{"game_changer", rec.GameChanger}}
		body = []kv{{"slug", d.Slug}, {"card", orderedMap(card)}, {"supertypes", supertypes}, {"subtypes", subtypeRows},
			{"other_cards", others}, {"printings", orderedMap([]kv{{"count", count}, {"first_released", firstReleased},
				{"first_set", firstSet}})}}
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		body = empty
		err = nil
	}
	if a.refuse(w, "commander", err) {
		return
	}
	raw, err := marshalOrdered(body)
	if a.refuse(w, "commander", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// commanderPrintings is `GET .../printings` -- `service.commander_printings`:
// every non-digital printing of the commander (or, with `?card=`, of any
// card the deck holds), newest first; a card the deck does not hold is a
// 422.
func (a *API) commanderPrintings(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "printings", err) {
		return
	}
	name, selected := "", d.CommanderArt
	if vals := r.URL.Query()["card"]; len(vals) > 0 {
		wanted := strings.ToLower(strings.TrimSpace(vals[len(vals)-1]))
		var entry *deck.CardEntry
		for _, list := range [][]deck.CardEntry{d.Cards, d.SwapBoard} {
			for i := range list {
				if strings.ToLower(list[i].Name) == wanted {
					entry = &list[i]
					break
				}
			}
			if entry != nil {
				break
			}
		}
		if entry == nil {
			wire.Detail(w, http.StatusUnprocessableEntity, wire.PyRepr(vals[len(vals)-1])+" is not in this deck")
			return
		}
		name, selected = entry.Name, entry.Art
	} else if len(d.Commander) > 0 {
		name = d.Commander[0]
	}
	printings := []orderedMap{}
	if name != "" {
		err = a.usePool(r.Context(), func(c *pool.Conn) error {
			rows, err := c.DB().QueryContext(r.Context(), `SELECT p.id, p.set_code, p.set_name, p.collector_number,
				p.rarity, p.released_at, p.promo, p.image_normal, p.price_usd
				FROM printings p
				WHERE p.oracle_id = (SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1)
				  AND p.digital IS NOT TRUE AND p.image_normal IS NOT NULL
				ORDER BY p.released_at DESC NULLS LAST, p.set_code, p.collector_number`, name)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var v [9]any
				ptrs := make([]any, len(v))
				for i := range v {
					ptrs[i] = &v[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return err
				}
				var released *string
				if t, ok := v[5].(time.Time); ok {
					s := t.Format("2006-01-02")
					released = &s
				}
				image := pool.AsStringPtr(v[7])
				id := pool.AsString(v[0])
				printings = append(printings, orderedMap([]kv{{"id", id}, {"set_code", strings.ToUpper(pool.AsString(v[1]))},
					{"set_name", pool.AsStringPtr(v[2])}, {"collector_number", pool.AsStringPtr(v[3])},
					{"rarity", pool.AsStringPtr(v[4])}, {"released_at", released}, {"promo", pool.AsBool(v[6])},
					{"image", image}, {"art_crop", pool.ArtCropFrom(image)}, {"price_usd", asFloatPtr(v[8])},
					{"selected", id == selected}}))
			}
			return rows.Err()
		})
		if errors.Is(err, pool.ErrNoPool) {
			err = nil
		}
		if a.refuse(w, "printings", err) {
			return
		}
	}
	raw, _ := marshalOrdered([]kv{{"slug", d.Slug}, {"commander", name}, {"selected", selected}, {"printings", printings}})
	wire.Raw(w, http.StatusOK, raw)
}

// deckLog is `GET .../log` -- `service.history_for`: what has been done to
// this deck, newest first (ADR 28); `source.Get` first is the whole
// authorisation check.
func (a *API) deckLog(w http.ResponseWriter, r *http.Request) {
	var errs []wire.ValidationError
	limit := boundedInt(r.URL.Query(), "limit", decklog.DefaultLimit, 1, 500, &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	if _, err := src.Get(r.Context(), slug); a.refuse(w, "log", err) {
		return
	}
	entries, err := decklog.Entries(r.Context(), a.appDB(), src.OwnerID(), slug, limit)
	if a.refuse(w, "log", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"slug": slug, "entries": entries})
}

// baselineState is `service._baseline_state`: the deck against the last
// build's snapshot -- current, different, or unknown.
func baselineState(d *deck.Deck, baseline string, present bool) string {
	if !present {
		return "unknown"
	}
	previous, err := deck.FromText(baseline, d.Slug)
	if err != nil {
		return "unknown"
	}
	if previous.SameAs(d) {
		return "current"
	}
	return "different"
}

// deckArtifacts is `GET .../artifacts` -- `service.list_artifacts`.
func (a *API) deckArtifacts(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "artifacts", err) {
		return
	}
	held, err := src.Artifacts(r.Context(), d.Slug)
	if a.refuse(w, "artifacts", err) {
		return
	}
	baseline, present, err := src.ReadBaseline(r.Context(), d.Slug)
	if a.refuse(w, "artifacts", err) {
		return
	}
	list := []orderedMap{}
	for _, art := range held {
		list = append(list, orderedMap([]kv{{"name", art.Name}, {"size", art.Size}, {"built_at", isoformat(art.BuiltAt)}}))
	}
	raw, _ := marshalOrdered([]kv{{"artifacts", list}, {"baseline", baselineState(d, baseline, present)},
		{"buildable", d.Stage != "draft"}, {"stage", d.Stage}})
	wire.Raw(w, http.StatusOK, raw)
}

// deckArtifact is `GET .../artifacts/{name}` -- one deliverable, verbatim.
func (a *API) deckArtifact(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug, name := r.PathValue("slug"), r.PathValue("name")
	if _, err := src.Get(r.Context(), slug); a.refuse(w, "artifact", err) {
		return
	}
	text, err := src.ReadArtifact(r.Context(), slug, name)
	if a.refuse(w, "artifact", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"name": name, "text": text})
}

// isoformat is `datetime.isoformat()` of a UTC instant with microseconds
// when they are non-zero -- what `Artifact.built_at.isoformat()` writes
// off a file's mtime (`datetime.fromtimestamp(st_mtime, UTC)`).
func isoformat(t time.Time) string {
	t = t.UTC()
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}

// challengeProgress is `GET /api/colors/progress` -- `service.challenge_progress`:
// which of the 32 slots the library has filled, the pool asked once.
func (a *API) challengeProgress(w http.ResponseWriter, r *http.Request) {
	// `Decks` is the instance's file tier (`deps.deck_source`), not the
	// caller's library: the 32 Deck Challenge is scored over the curated six.
	src := library.NewFileSource(a.decksDir, auth.ScopeFrom(r.Context()).IsAdmin)
	filled := map[string][]orderedMap{}
	havePool := false
	err := a.withPool(r.Context(), func(c *pool.Conn) error {
		if c == nil {
			return nil
		}
		havePool = true
		slugs, err := src.Slugs(r.Context())
		if err != nil {
			return err
		}
		type entry struct {
			slug string
			d    *deck.Deck
		}
		ordered := []entry{}
		wantedSet := map[string]bool{}
		for _, slug := range slugs {
			text, err := src.ReadText(r.Context(), slug)
			if err != nil {
				return err
			}
			d, err := deck.FromText(text, slug)
			if err != nil {
				return err
			}
			if len(d.Commander) == 0 {
				continue
			}
			ordered = append(ordered, entry{slug, d})
			for _, cmd := range d.Commander {
				wantedSet[cmd] = true
			}
		}
		wanted := make([]string, 0, len(wantedSet))
		for n := range wantedSet {
			wanted = append(wanted, n)
		}
		sort.Strings(wanted)
		found := map[string]*pool.CardRecord{}
		if len(wanted) > 0 {
			if found, err = c.GetCards(r.Context(), wanted); err != nil {
				return err
			}
		}
		for _, e := range ordered {
			identity := []string{}
			any := false
			for _, cmd := range e.d.Commander {
				if rec := found[cmd]; rec != nil {
					any = true
					identity = append(identity, rec.ColorIdentity...)
				}
			}
			if !any {
				continue
			}
			key := reference.KeyFor(identity)
			filled[key] = append(filled[key], orderedMap([]kv{{"slug", e.slug}, {"name", e.d.Name}}))
		}
		return nil
	})
	if a.refuse(w, "colors/progress", err) {
		return
	}
	slots := []orderedMap{}
	for _, combo := range reference.Colors().Combinations {
		decks := filled[combo.Key]
		if decks == nil {
			decks = []orderedMap{}
		}
		slots = append(slots, orderedMap([]kv{{"key", combo.Key}, {"name", combo.Name}, {"tier", combo.Tier}, {"decks", decks}}))
	}
	raw, _ := marshalOrdered([]kv{{"pool", havePool || len(filled) > 0}, {"filled", len(filled)},
		{"total", len(reference.Colors().Combinations)}, {"slots", slots}})
	wire.Raw(w, http.StatusOK, raw)
}

// ---- ordered JSON -------------------------------------------------------
//
// The wire keeps Python's key order where a reader might notice (the deck
// page's payload is read by people in DevTools as much as by the client),
// so the hand-built bodies here are ordered pairs rather than maps.

type kv struct {
	key   string
	value any
}

// orderedMap is a map that marshals in its own order.
type orderedMap []kv

func (o orderedMap) MarshalJSON() ([]byte, error) { return marshalOrdered(o) }

func marshalOrdered(pairs []kv) ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(p.key))
		b.WriteByte(':')
		raw, err := wire.Marshal(p.value)
		if err != nil {
			return nil, err
		}
		b.Write(raw)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}
