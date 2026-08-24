// Package ledger is the match ledger (ADR 36): every
// Forge match recorded as it finishes.
//
// The Simulator's next phase is measurement over accumulated games — rating
// boards with honest uncertainty, a win-probability regression, game-length
// survival curves, and eventually Tier 2's calibration anchor. All of it
// drinks from one dataset, and this package is that dataset's only writer:
// [Recorder.Record] is called from the two places a match finishes (the API
// job, and `mtglab sim forge`), the same single-call-site argument the deck
// activity log made in ADR 28. **The worker machine never records** — its
// rootfs is ephemeral scratch and the app's `app.db` is the only ledger there
// is; results cross the wire first and are recorded where they land.
//
// Three rules, inherited deliberately:
//
//   - **Record never fails the match that produced it.** The match has already
//     been played and paid for; a ledger failure is a logged warning, exactly
//     as in `decklog`, `sim/cache` and `claude/ledger`. Reading returns
//     errors, for the opposite reason: an empty answer for a ledger that has
//     rows would be worse than an error.
//   - **Games are stored as parsed, and readers apply the rules.** A
//     clocked-out game keeps the winner line Forge printed after giving up;
//     [Recent]'s `wins` is the reference reading — a clock-out counts for
//     nobody — and quoting the raw rows without that rule is quoting the
//     measurement's surrender as a trophy.
//   - **Seats snapshot the deck's labels.** Archetype and themes are copied
//     from the deck at match time, never joined from the live deck at read
//     time: the boards group by the class a deck wore when it played, and
//     relabelling a deck must not rewrite its history. Since ADR 37 the
//     archetype copied is a *reading* of the declared themes (worst-piloted
//     declared class word wins) rather than a second declaration; the snapshot
//     rule is unchanged, and rows written before the rule moved mean exactly
//     what they did.
//
// `app.db` is opened `mode=rw` and never `rwc`, like the activity log's and
// the Claude ledger's handles: the ladder runs once at boot (`auth.Migrate`),
// so a missing file is a broken deployment that must say so loudly rather
// than a silently-created empty database this handle then writes into.
package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// DefaultLimit is how many matches a reader gets when it does not say. The
// ledger is read as a recent-history panel first; the analysis passes that
// want everything ask for everything.
const DefaultLimit = 20

// Recorder holds the app.db handle the ledger writes through.
//
// A value rather than a package-level connection so a test can point one at
// a scratch file.
type Recorder struct {
	db  *sql.DB
	log *slog.Logger
}

