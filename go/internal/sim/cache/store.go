package cache

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	// Registers the "sqlite" driver. Same choice as `internal/auth` and
	// `internal/decklog`: the pure-Go translation, so the door stays a binary
	// with no C toolchain behind SQLite.
	_ "modernc.org/sqlite"
)

// Store is the `sim_cache` table in `app.db`, per ADR 4 and ADR 18.
//
// **A nil `*Store` is a working store that caches nothing.** Every method
// below is safe on one, and returns exactly what a miss returns. That is the
// Go spelling of Python's "a `None` key is a miss without touching the
// database", and it is what lets a caller hold `*Store` without a branch for
// the machine that has no `app.db`.
//
// **Nothing here returns an error to the caller.** A cache is an optimisation,
// and an optimisation that can turn a working simulation into a failed one is
// a bad trade -- the same call `sim/cache.py` makes, and the same call
// `decklog.Record` and `claude/ledger.py` make. Failures are logged.
type Store struct {
	db  *sql.DB
	log *slog.Logger
}

// Open attaches to `app.db` for the simulation cache.
//
// **`mode=rw`, never `rwc`.** The ladder (`auth.Migrate`) runs once at boot
// and is the only creator; a file this created would be a database at version
// zero beside the real one. An absent `app.db` is an error here and a caller
// that treats it as "no cache" gets exactly that by holding a nil `*Store`.
//
// Failing at Open rather than at the first lookup is deliberate: a cache that
// discovers its own absence on the hot path discovers it in a worker thread,
// where the only evidence is a log line nobody reads.
func Open(path string, logger *slog.Logger) (*Store, error) {
	if logger == nil {
		logger = slog.Default()
	}
	dsn := "file:" + url.PathEscape(path) +
		"?mode=rw&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app.db for the simulation cache: %w", err)
	}
	// One writer. SQLite serialises them anyway, and a pool of them would only
	// convert waiting-in-Go into waiting-on-the-file lock. The file is in WAL
	// mode (Python set it, and WAL is persistent in the file), so this never
	// blocks a reader.
	db.SetMaxOpenConns(1)
	var one int
	err = db.QueryRow("SELECT 1 FROM sim_cache LIMIT 1").Scan(&one)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		_ = db.Close()
		return nil, fmt.Errorf("app.db has no usable sim_cache: %w", err)
	}
	return &Store{db: db, log: logger}, nil
}

// Close releases the handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB is the handle, for a caller that has to share the connection.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Hit is `cache.Hit`: a stored result, and when it was computed.
//
// The timestamp is not decoration -- a cached figure that cannot say how old
// it is cannot be reported honestly, and the app shows it.
//
// `Result` stays raw. Decoding it here would mean a `map[string]any`, and
// re-encoding one of those sorts the keys where a Python dict keeps its
// insertion order; the caller unmarshals into the same struct it stored.
type Hit struct {
	Result    json.RawMessage
	CreatedAt string
}

// Get is `cache.get`: the stored result for `key`, or nil for a miss.
//
// An empty key is a miss without touching the database, which is what makes
// "caching is off" a one-line concern for callers.
func (s *Store) Get(ctx context.Context, key string) *Hit {
	if s == nil || s.db == nil || key == "" {
		return nil
	}
	var blob, createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT result_json, created_at FROM sim_cache WHERE key = ?",
		key).Scan(&blob, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		s.log.Warn("simulation cache read failed", "err", err)
		return nil
	}
	// Touched on read so eviction is least-recently-*used* rather than
	// oldest-computed. The numbers everybody looks at are exactly the ones
	// worth keeping. A failure here is not a failure of the read: the answer
	// is already in hand, and the only cost is one row's place in the queue.
	if _, err := s.db.ExecContext(ctx,
		"UPDATE sim_cache SET last_used_at = ? WHERE key = ?",
		stamp(time.Now()), key); err != nil {
		s.log.Warn("simulation cache touch failed", "err", err)
	}
	return &Hit{Result: json.RawMessage(blob), CreatedAt: createdAt}
}

