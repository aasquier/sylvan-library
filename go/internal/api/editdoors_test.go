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
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The edit routes' remaining doors: the promotion off the swap board, the two
// removals that are not entombments, the patch that needs a commander, and the
// combo list's own reading of what a client sent.
//
// None of these is exotic. They are the answers somebody gets for pressing the
// button next to the one they meant, which is the traffic commandment 2 is
// written for -- and they were unreached for the ordinary reason: the write
// tests drive the happy path of each route and stop, because the happy path is
// what the goldens describe.

// boardRig plants a deck with a card standing on its swap board that the pool
// actually holds, which the recorded fixtures do not have: `rich.yaml`'s board
// card is outside the 21.
func boardRig(t *testing.T) (*writeRig, string) {
	t.Helper()
	decks := decksDir(t)
	plantDeck(t, decks, "boarded", `slug: boarded
name: Boarded Fixture
status: theoretical
stage: curated
commander:
  - Goreclaw, Terror of Qal Sisma
bracket: 2
strategy: A fixture, not a deck.
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one.
  - name: Forest
    category: land
    why: It taps for green.
swap_board:
  - name: Craterhoof Behemoth
    category: threat
    why: Waiting -- eight mana is a lot for a deck this slow.
`)
	plantDeck(t, decks, "headless-edit", `slug: headless-edit
name: Headless Fixture
status: theoretical
stage: curated
commander: []
bracket: 2
strategy: A fixture, not a deck.
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one.
`)
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
		Pool: pooltest.Open(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil),
	})
	return &writeRig{api: a, decks: decks, dbPath: dbPath, recorder: recorder,
		close: func() { recorder.Close(); db.Close() }}, "/api/decks/alice/boarded"
}

// A card is promoted off the board into the 99 by adding it, and the
// promotion is marked so the deck's history says where the card came from.
//
// The fresh `why` is rule 4 at its sharpest: the board entry's rationale
// argues why the card had NOT made the deck, and reusing it to argue the
// reversal would put words in somebody's mouth. So the refusal without one is
// half this test.
func TestACardIsPromotedOffTheBoardAndNeedsAFreshReasonToGo(t *testing.T) {
	t.Parallel()
	rig, deckURL := boardRig(t)
	defer rig.close()

	// Without a reason: refused, and the sentence says the rationale it has
	// is the wrong one rather than that it has none.
	status, payload, raw := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Craterhoof Behemoth","category":"threat"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a promotion with no fresh reason answered %d: %s", status, raw)
	}
	detail := fmtDetail(payload)
	for _, want := range []string{"swap board", "argues why it has NOT", "fresh"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal never says %q: %q", want, detail)
		}
	}

	// With one: it moves, and the answer marks the door it came through.
	status, payload, raw = rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Craterhoof Behemoth","category":"threat",`+
			`"why":"The deck got faster, so eight mana is reachable after all."}`)
	if status != http.StatusOK {
		t.Fatalf("a promotion with a fresh reason answered %d: %s", status, raw)
	}
	if payload["from"] != "swap_board" {
		t.Errorf("the answer does not mark the door the card came through: %v", payload)
	}
	text := rig.textOf(t, "boarded")
	if strings.Contains(text, "Waiting -- eight mana") {
		t.Errorf("the board's own rationale survived the promotion:\n%s", text)
	}
	if !strings.Contains(text, "The deck got faster") {
		t.Errorf("the fresh rationale did not land:\n%s", text)
	}
	// ADR 28: one entry, and it says where the card came from.
	entries := rig.history(t, "boarded", nil)
	if len(entries) != 1 || !strings.Contains(entries[0].Summary, "Craterhoof Behemoth") {
		t.Errorf("the promotion recorded %+v", entries)
	}
}

// A card standing on the board is removed outright rather than entombed: it
// was never in the deck, and the board is its own record of why.
func TestRemovingABoardCardTakesItAwayRatherThanBuryingIt(t *testing.T) {
	t.Parallel()
	rig, deckURL := boardRig(t)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "DELETE",
		deckURL+"/cards/Craterhoof%20Behemoth", "")
	if status != http.StatusOK {
		t.Fatalf("removing a board card answered %d: %s", status, raw)
	}
	if payload["removed"] != "Craterhoof Behemoth" {
		t.Errorf("the answer says %v, and an entombment would say `entombed`", payload)
	}
	text := rig.textOf(t, "boarded")
	if strings.Contains(text, "Craterhoof Behemoth") {
		t.Errorf("the board card is still in the file:\n%s", text)
	}
	if strings.Contains(text, "graveyard:") {
		t.Errorf("a board card was buried:\n%s", text)
	}

	// A card in neither place is refused by name, and nothing is written.
	before := rig.textOf(t, "boarded")
	status, payload, raw = rig.do(t, alice, "DELETE",
		deckURL+"/cards/Fixture%20Nothing%20Like%20This", "")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("removing a card that is not there answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "not in this deck") {
		t.Errorf("the refusal does not say what is wrong: %q", detail)
	}
	if rig.textOf(t, "boarded") != before {
		t.Error("a refused removal rewrote the file")
	}
}

