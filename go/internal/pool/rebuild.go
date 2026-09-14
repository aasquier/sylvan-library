package pool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Rebuilding the pool into a new file instead of over the old one.
//
// **Why, measured.** The loaders empty a table and refill it inside one
// transaction, which is the right shape for not leaving a half-loaded library
// behind — but DuckDB keeps the emptied pages and never gives them back, so
// every refresh grew the file by roughly 37 MB of nothing. On 2026-09-13 the
// served pool was 261,894,144 bytes; a from-scratch build of the very same
// rows — same loader, same bulk files, 35,517 oracle cards and 108,583
// printings either way — came to 54,538,240. Four fifths of the file was dead
// pages. Over the same fortnight the source data had grown 0.3% and the file
// had grown 16.8%, which is the shape of a leak rather than of growth.
//
// **What it buys besides the bytes.** The old path's guarantee was that an
// interrupted refresh rolls back to the pool it started with. This one never
// touches the old pool at all: the rows go into a file beside it and the last
// act is a rename, so a refresh that dies at any point — including one whose
// machine disappears mid-load — leaves the served pool exactly as it was,
// with a stray `.rebuilding` file as the only trace. The next run removes it.
//
// **What it must not lose.** `price_history` is append-only and irreplaceable
// (`schema.sql` says so where it is declared): it is the baseline that makes
// "cheap right now" mean anything, and no bulk file can reconstruct it. A
// rebuild that simply rebuilt would have thrown away every snapshot ever
// taken, silently, and the loss would only have surfaced weeks later as a
// price chart with no history behind it. Carrying it across is therefore the
// one non-negotiable step in here, and [rebuild.finish] does it before
// anything is renamed.

// rebuildCatalog is the name the half-built file is ATTACHed under. It is
// visible in nothing a person reads; it exists so the loaders can qualify a
// table.
const rebuildCatalog = "rebuild"

// rebuildSuffix marks the half-built file. It sits beside the pool on purpose
// — [os.Rename] is only atomic within a filesystem, and "next to the thing it
// will become" is the only placement that can promise that.
const rebuildSuffix = ".rebuilding"

// carried are the tables a rebuild copies from the old pool rather than
// loading from Scryfall, because nothing upstream could put them back.
//
// A list rather than a line of SQL so that adding a table to `schema.sql`
// that a bulk file cannot fill is a change *here*, next to the reason. If you
// are adding one and wondering whether it belongs: ask whether a fresh
// `mtglab data refresh` on an empty volume would reproduce it. If yes, leave
// it out; if no, it goes in this list or it gets destroyed on the next
// refresh.
var carried = []string{"price_history"}

// rebuild is one in-flight rebuild: the writer holding the old pool, a
// connection with the new file attached to it, and the paths the last step
// renames between.
type rebuild struct {
	db        *sql.DB
	conn      *sql.Conn
	dbPath    string
	buildPath string
	attached  bool
}

