package tier3_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The parity gate: the scribe plays the same Magic as stock `sim`, or nothing
// it produces may be recorded.
//
// **This is ADR 36's argument turned on ourselves.** The match ledger records
// which Forge played so that a match from March and a match from August can be
// compared, and the whole value of that record rests on the games being driven
// identically. The scribe cannot reuse `SimulateMatch.simulateSingleMatch`,
// because Forge builds each `Game` with its own private `EventBus` and
// `Match.subscribeToEvents` does not propagate — a listener can only be
// attached between `createGame()` and `startGame()`. So `scribe/Main.java`
// owns its own copy of those lines, and **that copy is the risk this file
// exists to police**: if it seeds the global RNG differently, or asks for a
// different variant, or builds the seats in another order, every row already
// in the ledger becomes incomparable with every row after it. Silently.
//
// Five things must not drift. They are marked PARITY in `Main.java`:
// `MyRandom.setRandom` on a global before any game is created,
// `RegisteredPlayer.forCommander`, `setAppliedVariants`, `setSimTimeout` with
// Forge's own timed block, and the AI player with its seat index.
//
// **Opt-in, because CI has no JVM and no distribution.** It needs a real
// Forge, a real JDK and minutes of wall clock — the same bargain
// `TestARealMatchPlaysAndParses` and the shim's live test already make:
//
//	MTGLAB_LIVE_FORGE=1 \
//	MTGLAB_SCRIBE_CLASSES=$(pwd)/../scribe/out \
//	  go test ./internal/sim/tier3/ -run Parity -v
//
// A seeded pair of games is the unit. Two rather than one because a seeded
// *sequence* is the property that matters: a scribe that reseeded per game
// would match on game one and diverge on game two, which is exactly the bug a
// single game cannot see.

// scribeResult is the scribe's own `{"t":"result"}` line.
type scribeResult struct {
	Kind     string `json:"t"`
	Game     int    `json:"game"`
	Seat     int    `json:"seat"`
	Winner   string `json:"winner"`
	Draw     bool   `json:"draw"`
	TimedOut bool   `json:"timed_out"`
}

func TestTheScribePlaysTheSameMagicAsStockSim(t *testing.T) {
	t.Parallel()
	if os.Getenv("MTGLAB_LIVE_FORGE") != "1" {
		t.Skip("set MTGLAB_LIVE_FORGE=1 to run a real Forge match")
	}
	classes := os.Getenv("MTGLAB_SCRIBE_CLASSES")
	if classes == "" {
		t.Skip("set MTGLAB_SCRIBE_CLASSES to the scribe's compiled classes " +
			"(scribe/build.sh writes scribe/out)")
	}
	forge := tier3.LoadSettings()
	if _, err := forge.DesktopJar(); err != nil {
		t.Skipf("no Forge distribution: %v", err)
	}

	decks := parityDecks(t)
	const games, clock = 2, 300
	seed := big.NewInt(11)

	// The stock path: exactly what the app runs today.
	stock, err := forge.RunGames(decks, tier3.RunOptions{
		Games: games, Clock: clock, Seed: seed})
	if err != nil {
		t.Fatalf("the stock run failed: %v", err)
	}
	if len(stock.Games()) != games {
		t.Fatalf("stock played %d games, want %d", len(stock.Games()), games)
	}

	told := runScribe(t, forge, decks, classes, games, clock, seed)
	if len(told) != games {
		t.Fatalf("the scribe reported %d games, want %d", len(told), games)
	}

	for i, want := range stock.Games() {
		got := told[i]
		t.Logf("game %d — stock: seat %v turns %v draw %v clock %v | "+
			"scribe: seat %d draw %v clock %v",
			i+1, seatOf(want), turnsOf(want), want.Draw, want.TimedOut,
			got.Seat, got.Draw, got.TimedOut)

		// **The winner, by seat.** Two decks can share a name and never share
		// a seat, and the seat is what `SimRun.Seats` and every shaping layer
		// above it work in.
		if seatOf(want) != got.Seat {
			t.Errorf("game %d: stock says seat %v won, the scribe says seat %d "+
				"— the two are not playing the same game",
				i+1, seatOf(want), got.Seat)
		}
		if want.TimedOut != got.TimedOut {
			t.Errorf("game %d: stock clock-out %v, scribe %v",
				i+1, want.TimedOut, got.TimedOut)
		}
		// A real draw and a clock-out are different facts everywhere else in
		// this package, so they are compared apart here too.
		wantDraw := want.Draw && !want.TimedOut
		if wantDraw != got.Draw {
			t.Errorf("game %d: stock draw %v, scribe %v", i+1, wantDraw, got.Draw)
		}
	}
}

// parityDecks are the two fixtures every live test in this repo plays, so a
// failure here is about the scribe rather than about a deck nobody else uses.
func parityDecks(t *testing.T) []*deck.Deck {
	t.Helper()
	var out []*deck.Deck
	for _, name := range []string{"mono-green-clean", "kaheera"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "gate", "testdata", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := deck.FromText(string(raw), name)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

// runScribe plays the same match through the scribe and returns its result
// lines in game order.
//
// The `.dck` files are written by the same [tier3.WriteDck] the stock path
// uses, from the same coverage report — so a difference in what Forge was
// handed cannot be mistaken for a difference in how it played.
func runScribe(t *testing.T, forge tier3.Settings, decks []*deck.Deck,
	classes string, games, clock int, seed *big.Int) []scribeResult {
	t.Helper()

	reports, err := forge.CheckCoverage(decks)
	if err != nil {
		t.Fatalf("the pre-flight failed: %v", err)
	}
	deckDir, err := forge.EnsureProfile()
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(decks))
	for i, d := range decks {
		path, err := tier3.WriteDck(d, deckDir, reports[i].Resolved)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}

	java, err := forge.JavaBinary()
	if err != nil {
		t.Fatal(err)
	}
	jar, err := forge.DesktopJar()
	if err != nil {
		t.Fatal(err)
	}
	argv := append([]string{
		"-Xmx3072m", "-Dfile.encoding=UTF-8",
		"-cp", jar + string(os.PathListSeparator) + classes,
		"scribe.Main", fmt.Sprint(clock), fmt.Sprint(games), seed.String(),
	}, paths...)

	cmd := exec.Command(java, argv...) //nolint:gosec // an operator-chosen JVM and our own classes
	cmd.Dir = forge.Home
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var results []scribeResult
	kinds := map[string]int{}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r scribeResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		kinds[r.Kind]++
		if r.Kind == "result" {
			results = append(results, r)
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("the scribe exited badly: %v", err)
	}
	t.Logf("the scribe said: %v", kinds)

	// The gate is about parity, but a scribe that reported perfect results and
	// no board would pass it while being useless — so the reason it exists is
	// asserted here rather than assumed.
	if kinds["zone"] == 0 {
		t.Error("the scribe reported no zone changes; there is no board in this")
	}
	return results
}

func seatOf(g tier3.GameResult) int {
	if g.WinnerSeat == nil {
		return 0
	}
	return *g.WinnerSeat
}

func turnsOf(g tier3.GameResult) int {
	if g.Turns == nil {
		return 0
	}
	return *g.Turns
}
