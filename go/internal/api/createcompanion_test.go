package api

import (
	"net/http"
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

// The two commanders and the companion: the parts of `createDeck` and
// `importDeck` the 21-card fixture pool could not reach.
//
// Both routes take three things off the command zone -- one commander, two
// commanders, and a companion -- and the recorded fixture holds no pairing
// ability and no companion clause at all, so two thirds of that had never run.
// `docs/polish/COVERAGE.md` named them as the top two functions in the tree
// and said the fixture was the obstacle; it is, and the answer is the one
// `rulebreakerwrite_test.go` already established: the rows are doctored into
// the **test's own temp copy** of the pool, and the recorded corpus on disk is
// untouched.
//
// The invented cards are named "Fixture ..." as `pooltest.Card` asks. The one
// real card among them is Kaheera, whose companion sentence is a template a
// checker reads, copied out of the pool rather than remembered:
//
//	./mtglab cards show 'Kaheera, the Orphanguard'
//
// The pairing lines are the printed keyword itself, which is the whole of what
// `gate.PairingOf` reads.

// pairedPool is the tiny pool plus a companion and three legends: two that
// carry Partner and one that carries nothing, so a legal pair and an illegal
// one are both available.
func pairedPool(t *testing.T) *pool.Pool {
	t.Helper()
	return pooltest.OpenWith(t,
		pooltest.Card{Name: "Kaheera, the Orphanguard", ManaCost: "{1}{G/W}{G/W}", CMC: 3,
			TypeLine: "Legendary Creature — Cat Beast", ColorIdentity: []string{"G", "W"},
			OracleText: "Companion — Each creature card in your starting deck is a Cat, " +
				"Elemental, Nightmare, Dinosaur, or Beast card. (If this card is your chosen " +
				"companion, you may put it into your hand from outside the game for {3} as a " +
				"sorcery.)\nVigilance"},
		pooltest.Card{Name: "Fixture Partnered Knight", ManaCost: "{2}{W}", CMC: 3,
			TypeLine: "Legendary Creature — Human Knight", ColorIdentity: []string{"W"},
			OracleText: "Partner (You can have two commanders if both have partner.)"},
		pooltest.Card{Name: "Fixture Partnered Mage", ManaCost: "{1}{U}", CMC: 2,
			TypeLine: "Legendary Creature — Human Wizard", ColorIdentity: []string{"U"},
			OracleText: "Partner (You can have two commanders if both have partner.)"},
		pooltest.Card{Name: "Fixture Lonely Baron", ManaCost: "{3}{B}", CMC: 4,
			TypeLine: "Legendary Creature — Vampire Noble", ColorIdentity: []string{"B"},
			OracleText: "Flying"},
	)
}

// pairedRig is `newWriteRig`'s wiring over that pool.
func pairedRig(t *testing.T) *writeRig {
	t.Helper()
	decks := decksDir(t)
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
		Pool: pairedPool(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil),
	})
	return &writeRig{api: a, decks: decks, dbPath: dbPath, recorder: recorder,
		close: func() { recorder.Close(); db.Close() }}
}

// A companion named at creation is checked against the pool like the
// commander, and is written into the deck file rather than reported and
// dropped.
//
// It matters that it reaches the *file*: the companion is one of the two facts
// the gate's companion check reads, so a create that answered with a companion
// and wrote a deck without one would leave the gate silently approving a deck
// nobody built.
func TestACreateChecksItsCompanionAndKeepsIt(t *testing.T) {
	t.Parallel()
	rig := pairedRig(t)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"with-cat","commander":"Goreclaw, Terror of Qal Sisma",`+
			`"companion":"Kaheera, the Orphanguard"}`)
	if status != http.StatusOK {
		t.Fatalf("a create with a companion answered %d: %s", status, raw)
	}
	if payload["companion"] != "Kaheera, the Orphanguard" {
		t.Errorf("the answer carries companion %v", payload["companion"])
	}
	text := rig.textOf(t, "with-cat")
	if !strings.Contains(text, "companion: Kaheera, the Orphanguard") {
		t.Errorf("the companion never reached the deck file:\n%s", text)
	}

	// A companion the pool does not hold is refused by name, in the same
	// sentence a missing commander gets -- and nothing is created.
	status, payload, _ = rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"no-cat","commander":"Goreclaw, Terror of Qal Sisma",`+
			`"companion":"Fixture Imaginary Cat"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown companion was accepted: %d", status)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "Fixture Imaginary Cat") {
		t.Errorf("the refusal does not name the card that was not found: %q", detail)
	}
	if _, ok := rig.read(t, "no-cat"); ok {
		t.Error("a refused create left a deck behind")
	}
}

// Two commanders are not "any two legends", and the create door runs the same
// pairing rule the gate does.
//
// The refusal is the point rather than the acceptance: a create that let two
// unrelated legends through would build a deck that can never be legal, and
// the person would find out from the gate afterwards, with ninety-nine cards
// already filed under it.
func TestACreateRunsThePairingRuleOnTwoCommanders(t *testing.T) {
	t.Parallel()
	rig := pairedRig(t)
	defer rig.close()

	// Two that carry the keyword: the deck is created, and its identity is
	// the union of theirs (rule 2 -- read off each card, never off the cost).
	status, payload, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"a-pair","commander":["Fixture Partnered Knight","Fixture Partnered Mage"]}`)
	if status != http.StatusOK {
		t.Fatalf("a legal pair was refused: %d %s", status, raw)
	}
	letters, _ := payload["color_identity"].([]any)
	got := []string{}
	for _, l := range letters {
		got = append(got, l.(string))
	}
	if strings.Join(got, "") != "UW" {
		t.Errorf("a white and a blue commander made identity %v", got)
	}

	// One that does not: refused, with the reason on the screen rather than
	// "invalid pair".
	status, payload, _ = rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"not-a-pair","commander":["Fixture Partnered Knight","Fixture Lonely Baron"]}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a pair with one pairing ability was accepted: %d", status)
	}
	// The sentence names the card that *does* carry the ability and says the
	// other needs a matching one, which is the way round that tells somebody
	// what to change.
	detail := fmtDetail(payload)
	if !strings.Contains(detail, "Fixture Partnered Knight") || !strings.Contains(detail, "pairing ability") {
		t.Errorf("the refusal does not say which card cannot pair or why: %q", detail)
	}
	if _, ok := rig.read(t, "not-a-pair"); ok {
		t.Error("a refused pair left a deck behind")
	}

	// And three is refused before the pool is ever asked.
	status, payload, _ = rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"a-trio","commander":["Fixture Partnered Knight","Fixture Partnered Mage","Fixture Lonely Baron"]}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("three commanders were accepted: %d", status)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "at most two") {
		t.Errorf("the refusal does not say what the limit is: %q", detail)
	}
}

