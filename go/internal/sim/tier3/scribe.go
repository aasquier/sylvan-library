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
	// Entered is how a permanent reached the battlefield — `cast` or `put`,
	// Magic's own two words — and **the empty string is a third state**: it
	// means nobody said, not that nothing was cast. A worker image built before
	// the scribe learned to ask sends no such field, and neither does the prose
	// path, so a reader that took absence for `put` would tell every old match
	// that every creature in it appeared out of thin air.
	//
	// Forge's own `Card.wasCast()`, reached through the model rather than read
	// off a view — `Scribe.java`'s `entered` argues why the view cannot answer
	// and how the answer was cross-checked. Only ever on a `zone` line
	// arriving on the battlefield; a land is `put`, because a land is played
	// and never cast.
	Entered string `json:"entered"`
	// Keywords is the card instance's **live** keyword set, comma-joined —
	// granted ones included, which is the whole reason it is here. Scryfall's
	// list one layer up is keyed by card name and says what a *printing* has;
	// this says what this copy of it has right now.
	Keywords string `json:"keywords"`
	// CopiedBy is the board id of the card whose ability made this one a copy,
	// and zero for everything else. **Not what it was copied from** — see
	// [BoardCard.CopiedBy], which records why that reading is wrong.
	CopiedBy int `json:"copied_by"`
	// Pool is a seat's floating mana as symbols, `"GGW"`, and the empty string
	// is a drained pool rather than a missing field.
	Pool string `json:"pool"`
	// Trigger is whether an `ability` line is one the game raised rather than
	// one a player activated.
	Trigger bool `json:"trigger"`

	Counter string `json:"counter"`
	// Was and Now are both totals rather than a delta, which is the scribe's
	// own decision and stated in `Scribe.java`: a consumer adding deltas would
	// drift the first time one was dropped. `Was` is read for the history a
	// hover shows — see [BoardCounterMove] — and this reader used to drop it
	// on the floor, which left the board able to say a creature had three
	// counters and unable to say when it got them.
	Was int `json:"was"`
	Now int `json:"now"`

	TargetID int    `json:"target_id"`
	Target   string `json:"target"`
	// Targets is every card an `ability` was aimed at, by board id, comma-joined
	// the way [scribeLine.Keywords] is — the scribe writes flat objects of
	// scalars, so a list travels as a string. Empty for the three abilities in
	// four that target nothing at all.
	//
	// [scribeLine.Target] beside it is the *first* of these, by name, and the
	// two are not a duplicate: a beat says "pumps Bronzehide Lion" and has
	// nowhere to put a list, while the board points at exact cards and cannot
	// use a name — two Egg Tokens are one string between them.
	Targets     string `json:"targets"`
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
	// phantoms is every card id this reader has refused as Forge's own
	// bookkeeping. Per game, like Forge's ids. [ScribeParser.refused] argues
	// why the answer is remembered rather than asked again.
	phantoms map[int]bool
	// sidelined is every card id seen leaving a sideboard, and companions is
	// the ones that went from there into a command zone. Both per game, like
	// Forge's ids. [ScribeParser.outsideTheGame] argues why two steps rather
	// than one, and why this reader identifies a companion at all when the
	// deck one layer up already names it.
	sidelined  map[int]bool
	companions map[int]bool
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
		board: newBoard(), seats: map[int]string{}, phantoms: map[int]bool{},
		sidelined: map[int]bool{}, companions: map[int]bool{}}
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
	p.phantoms = map[int]bool{}
	p.sidelined, p.companions = map[int]bool{}, map[int]bool{}
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
	// **A line whose card came back blank is a line that says nothing about
	// that card**, and it is not a card with no name. See [blankView] for what
	// Forge is doing and how it was measured; the rule here is what to do about
	// it.
	//
	// Only a `zone` line survives one, and only for the half that never came
	// off the view: the zone, the mode and the seat are the event's own fields
	// and are as good as any other line's. Everything else on a blank line —
	// the name, the power, the toughness, the type line, the keywords — is a
	// trackable property sitting at its default, and folding a default is
	// worse than folding nothing. Three things went wrong when this was
	// believed, all of them on the same measured line:
	//
	//   - **A creature died with no name.** The `dies` beat below carried
	//     `l.Card`, so a death inside the sweep raised a beat naming nobody,
	//     and a beat naming nobody draws nothing at all on the centre stage —
	//     a creature died and the room was silent. **Eleven of twelve deaths
	//     in one measured game**, and thirteen of fifteen across two.
	//   - **A dying creature was stamped 0/0.** `board.stats` takes the line's
	//     numbers at face value, so the last thing said about a 3/3 Cat Token
	//     was that it was a 0/0 — which is the size it is drawn at while the
	//     skull is on it.
	//   - **A commander was blacklisted forever.** A commander that dies goes
	//     back to the command zone, and that arrival is blank too — an untyped
	//     card in a command zone, which is [isForgeEffect]'s rule exactly. The
	//     commander was marked one of Forge's own bookkeeping cards and every
	//     later line about it was dropped for the rest of the game: recast, it
	//     never came back to the board. [ScribeParser.refused] holds that half
	//     of the fix, where the rest of that argument already lives.
	//
	// The beats a blank `zone` line still raises name the card out of the
	// dictionary instead — [ScribeParser.named] — which is the same move the
	// `dies` check below already makes for the type line, and for the same
	// reason: the board is holding what this card is, and the answer has to be
	// the same whichever line asks.
	if blankView(l) && l.Kind != "zone" {
		return
	}
	// **The phantoms, on every line that carries a card.** Forge keeps
	// bookkeeping cards with a real id, a real name and — usually — an empty
	// type line; they are invisible in any real game and drawing one puts a
	// blank card beside somebody's commander. `isForgeEffect` is the rule.
	//
	// Checked here rather than inside `case "zone"`, where it used to be, for
	// two reasons. A zone line is not the only way one of these reaches the
	// board: `stats`, `counters` and `attach` all name a card and all raise a
	// change against it. And the name went into the dictionary *before* the
	// old check, so every effect in the match was carried to the browser as a
	// card even when nothing ever moved it.
	//
	// The answer is [ScribeParser.refused]'s rather than [isForgeEffect]'s
	// directly, because it has to be the same answer on every line.
	switch l.Kind {
	case "zone", "attach", "detach", "tapped", "stats", "counters",
		"sacrificed", "ability":
		if p.refused(l) {
			return
		}
	}
	// **The keywords ride every line that names a card**, because the scribe
	// puts them on every line that names a card — a creature that gains
	// vigilance is usually re-announced by whatever gave it, and waiting for a
	// `stats` line would miss a grant that changed no numbers.
	//
	// Here as well as in [ScribeParser.note] because the two cover different
	// halves: this catches the kinds that never name a card of their own —
	// `attack`, `block`, `damage` — and `note` catches the first sighting, where
	// the card is not in the dictionary yet and this can do nothing. Both are
	// idempotent, so the overlap costs a map lookup.
	if l.ID != 0 && !blankView(l) {
		p.board.keywords(l.ID, l.Keywords)
	}
	switch l.Kind {
	case "zone":
		if !blankView(l) {
			p.note(l, l.Seat)
		}
		was := p.board.zone[l.ID]
		if was == ZoneGone {
			// Forge announces the leaving before the arriving, so the zone a
			// card is *in* is already nothing by the time it lands somewhere.
			// The zone it came *from* is what this asks about.
			was = p.board.left[l.ID]
		}
		p.board.moved(l.ID, l.Zone, l.Mode, l.Seat)
		if !blankView(l) {
			p.board.stats(l.ID, l.Power, l.Toughness, l.Types)
		}
		// **A companion arriving in a hand from the command zone**, which is
		// the {3} being paid and was the one moment on this board with nothing
		// said about it at all. See [EventCompanion] — and see
		// [ScribeParser.outsideTheGame], which is also the reader that learns
		// which card is the companion in the first place.
		if p.outsideTheGame(l, was) {
			p.raise(GameEvent{Kind: EventCompanion, Seat: p.seated(l),
				Card: p.named(l), ID: l.ID})
			return
		}
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
				p.raise(GameEvent{Kind: EventDies, Seat: p.seated(l),
					Card: p.named(l), ID: l.ID})
			}
			return
		}
		// **A permanent exiled**, which is the other way off the sand and had
		// nothing to say for itself. See [EventExiled] for why this is every
		// permanent rather than only creatures, and why it is only ever the
		// battlefield it is leaving.
		//
		// `was` is the zone the card came *from*, worked out above — Forge
		// announces the leaving before the arriving, so by the time a card
		// lands anywhere its own zone already reads as nothing. `ZoneLand` is
		// in here beside `ZoneBattlefield` for the reason it is in the `dies`
		// check above: the board draws lands in a row of their own, and a
		// fetchland cracked for a land is a permanent leaving play.
		if l.Mode == "in" && l.Zone == "Exile" &&
			(was == ZoneBattlefield || was == ZoneLand) {
			p.raise(GameEvent{Kind: EventExiled, Seat: p.seated(l),
				Card: p.named(l), ID: l.ID})
			return
		}
		// A permanent arriving. See [EventEnters] for why lands are excluded
		// and why this is not called "resolves".
		//
		// **Whether it was cast rides along**, which is the difference between
		// a creature the room has just watched somebody pay for and one that
		// simply appeared. `l.Entered` is Forge's own answer, carried straight
		// through — see [GameEvent.Entered] for why the empty string is a third
		// state rather than a `put`, and why nothing here supplies a default.
		if p.board.zone[l.ID] == ZoneBattlefield && was != ZoneBattlefield {
			p.raise(GameEvent{Kind: EventEnters, Seat: p.seated(l),
				Card: p.named(l), ID: l.ID, Entered: l.Entered})
		}
	case "attach", "detach":
		p.note(l, 0)
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
		p.note(l, 0)
		p.board.tap(l.ID, l.Tapped)
	case "stats":
		p.note(l, 0)
		p.board.stats(l.ID, l.Power, l.Toughness, l.Types)
	case "counters":
		p.note(l, 0)
		p.board.counter(l.ID, l.Counter, l.Was, l.Now)
	case "life":
		if l.Life == nil {
			return
		}
		p.board.lives(l.Seat, *l.Life)
		after := *l.Life
		p.raise(GameEvent{Kind: EventLife, Seat: l.Seat, Life: &after})
	case "player_counters":
		// A player's own counters — poison and its relatives. No beat, for
		// `mana`'s reason a few lines down: this is the scoreboard moving, and
		// the sentence that goes with it has already been said by the combat
		// that caused it. The step it lands on is whichever beat comes next, so
		// the bead climbs as the damage is narrated.
		//
		// `Now` is a total rather than a delta on both sides of the pipe —
		// `Scribe.java` argues it against Forge's bytecode, because the field
		// it is read from is called `amount`.
		p.board.playerCounter(l.Seat, l.Counter, l.Now)
	case "mana":
		// The floating pool, as it stands, for one seat. No beat: mana filling
		// and draining is a picture rather than a sentence, and a room that
		// narrated every tap would say nothing else all game.
		p.board.floating(l.Seat, l.Pool)
	case "sacrificed":
		// **The word for how a permanent left**, which the board had no way to
		// say. A Treasure or a fetchland cracked for value is not a death — rule
		// 700.4 gives that word to creatures and planeswalkers — so `dies` was
		// right to stay silent and there was nothing else to say instead.
		//
		// The fate is recorded before the zone change arrives, which is the
		// order Forge announces them in: `sacrificed` then `Battlefield out`
		// then `Graveyard in`, measured on a real match. All three land on the
		// same step, so the card leaves and says why in one movement.
		//
		// **The seat is the board's when the line has none**, and for one
		// season it always had none: `GameEventCardSacrificed` is the one
		// card-shaped event on Forge's bus with no player component at all, so
		// this beat reached the room with `who: null` and a plate that could
		// only say "Sacrificed" where every other beat says who did it. The
		// scribe now reads the controller off the card and sends a seat
		// (`Scribe.java` argues it against `GameAction.sacrifice`'s bytecode) —
		// and this falls back to the board for the worker deployed before that,
		// which is ADR 42's fourth decision applied one field down: an older
		// scribe degrades, it does not lie.
		p.note(l, l.Seat)
		p.board.fate(l.ID, FateSacrificed)
		p.raise(GameEvent{Kind: EventSacrificed, Seat: p.seated(l),
			Card: p.named(l), ID: l.ID})
	case "combat_end":
		// Forge saying combat is over, rather than the next turn implying it.
		// [board.combatEnded] argues why this is the event and why the turn
		// boundary survives beneath it.
		p.board.combatEnded()
	case "ability":
		// An ability going on the stack. **This is not the stack**: nothing is
		// held, nothing waits to come off, and `Stack` zone events are still
		// dropped whole — see ruling 1 in `board.go`. It is one moment on one
		// step, which is why it rides [BoardStep.Abilities] rather than a card.
		//
		// Eminence is the ask, and it is why the zone travels: a commander using
		// an ability from the command zone never moves, so there is no other
		// signal anywhere in this stream that it did anything at all.
		//
		// **What it was aimed at travels twice, at two grains, and neither is
		// the other's copy.** The board takes the ids, because pointing at a
		// card is the only way to say *which* Bronzehide Lion; the beat takes
		// the first target's name, because a sentence has nowhere to put a list
		// and "pumps Bronzehide Lion" is what a room reads out. See
		// [BoardAbility.Targets] for how often either is there at all.
		p.note(l, l.Seat)
		p.board.usedAbility(l.ID, l.Seat, l.Zone, l.Trigger, l.Targets)
		p.raise(GameEvent{Kind: EventAbility, Seat: l.Seat, Card: l.Card,
			ID: l.ID, Zone: l.Zone, Trigger: l.Trigger, Target: l.Target})
	case "turn":
		p.board.began(l.Turn, l.Seat)
		if l.Life != nil {
			p.board.lives(l.Seat, *l.Life)
		}
		p.raise(GameEvent{Kind: EventTurn, Seat: l.Seat})
	case "mulligan":
		p.raise(GameEvent{Kind: EventMulligan, Seat: l.Seat})
	case "land":
		p.raise(GameEvent{Kind: EventLand, Seat: l.Seat, Card: l.Card, ID: l.ID})
	case "cast":
		p.raise(GameEvent{Kind: EventCast, Seat: l.Seat, Card: l.Card, ID: l.ID})
	case "attack":
		// **The board learns the fight too, not only the account.** An
		// attacking creature and a sleeping one used to be the same picture,
		// so a wall of tokens said nothing about which of them was swinging
		// (Aaron, 2026-08-26, on wanting attacking and blocking token piles
		// apart). `against_seat` is who is being attacked; `seat` is the
		// attacker's own controller and is not it.
		p.board.inCombat(l.ID, CombatAttacking, l.AgainstSeat, 0)
		p.raise(GameEvent{Kind: EventAttack, Seat: l.Seat, Card: l.Card, ID: l.ID,
			TargetSeat: l.AgainstSeat})
	case "block":
		// `target_id` is the **attacker** this creature stopped, and it is the
		// half of the line the beat cannot carry: two Egg Tokens are one name
		// and the account has nowhere to put an id. On the board a blocker
		// points at the exact card it is facing.
		p.board.inCombat(l.ID, CombatBlocking, 0, l.TargetID)
		p.raise(GameEvent{Kind: EventBlock, Seat: l.Seat, Card: l.Card, ID: l.ID,
			Target: l.Target})
	case "unblocked":
		// The seat is who declined to block, so the card is the *attacker* —
		// the same reading the prose parser's own comment records. **The board
		// is deliberately not touched here**: the attacker was already marked
		// by its own `attack` line, and this line's seat is the defender's, so
		// folding it would name the wrong side of the table as the one under
		// attack.
		p.raise(GameEvent{Kind: EventUnblocked, Seat: l.Seat, Card: l.Card, ID: l.ID})
	case "damage":
		p.raise(GameEvent{Kind: EventDamage, Card: l.Card, Amount: l.Amount,
			Target: l.Target, TargetSeat: l.AgainstSeat})
	case "outcome":
		p.outcome(l)
	}
}

