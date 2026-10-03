package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The intake's remaining arms: the stance the route cannot read, the filing that
// could not be saved, the drafting the stance forbids, and the two sweep
// outcomes where some calls landed and some did not.

// **A stance nobody offers is refused at the route, before any job.** The
// request's own `stance` is free text on the wire, so this is the arm a stale
// page and a direct call both reach, and it has to be a 422 with the reason
// rather than a job that quietly resolves the deck's default instead.
func TestAnIntakeWithAStanceNobodyOffersIsRefused(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()

	status, payload, raw := callAs(t, rig.api, alice, "POST", intakeAt,
		`{"argue":true,"stance":"magnificent"}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown stance answered %d: %s", status, raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "stance") {
		t.Errorf("the refusal says %q and never names the setting", detail)
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by a request the route refused", len(got))
	}
}

// **The drafting refuses the write a second time, at the write.** The route
// checks the stance's write axis and so does this, because the two answer
// different questions — the route refuses the request, this refuses the write —
// and a stance that reached the job without one must get nothing rather than a
// deck full of drafts.
func TestTheDraftingRefusesAStanceWithNoWriteAxisAtTheWriteToo(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "draft-no-write")
	run := intakeRunOver(t, rig, "draft-no-write", true, noCredential)
	consultant, err := claude.Preset("consultant")
	if err != nil {
		t.Fatal(err)
	}
	run.stance = consultant

	step := run.draft(t.Context())
	if changed := stepValue(t, step, "changed"); changed != 0 {
		t.Errorf("a stance with no write axis drafted %v rationales", changed)
	}
	note, _ := stepValue(t, step, "note").(string)
	if strings.TrimSpace(note) == "" {
		t.Fatalf("the refusal said nothing: %+v", step)
	}
	if strings.Contains(readDeckFile(t, rig.decks, "draft-no-write"), "why_by") {
		t.Error("a drafting the stance forbids marked a rationale anyway")
	}
}

// Both writing passes report the **save** failing, not just the call.
//
// A filing or a drafting that reached an answer and could not keep it is the
// expensive failure: the call is paid for and the deck is unchanged, and a step
// that reported a silent zero would read as a deck that needed no filing.
func TestTheWritingPassesSaySoWhenTheShelfRefusesTheSave(t *testing.T) {
	t.Parallel()
	for _, pass := range []struct {
		name  string
		slug  string
		reply string
		step  func(run *intakeRun) wire.OrderedMap
	}{
		{
			name:  "the filing",
			slug:  "file-read-only",
			reply: `{"filings":[{"card":"Sol Ring","category":"ramp","fact":"Adds two colorless."}]}`,
			step:  func(run *intakeRun) wire.OrderedMap { return run.file(t.Context()) },
		},
		{
			name:  "the drafting",
			slug:  "draft-read-only",
			reply: `{"drafts":[{"card":"Sol Ring","why":"Two mana on turn one.","fact":"Adds two colorless."}]}`,
			step:  func(run *intakeRun) wire.OrderedMap { return run.draft(t.Context()) },
		},
	} {
		t.Run(pass.name, func(t *testing.T) {
			t.Parallel()
			script := &scriptedClaude{replies: []string{
				answer("end_turn", said(pass.reply))}}
			set := script.start(t)
			rig := newJobRig(t, noCredential)
			t.Cleanup(rig.close)
			plantFresh(t, rig, pass.slug)
			// A tier nothing may write: the answer arrives and the save is
			// refused.
			run := intakeRunOver(t, rig, pass.slug, false, noCredential)
			run.req.Endpoint = set.Endpoint

			step := pass.step(run)
			if changed := stepValue(t, step, "changed"); changed != 0 {
				t.Errorf("%s reported %v changed over a tier that refuses writes",
					pass.name, changed)
			}
			if note, _ := stepValue(t, step, "note").(string); strings.TrimSpace(note) == "" {
				t.Fatalf("%s reported a silent zero: %+v", pass.name, step)
			}
		})
	}
}

// ---- the slot sweep --------------------------------------------------------

// **A credential that goes away mid-sweep stops the sweep.** Ninety-eight more
// doomed calls is ninety-eight more round trips for the same answer, so the
// sweep gives up and says it did not finish — which is a different sentence
// from "Claude had no opinion about your deck".
func TestTheSweepStopsWhenTheCredentialGoesAway(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantFresh(t, rig, "sweep-no-key")
	// No endpoint at all: every call is refused as unavailable rather than as
	// an upstream error, which is the one refusal the sweep stops on.
	run := intakeRunOver(t, rig, "sweep-no-key", true, noCredential)

	step := run.argue(t.Context())
	if changed := stepValue(t, step, "changed"); changed != 0 {
		t.Errorf("a sweep with no credential made %v arguments", changed)
	}
	note, _ := stepValue(t, step, "note").(string)
	if !strings.Contains(note, "not attempted") {
		t.Errorf("the sweep said %q and never said it had stopped part way", note)
	}
	// Commandment 10: the sentence a player reads names nothing underneath.
	for _, machinery := range []string{"ANTHROPIC", "API_KEY", "http", "401"} {
		if strings.Contains(note, machinery) {
			t.Errorf("the sentence carries %q: %q", machinery, note)
		}
	}
}

// **A sweep that argued some slots and not others says how many it skipped.**
// The two numbers alone ("3 of 5") read as a sweep that chose to look at three,
// so the count of refusals rides beside them.
func TestASweepThatLostSomeCallsSaysHowManyItSkipped(t *testing.T) {
	t.Parallel()
	// Two cards outside the mana base: the first argued, the second refused.
	// The first card is argued; the second gets an answer that is not a message
	// at all — half a document, the shape of a proxy that cut the response off.
	// It is refused once rather than tried again, which is what makes it the
	// refusal the sweep *counts* instead of the one it stops on.
	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(charges("Sol Ring", "there are cheaper rocks"))),
		`{"type":"message","content":`,
	}}
	set := script.start(t)
	rig := newJobRig(t, noCredential)
	defer rig.close()
	plantDeck(t, rig.decks, "sweep-half-lost", `slug: sweep-half-lost
name: Half Lost
status: theoretical
stage: draft
commander:
  - Goreclaw, Terror of Qal Sisma
cards:
  - name: Sol Ring
    category: utility
    why: Mana.
  - name: A Card This Pool Has Never Held
    category: utility
    why: Unknown.
`)
	run := intakeRunOver(t, rig, "sweep-half-lost", true, noCredential)
	run.req.Endpoint = set.Endpoint

	step := run.argue(t.Context())
	if changed := stepValue(t, step, "changed"); changed != 1 {
		t.Fatalf("the sweep made %v arguments of two cards, want 1: %+v",
			changed, step)
	}
	note, _ := stepValue(t, step, "note").(string)
	if !strings.Contains(note, "1 of the 2") {
		t.Errorf("the sweep said %q and never counted what it skipped", note)
	}
}
