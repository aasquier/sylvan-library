package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/flymetrics"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/traffic"
)

// The admin dashboard's numbers. Every view here is a fact about the box read
// from the box, so the tests build the box: a temp directory with files of
// known sizes standing in for the volume, a real `app.db` for the counts, and
// a stub for the one view that leaves the machine.
//
// The shape is the contract. The frontend renders these keys, so a view that
// answered `null` where a number belongs -- or dropped a key on the degraded
// path -- would be a blank panel rather than an error, which is the failure
// mode these tests exist to catch.

// statsRig is an admin-scoped API over a populated volume.
type statsRig struct {
	*accountRig
	dataDir string
}

// newStatsRig lays out a volume the storage view can measure: a cache with
// its three named shelves plus an unnamed fourth, two decks and one in the
// trash, a pool file and a Scryfall drop.
func newStatsRig(t *testing.T) *statsRig {
	t.Helper()
	rig := newAccountRig(t, true)
	t.Cleanup(rig.close)

	dataDir := t.TempDir()
	write := func(rel string, size int) {
		t.Helper()
		path := filepath.Join(dataDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("cache/symbols/w.svg", 100)
	write("cache/cardmotion/a.mp4", 200)
	write("cache/ocr/scan.json", 300)
	write("cache/reading/spread.json", 400) // the unnamed fourth tenant
	write("pool.duckdb", 512)
	write("scryfall/oracle-cards.json", 64)

	decksDir := filepath.Join(dataDir, "decks")
	write("decks/gyome/deck.yaml", 32)
	write("decks/trostani/deck.yaml", 32)
	write("decks/.trash/retired/deck.yaml", 16)

	rig.api.dataDir = dataDir
	rig.api.poolPath = filepath.Join(dataDir, "pool.duckdb")
	rig.api.scryfallDir = filepath.Join(dataDir, "scryfall")
	rig.api.decksDir = decksDir
	return &statsRig{accountRig: rig, dataDir: dataDir}
}

// get runs an admin GET and decodes the payload.
func (r *statsRig) get(t *testing.T, target string) map[string]any {
	t.Helper()
	rec := r.call(t, adminScope, http.MethodGet, target, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s answered %d: %s", target, rec.Code, rec.Body.String())
	}
	return body(t, rec)
}

// nested walks a decoded payload, failing rather than panicking on a missing
// key -- a dropped key is exactly the bug these tests watch for.
func nested(t *testing.T, m map[string]any, path ...string) any {
	t.Helper()
	var cur any = m
	for i, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object at %q", strings.Join(path[:i], "."), key)
		}
		cur, ok = obj[key]
		if !ok {
			t.Fatalf("no key %q under %q", key, strings.Join(path[:i], "."))
		}
	}
	return cur
}

// The system view reports the process and the machine under it. The numbers
// vary per box, so what is pinned is that every key is present and that the
// ones that must be numbers are numbers -- a `null` where the page expects a
// byte count renders as a blank tile.
func TestTheSystemViewReportsTheBoxItIsRunningOn(t *testing.T) {
	t.Parallel()
	rig := newStatsRig(t)
	payload := rig.get(t, "/api/admin/stats/system")

	if got := nested(t, payload, "schema", "expected"); got != float64(auth.SchemaVersion) {
		t.Errorf("expected schema %v, want %d", got, auth.SchemaVersion)
	}
	// The rig's app.db was built from the recorded fixture, so the applied
	// version is the ladder's full height rather than nil.
	if got := nested(t, payload, "schema", "applied"); got != float64(auth.SchemaVersion) {
		t.Errorf("applied schema %v, want %d", got, auth.SchemaVersion)
	}
	if _, ok := nested(t, payload, "process", "bytes").(float64); !ok {
		t.Error("the process view carries no byte count")
	}
	if kind := nested(t, payload, "process", "kind"); kind != "peak" && kind != "current" {
		t.Errorf("the process view labels its number %q", kind)
	}
	// Present, and permitted to be null on a box with no /proc.
	for _, key := range []string{"total_bytes", "available_bytes"} {
		nested(t, payload, "memory", key)
	}
	if _, ok := payload["load"].([]any); !ok {
		t.Errorf("load is %T, not a list -- the page iterates it", payload["load"])
	}
	if cpus, ok := payload["cpus"].(float64); !ok || cpus < 1 {
		t.Errorf("cpus is %v", payload["cpus"])
	}
	if got := nested(t, payload, "disk", "path"); got != rig.dataDir {
		t.Errorf("disk path %q, want %q", got, rig.dataDir)
	}
	for _, key := range []string{"total_bytes", "used_bytes", "free_bytes"} {
		if _, ok := nested(t, payload, "disk", key).(float64); !ok {
			t.Errorf("disk.%s is not a number", key)
		}
	}
}

