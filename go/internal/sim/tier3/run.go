package tier3

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The Forge runner: find Forge, hand it decks, run games, time them.

// JavaMinimum is the floor Forge needs. It runs on 21 here; the floor is what
// matters.
const JavaMinimum = 17

// ------------------------------------------------------------------ the JVM

var javaVersionRe = regexp.MustCompile(`version "(\d+)`)

// javaMajor probes one candidate. A binary that will not answer is not a
// candidate, which is the point: this machine's `/usr/bin/java` is 10.0.1 and
// fails Forge in a way that reads like a Forge bug rather than a Java one.
func javaMajor(binary string) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The binary is `MTGLAB_JAVA`, the bundled JDK, or `java` on PATH — three
	// paths the operator chose, on a machine the operator runs. gosec's taint
	// analysis cannot tell an environment variable from a request parameter;
	// this one is the same kind of trust as `MTGLAB_FORGE_HOME` pointing at
	// half a gigabyte of somebody's card scripts.
	cmd := exec.CommandContext(ctx, binary, "-version") //nolint:gosec // an operator-chosen JVM, never a request value
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil && errOut.Len() == 0 && out.Len() == 0 {
		return 0, false
	}
	// `java -version` writes to stderr. Has done for decades.
	text := errOut.String()
	if text == "" {
		text = out.String()
	}
	m := javaVersionRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// JavaBinary is a JVM new enough to run Forge.
//
// `MTGLAB_JAVA` wins, then the JDK unpacked beside the distribution, then
// whatever is on PATH. The PATH entry is checked rather than trusted.
func (s Settings) JavaBinary() (string, error) {
	var candidates []string
	if s.Java != "" {
		candidates = append(candidates, s.Java)
	}
	candidates = append(candidates,
		filepath.Join(s.BundledJDK, "Contents", "Home", "bin", "java"),
		filepath.Join(s.BundledJDK, "bin", "java"))
	if found, err := exec.LookPath("java"); err == nil {
		candidates = append(candidates, found)
	}

	var tried []string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil { //nolint:gosec // the same operator-chosen path the probe above runs
			continue
		}
		major, ok := javaMajor(candidate)
		if ok && major >= JavaMinimum {
			return candidate, nil
		}
		// A candidate renders with its major version, and a failed probe
		// renders as the word `None` -- the served message's long-standing
		// spelling of "could not tell", kept verbatim.
		shown := "None"
		if ok {
			shown = strconv.Itoa(major)
		}
		tried = append(tried, fmt.Sprintf("%s (Java %s)", candidate, shown))
	}

	checked := strings.Join(tried, ", ")
	if checked == "" {
		checked = "nothing"
	}
	return "", NotInstalled("no Java %d+ found -- set MTGLAB_JAVA to one. "+
		"Checked: %s", JavaMinimum, checked)
}

// DesktopJar is the distribution's simulator jar.
func (s Settings) DesktopJar() (string, error) {
	home := s.Home
	// A directory this process cannot even look inside is a directory with no
	// Forge in it. Deployed, the home directory is `/root` while the app runs
	// as `mtglab`, so the glob's stat raises a permission error — and the gate
	// at `/api/forge` answered 500 instead of `available: false` until that
	// was caught on the live instance — the only place it appears, since a
	// laptop's Forge home is readable by the process that reads it.
	jars, err := filepath.Glob(filepath.Join(home, "forge-gui-desktop-*-jar-with-dependencies.jar"))
	if err != nil {
		return "", NotInstalled("no Forge distribution readable at %s (%v) -- "+
			"set MTGLAB_FORGE_HOME to an unpacked Forge distribution", home, err)
	}
	if len(jars) == 0 {
		// Go's `filepath.Glob` reports no error for an unreadable
		// directory, so the readability question is
		// asked explicitly rather than inferred from an empty match.
		//
		// **A missing directory is not an unreadable one**, and the first
		// version of this conflated them -- caught against the recorded
		// messages, which say "no Forge desktop jar in X" for an absent
		// directory and reserve "no Forge
		// distribution readable at X (...)" for a real refusal, where the
		// conflated version served the second for both. Only a permission
		// error is a refusal -- the deployed shape: the home
		// directory is `/root` while the app runs as `mtglab`. So absence
		// falls through to the jar message and only a real refusal takes the
		// other branch.
		if _, statErr := os.ReadDir(home); statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return "", NotInstalled("no Forge distribution readable at %s (%v) -- "+
				"set MTGLAB_FORGE_HOME to an unpacked Forge distribution", home, statErr)
		}
		return "", NotInstalled("no Forge desktop jar in %s -- set "+
			"MTGLAB_FORGE_HOME to an unpacked Forge distribution", home)
	}
	sort.Strings(jars)
	return jars[len(jars)-1], nil
}

