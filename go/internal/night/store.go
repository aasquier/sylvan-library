package night

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// State is where a bout stands. The order below is the only road: a bout is
// born planned, is claimed into playing, and settles exactly once — done,
// failed, or skipped. The three settled states are terminal, and the store's
// transitions refuse to leave them.
type State string

// The five states rung 14's `night_bouts.state` column holds.
const (
	StatePlanned State = "planned"
	StatePlaying State = "playing"
	StateDone    State = "done"
	StateFailed  State = "failed"
	StateSkipped State = "skipped"
)

// Run is one night's row: scheduled or sample, when it opened, when it
// closes, and whether the runner has declared it over.
type Run struct {
	ID       int64
	NightKey string
	// Sample marks an admin-triggered measurement run, bounded by its own
	// deadline rather than by the schedule and exempt from the caps.
	Sample   bool
	OpenedAt time.Time
	ClosesAt time.Time
	// FinishedAt is nil while this is the open run — the resume read keys
	// on exactly that.
	FinishedAt *time.Time
}

// Open reports whether the runner still owes this run work.
func (r Run) Open() bool { return r.FinishedAt == nil }

// Seat is one side of a bout: an owner and a slug. A nil Owner is the house —
// a deck off the file tier, which plays every night and has no `user_decks`
// row to point at.
type Seat struct {
	Owner *int64
	Slug  string
}

// House reports whether this seat is the house's.
func (s Seat) House() bool { return s.Owner == nil }

// Bout is one planned pairing and what became of it.
type Bout struct {
	ID    int64
	RunID int64
	SeatA Seat
	SeatB Seat
	Games int
	// Seed is derived and stable per bout, so a night is reproducible in
	// principle.
	Seed  int64
	State State
	// Reason is the skip or failure diagnosis in log-grade words; empty on
	// every other state. Nothing in it ever renders to a player.
	Reason string
	// MatchID is `forge_matches.id` once the ledger recorded the bout, and
	// nil until then — including on a bout orphaned by a restart, where the
	// truth is genuinely unknown and NULL is the honest answer.
	MatchID   *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Plan is one bout as the roster hands it over, before it has a row.
type Plan struct {
	SeatA Seat
	SeatB Seat
	Games int
	Seed  int64
}

// Store holds the night's rows in `app.db`. Reads and writes both return
// their errors: unlike the match ledger, whose Record must never fail the
// match it was handed, this store *is* the runner's memory, and a runner
// that shrugs off a failed write resumes a night that never happened.
//
// The clock is a field so a test — and the runner, which shares one clock
// with it — can hand a fake in; nothing here reads time.Now by name.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// NewStore opens app.db for the night. `mode=rw`, never `rwc`: the ladder
// runs at boot, so a missing file is a broken deployment that must say so
// loudly rather than a silently-minted empty database.
func NewStore(path string, now func() time.Time) (*Store, error) {
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		return nil, fmt.Errorf("opening app.db for the night: %w", err)
	}
	return FromDB(db, now), nil
}

// FromDB wraps a handle somebody else opened — how the served process shares
// one app.db connection across the activity log, the ledgers and this. A nil
// clock means the wall's.
func FromDB(db *sql.DB, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: db, now: now}
}

