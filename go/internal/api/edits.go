package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"

	"database/sql"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckedit"
	"github.com/aasquier/sylvan-library/go/internal/deckimport"
	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The deck writes: the ten editing routes over
// `internal/deckedit`'s surgical operations, whose output the
// recorded goldens pin byte for byte.
//
// **Every write goes out through `commit`**, which
// carries its two jobs: no caller can change a deck without being told what
// the change did to the gate, and no caller can change a deck without the
// change being recorded (ADR 28). That it is one function rather than ten is
// the whole design -- and it has now been proved rather than argued: the tenth
// route, the bulk edit, arrived on 2026-08-29 and inherited both without
// touching either.
//
// Three refusals, and which is which is a decision rather than a default.
// A deck the caller may not *see* is absent from their source, so every verb
// against it is **404** before writability is consulted (ADR 5, ADR 22). A
// deck they can see but not edit is **403** -- the existence is not the
// secret, and the request is not malformed. Everything the editor itself
// refuses is **422** with the editor's own sentence, because those are
// answers about the deck rather than about the caller.

// commitOutcome is what a commit answers: the operation's own keywords,
// then what the edit did to the deck.
//
// The operation's keys are spread in by the caller rather than modelled here,
// because they differ per operation and the wire has always carried them that
// way -- `added`/`category`/`into` for one, `swapped_out`/`swapped_in` for
// another. `extra` is the same map `decklog.Describe` reads, so the response
// and the history cannot describe one edit two ways.
type commitOutcome struct {
	extra map[string]any
	edit  decklog.Edit
}

// commit writes an edited deck and hands back the gate's verdict on the
// result.
//
// The order is the honest one: the deck is written, then
// the entry is recorded, then the gate runs. The edit has happened by the time
// the log is written, so a validation that blows up afterwards must not be
// able to erase the record of it -- and `Record` never fails the edit, for the
// same reason one step further on.
func (a *API) commit(ctx context.Context, src library.Source, slug, updated string,
	outcome commitOutcome) (map[string]any, error) {

	writer, err := library.WriterFor(src, slug)
	if err != nil {
		return nil, err
	}
	if err := writer.WriteText(ctx, slug, updated); err != nil {
		return nil, err
	}

	a.recorder().Record(ctx, slug, src.OwnerID(), a.actor(ctx), outcome.edit)

	after, err := src.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	var report *gate.Report
	err = a.withPool(ctx, func(c *pool.Conn) error {
		var cards map[string]*pool.CardRecord
		if c != nil {
			var err error
			if cards, err = deckread.PoolFor(ctx, c, after); err != nil {
				return err
			}
		}
		report = gate.Validate(after, cards, gate.DefaultSize)
		return nil
	})
	if err != nil {
		return nil, err
	}

	body := map[string]any{"slug": slug}
	for k, v := range outcome.extra {
		body[k] = v
	}
	body["stage"] = after.Stage
	body["total_cards"] = after.TotalCards()
	body["needs_rationale"] = len(after.Unjustified())
	body["ok"] = report.OK()
	body["errors"] = report.Errors()
	body["warnings"] = report.Warnings()
	return body, nil
}

// refuseWrite is `refuse` plus the two answers only a write can give:
// ErrReadOnly -> 403, and the editor's own refusal -> 422 with its sentence.
func (a *API) refuseWrite(w http.ResponseWriter, where string, err error) bool {
	if err == nil {
		return false
	}
	var rejected *editRejected
	switch {
	case library.IsReadOnly(err):
		wire.Detail(w, http.StatusForbidden, err.Error())
	case errors.As(err, &rejected):
		wire.Detail(w, http.StatusUnprocessableEntity, rejected.Error())
	case deckedit.IsFailed(err):
		wire.Detail(w, http.StatusUnprocessableEntity, err.Error())
	default:
		return a.refuse(w, where, err)
	}
	return true
}

// editRejected is `EditRejected`: a refusal about the deck rather than about
// the caller. 422, with the sentence the caller reads.
type editRejected struct{ reason string }

func (e *editRejected) Error() string { return e.reason }

func rejectf(format string, args ...any) error {
	return &editRejected{reason: fmt.Sprintf(format, args...)}
}

