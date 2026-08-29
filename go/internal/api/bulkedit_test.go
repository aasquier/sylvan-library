package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// The bulk edit route, over the real fixture pool and the real file tier.
//
// `internal/deckedit` proves the diff and the bytes; what these prove is the
// layer above it -- that names resolve the way an import resolves them, that a
// preview writes nothing, that a confirm without a plan is refused, that a
// plan cannot land on a deck that moved under it, and that the whole pass
// leaves one honest entry in the activity log rather than ninety-nine.
//
// `mono-green-clean` is the fixture: Goreclaw in the command zone, Sol Ring,
// Regal Behemoth, Vorinclex, Cultivator Colossus and 95 Forests in the 99.

// bulkPost is one call to the route, with the JSON body assembled from a
// pasted list and whatever else the case is about.
func (r *writeRig) bulk(t *testing.T, list string, extra map[string]any) (int, map[string]any, []byte) {
	t.Helper()
	body := map[string]any{"text": list}
	for k, v := range extra {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return r.do(t, alice, "POST", cleanDeck+"/bulk", string(raw))
}

// preview runs the dry run and hands back the plan, failing the test if the
// route refused.
func (r *writeRig) preview(t *testing.T, list string) map[string]any {
	t.Helper()
	status, body, raw := r.bulk(t, list, map[string]any{"dry_run": true})
	if status != 200 {
		t.Fatalf("the preview was refused: %d %s", status, raw)
	}
	plan, ok := body["plan"].(map[string]any)
	if !ok {
		t.Fatalf("the answer carries no plan: %s", raw)
	}
	return plan
}

func planNames(t *testing.T, plan map[string]any, key string) []string {
	t.Helper()
	out := []string{}
	items, _ := plan[key].([]any)
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			name, _ := v["name"].(string)
			out = append(out, name)
		}
	}
	return out
}

// The whole 99 as the fixture holds it, so a case only has to say what it
// changes. Written the way a person pastes one, with the printing column an
// export writes and this route ignores.
const wholeFixtureList = `1 Sol Ring (LTC) 284
1 Regal Behemoth
1 Vorinclex, Voice of Hunger
1 Cultivator Colossus
95 Forest
`

// A preview writes nothing. The whole design rests on this being true rather
// than intended: the plan is computed by the code that will apply it, and the
// only difference between the two calls is the write.
func TestABulkPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.text(t)
	plan := rig.preview(t, "1 Sol Ring \"a brand new reason\"\n1 Llanowar Reborn \"a land\"\n")
	if len(planNames(t, plan, "entomb")) == 0 {
		t.Fatal("a paste naming two cards planned no burials, so this proves nothing")
	}
	if rig.text(t) != before {
		t.Error("the preview wrote to the deck file")
	}
	if len(rig.history(t, "mono-green-clean", nil)) != 0 {
		t.Error("the preview left an entry in the activity log")
	}
}

// The feature, end to end: the descriptions, in bulk, in the player's own
// words, with the cards the list does not name going to the graveyard.
func TestABulkEditRewritesTheReasonsAndEntombsWhatIsLeftOut(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	list := `1 Sol Ring "Two mana, turn one, every deck, forever."
1 Regal Behemoth
1 Vorinclex, Voice of Hunger "Taxes them and doubles me."
95 Forest
1 Llanowar Reborn "A land that grows a counter."
`
	plan := rig.preview(t, list)
	if got := planNames(t, plan, "rewrite"); len(got) != 2 {
		t.Fatalf("the rewrites were %v", got)
	}
	if got := planNames(t, plan, "add"); len(got) != 1 || got[0] != "Llanowar Reborn" {
		t.Fatalf("the additions were %v", got)
	}
	if got := planNames(t, plan, "entomb"); len(got) != 1 || got[0] != "Cultivator Colossus" {
		t.Fatalf("the burials were %v", got)
	}
	if ready, _ := plan["basis"].(string); ready == "" {
		t.Fatal("the plan carries nothing to confirm against")
	}

	status, body, raw := rig.bulk(t, list, map[string]any{"basis": plan["basis"]})
	if status != 200 {
		t.Fatalf("the edit was refused: %d %s", status, raw)
	}
	// The gate reports rather than blocking, and its verdict rides back.
	if _, has := body["errors"]; !has {
		t.Errorf("the answer carries no gate verdict: %s", raw)
	}

	after := rig.text(t)
	if !strings.Contains(after, "why: Two mana, turn one, every deck, forever.") {
		t.Errorf("the pasted reason did not land:\n%s", after)
	}
	if !strings.Contains(after, "name: Llanowar Reborn") {
		t.Errorf("the new card did not land:\n%s", after)
	}
	// Entombed, not deleted (Aaron's ruling): the card is in the graveyard
	// with the reason it was played for, one control away from coming back.
	grave := after[strings.Index(after, "graveyard:"):]
	if !strings.Contains(grave, "name: Cultivator Colossus") {
		t.Errorf("the card left out of the list did not reach the graveyard:\n%s", after)
	}
	if !strings.Contains(grave, "why: Trample body") {
		t.Errorf("the buried card lost the reason it was played for:\n%s", after)
	}
}

