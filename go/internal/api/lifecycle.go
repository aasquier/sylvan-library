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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckedit"
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
// /api/decks/import`, `DELETE /api/decks/{owner}/{slug}`, `PUT
// .../shared`, and the crypt's three -- `GET /api/decks/entombed`, `POST
// /api/decks/entombed/{id}/return` and `DELETE /api/decks/entombed` -- the
// seven lifecycle routes: create, import and delete, the source's own sharing
// verb, the way back out of a deletion, and the way to make one final.
//
// Kept apart from the eleven editing routes on purpose, and the reason is
// visible in the shapes below: **none of these goes through `commit`.**
// Creation and deletion are deliberately
// outside ADR 28's activity log, which is a decision rather than an oversight
// -- adding them means a second call site, and the log's whole design is that
// there is one. Sharing is outside it for the same reason and one more: it
// changes who may *read* a deck, not what the deck is. A restore is outside
// it because it is the undo of something the log never recorded, and emptying
// the crypt is outside it because a deck's own history cannot outlive the
// deck.
//
// Five of the seven write into the caller's **own** library and never into an
// owner named in the path (no ownership question exists for
// them), so they resolve the caller rather than a path segment -- `Mine` for
// create and import, `myCrypts` for the crypt's three, which is `Mine` widened
// to every library the caller may write and argued where it stands. The other
// two take the owner segment like every other per-deck route, and inherit
// ADR 22's answers with it: a deck the caller cannot see is absent from their
// source and every verb against it is a 404 before writability is consulted.

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
	// Who drafted the quoted reasons in the paste (ADR 49). One hand ever
	// drafts, so one value is ever legal -- anything else is a claim the
	// deck file has no way to record, refused rather than stored.
	whyBy := strings.TrimSpace(str(body, "why_by"))
	if whyBy != "" && whyBy != deckedit.DraftedBy {
		wire.Detail(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"why_by can only be %q -- it names the hand that drafted the "+
				"quoted reasons, and only one hand ever drafts", deckedit.DraftedBy))
		return
	}

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
		// Which reading of a trailing quoted run the pool has, decided before
		// anything is spelled or built. It runs first because `Respell` must
		// never be handed a name with somebody's sentence still glued to it:
		// scoring that against 35,393 names is asking the wrong question, and
		// a reported correction from it would be a correction nobody made.
		read, chosen := deckimport.ReadRationales(parsed, found)
		wanted = deckimport.NamesIn(read, commander, companion)
		// Before the deck is built, so a corrected card is simply a card by
		// the time anything downstream sees it -- the count, the category,
		// the colour identity and the gate all read the real record.
		corrections, err := deckimport.Respell(r.Context(), poolReader{c}, wanted, found)
		if err != nil {
			return err
		}
		built, err := deckimport.BuildDeck(read, found, deckimport.Options{
			Slug: slug, Name: str(body, "name"), Commander: commander,
			Companion: companion, Bracket: bracket, Status: status,
			Read: corrections, Notes: chosen, WhyBy: whyBy})
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
		"commander": orEmpty(d.Commander), "companion": companionOrNil(d.Companion),
		"total_cards": d.TotalCards(), "land_count": d.LandCount(),
		"swap_board": orEmpty(swaps), "needs_rationale": report.NeedsRationale(),
		// How many reasons the paste carried in its own quoted column,
		// beside how many are still owed. Two numbers rather than one
		// because a person who wrote 60 of them should be told that, and
		// "40 still owed" on its own reads like nothing arrived.
		"rationales": report.Rationales,
		// Echoed so a dry-run preview can say the reasons will land signed,
		// from the answer rather than from what the page remembers asking.
		"why_by":  whyBy,
		"unknown": orEmpty(report.Unknown),
		// The shortlist beside the misses, and how many misses did not get
		// one. Never applied here: see `didYouMean`.
		"did_you_mean":         orEmpty(nearby),
		"did_you_mean_skipped": over,
		// What was misspelled and what it was read as. On the wire in its own
		// right as well as in `notes`, so the page can put it where somebody
		// will look rather than at the end of a list of remarks.
		"read":       orEmpty(report.Read),
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
// that refuses, and a deck that **moves** rather than vanishing.
//
// The answer used to carry *where* it moved to, and that was the whole way
// back: a filesystem path, rendered to the player, under a sentence telling
// them to open a shell. It carries a crypt id now -- an opaque handle the
// route below turns back into a deck -- and `recoverable` beside it, because
// "deleted" and "recoverable" have to be separately true and separately
// visible, which was the good half of the old answer's argument.
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

	if _, err := writer.Delete(r.Context(), slug); a.refuseWrite(w, "delete", err) {
		return
	}
	// The id the crypt itself hands out, found by asking it rather than by
	// deriving one from what `Delete` returned: which handle names this
	// burial is the tier's business, and one round trip through the crypt is
	// what keeps that knowledge from being copied up here.
	//
	// An empty id is not an error. It means the crypt could not be read back
	// in this instant, and the surface has to say "it is in the crypt" without
	// offering the one-click way out -- which is true, and is not the same
	// claim as "there is no way back".
	id := ""
	if crypt, err := library.CryptFor(src); err == nil {
		if entombed, err := crypt.Entombed(r.Context()); err == nil {
			for _, e := range entombed {
				if e.Slug == slug {
					id = e.ID
					break
				}
			}
		} else {
			a.log.Warn("the crypt could not be read after a delete", "slug", slug, "error", err)
		}
	}
	wire.JSON(w, http.StatusOK, map[string]any{
		"slug": slug, "name": d.Name, "deleted": true,
		// Two fields rather than one: whether it can be raised again, and the
		// handle that raises it.
		"recoverable": id != "", "crypt_id": id,
		"total_cards": d.TotalCards(),
		"stage":       d.Stage, "status": d.Status,
	})
}

