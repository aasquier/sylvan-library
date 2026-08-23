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

// dumpSchema renders a migrated file the way `tests/go_fixtures.py`'s
// `render_app_schema` does — `PRAGMA user_version` first, then every
// `sqlite_master` row that carries SQL, sorted `type DESC, name`, each
// statement closed with a semicolon — so the comparison against the
// generated fixture is byte equality, not a looser reading of it.
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

// The fixture with its comment header off: what Python's ladder built,
// rendered by `render_app_schema`, ready to compare against dumpSchema.
func pythonSchema(t *testing.T) string {
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

// The retirement gate for the ladder: a file the Go migrator built is
// byte-for-byte the file Python's ladder builds — same `sqlite_master`
// text, comments in the CREATE statements included, same `user_version`.
// `app_schema.sql` is generated from a database `auth/db.py` just migrated
// (`tests/go_fixtures.py`) and `tests/test_go_fixtures.py` keeps it
// current, so this one assertion holds the two ladders equal in both
// directions: a rung added to either side fails here until the other side
// carries it too.
func TestMigrateBuildsWhatPythonBuilt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	if err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	got := dumpSchema(t, path)
	want := pythonSchema(t)
	if got != want {
		t.Fatalf("the Go ladder built a different schema than Python's:\n"+
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
// property `auth/db.py` bought by running `foreign_key_check` after
// switching enforcement off, carried across with the ladder.
func TestMigrateRefusesAFileThatFailsTheCheck(t *testing.T) {
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

// The embedded ladder has exactly SchemaVersion rungs: a 13th file lying
// beside a version of 12 would otherwise sit there unapplied, looking
// landed.
func TestTheEmbeddedLadderIsExactlyTheVersion(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != SchemaVersion {
		t.Fatalf("%d files under migrations/ beside SchemaVersion %d",
			len(entries), SchemaVersion)
	}
}
