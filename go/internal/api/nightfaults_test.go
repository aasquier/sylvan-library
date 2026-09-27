package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/night"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// What the night does when the *instance* is the thing that is wrong.
//
// `night_test.go` drives the night as it runs: a bout played, a pre-flight
// refused, a panicking core settled, the three admin routes walked end to
// end. What none of that reaches is the half of this file where the
// deployment has gone — no registry to fight in, no arena to fight in, no
// database left to read the rows out of. Every one of those is a path that
// only ever fires on a machine somebody has to go and look at, which is
// exactly when a nil dereference or a silent empty answer costs the most.
//
// Two things are asked of each. It must **say something**, because a night
// that stops with nothing written down is a night nobody can diagnose in the
// morning. And a fault of the environment must not be recorded as a fault of
// the *pairing*: ADR 46's row states carry that difference, and a `skipped`
// where a `failed` belongs quietly retires a deck that did nothing wrong.

// An instance with no job registry cannot fight a bout, and says so rather
// than reaching into a nil.
func TestABoutWithNowhereToFightItFailsInWords(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	id, err := a.playNightBout(t.Context(), night.Bout{
		ID: 1, Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}, Games: 1})
	if err == nil {
		t.Fatal("a bout with no registry reported success")
	}
	if id != 0 {
		t.Errorf("it claims match %d", id)
	}
	var skip night.Skip
	if errors.As(err, &skip) {
		t.Errorf("an instance with no registry was blamed on the pairing: %q", skip.Reason)
	}
}

// An instance with no arena is the environment's fault and not the pairing's,
// so it is a failure: a fixed arena replays the deck another night, where a
// skip would have retired it for nothing.
func TestABoutWithNoArenaFailsRatherThanSkipping(t *testing.T) {
	t.Parallel()
	// No Forge settings at all and no worker: `forgeStatus` finds neither a
	// distribution nor a JVM to name, which is the state CI runs in.
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Jobs: jobs.New(jobs.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})})
	_, err := a.playNightBout(t.Context(), night.Bout{
		ID: 2, Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}, Games: 1})
	if err == nil {
		t.Fatal("a bout with no arena reported success")
	}
	var skip night.Skip
	if errors.As(err, &skip) {
		t.Fatalf("a missing arena was recorded against the pairing: %q", skip.Reason)
	}
	if !strings.Contains(err.Error(), "arena") {
		t.Errorf("the failure reads %q and does not name the arena", err)
	}
	// And the maintainer-facing `why` — which names paths and version floors —
	// stayed out of the row. Commandment 10 applies to an admin too.
	for _, leak := range []string{"MTGLAB", "java", "jar", ".go:"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("the row's reason carries %q: %q", leak, err)
		}
	}
}

// A player's seat on an instance with no `app.db` is a failure with a
// sentence, not a panic on a nil handle.
func TestAPlayersSeatWithNoDatabaseFailsRatherThanPanicking(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DecksDir: decksDir(t)})
	owner := int64(2)
	_, _, _, err := a.nightDeck(t.Context(), night.Seat{Owner: &owner, Slug: "bobs-private"})
	if err == nil {
		t.Fatal("a player's deck was read off an instance with no database")
	}
	if !strings.Contains(err.Error(), "deck") {
		t.Errorf("the failure reads %q", err)
	}
}

// A pre-flight that broke for a reason that is *not* coverage is a failure
// too: coverage is a fact about the decks, and everything else is a fact
// about the machine.
func TestAPreflightThatBrokeIsAFailureNotASkip(t *testing.T) {
	t.Parallel()
	// A shim that refuses the coverage call outright, which is a transport
	// fault rather than `ErrCoverageFailed`.
	shim := &stubShim{failCoverage: true}
	a, _, _, _ := nightAPI(t, shim)
	_, err := a.playNightBout(t.Context(), night.Bout{
		ID: 3, Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}, Games: 1, Seed: 7})
	if err == nil {
		t.Fatal("a broken pre-flight reported success")
	}
	var skip night.Skip
	if errors.As(err, &skip) {
		t.Errorf("a broken pre-flight was recorded against the pairing: %q", skip.Reason)
	}
}

