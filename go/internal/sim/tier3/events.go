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
	EventTurn     EventKind = "turn"
	EventMulligan EventKind = "mulligan"
	EventLand     EventKind = "land"
	EventCast     EventKind = "cast"
	EventResolve  EventKind = "resolve"
	// EventEnters is a permanent arriving on the battlefield, and it is raised
	// only by the scribe (`scribe.go`) — this file cannot see one, because
	// Forge's log's own resolve line is a card with a type line, or a bare
	// name, or the rules text of a trigger, and nothing separates the last two.
	//
	// **Magic's own words, and they are the right ones twice over.** A token
	// does not "resolve", so [EventResolve] would be wrong for the Food and the
	// Eggs this whole road was built for; and "enters the battlefield" is the
	// phrase printed on the cards themselves, which is the phrase a person
	// learning the game is already reading.
	//
	// A card arriving in the **land** row raises nothing: a land drop is
	// already [EventLand], and a land fetched onto the battlefield is a land
	// that will still be there at the next beat. Creatures are the ones that
	// arrive and leave inside a single beat — `Sakura-Tribe Elder` sacrifices
	// itself the moment it lands, and without this beat the board never showed
	// it at all.
	EventEnters EventKind = "enters"
	// EventAttach is an Aura, Equipment or Fortification finding a host — and
	// **only ever finding one**. Coming off raises nothing: an unequip is a
	// bookkeeping moment rather than a moment anybody at the table narrates,
	// and every way of losing a host that a person *would* narrate already has
	// a beat of its own — the host died, the Aura was destroyed, somebody
	// bounced it. A second sentence for the same event is a room saying
	// everything twice.
	//
	// Raised only by the scribe. Forge's log has no attachment line at all,
	// which is the same hole ADR 42 was written about: `GameLogEntryType` has
	// no category for it, so a Voltron deck could win a game through the log
	// without a word about the sword that did it.
	EventAttach    EventKind = "attach"
	EventAttack    EventKind = "attack"
	EventBlock     EventKind = "block"
	EventUnblocked EventKind = "unblocked"
	EventDamage    EventKind = "damage"
	EventLife      EventKind = "life"
	EventDies      EventKind = "dies"
	// EventExiled is a permanent leaving the battlefield for exile.
	//
	// **[EventDies]'s twin, and it exists for the same hole.** A great deal of
	// what removal does in Commander exiles rather than destroys — Path to
	// Exile, Swords to Plowshares, Anguished Unmaking — and every one of those
	// was a card that vanished off the sand with nothing said about it (Aaron,
	// 2026-08-27: *"we don't show exiled cards there like we do for destroyed
	// creatures going to the graveyard"*). The board moved the card, correctly
	// and silently, and the moment a player would have reacted to went past
	// unmarked.
	//
	// **Every permanent, not only creatures**, which is the one place this
	// deliberately parts company with `dies`. That restriction is rule 700.4
	// giving the *word* "dies" to creatures and planeswalkers; it is about what
	// a sentence may say, not about what is worth saying. Exile has no such
	// rule and needs no such narrowing — an artifact exiled is as removed as a
	// creature exiled, and rather more permanently, since it is not coming back
	// from there.
	//
	// **Only off the battlefield**, which is where the noise would otherwise
	// come from. A card exiled from a graveyard, a hand or the top of a library
	// is exiled too, and the library ones are the reason the line is drawn
	// here: impulse draw and cascade exile several cards a turn and put most of
	// them straight back, so a room that raised a beat for each would spend the
	// centre of its arena on bookkeeping and have nothing left over for the
	// Swords to Plowshares. The board still *shows* every one of them; they
	// simply arrive without a sentence, which is already what happens to every
	// change this account does not narrate.
	EventExiled EventKind = "exiled"
	// EventSacrificed is a permanent its controller sacrificed — a cost paid,
	// not something that happened to it.
	//
	// **Deliberately not [EventDies], and the distinction is Magic's rather
	// than ours.** Rule 700.4 gives "dies" to a creature or planeswalker put
	// into a graveyard from the battlefield; a Treasure cracked for mana and a
	// fetchland cracked for a land do neither, so `dies` was right to stay
	// silent about them and the account simply had nothing to say instead
	// (Aaron, 2026-08-26, on the Treasure that taps and then goes "into the
	// ether"). A creature *sacrificed* raises both, which is correct: it was
	// sacrificed and it did die.
	//
	// **[GameEvent.Seat] is the controller** — the player who paid the cost,
	// never the owner of a permanent somebody stole. It was empty for one
	// season, because `GameEventCardSacrificed` is the only card-shaped event
	// on Forge's bus with no player component at all, and a plate that cannot
	// name a seat can only say "Sacrificed" where the rest of this account
	// says who. `Scribe.java` now reads the controller off the card — Forge's
	// own answer, taken out of `GameAction.sacrifice`'s bytecode — and
	// [ScribeParser.seated] falls back to the board for a worker that predates
	// it.
	//
	// Raised only by the scribe, from `GameEventCardSacrificed`. Forge's log
	// has no category for it either.
	EventSacrificed EventKind = "sacrificed"
	// EventAbility is an ability going on the stack — activated by a player, or
	// triggered by the game. [GameEvent.Zone] says where its source was and
	// [GameEvent.Trigger] says which of the two it is.
	//
	// **Abilities reached nothing at all before this**, because the scribe
	// returned on anything that was not a spell, so eminence — a triggered
	// ability whose source sits in the command zone and never moves — was
	// invisible from end to end (Aaron, 2026-08-26: *"It should just visually
	// indicate that an ability is being used"*).
	//
	// **This does not reopen the stack**, which is the fear it looks like it
	// should raise. `board.go`'s first ruling drops Stack *zone* events because
	// they never balance — 52 in against 14 out in one game — and that is
	// untouched. This is a different event about a different subject: it fires
	// once when an ability is put on the stack, nothing accumulates, and nothing
	// waits for it to come off. Measured before it was added, as ADR 42 asks of
	// any new listener on this bus: **ten in a forty-six-turn game**, against 46
	// lands and 548 zone movements.
	EventAbility EventKind = "ability"
	// EventCompanion is a companion bought in from outside the game: it leaves
	// the command zone for its controller's hand, which is the {3} being paid.
	//
	// **It exists because the board looked like it was cheating** (Aaron,
	// 2026-08-27: *"I watched a match play and I swear Kaheera was dealt in a
	// hand? That should not be possible, you don't shuffle your companion in
	// with normal cards to be dealt, they come from outside the game like the
	// commander does"*). He is right about the rules and the engine was right
	// about the game: a companion is never shuffled in, and what he watched was
	// the {3} being paid. The card simply appeared in a hand with nothing said,
	// and a beginner watching a hand gain a card it was never dealt has been
	// shown a game that cheats — which commandment 2 will not have.
	//
	// **The whole sequence, measured on a real match** (Arahbo/Kaheera against
	// itself, seed 11, 2026-08-27) rather than reasoned about:
	//
	//	Sideboard out  ->  Command in     (setup, before the first draw)
	//	  ... three lands tap, the pool rises and drains three times ...
	//	Command out    ->  Hand in        (turn 5)
	//	Hand out       ->  Stack in       (cast, normally, from the hand)
	//
	// It matches Forge's own bytecode. `Match` loads `[Main]` into
	// `ZoneType.Library` and `[Sideboard]` into `ZoneType.Sideboard` — two
	// zones, and only the first is shuffled — then calls
	// `Player.assignCompanion`, which scans the sideboard for `Keyword.COMPANION`,
	// checks the deck restriction and ends in
	// `GameAction.moveTo(ZoneType.Command, companion, ...)`. The effect it
	// leaves beside it carries
	// `Cost$ 3 | Origin$ Command | Destination$ Hand | SorcerySpeed$ True`,
	// which is the ability being activated here.
	//
	// **The {3} is not on the wire, because it is a rule rather than an
	// observation.** Rule 702.139b fixes every companion's cost at {3}; the
	// stream attributes no mana to the ability, and inventing an amount from
	// the lands that happened to tap is the class of inference ADR 44 forbids.
	// A room saying "three mana" is saying what the rule says.
	//
	// **Raised only for a card Forge itself took out of a sideboard**, which is
	// what makes this a fact instead of a guess. Nothing else in a Commander
	// game moves a card from a sideboard into a command zone, and nothing else
	// moves a card from a command zone into a hand — a commander leaving its
	// zone goes to the *stack*, which the same recorded match shows twice.
	EventCompanion EventKind = "companion"
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
	// ID is that card's board id — the same per-game instance id
	// [BoardCard.ID] uses, so a beat and the picture can be pointed at each
	// other.
	//
	// **A name cannot answer "which one".** Two Egg Tokens are one string
	// between them and so are two copies of a commander, so a room lighting up
	// "the card this beat is about" from the name alone lights the wrong one
	// as often as not. The board has always had ids and the account never did.
	//
	// **Zero on the prose path**, which has no ids to give: Forge's log writes
	// `Forest (24)` and the regexes never took the number. A consumer must
	// treat zero as "not said" rather than as a card, which is what
	// `omitempty` is saying.
	ID int `json:"id,omitempty"`
	// Target is the card on the other end: blocked, or damaged.
	Target string `json:"target,omitempty"`
	// TargetSeat is the player on the other end: attacked, or damaged.
	TargetSeat int `json:"target_seat,omitempty"`
	// Amount is damage dealt, or cards kept on a mulligan.
	Amount int `json:"amount,omitempty"`
	// Life is the life total *after* a life change. A pointer because zero
	// life is a real and rather important total.
	Life *int `json:"life,omitempty"`
	// Zone is where an [EventAbility]'s source was standing, in Forge's own
	// word — `Command` or `Battlefield`. Empty on every other kind.
	//
	// **It is the whole of what makes eminence legible.** A commander using an
	// ability from the command zone never moves, so without this the beat is
	// indistinguishable from the same commander doing something on the
	// battlefield, and those are different abilities of different cards.
	Zone string `json:"zone,omitempty"`
	// Trigger is whether an [EventAbility] was raised by the game rather than
	// activated by a player — the difference between "triggers" and "activates",
	// which is the verb the sentence turns on.
	Trigger bool `json:"trigger,omitempty"`
	// Note is an outcome's reason in Forge's own words, kept whole and
	// designed to follow the verb: "because life total reached 0", "trying to
	// draw cards from empty library", "due to accumulation of 21 damage from
	// generals". Render it as `<player> <verb> <note>` and it is a sentence.
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
	// Board is the battlefield as it moved, and it is **null on this path**:
	// nothing in this file can produce one. Forge's game log has no category
	// for a token or a counter at all (ADR 42 measured it — four Food-makers
	// resolved, zero tokens in 453 lines), so a board reconstructed from prose
	// would be right about lands and silently wrong about exactly the decks
	// that most want a picture. `scribe.go` fills it; a worker without the
	// scribe leaves it empty and the room draws the account alone.
	//
	// It rides [EventLog] rather than a wire type of its own for the reason
	// `wire.go` gives about beats: a struct both ends marshal directly cannot
	// drift, because there is no second spelling of it to keep in step.
	Board *BoardReel `json:"board,omitempty"`
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
	//
	// **The only pattern here that is not greedy, and the only one whose
	// reason keeps its first word.** Both come from the same place: Forge's
	// nine outcome sentences, read out of `res/languages/en-US.properties`
	// rather than guessed from the two a test happened to produce.
	//
	//	has won because all opponents have lost
	//	has won due to effect of '%s'
	//	has lost because life total reached 0
	//	has lost because of obtaining 10 poison counters
	//	has lost because an opponent has won by spell '%s'
	//	has lost trying to draw cards from empty library
	//	has lost due to effect of spell '%s'
	//	has lost due to accumulation of 21 damage from generals
	//	has lost for unknown reason (this is a bug)
	//
	// Only three of them say "because", so requiring the word dropped six —
	// including decking and **21 damage from generals**, which is the loss
	// condition this format is named for. It was found by playing two fixture
	// decks and watching one of them mill itself while the parser recorded a
	// winner and no loser.
	//
	// Non-greedy because `has lost because an opponent has won by spell` holds
	// *both* verbs, and the last one is the wrong one: a greedy `.*` reads
	// that line as a win. Non-greedy takes the first, which is the player's
	// own verdict. The trade is a deck literally named "... has won ...",
	// which is nobody's deck; the alternative misreads a sentence Forge
	// really ships.
	//
	// The reason is kept whole — "because life total reached 0", "trying to
	// draw cards from empty library" — because Forge writes these to follow
	// "<player> has won/lost", so the sentence reads correctly wherever it is
	// shown and nothing has to re-grammar it.
	eventOutcomeRe = regexp.MustCompile(`^Game Outcome: Ai\((\d+)\)-.*? has (won|lost) (.+)$`)
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
