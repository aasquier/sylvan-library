package pool

import (
	"encoding/json"
	"time"
)

// OracleColumns and PrintingColumns are `_ORACLE_COLUMNS` and
// `_PRINTING_COLUMNS`: named rather than positional, so adding a column is
// one edit here instead of a silent off-by-one between the row and the
// placeholder count — the same argument, inherited with the lists.
var OracleColumns = []string{
	"oracle_id", "name", "mana_cost", "cmc", "type_line", "oracle_text",
	"colors", "color_identity", "keywords", "produced_mana", "legalities",
	"layout", "card_faces", "reserved", "edhrec_rank", "released_at",
	"set_code", "scryfall_uri", "image_normal", "image_art_crop",
	"power", "toughness", "loyalty", "defense", "game_changer",
	"flavor_text", "artist", "all_parts",
}

var PrintingColumns = []string{
	"id", "oracle_id", "name", "set_code", "set_name", "collector_number",
	"rarity", "released_at", "digital", "promo", "finishes", "image_normal",
	"price_usd", "price_usd_foil", "price_eur", "tcg_product_id",
	"flavor_text", "artist",
}

// front reads a field that Scryfall puts on the *faces* of a double-faced
// card rather than on the card itself (`_FRONT_FACE_FIELDS`: mana_cost,
// power, toughness, loyalty, defense, flavor_text, artist). Reading only the
// top level left `mana_cost` NULL for all 501 DFCs and the compiler handed
// every one of them to the simulator as a free spell — Etali cast on turn
// one. The front face is the face you cast from hand, which makes it the
// right answer rather than a convenient one.
func front(c map[string]any, field string) any {
	if value := c[field]; value != nil {
		return value
	}
	faces, _ := c["card_faces"].([]any)
	if len(faces) == 0 {
		return nil
	}
	first, _ := faces[0].(map[string]any)
	return first[field]
}

func images(c map[string]any) map[string]any {
	img, _ := c["image_uris"].(map[string]any)
	if len(img) > 0 {
		return img
	}
	if faces, _ := c["card_faces"].([]any); len(faces) > 0 {
		first, _ := faces[0].(map[string]any)
		img, _ = first["image_uris"].(map[string]any)
	}
	return img
}

// OracleRow is `_oracle_row`, over a UseNumber-decoded card, as the values
// the Appender writes in OracleColumns order.
func OracleRow(c map[string]any) []any {
	img := images(c)
	return []any{
		asStr(c["oracle_id"]), asStr(c["name"]), asStr(front(c, "mana_cost")),
		asDouble(c["cmc"]), asStr(c["type_line"]), asStr(c["oracle_text"]),
		asList(c["colors"]), asList(c["color_identity"]),
		asList(c["keywords"]), asList(c["produced_mana"]),
		// The absent values are decoded documents too, never the text `"{}"`
		// and `"[]"` — see [jsonText] for what a string does to a JSON column.
		jsonText(c["legalities"], map[string]any{}), asStr(c["layout"]),
		jsonText(c["card_faces"], []any{}), truthy(c["reserved"]),
		asInt32(c["edhrec_rank"]), asDate(c["released_at"]),
		asStr(c["set"]), asStr(c["scryfall_uri"]),
		asStr(img["normal"]), asStr(img["art_crop"]),
		asStr(front(c, "power")), asStr(front(c, "toughness")),
		asStr(front(c, "loyalty")), asStr(front(c, "defense")),
		// Scryfall omits this on older rows rather than sending false, and
		// an absent game changer is not a game changer.
		truthy(c["game_changer"]),
		asStr(front(c, "flavor_text")), asStr(front(c, "artist")),
		// Top level, never `front`: what a card relates to belongs to the
		// card, not to the face you cast it from. A transforming
		// planeswalker's tokens are listed once, for the whole card.
		jsonOrNull(c["all_parts"]),
	}
}

// PrintingRow is `_printing_row`.
func PrintingRow(c map[string]any) []any {
	prices, _ := c["prices"].(map[string]any)
	img := images(c)
	price := func(key string) any {
		// `float(v) if v else None`: an empty string — and Scryfall's
		// literal "0.00" is not one, but an absent or empty value is —
		// stays NULL rather than becoming a zero price.
		v, _ := prices[key].(string)
		if v == "" {
			return nil
		}
		return asDouble(json.Number(v))
	}
	return []any{
		asStr(c["id"]), asStr(c["oracle_id"]), asStr(c["name"]), asStr(c["set"]),
		asStr(c["set_name"]), asStr(c["collector_number"]), asStr(c["rarity"]),
		asDate(c["released_at"]), truthy(c["digital"]), truthy(c["promo"]),
		asList(c["finishes"]), asStr(img["normal"]),
		price("usd"), price("usd_foil"), price("eur"),
		tcgID(c["tcgplayer_id"]),
		// The same front-face fallback as OracleRow, for the same reason: a
		// double-faced printing puts both on its faces, and reading only the
		// top level records a painting as unattributed.
		asStr(front(c, "flavor_text")), asStr(front(c, "artist")),
	}
}

