package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The intake sheet's refusals (ADR 41).
//
// What is proved here is the gate, not the modes -- `internal/claude` holds
// those to their own tests. This is the file where ADR 41's narrowing lives,
// so this is where the narrowing's edges have to be pinned: who may ask for a
// drafted rationale, what happens to somebody who may not, and that the four
// actions which were always allowed are not dragged down with the one that
// was not.

const intakeAt = "/api/decks/alice/kaheera/intake"

func intake(t *testing.T, rig *jobRig, body string) (int, map[string]any, []byte) {
	t.Helper()
	return callAs(t, rig.api, alice, "POST", intakeAt, body)
}

// **The second gate, and the one the ADR turns on.** A stance whose write axis
// is `none` cannot ask for a drafted rationale, and the refusal names the
// setting rather than silently handing back a deck of blanks -- somebody who
// ticked the box and got nothing would reasonably report the feature broken.
func TestDraftingARationaleNeedsAStanceThatAllowsAWrite(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	for _, preset := range []string{"off", "consultant", "second-opinion"} {
		status, payload, raw := intake(t, rig,
			`{"rationales":true,"stance":"`+preset+`"}`)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s: drafting was allowed at a stance that writes nothing "+
				"(%d): %s", preset, status, raw)
			continue
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "write") {
			t.Errorf("%s: the refusal does not name the setting that decided it: %q",
				preset, detail)
		}
	}
}

// The four actions that were always inside the rules are not gated on the
// write axis, because they are not the thing ADR 41 narrowed. Filing a card
// and describing a deck are fields a person sets; a rationale is a claim about
// their thinking, and only that one needed a decision.
func TestTheOtherActionsAreNotGatedOnTheWriteAxis(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	// `consultant` makes calls and writes nothing. Every action but the first
	// must get past the gate -- what happens after is a job, and with no
	// credential it is a job that reports rather than one that runs.
	for _, action := range []string{"categories", "description", "dossier", "argue"} {
		status, payload, raw := intake(t, rig,
			`{"`+action+`":true,"stance":"consultant"}`)
		if status == http.StatusUnprocessableEntity &&
			strings.Contains(fmtDetail(payload), "write") {
			t.Errorf("%s was refused for the write axis, which is the rationale "+
				"gate leaking onto actions it was never about: %s", action, raw)
		}
	}
}

// A sheet with nothing ticked is refused rather than submitted as a job that
// does nothing, which would be a spinner with no work behind it.
func TestAnEmptyIntakeSheetIsRefused(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	for _, body := range []string{`{}`, `{"rationales":false}`,
		`{"rationales":false,"categories":false,"dossier":false}`} {
		status, _, raw := intake(t, rig, body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s was accepted (%d): %s", body, status, raw)
		}
	}
}

// The deck is resolved before any action is read, which is ADR 5 and not a
// detail: a deck this caller cannot see is absent from their source, so every
// verb against it is a 404 -- and a 403, or a 422 about the actions, would
// both confirm it exists.
func TestAnIntakeOnSomebodyElsesDeckIsAFourOhFour(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	status, _, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/no-such-deck/intake", `{"rationales":true}`)
	if status != http.StatusNotFound {
		t.Errorf("a deck that is not there answered %d rather than 404: %s", status, raw)
	}
}

// The de-duplication key is what the sheet IS, so the same sheet asked twice
// is one piece of work and a different sheet is different work.
func TestTheIntakeKeyIsTheSheetAndNotItsSpelling(t *testing.T) {
	t.Parallel()
	same := []intakeActions{
		{Rationales: true, Dossier: true},
		{Dossier: true, Rationales: true},
	}
	if intakeKey(same[0]) != intakeKey(same[1]) {
		t.Errorf("two spellings of one sheet have different keys (%q, %q), so a "+
			"double-click starts a second intake over the same deck",
			intakeKey(same[0]), intakeKey(same[1]))
	}
	if intakeKey(intakeActions{Rationales: true}) ==
		intakeKey(intakeActions{Categories: true}) {
		t.Error("two different sheets share a key, so asking for categories would " +
			"join an intake that is drafting rationales")
	}
	if got := intakeKey(intakeActions{}); got != "" {
		t.Errorf("an empty sheet keyed as %q", got)
	}
}

// At a stance that makes no call the job is born finished, and it says why
// rather than reporting five actions that did nothing.
func TestAnIntakeAtAStanceThatIsOffIsBornFinished(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	status, payload, raw := intake(t, rig, `{"categories":true,"stance":"off"}`)
	if status != http.StatusOK {
		t.Fatalf("a stance of off answered %d: %s", status, raw)
	}
	// A job, and one that already has its answer: nothing was going to be
	// called, so paying for a worker to discover that would be theatre.
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("the job carries no result: %s", raw)
	}
	if result["asked"] != false {
		t.Errorf("the result claims a call was made: %v", result["asked"])
	}
	if reason, _ := result["reason"].(string); !strings.Contains(reason, "rationale") {
		t.Errorf("the reason does not say what the rule is: %q", reason)
	}
}

