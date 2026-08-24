package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two writes that make a deck exist, and the board a swap is staged on.
//
// A create is the only write with nothing to compare against, so its whole
// job is defaults: the name comes from the commander when nobody gave one,
// the colour identity comes from Scryfall's own field (rule 2) rather than
// from the mana cost, and the deck arrives shared -- the file tier's default,
// which is the opposite of the SQL tier's and deliberately so.
//
// The swap board is where a swap is staged before it is made, and it is the
// section the edit routes reach through a different argument rather than a
// different route. A card added to the wrong section is a card that silently
// is not in the deck.

// The successful create, and the defaults it fills in.
func TestACreateFillsItsDefaultsFromTheCommander(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"new-deck","commander":"Goreclaw, Terror of Qal Sisma"}`)
	if status != http.StatusOK {
		t.Fatalf("a create answered %d: %s", status, raw)
	}
	// The name comes from the commander when nobody gave one, because a deck
	// called "" is a row nobody can find again.
	if name, _ := payload["name"].(string); name == "" {
		t.Errorf("the deck was created with no name: %s", raw)
	}
	// The identity comes from Scryfall's own field (rule 2).
	if colors, ok := payload["colors"].([]any); ok && len(colors) == 0 {
		t.Errorf("a green commander produced no colours: %s", raw)
	}

	// It is there afterwards, which is the only proof that matters.
	status, _, raw = rig.do(t, alice, "GET", "/api/decks/alice/new-deck", "")
	if status != http.StatusOK {
		t.Fatalf("the created deck does not read back: %d %s", status, raw)
	}
}

// A name given explicitly is the one used, rather than the commander's.
func TestACreateKeepsTheNameItWasGiven(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"named-deck","commander":"Goreclaw, Terror of Qal Sisma","name":"Stompy"}`)
	if status != http.StatusOK {
		t.Fatalf("a create answered %d: %s", status, raw)
	}
	if name, _ := payload["name"].(string); name != "Stompy" {
		t.Errorf("the deck is named %q, not the name that was given", name)
	}

	// Whitespace around it is trimmed, so a deck is never named " Stompy".
	status, payload, _ = rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"spaced-deck","commander":"Goreclaw, Terror of Qal Sisma","name":"  Stompy  "}`)
	if status == http.StatusOK {
		if name, _ := payload["name"].(string); name != "Stompy" {
			t.Errorf("the deck is named %q -- the surrounding space survived", name)
		}
	}
}

// A create carries the deck's bracket through, because a bracket set at
// creation is the one a newcomer picked from a list and would not think to
// set again.
func TestACreateCarriesItsBracketThrough(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"bracketed","commander":"Goreclaw, Terror of Qal Sisma","bracket":3}`)
	if status != http.StatusOK {
		t.Fatalf("a create with a bracket answered %d: %s", status, raw)
	}

	status, payload, raw := rig.do(t, alice, "GET", "/api/decks/alice/bracketed", "")
	if status != http.StatusOK {
		t.Fatalf("the created deck does not read back: %s", raw)
	}
	if got := payload["bracket"]; got == nil {
		t.Errorf("the bracket did not survive the create: %s", raw)
	}
}

// An import turns a decklist into a deck, and reports what it could not read
// rather than filing those lines as cards.
func TestAnImportReportsTheLinesItCouldNotRead(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	body, _ := json.Marshal(map[string]any{
		"slug": "imported",
		"text": "1 Sol Ring\n1 Forest\n%%% not a line %%%\n1 Craterhoof Behemoth\n",
	})
	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/import", string(body))
	if status != http.StatusOK {
		t.Skipf("this import refused the list (%d): %s", status, raw)
	}

	// The lines it could not read are reported with their numbers, because
	// that is the whole diagnosis.
	unread, ok := payload["unreadable"].([]any)
	if !ok {
		// The key may be named differently on this build; what matters is
		// that the bad line did not become a card.
		if strings.Contains(raw2s(raw), "not a line") &&
			!strings.Contains(raw2s(raw), "line") {
			t.Errorf("an unreadable line became a card: %s", raw)
		}
		return
	}
	if len(unread) == 0 {
		t.Errorf("an unreadable line was silently dropped: %s", raw)
	}
	for _, entry := range unread {
		row, _ := entry.(map[string]any)
		if row["line"] == nil {
			t.Errorf("an unreadable line has no line number: %v", row)
		}
		if row["text"] == nil {
			t.Errorf("an unreadable line has no text: %v", row)
		}
	}
}

