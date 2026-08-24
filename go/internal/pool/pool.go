// Package pool is the card pool as the served app holds it -- the DuckDB file
// `mtglab data refresh` writes, opened read-only and **held on a lease**,
// and every read the app makes of it.
//
// Read-only, so a running refresh
// (which wants DuckDB's single writer lock) degrades the app rather than being
// locked out by it. And a lease rather than a handle held for the life of the
// process, because **a held
// read-only handle is a held shared lock**, and a door that kept the pool
// open forever would refuse every `data refresh` on the instance -- which is
// exactly what the app did until 2026-08-19, when its keeper's lease
// was longer than the health check's cadence. So `Use` opens on demand, a
// reaper closes the database once nobody has asked for it in `IdleLease`
// (ten seconds, settled deliberately: shorter
// than `fly.toml`'s 30-second health check, longer than a page load's burst
// of requests), and the file's stamp (mtime and size) is checked on the way
// past so a pool that moved is re-opened rather than served from a snapshot.
// DuckDB's Go driver makes the one-database-per-DSN arrangement natural: a
// `*sql.DB` is the loaded instance, its connections are `duckdb_connect` on
// that instance, and closing it releases the file.
//
// Caching is keyed on the stamp, because the stamp
// is what makes a cache of a read-only file honest: a refresh rewrites the
// file, changes `mtime_ns` and size, and every entry keyed on the old stamp
// becomes unreachable.
//
// The driver is github.com/duckdb/duckdb-go (the community driver that used
// to live at marcboeker/go-duckdb), which links a prebuilt libduckdb per
// platform -- pinned at 1.5.5, the version the pool files were written
// under. Importing this
// package into the door is what makes the door a CGO build.
package pool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	// The driver registers itself as "duckdb".
	_ "github.com/duckdb/duckdb-go/v2"
)

// IdleLease is how long the pool may go unwanted before it is handed back
// (`service._KEEPER_IDLE`): spans a page load's burst, shorter than the
// platform health check's interval, so a refresh started against an idle app
// waits this long at worst.
const IdleLease = 10 * time.Second

// ErrNoPool is `_connect()` returning None: the file is not there yet, or it
// could not be opened -- most likely a `data refresh` holding the write lock.
// The app answers in degraded form rather than failing every request.
var ErrNoPool = errors.New("no card pool yet -- run `mtglab data refresh`")

// Open opens the pool at path read-only, once. An empty path opens an
// in-memory database, which is what the tests use to build a pool of their
// own. Callers that want the lease use a Pool.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := path
	if path != "" {
		dsn = path + "?access_mode=READ_ONLY"
	}
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open pool: %w", err)
	}
	return db, nil
}

// Count answers `SELECT count(*) FROM <table>`, the query `service.health`
// runs for `oracle_cards` and `printings`.
func Count(ctx context.Context, db *sql.DB, table string) (int64, error) {
	if table != "oracle_cards" && table != "printings" {
		return 0, fmt.Errorf("count: %q is not a pool table", table)
	}
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return n, nil
}

// stamp is how the pool file looked when it was opened.
type stamp struct {
	mtime int64
	size  int64
}

func stat(path string) (stamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return stamp{}, err
	}
	return stamp{mtime: info.ModTime().UnixNano(), size: info.Size()}, nil
}

// Pool is the leased, stamp-checked pool. Safe for concurrent use.
type Pool struct {
	path string
	idle time.Duration
	log  *slog.Logger

	mu       sync.Mutex
	db       *sql.DB
	stamp    stamp
	leases   int
	lastUsed time.Time
	reaping  bool
	columns  map[string]map[string]bool // per open: table -> column set
	cards    *cardCache                 // per open: get_cards memo
	stale    *bool                      // per open: the staleness verdict
	memos    map[string]*counts         // per open: each memo's hits and misses
}

// The memo names `counts` are kept under. Named constants rather than
// literals at the call sites, because a typo in a counter's name is a
// silently separate counter that always reads zero -- which looks exactly
// like the dead cache the counter exists to find.
const (
	MemoColumns = "columns"
	MemoCards   = "cards"
	MemoStale   = "staleness"
)

// counts is one memo's answers since the pool was opened.
//
// **A cache can be correct, tested, and never once used**, and no test finds
// that -- the standing example in this tree was keyed on a per-request handle
// in an app where every request opened its own, so every entry was written and
// thrown away by the connection that owned it. Only a hit count finds that
// shape. These are the smallest instrument that does: per open, beside the
// memos themselves, read by the package's tests, which is what holds each memo
// to being a memo rather than a decoration.
//
// Nothing renders them. A hit rate is a fact about the machinery, and
// commandment 10 keeps the machinery out of sight.
type counts struct{ hits, misses int }

// note records one memo answer. `noteLocked` is the same for a caller that
// already holds p.mu -- which `Columns` does at the moment it learns the
// answer, and re-taking a `sync.Mutex` there would deadlock rather than
// mis-count.
func (p *Pool) note(name string, hit bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.noteLocked(name, hit)
}

func (p *Pool) noteLocked(name string, hit bool) {
	// nil between opens: a memo answer with no memo behind it is not a hit
	// rate, and counting it would make a closed pool look busy.
	if p.memos == nil {
		return
	}
	c := p.memos[name]
	if c == nil {
		c = &counts{}
		p.memos[name] = c
	}
	if hit {
		c.hits++
		return
	}
	c.misses++
}

