package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The last mile: the sentences, statuses and single lines that nothing else in
// this package has a reason to reach, each asked where it stands.
//
// They are not a category. What they have in common is that every one of them
// is what somebody *reads* — an exit status a script branches on, the class
// name on a failure a maintainer greps a job row for, the word that tells a
// planeswalker from a creature in a card's line — and a report is exactly as
// good as its worst sentence.

// What the process leaves behind, and what it says on the way.
//
// **A refused gate is deliberately silent**, and that is the assertion worth
// having: `decks validate` has already printed its report, so a second
// sentence on the error stream would be noise in a script that only wanted the
// code. Every other failure is named, because the caller has not been told
// about it yet.
func TestTheExitStatusSaysNothingTwiceAndNothingNotAtAll(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		err  error
		code int
		says string
	}{
		{"a command that worked", nil, 0, ""},
		{"a deck that failed its gate", errFailedGate, 1, ""},
		// Wrapped, because that is how it reaches `main` from inside a
		// command rather than as the sentinel itself.
		{"a gate failure carried up", errors.New("wrapped"), 1, "mtglab: wrapped\n"},
		{"anything else", errors.New("the volume did not mount"), 1,
			"mtglab: the volume did not mount\n"},
	} {
		var said bytes.Buffer
		if got := exitCode(tc.err, &said); got != tc.code {
			t.Errorf("%s exited %d, want %d", tc.what, got, tc.code)
		}
		if said.String() != tc.says {
			t.Errorf("%s said %q, want %q", tc.what, said.String(), tc.says)
		}
	}
	// And the sentinel survives wrapping, which is what makes the silence a
	// property of the verdict rather than of one `return` statement.
	var said bytes.Buffer
	if got := exitCode(errors.Join(errFailedGate, nil), &said); got != 1 || said.Len() != 0 {
		t.Errorf("a wrapped gate failure exited %d saying %q", got, said.String())
	}
}

// Every class name a Forge failure can wear, because the class is the half
// that survives translation: the client shows the sentence, and a maintainer
// reading a job row wants to know whether Forge was missing, the match timed
// out, or the results were untrustworthy.
func TestEveryForgeFailureWearsItsClassName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		want string
	}{
		{tier3.NotInstalled("no Forge at /nowhere"), "ForgeNotInstalled: "},
		{errors.New("something else went wrong"), "RuntimeError: "},
		// Wrapped rather than constructed, because these two are raised inside
		// the runner and reach this function through whatever wrapped them.
		{errors.Join(tier3.ErrCoverageFailed, errors.New("three cards missing")),
			"CoverageFailed: "},
		{errors.Join(tier3.ErrResultsUntrustworthy, errors.New("a card was dropped")),
			"ResultsUntrustworthy: "},
		{errors.Join(tier3.ErrTimedOut, errors.New("the subprocess overran")),
			"TimeoutExpired: "},
	} {
		got := failureText(tc.err)
		if !strings.HasPrefix(got, tc.want) {
			t.Errorf("%v rendered as %q, want it to open with %q", tc.err, got, tc.want)
		}
		if strings.TrimPrefix(got, tc.want) == "" {
			t.Errorf("%v rendered as a class with no sentence behind it", tc.err)
		}
	}
}

// deafWriter is a listener that has gone: the headers went out and every write
// after them fails, which is what a socket whose peer has closed does.
type deafWriter struct {
	header http.Header
	wrote  int
}

func (w *deafWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}
func (w *deafWriter) WriteHeader(int) {}
func (w *deafWriter) Write(p []byte) (int, error) {
	w.wrote++
	return 0, errors.New("write: broken pipe")
}

