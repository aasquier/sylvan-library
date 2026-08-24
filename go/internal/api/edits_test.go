package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The write routes' own tests. `internal/deckedit` already proves the *bytes*
// against its goldens; what these prove is the layer above it -- that the right
// operation runs, that the pool checks happen before anything is written, that
// the refusals land on the right status, and that every write leaves a deck
// file and a log entry behind.

type writeRig struct {
	api      *API
	decks    string
	dbPath   string
	recorder *decklog.Recorder
	close    func()
}

func newWriteRig(t *testing.T) *writeRig {
	t.Helper()
	decks := decksDir(t)
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
		// The same handle the activity log writes through, which is how the
		// door wires it: two ledgers, two tables, one app.db.
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil),
	})
	return &writeRig{api: a, decks: decks, dbPath: dbPath, recorder: recorder,
		close: func() { recorder.Close(); db.Close() }}
}

// text reads a deck straight off the file tier, which is how these tests check
// that a route wrote what it said it wrote.
func (r *writeRig) text(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.decks, "mono-green-clean", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// history is the deck's activity log, newest first.
func (r *writeRig) history(t *testing.T, slug string, owner *int64) []decklog.Entry {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+r.dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	entries, err := decklog.Entries(context.Background(), db, owner, slug, decklog.DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func (r *writeRig) do(t *testing.T, scope auth.Scope, method, target, body string) (int, map[string]any, []byte) {
	t.Helper()
	return callAs(t, r.api, scope, method, target, body)
}

// mono-green-clean is the fixture with no gate errors; alice is the
// maintainer, so the file tier is hers to write.
const cleanDeck = "/api/decks/alice/mono-green-clean"

func TestEveryWriteLeavesADeckAndAnEntry(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()

	before := rig.text(t)

	status, body, raw := rig.do(t, alice, "POST", cleanDeck+"/cards",
		`{"name":"Llanowar Reborn","category":"land","why":"A land that grows a counter."}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	// `_commit`'s envelope: the operation's own keys, then what the edit did.
	for _, key := range []string{"slug", "added", "category", "into", "stage",
		"total_cards", "needs_rationale", "ok", "errors", "warnings"} {
		if _, present := body[key]; !present {
			t.Errorf("the response lacks %q: %v", key, body)
		}
	}
	if body["added"] != "Llanowar Reborn" || body["category"] != "land" || body["into"] != "cards" {
		t.Errorf("the response describes a different edit: %v", body)
	}

	after := rig.text(t)
	if after == before {
		t.Fatal("the route answered 200 and wrote nothing")
	}
	if !strings.Contains(after, "Llanowar Reborn") {
		t.Errorf("the card is not in the file:\n%s", after)
	}
	// ADR 12 rule 1: an edit touches only what it changes.
	if changed := len(strings.Split(after, "\n")) - len(strings.Split(before, "\n")); changed > 4 {
		t.Errorf("adding one card grew the file by %d lines", changed)
	}

	// ADR 28: the file tier's owner is NULL, never the URL's owner segment.
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 1 {
		t.Fatalf("the history has %d entries, expected 1: %+v", len(entries), entries)
	}
	if entries[0].Action != "add" || entries[0].Summary != "added Llanowar Reborn as land" {
		t.Errorf("recorded %q / %q", entries[0].Action, entries[0].Summary)
	}
	if entries[0].Actor == nil || *entries[0].Actor != "alice" {
		t.Errorf("the actor is %v, expected alice", entries[0].Actor)
	}
}

func TestTheNineOperationsAllRun(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()

	// A chain: each step needs the one before it, which is also the only way
	// the graveyard round trip is reachable.
	steps := []struct {
		name, method, target, body string
		expect                     string // a key the response must carry
	}{
		{"add", "POST", cleanDeck + "/cards",
			`{"name":"Llanowar Reborn","category":"land","why":"A land that grows a counter."}`, "added"},
		{"patch-card", "PATCH", cleanDeck + "/cards/Llanowar%20Reborn",
			`{"field":"why","value":"A rewritten rationale, long enough to make the emitter fold it across more than one line."}`, "card"},
		// Craterhoof is green and not already in the deck, which the swap
		// needs on both counts -- the identity check and the duplicate check
		// are the two the route makes before the editor is reached.
		{"swap", "POST", cleanDeck + "/swap",
			`{"out":"Llanowar Reborn","into":"Craterhoof Behemoth","why":"The finisher the ramp is for."}`, "swapped_out"},
		{"remove", "DELETE", cleanDeck + "/cards/Craterhoof%20Behemoth", "", "entombed"},
		{"return", "POST", cleanDeck + "/graveyard/Craterhoof%20Behemoth/return", "", "returned"},
		{"entomb", "POST", cleanDeck + "/entomb", `{"names":["Craterhoof Behemoth"]}`, "entombed"},
		{"exile", "DELETE", cleanDeck + "/graveyard/Craterhoof%20Behemoth", "", "exiled"},
		{"patch-deck", "PATCH", cleanDeck, `{"field":"status","value":"built"}`, "field"},
		{"note", "PUT", cleanDeck + "/notes/mulligan",
			`{"value":"Keep any seven with two lands and something to do with them."}`, "note"},
	}
	for _, step := range steps {
		status, body, raw := rig.do(t, alice, step.method, step.target, step.body)
		if status != 200 {
			t.Fatalf("%s: %d %s", step.name, status, raw)
		}
		if _, present := body[step.expect]; !present {
			t.Errorf("%s: the response lacks %q: %v", step.name, step.expect, body)
		}
		if _, present := body["ok"]; !present {
			t.Errorf("%s: the response carries no gate verdict", step.name)
		}
	}

	// One entry per operation, newest first, and every verb represented --
	// which is what "one call site" buys: no route can write without the log
	// following it.
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != len(steps) {
		t.Fatalf("%d entries for %d writes", len(entries), len(steps))
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Action] = true
		if e.Summary == "" {
			t.Errorf("an entry has no sentence: %+v", e)
		}
	}
	for _, action := range []string{"add", "set-card", "swap", "entomb", "return", "exile", "set-deck", "note"} {
		if !got[action] {
			t.Errorf("nothing recorded a %q", action)
		}
	}

	// The note landed in the file as prose, folded at the narrower width.
	if text := rig.text(t); !strings.Contains(text, "  mulligan: >-") {
		t.Errorf("the note is not a folded block:\n%s", text)
	}
}

func TestARationaleIsNeverWrittenForYou(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()

	// Rule 4 at the boundary: a swap with no `why` is refused before the
	// editor is reached, and the deck is untouched.
	before := rig.text(t)
	status, body, _ := rig.do(t, alice, "POST", cleanDeck+"/swap",
		`{"out":"Sol Ring","into":"Mana Crypt","why":"   "}`)
	if status != 422 || body["detail"] != "a replacement needs a `why`" {
		t.Errorf("%d %v", status, body)
	}
	// ... and in the editor: a curated deck refuses a blank one on an add.
	status, body, _ = rig.do(t, alice, "POST", cleanDeck+"/cards",
		`{"name":"Llanowar Reborn","category":"land","why":""}`)
	if status != 422 || !strings.Contains(fmtDetail(body), "refusing to invent one") {
		t.Errorf("%d %v", status, body)
	}
	if rig.text(t) != before {
		t.Error("a refused edit changed the file")
	}
	if entries := rig.history(t, "mono-green-clean", nil); len(entries) != 0 {
		t.Errorf("a refused edit was recorded: %+v", entries)
	}
}

func TestTheCardIsLookedUpBeforeItIsWritten(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()

	// Rule 1 applied to a write: three ways a card can fail the pool, and
	// none of them may reach the file.
	cases := []struct {
		name, body, wants string
	}{
		{"unknown", `{"name":"Not A Real Card","category":"ramp","why":"Nope."}`,
			"is not a card the pool knows"},
		{"banned", `{"name":"Primeval Titan","category":"ramp","why":"Banned."}`,
			"is not legal in Commander"},
		// Ajani, Nacatl Pariah is white on its front and red-white on its
		// back, which is exactly why it is in the fixture pool: the identity
		// that matters is Scryfall's, not the mana cost's (rule 2).
		{"off-identity", `{"name":"Ajani, Nacatl Pariah","category":"threat","why":"Off colour."}`,
			"outside the commander's"},
		{"category", `{"name":"Llanowar Reborn","category":"mystery","why":"Bad category."}`,
			"is not a category"},
	}
	before := rig.text(t)
	for _, c := range cases {
		status, body, raw := rig.do(t, alice, "POST", cleanDeck+"/cards", c.body)
		if status != 422 {
			t.Errorf("%s: %d %s", c.name, status, raw)
			continue
		}
		if !strings.Contains(fmtDetail(body), c.wants) {
			t.Errorf("%s: %q does not mention %q", c.name, fmtDetail(body), c.wants)
		}
	}
	if rig.text(t) != before {
		t.Error("a refused card reached the file")
	}
}

// TestWhoMayWriteIsDecidedByTheSource holds the three-way refusal, which is
// the part of ADR 5 and ADR 22 a write can get wrong in a way a read cannot.
func TestWhoMayWriteIsDecidedByTheSource(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()

	// A deck bob cannot see is a 404 -- not a 403, which would confirm it
	// exists. `rich` is the fixture that says `shared: false`, so the
	// shared-only view bob gets over the file tier does not contain it.
	// (`shared` absent means *shared*, which is why the deck named here has
	// to be that one: most fixtures are visible to everybody.)
	if status, _, _ := rig.do(t, bob, "PATCH", "/api/decks/alice/rich",
		`{"field":"status","value":"built"}`); status != 404 {
		t.Errorf("a deck bob cannot see answered %d, expected 404", status)
	}
	// One he *can* see and may not edit is a 403, and the distinction is the
	// whole of ADR 5 at a write: absence hides, refusal explains.
	if status, _, _ := rig.do(t, bob, "PATCH", cleanDeck, `{"field":"status","value":"built"}`); status != 403 {
		t.Errorf("a shared deck bob may not edit answered %d, expected 403", status)
	}
	// Alice cannot see bob's private deck either, maintainer or not.
	if status, _, _ := rig.do(t, alice, "PATCH", "/api/decks/bob/bobs-private",
		`{"field":"status","value":"built"}`); status != 404 {
		t.Errorf("bob's private deck answered %d to alice, expected 404", status)
	}
	// A deck she *can* see and may not edit is a 403: the existence is not
	// the secret, and the request is not malformed.
	status, body, raw := rig.do(t, alice, "PATCH", "/api/decks/bob/bobs-public", `{"field":"status","value":"built"}`)
	if status != 403 {
		t.Fatalf("bob's shared deck answered %d to alice, expected 403: %s", status, raw)
	}
	if !strings.Contains(fmtDetail(body), "bobs-public") {
		t.Errorf("the refusal does not name the deck: %v", body)
	}
	// And an account that does not exist is the same 404 as a deck that does
	// not, so the owner segment cannot enumerate the account list.
	if status, _, _ := rig.do(t, alice, "PATCH", "/api/decks/nobody/whatever",
		`{"field":"status","value":"built"}`); status != 404 {
		t.Errorf("an unknown owner answered %d, expected 404", status)
	}
}

// TestTheSQLTierWritesToo proves the second tier, which the file tier's tests
// cannot: bob editing his own deck writes a row, not a file.
func TestTheSQLTierWritesToo(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()

	status, body, raw := rig.do(t, bob, "PATCH", "/api/decks/bob/bobs-public", `{"field":"status","value":"built"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["field"] != "status" || body["value"] != "built" {
		t.Errorf("the response describes a different edit: %v", body)
	}
	// Read it back through the route, so the assertion is about what the app
	// serves rather than about a column.
	_, deckBody, _ := rig.do(t, bob, "GET", "/api/decks/bob/bobs-public", "")
	if deckBody["status"] != "built" {
		t.Errorf("the deck reads back as %v", deckBody["status"])
	}
	// The owned tier keys on the account, not on the URL's owner segment.
	owner := int64(2)
	entries := rig.history(t, "bobs-public", &owner)
	if len(entries) < 1 || entries[0].Summary != "set status to built" {
		t.Fatalf("the owned history is %+v", entries)
	}
	if entries[0].Actor == nil || *entries[0].Actor != "bob" {
		t.Errorf("the actor is %v", entries[0].Actor)
	}
	// ... and nothing landed on the file tier's NULL owner.
	if fileTier := rig.history(t, "bobs-public", nil); len(fileTier) != 0 {
		t.Errorf("an owned deck's edit reached the file tier's history: %+v", fileTier)
	}
}

// TestAnEditWithoutADatabaseStillEdits is the log's promise at the route
// layer: `Record` never fails the edit that produced it, so an instance with
// no `app.db` still writes decks.
func TestAnEditWithoutADatabaseStillEdits(t *testing.T) {
	t.Parallel()
	decks := decksDir(t)
	// No AppDB, no write handle, no recorder -- a laptop with auth off.
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks})
	ctx := auth.WithScope(context.Background(), auth.Scope{})
	status, body, raw := callCtx(t, a, ctx, "PATCH", "/api/decks/local/mono-green-clean",
		`{"field":"status","value":"built"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["field"] != "status" {
		t.Errorf("%v", body)
	}
	text, err := os.ReadFile(filepath.Join(decks, "mono-green-clean", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "status: built") {
		t.Errorf("the edit did not land:\n%s", text)
	}
}

// TestABodyThatNamesNothingIsRefused holds the PATCH bodies' own contract: a
// field the editor will not write is named as such rather than silently
// ignored, which is what an edit that answered 200 and changed nothing would
// be.
func TestABodyThatNamesNothingIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()
	// A body with no `value` is refused before the editor: `value` is the one
	// key whose absence cannot be told from a deliberate blank, and clearing a
	// field is a real edit.
	if status, answer, _ := rig.do(t, alice, "PATCH", cleanDeck, `{"field":"status"}`); status != 422 ||
		fmtDetail(answer) != "value is required" {
		t.Errorf("a body with no value answered %d %v", status, answer)
	}
	// A field the editor will not write is refused **by the editor**, so the
	// caller learns why rather than only that: since ADR 37 the archetype is a
	// reading of the themes, and the refusal says where the label went.
	status, answer, _ := rig.do(t, alice, "PATCH", cleanDeck,
		`{"field":"archetype","value":"combo"}`)
	if status != 422 || !strings.Contains(fmtDetail(answer), "reading of the themes") {
		t.Errorf("the archetype refusal is %d %v", status, answer)
	}
	if status, answer, _ := rig.do(t, alice, "PATCH", cleanDeck,
		`{"field":"commander","value":"Somebody Else"}`); status != 422 ||
		!strings.Contains(fmtDetail(answer), "not a settable deck field") {
		t.Errorf("an unsettable field answered %d %v", status, answer)
	}
	// A card patch naming an unsettable field is the same shape of refusal.
	if status, answer, _ := rig.do(t, alice, "PATCH", cleanDeck+"/cards/Sol%20Ring",
		`{"field":"name","value":"Nope"}`); status != 422 ||
		!strings.Contains(fmtDetail(answer), "is not settable") {
		t.Errorf("patching a card's name answered %d %v", status, answer)
	}
}

// TestTheBulkSweepIsAllOrNothing: a name that is not in the 99 refuses the
// batch with nothing written, because a sweep that silently skipped two of its
// ten cards would report a deck state nobody chose.
func TestTheBulkSweepIsAllOrNothing(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t)
	defer rig.close()
	before := rig.text(t)
	for _, body := range []string{
		`{"names":["Sol Ring","Not In This Deck"]}`,
		`{"names":["Sol Ring","sol ring"]}`,
		`{"names":[]}`,
		`{"names":"Sol Ring"}`,
	} {
		if status, _, _ := rig.do(t, alice, "POST", cleanDeck+"/entomb", body); status != 422 {
			t.Errorf("%s answered %d, expected 422", body, status)
		}
	}
	if rig.text(t) != before {
		t.Error("a refused sweep changed the file")
	}
	if entries := rig.history(t, "mono-green-clean", nil); len(entries) != 0 {
		t.Errorf("a refused sweep was recorded: %+v", entries)
	}

	// The batch that does resolve entombs every card in one write, one
	// verdict, and one entry.
	status, body, raw := rig.do(t, alice, "POST", cleanDeck+"/entomb",
		`{"names":["Sol Ring","Regal Behemoth"]}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	names, _ := body["entombed"].([]any)
	if len(names) != 2 {
		t.Errorf("entombed %v", body["entombed"])
	}
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 1 || entries[0].Summary != "entombed Sol Ring, Regal Behemoth" {
		t.Fatalf("one sweep recorded %+v", entries)
	}
}

func fmtDetail(body map[string]any) string {
	if d, ok := body["detail"].(string); ok {
		return d
	}
	raw, _ := json.Marshal(body["detail"])
	return string(raw)
}
