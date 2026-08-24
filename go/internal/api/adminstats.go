// The admin dashboard's numbers — `api/adminstats.py`, the last route family
// to cross (Phase 8). Everything lives under the admin prefix the middleware
// refuses before routing (ADR 17), and every handler also asks
// `requireAdmin`, the same two walls Python keeps.
//
// All the views are facts about this box, read from the box. Two couplings
// dissolved to let them cross: the job registry census is the door's own now
// (no Python route creates a job any more, so uvicorn's registry is always
// empty and this one is the honest total), and `_rss` reports the process on
// the port — which during the last of the coexistence is the door rather
// than the pair, and at retirement is simply the process.

package api

import (
	"database/sql"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/prices"
	"github.com/aasquier/sylvan-library/go/internal/tiers"
	"github.com/aasquier/sylvan-library/go/internal/wire"

	// The stats driver for the read-only schema peek; registered by the auth
	// package's import already, named here for clarity.
	_ "modernc.org/sqlite"
)

// statsSystem is `GET /api/admin/stats/system`: the process and the machine
// under it, as the box reports them.
func (a *API) statsSystem(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	rssBytes, rssKind := processRSS()
	memTotal, memAvailable := machineMemory()
	loads := loadAverages()
	loadOut := make([]any, 0, len(loads))
	for _, l := range loads {
		loadOut = append(loadOut, floats.Float(l))
	}
	total, used, free := diskUsage(a.dataDir)
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "schema", Value: wire.OrderedMap{
			{Key: "applied", Value: a.schemaApplied()},
			{Key: "expected", Value: auth.SchemaVersion},
		}},
		{Key: "process", Value: wire.OrderedMap{
			{Key: "bytes", Value: rssBytes},
			{Key: "kind", Value: rssKind},
		}},
		{Key: "memory", Value: wire.OrderedMap{
			{Key: "total_bytes", Value: intOrNil(memTotal)},
			{Key: "available_bytes", Value: intOrNil(memAvailable)},
		}},
		{Key: "load", Value: loadOut},
		{Key: "cpus", Value: runtime.NumCPU()},
		{Key: "disk", Value: wire.OrderedMap{
			{Key: "path", Value: a.dataDir},
			{Key: "total_bytes", Value: total},
			{Key: "used_bytes", Value: used},
			{Key: "free_bytes", Value: free},
		}},
	})
}

// schemaApplied reads `user_version` off `app.db` through a bare read-only
// connection — never through the ladder, because a stats panel that could
// change the schema by being looked at is not a stats panel. `mode=ro`
// refuses to create the file and the exists() check keeps a bare laptop
// from logging a warning per refresh; the redundancy is deliberate, exactly
// as `adminstats._schema` argues it.
func (a *API) schemaApplied() any {
	if a.dbPath == "" {
		return nil
	}
	if _, err := os.Stat(a.dbPath); err != nil {
		return nil
	}
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(a.dbPath)+"?mode=ro")
	if err != nil {
		a.log.Warn("could not read app.db schema version", "error", err)
		return nil
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		a.log.Warn("could not read app.db schema version", "error", err)
		return nil
	}
	return version
}

// statsStorage is `GET /api/admin/stats/storage`: what is on the volume,
// named the way the architecture names it. Sizes are bytes or null — null
// meaning "nothing there yet", which on a fresh instance is most of them
// and is information, not an error.
func (a *API) statsStorage(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	cache := filepath.Join(a.dataDir, "cache")
	cacheBytes := sizeOf(cache)
	wire.JSON(w, http.StatusOK, wire.OrderedMap{
		{Key: "app_db_bytes", Value: intOrNil(sizeOf(a.dbPath))},
		{Key: "pool_bytes", Value: intOrNil(sizeOf(a.poolPath))},
		{Key: "scryfall_bulk_bytes", Value: intOrNil(sizeOf(a.scryfallDir))},
		{Key: "cache_bytes", Value: intOrNil(cacheBytes)},
		{Key: "cache", Value: cacheBreakdown(cache, cacheBytes)},
		{Key: "decks", Value: wire.OrderedMap{
			{Key: "count", Value: countDirs(a.decksDir)},
			{Key: "bytes", Value: intOrNil(sizeOf(a.decksDir))},
			{Key: "trashed", Value: countDirs(filepath.Join(a.decksDir, ".trash"))},
		}},
	})
}

// cacheBreakdown names the three shelves and whatever is left over as
// `other_bytes` — the part that matters, because this panel once shipped
// naming two of three tenants while the reading engine was 38% of the
// cache. A remainder cannot hide a fourth shelf the way a fixed list can.
func cacheBreakdown(cache string, total *int64) wire.OrderedMap {
	symbols := sizeOf(filepath.Join(cache, "symbols"))
	cardmotion := sizeOf(filepath.Join(cache, "cardmotion"))
	ocr := sizeOf(filepath.Join(cache, "ocr"))
	var other any
	if total != nil {
		named := int64(0)
		for _, v := range []*int64{symbols, cardmotion, ocr} {
			if v != nil {
				named += *v
			}
		}
		rest := *total - named
		if rest < 0 {
			rest = 0
		}
		other = rest
	}
	return wire.OrderedMap{
		{Key: "symbols_bytes", Value: intOrNil(symbols)},
		{Key: "cardmotion_bytes", Value: intOrNil(cardmotion)},
		{Key: "ocr_bytes", Value: intOrNil(ocr)},
		{Key: "other_bytes", Value: other},
	}
}