// SkipOracleLayout and SkipPrinting are the loaders' filters, exported so
// the command and the corpus test share one spelling.
func SkipOracleLayout(c map[string]any) bool {
	layout, _ := c["layout"].(string)
	return layout == "art_series" || layout == "token" || layout == "double_faced_token"
}

func SkipPrinting(c map[string]any) bool { return truthy(c["digital"]) }

func asStr(v any) any {
	if s, ok := v.(string); ok {
		return s
	}
	return nil
}

func asDouble(v any) any {
	switch value := v.(type) {
	case json.Number:
		f, err := value.Float64()
		if err != nil {
			return nil
		}
		return f
	case float64:
		return value
	}
	return nil
}

func asInt32(v any) any {
	switch value := v.(type) {
	case json.Number:
		n, err := value.Int64()
		if err != nil {
			return nil
		}
		return int32(n) // #nosec G115 -- an EDHREC rank fits comfortably
	case float64:
		return int32(value)
	}
	return nil
}

func asList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// jsonText hands the sub-document to the Appender as the decoded value it
// already is, `empty` when absent.
//
// **It must not be marshalled to a string first, and that is not a style
// preference — it is the bug that emptied the library.** A JSON column takes
// a *value*; give the Appender a Go `string` and DuckDB stores a JSON string
// whose content happens to be JSON, so `legalities` lands as
// `"{\"commander\":\"legal\"}"` rather than `{"commander":"legal"}`. Every
// read through `json_extract_string` then returns NULL, which is the WHERE
// clause on **every** card query in the app: the card finder, the card
// search, the Wheel, the colour rooms. All of them answered "no such card"
// against a pool holding all 35,393 of them, because `count(*)` has no
// predicate and so the pool reported itself perfectly healthy.
//
// Three things hid it for a week and each is worth a sentence:
//
//   - The prepared-statement loader this replaced **parsed** the text on the
//     way in (`?::JSON` does), so the encoding was right for as long as the
//     slow path existed and became wrong the moment the Appender arrived.
//   - `pooltest` still binds `?::JSON`, so the whole suite writes its
//     fixtures down the *working* path and cannot see this. A pool loaded
//     through [LoadOracle] is the only fixture that can, which is what
//     `cards.TestAPoolWrittenByTheRealLoaderAnswersTheCardQueries` is for.
//   - The comment that used to stand here said the difference was Go's
//     marshalling "rather than json.dumps's ASCII escapes", and invisible.
//     It was written looking straight at `"{\"commander\":...}"` — the
//     double-encoding itself — and read the backslashes as escaping.
//
// The decoded value is safe to hand over exactly as `IterCards` produced it:
// `UseNumber` leaves numbers as [json.Number], and the Appender writes those
// as numbers, so a face's `"cmc":3.0` stays `3.0` and does not become
// `"3.0"`. Verified against a heterogeneous `card_faces` — nested objects,
// nulls, arrays, and a key one face carries and the other does not.
//
// The marshal survives as a *validity check* only: its bytes are thrown
// away. Values off `encoding/json` are always representable, so this is the
// defensive branch it always was, and a document that somehow is not
// representable becomes `empty` rather than failing the whole refresh.
func jsonText(v any, empty any) any {
	if v == nil {
		return empty
	}
	if _, err := json.Marshal(v); err != nil {
		return empty
	}
	return v
}

// jsonOrNull is [jsonText]'s sibling for a document most cards do not have:
// absent stays NULL rather than becoming an empty list.
//
// `card_faces` gets `[]` because "this card has no faces" is a statement about
// every card; `all_parts` gets NULL because relating to nothing is the normal
// case and NULL is the cheaper and truer spelling of it. It also leaves the
// frozen refresh corpus comparing equal without being regenerated: none of
// those recorded cards carries `all_parts`, and NULL is what they always
// said.
// The same rule as [jsonText] about not stringifying it on the way in.
func jsonOrNull(v any) any {
	if v == nil {
		return nil
	}
	if _, err := json.Marshal(v); err != nil {
		return nil
	}
	return v
}

func truthy(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func asDate(v any) any {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return t
}

// tcgID is `str(c.get("tcgplayer_id") or "") or None`: an integer coerced to
// its decimal text, anything falsy to NULL.
func tcgID(v any) any {
	switch value := v.(type) {
	case json.Number:
		if value.String() == "" || value.String() == "0" {
			return nil
		}
		return value.String()
	case string:
		if value == "" {
			return nil
		}
		return value
	}
	return nil
}
