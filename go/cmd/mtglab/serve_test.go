package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The boot itself, and the health probe the container asks it with.
//
// `serve` is the single most important function in the binary -- it is the
// whole app coming up -- and it had never been run by a test. What that costs
// is specific: the signal handler, the ladder, the configuration summary, the
// maintainer reconciliation, the door, the listener and the shutdown are seven
// things in a fixed order, and an ordering mistake among them is a boot that
// half works. The deployed instance is where those were found the first four
// times (CLAUDE.md's "faults live below seams"), which is exactly the loop
// worth closing here.
//
// The signal path is driven for real rather than simulated. **The test
// installs its own `signal.Notify` first**, so the runtime can never take
// SIGTERM's default action and kill the test binary if `serve` returns before
// its own handler is up.
//
// **And that ordering was the bug, one function over.** `serveOn` used to arm
// its own handler three statements *after* it started serving, so between the
// bind and that call the process was reachable and deaf: a SIGTERM landing
// there was taken by the runtime's default action, which is to kill the
// process. On CI this test hung on it five times in a single day, on branches
// that touched no server code at all. The signature was the same every time
// and misread every time — a FAIL at the test's whole budget, and printed
// above it an ERROR reading `accept tcp ...: use of closed network
// connection`. That ERROR looks like a shutdown that started and stalled. It
// is the opposite: `slog` writes straight to an unbuffered `os.Stderr` while
// `testing` holds a test's own output until the test ends, so the two lines
// appear in the wrong order, and the line is `Serve` returning because
// [heldPort]'s **cleanup** closed the listener after `t.Fatal`. Read the
// embedded `time=` fields instead and the gap is the test's budget exactly.
// The goroutine dump settled it in one look: `serveOn` parked in its select,
// `Serve` still in `Accept`, and nothing anywhere inside `Shutdown`. Which is
// why the timeout branch below now takes that dump itself.

