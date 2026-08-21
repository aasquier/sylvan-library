package door

import (
	"database/sql"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	_ "modernc.org/sqlite"
)

// writeFixtureDB writes an app.db the way Python would have: WAL, the v1
// users/sessions tables, alice (admin) and bob, each with a live session whose
// token is "<name>-token".
func writeFixtureDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const ddl = `
CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT, email TEXT UNIQUE COLLATE NOCASE, is_admin INTEGER NOT NULL DEFAULT 0,
  disabled_at TEXT, created_at TEXT NOT NULL);
CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL, last_seen_at TEXT);`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000") + "+00:00"
	later := time.Now().UTC().Add(auth.Lifetime).Format("2006-01-02T15:04:05.000000") + "+00:00"
	for _, u := range []struct {
		id    int
		name  string
		admin int
	}{{1, "alice", 1}, {2, "bob", 0}} {
		if _, err := db.Exec("INSERT INTO users (id, username, is_admin, created_at) VALUES (?, ?, ?, ?)",
			u.id, u.name, u.admin, now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
			auth.HashToken(u.name+"-token"), u.id, now, later); err != nil {
			t.Fatal(err)
		}
	}
}
