package tier3_test

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The live Forge test: a real distribution, a real JVM, a real match.
//
// **Opt-in and skipped everywhere but a machine with Forge on it.** CI has no
// distribution and never will — `res/cardsfolder` is 470MB of somebody else's
// card data, which CLAUDE.md rule 5 forbids committing for the same reason it
// forbids committing the card pool. So this runs when `MTGLAB_LIVE_FORGE=1` is
// set and a distribution is where `MTGLAB_FORGE_HOME` (or the default) says.
//
// It exists because the corpus beside it cannot reach any of what it checks.
// The zip index is 33,587 files of real card scripts; `forge.profile.
// properties` is the one thing that reaches into somebody else's install; the
// JVM's output is the format strings the parser was written from. Every one of
// those is a claim about the world rather than about a function, and the only
// honest way to check a claim about the world is to go and look.
//
// Two games, not ten: this is a smoke test for the machinery, and the numbers
// it produces are nobody's evidence about a deck.

// liveForge is this machine's real Forge, or a skip.
//
// The one place in the tree that still *wants* [tier3.LoadSettings] in a test:
// these tests are claims about the world, so the installation they run against
// has to be the one the operator actually has.
func liveForge(t *testing.T) tier3.Settings {
	t.Helper()
	if os.Getenv("MTGLAB_LIVE_FORGE") != "1" {
		t.Skip("set MTGLAB_LIVE_FORGE=1 to run a real Forge match")
	}
	forge := tier3.LoadSettings()
	if _, err := forge.DesktopJar(); err != nil {
		t.Skipf("no Forge distribution: %v", err)
	}
	return forge
}

func liveDecks(t *testing.T) []*deck.Deck {
	t.Helper()
	var decks []*deck.Deck
	for _, name := range []string{"mono-green-clean", "kaheera"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "gate", "testdata", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := deck.FromText(string(raw), name)
		if err != nil {
			t.Fatal(err)
		}
		decks = append(decks, d)
	}
	return decks
}

// TestTheRealIndexIsReadFromTheRealZip checks the pre-flight against Forge's
// own card scripts — the data the engine itself loads at startup, which is
// what makes agreeing with it agreeing with Forge.
// **Serial**: it clears the package-level index and then asserts on its hit
// counters, which any other test reading the index concurrently would move.
func TestTheRealIndexIsReadFromTheRealZip(t *testing.T) {
	t.Parallel()
	forge := liveForge(t)
	tier3.ClearIndex()
	started := time.Now()
	index, err := forge.ImplementedNames()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("read %d implemented names in %s", len(index), time.Since(started).Round(time.Millisecond))
	// The measured shape is ~34,500 names; a floor rather than an equality,
	// because a Forge upgrade moves the number and must not fail this.
	if len(index) < 30000 {
		t.Errorf("the index holds %d names, which is too few to be Forge's", len(index))
	}
	// Face names only, never Scryfall's combined `A // B`. The exporter
	// depends on it and the corpus asserts it against a hand-built index; this
	// is the same claim against the real one.
	for name := range index {
		if len(name) > 4 && contains(name, " // ") {
			t.Errorf("the real index holds a combined name: %q", name)
			break
		}
	}
	// And it is cached on (path, mtime, size): a second ask must not re-read
	// 33,587 files.
	hits, misses := tier3.IndexStats()
	if _, err := forge.ImplementedNames(); err != nil {
		t.Fatal(err)
	}
	hits2, misses2 := tier3.IndexStats()
	if hits2 != hits+1 || misses2 != misses {
		t.Errorf("the second ask was a miss: hits %d->%d, misses %d->%d",
			hits, hits2, misses, misses2)
	}
}

// TestARealMatchPlaysAndParses is the whole local path, end to end.
//
// It is the one test that can fail for a reason no corpus can express: Forge
// changed a format string, the profile file stopped being read, the JVM is too
// old, AWT would not initialise. Each of those has happened at least once.
func TestARealMatchPlaysAndParses(t *testing.T) {
	t.Parallel()
	forge := liveForge(t)
	decks := liveDecks(t)

	var ticks []int
	var seated []tier3.GameResult
	seed := int64(7)
	started := time.Now()
	run, err := forge.RunGames(decks, tier3.RunOptions{
		Games: 2, Clock: 300, Seed: bigSeed(seed),
		OnGame: func(finished int, game tier3.GameResult) {
			ticks = append(ticks, finished)
			seated = append(seated, game)
		},
	})
	if err != nil {
		t.Fatalf("the match failed after %s: %v", time.Since(started).Round(time.Second), err)
	}
	t.Logf("played %d games in %.1fs (%.1fs of it startup), Forge %s",
		len(run.Games()), run.WallSeconds, run.StartupSeconds(), run.ForgeVersion)

	if len(run.Games()) != 2 {
		t.Fatalf("Forge produced %d games, want 2", len(run.Games()))
	}
	// The tick and the tally are one parser, so every game the callback saw is
	// a game the tally holds — by identity, not by resemblance.
	if len(ticks) != 2 || ticks[0] != 1 || ticks[1] != 2 {
		t.Errorf("the bar ticked %v, want [1 2]", ticks)
	}
	for i, game := range seated {
		if game.Index != run.Games()[i].Index || game.Milliseconds != run.Games()[i].Milliseconds {
			t.Errorf("tick %d seated %+v, the tally holds %+v", i+1, game, run.Games()[i])
		}
	}
	// Every game names a seat this run knows, or is a draw. A seat the run
	// cannot name means the label format moved.
	for _, game := range run.Games() {
		if game.WinnerSeat == nil {
			if !game.Draw {
				t.Errorf("game %d has no winner and is not a draw: %+v", game.Index, game)
			}
			continue
		}
		if slug := run.WinnerSlug(game); slug == "" {
			t.Errorf("game %d was won by seat %d, which this run does not name",
				game.Index, *game.WinnerSeat)
		}
	}
	// The version comes off the jar's name, and the ledger records it because
	// Forge's AI is the instrument every recorded game was measured with.
	if run.ForgeVersion == "" {
		t.Error("the run reported no Forge version")
	}
	// And the pre-flight really ran: a coverage report per deck.
	if len(run.Coverage) != 2 {
		t.Errorf("the run carries %d coverage reports, want 2", len(run.Coverage))
	}
}

func bigSeed(n int64) *big.Int { return big.NewInt(n) }