// atoi is a port number as an int, failing the test rather than the boot.
func atoi(t *testing.T, port string) int {
	t.Helper()
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// heldPort takes an ephemeral port and **keeps** it, handing back the
// listener itself so the caller can give it to whatever is about to serve on
// it.
//
// **A free port is not a held one**, and that is what this replaces. The old
// helper asked the kernel for an unused port with `:0` and gave it straight
// back the moment it had the number, so between that close and the bind in
// whatever was about to use it, anything on the machine could take it — and
// `go test ./...` runs forty-odd binaries at once, most of them standing up
// `httptest.Server`s out of the same ephemeral range.
//
// The window was neither theoretical nor small, and the retry that used to
// stand here was covering the wrong half of it. Measured on this machine, the
// boot took 0.2s to reach its bind when idle and 5.3s under load, while the
// probe waiting for it gave up after a flat ~2.2s — 100 sleeps of 20ms, a
// wall clock that a starved CPU does not stretch. **The boot's cost is
// unbounded and the budget was a constant**, so the busier the machine the
// likelier the budget lost; and because nothing held the port meanwhile,
// every attempt fast-failed with ECONNREFUSED instead of waiting. The failure
// it produced said `the server never answered`, which blames the server for a
// race in the test, and it cost a green `main` a deploy (ADR 23) when the job
// it failed was the one gating one.
//
// Holding the port closes both halves at once: nothing can take it, and a
// probe against a bound port is accepted into the backlog immediately and
// then **waits for the boot rather than racing it**. No timeout moved.
//
// **That closed the boot half, and only the boot half** — which this comment
// used to read as though it had closed the bug class. It had not. The stop
// half kept its own flat wall clock and went on failing on exactly the shape
// described above: a constant in a test standing against something in the
// product that no constant bounds.
//
// The difference between the halves is worth stating, because it decided both
// fixes. A boot's cost has no ceiling anywhere in the product, so no number in
// a test could ever have been the right one and the clock had to go. A
// **stop's** cost does have a ceiling in the product — `shutdownGrace` — so a
// clock is honest there, provided it is derived from that ceiling rather than
// guessed a few seconds above it.
// [TestTheServerBootsAnswersAndStopsOnASignal] does that now.
func heldPort(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Whoever serves on it closes it; this is for the paths that never get
	// that far, and a second close is not a failure worth reporting.
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// portOf is the port half of a listener's address.
func portOf(t *testing.T, l net.Listener) string {
	t.Helper()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// The whole boot: ladder, door, listener, a real request over TCP, and a
// clean stop on SIGTERM.
func TestTheServerBootsAnswersAndStopsOnASignal(t *testing.T) {
	// **Serial**, and the audit cannot see why: it passes alone, but it sends
	// SIGTERM to the whole process. A second `serve` running beside it would
	// take the same signal and stop too, so the failure would land on the
	// other test and read as a flake there.
	d := scratchDeployment(t)

	// Ours first: the runtime's default for SIGTERM is to exit, and this
	// keeps the test binary alive whatever `serve` does with its own.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	// The ladder ran on the way up, which is the first thing the boot owes.
	port, base, done := bootServer(t, d)
	resp, err := waitForHealth(t, base+"/api/health", done)
	if err != nil {
		t.Fatalf("the server never answered on %s: %v", base, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/health answered %d: %s", resp.StatusCode, body)
	}
	if _, err := os.Stat(d.AppDBPath()); err != nil {
		t.Errorf("the boot did not run the schema ladder: %v", err)
	}

	// The probe is the container's HEALTHCHECK, and this is the only place
	// it can be asked a real question.
	if err := runProbe(t, base+"/api/health"); err != nil {
		t.Errorf("the health probe failed against a healthy server: %v", err)
	}

	// SIGTERM stops it, and the stop is clean: `serve` returns nil rather
	// than reporting the shutdown as a failure.
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a clean shutdown reported %v", err)
		}
	// **Derived from the product's own ceiling, not chosen near it.** This was
	// a flat 25 seconds against a `shutdownGrace` of 20, and five seconds is
	// not a margin — the first stop to legitimately spend its whole grace
	// draining would have decided this test by coin flip. Doubling the grace
	// means the only thing that can expire this is a stop that overran the
	// product's own budget, which is a bug worth failing for rather than a
	// flake to re-run.
	case <-time.After(2 * shutdownGrace):
		// The dump, because the four CI failures cost a day between them and
		// every one of them was diagnosed from two log lines in the wrong
		// order (see this file's opening comment). A stack says in one look
		// what is holding the stop: parked in the select is a signal that
		// never arrived, and anything inside `Shutdown` or `Door.Close` is a
		// genuinely blocked drain. They want opposite fixes.
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Logf("every goroutine at the moment the stop was given up on:\n%s", buf[:n])
		t.Fatal("the server did not stop on SIGTERM")
	}

	// And the port is free again, which is what "stopped" has to mean.
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Errorf("the port is still held after shutdown: %v", err)
	} else {
		_ = l.Close()
	}
}

// bootServer starts `serve` on a listener this test is **already holding**
// (see [heldPort]) and returns straight away: there is no port to lose between
// here and the bind, so there is nothing to poll for and nothing to retry. The
// caller waits for health, and [waitForHealth] is what watches the boot.
func bootServer(t *testing.T, d deployment) (port, base string, done chan error) {
	t.Helper()
	l := heldPort(t)
	port = portOf(t, l)
	// Taken on this goroutine: `t.TempDir` belongs to the test, not to the
	// server it is about to start.
	webDist, tarot := t.TempDir(), t.TempDir()
	done = make(chan error, 1)
	go func() {
		done <- serveOn(d.Config, tier3.Settings{}, webDist, tarot,
			func() (net.Listener, error) { return l, nil })
	}()
	return port, "http://127.0.0.1:" + port, done
}

// bootShim is [bootServer] for the worker's door: the same held listener, for
// the same reason.
//
// **Zero idle seconds means never stop**, which is what a laptop running the
// shim by hand wants -- and what keeps this test from having its own process
// exited out from under it by the watchdog.
func bootShim(t *testing.T, ctx context.Context) (base string, done chan error) {
	t.Helper()
	l := heldPort(t)
	port := portOf(t, l)
	forge := tier3.Settings{
		ShimHost: "127.0.0.1", ShimPort: atoi(t, port),
		Home: t.TempDir(), IdleSeconds: 0,
	}
	done = make(chan error, 1)
	go func() {
		done <- serveShimOn(ctx, forge, func() (net.Listener, error) { return l, nil })
	}()
	return "http://127.0.0.1:" + port, done
}

// waitForHealth polls a held port until the server behind it answers, and
// gives up only when the boot itself ends.
//
// The hundred rounds below are the same hundred as before; what changed is
// what one costs. Against an **unbound** port each is a refusal that returns
// instantly, so the hundred were 100 sleeps of 20ms — a flat ~2.2s, measured,
// and a wall clock a starved CPU does not stretch, which is how a fixed budget
// came to be racing a boot that took 0.2s idle and 5.3s loaded. Against a
// **held** port each round is instead accepted into the listen backlog and
// waits on `Serve` for up to the client's own 2s, so the same hundred span
// minutes rather than seconds. The budget is no longer the thing that decides.
//
// What decides is `done`: a boot that returned has a reason, and **reporting
// that as a server that never answered sends the next person after a bug that
// is not there**, which is exactly what this test did on #337, on `main` at
// 46474eb, and again on #341 — where it shared a job with a genuine failure and
// gave it somewhere to hide.
//
// This is therefore only honest while every caller hands over a listener it is
// already holding, which is [heldPort]'s whole job.
func waitForHealth(t *testing.T, target string, done <-chan error) (*http.Response, error) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	var err error
	for range 100 {
		if resp, err = client.Get(target); err == nil {
			return resp, nil
		}
		select {
		case bootErr := <-done:
			if bootErr == nil {
				// Its own fault, and a stranger one: nothing refused, and
				// nothing served either.
				return nil, errors.New("the boot returned cleanly without ever answering")
			}
			return nil, fmt.Errorf("the boot ended before it answered: %w", bootErr)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, err
}

// **The fix, in one assertion.** The address [heldPort] hands back is already
// bound, so there is no instant between choosing it and serving on it in which
// anything else could take it.
//
// This is the guard the flake never had, and it fails against the helper it
// replaces rather than merely passing against the new one: run the same
// `net.Listen` at `freePort`'s old return and it **succeeds every time** —
// measured 5 of 5 — because a port that has just been closed is by
// construction available to whoever asks next. That is not a rare race that
// needs a loaded machine to show; it is the helper's whole contract, and the
// only reason it did not fail constantly is that the thief usually never came.
func TestAHeldPortIsHeld(t *testing.T) {
	t.Parallel()
	l := heldPort(t)
	second, err := net.Listen("tcp", "127.0.0.1:"+portOf(t, l))
	if err == nil {
		_ = second.Close()
		t.Fatal("the address was free to take, so the boot it is handed to is racing for it")
	}
}

// **The stop half's fix, in one assertion**, and the sibling of
// [TestAHeldPortIsHeld] above: a stop that arrives while the process is still
// coming up is heard, because the handler is armed before the boot rather than
// after it.
//
// This is not a timing test and it does not want a loaded machine. The seam it
// uses is the listener callback — the instant `serveOn` reaches for the port —
// and it does two things there. It sends the process a SIGTERM, and then it
// **waits on the test's own guard channel** before returning the listener.
// That second half is what makes this a fact rather than a race: the runtime
// hands a signal to every channel registered at the moment it dispatches, so
// once the guard has it, dispatch has happened, and a `signal.Notify` that has
// not run by then has missed the signal for good.
//
// So the two orderings give two different outcomes with nothing in between.
// Armed before the bind, `serveOn` already holds the stop in its buffer, and
// finds it waiting the moment it reaches the select. Armed after, as it was,
// the stop is gone: measured 3 of 3 against the old shape, where the server
// went on serving until this test's own cleanup pulled the listener out from
// under it — which is precisely the shape CI kept failing in.
//
// The budget below is not racing anything. The signal is delivered before the
// ladder even runs, so what is being waited for is a boot and an immediate
// shutdown with no connection open to drain.
func TestAStopArrivingDuringTheBootIsNotLost(t *testing.T) {
	// **Serial**, for [TestTheServerBootsAnswersAndStopsOnASignal]'s reason:
	// the SIGTERM goes to the whole process, so any other `serve` alive beside
	// it would stop too and the failure would land on that test instead.
	d := scratchDeployment(t)

	// Ours first, as ever -- without it the runtime's default action ends the
	// test binary, which is the production behaviour this test is about.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	l := heldPort(t)
	// Taken here rather than in the goroutine: `t.TempDir` belongs to the test.
	webDist, tarot := t.TempDir(), t.TempDir()
	done := make(chan error, 1)
	go func() {
		done <- serveOn(d.Config, tier3.Settings{}, webDist, tarot,
			func() (net.Listener, error) {
				if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
					return nil, err
				}
				<-guard
				return l, nil
			})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a stop taken during the boot reported %v", err)
		}
	case <-time.After(2 * shutdownGrace):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Logf("every goroutine at the moment the stop was given up on:\n%s", buf[:n])
		t.Fatal("a stop that arrived while the process was binding was never heard")
	}
}

