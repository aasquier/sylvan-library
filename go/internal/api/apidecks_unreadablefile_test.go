package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The deck that is still in memory and no longer on disk.
//
// Every write route resolves its deck through `writeTarget`, which asks
// `Source.Get` -- and the file tier answers that from the library's parsed-deck
// memory whenever the file's stamp has not moved. A mode change moves neither
// the mtime nor the size, so a `deck.yaml` that stops answering is a deck the
// route still finds and the editor can no longer read: `src.ReadText` is the
// *second* read of the same file, and it is the one that fails.
//
// That is not a contrived shape. It is a volume whose permissions were repaired
// by hand, a deck restored out of a backup under the wrong owner, a container
// that changed users between two requests -- and the question it asks is the
// one the brief asks of every fault: does the route refuse, or does it answer
// 200 over a deck it never read? Eleven routes shared the branch and none of
// them had been asked.

// warmed is a rig whose `mono-green-clean` is in the deck memory, with the path
// of the file that memory stands in for.
func warmedDeckMemory(t *testing.T) (*writeRig, string) {
	t.Helper()
	rig := newWriteRig(t, noCredential)
	t.Cleanup(rig.close)

	// A plain read is what fills the memory -- the same call the browser makes
	// when it opens the deck page.
	if status, _, raw := rig.do(t, alice, "GET", cleanDeck, ""); status != http.StatusOK {
		t.Fatalf("reading the deck first answered %d: %s", status, raw)
	}
	hits, _ := rig.api.deckMemo.Counts()
	path := filepath.Join(rig.decks, "mono-green-clean", "deck.yaml")
	if status, _, raw := rig.do(t, alice, "GET", cleanDeck, ""); status != http.StatusOK {
		t.Fatalf("reading the deck again answered %d: %s", status, raw)
	}
	if after, _ := rig.api.deckMemo.Counts(); after <= hits {
		t.Fatalf("the deck is not remembered: hits went %d -> %d", hits, after)
	}
	return rig, path
}

func TestEveryWriteRefusesADeckFileItCanNoLongerRead(t *testing.T) {
	t.Parallel()
	rig, path := warmedDeckMemory(t)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	// Every route that reads the deck's text for itself, with a body that would
	// otherwise have been written. The bodies are the ones `edits_test.go`'s
	// chain uses, so a route that starts refusing earlier than this fault shows
	// up as a changed status here rather than as a silent hole.
	writes := []struct{ name, method, target, body string }{
		{"swap", "POST", cleanDeck + "/swap",
			`{"out":"Sol Ring","into":"Craterhoof Behemoth","why":"The finisher the ramp is for."}`},
		{"add", "POST", cleanDeck + "/cards",
			`{"name":"Llanowar Reborn","category":"land","why":"A land that grows a counter."}`},
		{"board", "POST", cleanDeck + "/board",
			`{"name":"Craterhoof Behemoth","category":"payoff","why":"the finisher, if the curve can carry it"}`},
		{"remove", "DELETE", cleanDeck + "/cards/Sol%20Ring", ""},
		{"entomb", "POST", cleanDeck + "/entomb", `{"names":["Sol Ring"]}`},
		{"bulk", "POST", cleanDeck + "/bulk", `{"text":"1 Sol Ring"}`},
		{"return", "POST", cleanDeck + "/graveyard/Sol%20Ring/return", ""},
		{"exile", "DELETE", cleanDeck + "/graveyard/Sol%20Ring", ""},
		{"patch-card", "PATCH", cleanDeck + "/cards/Sol%20Ring",
			`{"field":"category","value":"ramp"}`},
		{"patch-deck", "PATCH", cleanDeck, `{"field":"status","value":"built"}`},
		{"note", "PUT", cleanDeck + "/notes/mulligan",
			`{"value":"Keep any seven with two lands and something to do with them."}`},
		{"combos", "PUT", cleanDeck + "/combos", wallCombo},
	}
	for _, wr := range writes {
		status, payload, raw := rig.do(t, alice, wr.method, wr.target, wr.body)
		// Not the wording -- the operating system's words are not ours -- but
		// that it refused at all, with the library's own sentence rather than a
		// cause nobody can act on (commandment 10).
		if status != http.StatusInternalServerError {
			t.Errorf("%s: a deck whose file cannot be read answered %d: %s", wr.name, status, raw)
			continue
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
			t.Errorf("%s: the refusal reads %q", wr.name, detail)
		}
	}

	// And the file is byte-identical: a refusal that had already written
	// something would be worse than a 200.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("a refused write changed the deck file")
	}
}

// One step further in: the file reads and the directory will not take a write.
//
// `WriteText` replaces `deck.yaml` by writing a temporary file beside it and
// renaming, so a directory the process may read and may not write is the fault
// that lands on the write itself rather than on either read. `commit` is the
// one call site every edit goes out through (ADR 28), and this is the branch
// where it has computed the new deck and cannot land it.
func TestACommitRefusesAShelfItCannotWriteBackTo(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	dir := filepath.Join(rig.decks, "mono-green-clean")
	before := rig.text(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	status, payload, raw := rig.do(t, alice, "PATCH", cleanDeck,
		`{"field":"status","value":"built"}`)
	if status != http.StatusInternalServerError {
		t.Fatalf("a shelf that will not take a write answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "could not answer") {
		t.Errorf("the refusal reads %q", detail)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if rig.text(t) != before {
		t.Error("the write that could not land landed anyway")
	}
	// Nothing claimed the edit happened either: the activity log is written
	// after the file, so a refused write leaves no line.
	if entries := rig.history(t, "mono-green-clean", nil); len(entries) != 0 {
		t.Errorf("a refused write recorded %d entries", len(entries))
	}
}
