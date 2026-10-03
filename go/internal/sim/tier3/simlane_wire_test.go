package tier3

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// What crosses the wire, asked about the shapes that cannot.
//
// `wireroundtrip_test.go` holds the encoders against their own decoders, which
// is the property that matters and says nothing about a deck that refuses to be
// written down or a `seats` key that is not a number. Both are reachable and
// neither had run — and the first of them is the only way *three* refusals in
// this package are ever reached, because a deck is turned into text before every
// hosted call there is.

// aDeckThatCannotBeWritten is a deck whose `strategy` is a mapping rather than
// prose. `FromText` passes a hand-written file's strategy through whatever shape
// it is in, and a whole-file dump cannot order a mapping — so this is a deck
// that lives happily in the library and cannot be handed to the worker.
func aDeckThatCannotBeWritten() *deck.Deck {
	d := testDeck("alpha")
	d.Strategy = map[string]any{"plan": "go wide"}
	return d
}

// A deck that cannot be written down is refused before anything crosses, and
// refused the same way by all three callers — the pre-flight, the match and the
// encoder itself. A half-written seat would be a game played with somebody
// else's deck.
func TestADeckThatCannotBeWrittenDownNeverCrosses(t *testing.T) {
	t.Parallel()
	decks := []*deck.Deck{aDeckThatCannotBeWritten(), testDeck("beta")}

	if texts, err := DecksToWire(decks); err == nil {
		t.Errorf("a deck with a mapping for a strategy crossed as %q", texts)
	} else if !strings.Contains(err.Error(), "alpha") {
		t.Errorf("the refusal does not name the deck: %v", err)
	}

	shim := newStubShim(t)
	w := shim.worker(time.Second, "")
	if reports, err := w.CheckCoverage(context.Background(), decks); err == nil {
		t.Errorf("a pre-flight ran on a deck that cannot cross: %+v", reports)
	}
	if run, err := w.RunMatch(context.Background(), decks, MatchAsk{Games: 1}); err == nil {
		t.Errorf("a match was played with a deck that cannot cross: %+v", run)
	}
	// Each call checks the worker is awake before it tries to write the decks
	// down, so two health checks reached the shim and nothing else did.
	if got := shim.headers(); len(got) != 2 {
		t.Errorf("the shim was asked %d times, want the two health checks and "+
			"no work at all", len(got))
	}
}

// A run with no games in it crosses as an empty list rather than as nothing: a
// browser reading `null` where it expects an array draws no bout at all, and a
// match that was refused before its first game is exactly when that happens.
func TestARunWithNoGamesCrossesAsAnEmptyList(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(WireRun{WallSeconds: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"games":[]`) {
		t.Errorf("a run with no games crossed as %s", raw)
	}
	// And it reads back as a run with no games rather than as a refusal.
	var back WireRun
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("a run with no games would not read back: %v", err)
	}
	if len(back.Games) != 0 || back.WallSeconds != 1.5 {
		t.Errorf("it read back as %+v", back)
	}
}

// A `seats` key that is not a number is refused rather than dropped: the seat
// number is how every row in the run names its player, and a seat the reader
// quietly skipped would leave a winner nobody can match to a deck.
func TestASeatKeyThatIsNotANumberIsRefused(t *testing.T) {
	t.Parallel()
	var run WireRun
	err := json.Unmarshal([]byte(`{"games":[],"seats":{"one":"alpha"},`+
		`"wall_seconds":1}`), &run)
	if err == nil {
		t.Fatalf("a seat called \"one\" read back as %+v", run.Seats)
	}
	// The shim's own answer is read through the same path, so the refusal has
	// to survive the whole decode rather than being swallowed per key.
	if len(run.Seats) != 0 {
		t.Errorf("a refused run still carries %v", run.Seats)
	}
}

// A subprocess that cannot be started is a refusal naming the thing that would
// not start, and nothing comes back as a run — a bout with no JVM behind it must
// never read as a bout that finished with no games in it.
func TestAJVMThatWillNotStartIsARefusalRatherThanAnEmptyBout(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "java-that-is-not-there")
	run, err := spawn([]string{missing, "-jar", "forge.jar"}, t.TempDir(),
		RunOptions{Timeout: time.Minute}, newProseTelling(false))
	if err == nil {
		t.Fatalf("a JVM that is not there played %d games", len(run.Games()))
	}
	if run != nil {
		t.Error("a refused subprocess still handed back a run")
	}
	if !strings.Contains(err.Error(), "starting Forge") {
		t.Errorf("the refusal reads %q", err)
	}

	// And the killer survives that: a subprocess that never started has no
	// process to signal, and a group kill aimed at nothing must not be what
	// takes the app down. It is reached for real — a timer armed before a
	// failed start still fires.
	endGroup(nil)
}

// **A match nobody abandoned leaves nobody watching for it.** The abort watcher
// is wound up when the read loop ends, so a bout that finished normally has no
// goroutine left holding the channel — which is what this asserts directly: a
// send that finds no receiver is a watcher that has gone.
func TestAMatchThatFinishesLeavesNoOneWatchingForAnAbort(t *testing.T) {
	t.Parallel()
	abort := make(chan struct{})
	run, err := spawn([]string{"/bin/sh", "-c", won(1, 1, 900) + ":"},
		t.TempDir(), RunOptions{Timeout: time.Minute, Abort: abort},
		newProseTelling(false))
	if err != nil {
		t.Fatalf("a bout with an abort channel wired failed: %v", err)
	}
	if len(run.Games()) != 1 {
		t.Fatalf("the bout played %d games, want 1", len(run.Games()))
	}
	if run.clockedOut {
		t.Error("a bout nobody abandoned was reported as cut")
	}

	// An unbuffered channel accepts a send only while somebody is receiving.
	select {
	case abort <- struct{}{}:
		t.Error("the abort watcher outlived the match it was watching")
	case <-time.After(100 * time.Millisecond):
	}
}
