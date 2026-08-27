package tier3

import (
	"strconv"
	"strings"
)

// The board: Forge's events folded into somewhere to put the cards.
//
// The scribe reports events and never state (ADR 42, and `scribe/README.md`
// says so in as many words). This is the far side of that division — the place
// where "a card entered the graveyard" becomes "the graveyard has this card in
// it" — and it is in Go rather than in the browser for ADR 14's reason: it is
// a decision about a game of Magic, so it belongs with the deterministic code,
// where it can be tested against a recorded match with no JVM anywhere near
// it. `testdata/scribed-match.ndjson` is that match.
//
// **Four rulings are baked in here, and every one was measured rather than
// assumed.** They are the difference between a board and a plausible-looking
// board, which is the worse of the two.
//
//  1. **The stack is not a zone.** One real game raised 52 `Stack in` events
//     against 14 `Stack out`: an activated ability puts its source card on the
//     stack and Forge never announces it coming off. Modelled as a set, the
//     stack would accumulate about thirty-eight phantom cards a game — and
//     worse, `Sakura-Tribe Elder` goes `Graveyard in` and *then* `Stack in`
//     when its sacrifice ability is activated, so a naive reading resurrects
//     it. Stack events are dropped whole. A card being cast is simply in
//     transit between leaving hand and arriving somewhere, which is what
//     being on the stack is.
//  2. **The library is dropped on the floor.** Forge reports it — we could
//     show the top of anyone's deck. Showing a hand is a broadcast; showing
//     the library is showing the answers. It is discarded *here*, in Go, so
//     that it cannot reach a browser by being forgotten about later.
//  3. **A Forge "effect" is not a card.** Forge keeps bookkeeping cards in
//     the command zone with a real name, a real id and an *empty type line* —
//     `Commander Effect` for each player (ids 101 and 202 in the recorded
//     match), one per companion, and one for every activated ability that
//     needs somewhere to hang. They are invisible in any real game and
//     drawing one puts a blank card beside somebody's commander. Neither half
//     of that description holds always — a real board produced one carrying a
//     type line and one outside a command zone — so a fourth fact stands
//     beside the three bytecode ones: an effect an ability built writes its
//     own source into its name. `isForgeEffect` in `scribe.go` is the rule and
//     carries all four; `ScribeParser.refused` is why the answer is asked once
//     per card rather than again on every line, and `board.change` is the
//     other side of the same coin — the board says nothing about a card the
//     dictionary has not named.
//  4. **`out` only clears a zone the card is actually in.** Forge does not
//     promise that a card's `out` of its old zone precedes its `in` to the
//     new one, and an unconditional clear on a late `out` blanks a card that
//     has already arrived somewhere. Measured on the same match: the ordering
//     holds for hand-to-battlefield and does not for every path.
//
//  5. **A permanent that changes zones is a new object, and it arrives with
//     nothing on it.** Rule 400.7: the object that leaves a zone and the one
//     that arrives in the next are not the same object, so counters do not
//     travel with it and it is not attacking anything any more. Forge raises
//     no event saying so — `GameEventCardCounters` fires when a counter is
//     put on or taken off, never when the card carrying them stops existing —
//     so the board sheds them itself, on the zone change (Aaron, 2026-08-26:
//     *"counters are following things into exile, the graveyard, and the
//     command zone"*). `shed` is the rule and `magicZone` is the care it
//     needs: the [ZoneLand] split below is this package's furniture rather
//     than a zone of Magic's, so an animated Dryad Arbor changing rows has
//     changed nothing and keeps everything.
//
// A sixth thing is a choice rather than a finding: **lands sit in their own
// zone**. That is Aaron's ask and it is right — a battlefield where six
// Forests are shuffled in among the creatures is a battlefield nobody can
// read. The split is on the type line, so a creature-land that animates moves
// rows, which is honest about what it currently is.

// Board zone names, as a browser receives them.
//
// Words rather than Forge's own enum: these are read by a person debugging a
// payload, and they are not all Forge zones anyway — [ZoneLand] is a split
// this package makes and [ZoneGone] is the absence of one.
const (
	ZoneBattlefield = "battlefield"
	ZoneLand        = "land"
	ZoneHand        = "hand"
	ZoneGraveyard   = "graveyard"
	ZoneExile       = "exile"
	ZoneCommand     = "command"
	// ZoneGone is a card that has left every zone this board draws — shuffled
	// back into the library, most often. It is reported rather than silently
	// dropped so a browser removes the card it is holding instead of leaving
	// it on the table forever.
	ZoneGone = "gone"
)

// BoardCard is one card in one game, named once.
//
// **The dictionary is what makes a reel small.** Every step below refers to a
// card by Forge's per-game instance id, and this is the only place the name,
// the type line and the token flag are spelled out — a game touches about
// seventy-five distinct cards and raises several hundred changes about them,
// so naming each card once instead of once per change is most of the payload.
//
// `Token` is not decoration: a token's id is real but its card is not, so
// whatever resolves these names to art has to know not to look for a printing
// of "Food Token" the way it would look for one of "Academy Manufactor".
type BoardCard struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Token bool   `json:"token,omitempty"`
	Types string `json:"types,omitempty"`
	// Seat is whose card this is, 1-based, from the first zone it was seen in.
	// A stolen permanent moves seats with its zone change; this is where it
	// started.
	Seat int `json:"seat,omitempty"`
	// CopiedBy is the board id of the card whose ability made this one, and it
	// is set only when this card is a **copy** of something.
	//
	// Populate is the ask (Aaron, 2026-08-26: *"It really is making a clone, or
	// splitting one thing into two"*), and its presence is the copy: a token
	// minted fresh carries nothing here and a populated one carries this, so
	// "was this card copied into existence" is answerable at all for the first
	// time. `GameEventTokenCreated` is a bare signal with no fields and could
	// never have answered it.
	//
	// **It is not what the card was copied *from*.** Forge sets a token's clone
	// origin to the *ability's host* — a Centaur Token populated by Growing
	// Ranks names Growing Ranks, not the Centaur Token it duplicated — which
	// `scribe/Scribe.java` records against the bytecode. The permanent that was
	// copied lives on Forge's model rather than its view and does not reach this
	// pipe, so it is not here and is not guessed at.
	//
	// In the dictionary rather than in a change because it never moves: a card
	// is made a copy once, at the instant it exists.
	CopiedBy int `json:"copied_by,omitempty"`
}