var jarVersionRe = regexp.MustCompile(`^forge-gui-desktop-(.+)-jar-with-dependencies\.jar$`)

// ForgeVersion is which Forge is installed, read off the jar's own name.
//
// The match ledger records it (ADR 36): Forge's AI is the instrument every
// recorded game was measured with, and an upgrade changes the instrument —
// ratings computed across an unversioned upgrade would silently mix two
// judges. Empty when the name does not parse, which the ledger stores as "not
// reported" rather than guessing.
func (s Settings) ForgeVersion() string {
	jar, err := s.DesktopJar()
	if err != nil {
		return ""
	}
	m := jarVersionRe.FindStringSubmatch(filepath.Base(jar))
	if m == nil {
		return ""
	}
	return m[1]
}

// -------------------------------------------------------------- the profile

// EnsureProfile writes `forge.profile.properties` and returns the commander
// deck directory.
//
// This is the one thing that reaches into the Forge installation, because
// Forge reads that file from its own program directory and nowhere else. It is
// rewritten only when the contents would change, so a run does not needlessly
// touch a shared install.
func (s Settings) EnsureProfile() (string, error) {
	home := s.Home
	if _, err := os.Stat(home); err != nil {
		return "", NotInstalled("no Forge distribution at %s", home)
	}

	profile := s.Profile
	wanted := fmt.Sprintf("userDir=%s\ncacheDir=%s\ncardPicsDir=%s\n",
		profile, filepath.Join(profile, "cache"),
		filepath.Join(profile, "cache", "pics"))
	marker := filepath.Join(home, "forge.profile.properties")
	if current, err := os.ReadFile(marker); err != nil || string(current) != wanted {
		if err := os.WriteFile(marker, []byte(wanted), 0o644); err != nil { //nolint:gosec // Forge reads this from its own program directory
			return "", fmt.Errorf("writing %s: %w", marker, err)
		}
	}

	deckDir := filepath.Join(profile, "decks", "commander")
	if err := os.MkdirAll(deckDir, 0o755); err != nil { //nolint:gosec // scratch decks; Forge reads the directory as the same user
		return "", err
	}
	return deckDir, nil
}

// -------------------------------------------------------------- the running

// SimRun is one `forge sim` invocation, and what it cost.
type SimRun struct {
	Argv   []string
	Output SimOutput
	// WallSeconds is the whole subprocess: JVM start, card database load, and
	// every game. The startup share is fixed, so it amortises across `-n`.
	WallSeconds float64
	// Seats maps a seat number (1-based, the order decks were passed) to a
	// deck slug.
	Seats    map[int]string
	Coverage []CoverageReport
	// ForgeVersion is which Forge played, from the jar's name — or "" when the
	// run was rebuilt from a wire payload that predates the field.
	ForgeVersion string
	// Events is one [EventLog] per game, and is empty unless the run asked to
	// be narrated. It is **not** on [WireRun]: the recorded worker-wire corpus
	// pins that struct's bytes, and a finished run is the wrong place for a
	// thousand beats anyway — they cross on the stream, one line per game, and
	// a run rebuilt from the wire carries none.
	Events []EventLog
}

// Games is the run's completed games.
func (r *SimRun) Games() []GameResult { return r.Output.Games }

// StartupSeconds is wall time not spent inside a game — JVM boot and the card
// database.
//
// The number that decided whether a hosted Forge is one long-lived process or
// a subprocess per request.
func (r *SimRun) StartupSeconds() float64 {
	played := 0.0
	for _, g := range r.Games() {
		played += float64(g.Milliseconds)
	}
	return max(0.0, r.WallSeconds-played/1000)
}

// WinnerSlug is which deck won a game, or "" for nobody.
func (r *SimRun) WinnerSlug(g GameResult) string {
	if g.WinnerSeat == nil {
		return ""
	}
	return r.Seats[*g.WinnerSeat]
}

