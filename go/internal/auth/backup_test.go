package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// TestBackupCopiesALiveAppDB proves the whole runbook step: a migrated
// database, backed up while a writer holds it open, yields a copy that
// opens, carries the same schema version, and holds the same rows.
func TestBackupCopiesALiveAppDB(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.db")
	if err := auth.Migrate(src); err != nil {
		t.Fatal(err)
	}

	// A live writer stays open across the backup, as the serving app would.
	writer, err := auth.OpenReadWrite(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()
	if _, err := writer.Exec(
		"INSERT INTO users (username, email, password_hash, is_admin, created_at) VALUES ('gyome', 'g@example.com', 'x', 0, '2026-08-24T00:00:00+00:00')",
	); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "app-backup.db")
	version, err := auth.Backup(context.Background(), src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if version < 12 {
		t.Fatalf("backup reports schema version %d; the ladder tops out past 11", version)
	}

	copyDB, err := auth.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copyDB.Close() }()
	var copied int
	if err := copyDB.QueryRow("PRAGMA user_version").Scan(&copied); err != nil {
		t.Fatal(err)
	}
	if copied != version {
		t.Fatalf("copy is at schema version %d, source reported %d", copied, version)
	}
	var name string
	if err := copyDB.QueryRow("SELECT username FROM users").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "gyome" {
		t.Fatalf("the copy's user is %q", name)
	}
}

// TestBackupRefusesAnExistingDestination pins the refusal: the command that
// protects the good copy may not be the one that overwrites it.
func TestBackupRefusesAnExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "app.db")
	if err := auth.Migrate(src); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "already.db")
	if _, err := auth.Backup(context.Background(), src, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Backup(context.Background(), src, dest); err == nil {
		t.Fatal("a second backup onto the same path did not refuse")
	}
}