// BoardCounter is one kind of counter and how many are on the card.
type BoardCounter struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
}

// BoardCounterMove is one counter event: which kind moved, and from what to
// what.
//
// **The set says what a card has; this says how it got there** (Aaron,
// 2026-08-26: *"keep a history of why a creature has all of the counters it
// does"*). A hover wants the account — two counters on turn four, one more on
// turn six — and a current set of `+1/+1: 3` cannot produce it, because by
// then the arithmetic that made the three has scrolled past.
//
// **It says how, and it does not say who.** Forge's `GameEventCardCounters`
// carries the card the counters landed on and nothing about the source
// (`docs/adr/0042-a-scribe-rides-forges-event-bus.md` records the gap), so
// there is no honest way to name the card that put them there. Blaming the
// spell cast most recently would be inference wearing a fact's clothes, and
// ADR 14 is explicit that deterministic code says only what it knows. So the
// truthful half ships and the attributed half does not exist.
//
// `Was` is Forge's own `oldValue()` rather than this board's previous total.
// They agree in every ordinary case; where they could not, Forge is the game
// and this is a reader of it.
type BoardCounterMove struct {
	Kind string `json:"kind"`
	Was  int    `json:"was"`
	Now  int    `json:"now"`
}

// What a creature is doing in the combat happening now — [BoardChange.Combat].
//
// Words rather than flags, for [ZoneBattlefield]'s reason: a person reading a
// payload should not need a table. The empty string is the third value and it
// is a real one — a creature that has stopped attacking — which is why the
// field is a pointer.
const (
	CombatAttacking = "attacking"
	CombatBlocking  = "blocking"
)

// BoardChange is one card, as much of it as changed at one step.
//
// Everything is optional because almost everything is usually unchanged: a
// land tapping is one field, and a creature arriving is three. The pointers
// are the difference between "did not change" and "changed to zero", which for
// a 0/0 token and for an untapping land are both real states.
type BoardChange struct {
	ID   int    `json:"id"`
	Zone string `json:"zone,omitempty"`
	// Seat is whose zone it moved into, when that changed — a permanent
	// changing controller is a seat change with no zone change.
	Seat      int    `json:"seat,omitempty"`
	Tapped    *bool  `json:"tapped,omitempty"`
	Power     *int   `json:"power,omitempty"`
	Toughness *int   `json:"toughness,omitempty"`
	Types     string `json:"types,omitempty"`
	// Counters is the card's whole counter set, whenever any of it moved. The
	// whole set rather than the part that changed, because a browser holding a
	// partial one has no way to know a kind went to zero.
	//
	// **A pointer, so that "none" can be said out loud.** An empty slice and an
	// absent field are the same bytes under `omitempty`, and the two states are
	// not the same fact at all: absent is "nothing about counters changed this
	// step", which is true of nearly every card nearly every step, and empty is
	// "this card has none", which is what a creature that just died has. Sent
	// as a plain slice, the last counter coming off a card never reached the
	// browser and the card kept wearing it.
	Counters *[]BoardCounter `json:"counters,omitempty"`
	// CounterMoves is every counter event this card raised at this step, in
	// order. See [BoardCounterMove] — this is the history a hover reads, and
	// the browser accumulates it rather than the server holding one.
	//
	// **A delta rather than a running list**, which is the whole reason it is
	// affordable: a board is folded from the first step on every render, so a
	// per-card history carried in full would be re-sent and re-walked on every
	// one of a hundred and thirty steps. One entry per event is the same volume
	// the stream already has.
	CounterMoves []BoardCounterMove `json:"counter_moves,omitempty"`
	// Combat is this creature's part in the combat happening now:
	// [CombatAttacking], [CombatBlocking], or the empty string for one standing
	// out of it.
	//
	// A pointer for [BoardChange.AttachedTo]'s reason — the empty string is the
	// value that says a creature has *stopped* — and it exists at all because
	// the board could not draw a fight. `attack` and `block` arrive as beats,
	// so the account said who was swinging and the picture never did: a wall of
	// tokens looked the same whether it was attacking, blocking or asleep.
	Combat *string `json:"combat,omitempty"`
	// Attacking is the seat this creature is attacking, and zero for a creature
	// that is not. A pointer for the same reason as [BoardChange.Combat].
	Attacking *int `json:"attacking,omitempty"`
	// Blocking is the **board id** of the attacker this creature is blocking,
	// and zero for a creature that is not blocking.
	//
	// The id rather than the name, because the name cannot answer the question:
	// two Egg Tokens across the seam are the same string, and pairing a blocker
	// to "the attacker called Egg Token" pairs it to whichever one is drawn
	// first. Forge sends `target_id` on every block line and this reader used to
	// drop it.
	Blocking *int `json:"blocking,omitempty"`
	// Casts is how many times this card has **left the command zone**, which is
	// what commander tax is counted from — two generic for each previous cast.
	//
	// Here rather than in the browser, which used to count the same zone
	// transitions itself. Forge reports no tax and `CardView.isCommander()` is
	// deliberately not carried (`go/internal/api/forge.go` argues that), so the
	// count is the only answer available — and counting is a reading of the
	// game, which ADR 14 puts on this side of the wire. The browser deciding it
	// was a second place for the rule to drift.
	//
	// Every card is counted, not only commanders: this layer does not know
	// which cards a deck named, and `forgeBoardSeat` one layer up does.
	Casts *int `json:"casts,omitempty"`
	// AttachedTo is the card this one is attached to: an Aura on what it
	// enchants, an Equipment on what it is equipping.
	//
	// **A pointer to zero is the detach**, and that is why this is not a plain
	// int. Nil means "did not change this step", which is true of every card
	// on the board almost every step; zero means "attached to nothing now",
	// which a sword coming off a bear really is. An int alone would make those
	// two the same value and a detached sword would stay drawn on the bear
	// forever.
	//
	// An Aura on a *player* — a curse — reports no host, because the board
	// draws players as rails rather than as cards and there is nothing there
	// to hang it on. It stays in its own row, which is where it was.
	AttachedTo *int `json:"attached_to,omitempty"`
	// Live is every keyword this card **instance** has right now, granted ones
	// included — not the keywords its printing carries.
	//
	// The difference is the whole reason it exists (Aaron, 2026-08-26: *"Some
	// cards like Kaheera give vigilance or another effect to other cards, we
	// currently are not representing that symbolically"*). The board's only
	// keywords until now were Scryfall's, keyed by card *name*, so every copy of
	// a card wore the same marks all game and a Beast standing beside Kaheera
	// gained a visible +1/+1 and an invisible vigilance.
	//
	// A pointer for [BoardChange.Counters]'s reason: a creature that *loses* its
	// last granted keyword has an empty set, and under a plain slice that is the
	// same bytes as "nothing changed".
	//
	// **Which of these are granted is not decided here**, because this layer
	// does not know what any card was printed with — `api/forge.go` does, and it
	// fills [BoardChange.Granted] from this. Forge itself cannot be asked: its
	// view layer erases the `isIntrinsic` flag and every trace of what granted a
	// keyword, which `Scribe.java` records.
	Live *[]string `json:"live,omitempty"`
	// Granted is the subset of [BoardChange.Live] that this card's **printing
	// does not carry** — the keywords something else gave it.
	//
	// **Filled one layer up, in `api/forge.go`, and never here.** It is a
	// comparison against Scryfall's keyword list for the card, which is a fact
	// this package has no access to and no business holding; the shaping layer
	// already resolves every card's printing to paint the board and knows it for
	// free.
	//
	// It says *that* a keyword was granted and never *by what*. Forge's view
	// layer erases attribution completely — `KeywordView` is four fields and
	// none of them is a source — so the card to blame is not available at any
	// price, and inventing one is what ADR 44 exists to forbid. The copy that
	// renders this must not imply an agent.
	Granted *[]string `json:"granted,omitempty"`
	// Fate is **how** this permanent left, when Forge said so: [FateSacrificed],
	// and nothing else.
	//
	// A Treasure cracked for mana raised no beat at all and folded silently into
	// the next step (Aaron, 2026-08-26: *"things that tap before being
	// sacrificed… they must tap to sacrifice and they go into the ether"*). A
	// fetchland is the same shape. `dies` cannot cover them: rule 700.4 gives
	// that word to creatures and planeswalkers, and an artifact cracked for mana
	// does not die.
	//
	// **Sacrifice is the only word this bus has, and the other two are not
	// coming.** `GameEventCardDestroyed` is a record with no components at all —
	// it cannot say which card — and a combat death is announced nowhere as
	// such. So the board says "sacrificed" where Forge said it and says nothing
	// about the rest, rather than reading a word off the circumstances.
	Fate string `json:"fate,omitempty"`
}