// RunOptions are RunGames's levers.
type RunOptions struct {
	Games int
	Clock int
	// Seed is a `*big.Int` because a request's seed has no ceiling and this
	// one is echoed back to whoever asked: it goes onto Forge's command
	// line as text, so narrowing it here would hand Forge a
	// different number than the request named.
	Seed   *big.Int
	Memory int
	// Timeout bounds the whole subprocess. Zero means the derived
	// default.
	Timeout time.Duration
	// GameCeiling bounds **one game**, and is the only bound in this package
	// at that scale. Zero means [GameBudget] of [RunOptions.Clock]; a caller
	// that wants no per-game cut names a duration longer than the bout.
	//
	// [Settings.RunGames] fills it in and [spawn] honours it; a test driving
	// `spawn` directly and leaving it zero is asking about something else and
	// gets no pace timer at all.
	//
	// **Rearmed by a finished game and by nothing else.** Not by a beat, which
	// is the distinction the live fault turned on: the game that ran fifteen
	// minutes past its clock narrated the entire way, so every bound watching
	// for a *silence* was correct to stay quiet. What was missing was a bound
	// watching for a *game*.
	GameCeiling time.Duration
	// OnGame is called with the count of games finished so far (1, 2, ...)
	// and the game just parsed, as each result line arrives — what lets a job
	// tick per game and the match theater show the row behind the tick. Ticks
	// are best-effort progress, never results.
	OnGame func(finished int, game GameResult)
	// Narrate drops Forge's `-q`, so it prints the whole game log rather than
	// one line per game, and [OnEvents] hears the beats.
	//
	// **Off for anything measured, on for anything watched.** The flag itself
	// is free — the same seed narrated and quiet played in 8055ms and 8205ms,
	// inside the noise of one sample — but it turns a nine-turn game from one
	// line into 477, and a nightly sweep has nobody watching it. `events.go`
	// carries the measurement.
	Narrate bool
	// OnEvents is called once per game with that game's beats, after the
	// result line closes it.
	//
	// Per game rather than per beat, deliberately. Forge plays a game in
	// about eight seconds and a person cannot watch eight seconds of
	// Commander; the beats are handed over whole and whoever is showing them
	// paces them. That also keeps the stream to one extra line per game
	// instead of a hundred.
	OnEvents func(log EventLog)
	// Abort ends the match early when it closes: the JVM is killed and the
	// read loop ends the way it does on the whole-subprocess timeout.
	//
	// **This exists because nobody could stop a match once it started**, and
	// that is how a worker became a zombie on the deployed instance
	// (2026-08-30). The app's side of a bout was cancelled mid-match; the shim
	// went on playing for whoever was no longer listening, counted itself busy
	// the whole time, and so its idle watchdog never stopped the machine —
	// every bout after it queued behind a match with no audience. The only
	// bound was this subprocess's own ceiling, which on a twenty-game bout is
	// over an hour.
	//
	// A channel rather than a `context.Context` because [RunGames] takes none
	// and giving it one would imply a cancellation the rest of the call does
	// not honour; this says exactly what it does. Nil never aborts, which is
	// every caller that measures rather than watches.
	Abort <-chan struct{}
}

// bootAllowance is everything a subprocess spends that is not a game: the
// JVM's own start and the card database it loads before the first hand is
// dealt, measured at fifteen to twenty-five seconds.
//
// It is the `60` [SubprocessBudget] has always opened with, named rather than
// written twice so that [GameBudget] can spend the same allowance for the same
// reason — the first game of a match is the one that waits behind the boot.
const bootAllowance = 60 * time.Second

// GameBudget is how long **one game** may take before the run is cut and that
// game recorded as a clock-out: Forge's own clock, plus [bootAllowance].
//
// **This exists because Forge's clock cannot end a game**, which is not what
// the flag's name suggests and is worth stating in full, because every bound
// in this package was written believing otherwise.
//
// Both programs that play a match — the scribe (`scribe/src/scribe/Main.java`)
// and Forge's own `sim` — spend the clock the same way, through
// `forge.view.TimeLimitedCodeBlock.runWithTimeout`. Read out of the shipped jar
// with `javap -c`, that method is: a single-thread executor, `submit`, then
// `future.get(clock, SECONDS)`; and when the wait expires, `future.cancel(true)`
// and the `TimeoutException` is rethrown to the caller. `cancel(true)` sets the
// game thread's **interrupt flag**, and an interrupt is a request that only a
// thread which looks at it can honour. Nothing looks: across the 1,139 classes
// of `forge.game` and `forge.ai` in Forge 2.0.14 there is not one reference to
// `interrupted`, `isInterrupted` or `InterruptedException`. So the clock bounds
// how long Forge **waits** for a game, never how long the game **runs** — the
// wait ends, the game plays on, and because the executor's thread is not a
// daemon it also keeps the JVM alive after `main` has returned.
//
// That is the hole this closes, and it was live: on 2026-08-31 a game of a
// ten-game bout on the deployed arena ran fifteen minutes past a three-hundred
// second clock. Nothing cut it. The app's silence budget was right not to — the
// game narrated the whole way, and [StallBudget] bounds silence — and every
// other bound in the chain kills the **bout**, so the only ceiling left was one
// that would have thrown away nine finished games to reach the tenth.
//
// The grace is [bootAllowance] because that is what the first game of a segment
// genuinely waits behind. Forge's own cut fires at exactly `clock` wall-seconds
// and prints its row immediately after, so a game still running at `clock` plus
// a whole JVM start is a game Forge has already given up on and failed to stop.
func GameBudget(clock int) time.Duration {
	if clock < 1 {
		clock = ClockDefault
	}
	return time.Duration(clock)*time.Second + bootAllowance
}