// The runner stopping mid-bout: the waiter gives up with the context's own
// error and claims no match, while the job fights on in the registry — the
// seam's documented shape, and the reason the row stays `playing` for the
// next boot's sweep rather than being settled by a waiter that left.
func TestAStoppingRunnerLeavesTheBoutToTheNextBootsSweep(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 5421, 1, 11)}}
	a, reg, _, _ := nightAPI(t, shim)

	release := make(chan struct{})
	started := make(chan struct{})
	a.playCore = func(jobs.Progress, forgeMatch) (forgeResult, int64, error) {
		close(started)
		<-release
		return forgeResult{}, 0, nil
	}
	ctx, stop := context.WithCancel(context.Background())
	type answer struct {
		id  int64
		err error
	}
	got := make(chan answer, 1)
	go func() {
		id, err := a.playNightBout(ctx, night.Bout{ID: 4,
			Seats: []night.Seat{{Slug: "kaheera"}, {Slug: "mono-green"}}, Games: 1, Seed: 8})
		got <- answer{id, err}
	}()
	<-started
	stop()
	select {
	case o := <-got:
		if !errors.Is(o.err, context.Canceled) {
			t.Errorf("a stopping runner heard %v, want the context's own error", o.err)
		}
		if o.id != 0 {
			t.Errorf("an abandoned bout claims match %d", o.id)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the waiter never gave up; a stopping runner would hang on shutdown")
	}
	close(release)
	reg.Wait()
}

// The three admin routes against a night whose rows have gone: every one of
// them refuses with a sentence, and none of them answers as though the night
// were simply empty. "No night has run yet" over a database that will not
// answer is the lie `closeddb_test.go` is about, one room along.
func TestTheNightRoutesRefuseALedgerThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	a, _, runner, _ := nightAPI(t, nil)
	if err := runner.Store().Close(); err != nil {
		t.Fatal(err)
	}

	for _, ask := range []struct{ method, target, body string }{
		{"GET", "/api/admin/night", ""},
		{"POST", "/api/admin/night/close", `{}`},
		{"POST", "/api/admin/night/sample", `{"minutes":5}`},
	} {
		status, payload, raw := callAs(t, a, alice, ask.method, ask.target, ask.body)
		if status == http.StatusOK || status == http.StatusCreated {
			t.Errorf("%s %s answered %d over a night that cannot be read: %s",
				ask.method, ask.target, status, raw)
			continue
		}
		if status == http.StatusNotFound {
			t.Errorf("%s %s answered 404 -- \"no night has run\" is a different "+
				"sentence from \"the rows cannot be read\": %s", ask.method, ask.target, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
			t.Errorf("%s %s answered %d with nothing a person could read: %s",
				ask.method, ask.target, status, raw)
		}
	}
}

// A night whose run is open and whose bouts have gone: the watching read
// refuses rather than reporting an hour with nothing in it, which would read
// as a night that dealt no cards.
func TestTheWatchingReadRefusesWhenTheBoutsCannotBeRead(t *testing.T) {
	t.Parallel()
	a, _, _, dbPath := nightAPI(t, nil)

	if status, _, raw := callAs(t, a, alice, "POST", "/api/admin/night/sample",
		`{"minutes":60}`); status != http.StatusCreated {
		t.Fatalf("the sample answered %d: %s", status, raw)
	}
	// The run row survives; the bouts do not. Nothing in the app does this --
	// it is a half-applied restore, and the point is that the read notices.
	db, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`DROP TABLE night_bouts`); err != nil {
		t.Fatal(err)
	}

	status, payload, raw := as(t, a, alice, "/api/admin/night")
	if status == http.StatusOK {
		t.Fatalf("a night whose bouts cannot be read answered 200: %s", raw)
	}
	if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
		t.Fatalf("the refusal carries nothing a person could read: %s", raw)
	}
}