// `qty` is coerced rather than trusted, and a quantity that is not a number
// names the field -- an add carrying ninety-nine good characters and one bad
// one has to say which one.
func TestAQuantityThatIsNotANumberNamesTheFieldRatherThanTheCard(t *testing.T) {
	t.Parallel()
	rig, deckURL := boardRig(t)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Swamp","category":"land","why":"A land.","qty":"lots"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a quantity of \"lots\" answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "qty must be a number") {
		t.Errorf("the refusal does not name the field: %q", detail)
	}
}

// A deck with no commander has no colour identity to be outside of, and no
// art to set -- two different sentences, and neither is "unknown error".
//
// The identity one renders `{C}` rather than `{}`, which is the difference
// between "colourless" and a bracket pair somebody reads as a bug.
func TestADeckWithNoCommanderRefusesInWordsRatherThanInBrackets(t *testing.T) {
	t.Parallel()
	rig, _ := boardRig(t)
	defer rig.close()
	const headless = "/api/decks/alice/headless-edit"

	status, payload, raw := rig.do(t, alice, "POST", headless+"/cards",
		`{"name":"Swamp","category":"land","why":"A land, and a black one."}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an off-identity card went into a commanderless deck: %d %s", status, raw)
	}
	detail := fmtDetail(payload)
	if !strings.Contains(detail, "{C}") {
		t.Errorf("a colourless identity rendered as something other than {C}: %q", detail)
	}
	if strings.Contains(detail, "{}") {
		t.Errorf("the refusal shows an empty pair of braces, which reads as a bug: %q", detail)
	}

	status, payload, raw = rig.do(t, alice, "PATCH", headless,
		`{"field":"commander_art","value":"0aae2e33-0000-4000-8000-000000000000"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("art was set on a deck with no commander: %d %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "no commander") {
		t.Errorf("the refusal does not say why: %q", detail)
	}
}

// The printing check behind `commander_art`, over a pool that answers an
// error rather than a row.
//
// It has three answers and they are deliberately not two: no pool at all is
// silent (the field is set and the check is skipped), a row that is not there
// is a refusal naming the printing, and a query that *failed* is neither -- it
// must not be reported as "that is not a printing of your commander", which
// sends somebody to a list to pick again from.
func TestAFailedPrintingQueryIsNotReportedAsTheWrongPrinting(t *testing.T) {
	t.Parallel()
	a := failingPoolAPI(t)

	status, payload, raw := callAs(t, a, alice, "PATCH", "/api/decks/alice/kaheera",
		`{"field":"commander_art","value":"0aae2e33-0000-4000-8000-000000000000"}`)
	if status == http.StatusOK {
		t.Fatalf("the art was set over a pool that could not check it: %s", raw)
	}
	if detail := fmtDetail(payload); strings.Contains(detail, "is not a printing of") {
		t.Errorf("a failed query was reported as the wrong printing: %q", detail)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries no sentence: %s", raw)
	}
}

// The combo list reads what a client sends rather than only what the form
// writes: one card as a bare string, and the blank rows a form leaves behind
// when somebody clears one instead of removing it.
func TestTheComboListReadsAShorthandAndDropsTheBlanksAFormLeaves(t *testing.T) {
	t.Parallel()
	rig, deckURL := boardRig(t)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "PUT", deckURL+"/combos",
		`{"combos":[{"cards":"Sol Ring","produces":"two mana"},`+
			`{"cards":["Forest",null,"  ","Sol Ring"],"produces":"a green mana and two more"}]}`)
	if status != http.StatusOK {
		t.Fatalf("the combo list answered %d: %s", status, fmtDetail(payload)+string(raw))
	}
	text := rig.textOf(t, "boarded")
	if !strings.Contains(text, "two mana") {
		t.Errorf("a one-card combo written as a bare string did not land:\n%s", text)
	}
	// The blanks are gone and the two real names are not.
	if strings.Contains(text, "- ''") || strings.Contains(text, `- ""`) {
		t.Errorf("a cleared row was written as a blank card:\n%s", text)
	}
}

// The combo list over an instance with no pool at all. The names cannot be
// canonicalised, and the recorded answer is to write them as typed rather than
// to refuse -- a combo is the player's own note about their own deck, and an
// instance without a card pool still lets them keep one.
func TestTheComboListStillWritesWhenThereIsNoPoolToCanonicaliseAgainst(t *testing.T) {
	t.Parallel()
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
	defer func() { recorder.Close(); db.Close() }()
	a := New(Config{DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder})

	status, payload, raw := callAs(t, a, alice, "PUT",
		"/api/decks/alice/mono-green-clean/combos",
		`{"combos":[{"cards":["sol ring","forest"],"produces":"two mana and a green one"}]}`)
	if status != http.StatusOK {
		t.Fatalf("the combo list answered %d with no pool: %s",
			status, fmtDetail(payload)+string(raw))
	}
	written, err := os.ReadFile(filepath.Join(decks, "mono-green-clean", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "sol ring") {
		t.Errorf("with no pool to check spellings against, the names were not "+
			"written as typed:\n%s", written)
	}
}
