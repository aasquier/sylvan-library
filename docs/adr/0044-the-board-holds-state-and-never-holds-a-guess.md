# 44. The board holds the state the events do not, and never holds a guess

**Status:** Accepted · **Decided:** 2026-08-26 · Applies
[ADR 14](0014-python-decides-claude-advises.md)'s division to the Coliseum's
board, and answers the gap
[ADR 42](0042-a-scribe-rides-forges-event-bus.md) recorded in its own
Consequences.

## Context

ADR 42 put a listener on Forge's event bus and said the thing that matters
here: **the scribe reports events and never state.** `go/internal/sim/tier3`'s
board is the far side of that division — the place where "a card entered the
graveyard" becomes "the graveyard has this card in it".

Four questions arrived at once from a real match, and they are all the same
question wearing different clothes: *what does the board know that no event
says?*

- **Counters followed cards out of play.** Aaron: *"counters are following
  things into exile, the graveyard, and the command zone, they fall off a
  creature when they move to any of those zones."* He is describing rule
  400.7. Forge raises `GameEventCardCounters` when a counter is put on or
  taken off, and raises nothing at all when the object carrying them stops
  existing — because from Forge's side nothing was removed. So a creature that
  died with two +1/+1 counters lay in the graveyard still wearing them.
- **The board could not draw a fight.** `attack` and `block` arrive as beats,
  so the account said who was swinging and the picture never did. A row of
  tokens looked identical whether it was attacking, blocking or asleep — and
  Aaron wants attacking and blocking token piles told apart, which is not a
  drawing problem at all until the data exists.
- **Commander tax was counted in the browser**, by watching cards leave the
  command zone. Forge reports no tax and `CardView.isCommander()` is
  deliberately not on the wire, so counting is the only answer available. The
  question was never *whether* to count; it was *where*.
- **A hover wanted the history of a card's counters.** Aaron: *"could we keep
  a history of why a creature has all of the counters it does and show it as
  help text on a hover."* Forge sends both totals on every counter event —
  `was` and `now` — and this reader dropped the first one. It sends nothing
  about the source.

## Options considered

**Let the browser derive it.** The status quo for the tax, and the cheapest
thing for the rest: `web/src/lib/board.ts` already walks every change in order
and could notice a zone transition as easily as Go can. Rejected because that
file's own contract is that **it decides no Magic** — it folds deltas and
picks which row a permanent stands in, and everything else is answered before
it arrives. "A card that changes zones is a new object" is a rule of the game,
not a layout convention, and putting it in a browser gives it a second home to
drift in. The tax is the proof: it was derived there, and the derivation had
to be argued in four separate comments in the file that is not supposed to
argue about Magic.

**Ask Forge for the state instead of deriving it.** `CardView` carries most of
this — `isCommander()`, the counter map, combat assignments — and the scribe
could send it. Rejected for ADR 42's own reason, unchanged: the scribe reports
events, the pipe stays small, and a per-card state dump on every step is the
payload shape the delta stream exists to avoid. It also puts every future
question behind a new worker image.

**Attribute the counters.** The obviously desirable version of the history:
*Hazel's Brewmaster gave this creature two +1/+1 counters on turn four.* The
only way to build it from what Forge sends is to blame whatever was cast or
resolved most recently, because `GameEventCardCounters` carries the card the
counters landed on and nothing about the source. **Rejected outright.** That
is inference wearing a fact's clothes, and this project's line is that
deterministic code says only what it knows and says which system answered. A
board that guesses wrong once about a card a player is looking straight at has
spent everything the rest of the picture earned.

**Name the command zone's shape in the browser.** The zone has exactly three
legal shapes — one commander, two partners, or either of those plus a
companion — and the browser receives a list of ids. Rejected: `internal/gate`
has already refused a deck whose pairing is not legal and whose companion is
not one, so `deck.yaml`'s own declaration is a *validated* answer and working
it back out of the cards would be a third implementation of the partner rules.

## Decision

**Every reading of the game happens in Go, and the wire carries the answer.**

1. **A permanent that changes zones is a new object** (rule 400.7). It sheds
   its counters and leaves combat, on the zone change, in `board.became`. The
   [land row](0042-a-scribe-rides-forges-event-bus.md) is this package's own
   furniture rather than a zone of Magic's, so the question is asked of the
   *Magic* zone — an animated Dryad Arbor changing rows has changed nothing
   and keeps everything.
2. **An empty set is a value, not an absence.** `BoardChange.Counters` became
   a pointer so that "this card has none" can be said out loud; under a plain
   slice it was the same bytes as "nothing changed", and the last counter
   coming off a card never reached the browser at all. The board's one
   deliberate lateness stands: a creature held on the sand for the beat that
   says it died keeps the counters it died with, because that is the instant
   somebody is looking at it.
3. **Combat is board state.** A creature carries `combat`, the seat it is
   `attacking`, and the board id of the attacker it is `blocking` — the id
   rather than the name, because two Egg Tokens are one name between them.
   Combat ends when a turn begins, which is the only boundary the stream has;
   `board.endCombat` states what that costs and why it is the right trade.
4. **The tax is counted where the game is read.** `BoardChange.Casts` is the
   number of times a card has left the command zone. Every card is counted,
   because this layer does not know which cards a deck named; `forgeBoardSeat`
   one layer up does.
5. **The command zone declares its shape** — `commander` or `partners`, with
   the companion beside it rather than inside it — read off `deck.yaml` and
   never worked out from the cards.
6. **The counter history says what happened and never who did it.**
   `BoardCounterMove` carries the kind and both totals, per step, and the
   browser accumulates it. There is no field for a source and there will not
   be one until Forge carries one.

## Consequences

**A hover can say "two +1/+1 counters on turn four, one more on turn six" and
cannot say what put them there.** That gap is permanent until
`GameEventCardCounters` grows a source, and it should read as a gap rather
than be papered over: the copy that renders this must not imply an agent.

**The history is a delta, and that is what makes it affordable.** The board is
folded from step zero on every render — about a hundred and forty times a game
— so a running history carried on the wire would be re-sent and re-walked
every time. One entry per event is the volume the stream already has. The
browser folds repeats on the same turn into one moment, which is both what
bounds it and what a person would say.

**Forge's bookkeeping cards are refused by name shape now, not only by zone.**
The old rule was an *and* — untyped and in the command zone — and a real board
got past it with `Olinda the Oblivious (99)'s Effect`. `Card.toString()`
renders a host card as `Name (id)`, and no Magic card name has ever held a
parenthesis, so that shape is safe to drop wherever it appears. A phantom no
longer reaches the dictionary either, which it used to do even when nothing
ever moved it.

**Two things Forge's bus has and the scribe does not listen for stay
undone**: `GameEventManaPool`, for showing the pool as permanents tap into it,
and `GameEventCardSacrificed`, for marking a Treasure tapped and then
sacrificed. Both need Java, both are additive to this shape, and neither is
blocked by anything decided here.