// writeTarget is the preamble every editing route shares: resolve the owner,
// resolve the deck (404 before anything else), and refuse a caller who may
// read but not write before the body is even parsed.
func (a *API) writeTarget(w http.ResponseWriter, r *http.Request) (library.Source, *deck.Deck, bool) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return nil, nil, false
	}
	slug := r.PathValue("slug")
	d, err := src.Get(r.Context(), slug)
	if a.refuseWrite(w, "write", err) {
		return nil, nil, false
	}
	if _, err := library.WriterFor(src, slug); a.refuseWrite(w, "write", err) {
		return nil, nil, false
	}
	return src, d, true
}

// answer writes a commit's result, or the refusal it produced.
func (a *API) answer(w http.ResponseWriter, r *http.Request, src library.Source,
	updated string, outcome commitOutcome, err error) {
	if a.refuseWrite(w, "commit", err) {
		return
	}
	body, err := a.commit(r.Context(), src, r.PathValue("slug"), updated, outcome)
	if a.refuseWrite(w, "commit", err) {
		return
	}
	wire.JSON(w, http.StatusOK, body)
}

// ---- the bodies ------------------------------------------------------------

// readBody parses a JSON object body, answering the recorded validation
// 422 for everything that is not one.
//
// It answered `missing` for all four failures until 2026-08-22, and a wire
// diff is what said so: `POST /api/decks` with a body of `{` and no
// content type is `dict_type`, because a body whose content type does not
// say JSON is never parsed -- the raw bytes stand
// as a string, and a string is not a dictionary. Every write route
// shared this function and none of their goldens sent anything but a valid
// object, so the branch had never been asked a question.
//
// The recorded decision procedure, in its order:
//
//  1. no body at all -> `missing`;
//  2. a content type that is not `application/json` (or `application/…+json`),
//     including none -> the raw body as a *string*, which is `dict_type`;
//  3. JSON that will not parse -> `json_invalid`;
//  4. JSON that parses to null -> `missing`, since a null body is an absent
//     one;
//  5. JSON that parses to anything but an object -> `dict_type`.
func readBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(raw) == 0 {
		wire.Unprocessable(w, wire.Missing("body"))
		return nil, false
	}
	if !isJSONRequest(r.Header.Get("Content-Type")) {
		wire.Unprocessable(w, wire.DictType("body", string(raw)))
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		wire.Unprocessable(w, wire.JSONInvalid("body", decodeOffset(err), err.Error()))
		return nil, false
	}
	// The body is read as *one* JSON document, so anything after the first
	// value is a decode error rather than something to ignore.
	if decoder.More() {
		wire.Unprocessable(w, wire.JSONInvalid("body", int(decoder.InputOffset())+1, "Extra data"))
		return nil, false
	}
	if value == nil {
		wire.Unprocessable(w, wire.Missing("body"))
		return nil, false
	}
	body, ok := value.(map[string]any)
	if !ok {
		wire.Unprocessable(w, wire.DictType("body", value))
		return nil, false
	}
	return body, true
}

// isJSONRequest is the recorded content-type test: maintype `application`,
// subtype `json` or
// something ending `+json`. Parameters (`; charset=utf-8`) are ignored, and an
// absent header is not JSON.
func isJSONRequest(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	maintype, subtype, found := strings.Cut(media, "/")
	if !found || maintype != "application" {
		return false
	}
	return subtype == "json" || strings.HasSuffix(subtype, "+json")
}

// decodeOffset is the recorded parse position -- one-based -- read from
// whichever error Go's decoder raised.
func decodeOffset(err error) int {
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return int(syntax.Offset)
	}
	var unmarshalType *json.UnmarshalTypeError
	if errors.As(err, &unmarshalType) {
		return int(unmarshalType.Offset)
	}
	return 1
}

// str is the coercion four route families
// apply to a body field before it becomes a slug, an owner or a card name.
//
// An absent or null value is `""` and not `"None"` -- the recorded default
// -- and everything else goes through the recorded stringification, which
// is **not** `fmt.Sprint` for a container: a decoded list renders `['x']`
// where `fmt.Sprint` gives `[x]`, and that lands in a 404's `detail`
// verbatim. Found by a wire diff; see [wire.Plain].
func str(body map[string]any, key string) string {
	if v, ok := body[key]; !ok || v == nil {
		return ""
	}
	return wire.Plain(body[key])
}

