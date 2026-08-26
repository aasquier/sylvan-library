package tier3

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The scribe's stream, read (ADR 42).
//
// `scribe/` is a small GPL-3.0 Java program that plays a match through Forge's
// own code with one extra listener on the game's event bus, and prints what
// happens as newline-delimited JSON. This is the far side of that pipe: it
// produces exactly what the prose parser produces — a [GameResult] per game
// and an [EventLog] of beats — plus the thing the prose could never produce,
// a [BoardReel].
//
// **Why a second reader at all**, restated because it is the whole argument:
// `forge.game.GameLogEntryType` is a closed enum of nineteen values with no
// TOKEN and no COUNTER in it, so a Food deck can play fifteen turns without
// the log mentioning one Food. The events were always on the bus; nothing was
// listening. `docs/adr/0042-a-scribe-rides-forges-event-bus.md` has the
// measurement.
//
// **The beats this produces are strictly better than the regexes', not
// merely different**, and that is worth knowing before choosing between them:
//
//   - A blocked attacker and an unblocked one are separate typed events, where
//     the log said "didn't block" and dropped its `Combat: ` prefix on every
//     line of a group after the first — which cost the old parser two
//     unblocked attackers out of three.
//   - `cast` is `SpellAbilityView.isSpell()`, where the log offered `cast`,
//     `activated` and `triggered` as three shapes of one sentence.
//   - An outcome is Forge's own sentence handed over whole, where the regex
//     had to be non-greedy because "has lost because an opponent has won by
//     spell" holds both verbs and the last one is the wrong one.
//   - A deck name is a field rather than unbounded text between two anchors.
//
// What is lost is [EventResolve]: Forge raises `GameEventSpellResolved`, but
// the scribe does not listen for it, because the log's own resolve line was
// unreliable enough that only its typed form was ever read and the question a
// watcher actually has — what arrived on the battlefield — is now answered by
// the board itself.

// scribeLine is one line of the scribe's output.
//
// One flat struct for every kind, matching the flat JSON the scribe writes
// (`Json.java` is forty lines and writes objects of strings, ints and bools —
// deliberately, so that no Forge release gets to choose when our JSON library
// breaks). A field per kind would be a dozen structs and a discriminator to
// pick between them, for a shape that is already discriminated by `t`.
//
// **An absent boolean is false and that is the encoding**, not an accident:
// the scribe's writer drops a false rather than spelling it, so `tapped`
// missing means untapped and `token` missing means a real card.
type scribeLine struct {
	Kind string `json:"t"`
	Game int    `json:"game"`

	Turn int    `json:"turn"`
	Seat int    `json:"seat"`
	Who  string `json:"who"`
	Life *int   `json:"life"`

	ID        int    `json:"id"`
	Card      string `json:"card"`
	Token     bool   `json:"token"`
	Power     int    `json:"power"`
	Toughness int    `json:"toughness"`
	Types     string `json:"types"`

	Zone   string `json:"zone"`
	Mode   string `json:"mode"`
	Tapped bool   `json:"tapped"`

	Counter string `json:"counter"`
	Now     int    `json:"now"`

	TargetID    int    `json:"target_id"`
	Target      string `json:"target"`
	Against     string `json:"against"`
	AgainstSeat int    `json:"against_seat"`
	Amount      int    `json:"amount"`
	Combat      bool   `json:"combat"`

	Milliseconds int    `json:"ms"`
	Winner       string `json:"winner"`
	Draw         bool   `json:"draw"`
	TimedOut     bool   `json:"timed_out"`
	Said         string `json:"said"`
	Detail       string `json:"detail"`
}