// note records a card the board is about to be told something about: its name,
// and the two facts that ride every line naming it.
//
// **The order is the point.** `copiedBy` and `keywords` both need the card in
// the dictionary, and on a card's *first* line it is not there until `name` has
// run — so the prelude in [ScribeParser.fold], which cannot know whether a kind
// names its card, silently did nothing for exactly the line that introduces a
// populated token. Both are idempotent, so the prelude and this can overlap
// without either having to know about the other.
func (p *ScribeParser) note(l scribeLine, seat int) {
	p.board.name(l.ID, l.Card, l.Types, l.Token, seat)
	p.board.copiedBy(l.ID, l.CopiedBy)
	p.board.keywords(l.ID, l.Keywords)
}

// blankView is whether this line's card came back at its defaults — a view
// Forge had not filled in yet, rather than a card with nothing to say.
//
// **Forge freezes its own view layer while it applies state-based actions**,
// and that is the whole mechanism. `forge.trackable.TrackableObject.set`
// defers a write while `Tracker.isFrozen()` and the property is
// `RespectsFreeze`; `TrackableProperty.Name` is one of those and its default
// value is the empty string, measured by asking the enum on Forge 2.0.14.
// `javap -c` puts `Tracker.freeze()` at the top of
// `GameAction.checkStateEffects` and the matching `unfreeze()` at the bottom,
// so every death by lethal damage, every legend-rule sacrifice and every
// commander going home happens inside the frozen window — while
// `GameAction.sacrifice`, which is a cost being paid, does not.
//
// That is exactly what a real match shows. Two Squirrel Tokens on one board:
// the one given up as a cost arrived in the graveyard as
// `"card":"Squirrel Token"`, and the one that died in combat arrived as
// `"card":"","power":0,"toughness":0,"types":""` one line after a
// `Battlefield out` that named it in full (Arahbo/Cats against Gyome/Food,
// seed 11, 2026-08-27: thirteen of that game's twenty-one graveyard arrivals
// blank, and nineteen of thirty-six across the two games).
//
// **Nothing is wrong on the scribe's side and there is nothing for it to
// send.** It renders the event as itself, which is this package's whole
// division of labour — the view really was empty at the instant Forge
// announced the move. Reassembling a card from what came before belongs on
// this side of the pipe (ADR 14), which is what [ScribeParser.named] does.
//
// **The name is the test, and it is deliberately the loose one.** The rest of
// a frozen line is empty too — every measured one carried no type line and 0/0
// — so `card == "" && types == ""` would be the tighter rule. It is not used,
// because the two failures are not the same size: a frozen line this misses
// puts all three bugs above straight back, and the only thing the loose rule
// over-catches is a **face-down** card. Forge builds that state as a nameless
// 2/2 Creature — `CardUtil.getFaceDownCharacteristic` calls `setName("")` on
// it, which `javap -c` shows plainly — so a morph reads as blank here and this
// board would go on calling it whatever it was called last. That costs
// something only for a permanent turned face down *after* being seen face up,
// which is a card Forge's AI decks do not play; a creature cast face down was
// never in the dictionary, so nothing is revealed and nothing is drawn, which
// is what already happened before any of this.
func blankView(l scribeLine) bool { return l.ID != 0 && l.Card == "" }