// Put is `cache.put`: store `result`, evicting the least recently used rows if
// over `MaxRows`. Never fails the caller.
//
// `result` is marshalled with `encoding/json`, and the bytes it produces are
// deliberately **not** held to Python's: nothing hashes a stored blob, and the
// two runtimes never read each other's rows because they never share a key.
// One thing about the value does matter, and it is the constraint
// `internal/jobs` found first: **pass a struct, never a `map[string]any`.**
// `encoding/json` sorts map keys and a Python dict does not, so a result
// assembled as a map would come back out of the cache with its fields in a
// different order from the one the same route serves on a miss.
func (s *Store) Put(ctx context.Context, key, kind string, result any) {
	if s == nil || s.db == nil || key == "" {
		return
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Python does not escape `<`, `>` or `&`, and neither should a row that a
	// person may read out of the table with `sqlite3`.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		s.log.Warn("simulation result is not storable", "err", err)
		return
	}
	blob := bytes.TrimRight(buf.Bytes(), "\n")

	now := stamp(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.log.Warn("simulation cache write failed", "err", err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO sim_cache (key, kind, result_json, created_at,"+
			" last_used_at) VALUES (?, ?, ?, ?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET "+
			"result_json = excluded.result_json, "+
			"created_at = excluded.created_at, "+
			"last_used_at = excluded.last_used_at",
		key, kind, string(blob), now, now); err != nil {
		s.log.Warn("simulation cache write failed", "err", err)
		return
	}
	var total int
	if err := tx.QueryRowContext(ctx,
		"SELECT count(*) FROM sim_cache").Scan(&total); err != nil {
		s.log.Warn("simulation cache write failed", "err", err)
		return
	}
	if total > MaxRows {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM sim_cache WHERE key IN ("+
				"SELECT key FROM sim_cache ORDER BY last_used_at LIMIT ?)",
			total-MaxRows); err != nil {
			s.log.Warn("simulation cache eviction failed", "err", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.log.Warn("simulation cache write failed", "err", err)
	}
}

// Stats is `cache.stats`: what is in there, for `mtglab sim cache`.
//
// Reports whether caching is even switched on, because "the cache is empty"
// and "the cache is disabled" look identical from a row count and want
// completely different responses.
//
// Field order is Python's dict order, so a served payload built from this
// reads the same in either runtime.
type Stats struct {
	Enabled bool           `json:"enabled"`
	Rows    int            `json:"rows"`
	ByKind  map[string]int `json:"by_kind"`
	Oldest  *string        `json:"oldest"`
	Newest  *string        `json:"newest"`
	Bytes   int64          `json:"bytes"`
}

// Stats reports the table's contents. A nil store reports a disabled,
// empty cache, which is what a machine with no `app.db` should say.
func (s *Store) Stats(ctx context.Context) Stats {
	out := Stats{Enabled: Fingerprint() != "", ByKind: map[string]int{}}
	if s == nil || s.db == nil {
		return out
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT kind, count(*) AS n, sum(length(result_json)) AS b, "+
			"min(created_at) AS oldest, max(created_at) AS newest "+
			"FROM sim_cache GROUP BY kind ORDER BY kind")
	if err != nil {
		s.log.Warn("simulation cache stats failed", "err", err)
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind string
		var n int
		var b sql.NullInt64
		var oldest, newest sql.NullString
		if err := rows.Scan(&kind, &n, &b, &oldest, &newest); err != nil {
			s.log.Warn("simulation cache stats failed", "err", err)
			return out
		}
		out.ByKind[kind] = n
		out.Rows += n
		out.Bytes += b.Int64
		if oldest.Valid && (out.Oldest == nil || oldest.String < *out.Oldest) {
			v := oldest.String
			out.Oldest = &v
		}
		if newest.Valid && (out.Newest == nil || newest.String > *out.Newest) {
			v := newest.String
			out.Newest = &v
		}
	}
	if err := rows.Err(); err != nil {
		s.log.Warn("simulation cache stats failed", "err", err)
	}
	return out
}

// Clear is `cache.clear`: drop every row. Returns how many went.
func (s *Store) Clear(ctx context.Context) int {
	if s == nil || s.db == nil {
		return 0
	}
	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sim_cache").Scan(&total); err != nil {
		s.log.Warn("simulation cache clear failed", "err", err)
		return 0
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sim_cache"); err != nil {
		s.log.Warn("simulation cache clear failed", "err", err)
		return 0
	}
	return total
}

// stamp is `datetime.now(UTC).isoformat()`, and both details are the ones
// `internal/jobs` had to find the hard way: **the fractional part vanishes
// entirely when there is none** (Python writes `...:20+00:00`, not
// `...:20.000000+00:00`), and **the offset is spelled `+00:00`, never `Z`**.
//
// It matters here for the same reason it mattered there: `created_at` is what
// the app renders beside a cached figure, and `last_used_at` is the **sort
// key** eviction reads -- as text, in SQL. Every field is fixed width, and
// within one second the elided form begins with `+` (0x2B), which sorts below
// every digit the six-digit form can start with, so the microsecond-zero row
// is correctly the older one.
func stamp(t time.Time) string {
	t = t.UTC().Truncate(time.Microsecond)
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}
