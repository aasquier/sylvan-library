package door

import (
	"database/sql"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	_ "modernc.org/sqlite"
)

// writeFixtureDB writes a real app.db -- the recorded schema the ladder
// leaves, from `authtest` -- with alice (admin) and bob, each holding
// a live session whose token is "<name>-token".
func writeFixtureDB(t *testing.T, path string) {
	t.Helper()
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
