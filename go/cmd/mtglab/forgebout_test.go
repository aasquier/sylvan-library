package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// `mtglab sim forge` reporting a whole bout, on a machine with no JVM.
//
// **`ci.yml` lists this command's reporting as unreachable and it is not.**
// The report is the product here — a tally per deck, the wall clock split
// between the JVM and the games, the draws, the clock-outs, and the sentence
// about what Forge's AI is good at — and not one line of it had ever been
// read by anything, because the only way to reach it was to own 470MB of
// Forge. [tier3.Settings.Java] is the JVM this machine will run, so a machine
// whose `java` is a shell script is a machine the runner hands a command line
// to and reads back. Nothing in the production path changes: a real
// deployment's `MTGLAB_JAVA` is honoured in exactly the same way, and
// `internal/sim/tier3`'s own bout tests have been doing this since the
// clock-out landed.
//
// What the fake says is real Forge output — the result lines are the format
// strings in `forge.view.SimulateMatch`, which `parse.go` reads — so the bytes
// this command's report is built from are bytes Forge writes.
//
// **It cannot replace the live test.** `TestTheGoShimPlaysARealMatchForTheGoClient`
// and `internal/sim/tier3`'s `live_test.go` prove the claims about the world;
// a script proves that this file's printing says what it means to say.

// forgeMachine is a machine with a distribution, a profile of its own, and a
// `java` that runs `body`. Everything the runner resolves before it spawns
// anything is real: the cardsfolder is a zip, the jar is a file with a version
// in its name, and the deck files are written where Forge would read them.
func forgeMachine(t *testing.T, body string) tier3.Settings {
	t.Helper()
	return tier3.Settings{
		Home:    fakeForgeHome(t, "1.6.50", "Sol Ring", "Forest"),
		Profile: filepath.Join(t.TempDir(), "profile"),
		Java:    fakeJavaBinary(t, body),
	}
}

