package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The boot on a machine where something is wrong, and the rule it follows.
//
// **Warnings, never a refusal — except where a refusal is the whole point.**
// Merging deploys (ADR 23), so a boot that refused over a setting the site
// does not need to serve an anonymous page would take the site down for
// nothing. Two things are exceptions and both are argued where they stand: a
// schema ladder that cannot be applied (a request answered over a half-migrated
// file is worse than no answer) and a misconfigured night (a scheduler that
// would quietly run on the wrong clock at an hour nobody is watching).
//
// What is tested here is the line between them. A database the ladder skips
// and the app cannot read is a **degraded** boot that serves anonymous, so
// somebody can sign in past it once the disk is fixed; a maintainer that
// cannot be reconciled over that same database is a **refusal**, because ADR
// 17 is the instance's last door and a boot that shrugged at it would leave an
// instance nobody administers.

// `mtglab ui` is a command like any other, and its refusal reaches the
// operator as a sentence rather than as a usage dump — which is what the
// root's silences decide, and why this is driven through the tree rather than
// by calling `serve`.
func TestTheUiCommandRefusesAPortSomebodyElseIsHolding(t *testing.T) {
	t.Parallel()
	held := heldPort(t)
	d := scratchDeployment(t)

	out, err := d.run(t, "ui", "--host", "127.0.0.1", "--port", portOf(t, held),
		"--web-dist", t.TempDir(), "--tarot", t.TempDir())
	if err == nil {
		t.Fatalf("`mtglab ui` bound a port somebody else was holding:\n%s", out)
	}
	if !strings.Contains(err.Error(), "listen on") ||
		!strings.Contains(err.Error(), portOf(t, held)) {
		t.Errorf("the refusal said %q without naming the address", err)
	}
}

// A maintainer who cannot be reconciled stops the boot, over a database the
// ladder is happy with.
//
// It is the pairing that makes this specific: `auth.Migrate` is a no-op on a
// file whose `user_version` already reads current, so a restore that kept the
// pragma and lost the tables arrives here *past* the ladder. ADR 17's
// reconciliation is the next thing that touches it, and it is the instance's
// last door — an admin demoted by accident repairs itself with a restart, and
// only because this runs.
func TestAMaintainerWhoCannotBeReconciledStopsTheBoot(t *testing.T) {
	t.Parallel()
	d := claimedSchema(t, scratchDeployment(t))
	d.AdminEmail = "keeper@example.com"
	d.AdminUsername = "keeper"

	err := serve(d.Config, tier3.Settings{}, "127.0.0.1", "0",
		t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("an instance nobody could be made admin of served anyway")
	}
}

// The door refuses the same database one line later, and nothing was bound to
// serve over it.
//
// **This is the ladder's refusal arriving late**, and it is worth having
// twice. `auth.Migrate` is a no-op on a file that claims to be current, so the
// one check that would have caught it is skipped by design — and the door's
// own writability check is what is left. A boot that got past it would have a
// listener open on an instance where every write fails.
func TestADoorThatCannotWriteRefusesRatherThanOpening(t *testing.T) {
	t.Parallel()
	d := claimedSchema(t, scratchDeployment(t))
	// With auth on, because that is the deployment where it matters: sessions,
	// tokens and the visitor ledger are all writes.
	d.RequireAuth = true

	l := heldPort(t)
	done := make(chan error, 1)
	go func() {
		done <- serveUntil(d.Config, tier3.Settings{}, t.TempDir(), t.TempDir(),
			func() (net.Listener, error) { return l, nil }, make(chan os.Signal))
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an instance whose app.db takes no writes served anyway")
		}
		if !strings.Contains(err.Error(), "app.db") {
			t.Errorf("the refusal said %q without naming the file", err)
		}
	case <-time.After(2 * shutdownGrace):
		t.Fatal("the boot neither came up nor refused")
	}
	// The listener callback is below the door, so nothing was ever handed
	// over: the port this test is holding is still only this test's.
	if _, err := net.Dial("tcp", "127.0.0.1:"+portOf(t, l)); err != nil {
		t.Logf("nothing is accepting on the port, as expected: %v", err)
	}
}

// A night with a window on it says so where a `fly logs` tail can see it,
// because the first scheduler this app has ever had is the sort of thing an
// operator should not have to shell in to discover.
//
// The boot is the assertion rather than the log line: a window the boot
// refuses is a window nobody is running, and `SettingsFromConfig` refuses
// several shapes that look fine in an environment file.
func TestANightWithAWindowComesUpScheduled(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.NightWindow = "02:00-05:00"
	d.NightZone = "America/Los_Angeles"

	stop := make(chan os.Signal, 1)
	_, base, done := bootServerUntil(t, d, stop)
	resp, err := waitForHealth(t, base+"/api/health", done)
	if err != nil {
		t.Fatalf("an instance with a scheduled night never came up: %v", err)
	}
	_ = resp.Body.Close()
	stop <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a scheduled night's boot ended with %v", err)
		}
	case <-time.After(2 * shutdownGrace):
		t.Fatal("the boot never stopped")
	}
}

// A night the settings refuse is a **refusal to serve**, and it is the one
// exception to the warnings-never-refusal rule besides the ladder: a
// scheduler on the wrong clock runs at an hour nobody is watching, and the
// sentence is the fix.
func TestANightOnAClockNobodyOwnsRefusesToServe(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.NightWindow = "02:00-05:00"
	// A window is wall-clock time and the instance has not been told whose
	// wall.
	d.NightZone = ""

	err := serve(d.Config, tier3.Settings{}, "127.0.0.1", "0",
		t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("a night with a window and no clock was scheduled anyway")
	}
	if !strings.Contains(err.Error(), "MTGLAB_NIGHT_ZONE") {
		t.Errorf("the refusal said %q without naming what to set", err)
	}
	// And nothing was bound on the way: the night is resolved before the
	// ladder, so a boot that dies here never touched the volume either.
	if _, err := os.Stat(d.AppDBPath()); err == nil {
		t.Error("a boot that refused its night created app.db first")
	}
}

// A server that **stops serving** on its own — the listener pulled out from
// under it, which is what a volume unmounting or a socket being closed looks
// like from inside — ends the boot with that error rather than with silence.
//
// The distinction is the one an exit code carries: a stop by signal is a clean
// nil, and a server that fell over is not, so a container restarting a crashed
// process can tell the two apart.
func TestAServerThatStopsServingEndsTheBootWithItsReason(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	l := heldPort(t)
	stop := make(chan os.Signal) // never spoken to; the listener is what ends this
	done := make(chan error, 1)
	go func() {
		done <- serveUntil(d.Config, tier3.Settings{}, t.TempDir(), t.TempDir(),
			func() (net.Listener, error) { return l, nil }, stop)
	}()

	base := "http://127.0.0.1:" + portOf(t, l)
	resp, err := waitForHealth(t, base+"/api/health", done)
	if err != nil {
		t.Fatalf("the server never answered: %v", err)
	}
	_ = resp.Body.Close()

	// The listener goes, and `Serve` returns something that is not
	// `http.ErrServerClosed` — the shape a clean shutdown produces and this
	// deliberately does not.
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a server that stopped serving reported a clean stop")
		}
	case <-time.After(2 * shutdownGrace):
		t.Fatal("the boot never noticed its own listener had gone")
	}
}