// SubprocessBudget is how long **one** Forge subprocess may run: an allowance
// for JVM start plus one [GameBudget] per game it was asked for.
//
// **Exported because both sides of the wire have to agree about it.** The app,
// waiting on the far end of a socket, must never hold a shorter one. It did:
// the client bounded a whole bout with a constant sized for a single game while
// the subprocess it was waiting on had this much rope, so the belt was tighter
// than the suspenders it was described as backing up. See [MatchBudget], which
// is this plus a restart's worth of boots and what the wire costs.
//
// **A bout may spend several of these.** A game cut at [GameBudget] takes its
// JVM with it — that is the only mechanism that ends a Forge game, per the
// argument there — so [Settings.RunGames] plays what is left in a fresh
// subprocess, and this bounds each one rather than the bout.
func SubprocessBudget(games, clock int) time.Duration {
	if games < 1 {
		games = 1
	}
	return bootAllowance + time.Duration(games)*GameBudget(clock)
}

// ClockDefault is Forge's `-c` when a caller names none: seconds before a game
// is called a draw.
const ClockDefault = 300

// MemoryDefault is `run_games`'s `memory_mb`.
const MemoryDefault = 4096

// ErrTimedOut is what a match that outran its whole-subprocess budget returns
// — `subprocess.TimeoutExpired`, which the shim renders by class name.
var ErrTimedOut = errors.New("TimeoutExpired")

// ErrAbandoned is a match stopped through [RunOptions.Abort]: whoever asked
// for it is not there any more.
//
// It is not a failure and it is never rendered to anybody — by the time it
// exists, the person it would have been rendered to has gone. It exists so the
// shim can tell "the client left" from "Forge fell over" in its own log, and
// so a stopped match is never mistaken for a result.
var ErrAbandoned = errors.New("the match was abandoned")

type timedOut struct{ msg string }

func (e *timedOut) Error() string        { return e.msg }
func (e *timedOut) Is(target error) bool { return target == ErrTimedOut }

