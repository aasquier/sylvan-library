package api

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
)

// Faults that land **after** the card pool has already answered once.
//
// The schema-less pool (`failingpool_test.go`) fails the first query a route
// makes, which is the right fixture for "is this route honest about a pool it
// cannot read at all". What it cannot do is land the failure on a *named* read:
// every count fails, so a route that reads two tables in order proves only that
// it stopped at the first one. Lever 16 is the finer instrument — a real pool
// with one table taken away — and these are the branches that need it.

// quiet is a logger nothing reads, for an API built outside a rig.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// halfAPool is the tiny pool with `printings` removed: every card still
// resolves and nothing about a printing does. A refresh interrupted after the
// oracle pass is this file, and so is a restore that replayed one table.
func halfAPool(t *testing.T) *pool.Pool {
	t.Helper()
	path := pooltest.Build(t)
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), "DROP TABLE printings"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	return p
}

// **Health never calls a half-read library well.** The route counts two tables
// and then asks whether the pool is stale; a pool whose printings have gone
// answers the first count and fails the second, and the whole point of the
// route is that whoever is watching from outside can tell the difference
// between "no pool yet" (a 200 in the degraded shape) and "this pool is
// broken".
func TestHealthRefusesAPoolThatAnswersItsCardsAndNotItsPrintings(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(), Pool: halfAPool(t),
		DecksDir: decksDir(t)})
	status, payload, raw := call(t, a, "GET", "/api/health", "")
	if status == http.StatusOK {
		t.Fatalf("a pool missing half its tables was called healthy: %s", raw)
	}
	// And not as the degraded "there is no pool here" answer, which would send
	// somebody to run a refresh they have already run.
	if payload["pool"] == false {
		t.Errorf("a broken pool was reported as an absent one: %s", raw)
	}
	if detail, _ := payload["detail"].(string); detail == "" {
		t.Errorf("health answered %d with nothing to read: %s", status, raw)
	}
}

// The other half of the same route: the pool is fine and the **library** will
// not open. Health counts the decks for every caller, so a volume whose decks
// directory the process cannot read has to refuse rather than report a library
// of nought — the number a watcher would read as "the decks are gone".
func TestHealthRefusesWhenTheLibraryWillNotOpen(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable directory to build")
	}
	decks := filepath.Join(t.TempDir(), "decks")
	if err := os.MkdirAll(filepath.Join(decks, "gyome"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decks, "gyome", "deck.yaml"),
		[]byte("slug: gyome\nname: Gyome\ncards: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(decks, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(decks, 0o750) })

	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t), DecksDir: decks})
	status, payload, raw := call(t, a, "GET", "/api/health", "")
	if status == http.StatusOK {
		t.Fatalf("health answered 200 over a library it cannot open: %s", raw)
	}
	if got, ok := payload["decks"]; ok {
		t.Errorf("health counted %v decks in a directory it cannot read", got)
	}
}

// ---- the feat paintings ----------------------------------------------------

// **A board nobody has played is no paintings and no fuss.** The three feat
// boards are empty on a fresh instance, and the room draws them empty.
func TestTheFeatPaintingsOfNoBoardAreNone(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t)})
	if got := a.featArt(context.Background(), nil); len(got) != 0 {
		t.Errorf("a nil board was painted %v", got)
	}
}

// **A pool that cannot answer costs the pictures and never the record.**
// `featArt` swallows its pool error on purpose: refusing somebody their own
// bouts because the card pool has not been refreshed would be the worse answer
// by a mile. So the fault has to end as an empty map rather than as anything a
// caller branches on.
func TestFeatPaintingsSurviveAPoolThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(), Pool: schemalessPool(t)})
	got := a.featArt(context.Background(), &ledger.Standings{
		Blows:  []ledger.KillRecord{{Card: "Craterhoof Behemoth"}},
		Giants: []ledger.GiantRecord{{Card: "Primeval Titan"}},
		Stacks: []ledger.StackRecord{{Card: "Food Token"}},
	})
	if len(got) != 0 {
		t.Errorf("a pool that answered nothing produced paintings: %v", got)
	}
}

// And the standings route itself, over a ledger whose handle has gone: a 200
// carrying an empty board would say "nothing has ever been played here" to
// somebody whose records are sitting on a volume that stopped answering.
func TestTheStandingsRefuseALedgerThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	path := appDB(t)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Logger: quietLogger(),
		MatchLedger: ledger.FromDB(db, quietLogger())})
	status, payload, raw := callAs(t, a, alice, "GET", "/api/coliseum/standings", "")
	if status == http.StatusOK {
		t.Fatalf("the standings answered 200 over a closed ledger: %s", raw)
	}
	if detail, _ := payload["detail"].(string); detail == "" {
		t.Errorf("the standings answered %d with nothing to read: %s", status, raw)
	}
}