// FateSacrificed is [BoardChange.Fate] for a permanent its controller
// sacrificed — a cost paid, rather than something that happened to it.
const FateSacrificed = "sacrificed"

// BoardFloating is one seat's floating mana, as the symbols a person writes.
//
// `"GGW"` is two green and one white; `""` is an empty pool, which is a real
// answer and the one that ends every step. Per seat rather than per card
// because a pool belongs to a player (ADR 44 says so in as many words), which
// is why this sits beside [BoardStep.Life] and not on a permanent.
//
// **Not to be confused with `forgeBoardCard.Mana`**, one layer up, which is
// Scryfall's `produced_mana` — a static fact about whether a printing can make
// mana at all. This is mana that exists right now and drains at the end of the
// step. The names are deliberately different for that reason.
type BoardFloating struct {
	Seat int    `json:"seat"`
	Pool string `json:"pool"`
}

// BoardAbility is one ability going on the stack at one step: whose, whose
// card, and from where.
//
// **Transient by construction.** It rides the step rather than the card because
// using an ability is a moment rather than a state — there is nothing to clear
// afterwards and nothing that can leak. A board folded to step N reads the
// abilities of step N and no others.
//
// Zone is Forge's own name for where the source was — `Command` for eminence,
// `Battlefield` for the rest — which is what lets a room draw a commander doing
// something from a zone it never leaves (Aaron, 2026-08-26: *"It can be used on
// the battlefield or from the command zone… It should just visually indicate
// that an ability is being used"*).
type BoardAbility struct {
	ID   int    `json:"id"`
	Seat int    `json:"seat,omitempty"`
	Zone string `json:"zone,omitempty"`
	// Trigger is whether the game raised this ability rather than the player
	// activating it. Eminence is a triggered ability; a Treasure being cracked
	// is an activated one.
	Trigger bool `json:"trigger,omitempty"`
	// Targets is what the ability was aimed at, by board id — the cards a room
	// can draw an arrow to.
	//
	// **This is the half that makes eminence a picture.** Zone says a commander
	// in the command zone did something; this says which cat got bigger. Both
	// come off `StackItemView`, which had them the whole time and was only ever
	// asked whether it was a trigger.
	//
	// **Empty is the common case and not a gap**: seventeen of seventy-five
	// abilities in a measured match were aimed at anything at all. Arahbo's
	// *attack* pump picks its creature with `Defined$` rather than targeting
	// it, and a surveil trigger is aimed at nothing. A room that drew an arrow
	// per ability would invent three of every four.
	Targets []int `json:"targets,omitempty"`
}

// BoardLife is one seat's life total after it changed.
type BoardLife struct {
	Seat int `json:"seat"`
	Life int `json:"life"`
}

// BoardPlayerCounters is one seat's counters after one of them moved.
//
// **Counters go on players as well as on permanents**, and until now this reel
// carried only the permanents' — so a game decided on the tenth poison counter
// arrived as a board where nothing had happened, followed by an outcome
// sentence that explained it after the fact. Forge's bus has always said so
// (`GameEventPlayerCounters`); the scribe now listens.
//
// **The whole set crosses rather than the one that moved**, matching
// [BoardChange.Counters] and for the same reason: a reader holding a set and
// applying deltas drifts the first time one is dropped, and the question a
// scoreboard asks is "how much poison does this player have", never "how much
// arrived just then". [BoardCounter] is reused whole — a counter is a kind and
// a count wherever it is sitting.
//
// **Every kind Forge announces is carried, not only poison.** Deciding here
// which counters a room cares about would be this layer taking a view, and the
// division this package is built on puts that on the far side (ADR 14): energy
// and experience are real, they arrive through the same event, and a reel that
// dropped them would be silently lossy at the exact moment somebody wrote the
// component that wanted them.
type BoardPlayerCounters struct {
	Seat     int            `json:"seat"`
	Counters []BoardCounter `json:"counters"`
}

