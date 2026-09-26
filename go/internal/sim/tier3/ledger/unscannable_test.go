package ledger

import (
	"context"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// A row the ledger wrote and can no longer read.
//
// `ledgerfaults_test.go` covers the two faults that stop a read at the door: a
// handle that has gone, and a table that is not there. Both fail the *query*.
// This file is the fault that gets past the query — a row comes back and the
// value in it is not the kind of value the column promised — and it is a
// different sentence in the code: the `rows.Scan` arm inside the loop rather
// than the `err != nil` arm above it, and `rows.Err()` rather than either.
//
// It is not contrived. SQLite applies column affinity rather than enforcing
// it, so a text that will not parse as a number **stays text** in an INTEGER
// column: one hand-run `UPDATE` during an incident, one half-finished restore,
// one row written by a version that spelled a column differently, and the
// ledger holds a row it cannot read back. The file this file sits beside
// already says the same thing about the two JSON columns ("a promise the
// column itself cannot keep"); these are the numeric ones.
//
// What is asserted is never the driver's wording — it is that the read
// **fails** rather than skipping the row or reporting a shorter history than
// the ledger holds. A board that silently dropped the row it could not scan
// would show a player a leaderboard missing the very game they came to look
// for, with nothing anywhere saying so.

// poisoned records one full match — two seats, one game, with a killer, a
// giant and a token pile so every feat board has a row — and then writes a
// value into `table.column` that no numeric scan can take.
//
// The returned recorder is healthy in every other respect: the query runs, the
// row comes back, and the failure lands where this file is about.
func poisoned(t *testing.T, table, column string) *Recorder {
	t.Helper()
	rec, db := scratch(t)
	seedOneFeatfulMatch(t, rec)
	// One row, not all of them: `forge_seats` has a UNIQUE on (match_id,
	// seat), so poisoning every row of that table would be refused by the
	// schema before the read ever ran.
	one := " WHERE rowid = (SELECT MIN(rowid) FROM " + table + ")"
	if _, err := db.Exec(
		"UPDATE " + table + " SET " + column + " = 'not a number'" + one); err != nil {
		t.Fatalf("poisoning %s.%s: %v", table, column, err)
	}
	// The affinity check, made rather than assumed: if a future SQLite coerced
	// this to 0 the tests below would pass while proving nothing.
	var kind string
	if err := db.QueryRow(
		"SELECT typeof(" + column + ") FROM " + table + one).Scan(&kind); err != nil {
		t.Fatalf("reading the type of %s.%s: %v", table, column, err)
	}
	if kind != "text" {
		t.Fatalf("%s.%s holds %q after the UPDATE, not text -- this fixture "+
			"no longer poisons anything", table, column, kind)
	}
	return rec
}

// seedOneFeatfulMatch is one recorded match with every feat column filled, so
// all three leaderboards have a row to read and the board walks its whole
// length.
func seedOneFeatfulMatch(t *testing.T, rec *Recorder) {
	t.Helper()
	g := game(1, 4200, intp(1), intp(9), false, false)
	g.Killer = &tier3.KillingBlow{Amount: 16, Card: "Syr Gwyn, Hero of Ashvale",
		Sources: 1, Combat: true, Seat: 2, Turn: 9}
	g.Biggest = &tier3.BigCreature{Card: "Sol Ring", Power: 15, Toughness: 15,
		Seat: 1, Turn: 8}
	g.TallestStack = &tier3.TokenStack{Card: "Sol Ring", Count: 11, Seat: 1, Turn: 8}
	if id := rec.Record(context.Background(), matchOf(big.NewInt(3),
		[]tier3.GameResult{g},
		[]*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)})); id == 0 {
		t.Fatal("the fixture match was not recorded")
	}
}

// The anti-vacuity half: the fixture without the poison reads back whole, so
// every failure below is the UPDATE's doing and not the fixture's.
func TestTheFeatfulFixtureReadsBackWholeBeforeAnythingIsPoisoned(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	seedOneFeatfulMatch(t, rec)

	recent, err := rec.Recent(t.Context(), 5)
	if err != nil {
		t.Fatalf("reading the fixture back: %v", err)
	}
	if len(recent) != 1 || len(recent[0].Seats) != 2 {
		t.Fatalf("the fixture is not one match of two seats: %#v", recent)
	}
	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatalf("building the board: %v", err)
	}
	if len(board.Blows) != 1 || len(board.Giants) != 1 || len(board.Stacks) != 1 {
		t.Fatalf("the fixture does not fill all three feat boards: "+
			"%d blows, %d giants, %d stacks",
			len(board.Blows), len(board.Giants), len(board.Stacks))
	}
}

// `wall_seconds` is a REAL the match row carries. A text in it stops the match
// list itself, which is the read a player's history page is made of.
func TestAMatchRowThatWillNotScanFailsTheHistoryRatherThanShorteningIt(t *testing.T) {
	t.Parallel()
	rec := poisoned(t, "forge_matches", "wall_seconds")

	out, err := rec.Recent(t.Context(), 5)
	if err == nil {
		t.Fatalf("a match row that will not scan was read as %d matches -- "+
			"a history with a row missing and nothing saying so", len(out))
	}
	if out != nil {
		t.Errorf("the failed read handed back %d matches beside its error", len(out))
	}
}