// deleteWord is `service.DELETE_WORD`.
//
// "Bury" is Magic's own retired templating for destroying a permanent so that
// it cannot regenerate, and it is the right verb here for the reason the
// obvious alternatives are the wrong ones: the deck goes to the crypt and can
// be raised again, so "exile" -- which in Magic means gone for good -- would
// misdescribe what this does.
const deleteWord = "bury"

// ---- the crypt -------------------------------------------------------------

// The routes that make a deletion survivable, and the reason they exist: the
// deck level had ADR 27's *entomb* and none of its *graveyard*. What a
// player was given instead was the path the deck had been moved to and the
// suggestion that they go and get it -- a leak of what runs underneath this
// site (commandment 10) offered as a feature, and, to anyone who has never
// opened a terminal, not an offer at all.
//
// The third route is the one that makes a deletion *final*, and it belongs
// with the other two rather than beside `deleteDeck`: it is about the crypt
// as a room, not about any deck in it. A place decks only ever accumulate in
// eventually stops being a safety net and starts being a drawer nobody opens.
//
// **None of them takes an owner segment.** Your crypt is yours -- resolved
// from who is asking, like create and import -- so there is no path a caller
// can write that names somebody else's, which settles ADR 5 for this family by
// construction rather than by a check: an entombed deck is still somebody's,
// and the only 404 available here is about your own.