// Close releases the handle. A nil Store closes nothing.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// StartRun opens tonight's run. One night at a time: while any run is
// unfinished this refuses, which keeps "the open run" a phrase with exactly
// one referent however many tickers, admins and restarts ask at once. The
// schema adds its own wall behind this one — a second *scheduled* run on the
// same night_key is refused by `night_runs_one_per_night` even after the
// first finished.
func (s *Store) StartRun(ctx context.Context, nightKey string, sample bool,
	closesAt time.Time) (Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var openID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM night_runs WHERE finished_at IS NULL LIMIT 1`).Scan(&openID)
	switch {
	case err == nil:
		return Run{}, fmt.Errorf("run %d is still open; one night at a time", openID)
	case !errors.Is(err, sql.ErrNoRows):
		return Run{}, err
	}

	openedAt := s.now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO night_runs (night_key, sample, opened_at, closes_at)`+
			` VALUES (?, ?, ?, ?)`,
		nightKey, boolToInt(sample), stamp(openedAt), stamp(closesAt))
	if err != nil {
		return Run{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return Run{ID: id, NightKey: nightKey, Sample: sample,
		OpenedAt: restamp(openedAt), ClosesAt: restamp(closesAt)}, nil
}

// PlanBouts writes a run's roster, in the order the pairing dealt it — one
// transaction, so a night is planned whole or not at all.
func (s *Store) PlanBouts(ctx context.Context, runID int64, plans []Plan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	at := stamp(s.now())
	for _, p := range plans {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO night_bouts (run_id, seat_a_owner, seat_a_slug,`+
				` seat_b_owner, seat_b_slug, games, seed, state,`+
				` created_at, updated_at)`+
				` VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, nullableID(p.SeatA.Owner), p.SeatA.Slug,
			nullableID(p.SeatB.Owner), p.SeatB.Slug, p.Games, p.Seed,
			string(StatePlanned), at, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OpenRun reads the run the runner still owes work — finished_at unset — or
// reports there is none. StartRun holds there to be at most one.
func (s *Store) OpenRun(ctx context.Context) (Run, bool, error) {
	return s.oneRun(ctx,
		`SELECT id, night_key, sample, opened_at, closes_at, finished_at`+
			` FROM night_runs WHERE finished_at IS NULL ORDER BY id DESC LIMIT 1`)
}

// LatestRun reads the most recent run, open or not — the admin's watching
// read, which after a night ends still has a night to show.
func (s *Store) LatestRun(ctx context.Context) (Run, bool, error) {
	return s.oneRun(ctx,
		`SELECT id, night_key, sample, opened_at, closes_at, finished_at`+
			` FROM night_runs ORDER BY id DESC LIMIT 1`)
}

func (s *Store) oneRun(ctx context.Context, query string) (Run, bool, error) {
	r, err := scanRun(s.db.QueryRowContext(ctx, query))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	return r, true, nil
}

// HasScheduledRun reports whether a scheduled run — open or finished — exists
// for this night_key: the tick's "has tonight already happened?", which must
// stay true after the night finishes or a long window would reopen it.
func (s *Store) HasScheduledRun(ctx context.Context, nightKey string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM night_runs WHERE night_key = ? AND sample = 0 LIMIT 1`,
		nightKey).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Bouts reads a run's bouts in the order they were planned.
func (s *Store) Bouts(ctx context.Context, runID int64) ([]Bout, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, seat_a_owner, seat_a_slug, seat_b_owner,`+
			` seat_b_slug, games, seed, state, reason, match_id,`+
			` created_at, updated_at`+
			` FROM night_bouts WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Bout{}
	for rows.Next() {
		b, err := scanBout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Playing reads a run's bouts currently marked in flight — after a restart,
// with the job registry's memory gone, these are the orphans the first tick
// re-marks failed.
func (s *Store) Playing(ctx context.Context, runID int64) ([]Bout, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, seat_a_owner, seat_a_slug, seat_b_owner,`+
			` seat_b_slug, games, seed, state, reason, match_id,`+
			` created_at, updated_at`+
			` FROM night_bouts WHERE run_id = ? AND state = ? ORDER BY id`,
		runID, string(StatePlaying))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Bout{}
	for rows.Next() {
		b, err := scanBout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ClaimNext moves the run's next planned bout into playing and hands it
// over, one transaction, so two claimants cannot hold the same bout. It
// claims nothing while any bout of the run is already in flight — the lane
// is one wide, and the store keeps that promise even for a caller that
// forgot it. ok is false when there is nothing to claim.
func (s *Store) ClaimNext(ctx context.Context, runID int64) (Bout, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Bout{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var inFlight int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM night_bouts WHERE run_id = ? AND state = ? LIMIT 1`,
		runID, string(StatePlaying)).Scan(&inFlight)
	if err == nil {
		return Bout{}, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Bout{}, false, err
	}

	b, err := scanBout(tx.QueryRowContext(ctx,
		`SELECT id, run_id, seat_a_owner, seat_a_slug, seat_b_owner,`+
			` seat_b_slug, games, seed, state, reason, match_id,`+
			` created_at, updated_at`+
			` FROM night_bouts WHERE run_id = ? AND state = ?`+
			` ORDER BY id LIMIT 1`, runID, string(StatePlanned)))
	if errors.Is(err, sql.ErrNoRows) {
		return Bout{}, false, nil
	}
	if err != nil {
		return Bout{}, false, err
	}

	at := s.now()
	if _, err := tx.ExecContext(ctx,
		`UPDATE night_bouts SET state = ?, updated_at = ? WHERE id = ?`,
		string(StatePlaying), stamp(at), b.ID); err != nil {
		return Bout{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Bout{}, false, err
	}
	b.State = StatePlaying
	b.UpdatedAt = restamp(at)
	return b, true, nil
}

// MarkDone settles a bout that played and recorded: done, carrying the
// `forge_matches` id the shelf will join on. Only a playing bout can be
// done — anything else is a caller confused about whose bout it holds, and
// the error says so instead of rewriting history.
func (s *Store) MarkDone(ctx context.Context, boutID, matchID int64) error {
	return s.settle(ctx, boutID, StateDone, "", &matchID, []State{StatePlaying})
}

// MarkFailed settles a bout that could not produce a result, with the
// diagnosis in log-grade words. Allowed from playing — a job that errored,
// or an orphan a restart left behind — and from planned, for a submit that
// refused on the spot.
func (s *Store) MarkFailed(ctx context.Context, boutID int64, reason string) error {
	return s.settle(ctx, boutID, StateFailed, reason, nil,
		[]State{StatePlaying, StatePlanned})
}

// MarkSkipped settles a bout the night chose not to play — a pre-flight that
// said no, a window that closed first — with the reason.
func (s *Store) MarkSkipped(ctx context.Context, boutID int64, reason string) error {
	return s.settle(ctx, boutID, StateSkipped, reason, nil,
		[]State{StatePlaying, StatePlanned})
}

// settle is the one write that ends a bout: state, reason and match id
// together, guarded by the states the move may leave. Zero rows moved is an
// error — either the bout does not exist or it was already settled, and both
// mean the caller's picture of the night is stale.
func (s *Store) settle(ctx context.Context, boutID int64, to State,
	reason string, matchID *int64, from []State) error {
	// One placeholder per state the move may leave.
	holes := "?"
	for range from[1:] {
		holes += ", ?"
	}
	q := `UPDATE night_bouts SET state = ?, reason = ?, match_id = ?,` +
		` updated_at = ? WHERE id = ? AND state IN (` + holes + `)`
	args := []any{string(to), nullableText(reason), nullableID(matchID),
		stamp(s.now()), boutID}
	for _, f := range from {
		args = append(args, string(f))
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("bout %d is not open to being %s: it is settled or was never planned",
			boutID, to)
	}
	return nil
}

// SkipRemaining settles every still-planned bout of a run at once — the
// window closed, and the reason says so on each row. Returns how many were
// skipped; zero is a night that finished its whole card, not an error.
func (s *Store) SkipRemaining(ctx context.Context, runID int64, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE night_bouts SET state = ?, reason = ?, updated_at = ?`+
			` WHERE run_id = ? AND state = ?`,
		string(StateSkipped), nullableText(reason), stamp(s.now()),
		runID, string(StatePlanned))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FailPlaying settles every bout of a run still marked in flight — the
// restart sweep. The job memory that would have finished them is gone, so
// failed-with-a-reason is the honest state; match_id stays NULL because
// whether the match recorded before the process died is genuinely unknown,
// and the row must not claim to know. Never re-submit these blind.
func (s *Store) FailPlaying(ctx context.Context, runID int64, reason string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE night_bouts SET state = ?, reason = ?, updated_at = ?`+
			` WHERE run_id = ? AND state = ?`,
		string(StateFailed), nullableText(reason), stamp(s.now()),
		runID, string(StatePlaying))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CloseRun pulls the run's close up to now — the admin's "enough". The run
// is not finished by this: a bout in flight still plays to its end (ADR 46
// decision 6), and the next tick finds the window shut and settles the rest.
func (s *Store) CloseRun(ctx context.Context, runID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE night_runs SET closes_at = ? WHERE id = ? AND finished_at IS NULL`,
		stamp(s.now()), runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run %d is not open, so there is nothing to close", runID)
	}
	return nil
}

// FinishRun declares the night over. Only an open run can finish, and
// finishing twice is an error for the same reason settling a bout twice is.
func (s *Store) FinishRun(ctx context.Context, runID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE night_runs SET finished_at = ? WHERE id = ? AND finished_at IS NULL`,
		stamp(s.now()), runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run %d is not open, so there is nothing to finish", runID)
	}
	return nil
}

// scanner is the one Scan shape *sql.Row and *sql.Rows share.
type scanner interface{ Scan(dest ...any) error }

func scanRun(row scanner) (Run, error) {
	var r Run
	var sample int
	var opened, closes string
	var finished sql.NullString
	if err := row.Scan(&r.ID, &r.NightKey, &sample, &opened, &closes,
		&finished); err != nil {
		return Run{}, err
	}
	r.Sample = sample != 0
	var err error
	if r.OpenedAt, err = parseStamp(opened); err != nil {
		return Run{}, err
	}
	if r.ClosesAt, err = parseStamp(closes); err != nil {
		return Run{}, err
	}
	if finished.Valid {
		at, err := parseStamp(finished.String)
		if err != nil {
			return Run{}, err
		}
		r.FinishedAt = &at
	}
	return r, nil
}

func scanBout(row scanner) (Bout, error) {
	var b Bout
	var state string
	var reason sql.NullString
	var created, updated string
	if err := row.Scan(&b.ID, &b.RunID, &b.SeatA.Owner, &b.SeatA.Slug,
		&b.SeatB.Owner, &b.SeatB.Slug, &b.Games, &b.Seed, &state, &reason,
		&b.MatchID, &created, &updated); err != nil {
		return Bout{}, err
	}
	b.State = State(state)
	b.Reason = reason.String
	var err error
	if b.CreatedAt, err = parseStamp(created); err != nil {
		return Bout{}, err
	}
	if b.UpdatedAt, err = parseStamp(updated); err != nil {
		return Bout{}, err
	}
	return b, nil
}

// stamp writes the app's recorded timestamp; parseStamp reads it back. The
// recorded format is UTC with an offset, so RFC3339 covers both the
// fraction-carrying and the elided-fraction spellings.
func stamp(t time.Time) string { return textutil.Isoformat(t) }

func parseStamp(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("night row timestamp %q: %w", s, err)
	}
	return t, nil
}

// restamp is what a stored instant reads back as: the recorded format's own
// precision, so a Run handed straight out of StartRun equals the same Run
// read later.
func restamp(t time.Time) time.Time {
	parsed, err := parseStamp(stamp(t))
	if err != nil {
		// Isoformat's own output always parses; this is unreachable.
		return t
	}
	return parsed
}

func nullableID(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableText(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