// A boot slower than the probe's old patience is **waited for**, not called a
// server that never answered.
//
// The old shape gave the wait a fixed budget — 100 sleeps of 20ms against an
// unbound port, a flat ~2.2s measured, which a busy CPU does not stretch
// because sleeping needs none of it — while the boot behind it cost 0.2s idle
// and 5.3s under load. A constant racing something unbounded loses eventually,
// and it lost on GitHub's runners three times in one evening. Holding the port
// is what removes the clock: the connection is accepted the moment it is made
// and the request waits for `Serve` to reach it.
//
// The delay here is past that whole old budget on purpose. Nothing sleeps in
// the fix; the sleep is this test standing in for a slow machine.
func TestASlowBootIsWaitedForRatherThanCalledDead(t *testing.T) {
	t.Parallel()
	l := heldPort(t)
	go func() {
		time.Sleep(3 * time.Second)
		_ = http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}()
	// A boot that has not returned: the wait has no reason to stop early.
	resp, err := waitForHealth(t, "http://"+l.Addr().String()+"/api/health", make(chan error))
	if err != nil {
		t.Fatalf("a boot that was merely slow was reported as no server at all: %v", err)
	}
	_ = resp.Body.Close()
}

// And the other half: a boot that **ended** is reported as a boot that ended,
// naming its reason, rather than polled at until the budget runs out and then
// blamed on a server that never answered. That mistake is what hid a real
// failure behind a familiar-looking flake on #341.
func TestABootThatDiedIsNotReportedAsASlowOne(t *testing.T) {
	t.Parallel()
	l := heldPort(t)
	done := make(chan error, 1)
	done <- errors.New("the ladder could not be applied")

	_, err := waitForHealth(t, "http://"+l.Addr().String()+"/api/health", done)
	if err == nil {
		t.Fatal("a boot that had already ended was reported as healthy")
	}
	if !strings.Contains(err.Error(), "the ladder could not be applied") {
		t.Errorf("the failure said %q without naming why the boot ended", err)
	}
}

