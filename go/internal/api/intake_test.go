package api

import (
	"net/http"
	"strings"
	"testing"
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