// startRebuild creates the file the refresh will fill and attaches it to the
// connection that is already holding the old pool open.
//
// **The new file is created through [OpenWriter] and immediately closed**,
// rather than by letting ATTACH conjure an empty database. That is what makes
// the rebuilt pool the same pool: OpenWriter is the one place that applies
// `schema.sql` *and* the added-column ladder, so a file built here has
// exactly the shape the app expects and can never be a rung behind. Opening
// and closing it costs a few milliseconds and removes a whole class of "the
// rebuilt pool is missing a column" bug.
func startRebuild(ctx context.Context, db *sql.DB, dbPath string) (*rebuild, error) {
	buildPath := dbPath + rebuildSuffix

	// A previous run that died leaves one of these. It is ours by name and
	// carries nothing anybody wants, so it goes rather than blocking the run.
	if err := removeBuild(buildPath); err != nil {
		return nil, err
	}

	fresh, err := OpenWriter(ctx, buildPath)
	if err != nil {
		return nil, fmt.Errorf("pool rebuild: preparing %s: %w", buildPath, err)
	}
	if err := fresh.Close(); err != nil {
		_ = removeBuild(buildPath)
		return nil, fmt.Errorf("pool rebuild: preparing %s: %w", buildPath, err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		_ = removeBuild(buildPath)
		return nil, fmt.Errorf("pool rebuild: %w", err)
	}

	r := &rebuild{db: db, conn: conn, dbPath: dbPath, buildPath: buildPath}
	if _, err := conn.ExecContext(ctx,
		"ATTACH '"+buildPath+"' AS "+rebuildCatalog); err != nil {
		_ = conn.Close()
		_ = removeBuild(buildPath)
		return nil, fmt.Errorf("pool rebuild: attaching %s: %w", buildPath, err)
	}
	r.attached = true
	return r, nil
}

// finish carries the irreplaceable tables across, lets go of both files and
// renames the new one into place. After it returns nil the pool on disk is
// the rebuilt one and [rebuild.abandon] has nothing left to undo.
//
// **The order is the whole safety argument** and it only reads one way: copy
// what cannot be reconstructed, detach, close every handle, *then* rename. A
// rename before the copy would publish a pool with no price history; a rename
// before the close would hand the app a file that DuckDB still thought it
// owned.
func (r *rebuild) finish(ctx context.Context) error {
	for _, table := range carried {
		if _, err := r.conn.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO %s.%s SELECT * FROM %s", rebuildCatalog, table, table),
		); err != nil {
			return fmt.Errorf("pool rebuild: carrying %s across: %w", table, err)
		}
	}

	// Record what the prices were, now that the new printings are in and the
	// history they extend has been carried over.
	//
	// **A refresh is the only moment the numbers move**, which is the whole
	// argument for doing it here. `printings.price_usd` is written by the
	// loader and by nothing else in the app, so between one refresh and the
	// next every price in the pool is a constant -- and a snapshot taken on
	// any other day records the same figures again under a new date, which
	// is not history, it is a repeated reading of a stopped clock. Left to a
	// separate command, the table gained a row whenever somebody happened to
	// remember: the instance had exactly two days in it by 2026-09-14, and
	// they were seventeen days apart.
	//
	// Inside the rebuild rather than after the rename, so it lands or does
	// not land with everything else -- a refresh cannot half-succeed into a
	// pool whose prices are recorded but whose rows are not.
	if _, err := r.conn.ExecContext(ctx, snapshotInto(rebuildCatalog)); err != nil {
		return fmt.Errorf("pool rebuild: recording the prices it loaded: %w", err)
	}

	if _, err := r.conn.ExecContext(ctx, "DETACH "+rebuildCatalog); err != nil {
		return fmt.Errorf("pool rebuild: detaching: %w", err)
	}
	r.attached = false

	if err := r.conn.Close(); err != nil {
		return fmt.Errorf("pool rebuild: releasing the connection: %w", err)
	}
	if err := r.db.Close(); err != nil {
		return fmt.Errorf("pool rebuild: releasing the old pool: %w", err)
	}

	// Wear the old file's mode and owner before taking its place. On the
	// instance a refresh driven over `fly ssh` runs as root while the app
	// runs as its own user, so a file that arrives root-owned is a pool the
	// app cannot write at the next refresh. Mode is the half that always
	// works; the chown needs privilege we may not have, and failing the whole
	// refresh over it would be worse than the thing it prevents -- the rows
	// are good, and `docs/HOSTING.md` already documents the restart that
	// repairs ownership.
	if err := wearTheOldSkin(r.dbPath, r.buildPath); err != nil {
		return err
	}

	if err := os.Rename(r.buildPath, r.dbPath); err != nil {
		return fmt.Errorf("pool rebuild: putting the new pool in place: %w", err)
	}
	return nil
}

// abandon is the failure path: detach if still attached, and take the
// half-built file with it. **The old pool is never touched by any of this**,
// which is why abandon has nothing to restore -- there is no torn state to
// repair, only litter to pick up.
//
// Errors are dropped deliberately. Every caller is already returning the
// error that brought it here, and a failure to tidy must not replace a
// diagnosis of what actually went wrong.
func (r *rebuild) abandon(ctx context.Context) {
	if r.attached {
		_, _ = r.conn.ExecContext(ctx, "DETACH "+rebuildCatalog)
		r.attached = false
	}
	_ = r.conn.Close()
	_ = removeBuild(r.buildPath)
}

// removeBuild deletes a half-built pool, tolerating its absence. Anything
// else -- a permission, a directory in the way -- is reported, because a
// build path that will not clear is a run that cannot safely start.
func removeBuild(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("pool rebuild: clearing %s: %w", path, err)
}

// wearTheOldSkin copies the served pool's mode, and best-effort its
// ownership, onto the file about to replace it. A pool that does not exist
// yet has no skin to pass on, which is not an error: the first refresh on an
// empty volume creates the file for the first time.
func wearTheOldSkin(dbPath, buildPath string) error {
	info, err := os.Stat(dbPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("pool rebuild: reading %s: %w", dbPath, err)
	}
	if err := os.Chmod(buildPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("pool rebuild: setting the mode on %s: %w", buildPath, err)
	}
	if uid, gid, ok := ownerOf(info); ok {
		// Best effort, and silent on refusal -- see [rebuild.finish].
		_ = os.Chown(buildPath, uid, gid)
	}
	return nil
}