// A port already in use is a refusal that names the address, because the
// operator's next move is to find what is holding it.
func TestABusyPortIsRefusedByName(t *testing.T) {
	t.Parallel()

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	_, port, err := net.SplitHostPort(held.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	err = serve(scratchDeployment(t).Config, tier3.Settings{},
		"127.0.0.1", port, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("the server bound a port somebody else was holding")
	}
	if !strings.Contains(err.Error(), "listen on") || !strings.Contains(err.Error(), port) {
		t.Errorf("the refusal said %q without naming the address", err)
	}
}

// A ladder that cannot be applied is a **refusal to serve**, not a warning: a
// request answered over a half-migrated file is worse than no answer at all.
func TestAnUnrunnableLadderRefusesToServe(t *testing.T) {
	t.Parallel()
	// A data directory that is not there and cannot be made.
	unmounted := config.Config{
		DataDir:  "/nonexistent/never-mounted",
		DecksDir: "/nonexistent/never-mounted/decks",
	}
	// Port "0" and no port at all are the same thing here: the ladder fails
	// above the listener, so nothing is ever bound — which is the assertion
	// underneath this one, and the reason this needs no port of its own.
	err := serve(unmounted, tier3.Settings{}, "127.0.0.1", "0", t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("a boot with no writable volume served anyway")
	}
}

// The probe is the image's HEALTHCHECK: the runtime image carries nothing but
// the binary, so the binary asks after its own health. A 2xx is health and
// everything else is not -- including a server that is simply not there.
func TestTheHealthProbeAnswersOnlyToATwoHundred(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		code int
		ok   bool
	}{
		{"200", http.StatusOK, true},
		{"204", http.StatusNoContent, true},
		{"299", 299, true},
		{"301", http.StatusMovedPermanently, false},
		{"404", http.StatusNotFound, false},
		{"500", http.StatusInternalServerError, false},
		{"503", http.StatusServiceUnavailable, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, "a body the probe must drain")
			}))
			defer srv.Close()

			err := runProbe(t, srv.URL)
			if tc.ok && err != nil {
				t.Errorf("a %d was reported unhealthy: %v", tc.code, err)
			}
			if !tc.ok {
				if err == nil {
					t.Errorf("a %d was reported healthy", tc.code)
				} else if !strings.Contains(err.Error(), srv.URL) {
					t.Errorf("the failure said %q without naming the URL", err)
				}
			}
		})
	}

	// A server that is not there at all is unhealthy rather than a panic --
	// this is the shape during a deploy, and the container reads the exit
	// code.
	if err := runProbe(t, "http://127.0.0.1:1/api/health"); err == nil {
		t.Error("a probe of nothing reported health")
	}
}

