package tier3

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// Bounding **one game**, and keeping the bout that game was part of.
//
// The live fault, 2026-08-31: a game of a ten-game bout on the deployed arena
// ran fifteen minutes past a three-hundred second clock and nothing cut it. The
// app's silence budget was right to stay quiet — the game narrated the whole
// way — and every other bound in the chain kills the bout, so the only ceiling
// left would have thrown away nine finished games to reach the tenth.
//
// Two claims are being tested, and they are the two halves of the fix. A game
// that talks forever is cut anyway, because a beat is not progress. And the cut
// costs one game rather than the match.
//
// These drive the real trigger — real subprocesses, killed by the real pace
// timer — with `/bin/sh` standing in for the JVM, because what is under test is
// the cutting and the salvage rather than the playing.

// wedge is the game that cannot be stopped: a subprocess that talks forever and
// finishes nothing. `before` is whatever it says first.
//
// **Nested shells, for the reason [sleeper] gives**, and it matters more here
// than it does there. The deployed worker points `MTGLAB_JAVA` at a `/bin/sh`
// wrapper around `xvfb-run`, which is another `/bin/sh` that *forks* what it
// was handed — so the thing playing the games is a grandchild, and a kill
// aimed at the direct child leaves it holding the write end of the pipe this
// package reads to EOF. That is not a smaller bug than the one being fixed; it
// is the same hour-long hang wearing different clothes.
func wedge(before string) []string {
	return []string{"/bin/sh", "-c", before +
		`sh -c 'while :; do echo "the game goes on"; sleep 0.05; done' ; :`}
}

// won is the line Forge prints to end a game, in the format `parse.go` reads
// off `forge.view.SimulateMatch`'s own format strings.
func won(game, seat int, ms int) string {
	return fmt.Sprintf(
		`echo "Game Result: Game %d ended in %d ms. Ai(%d)-deck has won!" ; `,
		game, ms, seat)
}

