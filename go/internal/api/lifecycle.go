package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckimport"
	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The moments a deck begins and ends: `POST /api/decks`, `POST
// /api/decks/import`, `DELETE /api/decks/{owner}/{slug}` and `PUT
// .../shared` -- the four lifecycle routes: create, import and delete, and
// the source's own sharing
// verb.
//
// Kept apart from the nine editing routes on purpose, and the reason is
// visible in the shapes below: **none of these goes through `commit`.**
// Creation and deletion are deliberately
// outside ADR 28's activity log, which is a decision rather than an oversight
// -- adding them means a second call site, and the log's whole design is that
// there is one. Sharing is outside it for the same reason and one more: it
// changes who may *read* a deck, not what the deck is.
//
// Two of the four write into the caller's **own** library and never into an
// owner named in the path (no ownership question exists for
// them), so they resolve `Mine` rather than `SourceFor`. The other two
// take the owner segment like every other per-deck route, and inherit ADR 22's
// answers with it: a deck the caller cannot see is absent from their source
// and every verb against it is a 404 before writability is consulted.

// slugPattern is the slug grammar. A slug becomes a directory name under
// `decks/`, so it is checked rather than trusted: the API takes it from a
// request body, and "sanitise it later" is how a path component turns into a
// path.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ---- create ----------------------------------------------------------------

// createDeck is `POST /api/decks` -- `service.create_deck`.
//
// Starts a deck from a commander and nothing else, which was the last gap in
// the lifecycle. `import_deck` refuses an empty list -- correctly, since an
// import with no cards is a mistake -- so create is its own path rather than
// an import of nothing.
//
// What it will not do is exactly what import will not do: **it never picks a
// commander for you**, and the deck arrives as a `draft` with no rationales,
// because there is nothing yet to justify. The 99 gets filled in afterwards by
// the edit operations that already exist.
//
// There is no `color_identity` field on purpose: identity is derived from the
// commander (rule 2), and accepting one here would be a second source of truth
// for the one fact this project will not guess at.
func (a *API) createDeck(w http.ResponseWriter, r *http.Request) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	bracket, err := bodyBracket(body)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, "bracket must be a number: "+err.Error())
		return
	}
	src, err := lib.Mine()
	if a.refuseWrite(w, "create", err) {
		return
	}
	writer, err := library.WriterFor(src, "")
	if a.refuseWrite(w, "create", err) {
		return
	}

	slug := strings.ToLower(strings.TrimSpace(str(body, "slug")))
	commander := []string{}
	for _, c := range bodyStrings(body, "commander") {
		if strings.TrimSpace(c) != "" {
			commander = append(commander, strings.TrimSpace(c))
		}
	}
	status := str(body, "status")
	if status == "" {
		status = "theoretical"
	}

	if !slugPattern.MatchString(slug) {
		a.refuseWrite(w, "create", rejectf(
			"%s is not a usable slug -- lowercase letters, digits and single "+
				"hyphens, e.g. 'arahbo-cats'", wire.Quote(slug)))
		return
	}
	slugs, err := src.Slugs(r.Context())
	if a.refuseWrite(w, "create", err) {
		return
	}
	if slices.Contains(slugs, slug) {
		a.refuseWrite(w, "create", rejectf(
			"a deck called %s already exists; pick another slug", wire.Quote(slug)))
		return
	}
	if len(commander) == 0 {
		a.refuseWrite(w, "create", rejectf("a new deck needs a commander"))
		return
	}
	if len(commander) > 2 {
		a.refuseWrite(w, "create", rejectf(
			"%d commanders listed; Commander allows at most two", len(commander)))
		return
	}
	if !slices.Contains(reference.Deck().DeckStatuses, status) {
		a.refuseWrite(w, "create", rejectf("status %s is not one of %s",
			wire.Quote(status), strings.Join(reference.Deck().DeckStatuses, ", ")))
		return
	}

	companion := strings.TrimSpace(str(body, "companion"))
	names := append(slices.Clone(commander), []string{}...)
	if companion != "" {
		names = append(names, companion)
	}

	var found map[string]*pool.CardRecord
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		var err error
		found, err = c.GetCards(r.Context(), names)
		return err
	})
	if errors.Is(err, pool.ErrNoPool) {
		// Same refusal as import, for the same reason: a deck whose commander
		// was never checked is a deck whose colour identity is a guess.
		a.refuseWrite(w, "create", rejectf(
			"creating a deck needs the card pool -- run `mtglab data refresh`"))
		return
	}
	if a.refuseWrite(w, "create", err) {
		return
	}

	missing := []string{}
	for _, n := range names {
		if _, ok := found[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		a.refuseWrite(w, "create", rejectf("not in the pool: %s", strings.Join(missing, ", ")))
		return
	}

	paired := len(commander) == 2
	for _, name := range commander {
		if !gate.CanBeCommander(found[name], paired) {
			a.refuseWrite(w, "create", rejectf(
				"%s cannot be your commander -- it is %s",
				found[name].Name, found[name].TypeLine))
			return
		}
	}
	if paired {
		// The same check the gate runs. Two commanders is not "any two
		// legends" -- Partner, Partner with, Friends forever, Choose a
		// Background and Doctor's companion each have their own rule.
		if why := gate.CheckPair(found[commander[0]], found[commander[1]]); why != "" {
			a.refuseWrite(w, "create", rejectf("%s", why))
			return
		}
	}

	identity := map[string]bool{}
	for _, name := range commander {
		for _, c := range found[name].ColorIdentity {
			identity[c] = true
		}
	}

	name := strings.TrimSpace(str(body, "name"))
	if name == "" {
		name = found[commander[0]].Name
	}
	var companionPtr *string
	if companion != "" {
		companionPtr = &companion
	}
	built := &deck.Deck{Slug: slug, Name: name, Status: status, Stage: "draft",
		Shared: true, Commander: commander, Companion: companionPtr, Bracket: bracket}
	text, err := built.Dump()
	if a.refuseWrite(w, "create", err) {
		return
	}
	if err := writer.Create(r.Context(), slug, text); err != nil {
		var exists library.ErrExists
		if errors.As(err, &exists) {
			// Only reachable by two creates racing for one slug: the check
			// above has already answered the ordinary case. The recorded
			// answer left this race a bare 500; refusing it in the same
			// words as the check is the honest answer and costs nothing.
			a.refuseWrite(w, "create", rejectf(
				"a deck called %s already exists; pick another slug", wire.Quote(slug)))
			return
		}
		a.refuseWrite(w, "create", err)
		return
	}

	letters := sortedColors(identity)
	combo, _ := reference.CombinationByKey(reference.KeyFor(letters))
	wire.JSON(w, http.StatusOK, map[string]any{
		"slug": slug, "owner": lib.MyOwner(), "name": built.Name,
		"stage": built.Stage, "status": built.Status, "created": true,
		"commander": built.Commander, "companion": companionOrNil(companionPtr),
		"color_identity": letters,
		"combination": map[string]any{
			"key": combo.Key, "name": combo.Name, "tier": combo.Tier},
		"total_cards": 0,
	})
}