// ---- the nine routes -------------------------------------------------------

// swapCard is `POST .../swap`. The operation the whole
// editor was written for, and the one that carries rule 4 at the boundary as
// well as inside the editor.
func (a *API) swapCard(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	// `into`, not `in`. The wire has always spelled it that way, and the
	// spelling is frozen with the rest of the contract.
	out, into, why := str(body, "out"), str(body, "into"), str(body, "why")
	if strings.TrimSpace(why) == "" {
		// Rule 4. A card that cannot justify its slot is a card to cut, and
		// nothing on this path may write the text that justifies it.
		a.refuseWrite(w, "swap", rejectf("a replacement needs a `why`"))
		return
	}
	entry := findCard(d, out)
	if entry == nil {
		a.refuseWrite(w, "swap", rejectf("%s is not in this deck", wire.Quote(out)))
		return
	}
	wanted := strings.ToLower(strings.TrimSpace(into))
	for _, c := range append(append([]deck.CardEntry{}, d.Cards...), d.SwapBoard...) {
		if strings.ToLower(c.Name) == wanted {
			a.refuseWrite(w, "swap", rejectf("%s is already in this deck", wire.Quote(into)))
			return
		}
	}
	for _, c := range d.Commander {
		if strings.ToLower(c) == wanted {
			a.refuseWrite(w, "swap", rejectf("%s is the commander", wire.Quote(into)))
			return
		}
	}

	rec, err := a.playableCard(r.Context(), d, into, "swapping needs the card pool -- run `mtglab data refresh`")
	if a.refuseWrite(w, "swap", err) {
		return
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "swap", err) {
		return
	}
	updated, err := deckedit.ReplaceCard(text, entry.Name, rec.Name, strings.TrimSpace(why), nil)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"swapped_out": entry.Name, "swapped_in": rec.Name,
			// The `why` rides in the response, as it always has, and is the
			// one thing the log does not carry across (ADR 28).
			"why": strings.TrimSpace(why)},
		edit: decklog.Edit{Kind: decklog.EditSwap, Card: entry.Name, SwapIn: rec.Name},
	}, err)
}

// addCard is `POST .../cards` -- `service.add_card`.
func (a *API) addCard(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	into := str(body, "to")
	if into == "" {
		into = "cards"
	}
	if into != "cards" && into != "swap_board" {
		a.refuseWrite(w, "add", rejectf("cards go into %s, not %s",
			strings.Join(deckedit.CardLists, " or "), wire.Quote(into)))
		return
	}
	qty, err := bodyQty(body)
	if err != nil {
		// The qty coercion refuses before the handler
		// body, and the route answers 422 with this sentence.
		wire.Detail(w, http.StatusUnprocessableEntity, "qty must be a number: "+err.Error())
		return
	}
	category, err := checkCategory(str(body, "category"))
	if a.refuseWrite(w, "add", err) {
		return
	}

	name := str(body, "name")
	rec, err := a.playableCard(r.Context(), d, name,
		"adding a card needs the card pool -- run `mtglab data refresh`")
	if a.refuseWrite(w, "add", err) {
		return
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "add", err) {
		return
	}
	updated, err := deckedit.AddCard(text, rec.Name, category, str(body, "why"), qty, into)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"added": rec.Name, "category": category, "into": into},
		edit:  decklog.Edit{Kind: decklog.EditAdd, Card: rec.Name, Category: category, Into: into},
	}, err)
}