// One action's outcome, and the shapes it has to keep: a count somebody can
// read, and a sentence exactly when the count alone would read as a failure.
func TestAnIntakeStepSaysWhatItDidAndWhyWhenZeroIsNotAFault(t *testing.T) {
	t.Parallel()
	worked := intakeStep(12, 40, "")
	if len(worked) != 2 {
		t.Errorf("a step that worked carries a note it does not need: %+v", worked)
	}
	if worked[0].Key != "changed" || worked[0].Value != 12 {
		t.Errorf("the count is not first or not right: %+v", worked)
	}

	// Zero with a reason is the important case. "0 changed" on its own reads
	// as a failure, and most of the ways this reaches zero are the guards
	// working -- every card already filed, every card already reasoned about.
	nothing := intakeStep(0, 0, "Every card already had a reason.")
	if len(nothing) != 3 || nothing[2].Key != "note" {
		t.Fatalf("a zero step carries no explanation: %+v", nothing)
	}
}

// A failed action reports in words a player can read, and never the machinery
// underneath it (commandment 10).
func TestAFailedIntakeStepSaysSomethingAPlayerCanRead(t *testing.T) {
	t.Parallel()
	got := intakeFailed(errors.New("dial tcp 10.0.0.1:443: connect: refused"), claude.Endpoint{})
	if got[0].Value != 0 {
		t.Errorf("a failed step claims it changed something: %+v", got)
	}
	note, _ := got[2].Value.(string)
	if note == "" {
		t.Fatalf("a failed step says nothing at all: %+v", got)
	}
	for _, leak := range []string{"tcp", "10.0.0.1", "443", "connect:"} {
		if strings.Contains(note, leak) {
			t.Errorf("the note recites the plumbing (%q): %q", leak, note)
		}
	}
}

// The job's answer keeps its shape whether or not anything ran, so a caller
// reading `steps` never meets a null.
func TestAnIntakeResultAlwaysCarriesItsSteps(t *testing.T) {
	t.Parallel()
	empty := intakeResult("gyome-food", false, "the stance is off", nil)
	var steps any
	for _, kv := range empty {
		if kv.Key == "steps" {
			steps = kv.Value
		}
	}
	if steps == nil {
		t.Fatalf("a result with no steps carries a null: %+v", empty)
	}
	full := intakeResult("gyome-food", true, "", []wire.KV{
		{Key: "categories", Value: intakeStep(3, 3, "")},
	})
	// No reason when there is nothing to explain: an empty string on the wire
	// is a field a client has to decide about.
	for _, kv := range full {
		if kv.Key == "reason" {
			t.Errorf("a result that ran carries an empty reason: %+v", full)
		}
	}
}

// ---- the whole write path, end to end --------------------------------------

// **The test ADR 41 is actually about.** A draft deck goes in, the intake runs
// both writing passes, and what comes out is a deck file with the categories
// filed, the rationales written, and every drafted sentence marked as drafted.
//
// It is here rather than in `internal/claude` because that package deliberately
// cannot do this: it answers, and the write is this layer's. So the only place
// the two halves meet is a route test, and the only thing that proves ADR 41
// was implemented rather than described is reading the file afterwards.
func TestTheIntakeWritesWhatItWasAnsweredAndMarksIt(t *testing.T) {
	t.Parallel()
	script := &scriptedClaude{replies: []string{
		// Categories first -- the order `all` runs them in, because every
		// later action reads better categories.
		answer("end_turn", said(`{"filings":[`+
			`{"card":"Sol Ring","category":"ramp","fact":"Adds two colorless."},`+
			`{"card":"Beast Within","category":"interaction","fact":"Destroy target permanent."}]}`)),
		answer("end_turn", said(`{"drafts":[`+
			`{"card":"Sol Ring","why":"Two mana on turn one, every game.","fact":"Adds two colorless."},`+
			`{"card":"Beast Within","why":"Answers anything, at the cost of a 3/3.","fact":"Destroy target permanent."}]}`)),
		answer("end_turn", said(`{"strategy":"Ramp into big green creatures and `+
			`swing. Slow to interact, and it folds to a fast combo.",`+
			`"themes":["stompy","ramp"],"fact":"Goreclaw discounts creatures."}`)),
	}}
	rig := newJobRig(t, script.start(t))
	defer rig.close()
	writeDraftDeck(t, rig.decks, "fresh-import")

	status, payload, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/fresh-import/intake",
		`{"categories":true,"rationales":true,"description":true,"stance":"collaborator"}`)
	if status != http.StatusOK {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	if id == "" {
		t.Fatalf("no job came back: %s", raw)
	}
	rig.await(t, id)

	written := readDeckFile(t, rig.decks, "fresh-import")

	// The filings landed.
	for _, want := range []string{"category: ramp", "category: interaction"} {
		if !strings.Contains(written, want) {
			t.Errorf("%q is not in the file:\n%s", want, written)
		}
	}
	// The rationales landed, verbatim.
	if !strings.Contains(written, "Two mana on turn one, every game.") {
		t.Errorf("the drafted rationale did not reach the file:\n%s", written)
	}
	// **And they are marked.** Since a drafted rationale now satisfies
	// promotion to `curated`, this mark is the only thing left recording that
	// a person did not write these sentences.
	if got := strings.Count(written, "why_by: claude"); got != 2 {
		t.Errorf("%d of 2 drafted rationales are marked:\n%s", got, written)
	}
	// The card that already had a reason keeps it, unmarked. A draft that
	// wrote over somebody's own words would lose them and then mis-credit the
	// replacement.
	if !strings.Contains(written, "why: A reason somebody actually wrote.") {
		t.Errorf("a rationale that was already there was overwritten:\n%s", written)
	}
	// **The deck's own `strategy`**, which is the paragraph the library shelf,
	// the deck page and the primer all render. It is NOT marked: `why_by`
	// exists because a rationale is a claim about somebody's thinking, and a
	// paragraph about the whole deck is a different object.
	if !strings.Contains(written, "strategy:") {
		t.Errorf("the description did not land as the deck's strategy:\n%s", written)
	}
	if !strings.Contains(written, "Ramp into big green creatures") {
		t.Errorf("the description did not reach the file:\n%s", written)
	}
	if !strings.Contains(written, "stompy") {
		t.Errorf("the themes did not reach the file:\n%s", written)
	}
}