// ---- import ----------------------------------------------------------------

// ---- reading a misspelled name ---------------------------------------------

// poolReader is `deckimport.Reader` over a live pool connection.
//
// The scoring is `cards.ByTitle`, unchanged and unwrapped -- the same
// function the camera door resolves a photographed title through (ADR 34).
// One scorer for both doors, so a name typed into the import box and a name
// read off a photograph are judged by the same measure and answer to the same
// thresholds.
type poolReader struct{ conn *pool.Conn }

func (p poolReader) Nearest(ctx context.Context, written string, limit int) (
	[]deckimport.Candidate, error) {

	found, err := cards.ByTitle(ctx, p.conn, written, limit)
	if err != nil {
		return nil, err
	}
	out := make([]deckimport.Candidate, 0, len(found))
	for _, c := range found {
		out = append(out, deckimport.Candidate{Name: c.Name, Score: c.Score})
	}
	return out, nil
}

func (p poolReader) Cards(ctx context.Context, names []string) (
	map[string]*pool.CardRecord, error) {

	return p.conn.GetCards(ctx, names)
}

// Where a suggestion stops being help and starts being noise.
//
// This is the FALLBACK tier, and it is what is left after `deckimport.Respell`
// has already read every name it could read with confidence. What reaches
// here is the close-run field and the genuine non-word: a shortlist somebody
// has to choose from, offered at a lower bar than a correction is made at
// (`deckimport.nearFloor`), because offering three names costs nothing and
// picking the wrong one of them is impossible without a click.
const mentionFloor = 0.90

// nearMisses is how many names are asked about. A list can arrive with
// ninety-nine names the pool has never heard of -- a paste of the wrong thing
// entirely, or a pool that was never refreshed -- and ninety-nine full-table
// similarity scans to tell somebody their paste was wrong is work nobody
// wanted done. The rest are still reported as unknown; they simply arrive
// without a shortlist, and the response says how many were skipped.
const nearMisses = 12