// sizeOf is `_size_of`: bytes on disk, or nil for "nothing there".
func sizeOf(path string) *int64 {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().IsRegular() {
		size := info.Size()
		return &size
	}
	if !info.IsDir() {
		return nil
	}
	var total int64
	err = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // a vanished entry is skipped, as rglob skips it
		}
		if fi, err := d.Info(); err == nil && fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return &total
}

func countDirs(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}

func intOrNil(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// statsClaude is `GET /api/admin/stats/claude`: where the Claude tokens
// went — per mode, per model, and in dollars. Two axes per window because
// they answer different questions; the dollar figure is estimated from the
// per-model rollup only, since pricing per-mode rows would price
// `(various)` and the guess would look like arithmetic. The caveat rides in
// the payload because it must ride with the numbers anywhere they go.
func (a *API) statsClaude(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	// An instance with no app.db answers the shapes Python answers over the
	// empty database `db.connection()` would mint there: empty roll-ups,
	// never a 404 — the stats are about the instance, not about an account.
	db, present := a.accountsDB()
	rec := a.claudeLedger
	if rec == nil && present {
		rec = ledger.RecorderFrom(db, a.log)
	}
	window := func(since string) (wire.OrderedMap, error) {
		byMode, byModel := []ledger.Summary{}, []ledger.Summary{}
		if rec != nil {
			var err error
			if byMode, err = rec.Summarise(r.Context(), "mode", since); err != nil {
				return nil, err
			}
			if byModel, err = rec.Summarise(r.Context(), "model", since); err != nil {
				return nil, err
			}
		}
		priceRows := make([]prices.Row, 0, len(byModel))
		for _, row := range byModel {
			priceRows = append(priceRows, prices.Row{Model: row.Model,
				Conversations: int64(row.Conversations),
				InputTokens:   int64(row.InputTokens),
				OutputTokens:  int64(row.OutputTokens),
				CacheRead:     int64(row.CacheReadTokens)})
		}
		return wire.OrderedMap{
			{Key: "by_mode", Value: labelled(byMode, "mode")},
			{Key: "by_model", Value: labelled(byModel, "model")},
			{Key: "estimated_usd", Value: prices.Over(priceRows, prices.Today()).AsDict()},
		}, nil
	}
	week, err := window(ago(7))
	if err == nil {
		var month, all wire.OrderedMap
		if month, err = window(ago(30)); err == nil {
			all, err = window("")
		}
		if err == nil {
			wire.JSON(w, http.StatusOK, wire.OrderedMap{
				{Key: "windows", Value: wire.OrderedMap{
					{Key: "week", Value: week},
					{Key: "month", Value: month},
					{Key: "all", Value: all},
				}},
				{Key: "caveat", Value: "Token counts are a floor on the bill, " +
					"not the bill: cache writes bill at 1.25x input and are " +
					"not captured."},
				{Key: "prices", Value: wire.OrderedMap{
					{Key: "checked", Value: prices.Checked},
					{Key: "source", Value: prices.Source},
					{Key: "note", Value: "Estimated from list rates read by a " +
						"person on the date above, not from an invoice. A " +
						"conversation whose model is not in the table is " +
						"counted, never priced at zero."},
				}},
			})
			return
		}
	}
	a.fail(w, "stats/claude", err)
}

// labelled is each row plus how to name the model on a screen — beside the
// id, never instead of it (commandment 10: the label renders, the id is
// what the pricing question is about). The key order follows the axis: the
// grouped column first, then `(various)` — the SELECT order a `dict(row)`
// preserves in Python.
func labelled(rows []ledger.Summary, by string) []wire.OrderedMap {
	out := make([]wire.OrderedMap, 0, len(rows))
	for _, row := range rows {
		first := wire.KV{Key: "mode", Value: row.Mode}
		second := wire.KV{Key: "model", Value: row.Model}
		if by == "model" {
			first, second = second, first
		}
		out = append(out, wire.OrderedMap{
			first, second,
			{Key: "conversations", Value: row.Conversations},
			{Key: "requests", Value: row.Requests},
			{Key: "input_tokens", Value: row.InputTokens},
			{Key: "output_tokens", Value: row.OutputTokens},
			{Key: "cache_read_tokens", Value: row.CacheReadTokens},
			{Key: "first_at", Value: row.FirstAt},
			{Key: "last_at", Value: row.LastAt},
			{Key: "model_label", Value: tiers.LabelFor(row.Model)},
		})
	}
	return out
}

// ago is `_ago`: an ISO-8601 UTC instant `days` back, for string-compared
// `created_at` windows.
func ago(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05.000000+00:00")
}

// statsFly is `GET /api/admin/stats/fly`: what the platform sees, when the
// platform is asked — the one view that leaves the box, and the only one
// that can be switched off.
func (a *API) statsFly(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	wire.JSON(w, http.StatusOK, a.fly.Fetch())
}

// statsTraffic is `GET /api/admin/stats/traffic`: the visitor ledger. The
// note rides in the payload the way the Claude view's caveat does, because
// the sentence belongs wherever the numbers go.
func (a *API) statsTraffic(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	summary, err := a.traffic.Summary(r.Context(), 30)
	if err != nil {
		a.fail(w, "stats/traffic", err)
		return
	}
	wire.JSON(w, http.StatusOK, append(summary,
		wire.KV{Key: "note", Value: "Route templates and status classes " +
			"only — the ledger never records an address, an agent, a name, " +
			"or a concrete path."}))
}

// statsActivity is `GET /api/admin/stats/activity`: who has been here and
// what the instance has been doing — accounts by state, sessions by
// recency, deck edits by day, memoised simulations, and the job registry's
// census.
func (a *API) statsActivity(w http.ResponseWriter, r *http.Request) {
	if a.requireAdmin(w, r) {
		return
	}
	// Like `statsClaude`: an absent app.db is Python's freshly-minted empty
	// one — zeroes and empty lists, never a 404.
	db, present := a.accountsDB()
	if !present {
		census := map[string]int{}
		if a.jobs != nil {
			census = a.jobs.Census()
		}
		wire.JSON(w, http.StatusOK, wire.OrderedMap{
			{Key: "accounts", Value: wire.OrderedMap{}},
			{Key: "sessions", Value: wire.OrderedMap{
				{Key: "total", Value: 0}, {Key: "seen_day", Value: 0},
				{Key: "seen_week", Value: 0},
			}},
			{Key: "deck_edits_by_day", Value: []any{}},
			{Key: "sim_cache_rows", Value: 0},
			{Key: "jobs", Value: census},
		})
		return
	}
	ctx := r.Context()
	users, err := auth.AllUsers(ctx, db)
	if err != nil {
		a.fail(w, "stats/activity", err)
		return
	}
	// First-encounter order over the username-sorted list, exactly the
	// order Python's dict grows in.
	states := wire.OrderedMap{}
	for _, u := range users {
		state, err := accountState(ctx, db, u)
		if err != nil {
			a.fail(w, "stats/activity", err)
			return
		}
		found := false
		for i := range states {
			if states[i].Key == state {
				states[i].Value = states[i].Value.(int64) + 1
				found = true
			}
		}
		if !found {
			states = append(states, wire.KV{Key: state, Value: int64(1)})
		}
	}

	one := func(query string, args ...any) (int64, error) {
		var n sql.NullInt64
		if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
			return 0, err
		}
		return n.Int64, nil
	}
	sessionsTotal, err := one("SELECT count(*) FROM sessions")
	if err == nil {
		var seenDay, seenWeek, simRows int64
		if seenDay, err = one("SELECT count(*) FROM sessions WHERE last_seen_at >= ?", ago(1)); err == nil {
			if seenWeek, err = one("SELECT count(*) FROM sessions WHERE last_seen_at >= ?", ago(7)); err == nil {
				simRows, err = one("SELECT count(*) FROM sim_cache")
			}
		}
		if err == nil {
			edits := []any{}
			rows, queryErr := db.QueryContext(ctx,
				"SELECT substr(created_at, 1, 10) AS day, count(*)"+
					" FROM deck_log WHERE created_at >= ?"+
					" GROUP BY day ORDER BY day", ago(30))
			if queryErr != nil {
				a.fail(w, "stats/activity", queryErr)
				return
			}
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var day string
				var count int64
				if err := rows.Scan(&day, &count); err != nil {
					a.fail(w, "stats/activity", err)
					return
				}
				edits = append(edits, wire.OrderedMap{
					{Key: "day", Value: day},
					{Key: "edits", Value: count},
				})
			}
			if err := rows.Err(); err != nil {
				a.fail(w, "stats/activity", err)
				return
			}
			census := map[string]int{}
			if a.jobs != nil {
				census = a.jobs.Census()
			}
			wire.JSON(w, http.StatusOK, wire.OrderedMap{
				{Key: "accounts", Value: states},
				{Key: "sessions", Value: wire.OrderedMap{
					{Key: "total", Value: sessionsTotal},
					{Key: "seen_day", Value: seenDay},
					{Key: "seen_week", Value: seenWeek},
				}},
				{Key: "deck_edits_by_day", Value: edits},
				{Key: "sim_cache_rows", Value: simRows},
				// A Python dict's order here is whatever order statuses were
				// first seen in an arbitrary registry walk — not a shape any
				// golden can hold — so the sorted rendering a Go map gets is
				// one of the orders Python itself produces.
				{Key: "jobs", Value: census},
			})
			return
		}
	}
	a.fail(w, "stats/activity", err)
}

// The four account states come from `accountState` in admin.go — Python
// restates `_user_state` beside `_state` so a disagreement is visible on one
// page, but the two are the same predicate and this port's admin surface
// already carries it; a second copy here would be the drift, not the guard.