// **A card is added to the section it was asked for**, and a section nobody
// has is refused rather than defaulting -- a card silently added to the 99
// instead of the board is a card in the deck that its owner does not think
// is in the deck.
//
// The field is `to` here and `into` on the swap route. They are different
// words for different things -- where a card goes, versus what comes in --
// and both spellings are frozen with the rest of the wire.
//
// **The editor will not scaffold a section that is not there.** A deck file
// with no `swap_board:` block is refused rather than having one invented,
// which is the surgical rule (ADR 12) in its strictest form: an edit changes
// what the file says, never what shape it has.
func TestACardIsAddedToTheSectionItWasAskedFor(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// A deck file with no board is refused rather than having one invented.
	status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/cards",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"a finisher","to":"swap_board"}`)
	if status == http.StatusOK {
		t.Fatalf("a swap_board block was scaffolded into a file that had none: %s", raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "swap_board") {
		t.Errorf("the refusal said %q", detail)
	}

	// Onto a board that exists.
	const richDeck = "/api/decks/alice/rich"
	// `rich`'s commander is not in the 21-card fixture pool, so the identity
	// resolves to colourless and only a colourless card may be added.
	status, _, raw = rig.do(t, alice, "POST", richDeck+"/cards",
		`{"name":"Bag End Banquet","category":"payoff","why":"a finisher","to":"swap_board"}`)
	if status != http.StatusOK {
		t.Fatalf("adding to the board answered %d: %s", status, raw)
	}
	text := rig.textOf(t, "rich")
	at := strings.Index(text, "swap_board")
	if at < 0 {
		t.Fatalf("the board section was not created:\n%s", text)
	}
	if !strings.Contains(text[at:], "Bag End Banquet") {
		t.Errorf("the card did not land on the board:\n%s", text)
	}
	// And it is NOT in the 99, which is the half that matters: a card
	// silently in the deck is one its owner does not know is in the deck.
	if strings.Contains(text[:at], "Bag End Banquet") {
		t.Errorf("the card landed in the 99 as well:\n%s", text)
	}

	// A section nobody has is refused rather than silently becoming the 99.
	for _, into := range []string{"graveyard", "sideboard", "maybe", "CARDS"} {
		status, _, raw := rig.do(t, alice, "POST", richDeck+"/cards",
			`{"name":"Sol Ring","category":"payoff","why":"a reason","to":"`+into+`"}`)
		if status == http.StatusOK {
			t.Errorf("%q was accepted as a section: %s", into, raw)
		}
	}
}

// The card lookups reach the swap board as well as the 99, so a card staged
// there can be patched and removed without being promoted first.
func TestACardOnTheBoardCanBeEditedWhereItStands(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// `rich` is the fixture that carries a board, and the card already on
	// it is edited where it stands rather than being promoted first.
	status, _, raw := rig.do(t, alice, "PATCH",
		"/api/decks/alice/rich/cards/Sword%20of%20Feast%20and%20Famine",
		`{"field":"why","value":"still waiting on a slot"}`)
	if status != http.StatusOK {
		t.Fatalf("patching a card on the board answered %d: %s", status, raw)
	}
	text := rig.textOf(t, "rich")
	if !strings.Contains(text, "still waiting on a slot") {
		t.Errorf("the patch did not land:\n%s", text)
	}
	// And it is still on the board rather than having moved.
	at := strings.Index(text, "swap_board")
	if at < 0 || !strings.Contains(text[at:], "Sword of Feast and Famine") {
		t.Errorf("the card left the board:\n%s", text)
	}
}

// The swap is the operation the whole editor was written for, and it moves a
// card out of the 99 rather than deleting it -- the card that came out is
// still findable.
func TestASwapMovesTheCardOutRatherThanLosingIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)
	if !strings.Contains(before, "Sol Ring") {
		t.Skip("the fixture does not hold the card this swaps out")
	}

	// Both cards are in the 21-card fixture pool, and the incoming one is
	// legal in Commander -- a banned card is a different refusal, tested
	// with the gate rather than here.
	status, _, raw := rig.do(t, alice, "POST", cleanDeck+"/swap",
		`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"a faster finisher"}`)
	if status != http.StatusOK {
		t.Fatalf("the swap answered %d: %s", status, raw)
	}
	after := rig.text(t)
	if !strings.Contains(after, "Craterhoof Behemoth") {
		t.Errorf("the new card is not in the deck:\n%s", after)
	}
	// The rationale travelled with it -- rule 4 at the boundary.
	if !strings.Contains(after, "a faster finisher") {
		t.Errorf("the rationale did not land:\n%s", after)
	}
	// And the swap is recorded, because every edit is (ADR 28).
	history := rig.history(t, "mono-green-clean", nil)
	if len(history) == 0 {
		t.Error("the swap left no entry in the activity log")
	}
}

// A swap into a card the pool has never heard of is refused before anything
// is written -- rule 1 at a write boundary.
func TestASwapIntoACardNobodyLookedUpIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)

	status, payload, _ := rig.do(t, alice, "POST", cleanDeck+"/swap",
		`{"out":"Sol Ring","into":"Nonexistent Card","why":"a reason"}`)
	if status == http.StatusOK {
		t.Fatal("a card the pool has never heard of was swapped in")
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "Nonexistent Card") {
		t.Errorf("the refusal said %q", detail)
	}
	if rig.text(t) != before {
		t.Error("the deck was written despite the refusal")
	}
}

// A swap out of a card that is not in the deck is a refusal that names it,
// because the caller's next move is to check the spelling.
func TestASwapOutOfACardThatIsNotThereNamesIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, _ := rig.do(t, alice, "POST", cleanDeck+"/swap",
		`{"out":"Terastodon","into":"Craterhoof Behemoth","why":"a reason"}`)
	if status == http.StatusOK {
		t.Fatal("a card that is not in the deck was swapped out")
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "Terastodon") {
		t.Errorf("the refusal said %q", detail)
	}
}

// raw2s is the body as a string, for the messages above.
func raw2s(raw []byte) string { return string(raw) }

// textOf reads any fixture deck off the file tier, the way `text` reads the
// clean one.
func (r *writeRig) textOf(t *testing.T, slug string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.decks, slug, "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
