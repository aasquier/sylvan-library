package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The edit routes' remaining refusals: a deck file the editor cannot scan, a
// board entry with no quantity on it, the same card entombed twice, and a
// machine with no card pool asked to write.

// writeDeck puts a hand-written deck on a rig's shelf. Hand-written is the
// point: the emitter would never produce these shapes, and `deck.yaml` is the
// source of truth (ADR 1), so a file somebody typed is the input every one of
// these branches actually exists for.
func writeDeck(t *testing.T, decks, slug, body string) {
	t.Helper()
	dir := filepath.Join(decks, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// noPoolWriteRig is a writable library on a machine that has never run a
// refresh: every write that needs a card looked up has to refuse, and the
// sentence has to send somebody to the refresh rather than to their deck.
func noPoolWriteRig(t *testing.T) *API {
	t.Helper()
	db := auth.Open(appDB(t))
	t.Cleanup(func() { _ = db.Close() })
	return New(Config{DecksDir: decksDir(t), AdminEmail: "alice@example.com", AppDB: db})
}

// A write that needs a card looked up, on a machine with no pool.
//
// Two routes, and they refuse in two different places: `/cards` through the
// shared playability check, `/bulk` after its own whole-list read. Both are 422
// with a sentence about the pool, because the deck is fine and the machine is
// not -- and neither may write, since a card nobody could look up is a card
// whose legality is a guess (rule 1).
func TestAWriteThatNeedsACardLookedUpRefusesWithoutAPool(t *testing.T) {
	t.Parallel()
	a := noPoolWriteRig(t)

	for _, tc := range []struct{ name, target, body string }{
		{"add", cleanDeck + "/cards",
			`{"name":"Craterhoof Behemoth","category":"payoff","why":"the finisher"}`},
		{"bulk", cleanDeck + "/bulk", `{"text":"1 Sol Ring","dry_run":true}`},
	} {
		status, payload, raw := callAs(t, a, alice, "POST", tc.target, tc.body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d on a machine with no pool: %s", tc.name, status, raw)
			continue
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "card pool") {
			t.Errorf("%s: the refusal reads %q", tc.name, detail)
		}
	}
}

// A deck file that parses perfectly and cannot be edited.
//
// ADR 12's refusal, from the one direction that is dangerous. Every
// malformed-input sweep reaches for files that fail at the *parse*, and those
// are refused before anything looks at anything -- where this file is a real
// deck to the reader and half a deck to the writer: the edit engine recognises
// a card by the literal line `- name: ...`, so a block list whose last entry is
// a bare string parses to two cards and scans to one.
//
// Both halves of the refusal are asked. The **plan** is read through the same
// scanner as the write, so a bulk edit is refused while it is still on screen
// rather than half way through a fold; and the one operation that walks a list
// of names (`entomb`) stops on the first card it cannot find in the text, with
// the file exactly as it was.
func TestADeckTheEditorCannotScanIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	const mixed = "/api/decks/alice/half-scanned"
	writeDeck(t, rig.decks, "half-scanned",
		"slug: half-scanned\nname: Half Scanned\nstatus: theoretical\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\n"+
			"cards:\n  - name: Sol Ring\n    category: ramp\n    why: it ramps\n  - Forest\n")

	before, err := os.ReadFile(filepath.Join(rig.decks, "half-scanned", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, target, body string }{
		{"bulk", mixed + "/bulk", `{"text":"1 Sol Ring\n1 Craterhoof Behemoth"}`},
		// Forest is in the parsed 99 and nowhere in the text as an entry, which
		// is the disagreement the whole refusal is about.
		{"entomb", mixed + "/entomb", `{"names":["Forest"]}`},
	} {
		status, payload, raw := rig.do(t, alice, "POST", tc.target, tc.body)
		if status == http.StatusOK {
			t.Errorf("%s: a deck the editor cannot scan was written anyway: %s", tc.name, raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s: the refusal carries nothing a person could read: %s", tc.name, raw)
		}
	}
	after, err := os.ReadFile(filepath.Join(rig.decks, "half-scanned", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("a refused edit changed the deck file")
	}
}

// The same card named twice in one entomb.
//
// The first one moves it to the graveyard and the second has nothing left to
// move, so the loop stops on the second and the whole request is refused -- one
// write or none, never a file with half the list applied.
func TestEntombingTheSameCardTwiceIsRefusedWithoutWritingHalfOfIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.text(t)
	status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/entomb",
		`{"names":["Sol Ring","Sol Ring"]}`)
	if status == http.StatusOK {
		t.Fatalf("the same card was entombed twice: %s", raw)
	}
	if !saysSomething(payload) {
		t.Errorf("the refusal carries nothing a person could read: %s", raw)
	}
	if rig.text(t) != before {
		t.Error("the refused entomb wrote the first half of the list")
	}
}

// A board entry carrying no quantity, promoted into the 99.
//
// A board is a place where a card is weighed rather than played, and a
// hand-written one may say `qty: 0` or leave the key off. Promoting it adds one
// copy, which is the only reading of "put this in the deck" -- never zero, which
// would be an add that adds nothing and reports that it did.
func TestPromotingABoardEntryWithNoQuantityAddsOneCopy(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	writeDeck(t, rig.decks, "weighing",
		"slug: weighing\nname: Weighing\nstatus: theoretical\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\n"+
			"cards:\n  - name: Forest\n    category: land\n    why: green mana\n"+
			"swap_board:\n  - name: Craterhoof Behemoth\n    category: payoff\n"+
			"    qty: 0\n    why: not yet convinced\n")

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/alice/weighing/cards",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"the curve can carry it now"}`)
	if status != http.StatusOK {
		t.Fatalf("promoting a board entry answered %d: %s", status, raw)
	}
	if payload["added"] != "Craterhoof Behemoth" {
		t.Errorf("the response describes %v", payload["added"])
	}
	text, err := os.ReadFile(filepath.Join(rig.decks, "weighing", "deck.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// One copy in the 99, and the board entry gone: a promotion moves a card,
	// it does not copy it.
	if strings.Count(string(text), "Craterhoof Behemoth") != 1 {
		t.Errorf("the card appears %d times:\n%s",
			strings.Count(string(text), "Craterhoof Behemoth"), text)
	}
	if strings.Contains(string(text), "qty: 0") {
		t.Errorf("a quantity of nothing was written into the deck:\n%s", text)
	}
}

// A build whose deck cannot be written back out as a file.
//
// `Dump` refuses a `strategy` that is not prose, because a whole-file write
// cannot give a mapping the ordering it needs -- and the build stashes a snapshot
// of the deck as one of its five artifacts, so a deck that cannot be dumped
// cannot be built. A hand-written file is the only way to hold that shape, which
// is exactly why the branch exists.
func TestABuildOfADeckThatCannotBeWrittenBackIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	writeDeck(t, rig.decks, "listed-strategy",
		"slug: listed-strategy\nname: Listed Strategy\nstatus: theoretical\nstage: built\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\n"+
			"strategy:\n  - ramp into it\n  - swing\n"+
			"cards:\n  - name: Forest\n    category: land\n    why: green mana\n")

	status, payload, raw := rig.do(t, alice, "POST",
		"/api/decks/alice/listed-strategy/artifacts", `{"force":true}`)
	if status != http.StatusInternalServerError {
		t.Fatalf("a deck that cannot be dumped was built anyway: %d %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
	if _, err := os.Stat(filepath.Join(rig.decks, "listed-strategy", "artifacts")); err == nil {
		t.Error("the refused build wrote a shelf anyway")
	}
}

// A snapshot the build cannot parse is no baseline, and a snapshot it cannot
// read is a refusal. Two faults one file apart, and the difference is the whole
// reason the second exists: a snapshot that will not parse is replaced by the
// build that is running, where a snapshot behind a mode nobody can read is a
// shelf the process cannot see.
func TestABuildTreatsAnUnparseableSnapshotAsNoneAndAnUnreadableOneAsAFault(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable file to build")
	}
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	const built = "/api/decks/alice/kaheera/artifacts"
	snapshot := filepath.Join(rig.decks, "kaheera", "artifacts", "deck.last-built.yaml")

	// Unparseable: the build runs, and `swaps.md` is simply not among what it
	// wrote, because there was no previous deck to diff against.
	if err := os.WriteFile(snapshot, []byte("cards: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, payload, raw := rig.do(t, alice, "POST", built, `{"force":true}`)
	if status != http.StatusOK {
		t.Fatalf("a build over an unparseable snapshot answered %d: %s", status, raw)
	}
	names := map[string]bool{}
	list, _ := payload["artifacts"].([]any)
	for _, item := range list {
		if row, ok := item.(map[string]any); ok {
			names[str(row, "name")] = true
		}
	}
	if len(names) == 0 {
		t.Fatalf("the build wrote nothing: %s", raw)
	}
	if names["swaps.md"] {
		t.Error("a build with no readable baseline wrote a swap list anyway")
	}

	// Unreadable: the same file, one mode change on. The shelf still lists --
	// `Artifacts` only stats -- and the snapshot read is the one that fails.
	if err := os.Chmod(snapshot, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(snapshot, 0o644) })
	status, payload, raw = rig.do(t, alice, "GET", built, "")
	if status != http.StatusInternalServerError {
		t.Fatalf("a shelf whose snapshot cannot be read answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
}

// The shelf, when the accounts database has gone and the maintainer lookup is
// not what fails.
//
// With no maintainer configured the library resolves without reading a row, so
// the first read that touches `app.db` is the one asking which other people's
// decks this caller may see -- and an empty shelf over a database that has gone
// would tell somebody their library was gone.
func TestTheShelfRefusesRatherThanEmptyingWhenTheAccountsHaveGone(t *testing.T) {
	t.Parallel()
	db := auth.Open(appDB(t))
	// No AdminEmail: the maintainer lookup is skipped entirely, so the failure
	// lands one layer in rather than on the resolve.
	a := New(Config{DecksDir: decksDir(t), Pool: pooltest.Open(t), AppDB: db})
	if status, _, raw := as(t, a, alice, "/api/decks"); status != http.StatusOK {
		t.Fatalf("the shelf answered %d before the database went: %s", status, raw)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	status, payload, raw := as(t, a, alice, "/api/decks")
	if status != http.StatusInternalServerError {
		t.Fatalf("the shelf answered %d over a database that has gone: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
}

// A simulation asked for while the accounts database is gone.
//
// Every simulation names its deck in the body, so it resolves the caller's
// library for itself -- and that resolve reads a row when a maintainer is
// configured. A 500 with a sentence is the answer; a run over a library nobody
// could resolve is not.
func TestASimulationRefusesWhenTheLibraryCannotBeResolved(t *testing.T) {
	t.Parallel()
	db := auth.Open(appDB(t))
	a := New(Config{DecksDir: decksDir(t), Pool: pooltest.Open(t),
		AdminEmail: "alice@example.com", AppDB: db})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	status, payload, raw := callAs(t, a, alice, "POST", "/api/sim/mana",
		`{"slug":"kaheera","games":120}`)
	if status != http.StatusInternalServerError {
		t.Fatalf("a simulation over an unresolvable library answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
}

// A deck raised out of the crypt whose file cannot be read back.
//
// The crypt keeps the deck's own text, so a restore is a rename and then a read
// -- and a trashed file that was touched while it was buried comes back as
// something the parser refuses. The restore has already happened by then, which
// is why this answers a fault rather than a 404: the deck is on the shelf, and
// what cannot be described is its contents.
func TestRaisingADeckWhoseBuriedFileWasTouchedIsAFaultRatherThanASilentRestore(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// Bury bob's own deck, which lives on the SQL tier, then bury alice's from
	// the file tier -- the file tier is the one with a `.trash` directory a test
	// can reach into.
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

	// The buried file, rewritten into something no parser takes. The crypt
	// names its folders `<slug>-<stamp>` and hands out an id that is not the
	// folder, so the folder is found by the slug it was buried under.
	folders, err := filepath.Glob(filepath.Join(rig.decks, ".trash", "mono-green-clean-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 {
		t.Fatalf("the crypt holds %v", folders)
	}
	if err := os.WriteFile(filepath.Join(folders[0], "deck.yaml"),
		[]byte("cards: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/entombed/"+id+"/return", "")
	if status != http.StatusInternalServerError {
		t.Fatalf("raising a deck nobody can parse answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
}

// A card pool one table short of the one the binary reads.
//
// `tokens_made` is not a table -- the sheet is assembled out of `oracle_cards`
// and `printings` -- so a pool missing the printings is one that can still name
// every card and cannot say what any token looks like. That is the deploy window
// ADR 23 describes read from the other end, and the route must refuse rather
// than answer "this deck makes nothing", which is the one sentence that is false.
func TestATokenSheetRefusesWhenThePrintingsHaveGone(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE printings`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)

	appdb := auth.Open(appDB(t))
	t.Cleanup(func() { _ = appdb.Close() })
	decks := decksDir(t)
	// Terastodon is the fixture card that makes a token, so this is the deck
	// whose sheet has something to assemble.
	writeDeck(t, decks, "elephants",
		"slug: elephants\nname: Elephants\nstatus: theoretical\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\n"+
			"cards:\n  - name: Terastodon\n    category: payoff\n    why: it makes elephants\n")
	a := New(Config{DecksDir: decks, Pool: p, AdminEmail: "alice@example.com", AppDB: appdb})

	status, payload, raw := as(t, a, alice, "/api/decks/alice/elephants/tokens")
	if status != http.StatusInternalServerError {
		t.Fatalf("a token sheet over a pool missing its printings answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
}