// BoardStep is the board's movement between one beat and the next.
//
// **Steps are parallel to [EventLog.Events], one for one**, and that is the
// whole pacing design rather than a coincidence. A room watching a match
// drains the beats at reading speed (Forge plays a game in twenty seconds and
// nobody can watch twenty seconds of Commander); with the board on the same
// index, the picture moves exactly when the sentence is spoken, from one
// clock, with nothing to keep in step. It also means the beat cap bounds both.
//
// The consequence, stated because it is real: everything Forge does between
// two beats lands on the later one. A player drawing seven cards and playing a
// land is one step. That is not a loss — it is how a game reads.
type BoardStep struct {
	// Turn is Forge's own turn number in progress, which counts each player's
	// turn separately. The wire keeps Forge's number everywhere in this
	// package; `web/src/lib/theater.ts` converts at the last moment.
	Turn    int           `json:"turn,omitempty"`
	Seat    int           `json:"seat,omitempty"`
	Life    []BoardLife   `json:"life,omitempty"`
	Changes []BoardChange `json:"changes,omitempty"`
	// Counters is every seat whose own counters moved at this step — poison
	// and its relatives. See [BoardPlayerCounters]; beside the life totals
	// because both are facts about a player rather than about a card.
	Counters []BoardPlayerCounters `json:"counters,omitempty"`
	// Floating is every seat whose mana pool moved at this step. See
	// [BoardFloating] — beside the life totals because a pool is a player's,
	// not a permanent's.
	Floating []BoardFloating `json:"floating,omitempty"`
	// Abilities is every ability that went on the stack at this step, in order.
	// See [BoardAbility] — a moment rather than a state, which is why it is
	// here and not on a card.
	Abilities []BoardAbility `json:"abilities,omitempty"`
}

// BoardReel is one game's board: who is at the table, what the cards are, and
// how they moved.
type BoardReel struct {
	// Seats is who sat where, in seat order, from Forge's own player list.
	Seats []BoardSeat `json:"seats"`
	Cards []BoardCard `json:"cards"`
	Steps []BoardStep `json:"steps"`
}

// BoardSeat is one player at the table.
//
// `Name` is what Forge was handed, which is the deck's own title — the same
// string the result line carries. A slug is put beside it further up, where
// the seat-to-deck map lives; this layer knows only what Forge said.
type BoardSeat struct {
	Seat int    `json:"seat"`
	Name string `json:"name"`
	Life int    `json:"life"`
}

// board is the assembler: the mutable state a reel is built from.
//
// Not exported, because the only sensible way to make one is to feed it a
// scribe's stream in order — a board built by hand would be a board that never
// happened.
type board struct {
	seats []BoardSeat
	cards []BoardCard
	// known is the index into cards, so a card is named once.
	known map[int]int
	// zone, seat, tapped and stats are the board as it stands.
	zone      map[int]string
	seat      map[int]int
	tapped    map[int]bool
	power     map[int]int
	toughness map[int]int
	types     map[int]string
	counters  map[int]map[string]int
	// attached is what each card is attached to, by host id. Absent and zero
	// are the same thing here — nothing — because a board only ever asks the
	// question about a card it is already drawing.
	attached map[int]int
	// combat, attacking and blocking are the fight as it stands: what each
	// creature is doing, the seat an attacker is attacking, and the board id of
	// the attacker a blocker stopped.
	combat    map[int]string
	attacking map[int]int
	blocking  map[int]int
	// fighting is who is in the combat, in the order they joined it.
	//
	// A slice beside the map for `pending`'s reason: combat ends for everybody
	// at once, and clearing it by walking a map would order the changes
	// randomly and make a recorded golden flap between runs.
	fighting []int
	// casts is how many times each card has left the command zone.
	casts map[int]int
	life  map[int]int
	// held is each seat's own counters — poison, energy, experience — by kind.
	// A seat holding none holds no map, the way `counters` treats a card.
	held map[int]map[string]int
	// left is the last real zone a card was cleared out of.
	//
	// Needed because **Forge announces the leaving before the arriving**: a
	// creature dying raises `Battlefield out` and then `Graveyard in`, so by
	// the time the graveyard arrives the card's current zone is already
	// [ZoneGone] and "did this come from the battlefield?" has no answer left.
	// Without this the `dies` beat never fired at all — eighteen cards reached
	// graveyards in the recorded match and the account never said one of them
	// was destroyed.
	left map[int]string

	// live is the keyword set each card instance currently has, joined as it
	// arrived. Compared as one string rather than as a set because the scribe
	// renders Forge's own order and a re-send of the same set is the common
	// case; the split happens only when it has actually moved.
	live map[int]string
	// pool is each seat's floating mana as it stands.
	pool map[int]string
	// sawCombatEnd is whether Forge has told this game when combat ended.
	//
	// **One rule with a stated precedence, rather than two that can disagree.**
	// Combat ends when Forge says it ends; the turn boundary stands in only
	// while this is false, which is what a stream from a worker built before
	// `GameEventCombatEnded` was listened for looks like. Latched per game
	// because the answer cannot go backwards inside one — a stream that has said
	// it once will say it every combat.
	sawCombatEnd bool

	// pending is what has changed since the last beat, in the order the cards
	// were first touched — deterministic, because a map's iteration order is
	// not and a recorded golden would flap.
	pending   []int
	changing  map[int]*BoardChange
	lifeMoved []int
	// heldMoved is the seats whose own counters moved since the last beat, in
	// the order they were touched — `lifeMoved`'s reason exactly: a slice
	// rather than a map so a recorded golden cannot flap between runs.
	heldMoved []int
	// poolMoved is every value a pool took since the last beat, in order. A
	// seat appears once per change rather than once per step — see
	// [board.floating] for why the sequence is the point.
	poolMoved []BoardFloating
	// used is the abilities raised since the last beat, in order.
	used []BoardAbility

	turn int
	// active is whose turn it is. Kept apart from the turn number because a
	// step raised outside anybody's turn (the opening draw) still belongs to
	// the game.
	active int
	steps  []BoardStep
}

func newBoard() *board {
	return &board{
		known: map[int]int{}, zone: map[int]string{}, seat: map[int]int{},
		tapped: map[int]bool{}, power: map[int]int{}, toughness: map[int]int{},
		types: map[int]string{}, counters: map[int]map[string]int{},
		attached: map[int]int{},
		combat:   map[int]string{}, attacking: map[int]int{},
		blocking: map[int]int{}, casts: map[int]int{},
		life: map[int]int{}, held: map[int]map[string]int{},
		left: map[int]string{},
		live: map[int]string{}, pool: map[int]string{},
		changing: map[int]*BoardChange{},
	}
}

