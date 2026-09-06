// Package ledger is where the Claude money went:
// one row per conversation and a roll-up over them.
//
// Every mode already counted its tokens — a Turn carries them and the CLI
// prints them — but the hosted instance, where the spending actually happens,
// discarded the numbers with the job payload. So the only cost figure this
// project had was one argue run measured by hand, and every efficiency
// decision (effort levels, a cheaper model for a mode, the Batch API) was
// going to be made against vibes. This is the accounting those decisions wait
// on.
//
// Three properties, all copied from the sim cache and the deck log, which
// solved the same problems for the same file:
//
//   - **Record never fails the conversation that produced it.** Accounting
//     that can fail a paid, four-minute dossier run is worse than no
//     accounting. A failed write is a logged warning and nothing else.
//   - **app.db is opened `mode=rw`, never `rwc`.** The ladder runs once at
//     boot (`auth.Migrate`), so a missing file is a loud failure at startup
//     rather than a silently-created empty database this handle then writes
//     into.
//   - **Aggregate on purpose.** A row is counters, a mode name, a model id
//     and a stop reason. No user id, no deck slug, no question text — it
//     cannot drift into being a chat log, and ADR 17's who-may-read-what
//     argument never has to be made for it.
//
// The roll-up has two readers — the Admin panel and `mtglab claude usage`, a
// browser and a shell — and [Recorder.Window] is deliberately the only place
// either of them rolls up and prices. Two implementations of one arithmetic is
// two answers to "what is Claude costing me", and the one nobody is looking at
// is the one that goes wrong.
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/prices"
)

// Recorder holds the app.db handle the ledger writes through.
//
// A value rather than a package-level connection so that a test can point
// one at a scratch file.
type Recorder struct {
	db  *sql.DB
	log *slog.Logger
}