// **The chatty wedge: the exact case that ran fifteen minutes over.** A game
// that narrates is a game every silence-shaped bound is correct to leave alone,
// so if this is ever cut it can only be by something counting *games*.
func TestAGameThatNarratesForeverIsStillCutAtItsCeiling(t *testing.T) {
	t.Parallel()
	// The whole-subprocess ceiling is far beyond anything this waits for, so a
	// pass can only mean the per-game ceiling did it.
	opt := RunOptions{Timeout: time.Hour, GameCeiling: 300 * time.Millisecond}

	done := make(chan *spawned, 1)
	go func() {
		run, err := spawn(wedge(""), t.TempDir(), opt, newProseTelling(false))
		if err != nil {
			t.Errorf("a clocked-out game came back as an error: %v", err)
		}
		done <- run
	}()

	select {
	case run := <-done:
		if run == nil {
			t.Fatal("no run came back from a clocked-out game")
		}
		if !run.clockedOut {
			t.Error("a game that outran its ceiling was not reported as a " +
				"clock-out")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("a game that narrated forever outlived its ceiling — this is " +
			"the game that held a deployed bout for fifteen minutes")
	}
}

// **A beat is not a game.** The same wedge, asked the other way round: it emits
// roughly twenty lines per second, so by the time the ceiling falls it has said
// plenty — and it must be cut anyway. The bug this pins is the tempting one:
// rearming the pace timer on any output at all, which would restore exactly the
// budget that already failed to notice.
func TestNarrationDoesNotRearmTheGameCeiling(t *testing.T) {
	t.Parallel()
	beats := 0
	read := &counting{inner: newProseTelling(false), beat: func() { beats++ }}

	started := time.Now()
	run, err := spawn(wedge(""), t.TempDir(),
		RunOptions{Timeout: time.Hour, GameCeiling: 400 * time.Millisecond}, read)
	if err != nil {
		t.Fatalf("a clocked-out game came back as an error: %v", err)
	}
	if !run.clockedOut {
		t.Fatal("the wedge was not cut at all")
	}
	if beats < 2 {
		t.Fatalf("the wedge only spoke %d times, so this never asked its "+
			"question — it wants a game that is talking when the clock falls",
			beats)
	}
	// Generous, because a loaded machine is allowed to be slow; what it refuses
	// is a cut that never came until something else ended the process.
	if waited := time.Since(started); waited > 20*time.Second {
		t.Errorf("the cut took %v against a %v ceiling", waited,
			400*time.Millisecond)
	}
}

// counting is a [telling] that reports how much was said, so a test can prove
// the wedge was mid-sentence rather than silent when the ceiling fell.
type counting struct {
	inner telling
	beat  func()
}

func (c *counting) Feed(line string) (*EventLog, *GameResult) {
	c.beat()
	return c.inner.Feed(line)
}

func (c *counting) Output() SimOutput { return c.inner.Output() }

// **The games played before the cut are the whole point.** Every bound this
// package had before returned an error and nothing else, which on a bout of ten
// meant nine finished games thrown away to report the tenth.
func TestTheGamesPlayedBeforeACutSurviveIt(t *testing.T) {
	t.Parallel()
	run, err := spawn(wedge(won(1, 1, 1200)+won(2, 2, 1400)), t.TempDir(),
		RunOptions{Timeout: time.Hour, GameCeiling: 400 * time.Millisecond},
		newProseTelling(false))
	if err != nil {
		t.Fatalf("a clocked-out game came back as an error: %v", err)
	}
	if !run.clockedOut {
		t.Fatal("the third game outran its ceiling and was not cut")
	}
	if got := len(run.Output.Games); got != 2 {
		t.Fatalf("%d games survived the cut, want the 2 that finished", got)
	}
	if run.Output.Games[1].WinnerSeat == nil ||
		*run.Output.Games[1].WinnerSeat != 2 {
		t.Error("a game that finished before the cut lost its winner")
	}
}

// **The cut reaches the whole tree, or it reaches nothing.** Reaching this
// assertion at all is the property: `spawn` reads its subprocess to EOF, and a
// pipe reaches EOF only once every process holding the write end is gone — so a
// grandchild that survived would hold this open until the test's own deadline,
// which is [wedge]'s hour-long hang in miniature.
func TestACutGameTakesTheWholeTreeWithIt(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := spawn(wedge(""), t.TempDir(),
			RunOptions{Timeout: time.Hour, GameCeiling: 300 * time.Millisecond},
			newProseTelling(false)); err != nil {
			t.Errorf("a clocked-out game came back as an error: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the cut left a grandchild holding the pipe — the group kill " +
			"missed the process actually playing the game")
	}
}

// A bout whose games all arrive inside the ceiling is untouched by any of this,
// and the timer it never needed is rearmed by each one rather than running down
// across the match.
func TestGamesArrivingUnderTheCeilingAreNeverCut(t *testing.T) {
	t.Parallel()
	// Four games, a tenth of a second apart, under a ceiling shorter than the
	// four of them together: a timer armed once for the whole run would fire
	// here, and one rearmed per game cannot.
	var script strings.Builder
	for i := 1; i <= 4; i++ {
		script.WriteString(won(i, 1, 100))
		script.WriteString("sleep 0.1 ; ")
	}
	run, err := spawn([]string{"/bin/sh", "-c", script.String() + ":"},
		t.TempDir(),
		RunOptions{Timeout: time.Hour, GameCeiling: 3 * time.Second},
		newProseTelling(false))
	if err != nil {
		t.Fatalf("a bout of four ordinary games failed: %v", err)
	}
	if run.clockedOut {
		t.Error("a bout whose games all finished was reported as a clock-out")
	}
	if got := len(run.Output.Games); got != 4 {
		t.Errorf("%d games came back, want 4", got)
	}
}

// ---------------------------------------------------------- the whole bout

// fakeJava is a stand-in for the JVM that answers the version probe and then
// plays whatever the test scripted for *this* invocation.
//
// The invocation count is the point: a bout that clocks out a game runs a
// second subprocess, and nothing else in this file can tell one JVM from two.
func fakeJava(t *testing.T, segments ...string) (java string, runs func() int) {
	t.Helper()
	dir := t.TempDir()
	for i, body := range segments {
		path := filepath.Join(dir, fmt.Sprintf("segment%d.sh", i+1))
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	counted := filepath.Join(dir, "runs")
	java = filepath.Join(dir, "java")
	// The trailing `:` keeps this shell alive behind the segment it started,
	// which is the deployed shape [wedge] argues for: the thing playing the
	// games is a grandchild of the process `spawn` holds.
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"-version\" ]; then\n" +
		"  echo 'openjdk version \"21.0.1\" 2023-10-17' 1>&2\n" +
		"  exit 0\n" +
		"fi\n" +
		"n=1\n" +
		"if [ -f " + counted + " ]; then n=$(( $(cat " + counted + ") + 1 )); fi\n" +
		"echo $n > " + counted + "\n" +
		"/bin/sh " + dir + "/segment$n.sh ; :\n"
	if err := os.WriteFile(java, []byte(body), 0o700); err != nil { //nolint:gosec // a test's own temp dir, and it has to be executable
		t.Fatal(err)
	}
	return java, func() int {
		raw, err := os.ReadFile(counted) //nolint:gosec // the path this helper just wrote
		if err != nil {
			return 0
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &n); err != nil {
			return 0
		}
		return n
	}
}

// **The bout survives its casualty**, which is the whole reason the cut is
// scoped to a game instead of to the match.
//
// Five games asked for. The first subprocess plays two and then wedges on the
// third; the ceiling cuts it, that game is written as a clock-out, and a second
// subprocess plays the two that were still owed. What comes back is a whole
// bout — five rows, in the bout's own numbering, exactly one of them stopped by
// the clock — rather than the error every ceiling in this package used to
// return with nine finished games inside it.
func TestABoutPlaysOnAfterAGameIsClockedOut(t *testing.T) {
	t.Parallel()
	// The second segment numbers its own games 1 and 2, because a fresh JVM
	// starts counting again. They have to come back as 4 and 5.
	java, runs := fakeJava(t,
		strings.Join(wedge(won(1, 1, 1200) + won(2, 2, 1400))[2:], ""),
		won(1, 2, 900)+won(2, 1, 1100)+":")
	forge := Settings{Home: fakeForge(t, "2.0.14", "Sol Ring", "Forest"),
		Profile: t.TempDir(), Java: java}

	var ticks []int
	run, err := forge.RunGames(
		[]*deck.Deck{testDeck("alpha"), testDeck("beta")},
		RunOptions{Games: 5, Timeout: time.Minute,
			GameCeiling: 500 * time.Millisecond,
			Seed:        big.NewInt(7),
			OnGame: func(finished int, _ GameResult) {
				ticks = append(ticks, finished)
			}})
	if err != nil {
		t.Fatalf("a bout with one clocked-out game failed whole: %v", err)
	}

	if got := len(run.Games()); got != 5 {
		t.Fatalf("%d rows came back, want the 5 games asked for", got)
	}
	if got := runs(); got != 2 {
		t.Errorf("the bout ran %d subprocesses, want 2 — one cut and one "+
			"salvage", got)
	}
	var clocked []int
	for i, g := range run.Games() {
		if want := i + 1; g.Index != want {
			t.Errorf("row %d is numbered %d — a second arena renumbered the "+
				"bout from one", want, g.Index)
		}
		if g.TimedOut {
			clocked = append(clocked, g.Index)
		}
	}
	if len(clocked) != 1 || clocked[0] != 3 {
		t.Errorf("the clock-outs are %v, want game 3 alone", clocked)
	}
	// The tick is what the room draws its bar from, and what breaks the silence
	// the app would otherwise read as a dead worker.
	if want := []int{1, 2, 3, 4, 5}; fmt.Sprint(ticks) != fmt.Sprint(want) {
		t.Errorf("the room was ticked %v rather than %v", ticks, want)
	}
	// A clock-out belongs to nobody. Everything downstream reads it off this
	// field — the API masks the winner and the draw off the row, the tally
	// counts it in its own column, and the room says it was stopped by the
	// clock — so a cut game carrying a seat would put a trophy on a measurement
	// that gave up.
	cut := run.Games()[2]
	if cut.WinnerSeat != nil || cut.Winner != nil || cut.Draw {
		t.Errorf("the clocked-out game came back as %+v, want it belonging to "+
			"nobody and called no draw", cut)
	}
	if run.WinnerSlug(cut) != "" {
		t.Error("a clocked-out game named a winner")
	}
}

// **A bout that nothing cuts is the bout it always was.** Every property this
// file adds is reachable only through a clock-out, so the ordinary path is
// pinned separately: one subprocess, the games asked for, no clock-out row.
func TestAnUncutBoutStillRunsAsOneMatch(t *testing.T) {
	t.Parallel()
	java, runs := fakeJava(t, won(1, 1, 900)+won(2, 2, 1100)+":")
	forge := Settings{Home: fakeForge(t, "2.0.14", "Sol Ring", "Forest"),
		Profile: t.TempDir(), Java: java}

	run, err := forge.RunGames(
		[]*deck.Deck{testDeck("alpha"), testDeck("beta")},
		RunOptions{Games: 2, Timeout: time.Minute, Seed: big.NewInt(7)})
	if err != nil {
		t.Fatalf("an ordinary bout failed: %v", err)
	}
	if got := runs(); got != 1 {
		t.Errorf("an ordinary bout ran %d subprocesses, want 1", got)
	}
	for _, g := range run.Games() {
		if g.TimedOut {
			t.Errorf("game %d was called a clock-out in a bout nothing cut",
				g.Index)
		}
	}
}

// A subprocess that produces nothing and is cut by nothing is still the
// diagnosis it always was: a broken distribution or a deck Forge could not
// load, reported with the tail of what it said. The clock-out path must not
// swallow that, which it would if an empty segment were simply retried.
func TestASilentForgeIsStillReportedRatherThanRetried(t *testing.T) {
	t.Parallel()
	java, runs := fakeJava(t, `echo "Could not load deck - alpha, match cannot start" ; :`)
	forge := Settings{Home: fakeForge(t, "2.0.14", "Sol Ring", "Forest"),
		Profile: t.TempDir(), Java: java}

	_, err := forge.RunGames([]*deck.Deck{testDeck("alpha"), testDeck("beta")},
		RunOptions{Games: 5, Timeout: time.Minute, GameCeiling: time.Minute})
	if err == nil {
		t.Fatal("a Forge that loaded no deck produced a bout")
	}
	if got := runs(); got != 1 {
		t.Errorf("a broken Forge was asked %d times, want 1 — a segment that "+
			"nothing cut is not a segment to play again", got)
	}
}

// ------------------------------------------------------------- the numbers

// **A resumed segment must not replay the games already played.** Forge seeds
// one global generator before the first game — the scribe's PARITY 1 — so a
// fresh JVM handed the same number deals the same hands in the same order, and
// a resume on the original seed would replay what already finished and walk
// back into the game that wedged.
func TestAResumedSegmentDoesNotReplayTheGamesAlreadyPlayed(t *testing.T) {
	t.Parallel()
	seed := big.NewInt(1909)

	// **The first segment is the bout as it was asked for**, which is what the
	// parity gate rests on: a match with no clock-out in it builds the same
	// command line it built before any of this existed.
	if got := resumeSeed(seed, 0); got != seed {
		t.Errorf("the first segment plays under %v rather than the seed the "+
			"caller named", got)
	}
	if got := resumeSeed(seed, 3); got.Cmp(seed) == 0 {
		t.Error("a resumed segment plays the seed that was already played")
	}
	// Deterministic, so one number still describes a whole bout.
	if resumeSeed(seed, 3).Cmp(resumeSeed(seed, 3)) != 0 {
		t.Error("the same bout resumed twice picked two different seeds")
	}
	// Distinct per resume, so two clock-outs in one bout are two matches.
	if resumeSeed(seed, 3).Cmp(resumeSeed(seed, 4)) == 0 {
		t.Error("two different resumes of one bout play the same games")
	}
	// A caller who named no seed is letting Forge pick, and a new JVM picks
	// again on its own.
	if resumeSeed(nil, 3) != nil {
		t.Error("an unnamed seed was invented for the resume")
	}
	// The caller's own number is never mutated: it is echoed back to whoever
	// asked and recorded in the match ledger.
	if seed.Cmp(big.NewInt(1909)) != 0 {
		t.Errorf("the bout's seed was rewritten to %v", seed)
	}
}

// **One silence must outlast a segment restart**, or the fix for the wedge
// becomes a new way to be presumed dead. The longest quiet a healthy bout has
// is the boot of a fresh arena and then a full game inside it — the cut itself
// is not silent, because the clock-out row is ticked the moment it is written.
func TestOneSilenceOutlastsASegmentRestart(t *testing.T) {
	t.Parallel()
	for _, clock := range []int{120, ClockDefault, 600} {
		quiet := bootAllowance + GameBudget(clock)
		if got := StallBudget(clock); got <= quiet {
			t.Errorf("at a clock of %d one silence may last %v, and a bout "+
				"restarting an arena is quiet for %v", clock, got, quiet)
		}
	}
}

// The per-game ceiling is Forge's own clock plus a stated grace, and it is
// bound against the question it asks: how long may **one game** take. A caller
// who named no clock is using the default rather than asking for a game with no
// time in it.
func TestTheGameCeilingIsTheClockPlusAStatedGrace(t *testing.T) {
	t.Parallel()
	if got, want := GameBudget(300), 300*time.Second+bootAllowance; got != want {
		t.Errorf("a game at a 300s clock is given %v rather than %v", got, want)
	}
	if GameBudget(0) != GameBudget(ClockDefault) {
		t.Error("an unnamed clock did not fall back to the default")
	}
	if GameBudget(600) <= GameBudget(300) {
		t.Error("a longer clock did not buy a longer game")
	}
}
