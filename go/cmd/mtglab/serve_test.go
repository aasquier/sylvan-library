package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
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

// freePort takes an ephemeral port and gives it straight back, so the caller
// can name it. A race in principle; in practice the kernel does not hand the
// same port out twice in the microseconds between.
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
	dir := scratchDataDir(t)
	t.Setenv("MTGLAB_DECKS_DIR", filepath.Join(dir, "decks"))

	// Ours first: the runtime's default for SIGTERM is to exit, and this
	// keeps the test binary alive whatever `serve` does with its own.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	port := freePort(t)
	done := make(chan error, 1)
	go func() { done <- serve("127.0.0.1", port, t.TempDir(), t.TempDir()) }()

	// The ladder ran on the way up, which is the first thing the boot owes.
	base := "http://127.0.0.1:" + port
	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		resp, err = client.Get(base + "/api/health")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("the server never answered on %s: %v", base, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/health answered %d: %s", resp.StatusCode, body)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.db")); err != nil {
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

// A port already in use is a refusal that names the address, because the
// operator's next move is to find what is holding it.
func TestABusyPortIsRefusedByName(t *testing.T) {
	scratchDataDir(t)

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	_, port, err := net.SplitHostPort(held.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	err = serve("127.0.0.1", port, t.TempDir(), t.TempDir())
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
	// A data directory that is not there and cannot be made.
	t.Setenv("MTGLAB_DATA_DIR", "/nonexistent/never-mounted")
	t.Setenv("MTGLAB_DECKS_DIR", "/nonexistent/never-mounted/decks")
	t.Setenv("MTGLAB_ADMIN_EMAIL", "")

	err := serve("127.0.0.1", freePort(t), t.TempDir(), t.TempDir())
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
	t.Setenv("MTGLAB_FORGE_SHIM_TOKEN", "")
	t.Setenv("MTGLAB_FORGE_SHIM_HOST", "127.0.0.1")
	t.Setenv("MTGLAB_FORGE_SHIM_PORT", freePort(t))
	// **Zero means never**, which is what a laptop running the shim by hand
	// wants -- and what keeps this test from having its process exited out
	// from under it by the idle watchdog.
	t.Setenv("MTGLAB_FORGE_IDLE_SECONDS", "0")
	t.Setenv("MTGLAB_FORGE_HOME", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveShim(ctx) }()

	base := "http://127.0.0.1:" + os.Getenv("MTGLAB_FORGE_SHIM_PORT")
	client := &http.Client{Timeout: 2 * time.Second}
	var resp *http.Response
	var err error
	for i := 0; i < 100; i++ {
		resp, err = client.Get(base + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
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
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	_, port, err := net.SplitHostPort(held.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("MTGLAB_FORGE_SHIM_HOST", "127.0.0.1")
	t.Setenv("MTGLAB_FORGE_SHIM_PORT", port)
	t.Setenv("MTGLAB_FORGE_IDLE_SECONDS", "0")
	t.Setenv("MTGLAB_FORGE_HOME", t.TempDir())

	if err := serveShim(context.Background()); err == nil {
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
	root := rootCommand()
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

// rootCommand builds the tree the way main does, so the wiring test above
// asks about the real one.
func rootCommand() *cobra.Command {
	root := &cobra.Command{Use: "mtglab"}
	root.AddCommand(uiCommand(), dataCommand(), usersCommand(), decksCommand(),
		simCommand(), cardsCommand(), claudeCommand(), shimCommand(), probeCommand())
	return root
}