// RunGames plays `Games` Commander games between `decks` and returns the
// results.
//
// `Clock` is Forge's `-c`: seconds before a game is called a draw. The default
// is 300 rather than Forge's 120 because CLAUDE.md says so and because Tivit
// games really do run long — a clock-out is recorded as TimedOut, never
// quietly folded into the draw rate.
//
// Every deck is pre-flighted first, and the output is checked again
// afterwards. Both have to pass or this returns an error.
//
// `Timeout` bounds one subprocess and defaults to something derived rather than
// to nothing; `GameCeiling` bounds one game and does the same. Both are needed,
// because Forge's own clock bounds neither — it ends the *wait* for a game and
// leaves the game running (see [GameBudget]).
//
// **A bout is one subprocess until a game outruns its ceiling, and then it is
// two.** Killing the JVM is the only way to end a Forge game, and a JVM holds
// every game the bout had left, so the cut is followed by a fresh subprocess
// playing what remains. The bout that comes back is whole: `Games` rows, one of
// them a clock-out. Nine finished games are not thrown away to report the tenth
// — which is what every ceiling in this package did before, and what a bout on
// the deployed arena came within minutes of on 2026-08-31.
func (s Settings) RunGames(decks []*deck.Deck, opt RunOptions) (*SimRun, error) {
	if len(decks) < 2 {
		return nil, errors.New("a game needs at least two decks")
	}
	if opt.Games == 0 {
		opt.Games = 1
	}
	if opt.Clock == 0 {
		opt.Clock = ClockDefault
	}
	if opt.Memory == 0 {
		opt.Memory = MemoryDefault
	}
	if opt.GameCeiling == 0 {
		opt.GameCeiling = GameBudget(opt.Clock)
	}

	reports, err := s.CheckCoverage(decks)
	if err != nil {
		return nil, err
	}
	deckDir, err := s.EnsureProfile()
	if err != nil {
		return nil, err
	}
	home := s.Home

	var names []string
	seats := map[int]string{}
	for i, d := range decks {
		path, err := WriteDck(d, deckDir, reports[i].Resolved)
		if err != nil {
			return nil, err
		}
		names = append(names, filepath.Base(path))
		seats[i+1] = d.Slug
	}

	java, err := s.JavaBinary()
	if err != nil {
		return nil, err
	}
	jar, err := s.DesktopJar()
	if err != nil {
		return nil, err
	}

	// **Two programs, one contract.** The scribe plays the match through
	// Forge's own code with a listener on the game's event bus and prints
	// typed JSON; `sim` plays it and prints prose. Everything below this line
	// — the trustworthiness checks, the callbacks, the run that comes back —
	// is identical, which is ADR 42's fifth decision: the board is a renderer,
	// not a second pipeline.
	//
	// The choice is the presence of the classes, never a flag. A worker image
	// built before the scribe existed, or one whose build stage failed, plays
	// the match through `sim` and narrates from the log — a room with no board
	// in it rather than a match that will not start.
	base := []string{java, fmt.Sprintf("-Xmx%dm", opt.Memory),
		"-Dio.netty.tryReflectionSetAccessible=true", "-Dfile.encoding=UTF-8"}
	build := func(games int, seed *big.Int) ([]string, telling) {
		// **A copy, because this is called once per segment now.** `base` is a
		// slice literal, so its capacity is its length and appending onto it
		// allocates today — but the day somebody gives it spare capacity, two
		// segments would build their command lines into one array and the
		// second would rewrite the first. It costs four strings, and the bug it
		// forecloses appears only on a bout that clocked out.
		argv := append([]string(nil), base...)
		if s.Scribed() {
			// Positional and dumb, because this is the only place that builds
			// it:  scribe.Main <clock> <games> <seed|-> <deck.dck> ...
			text := "-"
			if seed != nil {
				text = seed.String()
			}
			argv = append(argv,
				"-cp", jar+string(os.PathListSeparator)+s.ScribeClasses,
				"scribe.Main", strconv.Itoa(opt.Clock), strconv.Itoa(games), text)
			for _, name := range names {
				argv = append(argv, filepath.Join(deckDir, name))
			}
			return argv, NewScribeParser(opt.Narrate)
		}
		argv = append(argv, "-jar", jar, "sim", "-d")
		argv = append(argv, names...)
		argv = append(argv, "-f", "Commander",
			"-n", strconv.Itoa(games), "-c", strconv.Itoa(opt.Clock))
		// `-q` is Forge's own flag for "the game result, not the entire game
		// log", read off its `sim -h`. Narrating is its absence rather than a
		// flag of its own.
		if !opt.Narrate {
			argv = append(argv, "-q")
		}
		if seed != nil {
			argv = append(argv, "-s", seed.String())
		}
		return argv, newProseTelling(opt.Narrate)
	}

	// **A segment is the rest of the bout**, and the loop ends because every
	// pass makes progress: a segment either finishes games or is cut on one,
	// and a cut game counts as played. Everything above this line — the
	// pre-flight, the `.dck` files, the JVM and the jar — was resolved once and
	// every segment reuses it as it is.
	run := &SimRun{Seats: seats, Coverage: reports,
		ForgeVersion: s.ForgeVersion()}
	for played := 0; played < opt.Games; {
		played, err = s.playSegment(run, home, opt, played, build)
		if err != nil {
			return nil, err
		}
	}

	if !run.Output.Trustworthy() {
		var lines []string
		for _, n := range run.Output.Unsupported {
			lines = append(lines, "  dropped card: "+n)
		}
		for _, n := range run.Output.DeckLoadFailures {
			lines = append(lines, "  deck failed to load: "+n)
		}
		return nil, &untrustworthy{msg: "Forge reported problems that " +
			"invalidate the run:\n" + strings.Join(lines, "\n")}
	}
	return run, nil
}

// resumeSeed is the seed the segment after a clock-out plays under.
//
// **A resumed segment must not replay the games already played.** Forge seeds
// one global generator before the first game is created — `MyRandom.setRandom`,
// which the scribe marks PARITY 1 — so a fresh JVM handed the same number deals
// the same opening hands in the same order. A resume on the original seed would
// replay the games that already finished and walk straight back into the game
// that wedged, which is a loop rather than a salvage.
//
// Advancing by the games already played keeps both properties that matter: the
// whole bout is still a function of the one number the caller named, and no two
// segments of it are the same match. `played` is zero for the first segment, so
// **a bout with no clock-out in it builds byte-identical argv to before this
// existed** — which is what the parity gate rests on.
//
// Nil stays nil: a caller who named no seed is letting Forge pick, and a new
// JVM picks again on its own.
func resumeSeed(seed *big.Int, played int) *big.Int {
	if seed == nil || played == 0 {
		return seed
	}
	return new(big.Int).Add(seed, big.NewInt(int64(played)))
}