// **A failed write is a listener that has gone, and it stops the match.**
//
// This is the zombie from the other side. `shimabandon_test.go` drives the
// cancelled *request*; this drives the failed *write*, and the two are
// deliberately both wired because neither is reliable alone — a write only
// fails once the peer's close has been noticed, and a request context is only
// cancelled once the server's background read sees it.
//
// The old behaviour swallowed the error and played on, on the reasoning that a
// vanished listener must not kill the JVM mid-game. The cost was backwards:
// the request stayed in flight, `inFlight` never fell to zero, and the idle
// watchdog cannot stop a machine with work on it.
func TestAFailedWriteStopsTheMatchItCouldNotReport(t *testing.T) {
	t.Parallel()
	aborted := make(chan struct{})
	handler := &shim{
		state: newShimState(),
		log:   log.New(io.Discard, "", 0),
		play: func(_ []*deck.Deck, opt tier3.RunOptions) (*tier3.SimRun, error) {
			// One line out, which fails; then the runner waits to be told.
			opt.OnGame(1, finishedRun().Games()[0])
			select {
			case <-opt.Abort:
				close(aborted)
			case <-time.After(10 * time.Second):
			}
			return nil, tier3.ErrAbandoned
		},
	}
	deaf := &deafWriter{}
	req := httptest.NewRequest(http.MethodPost, "/match",
		strings.NewReader(matchBody(t, 2)))
	handler.ServeHTTP(deaf, req)

	select {
	case <-aborted:
	default:
		t.Fatal("a match whose report could not be delivered played on -- this " +
			"is the zombie that held a deployed arena for an hour")
	}
	if deaf.wrote == 0 {
		t.Error("nothing was ever written, so nothing could fail")
	}
}

// A door with no stand-in plays its match through the machine it was given,
// and a machine with no Forge on it answers 503 — the status the app reads as
// "this worker is not ready" rather than as "the match broke".
func TestADoorWithNoStandInAsksTheMachineItWasGiven(t *testing.T) {
	t.Parallel()
	srv := newShimFor(t, tier3.Settings{Home: t.TempDir()})
	resp := post(t, srv, "/match", "", matchBodyPlain(t, 1, 0))
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("a worker with no distribution answered %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "MTGLAB_FORGE_HOME") {
		t.Errorf("the refusal did not say what is missing: %s", body)
	}
}

// A shim given an idle limit arms the watch; one given none does not. Both
// halves answer their health route, which is what the app polls after a
// machine start.
func TestAShimWithAnIdleLimitStillComesUpAndAnswers(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := heldPort(t)
	forge := tier3.Settings{
		ShimHost: "127.0.0.1", ShimPort: atoi(t, portOf(t, l)),
		Home: t.TempDir(),
		// A day, so the watch this arms cannot reach its verdict inside any
		// run of this suite -- what is being tested is that it was armed and
		// that arming it changes nothing about coming up.
		IdleSeconds: 24 * 60 * 60,
	}
	done := make(chan error, 1)
	go func() {
		done <- serveShimOn(ctx, forge, func() (net.Listener, error) { return l, nil })
	}()
	resp, err := waitForHealth(t, "http://127.0.0.1:"+portOf(t, l)+"/healthz", done)
	if err != nil {
		t.Fatalf("a shim with an idle limit never answered: %v", err)
	}
	_ = resp.Body.Close()
}

// `mtglab data snapshot` appends today's prices to the history, and says how
// many — the runbook's daily line, and the one command that takes the pool's
// writer without rebuilding anything.
func TestASnapshotSaysHowManyPricesItKept(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t).withPool(t)
	out, err := d.run(t, "data", "snapshot")
	if err != nil {
		t.Fatalf("data snapshot: %v", err)
	}
	if !strings.Contains(out, "snapshotted ") || !strings.Contains(out, "prices for today") {
		t.Errorf("the snapshot said %q", out)
	}
	// A second snapshot on the same day is the same day's row rewritten, not a
	// failure: the runbook's line runs on a timer and a retry must be safe.
	if _, err := d.run(t, "data", "snapshot"); err != nil {
		t.Errorf("a second snapshot on the same day failed: %v", err)
	}
}

