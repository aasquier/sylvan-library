// Package tier3 is `mtglab/sim/tier3`: the Forge bridge (ADR 35).
//
// Forge is somebody else's rules engine — 470MB of card scripts and a JVM —
// and this package is everything that talks to it: the `.dck` exporter, the
// coverage pre-flight, the output parser, the subprocess runner, the wire the
// hosted worker speaks, that worker's client, and the match ledger (ADR 36).
//
// Three facts about `forge.jar sim` shape all of it, and each was established
// by *running* it rather than by reading a wiki page (docs/FORGE.md keeps the
// list):
//
//   - `-D` does not work for a single match. The single-match path resolves
//     deck names against Forge's user profile and ignores the flag, so decks
//     are written where Forge will look and `forge.profile.properties` is what
//     moves that.
//   - Forge must run with its own directory as the working directory, the way
//     `forge.sh` does, or it cannot find `res/`.
//   - **An unimplemented card does not stop a game.** It prints a warning and
//     plays on, reporting a winner and a turn count that look entirely
//     normal. That is the one failure this package exists to never serve, and
//     it is why coverage is checked twice — once from the card scripts before
//     a JVM starts, once from Forge's own mouth afterwards.
package tier3

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// Forge's `sim -q` output is line-oriented text, and not a stable API. The
// patterns below are the literal format strings in `forge.view.SimulateMatch`,
// read out of the shipped jar so that the parser matches what the code prints:
//
//	Game Result: Game %d ended in %d ms. %s has won!
//	Game Result: Game %d ended in a Draw! Took %d ms.
var (
	// playerRe matches the label `%s has won!` interpolates, which Forge
	// builds as "Ai(<seat>)-<deck name>". Seat is what identifies a deck: it
	// is the position in the `-d` argument list, and unlike the name it
	// cannot collide or contain an em dash.
	playerRe = regexp.MustCompile(`^Ai\((\d+)\)-(.*)$`)

	wonRe  = regexp.MustCompile(`^Game Result: Game (\d+) ended in (\d+) ms\. (.+) has won!$`)
	drawRe = regexp.MustCompile(`^Game Result: Game (\d+) ended in a Draw! Took (\d+) ms\.`)
	turnRe = regexp.MustCompile(`^Game Outcome: Turn (\d+)$`)
	// unsupportedRe is the line that must never appear. `[N.A.]` is the
	// edition Forge substitutes when it cannot place the card at all. A deck
	// containing three names Forge does not implement produced this and then
	// played the game anyway:
	//
	//	An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".
	//	Game Result: Game 1 ended in 7212 ms. Ai(2)-... has won!
	//
	// A 96-card deck, a clean winner, and a result line saying nothing is
	// wrong. `coverage.go` catches this before a JVM starts; this catches it
	// again if a name ever slips past the index. Two independent checks,
	// because the thing they prevent is silent.
	unsupportedRe = regexp.MustCompile(`^An unsupported card was requested: "(.+?)" from `)
	deckFailedRe  = regexp.MustCompile(`^Could not load deck - (.+), match cannot start$`)
)

// slowMatch is the warning Forge prints when a game hits `-c`. It arrives
// *before* the result line it belongs to, which is why the parser holds it.
const slowMatch = "Stopping slow match as draw"

// GameResult is one completed game.
//
// The fields are in the recorded wire order, because the wire
// codec below writes them by name and the ledger reads them by name.
type GameResult struct {
	Index        int
	Milliseconds int
	Draw         bool
	// Winner is the raw "Ai(2)-Atla Palani..." label; WinnerSeat is the seat
	// parsed out of it.
	Winner     *string
	WinnerSeat *int
	Turns      *int
	// TimedOut is true when the game hit `-c` and was called a draw rather
	// than finishing. Folded into `Draw` by Forge, separated here because a
	// clock-out is a measurement problem and a real draw is a game outcome.
	TimedOut bool
}

// SimOutput is everything the run said, parsed.
//
// `Unsupported` being non-empty invalidates every result in `Games`. Callers
// do not get to decide that; [RunGames] raises.
type SimOutput struct {
	Games            []GameResult
	Unsupported      []string
	DeckLoadFailures []string
}

