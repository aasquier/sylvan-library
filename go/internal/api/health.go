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
//
// **Three of the facts here are about sickness rather than liveness, and the
// status stays 200 for all of them.** `app_db`, `disk_free_mb` and
// `schema_version` answer the two faults that used to leave this route green
// while the site was unusable: a corrupt auth database (every login fails, the
// card pool is fine) and a full volume (every write fails, likewise). They are
// reported in the body and never in the status, because the platform stops
// routing to a machine whose check fails and there is one machine — so a
// failing status turns "logins are broken" into "the site is down". Whoever is
// watching from outside decides what is worth waking somebody for; this route's
// job is to make the decision possible.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	var oracle, printings int64
	var stale bool

	// Read off the box before the pool is touched, so the degraded answer
	// carries these too: an instance with no card pool still has an auth
	// database and a volume, and nothing watching from outside should have to
	// branch on the pool's state to find out whether they are well.
	opens, applied := a.appDBReading()
	sickness := wire.OrderedMap{
		{Key: "app_db", Value: opens},
		{Key: "disk_free_mb", Value: diskFreeMB(a.dataDir)},
		{Key: "schema_version", Value: applied},
	}

	// **A probe, not a visitor.** This route is what Fly polls from outside the
	// container and what the image's own `HEALTHCHECK` polls from inside, both
	// every thirty seconds and neither aware of the other — and an ordinary
	// lease from either one keeps the card pool's file open for ten seconds
	// after it has finished with it. Two of those, out of phase, held the file
	// almost continuously and left `mtglab data refresh` racing for a window
	// under two seconds wide. [pool.Pool.UseWithoutHolding] carries the
	// measurement; the short of it is that the twenty milliseconds this costs
	// is worth less than a refresh that cannot get in.
	err := a.poolForAProbe(r.Context(), func(c *pool.Conn) error {
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
		body := wire.OrderedMap{
			{Key: "pool", Value: false},
			{Key: "oracle_cards", Value: 0},
			{Key: "printings", Value: 0},
		}
		body = append(body, sickness...)
		wire.JSON(w, http.StatusOK, append(body,
			wire.KV{Key: "message", Value: noPoolMessage}))
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
	body = append(body, sickness...)
	if stale {
		body = append(body, wire.KV{Key: "message",
			Value: "pool predates the printed stats or the painters -- " +
				"run `mtglab data refresh`"})
	}
	wire.JSON(w, http.StatusOK, body)
}

// diskFreeMB is the data directory's headroom in whole megabytes, or nil when
// there is nothing to ask — no data directory configured, or a statfs that
// answered nothing.
//
// **Never 0 for the unanswered case**, which is the only interesting line in
// this function. [diskUsage] reports a failure as three zeros, and a monitor
// reading `disk_free_mb: 0` would page somebody at four in the morning for a
// full volume that is really a question nobody asked. A total of zero is how
// that failure is told apart from a real reading: no volume this app runs on
// has a zero-byte total.
func diskFreeMB(dataDir string) any {
	if dataDir == "" {
		return nil
	}
	total, _, free := diskUsage(dataDir)
	if total <= 0 {
		return nil
	}
	return free / (1 << 20)
}