// ScribeParser reads the scribe's stream, one line at a time.
//
// Per *run* rather than per game, exactly as [EventParser] is: a game closes
// on its `result` line and the next one starts collecting.
//
// **Non-JSON lines are not ignored.** Forge prints its card-database
// complaints on the same streams whatever is driving it, and
// `An unsupported card was requested` is the one line that invalidates every
// result in the run — the failure this package exists to never serve. Those
// lines go to a [StreamParser] whose *tally* is used and whose games are not,
// because in scribe mode Forge prints no game-result lines at all.
type ScribeParser struct {
	// prose is the tally of Forge's own complaints. Its games are never read.
	prose *StreamParser
	// watching is whether anybody is looking. Off, the beats and the board are
	// not built at all — a nightly sweep wants rows and nothing else, and the
	// board is the expensive half of this file. The turn number is still read
	// either way, because a row carries one.
	//
	// The same trade `Narrate` makes on the prose path, and the same reason:
	// free in time, expensive in volume, so it is asked for rather than
	// assumed.
	watching bool

	game      int
	events    []GameEvent
	truncated bool
	board     *board
	// seats is the roster, for turning an outcome sentence back into a seat.
	seats map[int]string
	// turn is the highest turn number Forge announced, which is what
	// `GameResult.Turns` means everywhere else in this package.
	turn int
	// outcomeTurn is the turn the outcome event named, which is Forge's own
	// answer and beats counting.
	outcomeTurn int
}

// NewScribeParser returns a parser at the start of the first game.
//
// `watching` asks for the beats and the board. A caller that only wants the
// rows passes false and pays for none of it.
func NewScribeParser(watching bool) *ScribeParser {
	return &ScribeParser{prose: NewStreamParser(), watching: watching, game: 1,
		board: newBoard(), seats: map[int]string{}}
}

// Output is the run's tally — the complaints, and the games.
func (p *ScribeParser) Output() SimOutput { return p.prose.Output }

// Feed reads one line and hands back a finished game's beats and row when that
// line ended one.
//
// Both together, and the beats first in every consumer, because that is the
// order one pass over the subprocess produces them in: a listener that stashes
// the beats and publishes them with the row never has a row waiting on beats.
func (p *ScribeParser) Feed(raw string) (*EventLog, *GameResult) {
	line := strings.TrimSpace(raw)
	if !strings.HasPrefix(line, "{") {
		// Forge's own noise. The tally reads it; the games it might find are
		// discarded, because `sim`'s result lines are not printed on this path
		// and anything shaped like one would be a coincidence.
		p.prose.Feed(raw)
		return nil, nil
	}
	var l scribeLine
	if err := json.Unmarshal([]byte(line), &l); err != nil {
		// A line this reader cannot parse is a line dropped, never a run
		// abandoned: the scribe is a picture and the row is the record.
		return nil, nil
	}

	switch l.Kind {
	case "game":
		p.startGame(l.Game)
		return nil, nil
	case "seat":
		p.seats[l.Seat] = l.Who
		life := 0
		if l.Life != nil {
			life = *l.Life
		}
		p.board.sit(l.Seat, l.Who, life)
		return nil, nil
	case "turn":
		// Read on both paths: `GameResult.Turns` is a row's field and a row is
		// what a run is for, so the count is never skipped for not watching.
		if l.Turn > p.turn {
			p.turn = l.Turn
		}
	case "result":
		return p.finishGame(l)
	}

	p.fold(l)
	return nil, nil
}

// startGame resets everything that is per game.
//
// Forge's instance ids are per game, so a board carried across would draw game
// one's battlefield under game two's first land.
func (p *ScribeParser) startGame(number int) {
	if number > 0 {
		p.game = number
	}
	p.events, p.truncated = nil, false
	p.board = newBoard()
	p.seats = map[int]string{}
	p.turn, p.outcomeTurn = 0, 0
}

