package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// searchCard is one row of `service.search_cards`' answer, in its key order.
type searchCard struct {
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

// search is `GET /api/cards/search` -- `service.search_cards`, the 'deep
// hits from the whole history' tool. `identity` is a subset filter;
// `identity_exact` flips it for the create flow; `commanders_only` narrows
// in SQL and then decides in Go with the same `CanBeCommander` the gate
// uses -- one implementation of the rule, not two.
func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []wire.ValidationError
	text := last(q, "q")
	identity := last(q, "identity")
	typeLine := last(q, "type_line")
	cmcMax, haveCMC := optionalFloat(q, "cmc_max", &errs)
	priceMax, havePrice := optionalFloat(q, "price_max", &errs)
	sortBy := last(q, "sort")
	if sortBy == "" {
		sortBy = "edhrec"
	}
	limit := boundedInt(q, "limit", 60, 1, 200, &errs)
	identityExact := flag(q, "identity_exact", &errs)
	commandersOnly := flag(q, "commanders_only", &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	type answer struct {
		Cards []searchCard `json:"cards"`
		Total int          `json:"total"`
	}
	out := answer{Cards: []searchCard{}}
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
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
				quotedList(allowed)))
			if identityExact {
				// Subset plus the right size is set equality, and it lets the
				// colourless slot work: an empty identity with length 0.
				where = append(where, fmt.Sprintf("len(color_identity) = %d", len(allowed)))
			}
		}
		// `contains(lower(col), ?)` rather than ILIKE, as Python: the same
		// question asked cheaply, and `%` and `_` stop being wildcards.
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
		sql := `
            SELECT o.name, o.mana_cost, o.cmc, o.type_line, o.oracle_text,
                   o.color_identity, o.edhrec_rank, o.image_normal,
                   o.image_art_crop, o.reserved,
                   (SELECT min(p.price_usd) FROM printings p
                     WHERE p.oracle_id = o.oracle_id AND p.price_usd IS NOT NULL) AS usd
            FROM oracle_cards o
            WHERE ` + strings.Join(where, " AND ") + `
            ORDER BY ` + order + `
            LIMIT ?`
		rows, err := c.DB().QueryContext(r.Context(), sql, append(params, limit)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		found := []searchCard{}
		for rows.Next() {
			var v [11]any
			ptrs := make([]any, len(v))
			for i := range v {
				ptrs[i] = &v[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			card := searchCard{
				Name: pool.AsString(v[0]), ManaCost: pool.AsStringPtr(v[1]), CMC: pool.AsFloat(v[2]),
				TypeLine: pool.AsStringPtr(v[3]), OracleText: pool.AsStringPtr(v[4]),
				ColorIdentity: pool.AsStrings(v[5]), EdhrecRank: pool.AsIntPtr(v[6]),
				Image: pool.AsStringPtr(v[7]), ArtCrop: pool.AsStringPtr(v[8]), Reserved: pool.AsBool(v[9]),
				PriceUSD: asFloatPtr(v[10]),
			}
			sort.Strings(card.ColorIdentity)
			found = append(found, card)
		}
		if err := rows.Err(); err != nil {
			return err
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
			keep, err := c.GetCards(r.Context(), names)
			if err != nil {
				return err
			}
			kept := found[:0]
			for _, card := range found {
				if rec := keep[card.Name]; rec != nil && gate.CanBeCommander(rec, false) {
					kept = append(kept, card)
				}
			}
			found = kept
		}
		out.Cards = found
		out.Total = len(found)
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, map[string]any{"cards": []any{}, "total": 0, "message": noPoolMessage})
		return
	}
	if err != nil {
		a.fail(w, "search", err)
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// identifiedCard is `service._identified_card`: the compact card a camera
// review list renders -- forty of them, each with a picture, on a phone.
type identifiedCard struct {
	Name          string   `json:"name"`
	ManaCost      *string  `json:"mana_cost"`
	TypeLine      string   `json:"type_line"`
	ColorIdentity []string `json:"color_identity"`
	Image         *string  `json:"image"`
	ArtCrop       *string  `json:"art_crop"`
}

func identified(rec *pool.CardRecord) *identifiedCard {
	crop := rec.ImageArtCrop
	if crop == nil {
		crop = pool.ArtCropFrom(rec.ImageNormal)
	}
	return &identifiedCard{Name: rec.Name, ManaCost: rec.ManaCost, TypeLine: rec.TypeLine,
		ColorIdentity: rec.ColorIdentity, Image: rec.ImageNormal, ArtCrop: crop}
}

type identifyCandidate struct {
	identifiedCard
	Score float64 `json:"score"`
}

type identifyReading struct {
	Via        string              `json:"via"`
	Resolved   *identifiedCard     `json:"resolved"`
	Candidates []identifyCandidate `json:"candidates"`
}

// identify is `POST /api/cards/identify` -- `service.identify_cards`, the
// serving half of the camera reader: hydration, and the counts, with
// `resolved` and `offered` counted apart because only a corner lookup
// resolves; a title is a shortlist somebody still has to choose from. A name
// that no longer resolves is dropped and counted, which can only mean the
// pool moved under a reading.
func (a *API) identify(w http.ResponseWriter, r *http.Request) {
	payload, ok := readObject(w, r)
	if !ok {
		return
	}
	raw, isList := payload["sightings"].([]any)
	if !isList {
		wire.Detail(w, http.StatusUnprocessableEntity, "sightings must be a list of {set, number, title}")
		return
	}
	sightings := []cards.Sighting{}
	for _, item := range raw {
		s, isObject := item.(map[string]any)
		if !isObject {
			continue
		}
		sightings = append(sightings, cards.Sighting{
			SetCode: stringOf(s["set"]), CollectorNumber: stringOf(s["number"]),
			Title: stringOf(s["title"]), Corner: stringOf(s["corner"]),
		})
		if len(sightings) == cards.MaxSightings {
			break
		}
	}
	type answer struct {
		Readings []identifyReading `json:"readings"`
		Resolved int               `json:"resolved"`
		Offered  int               `json:"offered"`
		Unread   int               `json:"unread"`
		Dropped  int               `json:"dropped"`
	}
	out := answer{Readings: []identifyReading{}}
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		readings, err := cards.Read(r.Context(), c, sightings)
		if err != nil {
			return err
		}
		wanted := map[string]bool{}
		for _, rd := range readings {
			if rd.Resolved != "" {
				wanted[rd.Resolved] = true
			}
			for _, cand := range rd.Candidates {
				wanted[cand.Name] = true
			}
		}
		names := make([]string, 0, len(wanted))
		for n := range wanted {
			names = append(names, n)
		}
		sort.Strings(names)
		records, err := c.GetCards(r.Context(), names)
		if err != nil {
			return err
		}
		for _, rd := range readings {
			var resolved *identifiedCard
			if rd.Resolved != "" {
				if rec := records[rd.Resolved]; rec != nil {
					resolved = identified(rec)
				} else {
					out.Dropped++
				}
			}
			candidates := []identifyCandidate{}
			for _, cand := range rd.Candidates {
				rec := records[cand.Name]
				if rec == nil {
					out.Dropped++
					continue
				}
				candidates = append(candidates, identifyCandidate{
					identifiedCard: *identified(rec), Score: math.Round(cand.Score*10000) / 10000})
			}
			// Recomputed rather than passed through: a reading whose names all
			// dropped is a reading of nothing, whichever tier found them.
			via := "nothing"
			switch {
			case resolved != nil:
				via = "printing"
			case len(candidates) > 0:
				via = "title"
			}
			out.Readings = append(out.Readings, identifyReading{Via: via, Resolved: resolved, Candidates: candidates})
		}
		for _, rd := range out.Readings {
			switch {
			case rd.Resolved != nil:
				out.Resolved++
			case len(rd.Candidates) > 0:
				out.Offered++
			default:
				out.Unread++
			}
		}
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, map[string]any{"readings": []any{}, "resolved": 0, "offered": 0,
			"unread": 0, "dropped": 0, "message": noPoolMessage})
		return
	}
	if err != nil {
		a.fail(w, "identify", err)
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// --------------------------------------------------------- request helpers

// last is what Starlette's QueryParams.get returns for a repeated name: the
// last value. Go's Values.Get would return the first.
func last(q map[string][]string, name string) string {
	vals := q[name]
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// optionalFloat is `name: float | None = None`: absent is (0, false);
// present and unparseable is pydantic's float_parsing error.
func optionalFloat(q map[string][]string, name string, errs *[]wire.ValidationError) (float64, bool) {
	vals := q[name]
	if len(vals) == 0 {
		return 0, false
	}
	raw := vals[len(vals)-1]
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		*errs = append(*errs, wire.FloatParsing("query", name, raw))
		return 0, false
	}
	return f, true
}

// boundedInt is `Query(default, ge=lo, le=hi)` on an int.
func boundedInt(q map[string][]string, name string, fallback, lo, hi int, errs *[]wire.ValidationError) int {
	vals := q[name]
	if len(vals) == 0 {
		return fallback
	}
	raw := vals[len(vals)-1]
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		*errs = append(*errs, wire.IntParsing("query", name, raw))
		return fallback
	}
	if n < lo {
		*errs = append(*errs, wire.GreaterThanEqual("query", name, raw, lo))
		return fallback
	}
	if n > hi {
		*errs = append(*errs, wire.LessThanEqual("query", name, raw, hi))
		return fallback
	}
	return n
}

// flag is a `bool = False` query parameter, read with pydantic's spellings.
func flag(q map[string][]string, name string, errs *[]wire.ValidationError) bool {
	vals := q[name]
	if len(vals) == 0 {
		return false
	}
	raw := vals[len(vals)-1]
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	}
	*errs = append(*errs, wire.BoolParsing("query", name, raw))
	return false
}