// removeCard is `DELETE .../cards/{name}` -- `service.remove_card`.
//
// Since ADR 27 a removal from the 99 is an **entombment**: the entry moves to
// the graveyard with its category and its `why` intact. A swap-board card is
// still removed outright -- it was already outside the deck, and the board is
// its own record of why. Needs no card pool either way: removing a card is a
// fact about this deck file, not about Magic.
func (a *API) removeCard(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	entry := findCard(d, name)
	if entry == nil {
		a.refuseWrite(w, "remove", rejectf("%s is not in this deck", wire.Quote(name)))
		return
	}
	inThe99 := false
	for _, c := range d.Cards {
		if c.Name == entry.Name {
			inThe99 = true
			break
		}
	}
	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "remove", err) {
		return
	}
	if inThe99 {
		updated, err := deckedit.EntombCard(text, entry.Name)
		a.answer(w, r, src, updated, commitOutcome{
			extra: map[string]any{"entombed": entry.Name},
			edit:  decklog.Edit{Kind: decklog.EditEntomb, Card: entry.Name},
		}, err)
		return
	}
	updated, err := deckedit.RemoveCard(text, entry.Name)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"removed": entry.Name},
		edit:  decklog.Edit{Kind: decklog.EditRemove, Card: entry.Name},
	}, err)
}