// Nothing to do is reported as nothing to do, and costs no call at all.
func TestAnIntakeOverAFinishedDeckAsksForNothing(t *testing.T) {
	t.Parallel()
	// No replies scripted: any call would fail the test.
	script := &scriptedClaude{}
	rig := newJobRig(t, script.start(t))
	defer rig.close()

	// `kaheera` is curated: every card filed, every card reasoned about.
	status, payload, raw := callAs(t, rig.api, alice, "POST", intakeAt,
		`{"categories":true,"rationales":true,"description":true,"stance":"collaborator"}`)
	if status != http.StatusOK {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	done, doneRaw := rig.await(t, id)

	result, ok := done["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %s", doneRaw)
	}
	steps, _ := result["steps"].(map[string]any)
	for _, name := range []string{"categories", "rationales"} {
		step, _ := steps[name].(map[string]any)
		if step == nil {
			t.Errorf("%s did not report at all: %s", name, doneRaw)
			continue
		}
		if changed, _ := step["changed"].(float64); changed != 0 {
			t.Errorf("%s changed %v cards on a finished deck", name, changed)
		}
		if note, _ := step["note"].(string); note == "" {
			t.Errorf("%s changed nothing and does not say why, which reads as a "+
				"failure: %v", name, step)
		}
	}
}

// writeDraftDeck puts a freshly-imported-looking deck on the file tier: two
// cards owing a rationale and filed under the importer's default, and one that
// somebody has already written.
func writeDraftDeck(t *testing.T, decks, slug string) {
	t.Helper()
	dir := filepath.Join(decks, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "slug: " + slug + "\nname: Fresh Import\nstatus: theoretical\n" +
		"stage: draft\ncommander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n" +
		"  - name: Sol Ring\n    category: utility\n    why: ''\n" +
		"  - name: Beast Within\n    category: utility\n    why: ''\n" +
		"  - name: Regal Behemoth\n    category: threat\n" +
		"    why: A reason somebody actually wrote.\n"
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readDeckFile(t *testing.T, decks, slug string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(decks, slug, "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The slot sweep, run at intake. It writes nothing to the deck -- it is the
// existing ADR 25 argument, once per card -- so what is proved here is that it
// runs over the 99 and skips the mana base.
//
// One call per card by construction: an argument is about one slot, and
// batching it would be the balanced answer ADR 25 exists to prevent wearing a
// different hat.
func TestTheIntakesArgueStepSkipsTheManaBase(t *testing.T) {
	t.Parallel()
	// `kaheera` holds four cards that are not lands, and one Forest. Exactly
	// four replies are scripted, so a fifth call -- the land -- runs the
	// script out and fails the test rather than passing quietly.
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(charges("Sol Ring", "there are cheaper rocks"))),
		answer("end_turn", said(charges("Regal Behemoth", "six mana is a lot"))),
		answer("end_turn", said(charges("Vorinclex, Voice of Hunger", "and again"))),
		answer("end_turn", said(charges("Cultivator Colossus", "eight mana"))),
	}}
	rig := newJobRig(t, script.start(t))
	defer rig.close()

	status, payload, raw := callAs(t, rig.api, alice, "POST", intakeAt,
		`{"argue":true,"stance":"collaborator"}`)
	if status != http.StatusOK {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	done, doneRaw := rig.await(t, id)

	result, _ := done["result"].(map[string]any)
	steps, _ := result["steps"].(map[string]any)
	step, _ := steps["argue"].(map[string]any)
	if step == nil {
		t.Fatalf("the argue step did not report: %s", doneRaw)
	}
	if considered, _ := step["considered"].(float64); considered != 4 {
		t.Errorf("looked at %v cards; the deck has four that are not lands, and "+
			"arguing about a Forest is a call nobody wanted", considered)
	}
	if changed, _ := step["changed"].(float64); changed != 4 {
		t.Errorf("made %v arguments of 4: %s", changed, doneRaw)
	}
}
