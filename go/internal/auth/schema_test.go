package auth

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// dumpSchema renders a migrated file the way `authtest/app_schema.sql` is
// rendered — `PRAGMA user_version` first, then every
// `sqlite_master` row that carries SQL, sorted `type DESC, name`, each
// statement closed with a semicolon — so the comparison against the
// recorded fixture is byte equality, not a looser reading of it.
func dumpSchema(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	rows, err := db.Query("SELECT type, name, sql FROM sqlite_master" +
		" WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'" +
		" ORDER BY type DESC, name")
	if err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()
	lines := []string{fmt.Sprintf("PRAGMA user_version = %d;", version)}
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("scan: %v", err)
		}
		lines = append(lines, ddl+";")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return strings.Join(lines, "\n") + "\n"
}

// The fixture with its comment header off: the recorded schema,
// ready to compare against dumpSchema.
func recordedSchema(t *testing.T) string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(authtest.Schema(), "\n") {
		if strings.HasPrefix(line, "--") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// The ladder's whole promise in one assertion: a file `Migrate` built is
// byte-for-byte the recorded schema — same `sqlite_master`
// text, comments in the CREATE statements included, same `user_version`.
// `app_schema.sql` records the schema every deployed `app.db` already
// wears, so a rung that changes what the ladder builds fails here until
// the recorded schema is updated in the same change — never by drift.
func TestMigrateBuildsTheRecordedSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	got := dumpSchema(t, path)
	want := recordedSchema(t)
	if got != want {
		t.Fatalf("the ladder built a different schema than the recorded one:\n"+
			"got:\n%s\nwant:\n%s", got, want)
	}
}

// buildAtRung applies the first k scripts and stamps the version, standing
// in for an instance that stopped upgrading at an earlier deploy.
func buildAtRung(t *testing.T, path string, k int) {
	t.Helper()
	scripts, err := migrations()
	if err != nil {
		t.Fatalf("migrations: %v", err)
	}
	db, err := sql.Open("sqlite",
		"file:"+path+"?mode=rwc&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	for i, script := range scripts[:k] {
		if _, err := conn.ExecContext(ctx, script); err != nil {
			t.Fatalf("script %d: %v", i+1, err)
		}
	}
	if _, err := conn.ExecContext(ctx,
		fmt.Sprintf("PRAGMA user_version = %d", k)); err != nil {
		t.Fatalf("stamp: %v", err)
	}
}