// named is the card a beat is about: the line's name when it has one, and the
// board's when the line's view came back blank.
//
// The dictionary is the right second source and not a guess — [board.name]
// refuses to record an empty name, so an answer from here is one the scribe
// itself said about this exact id earlier in this exact game. A card the board
// has never been told about still comes back empty, which is honest: nothing
// downstream can draw it either.
func (p *ScribeParser) named(l scribeLine) string {
	if l.Card != "" {
		return l.Card
	}
	return p.board.nameOf(l.ID)
}

// seated is the seat a beat belongs to: the line's when it carries one, and
// the board's when it does not.
//
// Two lines arrive without a seat and mean different things by it. A blank
// view has lost the *whole* card, seat included. And `sacrificed` never
// carried one at all until the scribe learned to read the controller off the
// card — so this is also what a worker image built before that change falls
// back to.
func (p *ScribeParser) seated(l scribeLine) int {
	if l.Seat != 0 {
		return l.Seat
	}
	return p.board.seatOf(l.ID)
}

// outsideTheGame follows a companion, and answers whether this line is the
// {3} being paid for it. See [EventCompanion] for the whole sequence and the
// bytecode behind it.
//
// **Three lines make the answer, and they are turns apart.** Forge announces
// `Sideboard out` and then `Command in` before the first turn begins, and that
// pair is the signature: `forge.game.Match` fills `ZoneType.Sideboard` from the
// `.dck`'s `[Sideboard]` and immediately calls `Player.assignCompanion`, which
// is the only thing in a game of Commander that moves a card from a sideboard
// into a command zone. Much later — turn 5 in the recorded match — the same
// card goes `Command out`, `Hand in`, and that is the ability being activated.
//
// **A wish is the near miss, and the destination is what parts them.** Forge's
// `ChangeZoneEffect` takes cards out of a sideboard too — that is what Living
// Wish is — but it puts them in a hand or on the battlefield, never in a
// command zone. Requiring the command zone *and* the setup keeps the two
// apart without this having to know what a wish is.
//
// **A second reader of the same fact, deliberately.** `api/forge.go` already
// names the companion, off `deck.yaml`'s own declaration, and normally a
// second derivation of one fact is the thing this package refuses to build
// ([forgeBoardSeat.Shape] argues exactly that). These two are not the same
// fact. The deck says what was *declared*; this says what Forge actually
// *did*, and they part company for real reasons — Forge checks the companion's
// deck restriction against the library it built and simply leaves a companion
// that fails it sitting in the sideboard, where nothing ever moves it. The
// account may only narrate what happened.
//
// A card leaving a sideboard is remembered rather than matched against the
// next line, because Forge promises nothing about what sits between the two.
func (p *ScribeParser) outsideTheGame(l scribeLine, was string) bool {
	if l.ID == 0 {
		return false
	}
	switch {
	case l.Mode == "out" && l.Zone == "Sideboard":
		p.sidelined[l.ID] = true
	case l.Mode == "in" && l.Zone == "Command" && p.sidelined[l.ID] && p.turn == 0:
		p.companions[l.ID] = true
	case l.Mode == "in" && l.Zone == "Hand" && was == ZoneCommand:
		// **The zone it came from is checked as well as the card**, because a
		// companion is a card like any other once it has been bought: it can
		// be discarded, milled, and bounced back to the hand by anything that
		// bounces a creature. Only the trip out of the command zone is the
		// one somebody paid three mana for.
		return p.companions[l.ID]
	}
	return false
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
	p.phantoms = map[int]bool{}
	p.turn, p.outcomeTurn = 0, 0
	return log, &game
}

