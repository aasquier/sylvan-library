package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The promotion: the swap route's second door. A swap whose incoming card
// stands on the deck's own swap board is the stake being played, not a
// duplicate to refuse -- the outgoing card entombs with its rationale intact,
// the incoming card lifts off the board, and it lands in the outgoing card's
// category. What these tests hold is the layer above the editor: that the fold
// is composed from the verified operations and nothing else, that it is
// all-or-nothing, that every refusal the straight swap makes still lands, and
// that the response and the history both say which door was taken.

// writeDeck plants a bespoke fixture on the rig's file tier. The gate's
// shared fixtures cannot carry the promotion's shapes -- a board card the
// fixture pool knows, a card standing in two sections at once, a commander
// misfiled onto the board -- without teaching every other suite about them,
// so the shapes live here, beside the only tests that want them.
func (r *writeRig) writeDeck(t *testing.T, slug, text string) {
	t.Helper()
	dir := filepath.Join(r.decks, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The promotion fixture: a curated green deck whose board card -- Craterhoof
// Behemoth -- is in the 21-card fixture pool, legal in Commander, and inside
// Goreclaw's identity, which is what lets the happy path reach the editor.
const promotionFixture = `slug: promotion
name: Promotion Fixture
status: theoretical
stage: curated
commander:
  - Goreclaw, Terror of Qal Sisma
strategy: A fixture, not a deck; the board card is pool-known and green on purpose.
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one, and it always has been.
  - name: Regal Behemoth
    category: threat
    why: Doubles the mana and refills the hand.
  - name: Forest
    category: land
    why: Basic, untapped, and the only green source this list needs.
    qty: 97

swap_board:
  - name: Craterhoof Behemoth
    category: threat
    why: Waiting for a threat slot to open up.
`

const promotionDeck = "/api/decks/alice/promotion"

// The happy path, end to end: three moves, one write, one gate verdict, one
// history row.
func TestAPromotionMovesThreeCardsInOneWrite(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion", promotionFixture)

	status, body, raw := rig.do(t, alice, "POST", promotionDeck+"/swap",
		`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"The finisher the ramp was holding a seat for."}`)
	if status != http.StatusOK {
		t.Fatalf("the promotion answered %d: %s", status, raw)
	}

	// The straight swap's keys, plus `from` -- the one key that says which
	// door this was.
	if body["swapped_out"] != "Sol Ring" || body["swapped_in"] != "Craterhoof Behemoth" {
		t.Errorf("the response describes a different edit: %v", body)
	}
	if body["from"] != "swap_board" {
		t.Errorf("the response does not say the card came off the board: %v", body)
	}
	if body["why"] != "The finisher the ramp was holding a seat for." {
		t.Errorf("the fresh why did not ride the response: %v", body)
	}
	// The gate re-ran on the result, which is `commit`'s half of the promise.
	for _, key := range []string{"ok", "errors", "warnings", "stage", "total_cards", "needs_rationale"} {
		if _, present := body[key]; !present {
			t.Errorf("the response lacks %q: %v", key, body)
		}
	}

	after := rig.textOf(t, "promotion")
	boardAt := strings.Index(after, "swap_board:")
	graveAt := strings.Index(after, "graveyard:")
	if boardAt < 0 || graveAt < 0 {
		t.Fatalf("the file lost a section:\n%s", after)
	}
	// The board entry is gone -- lifted, never copied -- and its own `why`
	// went with it: that sentence argued why the card was NOT in the deck.
	if n := strings.Count(after, "Craterhoof Behemoth"); n != 1 {
		t.Errorf("the incoming card appears %d times; a promotion moves it exactly once:\n%s", n, after)
	}
	if !strings.Contains(after, "swap_board: []") {
		t.Errorf("the board entry survived the promotion:\n%s", after)
	}
	if strings.Contains(after, "Waiting for a threat slot") {
		t.Errorf("the board's own why travelled with the card:\n%s", after)
	}
	// The outgoing card is ENTOMBED -- never deleted -- with its rationale
	// verbatim, one click from coming back (ADR 27).
	if n := strings.Count(after, "Sol Ring"); n != 1 {
		t.Errorf("the outgoing card appears %d times, expected 1 (in the graveyard):\n%s", n, after)
	}
	if !strings.Contains(after[graveAt:], "Sol Ring") ||
		!strings.Contains(after[graveAt:], "Two mana on turn one, and it always has been.") {
		t.Errorf("the outgoing card is not in the graveyard with its rationale:\n%s", after)
	}
	// The incoming card holds the outgoing card's SLOT: its category, with
	// the fresh why -- ReplaceCard's category-keeping, worn by the fold.
	hoofAt := strings.Index(after, "- name: Craterhoof Behemoth")
	if hoofAt < 0 || hoofAt > boardAt {
		t.Fatalf("the incoming card did not land in the 99:\n%s", after)
	}
	entry := after[hoofAt:min(hoofAt+240, len(after))]
	if !strings.Contains(entry, "category: ramp") {
		t.Errorf("the incoming card did not take the outgoing card's category:\n%s", entry)
	}
	if !strings.Contains(entry, "The finisher the ramp was holding a seat for.") {
		t.Errorf("the fresh why did not land on the incoming card:\n%s", entry)
	}

	// ONE history row (ADR 28), with the promotion's own sentence.
	entries := rig.history(t, "promotion", nil)
	if len(entries) != 1 {
		t.Fatalf("the promotion recorded %d entries, expected 1: %+v", len(entries), entries)
	}
	if entries[0].Action != "swap" ||
		entries[0].Summary != "swapped Sol Ring out for Craterhoof Behemoth from the swap board" {
		t.Errorf("recorded %q / %q", entries[0].Action, entries[0].Summary)
	}
}

// The straight swap carries no `from` key at all: what door 1 and an old
// payload share is the client's default, never a value -- the new-wire-key
// rule from the server's side. The sentence in the history is the recorded
// one, byte for byte.
func TestAStraightSwapCarriesNoFromKey(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "POST", cleanDeck+"/swap",
		`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"The finisher the ramp is for."}`)
	if status != http.StatusOK {
		t.Fatalf("the straight swap answered %d: %s", status, raw)
	}
	if _, present := body["from"]; present {
		t.Errorf("a straight swap grew a `from` key: %v", body)
	}
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 1 ||
		entries[0].Summary != "swapped Sol Ring out for Craterhoof Behemoth" {
		t.Fatalf("the straight swap's recorded sentence moved: %+v", entries)
	}
}

// Rule 4 holds on the promotion path, and on a draft as hard as on a curated
// deck: the board's own rationale argues the opposite decision, so there is
// nothing on this door a blank why could borrow.
func TestAPromotionStillDemandsAFreshWhy(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion", promotionFixture)
	// The same deck one stage earlier: a draft owes rationales it has not
	// written, which is not the same thing as a slot actively reconsidered.
	rig.writeDeck(t, "promotion-draft",
		strings.Replace(promotionFixture, "stage: curated", "stage: draft", 1))

	for _, deck := range []string{"promotion", "promotion-draft"} {
		before := rig.textOf(t, deck)
		status, body, _ := rig.do(t, alice, "POST", "/api/decks/alice/"+deck+"/swap",
			`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"   "}`)
		if status != 422 || fmtDetail(body) != "a replacement needs a `why`" {
			t.Errorf("%s: a blank why answered %d %v", deck, status, body)
		}
		if rig.textOf(t, deck) != before {
			t.Errorf("%s: a refused promotion changed the file", deck)
		}
		if entries := rig.history(t, deck, nil); len(entries) != 0 {
			t.Errorf("%s: a refused promotion was recorded: %+v", deck, entries)
		}
	}
}

// The board entry's quantity travels into the 99: the stake is played at its
// own size, not re-dealt as a single copy. This is the deviation the PR flags
// for Aaron's ruling, so the test pins exactly the behaviour being put in
// front of him.
func TestAPromotionCarriesTheBoardQuantity(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	// The fixture ends inside the board entry, so an appended line is the
	// board entry's own.
	rig.writeDeck(t, "promotion-qty", promotionFixture+"    qty: 2\n")

	status, _, raw := rig.do(t, alice, "POST", "/api/decks/alice/promotion-qty/swap",
		`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"Two seats were being held."}`)
	if status != http.StatusOK {
		t.Fatalf("the promotion answered %d: %s", status, raw)
	}
	if entry := promoted(t, rig, "promotion-qty"); !strings.Contains(entry, "qty: 2") {
		t.Errorf("the board's quantity did not travel into the 99:\n%s", entry)
	}
}

// And the floor under it: a hand-written board entry can say `qty: 0` (the
// parser keeps an explicit zero; only an absent qty defaults to 1), and the
// promotion lands it as the single copy it really is. Without the floor the
// fold would hand back AddCard's "quantity must be at least 1" -- a refusal
// about a number the user never typed.
func TestAPromotionFloorsAQuantityBelowOne(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion-zero", promotionFixture+"    qty: 0\n")

	status, _, raw := rig.do(t, alice, "POST", "/api/decks/alice/promotion-zero/swap",
		`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"One copy is what it always was."}`)
	if status != http.StatusOK {
		t.Fatalf("the promotion answered %d: %s", status, raw)
	}
	// A quantity of one is the emitter's silence: the landed entry carries no
	// qty line at all.
	if entry := promoted(t, rig, "promotion-zero"); strings.Contains(entry, "qty:") {
		t.Errorf("a floored quantity still wrote a qty line:\n%s", entry)
	}
}

// promoted cuts the landed 99 entry out of a promotion fixture's file: from
// the incoming card's name line to the board section below it. Failing when
// the card never landed keeps the quantity tests honest about what they read.
func promoted(t *testing.T, rig *writeRig, slug string) string {
	t.Helper()
	after := rig.textOf(t, slug)
	at := strings.Index(after, "- name: Craterhoof Behemoth")
	boardAt := strings.Index(after, "swap_board:")
	if at < 0 || boardAt < at {
		t.Fatalf("the incoming card did not land in the 99:\n%s", after)
	}
	return after[at:boardAt]
}

// Only a 99 card holds a slot a promotion can take over. An `out` standing on
// the board itself passes the route's own checks (findCard reads the board
// too) and is refused by the fold's first step: EntombCard's frozen sentence,
// verbatim, with nothing written. The engine's goldens pin the sentence; this
// pins that THIS input still receives it if the fold is ever reordered.
func TestAPromotionRefusesAnOutStandingOnTheBoard(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion-bench", promotionFixture+`  - name: Llanowar Elves
    category: ramp
    why: Waiting on the bench beside the hoof.
`)

	before := rig.textOf(t, "promotion-bench")
	status, body, _ := rig.do(t, alice, "POST", "/api/decks/alice/promotion-bench/swap",
		`{"out":"Llanowar Elves","into":"Craterhoof Behemoth","why":"a reason"}`)
	if status != 422 || fmtDetail(body) !=
		"'Llanowar Elves' is on the swap board, which has no graveyard; remove it instead" {
		t.Errorf("an out standing on the board answered %d %v", status, body)
	}
	if rig.textOf(t, "promotion-bench") != before {
		t.Error("a refused promotion changed the file")
	}
	if entries := rig.history(t, "promotion-bench", nil); len(entries) != 0 {
		t.Errorf("a refused promotion was recorded: %+v", entries)
	}
}

// The half of the duplicate scan that did not move: a card already in the 99
// still refuses with the same sentence, on a deck that has a board to relax.
func TestASwapIntoACardAlreadyInThe99StillRefuses(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion", promotionFixture)

	before := rig.textOf(t, "promotion")
	status, body, _ := rig.do(t, alice, "POST", promotionDeck+"/swap",
		`{"out":"Sol Ring","into":"Regal Behemoth","why":"a reason"}`)
	if status != 422 || !strings.Contains(fmtDetail(body), "is already in this deck") {
		t.Errorf("a 99 duplicate answered %d %v", status, body)
	}
	if rig.textOf(t, "promotion") != before {
		t.Error("a refused swap changed the file")
	}
	if entries := rig.history(t, "promotion", nil); len(entries) != 0 {
		t.Errorf("a refused swap was recorded: %+v", entries)
	}
}

// Rule 1 does not age out: the board card was checked when it was staged, but
// the pool may have moved since, so the promotion asks it again. `rich`'s own
// board card is exactly that state -- staged in the file, absent from the
// fixture pool.
func TestAPromotionStillAsksThePool(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.textOf(t, "rich")
	status, body, _ := rig.do(t, alice, "POST", "/api/decks/alice/rich/swap",
		`{"out":"Sol Ring","into":"Sword of Feast and Famine","why":"a reason"}`)
	if status != 422 || !strings.Contains(fmtDetail(body), "Sword of Feast and Famine") {
		t.Errorf("a board card the pool does not know answered %d %v", status, body)
	}
	if rig.textOf(t, "rich") != before {
		t.Error("a refused promotion changed the file")
	}
	if entries := rig.history(t, "rich", nil); len(entries) != 0 {
		t.Errorf("a refused promotion was recorded: %+v", entries)
	}
}

// The fold is all-or-nothing. A card standing on the board AND in the
// graveyard fails at the fold's last step -- AddCard's own graveyard sentence,
// the return/exile door -- and the steps that succeeded before it must not
// survive: no entombment, no board removal, no history row.
func TestAPromotionIsAllOrNothing(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion-doubled", promotionFixture+`
graveyard:
  - name: Craterhoof Behemoth
    category: threat
    why: Entombed once already; the board copy is the mistake.
`)

	before := rig.textOf(t, "promotion-doubled")
	status, body, _ := rig.do(t, alice, "POST", "/api/decks/alice/promotion-doubled/swap",
		`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"a reason"}`)
	if status != 422 || !strings.Contains(fmtDetail(body), "return it or exile it") {
		t.Errorf("a graveyard double answered %d %v", status, body)
	}
	if rig.textOf(t, "promotion-doubled") != before {
		t.Error("a fold that failed at its last step left earlier steps behind")
	}
	if entries := rig.history(t, "promotion-doubled", nil); len(entries) != 0 {
		t.Errorf("a refused promotion was recorded: %+v", entries)
	}
}

// The commander refusal covers the second door too: even a file odd enough to
// hold the commander on its own board refuses the promotion by name, before
// the fold is reached.
func TestAPromotionOfTheCommanderRefuses(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	rig.writeDeck(t, "promotion-cmdr",
		strings.Replace(promotionFixture, "Craterhoof Behemoth",
			"Goreclaw, Terror of Qal Sisma", 1))

	before := rig.textOf(t, "promotion-cmdr")
	status, body, _ := rig.do(t, alice, "POST", "/api/decks/alice/promotion-cmdr/swap",
		`{"out":"Sol Ring","into":"Goreclaw, Terror of Qal Sisma","why":"a reason"}`)
	if status != 422 || !strings.Contains(fmtDetail(body), "is the commander") {
		t.Errorf("promoting the commander answered %d %v", status, body)
	}
	if rig.textOf(t, "promotion-cmdr") != before {
		t.Error("a refused promotion changed the file")
	}
}
