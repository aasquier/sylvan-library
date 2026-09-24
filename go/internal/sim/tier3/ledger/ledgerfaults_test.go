package ledger

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The ledger when the database underneath it is not what it expects.
//
// **A read and a write answer differently here, on purpose, and that is the
// claim this file makes.** [Recorder.Record] never fails its caller: by the
// time it runs, somebody has spent minutes of JVM work and the results are in
// hand, so a ledger that cannot write them costs a warning rather than the
// match. Every *read* does the opposite and returns its error, because an
// empty answer from a ledger that has rows is a sentence — "you have played no
// matches" — and it is false. That is `internal/api/closeddb_test.go`'s
// question asked one layer down: not "did it fail" but **"did it lie"**.
//
// Three faults, each a real shape rather than a contrived one:
//
//   - **A handle that has gone.** The volume unmounted, the file deleted under
//     a running process, `Close` called by a shutdown that outran a job.
//   - **A schema older than the binary.** The ladder is forward-only, so a
//     rolled-back image meets a database with tables it has never heard of —
//     and a table this binary needs can be the one that is missing. Dropping
//     one names exactly which read depends on it.
//   - **A row that will not parse.** `commander` and `themes` are JSON in a
//     text column, which is a promise the column itself cannot keep.

// gone is a real recorder over a real migrated database whose handle has been
// closed underneath it. Every query fails; nothing is nil.
func gone(t *testing.T) *Recorder {
	t.Helper()
	rec, db := scratch(t)
	if err := db.Close(); err != nil {
		t.Fatalf("closing the database: %v", err)
	}
	return rec
}

// wounded is a ledger whose named tables are not there — a database from
// before the rung that added them, which a rolled-back binary really can meet.
func wounded(t *testing.T, drop ...string) *Recorder {
	t.Helper()
	rec, db := scratch(t)
	seedOneMatch(t, rec)
	for _, table := range drop {
		if _, err := db.Exec("DROP TABLE " + table); err != nil {
			t.Fatalf("dropping %s: %v", table, err)
		}
	}
	return rec
}

// seedOneMatch puts one recorded match in the ledger, so a read has something
// to walk rather than stopping at an empty first page.
func seedOneMatch(t *testing.T, rec *Recorder) {
	t.Helper()
	cats, dinos := deckOf(t, catText), deckOf(t, dinoText)
	if id := rec.Record(context.Background(), matchOf(nil,
		[]tier3.GameResult{game(1, 900, intp(1), intp(7), false, false)},
		[]*deck.Deck{cats, dinos})); id == 0 {
		t.Fatal("the fixture match was not recorded")
	}
}

// Every read on a database that has gone says so, rather than answering an
// emptiness that reads as a fact about the player's history.
func TestEveryLedgerReadOnAGoneDatabaseFailsRatherThanReportsEmptiness(t *testing.T) {
	t.Parallel()
	rec := gone(t)
	ctx := context.Background()

	if _, err := rec.Recent(ctx, 5); err == nil {
		t.Error("a ledger with no database answered `no recent matches`")
	}
	if _, err := rec.Board(ctx, Scope{Open: true}); err == nil {
		t.Error("a ledger with no database answered an empty board")
	}
	// The readers underneath, each named so a failure says which one stopped
	// reporting and started guessing.
	if _, _, _, _, err := rec.tally(ctx, 1); err == nil {
		t.Error("the game tally answered over a gone database")
	}
	if _, err := rec.seats(ctx, 1, map[int]int{}); err == nil {
		t.Error("the seat reader answered over a gone database")
	}
	where, args := Scope{Open: true}.visible()
	if _, err := rec.boardSeats(ctx, where, args); err == nil {
		t.Error("the board's seat reader answered over a gone database")
	}
	if _, err := rec.topBlows(ctx, where, args); err == nil {
		t.Error("the killing-blow board answered over a gone database")
	}
	if _, err := rec.topGiants(ctx, where, args); err == nil {
		t.Error("the biggest-creature board answered over a gone database")
	}
	if _, err := rec.topStacks(ctx, where, args); err == nil {
		t.Error("the token-stack board answered over a gone database")
	}
}

