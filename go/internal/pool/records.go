package pool

import (
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// CardRecord is one card as the pool knows it.
// Immutable by convention -- `GetCards` hands the same record to every caller
// that asks, which is what lets its cache share one.
// Pointer fields are the columns that may be NULL; a nil is an absent value
// and serialises as `null`.
type CardRecord struct {
	Name       string
	ManaCost   *string
	CMC        float64
	TypeLine   string
	OracleText string
	// Sorted, because every recorded serialisation writes the identity
	// sorted, and the set is kept sorted here so the
	// wire never depends on a map walk.
	ColorIdentity  []string
	ProducedMana   []string
	LegalCommander bool
	Reserved       bool
	EdhrecRank     *int
	ImageNormal    *string
	ImageArtCrop   *string
	Layout         string
	Keywords       []string
	Power          *string
	Toughness      *string
	Loyalty        *string
	Defense        *string
	GameChanger    bool
	FlavorText     *string
	Artist         *string
	// Faces is Scryfall's `card_faces`, in printed order, and empty for the
	// cards that have one face -- which is nearly every card.
	//
	// **The point of it is that a face may carry its own picture.** Scryfall
	// put `image_uris` on the *card* for the layouts that print two names on
	// one piece of cardboard -- an Adventure, a split card, a flip card -- and
	// on the *faces* for the ones where each face is a painting of its own: a
	// transforming card, a modal double-faced card. So the presence of
	// [CardFace.ImageNormal] is not a detail of the encoding, it is Scryfall
	// saying which kind of card this is, and it is the only place that
	// distinction is written down as a fact rather than as a list of layout
	// names somebody has to keep.
	//
	// Measured over this pool on 2026-08-29: six layouts carry an `A // B`
	// name -- transform (401), adventure (170), split (137), modal_dfc (100),
	// prepare (55), flip (26) -- and exactly two of them, transform and
	// modal_dfc, put an `image_uris` on each face. `meld` carries no faces at
	// all and no `//` in its name, because each meld part is its own record.
	Faces []CardFace
}

// CardFace is one face of a card that has more than one.
//
// **Every field is the face's own answer to a question the card also answers**,
// which is the whole reason it exists: `Agadeem's Awakening // Agadeem, the
// Undercrypt` is a Sorcery *and* a Land, painted twice, and a room holding up
// one picture has to be able to say which. [CardRecord.ImageNormal] is the
// front for such a card (`images` in rows.go falls back to the first face), so
// it is the right answer for the card and the wrong one for the back half.
//
// Pointer fields are absent values rather than empty ones, for
// [CardRecord]'s reason: a face with no picture of its own is a face printed
// on the card's, and that is not the same fact as a face whose picture we
// failed to record.
type CardFace struct {
	Name     string
	TypeLine string
	// ImageNormal and ImageArtCrop are this face's own picture, and are nil
	// for a face printed on the card's one picture.
	ImageNormal  *string
	ImageArtCrop *string
}

// HasColor is `color in rec.color_identity`.
func (r *CardRecord) HasColor(code string) bool {
	for _, c := range r.ColorIdentity {
		if c == code {
			return true
		}
	}
	return false
}

// FrontTypeLine is the type line's front face.
func (r *CardRecord) FrontTypeLine() string {
	front, _, _ := strings.Cut(r.TypeLine, " // ")
	return front
}

var (
	// reminderText is anything in parentheses, which in Magic is always
	// reminder text and never a rule.
	reminderText = regexp.MustCompile(`\([^)]*\)`)
	// manaAbility is a mana ability written in a card's own rules text. "Add"
	// is the game's one verb for producing mana and is used for nothing else
	// — counters are "put", cards are "draw" — so the word plus what it is
	// adding is the whole test.
	manaAbility = regexp.MustCompile(
		`(?i)\badd\b\s*(\{|one mana|two mana|three mana|X mana|mana of|an amount)`)
)

// MakesMana is whether the card taps for mana **itself**.
//
// **Not `len(ProducedMana) > 0`, and the difference is not academic.**
// Scryfall's `produced_mana` counts mana produced by *tokens the card makes*,
// read out of the token's reminder text. So Nuka-Cola Vending Machine reports
// five colours: it creates Food, sacrificing a Food creates a Treasure, and a
// Treasure's reminder text says "Add one mana of any color". It is a Food
// engine that has never made a mana in its life. Smothering Tithe reports five
// for the same reason.
//
// That was found by looking at a real board — the Vending Machine standing in
// the land row next to the Swamps, which is a claim about the deck's mana that
// its card does not make.
//
// So the test is the card's own rules text with reminder text stripped, with
// `produced_mana` kept as the cheap first pass: a card that produces no mana by
// any reading cannot produce mana by this one.
func (r *CardRecord) MakesMana() bool {
	if len(r.ProducedMana) == 0 {
		return false
	}
	return manaAbility.MatchString(
		reminderText.ReplaceAllString(r.OracleText, " "))
}

// IsCreature reads the front face, matching `power`.
func (r *CardRecord) IsCreature() bool {
	return strings.Contains(r.FrontTypeLine(), "Creature")
}

// IsLand is `CardRecord.is_land`: a land you can put onto the battlefield as
// a land drop -- the front face says Land, or the card is a modal DFC with a
// land on a back face. A transforming permanent whose back is a land is not
// one: you cast the front, and the back arrives only by flipping.
func (r *CardRecord) IsLand() bool {
	faces := strings.Split(r.TypeLine, " // ")
	if strings.Contains(faces[0], "Land") {
		return true
	}
	if r.Layout != "modal_dfc" {
		return false
	}
	for _, f := range faces[1:] {
		if strings.Contains(f, "Land") {
			return true
		}
	}
	return false
}

// readColumns is the pool's read column list, in the order `toRecord`
// reads.
var readColumns = []string{
	"name", "mana_cost", "cmc", "type_line", "oracle_text", "color_identity",
	"produced_mana", "reserved", "legalities", "edhrec_rank",
	"image_normal", "image_art_crop", "layout", "keywords",
	"power", "toughness", "loyalty", "defense", "game_changer",
	"flavor_text", "artist", "card_faces",
}

// selectClause is `_select`: the SELECT, with any column this pool lacks
// filled as NULL, so a schema change degrades to "we do not know" on an old
// pool rather than failing to bind.
func (c *Conn) selectClause(ctx context.Context) (string, error) {
	have, err := c.Columns(ctx, "oracle_cards")
	if err != nil {
		return "", err
	}
	cols := make([]string, 0, len(readColumns))
	for _, col := range readColumns {
		if have[col] {
			cols = append(cols, col)
		} else {
			cols = append(cols, "NULL AS "+col)
		}
	}
	return "SELECT " + strings.Join(cols, ", ") + " FROM oracle_cards", nil
}

// scanRecord reads one row of `selectClause` into a record.
func scanRecord(rows *sql.Rows) (*CardRecord, error) {
	vals := make([]any, len(readColumns))
	ptrs := make([]any, len(readColumns))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	return toRecord(vals), nil
}

// toRecord is `_to_record`, over the driver's Go values: VARCHAR -> string,
// VARCHAR[] -> []any of string, JSON -> map (decoded by the driver), BOOLEAN
// -> bool, INTEGER -> int32, DOUBLE -> float64, NULL -> nil.
func toRecord(v []any) *CardRecord {
	rec := &CardRecord{
		Name:          asString(v[0]),
		ManaCost:      asStringPtr(v[1]),
		CMC:           asFloat(v[2]),
		TypeLine:      asString(v[3]),
		OracleText:    asString(v[4]),
		ColorIdentity: asStrings(v[5]),
		ProducedMana:  asStrings(v[6]),
		Reserved:      asBool(v[7]),
		EdhrecRank:    asIntPtr(v[9]),
		ImageNormal:   asStringPtr(v[10]),
		ImageArtCrop:  asStringPtr(v[11]),
		Layout:        asString(v[12]),
		Keywords:      asStrings(v[13]),
		Power:         asStringPtr(v[14]),
		Toughness:     asStringPtr(v[15]),
		Loyalty:       asStringPtr(v[16]),
		Defense:       asStringPtr(v[17]),
		GameChanger:   asBool(v[18]),
		FlavorText:    asStringPtr(v[19]),
		Artist:        asStringPtr(v[20]),
		Faces:         cardFaces(v[21]),
	}
	if rec.Layout == "" {
		rec.Layout = "normal"
	}
	sort.Strings(rec.ColorIdentity)
	rec.LegalCommander = legalities(v[8])["commander"] == "legal"
	return rec
}

// legalities reads the JSON column however the driver hands it over: the
// decoded map for a JSON-typed column, or text for a pool that stored it as
// VARCHAR.
func legalities(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case string:
		var out map[string]any
		if err := json.Unmarshal([]byte(t), &out); err == nil {
			return out
		}
	case []byte:
		var out map[string]any
		if err := json.Unmarshal(t, &out); err == nil {
			return out
		}
	}
	return map[string]any{}
}