// fold applies one line to the board and raises the beat it is worth, if any.
//
//nolint:gocyclo // one branch per event kind; splitting it only moves the list
func (p *ScribeParser) fold(l scribeLine) {
	if !p.watching {
		// Nobody is looking. The outcome still lands, because it carries the
		// turn number a row reports and it is one line a game.
		if l.Kind == "outcome" && l.Turn > p.outcomeTurn {
			p.outcomeTurn = l.Turn
		}
		return
	}
	switch l.Kind {
	case "zone":
		p.board.name(l.ID, l.Card, l.Types, l.Token, l.Seat)
		// **The phantoms.** Forge keeps bookkeeping cards in the command
		// zone: a real id, a real name, and an empty type line. They are
		// invisible in any real game and drawing one puts a blank card beside
		// somebody's commander.
		if isForgeEffect(l.Zone, l.Card, l.Types) {
			return
		}
		was := p.board.zone[l.ID]
		if was == ZoneGone {
			// Forge announces the leaving before the arriving, so the zone a
			// card is *in* is already nothing by the time it lands somewhere.
			// The zone it came *from* is what this asks about.
			was = p.board.left[l.ID]
		}
		p.board.moved(l.ID, l.Zone, l.Mode, l.Seat)
		p.board.stats(l.ID, l.Power, l.Toughness, l.Types)
		// A **creature or planeswalker** leaving the battlefield for a
		// graveyard is the one zone change worth a sentence — it is `dies`
		// everywhere in this package, and it is the same reading the log's
		// `Zone Change: ... was put into Graveyard from Battlefield` gave,
		// from typed fields instead.
		//
		// **Only those two types, because that is what the word means.** Rule
		// 700.4 defines "dies" as put into a graveyard from the battlefield
		// and gives the term to creatures and planeswalkers; a sacrificed
		// fetchland does not die, and an artifact cracked for mana does not
		// either. This fired for every permanent, so a Commander game narrated
		// "Wooded Foothills dies" several times a turn — teaching a newcomer
		// the wrong word for a real Magic term, which commandment 2 will not
		// have.
		//
		// It hid for as long as it did because the skull it draws used to land
		// on the *graveyard pile*, where a fetchland's death was one anonymous
		// flicker among many. Holding the dying card on the sand put the skull
		// on the card, and a skull on a land is impossible to miss.
		//
		// The type line is the board's rather than the line's: `stats` and
		// `zone` both carry types, but a `zone` line for a card Forge has
		// already described need not repeat them, and the answer has to be the
		// same either way. An animated Dryad Arbor is a Creature Land and does
		// die; the check is on what it *is*, not which row it was drawn in.
		//
		// Everything else still moves — the board shows any permanent leaving
		// play — it simply does so without a sentence, and therefore on the
		// next beat, which is what already happens to every change this
		// account does not narrate.
		if l.Mode == "in" && l.Zone == "Graveyard" &&
			(was == ZoneBattlefield || was == ZoneLand) {
			types := p.board.types[l.ID]
			if strings.Contains(types, "Creature") ||
				strings.Contains(types, "Planeswalker") {
				p.raise(GameEvent{Kind: EventDies, Seat: l.Seat, Card: l.Card})
			}
			return
		}
		// A permanent arriving. See [EventEnters] for why lands are excluded
		// and why this is not called "resolves".
		if p.board.zone[l.ID] == ZoneBattlefield && was != ZoneBattlefield {
			p.raise(GameEvent{Kind: EventEnters, Seat: l.Seat, Card: l.Card})
		}
	case "attach", "detach":
		p.board.name(l.ID, l.Card, l.Types, l.Token, 0)
		// `TargetID` is present on an attach and absent on a detach, so the
		// zero it leaves behind *is* the answer: attached to nothing. The
		// scribe deliberately does not report the old host on a detach —
		// this side is already holding it.
		moved := p.board.attach(l.ID, l.TargetID)
		// **Narrated only when the board moved**, which is the same guard one
		// level up: Forge re-announces an attachment that has not changed, and
		// a room that said "the sword goes on the bear" twice over a still
		// board would read as a stutter rather than as a game.
		//
		// A curse names a player rather than a card and finds no host, so it
		// never moves the board — and it is still worth saying out loud, which
		// is why the target check is an `||` rather than a second `&&`.
		if l.Kind == "attach" && (moved || l.Against != "") &&
			(l.Target != "" || l.Against != "") {
			target := l.Target
			if target == "" {
				target = l.Against
			}
			p.raise(GameEvent{Kind: EventAttach, Seat: l.Seat,
				Card: l.Card, Target: target})
		}
	case "tapped":
		p.board.name(l.ID, l.Card, l.Types, l.Token, 0)
		p.board.tap(l.ID, l.Tapped)
	case "stats":
		p.board.name(l.ID, l.Card, l.Types, l.Token, 0)
		p.board.stats(l.ID, l.Power, l.Toughness, l.Types)
	case "counters":
		p.board.name(l.ID, l.Card, l.Types, l.Token, 0)
		p.board.counter(l.ID, l.Counter, l.Now)
	case "life":
		if l.Life == nil {
			return
		}
		p.board.lives(l.Seat, *l.Life)
		after := *l.Life
		p.raise(GameEvent{Kind: EventLife, Seat: l.Seat, Life: &after})
	case "turn":
		p.board.began(l.Turn, l.Seat)
		if l.Life != nil {
			p.board.lives(l.Seat, *l.Life)
		}
		p.raise(GameEvent{Kind: EventTurn, Seat: l.Seat})
	case "mulligan":
		p.raise(GameEvent{Kind: EventMulligan, Seat: l.Seat})
	case "land":
		p.raise(GameEvent{Kind: EventLand, Seat: l.Seat, Card: l.Card})
	case "cast":
		p.raise(GameEvent{Kind: EventCast, Seat: l.Seat, Card: l.Card})
	case "attack":
		p.raise(GameEvent{Kind: EventAttack, Seat: l.Seat, Card: l.Card,
			TargetSeat: l.AgainstSeat})
	case "block":
		p.raise(GameEvent{Kind: EventBlock, Seat: l.Seat, Card: l.Card,
			Target: l.Target})
	case "unblocked":
		// The seat is who declined to block, so the card is the *attacker* —
		// the same reading the prose parser's own comment records.
		p.raise(GameEvent{Kind: EventUnblocked, Seat: l.Seat, Card: l.Card})
	case "damage":
		p.raise(GameEvent{Kind: EventDamage, Card: l.Card, Amount: l.Amount,
			Target: l.Target, TargetSeat: l.AgainstSeat})
	case "outcome":
		p.outcome(l)
	}
}