// sit records a seat from the scribe's roster line.
func (b *board) sit(seat int, name string, life int) {
	if seat <= 0 {
		return
	}
	for i := range b.seats {
		if b.seats[i].Seat == seat {
			b.seats[i].Name, b.seats[i].Life = name, life
			return
		}
	}
	b.seats = append(b.seats, BoardSeat{Seat: seat, Name: name, Life: life})
	b.life[seat] = life
}

// name records a card the first time it is seen, and keeps its type line
// current after that — a type line changes when a land animates or a creature
// becomes an artifact, and the land row is read off it.
func (b *board) name(id int, card, types string, token bool, seat int) {
	if id == 0 || card == "" {
		return
	}
	if at, seen := b.known[id]; seen {
		if types != "" {
			b.cards[at].Types = types
		}
		return
	}
	b.known[id] = len(b.cards)
	b.cards = append(b.cards, BoardCard{ID: id, Name: card, Token: token,
		Types: types, Seat: seat})
	b.types[id] = types
}

// nameOf is what this board calls a card, or the empty string for one it has
// never been told about.
//
// The dictionary's answer rather than the line's, for [board.isToken]'s reason
// and one more: a line can arrive carrying *no* name for a card this board has
// been drawing all game. See [ScribeParser.named], which is the only caller
// and holds the measurement.
func (b *board) nameOf(id int) string {
	at, seen := b.known[id]
	if !seen {
		return ""
	}
	return b.cards[at].Name
}

// seatOf is whose card this is, or zero for one this board has never placed.
//
// Kept current by [board.moved] — a permanent that changes controller changes
// seats here — so it is the controller now rather than whoever cast it. See
// [ScribeParser.seated].
func (b *board) seatOf(id int) int { return b.seat[id] }

// isToken is whether a card is a token, asked of the dictionary rather than of
// the line — a `stats` or `counters` line carries the flag too, but a `zone`
// line for a token Forge has already named does not have to, and the answer
// must be the same either way.
func (b *board) isToken(id int) bool {
	at, seen := b.known[id]
	return seen && b.cards[at].Token
}

// change is the pending change for a card, made on first touch this step.
//
// **Only for a card the dictionary holds**, which is ADR 44's rule read the
// strict way: the board may only say things about objects it has named. A
// change against an id with no entry in `b.cards` is a change nothing can
// render — the browser folds it into a card whose name is the empty string,
// and an empty name is not a gap a reader can see. It is a blank card tucked
// under a creature, and it was worse than blank before `stackRow` learned to
// count attachments: an empty name joins to the empty string, which is
// byte-for-byte the key of a card carrying nothing at all, so an equipped
// token merged into the unequipped pile beside it and the pile drew one sword
// above a count of two.
//
// [board.keywords] has always said this for itself, in as many words, and
// nothing else said it for anybody — so an `attach` line naming an id the
// dictionary had refused put a nameless card on somebody's battlefield. Said
// here now, once, for every fold: the dictionary and the changes cannot
// disagree about which cards exist if only one of them decides.
//
// The refused change goes into a scratch [BoardChange] rather than a nil, so
// that every caller keeps writing the same three lines and none of them has to
// ask first. Nothing reads it, and it never joins `pending`.
func (b *board) change(id int) *BoardChange {
	if _, drawn := b.known[id]; !drawn {
		return &BoardChange{ID: id}
	}
	if c, ok := b.changing[id]; ok {
		return c
	}
	c := &BoardChange{ID: id}
	b.changing[id] = c
	b.pending = append(b.pending, id)
	return c
}

// drawnZone is where a Forge zone lands on a drawn board, and whether it lands
// there at all.
//
// The two `false`s are rulings 1 and 2 at the top of this file: the stack is
// dropped because its events never balance, and the library is dropped because
// showing it is showing the answers. `Library` returns [ZoneGone] rather than
// nothing, because a card shuffled back has to *leave* the board somebody is
// looking at.
func drawnZone(forge, types string) (string, bool) {
	switch forge {
	case "Battlefield":
		if strings.Contains(types, "Land") {
			return ZoneLand, true
		}
		return ZoneBattlefield, true
	case "Hand":
		return ZoneHand, true
	case "Graveyard":
		return ZoneGraveyard, true
	case "Exile":
		return ZoneExile, true
	case "Command":
		return ZoneCommand, true
	case "Library":
		return ZoneGone, true
	default:
		// Stack, Sideboard, Ante, Merged, and whatever a future Forge adds.
		return "", false
	}
}

// magicZone is a drawn zone as Magic has it.
//
// [ZoneLand] is this package's own furniture — the row a player puts their
// lands in — and not a zone of the game, so a Dryad Arbor animating and a
// creature-land going back to being a land have not changed zones at all.
// Everything that turns on a zone *change* asks this rather than comparing the
// drawn names, or an animated land would shed its counters every time it woke
// up.
func magicZone(drawn string) string {
	if drawn == ZoneLand {
		return ZoneBattlefield
	}
	return drawn
}