// cardFaces reads the `card_faces` column however the driver hands it over,
// on `legalities`' terms one column across: the decoded list for a JSON-typed
// column, or text for a pool that stored it as VARCHAR.
//
// **A face with no name is dropped, and dropping one drops them all.** The one
// thing every caller does with this list is index into it alongside the names
// the card's own `A // B` splits into, so a list that is *short* is worse than
// no list: it is an index that silently answers for the wrong half. Nothing
// but a malformed record can produce one, which is exactly when a confident
// wrong answer is least wanted.
func cardFaces(v any) []CardFace {
	var raw []any
	switch t := v.(type) {
	case []any:
		raw = t
	case string:
		if json.Unmarshal([]byte(t), &raw) != nil {
			return nil
		}
	case []byte:
		if json.Unmarshal(t, &raw) != nil {
			return nil
		}
	default:
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	faces := make([]CardFace, 0, len(raw))
	for _, item := range raw {
		face, _ := item.(map[string]any)
		name := asString(face["name"])
		if name == "" {
			return nil
		}
		img, _ := face["image_uris"].(map[string]any)
		faces = append(faces, CardFace{
			Name:         name,
			TypeLine:     asString(face["type_line"]),
			ImageNormal:  asStringPtr(img["normal"]),
			ImageArtCrop: asStringPtr(img["art_crop"]),
		})
	}
	return faces
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asStringPtr(v any) *string {
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return 0
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asIntPtr(v any) *int {
	var n int
	switch t := v.(type) {
	case int32:
		n = int(t)
	case int64:
		n = int(t)
	case int:
		n = t
	case float64:
		n = int(t)
	default:
		return nil
	}
	return &n
}

// asStrings reads a VARCHAR[] -- `[]any` of string from the driver -- as a
// string slice, never nil: an empty list is `[]` on the wire.
func asStrings(v any) []string {
	out := []string{}
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	}
	return out
}

// GetCards is many cards at once, case-insensitively,
// a double-faced card by either face name as well as by its combined
// `Front // Back` name, and the record returned is always the WHOLE card so
// `ColorIdentity` covers every face (Ajani, Nacatl Pariah looked up by its
// white front still reports {R}{W}). Missing names are simply absent from
// the result; callers handle that, loudly. Exact full-name matches win; a
// face-name match only fills a gap. Memoised per open on the exact name list.
func (c *Conn) GetCards(ctx context.Context, names []string) (map[string]*CardRecord, error) {
	if len(names) == 0 {
		return map[string]*CardRecord{}, nil
	}
	key := strings.Join(names, "\x00")
	if cache := c.cache(); cache != nil {
		hit, ok := cache.get(key)
		c.pool.note(MemoCards, ok)
		if ok {
			return hit, nil
		}
	}
	lowered := make([]string, len(names))
	for i, n := range names {
		lowered[i] = strings.ToLower(n)
	}
	sel, err := c.selectClause(ctx)
	if err != nil {
		return nil, err
	}
	// The CTE form settled on 2026-08-19: one list parameter, three
	// hash semi-joins, and the face split gated on the cards that have one.
	rows, err := c.db.QueryContext(ctx,
		`WITH wanted(w) AS (SELECT unnest(?::VARCHAR[])) `+sel+
			` WHERE lower(name) IN (SELECT w FROM wanted)
			   OR (contains(name, ' // ') AND (
			          lower(split_part(name, ' // ', 1)) IN (SELECT w FROM wanted)
			       OR lower(split_part(name, ' // ', 2)) IN (SELECT w FROM wanted)))`,
		lowered)
	if err != nil {
		return nil, fmt.Errorf("get_cards: %w", err)
	}
	defer rows.Close()
	byLower := map[string]*CardRecord{}
	faces := map[string]*CardRecord{}
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("get_cards: %w", err)
		}
		low := strings.ToLower(rec.Name)
		byLower[low] = rec
		for _, face := range strings.Split(low, " // ") {
			if _, seen := faces[face]; !seen {
				faces[face] = rec
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get_cards: %w", err)
	}
	out := make(map[string]*CardRecord, len(names))
	for i, name := range names {
		found := byLower[lowered[i]]
		if found == nil {
			found = faces[lowered[i]]
		}
		if found != nil {
			out[name] = found
		}
	}
	if cache := c.cache(); cache != nil {
		cache.put(key, out)
	}
	return copyOf(out), nil
}

// Search is an ad-hoc WHERE over `oracle_cards`, with
// an ordering (without one a LIMIT is an arbitrary slice) and an offset that
// only means anything with one. The shape is interpolated and the values are
// bound, and the line holds: `where` and `orderBy` are
// the caller's own SQL, never a request's.
func (c *Conn) Search(ctx context.Context, where string, args []any, limit int, orderBy string, offset int) ([]*CardRecord, error) {
	sel, err := c.selectClause(ctx)
	if err != nil {
		return nil, err
	}
	q := sel + " WHERE " + where
	if orderBy != "" {
		q += " ORDER BY " + orderBy
	}
	q += fmt.Sprintf(" LIMIT %d", limit)
	if offset > 0 {
		q += fmt.Sprintf(" OFFSET %d", offset)
	}
	rows, err := c.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	out := []*CardRecord{}
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ArtCropFrom is the art_crop URL for a
// printing whose normal URL we have -- Scryfall's image URLs differ only in
// the size segment. Anything not of that shape is nil rather than a guess.
func ArtCropFrom(imageNormal *string) *string {
	if imageNormal == nil || !strings.Contains(*imageNormal, "/normal/") {
		return nil
	}
	crop := strings.Replace(*imageNormal, "/normal/", "/art_crop/", 1)
	return &crop
}

// cache is the per-open memo, or nil when the pool has been closed under us
// (a Use that outlived a Close, which only a shutdown does).
func (c *Conn) cache() *cardCache {
	c.pool.mu.Lock()
	defer c.pool.mu.Unlock()
	if c.pool.db != c.db {
		return nil
	}
	return c.pool.cards
}

func copyOf(m map[string]*CardRecord) map[string]*CardRecord {
	out := make(map[string]*CardRecord, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// cardCache is the last sixteen `GetCards` answers,
// keyed on the exact name list, least-recently-used first out. The shelf asks
// the same few hundred names on every page load until a deck is edited, and a
// lookup was a full scan of 35,390 rows (`lower(name)` cannot use the index).
// Per open, so the pool's stamp is the key's other half for free: a refresh
// re-opens, and the memo starts empty.
type cardCache struct {
	mu    sync.Mutex
	max   int
	order *list.List
	items map[string]*list.Element
}

type cacheEntry struct {
	key  string
	rows map[string]*CardRecord
}

func newCardCache() *cardCache {
	return &cardCache{max: 16, order: list.New(), items: map[string]*list.Element{}}
}

func (c *cardCache) get(key string) (map[string]*CardRecord, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return copyOf(el.Value.(*cacheEntry).rows), true
}

func (c *cardCache) put(key string, rows map[string]*CardRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).rows = rows
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&cacheEntry{key: key, rows: rows})
	for c.order.Len() > c.max {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*cacheEntry).key)
	}
}

// The scan helpers, exported for the handlers whose queries are their own
// (`/api/cards/search` selects a shape `toRecord` does not read) -- one
// reading of the driver's Go values, kept here with the one that knows them.

// AsString reads a VARCHAR, "" for NULL.
func AsString(v any) string { return asString(v) }

// AsStringPtr reads a nullable VARCHAR.
func AsStringPtr(v any) *string { return asStringPtr(v) }

// AsFloat reads a DOUBLE (or an integer column), 0 for NULL.
func AsFloat(v any) float64 { return asFloat(v) }

// AsBool reads a BOOLEAN, false for NULL.
func AsBool(v any) bool { return asBool(v) }

// AsIntPtr reads a nullable INTEGER.
func AsIntPtr(v any) *int { return asIntPtr(v) }

// AsStrings reads a VARCHAR[], never nil.
func AsStrings(v any) []string { return asStrings(v) }