// suggestion is one written name and the pool names closest to it.
type suggestion struct {
	Written    string          `json:"written"`
	Candidates []suggestedCard `json:"candidates"`
}

type suggestedCard struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// didYouMean builds a shortlist for each name that survived `Respell`
// unresolved -- so by construction, every one of these is a name no single
// card was clearly enough.
//
// A miss is not always a misspelling: a card printed since the last
// `data refresh` is absent from a pool that is otherwise perfect, and the
// closest thing to its name will be a real card that is not it. That is the
// case this tier exists for, and why it offers rather than decides.
func didYouMean(ctx context.Context, c *pool.Conn, names []string) ([]suggestion, int) {
	out := []suggestion{}
	skipped := 0
	for i, written := range names {
		if i >= nearMisses {
			skipped = len(names) - nearMisses
			break
		}
		found, err := cards.ByTitle(ctx, c, written, 4)
		if err != nil {
			// A shortlist nobody could build is not a failed import. The name
			// is already reported as unknown, which is the load-bearing half.
			continue
		}
		shown := []suggestedCard{}
		for _, cand := range found {
			if cand.Score < mentionFloor || len(shown) == 3 {
				continue
			}
			shown = append(shown, suggestedCard{
				Name: cand.Name, Score: math.Round(cand.Score*10000) / 10000})
		}
		if len(shown) == 0 {
			continue
		}
		out = append(out, suggestion{Written: written, Candidates: shown})
	}
	return out, skipped
}

// importDeck is `POST /api/decks/import` -- `service.import_deck`.
//
// Turns a pasted decklist into a draft deck. Declared as a literal beside
// `/api/decks/{owner}/{slug}` and safe from it twice over: it is one segment
// shorter, and it is a POST while that is a GET.
//
// `dry_run` runs the identical code path and writes nothing, which is what the
// app's preview uses: the user approves the actual result rather than a
// description of it.
func (a *API) importDeck(w http.ResponseWriter, r *http.Request) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	bracket, err := bodyBracket(body)
	if err != nil {
		wire.Detail(w, http.StatusUnprocessableEntity, "bracket must be a number: "+err.Error())
		return
	}
	src, err := lib.Mine()
	if a.refuseWrite(w, "import", err) {
		return
	}
	// The library, not the slug: this runs before the slug is even validated,
	// and a caller who may not write should not learn whether their slug was
	// acceptable.
	writer, err := library.WriterFor(src, "")
	if a.refuseWrite(w, "import", err) {
		return
	}

	slug := strings.ToLower(strings.TrimSpace(str(body, "slug")))
	status := str(body, "status")
	if status == "" {
		status = "theoretical"
	}
	dryRun := truthy(body["dry_run"])

	if !slugPattern.MatchString(slug) {
		a.refuseWrite(w, "import", rejectf(
			"%s is not a usable slug -- lowercase letters, digits and single "+
				"hyphens, e.g. 'arahbo-cats'", wire.Quote(slug)))
		return
	}
	slugs, err := src.Slugs(r.Context())
	if a.refuseWrite(w, "import", err) {
		return
	}
	if slices.Contains(slugs, slug) {
		a.refuseWrite(w, "import", rejectf(
			"a deck called %s already exists; pick another slug", wire.Quote(slug)))
		return
	}
	if !slices.Contains(reference.Deck().DeckStatuses, status) {
		a.refuseWrite(w, "import", rejectf("status %s is not one of %s",
			wire.Quote(status), strings.Join(reference.Deck().DeckStatuses, ", ")))
		return
	}

	parsed := decklist.Parse(str(body, "text"))
	if len(parsed.Cards) == 0 {
		a.refuseWrite(w, "import", rejectf("nothing in that list parsed as a card"))
		return
	}

	commander := bodyStrings(body, "commander")
	companion := str(body, "companion")

	var report *deckimport.Report
	var verdict *gate.Report
	var nearby []suggestion
	var over int
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		wanted := deckimport.NamesIn(parsed, commander, companion)
		found, err := c.GetCards(r.Context(), wanted)
		if err != nil {
			return err
		}
		// Before the deck is built, so a corrected card is simply a card by
		// the time anything downstream sees it -- the count, the category,
		// the colour identity and the gate all read the real record.
		corrections, err := deckimport.Respell(r.Context(), poolReader{c}, wanted, found)
		if err != nil {
			return err
		}
		built, err := deckimport.BuildDeck(parsed, found, deckimport.Options{
			Slug: slug, Name: str(body, "name"), Commander: commander,
			Companion: companion, Bracket: bracket, Status: status,
			Read: corrections})
		if err != nil {
			return err
		}
		report = built
		if !dryRun {
			if err := writer.Create(r.Context(), slug, built.YAML); err != nil {
				var exists library.ErrExists
				if errors.As(err, &exists) {
					return rejectf("a deck called %s already exists", wire.Quote(slug))
				}
				return err
			}
		}
		known, err := deckread.PoolFor(r.Context(), c, built.Deck)
		if err != nil {
			return err
		}
		verdict = gate.Validate(built.Deck, known, gate.DefaultSize)
		// Last, and inside the same pool lease: the shortlist is the only
		// part of this that is allowed to fail quietly.
		// Whatever is still unresolved after the reading: the close-run
		// fields and the genuine non-words.
		nearby, over = didYouMean(r.Context(), c, built.Unknown)
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		// Without the pool every name is unknown and no land is filed, so the
		// import would produce a deck whose facts were never checked -- the
		// one thing the gate exists to prevent. Refuse rather than half-do it.
		a.refuseWrite(w, "import", rejectf(
			"importing needs the card pool -- run `mtglab data refresh`"))
		return
	}
	if deckimport.IsRefused(err) {
		a.refuseWrite(w, "import", rejectf("%s", err.Error()))
		return
	}
	if a.refuseWrite(w, "import", err) {
		return
	}

	d := report.Deck
	swaps := []string{}
	for _, c := range d.SwapBoard {
		swaps = append(swaps, c.Name)
	}
	wire.JSON(w, http.StatusOK, map[string]any{
		"slug": slug, "owner": lib.MyOwner(), "name": d.Name,
		"stage": d.Stage, "status": d.Status, "created": !dryRun,
		"commander": d.Commander, "companion": companionOrNil(d.Companion),
		"total_cards": d.TotalCards(), "land_count": d.LandCount(),
		"swap_board": swaps, "needs_rationale": report.NeedsRationale(),
		"unknown": report.Unknown,
		// The shortlist beside the misses, and how many misses did not get
		// one. Never applied here: see `didYouMean`.
		"did_you_mean":         nearby,
		"did_you_mean_skipped": over,
		// What was misspelled and what it was read as. On the wire in its own
		// right as well as in `notes`, so the page can put it where somebody
		// will look rather than at the end of a list of remarks.
		"read":       report.Read,
		"unreadable": reportedLines(report.Unreadable),
		"skipped":    reportedLines(report.Skipped),
		"notes":      orEmpty(report.Notes), "yaml": report.YAML,
		"ok": verdict.OK(), "errors": verdict.Errors(), "warnings": verdict.Warnings(),
	})
}