// A write on a database that has gone costs a warning and nothing else: the
// caller is holding minutes of JVM work that is perfectly good, and a ledger
// is a record of what happened rather than the thing that happened.
func TestAWriteOnAGoneDatabaseNeverFailsTheMatchThatProducedIt(t *testing.T) {
	t.Parallel()
	rec := gone(t)
	cats, dinos := deckOf(t, catText), deckOf(t, dinoText)
	id := rec.Record(context.Background(), matchOf(nil,
		[]tier3.GameResult{game(1, 900, intp(1), intp(7), false, false)},
		[]*deck.Deck{cats, dinos}))
	if id != 0 {
		t.Errorf("a match recorded nowhere came back with id %d", id)
	}
}

// A table this binary needs and the database does not have is named by the
// read that wanted it, rather than folded into an empty answer.
//
// The two reads are asked separately because they depend on different tables,
// and a fixture that dropped both would prove only that something failed.
func TestAReadNamesTheTableAnOlderSchemaDoesNotHave(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Without the games, a match is a row with no result in it: the history
	// page's per-match tally is what fails.
	noGames := wounded(t, "forge_games")
	if _, err := noGames.Recent(ctx, 5); err == nil {
		t.Error("the history walked a match whose games table is not there")
	} else if !strings.Contains(err.Error(), "forge_games") {
		t.Errorf("the failure does not name the table: %v", err)
	}
	if _, err := noGames.Board(ctx, Scope{Open: true}); err == nil {
		t.Error("the board counted games from a table that is not there")
	}

	// Without the seats, the same two reads fail one step later, in the halves
	// that ask who was at the table.
	noSeats := wounded(t, "forge_seats")
	if _, err := noSeats.Recent(ctx, 5); err == nil {
		t.Error("the history seated a match whose seat table is not there")
	} else if !strings.Contains(err.Error(), "forge_seats") {
		t.Errorf("the failure does not name the table: %v", err)
	}
	if _, err := noSeats.Board(ctx, Scope{Open: true}); err == nil {
		t.Error("the board seated deck records from a table that is not there")
	}
}

// A seat row whose JSON will not parse fails the read rather than arriving as
// a deck with no commander.
//
// `commander` and `themes` are JSON in a text column, which is a promise the
// column cannot keep: anything that ever writes one of those rows by hand, or
// an encoding that changes underneath a stored row, produces exactly this. A
// silently empty commander list would render as a deck nobody is piloting.
func TestASeatWhoseJSONWillNotParseFailsTheReadRatherThanArrivingEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, column := range []string{"commander", "themes"} {
		rec, db := scratch(t)
		seedOneMatch(t, rec)
		if _, err := db.Exec(
			"UPDATE forge_seats SET " + column + " = 'not json at all'"); err != nil {
			t.Fatalf("wounding %s: %v", column, err)
		}
		if _, err := rec.Recent(ctx, 5); err == nil {
			t.Errorf("a seat with an unparseable %s came back as a seat", column)
		}
		if _, err := rec.Board(ctx, Scope{Open: true}); err == nil {
			t.Errorf("the board read a seat with an unparseable %s", column)
		}
	}
}

// A match is one row plus its seats plus its games, and half of one is worse
// than none: a table that refuses a write at any of the three steps leaves
// nothing behind.
//
// The fixture is a `BEFORE INSERT` trigger that aborts, which is the smallest
// thing that says "this statement will not run" to SQLite while leaving every
// read exactly as it was — a volume that filled up, a replica somebody pointed
// the app at, a file gone read-only under a process that already holds it.
func TestAMatchRefusedPartWayThroughLeavesNoHalfOfItself(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	for _, table := range []string{"forge_matches", "forge_seats", "forge_games"} {
		rec, db := scratch(t)
		if _, err := db.Exec("CREATE TRIGGER refuse BEFORE INSERT ON " + table +
			" BEGIN SELECT RAISE(ABORT, 'this database will not take a write');" +
			" END"); err != nil {
			t.Fatalf("arming the %s refusal: %v", table, err)
		}

		cats, dinos := deckOf(t, catText), deckOf(t, dinoText)
		if id := rec.Record(ctx, matchOf(nil,
			[]tier3.GameResult{game(1, 900, intp(1), intp(7), false, false)},
			[]*deck.Deck{cats, dinos})); id != 0 {
			t.Errorf("a match refused at %s came back with id %d", table, id)
		}
		// And the transaction took the rows before it with it, so there is no
		// match with no seats and no seats with no games.
		var matches, seats, games int
		row := db.QueryRow("SELECT (SELECT COUNT(*) FROM forge_matches)," +
			" (SELECT COUNT(*) FROM forge_seats)," +
			" (SELECT COUNT(*) FROM forge_games)")
		if err := row.Scan(&matches, &seats, &games); err != nil {
			t.Fatal(err)
		}
		if matches+seats+games != 0 {
			t.Errorf("a match refused at %s left %d matches, %d seats and %d "+
				"games behind", table, matches, seats, games)
		}
	}
}

