package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The intake sheet's fourth tick, which had never been pulled.
//
// `intake_test.go` proves ADR 41's gate -- who may ask for a drafted rationale
// -- and stops at the gate, because at every stance it asks about the job is
// either refused or born finished. So `intakeRun.dossier` sat at **nought of
// sixteen statements**: the one action on the sheet that fills a store rather
// than a file, and the one whose "already done" answer is the difference
// between an import that costs one call and an import that costs one every
// time somebody re-runs the sheet.
//
// Both of its outcomes are here, and the second matters more than the first.
// A dossier already in the store is not asked for again, and the step says so
// rather than reporting a silent zero -- because "0 changed" with no sentence
// beside it is the number somebody reads as a failure and clicks through
// again, which is exactly the call this branch exists to avoid.

// intakeStepOf pulls one action's outcome off a finished intake job.
func intakeStepOf(t *testing.T, done map[string]any, action string) map[string]any {
	t.Helper()
	result, _ := done["result"].(map[string]any)
	if result == nil {
		t.Fatalf("the intake job carries no result: %v", done)
	}
	steps, _ := result["steps"].(map[string]any)
	if steps == nil {
		t.Fatalf("the intake job's result carries no steps: %v", result)
	}
	step, _ := steps[action].(map[string]any)
	if step == nil {
		t.Fatalf("the intake reported no %q step: %v", action, steps)
	}
	return step
}

// runIntake submits the sheet and waits for it.
func runIntake(t *testing.T, rig *jobRig, body string) map[string]any {
	t.Helper()
	status, payload, raw := intake(t, rig, body)
	if status != 200 {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	if id == "" {
		t.Fatalf("the intake answered no job: %s", raw)
	}
	done, _ := rig.await(t, id)
	if done["status"] != "done" {
		t.Fatalf("the intake ended %v: %v", done["status"], done["error"])
	}
	return done
}

// The dossier tick, run for real: one call, one dossier, and the step
// reporting the work it did.
func TestTheIntakeSheetFillsTheDossierAndReportsTheOneCallItMade(t *testing.T) {
	t.Parallel()
	stub := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("The Real Page")+","+said(wholeDossier))}}
	rig := newJobRig(t, stub.start(t))
	defer rig.close()

	step := intakeStepOf(t, runIntake(t, rig, `{"dossier":true}`), "dossier")
	if step["changed"] != float64(1) || step["considered"] != float64(1) {
		t.Fatalf("the dossier step reports %v of %v", step["changed"], step["considered"])
	}
	if note, present := step["note"]; present {
		t.Errorf("a step that did its work carries an explanation as well: %v", note)
	}

	// It is in the store, which is the only proof that matters: the deck file
	// is untouched by this action and the report alone could say anything.
	status, payload, raw := callAs(t, rig.api, alice, "GET", dossierAt, "")
	if status != 200 {
		t.Fatalf("reading the dossier back answered %d: %s", status, raw)
	}
	if payload["cached"] != true {
		t.Fatalf("the intake reported a dossier and stored nothing: %s", raw)
	}
	body, _ := payload["dossier"].(map[string]any)
	if who, _ := body["who"].(map[string]any); who == nil ||
		!strings.Contains(who["prose"].(string), "Qal Sisma") {
		t.Errorf("the stored dossier is not the one that came back: %v", body)
	}
}

// The second time, and the branch that keeps an import cheap: the dossier is
// already written, so no call is made and the step says why the number is
// zero.
func TestTheIntakeSheetDoesNotAskForADossierItAlreadyHas(t *testing.T) {
	t.Parallel()
	// One reply, and one only. A second call would run the script out, which
	// the stub reports as a failure -- so "no second call" is enforced by the
	// fixture rather than asserted afterwards.
	stub := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("The Real Page")+","+said(wholeDossier))}}
	rig := newJobRig(t, stub.start(t))
	defer rig.close()

	status, payload, raw := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	if status != 200 {
		t.Fatalf("writing the dossier answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	if done, _ := rig.await(t, id); done["status"] != "done" {
		t.Fatalf("the dossier job ended %v: %v", done["status"], done["error"])
	}

	step := intakeStepOf(t, runIntake(t, rig, `{"dossier":true}`), "dossier")
	if step["changed"] != float64(0) || step["considered"] != float64(1) {
		t.Fatalf("the dossier step reports %v of %v -- a stored dossier was "+
			"asked for again", step["changed"], step["considered"])
	}
	note, _ := step["note"].(string)
	if !strings.Contains(note, "already") {
		t.Errorf("a zero with no sentence beside it reads as a failure: %q", note)
	}
}

// The slot sweep over a deck with nothing outside its mana base.
//
// An argument is about a slot, and a land is not a slot anybody argues over,
// so a deck of lands is a sweep with nothing to do -- reported with its own
// sentence rather than as zero calls made, for the reason above: a bare zero
// on a sheet reads as a step that failed.
func TestTheSlotSweepSaysThereWasNothingToArgueAboutRatherThanNothingHappened(t *testing.T) {
	t.Parallel()
	// A credential the sheet can see and a script with nothing in it: the
	// endpoint check passes, and any call the sweep made would run the script
	// out and fail the test. "No call was made" is held by the fixture rather
	// than asserted afterwards.
	stub := &scriptedClaude{}
	rig := newJobRig(t, stub.start(t))
	defer rig.close()
	plantDeck(t, rig.decks, "allland", `slug: allland
name: Nothing But Lands
status: theoretical
stage: curated
commander:
  - Goreclaw, Terror of Qal Sisma
bracket: 2
strategy: A fixture, not a deck.
cards:
  - name: Forest
    category: land
    why: It taps for green.
  - name: Llanowar Reborn
    category: land
    why: It taps for green and grows a counter.
`)

	// `consultant` makes calls and writes nothing, which is enough to get
	// past the sheet's gate; the sweep then finds nothing to call about, so
	// the absent credential is never reached.
	status, payload, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/allland/intake", `{"argue":true,"stance":"consultant"}`)
	if status != 200 {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	done, _ := rig.await(t, id)
	if done["status"] != "done" {
		t.Fatalf("the intake ended %v: %v", done["status"], done["error"])
	}
	step := intakeStepOf(t, done, "argue")
	if step["considered"] != float64(0) {
		t.Fatalf("a deck of lands offered %v slots to argue about", step["considered"])
	}
	if note, _ := step["note"].(string); !strings.Contains(note, "mana base") {
		t.Errorf("the step does not say why it did nothing: %q", note)
	}
}

// plantDeck writes one more deck into a file tier a rig has already built.
func plantDeck(t *testing.T, decks, slug, text string) {
	t.Helper()
	dir := filepath.Join(decks, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