// ---- delete ----------------------------------------------------------------

// deleteDeck is `DELETE /api/decks/{owner}/{slug}` -- `service.delete_deck`.
//
// The last operation in the lifecycle, and the only one that can lose work.
// Three safeguards, chosen so that each catches something the others do not:
// a typed `confirm` that is neither a boolean nor a flag, a read-only source
// that refuses, and a deck that **moves** rather than vanishing -- which is
// why the answer carries where it went.
//
// `confirm` is a query parameter because a DELETE with no body is the
// conventional shape. It takes the slug or the word `bury`, case-insensitively:
// the slug alone was the original rule and, in practice, a gate people could
// not get through -- `ishai-ojutai-dragonspeaker` is 26 characters to copy by
// eye, and a confirmation nobody can satisfy does not protect the deck, it
// moves the deletion to the shell, unconfirmed.
func (a *API) deleteDeck(w http.ResponseWriter, r *http.Request) {
	// **The deck is resolved before writability is asked**, which is ADR 5 and
	// not a detail: a deck this caller cannot see is absent from their source,
	// so every verb against it must be a 404, and a 403 raised first would
	// confirm it exists. `writeTarget` keeps the same order for the nine
	// editing routes.
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	writer, err := library.WriterFor(src, slug)
	if a.refuseWrite(w, "delete", err) {
		return
	}

	typed := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("confirm")))
	if typed != strings.ToLower(slug) && typed != deleteWord {
		a.refuseWrite(w, "delete", rejectf(
			"to delete %s, confirm by typing %s or the slug itself. Got %s. "+
				"This is deliberately not a yes/no: it is the one operation here "+
				"that can lose work nothing else recorded.",
			wire.Quote(slug), wire.Quote(deleteWord),
			wire.Quote(r.URL.Query().Get("confirm"))))
		return
	}

	movedTo, err := writer.Delete(r.Context(), slug)
	if a.refuseWrite(w, "delete", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{
		"slug": slug, "name": d.Name, "deleted": true,
		// Where it went, so the answer to "can I get it back" is in the
		// response rather than in someone's memory of how this was built.
		"moved_to": movedTo, "total_cards": d.TotalCards(),
		"stage": d.Stage, "status": d.Status,
	})
}