// Trustworthy reports whether the run may be quoted at all.
func (o *SimOutput) Trustworthy() bool {
	return len(o.Unsupported) == 0 && len(o.DeckLoadFailures) == 0
}

// IsGameResult reports whether this line just finished a game — a single-line
// predicate with no state.
//
// The runner once counted ticks with this and tallied with the parser: two
// readers of the same stream, kept honest only by sharing regexes. Both ride
// one [StreamParser] now, so this survives as the cheap question a caller with
// no state wants answered.
func IsGameResult(line string) bool {
	s := textutil.Strip(line)
	return wonRe.MatchString(s) || drawRe.MatchString(s)
}

// StreamParser is the parser, fed one line at a time as Forge speaks.
//
// [StreamParser.Feed] returns a game at the moment a result line completes
// one, and accumulates everything into Output exactly as [Parse] would —
// because Parse *is* this machine fed a whole text. One parser for the tick
// and the tally is what makes them unable to drift: they are the same pass.
// The match theater rides it one step further — the row a tick carries is the
// row the final tally holds, by identity.
type StreamParser struct {
	Output SimOutput

	// "Game Outcome: Turn N" and the slow-match warning are printed before
	// the "Game Result" line they belong to, so both are held until the
	// result arrives.
	pendingTurn    *int
	pendingTimeout bool
}

// NewStreamParser returns a parser with nothing pending.
func NewStreamParser() *StreamParser { return &StreamParser{} }

// Feed reads one line and hands back the game it completed, if it did.
//
// Forge interleaves game logs, AI warnings and card-database complaints on the
// same stream, so this matches lines it recognises and ignores the rest rather
// than trying to model the whole log.
func (p *StreamParser) Feed(raw string) *GameResult {
	line := textutil.Strip(raw)

	if strings.Contains(line, slowMatch) {
		p.pendingTimeout = true
		return nil
	}

	if m := unsupportedRe.FindStringSubmatch(line); m != nil {
		// Forge repeats the complaint per copy; a name is a name.
		for _, seen := range p.Output.Unsupported {
			if seen == m[1] {
				return nil
			}
		}
		p.Output.Unsupported = append(p.Output.Unsupported, m[1])
		return nil
	}

	if m := deckFailedRe.FindStringSubmatch(line); m != nil {
		p.Output.DeckLoadFailures = append(p.Output.DeckLoadFailures, m[1])
		return nil
	}

	if m := turnRe.FindStringSubmatch(line); m != nil {
		turn, _ := strconv.Atoi(m[1])
		p.pendingTurn = &turn
		return nil
	}

	if m := wonRe.FindStringSubmatch(line); m != nil {
		label := m[3]
		game := GameResult{
			Index:        atoi(m[1]),
			Milliseconds: atoi(m[2]),
			Winner:       &label,
			Turns:        p.pendingTurn,
			TimedOut:     p.pendingTimeout,
		}
		if seat := playerRe.FindStringSubmatch(label); seat != nil {
			n := atoi(seat[1])
			game.WinnerSeat = &n
		}
		return p.finish(game)
	}

	if m := drawRe.FindStringSubmatch(line); m != nil {
		return p.finish(GameResult{
			Index:        atoi(m[1]),
			Milliseconds: atoi(m[2]),
			Draw:         true,
			Turns:        p.pendingTurn,
			TimedOut:     p.pendingTimeout,
		})
	}

	return nil
}

// finish appends a completed game, clears what was pending, and hands back a
// pointer *into* Output — the identity the match theater depends on, since the
// row a tick carries has to be the row the final tally holds.
func (p *StreamParser) finish(game GameResult) *GameResult {
	p.Output.Games = append(p.Output.Games, game)
	p.pendingTurn, p.pendingTimeout = nil, false
	return &p.Output.Games[len(p.Output.Games)-1]
}

// Parse reads a whole `sim` run: a [StreamParser] fed every line at once.
func Parse(log string) SimOutput {
	p := NewStreamParser()
	for _, raw := range textutil.SplitLines(log) {
		p.Feed(raw)
	}
	return p.Output
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
