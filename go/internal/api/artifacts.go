package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The rebuild: `POST /api/decks/{owner}/{slug}/artifacts`, over
// `service.build_artifacts` over `artifacts.RenderAll`. The two GETs beside it
// -- the shelf and one deliverable -- flipped with the deck reads in Phase 3
// and live in `decks.go`; `artifactsJSON` below is what all three answer with,
// so the shelf a build returns and the shelf a reader asks for cannot differ.
//
// **A plain route rather than a job, and the number is why.** 70-83ms warm
// across four real decks measured on the deployed instance itself, which is
// the shelf's order of magnitude -- where the argument against a job is that
// submitting and polling costs more than the work. The sibling-duration rule
// that put the dossier and the theme turn into jobs was asked here and
// answered the other way, which is what measuring is for.
//
// **Deliberately not in the activity log.** ADR 28 records edits to
// `deck.yaml` from one call site, and a build changes no deck field -- it
// derives files *from* the deck. Logging it would mean a second call site
// outside `commit`, which CLAUDE.md names as a decision to take deliberately
// rather than by drift. Python took that decision and answered no; this is
// the same no, and it is why this handler does not go through `commit` even
// though it is a write.
//
// Two refusals, and only one of them is forceable. A **draft** is refused by
// the renderer and no flag here reaches it -- the way out is to write the
// rationales and promote it (ADR 13). **Gate errors** are refused by default
// and `force` overrides them, mirroring `mtglab decks build --force`.

