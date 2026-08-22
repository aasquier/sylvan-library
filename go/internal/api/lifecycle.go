package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckimport"
	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The moments a deck begins and ends: `POST /api/decks`, `POST
// /api/decks/import`, `DELETE /api/decks/{owner}/{slug}` and `PUT
// .../shared` -- `api/app.py`'s four lifecycle routes over
// `api/service.py`'s create, import and delete, and the source's own sharing
// verb.
//
// Held back from the nine editing routes on purpose, and the reason is
// visible in the shapes below: **none of these goes through `commit`.**
// Creation and deletion are outside `service._commit` in Python and therefore
// outside ADR 28's activity log, which is a decision rather than an oversight
// -- adding them means a second call site, and the log's whole design is that
// there is one. Sharing is outside it for the same reason and one more: it
// changes who may *read* a deck, not what the deck is.
//
// Two of the four write into the caller's **own** library and never into an
// owner named in the path (`routes.json` classifies them `shared` for exactly
// that reason), so they resolve `Mine` rather than `SourceFor`. The other two
// take the owner segment like every other per-deck route, and inherit ADR 22's
// answers with it: a deck the caller cannot see is absent from their source
// and every verb against it is a 404 before writability is consulted.

// slugPattern is `service._SLUG`. A slug becomes a directory name under
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
				"hyphens, e.g. 'arahbo-cats'", wire.PyRepr(slug)))
		return
	}
	slugs, err := src.Slugs(r.Context())
	if a.refuseWrite(w, "create", err) {
		return
	}
	if slices.Contains(slugs, slug) {
		a.refuseWrite(w, "create", rejectf(
			"a deck called %s already exists; pick another slug", wire.PyRepr(slug)))
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
			wire.PyRepr(status), strings.Join(reference.Deck().DeckStatuses, ", ")))
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
			// above has already answered the ordinary case. Python leaves this
			// one uncaught and answers 500; refusing it in the same words as
			// the check is the honest answer and costs nothing.
			a.refuseWrite(w, "create", rejectf(
				"a deck called %s already exists; pick another slug", wire.PyRepr(slug)))
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
	dryRun := pyTruthy(body["dry_run"])

	if !slugPattern.MatchString(slug) {
		a.refuseWrite(w, "import", rejectf(
			"%s is not a usable slug -- lowercase letters, digits and single "+
				"hyphens, e.g. 'arahbo-cats'", wire.PyRepr(slug)))
		return
	}
	slugs, err := src.Slugs(r.Context())
	if a.refuseWrite(w, "import", err) {
		return
	}
	if slices.Contains(slugs, slug) {
		a.refuseWrite(w, "import", rejectf(
			"a deck called %s already exists; pick another slug", wire.PyRepr(slug)))
		return
	}
	if !slices.Contains(reference.Deck().DeckStatuses, status) {
		a.refuseWrite(w, "import", rejectf("status %s is not one of %s",
			wire.PyRepr(status), strings.Join(reference.Deck().DeckStatuses, ", ")))
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
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		found, err := c.GetCards(r.Context(), deckimport.NamesIn(parsed, commander, companion))
		if err != nil {
			return err
		}
		built, err := deckimport.BuildDeck(parsed, found, deckimport.Options{
			Slug: slug, Name: str(body, "name"), Commander: commander,
			Companion: companion, Bracket: bracket, Status: status})
		if err != nil {
			return err
		}
		report = built
		if !dryRun {
			if err := writer.Create(r.Context(), slug, built.YAML); err != nil {
				var exists library.ErrExists
				if errors.As(err, &exists) {
					return rejectf("a deck called %s already exists", wire.PyRepr(slug))
				}
				return err
			}
		}
		cards, err := poolFor(r.Context(), c, built.Deck)
		if err != nil {
			return err
		}
		verdict = gate.Validate(built.Deck, cards, gate.DefaultSize)
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
		"unknown":    report.Unknown,
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
	// confirm it exists. Python arrives at the same order from the other side
	// -- `service._for_writing` looks the deck up itself when the source hides
	// things -- and it is what `writeTarget` does for the nine editing routes.
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
			wire.PyRepr(slug), wire.PyRepr(deleteWord),
			wire.PyRepr(r.URL.Query().Get("confirm"))))
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
	// The body first, then the deck: Python checks `"shared" not in payload`
	// in the handler and resolves the source on the line after, so a request
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
	if err := writer.SetShared(r.Context(), slug, pyTruthy(raw)); a.refuseWrite(w, "shared", err) {
		return
	}
	// The whole deck, which is what Python returns: `shared` is the one deck
	// field a client changes without already holding the rest of the deck's
	// new state. Answered by *calling the read route* rather than by a second
	// renderer -- the path shape is the same, so the two cannot describe one
	// deck two ways.
	a.getDeck(w, r)
}

// ---- the shared helpers ----------------------------------------------------

// bodyBracket is `int(bracket) if bracket not in (None, "") else None`, whose
// ValueError the route answers with a 422 naming the field.
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
			return nil, fmt.Errorf("invalid literal for int() with base 10: %s", wire.PyRepr(v))
		}
		return &n, nil
	case json.Number:
		// `int(4.0)` truncates rather than refusing, which is what a JSON
		// number arriving as a float has to do here too.
		f, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid literal for int(): %s", wire.PyRepr(v.String()))
		}
		n := int(f)
		return &n, nil
	case bool:
		// `int(True)` is 1. Nobody sends this; leaving it to the default
		// branch would refuse where Python accepts.
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
			// `payload.get("commander") or []` -- an empty string is falsey in
			// Python, so it never becomes a one-element list.
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

// pyTruthy is Python's `bool()` over a decoded JSON value. `{"shared": "no"}`
// is true and `{"shared": 0}` is false, which is not what a Go `bool` cast
// would say and is what the route has always done.
func pyTruthy(v any) bool {
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
