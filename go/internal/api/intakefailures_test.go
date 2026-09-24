package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// The intake sheet when every call it makes comes back refused.
//
// This is the shape an expired key, a rate limit and an upstream having a bad
// afternoon all take, and it is the one an import is most likely to meet: the
// sheet is five actions long and each one is a call, so a run that started
// fine can stop being fine half way down. Every step has an `intakeFailed`
// branch for exactly this, and **not one of the five had ever taken it** --
// the sheet's tests all stop at the gate, where no call is made at all.
//
// Two things are asked. Each step must **say** it failed, in a sentence, so
// the sheet does not render five silent zeroes. And the run must **finish**:
// a failing step is a step that failed, not a job that crashed, because the
// deck exists by now and the person is looking at a page about it.

// refusingClaude is an endpoint that answers every request with an error --
// the deployed shape of a key that has stopped working, rather than of a
// script that ran out.
func refusingClaude(t *testing.T) claude.Settings {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"nope"}}`))
	}))
	t.Cleanup(srv.Close)
	return claude.Settings{Endpoint: claude.EndpointAt(srv.URL, "test-key-not-a-real-one")}
}

// All five actions, against an endpoint that refuses everything.
func TestEveryIntakeActionReportsItsOwnFailureAndTheRunStillFinishes(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, refusingClaude(t))
	defer rig.close()
	// A deck with no description and cards still on the importer's default
	// category and with no reasons: everything the sheet does has something
	// to do, so no step short-circuits before it reaches its call.
	plantDeck(t, rig.decks, "fresh-import", `slug: fresh-import
name: Fresh Import
status: theoretical
stage: draft
commander:
  - Goreclaw, Terror of Qal Sisma
bracket: 2
cards:
  - name: Sol Ring
    category: utility
    why: ''
  - name: Craterhoof Behemoth
    category: utility
    why: ''
`)

	// `collaborator` is the one preset whose write axis reaches the drafting,
	// so all five actions get past the sheet's own gate and every one of them
	// reaches a call.
	status, payload, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/fresh-import/intake",
		`{"categories":true,"rationales":true,"description":true,`+
			`"dossier":true,"argue":true,"stance":"collaborator"}`)
	if status != http.StatusOK {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	done, _ := rig.await(t, id)
	// The run finishes. A refused call is an outcome the sheet renders, not
	// a job that fell over -- the deck is already on the shelf by here.
	if done["status"] != "done" {
		t.Fatalf("one refused call took the whole run down (%v): %v",
			done["status"], done["error"])
	}

	result, _ := done["result"].(map[string]any)
	steps, _ := result["steps"].(map[string]any)
	if len(steps) != 5 {
		t.Fatalf("the sheet reported %d of five steps: %v", len(steps), steps)
	}
	for _, action := range []string{"categories", "rationales", "description",
		"dossier", "argue"} {
		step, _ := steps[action].(map[string]any)
		if step == nil {
			t.Errorf("no %q step at all: %v", action, steps)
			continue
		}
		if step["changed"] != float64(0) {
			t.Errorf("%s reported %v changed against an endpoint that refused "+
				"everything", action, step["changed"])
		}
		note, _ := step["note"].(string)
		if action == "argue" {
			// **Recorded rather than fixed, and it is a real gap.** The slot
			// sweep counts a refused call as a call not made and carries on,
			// so a run where *every* call was refused renders "0 of 2" with
			// nothing said -- the silent zero the other four steps each have
			// a sentence to avoid, and the one a person reads as "Claude had
			// no opinion about my deck". Its one note fires only on
			// `claude.ErrUnavailable`, which is the credential going away
			// mid-sweep rather than the calls failing (`intake.go`'s `argue`).
			// Changing what a player is told is Aaron's call, not a patch, so
			// this holds the answer as it is and fails the day it improves.
			if strings.TrimSpace(note) != "" {
				t.Errorf("the slot sweep now says something when every call was "+
					"refused (%q) -- that is the better answer, and this test "+
					"recorded the older one", note)
			}
			continue
		}
		if strings.TrimSpace(note) == "" {
			t.Errorf("%s reported a zero with no sentence beside it, which reads "+
				"as a step that did nothing rather than one that could not: %v",
				action, step)
		}
	}

	// And nothing was written into the deck on the way past: a refused draft
	// must not leave a card with a blank reason marked as drafted (rule 4).
	text := rig.textOf(t, "fresh-import")
	if strings.Contains(text, "why_by") {
		t.Errorf("a refused drafting marked a rationale anyway:\n%s", text)
	}
}

// rigTextOf lets a jobRig read a deck off its file tier the way a writeRig
// does, since the two rigs stand up the same file source.
func (r *jobRig) textOf(t *testing.T, slug string) string {
	t.Helper()
	return (&writeRig{decks: r.decks}).textOf(t, slug)
}
