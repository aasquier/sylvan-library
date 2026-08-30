package api

import (
	"net/http"
	"strings"
	"testing"
)

// `PUT .../combos`: the deck's catalogue of machines, written whole.
//
// What `internal/deckedit` already proves is the bytes; what these prove is the
// layer above it -- that the block reaches the file, that the names are read
// against the pool before they are written, that the block comes back on the
// deck payload with a picture for every card it names, and that a deck somebody
// cannot see is a 404 to them whatever they PUT at it.

// One machine out of the fixture's own cards, spelled the way somebody types
// it at midnight: the pool's canonical names are what should end up in the
// file. One piece is in the 99 and one is in the command zone, which is the
// pair a client working `in_deck` out for itself would get wrong.
const wallCombo = `{"combos":[{` +
	`"cards":["sol ring","GORECLAW, TERROR OF QAL SISMA"],` +
	`"produces":"a turn-two commander and a cheaper board after it",` +
	`"how":"1) Sol Ring on one. 2) Goreclaw on two. 3) Everything after costs {2} less.",` +
	`"setup":"two lands and the ring surviving a turn"}]}`

func TestCataloguingAMachineWritesItIntoTheDeckFile(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos", wallCombo)
	if status != http.StatusOK {
		t.Fatalf("cataloguing answered %d: %s", status, raw)
	}
	// `commit`'s envelope, plus this operation's own key.
	for _, key := range []string{"slug", "combos", "stage", "total_cards",
		"needs_rationale", "ok", "errors", "warnings"} {
		if _, present := body[key]; !present {
			t.Errorf("the response lacks %q: %v", key, body)
		}
	}
	if n, _ := body["combos"].(float64); n != 1 {
		t.Errorf("the response says %v combos, not 1", body["combos"])
	}

	after := rig.text(t)
	if !strings.Contains(after, "\ncombos:") {
		t.Fatalf("no combos block reached the file:\n%s", after)
	}
	// **The names are the pool's, not the typing's.** A hover keys on the exact
	// name, so `sol ring` in the file would be a card with no picture.
	if !strings.Contains(after, "- Sol Ring") ||
		!strings.Contains(after, "- Goreclaw, Terror of Qal Sisma") {
		t.Errorf("the names were written as typed rather than as printed:\n%s", after)
	}
	if strings.Contains(after, "sol ring") {
		t.Errorf("the typed spelling survived into the deck file:\n%s", after)
	}

	// ADR 28: one entry for the block, counting rather than quoting.
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 1 {
		t.Fatalf("the history has %d entries, expected 1: %+v", len(entries), entries)
	}
	if entries[0].Action != "combos" || entries[0].Summary != "catalogued 1 combo" {
		t.Errorf("recorded %q / %q", entries[0].Action, entries[0].Summary)
	}
	// The log never carries what a combo says, only that one was catalogued.
	if strings.Contains(entries[0].Summary, "Elves") {
		t.Errorf("the history quoted the entry: %q", entries[0].Summary)
	}
}

// The block comes back on the deck payload with every name resolved, so the
// page can hover a card without a second fetch.
func TestTheDeckPayloadCarriesTheCatalogueResolved(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	if status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos", wallCombo); status != http.StatusOK {
		t.Fatalf("cataloguing answered %d: %s", status, raw)
	}
	status, payload, raw := rig.do(t, alice, "GET", cleanDeck, "")
	if status != http.StatusOK {
		t.Fatalf("reading the deck answered %d: %s", status, raw)
	}
	combos, ok := payload["combos"].([]any)
	if !ok || len(combos) != 1 {
		t.Fatalf("the payload does not carry the catalogue: %s", raw)
	}
	entry, _ := combos[0].(map[string]any)
	for _, key := range []string{"cards", "produces", "how", "setup", "needs", "cut"} {
		if _, present := entry[key]; !present {
			t.Errorf("the row lacks %q: %v", key, entry)
		}
	}
	// A machine that assembles has no trade, and says so with a null rather
	// than by leaving the keys out -- the page renders the difference.
	if entry["needs"] != nil || entry["cut"] != nil {
		t.Errorf("a complete machine came back with a trade: %v", entry)
	}
	// Nothing drafted this, so there is no mark at all -- an empty string would
	// be a claim where an absence is meant.
	if _, marked := entry["by"]; marked {
		t.Errorf("an entry nobody drafted carries a provenance mark: %v", entry)
	}

	cards, _ := entry["cards"].([]any)
	if len(cards) != 2 {
		t.Fatalf("the entry names %d cards, expected 2: %v", len(cards), entry)
	}
	for _, item := range cards {
		ref, _ := item.(map[string]any)
		for _, key := range []string{"name", "image", "art_crop", "in_deck"} {
			if _, present := ref[key]; !present {
				t.Errorf("a card reference lacks %q: %v", key, ref)
			}
		}
		// Both of these are in this deck -- one in the 99, one in the command
		// zone, which is the case a client working it out for itself gets wrong.
		if in, _ := ref["in_deck"].(bool); !in {
			t.Errorf("%v is in the deck and the payload says otherwise", ref["name"])
		}
	}
}