// moved folds one zone event.
//
// `mode` is the scribe's, "in" or "out". An `out` clears only the zone the
// card is currently recorded in — ruling 4 — so a late `out` for a zone
// already left cannot blank a card that has arrived somewhere else.
func (b *board) moved(id int, forgeZone, mode string, seat int) {
	zone, drawn := drawnZone(forgeZone, b.types[id])
	if !drawn {
		return
	}
	if mode == "out" {
		if b.zone[id] != zone {
			return
		}
		if b.zone[id] == ZoneGone {
			return
		}
		b.left[id] = zone
		b.zone[id] = ZoneGone
		b.change(id).Zone = ZoneGone
		b.became(id, zone, ZoneGone)
		return
	}
	// **A token that leaves the battlefield ceases to exist.** Rule 111.7 and
	// rule 704.5d, together: a token that would change zones does move — a
	// dying token really is put into its owner's graveyard, which is why its
	// death triggers and why the `dies` beat above is right to fire — and then
	// a state-based action removes it from the game. It cannot move again.
	//
	// So it is never *in* a graveyard by the time anybody looks, and a board
	// that piles tokens up in one is drawing a zone Magic does not have (Aaron,
	// 2026-08-25: "they don't go to the graveyard, they go to the ether"). A
	// deck like Trostani's ends a long game with thirty Saprolings in a
	// graveyard that should hold none of them, burying the real cards under
	// them.
	//
	// **Here rather than in the browser**, and only on the arrival: the beat is
	// raised in `scribe.go` from the line, not from this, so silencing the zone
	// costs nothing the account says. The `out` above is untouched because the
	// zone being *left* is a real one — the battlefield — and rewriting it
	// would break the match that clears it.
	if b.isToken(id) && zone != ZoneBattlefield && zone != ZoneLand {
		zone = ZoneGone
	}
	if b.zone[id] == zone && (seat == 0 || b.seat[id] == seat) {
		return
	}
	was := b.zone[id]
	b.zone[id] = zone
	c := b.change(id)
	c.Zone = zone
	if seat != 0 {
		b.seat[id] = seat
		c.Seat = seat
	}
	b.became(id, was, zone)
	// A card arriving somewhere is a card nobody has tapped yet. Forge agrees
	// — it raises no untap event for a permanent that leaves the battlefield —
	// and without this a creature that dies tapped comes back tapped.
	if b.tapped[id] {
		b.tapped[id] = false
		no := false
		c.Tapped = &no
	}
}

// became folds what a zone change means beyond the zone itself — ruling 5.
//
// **A permanent that changes zones is a new object** (rule 400.7), so three
// things are true of it the instant it lands and none of them is announced:
// it has no counters, it is not in combat, and — if it came from a command
// zone — somebody paid for it.
//
// Asked in terms of [magicZone] rather than the drawn one, so that a
// creature-land waking up keeps everything it had. A card arriving for the
// first time has no previous zone at all, which differs from every real one
// and costs nothing: there is nothing on a card nobody has seen yet.
func (b *board) became(id int, was, now string) {
	if magicZone(was) == magicZone(now) {
		return
	}
	b.shed(id)
	b.inCombat(id, "", 0, 0)
	// **Leaving the command zone is the cast**, and it is the only signal
	// there is: Forge reports no tax, and a commander that gets countered goes
	// command → stack → graveyard, so narrowing this to "arrived on the
	// battlefield" would stop charging for exactly the casts that failed. What
	// it over-counts is a commander *moved* out of the zone without being cast,
	// which Forge's AI does not do.
	if magicZone(was) == ZoneCommand {
		b.casts[id]++
		total := b.casts[id]
		b.change(id).Casts = &total
	}
}

// shed drops every counter a card is carrying, and says so out loud.
//
// Silent when there is nothing to shed, which is almost every card almost
// every time: a step that announced "this land still has no counters" for
// every land that moved would be most of the payload.
func (b *board) shed(id int) {
	if len(b.counters[id]) == 0 {
		return
	}
	delete(b.counters, id)
	none := []BoardCounter{}
	b.change(id).Counters = &none
}

// inCombat records what a creature is doing in the fight happening now.
//
// `attacking` is the seat under attack and `blocking` is the board id of the
// attacker stopped; each is zero for the other role and both are zero for a
// creature standing out of combat. Every field is emitted only when it moved,
// for `attach`'s reason: Forge re-announces a combat it has already announced
// — the recorded match raises the same attacker twice — and a change published
// for a fact that did not move is a step saying something happened when
// nothing did.
func (b *board) inCombat(id int, role string, attacking, blocking int) {
	if id == 0 {
		return
	}
	if b.combat[id] != role {
		if b.combat[id] == "" {
			b.fighting = append(b.fighting, id)
		}
		if role == "" {
			delete(b.combat, id)
		} else {
			b.combat[id] = role
		}
		value := role
		b.change(id).Combat = &value
	}
	if b.attacking[id] != attacking {
		b.attacking[id] = attacking
		value := attacking
		b.change(id).Attacking = &value
	}
	if b.blocking[id] != blocking {
		b.blocking[id] = blocking
		value := blocking
		b.change(id).Blocking = &value
	}
}

// combatEnded is Forge saying combat is over, which is the real boundary.
//
// **`GameEventCombatUpdate` is the wrong event, and ADR 44 named it as the
// right one.** That ADR left this undone with the note that the bus carries a
// combat signal the scribe does not listen for; it does, and it is not that
// one. `GameEventCombatUpdate` is constructed in exactly two places in the
// whole of Forge — `InputAttack` and `InputBlock`, the *human* declare-attackers
// and declare-blockers handlers — so it fires on a person's clicks and never
// once in a headless AI match. A listener built on it would have compiled,
// subscribed, and changed nothing. `GameEventCombatEnded` is the engine's own,
// raised from `PhaseHandler.onPhaseEnd()`.
//
// The latch is what keeps this one rule rather than two. See
// [board.sawCombatEnd].
func (b *board) combatEnded() {
	b.sawCombatEnd = true
	b.endCombat()
}

// endCombat takes everybody out of the fight.
//
// **The turn boundary stands in only until Forge says otherwise.** Before the
// scribe listened for `GameEventCombatEnded` this was the only boundary the
// stream had, and it was a phase late: a creature kept its sword mark through a
// second main phase it was no longer attacking in, and two combats in one turn
// piled the first one's attackers in with the second's. Both are now answered
// where the answer comes from Forge — but a worker image built before the
// scribe learned that event still sends no `combat_end` at all, and a board
// that dropped the fallback would leave those matches marked as attacking
// forever. So the old rule survives exactly as long as it is the only one
// there is: [board.began] asks this, and [board.sawCombatEnd] silences it the
// moment Forge speaks for itself.
//
// A creature that leaves the battlefield is taken out of combat immediately,
// by [board.became], on either path.
func (b *board) endCombat() {
	for _, id := range b.fighting {
		if b.combat[id] == "" {
			continue
		}
		b.inCombat(id, "", 0, 0)
	}
	b.fighting = b.fighting[:0]
}