// deleteWord is `service.DELETE_WORD`.
//
// "Bury" is Magic's own retired templating for destroying a permanent so that
// it cannot regenerate, and it is the right verb here for the reason the
// obvious alternatives are the wrong ones: the deck goes to `decks/.trash/`
// and can be brought back, so "exile" -- which in Magic means gone for good --
// would misdescribe what this does.
const deleteWord = "bury"

// ---- sharing ---------------------------------------------------------------

// setDeckShared is `PUT /api/decks/{owner}/{slug}/shared`.
//
// Its own route rather than a `field` on the PATCH beside it, because the two
// tiers hold this fact in different places and the source is what knows which
// -- `deck.yaml` for the curated six, a column for everybody else.
//
// Refusals are the ordinary two and neither is written here: somebody else's
// shared deck is ErrReadOnly (403), and their private one was never in the
// source at all, so it is ErrNotFound (404).
func (a *API) setDeckShared(w http.ResponseWriter, r *http.Request) {
	// The body first, then the deck -- the recorded order: a request
	// with no flag is 422 even for a deck the caller cannot see.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	raw, given := body["shared"]
	if !given {
		wire.Detail(w, http.StatusUnprocessableEntity, "shared is required")
		return
	}
	// Then the deck, then writability -- ADR 5's order, as in `deleteDeck`.
	src, _, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	writer, err := library.WriterFor(src, slug)
	if a.refuseWrite(w, "shared", err) {
		return
	}
	if err := writer.SetShared(r.Context(), slug, truthy(raw)); a.refuseWrite(w, "shared", err) {
		return
	}
	// The whole deck -- the recorded answer: `shared` is the one deck
	// field a client changes without already holding the rest of the deck's
	// new state. Answered by *calling the read route* rather than by a second
	// renderer -- the path shape is the same, so the two cannot describe one
	// deck two ways.
	a.getDeck(w, r)
}

// ---- the shared helpers ----------------------------------------------------

// bodyBracket reads the optional bracket: absent, null or empty is none;
// anything else goes through the recorded integer coercion, whose refusal
// the route answers with a 422 naming the field.
func bodyBracket(body map[string]any) (*int, error) {
	raw, given := body["bracket"]
	if !given || raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid literal for int() with base 10: %s", wire.Quote(v))
		}
		return &n, nil
	case json.Number:
		// A number with a fraction truncates rather than refusing -- the
		// recorded coercion for a JSON number arriving as a float.
		f, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid literal for int(): %s", wire.Quote(v.String()))
		}
		n := int(f)
		return &n, nil
	case bool:
		// A boolean coerces to 0 or 1. Nobody sends this; leaving it to the
		// default branch would refuse where the recorded coercion accepts.
		n := 0
		if v {
			n = 1
		}
		return &n, nil
	default:
		return nil, fmt.Errorf("int() argument must be a string or a number, not '%T'", raw)
	}
}

// bodyStrings reads a field that may arrive as one string or a list of them,
// which is `commander` on both collection routes.
func bodyStrings(body map[string]any, key string) []string {
	switch v := body[key].(type) {
	case nil:
		return []string{}
	case string:
		if v == "" {
			// An empty string is falsy in the recorded reading, so it never
			// becomes a one-element list.
			return []string{}
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, str(map[string]any{"x": item}, "x"))
		}
		return out
	default:
		return []string{}
	}
}

// truthy is the recorded truthiness over a decoded JSON value: an empty
// string, a zero, an empty container and null are false; everything else is
// true. So `{"shared": "no"}` is true and `{"shared": 0}` is false, which
// is not what a Go `bool` cast would say and is what the routes have
// always done.
func truthy(v any) bool {
	switch value := v.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case json.Number:
		f, err := value.Float64()
		return err == nil && f != 0
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return true
	}
}

func sortedColors(identity map[string]bool) []string {
	out := make([]string, 0, len(identity))
	for c := range identity {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// companionOrNil renders `None` rather than `""`, which is what the deck model
// holds and what every other route serialising a companion writes.
func companionOrNil(companion *string) any {
	if companion == nil || *companion == "" {
		return nil
	}
	return *companion
}

func reportedLines(lines []decklist.Line) []map[string]any {
	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		out = append(out, map[string]any{"line": l.LineNo, "text": l.Text})
	}
	return out
}

func orEmpty(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
