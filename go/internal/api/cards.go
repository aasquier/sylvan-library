package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// search is `GET /api/cards/search` -- `service.search_cards`, the 'deep
// hits from the whole history' tool. `identity` is a subset filter;
// `identity_exact` flips it for the create flow; `commanders_only` narrows
// in SQL and then decides in Go with the same `CanBeCommander` the gate
// uses -- one implementation of the rule, not two.
func (a *API) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []wire.ValidationError
	// Built field by field, in FastAPI's DECLARATION order, because that is
	// the order the 422 lists two bad parameters in -- and a struct literal
	// evaluates its fields in the order they are written, which reordered
	// `limit` ahead of `cmc_max` the first time this was extracted. A test
	// caught it; the wire shape is the contract, not just the values.
	var query deckread.SearchQuery
	query.Text = last(q, "q")
	query.Identity = last(q, "identity")
	query.TypeLine = last(q, "type_line")
	query.CMCMax, query.HaveCMC = optionalFloat(q, "cmc_max", &errs)
	query.PriceMax, query.HavePrice = optionalFloat(q, "price_max", &errs)
	query.Sort = last(q, "sort")
	query.Limit = boundedInt(q, "limit", 60, 1, 200, &errs)
	query.IdentityExact = flag(q, "identity_exact", &errs)
	query.CommandersOnly = flag(q, "commanders_only", &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	type answer struct {
		Cards []deckread.SearchCard `json:"cards"`
		Total int                   `json:"total"`
	}
	out := answer{Cards: []deckread.SearchCard{}}
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		found, err := deckread.SearchCards(r.Context(), c, query)
		if err != nil {
			return err
		}
		out.Cards, out.Total = found, len(found)
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

// identifyAnswer is what `service.identify_cards` returns: the readings and
// the four counts, in Python's key order.
type identifyAnswer struct {
	Readings []identifyReading `json:"readings"`
	Resolved int               `json:"resolved"`
	Offered  int               `json:"offered"`
	Unread   int               `json:"unread"`
	Dropped  int               `json:"dropped"`
}

// identifyAgainst is the body of `service.identify_cards`, lifted out of the
// handler because **two callers need it and neither is the other's parent**:
// the camera's own route, and the scan job, which resolves what Claude
// transcribed through exactly this door (ADR 34).
//
// That is Python's shape too -- `scanruns` calls `service.identify_cards`, the
// same function the route calls -- and it is the whole architecture in one
// line: a card read by Claude gets the same scrutiny as a card read by
// Tesseract, because it goes through the same code rather than through code
// that merely looks like it. A corner resolves only if its set code is one of
// the pool's real 986; a title only ever offers a shortlist.
//
// `resolved` and `offered` are counted apart deliberately: only a corner
// lookup resolves, and a title is a shortlist somebody still has to choose
// from, so adding them would report work as finished that nobody has done.
func identifyAgainst(ctx context.Context, c *pool.Conn, sightings []cards.Sighting) (identifyAnswer, error) {
	out := identifyAnswer{Readings: []identifyReading{}}
	readings, err := cards.Read(ctx, c, sightings)
	if err != nil {
		return out, err
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
	records, err := c.GetCards(ctx, names)
	if err != nil {
		return out, err
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
	return out, nil
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
	var out identifyAnswer
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		var readErr error
		out, readErr = identifyAgainst(r.Context(), c, sightings)
		return readErr
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

// fail is a handler's last line: the query failed for a reason that is not
// "no pool", which is a 500 in the envelope and a log line with the cause.
func (a *API) fail(w http.ResponseWriter, where string, err error) {
	a.log.Error("the pool query failed", "route", where, "error", err)
	wire.Detail(w, http.StatusInternalServerError, "the library could not answer that right now")
}
