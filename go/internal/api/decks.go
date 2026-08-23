package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/analyze"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The deck reads (Phase 3's third family): the library shelf and the
// per-deck GETs -- the deck, the gate's verdict, the analysis, the
// suggestions, the commander's panel, the printings, the history and the
// artifacts shelf -- plus `/api/colors/progress`, which scores the library
// against the 32. Every route resolves its source through `Library`
// (ADR 22), so a deck the caller may not see is ErrNotFound -> 404 before a
// row is read, and none of them writes anything.

// appDB is the read-only app.db: the door's handle when auth is on; opened
// lazily when auth is off and the file exists (a laptop that has run an
// edit, say), and left alone when it does not -- reading must not acquire
// a database (`test_the_local_app_touches_no_database`).
func (a *API) appDB() *sql.DB {
	if a.db != nil {
		return a.db
	}
	a.lazy.Lock()
	defer a.lazy.Unlock()
	if a.lazyDB != nil {
		return a.lazyDB
	}
	if a.dbPath == "" {
		return nil
	}
	if _, err := os.Stat(a.dbPath); err != nil {
		return nil
	}
	db, err := auth.Open(a.dbPath)
	if err != nil {
		a.log.Warn("app.db exists but could not be opened read-only", "error", err)
		return nil
	}
	a.lazyDB = db
	return db
}

// library is `api/deps.py:library`: every deck this caller may reach.
func (a *API) library(ctx context.Context) (*library.Library, error) {
	db := a.appDB()
	resolver := library.Resolver{DecksDir: a.decksDir, AppDB: db, AppWriteDB: a.writeDB,
		Maintainer: func(ctx context.Context) (string, error) {
			return library.MaintainerUsername(ctx, db, a.adminEmail)
		}}
	return resolver.For(ctx, auth.ScopeFrom(ctx))
}

// refuse turns the source's errors into the route layer's answers:
// ErrNotFound -> 404 `no deck '<x>'`, ErrArtifactNotFound -> 404
// `no artifact '<x>'`, anything else a 500. Reports whether it answered.
func (a *API) refuse(w http.ResponseWriter, where string, err error) bool {
	if err == nil {
		return false
	}
	var missing library.ErrNotFound
	var noArtifact library.ErrArtifactNotFound
	switch {
	case errors.As(err, &missing):
		wire.Detail(w, http.StatusNotFound, missing.Error())
	case errors.As(err, &noArtifact):
		wire.Detail(w, http.StatusNotFound, noArtifact.Error())
	default:
		a.log.Error("the deck route failed", "route", where, "error", err)
		wire.Detail(w, http.StatusInternalServerError, "the library could not answer that right now")
	}
	return true
}

// sourceFor resolves the owner segment, answering the 404 itself.
func (a *API) sourceFor(w http.ResponseWriter, r *http.Request) (library.Source, bool) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "library", err) {
		return nil, false
	}
	src, err := lib.SourceFor(r.Context(), r.PathValue("owner"))
	if a.refuse(w, "source", err) {
		return nil, false
	}
	return src, true
}

// withPool runs fn with a leased pool, or with nil when there is none --
// the shape every deck route takes: degrade, never fail, when the pool is
// missing.
func (a *API) withPool(ctx context.Context, fn func(c *pool.Conn) error) error {
	err := a.usePool(ctx, fn)
	if errors.Is(err, pool.ErrNoPool) {
		return fn(nil)
	}
	return err
}

// ---- the shelf ----------------------------------------------------------

