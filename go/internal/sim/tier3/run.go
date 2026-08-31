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

// SubprocessBudget is how long the whole Forge subprocess may run: an
// allowance for JVM start plus one clock per game.
//
// **Exported because both sides of the wire have to agree about it.** Forge's
// own `-c` bounds each game, so this is the honest whole-match ceiling — and
// the app, waiting on the far end of a socket, must never hold a shorter one.
// It did: the client bounded a whole bout with a constant sized for a single
// game while the subprocess it was waiting on had this much rope, so the belt
// was tighter than the suspenders it was described as backing up. See
// [MatchBudget], which is this plus what the wire and the boot cost.
func SubprocessBudget(games, clock int) time.Duration {
	if games < 1 {
		games = 1
	}
	if clock < 1 {
		clock = ClockDefault
	}
	return time.Duration(60+games*clock) * time.Second
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
// `Timeout` bounds the whole subprocess and defaults to something derived
// rather than to nothing. Forge's own `-c` bounds each *game*, so
// `games * clock` plus an allowance for JVM start is the honest ceiling — and
// an unbounded wait is not hypothetical here, since one measured Trostani game
// ran 134 seconds.
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
	if opt.Timeout == 0 {
		opt.Timeout = SubprocessBudget(opt.Games, opt.Clock)
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
	var argv []string
	var read telling
	base := []string{java, fmt.Sprintf("-Xmx%dm", opt.Memory),
		"-Dio.netty.tryReflectionSetAccessible=true", "-Dfile.encoding=UTF-8"}
	if s.Scribed() {
		// Positional and dumb, because this is the only place that builds it:
		//   scribe.Main <clock> <games> <seed|-> <deck.dck> ...
		seed := "-"
		if opt.Seed != nil {
			seed = opt.Seed.String()
		}
		argv = append(base,
			"-cp", jar+string(os.PathListSeparator)+s.ScribeClasses,
			"scribe.Main", strconv.Itoa(opt.Clock), strconv.Itoa(opt.Games), seed)
		for _, name := range names {
			argv = append(argv, filepath.Join(deckDir, name))
		}
		read = NewScribeParser(opt.Narrate)
	} else {
		argv = append(base, "-jar", jar, "sim", "-d")
		argv = append(argv, names...)
		argv = append(argv, "-f", "Commander",
			"-n", strconv.Itoa(opt.Games), "-c", strconv.Itoa(opt.Clock))
		// `-q` is Forge's own flag for "the game result, not the entire game
		// log", read off its `sim -h`. Narrating is its absence rather than a
		// flag of its own.
		if !opt.Narrate {
			argv = append(argv, "-q")
		}
		if opt.Seed != nil {
			argv = append(argv, "-s", opt.Seed.String())
		}
		read = newProseTelling(opt.Narrate)
	}

	run, err := spawn(argv, home, opt, read)
	if err != nil {
		return nil, err
	}
	run.Seats = seats
	run.Coverage = reports
	run.ForgeVersion = s.ForgeVersion()

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
	if len(run.Output.Games) == 0 {
		tail := run.tail
		if len(tail) > 15 {
			tail = tail[len(tail)-15:]
		}
		return nil, &untrustworthy{msg: fmt.Sprintf(
			"Forge produced no game results in %.1fs. Last output:\n%s",
			run.WallSeconds, strings.Join(tail, "\n"))}
	}
	return &run.SimRun, nil
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
	expired, abandoned, reaped := false, false, false
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
	killed, quit := expired, abandoned
	mu.Unlock()
	// Asked before the timeout, because a match killed by both was killed by
	// whoever stopped listening first — and a bout nobody is waiting on is not
	// news about the clock.
	if quit {
		return nil, ErrAbandoned
	}
	if killed {
		return nil, &timedOut{msg: fmt.Sprintf(
			"Command '%s' timed out after %v seconds",
			strings.Join(argv, " "), opt.Timeout.Seconds())}
	}

	return &spawned{
		SimRun: SimRun{Argv: argv, Output: read.Output(), WallSeconds: elapsed,
			Events: collected},
		tail: text,
	}, nil
}