// An import carries a companion through the same way, and the deck it writes
// is the one the answer describes.
func TestAnImportKeepsTheCompanionItWasGiven(t *testing.T) {
	t.Parallel()
	rig := pairedRig(t)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"cat-import","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"companion":"Kaheera, the Orphanguard",`+
			`"text":"1 Sol Ring\n1 Forest\n1 Craterhoof Behemoth"}`)
	if status != http.StatusOK {
		t.Fatalf("an import with a companion answered %d: %s", status, raw)
	}
	if payload["companion"] != "Kaheera, the Orphanguard" {
		t.Errorf("the answer carries companion %v", payload["companion"])
	}
	if text := rig.textOf(t, "cat-import"); !strings.Contains(text, "companion: Kaheera, the Orphanguard") {
		t.Errorf("the companion never reached the deck file:\n%s", text)
	}
}

// A sideboard line is staged on the swap board rather than filed into the 99,
// and the answer names what is standing there.
//
// The board is where a card waits, so an import that quietly folded a
// sideboard into the deck would hand somebody a 103-card deck and a gate error
// about a count they never chose.
func TestAnImportStagesItsSideboardOnTheSwapBoard(t *testing.T) {
	t.Parallel()
	rig := pairedRig(t)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"boarded","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"text":"1 Sol Ring\n1 Forest\n\nSideboard\n1 Craterhoof Behemoth"}`)
	if status != http.StatusOK {
		t.Fatalf("an import with a sideboard answered %d: %s", status, raw)
	}
	board, _ := payload["swap_board"].([]any)
	if len(board) == 0 {
		t.Fatalf("the sideboard line was not staged anywhere: %s", raw)
	}
	if board[0] != "Craterhoof Behemoth" {
		t.Errorf("the board holds %v", board)
	}
	if text := rig.textOf(t, "boarded"); !strings.Contains(text, "swap_board:") {
		t.Errorf("the board never reached the deck file:\n%s", text)
	}
}

// Neither route will build a deck without the card pool, and both say the same
// thing when it is missing.
//
// This is the one refusal in the lifecycle that is about the *instance* rather
// than about the request, and it is deliberately not a half-measure: a deck
// created with no pool has an identity nobody checked (rule 2), which is the
// one fact this project will not guess at.
func TestNeitherCreateNorImportWillBuildADeckWithoutThePool(t *testing.T) {
	t.Parallel()
	decks := decksDir(t)
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{DecksDir: decks, AdminEmail: "alice@example.com", AppDB: db})

	for _, row := range []struct{ target, body string }{
		{"/api/decks", `{"slug":"poolless","commander":"Goreclaw, Terror of Qal Sisma"}`},
		{"/api/decks/import", `{"slug":"poolless","commander":["Goreclaw, Terror of Qal Sisma"],"text":"1 Sol Ring"}`},
	} {
		status, payload, raw := callAs(t, a, alice, "POST", row.target, row.body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d with no pool: %s", row.target, status, raw)
			continue
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "card pool") {
			t.Errorf("%s refused without saying what is missing: %q", row.target, detail)
		}
		if _, err := os.Stat(filepath.Join(decks, "poolless", "deck.yaml")); err == nil {
			t.Errorf("%s wrote a deck it had refused", row.target)
		}
	}
}
