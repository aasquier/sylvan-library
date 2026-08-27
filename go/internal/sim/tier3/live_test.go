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

// TestARealMatchNarratesItself is the same path with `-q` withheld.
//
// The claim about the world here is that Forge's game log still looks the way
// `events.go`'s patterns were written from — a claim no corpus can make, since
// the corpus *is* a recording of that log and would agree with itself after
// Forge changed every line. If a release renames "didn't block" or moves the
// instance id, this is what says so.
func TestARealMatchNarratesItself(t *testing.T) {
	t.Parallel()
	forge := liveForge(t)
	decks := liveDecks(t)

	var told []tier3.EventLog
	run, err := forge.RunGames(decks, tier3.RunOptions{
		Games: 2, Clock: 300, Seed: bigSeed(11), Narrate: true,
		OnEvents: func(log tier3.EventLog) { told = append(told, log) },
	})
	if err != nil {
		t.Fatalf("the narrated match failed: %v", err)
	}

	// One log per game, in order, and the same logs on the run.
	if len(told) != 2 {
		t.Fatalf("heard %d games narrated, want 2", len(told))
	}
	if len(run.Events) != 2 {
		t.Fatalf("the run carries %d narrated games, want 2", len(run.Events))
	}
	for i, log := range told {
		if log.Game != i+1 {
			t.Errorf("game %d narrated as %d", i+1, log.Game)
		}
		if len(log.Events) != len(run.Events[i].Events) {
			t.Errorf("game %d: the callback heard %d beats, the run kept %d",
				i+1, len(log.Events), len(run.Events[i].Events))
		}
	}

	// The beats a game cannot be played without. A parser whose patterns had
	// rotted would still return a list — an empty one, or one of nothing but
	// turns — so this asks for the kinds that prove real lines were read.
	for i, log := range told {
		seen := map[tier3.EventKind]int{}
		for _, e := range log.Events {
			seen[e.Kind]++
		}
		t.Logf("game %d: %d beats %v", log.Game, len(log.Events), seen)
		for _, kind := range []tier3.EventKind{
			tier3.EventTurn, tier3.EventLand, tier3.EventCast, tier3.EventOutcome,
		} {
			if seen[kind] == 0 {
				t.Errorf("game %d has no %s beats; the pattern has rotted", i+1, kind)
			}
		}
		// Two players, two verdicts, exactly one of them a win.
		wins := 0
		for _, e := range log.Events {
			if e.Kind == tier3.EventOutcome && e.Amount == 1 {
				wins++
			}
		}
		if game := run.Games()[i]; !game.Draw && wins != 1 {
			t.Errorf("game %d was won, but %d beats claim a win", i+1, wins)
		}
	}
}

// TestAQuietMatchNarratesNothing pins the flag to its cost. Narrating is
// opt-in because it turns one line per game into hundreds; a run that
// collected them anyway would be paying for output nobody asked for.
func TestAQuietMatchNarratesNothing(t *testing.T) {
	t.Parallel()
	forge := liveForge(t)
	decks := liveDecks(t)

	called := 0
	run, err := forge.RunGames(decks, tier3.RunOptions{
		Games: 1, Clock: 300, Seed: bigSeed(11),
		OnEvents: func(tier3.EventLog) { called++ },
	})
	if err != nil {
		t.Fatalf("the quiet match failed: %v", err)
	}
	if called != 0 {
		t.Errorf("a quiet run narrated %d games", called)
	}
	if len(run.Events) != 0 {
		t.Errorf("a quiet run kept %d narrated games", len(run.Events))
	}
}

