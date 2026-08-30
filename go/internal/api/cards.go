package api

import (
	"context"
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
	"github.com/aasquier/sylvan-library/go/internal/deckimport"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
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
	// Built field by field, in the recorded DECLARATION order, because that
	// is the order the 422 lists two bad parameters in -- and a struct literal
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

// offeredCard is one card on the shortlist behind "add a card": enough to
// draw a row, draw the whole card beside it, credit the painter, and say the
// two things the write path would otherwise only say *after* somebody has
// written a rationale -- that a card is banned, or outside this deck's
// colours.
//
// `is_land` is here for the same reason the importer infers it: filing a card
// under `land` is a card pool fact (`CardRecord.IsLand`, right about the
// double-faced cards a type line is wrong about) and not an opinion. Every
// other category is an opinion and stays the person's. **Nothing here carries
// a rationale, and nothing ever will** -- rule 4, ADR 8, ADR 11.
type offeredCard struct {
	Name          string   `json:"name"`
	ManaCost      *string  `json:"mana_cost"`
	TypeLine      string   `json:"type_line"`
	OracleText    string   `json:"oracle_text"`
	ColorIdentity []string `json:"color_identity"`
	Image         *string  `json:"image"`
	// The painter, owed a credit wherever the painting renders (ADR 6's
	// hot-link terms, ADR 32, commandment 9). A card image with no line under
	// it is the violation, not a missing nicety.
	Artist         *string `json:"artist"`
	LegalCommander bool    `json:"legal_commander"`
	IsLand         bool    `json:"is_land"`
	// How alike the name is to what was typed, and which tier offered it:
	// `opens`, `holds`, `words` or `near`. The interface says different things
	// about a name it found and a name it is guessing at, and only the server
	// knows which happened.
	Score float64 `json:"score"`
	Via   string  `json:"via"`
}

// suggest is `GET /api/cards/suggest` -- the typeahead behind "add a card".
//
// Private, like every other card route: nothing is added to the door's
// allowlist, so it is refused without a session before routing.
//
// **It filters by neither legality nor the deck's colours, and that is the
// decision worth recording.** Both would have been easy and both are wrong
// here: a card hidden from the list is indistinguishable from a card that
// does not exist, and the person is left retyping a name that was right all
// along. The same argument as "an invalid deck is simulated, not refused" --
// removing the diagnosis exactly when it is wanted. So a banned card is
// listed and *marked*, the deck's identity is checked in the browser against
// `color_identity` and *marked*, and the authoritative refusal stays where it
// has always been: `playableCard`, one implementation of the rule.
func (a *API) suggest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []wire.ValidationError
	text := last(q, "q")
	limit := boundedInt(q, "limit", 8, 1, cards.MaxSuggestions, &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	out := []offeredCard{}
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		found, err := cards.Suggest(r.Context(), c, text, limit)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(found))
		for _, s := range found {
			names = append(names, s.Name)
		}
		records, err := c.GetCards(r.Context(), names)
		if err != nil {
			return err
		}
		for _, s := range found {
			rec := records[s.Name]
			if rec == nil {
				// The pool moved under the shortlist between the two queries.
				// Dropped rather than half-rendered: a row with no card
				// behind it is a row somebody would click.
				continue
			}
			out = append(out, offeredCard{
				Name: rec.Name, ManaCost: rec.ManaCost, TypeLine: rec.TypeLine,
				OracleText: rec.OracleText, ColorIdentity: rec.ColorIdentity,
				Image: rec.ImageNormal, Artist: rec.Artist,
				LegalCommander: rec.LegalCommander, IsLand: rec.IsLand(),
				Score: math.Round(s.Score*10000) / 10000, Via: s.Via,
			})
		}
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, map[string]any{"cards": []any{}, "message": noPoolMessage})
		return
	}
	if err != nil {
		a.fail(w, "suggest", err)
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"cards": out})
}

// ---- the commander field ---------------------------------------------------

