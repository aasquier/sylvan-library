package tier3

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// A whole bout, end to end, against something that is not Forge.
//
// **What `gameclock_test.go` proved about the salvage, this file asks of the
// ordinary path.** That file has the segment restart, the cut game and the
// seed; what nothing reached was the rest of [Settings.RunGames] — the command
// line it builds, the scribe's half of the choice, the second coverage check
// that discards a run Forge complained about, and the whole-subprocess ceiling.
// `ci.yml` lists that body as unreachable "without a JVM with Forge", and it is
// not: [Settings.Java] is the JVM this machine will run, so a machine whose
// `java` is a shell script is a machine this package will hand a command line
// to and read back. Nothing in the production path changes — the script is a
// *machine's* configuration, and a real deployment's `MTGLAB_JAVA` is honoured
// in exactly the same way.
//
// What the fake says is real Forge output: the result lines are the format
// strings in `forge.view.SimulateMatch` (see `parse.go`), and two of the tests
// below simply `cat` the frozen corpora — a real narrated game and a real
// scribed match — so the bytes crossing the pipe are bytes Forge wrote.
//
// **It cannot replace `live_test.go`.** A script proves the plumbing; only a
// JVM proves the claims about the world — that the profile file is read, that
// the format strings are still these, that 470MB of card scripts say what the
// index thinks. The two answer different questions and both are wanted.

// gamesAsked is the shell that reads Forge's own `-n` off the command line, so
// a fake plays the number of games it was asked for rather than a number the
// test wrote down twice.
const gamesAsked = `asked=1
prev=
for arg in "$@"; do
  if [ "$prev" = "-n" ]; then asked=$arg; fi
  prev=$arg
done
`

// arena is a machine with a distribution, a profile of its own, and a `java`
// that runs `body`. Everything [Settings.RunGames] resolves before it spawns
// anything is real: the cardsfolder is a zip, the jar is a file with a version
// in its name, and the profile is written where Forge would read it.
func arena(t *testing.T, body string) Settings {
	t.Helper()
	home := fakeForge(t, "1.6.50", "Sol Ring", "Forest")
	java, _ := fakeJava(t, home, body)
	return Settings{
		Home:    home,
		Profile: filepath.Join(t.TempDir(), "profile"),
		Java:    java,
		Index:   NewCardIndex(),
	}
}

// twoDecks is the smallest legal table, both decks covered by [arena]'s
// cardsfolder.
func twoDecks() []*deck.Deck { return []*deck.Deck{testDeck("first"), testDeck("second")} }

// corpus is an absolute path into `testdata`, so a fake running with Forge's
// home as its working directory can still find it.
func corpus(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// A bout comes back whole, and the command line under it is Forge's own
// spelling: the decks by filename, the format, the count, the clock, quiet
// unless somebody is watching, and the seed as text.
func TestABoutPlaysEveryGameAndComesBackWhole(t *testing.T) {
	t.Parallel()
	forge := arena(t, gamesAsked+`i=1
while [ "$i" -le "$asked" ]; do
  echo "Game Outcome: Turn 9"
  echo "Game Result: Game $i ended in 8055 ms. Ai(2)-second has won!"
  i=$((i + 1))
done
`)

	run, err := forge.RunGames(twoDecks(), RunOptions{Games: 3, Seed: big.NewInt(7)})
	if err != nil {
		t.Fatalf("a bout against a fake Forge failed: %v", err)
	}

	if got := len(run.Games()); got != 3 {
		t.Fatalf("the bout came back with %d games, want 3", got)
	}
	for i, g := range run.Games() {
		if g.Turns == nil || *g.Turns != 9 {
			t.Errorf("game %d lost the turn count: %v", i+1, g.Turns)
		}
		if got := run.WinnerSlug(g); got != "second" {
			t.Errorf("game %d was won by %q, want the deck in seat 2", i+1, got)
		}
	}
	// Seats are the order the decks were passed, and `SeatSlug` is the lookup
	// every question about a seat number asks.
	if run.SeatSlug(1) != "first" || run.SeatSlug(2) != "second" {
		t.Errorf("the seats are %v", run.Seats)
	}
	if run.SeatSlug(9) != "" {
		t.Error("a chair the run does not have named a deck")
	}
	// The pre-flight that ran before the JVM rides along on the result, which
	// is what lets a caller say which cards were checked.
	if len(run.Coverage) != 2 || run.Coverage[0].Checked != 2 {
		t.Errorf("the coverage reports are %+v", run.Coverage)
	}
	// Which Forge played, read off the jar's own name, because an upgrade
	// changes the instrument every recorded game was measured with (ADR 36).
	if run.ForgeVersion != "1.6.50" {
		t.Errorf("the run recorded Forge %q", run.ForgeVersion)
	}
	if run.WallSeconds <= 0 {
		t.Errorf("the bout took %v seconds of wall clock", run.WallSeconds)
	}
	// Startup is wall time not spent inside a game, floored at zero: three
	// recorded games claiming 8055ms each are more than a shell script spends,
	// and a negative startup is not a measurement.
	if got := run.StartupSeconds(); got != 0 {
		t.Errorf("startup is %v, want the floor", got)
	}

	argv := strings.Join(run.Argv, " ")
	for _, want := range []string{"-jar", "sim -d", "first.dck second.dck",
		"-f Commander", "-n 3", "-c 300", "-q", "-s 7", "-Xmx4096m"} {
		if !strings.Contains(argv, want) {
			t.Errorf("the command line has no %q in it: %s", want, argv)
		}
	}
	// And the `.dck` files really were written where Forge looks for them --
	// which is what `forge.profile.properties` moved, and the one thing this
	// package reaches into somebody else's installation to do.
	for _, slug := range []string{"first", "second"} {
		path := filepath.Join(forge.Profile, "decks", "commander", slug+".dck")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was never written: %v", path, err)
		}
	}
}

