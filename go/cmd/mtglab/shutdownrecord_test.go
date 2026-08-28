package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// The platform's patience has to outlast the app's own, and until now it did
// not.
//
// **Two files, one number, and they disagreed by fifteen seconds in the
// direction where the app loses.** [shutdownGrace] is twenty seconds — the time
// `serve` gives requests already in flight before it drops them — and Fly's
// default `kill_timeout` is five. So a deploy sent the signal, the app began a
// twenty-second drain, and the platform killed it a quarter of the way in.
//
// **The dropped connections are the smaller half.** `door.Close` is the
// deferred close in `serveOn` and runs *after* `server.Shutdown` returns, so a
// stop that never finished draining never reached it — and that is what flushes
// the visitor ledger and hands back `app.db` and the card pool. A busy deploy
// lost whatever the ledger had counted, with nothing anywhere saying so.
//
// **This parses the duration and compares it.** A test that grepped for
// `kill_timeout` would pass against the exact bug it was written for: the fault
// was never a missing line, it was a number too small, and `"5s"` matches that
// grep as happily as `"30s"` does. So the value is read, parsed as a duration
// and put beside the constant — which is the only form of this test that can
// fail for the reason it exists.
func TestTheStopHasLongerThanItNeeds(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "fly.toml"))
	if err != nil {
		t.Fatalf("reading fly.toml: %v", err)
	}
	// Top-level key only: `grace_period` under `[[http_service.checks]]` is a
	// health check's warm-up and has nothing to do with stopping.
	found := regexp.MustCompile(`(?m)^kill_timeout\s*=\s*"([^"]+)"`).
		FindSubmatch(raw)
	if found == nil {
		t.Fatalf("fly.toml sets no top-level kill_timeout, so the platform "+
			"uses its default of 5s and kills the app %s into a %s drain",
			5*time.Second, shutdownGrace)
	}
	patience, err := time.ParseDuration(string(found[1]))
	if err != nil {
		t.Fatalf("kill_timeout is %q, which is not a duration Go can read; "+
			"Fly would take its own reading of it and nothing here would "+
			"notice: %v", found[1], err)
	}
	if patience <= shutdownGrace {
		t.Errorf("kill_timeout is %s and shutdownGrace is %s: the platform "+
			"stops waiting before the app stops draining, so `door.Close` "+
			"never runs and the visitor ledger loses what it counted",
			patience, shutdownGrace)
	}
	// Headroom, not equality. The close work happens *after* the drain, so a
	// kill_timeout merely equal to the grace would race the deadline it was
	// waiting on — and a drain that legitimately spends its whole budget is
	// exactly when the flush matters most.
	if patience < shutdownGrace+5*time.Second {
		t.Errorf("kill_timeout is %s against a %s drain, which leaves %s for "+
			"the ledger flush and three handles; a stop that used its whole "+
			"grace would be killed doing the cleanup instead of the draining",
			patience, shutdownGrace, patience-shutdownGrace)
	}
	// Fly's own ceiling, quoted from its configuration reference. A value
	// above it is not a longer wait, it is a file Fly rejects at deploy —
	// which is a red deploy rather than anything a reader here would see.
	if patience > 300*time.Second {
		t.Errorf("kill_timeout is %s and Fly's maximum is 5m", patience)
	}
}