// outcome raises the beat for one of Forge's outcome sentences.
//
// Forge hands over the whole sentence — "Gyome, Master Chef — Food has won
// because all opponents have lost" — and the shape this package has always
// carried is the seat, the verb as a flag, and the reason as its tail. So the
// name is matched against the roster and cut off the front. The name is
// Forge's own and the roster's is the same string, so this is a comparison
// rather than a parse.
//
// **A deck played against itself leaves the seat off**, rather than crediting
// the first seat with both sentences. That is the same wart `shapeForge`
// records one layer up: unreachable by convention, and a guess would be worse
// than a gap.
func (p *ScribeParser) outcome(l scribeLine) {
	if l.Turn > p.outcomeTurn {
		p.outcomeTurn = l.Turn
	}
	said := strings.TrimSpace(l.Said)
	if said == "" {
		return
	}
	// **The name is cut off whether or not the seat is knowable.** Two seats
	// sharing a name leave the *seat* ambiguous — a guess would be worse than
	// a gap — but the sentence is the same sentence either way, and leaving
	// the player's name on the front of it makes the reason unreadable
	// ("Gyome — Food · Gyome — Food has won because…") and hides the verb the
	// win flag is read from.
	seat, matched, several := 0, "", false
	for at, name := range p.seats {
		if name == "" || !strings.HasPrefix(said, name) {
			continue
		}
		if len(name) > len(matched) {
			matched = name
		}
		if seat != 0 {
			several = true
			continue
		}
		seat = at
	}
	if several {
		seat = 0
	}
	rest := strings.TrimSpace(strings.TrimPrefix(said, matched))
	won := strings.HasPrefix(rest, "has won")
	for _, verb := range []string{"has won ", "has lost "} {
		if strings.HasPrefix(rest, verb) {
			rest = strings.TrimPrefix(rest, verb)
			break
		}
	}
	p.raise(GameEvent{Kind: EventOutcome, Seat: seat,
		Note: strings.TrimSpace(rest), Amount: boolToWin(winVerb(won))})
}

func winVerb(won bool) string {
	if won {
		return "won"
	}
	return "lost"
}

// raise records a beat and closes the board step that led to it.
//
// The two happen together and that is the pacing contract: [BoardStep] and
// [GameEvent] are parallel arrays, so a room draining the beats at reading
// speed moves the picture exactly when the sentence is spoken.
func (p *ScribeParser) raise(event GameEvent) {
	if len(p.events) >= EventCap {
		p.truncated = true
		return
	}
	event.Turn = p.turn
	p.events = append(p.events, event)
	p.board.beat()
}

