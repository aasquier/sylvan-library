package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
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
// is specific: the ladder, the configuration summary, the maintainer
// reconciliation, the door, the listener and the shutdown are six things in a
// fixed order, and an ordering mistake among them is a boot that half works.
// The deployed instance is where those were found the first four times
// (CLAUDE.md's "faults live below seams"), which is exactly the loop worth
// closing here.
//
// The signal path is driven for real rather than simulated. **The test
// installs its own `signal.Notify` first**, so the runtime can never take
// SIGTERM's default action and kill the test binary if `serve` returns before
// its own handler is up.

// atoi is a port number as an int, failing the test rather than the boot.
func atoi(t *testing.T, port string) int {
	t.Helper()
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// freePort takes an ephemeral port and gives it straight back, so the caller
// can name it. A race in principle; in practice the kernel does not hand the
// same port out twice in the microseconds between.
// **A free port is not a held one.** `:0` asks the kernel for an unused port
// and this hands it back the moment it has the number, so between the close
// here and the bind in whatever is about to use it, anything on the machine
// may take it — and `go test ./...` runs forty-odd test binaries at once, most
// of them standing up `httptest.Server`s out of the same ephemeral range.
//
// That is a race with no fix at this level, which is why nothing here trusts
// one port: [bootServer] and [bootShim] retry on a fresh one, and both watch
// the boot for a bind failure rather than waiting out a timeout that would
// report the wrong thing. **A test that reports "the server never answered"
// when the truth is "the port was taken" sends the next person after a bug
// that is not there** — this failed exactly once on CI, on amd64 only, and
// said the former.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
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
	resp, err := waitForHealth(t, base+"/api/health")
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
	case <-time.After(25 * time.Second):
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

// bootServer starts `serve` on a port nothing else is holding and returns once
// it is listening. Retries on a fresh port when the bind loses the race
// described on [freePort], and fails loudly on any other refusal — a boot that
// died for a real reason must not be reported as a boot that was slow.
func bootServer(t *testing.T, d deployment) (port, base string, done chan error) {
	t.Helper()
	for attempt := range 4 {
		port = freePort(t)
		base = "http://127.0.0.1:" + port
		done = make(chan error, 1)
		go func() {
			done <- serve(d.Config, tier3.Settings{}, "127.0.0.1", port, t.TempDir(), t.TempDir())
		}()
		if err := listening(base + "/api/health"); err == nil {
			return port, base, done
		}
		// It is not answering. Either the bind lost the port, in which case
		// `serve` has already returned and we take a new one, or it is simply
		// still coming up -- and then `done` is empty and we let it be.
		select {
		case err := <-done:
			if err != nil && strings.Contains(err.Error(), "listen on") {
				t.Logf("boot attempt %d lost the port (%v); taking another", attempt+1, err)
				continue
			}
			t.Fatalf("the boot ended for a reason that is not the port: %v", err)
		default:
			return port, base, done
		}
	}
	t.Fatal("four ports in a row were taken between the pick and the bind")
	return "", "", nil
}

// bootShim is [bootServer] for the worker's door: the same race, the same
// retry, and the same refusal to report a lost port as a slow boot.
//
// **Zero idle seconds means never stop**, which is what a laptop running the
// shim by hand wants -- and what keeps this test from having its own process
// exited out from under it by the watchdog.
func bootShim(t *testing.T, ctx context.Context) string {
	t.Helper()
	for attempt := range 4 {
		port := freePort(t)
		forge := tier3.Settings{
			ShimHost: "127.0.0.1", ShimPort: atoi(t, port),
			Home: t.TempDir(), IdleSeconds: 0,
		}
		done := make(chan error, 1)
		go func() { done <- serveShim(ctx, forge) }()

		base := "http://127.0.0.1:" + port
		if err := listening(base + "/healthz"); err == nil {
			return base
		}
		select {
		case err := <-done:
			t.Logf("shim attempt %d lost the port (%v); taking another", attempt+1, err)
			continue
		default:
			return base
		}
	}
	t.Fatal("four ports in a row were taken between the pick and the bind")
	return ""
}

// listening polls until something answers, for about two seconds.
func listening(target string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	var err error
	for range 100 {
		var resp *http.Response
		if resp, err = client.Get(target); err == nil {
			_ = resp.Body.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}

// waitForHealth is [listening] again, keeping the response this time.
func waitForHealth(t *testing.T, target string) (*http.Response, error) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	var err error
	for range 100 {
		if resp, err = client.Get(target); err == nil {
			return resp, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, err
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
	err := serve(unmounted, tier3.Settings{}, "127.0.0.1", freePort(t), t.TempDir(), t.TempDir())
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
	base := bootShim(t, ctx)

	resp, err := waitForHealth(t, base+"/healthz")
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