// playSegment runs one subprocess and folds what it produced into `run`,
// reporting how many games of the bout are now played.
//
// The three ways it can end, and only the first is an error:
//
//   - **Nothing came out and nothing cut it.** Forge started and produced no
//     game at all, which is a broken distribution or a deck it could not load —
//     the diagnosis this package has always raised, with the tail of the output
//     so somebody can read what happened.
//   - **A game outran [RunOptions.GameCeiling].** The games before it are real
//     and kept; the one that was in progress is written here as a clock-out,
//     because this is the half that knows what number it would have been. The
//     caller loops and plays the rest in a new subprocess.
//   - **The segment played everything it was asked for.** `played` reaches the
//     bout's count and the loop ends.
//
// `opt` is the **caller's** options throughout — the bout's count, the bout's
// seed, the bout's callbacks. The segment's own are derived here and nowhere
// else, so there is exactly one place that knows the difference between a
// game's number in this subprocess and its number in the bout.
func (s Settings) playSegment(run *SimRun, home string, opt RunOptions,
	played int, build func(int, *big.Int) ([]string, telling)) (int, error) {
	remaining := opt.Games - played
	seed := resumeSeed(opt.Seed, played)
	argv, read := build(remaining, seed)
	if run.Argv == nil {
		// The first segment's, which is the command that describes the bout as
		// it was asked for. A later one differs only in its count and its seed,
		// and naming it here would make a diagnostic about the whole match read
		// as though it were about the salvage.
		run.Argv = argv
	}

	segment := opt
	segment.Games, segment.Seed = remaining, seed
	if opt.Timeout == 0 {
		segment.Timeout = SubprocessBudget(remaining, opt.Clock)
	}
	// The live callbacks count in the bout's numbers rather than the segment's:
	// a fresh JVM starts its games at one again, and a room that has ticked to
	// nine must not watch the count fall back to one because the arena changed
	// underneath it.
	if opt.OnGame != nil {
		segment.OnGame = func(finished int, g GameResult) {
			g.Index = played + finished
			opt.OnGame(played+finished, g)
		}
	}
	if opt.OnEvents != nil {
		segment.OnEvents = func(log EventLog) {
			log.Game += played
			opt.OnEvents(log)
		}
	}

	seg, err := spawn(argv, home, segment, read)
	if err != nil {
		return played, err
	}
	if len(seg.Output.Games) == 0 && !seg.clockedOut {
		tail := seg.tail
		if len(tail) > 15 {
			tail = tail[len(tail)-15:]
		}
		return played, &untrustworthy{msg: fmt.Sprintf(
			"Forge produced no game results in %.1fs. Last output:\n%s",
			seg.WallSeconds, strings.Join(tail, "\n"))}
	}

	// Renumbered into the bout, for the reason the callbacks are: a second JVM
	// numbers its own games from one, and two rows called "game 1" would be two
	// rows the room cannot tell apart.
	for i := range seg.Output.Games {
		seg.Output.Games[i].Index = played + i + 1
	}
	for i := range seg.Events {
		seg.Events[i].Game += played
	}
	run.Output.Games = append(run.Output.Games, seg.Output.Games...)
	run.Output.Unsupported = append(run.Output.Unsupported,
		seg.Output.Unsupported...)
	run.Output.DeckLoadFailures = append(run.Output.DeckLoadFailures,
		seg.Output.DeckLoadFailures...)
	run.Events = append(run.Events, seg.Events...)
	run.WallSeconds += seg.WallSeconds
	played += len(seg.Output.Games)

	if !seg.clockedOut {
		return played, nil
	}
	played++
	// **The casualty, written as a row rather than as a failure.** `TimedOut`
	// is the field this repo has always kept apart from `Draw` — a clock-out is
	// the measurement giving up, not a game outcome — so nothing downstream
	// needs teaching: the API masks the winner and the draw off it, the tally
	// counts it in its own column, and the room says it was stopped by the
	// clock. The milliseconds are the ceiling, which is the honest floor on how
	// long the game had been running when it was cut.
	cut := GameResult{Index: played, TimedOut: true,
		Milliseconds: int(segment.GameCeiling / time.Millisecond)}
	run.Output.Games = append(run.Output.Games, cut)
	if opt.OnGame != nil {
		// **The tick is load-bearing beyond the room.** The app presumes a
		// worker dead after [StallBudget] of silence, and a segment restart is
		// the longest quiet a healthy bout ever has — the cut, and then a whole
		// JVM boot before another game can end. Saying so here breaks that
		// silence in two, and each half fits inside the budget on its own.
		//
		// The caller's callback rather than the segment's wrapper: this row is
		// already counted in the bout, and handing it to something whose job is
		// to add `played` to it would count that twice.
		opt.OnGame(played, cut)
	}
	return played, nil
}