// NewRecorder opens app.db for writing.
//
// `mode=rw` and not `rwc`, for the reason the package comment gives: the
// ladder runs at boot, so a missing app.db is a broken deployment and must
// say so here rather than at the first roll-up somebody reads.
func NewRecorder(path string, logger *slog.Logger) (*Recorder, error) {
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		return nil, fmt.Errorf("opening app.db for the Claude ledger: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{db: db, log: logger}, nil
}

// DB exposes the handle for tests and for a caller that already has one.
func (r *Recorder) DB() *sql.DB { return r.db }

// Close releases the handle. A nil Recorder closes nothing, which is the
// no-ledger case rather than an error.
func (r *Recorder) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Row is one conversation's accounting, in the recorded field order.
type Row struct {
	Mode            string
	Model           string
	StopReason      string
	Requests        int
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
}

// Record writes one conversation's accounting. It never fails the caller.
//
// Called on every way out of a conversation — answer, refusal, and the
// turn-ceiling exception too, because the tokens a conversation burned before
// failing are exactly the ones worth seeing in a roll-up.
func (r *Recorder) Record(ctx context.Context, row Row) {
	if r == nil || r.db == nil {
		slog.Default().Warn("claude usage record dropped: no app.db",
			"mode", row.Mode, "model", row.Model)
		return
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO claude_usage (created_at, mode, model, stop_reason,"+
			" requests, input_tokens, output_tokens, cache_read_tokens)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		now(), row.Mode, row.Model, row.StopReason, row.Requests,
		row.InputTokens, row.OutputTokens, row.CacheReadTokens)
	if err != nil {
		// A warning, never a return value. The conversation has already
		// happened and has already been paid for; failing it now would lose
		// the answer as well as the accounting.
		r.log.Warn("claude usage record failed",
			"mode", row.Mode, "model", row.Model, "err", err)
	}
}

// Axes are the columns a roll-up may group on.
//
// A fixed set rather than whatever the caller passes, because these names are
// interpolated into SQL: they are column names, which cannot be bound as
// parameters, so the safety has to come from the value never being
// caller-controlled in the first place.
var Axes = []string{"mode", "model"}

// Various is the marker a roll-up writes into the column it aggregated away.
// It matches `tiers.Various`, and the two are asserted equal by a test rather
// than one importing the other: the word belongs to both and neither owns it.
const Various = "(various)"

// Summary is one row of a roll-up. Field order is the SELECT's own order,
// which is also the order the Admin panel reads.
type Summary struct {
	Mode            string `json:"mode"`
	Model           string `json:"model"`
	Conversations   int    `json:"conversations"`
	Requests        int    `json:"requests"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	CacheReadTokens int    `json:"cache_read_tokens"`
	FirstAt         string `json:"first_at"`
	LastAt          string `json:"last_at"`
}

// Summarise returns totals per `by`, most expensive first.
//
// `since` and `until` are ISO-8601 instants compared against created_at, which
// is itself ISO-8601 UTC — string comparison is date comparison for free, and
// a bare date is a perfectly good bound because it sorts exactly where its own
// midnight does. `since` is inclusive and `until` exclusive, so abutting
// windows tile without double-counting the instant they meet; either may be
// empty for "no bound that side". Unlike Record, this DOES return its error:
// it runs when somebody asked a question, and a wrong silent answer would be
// worse than a failure.
//
// **`until` exists so a window that crosses a price change can be priced
// twice.** Money is the only reason: tokens are tokens whenever they were
// spent, but the rate they cost is a function of the date, so a roll-up that
// wants an honest dollar figure asks for one piece per rate window (see
// [Recorder.Window] and `prices.Segments`).
//
// `by` is "mode" (which surface spent it) or "model" (which Claude spent it).
// Both axes come back with the same fields, and every row carries BOTH:
// grouping by one aggregates the other, so the field that was not grouped on
// holds Various rather than an arbitrary winner picked by SQLite. A caller
// pricing a per-mode roll-up therefore gets "unpriced" rather than a number
// computed from whichever model happened to sort first — which would be wrong
// and would look right.
func (r *Recorder) Summarise(ctx context.Context, by, since, until string) ([]Summary, error) {
	if by != "mode" && by != "model" {
		// Quoted as a single-quoted literal, because the sentence reaches a
		// caller that may render it.
		return nil, fmt.Errorf("cannot group by %s -- one of %v", quoted(by), Axes)
	}
	other := "model"
	if by == "model" {
		other = "mode"
	}
	query := "SELECT " + by + ", '" + Various + "' AS " + other +
		", count(*) AS conversations," +
		" sum(requests) AS requests," +
		" sum(input_tokens) AS input_tokens," +
		" sum(output_tokens) AS output_tokens," +
		" sum(cache_read_tokens) AS cache_read_tokens," +
		" min(created_at) AS first_at," +
		" max(created_at) AS last_at" +
		" FROM claude_usage"
	var args []any
	var bounds []string
	if since != "" {
		bounds = append(bounds, "created_at >= ?")
		args = append(args, since)
	}
	if until != "" {
		bounds = append(bounds, "created_at < ?")
		args = append(args, until)
	}
	if len(bounds) > 0 {
		query += " WHERE " + strings.Join(bounds, " AND ")
	}
	query += " GROUP BY " + by +
		" ORDER BY sum(input_tokens + output_tokens) DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rolling up claude usage: %w", err)
	}
	defer rows.Close()

	out := []Summary{}
	for rows.Next() {
		var s Summary
		// The scan order follows the SELECT, and the grouped column comes
		// first — so which of Mode/Model is the group and which is the marker
		// depends on `by`.
		first, second := &s.Mode, &s.Model
		if by == "model" {
			first, second = &s.Model, &s.Mode
		}
		if err := rows.Scan(first, second, &s.Conversations, &s.Requests,
			&s.InputTokens, &s.OutputTokens, &s.CacheReadTokens,
			&s.FirstAt, &s.LastAt); err != nil {
			return nil, fmt.Errorf("reading a claude usage row: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Rollup is one window answered whole: both axes, and what it cost.
//
// **One value because there must be one arithmetic.** The Admin panel and
// `mtglab claude usage` answer the same question about the same rows for the
// same person, from a browser and from a shell — and the moment each of them
// rolls up and prices for itself, the two can disagree by a rounding, a
// window boundary or a `since` that means something slightly different. They
// read this instead, and a test holds their two renderings to the same
// numbers.
type Rollup struct {
	// ByMode is which surface spent it; ByModel is which Claude spent it.
	// Both cover the whole window.
	ByMode  []Summary
	ByModel []Summary
	// CostOf is what each model's row came to, keyed by the model id ByModel
	// carries. A model the table cannot price is present with a zero estimate
	// and its own `Unpriced` count, which is the difference between "cost
	// nothing" and "nobody can say".
	CostOf map[string]prices.Estimate
	// Cost is the window's whole estimate: the segments summed, in order.
	Cost prices.Estimate
}

// Window rolls a window up both ways and prices it at the rates that were in
// force while it was being spent.
//
// `now` is an ISO date, the caller's idea of today (`prices.Today()` in the
// app, a fixed day in a test): it bounds the segment split rather than the
// query, so nothing here reads a clock and every answer is reproducible from
// its arguments.
//
// The segmenting costs one extra roll-up per boundary the window crosses, and
// exactly none when it crosses none — which is every window on an instance
// whose rates have not moved lately, so the common case is the two queries it
// always was.
func (r *Recorder) Window(ctx context.Context, since, now string) (Rollup, error) {
	out := Rollup{ByMode: []Summary{}, ByModel: []Summary{},
		CostOf: map[string]prices.Estimate{}}
	if r == nil || r.db == nil {
		// No app.db is a real state — an instance before its first boot, a
		// laptop that has never signed anybody in. It answers the shape an
		// empty ledger answers rather than an error, because the question
		// ("what has this cost") has an honest answer and it is nothing.
		return out, nil
	}
	var err error
	if out.ByMode, err = r.Summarise(ctx, "mode", since, ""); err != nil {
		return Rollup{}, err
	}
	if out.ByModel, err = r.Summarise(ctx, "model", since, ""); err != nil {
		return Rollup{}, err
	}

	segments := prices.Segments(since, now)
	perModel := map[string][]prices.Estimate{}
	whole := make([]prices.Estimate, 0, len(segments))
	for _, segment := range segments {
		rows := out.ByModel
		if len(segments) > 1 {
			// Only a window that actually straddles a boundary pays for the
			// re-read; one that does not is priced from the roll-up already in
			// hand.
			if rows, err = r.Summarise(ctx, "model", segment.Since, segment.Until); err != nil {
				return Rollup{}, err
			}
		}
		for _, row := range rows {
			one := prices.Over([]prices.Row{{Model: row.Model,
				Conversations: int64(row.Conversations),
				InputTokens:   int64(row.InputTokens),
				OutputTokens:  int64(row.OutputTokens),
				CacheRead:     int64(row.CacheReadTokens)}}, segment.When)
			perModel[row.Model] = append(perModel[row.Model], one)
			whole = append(whole, one)
		}
	}
	for model, parts := range perModel {
		out.CostOf[model] = prices.Sum(parts)
	}
	out.Cost = prices.Sum(whole)
	return out, nil
}

func now() string {
	// The same shape the deck log writes, and for the same reason: the
	// recorded stamp is `2026-08-22T01:23:45.678901+00:00`
	// — microseconds, and an offset rather than a `Z`. The column is text,
	// `since` is compared against it as text, and the panel renders it, so
	// the shape is part of the contract three times over.
	return time.Now().UTC().Format("2006-01-02T15:04:05.000000-07:00")
}

// quoted single-quotes the one refusal this package builds. Not
// wire.Quote: importing the HTTP envelope package into the ledger would put
// a route's vocabulary underneath the accounting, and this needs one quote
// character, not an escaping table.
func quoted(s string) string { return "'" + s + "'" }

// RecorderFrom is a Recorder over an app.db handle somebody else opened.
//
// The door already holds one: `decklog.NewRecorder` opens app.db `mode=rw` for
// ADR 28's activity log, and the two ledgers write different tables in the same
// file. Sharing the handle means one connection pool and one place that decides
// what "no app.db" means, rather than a second `mode=rw` open that could fail
// on its own and leave the process half-accounted.
//
// A nil handle is passed straight through, because `Record` on a Recorder with
// no database already warns and returns -- the honest answer on an instance
// that has no app.db yet.
func RecorderFrom(db *sql.DB, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{db: db, log: logger}
}