// The commander counts as a card the deck has. Half the combos in Commander
// run through the command zone, and it is not in `cards`.
func TestTheCommanderReadsAsACardTheDeckHas(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos",
		`{"combos":[{"cards":["Goreclaw, Terror of Qal Sisma"],`+
			`"produces":"a discount on every big creature"}]}`)
	if status != http.StatusOK {
		t.Fatalf("cataloguing answered %d: %s", status, raw)
	}
	_, payload, raw := rig.do(t, alice, "GET", cleanDeck, "")
	combos, _ := payload["combos"].([]any)
	entry, _ := combos[0].(map[string]any)
	cards, _ := entry["cards"].([]any)
	ref, _ := cards[0].(map[string]any)
	if in, _ := ref["in_deck"].(bool); !in {
		t.Errorf("the commander reads as a card the deck does not have: %s", raw)
	}
	// And the gate is quiet about it, rather than warning that a piece is
	// missing from the 99.
	warnings, _ := payload["warnings"].([]any)
	for _, item := range warnings {
		issue, _ := item.(map[string]any)
		if code, _ := issue["code"].(string); strings.HasPrefix(code, "combo-") {
			t.Errorf("the gate warned about a combo through the commander: %v", issue)
		}
	}
}

// A near-miss carries its trade through the whole stack, and the gate says
// nothing about it: naming the cut is what makes the suggestion actionable.
func TestANearMissKeepsItsTradeThroughTheStack(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos",
		`{"combos":[{"cards":["Goreclaw, Terror of Qal Sisma"],`+
			`"needs":"Craterhoof Behemoth","cut":"Cultivator Colossus",`+
			`"produces":"a discount on the Behemoth and a lethal swing"}]}`)
	if status != http.StatusOK {
		t.Fatalf("cataloguing a near-miss answered %d: %s", status, raw)
	}
	if file := rig.text(t); !strings.Contains(file, "needs: Craterhoof Behemoth") ||
		!strings.Contains(file, "cut: Cultivator Colossus") {
		t.Errorf("the trade did not reach the file:\n%s", file)
	}

	_, payload, raw := rig.do(t, alice, "GET", cleanDeck, "")
	combos, _ := payload["combos"].([]any)
	entry, _ := combos[0].(map[string]any)
	needs, _ := entry["needs"].(map[string]any)
	cut, _ := entry["cut"].(map[string]any)
	if needs == nil || cut == nil {
		t.Fatalf("the near-miss lost its trade on the wire: %s", raw)
	}
	// The card it wants is not in the deck; the card it would cut is. That
	// pair is the whole shape of a near-miss.
	if in, _ := needs["in_deck"].(bool); in {
		t.Errorf("the card it is waiting for reads as already sleeved: %v", needs)
	}
	if in, _ := cut["in_deck"].(bool); !in {
		t.Errorf("the card it would cut is not in the deck: %v", cut)
	}
}

// Emptying the shelf is a real edit with its own sentence in the history --
// "catalogued 0 combos" is a score rather than an event.
func TestEmptyingTheCatalogueIsSaidAsItsOwnThing(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	if status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos", wallCombo); status != http.StatusOK {
		t.Fatalf("cataloguing answered %d: %s", status, raw)
	}
	status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos", `{"combos":[]}`)
	if status != http.StatusOK {
		t.Fatalf("emptying answered %d: %s", status, raw)
	}
	if file := rig.text(t); strings.Contains(file, "combos:") {
		t.Errorf("the emptied block is still asserted:\n%s", file)
	}
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 2 || entries[0].Summary != "cleared the combos" {
		t.Errorf("the emptying was recorded as %+v", entries)
	}
}