// With no URL at all the probe asks the container's own default, which is
// the only form the HEALTHCHECK line uses.
func TestTheProbeHasADefaultURL(t *testing.T) {
	t.Parallel()
	cmd := probeCommand()
	if !cmd.Hidden {
		t.Error("the probe is plumbing for the container, not a command anybody runs")
	}
	// Nothing is listening on the container's default here, so what is
	// pinned is that it asks *something* rather than refusing for want of an
	// argument.
	cmd.SetArgs(nil)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	if err := cmd.Execute(); err == nil {
		t.Skip("something is listening on the container's default port")
	} else if strings.Contains(err.Error(), "accepts") {
		t.Errorf("the probe refused to run without an argument: %v", err)
	}
}

// runProbe drives the probe command the way the container does.
func runProbe(t *testing.T, url string) error {
	t.Helper()
	cmd := probeCommand()
	cmd.SetArgs([]string{url})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	return cmd.Execute()
}

// The Forge shim's own listener, driven the same way: it comes up, answers
// its health route, and is not holding a JVM to do it.
func TestTheShimListensAndAnswersItsHealthRoute(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	base, done := bootShim(t, ctx)

	resp, err := waitForHealth(t, base+"/healthz", done)
	if err != nil {
		cancel()
		t.Fatalf("the shim never answered on %s: %v", base, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz answered %d: %s", resp.StatusCode, body)
	}
	// It answers even though this machine has no Forge, which is the whole
	// point: /healthz answering is what lets the app read the 503s.
	if !strings.Contains(string(body), "true") {
		t.Errorf("/healthz answered %s", body)
	}
	cancel()
}

// A shim that cannot bind says so rather than exiting silently.
func TestAShimThatCannotBindSaysSo(t *testing.T) {
	t.Parallel()
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	_, port, err := net.SplitHostPort(held.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	if err := serveShim(context.Background(), tier3.Settings{
		ShimHost: "127.0.0.1", ShimPort: atoi(t, port), Home: t.TempDir(),
	}); err == nil {
		t.Error("the shim bound a port somebody else was holding")
	} else if !strings.Contains(err.Error(), "forge shim") {
		t.Errorf("the refusal said %q", err)
	}
}

// The root command wires every family, so a subcommand that stopped being
// registered would be a command that vanished from the runbook without
// anything failing.
func TestEveryFamilyIsWiredIntoTheRootCommand(t *testing.T) {
	t.Parallel()
	// The real tree, not a copy of it: a second `AddCommand` list beside
	// [newRoot] is a list that can drift, which is the thing this test exists
	// to catch.
	root := newRoot(config.Config{}, tier3.Settings{}, claude.Endpoint{})
	got := map[string]bool{}
	for _, c := range root.Commands() {
		got[c.Name()] = true
	}
	for _, want := range []string{
		"ui", "data", "users", "decks", "sim", "cards", "claude",
		"forge-shim", "probe",
	} {
		if !got[want] {
			t.Errorf("`mtglab %s` is not wired into the root command", want)
		}
	}
}
