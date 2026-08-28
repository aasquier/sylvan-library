// The write side of the pool: the refresh behind `mtglab data refresh`.
//
// The shape is the recorded one — the bulk index asked with the same
// User-Agent, the
// download parked as `<kind>-<date><suffix>` and skipped when already there,
// written to a `.part` and renamed only once complete; the same skip rules
// (art-series and token layouts out of `oracle_cards`, digital printings out
// of `printings`); the same row builders, front-face fallback included,
// because a pool that recorded a double-faced card's mana cost as NULL once
// cast Etali on turn one. Two deliberate upgrades, both sanctioned: the
// rows go in through DuckDB's **Appender** rather than a
// prepared statement per row (the ledger's queued 28-minute item — the
// printings load measured ~110 rows/second on the old path), and the DELETE
// and
// the load share **one transaction**, closing the window where an
// interrupted refresh left the pool with no printings at all.
//
// One knowing divergence: older pools store `legalities` and `card_faces`
// as ASCII-escaped JSON text; this writes raw
// UTF-8. Nothing reads those columns except `json_extract_string` and a
// JSON parse, so the difference is invisible to every query — the refresh
// corpus therefore compares those two columns *parsed* and everything else
// exactly.
package pool

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BulkIndex and RefreshUserAgent are long-standing constants, verbatim --
// the User-Agent identifies this tool to Scryfall and must stay stable.
const (
	BulkIndex        = "https://api.scryfall.com/bulk-data"
	RefreshUserAgent = "mtg-lab/0.1 (local personal deckbuilding tool)"
)

// addedColumns is the ALTERs an existing pool needs before
// the loaders may bind the newer columns. `IF NOT EXISTS` makes them no-ops
// on a fresh file, whose schema already carries everything.
var addedColumns = map[string][][2]string{
	"oracle_cards": {
		{"power", "VARCHAR"}, {"toughness", "VARCHAR"}, {"loyalty", "VARCHAR"},
		{"defense", "VARCHAR"}, {"game_changer", "BOOLEAN"},
		{"flavor_text", "VARCHAR"}, {"artist", "VARCHAR"},
		{"all_parts", "JSON"},
	},
	"printings": {{"flavor_text", "VARCHAR"}, {"artist", "VARCHAR"}},
}

// WriterWait is how long a refresh will stand at the door before giving up.
//
// **Sixty seconds because the door opens and shuts on somebody else's clock.**
// A read-only handle held by the running app excludes a writer completely —
// DuckDB's own words, measured: *"Could not set lock on file ...: Conflicting
// lock is held in ... However, you would be able to open this database in
// read-only mode"* — so `mtglab data refresh` on a live instance has always
// been a race against [Pool]'s lease rather than a command. It usually won and
// sometimes did not, and the failure looked like a broken database rather than
// like bad timing, which is how it survived being reported twice.
//
// The lease is ten seconds and [Pool.UseWithoutHolding] now keeps a health
// probe from renewing it, so an idle instance hands the file over on the first
// try. This budget is for the other case: somebody is *actually* reading the
// library while an operator refreshes it, and every page load pushes the lease
// out another ten seconds. A minute of that and the honest answer is that the
// instance is busy — which is what the error then says.
const WriterWait = time.Minute

// writerPoll is how often the door is tried. A refused open costs about nine
// milliseconds (measured, cross-process, against a real 96MB pool), so this is
// cheap enough to be generous with and short enough to catch the one-second
// windows a busy instance leaves.
const writerPoll = 250 * time.Millisecond

// Locked reports whether an open failed because **another process is holding
// the file**, as opposed to failing for a reason waiting will not fix.
//
// **The string is measured rather than remembered**, and `writerlock_test.go`
// is what keeps it measured: it stands up a second process, takes a real
// conflict, and asserts this answers true for it. That is the difference
// between a classifier and a guess — if DuckDB rewords this message, the test
// says so on the next run instead of `data refresh` quietly waiting a minute
// for a permission error to fix itself.
//
// Two phrases rather than one, because the message names the file and the
// holder and neither of those is stable: `Could not set lock` is the sentence,
// and `Conflicting lock` is the diagnosis it carries.
func Locked(err error) bool {
	if err == nil {
		return false
	}
	said := err.Error()
	return strings.Contains(said, "Could not set lock") ||
		strings.Contains(said, "Conflicting lock")
}

// OpenWriterWaiting is [OpenWriter], with the patience to outlast the running
// app's read lease.
//
// `waiting` is called at most once, the first time the door is found shut, so
// a caller can say what is happening — an operator watching a refresh sit
// silent for forty seconds has no way to tell waiting from hung.
//
// **Anything that is not a lock fails immediately.** A missing directory, a
// permission, a corrupt file: none of those improve with time, and spending a
// minute on one before reporting it is worse than reporting it at once.
func OpenWriterWaiting(ctx context.Context, path string, wait time.Duration,
	waiting func()) (*sql.DB, error) {
	db, err := OpenWriter(ctx, path)
	if err == nil || !Locked(err) {
		return db, err
	}
	if waiting != nil {
		waiting()
	}
	// A deadline rather than a count of attempts: what matters is how long an
	// operator is kept standing there, and a refused open's own cost is not
	// fixed.
	until := time.Now().Add(wait)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(writerPoll):
		}
		db, err = OpenWriter(ctx, path)
		if err == nil || !Locked(err) {
			return db, err
		}
		if !time.Now().Before(until) {
			return nil, fmt.Errorf(
				"the card pool is still held by another process after %s "+
					"(the running app reads it, and lets go about %s after "+
					"its last use): %w", wait, IdleLease, err)
		}
	}
}