// refused is whether this line is about one of Forge's own bookkeeping cards,
// and it remembers the answer for the rest of the game.
//
// **A refusal is a fact about the card, not about the line**, and that is the
// whole of this. [isForgeEffect] asks three questions and two of them — where
// the card is, and whether it has a type line — are properties of the object
// rather than of the event. Only a `zone` line carries a zone. So moving the
// guard onto every card-shaped line moved the *name-shape* half of the rule
// and could not move the other: a `Commander Effect` refused on the line that
// created it was let straight back in by the next `stats`, `tapped` or
// `attach` line about it, because those carry no zone for the rule to read.
// [ScribeParser.note] then put it in the dictionary and the board folded a
// change against it — the phantom back on the wire, one line later, wearing
// the same name Aaron reported seeing in the command zone.
//
// The same inconsistency is what left a **nameless attachment** on the board.
// The dictionary and the changes were deciding separately whether a card
// exists, and once two places decide that, it is only a question of which line
// arrives to make them disagree. `board.change` holds the other half of the
// answer: a change is never folded against a card the dictionary does not
// hold.
//
// **Asked at the earliest moment there is.** Every one of the 145 first
// sightings in `testdata/scribed-match.ndjson` is a `zone` line, which is what
// Forge's bus does — an object exists because it entered a zone — so a card is
// judged on its arrival and never on a later line that knows less about it. A
// phantom whose first line was something else would still reach the dictionary;
// nothing in a recorded match does that, and the board refuses to draw it
// either way.
//
// Nothing here is looser than what stood before: the refusals are exactly the
// ones [isForgeEffect] already made, applied to every line about the card
// instead of to whichever line happened to carry the fields.
//
// # A blank view is not evidence, and it convicted a commander
//
// The paragraph above is the rule this file already lived by — *a refusal is a
// fact about the card* — and a line whose view came back empty ([blankView])
// carries no fact about the card to judge. It reads as an untyped card in a
// command zone, which is [isForgeEffect]'s rule exactly, so a **commander**
// that died and went home was convicted of being one of Forge's own
// bookkeeping cards and every later line about it was dropped for the rest of
// the game. Measured: Arahbo, Roar of the World, on the first of two deaths.
//
// The memory above still runs first, and has to: a real Forge effect is named
// on its own first `zone` line, so it is already in `phantoms` long before any
// blank line about it could arrive. Nothing that was refused stops being
// refused.
func (p *ScribeParser) refused(l scribeLine) bool {
	if l.ID != 0 && p.phantoms[l.ID] {
		return true
	}
	if blankView(l) {
		return false
	}
	if !isForgeEffect(l.Zone, l.Card, l.Types) {
		return false
	}
	if l.ID != 0 {
		p.phantoms[l.ID] = true
	}
	return true
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
//
// # The fourth fact, and why the first three were not enough
//
// The rule above is an **and** of two conditions, and a real board got past it
// (Aaron, 2026-08-26: *"hovering on the command zone pops up some things I
// don't understand, like 'Olinda the Oblivious (99)'s Effect'"*). Either half
// can fail — an effect reported outside a command zone, or one that arrived
// carrying a type line — and both leave a blank card in somebody's picture.
//
// So a fourth fact stands beside them, and it needs neither: **an effect built
// by an ability writes its own source into its name, id and all.**
// `SpellAbilityEffect.createEffect` names it from the host card, and Forge's
// `Card.toString()` renders a card as `Name (id)` — which is exactly the shape
// of `Rogue's Passage (123)'s Effect` and of Olinda's, where the number is the
// *host's* id and not the effect's. [forgeEffectName] matches that shape and
// nothing else.
//
// **No Magic card can collide with it.** Card names have never contained a
// parenthesis, so a real card called `Somebody's Effect` — the case the tests
// pin — is untouched, and so is one called `Butterfly Effect` if a set ever
// prints one. The narrow shape is the whole safety: matching on the word
// `Effect` alone would eventually eat a real card, and matching on the
// parenthesised id cannot.
func isForgeEffect(zone, name, types string) bool {
	if strings.HasPrefix(name, "Emblem") {
		return false
	}
	if forgeEffectName(name) {
		return true
	}
	return zone == "Command" && types == ""
}

// forgeEffectName is whether a name is one Forge built for an ability's
// effect: a host card rendered as `Name (id)`, with `'s Effect` after it.
func forgeEffectName(name string) bool {
	host, ok := strings.CutSuffix(name, "'s Effect")
	if !ok || !strings.HasSuffix(host, ")") {
		return false
	}
	open := strings.LastIndex(host, "(")
	if open < 0 {
		return false
	}
	id := host[open+1 : len(host)-1]
	if id == "" {
		return false
	}
	for _, digit := range id {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	// Something has to be the host. `(12)'s Effect` on its own is not a shape
	// Forge produces, and swallowing it would be swallowing an unknown.
	return strings.TrimSpace(host[:open]) != ""
}
