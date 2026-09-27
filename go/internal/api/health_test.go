package api

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The no-pool shape, as bytes: the degraded answer is the platform's normal
// state between deploy and seeding, and its key order is part of the wire.
//
// The three sickness keys are here as well as in the healthy shape, and that
// is the point of them: an instance between deploy and seeding still has an
// auth database and a volume, and whatever is watching from outside should
// never have to branch on the card pool to find out whether they are well.
// Nulls throughout because this instance was described with no data directory
// and no `app.db` — nothing is wrong; there is nothing to open.
func TestHealthWithNoPoolIsTheDegradedShapeExactly(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: t.TempDir()})
	status, _, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	want := `{"pool":false,"oracle_cards":0,"printings":0,` +
		`"app_db":null,"disk_free_mb":null,"schema_version":null,` +
		`"message":` + string(mustJSON(t, noPoolMessage)) + `}`
	if string(raw) != want {
		t.Fatalf("got %s\nwant %s", raw, want)
	}
}

// **The two faults that used to leave this route green while the site was
// unusable.** A corrupt `app.db` fails every login and touches the card pool
// not at all; a full volume fails every write the same way. Both are reported
// in the body, and the status stays 200 for both — the platform stops routing
// to a machine whose check fails, and with one machine that turns "logins are
// broken" into "the site is down".
//
// The rung is read back from [auth.SchemaVersion] rather than typed, because a
// test that restates the number cannot tell you the number moved.
func TestHealthReportsTheAuthDatabaseAndTheVolumesHeadroom(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	if err := auth.Migrate(path); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t),
		AppDBPath: path, DataDir: dir})
	status, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["app_db"] != true {
		t.Fatalf("a real app.db should read as open: %s", raw)
	}
	if got := body["schema_version"]; got != float64(auth.SchemaVersion) {
		t.Fatalf("schema_version %v, want the ladder's height %d: %s",
			got, auth.SchemaVersion, raw)
	}
	free, ok := body["disk_free_mb"].(float64)
	if !ok || free <= 0 {
		t.Fatalf("a real volume should report megabytes of headroom: %s", raw)
	}
}

// A file at `app.db`'s path that is not a database — half a restore, a
// download that landed on the wrong name — reads as `app_db: false` with no
// rung, and the route still answers 200. False rather than null is the whole
// signal: null says "this instance has no auth database", false says "it has
// one and it will not answer", and only the second is an alarm.
func TestHealthCallsOutAnUnreadableAuthDatabaseAndStaysGreen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := New(Config{Logger: quiet, Pool: pooltest.Open(t),
		DecksDir: decksDir(t), AppDBPath: path, DataDir: dir})
	status, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("a sick instance must still answer 200, or the platform stops "+
			"routing to the only machine there is: %d %s", status, raw)
	}
	if body["app_db"] != false {
		t.Fatalf("an unreadable app.db should read as false, not %v: %s",
			body["app_db"], raw)
	}
	if body["schema_version"] != nil {
		t.Fatalf("a database that would not open has no rung to report: %s", raw)
	}
}

// **Zero free megabytes is an alarm, so an unanswered question must not look
// like one.** [diskUsage] reports a failed statfs as three zeros, and a
// monitor reading `disk_free_mb: 0` would wake somebody for a full volume that
// was really a path nobody could ask about.
func TestTheFreeSpaceReadingIsAbsentRatherThanZero(t *testing.T) {
	t.Parallel()
	if got := diskFreeMB(""); got != nil {
		t.Errorf("an instance with no data directory reports %v free", got)
	}
	if got := diskFreeMB(filepath.Join(t.TempDir(), "no-such-volume")); got != nil {
		t.Errorf("a path that names nothing reports %v free", got)
	}
	free, ok := diskFreeMB(t.TempDir()).(int64)
	if !ok || free <= 0 {
		t.Errorf("a real directory reports %v free, so the two nils above are "+
			"about the fault and not about the reading being broken",
			diskFreeMB(t.TempDir()))
	}
}

// A healthy pool reports its counts, the bulk files on the shelf, the deck
// count and a false staleness flag -- with the keys in the recorded order.
func TestHealthReportsThePoolTheShelfAndTheDecks(t *testing.T) {
	t.Parallel()
	scryfall := t.TempDir()
	for _, name := range []string{"oracle_cards-2026-08-20.jsonl.gz",
		"default_cards-2026-08-21.jsonl.gz"} {
		if err := os.WriteFile(filepath.Join(scryfall, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decksDir(t),
		ScryfallDir: scryfall})
	status, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool"] != true || body["pool_stale"] != false {
		t.Fatalf("pool flags: %s", raw)
	}
	if body["oracle_cards"].(float64) < 20 || body["printings"].(float64) < 10 {
		t.Fatalf("counts: %s", raw)
	}
	files, _ := body["bulk_files"].([]any)
	if len(files) != 2 || files[0] != "default_cards-2026-08-21.jsonl.gz" {
		t.Fatalf("bulk_files should list the shelf sorted: %s", raw)
	}
	if body["decks"].(float64) < 1 {
		t.Fatalf("decks: %s", raw)
	}
	if _, present := body["message"]; present {
		t.Fatalf("a current pool carries no message: %s", raw)
	}
}

// **The probe reads the pool and does not keep it.**
//
// This is the deployment fault, written as the one thing about it a suite can
// hold. `mtglab data refresh` on the live instance would report the card pool
// as locked by the serving process and then succeed on a retry — because *two*
// health checks poll this route every thirty seconds, from outside the
// container and from inside it, and an ordinary lease keeps the file open for
// ten seconds past each one. Out of phase, that is a file held roughly thirteen
// seconds in every fifteen and a refresh with under two seconds to find.
//
// Asserted on the pool's own state rather than on which function the handler
// called, because the outcome is the thing that matters and a spy on the call
// would keep passing if the lease semantics changed underneath it.
func TestHealthDoesNotHoldTheCardPoolOpen(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	a := New(Config{Pool: p, DecksDir: decksDir(t)})
	for i := 0; i < 3; i++ {
		if status, _, raw := call(t, a, http.MethodGet, "/api/health", ""); status != 200 {
			t.Fatalf("%d: %s", status, raw)
		}
		if p.Held() {
			t.Fatal("the health probe left the card pool's file open behind it, " +
				"which is what locked `mtglab data refresh` out of the instance")
		}
	}
	// And it is still a working read, not a probe that has learned to answer
	// without looking.
	_, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if body["pool"] != true || body["oracle_cards"].(float64) < 20 {
		t.Fatalf("the probe stopped reading the pool: %s", raw)
	}
}

// A pool that predates the printed stats answers `pool_stale` and the
// re-ingest message -- `pool.Stale`'s verdict on the route.
func TestHealthReportsAStalePool(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE oracle_cards SET power = NULL"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	a := New(Config{Pool: p, DecksDir: t.TempDir()})
	status, body, raw := call(t, a, http.MethodGet, "/api/health", "")
	if status != 200 {
		t.Fatalf("%d: %s", status, raw)
	}
	if body["pool_stale"] != true {
		t.Fatalf("pool_stale: %s", raw)
	}
	msg, _ := body["message"].(string)
	if msg != "pool predates the printed stats or the painters -- "+
		"run `mtglab data refresh`" {
		t.Fatalf("message %q", msg)
	}
}