// The Claude ledger, in the three states a box can be in: never asked, asked
// and empty in this window, and unreadable.
//
// **An empty window and an unreadable ledger are different sentences**, and
// only one of them is true when the file is broken — the same distinction the
// roster draws, and the reason `no ledger at` is worded as a fact about the
// path rather than about the spend.
func TestTheSpendReportTellsEmptyApartFromUnreadable(t *testing.T) {
	t.Parallel()

	// Never asked: no file at all.
	fresh := scratchDeployment(t)
	out, err := fresh.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("claude usage on a fresh box: %v", err)
	}
	if !strings.Contains(out, "no ledger at ") ||
		!strings.Contains(out, "has asked Claude anything yet") {
		t.Errorf("a fresh box reads:\n%s", out)
	}

	// Asked and empty: the ladder has run, nothing was recorded.
	seeded := scratchDeployment(t)
	if _, err := seeded.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatalf("seeding app.db: %v", err)
	}
	out, err = seeded.run(t, "claude", "usage")
	if err != nil {
		t.Fatalf("claude usage over an empty ledger: %v", err)
	}
	if !strings.Contains(out, "nothing recorded in that window") {
		t.Errorf("an empty ledger reads:\n%s", out)
	}
	// And the window is named, because "nothing recorded" without one is a
	// claim about all of history rather than about what was asked.
	out, err = seeded.run(t, "claude", "usage", "--since", "2026-09-01")
	if err != nil {
		t.Fatalf("claude usage --since: %v", err)
	}
	if !strings.Contains(out, "from 2026-09-01") {
		t.Errorf("the report did not name its window:\n%s", out)
	}

	// Unreadable: the rows are gone and the file is still there.
	hollowed(t, seeded, "claude_usage")
	if _, err := seeded.run(t, "claude", "usage"); err == nil {
		t.Error("a ledger with no rows table reported a spend anyway")
	}

	// And a path that is there and is not a database at all.
	broken := scratchDeployment(t)
	if err := os.MkdirAll(broken.AppDBPath(), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.run(t, "claude", "usage"); err == nil {
		t.Error("a ledger that is a directory reported a spend")
	}
}

// One conversation is a conversation; two are conversations. Small, and the
// sort of thing a report gets wrong forever.
func TestACountAgreesWithItsNoun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "conversations"},
		{1, "conversation"},
		{2, "conversations"},
		{99, "conversations"},
	} {
		if got := plural(tc.n, "conversation"); got != tc.want {
			t.Errorf("plural(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// A card's stats line says power/toughness for a creature, loyalty for a
// planeswalker, and nothing at all for everything else — which is most cards,
// and the case a `%s` on a nil pointer would render as `<nil>`.
func TestACardsStatsLineSaysOnlyWhatTheCardHas(t *testing.T) {
	t.Parallel()
	power, toughness, loyalty := "4", "4", "5"
	for _, tc := range []struct {
		what string
		rec  pool.CardRecord
		want string
	}{
		{"a creature", pool.CardRecord{Power: &power, Toughness: &toughness}, "   4/4"},
		{"a planeswalker", pool.CardRecord{Loyalty: &loyalty}, "   loyalty 5"},
		{"an artifact", pool.CardRecord{}, ""},
		// Half a creature is not a creature: a card with a power and no
		// toughness renders nothing rather than "4/<nil>".
		{"half of one", pool.CardRecord{Power: &power}, ""},
	} {
		rec := tc.rec
		if got := cardStats(&rec); got != tc.want {
			t.Errorf("%s reads %q, want %q", tc.what, got, tc.want)
		}
	}
}

// The reconciliation refuses a path that is not a database, rather than
// carrying on with an instance nobody administers.
func TestAnAppDBThatIsNotOneFailsTheReconciliation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, AdminEmail: "keeper@example.com"}
	if err := os.MkdirAll(cfg.AppDBPath(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ensureMaintainerAtBoot(cfg); err == nil {
		t.Error("a directory wearing app.db's name reconciled a maintainer")
	}
}
