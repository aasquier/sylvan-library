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
swap_board:
  - name: Fixture Red Angel
    category: threat
    why: Waiting -- it is red, and I did not think the commander would take it.
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

// The empty-slot door, and the bug that found it.
//
// A card standing on the swap board had exactly one route into the 99: a swap,
// which demands a card to send the other way. On a deck still being built that
// is the interface inventing a rule the game does not have -- and the only
// other thing it would say was `deckedit.AddCard`'s duplicate refusal, "already
// in the swap board; change its quantity or rationale instead of adding a
// second entry". True, unhelpful, and the end of the road.
func TestACardOnTheSwapBoardMovesUpIntoAnEmptySlot(t *testing.T) {
	t.Parallel()
	rig, deckURL := selumaRig(t)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture Red Angel","category":"threat","why":"It flies, and the commander allows it."}`)
	if status != 200 {
		t.Fatalf("a board card could not move up into an empty slot: %d %s", status, raw)
	}
	if body["added"] != "Fixture Red Angel" {
		t.Errorf("the response describes a different edit: %v", body)
	}
	// `from` marks the door, exactly as the swap route marks its own.
	if body["from"] != "swap_board" {
		t.Errorf("the promotion did not say which door it came through: %v", body)
	}

	text := rig.textOf(t, "seluma")
	if strings.Contains(text, "swap_board:\n  - name: Fixture Red Angel") {
		t.Errorf("the card is still standing on the board:\n%s", text)
	}
	if !strings.Contains(text, "It flies, and the commander allows it.") {
		t.Errorf("the fresh rationale did not land:\n%s", text)
	}
	// Nothing was entombed: there was no slot to take over.
	if strings.Contains(text, "graveyard:\n  - name:") {
		t.Errorf("a move into an empty slot buried something:\n%s", text)
	}
}

// The board entry's own `why` argued the opposite decision, so it is never the
// one that carries the card in. Said here rather than left to AddCard, whose
// sentence ("a card in a curated deck needs a `why`") is true and does not
// explain why the rationale already on the screen will not do.
func TestMovingUpFromTheBoardNeedsItsOwnRationale(t *testing.T) {
	t.Parallel()
	rig, deckURL := selumaRig(t)
	defer rig.close()

	before := rig.textOf(t, "seluma")
	status, body, _ := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture Red Angel","category":"threat","why":"   "}`)
	if status != 422 {
		t.Fatalf("a promotion with no rationale was accepted: %d %v", status, body)
	}
	detail := fmtDetail(body)
	if !strings.Contains(detail, "argues why it has NOT made the deck") {
		t.Errorf("the refusal does not say which rationale is being asked for: %s", detail)
	}
	if rig.textOf(t, "seluma") != before {
		t.Error("a refused promotion changed the file")
	}
}

// The relaxation is exactly one door wide. Adding a board card back to the
// BOARD is a real duplicate and keeps the sentence it always had.
func TestAddingABoardCardToTheBoardIsStillADuplicate(t *testing.T) {
	t.Parallel()
	rig, deckURL := selumaRig(t)
	defer rig.close()

	status, body, _ := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture Red Angel","category":"threat","why":"A second entry.","to":"swap_board"}`)
	if status != 422 || !strings.Contains(fmtDetail(body), "already in the swap board") {
		t.Fatalf("a second board entry was accepted: %d %v", status, body)
	}
}

// The finder's own question, answered by the code that owns the rule.
//
// The browser used to work this out from `color_identity` against the
// commander's colours, and that reading was correct for exactly as long as
// colour identity had no exceptions. `deck=<owner>/<slug>` moves the question
// to the one implementation of the answer.
func TestTheCardFinderIsToldWhatThisDeckMayHold(t *testing.T) {
	t.Parallel()
	rig, _ := selumaRig(t)
	defer rig.close()

	verdicts := func(t *testing.T, query string) map[string]map[string]any {
		t.Helper()
		_, body, raw := rig.do(t, alice, "GET", query, "")
		out := map[string]map[string]any{}
		list, ok := body["cards"].([]any)
		if !ok {
			t.Fatalf("no cards in %s", raw)
		}
		for _, row := range list {
			card, _ := row.(map[string]any)
			name, _ := card["name"].(string)
			if p, ok := card["playable"].(map[string]any); ok {
				out[name] = p
			} else {
				out[name] = nil
			}
		}
		return out
	}

	// Without a deck: no verdict at all, which is what the commander field and
	// the card-search page want, and is the answer a caller gets for a deck
	// they may not read.
	for name, p := range verdicts(t, "/api/cards/suggest?q=Fixture&limit=8") {
		if p != nil {
			t.Errorf("%s carried a verdict nobody asked for: %v", name, p)
		}
	}

	got := verdicts(t, "/api/cards/suggest?q=Fixture&limit=8&deck=alice/seluma")
	angel, demon := got["Fixture Red Angel"], got["Fixture Black Demon"]
	if angel == nil || demon == nil {
		t.Fatalf("the offers were not measured against the deck: %v", got)
	}
	// The Angel: off-colour, and the commander's clause covers it. `outside`
	// is empty because the colours are not outside anything any more, and the
	// clause is named so the interface can say whose rule it was.
	if angel["ok"] != true || len(angel["outside"].([]any)) != 0 ||
		angel["allowed_by"] != "Seluma, Light of Aysen" {
		t.Errorf("the Angel the clause allows came back as %v", angel)
	}
	// The Demon: equally off-colour, named by no clause.
	if demon["ok"] != false || len(demon["outside"].([]any)) != 1 || demon["allowed_by"] != "" {
		t.Errorf("the Demon no clause covers came back as %v", demon)
	}

	// A deck this caller cannot read is answered as no deck at all -- never a
	// refusal, never a 404 of its own, so the response cannot be used to ask
	// whether somebody else's deck exists (ADR 5).
	for name, p := range verdicts(t, "/api/cards/suggest?q=Fixture&limit=8&deck=nobody/secret") {
		if p != nil {
			t.Errorf("%s was measured against a deck the caller cannot read: %v", name, p)
		}
	}
}
