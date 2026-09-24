package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The share toggle's own write, and the description that came back empty.
//
// Both are the same shape of answer: a route that did not do the thing, told
// apart from a route that did. The share flag is the sharper of the two,
// because the toggle's own answer is **the whole deck read back** -- so a
// failed write that fell through to the read would render the deck with the
// switch in the position nobody managed to put it in.

// The file tier keeps `shared` in the deck file, so a deck directory the
// process cannot write into is a toggle that cannot move -- and must say so
// rather than answering with the deck.
func TestAShareToggleThatCouldNotBeWrittenDoesNotAnswerWithTheDeck(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	decks := decksDir(t)
	dir := filepath.Join(decks, "mono-green-clean")
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o750) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: db})

	// `false` rather than `true` on purpose: `shared: true` removes the key
	// instead of writing one, so a test of the *write* has to ask for the
	// value that is actually spelled out in the file.
	status, payload, raw := callAs(t, a, alice, "PUT",
		"/api/decks/alice/mono-green-clean/shared", `{"shared":false}`)
	if status == http.StatusOK {
		t.Fatalf("the toggle answered with the deck over a file it could not "+
			"write: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries no sentence: %s", raw)
	}
	// The key never landed, which is what makes the refusal true.
	text, err := os.ReadFile(filepath.Join(dir, "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "shared: false") {
		t.Errorf("the flag was written and the route refused anyway:\n%s", text)
	}
}

// A description that comes back empty is not a description.
//
// The step keeps the paragraph the deck already had -- which for a deck with
// none is still none -- and says so, because a `strategy:` overwritten with
// an empty string is the one outcome here that loses somebody's words.
func TestAnEmptyDescriptionLeavesTheDeckWithTheOneItHad(t *testing.T) {
	t.Parallel()
	stub := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"strategy":"","themes":[],"fact":""}`))}}
	rig := newJobRig(t, stub.start(t))
	defer rig.close()
	plantDeck(t, rig.decks, "undescribed", `slug: undescribed
name: Undescribed
status: theoretical
stage: draft
commander:
  - Goreclaw, Terror of Qal Sisma
bracket: 2
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one.
`)

	status, payload, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/undescribed/intake", `{"description":true}`)
	if status != http.StatusOK {
		t.Fatalf("the intake answered %d: %s", status, raw)
	}
	id, _ := payload["id"].(string)
	done, _ := rig.await(t, id)
	if done["status"] != "done" {
		t.Fatalf("the intake ended %v: %v", done["status"], done["error"])
	}
	step := intakeStepOf(t, done, "description")
	if step["changed"] != float64(0) {
		t.Fatalf("an empty description was written as one: %v", step)
	}
	if note, _ := step["note"].(string); !strings.Contains(note, "No description") {
		t.Errorf("the step does not say why nothing changed: %q", note)
	}
	if text := rig.textOf(t, "undescribed"); strings.Contains(text, "strategy: ''") {
		t.Errorf("the deck was given an empty strategy:\n%s", text)
	}
}