// attach folds an attachment, and a `host` of zero folds it coming off.
// Reports whether anything actually moved.
//
// Idempotent on purpose, and **the answer is what the beat is raised on**.
// Forge re-announces an attachment when a permanent changes controller or
// re-enters, and a change raised for a fact that has not moved is a step
// saying something happened when nothing did — the same noise `Scribe.seen`
// exists to keep out of the stream, applied to the one event that can arrive
// twice for one state. Swallowing the change and narrating it anyway would
// have been worse than doing neither: a room saying "the sword goes on the
// bear" twice, with the board perfectly still both times.
func (b *board) attach(id, host int) bool {
	if id == 0 || b.attached[id] == host {
		return false
	}
	b.attached[id] = host
	to := host
	b.change(id).AttachedTo = &to
	return true
}

// tap folds a tapped event. `tapped` is false when the scribe omitted the
// field, which is its encoding for it (the JSON writer drops a false).
func (b *board) tap(id int, tapped bool) {
	if b.tapped[id] == tapped {
		return
	}
	b.tapped[id] = tapped
	value := tapped
	b.change(id).Tapped = &value
}

// stats folds a power/toughness/type change.
func (b *board) stats(id, power, toughness int, types string) {
	c := (*BoardChange)(nil)
	if p, seen := b.power[id]; !seen || p != power {
		b.power[id] = power
		c = b.change(id)
		value := power
		c.Power = &value
	}
	if t, seen := b.toughness[id]; !seen || t != toughness {
		b.toughness[id] = toughness
		if c == nil {
			c = b.change(id)
		}
		value := toughness
		c.Toughness = &value
	}
	if types != "" && b.types[id] != types {
		b.types[id] = types
		if c == nil {
			c = b.change(id)
		}
		c.Types = types
		// A type line changing can move a card between the battlefield and the
		// land row — an animated Forest is a creature that is still a land, and
		// a Dryad Arbor that stops being one goes the other way.
		if zone := b.zone[id]; zone == ZoneBattlefield || zone == ZoneLand {
			if drawn, ok := drawnZone("Battlefield", types); ok && drawn != zone {
				b.zone[id] = drawn
				c.Zone = drawn
			}
		}
	}
}

// counter folds a counter event. The whole set for that card crosses whenever
// any of it changes, because a browser holding a partial set has no way to
// know a kind went to zero — and the move that changed it crosses beside the
// set, because a set of three tells nobody how it got to three.
//
// `was` is Forge's own previous total, carried through rather than recomputed.
func (b *board) counter(id int, kind string, was, now int) {
	if kind == "" {
		return
	}
	on := b.counters[id]
	if on == nil {
		on = map[string]int{}
		b.counters[id] = on
	}
	if on[kind] == now {
		return
	}
	if now <= 0 {
		delete(on, kind)
	} else {
		on[kind] = now
	}
	c := b.change(id)
	c.CounterMoves = append(c.CounterMoves,
		BoardCounterMove{Kind: kind, Was: was, Now: now})
	set := sortedCounters(on)
	c.Counters = &set
	// A card holding nothing holds no map either. Housekeeping rather than
	// behaviour — `shed` reads the length and an empty map is a card the board
	// would otherwise remember forever.
	if len(on) == 0 {
		delete(b.counters, id)
	}
}

// keywords folds the live keyword set for one card instance.
//
// `joined` is the scribe's comma-joined string and the empty string is a real
// answer — a creature that has lost the last keyword something gave it — which
// is why an unseen card and a card known to have none are told apart by the
// map rather than by the value. Without that, the first card ever reported with
// no keywords would publish an empty set that nobody needed and every card that
// *lost* one would publish nothing.
//
// Compared as the whole string, because the scribe re-sends a card's keywords
// on every line that mentions it and almost none of them are news — the same
// trade `Scribe.seen` makes one layer earlier, for the same reason.
func (b *board) keywords(id int, joined string) {
	// Only for a card this board is already drawing. Every line the scribe
	// writes about a card carries its keywords, including lines about cards
	// that never enter a drawn zone, and a change against an id the dictionary
	// has no entry for is a change nothing can render.
	if _, drawn := b.known[id]; !drawn {
		return
	}
	was, seen := b.live[id]
	if seen && was == joined {
		return
	}
	b.live[id] = joined
	// **A card that has never had a keyword says nothing**, which is `shed`'s
	// rule applied to the other set a card carries. Most permanents in most
	// games have no keywords at all, so publishing "this Forest still has none"
	// the first time each land is mentioned was thirty-odd changes a game
	// carrying no information — measured on a real match before this guard
	// existed. Going *back* to none is still news and still published, because
	// that is a creature losing something it had.
	if !seen && joined == "" {
		return
	}
	set := []string{}
	if joined != "" {
		set = strings.Split(joined, ",")
	}
	b.change(id).Live = &set
}

// fate folds how a permanent left. Silent on the empty string, so that a card
// leaving for a reason Forge did not name says nothing rather than saying it
// left for no reason.
func (b *board) fate(id int, fate string) {
	if id == 0 || fate == "" {
		return
	}
	b.change(id).Fate = fate
}

// floating folds one seat's mana pool.
//
// **Every value it takes, in order, and not just the one it ends on.** This is
// the one place in this file where a step carries a *sequence* rather than a
// state, and it is the difference between answering Aaron's question and
// missing it entirely. He asked to see the pool *"as things tap into it before
// it is drained to cast things"* — and a pool fills and empties several times
// between two beats, so the value at the end of a step is almost always zero.
// Measured on a real match before this was a sequence: ten pool changes reached
// the browser and **nine of them were an empty pool**, which is a truthful
// answer to a question nobody asked.
//
// So the room gets the whole movement. A consumer that only wants the resting
// state reads the last entry for that seat, which is what a fold does anyway;
// one that wants to draw the mana arriving and being spent has the frames to do
// it with. Every entry is a state Forge announced — nothing here is
// interpolated.
//
// **A pool that has never held anything says nothing**, the same rule
// [board.keywords] follows: a seat's first event is often the drain at the end
// of its first step, and "this empty pool is still empty" is not news.
func (b *board) floating(seat int, mana string) {
	if seat <= 0 {
		return
	}
	was, seen := b.pool[seat]
	if seen && was == mana {
		return
	}
	b.pool[seat] = mana
	if !seen && mana == "" {
		return
	}
	b.poolMoved = append(b.poolMoved, BoardFloating{Seat: seat, Pool: mana})
}