// entombCards is `POST .../entomb` -- the bulk sweep.
//
// All or nothing: a name that is not in the 99 refuses the whole batch before
// anything is written, because a sweep that silently skipped two of its ten
// cards would report a deck state nobody chose. One write, one gate verdict,
// one entry in the activity log.
func (a *API) entombCards(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	raw, isList := body["names"].([]any)
	if !isList {
		wire.Detail(w, http.StatusUnprocessableEntity, "names must be a list")
		return
	}
	var wanted []string
	for _, item := range raw {
		if name := strings.TrimSpace(fmt.Sprint(item)); name != "" {
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		a.refuseWrite(w, "entomb", rejectf("nothing to entomb; give at least one card name"))
		return
	}
	in99 := map[string]string{}
	for _, c := range d.Cards {
		in99[strings.ToLower(c.Name)] = c.Name
	}
	var resolved []string
	for _, name := range wanted {
		match, found := in99[strings.ToLower(name)]
		if !found {
			a.refuseWrite(w, "entomb", rejectf("%s is not in the 99, so nothing was entombed",
				wire.Quote(name)))
			return
		}
		for _, already := range resolved {
			if already == match {
				a.refuseWrite(w, "entomb", rejectf("%s is listed twice", wire.Quote(match)))
				return
			}
		}
		resolved = append(resolved, match)
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "entomb", err) {
		return
	}
	for _, name := range resolved {
		if text, err = deckedit.EntombCard(text, name); err != nil {
			break
		}
	}
	a.answer(w, r, src, text, commitOutcome{
		extra: map[string]any{"entombed": resolved},
		edit:  decklog.Edit{Kind: decklog.EditEntomb, Cards: resolved},
	}, err)
}

// bulkEdit is `POST .../bulk` -- the whole 99 from one pasted list, in two
// round trips.
//
// **The one editing route that answers before it acts.** `dry_run` runs the
// identical code path and writes nothing, which is the shape `importDeck`
// argued and this borrows: the plan on screen is the real plan, computed by
// the code that will apply it, rather than a description of what that code
// intends to do. All the two calls do differently is one write and one check.
//
// **The check is what makes "you were asked" true.** A confirm carries back
// the `basis` the preview handed out, and the deck must still be that deck: a
// second tab, a window left open over lunch, or a double-click that beat the
// first response home would otherwise apply a plan whose burials were worked
// out against a comparison nobody made. A confirm with no `basis` at all is
// refused too, so "somebody saw the plan" is a fact about the protocol rather
// than a promise about the browser.
//
// **The gate reports; it does not block.** An invalid deck is edited like any
// other and the answer carries `errors` and `warnings` from the same `commit`
// every write here goes through -- a player fixing a broken deck is exactly
// who this is for. Two things are refused outright: an empty box, and a
// curated deck asked to take a card with no reason (rule 4, predicted in the
// plan rather than discovered mid-fold).
func (a *API) bulkEdit(w http.ResponseWriter, r *http.Request) {
	src, _, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	dryRun := truthy(body["dry_run"])

	parsed := decklist.Parse(str(body, "text"))
	// Only the deck's own section. A `Commander:` or `SIDEBOARD:` heading names
	// cards that live outside the 99 -- our own moxfield.txt artifact puts the
	// commander under `SIDEBOARD:` -- and this edit is about the 99. Those
	// lines are reported rather than read, because a line somebody pasted and
	// nothing happened to is the silence this repo keeps writing by accident.
	if len(parsed.Section("deck")) == 0 {
		a.refuseWrite(w, "bulk", rejectf("there are no cards in that box, so "+
			"nothing was changed. Paste the 99 first: an empty list would send "+
			"every card in this deck to the graveyard, which is not something "+
			"to do by accident."))
		return
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "bulk", err) {
		return
	}

	var plan *deckedit.BulkPlan
	var corrections []deckimport.Correction
	var unknown []string
	var nearby []suggestion
	var over int
	var quoted []string
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		// The import's own resolution sequence, in its order and for its
		// reasons: which reading of a trailing quoted run the pool has, then
		// the misspellings, then the cards. Reused rather than rewritten --
		// `deckimport` measured those thresholds (0.95, with a 0.04 lead) and
		// a second matcher here would read a name differently from the page
		// that created the deck.
		found, err := c.GetCards(r.Context(), deckimport.NamesIn(parsed, nil, ""))
		if err != nil {
			return err
		}
		read, chosen := deckimport.ReadRationales(parsed, found)
		quoted = chosen
		corrections, err = deckimport.Respell(r.Context(), poolReader{c},
			deckimport.NamesIn(read, nil, ""), found)
		if err != nil {
			return err
		}

		wanted := []deckedit.BulkCard{}
		for _, line := range read.Section("deck") {
			name, rec := deckimport.CanonicalName(line.Name, found)
			if rec == nil {
				// Rule 1: a card nobody looked up is a card whose legality is a
				// guess, so it never reaches the plan. It is reported by name
				// with a shortlist, and the deck keeps whatever it has.
				if !slices.Contains(unknown, name) {
					unknown = append(unknown, name)
				}
				continue
			}
			// The only inference, and only because `IsLand` is a card pool
			// fact -- `deckimport.buildEntries` argues it and this is the same
			// rule for the same reason. Read only for a card being added; one
			// already in the 99 keeps whatever it is filed under.
			category := "utility"
			if rec.IsLand() {
				category = "land"
			}
			wanted = append(wanted, deckedit.BulkCard{Name: name, Qty: line.Qty,
				Category: category, Why: line.Why})
		}
		plan, err = deckedit.PlanBulk(text, wanted)
		if err != nil {
			return err
		}
		// Last, and inside the same lease: the shortlist is the only part of
		// this allowed to fail quietly, exactly as it is on the import page.
		nearby, over = didYouMean(r.Context(), c, unknown)
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		// Without the pool every name is unknown, so the plan would bury the
		// whole deck and add nothing back. Refuse rather than half-do it.
		a.refuseWrite(w, "bulk", rejectf(
			"a bulk edit needs the card pool -- run `mtglab data refresh`"))
		return
	}
	if a.refuseWrite(w, "bulk", err) {
		return
	}

	// **What was read is reported beside what would happen**, and this is
	// where the preview earns its keep: a name the pool could not resolve is
	// also a name the paste did not match, so somewhere in `plan.Entomb` is
	// the card that line was meant to be. Saying both, together, is the whole
	// difference between a burial somebody chose and one that surprised them.
	reading := map[string]any{
		"unknown":              orEmpty(unknown),
		"did_you_mean":         orEmpty(nearby),
		"did_you_mean_skipped": over,
		"read":                 orEmpty(corrections),
		"unreadable":           reportedLines(parsed.Unreadable),
		"skipped":              reportedLines(parsed.Skipped),
		// Lines under a heading that is not the 99 -- a commander, a companion,
		// a sideboard. Read as lines and reported as lines, never applied.
		"outside": reportedLines(outsideThe99(parsed)),
		"notes":   orEmpty(quoted),
	}

	if dryRun {
		out := map[string]any{
			"slug": r.PathValue("slug"), "dry_run": true, "plan": plan,
			// Whether the confirm may be pressed at all: a blocked plan cannot
			// be applied and a plan that changes nothing must not be, so the
			// surface never offers a control that would only refuse.
			"ready": len(plan.Blocked) == 0 && plan.Touches(),
		}
		for k, v := range reading {
			out[k] = v
		}
		wire.JSON(w, http.StatusOK, out)
		return
	}

	// A confirm names the deck it was agreed against. Absent is refused rather
	// than waved through: this route is reachable without a browser, and the
	// preview is the only thing that makes the write a consented one.
	basis := strings.TrimSpace(str(body, "basis"))
	switch {
	case basis == "":
		a.refuseWrite(w, "bulk", rejectf("take a look at what this would change "+
			"before it happens; nothing was written."))
		return
	case basis != plan.Basis:
		a.refuseWrite(w, "bulk", rejectf("this deck changed while the plan was "+
			"on screen, so nothing was written. Take another look and the plan "+
			"will be worked out again against the deck as it is now."))
		return
	}

	updated, err := deckedit.ApplyBulk(text, plan)
	extra := map[string]any{"plan": plan, "entombed": orEmpty(plan.Entomb)}
	for k, v := range reading {
		extra[k] = v
	}
	a.answer(w, r, src, updated, commitOutcome{
		extra: extra,
		// One entry for the whole pass (ADR 28), naming the cards it buried
		// and counting everything else. Never a word of what a rationale says.
		edit: decklog.Edit{Kind: decklog.EditBulk, Cards: plan.Entomb,
			Bulk: &decklog.BulkTally{
				Added: len(plan.Add), Rewrote: len(plan.Rewrite),
				Requantified: len(plan.Requantify), Entombed: len(plan.Entomb)}},
	}, err)
}