// readObject reads a JSON object body the way a `payload: dict[str, Any]`
// parameter does: a body that is not JSON, or not an object, is FastAPI's
// own 422 rather than the handler's.
func readObject(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		wire.Detail(w, http.StatusBadRequest, "could not read the request body")
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		var syn *json.SyntaxError
		offset := 1
		if errors.As(err, &syn) {
			offset = int(syn.Offset)
		}
		wire.Unprocessable(w, wire.ValidationError{Type: "json_invalid", Loc: []any{"body", offset},
			Msg: "JSON decode error", Input: map[string]any{}, Ctx: map[string]any{"error": err.Error()}})
		return nil, false
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		wire.Unprocessable(w, wire.ValidationError{Type: "dict_type", Loc: []any{"body"},
			Msg: "Input should be a valid dictionary", Input: parsed})
		return nil, false
	}
	return obj, true
}

// stringOf is `payload.get(k) or None` for a field that is meant to be text:
// a string as it is, anything else -- a number, a null, a list -- as absent.
func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func asFloatPtr(v any) *float64 {
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

func quotedList(items []string) string {
	if len(items) == 0 {
		return "''"
	}
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "'" + s + "'"
	}
	return strings.Join(quoted, ", ")
}

// fail is a handler's last line: the query failed for a reason that is not
// "no pool", which is a 500 in the envelope and a log line with the cause.
func (a *API) fail(w http.ResponseWriter, where string, err error) {
	a.log.Error("the pool query failed", "route", where, "error", err)
	wire.Detail(w, http.StatusInternalServerError, "the library could not answer that right now")
}