// listEntombed is `GET /api/decks/entombed`: the caller's crypt, newest
// first.
//
// `entombed_at` is null rather than absent when nothing recorded the burial
// -- an entry somebody put on the volume by hand. A missing timestamp
// rendered as a date would be this repo's most-repeated bug (a fallback that
// reads as a fact) in the one place a player looks to check that their deck
// is still there.
func (a *API) listEntombed(w http.ResponseWriter, r *http.Request) {
	crypts, ok := a.myCrypts(w, r)
	if !ok {
		return
	}
	entombed := []library.Entombed{}
	for _, c := range crypts {
		found, err := c.crypt.Entombed(r.Context())
		if a.refuseWrite(w, "crypt", err) {
			// One crypt that will not answer fails the whole list. Returning
			// the half that did would be a list of "everything you buried"
			// with something missing from it, which is the worst answer
			// available on this screen.
			return
		}
		entombed = append(entombed, found...)
	}
	// Newest first across all of them. Each crypt sorts its own; merging two
	// sorted lists by concatenation does not, and the entry somebody just lost
	// has to be the one on top.
	sort.SliceStable(entombed, func(i, j int) bool { return entombed[i].At.After(entombed[j].At) })
	out := []map[string]any{}
	for _, e := range entombed {
		row := map[string]any{
			"id": e.ID, "slug": e.Slug, "name": e.Name,
			"total_cards": e.Cards, "commander": orEmpty(e.Commander),
			"entombed_at": nil,
		}
		if !e.At.IsZero() {
			row["entombed_at"] = e.At.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	wire.JSON(w, http.StatusOK, map[string]any{"entombed": out})
}

// restoreDeck is `POST /api/decks/entombed/{id}/return` -- the deck-level
// twin of the graveyard's `return`, and named the same verb on purpose:
// ADR 27 taught the word for a card, and a player who has entombed one has
// already been taught how this ends.
//
// A restore never renames (`FileSource.Restore` argues why), so the one
// refusal that is about the library rather than about the caller is a slug a
// living deck already holds. That answers 422 with the question only the
// player can settle, and never with a rename nobody asked for.
func (a *API) restoreDeck(w http.ResponseWriter, r *http.Request) {
	crypts, ok := a.myCrypts(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	for _, c := range crypts {
		slug, err := c.crypt.Restore(r.Context(), id)
		// Not this crypt's: ask the next one. An id nobody holds falls out of
		// the loop, which is the only 404 this route has.
		if library.IsNotFound(err) {
			continue
		}
		var exists library.ErrExists
		if errors.As(err, &exists) {
			a.refuseWrite(w, "restore", rejectf(
				"a deck called %s is already on your shelf, and a deck always comes "+
					"back under its own name. Entomb or rename that one first, and this "+
					"deck can have its name back.", wire.Quote(exists.Slug)))
			return
		}
		if a.refuseWrite(w, "restore", err) {
			return
		}
		d, err := c.src.Get(r.Context(), slug)
		if a.refuseWrite(w, "restore", err) {
			return
		}
		wire.JSON(w, http.StatusOK, map[string]any{
			"slug": slug, "name": d.Name, "restored": true,
			"total_cards": d.TotalCards(), "stage": d.Stage, "status": d.Status,
		})
		return
	}
	// Its own sentence rather than `refuse`'s: that one names the thing that
	// was not found, and what was not found here is a handle no player has
	// ever seen or typed.
	wire.Detail(w, http.StatusNotFound,
		"that deck is not in your crypt -- it may already be back on your shelf")
}

// emptyCrypt is `DELETE /api/decks/entombed`: the crypt swept clean, and the
// only route in this app that destroys a deck.
//
// It exists because a crypt is the one room here that only ever fills up.
// Every other list a player keeps has a way to get smaller; this one grew by
// one every time somebody changed their mind, forever, and the way to shorten
// it was to raise a deck you did not want in order to bury it again -- which
// does not shorten it either. A holding area with no drain is a holding area
// people stop opening.
//
// **`exile` is the confirmation word, and it is the exact opposite of the one
// next door on purpose.** `deleteDeck` takes `bury` and argues, where it
// stands, that "exile" would be wrong there because in Magic exile means gone
// for good and an entombed deck comes back. Here nothing comes back, so the
// word this project already refused to misuse is the correct one at last. A
// player who has read either dialog has been taught the difference by the
// game's own vocabulary rather than by a warning label.
//
// The confirmation is a typed word rather than a flag for the reason
// `deleteDeck` gives: a boolean is answered identically by somebody who read
// the screen and somebody who clicked through it. Unlike the delete it does
// **not** also accept a slug -- there is no one deck being named here, which
// is the entire hazard.
func (a *API) emptyCrypt(w http.ResponseWriter, r *http.Request) {
	crypts, ok := a.myCrypts(w, r)
	if !ok {
		return
	}
	typed := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("confirm")))
	if typed != emptyWord {
		a.refuseWrite(w, "empty-crypt", rejectf(
			"to empty your crypt, confirm by typing %s. Got %s. Everything in "+
				"the crypt is destroyed and no deck comes back from this, which is "+
				"why it is not a yes/no.",
			wire.Quote(emptyWord), wire.Quote(r.URL.Query().Get("confirm"))))
		return
	}

	// Emptied one crypt at a time, and a failure part-way is reported rather
	// than swallowed -- with the count that did go, because by then those
	// decks are already gone and an error implying otherwise would be the
	// worst sentence available here. The surface re-reads the crypt on both
	// paths, so what is left is what the player sees.
	destroyed := 0
	for _, c := range crypts {
		gone, err := c.crypt.Empty(r.Context())
		destroyed += gone
		if err != nil {
			a.log.Warn("the crypt could not be emptied",
				"actor", a.actor(r.Context()), "destroyed", destroyed, "error", err)
			a.refuseWrite(w, "empty-crypt", err)
			return
		}
	}
	// Recorded on the server and nowhere else. ADR 28's log is a deck's own
	// history and these decks no longer have one -- but an operation with no
	// undo should leave a trace somewhere a maintainer can find it, and the
	// player is told the count on screen rather than in a log.
	a.log.Info("the crypt was emptied", "actor", a.actor(r.Context()), "destroyed", destroyed)
	wire.JSON(w, http.StatusOK, map[string]any{"emptied": true, "destroyed": destroyed})
}

// emptyWord is the crypt's own confirmation, and the counterpart to
// `deleteWord`.
//
// Magic's "exile" is removal from the game with no way back, which is exactly
// what this does and exactly what burying a deck does not. The two words are
// the game's own distinction between the reversible and the final, and using
// them for the two operations means the vocabulary is doing the teaching.
const emptyWord = "exile"

// ownedCrypt is one crypt the caller may open, with the source behind it: a
// restore has to read the deck it just raised, and asking the library twice
// for the same half would leave two answers that could disagree.
type ownedCrypt struct {
	crypt library.Crypt
	src   library.Source
}

// myCrypts is every crypt this caller may open -- the crypts of the libraries
// they can *write* -- answering the refusal itself.
//
// Almost always exactly one, and "almost always" is the word that makes a
// button which cannot work. `Mine` alone would be wrong for one real
// arrangement: an admin on an instance with no maintainer configured writes
// the file tier through the owner segment (ADR 17) while `Mine` answers with
// their own rows, so a deck they buried through one would have been raised
// through the other, and the notice on screen would have offered a handle
// this route could not find.
//
// Derived from `Visible` rather than listed, for the same reason the door's
// sweeps derive from the route table: a tier added later is included without
// anybody remembering to. **Writability is the whole test** -- a deck you may
// read is not a deck you may bury, and somebody else's shared shelf has no
// crypt to show you (`CryptFor` refuses one, and `SharedOnly` has none at
// all).
func (a *API) myCrypts(w http.ResponseWriter, r *http.Request) ([]ownedCrypt, bool) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "crypt", err) {
		return nil, false
	}
	visible, err := lib.Visible(r.Context())
	if a.refuse(w, "crypt", err) {
		return nil, false
	}
	out := []ownedCrypt{}
	for _, owned := range visible {
		if !owned.Source.Writable() {
			continue
		}
		crypt, err := library.CryptFor(owned.Source)
		if err != nil {
			continue
		}
		out = append(out, ownedCrypt{crypt: crypt, src: owned.Source})
	}
	if len(out) == 0 {
		// No library of your own is not an empty crypt, and it is not a
		// server fault either: it is the answer a caller with nothing to write
		// gets, in the same words every other write refusal uses.
		a.refuseWrite(w, "crypt", library.ErrReadOnly{})
		return nil, false
	}
	return out, true
}

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

