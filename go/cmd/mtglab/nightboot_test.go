package main

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The night's switches are the one place the boot refuses rather than warns
// (argued at the call in serveOn): a misconfigured scheduler would run on
// the wrong clock at an hour nobody is watching, and the sentence at boot is
// the whole fix.

func TestAMisconfiguredNightRefusesToServe(t *testing.T) {
	t.Parallel()
	// A window with no zone: the window is wall-clock time, and the instance
	// was never told whose wall. The refusal lands before the ladder and the
	// listener — no port is needed, exactly as in the unrunnable-ladder test.
	windowed := config.Config{NightWindow: "22:00-23:30"}
	err := serve(windowed, tier3.Settings{}, "127.0.0.1", "0", t.TempDir(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "MTGLAB_NIGHT_ZONE") {
		t.Fatalf("a windowed night with no zone served anyway: %v", err)
	}

	// And a games count past the arena's ceiling refuses by name too.
	greedy := config.Config{NightGames: "50"}
	err = serve(greedy, tier3.Settings{}, "127.0.0.1", "0", t.TempDir(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "MTGLAB_NIGHT_GAMES") {
		t.Fatalf("an over-cap games count served anyway: %v", err)
	}
}
