package ledger

import (
	"database/sql"
	"io"
	"log/slog"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// scratch builds an app.db from the schema Python's ladder produces, which is
// the point of `authtest`: four packages had each transcribed the ladder by
// hand and frozen it at a different rung, and two broke the day a new column
// was first read.
func scratch(t *testing.T) (*Recorder, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return FromDB(db, quiet()), db
}

func deckOf(t *testing.T, text string) *deck.Deck {
	t.Helper()
	d, err := deck.FromText(text, "")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const catText = `slug: cats
name: Arahbo Cats
commander:
  - Arahbo, Roar of the World
themes:
  - cats
  - aggro
cards:
  - name: Sol Ring
    category: ramp
    why: fast
`

const dinoText = `slug: dinos
name: Atla Dinos
commander:
  - Atla Palani, Nest Tender
themes:
  - dinosaurs
  - midrange
  - combo
cards:
  - name: Sol Ring
    category: ramp
    why: fast
`

func matchOf(seed *big.Int, games []tier3.GameResult, decks []*deck.Deck) Match {
	version := "2.0.14"
	return Match{
		Run: &tier3.SimRun{Output: tier3.SimOutput{Games: games},
			WallSeconds: 61.5, Seats: map[int]string{1: "cats", 2: "dinos"},
			ForgeVersion: version},
		Decks: decks, Seed: seed, Clock: 300, GamesRequested: len(games),
		Hosted: true,
	}
}

func game(index, ms int, seat *int, turns *int, draw, timedOut bool) tier3.GameResult {
	return tier3.GameResult{Index: index, Milliseconds: ms, WinnerSeat: seat,
		Turns: turns, Draw: draw, TimedOut: timedOut}
}

func intp(n int) *int { return &n }

// TestAMatchIsRecordedAndReadBack is the round trip, and the reading rules
// with it.
func TestAMatchIsRecordedAndReadBack(t *testing.T) {
	rec, _ := scratch(t)
	decks := []*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)}

	id := rec.Record(t.Context(), matchOf(big.NewInt(7), []tier3.GameResult{
		game(1, 5421, intp(1), intp(11), false, false),
		game(2, 4000, intp(2), intp(9), false, false),
		game(3, 900, nil, intp(4), true, false),
		// **A clock-out that Forge gave a winner line.** It counts for
		// nobody, and it is reported apart from the real draw above.
		game(4, 300000, intp(1), nil, true, true),
	}, decks))
	if id == 0 {
		t.Fatal("the match was not recorded")
	}

	matches, err := rec.Recent(t.Context(), DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("read back %d matches, want 1", len(matches))
	}
	m := matches[0]
	if m.ID != id {
		t.Errorf("id %d, want %d", m.ID, id)
	}
	if m.Played != 4 {
		t.Errorf("played %d, want 4", m.Played)
	}
	// A real draw and a clock-out are separate columns, because they are
	// separate facts: one is a game outcome, the other is the measurement
	// giving up. CLAUDE.md requires they never be folded together.
	if m.Draws != 1 {
		t.Errorf("draws %d, want 1", m.Draws)
	}
	if m.TimedOut != 1 {
		t.Errorf("timed out %d, want 1", m.TimedOut)
	}
	if m.Seats[0].Wins != 1 {
		t.Errorf("seat 1 has %d wins; the clock-out must count for nobody", m.Seats[0].Wins)
	}
	if m.Seats[1].Wins != 1 {
		t.Errorf("seat 2 has %d wins, want 1", m.Seats[1].Wins)
	}
	if m.Seed == nil || *m.Seed != 7 {
		t.Errorf("seed %v, want 7", m.Seed)
	}
	if m.ForgeVersion == nil || *m.ForgeVersion != "2.0.14" {
		t.Errorf("forge version %v", m.ForgeVersion)
	}
	if !m.Hosted {
		t.Error("a hosted match was recorded as local")
	}
	if m.WallSeconds == nil || *m.WallSeconds != 61.5 {
		t.Errorf("wall seconds %v", m.WallSeconds)
	}
	if m.CreatedAt == "" {
		t.Error("the match has no timestamp")
	}
}