// fakeForgeHome is a Forge distribution good enough for every check that does
// not need a JVM: a versioned desktop jar and a cardsfolder zip holding the
// named card scripts. The same shape `internal/sim/tier3` builds for its own
// tests, spelled again here because an unexported helper does not cross a
// package boundary.
func fakeForgeHome(t *testing.T, version string, cards ...string) string {
	t.Helper()
	home := t.TempDir()
	jar := filepath.Join(home,
		fmt.Sprintf("forge-gui-desktop-%s-jar-with-dependencies.jar", version))
	if err := os.WriteFile(jar, []byte("not really a jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "res", "cardsfolder", "cardsfolder.zip")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path) //nolint:gosec // a test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for i, card := range cards {
		w, err := zw.Create(fmt.Sprintf("cardsfolder/%c/card%d.txt",
			strings.ToLower(card)[0], i))
		if err != nil {
			t.Fatal(err)
		}
		// Forge's card scripts lead with the name and carry more after it.
		if _, err := fmt.Fprintf(w, "Name:%s\nManaCost:G\nTypes:Creature\n", card); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return home
}

// fakeJavaBinary is a stand-in for the JVM: it answers the version probe the
// runner insists on, and then says whatever the test scripted.
//
// The version probe is not optional scaffolding — a binary that will not
// answer `-version` is not a candidate at all, which is the rule that keeps
// this machine's Java 10 from failing Forge in a way that reads like a Forge
// bug.
func fakeJavaBinary(t *testing.T, body string) string {
	t.Helper()
	java := filepath.Join(t.TempDir(), "java")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-version\" ]; then\n" +
		"  echo 'openjdk version \"21.0.1\" 2023-10-17' 1>&2\n" +
		"  exit 0\n" +
		"fi\n" + body
	if err := os.WriteFile(java, []byte(script), 0o700); err != nil { //nolint:gosec // a test's own temp dir, and it has to be executable
		t.Fatal(err)
	}
	return java
}

// forgeDeck is one legal deck file, every card of it covered by
// [forgeMachine]'s cardsfolder.
func forgeDeck(t *testing.T, d deployment, slug string) {
	t.Helper()
	writeSimDeck(t, d, slug, "slug: "+slug+"\nname: "+strings.ToUpper(slug)+
		"\ncommander:\n  - Sol Ring\ncards:\n  - name: Forest\n    why: a land\n")
}

// A bout of three games, reported whole: the tally names each deck by its
// slug, the draw is counted separately from the wins, and the clock-out is
// counted separately from the draw.
//
// **The clock-out is its own line on purpose**, and the report would be wrong
// without it: a game that hit the clock is the measurement giving up, not the
// game ending, and folding it into the draw rate would report the simulator's
// own limit as a fact about the decks.
func TestAForgeBoutIsTalliedPerDeckWithItsDrawsAndClockOuts(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	d.Forge = forgeMachine(t, `echo "Game Outcome: Turn 9"
echo "Game Result: Game 1 ended in 8055 ms. Ai(1)-first has won!"
echo "Game Outcome: Turn 12"
echo "Game Result: Game 2 ended in 9100 ms. Ai(2)-second has won!"
echo "Stopping slow match as draw"
echo "Game Result: Game 3 ended in a Draw! Took 300000 ms."
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")

	out, err := d.run(t, "sim", "forge", "first", "second",
		"--games", "3", "--clock", "300", "--seed", "7")
	if err != nil {
		t.Fatalf("a bout against a stand-in JVM failed: %v", err)
	}
	t.Logf("sim forge:\n%s", out)

	for _, want := range []string{
		"3 games in ",
		"s of it JVM + card database)",
		"per game: ",
		"s min / ",
		"s max\n",
		"  first                  1\n",
		"  second                 1\n",
		"  draw                   1\n",
		"  (1 hit the 300s clock and were called draws)\n",
		"Forge's AI is best at aggro and midrange",
		"Read these per archetype, not as one ranking.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report is missing %q", want)
		}
	}
	// The clock-out is a draw for the tally **and** its own line, which is the
	// arithmetic that would be wrong if either half were dropped: one win
	// each, one draw, three games.
	if strings.Contains(out, "  draw                   2\n") {
		t.Error("the clock-out was counted twice")
	}
}

// A match that was played and recorded is a match `sim matches` can read back,
// which is the whole of ADR 36 from this end: the CLI and the API job are the
// two places a match finishes, and both record.
//
// The ledger is minted here rather than found — `recordForgeMatch` may create
// `app.db`, unlike `sim cache`, because a match silently unrecorded on a fresh
// machine would be a regression.
func TestAFinishedBoutIsWaitingInTheLedgerAfterwards(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	d.Forge = forgeMachine(t, `echo "Game Outcome: Turn 7"
echo "Game Result: Game 1 ended in 5000 ms. Ai(2)-second has won!"
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")

	if _, err := os.Stat(d.AppDBPath()); err == nil {
		t.Fatal("app.db was there before the match; this test measures nothing")
	}
	if _, err := d.run(t, "sim", "forge", "first", "second", "--games", "1"); err != nil {
		t.Fatalf("the bout failed: %v", err)
	}

	out, err := d.run(t, "sim", "matches")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	t.Logf("sim matches:\n%s", out)
	for _, want := range []string{
		"ledger: " + d.AppDBPath(),
		// Played locally and unseeded, which is what the run said about itself.
		"(local, Forge 1.6.50, unseeded)",
		"  first                   0 wins  (unlabelled)",
		"  second                  1 win  (unlabelled)",
		"  1 of 1 games",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the ledger is missing %q", want)
		}
	}
}

// A seeded bout records its seed, and the ledger says so rather than calling
// it unseeded — the difference between a match somebody can replay and one
// nobody can.
func TestASeededBoutIsRecordedWithItsSeed(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	d.Forge = forgeMachine(t, `echo "Game Result: Game 1 ended in a Draw! Took 4000 ms."
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")

	if _, err := d.run(t, "sim", "forge", "first", "second",
		"--games", "1", "--seed", "4242"); err != nil {
		t.Fatalf("the bout failed: %v", err)
	}
	out, err := d.run(t, "sim", "matches", "--limit", "1")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	if !strings.Contains(out, "seed 4242") {
		t.Errorf("the ledger forgot the seed:\n%s", out)
	}
	// A real draw is not a clock-out, and the ledger keeps them apart: this
	// game ended in a draw without the slow-match warning above it.
	if !strings.Contains(out, "1 real draw") {
		t.Errorf("the draw was not recorded as one:\n%s", out)
	}
	if strings.Contains(out, "hit the") {
		t.Errorf("a draw was recorded as a clock-out:\n%s", out)
	}
}

// Narrating tells each game as it is played, beat by beat, and the beats are
// the same ones the match theater animates — which is the argument for putting
// them on the terminal first: a beat nobody can follow here is a beat nothing
// can animate there.
//
// The seats are named by deck slug rather than by number, because "seat 2
// attacks seat 1" is not a sentence about a game of Magic.
func TestANarratedBoutTellsTheGameBySeatName(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	// Forge's own log, quoted: a turn header, a land, a spell resolving, an
	// attack, damage, a death and the outcome.
	d.Forge = forgeMachine(t, `echo "Turn 3 (Ai(1)-first)"
echo "Ai(1)-first played Forest"
echo "Ai(2)-second played Forest"
echo "Ai(1)-first has won!"
echo "Game Outcome: Turn 3"
echo "Game Result: Game 1 ended in 3000 ms. Ai(1)-first has won!"
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")

	out, err := d.run(t, "sim", "forge", "first", "second",
		"--games", "1", "--narrate")
	if err != nil {
		t.Fatalf("a narrated bout failed: %v", err)
	}
	t.Logf("sim forge --narrate:\n%s", out)

	if !strings.Contains(out, "--- game 1 ---") {
		t.Errorf("the narration never opened a game:\n%s", out)
	}
	// The seat map is this command's own, built from the order the decks were
	// passed, because narration happens *during* the run when there is no run
	// yet to read seats off.
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Errorf("the narration never named a deck:\n%s", out)
	}
	if strings.Contains(out, "seat 1 ") || strings.Contains(out, "seat 2 ") {
		t.Errorf("the narration fell back to seat numbers:\n%s", out)
	}
}

// A bout Forge itself complained about is refused rather than reported, and
// the refusal names the class — `ResultsUntrustworthy`, which is the half of
// the sentence that survives translation into a job row.
//
// This is the second of the two coverage checks and the reason there are two:
// an unimplemented card does not stop a game, it prints a warning and plays
// on, reporting a winner and a turn count that look entirely normal.
func TestABoutForgeComplainedAboutIsRefusedRatherThanTallied(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	d.Forge = forgeMachine(t, `echo 'An unsupported card was requested: "Chromatic Vortex" from "[N.A.]".'
echo "Game Result: Game 1 ended in 1000 ms. Ai(1)-first has won!"
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")

	out, err := d.run(t, "sim", "forge", "first", "second", "--games", "1")
	if err == nil {
		t.Fatalf("a run Forge complained about was reported anyway:\n%s", out)
	}
	if !strings.Contains(err.Error(), "Chromatic Vortex") {
		t.Errorf("the refusal said %q without naming the card", err)
	}
}

// The pre-flight on its own: `--check-only` reads the card scripts, needs no
// JVM, and is the only half of this command that works without one.
//
// A deck Forge covers whole gets its summary printed; a deck with a card the
// distribution does not implement is refused **and the card is named**,
// because the operator's next move is to cut it or to leave the deck out of
// the bout.
func TestTheCoveragePreFlightNamesWhatForgeDoesNotHave(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	// A machine that has Forge but not every card, so what follows is about
	// the cards rather than about the distribution.
	d.Forge = tier3.Settings{Home: fakeForgeHome(t, "1.6.50", "Sol Ring", "Forest")}
	forgeDeck(t, d, "covered")
	writeSimDeck(t, d, "short", "slug: short\nname: Short\ncommander:\n  - Sol Ring\n"+
		"cards:\n  - name: Chromatic Vortex\n    why: it is not real\n")

	out, err := d.run(t, "sim", "forge", "covered", "covered", "--check-only")
	if err != nil {
		t.Fatalf("the pre-flight failed on a deck Forge covers whole: %v", err)
	}
	if !strings.Contains(out, "covered") {
		t.Errorf("the pre-flight printed no summary:\n%s", out)
	}

	_, err = d.run(t, "sim", "forge", "covered", "short", "--check-only")
	if err == nil {
		t.Fatal("a deck full of cards Forge has never heard of passed the pre-flight")
	}
	if !strings.Contains(err.Error(), "Chromatic Vortex") {
		t.Errorf("the refusal said %q without naming the card", err)
	}
}