// ADR 28: one entry for the pass, naming what it buried and counting the rest.
// Ninety-nine rows would bury the history somebody comes here to read.
func TestABulkEditLeavesOneHonestEntryInTheLog(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	list := `1 Sol Ring "a new reason"
1 Regal Behemoth "another new reason"
1 Vorinclex, Voice of Hunger
95 Forest
`
	plan := rig.preview(t, list)
	status, _, raw := rig.bulk(t, list, map[string]any{"basis": plan["basis"]})
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}

	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 1 {
		t.Fatalf("%d entries for one pass; the history is unreadable at that "+
			"rate: %+v", len(entries), entries)
	}
	if entries[0].Action != "bulk" {
		t.Errorf("the row is filed under %q", entries[0].Action)
	}
	if !strings.Contains(entries[0].Summary, "Cultivator Colossus") {
		t.Errorf("the entry does not say where the card went: %q", entries[0].Summary)
	}
	if !strings.Contains(entries[0].Summary, "2 reasons rewritten") {
		t.Errorf("the entry does not count the pass: %q", entries[0].Summary)
	}
	// ADR 28's oldest rule. The log says a rationale changed and never what
	// it says.
	if strings.Contains(entries[0].Summary, "a new reason") {
		t.Errorf("a rationale reached the activity log: %q", entries[0].Summary)
	}
}

// The stale-plan guard, driven the way it actually happens: somebody edits in
// another tab between looking at the plan and agreeing to it.
func TestAPlanIsRefusedWhenTheDeckMovedUnderIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	plan := rig.preview(t, wholeFixtureList+"1 Llanowar Reborn \"a land\"\n")

	// The other tab. An ordinary single-card edit, through an ordinary route.
	status, _, raw := rig.do(t, alice, "PATCH", cleanDeck+"/cards/Sol%20Ring",
		`{"field":"why","value":"Somebody else wrote this."}`)
	if status != 200 {
		t.Fatalf("the interfering edit failed: %d %s", status, raw)
	}
	before := rig.text(t)

	status, _, raw = rig.bulk(t, wholeFixtureList+"1 Llanowar Reborn \"a land\"\n",
		map[string]any{"basis": plan["basis"]})
	if status != 422 {
		t.Fatalf("a stale plan answered %d %s", status, raw)
	}
	if !strings.Contains(string(raw), "changed while the plan was on screen") {
		t.Errorf("the refusal does not say what happened: %s", raw)
	}
	if rig.text(t) != before {
		t.Error("a refused bulk edit wrote to the deck file")
	}

	// And the recovery is honest: a fresh look lands.
	fresh := rig.preview(t, wholeFixtureList+"1 Llanowar Reborn \"a land\"\n")
	status, _, raw = rig.bulk(t, wholeFixtureList+"1 Llanowar Reborn \"a land\"\n",
		map[string]any{"basis": fresh["basis"]})
	if status != 200 {
		t.Fatalf("the re-read plan was refused too: %d %s", status, raw)
	}
}

// Confirming without ever having looked is refused. This route is reachable
// without a browser, so "somebody saw the plan" has to be a fact about the
// protocol rather than a promise about the UI.
func TestAConfirmWithNoPlanBehindItIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.text(t)
	status, _, raw := rig.bulk(t, "1 Sol Ring \"mine now\"\n", nil)
	if status != 422 {
		t.Fatalf("an unconfirmed bulk edit answered %d %s", status, raw)
	}
	if !strings.Contains(string(raw), "take a look") {
		t.Errorf("the refusal said %s", raw)
	}
	if rig.text(t) != before {
		t.Error("the refused edit wrote to the deck file")
	}
}

// An empty box would send every card in the deck to the graveyard. That is a
// real operation nobody performs on purpose by pressing a button next to an
// empty textarea.
func TestAnEmptyListIsRefusedRatherThanEmptyingTheDeck(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, empty := range []string{"", "   \n\n", "# just a comment\n", "Commander\n"} {
		status, _, raw := rig.bulk(t, empty, map[string]any{"dry_run": true})
		if status != 422 {
			t.Errorf("an empty list (%q) answered %d %s", empty, status, raw)
			continue
		}
		if !strings.Contains(string(raw), "graveyard") {
			t.Errorf("the refusal does not say what was avoided: %s", raw)
		}
	}
}