// The body's guards. Each of these is a shape a script reaches on purpose and
// a stale client reaches by accident, and every one of them refuses without
// writing anything.
func TestAMalformedCatalogueIsRefusedWithoutWriting(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)

	for _, tc := range []struct{ name, body, says string }{
		{"no list at all", `{}`, "combos is required"},
		{"not a list", `{"combos":"Axebane Guardian + High Alert"}`, "combos must be a list"},
		{"an entry that is not a mapping", `{"combos":["a combo"]}`, "not a mapping"},
		{"an entry with no pieces", `{"combos":[{"produces":"mana"}]}`, "at least one piece"},
		{"an entry that produces nothing",
			`{"combos":[{"cards":["Sol Ring"]}]}`, "say what it produces"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos", tc.body)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("%s answered %d: %s", tc.name, status, raw)
			}
			if !strings.Contains(string(raw), tc.says) {
				t.Errorf("the refusal does not say %q: %s", tc.says, raw)
			}
		})
	}
	if rig.text(t) != before {
		t.Error("a refused catalogue changed the deck file")
	}
}

// **A caller cannot claim Claude wrote their entry.** The mark is provenance
// (ADR 41), so it comes from the deck file or from nowhere.
func TestACallerCannotForgeTheDraftedMark(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos",
		`{"combos":[{"cards":["Sol Ring"],"produces":"two colourless","by":"claude"}]}`)
	if status != http.StatusOK {
		t.Fatalf("cataloguing answered %d: %s", status, raw)
	}
	if file := rig.text(t); strings.Contains(file, "by:") {
		t.Errorf("a caller forged a provenance mark into the deck file:\n%s", file)
	}
	_, payload, _ := rig.do(t, alice, "GET", cleanDeck, "")
	combos, _ := payload["combos"].([]any)
	entry, _ := combos[0].(map[string]any)
	if _, marked := entry["by"]; marked {
		t.Errorf("the forged mark reached the page: %v", entry)
	}
}

// A name the pool does not know is written as typed and warned about, never
// refused: refusing would be this route deciding that a card the local pool has
// not been refreshed for does not exist.
func TestAnUnknownNameIsCataloguedAndWarnedAbout(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos",
		`{"combos":[{"cards":["Sol Ring","Sporeback Wossname"],`+
			`"produces":"something the pool has never heard of"}]}`)
	if status != http.StatusOK {
		t.Fatalf("a card the pool lacks was refused: %d %s", status, raw)
	}
	if file := rig.text(t); !strings.Contains(file, "Sporeback Wossname") {
		t.Errorf("the name was not written as typed:\n%s", file)
	}
	warnings, _ := body["warnings"].([]any)
	found := ""
	for _, item := range warnings {
		issue, _ := item.(map[string]any)
		if code, _ := issue["code"].(string); code == "combo-unknown-card" {
			found, _ = issue["card"].(string)
		}
	}
	if found != "Sporeback Wossname" {
		t.Errorf("the gate did not name the unresolvable card: %s", raw)
	}
	// And it stayed a warning: an invalid deck is diagnosed, not refused.
	if ok, _ := body["ok"].(bool); !ok {
		errs, _ := body["errors"].([]any)
		for _, item := range errs {
			issue, _ := item.(map[string]any)
			if code, _ := issue["code"].(string); strings.HasPrefix(code, "combo-") {
				t.Errorf("a combos issue was raised as an error: %v", issue)
			}
		}
	}
}

// Who may catalogue: the deck's owner, and nobody else.
//
// The two refusals are the write surface's own pair and the distinction is
// deliberate (ADR 5, ADR 22). A deck somebody cannot **see** is absent from
// their library, so it is a **404** -- a 403 there would confirm to a stranger
// that the deck exists. A deck they can see but not write is a **403**: the
// existence is not the secret, and the request is not malformed. The fixture is
// shared, so bob is the second case; a deck that is not there at all is the
// first, and both are asked here rather than one being assumed from the other.
func TestOnlyTheOwnerMayCatalogue(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)

	status, _, raw := rig.do(t, bob, "PUT", cleanDeck+"/combos", wallCombo)
	if status != http.StatusForbidden {
		t.Fatalf("another account's write answered %d, expected 403: %s", status, raw)
	}
	if rig.text(t) != before {
		t.Error("another account wrote into the deck")
	}

	// A deck nobody can see is absent, on this route as on every other.
	status, _, raw = rig.do(t, bob, "PUT", "/api/decks/alice/no-such-deck/combos", wallCombo)
	if status != http.StatusNotFound {
		t.Errorf("a deck that is not there answered %d, expected 404: %s", status, raw)
	}

	// And the owner is unaffected -- the same request, from the person the
	// deck belongs to, works.
	if status, _, raw := rig.do(t, alice, "PUT", cleanDeck+"/combos", wallCombo); status != http.StatusOK {
		t.Fatalf("the owner was refused too: %d %s", status, raw)
	}
}