// listDecks is `GET /api/decks` -- `service.list_library`: every deck this
// caller may see, across every owner, their own first; one pool connection
// for the whole page.
func (a *API) listDecks(w http.ResponseWriter, r *http.Request) {
	lib, err := a.library(r.Context())
	if a.refuse(w, "decks", err) {
		return
	}
	visible, err := lib.Visible(r.Context())
	if a.refuse(w, "decks", err) {
		return
	}
	out := []deckread.Tile{}
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		out = []deckread.Tile{}
		showcase := strings.ToLower(lib.FileOwner())
		for _, owned := range visible {
			decks, err := owned.Source.All(r.Context())
			if err != nil {
				return err
			}
			rows, err := deckread.Tiles(r.Context(), c, decks, owned.Source.Writable(), owned.Owner)
			if err != nil {
				return err
			}
			for i := range rows {
				rows[i].Showcase = strings.ToLower(owned.Owner) == showcase
			}
			out = append(out, rows...)
		}
		return nil
	})
	if a.refuse(w, "decks", err) {
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// ---- one deck -----------------------------------------------------------

// getDeck is `GET /api/decks/{owner}/{slug}` -- `service.get_deck`.
func (a *API) getDeck(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "deck", err) {
		return
	}
	var body []wire.KV
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var err error
		body, err = deckread.DeckPayload(r.Context(), c, d, src.Writable(), r.PathValue("owner"))
		return err
	})
	if a.refuse(w, "deck", err) {
		return
	}
	raw, err := wire.MarshalOrdered(body)
	if a.refuse(w, "deck", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// validateDeck is `GET .../validate` -- `service.validate_deck`.
func (a *API) validateDeck(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "validate", err) {
		return
	}
	var rep *gate.Report
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var err error
		rep, err = deckread.Validate(r.Context(), c, d)
		return err
	})
	if a.refuse(w, "validate", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"ok": rep.OK(), "errors": rep.Errors(), "warnings": rep.Warnings()})
}

// deckStats is `GET .../stats` -- `service.stats_for`.
func (a *API) deckStats(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "stats", err) {
		return
	}
	var stats analyze.Stats
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var err error
		stats, err = deckread.Stats(r.Context(), c, d)
		return err
	})
	if a.refuse(w, "stats", err) {
		return
	}
	wire.JSON(w, http.StatusOK, stats)
}

// suggestions is `GET .../suggestions` -- `service.suggestions_for`:
// replacement shortlists for the cards the gate says have to go, only for
// errors a different card would fix. Reports rather than resolves (ADR 8).
func (a *API) suggestions(w http.ResponseWriter, r *http.Request) {
	var errs []wire.ValidationError
	limit := boundedInt(r.URL.Query(), "limit", 5, 1, 20, &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	d, err := src.Get(r.Context(), slug)
	if a.refuse(w, "suggestions", err) {
		return
	}
	targets := []deckread.SuggestionTarget{}
	err = a.usePool(r.Context(), func(c *pool.Conn) error {
		var err error
		targets, err = deckread.Suggestions(r.Context(), c, d, limit)
		return err
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, map[string]any{"slug": slug, "pool_available": false, "targets": []any{}})
		return
	}
	if a.refuse(w, "suggestions", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"slug": slug, "pool_available": true, "targets": targets})
}