// telling is whatever is reading the subprocess's output.
//
// Two of them, and they are interchangeable by construction rather than by
// convention: [ScribeParser] over the scribe's typed JSON, and [proseTelling]
// over Forge's own game log. [spawn] holds one and knows which it is only by
// what it was handed — so the tick, the tally, the trustworthiness check and
// the beats are one code path whichever program played the match.
type telling interface {
	// Feed takes one line and reports the beats and the row it completed.
	// Beats first, because that is the order one pass produces them in and
	// every consumer downstream relies on it.
	Feed(line string) (*EventLog, *GameResult)
	// Output is the tally, valid once the stream has ended.
	Output() SimOutput
}

// proseTelling reads Forge's own narration: the parser this package has always
// had, behind the interface the scribe also satisfies.
type proseTelling struct {
	games  *StreamParser
	events *EventParser
}

func newProseTelling(narrate bool) *proseTelling {
	t := &proseTelling{games: NewStreamParser()}
	if narrate {
		t.events = NewEventParser()
	}
	return t
}

func (t *proseTelling) Feed(line string) (*EventLog, *GameResult) {
	// Both readers, one pass. A game's beats and the row that closes it can
	// never come from different passes and disagree — the property
	// [StreamParser] was folded into one machine for in the first place.
	var log *EventLog
	if t.events != nil {
		log = t.events.Feed(line)
	}
	return log, t.games.Feed(line)
}

func (t *proseTelling) Output() SimOutput { return t.games.Output }

// spawn is the subprocess half, split out so [RunGames] reads as the contract
// it enforces rather than as process management.
type spawned struct {
	SimRun
	tail []string
	// clockedOut is a run cut by [RunOptions.GameCeiling]: the games above are
	// real and finished, and the one that was in progress is not here at all —
	// its row is written by [Settings.RunGames], which is the half that knows
	// what number it would have been.
	//
	// Not an error, deliberately. A whole-subprocess timeout has nothing to
	// hand back and a clock-out has everything except one game, so folding them
	// together would throw away the bout to report the casualty.
	clockedOut bool
}

// endGroup kills a match and everything the match started.
//
// The group is the point — see the `Setpgid` in [spawn], which is what makes
// the child a group leader and so makes its pid double as the group's. The
// fallback is not decoration: if `Setpgid` never took, the child is still in
// this process's own group, no group carries its pid, the signal comes back
// as "no such process", and the direct child is then all there is to kill.
// The order matters in that case, because `-pid` naming *our* group would be
// the app killing itself.
func endGroup(p *os.Process) {
	if p == nil {
		return
	}
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err == nil {
		return
	}
	_ = p.Kill()
}

