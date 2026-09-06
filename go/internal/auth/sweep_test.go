package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"
)

// discardLog keeps the sweeps' own narration out of the test output.
func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The sweeper: the three purges finally have a caller, and these hold the
// caller to the shape Aaron said yes to -- one sweep on boot, then daily,
// logging what it removed, stopping with the process. The purges' own
// correctness lives with them (`accounts_test.go`, `errorpaths_test.go`);
// what is proven here is that the sweep drives all three against real rows,
// that its clock is the injectable kind the night runner established, and
// that nothing of it outlives a Stop.

// seedSweepables writes one doomed row and one live row into each of the
// three tables, through the package's own write paths wherever one exists,
// and returns a probe that reports (doomed, live) rows remaining per table.
func seedSweepables(t *testing.T, db *sql.DB) func() (doomed, live int64) {
	t.Helper()
	ctx := context.Background()
	ada := mustCreate(t, db, "ada", "ada@example.com", false)

	// Sessions: one long lapsed, one current.
	if _, err := createSession(ctx, db, ada.ID, -time.Hour); err != nil {
		t.Fatal(err)
	}
	liveSession, err := CreateSession(ctx, db, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Links: an expired unused reset, and a live invite.
	if _, err := issueToken(ctx, db, ada.ID, PurposeReset, -time.Hour); err != nil {
		t.Fatal(err)
	}
	liveInvite, err := IssueToken(ctx, db, ada.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	// Rate-limit windows: one from the day before yesterday, one just spent.
	// No write path backdates a window -- time is the only thing that ages
	// one -- so the stale row is aged by hand in the recorded schema's table.
	if _, err := RecordFailure(ctx, db, AccountKey("ada"), PerAccount); err != nil {
		t.Fatal(err)
	}
	stale := isoAt(time.Now().UTC().Add(-2 * KeepLimitsFor))
	if _, err := db.ExecContext(ctx,
		"INSERT INTO login_attempts (key, window_start, failures) VALUES (?, ?, ?)",
		"user:cobweb", stale, 3); err != nil {
		t.Fatal(err)
	}

	return func() (int64, int64) {
		count := func(query string, args ...any) int64 {
			var n int64
			if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		doomed := count("SELECT count(*) FROM sessions WHERE expires_at <= ?", nowISO()) +
			count("SELECT count(*) FROM auth_tokens WHERE used_at IS NULL AND expires_at <= ?", nowISO()) +
			count("SELECT count(*) FROM login_attempts WHERE key = ?", "user:cobweb")
		var live int64
		if s, err := LookupTouching(ctx, db, liveSession); err == nil && s != nil {
			live++
		}
		if _, err := LookupToken(ctx, db, liveInvite, PurposeInvite); err == nil {
			live++
		}
		live += count("SELECT count(*) FROM login_attempts WHERE key = ?", AccountKey("ada"))
		return doomed, live
	}
}

func TestASweepTakesTheLapsedRowsAndSparesTheLiving(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)
	remaining := seedSweepables(t, db)
	if doomed, _ := remaining(); doomed != 3 {
		t.Fatalf("the seed left %d doomed rows, want 3", doomed)
	}

	s := NewSweeper(SweeperConfig{DB: db, Log: discardLog()})
	got := s.SweepOnce(context.Background())
	if got.Sessions != 1 || got.Tokens != 1 || got.Limits != 1 {
		t.Fatalf("the sweep counted %+v, want one row from each table", got)
	}
	doomed, live := remaining()
	if doomed != 0 {
		t.Errorf("%d lapsed rows survived the sweep", doomed)
	}
	if live != 3 {
		t.Errorf("only %d of the 3 live rows survived the sweep", live)
	}
}

func TestTheBootSweepHasRunByTheTimeStartReturns(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)
	remaining := seedSweepables(t, db)

	swept := make(chan SweepCounts, 16)
	s := NewSweeper(SweeperConfig{DB: db, Log: discardLog(), Interval: time.Hour,
		Swept: func(c SweepCounts) { swept <- c }})
	s.Start()
	defer s.Stop()

	// Synchronous on purpose: "one sweep on boot" means a door that has
	// started its sweeper stands over a database already swept, so the boot
	// sweep is a fact by now rather than an event to await.
	select {
	case c := <-swept:
		if c.Sessions != 1 || c.Tokens != 1 || c.Limits != 1 {
			t.Fatalf("the boot sweep counted %+v", c)
		}
	default:
		t.Fatal("Start returned before the boot sweep ran")
	}
	if doomed, _ := remaining(); doomed != 0 {
		t.Fatalf("%d lapsed rows survived the boot sweep", doomed)
	}
}

func TestTheSweepKeepsItsCadenceAndStopsClean(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)

	// The injectable interval is the fake clock here, the same trade the
	// night runner's tests make: a day becomes milliseconds, and the test
	// awaits sweep events rather than reading a wall clock.
	//
	// **The send may not block**, which is the one place this test differs
	// from the night runner's. A five-millisecond cadence can outrun a reader
	// on a saturated machine, and a callback parked on a full channel parks
	// the sweeper's own goroutine -- the goroutine `Stop` then waits for
	// forever. That is a hang rather than a flake, so the event is dropped
	// instead: what is asked below is that ticks keep arriving and then stop,
	// and neither question needs every one of them.
	swept := make(chan SweepCounts, 64)
	s := NewSweeper(SweeperConfig{DB: db, Log: discardLog(), Interval: 5 * time.Millisecond,
		Swept: func(c SweepCounts) {
			select {
			case swept <- c:
			default:
			}
		}})
	// Awaited with a ceiling rather than read bare: a sweeper whose ticker
	// never starts would park a bare receive until the package's own timeout,
	// and a ten-second hang that names nothing is a worse answer than a
	// failure that says which sweep never came.
	await := func(which string) {
		t.Helper()
		select {
		case <-swept:
		case <-time.After(10 * time.Second):
			t.Fatalf("the %s sweep never arrived on a %s cadence", which, s.interval)
		}
	}
	s.Start()
	await("boot")
	await("first standing")
	await("second standing")

	// Stop waits for the loop's goroutine, so a Stop that returns is a
	// sweeper that is gone -- the same leak assertion the door makes of the
	// night runner. Twice, because a shutdown path may cross itself.
	s.Stop()
	s.Stop()

	// Nothing sweeps after a stop: drain what was in flight, then hold the
	// channel silent across several would-be ticks.
	for len(swept) > 0 {
		<-swept
	}
	select {
	case <-swept:
		t.Fatal("a sweep ran after Stop returned")
	case <-time.After(50 * time.Millisecond):
	}
}