// companionDeck is a deck Forge will actually let a companion into.
//
// **Built here rather than borrowed from `internal/gate/testdata`**, because
// the fixture there is a fixture for the *gate* and its 99 hold a Praetor —
// so Forge tests Kaheera's restriction, finds a creature that is not a Cat,
// Elemental, Nightmare, Dinosaur or Beast, and leaves her in the sideboard
// forever. A test about a companion arriving needs a deck whose companion
// arrives, and the restriction is the deck's own business rather than
// something to work around.
//
// Every name is one Forge implements — read out of its own `cardsfolder.zip`,
// not remembered — and the creatures are all Cats. Singleton, because Forge's
// Commander format takes a view about that, and 99 plus a commander because it
// takes a view about that too.
func companionDeck(t *testing.T, slug, name string) *deck.Deck {
	t.Helper()
	cards := []deck.CardEntry{{Name: "Sol Ring", Category: "ramp",
		Why: "Two mana on turn one, and it always has been."}}
	for _, cat := range []string{
		"Savannah Lions", "Sanctuary Cat", "Trained Caracal", "Prowling Caracal",
		"Silvercoat Lion", "Maned Serval", "Mesa Lynx", "Pouncing Lynx",
		"Jungle Lion", "Pouncing Jaguar", "Grizzled Leotau", "Fleecemane Lion",
		"Noble Panther", "Springing Tiger", "King Cheetah", "Zarichi Tiger",
		"Pride of Lions", "Guardian Lions", "Cave Tiger", "Glittering Lion",
	} {
		cards = append(cards, deck.CardEntry{Name: cat, Category: "threat",
			Why: "A Cat, which is the whole of what this fixture needs."})
	}
	for _, land := range []string{"Plains", "Forest"} {
		cards = append(cards, deck.CardEntry{Name: land, Category: "land",
			Qty: 39, Why: "Basic, untapped, and enough of them to pay {3}."})
	}
	companion := "Kaheera, the Orphanguard"
	return &deck.Deck{Slug: slug, Name: name, Status: "theoretical",
		Stage: "curated", Commander: []string{"Arahbo, Roar of the World"},
		Companion: &companion, Cards: cards}
}

// TestARealMatchNeverDealsACompanion is the report, driven rather than argued.
//
// **Aaron watched a match and thought a companion had been dealt into a hand**
// (2026-08-27: *"That should not be possible, you don't shuffle your companion
// in with normal cards to be dealt, they come from outside the game like the
// commander does"*). He is right about the rules. This goes and looks at what
// Forge does with the `[Sideboard]` line `dck.go` writes, because that line is
// the only thing in this repository that could have made him wrong — and a
// claim about somebody else's rules engine is a claim about the world, which
// is what this file is for.
//
// **Two assertions, and neither can flake.** The companion is never dealt: the
// first zone it is ever drawn in is the command zone, whatever else the game
// does. And every time it does reach a hand from the command zone — the {3}
// being paid, which Forge's AI may or may not choose to do in any given game —
// the account says so on the very same step. Nothing here asserts that the AI
// *must* buy it; that is a decision about Magic and Forge's to make.
func TestARealMatchNeverDealsACompanion(t *testing.T) {
	t.Parallel()
	forge := liveForge(t)
	decks := []*deck.Deck{
		companionDeck(t, "companion-probe-a", "Companion Probe A"),
		companionDeck(t, "companion-probe-b", "Companion Probe B"),
	}

	run, err := forge.RunGames(decks, tier3.RunOptions{
		Games: 2, Clock: 240, Seed: bigSeed(11), Narrate: true,
	})
	if err != nil {
		t.Fatalf("the companion match failed: %v", err)
	}
	if len(run.Events) != 2 {
		t.Fatalf("the run narrated %d games, want 2", len(run.Events))
	}

	const companion = "Kaheera, the Orphanguard"
	bought := 0
	for _, log := range run.Events {
		if log.Board == nil {
			t.Fatalf("game %d reported no board", log.Game)
		}
		// Forge's ids are per game and per seat, so both copies are followed.
		wanted := map[int]bool{}
		for _, card := range log.Board.Cards {
			if card.Name == companion {
				wanted[card.ID] = true
			}
		}
		if len(wanted) != 2 {
			t.Fatalf("game %d drew %d companions, want one per seat — a deck "+
				"whose companion Forge refused proves nothing here",
				log.Game, len(wanted))
		}
		first := map[int]string{}
		for i, step := range log.Board.Steps {
			for _, change := range step.Changes {
				if !wanted[change.ID] || change.Zone == "" {
					continue
				}
				if _, seen := first[change.ID]; !seen {
					first[change.ID] = change.Zone
				}
				if change.Zone != tier3.ZoneHand {
					continue
				}
				// The companion is in a hand. Either it was bought — and the
				// beat on this very step says so — or it arrived some other
				// way, which for this deck there is none of.
				if log.Events[i].Kind == tier3.EventCompanion &&
					log.Events[i].ID == change.ID {
					bought++
					continue
				}
				t.Errorf("game %d, step %d: %s reached a hand and the account "+
					"said %q — to a watcher that is a card being dealt",
					log.Game, i, companion, log.Events[i].Kind)
			}
		}
		for id, zone := range first {
			if zone != tier3.ZoneCommand {
				t.Errorf("game %d: companion %d was first drawn in %q; a "+
					"companion begins in the command zone and is never shuffled "+
					"in to be dealt", log.Game, id, zone)
			}
		}
	}
	t.Logf("two games, %d companions bought in from outside the game", bought)
}