// outsideThe99 is every pasted line filed under a heading that is not the deck
// itself, in the order it was pasted.
//
// Reported and never read. A paste carrying a `Commander` or `SIDEBOARD`
// section is usually a whole-deck export -- ours puts the commander in the
// sideboard -- so those lines are somebody's cards and deserve to be told they
// were not applied, rather than to vanish.
func outsideThe99(parsed decklist.List) []decklist.Line {
	out := []decklist.Line{}
	for _, c := range parsed.Cards {
		if c.Section != "deck" {
			out = append(out, decklist.Line{LineNo: c.LineNo, Text: c.Name})
		}
	}
	return out
}

// returnCard is `POST .../graveyard/{name}/return` -- the undo half of ADR 27.
func (a *API) returnCard(w http.ResponseWriter, r *http.Request) {
	src, _, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "return", err) {
		return
	}
	updated, err := deckedit.ReturnCard(text, name)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"returned": name},
		edit:  decklog.Edit{Kind: decklog.EditReturn, Card: name},
	}, err)
}

// exileCard is `DELETE .../graveyard/{name}` -- the only permanent delete
// (ADR 27), and it only ever acts on a card that was already entombed.
func (a *API) exileCard(w http.ResponseWriter, r *http.Request) {
	src, _, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "exile", err) {
		return
	}
	updated, err := deckedit.ExileCard(text, name)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"exiled": name},
		edit:  decklog.Edit{Kind: decklog.EditExile, Card: name},
	}, err)
}

// patchCard is `PATCH .../cards/{name}` -- `service.set_card_field`, the write
// path behind the rationale editor and the one place a `why` can be filled in
// without replacing the card. The value comes from the caller and is written
// as given: nothing here composes, templates, tidies or infers one.
func (a *API) patchCard(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	field, value, ok := namedField(w, body)
	if !ok {
		return
	}
	name := r.PathValue("name")
	entry := findCard(d, name)
	if entry == nil {
		a.refuseWrite(w, "patch-card", rejectf("%s is not in this deck", wire.Quote(name)))
		return
	}
	if field == "category" {
		checked, err := checkCategory(fmt.Sprint(value))
		if a.refuseWrite(w, "patch-card", err) {
			return
		}
		value = checked
	}
	// `art` is checked against the pool here for the same reason
	// `commander_art` is in the deck patch: the editor can tell a printing id
	// from a typo by its shape, and only a query can tell whether that id is a
	// printing *of this card*.
	if field == "art" {
		if id := strings.TrimSpace(fmt.Sprint(value)); id != "" && value != nil {
			err := a.checkPrintingOf(r.Context(), entry.Name, id,
				"the card's own printings list has the ones that are.")
			if a.refuseWrite(w, "patch-card", err) {
				return
			}
		}
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "patch-card", err) {
		return
	}
	updated, err := deckedit.SetCardField(text, entry.Name, field, value)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"card": entry.Name, "field": field},
		edit:  decklog.Edit{Kind: decklog.EditSetCard, Card: entry.Name, Field: field},
	}, err)
}

