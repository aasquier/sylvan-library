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
	"flavor_text", "artist",
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
		jsonText(c["legalities"], "{}"), asStr(c["layout"]),
		jsonText(c["card_faces"], "[]"), truthy(c["reserved"]),
		asInt32(c["edhrec_rank"]), asDate(c["released_at"]),
		asStr(c["set"]), asStr(c["scryfall_uri"]),
		asStr(img["normal"]), asStr(img["art_crop"]),
		asStr(front(c, "power")), asStr(front(c, "toughness")),
		asStr(front(c, "loyalty")), asStr(front(c, "defense")),
		// Scryfall omits this on older rows rather than sending false, and
		// an absent game changer is not a game changer.
		truthy(c["game_changer"]),
		asStr(front(c, "flavor_text")), asStr(front(c, "artist")),
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

// jsonText stores the sub-document as text, `empty` when absent — Go's
// marshalling rather than json.dumps's ASCII escapes; see the package
// comment in refresh.go for why that difference is invisible.
func jsonText(v any, empty string) any {
	if v == nil {
		return empty
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return empty
	}
	return string(raw)
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
