package door

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// The door's half of the accounts sweep: built beside the night runner over
// the same write handle, its boot sweep already done by the time New returns,
// and stopped with the door. The sweeper's own behaviour -- what it takes,
// what it spares, its cadence -- lives in internal/auth's tests; what is
// asserted here is the wiring and the lifetime.

func TestTheDoorSweepsTheAccountsDatabaseAsItStands(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	writeFixtureDB(t, dbPath)

	// One session long past its expiry, beside the fixture's two live ones.
	// `LookupTouching` would only ever delete it if its own token came back;
	// the boot sweep is what takes it unprompted.
	seed, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	lapsed := time.Now().UTC().Add(-2*time.Hour).Format("2006-01-02T15:04:05.000000") + "+00:00"
	if _, err := seed.Exec(
		"INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		auth.HashToken("bob-stale"), 2, lapsed, lapsed); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	web, tarot := site(t)
	d, err := New(Config{RequireAuth: true, AppDB: dbPath, WebDist: web,
		TarotDir: tarot, DecksDir: t.TempDir(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}

	// The boot sweep is a fact by now, not an event to await: the lapsed row
	// is gone and both live sessions stand.
	check, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var stale, all int64
	if err := check.QueryRow("SELECT count(*) FROM sessions WHERE token_hash = ?",
		auth.HashToken("bob-stale")).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if err := check.QueryRow("SELECT count(*) FROM sessions").Scan(&all); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Error("the lapsed session survived the door standing; the boot sweep did not run")
	}
	if all != 2 {
		t.Errorf("%d sessions remain, want the fixture's two live ones", all)
	}

	// The lifetime half, the night runner's own bar: Close stops the sweeper
	// it started, and its return is the leak assertion -- Stop waits for the
	// goroutine, so a Close that comes back is a sweeper that is gone.
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestADoorWithNoAppDBHasNoSweeper(t *testing.T) {
	t.Parallel()
	// No write handle means nothing to delete from, so no janitor is hired --
	// the night runner's condition, shared deliberately.
	web, tarot := site(t)
	d, err := New(Config{WebDist: web, TarotDir: tarot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.sweeper != nil {
		t.Fatal("a door with no app.db built an accounts sweeper anyway")
	}
}