// New makes a pool over path. Nothing is opened until the first Use.
func New(path string, log *slog.Logger) *Pool {
	if log == nil {
		log = slog.Default()
	}
	return &Pool{path: path, idle: IdleLease, log: log}
}

// Conn is one leased use of the pool: the database and the per-open caches.
type Conn struct {
	db   *sql.DB
	pool *Pool
}

// Use runs fn against the pool, opening it if it is closed and re-opening it
// if the file has moved since. ErrNoPool when there is no pool to open; any
// other error is fn's own. The lease is held for the duration of fn and
// released after, so the reaper never closes a database under a query.
func (p *Pool) Use(ctx context.Context, fn func(*Conn) error) error {
	conn, err := p.acquire(ctx)
	if err != nil {
		return err
	}
	defer p.release()
	return fn(conn)
}

func (p *Pool) acquire(ctx context.Context) (*Conn, error) {
	now, err := stat(p.path)
	if err != nil {
		// No file: `_connect` returns None. (A pool mid-rewrite by a refresh
		// usually still exists; that case fails at open, below.)
		p.mu.Lock()
		p.closeLocked()
		p.mu.Unlock()
		return nil, ErrNoPool
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil && p.stamp != now {
		// The file moved under us -- a refresh finished. Let the old instance
		// go once its leases drain; until then, new users still get it: a
		// coherent snapshot, a moment out of date, never a torn read.
		if p.leases == 0 {
			p.closeLocked()
		}
	}
	if p.db == nil {
		db, err := Open(ctx, p.path)
		if err != nil {
			p.log.Warn("the card pool could not be opened; answering without it", "error", err)
			return nil, ErrNoPool
		}
		// A handful of connections is plenty; DuckDB parallelises inside one.
		db.SetMaxOpenConns(4)
		p.db = db
		p.stamp = now
		p.columns = map[string]map[string]bool{}
		p.cards = newCardCache()
		p.stale = nil
		p.memos = map[string]*counts{}
		if !p.reaping {
			p.reaping = true
			go p.reap()
		}
	}
	p.leases++
	p.lastUsed = time.Now()
	return &Conn{db: p.db, pool: p}, nil
}

func (p *Pool) release() {
	p.mu.Lock()
	p.leases--
	p.lastUsed = time.Now()
	p.mu.Unlock()
}

// closeLocked drops the open database. Callers hold p.mu.
func (p *Pool) closeLocked() {
	if p.db != nil {
		_ = p.db.Close()
	}
	p.db = nil
	p.columns = nil
	p.cards = nil
	p.stale = nil
	p.memos = nil
}

// staleness is the memoised verdict, if this open has one yet.
func (c *Conn) staleness() (bool, bool) {
	c.pool.mu.Lock()
	defer c.pool.mu.Unlock()
	if c.pool.db != c.db || c.pool.stale == nil {
		c.pool.noteLocked(MemoStale, false)
		return false, false
	}
	c.pool.noteLocked(MemoStale, true)
	return *c.pool.stale, true
}

// rememberStaleness files the verdict for the rest of this open. A Conn that
// outlived its open (a shutdown) files nothing rather than teaching the next
// open an answer about the previous pool.
func (c *Conn) rememberStaleness(answer bool) {
	c.pool.mu.Lock()
	defer c.pool.mu.Unlock()
	if c.pool.db != c.db {
		return
	}
	c.pool.stale = &answer
}

// reap is `service._reap_keeper`: hand the pool back once nobody has wanted
// it for the lease. One goroutine per Pool, started at the first open and
// never stopped -- it costs one timer, and a process that opened the pool
// once will open it again.
func (p *Pool) reap() {
	ticker := time.NewTicker(p.idle / 3)
	defer ticker.Stop()
	for range ticker.C {
		p.ReapOnce()
	}
}

// ReapOnce is one pass of the lease check, split out so the decision is
// testable without waiting on a clock. True when the pool was handed back.
func (p *Pool) ReapOnce() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil && p.leases == 0 && time.Since(p.lastUsed) > p.idle {
		p.closeLocked()
		return true
	}
	return false
}

// Held reports whether the pool is open right now -- what `mtglab bench
// caches` would call the keeper's size.
func (p *Pool) Held() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.db != nil
}

// Close hands the pool back now, leases or not. For a shutdown.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

// DB is the leased database, for a query this package has no helper for.
func (c *Conn) DB() *sql.DB { return c.db }

// Columns is the columns `table` actually has
// on this pool, memoised per open. Needed because the read-only handle
// cannot migrate itself, so a pool built before a column existed must
// degrade to "we do not know" rather than fail to bind.
func (c *Conn) Columns(ctx context.Context, table string) (map[string]bool, error) {
	c.pool.mu.Lock()
	if cols, ok := c.pool.columns[table]; ok {
		c.pool.noteLocked(MemoColumns, true)
		c.pool.mu.Unlock()
		return cols, nil
	}
	c.pool.noteLocked(MemoColumns, false)
	c.pool.mu.Unlock()
	rows, err := c.db.QueryContext(ctx,
		"SELECT column_name FROM information_schema.columns WHERE table_name = ?", table)
	if err != nil {
		return nil, fmt.Errorf("columns of %s: %w", table, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	c.pool.mu.Lock()
	if c.pool.columns != nil {
		c.pool.columns[table] = cols
	}
	c.pool.mu.Unlock()
	return cols, nil
}
