package api

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
)

// A simulation of a deck that compiles to nothing.
//
// Every other refusal in the simulators is about the machine -- no pool, a pool
// that cannot answer. This one is about the deck, and it is the only state the
// compiler refuses outright: a commander the pool knows and a 99 it does not
// leaves nothing to shuffle, and a figure computed over nothing is worse than no
// figure. The shelf is asked because it answers in the response, where the two
// jobs carry the same refusal into a failed job.
func TestASimulationOfADeckThatCompilesToNothingIsRefusedAboutTheDeck(t *testing.T) {
	t.Parallel()
	rig := newCacheRig(t)

	writeDeck(t, rig.decks, "no-cards-at-all",
		"slug: no-cards-at-all\nname: No Cards At All\nstatus: theoretical\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
			"  - name: Nothing This Pool Has Heard Of\n    category: ramp\n    why: a stranger\n")

	status, payload, raw := callAs(t, rig.api, alice, "POST", "/api/sim/shelf",
		`{"slug":"no-cards-at-all","games":120}`)
	if status == http.StatusOK {
		t.Fatalf("the shelf answered 200 over a deck that compiles to nothing: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries nothing a person could read: %s", raw)
	}

	// And the same deck through a job: the refusal reaches the caller as a
	// failed job rather than as a run over an empty library.
	submitted := rig.submitted(t, "/api/sim/mana", `{"slug":"no-cards-at-all","games":120}`)
	rig.jobs.Wait()
	job := rig.jobs.Get(submitted["id"].(string), alice.UserID)
	if job == nil || job.Status() != jobs.Errored {
		t.Fatalf("the mana run ended %v", job)
	}
	if job.Payload().Error == nil {
		t.Error("the failed run carries no sentence")
	}
}

// A deck raised out of the crypt onto a shelf that will not take it back.
//
// A restore is a rename out of `.trash` and into the library root, so a root the
// process may read and may not write refuses it -- and that refusal is neither
// "no such entry" nor "a deck already holds that slug", which are the two the
// route answers in its own words. It is the third: something went wrong, said
// plainly, with the deck still buried.
func TestRaisingADeckOntoAShelfThatWillNotTakeItIsRefused(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere, so there is no unwritable shelf to build")
	}
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	if status, _, raw := rig.do(t, alice, "DELETE", cleanDeck+"?confirm=bury",
		""); status != http.StatusOK {
		t.Fatalf("burying the deck answered %d: %s", status, raw)
	}
	status, listing, raw := rig.do(t, alice, "GET", "/api/decks/entombed", "")
	if status != http.StatusOK {
		t.Fatalf("the crypt answered %d: %s", status, raw)
	}
	rows, _ := listing["entombed"].([]any)
	if len(rows) == 0 {
		t.Fatalf("nothing is in the crypt: %s", raw)
	}
	row, _ := rows[0].(map[string]any)
	id := str(row, "id")
	if id == "" {
		t.Fatalf("the entry carries no id: %s", raw)
	}

	// The shelf, readable and not writable: the crypt can still be listed, and
	// nothing can be moved out of it.
	if err := os.Chmod(rig.decks, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(rig.decks, 0o755) })

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/entombed/"+id+"/return", "")
	if status != http.StatusInternalServerError {
		t.Fatalf("raising a deck onto an unwritable shelf answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}

	// The deck is still buried, which is the half that matters: a refused
	// restore must not have half-moved somebody's deck.
	if err := os.Chmod(rig.decks, 0o755); err != nil {
		t.Fatal(err)
	}
	_, listing, raw = rig.do(t, alice, "GET", "/api/decks/entombed", "")
	if rows, _ := listing["entombed"].([]any); len(rows) == 0 {
		t.Fatalf("the refused restore emptied the crypt: %s", raw)
	}
}

// A scan whose reading cannot be checked against the pool.
//
// The model transcribes and the pool decides what the transcription is (ADR 34),
// so a pool that opens and cannot answer leaves the second half of that sentence
// unsaid -- and the job has to fail rather than return a transcription dressed as
// a reading. Nothing about a photograph is trusted more for having come from a
// better camera, and nothing about it is trusted at all without the pool.
func TestAScanFailsRatherThanOfferATranscriptionAsAReading(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := decklog.NewRecorder(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { recorder.Close(); _ = db.Close() })
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Claude: noCredential, Logger: quiet,
		// The library is fine and the pool is not, which is the shape that
		// separates "I could not read your photograph" from "I could not look
		// up what I read".
		Pool: schemalessPool(t), DecksDir: decksDir(t), AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil), Jobs: reg})

	script := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"title":"Black Lotus","corner":""}`)),
	}}
	a.claude = script.start(t)

	status, payload, raw := callAs(t, a, alice, "POST", "/api/claude/scan",
		scanBody(fmt.Sprintf(`"image":%q`, aCapture("lotus"))))
	if status != http.StatusOK {
		t.Fatalf("the scan was not accepted: %d %s", status, raw)
	}
	id, _ := payload["id"].(string)
	if id == "" {
		t.Fatalf("no job came back: %s", raw)
	}
	reg.Wait()
	job := reg.Get(id, alice.UserID)
	if job == nil {
		t.Fatal("the registry lost the scan")
	}
	if job.Status() != jobs.Errored {
		t.Fatalf("a scan whose reading could not be checked ended %q with %v",
			job.Status(), job.Result())
	}
	if job.Payload().Error == nil {
		t.Error("the failed scan carries no sentence")
	}
}
