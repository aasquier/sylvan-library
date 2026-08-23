// Package traffic is `api/traffic.py`: the visitor ledger — how much this
// instance is used, without who. Four facts per request and no fifth: the
// UTC day, the matched route TEMPLATE (never the concrete path — a path can
// carry a slug and a slug can carry a person), the status class, and a
// count. No IP, no user agent, no username, no timestamp finer than the day.
//
// The door records **what the door answers**: its own routes by their
// pattern, the static tiers by their mount prefix (`/assets`, `/tarot`), the
// shell as `/{full_path}` (the template Starlette's catch-all records — read
// off the deployed ledger, not guessed), and everything refused before
// routing or matched by nothing as `(unrouted)`. During coexistence a
// proxied request is deliberately NOT recorded here — Python's own
// middleware counts what Python answers, so the two ledgers partition the
// traffic instead of double-counting it; when the proxy retires, this is
// the only recorder left and the partition is everything.
//
// The two mechanical rules are Python's: counts buffer in memory and flush
// when a request lands more than FlushEvery after the last flush (and when
// the traffic endpoint reads, so the dashboard is never a minute behind),
// and the flush never raises — a full disk loses a minute of counts, not a
// request. The ledger never mints a database: a nil handle drops counts,
// exactly as Python skips a path whose file does not exist.
package traffic

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// FlushEvery is the most a count waits in memory.
const FlushEvery = 60 * time.Second

// Unrouted is the one bucket concrete paths are allowed to share.
const Unrouted = "(unrouted)"

// statusClasses seeds every day row, so a quiet day still carries its
// series — a payload whose keys depend on its data is not a shape, and no
// golden can hold one (the recorded lesson from Phase 7's forge flip).
var statusClasses = [...]string{"2xx", "4xx", "5xx"}

type key struct {
	day, template, class string
}

// Recorder buffers and writes the counts. A nil *Recorder records nothing
// and never fails a request, which is the honest shape for an instance with
// no app.db.
type Recorder struct {
	db  *sql.DB
	log *slog.Logger
	now func() time.Time

	mu        sync.Mutex
	buffer    map[key]int
	lastFlush time.Time
}

// New builds a recorder over the shared read-write app.db handle, or nil
// when there is none.
func New(db *sql.DB, log *slog.Logger) *Recorder {
	if db == nil {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &Recorder{db: db, log: log, now: time.Now,
		buffer: map[key]int{}, lastFlush: time.Now()}
}

// ClassOf is `f"{status // 100}xx"`.
func ClassOf(status int) string { return fmt.Sprintf("%dxx", status/100) }

// Record counts one response, and flushes if one is due. Never raises.
func (r *Recorder) Record(template string, status int) {
	if r == nil {
		return
	}
	day := r.now().UTC().Format("2006-01-02")
	k := key{day, template, ClassOf(status)}
	var taken map[key]int
	r.mu.Lock()
	r.buffer[k]++
	if r.now().Sub(r.lastFlush) >= FlushEvery {
		taken = r.buffer
		r.buffer = map[key]int{}
		r.lastFlush = r.now()
	}
	r.mu.Unlock()
	if taken != nil {
		r.write(taken)
	}
}

// Flush writes everything buffered, now: shutdown, and the traffic
// endpoint's read.
func (r *Recorder) Flush() {
	if r == nil {
		return
	}
	r.mu.Lock()
	taken := r.buffer
	r.buffer = map[key]int{}
	r.lastFlush = r.now()
	r.mu.Unlock()
	if len(taken) > 0 {
		r.write(taken)
	}
}

func (r *Recorder) write(rows map[key]int) {
	for k, count := range rows {
		_, err := r.db.Exec(
			"INSERT INTO request_log (day, route, status_class, count)"+
				" VALUES (?, ?, ?, ?)"+
				" ON CONFLICT(day, route, status_class)"+
				" DO UPDATE SET count = count + excluded.count",
			k.day, k.template, k.class, count)
		if err != nil {
			// Deliberately not re-buffered: a broken database plus a growing
			// buffer is two problems.
			r.log.Warn("request counts were not written", "error", err)
			return
		}
	}
}

// Summary is `traffic.summary`: requests per day by class over `days`, and
// the top routes. Flushes first, and unlike the write path it raises — the
// caller asked a question, and a wrong silent answer would be worse.
func (r *Recorder) Summary(ctx context.Context, days int) (wire.OrderedMap, error) {
	r.Flush()
	now := time.Now
	if r != nil && r.now != nil {
		now = r.now
	}
	cutoff := now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	db := r.handle()
	if db == nil {
		// No app.db: an empty ledger, the same answer Python gives over a
		// database with no rows.
		return wire.OrderedMap{
			{Key: "days", Value: []any{}},
			{Key: "top_routes", Value: []any{}},
		}, nil
	}
	rows, err := db.QueryContext(ctx,
		"SELECT day, status_class, sum(count) FROM request_log"+
			" WHERE day >= ? GROUP BY day, status_class ORDER BY day", cutoff)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var order []string
	byDay := map[string]wire.OrderedMap{}
	for rows.Next() {
		var day, class string
		var count int64
		if err := rows.Scan(&day, &class, &count); err != nil {
			return nil, err
		}
		row, present := byDay[day]
		if !present {
			row = emptyDay(day)
			order = append(order, day)
		}
		row = setClass(row, class, count)
		byDay[day] = row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	top := []any{}
	topRows, err := db.QueryContext(ctx,
		"SELECT route, sum(count) FROM request_log"+
			" WHERE day >= ? GROUP BY route"+
			" ORDER BY sum(count) DESC LIMIT 12", cutoff)
	if err != nil {
		return nil, err
	}
	defer func() { _ = topRows.Close() }()
	for topRows.Next() {
		var route string
		var count int64
		if err := topRows.Scan(&route, &count); err != nil {
			return nil, err
		}
		top = append(top, wire.OrderedMap{
			{Key: "route", Value: route},
			{Key: "count", Value: count},
		})
	}
	if err := topRows.Err(); err != nil {
		return nil, err
	}

	daysOut := make([]any, 0, len(order))
	for _, day := range order {
		daysOut = append(daysOut, byDay[day])
	}
	return wire.OrderedMap{
		{Key: "days", Value: daysOut},
		{Key: "top_routes", Value: top},
	}, nil
}

func (r *Recorder) handle() *sql.DB {
	if r == nil {
		return nil
	}
	return r.db
}

func emptyDay(day string) wire.OrderedMap {
	row := wire.OrderedMap{
		{Key: "day", Value: day},
		{Key: "total", Value: int64(0)},
	}
	for _, class := range statusClasses {
		row = append(row, wire.KV{Key: class, Value: int64(0)})
	}
	return row
}

// setClass is `row[cls] = count; row["total"] += count` — a class outside
// the seeded three (a `3xx`) is appended in encounter order, exactly as a
// Python dict grows.
func setClass(row wire.OrderedMap, class string, count int64) wire.OrderedMap {
	found := false
	for i := range row {
		switch row[i].Key {
		case class:
			row[i].Value = count
			found = true
		case "total":
			row[i].Value = row[i].Value.(int64) + count
		}
	}
	if !found {
		row = append(row, wire.KV{Key: class, Value: count})
	}
	return row
}
