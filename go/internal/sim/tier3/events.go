package tier3

import (
	"regexp"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// The game log, typed: what Forge says when it is not being quiet.
//
// `sim -q` prints one line per finished game, which is what [StreamParser]
// reads and what every recorded match in the ledger is built from. Drop the
// flag and Forge narrates the whole game instead — its own help calls `-q`
// the flag that prints "just the game result, not the entire game log" — and
// this file is the second reader of that stream: the beats a person would
// want to watch, in the order they happened.
//
// **Measured, not assumed** (2026-08-24), in the house tradition of
// docs/FORGE.md. Dropping `-q` on the same seed cost nothing: 8055ms of game
// against 8205ms quiet, inside the noise of one sample, with the whole
// subprocess at 17s either way because JVM boot and the card database
// dominate both. What the flag costs is volume — 477 lines for a nine-turn
// game, 727 for a longer one — which is why it is asked for per run and never
// by the nightly sweep.
//
// Four things about the format, each found by reading real output rather than
// Forge's source:
//
//   - **A player is `Ai(<seat>)-<deck name>`, and the deck name is
//     unbounded.** It is whatever `[metadata] name=` said, which is the deck's
//     own title — commas, em dashes and all. So nothing here tries to find
//     where a label ends: the seat is read off the front, and the interesting
//     half is anchored to the *end* of the line. A greedy `.*` between them
//     takes the last ` played `, ` cast `, ` assigned ` in the line, which is
//     right because a card name contains none of them.
//   - **Forge drops the `Combat: ` prefix on grouped lines.** The first
//     "didn't block" in a combat carries it and the rest do not, so the prefix
//     is optional in that pattern. A parser that required it saw one
//     unblocked attacker out of three.
//   - **`Add To Stack` has three verbs** — `cast`, `triggered` and
//     `activated` — and only the first is a spell somebody paid for. Triggers
//     are the bulk of the stack traffic in any real game and are not beats a
//     person is watching for.
//   - **`Resolve Stack` is not reliably a card.** It is a card with a type
//     line (`Fauna Shaman - Creature 2 / 2`), or a bare name (`Survival of the
//     Fittest`), or the full rules text of a trigger, or empty with a
//     targeting note. **Only the typed form is read**, because a bare name and
//     a sentence of rules text are the same shape and nothing in the line
//     separates them. The cost is that a resolving non-creature raises no beat
//     — which is survivable, since `cast` already announced it and the
//     theater's question is what arrived on the battlefield.
//
// Like [StreamParser], this matches the lines it recognises and ignores
// everything else rather than trying to model the whole log. Forge's output is
// not an API, and a parser that insisted on understanding every line would
// break on the first release that added one.

// EventKind is what kind of beat a [GameEvent] is.
//
// The set is deliberately smaller than the log: these are the moments a person
// watching a match would name, not every line Forge prints. Phase steps are
// the clearest omission — 217 of the 477 lines in a nine-turn game are
// `Phase:`, and "Untap step" is not a beat.
type EventKind string

// The beats. String values, because they cross the wire and are read by a
// browser; an integer enum would be smaller and unreadable in a payload
// somebody is debugging.
const (
	EventTurn      EventKind = "turn"
	EventMulligan  EventKind = "mulligan"
	EventLand      EventKind = "land"
	EventCast      EventKind = "cast"
	EventResolve   EventKind = "resolve"
	EventAttack    EventKind = "attack"
	EventBlock     EventKind = "block"
	EventUnblocked EventKind = "unblocked"
	EventDamage    EventKind = "damage"
	EventLife      EventKind = "life"
	EventDies      EventKind = "dies"
	EventOutcome   EventKind = "outcome"
)

// GameEvent is one beat of a game.
//
// One struct rather than a type per kind, because every one of these crosses
// the same wire and lands in the same list; the zero fields cost a `,omitempty`
// each and save a discriminated union in TypeScript.
//
// `Seat` is 1-based and matches [SimRun.Seats], so a browser resolves an event
// to a deck the same way it resolves a winner. Zero means the line named no
// player — which is most of them, since Forge usually names the card instead.
type GameEvent struct {
	Kind EventKind `json:"kind"`
	// Turn is the turn in progress when this happened, carried onto every
	// event by the parser so a consumer never has to track it. Zero before
	// Forge announces the first turn, which mulligans are.
	Turn int `json:"turn,omitempty"`
	// Seat is who acted, or who the outcome is about.
	Seat int `json:"seat,omitempty"`
	// Card is what the beat is about, in Forge's spelling — which is a *face*
	// name, never Scryfall's combined `A // B` (docs/FORGE.md's fourth fact).
	// A consumer resolving this to art must resolve a face.
	Card string `json:"card,omitempty"`
	// Target is the card on the other end: blocked, or damaged.
	Target string `json:"target,omitempty"`
	// TargetSeat is the player on the other end: attacked, or damaged.
	TargetSeat int `json:"target_seat,omitempty"`
	// Amount is damage dealt, or cards kept on a mulligan.
	Amount int `json:"amount,omitempty"`
	// Life is the life total *after* a life change. A pointer because zero
	// life is a real and rather important total.
	Life *int `json:"life,omitempty"`
	// Note is an outcome's reason, in Forge's own words ("life total reached
	// 0"). Never shown as an explanation of anything else.
	Note string `json:"note,omitempty"`
}

// EventCap is how many events one game may carry.
//
// A bound rather than a stream that can grow without one: a 300-second clock
// against two durdling decks is minutes of a JVM narrating, and the far side
// holds the whole list in memory before anybody reads it. Ten thousand is
// roughly twenty times the nine-turn game measured above and has never been
// approached in practice — [EventLog.Truncated] says plainly when it is, and
// no consumer is ever handed a silently short list.
const EventCap = 10000

// EventLog is one game's beats.
type EventLog struct {
	Game   int         `json:"game"`
	Events []GameEvent `json:"events"`
	// Truncated is set when the game outran [EventCap]. The events kept are
	// the first ones, because a game's opening is what makes the rest legible.
	Truncated bool `json:"truncated,omitempty"`
}

// The patterns. Each is anchored at both ends and reads the interesting half
// off the end of the line, for the reason the package comment gives: a deck
// name is unbounded text and there is no reliable way to find where one stops.
var (
	// `Turn: Turn 6 (Ai(2)-Goreclaw, Terror of Qal Sisma — Mono-Green Stompy)`
	eventTurnRe = regexp.MustCompile(`^Turn: Turn (\d+) \(Ai\((\d+)\)-.*\)$`)
	// `Mulligan: Ai(1)-<label> has kept a hand of 7 cards`
	eventMulliganRe = regexp.MustCompile(`^Mulligan: Ai\((\d+)\)-.* has kept a hand of (\d+) cards?$`)
	// `Land: Ai(1)-<label> played Hushwood Verge (99)`
	eventLandRe = regexp.MustCompile(`^Land: Ai\((\d+)\)-.* played (.+) \(\d+\)$`)
	// `Add To Stack: Ai(2)-<label> cast Fauna Shaman`, with an optional
	// ` targeting [...]` tail. `triggered` and `activated` share the shape and
	// are deliberately not matched.
	eventCastRe = regexp.MustCompile(`^Add To Stack: Ai\((\d+)\)-.* cast (.+?)(?: targeting \[.*\])?$`)
	// `Resolve Stack: Fauna Shaman - Creature 2 / 2`. The type line is not
	// captured — a card's characteristics come from the card pool, which is
	// authoritative and already loaded.
	eventResolveRe = regexp.MustCompile(`^Resolve Stack: (.+?) - (?:Creature|Artifact|Enchantment|Planeswalker|Land|Battle)\b`)
	// `Combat: Ai(2)-<label> assigned Fauna Shaman (126) to attack Ai(1)-<label>.`
	eventAttackRe = regexp.MustCompile(`^(?:Combat: )?Ai\((\d+)\)-.* assigned (.+) \(\d+\) to attack Ai\((\d+)\)-.*\.$`)
	// `... to block Pride Sovereign (82).`
	eventBlockRe = regexp.MustCompile(`^(?:Combat: )?Ai\((\d+)\)-.* assigned (.+) \(\d+\) to block (.+) \(\d+\)\.$`)
	// `Ai(1)-<label> didn't block Fauna Shaman (126).` — the `Combat: ` prefix
	// is present on the first of a group and absent on the rest.
	eventUnblockedRe = regexp.MustCompile(`^(?:Combat: )?Ai\((\d+)\)-.* didn't block (.+) \(\d+\)\.$`)
	// `Damage: Fauna Shaman (126) deals 2 combat damage to Ai(1)-<label>.`
	eventDamagePlayerRe = regexp.MustCompile(`^Damage: (.+) \(\d+\) deals (\d+) (?:combat )?damage to Ai\((\d+)\)-.*\.$`)
	// `Damage: Pride Sovereign (82) deals 14 damage to Goreclaw, ... (203).`
	eventDamageCardRe = regexp.MustCompile(`^Damage: (.+) \(\d+\) deals (\d+) (?:combat )?damage to (.+) \(\d+\)\.$`)
	// `Life: Life: Ai(1)-<label> 40 > 38` — the doubled prefix is Forge's.
	eventLifeRe = regexp.MustCompile(`^Life: (?:Life: )?Ai\((\d+)\)-.* (-?\d+) > (-?\d+)$`)
	// `Zone Change: Sakura-Tribe Elder (188) was put into Graveyard from Battlefield.`
	eventDiesRe = regexp.MustCompile(`^Zone Change: (.+) \(\d+\) was put into Graveyard from Battlefield\.$`)
	// `Game Outcome: Ai(2)-<label> has lost because life total reached 0`
	eventOutcomeRe = regexp.MustCompile(`^Game Outcome: Ai\((\d+)\)-.* has (won|lost) because (.+)$`)
)

// EventParser turns Forge's narration into beats, one line at a time.
//
// It rides the same single pass over the subprocess's output that
// [StreamParser] does — one read of the stream, two readers of it — so the
// beats of a game and the row that finishes it cannot come from different
// passes and disagree.
//
// A parser is per *run*, not per game: [EventParser.Feed] returns a finished
// [EventLog] at the moment a game's last line lands, and starts collecting the
// next one.
type EventParser struct {
	game      int
	turn      int
	events    []GameEvent
	truncated bool
}

// NewEventParser returns a parser at the start of the first game.
func NewEventParser() *EventParser { return &EventParser{game: 1} }

// Feed reads one line, and hands back a game's events when that line ended it.
//
// The result line is [IsGameResult]'s, which is the same predicate the tally
// uses — so a game ends in both readers on the same line, by construction
// rather than by two patterns kept in step by hand.
func (p *EventParser) Feed(raw string) *EventLog {
	line := textutil.Strip(raw)

	if IsGameResult(line) {
		log := &EventLog{Game: p.game, Events: p.events, Truncated: p.truncated}
		if log.Events == nil {
			log.Events = []GameEvent{}
		}
		p.game++
		p.turn, p.events, p.truncated = 0, nil, false
		return log
	}

	event, ok := p.match(line)
	if !ok {
		return nil
	}
	// The cap drops beats rather than growing without bound; the flag is what
	// keeps that from being silent.
	if len(p.events) >= EventCap {
		p.truncated = true
		return nil
	}
	event.Turn = p.turn
	p.events = append(p.events, event)
	return nil
}

// match reads one line into a beat. The order is by how often a line appears,
// so the common cases are cheap.
//
//nolint:gocyclo // one branch per pattern; splitting it would only move the list
func (p *EventParser) match(line string) (GameEvent, bool) {
	if m := eventTurnRe.FindStringSubmatch(line); m != nil {
		p.turn = atoi(m[1])
		return GameEvent{Kind: EventTurn, Seat: atoi(m[2])}, true
	}
	if m := eventLandRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventLand, Seat: atoi(m[1]), Card: m[2]}, true
	}
	if m := eventCastRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventCast, Seat: atoi(m[1]), Card: m[2]}, true
	}
	if m := eventResolveRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventResolve, Card: m[1]}, true
	}
	if m := eventAttackRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventAttack, Seat: atoi(m[1]), Card: m[2],
			TargetSeat: atoi(m[3])}, true
	}
	if m := eventBlockRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventBlock, Seat: atoi(m[1]), Card: m[2],
			Target: m[3]}, true
	}
	if m := eventUnblockedRe.FindStringSubmatch(line); m != nil {
		// Seat here is who declined to block, so the *attacker* is the card.
		return GameEvent{Kind: EventUnblocked, Seat: atoi(m[1]), Card: m[2]}, true
	}
	if m := eventDamagePlayerRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventDamage, Card: m[1], Amount: atoi(m[2]),
			TargetSeat: atoi(m[3])}, true
	}
	// After the player form: a card named `Ai(1)-...` does not exist, but the
	// player pattern is the stricter of the two and gets first refusal.
	if m := eventDamageCardRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventDamage, Card: m[1], Amount: atoi(m[2]),
			Target: m[3]}, true
	}
	if m := eventLifeRe.FindStringSubmatch(line); m != nil {
		after := atoi(m[3])
		return GameEvent{Kind: EventLife, Seat: atoi(m[1]), Life: &after,
			Amount: atoi(m[3]) - atoi(m[2])}, true
	}
	if m := eventDiesRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventDies, Card: m[1]}, true
	}
	if m := eventMulliganRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventMulligan, Seat: atoi(m[1]),
			Amount: atoi(m[2])}, true
	}
	if m := eventOutcomeRe.FindStringSubmatch(line); m != nil {
		return GameEvent{Kind: EventOutcome, Seat: atoi(m[1]),
			Note: strings.TrimSpace(m[3]), Amount: boolToWin(m[2])}, true
	}
	return GameEvent{}, false
}

// boolToWin is 1 for a win and 0 for a loss, which is what `Amount` carries on
// an outcome. A separate `Won bool` would be a thirteenth field set on one
// kind of event out of twelve.
func boolToWin(verb string) int {
	if verb == "won" {
		return 1
	}
	return 0
}

// ParseEvents reads a whole narrated run: an [EventParser] fed every line.
func ParseEvents(log string) []EventLog {
	p := NewEventParser()
	var out []EventLog
	for _, raw := range textutil.SplitLines(log) {
		if done := p.Feed(raw); done != nil {
			out = append(out, *done)
		}
	}
	return out
}
