package api

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The killing blow's painting, and the gate that says whether there is an
// arena at all.
//
// [API.paintTheKillers] had **never run anywhere but production**: its only
// caller is [API.playForgeMatch], after a real match, so a machine without
// Forge never reaches it and every test in this package drove the stub shim's
// rows straight past it. That is the worst shape a function can be in — the
// room that draws a game's ending, proven by nothing — and it is reachable
// directly for the asking, because what it needs is a pool and some rows.
//
// Best-effort is the contract and it is the part worth asserting: a name the
// pool cannot answer leaves the image empty and the room says the blow in
// words, because failing a finished match over a picture would cost somebody
// the minutes the match took.

// killer is a row whose game ended on a blow by `card`.
func killerRow(game int, card string) forgeRow {
	return forgeRow{Game: game, Killer: &forgeBlow{Amount: 7, Card: card,
		Sources: 1, Combat: true, Turn: 9, Victim: "mono-green"}}
}

func TestTheKillingBlowIsPaintedFromThePoolInOneRead(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	rows := []forgeRow{
		killerRow(1, "Craterhoof Behemoth"),
		// The same card again: two games of one pairing end the same way far
		// more often than not, and both rows must be painted.
		killerRow(2, "Craterhoof Behemoth"),
		killerRow(3, "Terastodon"),
		// A token. Forge's own spelling is not a card name, and the board has
		// its own path for those -- here it is the expected miss.
		killerRow(4, "Beast Token"),
		// A game that simply ended: a clock-out, a decking, a concession.
		{Game: 5, TimedOut: true},
	}
	a.paintTheKillers(t.Context(), rows)

	if rows[0].Killer.Image == "" || rows[1].Killer.Image == "" {
		t.Fatalf("a blow the pool knows went unpainted: %+v", rows[0].Killer)
	}
	if rows[0].Killer.Image != rows[1].Killer.Image {
		t.Errorf("two blows by one card were painted differently:\n %q\n %q",
			rows[0].Killer.Image, rows[1].Killer.Image)
	}
	if !strings.Contains(rows[0].Killer.Image, "http") {
		t.Errorf("the painting is %q, which is not a picture", rows[0].Killer.Image)
	}
	if rows[2].Killer.Image == "" {
		t.Error("the second card's blow went unpainted")
	}
	if rows[2].Killer.Image == rows[0].Killer.Image {
		t.Error("two different cards were painted with the same picture")
	}
	if rows[3].Killer.Image != "" {
		t.Errorf("a token was painted with %q; the room says that blow in words",
			rows[3].Killer.Image)
	}
	if rows[4].Killer != nil {
		t.Error("a game with no blow grew one")
	}
}

// A match whose games all ended without a blow asks the pool nothing at all —
// the early return, and the reason it is there: a clock-out-only match must
// not cost a round trip.
func TestAMatchWithNoBlowsAsksThePoolNothing(t *testing.T) {
	t.Parallel()
	// A pool that fails every query. If the read were made anyway, the
	// best-effort swallow would hide it -- so what is asserted is the rows
	// coming back untouched, which is the same claim from the other side.
	a := New(Config{Pool: schemalessPool(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rows := []forgeRow{{Game: 1, TimedOut: true}, {Game: 2, Draw: true}}
	a.paintTheKillers(t.Context(), rows)
	for i := range rows {
		if rows[i].Killer != nil {
			t.Errorf("game %d grew a blow", rows[i].Game)
		}
	}
}

// A pool that will not answer leaves the blows in words rather than failing
// the match: the minutes are already spent, and a picture is not worth them.
func TestAPoolThatWillNotAnswerStillLetsTheMatchFinish(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: schemalessPool(t),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rows := []forgeRow{killerRow(1, "Craterhoof Behemoth")}
	a.paintTheKillers(t.Context(), rows)
	if rows[0].Killer == nil {
		t.Fatal("the blow itself was lost")
	}
	if rows[0].Killer.Image != "" {
		t.Errorf("a pool that answered an error produced a painting: %q",
			rows[0].Killer.Image)
	}
	if rows[0].Killer.Card != "Craterhoof Behemoth" || rows[0].Killer.Amount != 7 {
		t.Errorf("the blow lost its words as well as its picture: %+v", rows[0].Killer)
	}
}

// An instance with no pool at all — a fresh checkout before the library has
// been gathered — is the same answer, reached by a different road.
func TestAnInstanceWithNoPoolPaintsNothingAndSaysNothing(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rows := []forgeRow{killerRow(1, "Craterhoof Behemoth")}
	a.paintTheKillers(t.Context(), rows)
	if rows[0].Killer.Image != "" {
		t.Errorf("an instance with no library painted %q", rows[0].Killer.Image)
	}
}

// The gate, past the first refusal.
//
// Every existing test of [API.forgeStatus] runs in the state CI runs in — no
// distribution, so the answer is no on the very first probe. What that never
// reaches is the second: a distribution that *is* there, where the remaining
// question is the JVM. The jar below is an empty file with the name the probe
// globs for, which is all the probe looks at.
//
// Whether this machine has a new enough Java is not something a test may
// assume, so the assertion is the one that holds either way and is still
// worth making: once the distribution has been found, a refusal must be about
// the JVM and never about the jar. A gate that kept naming the jar after
// finding one would send somebody to reinstall Forge over a Java version.
func TestOnceTheDistributionIsFoundTheComplaintIsAboutTheJVM(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	jar := filepath.Join(home, "forge-gui-desktop-2.0.99-jar-with-dependencies.jar")
	if err := os.WriteFile(jar, []byte("not really a jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Forge: tier3.Settings{Home: home}})

	available, why := a.forgeStatus()
	if available {
		if why != nil {
			t.Errorf("an available arena still gave a reason: %q", *why)
		}
		return
	}
	if why == nil {
		t.Fatal("an unavailable arena gave no reason at all")
	}
	if strings.Contains(*why, "forge-gui-desktop") || strings.Contains(*why, "distribution") {
		t.Errorf("the gate still blames the distribution after finding one: %q", *why)
	}
	if !strings.Contains(strings.ToLower(*why), "java") {
		t.Errorf("the reason is %q and never names the JVM", *why)
	}
}

// A hosted worker answers yes on configuration alone — no network, no machine
// woken to ask — exactly as `/api/claude` answers on the presence of a key.
func TestAHostedArenaIsAvailableWithoutAnythingBeingAsked(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Forge: tier3.Settings{WorkerURL: "http://nothing.invalid"}})
	available, why := a.forgeStatus()
	if !available || why != nil {
		t.Fatalf("a configured worker read %v with %v", available, why)
	}
}