// commanderDossier is `GET .../commander` -- `service.commander_dossier`:
// everything interesting the pool knows about a deck's commander, every
// part of it a query, not a recollection. The payload is built by
// `deckread.CommanderDossier`, which the Claude dossier's brief shares: the
// counted strip on the deck page and the facts the model is handed are one
// query, so the prose underneath the strip cannot disagree with it (ADR 19).
func (a *API) commanderDossier(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "commander", err) {
		return
	}
	var body wire.OrderedMap
	err = a.withPool(r.Context(), func(c *pool.Conn) error {
		var buildErr error
		body, buildErr = deckread.CommanderDossier(r.Context(), c, d)
		return buildErr
	})
	if a.refuse(w, "commander", err) {
		return
	}
	raw, err := wire.MarshalOrdered(body)
	if a.refuse(w, "commander", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// commanderPrintings is `GET .../printings` -- `service.commander_printings`:
// every non-digital printing of the commander (or, with `?card=`, of any
// card the deck holds), newest first; a card the deck does not hold is a
// 422.
func (a *API) commanderPrintings(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "printings", err) {
		return
	}
	name, selected := "", d.CommanderArt
	if vals := r.URL.Query()["card"]; len(vals) > 0 {
		wanted := strings.ToLower(strings.TrimSpace(vals[len(vals)-1]))
		var entry *deck.CardEntry
		for _, list := range [][]deck.CardEntry{d.Cards, d.SwapBoard} {
			for i := range list {
				if strings.ToLower(list[i].Name) == wanted {
					entry = &list[i]
					break
				}
			}
			if entry != nil {
				break
			}
		}
		if entry == nil {
			wire.Detail(w, http.StatusUnprocessableEntity, wire.PyRepr(vals[len(vals)-1])+" is not in this deck")
			return
		}
		name, selected = entry.Name, entry.Art
	} else if len(d.Commander) > 0 {
		name = d.Commander[0]
	}
	printings := []wire.OrderedMap{}
	if name != "" {
		err = a.usePool(r.Context(), func(c *pool.Conn) error {
			rows, err := c.DB().QueryContext(r.Context(), `SELECT p.id, p.set_code, p.set_name, p.collector_number,
				p.rarity, p.released_at, p.promo, p.image_normal, p.price_usd
				FROM printings p
				WHERE p.oracle_id = (SELECT oracle_id FROM oracle_cards WHERE name = ? LIMIT 1)
				  AND p.digital IS NOT TRUE AND p.image_normal IS NOT NULL
				ORDER BY p.released_at DESC NULLS LAST, p.set_code, p.collector_number`, name)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var v [9]any
				ptrs := make([]any, len(v))
				for i := range v {
					ptrs[i] = &v[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return err
				}
				var released *string
				if t, ok := v[5].(time.Time); ok {
					s := t.Format("2006-01-02")
					released = &s
				}
				image := pool.AsStringPtr(v[7])
				id := pool.AsString(v[0])
				printings = append(printings, wire.OrderedMap([]wire.KV{{Key: "id", Value: id}, {Key: "set_code", Value: strings.ToUpper(pool.AsString(v[1]))},
					{Key: "set_name", Value: pool.AsStringPtr(v[2])}, {Key: "collector_number", Value: pool.AsStringPtr(v[3])},
					{Key: "rarity", Value: pool.AsStringPtr(v[4])}, {Key: "released_at", Value: released}, {Key: "promo", Value: pool.AsBool(v[6])},
					{Key: "image", Value: image}, {Key: "art_crop", Value: pool.ArtCropFrom(image)}, {Key: "price_usd", Value: deckread.AsFloatPtr(v[8])},
					{Key: "selected", Value: id == selected}}))
			}
			return rows.Err()
		})
		if errors.Is(err, pool.ErrNoPool) {
			err = nil
		}
		if a.refuse(w, "printings", err) {
			return
		}
	}
	raw, _ := wire.MarshalOrdered([]wire.KV{{Key: "slug", Value: d.Slug}, {Key: "commander", Value: name}, {Key: "selected", Value: selected}, {Key: "printings", Value: printings}})
	wire.Raw(w, http.StatusOK, raw)
}

// deckLog is `GET .../log` -- `service.history_for`: what has been done to
// this deck, newest first (ADR 28); `source.Get` first is the whole
// authorisation check.
func (a *API) deckLog(w http.ResponseWriter, r *http.Request) {
	var errs []wire.ValidationError
	limit := boundedInt(r.URL.Query(), "limit", decklog.DefaultLimit, 1, 500, &errs)
	if len(errs) > 0 {
		wire.Unprocessable(w, errs...)
		return
	}
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug := r.PathValue("slug")
	if _, err := src.Get(r.Context(), slug); a.refuse(w, "log", err) {
		return
	}
	entries, err := decklog.Entries(r.Context(), a.appDB(), src.OwnerID(), slug, limit)
	if a.refuse(w, "log", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"slug": slug, "entries": entries})
}

// baselineState is `service._baseline_state`: the deck against the last
// build's snapshot -- current, different, or unknown.
func baselineState(d *deck.Deck, baseline string, present bool) string {
	if !present {
		return "unknown"
	}
	previous, err := deck.FromText(baseline, d.Slug)
	if err != nil {
		return "unknown"
	}
	if previous.SameAs(d) {
		return "current"
	}
	return "different"
}

// deckArtifacts is `GET .../artifacts` -- `service.list_artifacts`.
//
// A plain read, so it answers for a deck nobody may write: the artifacts are
// the *shareable* surface, and hiding them from a reader who can already see
// the deck would be the wrong way round. The body is `artifactsJSON` in
// `artifacts.go`, which the rebuild answers with too -- `service` shares
// `_artifacts_json` between them for the same reason.
func (a *API) deckArtifacts(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	d, err := src.Get(r.Context(), r.PathValue("slug"))
	if a.refuse(w, "artifacts", err) {
		return
	}
	shelf, err := a.artifactsJSON(r, src, d)
	if a.refuse(w, "artifacts", err) {
		return
	}
	raw, err := wire.MarshalOrdered(shelf)
	if a.refuse(w, "artifacts", err) {
		return
	}
	wire.Raw(w, http.StatusOK, raw)
}

