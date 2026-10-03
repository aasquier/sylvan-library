package main

import (
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The two things the boot does when something is wrong and serving anyway is
// the right answer: an `app.db` it cannot read, and a stop that did not go
// quietly.
//
// Both are deliberate degradations argued at their own lines in `ui.go`, and
// both were unreached -- which is the dangerous shape, because a degradation
// nothing drives is a degradation that may not degrade. The one next door is
// `bootfaults_test.go`'s: a database that takes no *writes* refuses to serve
// at all. The difference between that refusal and this warning is the whole
// decision, and only one of the two had a test.

// With auth on and the accounts unreadable, the app comes up anonymous instead
// of refusing to come up.
//
// **A warning rather than a refusal, on purpose**: the ladder has already run,
// so a read that fails here is a genuinely broken file rather than a fresh
// instance -- and an instance that refuses to boot over it is an instance
// nobody can even reach the sign-in page of, which is the state a maintainer
// has to be able to look at while they fix the disk.
//
// The fixture is a real migrated database with the accounts table taken out:
// the ladder no-ops over the current `user_version`, the write side still pings
// its own table so the door stands, and the one read that fails is the one
// about who may sign in. A whole claimed-and-empty schema would be refused one
// step earlier, by the door.
func TestAnInstanceThatCannotReadItsAccountsStillServesAnonymously(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.RequireAuth = true
	seedAccount(t, d, "keeper", "--admin")
	hollowed(t, d, "users")

	stop := make(chan os.Signal, 1)
	_, base, done := bootServerUntil(t, d, stop)
	resp, err := waitForHealth(t, base+"/api/health", done)
	if err != nil {
		t.Fatalf("an instance with an unreadable accounts table refused to "+
			"serve at all: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the health route answered %d", resp.StatusCode)
	}

	// And it is serving the degraded state rather than pretending: with auth
	// on and no session resolvable, a page behind the door is refused.
	client := &http.Client{Timeout: 2 * time.Second}
	guarded, err := client.Get(base + "/api/decks")
	if err != nil {
		t.Fatal(err)
	}
	_ = guarded.Body.Close()
	if guarded.StatusCode == http.StatusOK {
		t.Error("a request that belongs to somebody was answered over a " +
			"database that cannot say who anybody is")
	}

	stop <- syscall.SIGTERM
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the degraded instance stopped with %v", err)
		}
	case <-time.After(2 * shutdownGrace):
		t.Fatal("the degraded instance never stopped")
	}
}

// stubbornListener is a listener that refuses to be closed -- the one fault
// [net/http.Server.Shutdown] reports that a clock cannot be waited for.
//
// The real shape is a drain that runs out of grace with requests still in
// flight, and the grace is twenty seconds: a test cannot wait for it, and
// shortening it for a test would be testing a different constant. `Shutdown`
// hands back its listener-close error for the same reason it hands back the
// context's, so a listener that fails to close reaches the same arm by the same
// line.
type stubbornListener struct {
	net.Listener
	closes chan struct{}
}

func (s stubbornListener) Close() error {
	_ = s.Listener.Close()
	select {
	case s.closes <- struct{}{}:
	default:
	}
	// Not wrapped in anything of ours: what the arm does with it is log it.
	return syscall.EIO
}

// A stop that did not go quietly is a warning and still a clean exit.
//
// **The decision this holds is the exit status.** The process is going down
// either way, and turning an ordinary drain timeout into a non-zero exit would
// make every crowded deploy read as a crash -- which is what a deploy watcher
// branches on. So `serveUntil` warns and returns nil, and the only way to tell
// that apart from a shutdown that succeeded is to make the shutdown fail and
// look at what came back.
func TestAStopThatDidNotGoQuietlyIsNotAnExitFailure(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	webDist, tarot := t.TempDir(), t.TempDir()
	closes := make(chan struct{}, 1)
	held := heldPort(t)
	l := stubbornListener{Listener: held, closes: closes}
	stop := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- serveUntil(d.Config, tier3.Settings{}, webDist, tarot,
			func() (net.Listener, error) { return l, nil }, stop)
	}()

	// Waited for rather than stopped straight away: `Shutdown` can only close
	// a listener `Serve` has already *taken*, and the boot hands it over on
	// another goroutine. Written with the stop already in the buffer, this
	// raced -- the shutdown ran first, closed nothing, and the arm below never
	// fired. The check at the end of this test is what said so.
	resp, err := waitForHealth(t, "http://127.0.0.1:"+portOf(t, held)+"/api/health", done)
	if err != nil {
		t.Fatalf("the boot never answered: %v", err)
	}
	_ = resp.Body.Close()
	stop <- syscall.SIGTERM

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a drain that did not finish was reported as a crashed "+
				"process: %v", err)
		}
	case <-time.After(2 * shutdownGrace):
		t.Fatal("the boot neither stopped nor gave up")
	}
	// The fixture bit: the shutdown really did ask this listener to close and
	// really was refused, so the nil above is about the arm rather than about
	// a shutdown that quietly succeeded.
	select {
	case <-closes:
	default:
		t.Error("the shutdown never closed the listener, so nothing proved " +
			"what a failing drain does")
	}
}