// A stats panel that could change the schema by being looked at is not a
// stats panel: the read is `mode=ro` and both absences answer nil rather
// than creating anything.
func TestTheSchemaVersionIsReadWithoutAcquiringADatabase(t *testing.T) {
	t.Parallel()

	if got := New(Config{}).schemaApplied(); got != nil {
		t.Errorf("an instance with no app.db path reported schema %v", got)
	}

	missing := filepath.Join(t.TempDir(), "app.db")
	a := New(Config{AppDBPath: missing})
	if got := a.schemaApplied(); got != nil {
		t.Errorf("a path with no file reported schema %v", got)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("looking at the schema version created app.db")
	}
}

// The storage view names what is on the volume. The cache breakdown is the
// part that matters: it once shipped naming two of three tenants while the
// reading engine was a third of the cache, so the remainder is asserted
// against a fourth shelf the fixed list cannot see.
func TestTheStorageViewNamesTheCacheTenantsAndTheRemainder(t *testing.T) {
	t.Parallel()
	rig := newStatsRig(t)
	payload := rig.get(t, "/api/admin/stats/storage")

	if got := payload["pool_bytes"]; got != float64(512) {
		t.Errorf("pool_bytes %v, want 512", got)
	}
	if got := payload["scryfall_bulk_bytes"]; got != float64(64) {
		t.Errorf("scryfall_bulk_bytes %v, want 64", got)
	}
	if got, ok := payload["app_db_bytes"].(float64); !ok || got <= 0 {
		t.Errorf("app_db_bytes %v", payload["app_db_bytes"])
	}
	if got := payload["cache_bytes"]; got != float64(1000) {
		t.Errorf("cache_bytes %v, want 1000", got)
	}
	for key, want := range map[string]float64{
		"symbols_bytes": 100, "cardmotion_bytes": 200,
		"ocr_bytes": 300, "other_bytes": 400,
	} {
		if got := nested(t, payload, "cache", key); got != want {
			t.Errorf("cache.%s is %v, want %v", key, got, want)
		}
	}
	if got := nested(t, payload, "decks", "count"); got != float64(2) {
		t.Errorf("deck count %v, want 2 (the trash is counted separately)", got)
	}
	if got := nested(t, payload, "decks", "trashed"); got != float64(1) {
		t.Errorf("trashed %v, want 1", got)
	}
	if got, ok := nested(t, payload, "decks", "bytes").(float64); !ok || got < 64 {
		t.Errorf("deck bytes %v", got)
	}
}

// A fresh instance is mostly absences, and every one of them is information
// rather than an error: nulls, zeroes, and never a 404.
func TestTheStorageViewAnswersAFreshInstanceInNulls(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	rig.api.dataDir = filepath.Join(t.TempDir(), "nothing-here")
	rig.api.poolPath = ""
	rig.api.scryfallDir = ""
	rig.api.decksDir = filepath.Join(t.TempDir(), "no-decks")

	rec := rig.call(t, adminScope, http.MethodGet, "/api/admin/stats/storage", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("a fresh instance answered %d", rec.Code)
	}
	payload := body(t, rec)
	for _, key := range []string{"pool_bytes", "scryfall_bulk_bytes", "cache_bytes"} {
		if payload[key] != nil {
			t.Errorf("%s is %v on an instance with nothing there", key, payload[key])
		}
	}
	// With no total there is no remainder to compute -- null, not zero.
	if got := nested(t, payload, "cache", "other_bytes"); got != nil {
		t.Errorf("other_bytes is %v with no cache at all", got)
	}
	if got := nested(t, payload, "decks", "count"); got != float64(0) {
		t.Errorf("deck count %v on an instance with no decks", got)
	}
}