// buildArtifacts is `POST .../artifacts` -- `service.build_artifacts`.
func (a *API) buildArtifacts(w http.ResponseWriter, r *http.Request) {
	// The body first, then the deck, because that is the order FastAPI
	// resolves them in: `payload: dict[str, Any]` is validated while the
	// dependencies are being solved, and `lib.source_for(owner)` is not
	// reached until the handler's own first line. So a malformed body is a
	// 422 before any 404 about the deck it was aimed at.
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	force := pyTruthy(body["force"])

	src, d, ok := a.writeTarget(w, r)
	if !ok {
		return
	}
	// **The URL's slug, not the deck's own.** `service.build_artifacts` takes
	// the slug as an argument and passes that to `read_baseline` and
	// `write_artifacts`, while `_artifacts_json` below asks the *deck* for its
	// slug -- so a file whose `slug:` disagrees with its directory is written
	// under one and listed under the other. Every deck in the library has them
	// equal and this has never differed in practice, which is exactly why it
	// is reproduced rather than tidied: a flip is not the place to decide
	// which of two spellings Python meant.
	slug := r.PathValue("slug")

	// **Nil and not empty, which is a difference the gate makes and this has to
	// make with it.** `gate.Validate` reads a nil map and an empty one the same
	// way everywhere except its own early return: nil means the pool was never
	// consulted, so it warns `unverified` once and stops, where an empty map is
	// a pool that has never heard of any of these cards and every one of them
	// comes back `unknown-card`.
	//
	// This built an empty map until 2026-08-22, deliberately, because
	// `service.build_artifacts` wrote `_pool_for(deck, con)` where
	// `service.validate_deck` wrote `_pool_for(deck, con) if con is not None
	// else None` and `_pool_for` answered `{}` for a missing connection -- so on
	// a pool-less instance the validate route warned and this one refused the
	// build outright. The flip reproduced that rather than ruling on it, which
	// is the rule; it has since been ruled on, `_pool_for` answers `None`, and
	// the shape here is the ordinary one every other gate caller in this package
	// already used.
	var cards map[string]*pool.CardRecord
	err := a.withPool(r.Context(), func(c *pool.Conn) error {
		if c == nil {
			return nil
		}
		found, err := deckread.PoolFor(r.Context(), c, d)
		if err != nil {
			return err
		}
		cards = found
		return nil
	})
	if a.refuse(w, "build", err) {
		return
	}
	report := gate.Validate(d, cards, gate.DefaultSize)
	failing := len(report.Errors())
	if failing > 0 && !force {
		wire.Detail(w, http.StatusUnprocessableEntity, fmt.Sprintf(
			"the gate reports %d error(s) on %s. Fix them, or build again "+
				"with force if you know better.", failing, slug))
		return
	}

	// Resolved before anything is written, because the write replaces the
	// snapshot this reads.
	previous, err := a.baselineDeck(r, src, slug)
	if a.refuse(w, "build", err) {
		return
	}

	files, err := artifacts.RenderAll(d, artifacts.Options{Cards: cards, Previous: previous})
	if errors.Is(err, artifacts.ErrDraft) {
		// `BuildRejected(str(exc))` -- the renderer's own sentence, naming the
		// cards still owed a rationale, is what the caller reads.
		wire.Detail(w, http.StatusUnprocessableEntity, artifacts.Message(err))
		return
	}
	if a.refuse(w, "build", err) {
		return
	}

	writer, err := library.WriterFor(src, slug)
	if a.refuseWrite(w, "build", err) {
		return
	}
	if _, err := writer.WriteArtifacts(r.Context(), slug, files); a.refuseWrite(w, "build", err) {
		return
	}

	shelf, err := a.artifactsJSON(r, src, d)
	if a.refuse(w, "build", err) {
		return
	}
	shelf = append(shelf,
		wire.KV{Key: "issues", Value: map[string]any{"errors": report.Errors(), "warnings": report.Warnings()}},
		// `report.errors and force` -- true only when the flag actually
		// overrode something, so a clean build never claims to have been
		// forced.
		wire.KV{Key: "forced", Value: failing > 0 && force})
	raw, err := wire.MarshalOrdered(shelf)
	if a.refuse(w, "build", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// baselineDeck is the snapshot the last build stashed, parsed -- or nothing.
//
// None is ordinary rather than exceptional: a deck built for the first time
// has no baseline and so gets no `swaps.md`, and decks built before the
// snapshot mechanism existed (ADR 30) are in the same position. A snapshot
// that will not parse is the same answer, which is Python's
// `contextlib.suppress(*_UNPARSEABLE)`: the next build simply overwrites it,
// and reporting it as an error would fail a rebuild over the one file a
// rebuild is about to replace.
func (a *API) baselineDeck(r *http.Request, src library.Source, slug string) (*deck.Deck, error) {
	text, present, err := src.ReadBaseline(r.Context(), slug)
	if err != nil || !present {
		return nil, err
	}
	previous, err := deck.FromText(text, slug)
	if err != nil {
		return nil, nil //nolint:nilerr // an unreadable baseline is no baseline
	}
	return previous, nil
}

// artifactsJSON is `service._artifacts_json`: what this deck has been built
// into, and whether that is still true.
//
// `baseline` is the field worth reading -- `current`, `different` or
// `unknown` -- and it is why the shelf route exists at all. Every artifact on
// the volume was eight days older than its deck on 2026-08-21 and nothing in
// the app could say so. It is computed by comparing the stored snapshot
// against the deck rather than by looking at a timestamp, so reverting an edit
// correctly makes the artifacts current again.
func (a *API) artifactsJSON(r *http.Request, src library.Source, d *deck.Deck) ([]wire.KV, error) {
	held, err := src.Artifacts(r.Context(), d.Slug)
	if err != nil {
		return nil, err
	}
	baseline, present, err := src.ReadBaseline(r.Context(), d.Slug)
	if err != nil {
		return nil, err
	}
	list := []wire.OrderedMap{}
	for _, art := range held {
		list = append(list, wire.OrderedMap([]wire.KV{
			{Key: "name", Value: art.Name}, {Key: "size", Value: art.Size}, {Key: "built_at", Value: isoformat(art.BuiltAt)}}))
	}
	return []wire.KV{
		{Key: "artifacts", Value: list},
		{Key: "baseline", Value: baselineState(d, baseline, present)},
		{Key: "buildable", Value: d.Stage != "draft"},
		{Key: "stage", Value: d.Stage},
	}, nil
}