// `milliseconds` is the game row's own number, read by the tally rather than
// by the match query, so this names the second read in `Recent`'s walk.
func TestAGameRowThatWillNotScanFailsTheTally(t *testing.T) {
	t.Parallel()
	rec := poisoned(t, "forge_games", "milliseconds")

	if _, err := rec.Recent(t.Context(), 5); err == nil {
		t.Error("a game row that will not scan was tallied as if it were not there")
	}
	// The tally under it, named so a failure says which of the three reads in
	// `Recent` stopped answering.
	if _, _, _, _, err := rec.tally(t.Context(), 1); err == nil {
		t.Error("the tally read a game row it cannot scan")
	}
}

// `seat` is the seat number, and the third read in `Recent`'s walk — and the
// same column the board's own seat read scans, so one poison names two
// readers.
func TestASeatRowThatWillNotScanFailsBothTheMatchWalkAndTheBoard(t *testing.T) {
	t.Parallel()
	rec := poisoned(t, "forge_seats", "seat")

	if _, err := rec.Recent(t.Context(), 5); err == nil {
		t.Error("a seat row that will not scan was read as a match with no seats")
	}
	if _, err := rec.seats(t.Context(), 1, map[int]int{}); err == nil {
		t.Error("the seat read scanned a seat that is not a number")
	}
	if board, err := rec.Board(t.Context(), open); err == nil {
		t.Errorf("the board was built over a seat row it cannot scan: "+
			"%d decks", len(board.Decks))
	}
}

// Each feat board reads its own ranking column, and each is the only reader of
// it — so a poisoned column names exactly one leaderboard, and the board's own
// three call sites are three separate sentences in `Board`.
func TestAFeatRowThatWillNotScanFailsItsOwnBoardAndTheWholeBoardWithIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		column string
		read   func(*Recorder, context.Context) error
	}{
		{"kill_amount", func(r *Recorder, ctx context.Context) error {
			_, err := r.topBlows(ctx, "1 = 1", nil)
			return err
		}},
		{"big_power", func(r *Recorder, ctx context.Context) error {
			_, err := r.topGiants(ctx, "1 = 1", nil)
			return err
		}},
		{"stack_count", func(r *Recorder, ctx context.Context) error {
			_, err := r.topStacks(ctx, "1 = 1", nil)
			return err
		}},
	} {
		t.Run(tc.column, func(t *testing.T) {
			t.Parallel()
			rec := poisoned(t, "forge_games", tc.column)

			if err := tc.read(rec, t.Context()); err == nil {
				t.Errorf("the %s board read a row it cannot scan", tc.column)
			}
			if _, err := rec.Board(t.Context(), open); err == nil {
				t.Errorf("the board was built with the %s leaderboard "+
					"silently one row short", tc.column)
			}
		})
	}
}

// faulty is a recorder over a real migrated `app.db` reached through a handle
// that will stop answering after a set number of statements.
func faulty(t *testing.T) (*Recorder, *authtest.Fault) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return FromDB(db, quiet()), fault
}

// The commit is the statement whose failure decides whether a match was
// recorded or not, and it is the last one: every row of the match is already
// written by the time it runs.
//
// The rule this holds is [Recorder.Record]'s own — a ledger that cannot write
// costs a warning rather than the match — with the half that matters beside
// it: the caller is told **nothing was recorded** (id zero) rather than handed
// an id for a match the database rolled back. An id for a row that is not
// there is worse than no id: it is a receipt.
func TestAMatchWhoseCommitFailsIsRecordedNowhereAndReturnsNoID(t *testing.T) {
	t.Parallel()
	rec, fault := faulty(t)
	decks := []*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)}
	m := matchOf(big.NewInt(5), []tier3.GameResult{
		game(1, 1000, intp(1), intp(8), false, false)}, decks)

	// The budget is counted rather than guessed: one BEGIN, one match insert,
	// one insert per seat, one per game — and then the commit, which is the
	// statement this test is about.
	statements := 1 + 1 + len(decks) + len(m.Run.Games())
	fault.After(statements)

	if id := rec.Record(t.Context(), m); id != 0 {
		t.Errorf("a match whose commit failed came back with id %d -- "+
			"a receipt for a row that was rolled back", id)
	}

	fault.Heal()
	out, err := rec.Recent(t.Context(), 5)
	if err != nil {
		t.Fatalf("reading the healed ledger: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("the rolled-back match is in the ledger: %#v", out)
	}
}

// A read cut short partway down its result set is the one fault a healthy
// database and a gone handle both hide: the query succeeded, some rows came
// back, and the iteration failed rather than ended.
//
// `rows.Err()` is the only thing between that and a page of history with rows
// missing — which is the shape of wrong answer this whole module refuses,
// because a short list reads as a complete one.
func TestAHistoryCutShortMidIterationFailsRatherThanAnsweringPartOfIt(t *testing.T) {
	t.Parallel()
	rec, fault := faulty(t)
	decks := []*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)}
	for i := 1; i <= 3; i++ {
		if id := rec.Record(t.Context(), matchOf(big.NewInt(int64(i)),
			[]tier3.GameResult{game(1, 1000, intp(1), intp(8), false, false)},
			decks)); id == 0 {
			t.Fatalf("fixture match %d was not recorded", i)
		}
	}

	// One row is handed over and the next read of the set fails, so the walk
	// has a partial answer in hand at the moment it has to decide what to do
	// with it.
	fault.RowsAfter(1)
	out, err := rec.Recent(t.Context(), 10)
	if err == nil {
		t.Fatalf("a history cut short mid-iteration came back as %d matches "+
			"of the three recorded", len(out))
	}
	if out != nil {
		t.Errorf("the failed read handed back %d matches beside its error", len(out))
	}
}