// TestSeatsSnapshotTheLabels is ADR 36's third rule: the boards group by the
// class a deck wore when it played, so relabelling must not rewrite history.
func TestSeatsSnapshotTheLabels(t *testing.T) {
	rec, _ := scratch(t)
	cats, dinos := deckOf(t, catText), deckOf(t, dinoText)
	rec.Record(t.Context(), matchOf(big.NewInt(1),
		[]tier3.GameResult{game(1, 100, intp(1), nil, false, false)},
		[]*deck.Deck{cats, dinos}))

	// The deck is relabelled after the match.
	cats.Themes = []string{"cats", "control"}

	matches, err := rec.Recent(t.Context(), DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	seat := matches[0].Seats[0]
	if len(seat.Themes) != 2 || seat.Themes[0] != "cats" || seat.Themes[1] != "aggro" {
		t.Errorf("the seat's themes followed the live deck: %v", seat.Themes)
	}
	// And the archetype is a READING of the declared themes at match time
	// (ADR 37, worst-piloted-declared-class wins), not a second declaration.
	if seat.Archetype != "aggro" {
		t.Errorf("archetype %q, want aggro", seat.Archetype)
	}
	// The other seat declares midrange AND combo, and combo is the harder to
	// pilot, so the board reads it as combo and it wears combo's caveat.
	if got := matches[0].Seats[1].Archetype; got != "combo" {
		t.Errorf("a midrange-and-combo deck was read as %q, want combo", got)
	}
	if len(seat.Commander) != 1 || seat.Commander[0] != "Arahbo, Roar of the World" {
		t.Errorf("the commander did not survive: %v", seat.Commander)
	}
}

// TestRecordNeverFailsTheMatchThatProducedIt is the rule the whole package
// hangs on: the JVM minutes have already been spent, so a ledger problem costs
// a warning and never a match somebody watched finish.
func TestRecordNeverFailsTheMatchThatProducedIt(t *testing.T) {
	rec, db := scratch(t)
	decks := []*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)}
	good := []tier3.GameResult{game(1, 100, intp(1), nil, false, false)}

	for _, c := range []struct {
		note  string
		match Match
	}{
		{"no run at all", Match{Decks: decks, Clock: 300}},
		{"no decks", Match{Run: &tier3.SimRun{}, Clock: 300}},
		// **Fewer owner ids than decks**, and the ids are nil rather than
		// real: an id naming an account that does not exist would fail on
		// the foreign key and pass this test for the wrong reason, which is
		// exactly what it did until the mutation run asked. `zip(...,
		// strict=True)` is the guard, and without it seat two indexes past
		// the end.
		{"fewer owner ids than decks", func() Match {
			m := matchOf(nil, good, decks)
			m.OwnerIDs = []*int64{nil}
			return m
		}()},
		// **A failure INSIDE the transaction**, which every case above
		// happens before: two games sharing an index violate
		// `PRIMARY KEY (match_id, game_index)` on the second insert, after
		// the match row and both seats are already written. Without the
		// rollback they stay written, and the ledger grows a match with no
		// games that every board would count.
		{"a duplicate game index, after the match row is in", func() Match {
			return matchOf(big.NewInt(2), []tier3.GameResult{
				game(1, 100, intp(1), nil, false, false),
				game(1, 200, intp(2), nil, false, false),
			}, decks)
		}()},
		// **A seed past SQLite's 64-bit INTEGER**, which is where Python
		// raises an OverflowError its broad `except` turns into a warning.
		{"a seed too large for the column", func() Match {
			seed, _ := new(big.Int).SetString("1180591620717411303424", 10)
			return matchOf(seed, good, decks)
		}()},
	} {
		t.Run(c.note, func(t *testing.T) {
			if id := rec.Record(t.Context(), c.match); id != 0 {
				t.Errorf("a bad match reported id %d", id)
			}
		})
	}

	// And nothing half-written survived: a match is one row plus its seats
	// plus its games, and half of one is worse than none.
	var rows int
	if err := db.QueryRow("SELECT count(*) FROM forge_matches").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d partial matches were left behind", rows)
	}
	for _, table := range []string{"forge_seats", "forge_games"} {
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Errorf("%s holds %d orphan rows", table, rows)
		}
	}

	// The recorder still works afterwards, which is the point of never
	// raising: one bad match must not poison the ledger.
	if id := rec.Record(t.Context(), matchOf(big.NewInt(3), good, decks)); id == 0 {
		t.Error("the recorder stopped working after a refused match")
	}
}

// TestANilRecorderRecordsNothingAndSaysNothing is the no-app.db case: an
// instance with no database records no matches and still plays them.
func TestANilRecorderRecordsNothingAndSaysNothing(t *testing.T) {
	var rec *Recorder
	if id := rec.Record(t.Context(), Match{}); id != 0 {
		t.Errorf("a nil recorder reported id %d", id)
	}
	if err := rec.Close(); err != nil {
		t.Errorf("closing a nil recorder: %v", err)
	}
	empty := FromDB(nil, quiet())
	if id := empty.Record(t.Context(), Match{}); id != 0 {
		t.Errorf("a recorder with no database reported id %d", id)
	}
}

// TestAnUnseededRunIsRecordedAsNull: an unseeded CLI run is not reproducible,
// and the ledger says so rather than inventing a number.
func TestAnUnseededRunIsRecordedAsNull(t *testing.T) {
	rec, _ := scratch(t)
	decks := []*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)}
	rec.Record(t.Context(), matchOf(nil,
		[]tier3.GameResult{game(1, 100, intp(1), nil, false, false)}, decks))

	matches, err := rec.Recent(t.Context(), DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	if matches[0].Seed != nil {
		t.Errorf("an unseeded run recorded seed %v", *matches[0].Seed)
	}
	// And the owner ids default to the file tier, which is what the CLI
	// always records — it only ever loads file decks.
	for _, seat := range matches[0].Seats {
		if seat.OwnerID != nil {
			t.Errorf("seat %d recorded owner %v", seat.Seat, *seat.OwnerID)
		}
	}
}

// TestNewestFirstAndLimited is what a recent-history panel asks for.
func TestNewestFirstAndLimited(t *testing.T) {
	rec, _ := scratch(t)
	decks := []*deck.Deck{deckOf(t, catText), deckOf(t, dinoText)}
	var ids []int64
	for i := 1; i <= 5; i++ {
		ids = append(ids, rec.Record(t.Context(), matchOf(big.NewInt(int64(i)),
			[]tier3.GameResult{game(1, 100, intp(1), nil, false, false)}, decks)))
	}
	matches, err := rec.Recent(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 3 {
		t.Fatalf("asked for 3, got %d", len(matches))
	}
	for i, want := range []int64{ids[4], ids[3], ids[2]} {
		if matches[i].ID != want {
			t.Errorf("row %d is match %d, want %d", i, matches[i].ID, want)
		}
	}
	// A limit of zero or less is the default rather than an empty answer:
	// nobody asking for "recent matches" means none.
	all, err := rec.Recent(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Errorf("an unasked limit returned %d matches, want 5", len(all))
	}
}