// patchDeck is `PATCH .../{owner}/{slug}` -- `service.set_deck_field`: stage,
// status, bracket, the pilot, the commander's art, and ADR 37's themes.
func (a *API) patchDeck(w http.ResponseWriter, r *http.Request) {
	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	field, value, ok := namedField(w, body)
	if !ok {
		return
	}
	if field == "commander_art" {
		if id := strings.TrimSpace(fmt.Sprint(value)); id != "" && value != nil {
			if len(d.Commander) == 0 {
				a.refuseWrite(w, "patch-deck",
					rejectf("this deck has no commander, so it has no art to set"))
				return
			}
			err := a.checkPrintingOf(r.Context(), d.Commander[0], id,
				fmt.Sprintf("`GET /api/decks/%s/printings` lists the ones that are.", d.Slug))
			if a.refuseWrite(w, "patch-deck", err) {
				return
			}
		}
	}

	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "patch-deck", err) {
		return
	}
	updated, err := deckedit.SetDeckField(text, field, value)
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"field": field, "value": value},
		edit:  decklog.Edit{Kind: decklog.EditSetDeck, Field: field, Value: value},
	}, err)
}

// setNote is `PUT .../notes/{key}` -- `service.set_note`. Notes are the deck's
// thinking, and they survive regeneration because they live in the source of
// truth rather than in an artifact.
func (a *API) setNote(w http.ResponseWriter, r *http.Request) {
	src, _, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	key := r.PathValue("key")
	text, err := src.ReadText(r.Context(), r.PathValue("slug"))
	if a.refuseWrite(w, "note", err) {
		return
	}
	updated, err := deckedit.SetNote(text, key, str(body, "value"))
	a.answer(w, r, src, updated, commitOutcome{
		extra: map[string]any{"note": strings.TrimSpace(key)},
		edit:  decklog.Edit{Kind: decklog.EditNote, Note: strings.TrimSpace(key)},
	}, err)
}

// ---- the shared checks -----------------------------------------------------

// findCard is `service._find_card`: the 99 and the swap board, case-folded.
// The graveyard is deliberately outside it -- an entombed card is frozen.
func findCard(d *deck.Deck, name string) *deck.CardEntry {
	wanted := strings.ToLower(strings.TrimSpace(name))
	for i := range d.Cards {
		if strings.ToLower(d.Cards[i].Name) == wanted {
			return &d.Cards[i]
		}
	}
	for i := range d.SwapBoard {
		if strings.ToLower(d.SwapBoard[i].Name) == wanted {
			return &d.SwapBoard[i]
		}
	}
	return nil
}

// checkCategory is `service._check_category`: stricter than the gate, which
// only warns, and deliberately so -- the warning is there for hand-written
// files, while an edit through this path is a choice from a list the caller
// was shown. A typo accepted here would quietly cost the comparability that
// the fixed set exists for.
func checkCategory(category string) (string, error) {
	category = strings.ToLower(strings.TrimSpace(category))
	if !reference.IsCategory(category) {
		return "", rejectf("%s is not a category; choose one of %s",
			wire.Quote(category), strings.Join(reference.Deck().Categories, ", "))
	}
	return category, nil
}

// playableCard is the pool check `add_card` and `swap_card` share, and it is
// rule 1 applied to a write: a card nobody looked up is a card whose legality
// is a guess. It has to exist, be legal in Commander, and sit inside the
// commander's colour identity -- which comes from Scryfall's own
// `color_identity`, never from the mana cost (rule 2).
func (a *API) playableCard(ctx context.Context, d *deck.Deck, name, noPool string) (*pool.CardRecord, error) {
	var rec *pool.CardRecord
	var identity map[string]bool
	err := a.usePool(ctx, func(c *pool.Conn) error {
		found, err := c.GetCards(ctx, []string{name})
		if err != nil {
			return err
		}
		rec = found[name]
		if rec == nil {
			return rejectf("%s is not a card the pool knows", wire.Quote(name))
		}
		if !rec.LegalCommander {
			return rejectf("%s is not legal in Commander", rec.Name)
		}
		identity, err = commanderIdentity(ctx, c, d)
		return err
	})
	if errors.Is(err, pool.ErrNoPool) {
		return nil, &editRejected{reason: noPool}
	}
	if err != nil {
		return nil, err
	}
	var outside []string
	for _, colour := range sortedColours(rec.ColorIdentity) {
		if !identity[colour] {
			outside = append(outside, colour)
		}
	}
	if len(outside) > 0 {
		have := strings.Join(sortedKeysOf(identity), "")
		if have == "" {
			have = "C"
		}
		return nil, rejectf("%s's identity {%s} includes {%s}, outside the commander's {%s}",
			rec.Name, strings.Join(sortedColours(rec.ColorIdentity), ""),
			strings.Join(outside, ""), have)
	}
	return rec, nil
}