// commanderSeat is one card considered for the command zone.
//
// No picture and no painter, deliberately. This answers a text field while
// somebody is still typing in it, and a card image owes a credit in the same
// room it renders in (commandment 19, ADR 32) -- a strip under an input is not
// that room. The cost, the type line and the identity are what confirm a
// commander anyway: they are the three things you check before sleeving one.
type commanderSeat struct {
	Name          string   `json:"name"`
	ManaCost      *string  `json:"mana_cost"`
	TypeLine      string   `json:"type_line"`
	ColorIdentity []string `json:"color_identity"`
	// MayCommand is the rules question -- legendary creature, or a card that
	// says it can be your commander, or a Background when a pair is being
	// read. LegalCommander is the format question, and they fail apart: a
	// banned legend answers true and false.
	MayCommand     bool `json:"may_command"`
	LegalCommander bool `json:"legal_commander"`
	// Pairing is how this card partners, in words, or "" for a card that
	// does not. It is what makes "write both names" actionable rather than
	// mysterious when somebody types half of a pair.
	Pairing string  `json:"pairing"`
	Score   float64 `json:"score"`
}

// commanderOffer is one written name and the cards it might have been.
type commanderOffer struct {
	Written    string          `json:"written"`
	Candidates []commanderSeat `json:"candidates"`
}

// commanderAnswer is what the import page's commander box is told.
//
// `state` is the field's own condition and the only thing the border reads:
// `blank`, `ready`, `trouble`, `unknown`. `sentence` is always a whole
// sentence a newcomer can act on, never a code and never a scold.
type commanderAnswer struct {
	State      string           `json:"state"`
	Sentence   string           `json:"sentence"`
	Commanders []commanderSeat  `json:"commanders"`
	DidYouMean []commanderOffer `json:"did_you_mean"`
	Message    string           `json:"message,omitempty"`
}

// commanderOffers is how many near names one unknown entry is offered.
const commanderOffers = 5

// seatOf describes one card. `name` is how the card should be SPELLED back --
// the front face for a double-faced card somebody wrote by its front face,
// exactly as `deckimport.CanonicalName` spells it and for the same reason: the
// library writes face names, and answering "Etali, Primal Conqueror" with
// "Etali, Primal Conqueror // Etali, Primal Sickness" would be this box
// disagreeing with the import it sits above.
func seatOf(name string, rec *pool.CardRecord, paired bool, score float64) commanderSeat {
	seat := commanderSeat{Name: name, ManaCost: rec.ManaCost,
		TypeLine: rec.TypeLine, ColorIdentity: rec.ColorIdentity,
		MayCommand: gate.CanBeCommander(rec, paired),
		// A Background can never lead alone, but it is a legal commander in a
		// pair -- so the seat says what it is either way, and the sentence
		// beside it is what explains the difference.
		LegalCommander: rec.LegalCommander, Score: score}
	if p := gate.PairingOf(rec); p != nil {
		seat.Pairing = p.Describe()
	}
	return seat
}