// OpenWriter opens the pool file read-write, creating the schema if the file
// is fresh and applying the added-column ladder if it is old — `db.connect`'s
// write half. The one other read-write open in the repo is `pooltest`'s.
func OpenWriter(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	if _, err := db.ExecContext(ctx, Schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pool schema: %w", err)
	}
	for table, columns := range addedColumns {
		for _, col := range columns {
			if _, err := db.ExecContext(ctx, fmt.Sprintf(
				"ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s",
				table, col[0], col[1])); err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("pool column %s.%s: %w", table, col[0], err)
			}
		}
	}
	return db, nil
}

// BulkDownloadURL prefers the current `jsonl_download_uri` and falls back to
// the legacy `download_uri`, failing loudly rather than with a bare missing
// key if Scryfall's index format changes again.
func BulkDownloadURL(entry map[string]any) (string, error) {
	for _, key := range []string{"jsonl_download_uri", "download_uri"} {
		if url, _ := entry[key].(string); url != "" {
			return url, nil
		}
	}
	keys := make([]string, 0, len(entry))
	for k := range entry {
		keys = append(keys, k)
	}
	return "", fmt.Errorf("bulk entry %v has no download URL; Scryfall's "+
		"index format may have changed again (keys: %v)", entry["type"], keys)
}

// DownloadBulk fetches one Scryfall bulk file into destDir, exactly as
// served, compression included. Skips the download when the dated local copy
// already exists; writes to a `.part` and renames only once complete, so an
// interrupted download is never mistaken for a valid cached copy.
func DownloadBulk(ctx context.Context, kind, destDir string) (string, error) {
	return DownloadBulkFrom(ctx, BulkIndex, kind, destDir)
}

// DownloadBulkFrom is [DownloadBulk] against a named index, which is how the
// tests reach the download path at all.
//
// The index URL is a parameter rather than a package-level variable a test
// swaps: the swap would be shared state, `-race` would have something to say
// about it, and every test that touched it would have to be serial. Passed
// down, the whole path -- the entry pick, the suffix rules, the dated skip,
// the `.part` rename -- is exercised by parallel tests against an
// `httptest.Server`, and no production caller has a second way to spell the
// real index.
func DownloadBulkFrom(ctx context.Context, indexURL, kind, destDir string) (string, error) {
	index, err := fetchJSON(ctx, indexURL)
	if err != nil {
		return "", err
	}
	data, _ := index["data"].([]any)
	var entry map[string]any
	for _, item := range data {
		if e, ok := item.(map[string]any); ok && e["type"] == kind {
			entry = e
			break
		}
	}
	if entry == nil {
		return "", fmt.Errorf("unknown bulk type %q", kind)
	}
	url, err := BulkDownloadURL(entry)
	if err != nil {
		return "", err
	}
	suffix := ".json"
	for _, s := range []string{".jsonl.gz", ".jsonl", ".json.gz"} {
		if strings.HasSuffix(url, s) {
			suffix = s
			break
		}
	}
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("bulk download: %w", err)
	}
	updated, _ := entry["updated_at"].(string)
	stamp := updated
	if len(stamp) > 10 {
		stamp = stamp[:10]
	}
	target := filepath.Join(destDir, kind+"-"+stamp+suffix)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", RefreshUserAgent)
	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("bulk download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bulk download: HTTP %s", resp.Status)
	}
	part := target + ".part"
	fh, err := os.Create(part) // #nosec G304 -- destDir is the configured shelf
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fh, resp.Body); err != nil {
		_ = fh.Close()
		_ = os.Remove(part)
		return "", fmt.Errorf("bulk download: %w", err)
	}
	if err := fh.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(part, target); err != nil {
		return "", err
	}
	return target, nil
}

func fetchJSON(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", RefreshUserAgent)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bulk index: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bulk index: HTTP %s", resp.Status)
	}
	var payload map[string]any
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("bulk index: %w", err)
	}
	return payload, nil
}

// IterCards streams a bulk file, one card object at a time, into yield.
// Both formats Scryfall has served — JSONL (current) and a single array
// (legacy), either gzipped — dispatched on the first token rather than the
// filename, so a file decompressed or renamed by hand still reads.
func IterCards(path string, yield func(map[string]any) error) error {
	fh, err := os.Open(path) // #nosec G304 -- the caller downloaded it
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	var reader io.Reader = fh
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(fh)
		if err != nil {
			return err
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	dec := json.NewDecoder(reader)
	dec.UseNumber()
	first, err := dec.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	if delim, ok := first.(json.Delim); ok && delim == '[' {
		for dec.More() {
			var card map[string]any
			if err := dec.Decode(&card); err != nil {
				return err
			}
			if err := yield(card); err != nil {
				return err
			}
		}
		return nil
	}
	// JSONL: the token decoder has consumed the first object's opening
	// brace's worth of structure only if the first token was one — but for
	// an object stream, decode whole values in a loop instead.
	if delim, ok := first.(json.Delim); ok && delim == '{' {
		// Re-open cleanly: a mixed token/value read is fiddlier than a
		// second pass is expensive.
		return iterJSONL(path, yield)
	}
	return fmt.Errorf("%s: unrecognised bulk format", filepath.Base(path))
}

func iterJSONL(path string, yield func(map[string]any) error) error {
	fh, err := os.Open(path) // #nosec G304 -- as above
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	var reader io.Reader = fh
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(fh)
		if err != nil {
			return err
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	dec := json.NewDecoder(reader)
	dec.UseNumber()
	for {
		var card map[string]any
		if err := dec.Decode(&card); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		if err := yield(card); err != nil {
			return err
		}
	}
}
