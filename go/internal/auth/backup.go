package auth

import (
	"context"
	"fmt"
	"os"
)

// Backup writes a consistent copy of `app.db` to dest while the app serves.
//
// `VACUUM INTO` is the mechanism: it runs one read transaction over the
// source and writes a compact, checkpointed copy, so it is safe against a
// live writer and the copy carries no WAL sidecar to forget. The source is
// opened read-only — the command that protects the file may not be a way to
// change it.
//
// The destination must not exist. A backup that can overwrite a backup is a
// way to lose the good copy to a typo, and the runbook's procedure pulls
// each copy down and removes it from the volume anyway — `app.db` holds
// password hashes and email addresses, and a second copy of those should
// not sit on the box indefinitely.
//
// It returns the source's schema version, so the operator sees which rung
// the copy was taken at — the number that matters when the reason for the
// backup is a migration about to run.
func Backup(ctx context.Context, path, dest string) (int, error) {
	if _, err := os.Stat(dest); err == nil {
		return 0, fmt.Errorf("refusing to overwrite %s; move or remove it first", dest)
	}
	db, err := Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()

	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return 0, fmt.Errorf("backup %s: %w", path, err)
	}
	return version, nil
}