// deckArtifact is `GET .../artifacts/{name}` -- one deliverable, verbatim.
func (a *API) deckArtifact(w http.ResponseWriter, r *http.Request) {
	src, ok := a.sourceFor(w, r)
	if !ok {
		return
	}
	slug, name := r.PathValue("slug"), r.PathValue("name")
	if _, err := src.Get(r.Context(), slug); a.refuse(w, "artifact", err) {
		return
	}
	text, err := src.ReadArtifact(r.Context(), slug, name)
	if a.refuse(w, "artifact", err) {
		return
	}
	wire.JSON(w, http.StatusOK, map[string]any{"name": name, "text": text})
}

// isoformat is `datetime.isoformat()` of a UTC instant with microseconds
// when they are non-zero -- what `Artifact.built_at.isoformat()` writes
// off a file's mtime (`datetime.fromtimestamp(st_mtime, UTC)`).
func isoformat(t time.Time) string {
	t = t.UTC()
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}

// challengeProgress is `GET /api/colors/progress` -- `service.challenge_progress`:
// which of the 32 slots the library has filled, the pool asked once.
func (a *API) challengeProgress(w http.ResponseWriter, r *http.Request) {
	// `Decks` is the instance's file tier (`deps.deck_source`), not the
	// caller's library: the 32 Deck Challenge is scored over the curated six.
	src := library.NewFileSource(a.decksDir, auth.ScopeFrom(r.Context()).IsAdmin)
	filled := map[string][]wire.OrderedMap{}
	havePool := false
	err := a.withPool(r.Context(), func(c *pool.Conn) error {
		if c == nil {
			return nil
		}
		havePool = true
		slugs, err := src.Slugs(r.Context())
		if err != nil {
			return err
		}
		type entry struct {
			slug string
			d    *deck.Deck
		}
		ordered := []entry{}
		wantedSet := map[string]bool{}
		for _, slug := range slugs {
			text, err := src.ReadText(r.Context(), slug)
			if err != nil {
				return err
			}
			d, err := deck.FromText(text, slug)
			if err != nil {
				return err
			}
			if len(d.Commander) == 0 {
				continue
			}
			ordered = append(ordered, entry{slug, d})
			for _, cmd := range d.Commander {
				wantedSet[cmd] = true
			}
		}
		wanted := make([]string, 0, len(wantedSet))
		for n := range wantedSet {
			wanted = append(wanted, n)
		}
		sort.Strings(wanted)
		found := map[string]*pool.CardRecord{}
		if len(wanted) > 0 {
			if found, err = c.GetCards(r.Context(), wanted); err != nil {
				return err
			}
		}
		for _, e := range ordered {
			identity := []string{}
			any := false
			for _, cmd := range e.d.Commander {
				if rec := found[cmd]; rec != nil {
					any = true
					identity = append(identity, rec.ColorIdentity...)
				}
			}
			if !any {
				continue
			}
			key := reference.KeyFor(identity)
			filled[key] = append(filled[key], wire.OrderedMap([]wire.KV{{Key: "slug", Value: e.slug}, {Key: "name", Value: e.d.Name}}))
		}
		return nil
	})
	if a.refuse(w, "colors/progress", err) {
		return
	}
	slots := []wire.OrderedMap{}
	for _, combo := range reference.Colors().Combinations {
		decks := filled[combo.Key]
		if decks == nil {
			decks = []wire.OrderedMap{}
		}
		slots = append(slots, wire.OrderedMap([]wire.KV{{Key: "key", Value: combo.Key}, {Key: "name", Value: combo.Name}, {Key: "tier", Value: combo.Tier}, {Key: "decks", Value: decks}}))
	}
	raw, _ := wire.MarshalOrdered([]wire.KV{{Key: "pool", Value: havePool || len(filled) > 0}, {Key: "filled", Value: len(filled)},
		{Key: "total", Value: len(reference.Colors().Combinations)}, {Key: "slots", Value: slots}})
	wire.Raw(w, http.StatusOK, raw)
}
