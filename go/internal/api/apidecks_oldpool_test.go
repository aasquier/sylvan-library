package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// A card pool one column older than the binary reading it.
//
// ADR 23 makes merging deploying, and the pool cannot migrate itself: between a
// release that reads a new column and the next refresh, the file on the volume is
// exactly this -- every card still there, and one query that will not bind. It is
// the schema-less pool's question asked at the other end of the deploy window,
// and it separates two reads that a wholly broken pool cannot: a card looked up
// *by name*, which reads the pool's own column list and adapts, from a card
// looked up *by resemblance*, which names its columns in the query.
//
// What that reaches is the misspelling tier. A paste with a name nobody holds is
// scored against every card in the pool, and the ranking tie-break is a column
// -- so on a pool a column short, the correction pass fails where the lookup
// before it succeeded. Both routes that read a pasted list are asked, because
// each one calls the pass from inside its own pool lease and a swallowed failure
// there would mean a deck written from names nobody checked.

// oldPool is the tiny pool with `edhrec_rank` removed: `GetCards` still answers
// every card, and anything ranking by resemblance does not.
func oldPool(t *testing.T) *pool.Pool {
	t.Helper()
	path := pooltest.Build(t)
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuilt rather than altered: the schema's own index refuses a dropped
	// column, which is the recipe `docs/polish/COVERAGE.md` records.
	for _, stmt := range []string{
		`CREATE TABLE oracle_rebuilt AS SELECT * EXCLUDE (edhrec_rank) FROM oracle_cards`,
		`DROP TABLE oracle_cards`,
		`ALTER TABLE oracle_rebuilt RENAME TO oracle_cards`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	return p
}

// oldPoolRig is a writable library over a pool a column short.
type oldPoolRig struct {
	api   *API
	decks string
	close func()
}

func newOldPoolRig(t *testing.T) *oldPoolRig {
	t.Helper()
	decks := decksDir(t)
	dbPath := appDB(t)
	db := auth.Open(dbPath)
	recorder, err := decklog.NewRecorder(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{Pool: oldPool(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil)})
	return &oldPoolRig{api: a, decks: decks,
		close: func() { recorder.Close(); _ = db.Close() }}
}

// The premise first: the pool still answers a deck's cards by name, so the gate
// reaches a verdict. Without this the two refusals below would prove nothing --
// they would be the schema-less pool's test over again, aimed at a route that
// never got near the ranking.
func TestAPoolAColumnShortStillLooksEveryCardUpByName(t *testing.T) {
	t.Parallel()
	rig := newOldPoolRig(t)
	defer rig.close()

	status, payload, raw := as(t, rig.api, alice, cleanDeck+"/validate")
	if status != http.StatusOK {
		t.Fatalf("the gate answered %d over a pool a column short: %s", status, raw)
	}
	if _, present := payload["ok"]; !present {
		t.Fatalf("the verdict carries no answer: %s", raw)
	}
	// A card the pool could not answer for would read as `unknown-card` on every
	// entry, so a clean verdict standing at all is the premise this file needs.
	if payload["ok"] != true {
		t.Errorf("the clean fixture failed the gate over a readable pool: %s", raw)
	}
}

// Both routes that read a pasted list refuse rather than importing names the
// correction pass never got to look at.
func TestAPastedListIsRefusedWhenTheMisspellingTierCannotRank(t *testing.T) {
	t.Parallel()
	rig := newOldPoolRig(t)
	defer rig.close()

	for _, tc := range []struct{ name, target, body string }{
		// A name nobody holds, so the correction pass is actually entered: a
		// list every card of which resolves never asks the ranking anything.
		{"import", "/api/decks/import",
			`{"slug":"half-spelled","commander":["Goreclaw, Terror of Qal Sisma"],` +
				`"text":"1 Sol Ring\n1 Craterhoof Behemth"}`},
		{"bulk", cleanDeck + "/bulk", `{"text":"1 Sol Ring\n1 Craterhoof Behemth"}`},
	} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", tc.target, tc.body)
		if status != http.StatusInternalServerError {
			t.Errorf("%s answered %d over a pool that cannot rank: %s", tc.name, status, raw)
			continue
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
			t.Errorf("%s: the refusal reads %q", tc.name, detail)
		}
	}

	// And nothing was written: a deck built from a list the pass never finished
	// reading would be a deck whose cards were never checked.
	if status, _, _ := as(t, rig.api, alice, "/api/decks/alice/half-spelled"); status != http.StatusNotFound {
		t.Errorf("the refused import left a deck behind: %d", status)
	}
}