// finishGame closes a game on its `result` line.
func (p *ScribeParser) finishGame(l scribeLine) (*EventLog, *GameResult) {
	events := p.events
	if events == nil {
		events = []GameEvent{}
	}
	log := &EventLog{Game: p.game, Events: events, Truncated: p.truncated,
		Board: p.board.reel()}

	// **Forge's own halving, copied exactly.** `GameEventGameOutcome`'s
	// `lastTurnNumber()` is the *player-turn* count, and Forge's own
	// `GameLogFormatter` renders the outcome line as
	// `Math.ceil(lastTurnNumber() / 2.0)` — read out of the bytecode, not
	// guessed — so the log's `Game Outcome: Turn 9` is nine *rounds* against
	// seventeen player-turns. `GameResult.Turns` has therefore always been
	// rounds, and the prose parser reads the halved number off that line.
	//
	// So this halves too. Matching Forge is the requirement rather than being
	// right in general: the match ledger records whichever path played, and a
	// row that means rounds on one path and player-turns on the other poisons
	// every comparison across a deploy. The `2` is Forge's, hardcoded in
	// Forge, and wrong for a four-player pod in exactly the way Forge is
	// wrong — which is the behaviour to copy, not the bug to fix here.
	turns := (p.outcomeTurn + 1) / 2
	if p.outcomeTurn == 0 {
		turns = (p.turn + 1) / 2
	}
	game := GameResult{Index: l.Game, Milliseconds: l.Milliseconds,
		Draw: l.Draw, TimedOut: l.TimedOut}
	if turns > 0 {
		game.Turns = &turns
	}
	if l.Seat > 0 {
		seat := l.Seat
		game.WinnerSeat = &seat
		// **Forge's own label, rebuilt.** The prose parser stores the raw
		// `Ai(2)-<deck name>` off the result line, and the match ledger and
		// the recorded corpus carry that string. Rebuilding it from the two
		// halves the scribe reports keeps a row identical whichever path
		// played it, which is ADR 42's third decision applied to the bytes
		// rather than only to the outcome.
		label := fmt.Sprintf("Ai(%d)-%s", l.Seat, l.Winner)
		game.Winner = &label
	}
	p.prose.Output.Games = append(p.prose.Output.Games, game)

	p.game++
	p.events, p.truncated = nil, false
	p.board = newBoard()
	p.turn, p.outcomeTurn = 0, 0
	return log, &game
}

// isForgeEffect is whether a card arriving in a command zone is Forge's own
// bookkeeping rather than a card anybody put there.
//
// **Three facts from `javap -c` against Forge 2.0.14, and the rule is their
// intersection.** Guessing at any one of them would have been wrong: the
// first version of this dropped one effect by name and a real match then
// produced two more.
//
//  1. **An effect has no type line.** `SpellAbilityEffect.createEffect` builds
//     a bare `Card(id, game)` — no paper card — sets a name on it and marks it
//     `GamePieceType.EFFECT`; nothing ever gives it types. `Commander Effect`
//     and a companion's effect go through `DetachedCardEffect` instead, and
//     that one *is* handed a paper card — but `Card`'s constructor makes a
//     blank `CardState` and reads nothing off the paper card but foil and
//     marked colours, so it has no types either. A real card always has some.
//  2. **An effect is in the command zone.** `createEffect`'s caller ends in
//     `GameAction.moveToCommand`, and `Player.assignCompanion` and
//     `createCommanderEffect` both put theirs in the command zone by hand.
//     Nothing else is dropped anywhere: an untyped card the *battlefield*
//     somehow reported is news, and news is not something to swallow quietly.
//  3. **An emblem is not one of these**, even though it is built by the same
//     function and is equally untyped. `createEffect` branches on
//     `name.startsWith("Emblem")` and marks it, so the name is Forge's own
//     discriminator rather than a pattern read off one match. An emblem is a
//     real object in a real command zone and a player wants to see it.
//
// Three of these were live in one recorded match: `Commander Effect` for each
// player, `Kaheera, the Orphanguard's Companion Effect`, and
// `Rogue's Passage (123)'s Effect` — the last from an activated ability, which
// is why naming them one at a time was never going to hold.
//
// The exact discriminator is `GamePieceType.EFFECT`, which the scribe does not
// carry. It could: the cost is a field on every card line and a worker image.
// It is not obviously worth it while the shape above is this clean, and the
// fallback would have to exist anyway for a worker deployed before the field.
func isForgeEffect(zone, name, types string) bool {
	if zone != "Command" || types != "" {
		return false
	}
	return !strings.HasPrefix(name, "Emblem")
}