// A database parked at any rung climbs the rest of the ladder to the same
// schema a fresh file gets. Rung 4 -> 5 is the interesting passage — the
// `users` rebuild — and this drives it from every starting point rather
// than trusting one.
func TestMigrateClimbsFromEveryRung(t *testing.T) {
	t.Parallel()
	fresh := filepath.Join(t.TempDir(), "fresh.db")
	if err := Migrate(fresh); err != nil {
		t.Fatalf("Migrate fresh: %v", err)
	}
	want := dumpSchema(t, fresh)
	for k := 0; k < SchemaVersion; k++ {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("rung%d.db", k))
		buildAtRung(t, path, k)
		if err := Migrate(path); err != nil {
			t.Fatalf("Migrate from rung %d: %v", k, err)
		}
		if got := dumpSchema(t, path); got != want {
			t.Fatalf("rung %d climbed to a different schema", k)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := Migrate(path); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	before := dumpSchema(t, path)
	if err := Migrate(path); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if got := dumpSchema(t, path); got != before {
		t.Fatalf("a second Migrate changed the schema")
	}
}

// Rows that already violate a foreign key make the sign-off refuse — the
// property bought by running `foreign_key_check` after
// switching enforcement off for the climb.
func TestMigrateRefusesAFileThatFailsTheCheck(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	buildAtRung(t, path, 1)
	db, err := sql.Open("sqlite",
		"file:"+path+"?mode=rw&_pragma=foreign_keys(0)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO sessions (token_hash, user_id, created_at, expires_at)" +
			" VALUES ('x', 999, 't', 't')"); err != nil {
		t.Fatalf("orphan insert: %v", err)
	}
	_ = db.Close()
	err = Migrate(path)
	if err == nil {
		t.Fatalf("a file with a dangling foreign key was signed off")
	}
	if !strings.Contains(err.Error(), "foreign key") ||
		!strings.Contains(err.Error(), "docs/HOSTING.md") {
		t.Fatalf("the refusal should name the check and the runbook, got: %v", err)
	}
}

// WAL is set when the file is created, and it is a property of the file:
// every later handle inherits it, which is what lets a reader never block
// the writer without each DSN having to ask.
func TestMigrateLeavesTheFileInWAL(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

// The embedded ladder has exactly SchemaVersion rungs: a fifteenth file lying
// beside a version of fourteen would otherwise sit there unapplied, looking
// landed.
func TestTheEmbeddedLadderIsExactlyTheVersion(t *testing.T) {
	t.Parallel()
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != SchemaVersion {
		t.Fatalf("%d files under migrations/ beside SchemaVersion %d",
			len(entries), SchemaVersion)
	}
}

// Rung 13 lands on a shelf that is already full, which is the only thing about
// it that could go wrong on the deployed volume.
//
// `TestMigrateClimbsFromEveryRung` above proves the *schema* arrives from any
// starting point, and it proves it against empty files. This asks the question
// that matters to somebody who already owns decks: the ladder runs at boot
// (ADR 23), so the first instance to see this rung has rows in `user_decks`
// before the column exists -- and every one of them must come out the other
// side entered for nothing. An opt-in that defaulted to opted-in would enter
// somebody's whole library in a feature they never chose, which is the one
// failure this flag must not have.
func TestRungThirteenLeavesExistingDecksOutOfTheNightGames(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	buildAtRung(t, path, 12)

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	now := "2026-08-30T12:00:00+00:00"
	if _, err := db.Exec(
		`INSERT INTO users (id, username, created_at) VALUES (1, 'aaron', ?)`, now); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// One shared and one private, so the new column cannot be confused with
	// the old one by a rung that copied the wrong default across.
	if _, err := db.Exec(
		`INSERT INTO user_decks (owner_id, slug, name, yaml, shared, created_at, updated_at)
		 VALUES (1, 'gyome', 'Gyome', 'name: Gyome', 1, ?, ?),
		        (1, 'arahbo', 'Arahbo', 'name: Arahbo', 0, ?, ?)`,
		now, now, now, now); err != nil {
		t.Fatalf("seed decks: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close before migrating: %v", err)
	}

	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	db, err = sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query(
		"SELECT slug, shared, coliseum_at_night FROM user_decks ORDER BY slug")
	if err != nil {
		t.Fatalf("the column the rung adds is not there: %v", err)
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]int{}
	for rows.Next() {
		var slug string
		var shared, night int
		if err := rows.Scan(&slug, &shared, &night); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if night != 0 {
			t.Errorf("%s came up the ladder already entered for the night games", slug)
		}
		seen[slug] = shared
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	// The decks that were there are still there, wearing what they wore. A
	// rung that dropped and rebuilt the table would pass every assertion above
	// and fail this one.
	if len(seen) != 2 || seen["gyome"] != 1 || seen["arahbo"] != 0 {
		t.Errorf("the rung disturbed the decks it climbed past: %v", seen)
	}
}

// Rung 14's one schema-held promise: a second *scheduled* run for the same
// night is refused by the unique partial index, while sample runs -- the
// admin's measurement -- may recur on a date freely. The runner's manners are
// tested where the runner lives; this is about what the file itself enforces,
// which is what still holds when the process restarts mid-night and asks
// again.
func TestRungFourteenHoldsOneScheduledRunPerNight(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Foreign keys on, as every request-path handle has them (OpenReadWrite):
	// the pragma is per-connection, and a test that forgets it is asking the
	// one connection in the app that never exists.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	insert := func(key string, sample int) error {
		_, err := db.Exec(
			`INSERT INTO night_runs (night_key, sample, opened_at, closes_at)
			 VALUES (?, ?, '2026-09-06T06:00:00+00:00', '2026-09-06T08:00:00+00:00')`,
			key, sample)
		return err
	}
	if err := insert("2026-09-06", 0); err != nil {
		t.Fatalf("the first scheduled run of the night was refused: %v", err)
	}
	if err := insert("2026-09-06", 0); err == nil {
		t.Error("a second scheduled run landed on the same night_key")
	}
	if err := insert("2026-09-07", 0); err != nil {
		t.Errorf("the next night's run was refused: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := insert("2026-09-06", 1); err != nil {
			t.Errorf("sample run %d on a scheduled night was refused: %v", i+1, err)
		}
	}
	// A bout must name a real run: the foreign key is on, and a row pointing
	// at a night that never happened is exactly the orphan `foreign_key_check`
	// signs off against.
	if _, err := db.Exec(
		`INSERT INTO night_bouts (run_id, seat_a_slug, seat_b_slug, games, seed,
		                          state, created_at, updated_at)
		 VALUES (999, 'gyome', 'arahbo', 10, 7,
		         'planned', '2026-09-06T06:00:00+00:00', '2026-09-06T06:00:00+00:00')`); err == nil {
		t.Error("a bout pointing at a run that does not exist was accepted")
	}
}

// The flag is writable once the rung has run, and writing it does not disturb
// the flag beside it. Two columns of the same shape on one table is exactly
// where an UPDATE lands on the wrong one, and nothing above would notice.
func TestRungThirteenGivesTheNightFlagItsOwnColumn(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	now := "2026-08-30T12:00:00+00:00"
	if _, err := db.Exec(
		`INSERT INTO users (id, username, created_at) VALUES (1, 'aaron', ?)`, now); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO user_decks (id, owner_id, slug, name, yaml, shared, created_at, updated_at)
		 VALUES (1, 1, 'gyome', 'Gyome', 'name: Gyome', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed deck: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE user_decks SET coliseum_at_night = 1 WHERE id = 1"); err != nil {
		t.Fatalf("entering the deck for the night games: %v", err)
	}
	var shared, night int
	if err := db.QueryRow(
		"SELECT shared, coliseum_at_night FROM user_decks WHERE id = 1").
		Scan(&shared, &night); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if night != 1 {
		t.Errorf("the night flag did not stick: %d", night)
	}
	if shared != 1 {
		t.Errorf("entering the night games changed who can see the deck: shared = %d", shared)
	}
}