// declaredLimits is every package-level `Limit` this package declares, read
// off `ratelimit.go`'s own syntax tree rather than typed out here.
//
// The list has to be *discovered*, because the failure this guards against is
// somebody adding a sixth budget -- and a hand-copied list is exactly the
// thing that would still name five. Parsing is the only way to ask the
// package what it declares; a `Limit` is a plain struct with no registry, and
// nothing at run time can enumerate a package's variables.
func declaredLimits(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "ratelimit.go", nil, 0)
	if err != nil {
		t.Fatalf("reading the budgets out of ratelimit.go: %v", err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, v := range value.Values {
				lit, ok := v.(*ast.CompositeLit)
				if !ok {
					continue
				}
				if id, ok := lit.Type.(*ast.Ident); ok && id.Name == "Limit" {
					names = append(names, value.Names[i].Name)
				}
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no Limit declarations found; this test is measuring nothing")
	}
	slices.Sort(names)
	return names
}

// The sweep may never take a window somebody is still inside.
//
// [PurgeStaleLimits] deletes by age alone -- it knows nothing about which
// budget a row belongs to -- so [KeepLimitsFor] has to outlast **every**
// budget in the package, not merely the longest one at the time it was
// written. The two halves below are the gate: the set of budgets is read off
// the source, held equal to the set this test evaluates (both ways, so a new
// one cannot slip past by being unlisted), and then each one's real `Window`
// is put beside the constant.
//
// What a failure means is worth stating, because it is not a tidiness
// complaint: `current` treats a lapsed window as no window at all, so a row
// older than its budget hands nothing back when it goes -- but a row deleted
// while it is still inside its budget hands a locked-out client its attempts
// back early, which is the rate limiter quietly not limiting.
func TestTheSweepOutlastsEveryBudgetWindow(t *testing.T) {
	t.Parallel()
	budgets := map[string]Limit{
		"PerAccount":      PerAccount,
		"PerAddress":      PerAddress,
		"ResetPerMailbox": ResetPerMailbox,
		"ResetPerAddress": ResetPerAddress,
		"ClaimPerAddress": ClaimPerAddress,
	}
	evaluated := make([]string, 0, len(budgets))
	for name := range budgets {
		evaluated = append(evaluated, name)
	}
	slices.Sort(evaluated)
	if declared := declaredLimits(t); !slices.Equal(declared, evaluated) {
		t.Fatalf("ratelimit.go declares\n  %v\nand this test weighs\n  %v\n"+
			"-- a budget the sweep has never been measured against is a "+
			"window it may be deleting from under somebody", declared, evaluated)
	}
	for _, name := range evaluated {
		window := budgets[name].Window
		if window <= 0 {
			t.Errorf("%s has a %s window, which no age can be older than", name, window)
			continue
		}
		// Twice over, not merely past: a row is aged from `window_start`, and
		// a margin that only just clears the budget would be one clock skew
		// from taking a live window.
		if KeepLimitsFor < 2*window {
			t.Errorf("KeepLimitsFor is %s and %s runs for %s: the sweep can "+
				"take a window a client is still inside", KeepLimitsFor, name, window)
		}
	}
}

// repoRootFrom walks up from the working directory to the checkout, which is
// how a test in `go/internal/...` reaches a document at the top of the tree.
func repoRootFrom(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, mod := os.Stat(filepath.Join(dir, "go", "go.mod"))
		_, docs := os.Stat(filepath.Join(dir, "docs", "HOSTING.md"))
		if mod == nil && docs == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no repository root above the working directory")
		}
		dir = parent
	}
}

