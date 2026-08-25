package tier3_test

import (
	"math/big"
	"os"
	"path/filepath"
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

	// **Both paths through `RunGames`**, which is the whole point of driving
	// it this way rather than building an argv by hand: the two runs differ in
	// exactly one field of one struct, so anything that disagrees below is the
	// scribe playing different Magic and not the harness asking differently.
	//
	// The stock settings are forced empty rather than merely left alone, so
	// that a developer with `MTGLAB_SCRIBE_CLASSES` exported does not
	// accidentally compare the scribe against itself and see a green gate.
	stockSettings := forge
	stockSettings.ScribeClasses = ""
	if stockSettings.Scribed() {
		t.Fatal("the stock path still reports a scribe; this gate would be " +
			"comparing the scribe against itself")
	}
	stock, err := stockSettings.RunGames(decks, tier3.RunOptions{
		Games: games, Clock: clock, Seed: seed})
	if err != nil {
		t.Fatalf("the stock run failed: %v", err)
	}

	scribeSettings := forge
	scribeSettings.ScribeClasses = classes
	if !scribeSettings.Scribed() {
		t.Fatalf("MTGLAB_SCRIBE_CLASSES=%s holds no compiled scribe", classes)
	}
	var boards int
	told, err := scribeSettings.RunGames(decks, tier3.RunOptions{
		Games: games, Clock: clock, Seed: seed, Narrate: true,
		OnEvents: func(log tier3.EventLog) {
			if log.Board != nil {
				boards += len(log.Board.Steps)
			}
		}})
	if err != nil {
		t.Fatalf("the scribe run failed: %v", err)
	}

	if len(stock.Games()) != games || len(told.Games()) != games {
		t.Fatalf("stock played %d games and the scribe %d, want %d each",
			len(stock.Games()), len(told.Games()), games)
	}

	for i, want := range stock.Games() {
		got := told.Games()[i]
		t.Logf("game %d — stock: seat %v turns %v draw %v clock %v | "+
			"scribe: seat %v turns %v draw %v clock %v",
			i+1, seatOf(want), turnsOf(want), want.Draw, want.TimedOut,
			seatOf(got), turnsOf(got), got.Draw, got.TimedOut)

		// **The winner, by seat.** Two decks can share a name and never share
		// a seat, and the seat is what `SimRun.Seats` and every shaping layer
		// above it work in.
		if seatOf(want) != seatOf(got) {
			t.Errorf("game %d: stock says seat %v won, the scribe says seat %v "+
				"— the two are not playing the same game",
				i+1, seatOf(want), seatOf(got))
		}
		// **The turn count, which the two paths reach differently.** Stock
		// reads Forge's `Game Outcome: Turn N` line; the scribe reads
		// `GameEventGameOutcome.lastTurnNumber()`. Two answers to one question,
		// and the match ledger records whichever it is handed — so they agree
		// here or the ledger stops being comparable across a deploy.
		if turnsOf(want) != turnsOf(got) {
			t.Errorf("game %d: stock counted %v turns and the scribe %v",
				i+1, turnsOf(want), turnsOf(got))
		}
		if want.TimedOut != got.TimedOut {
			t.Errorf("game %d: stock clock-out %v, scribe %v",
				i+1, want.TimedOut, got.TimedOut)
		}
		// A real draw and a clock-out are different facts everywhere else in
		// this package, so they are compared apart here too.
		if want.Draw != got.Draw {
			t.Errorf("game %d: stock draw %v, scribe %v", i+1, want.Draw, got.Draw)
		}
	}

	// The gate is about parity, but a scribe that reported perfect results and
	// no board would pass it while being useless — so the reason it exists is
	// asserted here rather than assumed.
	if boards == 0 {
		t.Error("the scribe reported no board steps; there is no battlefield " +
			"in this")
	}
	t.Logf("the scribe drew %d board steps across %d games", boards, games)
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
