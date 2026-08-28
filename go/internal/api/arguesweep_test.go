package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The argue sweep. `internal/claude` holds the mode itself to a corpus; what
// these prove is the job around it -- which refusal lands on which status and
// BEFORE any job, that the selection is normalised the recorded way,
// that a failed card does not cost the rest of the sweep, and that the six
// keys come back in the recorded order with the errors in SWEEP order.
//
// Every refusal below was diffed against the live wire rather than
// read off the source, which is how the deck-before-stance ordering got
// pinned: both wrong at once answers the deck's.

const sweepAt = "/api/decks/alice/kaheera/argue/deck"

// The three cards `kaheera.yaml` actually holds, in file order.
var sweepCards = []string{"Sol Ring", "Regal Behemoth", "Vorinclex, Voice of Hunger"}

func sweep(t *testing.T, rig *jobRig, body string) (int, map[string]any, []byte) {
	t.Helper()
	return callAs(t, rig.api, alice, "POST", sweepAt, body)
}

// ---- what is refused, and in what order ---------------------------------

func TestTheSweepRefusesWhatItCanBeforeAnyJob(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	for _, row := range []struct {
		note, body string
		status     int
		detail     string
	}{
		{"no cards key", `{}`, 422, "cards must be a list of card names"},
		{"cards is a string", `{"cards":"Sol Ring"}`, 422, "cards must be a list of card names"},
		{"cards is an object", `{"cards":{"a":1}}`, 422, "cards must be a list of card names"},
		// `all(isinstance(c, str))`: one number spoils the list, and it is the
		// SAME refusal as "that is not a list" rather than a second sentence.
		{"a number among them", `{"cards":["Sol Ring",7]}`, 422, "cards must be a list of card names"},
		{"a null among them", `{"cards":["Sol Ring",null]}`, 422, "cards must be a list of card names"},
		{"an empty list", `{"cards":[]}`, 422, "no cards selected"},
		{"only whitespace", `{"cards":["  ",""]}`, 422, "no cards selected"},
		{"not in this deck", `{"cards":["Black Lotus"]}`, 422, "not in this deck: Black Lotus"},
		// Named and not counted: which card is the actionable part, and the
		// deck page sent these, so a mismatch means its list is stale.
		{"two missing, both named", `{"cards":["Black Lotus","Time Walk"]}`, 422,
			"not in this deck: Black Lotus, Time Walk"},
		{"a bad stance", `{"cards":["Sol Ring"],"stance":"garbage"}`, 422, "is not a stance preset"},
		{"no key", `{"cards":["Sol Ring"]}`, 503, "no ANTHROPIC_API_KEY"},
	} {
		status, payload, raw := sweep(t, rig, row.body)
		if status != row.status {
			t.Errorf("%s answered %d, want %d: %s", row.note, status, row.status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, row.detail) {
			t.Errorf("%s said %q, want it to contain %q", row.note, detail, row.detail)
		}
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by refused requests", len(got))
	}
}

// The selection is checked **before** the stance, so both-wrong answers the
// deck's. Measured on the live wire; the natural Go shape
// resolves the stance while it has the body open and answers the other way.
func TestTheSelectionIsRefusedBeforeTheStance(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	status, payload, raw := sweep(t, rig, `{"cards":["Black Lotus"],"stance":"garbage"}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	detail, _ := payload["detail"].(string)
	if !strings.Contains(detail, "Black Lotus") {
		t.Errorf("said %q, want the missing card -- it is checked first", detail)
	}
	if strings.Contains(detail, "stance") {
		t.Errorf("the stance was checked before the selection: %q", detail)
	}
	// And a deck this caller cannot see is a 404 before either (ADR 5).
	if status, _, _ := callAs(t, rig.api, alice, "POST",
		"/api/decks/bob/bobs-private/argue/deck", `{"cards":["x"],"stance":"garbage"}`); status != 404 {
		t.Errorf("a private deck answered %d, want 404", status)
	}
}

// ---- what a run answers with --------------------------------------------

// A stance of `off` is a job born finished, carrying ONE report for the sweep
// rather than N copies of "no call was made" -- and the six keys in the
// recorded order.
func TestASweepAtStanceOffIsAJobBornFinished(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	status, payload, raw := sweep(t, rig,
		fmt.Sprintf(`{"cards":["%s","%s"],"stance":"off"}`, sweepCards[0], sweepCards[1]))
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["status"] != "done" {
		t.Errorf("status is %v, want a job born done", payload["status"])
	}
	if payload["label"] != "argue: 2 slots of kaheera" {
		t.Errorf("label is %v", payload["label"])
	}
	result, _ := payload["result"].(map[string]any)
	if result["asked"] != false {
		t.Errorf("asked is %v, want false", result["asked"])
	}
	if !strings.HasPrefix(result["reason"].(string), "The stance is off") {
		t.Errorf("reason is %v", result["reason"])
	}
	if result["total"] != float64(2) {
		t.Errorf("total is %v, want 2", result["total"])
	}
	if reports, _ := result["reports"].([]any); len(reports) != 0 {
		t.Errorf("reports is %v, want []", reports)
	}
	if got := resultKeys(t, raw); got != "slug,asked,reason,total,reports,errors" {
		t.Errorf("result keys are %s, the record says slug,asked,reason,total,reports,errors", got)
	}
	// `[]` and `{}`, not two nulls: a client that iterates must not meet one.
	if !strings.Contains(string(raw), `"reports":[],"errors":{}`) {
		t.Errorf("the empty collections did not render as [] and {}: %s", raw)
	}
}

// One slot reads "1 slot", not "1 slots".
func TestTheSweepLabelCountsInEnglish(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	_, payload, _ := sweep(t, rig, fmt.Sprintf(`{"cards":["%s"],"stance":"off"}`, sweepCards[0]))
	if payload["label"] != "argue: 1 slot of kaheera" {
		t.Errorf("label is %v, want `argue: 1 slot of kaheera`", payload["label"])
	}
}

// The selection is normalised the recorded way: matched by
// casefold, stored in the DECK's spelling, stripped, de-duplicated, and in the
// order it was asked for.
//
// `total` is the observable: the reports come back in this order too, but the
// count is what separates "de-duplicated" from "not".
//
// **One half of this is unobservable here, and is recorded rather than
// arranged.** The match is `claude.Casefold`,
// but no card in any fixture deck holds a character where
// `casefold` and `lower` disagree -- it takes an `ß`, an `ſ` or a `ς`, and
// Magic's English names have none. So a `ToLower` mutation survives this
// test. It is still wrong: this is the one comparison in the family where the
// two sides are DIFFERENT strings -- a name somebody typed against the deck's
// own spelling -- which is exactly the case CLAUDE.md says the neighbours are
// exempt from. Manufacturing a fixture card for it would be arranging the
// evidence; naming the gap is the honest version.
func TestTheSelectionIsNormalisedTheRecordedWay(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	for _, row := range []struct {
		note, cards string
		total       int
	}{
		{"as written", `["Sol Ring"]`, 1},
		{"lowercased", `["sol ring"]`, 1},
		{"uppercased", `["SOL RING"]`, 1},
		{"padded", `["  Sol Ring  "]`, 1},
		// The same card twice, spelled two ways, is one slot.
		{"duplicated across spellings", `["Sol Ring","sol ring"]`, 1},
		{"a blank among the real", `["Sol Ring","  "]`, 1},
		{"two distinct", `["Sol Ring","Regal Behemoth"]`, 2},
		{"two distinct, one duplicated", `["Sol Ring","Regal Behemoth","SOL RING"]`, 2},
		// **`textutil.Strip` and not `strings.TrimSpace`.** U+001C-U+001F
		// are whitespace to the recorded strip and not to Go's, so a name
		// wrapped in them
		// strips clean under one and stays unmatched under the other -- a
		// 422 saying the card is not in a deck that holds it. This row is
		// what kills that mutation; without it the reflex spelling passes.
		{"the information separators strip", `["\u001cSol Ring\u001f"]`, 1},
	} {
		_, payload, raw := sweep(t, rig, `{"cards":`+row.cards+`,"stance":"off"}`)
		result, _ := payload["result"].(map[string]any)
		if result == nil {
			t.Errorf("%s: no result: %s", row.note, raw)
			continue
		}
		if result["total"] != float64(row.total) {
			t.Errorf("%s: total is %v, want %d", row.note, result["total"], row.total)
		}
	}
}

// A whole sweep: one call per card, the reports in selection order, and the
// progress the bar is drawn from.
func TestASweepArguesEachSlotInTurn(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(charges("Sol Ring", "It is colourless."))),
		answer("end_turn", said(charges("Regal Behemoth", "It costs six."))),
	}}
	rig.api.claude = script.start(t)

	status, payload, raw := sweep(t, rig,
		fmt.Sprintf(`{"cards":["%s","%s"]}`, sweepCards[0], sweepCards[1]))
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	done, doneRaw := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("job %v: %s", done["status"], doneRaw)
	}
	result, _ := done["result"].(map[string]any)
	if result["asked"] != true {
		t.Errorf("asked is %v, want true", result["asked"])
	}
	if result["reason"] != "" {
		t.Errorf("reason is %q, want empty on a real run", result["reason"])
	}
	reports, _ := result["reports"].([]any)
	if len(reports) != 2 {
		t.Fatalf("%d reports, want 2: %s", len(reports), doneRaw)
	}
	// **In selection order**, which is the order the user picked them.
	for i, want := range []string{sweepCards[0], sweepCards[1]} {
		report, _ := reports[i].(map[string]any)
		if report["card"] != want {
			t.Errorf("report %d is about %v, want %s", i, report["card"], want)
		}
	}
	if errs, _ := result["errors"].(map[string]any); len(errs) != 0 {
		t.Errorf("errors is %v, want empty", errs)
	}
	// The progress the bar is drawn from: done == total when it finishes.
	if done["done"] != done["total"] {
		t.Errorf("progress ended at %v/%v", done["done"], done["total"])
	}
}

// **A failed card is recorded and the sweep continues**, because partial
// results are the point of paying for a sweep -- one flaky call must not cost
// the other answers.
func TestOneFailedCardDoesNotCostTheSweep(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(charges("Sol Ring", "It is colourless."))),
		"!401", // the middle card fails
		answer("end_turn", said(charges("Vorinclex, Voice of Hunger", "It costs eight."))),
	}}
	claudeSet := script.start(t)
	rig.api.claude = claudeSet

	status, payload, raw := sweep(t, rig, fmt.Sprintf(`{"cards":["%s","%s","%s"]}`,
		sweepCards[0], sweepCards[1], sweepCards[2]))
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	done, doneRaw := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the sweep itself failed: %v %s", done["status"], doneRaw)
	}
	result, _ := done["result"].(map[string]any)
	reports, _ := result["reports"].([]any)
	if len(reports) != 2 {
		t.Errorf("%d reports, want the two that worked: %s", len(reports), doneRaw)
	}
	errs, _ := result["errors"].(map[string]any)
	if len(errs) != 1 {
		t.Fatalf("%d errors, want 1: %v", len(errs), errs)
	}
	if _, named := errs[sweepCards[1]]; !named {
		t.Errorf("the error is not keyed on the card that failed: %v", errs)
	}
	// `total` is the SELECTION, not the successes -- three were paid for.
	if result["total"] != float64(3) {
		t.Errorf("total is %v, want 3", result["total"])
	}
}

// The errors dict is built in **sweep order** and the wire keeps it. A
// `map[string]string` would alphabetise a list whose order is the order things
// went wrong in.
func TestTheErrorsKeepSweepOrder(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	// Vorinclex sorts LAST alphabetically and fails FIRST here, so the two
	// orders disagree and the test can tell them apart.
	script := &scriptedClaude{replies: []string{"!401", "!401"}}
	claudeSet := script.start(t)
	rig.api.claude = claudeSet
	status, payload, _ := sweep(t, rig,
		fmt.Sprintf(`{"cards":["%s","%s"]}`, sweepCards[2], sweepCards[0]))
	if status != 200 {
		t.Fatal(status)
	}
	_, doneRaw := rig.await(t, payload["id"].(string))
	var envelope struct {
		Result struct {
			Errors json.RawMessage `json:"errors"`
		} `json:"result"`
	}
	if err := json.Unmarshal(doneRaw, &envelope); err != nil {
		t.Fatal(err)
	}
	body := string(envelope.Result.Errors)
	firstAt := strings.Index(body, sweepCards[2])
	secondAt := strings.Index(body, sweepCards[0])
	if firstAt < 0 || secondAt < 0 {
		t.Fatalf("both cards should be in the errors: %s", body)
	}
	if firstAt > secondAt {
		t.Errorf("the errors are alphabetised, not in sweep order: %s", body)
	}
}

// The one failure that stops the sweep: the credential vanishing mid-run.
// Every remaining card would fail the same way, so the rest are marked
// unattempted rather than burning the selection on a dead key.
//
// **A sweep with no credential is refused before it starts**, with the 503 the
// UI already handles, rather than minutes into work that was never going to
// produce anything.
//
// This replaces `TestTheCredentialVanishingStopsTheSweep`, which cleared
// ANTHROPIC_API_KEY from inside the fake API so that card two's `Connect()` --
// re-reading the environment every time -- would find nothing. **That scenario
// no longer exists.** The endpoint is a value captured once at boot (ADR 39),
// so a credential cannot vanish out from under a running process, and the
// per-card `ErrUnavailable` break in `argueSweep` is now defence in depth that
// nothing can trigger. What is still reachable, and still worth holding, is the
// check before the job is ever submitted.
func TestASweepWithNoCredentialIsRefusedBeforeItStarts(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	status, _, raw := sweep(t, rig, fmt.Sprintf(`{"cards":["%s","%s","%s"]}`,
		sweepCards[0], sweepCards[1], sweepCards[2]))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("%d %s, want 503 before any job is made", status, raw)
	}
	if !strings.Contains(string(raw), "ANTHROPIC_API_KEY") {
		t.Errorf("the refusal does not name what is unset: %s", raw)
	}
}

func TestTheSameSelectionInADifferentOrderIsOneSweep(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	hold := make(chan struct{})
	script := &scriptedClaude{
		replies: []string{
			answer("end_turn", said(charges("Sol Ring", "x"))),
			answer("end_turn", said(charges("Regal Behemoth", "x"))),
			answer("end_turn", said(charges("Sol Ring", "x"))),
			answer("end_turn", said(charges("Regal Behemoth", "x"))),
			answer("end_turn", said(charges("Vorinclex, Voice of Hunger", "x"))),
		},
		hold: hold,
	}
	rig.api.claude = script.start(t)
	both := fmt.Sprintf(`{"cards":["%s","%s"]}`, sweepCards[0], sweepCards[1])
	reversed := fmt.Sprintf(`{"cards":["%s","%s"]}`, sweepCards[1], sweepCards[0])
	cased := fmt.Sprintf(`{"cards":["%s","%s"]}`, strings.ToUpper(sweepCards[0]), sweepCards[1])

	_, first, _ := sweep(t, rig, both)
	_, second, _ := sweep(t, rig, reversed)
	_, third, _ := sweep(t, rig, cased)
	if first["id"] != second["id"] {
		t.Errorf("the same slots in a different order made two sweeps (%v, %v)", first["id"], second["id"])
	}
	if first["id"] != third["id"] {
		t.Errorf("the same slots differently cased made two sweeps (%v, %v)", first["id"], third["id"])
	}
	// A genuinely different selection is its own job, so this is not passing
	// because the key is constant.
	_, other, _ := sweep(t, rig, fmt.Sprintf(`{"cards":["%s"]}`, sweepCards[2]))
	if other["id"] == first["id"] {
		t.Errorf("a different selection joined the first sweep")
	}
	close(hold)
	rig.jobs.Wait()
}

// charges is one scripted slot argument: the schema's shape, minimally filled.
func charges(card, fact string) string {
	return fmt.Sprintf(`{"charges":[{"ground":"redundancy","claim":"There are cheaper ways.",`+
		`"fact":%q,"strength":"weak"}],"alternatives":[]}`, card+": "+fact)
}
