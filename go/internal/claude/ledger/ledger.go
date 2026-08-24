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
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
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
// `since` is an ISO-8601 instant compared against created_at, which is itself
// ISO-8601 UTC — string comparison is date comparison for free. Unlike Record,
// this DOES return its error: it runs when somebody asked a question, and a
// wrong silent answer would be worse than a failure.
//
// `by` is "mode" (which surface spent it) or "model" (which Claude spent it).
// Both axes come back with the same fields, and every row carries BOTH:
// grouping by one aggregates the other, so the field that was not grouped on
// holds Various rather than an arbitrary winner picked by SQLite. A caller
// pricing a per-mode roll-up therefore gets "unpriced" rather than a number
// computed from whichever model happened to sort first — which would be wrong
// and would look right.
func (r *Recorder) Summarise(ctx context.Context, by, since string) ([]Summary, error) {
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
	if since != "" {
		query += " WHERE created_at >= ?"
		args = append(args, since)
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