// commanderCheck is `GET /api/cards/commander` -- what the import page's
// commander box is answered with while somebody types in it.
//
// Private, like every other card route: nothing is added to the door's
// allowlist, so it is refused without a session before routing, which is the
// same posture as the import it serves.
//
// **Its own route rather than a flag on `/api/cards/suggest`, and the reason
// is that they answer different questions.** Suggest answers "which cards are
// named like this", and its `legal_commander` is the *format's* answer --
// whether the card is legal in Commander at all, which is true of Sol Ring.
// Whether a card may sit in the command zone is `gate.CanBeCommander`, a
// different fact entirely, and neither one implies the other. Nor can a
// typeahead read `Tymna the Weaver + Thrasios, Triton Hero`: one field can be
// holding two commanders, deciding that takes a card pool (`CommanderReading`,
// the same one the import itself uses), and whether the two may sit together
// is a third fact again (`gate.CheckPair`). So this route answers about the
// FIELD, and the suggest route still answers about a name.
//
// Every fact in the answer is deterministic (ADR 14): the pool knows the
// cards, the gate knows the rules, and nothing here is an opinion.
//
// **A near name is offered, never hidden**, and commanders are listed first
// rather than alone -- the same argument `suggest` records above. Somebody who
// typed a card that cannot lead is told which card they typed and why it
// cannot, which is the diagnosis; a filtered list would have said "no such
// card" about a card they own.
func (a *API) commanderCheck(w http.ResponseWriter, r *http.Request) {
	text := strings.TrimSpace(last(r.URL.Query(), "q"))
	out := commanderAnswer{State: "blank", Commanders: []commanderSeat{},
		DidYouMean: []commanderOffer{}}
	if text == "" {
		wire.JSON(w, http.StatusOK, out)
		return
	}

	ctx := r.Context()
	err := a.usePool(ctx, func(c *pool.Conn) error {
		// Both readings fetched at once, exactly as the import fetches them,
		// so the choice below is made by lookup rather than by guessing.
		names := []string{text}
		for _, parts := range deckimport.PairParts(text) {
			names = append(names, parts...)
		}
		records, err := c.GetCards(ctx, names)
		if err != nil {
			return err
		}
		wanted, _ := deckimport.CommanderReading([]string{text}, records)
		paired := len(wanted) > 1

		missing, seated := []string{}, []*pool.CardRecord{}
		for _, written := range wanted {
			canonical, rec := deckimport.CanonicalName(written, records)
			if rec == nil {
				missing = append(missing, canonical)
				continue
			}
			seated = append(seated, rec)
			out.Commanders = append(out.Commanders, seatOf(canonical, rec, paired, 1))
		}
		if len(missing) > 0 {
			out.State, out.Sentence = "unknown", unknownSentence(missing)
			offers, err := commanderNearby(ctx, c, missing, paired)
			if err != nil {
				return err
			}
			out.DidYouMean = offers
			return nil
		}
		out.State, out.Sentence = commanderVerdict(out.Commanders, seated)
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		// The field cannot be answered at all, so it says nothing rather than
		// answering `unknown` -- a red box over a correctly-typed commander is
		// worse than no box.
		wire.JSON(w, http.StatusOK, commanderAnswer{State: "blank",
			Commanders: []commanderSeat{}, DidYouMean: []commanderOffer{},
			Message: noPoolMessage})
		return
	}
	if err != nil {
		a.fail(w, "commander check", err)
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// commanderVerdict reads the resolved seats into a state and a sentence.
// `seats` and `seated` are the same cards in the same order, wire-side and
// pool-side.
func commanderVerdict(seats []commanderSeat, seated []*pool.CardRecord) (string, string) {
	if len(seats) == 0 {
		// Only blanks were written, which the trim in the handler should
		// already have caught. Answered rather than left to fall through to
		// `ready`, which would light the box green over nothing.
		return "blank", ""
	}
	if len(seats) > 2 {
		return "trouble", fmt.Sprintf("A deck is led by one commander, or by two "+
			"that pair with each other. This reads as %d names — write one, or "+
			"two with a + between them.", len(seats))
	}
	for i, seat := range seats {
		if !seat.MayCommand {
			// A Background typed alone is the one near-miss that earns its own
			// sentence: the card IS a command-zone card, it simply cannot go
			// there alone, and "that is not a commander" would be a flat lie
			// about it -- the exact shape of thing commandment 2 forbids.
			if gate.IsBackground(seated[i]) {
				return "trouble", fmt.Sprintf("%s is a Background. It rides along "+
					"with a commander that chooses one, so write both names with "+
					"a + between them.", seat.Name)
			}
			return "trouble", fmt.Sprintf("%s is a real card, but it cannot sit "+
				"in the command zone. A commander is a legendary creature — or "+
				"a card whose text says it can be your commander.", seat.Name)
		}
		if !seat.LegalCommander {
			return "trouble", fmt.Sprintf("%s could sit in the command zone, but "+
				"it is not legal in Commander, so it cannot lead a deck.", seat.Name)
		}
	}
	if len(seats) == 2 {
		if why := gate.CheckPair(seated[0], seated[1]); why != "" {
			return "trouble", fmt.Sprintf("%s and %s cannot lead together: %s.",
				seats[0].Name, seats[1].Name, why)
		}
		return "ready", fmt.Sprintf("%s and %s can lead this deck together.",
			seats[0].Name, seats[1].Name)
	}
	return "ready", fmt.Sprintf("%s can lead this deck.", seats[0].Name)
}

// unknownSentence says which typed names are not cards, without scolding
// anybody for a typo (commandment 2).
func unknownSentence(missing []string) string {
	if len(missing) == 1 {
		return fmt.Sprintf("No card here is called %s. Check the spelling, pick "+
			"one below, or leave this blank and the list's own commander line "+
			"will be used.", wire.Quote(missing[0]))
	}
	return fmt.Sprintf("%d of those names are not cards here: %s. Pick from "+
		"below, or leave this blank and the list's own commander line will be "+
		"used.", len(missing), strings.Join(missing, ", "))
}

// commanderNearby is the shortlist for each name the pool did not know, with
// the cards that may actually command listed first.
func commanderNearby(ctx context.Context, c *pool.Conn, missing []string,
	paired bool) ([]commanderOffer, error) {

	offers := []commanderOffer{}
	for _, written := range missing {
		found, err := cards.Suggest(ctx, c, written, commanderOffers)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(found))
		for _, s := range found {
			names = append(names, s.Name)
		}
		records, err := c.GetCards(ctx, names)
		if err != nil {
			return nil, err
		}
		leads, rest := []commanderSeat{}, []commanderSeat{}
		for _, s := range found {
			rec := records[s.Name]
			if rec == nil {
				// The pool moved between the two queries. Dropped rather than
				// half-rendered: a row with no card behind it is a row
				// somebody would click.
				continue
			}
			seat := seatOf(rec.Name, rec, paired, math.Round(s.Score*10000)/10000)
			if seat.MayCommand && seat.LegalCommander {
				leads = append(leads, seat)
				continue
			}
			rest = append(rest, seat)
		}
		offers = append(offers, commanderOffer{Written: written,
			Candidates: append(leads, rest...)})
	}
	return offers, nil
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

// identifyAnswer is the camera's answer: the readings and
// the four counts, in the recorded key order.
type identifyAnswer struct {
	Readings []identifyReading `json:"readings"`
	Resolved int               `json:"resolved"`
	Offered  int               `json:"offered"`
	Unread   int               `json:"unread"`
	Dropped  int               `json:"dropped"`
}

// identifyAgainst is the identification engine, lifted out of the
// handler because **two callers need it and neither is the other's parent**:
// the camera's own route, and the scan job, which resolves what Claude
// transcribed through exactly this door (ADR 34).
//
// One function for both callers is the whole architecture in one
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

// last is the recorded reading of a repeated query parameter: the
// last value. Go's Values.Get would return the first.
func last(q map[string][]string, name string) string {
	vals := q[name]
	if len(vals) == 0 {
		return ""
	}
	return vals[len(vals)-1]
}

// optionalFloat is an optional float query parameter: absent is (0, false);
// present and unparseable is the recorded float_parsing error.
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
//
// `name` is "limit" at every call site today, which `unparam` notices and
// reports. It stays because this is one of a family -- `boundedFloat` and
// `flag` beside it take the same `(q, name, ...)` shape, and `flag` really
// does vary its name ("identity_exact", "commanders_only"). Dropping the
// parameter here would buy one fewer argument and cost the reader the
// symmetry that says these three are the same kind of thing; the error
// messages below would also have to hardcode a name they currently quote.
//
//nolint:unparam // one of a family; see above
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

// flag is a default-false boolean query parameter, read with the recorded
// boolean spellings.
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

// readObject reads a JSON object body under the recorded contract for a
// declared object body: a body that is not JSON, or not an object, is the
// validation 422 rather than the handler's.
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
