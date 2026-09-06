package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The write path's own half of the Rulebreaker fix, and it is the half that
// matters more.
//
// `internal/gate` deciding a deck is legal is only half an answer while
// `playableCard` still refuses the card at the door: a wrong *report* can be
// read past, and a refused *write* cannot -- there is no way to get the Angel
// into the file at all. So the check that widens the identity has to be the
// same check in both places, and this proves it is by driving the real route.
//
// The 21-card fixture pool carries no Mystery Booster commander, so three rows
// are doctored into the test's own temp copy of it. That is what
// `deckread_test.go` and `stale_test.go` already do and it is not a fixture
// edit: the recorded corpus on disk is untouched.

// rulebreakerPool is the tiny pool plus a Rulebreaker commander, an Angel
// outside her colours, and a Demon that is equally outside them and is not an
// Angel. The clause is the one printed on the card, read out of the card pool
// rather than remembered (rule 1); `internal/gate`'s rulebreaker_test.go
// records where it came from and how to re-derive it.
func rulebreakerPool(t *testing.T) *pool.Pool {
	t.Helper()
	return pooltest.OpenWith(t,
		pooltest.Card{Name: "Seluma, Light of Aysen", ManaCost: "{4}{W}", CMC: 5,
			TypeLine: "Legendary Creature — Angel Warrior", ColorIdentity: []string{"W"},
			OracleText: "Rulebreaker — A deck with this commander can have Angel cards of " +
				"any color identity and any basic land cards.\nFlying"},
		pooltest.Card{Name: "Fixture Red Angel", ManaCost: "{5}{R}{W}", CMC: 7,
			TypeLine: "Legendary Creature — Angel", OracleText: "Flying, first strike",
			ColorIdentity: []string{"R", "W"}},
		pooltest.Card{Name: "Fixture Black Demon", ManaCost: "{4}{B}", CMC: 5,
			TypeLine: "Creature — Demon", OracleText: "Flying",
			ColorIdentity: []string{"B"}},
	)
}

// selumaRig is newWriteRig's wiring over the doctored pool and one extra deck.
func selumaRig(t *testing.T) (*writeRig, string) {
	t.Helper()
	decks := decksDir(t)
	dir := filepath.Join(decks, "seluma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const text = `slug: seluma
name: Seluma Fixture
status: theoretical
stage: curated
commander:
  - Seluma, Light of Aysen
bracket: 2
strategy: A fixture, not a deck.
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one.
`
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := decklog.NewRecorder(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{
		Pool: rulebreakerPool(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil),
	})
	return &writeRig{api: a, decks: decks, dbPath: dbPath, recorder: recorder,
		close: func() { recorder.Close(); db.Close() }}, "/api/decks/alice/seluma"
}

// The whole bug, at the door: the Angel the commander's own card makes legal
// goes in, and the Demon it says nothing about is still refused -- and the
// refusal quotes the clause, so the answer to "why that one and not this one"
// is on the screen rather than in this file.
func TestARulebreakerCommanderLetsTheCardsItNamesThroughTheDoor(t *testing.T) {
	t.Parallel()
	rig, deckURL := selumaRig(t)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture Red Angel","category":"threat","why":"An Angel, and the commander says Angels may be any colour."}`)
	if status != 200 {
		t.Fatalf("the commander's own clause did not reach the write path: %d %s", status, raw)
	}
	if body["added"] != "Fixture Red Angel" {
		t.Errorf("the response describes a different edit: %v", body)
	}

	status, body, _ = rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture Black Demon","category":"threat","why":"Not an Angel."}`)
	if status != 422 {
		t.Fatalf("an off-identity card the clause does not name was accepted: %d %v", status, body)
	}
	detail := fmtDetail(body)
	if !strings.Contains(detail, "outside the commander's {W}") {
		t.Errorf("the refusal lost its sentence: %s", detail)
	}
	if !strings.Contains(detail, `Seluma, Light of Aysen's Rulebreaker does not cover it: `+
		`"A deck with this commander can have Angel cards`) {
		t.Errorf("the refusal does not quote the clause the card failed: %s", detail)
	}
}