// A deck with no commander and no themes is recorded as empty lists rather
// than as nulls, because the column is text holding JSON and the reader on the
// other side unmarshals it: `null` would come back as a nil slice through a
// path that cannot tell it from a decoding that went wrong.
func TestADeckWithNothingInItsListsRecordsThemAsEmptyRatherThanNull(t *testing.T) {
	t.Parallel()
	rec, db := scratch(t)
	bare := &deck.Deck{Slug: "bare", Name: "Bare"}
	id := rec.Record(context.Background(), matchOf(nil,
		[]tier3.GameResult{game(1, 900, intp(1), intp(7), false, false)},
		[]*deck.Deck{bare, deckOf(t, dinoText)}))
	if id == 0 {
		t.Fatal("a deck with empty lists was not recorded")
	}

	var commander, themes string
	if err := db.QueryRow("SELECT commander, themes FROM forge_seats"+
		" WHERE slug = 'bare'").Scan(&commander, &themes); err != nil {
		t.Fatal(err)
	}
	if commander != "[]" || themes != "[]" {
		t.Errorf("the empty lists were stored as %q and %q", commander, themes)
	}
	// And they come back out as a seat rather than as a failed read.
	got, err := rec.Recent(context.Background(), 5)
	if err != nil || len(got) != 1 {
		t.Fatalf("the ledger read back %d matches (%v)", len(got), err)
	}
	if len(got[0].Seats) != 2 {
		t.Errorf("the match came back with %d seats", len(got[0].Seats))
	}
}

// The recorder opens `app.db` for itself when nobody hands it a handle, which
// is the CLI's shape -- one command, one connection, closed on the way out.
func TestTheRecorderOpensAndClosesADatabaseOfItsOwn(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}

	rec, err := NewRecorder(path, quiet())
	if err != nil {
		t.Fatalf("opening a ledger over a real app.db: %v", err)
	}
	seedOneMatch(t, rec)
	if got, err := rec.Recent(context.Background(), 5); err != nil ||
		len(got) != 1 {
		t.Errorf("the ledger read back %d matches (%v)", len(got), err)
	}
	if err := rec.Close(); err != nil {
		t.Errorf("closing the ledger: %v", err)
	}
	// A nil recorder closes nothing, which is the no-ledger case rather than
	// an error: every caller holds one whether or not a database was found.
	var absent *Recorder
	if err := absent.Close(); err != nil {
		t.Errorf("closing a ledger that was never opened: %v", err)
	}

	// **A path with no database on it opens anyway, and this records that
	// rather than approving of it.** `sql.Open` only remembers the DSN, so the
	// absence is discovered by the first statement — which for a *write* is the
	// warning path above and costs nothing, and for a *read* is an error the
	// caller sees. `cache.Open` makes the opposite choice and proves the file
	// at Open; the difference is that nothing reads through this handle on a
	// hot path. Worth knowing if that ever changes.
	absentFile, err := NewRecorder(filepath.Join(t.TempDir(), "nothing.db"), nil)
	if err != nil {
		t.Fatalf("opening a ledger over a missing file: %v", err)
	}
	t.Cleanup(func() { _ = absentFile.Close() })
	if _, err := absentFile.Recent(context.Background(), 5); err == nil {
		t.Error("a ledger over a database that is not there answered " +
			"`no recent matches`")
	}
}

// A handle somebody else opened is wrapped rather than re-opened, and it
// arrives with a logger of its own when the caller names none -- the shape the
// API uses to share one `app.db` across the activity log, the Claude ledger
// and this.
func TestAHandleSomebodyElseOpenedIsWrappedWithADefaultLogger(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	shared := FromDB(db, nil)
	if shared.log == nil {
		t.Fatal("a wrapped handle has no logger, so a failed write would panic")
	}
	seedOneMatch(t, shared)
	// The handle is the caller's: closing the recorder closes what the caller
	// opened, which is why the API holds one recorder rather than one per read.
	if shared.db != db {
		t.Error("the recorder opened a second connection of its own")
	}
}
