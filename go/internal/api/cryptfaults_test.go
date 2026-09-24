package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// A crypt you can bury into and cannot read back, and a crypt you cannot bury
// into at all.
//
// These are the same deployed fault `unreadable_test.go` builds one level up
// -- a directory whose mode is not what the process expects -- aimed at the
// one room where the two halves come apart. The file tier's crypt is a
// directory: burying is a rename **into** it, and listing is a read **of** it,
// and a mode that permits one and refuses the other is not a contrivance, it
// is what a restore from a backup with the wrong umask leaves behind.
//
// The branch that matters is the first one. `deleteDeck` reads the crypt back
// after the burial purely to find the handle that raises the deck again, and
// an unreadable crypt there is deliberately **not** an error: the deck really
// is buried, and refusing would tell somebody their deletion failed when it
// did not. What it must do instead is answer `recoverable: false` -- "it is in
// the crypt, and I cannot offer you the one-click way out" -- which is true,
// and is a different claim from "there is no way back". Nothing had ever run
// that line.

// crypticRig is a writeRig whose `.trash` wears a mode the caller chooses.
// `0o333` is write-and-search without read: a rename into it lands, a listing
// of it does not.
func crypticRig(t *testing.T, mode os.FileMode) (*writeRig, string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root reads and writes everything, so there is no mode to set")
	}
	decks := decksDir(t)
	trash := filepath.Join(decks, ".trash")
	if err := os.Chmod(trash, mode); err != nil {
		t.Fatal(err)
	}
	// Restored before the temp dir is swept, or the sweep cannot read it
	// either.
	t.Cleanup(func() { _ = os.Chmod(trash, 0o750) })

	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := decklog.NewRecorder(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{
		Pool: pooltest.Open(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil),
	})
	return &writeRig{api: a, decks: decks, dbPath: dbPath, recorder: recorder,
		close: func() { recorder.Close(); db.Close() }}, decks
}

// The burial lands, the handle does not, and the answer says exactly that.
func TestADeckBuriedIntoACryptThatCannotBeReadIsStillBuried(t *testing.T) {
	t.Parallel()
	rig, decks := crypticRig(t, 0o333)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "DELETE",
		"/api/decks/alice/mono-green-clean?confirm=bury", "")
	if status != http.StatusOK {
		t.Fatalf("the delete answered %d over a crypt it could not read: %s", status, raw)
	}
	if payload["deleted"] != true {
		t.Errorf("the deck was not reported as deleted: %s", raw)
	}
	// The two fields that have to be separately true, and separately
	// visible: it is gone, and there is no handle to offer.
	if payload["recoverable"] != false || payload["crypt_id"] != "" {
		t.Errorf("a crypt that could not be read still offered a way back: %s", raw)
	}
	// And it really is gone from the shelf, which is the half the answer
	// would be lying about if this were reported as a failure.
	if _, err := os.Stat(filepath.Join(decks, "mono-green-clean", "deck.yaml")); err == nil {
		t.Error("the deck is still on the shelf and was reported as deleted")
	}

	// The crypt itself refuses rather than listing the half it could reach: a
	// list of everything you buried with something missing from it is the
	// worst answer this screen has.
	status, payload, raw = rig.do(t, alice, "GET", "/api/decks/entombed", "")
	if status == http.StatusOK {
		t.Fatalf("an unreadable crypt was listed as a crypt: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the crypt answered %d with nothing to read: %s", status, raw)
	}

	// Emptying it reports the failure rather than reporting a clean sweep of
	// nothing, which would read as "everything you buried is destroyed".
	status, payload, raw = rig.do(t, alice, "DELETE",
		"/api/decks/entombed?confirm=exile", "")
	if status == http.StatusOK {
		t.Fatalf("a crypt that could not be read reported itself emptied: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("emptying answered %d with nothing to read: %s", status, raw)
	}
}

// The other half of the mode: a crypt that can be read and not written. The
// burial cannot happen, and the deck must still be on the shelf afterwards.
func TestADeleteThatCannotReachTheCryptLeavesTheDeckWhereItIs(t *testing.T) {
	t.Parallel()
	rig, decks := crypticRig(t, 0o500)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "DELETE",
		"/api/decks/alice/mono-green-clean?confirm=bury", "")
	if status == http.StatusOK {
		t.Fatalf("a delete that could not reach the crypt reported success: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries no sentence: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(decks, "mono-green-clean", "deck.yaml")); err != nil {
		t.Errorf("the deck went somewhere after a delete that refused: %v", err)
	}
}

// A deck always comes back under its own name, so the one refusal a restore
// has is a name the shelf has taken since.
//
// The sentence is the whole test: it has to say which name, and it has to say
// what the person can do about it, because the route deliberately will not
// rename anything on its own.
func TestARestoreRefusesRatherThanRenamingWhenTheNameIsTaken(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "DELETE",
		"/api/decks/alice/mono-green-clean?confirm=bury", "")
	if status != http.StatusOK {
		t.Fatalf("the delete answered %d: %s", status, raw)
	}
	id, _ := payload["crypt_id"].(string)
	if id == "" {
		t.Fatalf("the burial offered no handle to raise it with: %s", raw)
	}

	// A new deck takes the name while the old one is in the ground.
	status, _, raw = rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"mono-green-clean","commander":"Goreclaw, Terror of Qal Sisma"}`)
	if status != http.StatusOK {
		t.Fatalf("creating over the freed slug answered %d: %s", status, raw)
	}

	status, payload, raw = rig.do(t, alice, "POST",
		"/api/decks/entombed/"+id+"/return", "")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a restore onto a taken name answered %d: %s", status, raw)
	}
	detail := fmtDetail(payload)
	for _, want := range []string{"mono-green-clean", "comes back under its own name"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal never says %q: %q", want, detail)
		}
	}
}

// The master switch over a library whose decks cannot be listed.
//
// `PUT /api/decks/shared` writes one flag across every deck the caller may
// write, which means it asks each library for its slugs -- and a library that
// will not answer that question must refuse rather than report `changed: 0`,
// which is "the switch did nothing" dressed as success.
func TestTheMasterSwitchRefusesWhenTheShelfWillNotList(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	decks := decksDir(t)
	if err := os.Chmod(decks, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(decks, 0o750) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: db})

	for _, row := range []struct{ target, body string }{
		{"/api/decks/shared", `{"shared":true}`},
		{"/api/decks/coliseum-at-night", `{"coliseum_at_night":true}`},
	} {
		status, payload, raw := callAs(t, a, alice, "PUT", row.target, row.body)
		if status == http.StatusOK {
			t.Errorf("%s reported %v decks changed over a shelf it could not "+
				"list: %s", row.target, payload["changed"], raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s answered %d with nothing to read: %s", row.target, status, raw)
		}
	}
}
