package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/aasquier/sylvan-library/go/internal/library"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// health is `GET /api/health`: the platform's health-check
// target, which must not 500 on any state an instance can actually be in. A
// missing pool is a correct state between deploy and seeding, answered in the
// degraded shape; a stale pool is reported so the app can say "re-ingest"
// instead of showing every creature as statless (`pool.Stale`).
//
// `decks` counts the file tier for every caller — the curated library
// regardless of who asks, deliberately: this route is on
// `door.PublicPaths`, so the platform's anonymous probe and a signed-in
// browser read the same number.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	var oracle, printings int64
	var stale bool
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		var countErr error
		if oracle, countErr = pool.Count(r.Context(), c.DB(), "oracle_cards"); countErr != nil {
			return countErr
		}
		if printings, countErr = pool.Count(r.Context(), c.DB(), "printings"); countErr != nil {
			return countErr
		}
		stale, countErr = pool.Stale(r.Context(), c)
		return countErr
	})
	if errors.Is(err, pool.ErrNoPool) {
		wire.JSON(w, http.StatusOK, wire.OrderedMap{
			{Key: "pool", Value: false},
			{Key: "oracle_cards", Value: 0},
			{Key: "printings", Value: 0},
			{Key: "message", Value: noPoolMessage},
		})
		return
	}
	if a.refuse(w, "health", err) {
		return
	}

	// `config.SCRYFALL_DIR`, not a relative literal: under MTGLAB_DATA_DIR
	// a hardcoded `data/scryfall` resolves against the working directory
	// instead of the volume — a fully seeded instance once reported no bulk
	// files at all. The path arrives through Config for the same reason.
	files := []string{}
	if a.scryfallDir != "" {
		matches, _ := filepath.Glob(filepath.Join(a.scryfallDir, "*.jsonl.gz"))
		sort.Strings(matches)
		for _, m := range matches {
			files = append(files, filepath.Base(m))
		}
	}

	slugs, err := library.NewFileSource(a.decksDir, false).Slugs(r.Context())
	if a.refuse(w, "health", err) {
		return
	}

	body := wire.OrderedMap{
		{Key: "pool", Value: true},
		{Key: "oracle_cards", Value: oracle},
		{Key: "printings", Value: printings},
		{Key: "bulk_files", Value: files},
		{Key: "decks", Value: len(slugs)},
		{Key: "pool_stale", Value: stale},
	}
	if stale {
		body = append(body, wire.KV{Key: "message",
			Value: "pool predates the printed stats or the painters -- " +
				"run `mtglab data refresh`"})
	}
	wire.JSON(w, http.StatusOK, body)
}