// ---- the night games -------------------------------------------------------

// setDeckColiseumAtNight is
// `PUT /api/decks/{owner}/{slug}/coliseum-at-night`: the owner entering one
// deck for the games the arena will run after dark, or withdrawing it.
//
// **Nothing schedules anything on the strength of this yet**, and the route
// exists anyway because consent recorded in advance is the only kind worth
// having here: the first night has to be played with decks whose owners chose
// them. The copy on the settings page carries that honestly; this route just
// records the answer.
//
// Shaped on `setDeckShared` above, refusal for refusal, with one addition it
// does not have: a tier with nowhere to keep this answers ErrNoNightGames,
// which is a 422 rather than a 403 because it is a fact about the deck and not
// about the caller (`library.ErrNoNightGames`).
func (a *API) setDeckColiseumAtNight(w http.ResponseWriter, r *http.Request) {
	// The body first, then the deck -- `setDeckShared`'s recorded order, kept
	// so the two flags cannot answer a malformed request differently.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	raw, given := body[nightField]
	if !given {
		wire.Detail(w, http.StatusUnprocessableEntity, nightField+" is required")
		return
	}
	src, _, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	writer, err := library.WriterFor(src, slug)
	if a.refuseWrite(w, nightField, err) {
		return
	}
	if err := writer.SetColiseumAtNight(r.Context(), slug, truthy(raw)); a.refuseWrite(w, nightField, err) {
		return
	}
	a.getDeck(w, r)
}

// The two body keys, named once. They are also the two path tails, hyphenated
// -- `shared` and `coliseum-at-night` -- which is the house's split: paths
// wear hyphens, wire keys wear underscores.
const (
	sharedField = "shared"
	nightField  = "coliseum_at_night"
)