func spawn(argv []string, home string, opt RunOptions, read telling) (*spawned, error) {
	// Forge writes the card-database complaints to stderr and the results to
	// stdout, but not reliably — the unsupported-card warning arrives on both.
	// stderr is folded into stdout so one stream can be read live (the tick)
	// and parsed whole (the tally); the union is what makes the second
	// coverage check dependable.
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = home
	// **The match gets a process group of its own, because killing it has to
	// kill all of it.** On the deployed worker the JVM is not this child: the
	// image points `MTGLAB_JAVA` at a `/bin/sh` wrapper that runs `xvfb-run`,
	// and `xvfb-run` is another `/bin/sh` script which *forks* the command it
	// was handed — it has to outlive it to take the display down again. So the
	// thing playing the games is a grandchild, [os.Process.Kill] kills a shell,
	// and the games go on. The pipe below is what turns that into an hour:
	// a surviving grandchild still holds its write end, so the read loop never
	// reaches EOF, `spawn` never returns, and the machine counts itself busy
	// until somebody stops it by hand.
	//
	// A negative pid signals the group, which is the whole tree.
	//
	// **The cost is stated rather than hidden**, in `ui.go`'s phrase. A group
	// of its own is also a group the terminal will not reach, so a Ctrl-C on
	// `mtglab sim forge` no longer reaches the JVM the way it used to — the two
	// killers below both die with the Go process, and the match is left
	// orphaned until its own clock. That is a laptop's papercut and it buys the
	// deployed fault: the worker has no terminal, Fly takes the whole machine
	// when it stops one, and `/match` is the one caller that wires
	// [RunOptions.Abort] at all. Covering the signal too means a handler armed
	// process-wide from a library, which is the bug `ui.go` refuses two hundred
	// lines up; it wants to be a deliberate change to that command, not a side
	// effect of this one.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	started := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting Forge: %w", err)
	}
	var collected []EventLog

	// A blocking read honours no deadline of its own, so the deadline is a
	// timer that kills the JVM — EOF then ends the read loop, and the flag is
	// what tells a killed run from a finished one.
	//
	// Both ways of ending a match early go through `stop`, which raises the
	// flag its caller came for and then takes the group down. `reaped` is why
	// it is one function rather than two: once [exec.Cmd.Wait] has returned,
	// the pid belongs to whoever the kernel hands it to next, and a group kill
	// names a *group* by that number — so a timer that fires a hair late must
	// not fire a SIGKILL into somebody else's work.
	var mu sync.Mutex
	expired, abandoned, clocked, reaped := false, false, false, false
	stop := func(mark *bool) {
		mu.Lock()
		defer mu.Unlock()
		if reaped {
			return
		}
		*mark = true
		endGroup(cmd.Process)
	}

	timer := time.AfterFunc(opt.Timeout, func() { stop(&expired) })

	// **The per-game ceiling, and killing the JVM is the whole mechanism.**
	// Forge offers nothing else: its own clock ends a *wait* rather than a game
	// (the argument is on [GameBudget]), so a game that has outrun it is a game
	// only the kernel can stop. That makes the cut coarse — it takes the games
	// this subprocess had left with it — which is why [Settings.RunGames] plays
	// them again in a new one rather than calling the bout over.
	//
	// Rearmed by a finished game and by nothing else. A beat is not progress
	// through the bout; it is the sound the wedge makes.
	var pace *time.Timer
	if opt.GameCeiling > 0 {
		pace = time.AfterFunc(opt.GameCeiling, func() { stop(&clocked) })
		defer pace.Stop()
	}

	// [RunOptions.Abort] kills the same way, and the watcher is wound up when
	// the read loop ends so a finished match leaves no goroutine behind.
	// `abandoned` is kept apart from `expired` because they are different news:
	// a match that ran out of clock is a result nobody got, and a match nobody
	// was left to watch is not a failure at all.
	done := make(chan struct{})
	defer close(done)
	if opt.Abort != nil {
		go func() {
			select {
			case <-opt.Abort:
				stop(&abandoned)
			case <-done:
			}
		}()
	}

	var text []string
	finished := 0
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			text = append(text, strings.TrimRight(line, "\n"))
			log, game := read.Feed(line)
			if log != nil {
				collected = append(collected, *log)
				if opt.OnEvents != nil {
					opt.OnEvents(*log)
				}
			}
			if game != nil {
				finished++
				if opt.OnGame != nil {
					opt.OnGame(finished, *game)
				}
				// After the tick rather than before it: a slow listener is not
				// the next game's fault, and `OnGame` writes to a socket that
				// can push back.
				if pace != nil {
					pace.Reset(opt.GameCeiling)
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				// A read that failed for a reason other than the end of the
				// stream still leaves whatever was parsed; the run's own
				// checks below decide whether that is enough.
				break
			}
			break
		}
	}
	_ = cmd.Wait()
	timer.Stop()
	elapsed := time.Since(started).Seconds()

	mu.Lock()
	reaped = true
	killed, quit, cut := expired, abandoned, clocked
	mu.Unlock()
	// Asked before the timeout, because a match killed by both was killed by
	// whoever stopped listening first — and a bout nobody is waiting on is not
	// news about the clock.
	if quit {
		return nil, ErrAbandoned
	}
	// The whole-subprocess ceiling is asked before the per-game one, and only
	// a race can put both here: [SubprocessBudget] is a [GameBudget] per game
	// plus a boot, so the pace timer is the tighter bound on every game and
	// fires first by construction. If the outer one did fire, the run has blown
	// the budget for the games it was asked for, and handing back a partial as
	// though a single game were the casualty would be a nicer story than the
	// truth.
	if killed {
		return nil, &timedOut{msg: fmt.Sprintf(
			"Command '%s' timed out after %v seconds",
			strings.Join(argv, " "), opt.Timeout.Seconds())}
	}

	return &spawned{
		SimRun: SimRun{Argv: argv, Output: read.Output(), WallSeconds: elapsed,
			Events: collected},
		tail:       text,
		clockedOut: cut,
	}, nil
}
