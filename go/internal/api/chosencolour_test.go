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

// Tolabow's one chosen colour, at the write door.
//
// `internal/gate` already proves the *report* -- that a deck whose instants
// and sorceries reach one colour past the commander is legal and one that
// reaches two is not. This is the other half of the Rulebreaker fix and the
// half that matters more, exactly as `rulebreakerwrite_test.go` argues for
// Seluma: a wrong report can be read past, and a refused write cannot. If
// `playableCard` did not know about the clause, the card the gate approves
// could never be put in the file at all.
//
// The colour is **read back off the deck** rather than stored anywhere
// (`gate.Rulebreakers.ColorChoice` argues why), so which card is allowed in
// depends on what is already there -- which makes this the one write check in
// the app whose answer changes as the deck does, and the reason
// `chosenColorReach` exists. Until this file it had never run: not one
// statement of it, in either direction.
//
// The clause is the sentence printed on the card, read out of the pool rather
// than remembered (rule 1):
//
//	./mtglab cards show 'Tolabow, Loch Rascal'
//
// `internal/gate/rulebreaker_test.go` records the same sentence and how to
// re-derive it. The cards it is tested *against* are invented, for the reason
// that file gives: what is asserted is that the check reads a type line and an
// identity, not that any particular sorcery is black.

// tolabowPool is the tiny pool plus Tolabow and two off-colour spells -- one
// sorcery and one creature -- so the clause's edge is available in both
// directions. `Swords to Plowshares` is already in the recorded fixture and is
// the white instant this uses.
func tolabowPool(t *testing.T) *pool.Pool {
	t.Helper()
	return pooltest.OpenWith(t,
		pooltest.Card{Name: "Tolabow, Loch Rascal", ManaCost: "{2}{U}{U}", CMC: 4,
			TypeLine: "Legendary Creature — Otter", ColorIdentity: []string{"U"},
			OracleText: "Rulebreaker — If Tolabow, Loch Rascal is your commander, the " +
				"color identity of instant and sorcery cards in your deck can include one " +
				"color of your choice not in your commander's color identity, and your deck " +
				"can have any basic land cards.\nWhenever you cast an instant or sorcery " +
				"spell, create a 1/1 blue and red Otter creature token with prowess."},
		pooltest.Card{Name: "Fixture Black Sorcery", ManaCost: "{2}{B}", CMC: 3,
			TypeLine: "Sorcery", ColorIdentity: []string{"B"},
			OracleText: "Each opponent discards a card."},
		pooltest.Card{Name: "Fixture White Bear", ManaCost: "{2}{W}", CMC: 3,
			TypeLine: "Creature — Bear", ColorIdentity: []string{"W"},
			OracleText: "Vigilance"},
	)
}

