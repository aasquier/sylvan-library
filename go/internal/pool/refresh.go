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
	},
	"printings": {{"flavor_text", "VARCHAR"}, {"artist", "VARCHAR"}},
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
	index, err := fetchJSON(ctx, BulkIndex)
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