// NewRecorder opens app.db for writing.
func NewRecorder(path string, logger *slog.Logger) (*Recorder, error) {
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		return nil, fmt.Errorf("opening app.db for the match ledger: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{db: db, log: logger}, nil
}

// FromDB wraps a handle somebody else opened — how the API shares one app.db
// connection across the activity log, the Claude ledger and this.
func FromDB(db *sql.DB, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{db: db, log: logger}
}

// Close releases the handle. A nil Recorder closes nothing, which is the
// no-ledger case rather than an error.
func (r *Recorder) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

// Match is one finished match, as [Recorder.Record] takes it.
type Match struct {
	Run   *tier3.SimRun
	Decks []*deck.Deck
	// Seed is nil for an unseeded CLI run, recorded as NULL: such a run is
	// not reproducible, and the ledger says so rather than inventing a number.
	//
	// A `*big.Int` for the reason the run options carry one — and a seed past
	// SQLite's 64-bit INTEGER is refused: the recorded behaviour is
	// a warning and a match recorded nowhere.
	// [seedForSQL] reproduces that rather than truncating, because a
	// truncated seed is a row claiming a reproducibility it does not have.
	Seed           *big.Int
	Clock          int
	GamesRequested int
	Hosted         bool
	// OwnerIDs matches Decks positionally and defaults to all-NULL, the
	// file-backed curated library, which is what the CLI always records.
	OwnerIDs []*int64
}

// Record writes one finished match. Never fails the caller; returns the match
// id, or 0.
//
// `Decks` arrive in seat order — the order they were handed to Forge, which is
// what `run.Seats` and every `WinnerSeat` already index.
func (r *Recorder) Record(ctx context.Context, m Match) int64 {
	if r == nil || r.db == nil {
		return 0
	}
	id, err := r.record(ctx, m)
	if err != nil {
		// Broader than the activity log's catch, deliberately: that writer is
		// handed strings, while this one is handed a run object — and a
		// malformed one must cost a warning, not the minutes of JVM work the
		// caller is holding the results of.
		r.log.Warn("match ledger record failed", "error", err)
		return 0
	}
	return id
}

func (r *Recorder) record(ctx context.Context, m Match) (int64, error) {
	if m.Run == nil {
		return 0, fmt.Errorf("no run to record")
	}
	if len(m.Decks) == 0 {
		return 0, fmt.Errorf("no decks to record")
	}
	owners := m.OwnerIDs
	if owners == nil {
		owners = make([]*int64, len(m.Decks))
	}
	if len(owners) != len(m.Decks) {
		// A strict pairing: a mismatch is a bug in the caller, and the
		// warning is where it becomes visible.
		return 0, fmt.Errorf("%d owner ids for %d decks", len(owners), len(m.Decks))
	}

	seed, err := seedForSQL(m.Seed)
	if err != nil {
		return 0, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var version any
	if m.Run.ForgeVersion != "" {
		version = m.Run.ForgeVersion
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO forge_matches (created_at, seed, clock,`+
			` games_requested, forge_version, hosted, wall_seconds)`+
			` VALUES (?, ?, ?, ?, ?, ?, ?)`,
		now(), seed, m.Clock, m.GamesRequested, version,
		boolToInt(m.Hosted), m.Run.WallSeconds)
	if err != nil {
		return 0, err
	}
	matchID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for i, d := range m.Decks {
		commander, err := json.Marshal(orEmpty(d.Commander))
		if err != nil {
			return 0, err
		}
		themes, err := json.Marshal(orEmpty(d.Themes))
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO forge_seats (match_id, seat, owner_id, slug,`+
				` commander, archetype, themes)`+
				` VALUES (?, ?, ?, ?, ?, ?, ?)`,
			matchID, i+1, nullableOwner(owners[i]), d.Slug,
			string(commander), d.Archetype(), string(themes)); err != nil {
			return 0, err
		}
	}

	for _, g := range m.Run.Games() {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO forge_games (match_id, game_index,`+
				` winner_seat, milliseconds, turns, draw, timed_out)`+
				` VALUES (?, ?, ?, ?, ?, ?, ?)`,
			matchID, g.Index, nullableSeat(g.WinnerSeat), g.Milliseconds,
			nullableSeat(g.Turns), boolToInt(g.Draw),
			boolToInt(g.TimedOut)); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return matchID, nil
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// seedForSQL narrows a seed for SQLite's 64-bit INTEGER, or refuses.
//
// The recorded behaviour for an oversized seed is a logged warning
// and a match recorded nowhere. Refusing here, before any row is written,
// puts the whole insert on the
// same side of the fence — a match is one row plus its seats plus its games,
// and half of one is worse than none.
func seedForSQL(seed *big.Int) (any, error) {
	if seed == nil {
		return nil, nil
	}
	if !seed.IsInt64() {
		//nolint:staticcheck // capitalised deliberately: the ledger's
		// long-standing warning text, preserved verbatim so old and new log
		// lines read as the same failure.
		return nil, fmt.Errorf("Python int too large to convert to SQLite INTEGER")
	}
	return seed.Int64(), nil
}

func nullableSeat(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// now is the app's recorded timestamp format, and a variable so a test can
// freeze
// it. Through `textutil` rather than a local format string, so the fraction
// is elided at a zero microsecond exactly as the recorded format elides
// it.
var now = func() string { return textutil.Isoformat(time.Now()) }

// Seat is one deck's row in a recorded match, with the labels it wore when it
// played.
type Seat struct {
	Seat      int      `json:"seat"`
	OwnerID   *int64   `json:"owner_id"`
	Slug      string   `json:"slug"`
	Commander []string `json:"commander"`
	Archetype string   `json:"archetype"`
	Themes    []string `json:"themes"`
	Wins      int      `json:"wins"`
}

// RecordedMatch is one row of [Recent], in the recorded wire key order.
type RecordedMatch struct {
	ID             int64    `json:"id"`
	CreatedAt      string   `json:"created_at"`
	Seed           *int64   `json:"seed"`
	Clock          int      `json:"clock"`
	GamesRequested int      `json:"games_requested"`
	ForgeVersion   *string  `json:"forge_version"`
	Hosted         bool     `json:"hosted"`
	WallSeconds    *float64 `json:"wall_seconds"`
	Played         int      `json:"played"`
	Draws          int      `json:"draws"`
	TimedOut       int      `json:"timed_out"`
	Seats          []Seat   `json:"seats"`
}

// Recent is the latest matches, newest first, each with its seats and tallies.
//
// The reference reading of the raw rows: `Wins` here applies the clock-out
// rule — a game that hit the clock counts for nobody, even when Forge printed
// a winner line after giving up — so a caller rendering this never has to know
// the rule exists. `Draws` counts real draws only; `TimedOut` is reported
// apart, because a clock-out is the measurement giving up and CLAUDE.md
// requires the two never be folded together.
//
// Unlike [Recorder.Record], this returns its errors: an empty answer for a
// ledger that has rows would be worse than a failure.
func (r *Recorder) Recent(ctx context.Context, limit int) ([]RecordedMatch, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, created_at, seed, clock, games_requested,`+
			` forge_version, hosted, wall_seconds`+
			` FROM forge_matches ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RecordedMatch{}
	for rows.Next() {
		var m RecordedMatch
		var hosted int
		if err := rows.Scan(&m.ID, &m.CreatedAt, &m.Seed, &m.Clock,
			&m.GamesRequested, &m.ForgeVersion, &hosted, &m.WallSeconds); err != nil {
			return nil, err
		}
		m.Hosted = hosted != 0
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		wins, played, draws, timedOut, err := r.tally(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Played, out[i].Draws, out[i].TimedOut = played, draws, timedOut
		seats, err := r.seats(ctx, out[i].ID, wins)
		if err != nil {
			return nil, err
		}
		out[i].Seats = seats
	}
	return out, nil
}

func (r *Recorder) tally(ctx context.Context, matchID int64) (
	wins map[int]int, played, draws, timedOut int, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT game_index, winner_seat, milliseconds, turns, draw,`+
			` timed_out FROM forge_games WHERE match_id = ?`+
			` ORDER BY game_index`, matchID)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()

	wins = map[int]int{}
	for rows.Next() {
		var index, ms int
		var seat, turns *int
		var draw, out int
		if err := rows.Scan(&index, &seat, &ms, &turns, &draw, &out); err != nil {
			return nil, 0, 0, 0, err
		}
		played++
		if seat != nil && out == 0 {
			wins[*seat]++
		}
		if draw != 0 && out == 0 {
			draws++
		}
		if out != 0 {
			timedOut++
		}
	}
	return wins, played, draws, timedOut, rows.Err()
}

func (r *Recorder) seats(ctx context.Context, matchID int64,
	wins map[int]int) ([]Seat, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT seat, owner_id, slug, commander, archetype,`+
			` themes FROM forge_seats WHERE match_id = ? ORDER BY seat`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seats := []Seat{}
	for rows.Next() {
		var s Seat
		var commander, themes string
		if err := rows.Scan(&s.Seat, &s.OwnerID, &s.Slug, &commander,
			&s.Archetype, &themes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(commander), &s.Commander); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(themes), &s.Themes); err != nil {
			return nil, err
		}
		s.Wins = wins[s.Seat]
		seats = append(seats, s)
	}
	return seats, rows.Err()
}

func nullableOwner(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