// The runbook hands an operator one line for checking that the sweep is
// running, and that line is a `grep` for a sentence this code writes.
//
// Which makes the sentence an interface. Reword it and the recipe returns
// nothing -- and *nothing* is exactly what a sweep that never ran looks like,
// so the failure reads as a broken janitor rather than as a stale document.
// The claim is checked by running a real sweep and reading what it actually
// said, rather than by grepping this package's source for a string literal: a
// literal can be present and unreachable, which is the shape of every guard
// that passes against its own target bug.
func TestTheRunbooksSweepRecipeGrepsForWhatTheSweepSays(t *testing.T) {
	t.Parallel()
	var spoke bytes.Buffer
	s := NewSweeper(SweeperConfig{DB: newAccountsDB(t),
		Log: slog.New(slog.NewJSONHandler(&spoke, nil))})
	s.SweepOnce(context.Background())

	var line struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(spoke.Bytes(), &line); err != nil {
		t.Fatalf("the sweep said %q, which is not one line of log: %v", spoke.String(), err)
	}
	if line.Msg == "" {
		t.Fatal("the sweep announced nothing, so no recipe could find it")
	}

	runbook := filepath.Join(repoRootFrom(t), "docs", "HOSTING.md")
	body, err := os.ReadFile(runbook)
	if err != nil {
		t.Fatal(err)
	}
	// The recipe's own argument, not merely the sentence loose in the prose:
	// a document that happens to quote the phrase in a paragraph would satisfy
	// a plain substring test while its `grep` still found nothing.
	found := regexp.MustCompile(`fly logs \| grep "([^"]*)"`).FindSubmatch(body)
	if found == nil {
		t.Fatalf("docs/HOSTING.md no longer tells anyone how to see the sweep; "+
			"it logs %q and nothing points at it", line.Msg)
	}
	if got := string(found[1]); got != line.Msg {
		t.Errorf("the runbook greps for\n  %q\nand the sweep says\n  %q\n"+
			"-- the recipe finds nothing, which reads exactly like a sweep "+
			"that never ran", got, line.Msg)
	}
}

func TestAStoppedSweeperThatNeverStartedIsFine(t *testing.T) {
	t.Parallel()
	s := NewSweeper(SweeperConfig{DB: newAccountsDB(t), Log: discardLog()})
	s.Stop() // no goroutine to wait for; must not hang or panic
}

func TestASweepCutShortByItsContextGoesQuietly(t *testing.T) {
	t.Parallel()
	// A shutdown crossing a tick is not a fault: the sweep stops where it
	// is, announces nothing -- no counts, no finished-sweep event -- and the
	// rows keep until the next boot's sweep.
	db := newAccountsDB(t)
	remaining := seedSweepables(t, db)
	fired := false
	var spoke bytes.Buffer
	s := NewSweeper(SweeperConfig{DB: db,
		Log:   slog.New(slog.NewTextHandler(&spoke, nil)),
		Swept: func(SweepCounts) { fired = true }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := s.SweepOnce(ctx); got != (SweepCounts{}) {
		t.Fatalf("a cancelled sweep reported %+v", got)
	}
	if fired {
		t.Fatal("a cancelled sweep announced itself as finished")
	}
	// Quietly means quietly: no "failed" line about a cancellation that was
	// ordered, and no swept line for a sweep that did not happen.
	if spoke.Len() != 0 {
		t.Fatalf("a cancelled sweep spoke: %s", spoke.String())
	}
	if doomed, _ := remaining(); doomed != 3 {
		t.Fatalf("%d doomed rows remain after a cancelled sweep, want all 3 untouched", doomed)
	}
}