// A run Forge itself complained about is discarded after the fact, however
// ordinary its games look.
//
// This is the second of the two coverage checks and the reason there are two:
// an unimplemented card does not stop a game, it prints a warning and plays on,
// reporting a winner and a turn count that look entirely normal. The first
// check reads the card scripts before a JVM starts; this one reads Forge's own
// mouth afterwards, and discards results that are otherwise perfectly plausible.
func TestForgesOwnComplaintsInvalidateAnOtherwiseNormalBout(t *testing.T) {
	t.Parallel()
	forge := arena(t, `echo 'An unsupported card was requested: "Chromatic Vortex" from "[N.A.]".'
echo 'An unsupported card was requested: "Chromatic Vortex" from "[N.A.]".'
echo 'Could not load deck - second, match cannot start'
echo 'Game Result: Game 1 ended in 8055 ms. Ai(1)-first has won!'
`)

	_, err := forge.RunGames(twoDecks(), RunOptions{Games: 1})
	if err == nil {
		t.Fatal("a run Forge complained about came back as a result")
	}
	if !errors.Is(err, ErrResultsUntrustworthy) {
		t.Fatalf("the refusal is %T, want ErrResultsUntrustworthy", err)
	}
	for _, want := range []string{"dropped card: Chromatic Vortex",
		"deck failed to load: second"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	// Forge repeats the complaint per copy and a name is a name, so the
	// refusal lists it once.
	if n := strings.Count(err.Error(), "Chromatic Vortex"); n != 1 {
		t.Errorf("the dropped card is named %d times", n)
	}
}

// The diagnosis for a subprocess that played nothing carries the **tail** of
// what it said rather than the whole of it: a JVM's boot is hundreds of lines
// and the last fifteen are the ones with the fault in them.
func TestASilentForgesDiagnosisCarriesTheTailRatherThanTheWholeLog(t *testing.T) {
	t.Parallel()
	forge := arena(t, `i=1
while [ "$i" -le 40 ]; do
  echo "line $i of the JVM's own noise"
  i=$((i + 1))
done
`)

	_, err := forge.RunGames(twoDecks(), RunOptions{Games: 1})
	if err == nil {
		t.Fatal("a subprocess that played nothing came back as a result")
	}
	if !strings.Contains(err.Error(), "Forge produced no game results") {
		t.Fatalf("the diagnosis reads %q", err)
	}
	if !strings.Contains(err.Error(), "line 40 of") {
		t.Error("the diagnosis dropped the last thing the subprocess said")
	}
	if strings.Contains(err.Error(), "line 1 of") {
		t.Error("the diagnosis carried the whole output rather than the tail")
	}
}

// A match that outruns its whole-subprocess budget is cut and reported as the
// timeout it is — `TimeoutExpired`, the class name the shim renders — with the
// command line in the message, because a bout that blew its budget is a bout
// somebody has to be able to look at.
func TestAMatchThatOutrunsItsBudgetIsCutRatherThanWaitedOut(t *testing.T) {
	t.Parallel()
	// The trailing `:` is `sleeper`'s shape: a shell that stays behind what it
	// started, so the kill has a tree to reach rather than a lone process.
	forge := arena(t, "sleep 120; :\n")

	_, err := forge.RunGames(twoDecks(), RunOptions{
		Games: 1, Timeout: 300 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("a subprocess that never finished came back as a result")
	}
	if !errors.Is(err, ErrTimedOut) {
		t.Fatalf("the refusal is %T, want ErrTimedOut", err)
	}
	if !strings.Contains(err.Error(), "timed out after 0.3 seconds") {
		t.Errorf("the message does not name the budget: %v", err)
	}
	if !strings.Contains(err.Error(), "sim -d") {
		t.Errorf("the message does not carry the command line: %v", err)
	}
}

// Narrating drops Forge's `-q`, so the subprocess tells the whole game and the
// beats reach whoever is watching — once per game, after the result line closes
// it, because a person cannot watch eight seconds of Commander.
//
// The fake plays back a real narrated game, so what crosses the pipe is what
// Forge wrote.
func TestANarratedBoutCarriesTheBeatsAsWellAsTheRow(t *testing.T) {
	t.Parallel()
	forge := arena(t, "cat '"+corpus(t, "narrated-game.log")+"'\n")

	var watched []EventLog
	run, err := forge.RunGames(twoDecks(), RunOptions{
		Games: 1, Narrate: true,
		OnEvents: func(log EventLog) { watched = append(watched, log) },
	})
	if err != nil {
		t.Fatalf("a narrated bout failed: %v", err)
	}
	if len(run.Games()) != 1 {
		t.Fatalf("the narrated bout came back with %d games", len(run.Games()))
	}
	if len(run.Events) != 1 || len(watched) != 1 {
		t.Fatalf("the run carries %d logs and %d reached the watcher",
			len(run.Events), len(watched))
	}
	if len(run.Events[0].Events) == 0 {
		t.Error("a whole game log produced no beats at all")
	}
	// The board is empty on this path and that is deliberate: Forge's own log
	// has no category for a token or a counter, so a board reconstructed from
	// prose would be right about lands and silently wrong about exactly the
	// decks that most want a picture (ADR 42).
	if run.Events[0].Board != nil {
		t.Error("the prose path invented a board")
	}
	if strings.Contains(strings.Join(run.Argv, " "), " -q") {
		t.Error("a narrated run still asked Forge to be quiet")
	}
}

// With the scribe's classes on the machine, the match is played through them
// rather than through Forge's own `sim`, and everything downstream is identical
// — the tick, the tally, the trustworthiness check. That is ADR 42's fifth
// decision: the board is a renderer, not a second pipeline.
//
// **The choice is the presence of the classes and never a flag**, so this
// describes a worker image that has them and plays back a real scribed match.
func TestAMachineWithTheScribeRunsTheMatchThroughIt(t *testing.T) {
	t.Parallel()
	classes := t.TempDir()
	if err := os.MkdirAll(filepath.Join(classes, "scribe"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(classes, "scribe", "Main.class"),
		[]byte("not really bytecode"), 0o600); err != nil {
		t.Fatal(err)
	}
	forge := arena(t, "cat '"+corpus(t, "scribed-match.ndjson")+"'\n")
	forge.ScribeClasses = classes

	run, err := forge.RunGames(twoDecks(), RunOptions{
		Games: 2, Clock: 900, Seed: big.NewInt(11), Narrate: true,
	})
	if err != nil {
		t.Fatalf("a scribed match failed: %v", err)
	}
	if len(run.Games()) != 2 {
		t.Fatalf("the scribed match came back with %d games", len(run.Games()))
	}
	// Positional and dumb, because this is the only place that builds it:
	// scribe.Main <clock> <games> <seed|-> <deck.dck> ...
	argv := strings.Join(run.Argv, " ")
	for _, want := range []string{"-cp", "scribe.Main 900 2 11",
		"first.dck", "second.dck", classes} {
		if !strings.Contains(argv, want) {
			t.Errorf("the scribe's command line has no %q in it: %s", want, argv)
		}
	}
	// Asked of the words rather than of the joined line: the jar's own filename
	// has `-jar-with-dependencies` in it, so a substring search here would pass
	// whatever the command line said.
	for _, word := range run.Argv {
		if word == "-jar" || word == "sim" {
			t.Errorf("the scribed path still asked for Forge's own `sim`: %v", run.Argv)
			break
		}
	}
	// The board is the thing only this path can produce, and it is why the
	// classes are on the machine at all.
	if len(run.Events) != 2 || run.Events[0].Board == nil {
		t.Error("a scribed match produced no board")
	}

	// A worker whose classes are not where the variable says plays the match
	// through `sim` and narrates from the log -- degrade rather than fail, the
	// rule every hop of this wire follows.
	forge.ScribeClasses = filepath.Join(t.TempDir(), "never-built")
	if forge.Scribed() {
		t.Error("a path with no scribe in it read as scribed")
	}
}

// An unseeded scribed match says so with a dash rather than leaving the
// position empty, because the command line is positional and a missing
// argument would shift every deck path one place left.
func TestAnUnseededScribedMatchNamesTheSeedPositionAnyway(t *testing.T) {
	t.Parallel()
	classes := t.TempDir()
	if err := os.MkdirAll(filepath.Join(classes, "scribe"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(classes, "scribe", "Main.class"),
		[]byte("not really bytecode"), 0o600); err != nil {
		t.Fatal(err)
	}
	forge := arena(t, "cat '"+corpus(t, "scribed-match.ndjson")+"'\n")
	forge.ScribeClasses = classes

	run, err := forge.RunGames(twoDecks(), RunOptions{Games: 2})
	if err != nil {
		t.Fatalf("an unseeded scribed match failed: %v", err)
	}
	if !strings.Contains(strings.Join(run.Argv, " "), "scribe.Main 300 2 -") {
		t.Errorf("an unseeded match's command line is %v", run.Argv)
	}
}

// Every refusal a bout raises before a JVM starts, each with the thing that is
// wrong with the machine or with the ask. They come first because they are all
// cheaper than a subprocess, and because a diagnosis is worth more than a
// crash — commandment 2, at the one moment a beginner meets this package.
func TestABoutRefusesEverythingItCannotPlayBeforeSpawningAnything(t *testing.T) {
	t.Parallel()
	// A `java` that complains if anything reaches it: nothing in this test may
	// spawn a match at all.
	never := "echo 'a JVM was started for a bout that should have been refused' >&2\nexit 3\n"
	for _, c := range []struct {
		what  string
		build func(t *testing.T) (Settings, []*deck.Deck)
		want  string
	}{
		{
			what: "a table with one chair",
			build: func(t *testing.T) (Settings, []*deck.Deck) {
				return arena(t, never), []*deck.Deck{testDeck("alone")}
			},
			want: "a game needs at least two decks",
		},
		{
			what: "a card Forge does not implement",
			build: func(t *testing.T) (Settings, []*deck.Deck) {
				d := testDeck("second")
				d.Cards = append(d.Cards, deck.CardEntry{Name: "Chromatic Vortex"})
				return arena(t, never), []*deck.Deck{testDeck("first"), d}
			},
			want: "Chromatic Vortex",
		},
		{
			what: "no distribution at all",
			build: func(t *testing.T) (Settings, []*deck.Deck) {
				forge := arena(t, never)
				forge.Home = filepath.Join(t.TempDir(), "gone")
				return forge, twoDecks()
			},
			want: "MTGLAB_FORGE_HOME",
		},
		{
			what: "a slug that would name a file nobody asked for",
			build: func(t *testing.T) (Settings, []*deck.Deck) {
				d := testDeck("second")
				d.Slug = "../../etc/passwd"
				return arena(t, never), []*deck.Deck{testDeck("first"), d}
			},
			want: "is not a usable slug",
		},
		{
			what: "no JVM anywhere",
			build: func(t *testing.T) (Settings, []*deck.Deck) {
				forge := arena(t, never)
				forge.Java = filepath.Join(t.TempDir(), "no-java-here")
				return forge, twoDecks()
			},
			want: fmt.Sprintf("no Java %d+ found", JavaMinimum),
		},
		{
			what: "a distribution with no desktop jar in it",
			build: func(t *testing.T) (Settings, []*deck.Deck) {
				forge := arena(t, never)
				forge.Home = fakeForge(t, "", "Sol Ring", "Forest")
				return forge, twoDecks()
			},
			want: "no Forge desktop jar in",
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			forge, decks := c.build(t)
			run, err := forge.RunGames(decks, RunOptions{Games: 1})
			if err == nil {
				t.Fatalf("%s played %d games", c.what, len(run.Games()))
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s was refused with %q, want a mention of %q",
					c.what, err, c.want)
			}
		})
	}
}

// The numbers a bout fills in for itself, asked of the command line it built
// rather than of the struct it was handed: one game, Forge's `-c` at this
// repo's 300 rather than Forge's own 120, and the heap `run_games` has always
// asked for.
func TestABoutWithNothingNamedFillsInItsOwnNumbers(t *testing.T) {
	t.Parallel()
	forge := arena(t, gamesAsked+`i=1
while [ "$i" -le "$asked" ]; do
  echo "Game Result: Game $i ended in 90 ms. Ai(1)-first has won!"
  i=$((i + 1))
done
`)

	run, err := forge.RunGames(twoDecks(), RunOptions{})
	if err != nil {
		t.Fatalf("a bout with no options at all failed: %v", err)
	}
	if len(run.Games()) != 1 {
		t.Errorf("a bout with no count played %d games", len(run.Games()))
	}
	argv := strings.Join(run.Argv, " ")
	for _, want := range []string{"-n 1", "-c 300", fmt.Sprintf("-Xmx%dm", MemoryDefault)} {
		if !strings.Contains(argv, want) {
			t.Errorf("the filled-in command line has no %q: %s", want, argv)
		}
	}
	// No seed named is no `-s` at all: Forge picks its own, rather than being
	// handed a zero that would make every unseeded bout the same match.
	if strings.Contains(argv, " -s ") {
		t.Errorf("an unseeded bout named a seed: %s", argv)
	}
}