// The remainder is clamped rather than allowed to go negative: the named
// shelves are measured separately from the total, and a file that lands
// between the two reads would otherwise render as a negative byte count.
func TestTheCacheRemainderNeverGoesNegative(t *testing.T) {
	t.Parallel()
	cache := t.TempDir()
	for _, shelf := range []string{"symbols", "cardmotion", "ocr"} {
		path := filepath.Join(cache, shelf, "f")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, make([]byte, 100), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A total smaller than the parts, as a torn read would produce.
	torn := int64(10)
	out := cacheBreakdown(cache, &torn)
	var other any
	for _, kv := range out {
		if kv.Key == "other_bytes" {
			other = kv.Value
		}
	}
	if other != int64(0) {
		t.Fatalf("other_bytes is %v, want a clamped 0", other)
	}
}

// sizeOf is the one measurement every storage number rests on, so its edges
// are pinned directly rather than through a route.
func TestSizeOfMeasuresFilesTreesAndAbsences(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if got := sizeOf(""); got != nil {
		t.Errorf("the empty path measured %v", *got)
	}
	if got := sizeOf(filepath.Join(dir, "nope")); got != nil {
		t.Errorf("a missing path measured %v", *got)
	}

	file := filepath.Join(dir, "one")
	if err := os.WriteFile(file, make([]byte, 7), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := sizeOf(file); got == nil || *got != 7 {
		t.Errorf("a 7-byte file measured %v", got)
	}

	tree := filepath.Join(dir, "tree")
	if err := os.MkdirAll(filepath.Join(tree, "deep", "deeper"), 0o750); err != nil {
		t.Fatal(err)
	}
	for i, rel := range []string{"a", "deep/b", "deep/deeper/c"} {
		if err := os.WriteFile(filepath.Join(tree, rel), make([]byte, i+1), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := sizeOf(tree); got == nil || *got != 6 {
		t.Errorf("a three-file tree measured %v, want 6", got)
	}

	// An empty directory is nothing rather than absent: zero, not nil.
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o750); err != nil {
		t.Fatal(err)
	}
	if got := sizeOf(empty); got == nil || *got != 0 {
		t.Errorf("an empty directory measured %v, want 0", got)
	}
}

func TestCountDirsCountsDirectoriesAndForgivesAnAbsence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if got := countDirs(filepath.Join(dir, "missing")); got != 0 {
		t.Errorf("a missing directory counted %d", got)
	}
	if got := countDirs(dir); got != 0 {
		t.Errorf("an empty directory counted %d", got)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "loose.yaml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := countDirs(dir); got != 2 {
		t.Errorf("counted %d, want 2 -- a loose file is not a deck", got)
	}
}

// The deck count and the deck list must agree. They did not: this panel
// counted every directory, so `.trash` became a deck the moment anybody
// deleted one, and the admin page reported one more deck than the library
// listed. The file tier skips `.` and `_` prefixes; so does this.
func TestTheDeckCountSkipsWhatTheFileTierSkips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"gyome", "trostani", ".trash", "_scaffold"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if got := countDirs(dir); got != 2 {
		t.Errorf("counted %d decks, want 2 -- .trash and _scaffold are not decks", got)
	}
	// The trash's own entries are `slug-stamp`, so the same rule counts
	// them all.
	trash := filepath.Join(dir, ".trash")
	for _, name := range []string{"gyome-20260101", "old-20251231"} {
		if err := os.Mkdir(filepath.Join(trash, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if got := countDirs(trash); got != 2 {
		t.Errorf("counted %d trashed decks, want 2", got)
	}
}

func TestIntOrNilDistinguishesZeroFromNothing(t *testing.T) {
	t.Parallel()
	if got := intOrNil(nil); got != nil {
		t.Errorf("nothing rendered as %v", got)
	}
	zero := int64(0)
	if got := intOrNil(&zero); got != int64(0) {
		t.Errorf("zero rendered as %v -- zero bytes is not no bytes", got)
	}
}

// The Claude view is two axes per window because they answer different
// questions, and the dollar figure comes off the per-model rollup only.
func TestTheClaudeViewRollsUpBothAxesOverThreeWindows(t *testing.T) {
	t.Parallel()
	rig := newStatsRig(t)
	rec := ledger.RecorderFrom(rig.db, slog.Default())
	ctx := context.Background()
	rec.Record(ctx, ledger.Row{Mode: "interview", Model: "claude-opus-5",
		StopReason: "end_turn", Requests: 1, InputTokens: 100,
		OutputTokens: 50, CacheReadTokens: 10})
	rec.Record(ctx, ledger.Row{Mode: "research", Model: "claude-sonnet-5",
		StopReason: "end_turn", Requests: 2, InputTokens: 200,
		OutputTokens: 80, CacheReadTokens: 0})

	payload := rig.get(t, "/api/admin/stats/claude")
	for _, window := range []string{"week", "month", "all"} {
		byMode, ok := nested(t, payload, "windows", window, "by_mode").([]any)
		if !ok {
			t.Fatalf("%s.by_mode is not a list", window)
		}
		if len(byMode) != 2 {
			t.Errorf("%s rolled up %d modes, want 2", window, len(byMode))
		}
		byModel, ok := nested(t, payload, "windows", window, "by_model").([]any)
		if !ok {
			t.Fatalf("%s.by_model is not a list", window)
		}
		if len(byModel) != 2 {
			t.Errorf("%s rolled up %d models, want 2", window, len(byModel))
		}
		// The grouped column leads, and the label rides beside the id
		// rather than instead of it (commandment 10).
		row, ok := byModel[0].(map[string]any)
		if !ok {
			t.Fatalf("%s.by_model[0] is not an object", window)
		}
		if _, ok := row["model_label"]; !ok {
			t.Errorf("%s.by_model[0] carries no model_label", window)
		}
		if row["model"] == nil || row["model"] == "" {
			t.Errorf("%s.by_model[0] dropped the model id in favour of the label", window)
		}
		nested(t, payload, "windows", window, "estimated_usd")
	}
	// The caveat and the pricing provenance ride with the numbers, because
	// the sentence belongs wherever they go.
	if caveat, _ := payload["caveat"].(string); !strings.Contains(caveat, "floor") {
		t.Errorf("the caveat is %q", payload["caveat"])
	}
	for _, key := range []string{"checked", "source", "note"} {
		if got := nested(t, payload, "prices", key); got == nil || got == "" {
			t.Errorf("prices.%s is %v", key, got)
		}
	}
}

// An instance with no app.db is answered in the shapes an empty database
// would answer -- never a 404, because the stats are about the instance
// rather than about an account.
func TestTheClaudeAndActivityViewsAnswerAnInstanceWithNoDatabase(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: t.TempDir(), DataDir: t.TempDir()})
	a.jobs = jobs.New(jobs.Config{Logger: a.log})
	a.jobs.Completed("sim.mana", map[string]any{}, "a run", 0)

	claude := httptest.NewRecorder()
	a.statsClaude(claude, adminRequest(t, "/api/admin/stats/claude"))
	if claude.Code != http.StatusOK {
		t.Fatalf("the Claude view answered %d with no app.db", claude.Code)
	}
	var claudeBody map[string]any
	if err := json.Unmarshal(claude.Body.Bytes(), &claudeBody); err != nil {
		t.Fatal(err)
	}
	for _, window := range []string{"week", "month", "all"} {
		rows, ok := nested(t, claudeBody, "windows", window, "by_mode").([]any)
		if !ok || len(rows) != 0 {
			t.Errorf("%s.by_mode is %v, want an empty list", window, claudeBody)
		}
	}

	activity := httptest.NewRecorder()
	a.statsActivity(activity, adminRequest(t, "/api/admin/stats/activity"))
	if activity.Code != http.StatusOK {
		t.Fatalf("the activity view answered %d with no app.db", activity.Code)
	}
	var act map[string]any
	if err := json.Unmarshal(activity.Body.Bytes(), &act); err != nil {
		t.Fatal(err)
	}
	if got := act["sim_cache_rows"]; got != float64(0) {
		t.Errorf("sim_cache_rows %v on an instance with no database", got)
	}
	if edits, ok := act["deck_edits_by_day"].([]any); !ok || len(edits) != 0 {
		t.Errorf("deck_edits_by_day is %v, want [] -- the page iterates it", act["deck_edits_by_day"])
	}
	if got := nested(t, act, "sessions", "total"); got != float64(0) {
		t.Errorf("session total %v", got)
	}
	// The registry census is this process's own, so it is real even here.
	census, ok := act["jobs"].(map[string]any)
	if !ok || len(census) == 0 {
		t.Errorf("the job census is %v, want this process's own", act["jobs"])
	}
}

// The activity view is who has been here and what the instance has been
// doing. The account states come from the same predicate the admin surface
// uses, so the rig's three seeded accounts are three different states.
func TestTheActivityViewCountsAccountsSessionsAndEdits(t *testing.T) {
	t.Parallel()
	rig := newStatsRig(t)
	ctx := context.Background()
	if _, err := auth.CreateSession(ctx, rig.db, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.db.ExecContext(ctx,
		"INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary)"+
			" VALUES (?, NULL, 'gyome', 'alice', 'add', 'added a card')",
		time.Now().UTC().Format("2006-01-02T15:04:05.000000+00:00")); err != nil {
		t.Fatal(err)
	}
	rig.api.jobs = jobs.New(jobs.Config{Logger: rig.api.log})
	rig.api.jobs.Completed("sim.mana", map[string]any{}, "a run", 1)

	payload := rig.get(t, "/api/admin/stats/activity")

	accounts, ok := payload["accounts"].(map[string]any)
	if !ok {
		t.Fatalf("accounts is %T", payload["accounts"])
	}
	// alice and bob claimed their invites; `waiting` still holds one.
	if got := accounts["active"]; got != float64(2) {
		t.Errorf("active accounts %v, want 2", got)
	}
	if got := accounts["invited"]; got != float64(1) {
		t.Errorf("invited accounts %v, want 1", got)
	}
	if got := nested(t, payload, "sessions", "total"); got != float64(1) {
		t.Errorf("session total %v, want 1", got)
	}
	if got := nested(t, payload, "sessions", "seen_day"); got != float64(1) {
		t.Errorf("seen_day %v, want 1 -- the session was just made", got)
	}
	edits, ok := payload["deck_edits_by_day"].([]any)
	if !ok || len(edits) != 1 {
		t.Fatalf("deck_edits_by_day is %v, want one day", payload["deck_edits_by_day"])
	}
	day, ok := edits[0].(map[string]any)
	if !ok {
		t.Fatalf("the day row is %T", edits[0])
	}
	if day["edits"] != float64(1) {
		t.Errorf("the day counted %v edits", day["edits"])
	}
	if len(day["day"].(string)) != 10 {
		t.Errorf("the day is %q, want a bare date", day["day"])
	}
	if got := payload["sim_cache_rows"]; got != float64(0) {
		t.Errorf("sim_cache_rows %v", got)
	}
	if census, ok := payload["jobs"].(map[string]any); !ok || census["done"] != float64(1) {
		t.Errorf("the census is %v, want one finished job", payload["jobs"])
	}
}

// The traffic view carries the note that says what the ledger does not
// record, because the sentence belongs wherever the numbers go.
func TestTheTrafficViewSummarisesTheLedgerAndCarriesItsNote(t *testing.T) {
	t.Parallel()
	rig := newStatsRig(t)
	rec := traffic.New(rig.db, slog.Default())
	rec.Record("/api/decks/{slug}", http.StatusOK)
	rec.Record("/api/decks/{slug}", http.StatusOK)
	rec.Record("/api/decks/{slug}", http.StatusNotFound)
	rig.api.traffic = rec

	payload := rig.get(t, "/api/admin/stats/traffic")
	note, _ := payload["note"].(string)
	if !strings.Contains(note, "never records an address") {
		t.Errorf("the traffic note is %q", note)
	}
	if len(payload) < 2 {
		t.Errorf("the summary carried nothing but its note: %v", payload)
	}
}

// A nil recorder summarises an empty ledger rather than failing: an instance
// with no app.db still has an admin page.
func TestTheTrafficViewAnswersWithNoRecorder(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	rec := httptest.NewRecorder()
	a.statsTraffic(rec, adminRequest(t, "/api/admin/stats/traffic"))
	if rec.Code != http.StatusOK {
		t.Fatalf("no recorder answered %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "note") {
		t.Errorf("the note went missing: %s", rec.Body.String())
	}
}

// The Fly view is the one that leaves the box, so the box is stubbed. What
// is pinned is that the handler answers the panel rather than reaching for
// the network itself.
func TestTheFlyViewAnswersThePanel(t *testing.T) {
	t.Parallel()
	calls := 0
	panel := &flymetrics.Panel{
		Log: slog.Default(),
		Now: func() time.Time { return time.Unix(0, 0) },
		Transport: func(_ string, _ map[string]string) (int, []byte, error) {
			calls++
			return http.StatusOK, []byte(`{"data":{"app":{"name":"mtglab"}}}`), nil
		},
	}
	a := New(Config{Fly: panel})
	rec := httptest.NewRecorder()
	a.statsFly(rec, adminRequest(t, "/api/admin/stats/fly"))
	if rec.Code != http.StatusOK {
		t.Fatalf("the Fly view answered %d", rec.Code)
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatalf("the Fly view answered non-JSON: %s", rec.Body.String())
	}
}

// The window boundaries are string-compared ISO-8601, which is only date
// comparison for free if the format is exactly the recorded one.
func TestAgoRendersAComparableInstant(t *testing.T) {
	t.Parallel()
	week := ago(7)
	if _, err := time.Parse("2006-01-02T15:04:05.000000+00:00", week); err != nil {
		t.Fatalf("ago(7) is %q: %v", week, err)
	}
	if today := ago(0); week >= today {
		t.Errorf("ago(7)=%q does not sort before ago(0)=%q", week, today)
	}
	parsed, err := time.Parse("2006-01-02T15:04:05.000000+00:00", week)
	if err != nil {
		t.Fatal(err)
	}
	if days := time.Since(parsed).Hours() / 24; days < 6.9 || days > 7.1 {
		t.Errorf("ago(7) is %.2f days back", days)
	}
}

// The axis decides which column leads, and the rest of the row is the same
// either way -- the recorded key order, which the frontend reads positionally.
func TestLabelledLeadsWithTheGroupedColumn(t *testing.T) {
	t.Parallel()
	rows := []ledger.Summary{{Mode: "interview", Model: "claude-opus-5",
		Conversations: 1, Requests: 1, InputTokens: 10, OutputTokens: 5}}

	byMode := labelled(rows, "mode")
	if len(byMode) != 1 {
		t.Fatalf("labelled dropped a row: %v", byMode)
	}
	if byMode[0][0].Key != "mode" || byMode[0][1].Key != "model" {
		t.Errorf("the mode axis leads with %q then %q", byMode[0][0].Key, byMode[0][1].Key)
	}

	byModel := labelled(rows, "model")
	if byModel[0][0].Key != "model" || byModel[0][1].Key != "mode" {
		t.Errorf("the model axis leads with %q then %q", byModel[0][0].Key, byModel[0][1].Key)
	}
	last := byModel[0][len(byModel[0])-1]
	if last.Key != "model_label" || last.Value == "" {
		t.Errorf("the label is %v", last)
	}
	if got := labelled(nil, "mode"); got == nil || len(got) != 0 {
		t.Errorf("no rows rendered as %v, want an empty list", got)
	}
}

// adminRequest is a request already carrying the admin scope, for the
// handlers called directly rather than through the rig's router.
func adminRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(auth.WithScope(r.Context(), adminScope))
}
