package authtest

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fixture nothing checks is a fixture that can lie to every test standing
// on it, and these two lie in opposite directions: a scratch database that
// quietly fails to build makes a test green over an empty file, and a faulty
// handle that quietly stays healthy makes a refusal test green over a
// database that never refused anything.

func TestAScratchDatabaseSaysSoWhenItCannotBeBuilt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A file where the parent directory has to go: MkdirAll refuses, and the
	// refusal has to reach the caller rather than leaving a handle over
	// nothing.
	blocker := filepath.Join(dir, "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewScratchDB(filepath.Join(blocker, "deeper", "app.db")); err == nil {
		t.Fatal("a scratch database built itself under a plain file")
	}

	// And the schema is executed rather than assumed: run it twice over the
	// same file and the second run is refused, which is the proof the first
	// one actually wrote the tables.
	path := filepath.Join(dir, "app.db")
	if err := NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	err := NewScratchDB(path)
	if err == nil {
		t.Fatal("the schema was applied twice to one file without complaint")
	}
	if !strings.Contains(err.Error(), "building the scratch app.db") {
		t.Fatalf("the failure did not say what it was building: %v", err)
	}
}

func TestAFaultyHandleAnswersUntilItIsArmedAndThenRefuses(t *testing.T) {
	t.Parallel()
	db, fault, err := OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	// Healthy: the recorded schema is there and writable.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, username, created_at)`+
			` VALUES (1, 'squire', '2026-09-01T00:00:00+00:00')`); err != nil {
		t.Fatalf("a healthy faulty handle refused a write: %v", err)
	}

	// Armed at one: the next statement lands, the one after it does not.
	fault.After(1)
	var name string
	if err := db.QueryRowContext(ctx,
		`SELECT username FROM users WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("the one statement the budget allowed was refused: %v", err)
	}
	if name != "squire" {
		t.Fatalf("the allowed statement answered %q", name)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT username FROM users WHERE id = 1`).Scan(&name); !errors.Is(err, ErrGoneAway) {
		t.Fatalf("the statement past the budget answered %v, want ErrGoneAway", err)
	}

	// A transaction is where this fixture earns its place: the BEGIN and the
	// statements inside it land, and the COMMIT is the one that goes away —
	// the branch a closed handle can never reach.
	fault.Heal()
	fault.After(2)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("the begin inside the budget was refused: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, username, created_at)`+
			` VALUES (2, 'hero', '2026-09-01T00:00:00+00:00')`); err != nil {
		t.Fatalf("the statement inside the budget was refused: %v", err)
	}
	if err := tx.Commit(); !errors.Is(err, ErrGoneAway) {
		t.Fatalf("the commit past the budget answered %v, want ErrGoneAway", err)
	}

	// And the abandoned transaction did not take the handle with it: the
	// write is gone, the connection still answers.
	fault.Heal()
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&rows); err != nil {
		t.Fatalf("the handle did not survive its abandoned transaction: %v", err)
	}
	if rows != 1 {
		t.Fatalf("%d rows survived a commit that never landed, want 1", rows)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatalf("a healed handle refused a write: %v", err)
	}
}

// The row budget is the other half of the fixture, and the one nothing else
// in the tree can imitate: a result set that fails partway through, so
// `rows.Err()` has something to report and a caller that ignores it hands
// back half an answer as though it were the whole one.
func TestARowBudgetFailsTheIterationRatherThanEndingIt(t *testing.T) {
	t.Parallel()
	db, fault, err := OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, name := range []string{"one", "two", "three"} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO users (username, created_at) VALUES (?, '2026-09-01T00:00:00+00:00')`,
			name); err != nil {
			t.Fatal(err)
		}
	}

	fault.RowsAfter(2)
	rows, err := db.QueryContext(ctx, `SELECT username FROM users ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for rows.Next() {
		var got string
		if err := rows.Scan(&got); err != nil {
			t.Fatal(err)
		}
		read++
	}
	if read != 2 {
		t.Fatalf("the budget let %d rows through, want 2", read)
	}
	if err := rows.Err(); !errors.Is(err, ErrGoneAway) {
		t.Fatalf("a failed iteration reported %v, want ErrGoneAway", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	// Healed, the same query reads the whole set — the fault is a knob, not
	// damage to the file.
	fault.Heal()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("the healed handle counts %d users, want 3", count)
	}
}

// Heal and a zero budget are the two ends of the knob, and a fixture whose
// off switch does not switch off is worse than no fixture: every refusal
// test built on it would pass over a database that never refused.
func TestTheFaultKnobRunsBothWays(t *testing.T) {
	t.Parallel()
	db, fault, err := OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	fault.After(0)
	if _, err := db.ExecContext(ctx, `DELETE FROM users`); !errors.Is(err, ErrGoneAway) {
		t.Fatalf("a zero budget answered %v", err)
	}
	if _, err := db.BeginTx(ctx, nil); !errors.Is(err, ErrGoneAway) {
		t.Fatalf("a zero budget began a transaction anyway: %v", err)
	}
	fault.Heal()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("a healed handle would not ping: %v", err)
	}
	var one int
	if err := db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("a healed handle answered %d, %v", one, err)
	}
	// The schema this fixture claims to be is the recorded one, and the
	// claim is checkable from here: the text it executes and the file it
	// built have to name the same tables.
	if !strings.Contains(Schema(), "user_version") {
		t.Fatal("the recorded schema no longer carries its user_version pragma")
	}
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == 0 {
		t.Fatal("a scratch database reports version zero; the ladder's height did not apply")
	}
}

// The handle is a real *sql.DB over a real driver, and a test that leans on
// that should be able to say so: the connector hands back the driver it
// wraps rather than a stand-in.
func TestAFaultyHandleIsARealDatabaseHandle(t *testing.T) {
	t.Parallel()
	db, _, err := OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	plain, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "other.db")+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plain.Close() })
	if db.Driver() != plain.Driver() {
		t.Fatal("the faulty handle is not standing on the app's own driver")
	}
}