// A misspelling is read, not guessed -- the import's own resolver, at the
// thresholds it measured. The correction is reported by name, because that is
// the other half of being allowed to correct at all.
func TestAMisspelledNameIsReadAndSaidSo(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.bulk(t, "1 Sol Rng \"still the same card\"\n1 Regal Behemoth\n"+
		"1 Vorinclex, Voice of Hunger\n1 Cultivator Colossus\n95 Forest\n",
		map[string]any{"dry_run": true})
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	read, _ := body["read"].([]any)
	if len(read) != 1 {
		t.Fatalf("the correction was not reported: %s", raw)
	}
	first, _ := read[0].(map[string]any)
	if first["written"] != "Sol Rng" || first["read"] != "Sol Ring" {
		t.Errorf("the correction was reported as %+v", first)
	}
	// And the read card is a card: it is matched against the deck rather than
	// buried and re-added under a name nobody typed.
	plan, _ := body["plan"].(map[string]any)
	if got := planNames(t, plan, "entomb"); len(got) != 0 {
		t.Errorf("a corrected name still buried something: %v", got)
	}
	if got := planNames(t, plan, "rewrite"); len(got) != 1 || got[0] != "Sol Ring" {
		t.Errorf("the corrected card's reason was handled as %v", got)
	}
}

// A name the pool will not read is reported with a shortlist and never
// applied -- and the preview is where somebody sees it *beside* the card it
// was probably meant to be, which is the whole reason there is a preview.
func TestAnUnreadableNameIsAskedAboutRatherThanPickedFor(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.bulk(t,
		"1 Sol Ring\n1 Regal Behemoth\n1 Vorinclex, Voice of Hunger\n"+
			// A name no pool holds, and beneath it a line that is nothing but
			// a printing column -- the parser leaves no name behind, so it is
			// unreadable rather than unknown, and the two are reported apart.
			"1 Cultivator Colossus\n95 Forest\n1 Qqzzxx Whatnot\n(LTC) 284\n",
		map[string]any{"dry_run": true})
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	unknown, _ := body["unknown"].([]any)
	if len(unknown) != 1 || unknown[0] != "Qqzzxx Whatnot" {
		t.Fatalf("the unresolved name was reported as %v", unknown)
	}
	plan, _ := body["plan"].(map[string]any)
	if got := planNames(t, plan, "add"); len(got) != 0 {
		t.Errorf("a name the pool does not know was added anyway: %v", got)
	}
	// The line that is not a card at all, reported rather than swallowed.
	if lines, _ := body["unreadable"].([]any); len(lines) == 0 {
		t.Errorf("an unreadable line vanished: %s", raw)
	}
}

// The Moxfield shape: the commander travels under `SIDEBOARD:` and must never
// be added to the 99 or buried out of the command zone.
func TestTheCommanderInAPastedExportIsReportedAndNeverApplied(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.bulk(t, wholeFixtureList+
		"\nSIDEBOARD:\n1 Goreclaw, Terror of Qal Sisma\n",
		map[string]any{"dry_run": true})
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	outside, _ := body["outside"].([]any)
	if len(outside) != 1 {
		t.Fatalf("the sideboard line was not reported: %s", raw)
	}
	plan, _ := body["plan"].(map[string]any)
	if got := planNames(t, plan, "add"); len(got) != 0 {
		t.Errorf("a line outside the 99 was applied to it: %v", got)
	}
	if ready, _ := body["ready"].(bool); ready {
		t.Errorf("a list that matches the deck offered a confirm: %s", raw)
	}
}

// Rule 4 at the boundary: a curated deck refuses a card with no reason, and
// says exactly what to type. All or nothing, because half a paste applied
// leaves a deck nobody chose.
func TestACuratedDeckRefusesANewCardWithNoReasonAndSaysHow(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	list := wholeFixtureList + "1 Llanowar Reborn\n"
	plan := rig.preview(t, list)
	blocked, _ := plan["blocked"].([]any)
	if len(blocked) != 1 {
		t.Fatalf("the unreasoned card was not blocked: %+v", plan)
	}
	first, _ := blocked[0].(map[string]any)
	if reason, _ := first["reason"].(string); !strings.Contains(reason, "quotes") {
		t.Errorf("the refusal does not say how to fix it: %q", reason)
	}

	before := rig.text(t)
	status, _, raw := rig.bulk(t, list, map[string]any{"basis": plan["basis"]})
	if status != 422 {
		t.Fatalf("a blocked plan answered %d %s", status, raw)
	}
	if rig.text(t) != before {
		t.Error("a blocked plan wrote to the deck file anyway")
	}
}

// The route inherits `writeTarget`'s refusals rather than repeating them: a
// deck somebody may read but not write is a 403, and the refusal lands before
// the body is looked at, so a reader never learns whether their paste parsed.
//
// Not a 404, and the difference is the one ADR 5 draws: bob can *see* the
// shared library deck, so its existence is not a secret. A 404 is for a deck
// that is absent from his source altogether.
func TestBulkEditingADeckYouMayOnlyReadIsRefusedBeforeTheBody(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.text(t)
	// A body that could not possibly be applied. If the refusal came from
	// anywhere but the preamble, the sentence would be about the paste.
	status, _, raw := rig.do(t, bob, "POST", cleanDeck+"/bulk", `{"text":""}`)
	if status != 403 {
		t.Fatalf("bob reached alice's deck with %d %s", status, raw)
	}
	if !strings.Contains(string(raw), "not yours to change") {
		t.Errorf("the refusal is about the paste rather than the deck: %s", raw)
	}
	if rig.text(t) != before {
		t.Error("a read-only caller changed the deck")
	}
}