// commanderIdentity is `service._identity_of`: the union of the commanders'
// own colour identities, and empty for a deck whose commander the pool lacks.
func commanderIdentity(ctx context.Context, c *pool.Conn, d *deck.Deck) (map[string]bool, error) {
	identity := map[string]bool{}
	if len(d.Commander) == 0 {
		return identity, nil
	}
	found, err := c.GetCards(ctx, d.Commander)
	if err != nil {
		return nil, err
	}
	for _, name := range d.Commander {
		rec := found[name]
		if rec == nil {
			continue
		}
		for _, colour := range rec.ColorIdentity {
			identity[colour] = true
		}
	}
	return identity, nil
}

// checkPrintingOf is `service._check_printing_of`: is this id a printing of
// *this card*? Silent when there is no pool, which is the same degraded answer
// every other pool-backed check gives.
func (a *API) checkPrintingOf(ctx context.Context, name, printingID, hint string) error {
	err := a.usePool(ctx, func(c *pool.Conn) error {
		var found string
		err := c.DB().QueryRowContext(ctx,
			"SELECT p.name FROM printings p WHERE p.id = ? AND p.oracle_id ="+
				" (SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1)",
			printingID, name).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return rejectf("%s is not a printing of %s. %s", printingID, name, hint)
		}
		return err
	})
	if errors.Is(err, pool.ErrNoPool) {
		return nil
	}
	return err
}

// namedField reads a PATCH body's `{"field": ..., "value": ...}` pair.
//
// **Named rather than inferred**, which is the shape both PATCH routes have
// always had: the field arrives as a value, not as the key. That matters more
// than it looks -- a body naming a field the editor will not write has to
// reach the editor, because the editor is where the *reason* lives (`archetype`
// says where the label went since ADR 37; anything else names the settable
// list). Inferring the field from the body's keys would turn every one of
// those into "no field given", and the caller would learn nothing.
//
// A body with no `value` at all is refused here, before the editor: `value` is
// the one key whose absence cannot be told from a deliberate blank, and
// clearing a field is a real edit.
func namedField(w http.ResponseWriter, body map[string]any) (string, any, bool) {
	if _, present := body["value"]; !present {
		wire.Detail(w, http.StatusUnprocessableEntity, "value is required")
		return "", nil, false
	}
	return str(body, "field"), plainJSON(body["value"]), true
}

// plainJSON turns `json.Number` into the int or float a caller meant, so the
// editor's own validation sees plain values. A number that is not whole
// stays a float and is refused by the field that cares.
func plainJSON(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = plainJSON(item)
		}
		return out
	default:
		return v
	}
}

func bodyQty(body map[string]any) (int, error) {
	raw, present := body["qty"]
	if !present || raw == nil {
		return 1, nil
	}
	switch v := plainJSON(raw).(type) {
	case int:
		if v == 0 {
			return 1, nil // `int(payload.get("qty") or 1)`: 0 is falsy
		}
		return v, nil
	case float64:
		return 0, fmt.Errorf("invalid literal for int() with base 10: %v", v)
	case string:
		return 0, fmt.Errorf("invalid literal for int() with base 10: %s", wire.Quote(v))
	default:
		return 0, fmt.Errorf("int() argument must be a string or a number")
	}
}

func sortedColours(colours []string) []string {
	out := slices.Clone(colours)
	slices.Sort(out)
	return out
}

func sortedKeysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