// usedAbility records an ability going on the stack at this step.
//
// Appended rather than deduplicated: a card really can use the same ability
// twice before the next beat, and two uses are two things that happened.
//
// `targets` is the scribe's comma-joined id list; see [BoardAbility.Targets]
// for why most abilities have none. Parsed here rather than at the call site so
// the wire's one spelling of a list is decoded in one place — [board.keywords]
// is the other, and they split the same way for the same reason.
func (b *board) usedAbility(id, seat int, zone string, trigger bool, targets string) {
	if id == 0 {
		return
	}
	b.used = append(b.used, BoardAbility{ID: id, Seat: seat, Zone: zone,
		Trigger: trigger, Targets: boardIDs(targets)})
}

// boardIDs reads the scribe's comma-joined list of board ids.
//
// **A number this cannot read is dropped, never folded as zero.** Zero is
// [BoardCard.ID]'s own "no card", so a malformed entry parsed leniently would
// point a room's arrow at nothing and look like a card that had gone missing.
// Returns nil rather than an empty slice, so the field stays off the wire.
func boardIDs(joined string) []int {
	if joined == "" {
		return nil
	}
	var ids []int
	for _, part := range strings.Split(joined, ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// copiedBy records that a card was made as a copy, by `by`.
//
// On the dictionary rather than in a change, because it is true from the
// instant the card exists and never changes after — see [BoardCard.CopiedBy].
// Silent for a card this board has not been told the name of yet; the scribe
// sends the two together on every line, so the next one settles it.
func (b *board) copiedBy(id, by int) {
	if id == 0 || by == 0 {
		return
	}
	at, seen := b.known[id]
	if !seen || b.cards[at].CopiedBy == by {
		return
	}
	b.cards[at].CopiedBy = by
}

// lives folds a life total.
func (b *board) lives(seat, life int) {
	if seat <= 0 || b.life[seat] == life {
		return
	}
	b.life[seat] = life
	for _, already := range b.lifeMoved {
		if already == seat {
			return
		}
	}
	b.lifeMoved = append(b.lifeMoved, seat)
}

// playerCounter folds one of a player's own counters. `now` is the new total.
//
// **The empty kind is a third case and not a missing field**: it is Forge
// clearing every counter this player has at once, which `Scribe.java` argues
// where it is decoded. Read as a set that is now empty rather than as a line to
// drop, because dropping it would leave a dead player poisoned forever on a
// board that had been told otherwise.
//
// Silent when nothing actually changed, for `lives`' reason — Forge announces
// a great deal that is not news, and a step recording a counter that stayed
// where it was is a beat where the scoreboard flinches for nothing.
func (b *board) playerCounter(seat int, kind string, now int) {
	if seat <= 0 {
		return
	}
	on := b.held[seat]
	if kind == "" {
		if len(on) == 0 {
			return
		}
		delete(b.held, seat)
		b.heldChanged(seat)
		return
	}
	if on[kind] == now {
		return
	}
	if now <= 0 {
		delete(on, kind)
		if len(on) == 0 {
			delete(b.held, seat)
		}
	} else {
		if on == nil {
			on = map[string]int{}
			b.held[seat] = on
		}
		on[kind] = now
	}
	b.heldChanged(seat)
}

// heldChanged marks a seat's counters as news for the beat being assembled.
func (b *board) heldChanged(seat int) {
	for _, already := range b.heldMoved {
		if already == seat {
			return
		}
	}
	b.heldMoved = append(b.heldMoved, seat)
}

// began records a turn, and ends the last one's combat if nothing better has.
//
// The fallback lives here rather than at the call site so that the precedence
// between the two boundaries is stated in one place — see [board.endCombat].
func (b *board) began(turn, seat int) {
	if !b.sawCombatEnd {
		b.endCombat()
	}
	if turn > 0 {
		b.turn = turn
	}
	if seat > 0 {
		b.active = seat
	}
}

// beat closes a step: everything that has changed since the last one, in the
// order it was touched.
//
// Called once per beat, which is what makes [BoardStep] and [GameEvent]
// parallel arrays — see [BoardStep]'s own comment for why that is the design
// rather than an accident of the loop.
func (b *board) beat() {
	step := BoardStep{Turn: b.turn, Seat: b.active}
	for _, seat := range b.lifeMoved {
		step.Life = append(step.Life, BoardLife{Seat: seat, Life: b.life[seat]})
	}
	for _, seat := range b.heldMoved {
		// A seat that has just lost its last counter publishes an empty set,
		// not nothing: the far side is holding the old one and has to be told
		// to put it down.
		step.Counters = append(step.Counters, BoardPlayerCounters{
			Seat: seat, Counters: sortedCounters(b.held[seat])})
	}
	if len(b.poolMoved) > 0 {
		step.Floating = append([]BoardFloating(nil), b.poolMoved...)
	}
	if len(b.used) > 0 {
		step.Abilities = append([]BoardAbility(nil), b.used...)
	}
	for _, id := range b.pending {
		step.Changes = append(step.Changes, *b.changing[id])
	}
	b.steps = append(b.steps, step)
	b.pending = b.pending[:0]
	b.lifeMoved = b.lifeMoved[:0]
	b.heldMoved = b.heldMoved[:0]
	b.poolMoved = b.poolMoved[:0]
	b.used = b.used[:0]
	b.changing = map[int]*BoardChange{}
}

// reel is the finished board.
func (b *board) reel() *BoardReel {
	if len(b.steps) == 0 && len(b.cards) == 0 {
		return nil
	}
	seats := b.seats
	if seats == nil {
		seats = []BoardSeat{}
	}
	cards := b.cards
	if cards == nil {
		cards = []BoardCard{}
	}
	steps := b.steps
	if steps == nil {
		steps = []BoardStep{}
	}
	return &BoardReel{Seats: seats, Cards: cards, Steps: steps}
}

// sortedCounters renders a counter set in a stable order.
//
// Insertion order would be the map's, which Go randomises — a recorded payload
// would flap between runs and a golden could never be written. Sorted by kind,
// which is also what a person reading a card wants: the same counters in the
// same places every time they look.
func sortedCounters(on map[string]int) []BoardCounter {
	kinds := make([]string, 0, len(on))
	for kind := range on {
		kinds = append(kinds, kind)
	}
	for i := 1; i < len(kinds); i++ {
		for j := i; j > 0 && kinds[j] < kinds[j-1]; j-- {
			kinds[j], kinds[j-1] = kinds[j-1], kinds[j]
		}
	}
	out := make([]BoardCounter, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, BoardCounter{Kind: kind, N: on[kind]})
	}
	return out
}