// setEveryDeckShared and setEveryDeckColiseumAtNight are the two master
// controls: `PUT /api/decks/shared` and `PUT /api/decks/coliseum-at-night`.
//
// **Neither takes an owner segment**, which is the crypt's argument (see
// `listEntombed`) rather than a shortcut: the decks these reach are the
// caller's own, resolved from who is asking, so there is no path anybody can
// write that turns every deck in somebody else's library on or off. ADR 5 is
// settled here by construction, and that matters more on a route that writes
// a whole shelf at once than on one that writes a single deck.
func (a *API) setEveryDeckShared(w http.ResponseWriter, r *http.Request) {
	a.setEveryDeckFlag(w, r, sharedField, library.Writer.SetShared)
}

func (a *API) setEveryDeckColiseumAtNight(w http.ResponseWriter, r *http.Request) {
	a.setEveryDeckFlag(w, r, nightField, library.Writer.SetColiseumAtNight)
}

// setEveryDeckFlag writes one flag across every deck the caller may write.
//
// **It loops the same per-deck verb the single-deck route calls**, rather than
// issuing one UPDATE across the shelf. That is deliberate and costs a
// statement per deck: the verb is where each tier's rule about this flag
// lives -- the file tier's YAML surgery for `shared`, its refusal for the
// night games -- so a bulk path that went around it would be a second
// implementation free to disagree with the first. Libraries here are tens of
// decks, not thousands.
//
// **The first refusal stops the run**, and the honest consequence is written
// down rather than hidden: a refusal that arrives partway leaves the decks
// before it written. In the one case that actually refuses -- the file tier
// having nowhere to keep the night flag -- it arrives on the first deck and
// nothing is written at all. The client re-reads the shelf afterwards either
// way, so what it renders is the truth rather than what it hoped for.
func (a *API) setEveryDeckFlag(w http.ResponseWriter, r *http.Request, field string,
	set func(library.Writer, context.Context, string, bool) error) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	raw, given := body[field]
	if !given {
		wire.Detail(w, http.StatusUnprocessableEntity, field+" is required")
		return
	}
	value := truthy(raw)
	owned, ok := a.myWritableLibraries(w, r)
	if !ok {
		return
	}
	changed := 0
	for _, o := range owned {
		writer, err := library.WriterFor(o.Source, "")
		if a.refuseWrite(w, field, err) {
			return
		}
		// Slugs rather than All: this writes a flag and reads no deck, so
		// parsing ninety-nine cards apiece to find out what to name is work
		// nobody asked for.
		slugs, err := o.Source.Slugs(r.Context())
		if a.refuse(w, field, err) {
			return
		}
		for _, slug := range slugs {
			if err := set(writer, r.Context(), slug, value); a.refuseWrite(w, field, err) {
				return
			}
			changed++
		}
	}
	// What was asked for and how many decks wear it now. The client re-reads
	// the shelf regardless -- this is the receipt, not the new state.
	wire.JSON(w, http.StatusOK, map[string]any{field: value, "changed": changed})
}

// myWritableLibraries is every library this caller may write, which is exactly
// their own: `Visible` filtered by `Writable`, the same derivation `myCrypts`
// uses and for the same reason -- a tier added later is included without
// anybody remembering to.
//
// A caller with nothing to write gets the ordinary read-only refusal rather
// than a 200 over an empty list, so "the master switch did nothing" is never
// reported as success.
func (a *API) myWritableLibraries(w http.ResponseWriter, r *http.Request) ([]library.Owned, bool) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "decks", err) {
		return nil, false
	}
	visible, err := lib.Visible(r.Context())
	if a.refuse(w, "decks", err) {
		return nil, false
	}
	out := []library.Owned{}
	for _, owned := range visible {
		if owned.Source.Writable() {
			out = append(out, owned)
		}
	}
	if len(out) == 0 {
		a.refuseWrite(w, "decks", library.ErrReadOnly{})
		return nil, false
	}
	return out, true
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

// orEmpty is a nil slice rendered as `[]` and never as `null`.
//
// Generic since 2026-08-28, and the reason is a bug this had already caused:
// `notes` went through here and `read` did not, so an import where nothing
// needed respelling sent `read: null` -- `Respell` returns a nil slice when it
// makes no corrections -- against a wire type declaring `Correction[]`. The
// page then read `.length` off null and the whole result panel went to the
// error boundary. Every clean import, which is most of them.
//
// Nothing caught it because nothing could: the Go tests assert on the report
// and not on the JSON, and the page's tests build their own fixture, which of
// course has an empty array in it. It took pasting a list with no typos in a
// browser. So the fix is the general one -- every list-shaped field on this
// response goes through here now, whatever its element type.
func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