// tolabowRig plants a Tolabow deck holding nothing but a Sol Ring, so the
// chosen colour starts out unspent.
func tolabowRig(t *testing.T) (*writeRig, string) {
	t.Helper()
	decks := decksDir(t)
	dir := filepath.Join(decks, "tolabow")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const text = `slug: tolabow
name: Tolabow Fixture
status: theoretical
stage: curated
commander:
  - Tolabow, Loch Rascal
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
		Pool: tolabowPool(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil),
	})
	return &writeRig{api: a, decks: decks, dbPath: dbPath, recorder: recorder,
		close: func() { recorder.Close(); db.Close() }}, "/api/decks/alice/tolabow"
}

// The whole rule in one deck's history: the first off-colour spell is the
// choice being made, the second in the same colour is fine, and the first in a
// different colour is refused -- because by then no single choice covers both.
func TestTheChosenColourIsSpentByTheFirstOffColourSpellAndReadBackAfterwards(t *testing.T) {
	t.Parallel()
	rig, deckURL := tolabowRig(t)
	defer rig.close()

	// Nothing off-colour yet, so a white instant is one colour past blue and
	// the clause covers it.
	status, body, raw := rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Swords to Plowshares","category":"interaction",`+
			`"why":"The cheapest answer in the game, and the clause reaches white."}`)
	if status != http.StatusOK {
		t.Fatalf("the first off-colour instant was refused: %d %s", status, raw)
	}
	if body["added"] != "Swords to Plowshares" {
		t.Errorf("the response describes a different edit: %v", body)
	}

	// A second spell in a *different* colour would make the instants and
	// sorceries reach two, and no single choice covers two.
	status, body, raw = rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture Black Sorcery","category":"interaction",`+
			`"why":"A second colour, which the clause does not grant."}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a second off-identity colour was accepted: %d %s", status, raw)
	}
	detail := fmtDetail(body)
	// The refusal has to name **both** colours and say what to do, because
	// the card being refused is not on its own the problem: the deck is.
	for _, want := range []string{"Fixture Black Sorcery", "{B", "Cut the ones"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal never says %q, so it reads as 'this card is "+
				"illegal' when the truth is 'this card and that one': %q", want, detail)
		}
	}
	if strings.Contains(detail, "Swords to Plowshares") {
		// Not required, and worth knowing if it changes: the sentence names
		// the colours rather than the cards that reach them.
		t.Logf("the refusal now names the card already holding the colour: %q", detail)
	}

	// And the clause covers instants and sorceries and stops there: an
	// off-colour creature is an ordinary identity refusal, with no mention of
	// a choice, even in the colour that was chosen.
	status, body, raw = rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Fixture White Bear","category":"threat",`+
			`"why":"White, like the instant that went in -- but a creature."}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an off-identity creature under Tolabow was accepted: %d %s", status, raw)
	}
	if detail := fmtDetail(body); strings.Contains(detail, "Cut the ones") {
		t.Errorf("a creature was refused with the colour-choice sentence, which "+
			"tells somebody to cut spells that are not the problem: %q", detail)
	}
}

// The same rule at the other door. A swap resolves the incoming card through
// the same check for the reason the route argues: a card that was legal when
// it was staged may not be legal now, and rule 1 does not age out.
//
// And it resolves it against the deck the swap would PRODUCE. This test used
// to record the older answer -- the check read the deck as it stood, outgoing
// card included, so a one-for-one trade of Tolabow's chosen colour was
// refused with {BW} even though the deck after the swap reaches exactly one
// colour and is legal. Aaron ruled on 2026-09-24 that a swap is one move, not
// a cut followed by an add, and `without` in the route is the change. What
// still has to be refused is a swap that genuinely widens the reach: trading
// a colourless rock for the black sorcery while the white instant stays.
func TestTheSwapDoorReadsTheChosenColourAsTheDeckWillStand(t *testing.T) {
	t.Parallel()
	rig, deckURL := tolabowRig(t)
	defer rig.close()

	status, _, raw := rig.do(t, alice, "POST", deckURL+"/swap",
		`{"out":"Sol Ring","into":"Swords to Plowshares",`+
			`"why":"One colour past blue is exactly what the commander grants."}`)
	if status != http.StatusOK {
		t.Fatalf("the swap door refused a card the clause covers: %d %s", status, raw)
	}

	// The rock goes back in beside the white instant, so there is a
	// colourless card to trade away.
	status, _, raw = rig.do(t, alice, "POST", deckURL+"/cards",
		`{"name":"Sol Ring","category":"ramp","why":"Two mana on turn one."}`)
	if status != http.StatusOK {
		t.Fatalf("adding the rock back answered %d: %s", status, raw)
	}

	// A trade that would widen the reach is still refused -- the white
	// instant stays, the black sorcery would come in -- and the refusal
	// still shows both colours and names the way through.
	status, body, raw := rig.do(t, alice, "POST", deckURL+"/swap",
		`{"out":"Sol Ring","into":"Fixture Black Sorcery",`+
			`"why":"A second colour past blue, which the clause does not grant."}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a swap that widens the reach to two colours answered %d: %s", status, raw)
	}
	detail := fmtDetail(body)
	if !strings.Contains(detail, "{BW}") {
		t.Errorf("the refusal does not show both colours, so it does not say why "+
			"the trade was refused: %q", detail)
	}
	if !strings.Contains(detail, "Cut the ones in the colour you are not keeping first") {
		t.Errorf("the refusal no longer tells the player what to do first: %q", detail)
	}

	// The one-for-one trade of the chosen colour: white out, black in, and
	// the deck the swap produces reaches exactly one colour. One move.
	status, body, raw = rig.do(t, alice, "POST", deckURL+"/swap",
		`{"out":"Swords to Plowshares","into":"Fixture Black Sorcery",`+
			`"why":"Swapping the white one out leaves black as the one colour."}`)
	if status != http.StatusOK {
		t.Fatalf("a one-for-one trade of the chosen colour was refused (%d), so the "+
			"door is reading the deck as it stands rather than as the swap leaves "+
			"it: %s %s", status, fmtDetail(body), raw)
	}
}
