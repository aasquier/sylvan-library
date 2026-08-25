package tier3

import "strings"

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
//  3. **`Commander Effect` is not a card.** Forge puts a phantom in each
//     command zone (ids 101 and 202 in the recorded match) with a real name,
//     a real id and an *empty type line*. It is the commander-tax bookkeeping,
//     it is invisible in any real game, and drawing it would put a blank card
//     beside every commander.
//  4. **`out` only clears a zone the card is actually in.** Forge does not
//     promise that a card's `out` of its old zone precedes its `in` to the
//     new one, and an unconditional clear on a late `out` blanks a card that
//     has already arrived somewhere. Measured on the same match: the ordering
//     holds for hand-to-battlefield and does not for every path.
//
// A fifth thing is a choice rather than a finding: **lands sit in their own
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
}

// BoardCounter is one kind of counter and how many are on the card.
type BoardCounter struct {
	Kind string `json:"kind"`
	N    int    `json:"n"`
}

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
	Seat      int            `json:"seat,omitempty"`
	Tapped    *bool          `json:"tapped,omitempty"`
	Power     *int           `json:"power,omitempty"`
	Toughness *int           `json:"toughness,omitempty"`
	Types     string         `json:"types,omitempty"`
	Counters  []BoardCounter `json:"counters,omitempty"`
}

// BoardLife is one seat's life total after it changed.
type BoardLife struct {
	Seat int `json:"seat"`
	Life int `json:"life"`
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
	life      map[int]int
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

	// pending is what has changed since the last beat, in the order the cards
	// were first touched — deterministic, because a map's iteration order is
	// not and a recorded golden would flap.
	pending   []int
	changing  map[int]*BoardChange
	lifeMoved []int

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
		life: map[int]int{}, left: map[int]string{},
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

// change is the pending change for a card, made on first touch this step.
func (b *board) change(id int) *BoardChange {
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
		return
	}
	if b.zone[id] == zone && (seat == 0 || b.seat[id] == seat) {
		return
	}
	b.zone[id] = zone
	c := b.change(id)
	c.Zone = zone
	if seat != 0 {
		b.seat[id] = seat
		c.Seat = seat
	}
	// A card arriving somewhere is a card nobody has tapped yet. Forge agrees
	// — it raises no untap event for a permanent that leaves the battlefield —
	// and without this a creature that dies tapped comes back tapped.
	if b.tapped[id] {
		b.tapped[id] = false
		no := false
		c.Tapped = &no
	}
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
// know a kind went to zero.
func (b *board) counter(id int, kind string, now int) {
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
	b.change(id).Counters = sortedCounters(on)
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

// began records a turn.
func (b *board) began(turn, seat int) {
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
	for _, id := range b.pending {
		step.Changes = append(step.Changes, *b.changing[id])
	}
	b.steps = append(b.steps, step)
	b.pending = b.pending[:0]
	b.lifeMoved = b.lifeMoved[:0]
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
