/**
 * The battlefield, folded.
 *
 * The server sends a board as a **dictionary and a list of deltas** — every
 * card named once, then a few hundred small changes referring to them by
 * Forge's per-game instance id. That shape is what keeps a whole game of
 * Commander inside about twenty-five kilobytes; the cost is that nothing is
 * drawable until the deltas up to a moment have been applied. This applies
 * them.
 *
 * **It decides no Magic.** Every judgement about the *game* was made in Go,
 * against a recorded match, and is argued in
 * `go/internal/sim/tier3/board.go`: which zone a card is in, what happens when
 * a card leaves a zone it had already left, that the stack is not a zone at
 * all, that the library is never sent, and that a token leaving the
 * battlefield ceases to exist rather than piling up in a graveyard.
 *
 * What is decided here is **furniture**: given the zone, which *row* of the
 * battlefield a permanent stands in (`rowFor`). That is a layout convention
 * rather than a rule — it is where a player puts their own cards, and it
 * changes when Aaron says it should. The card facts it needs are still Go's:
 * `types` and `mana` arrive answered, so nothing in this file reads rules text
 * or decides what a card does. A browser that had to know any Magic to draw
 * this would be a second place for those rulings to drift.
 *
 * Folded from the start on every call rather than kept incrementally. A game
 * is about a hundred and forty steps holding three hundred changes between
 * them, so the whole fold is a few hundred assignments — cheaper than the
 * bookkeeping needed to avoid it, and it cannot get out of step with the
 * count it was asked for.
 */

import type { ForgeBoard, ForgeBoardCard, ForgeBoardSeat } from './api'
import { poolGained, poolRaised } from './mana'

/** Where a card can be. The names are the server's; `gone` means it has left
 *  every zone the board draws — shuffled back into the library, most often. */
export const ZONES = [
  'battlefield', 'land', 'hand', 'graveyard', 'exile', 'command',
] as const

export type Zone = (typeof ZONES)[number]

/** What a creature is doing in the combat happening now. The words are the
 *  server's; `''` is a creature standing out of the fight, which is most of
 *  them most of the time. */
export const ATTACKING = 'attacking'
export const BLOCKING = 'blocking'

/** One counter event on one card: which kind moved, from what to what, and on
 *  whose turn.
 *
 *  **The account of how a card got its counters**, which the set alone cannot
 *  give — by the time a creature has three, the arithmetic that made three has
 *  scrolled past (Aaron, 2026-08-26: *"keep a history of why a creature has
 *  all of the counters it does"*).
 *
 *  It says what happened and never who did it. Forge reports no source for a
 *  counter, so naming the card that put them there would be a guess dressed as
 *  a fact, and this side of the wire does not make those. */
export interface CounterMoment {
  kind: string
  was: number
  now: number
  /** The turn number **a player would say** — the same count `BoardState.turn`
   *  carries, not Forge's. */
  turn: number
}

/** One card as it currently stands. */
export interface BoardCard {
  id: number
  name: string
  token: boolean
  types: string
  image: string
  art: string
  artist: string
  zone: string
  seat: number
  tapped: boolean
  /** Whether this card makes mana, from Scryfall's `produced_mana` by way of
   *  Go. A card fact rather than a layout rule — see `rowFor`. */
  mana: boolean
  /** Which mana this printing taps for, in Scryfall's spelling — `['G']`,
   *  `['G','W']`, the five for a Birds of Paradise. Empty for everything that
   *  does not tap for mana.
   *
   *  **A fact about the printing, and never about this activation.** Forge's
   *  mana event carries a seat and a pool and no source; the tap event
   *  carries a card and no mana. Nothing joins them, so a board that said
   *  *this creature filled the pool* would be guessing (ADR 44). What it may
   *  honestly say beside a tapped permanent is what that permanent taps
   *  for. */
  makes: string[]
  /** Scryfall's keywords for the card, unfiltered. `components/keywords.tsx`
   *  decides which of them the board has a sign for. */
  keywords: string[]
  /** The id of the card this one is attached to, or 0 for attached to
   *  nothing. Auras and Equipment only; everything else is 0 forever. */
  attachedTo: number
  /** What is attached to *this* card, in the order it was attached.
   *
   *  Filled at the end of the fold rather than as the changes are applied,
   *  because a sword can arrive before the creature it ends up on and the
   *  answer is only settled once every step has been read. */
  attachments: BoardCard[]
  /** The drawn zone this card left **on the very last beat applied**, or null
   *  the rest of the time. See `foldBoard`'s note on holding the dead. */
  leaving: string | null
  power: number | null
  toughness: number | null
  counters: { kind: string; n: number }[]
  /** How this card's counters arrived, oldest first — the account a hover
   *  reads out. Empty for a card carrying none, because a card with nothing
   *  on it has nothing to explain.
   *
   *  Accumulated from the wire's per-step moves rather than sent whole; see
   *  `CounterMoment` and `foldBoard`'s note on what bounds it. */
  counterHistory: CounterMoment[]
  /** What this creature is doing in the fight: `ATTACKING`, `BLOCKING`, or
   *  `''` for one standing out of it.
   *
   *  The server's word, kept as a string for `zone`'s reason — a newer server
   *  could learn another one, and a room that compares against the two it
   *  knows simply finds neither. */
  combat: string
  /** The seat this creature is attacking, or 0. */
  attacking: number
  /** The board id of the attacker this creature is blocking, or 0. Paired by
   *  id because two identical tokens have one name between them. */
  blocking: number
  /** How many times this card has *left* the command zone.
   *
   *  Commander tax is two generic for each previous cast from the command
   *  zone. Forge never reports the tax and the browser used to count these
   *  transitions itself; it is the server's count now, for ADR 14's reason —
   *  counting them is a reading of the game, and this file reads none. */
  casts: number
  /** Every keyword this *instance* has right now, granted ones included —
   *  where `keywords` above is what its printing carries and is identical for
   *  every copy of the card. */
  live: string[]
  /** The keywords this instance has that its printing does not: the ones
   *  something else is giving it.
   *
   *  **Answered on the server**, by subtracting the printing from the live
   *  set — a reading of the game, which this file makes none of. A Beast
   *  standing beside Kaheera has `['Vigilance']` here and nothing in its own
   *  text that explains it.
   *
   *  **It says that a keyword was granted and never by what.** Forge carries
   *  no source for a granted keyword at all, so the card to blame does not
   *  exist on this wire — copy that renders this must not imply one. */
  granted: string[]
  /** How this permanent left, when Forge said so: `'sacrificed'`, or `''`.
   *
   *  Sacrifice is the only word available. A destruction is announced without
   *  naming a card and a combat death is not announced at all, so the rest of
   *  a permanent's departures say nothing rather than guessing a word. */
  fate: string
  /** The id of the card whose ability made this one a **copy**, or 0.
   *
   *  Its presence is the copy — populate's whole signal. **Not what it was
   *  copied from**: a Centaur Token populated by Growing Ranks names Growing
   *  Ranks, and the permanent it duplicated never crosses the wire. */
  copiedBy: number
}

/** One ability going on the stack at one moment. */
export interface BoardMoment {
  /** The card using it. */
  id: number
  seat: number
  /** Forge's own name for where the source was — `'Command'` for an eminence
   *  trigger, `'Battlefield'` for most things. A commander using an ability
   *  from the command zone never moves, so this is the only sign it acted. */
  zone: string
  /** Whether the game raised this rather than a player activating it. */
  trigger: boolean
  /** What the ability was aimed at, by **board id** — the cards a room can
   *  draw on.
   *
   *  **Empty is the common case and it is the data rather than a gap.** Three
   *  abilities in four are aimed at nothing at all: a surveil trigger has no
   *  target, and Arahbo's attack pump picks the creature it pumps by
   *  definition rather than by targeting it, so the same commander produces
   *  both kinds within one turn. A room that drew on every ability would be
   *  inventing three of every four; a room that draws only on these is saying
   *  what it was told. */
  targets: number[]
}

/**
 * Which row a permanent belongs in, once it is on the battlefield.
 *
 * **This is how a player lays out their own side of a table**, and it was one
 * undifferentiated "permanents" row holding four unrelated kinds of card
 * (Aaron, 2026-08-25: *"normally in Magic I would put artifacts, and
 * enchantments, in their own special zones to stay organized"*). Sorting by
 * type is not tidiness for its own sake — it is how you find the thing you are
 * looking for without reading forty cards, and it is the layout every player
 * already has in their hands.
 *
 * Four calls worth stating:
 *
 * - **Creatures first, always.** A card that is a creature is in the fight,
 *   whatever else it also is — a crewed Vehicle, an animated Sage, a
 *   Planeswalker that got Gideon'd. The front line is decided by what can be
 *   blocked, not by what is printed first on the type line.
 * - **Mana rocks stand with the lands** (*"mana producing artifacts could
 *   really stay back with the lands"*), because those two rows together answer
 *   one question — what can this deck pay for — and a Sol Ring answers it the
 *   same way a Forest does. `mana` comes from Scryfall's `produced_mana`
 *   through Go, so nothing here reads rules text.
 * - **Battles sit with enchantments** (*"battles usually stay with
 *   enchantments basically"*). They are neither, strictly, and there are
 *   rarely more than one; a row of their own would be an empty row all game.
 * - **Everything else keeps a row**, which today is planeswalkers and whatever
 *   a future set invents. It draws only when it holds something.
 */
export type Row = 'creatures' | 'walkers' | 'artifacts' | 'enchantments'
  | 'land'

export function rowFor(card: BoardCard): Row {
  const types = card.types
  if (types.includes('Creature')) return 'creatures'
  if (types.includes('Artifact')) return card.mana ? 'land' : 'artifacts'
  if (types.includes('Enchantment') || types.includes('Battle')) {
    return 'enchantments'
  }
  return 'walkers'
}

/**
 * One side of the table, in the rows a Magic board actually has.
 *
 * **Which row a permanent sits in is the oldest layout convention the game
 * has**: creatures stand at the front, where combat happens, and everything
 * else sits behind them sorted by what it is. Every digital client does it and
 * so does every kitchen table — you put your blockers where you can see what
 * they are facing. Lands go furthest from the middle because they are the row
 * you touch and nobody else does.
 *
 * So a side reads, from the middle of the table outward:
 * **creatures · planeswalkers · artifacts · enchantments · lands · hand** —
 * and the two players' creature rows end up adjacent across the seam, which is
 * where the game is. `rowFor` above argues the sorting.
 */
export interface BoardSide {
  seat: number
  slug: string | null
  name: string
  life: number
  /** This player's **own** counters — poison first, and whatever else a game
   *  puts on a person.
   *
   *  Empty is the ordinary state and it is not the same as a zero: a room
   *  drawing `0 poison` on every game would be claiming that poison is a thing
   *  this match is doing, and almost no match is. The trench draws a bead only
   *  when there is one to draw. */
  counters: { kind: string; n: number }[]
  /** How much commander damage this player has taken, kept apart by the
   *  commander that dealt it and largest first.
   *
   *  **Twenty-one from one commander kills, and that is why this is a list.**
   *  Rule 903.10a counts per commander, so a player who has taken twenty from
   *  each of two is not dead and a single sum would draw them as though they
   *  were. The trench reads the head of this list — the worst one clock — and
   *  never a total.
   *
   *  Empty is the ordinary state and it is not a zero, for `counters`' reason:
   *  a commander has to connect in combat to put anything here, and most
   *  matches never see it. */
  generals: { id: number; name: string; damage: number }[]
  /** The front line. */
  creatures: BoardCard[]
  /** Planeswalkers, and anything a future set invents that is none of the
   *  below. Drawn only when it holds something. */
  walkers: BoardCard[]
  /** Artifacts that are not making mana — equipment, vehicles, the rest. */
  artifacts: BoardCard[]
  /** Enchantments, and battles alongside them. */
  enchantments: BoardCard[]
  /** Lands, and the mana rocks that stand with them. */
  land: BoardCard[]
  hand: BoardCard[]
  graveyard: BoardCard[]
  exile: BoardCard[]
  /** **One seat per commander, in the deck's own order** — one for most decks
   *  and two for a pairing.
   *
   *  A throne is a *place*, so it is drawn whether or not anybody is sitting
   *  in it: the commander is here when it is home and the seat stands empty
   *  when it is out on the sand, which is the one fact the command zone
   *  exists to tell. That is why this holds the card wherever it is standing
   *  rather than only the ones currently in the zone.
   *
   *  The order is the server's, from `deck.yaml`, so a pairing's two thrones
   *  do not swap sides between games. */
  thrones: BoardCard[]
  /** The companion, wherever it is standing, or null for the decks that
   *  brought none.
   *
   *  **It is not a commander and it does not sit on a throne.** It really is
   *  in the command zone — Forge puts it there at setup — but it is a card
   *  waiting to be bought into a hand for {3}, not a card that leads the
   *  deck, and it has never owed a penny of commander tax. */
  companion: BoardCard | null
  /** Whatever else is standing in the command zone: an emblem, or a card an
   *  effect put there. Usually empty, drawn as a pile when it is not.
   *
   *  The thrones and the companion above are taken out of it, because those
   *  have places of their own. */
  command: BoardCard[]
  /** This seat's commanders, **wherever they are standing.**
   *
   *  Not the same list as `thrones`, and the difference is the point: this is
   *  what the *tax* is read off, so it counts a card by having-been-in-the-zone
   *  rather than by the deck naming it. A commander on the battlefield still
   *  has a tax, and it is the tax that made the last cast expensive.
   *
   *  **The companion is excluded, and that is a fix rather than a detail.**
   *  The old rule here was "nothing but a commander begins in the command
   *  zone", which is false for exactly the decks that brought a companion —
   *  so a Kaheera in the zone read as a third commander and put a price on
   *  the rail for a card that has never been cast from anywhere. */
  commanders: BoardCard[]
  /** This seat's **floating mana**, as the symbols a player would write:
   *  `'GGW'` is two green and one white, and `''` is an empty pool.
   *
   *  Mana that exists right now and drains at the end of the step — not to be
   *  confused with `BoardCard.mana`, which is whether a *printing* can make
   *  mana at all. A pool is empty most of the time a person looks at it, which
   *  is why `BoardState.floating` carries the movement as well. */
  pool: string
  /** **The mana this seat had to spend during the last beat applied** — what
   *  it carried in plus everything that arrived — and `pool` itself the rest
   *  of the time.
   *
   *  This is what a pool drawn beside a hand fills *to* before it drains to
   *  `pool`. A sum rather than the pool's high-water mark, and that is a
   *  measurement rather than a preference: Forge taps one land and spends that
   *  mana before tapping the next, so the instantaneous peak is one for every
   *  spell in the game. `lib/mana.ts`'s `poolRaised` has the recorded
   *  sequence.
   *
   *  Computed here rather than in a room, because **this fold is the only
   *  place that knows what the pool held going into the beat.** `pool` is
   *  where it came to rest and `BoardState.floating` is the movement inside
   *  the beat; the value it started from is the previous beat's rest, which
   *  the fold has and nothing downstream does. */
  raised: string
  /** **The mana that arrived** in this seat's pool during the last beat
   *  applied, and empty the rest of the time.
   *
   *  Every rise across the beat's movement added up, so a beat that tapped two
   *  lands, spent them on a ritual, and tapped a third is credited with all
   *  three rather than with the difference between its ends. See `poolGained`.
   *
   *  **It says mana arrived. It never says what made it** — Forge's mana event
   *  carries a seat and a pool and no source, and ADR 44 is why the board says
   *  the first and refuses the second. */
  gained: string
}

/** The board at one moment. */
export interface BoardState {
  /** The turn number **a player would say**, which is the active player's own
   *  turn count — not Forge's.
   *
   * Forge increments once per *player*-turn and alternates seats, so its
   * "turn 15" is one player's eighth. The seam used to print Forge's number
   * straight, so a seventh turn read as 14 or 15. Counted per seat rather than
   * halved, for `playerTurns`' reason in `lib/theater.ts`: Time Warp gives one
   * player two turns in a row, and halving would credit the opponent with a
   * turn they never took. */
  turn: number
  active: number
  sides: BoardSide[]
  /** The abilities used at **the last beat applied**, and empty the rest of
   *  the time.
   *
   *  A moment rather than a state: a room draws a flash on these cards and
   *  the next beat clears it by simply not listing them. This is what makes an
   *  eminence trigger visible at all — its commander never leaves the command
   *  zone, so nothing else in the whole stream says it did anything. */
  abilities: BoardMoment[]
  /** Every value a mana pool took during **the last beat applied**, in order,
   *  and empty the rest of the time.
   *
   *  `BoardSide.pool` is where each seat's pool came to rest; this is the
   *  movement that got it there — mana arriving as permanents tap, and
   *  draining as it is spent. It is a sequence because a pool fills and
   *  empties several times between two beats, so the resting value alone is
   *  almost always an empty pool and shows none of what happened. */
  floating: { seat: number; pool: string }[]
}

function emptySide(seat: ForgeBoardSeat): BoardSide {
  return {
    seat: seat.seat, slug: seat.slug, name: seat.name, life: seat.life,
    counters: [], generals: [],
    creatures: [], walkers: [], artifacts: [], enchantments: [], land: [],
    hand: [], graveyard: [], exile: [], thrones: [], companion: null,
    command: [], commanders: [], pool: '', raised: '', gained: '',
  }
}

/**
 * The board after `steps` of them have happened.
 *
 * `steps` is the count of beats the room has shown, so the picture and the
 * account are the same clock — that parallelism is guaranteed on the server
 * (`BoardStep` and `GameEvent` are built one for one) and relied on here.
 */
export function foldBoard(board: ForgeBoard | null, steps: number): BoardState {
  if (!board) {
    return { turn: 0, active: 0, sides: [], abilities: [], floating: [] }
  }

  const named = new Map<number, ForgeBoardCard>()
  for (const card of board.cards) named.set(card.id, card)

  // The live state of every card that has been touched, plus the order they
  // were first touched in — so a battlefield does not reshuffle itself every
  // time somebody taps a land.
  const state = new Map<number, BoardCard>()
  const order: number[] = []
  const life = new Map<number, number>()
  for (const seat of board.seats) life.set(seat.seat, seat.life)
  // A player's own counters, folded exactly like their life: the last set a
  // step published is what they are holding. Nobody starts with any, so an
  // absent seat here is a seat that has never been given one.
  const held = new Map<number, { kind: string; n: number }[]>()
  // Commander damage, folded the same way: the last set a step published is
  // what that player has taken. An absent seat has never been hit by one.
  const generals = new Map<number, { id: number; damage: number }[]>()
  // Each seat's floating mana, and the two transients that belong to the beat
  // being drawn rather than to the game so far — see `BoardState`.
  const pool = new Map<number, string>()
  // What each seat's pool held *before* the beat being drawn — see the note at
  // the capture below, and `BoardSide.peak`.
  const entering = new Map<number, string>()
  let floating: { seat: number; pool: string }[] = []
  let abilities: BoardMoment[] = []

  let active = 0
  // Forge's number, and each seat's own count of its turns — see BoardState.
  let forgeTurn = 0
  const taken = new Map<number, number>()
  const upTo = Math.max(0, Math.min(steps, board.steps.length))
  for (let i = 0; i < upTo; i++) {
    const step = board.steps[i]
    if (!step) continue
    if (step.turn && step.turn !== forgeTurn) {
      forgeTurn = step.turn
      const whose = step.seat || active
      if (whose) taken.set(whose, (taken.get(whose) ?? 0) + 1)
    }
    if (step.seat) active = step.seat
    for (const moved of step.life ?? []) life.set(moved.seat, moved.life)
    for (const moved of step.counters ?? []) held.set(moved.seat, moved.counters)
    for (const moved of step.generals ?? []) generals.set(moved.seat, moved.from)
    // **The pool folds to its last value and the movement is kept only for the
    // beat being drawn now.** A seat appears more than once in one step — mana
    // arriving and being spent — so the last entry is where the pool came to
    // rest, and the sequence itself is only worth anything at the moment it is
    // happening. Both are `step.floating`; which one a room wants depends on
    // whether it is drawing a pool or an arrival.
    // **What the pool held going *into* the last beat, captured before the
    // beat is applied over it.** This is the whole reason `peak` and `gained`
    // are settled here and not in a room: a pool that carried two green into
    // the beat and came out with five did not gain five, and nothing
    // downstream of this loop can tell the difference — `BoardSide.pool` is
    // the resting value and the previous rest is gone the moment this line
    // runs. Cheap, and only on the beat being drawn.
    if (i === upTo - 1) {
      for (const moved of step.floating ?? []) {
        entering.set(moved.seat, pool.get(moved.seat) ?? '')
      }
    }
    for (const moved of step.floating ?? []) pool.set(moved.seat, moved.pool)
    if (i === upTo - 1) {
      floating = step.floating ?? []
      // **Every field the wire carries has to be named here.** This is a
      // rewrite and not a widening, so anything left out is dropped silently —
      // which is exactly how `targets` reached a browser that could not see
      // it, on a step that had been carrying it since the scribe learned to
      // ask. Adding a field to `ForgeBoardStep` and not to this line is a
      // change nothing fails on.
      abilities = (step.abilities ?? []).map((used) => ({
        id: used.id, seat: used.seat ?? 0, zone: used.zone ?? '',
        trigger: used.trigger ?? false, targets: used.targets ?? [],
      }))
    }
    for (const change of step.changes ?? []) {
      let card = state.get(change.id)
      const known = named.get(change.id)
      if (!card) {
        card = {
          id: change.id,
          name: known?.name ?? '',
          token: known?.token ?? false,
          types: known?.types ?? '',
          image: known?.image ?? '',
          art: known?.art ?? '',
          artist: known?.artist ?? '',
          zone: 'gone', seat: known?.seat ?? 0, tapped: false,
          mana: known?.mana ?? false, makes: known?.makes ?? [],
          keywords: known?.keywords ?? [],
          leaving: null,
          power: null, toughness: null, counters: [], counterHistory: [],
          combat: '', attacking: 0, blocking: 0, casts: 0,
          attachedTo: 0, attachments: [],
          live: [], granted: [], fate: '',
          copiedBy: known?.copied_by ?? 0,
        }
        state.set(change.id, card)
        order.push(change.id)
      }
      // **Whether this card is being held on the sand for the beat that says
      // it is leaving**, decided below and read at the bottom of this loop.
      //
      // A creature that dies keeps the counters it died with for exactly that
      // beat. The server sheds them on the zone change, correctly — a card
      // that changes zones is a new object and arrives with nothing on it —
      // and the same step is the one where the board deliberately stands the
      // dead creature back up so the skull has something to land on. Applying
      // the shed there would strip the counters off the card in the instant a
      // person is looking straight at it. It is a beat late, and being a beat
      // late is what the hold is.
      let held = false
      if (change.zone) {
        // **The dead are held where they died, for the length of one beat.**
        //
        // Forge reports a death and the zone change on the same line, so by
        // the time the room says "X dies" the card is already in a graveyard
        // and there is no instant at which the board holds a dead creature
        // still standing. The skull therefore had nowhere to land but the
        // grave — which is where a headstone goes and not where a death
        // happens (Aaron, 2026-08-25: *"it should appear over the card being
        // destroyed itself, like the shield"*).
        //
        // Only on the final step, which is the beat being spoken *now*: a card
        // that left the battlefield ten beats ago is simply gone. So this is a
        // property of the moment being drawn rather than of the card, which is
        // also why a scrub backwards un-kills it — the hold is a function of
        // the count, like everything else here.
        //
        // Uniform across every way of leaving play, not just dying. Bounced,
        // exiled, sacrificed, ceasing to exist: the board shows the departure
        // and then the card is where it went. The *mark* on top is the beat's
        // business, and only a death draws a skull.
        if (i === upTo - 1 && change.zone !== card.zone
            && (card.zone === 'battlefield' || card.zone === 'land')) {
          card.leaving = card.zone
          held = true
        }
        card.zone = change.zone
      }
      if (change.seat) card.seat = change.seat
      if (change.tapped != null) card.tapped = change.tapped
      if (change.power != null) card.power = change.power
      if (change.toughness != null) card.toughness = change.toughness
      if (change.types) {
        card.types = change.types
        // **A card with a painting per face turns over here, and nowhere
        // else.** The dictionary files a card under the name it was first seen
        // by and never revises it, so a modal double-faced card played as its
        // land is still filed as the sorcery on its front — and drawing that
        // sorcery in the land row is the board showing a card that is not the
        // card on the table (Aaron, 2026-08-29). A type line changing is the
        // one thing on this pipe that says a permanent turned over, and it
        // arrives on the step the game announced it. `faceInPlay` answers -1
        // for everything it cannot tell apart, and -1 leaves the card exactly
        // as it was.
        const face = known ? faceInPlay(known, change.types) : -1
        if (face >= 0 && known) {
          card.image = pictureOf(known, face)
          // The name goes with the picture or the tile labels a land with a
          // sorcery's name. Only ever for the cards this turned over, so every
          // other card in the match is named exactly as it was.
          card.name = known.faces?.[face] ?? card.name
        }
      }
      if (change.combat != null) card.combat = change.combat
      if (change.attacking != null) card.attacking = change.attacking
      if (change.blocking != null) card.blocking = change.blocking
      if (change.casts != null) card.casts = change.casts
      // `!= null` for both, because an empty array is the answer that says a
      // creature has lost the last keyword something was giving it — the same
      // distinction `counters` below turns on, and the same bug if it is read
      // as truthy.
      if (change.live != null) card.live = change.live
      if (change.granted != null) card.granted = change.granted
      if (change.fate) card.fate = change.fate
      if (!held) {
        for (const move of change.counter_moves ?? []) {
          remember(card.counterHistory, move, taken.get(active) ?? 0)
        }
        // **An empty array is the server saying this card has none.** Being
        // exact about where the bug was, because this line looks like the fix
        // and is not it: an empty array is truthy, so the old truthy test
        // would have applied one perfectly well. Nothing ever sent one — the
        // field was a plain slice and `omitempty` renders "none" and "nothing
        // changed" as the same absent bytes — so a dead creature went on
        // wearing the counters it had in life (Aaron, 2026-08-26). The fix is
        // in Go; `!= null` is this side saying the same thing in a way that
        // cannot be misread later.
        if (change.counters != null) {
          card.counters = change.counters
          // A card with nothing on it has nothing to explain, so the account
          // goes with the counters. This is also what keeps the history from
          // outliving the object it describes: a creature that dies and comes
          // back is a new card and starts its story again.
          if (change.counters.length === 0) card.counterHistory = []
        }
      }
      // `!= null` rather than truthy: zero is the detach, and it is the half
      // of this field that matters most — a sword that came off must stop
      // being drawn on the bear.
      if (change.attached_to != null) card.attachedTo = change.attached_to
    }
  }

  const sides = board.seats.map(emptySide)
  const bySeat = new Map(sides.map((side) => [side.seat, side]))
  // **The command zone's own furniture, named by the server.**
  //
  // Everything that begins in a command zone looks alike from here — a
  // commander, a partner and a companion all arrive as a card in `command` on
  // step zero — so which is which is decided in Go against `deck.yaml` and
  // arrives as board ids. This is the browser reading that answer, not making
  // one: a room that had to tell a companion from a commander would need to
  // know the companion rules, and `lib/board.ts` decides no Magic.
  const companionOf = new Map<number, number>()
  for (const seat of board.seats) {
    if (seat.companion) companionOf.set(seat.seat, seat.companion)
  }
  for (const [seat, total] of life) {
    const side = bySeat.get(seat)
    if (side) side.life = total
  }
  for (const [seat, on] of held) {
    const side = bySeat.get(seat)
    if (side) side.counters = on
  }
  for (const [seat, from] of generals) {
    const side = bySeat.get(seat)
    if (!side) continue
    // **Sorted worst-first here rather than in the trench**, because the only
    // question a scoreboard asks of this list is which clock is furthest along
    // — and a component that had to search for that would be a component that
    // could forget to. The name comes out of the dictionary the same way an
    // ability's targets do; a commander has been on the battlefield to have
    // dealt combat damage, so it is always in there.
    side.generals = from
      .map((one) => ({
        id: one.id, damage: one.damage, name: named.get(one.id)?.name ?? '',
      }))
      .sort((a, b) => b.damage - a.damage)
  }
  for (const [seat, held] of pool) {
    const side = bySeat.get(seat)
    if (!side) continue
    side.pool = held
    // **A pool that did not move this beat has only what it is holding**, so a
    // seat carrying mana across beats draws a standing row rather than filling
    // to nothing. Overwritten below for the seats that did move.
    side.raised = held
  }
  // **The beat's movement, read twice and for two different questions.** The
  // arena flashes what arrived; the pool beside the hand fills to what stood
  // there and drains to what is left. `lib/mana.ts` carries both arguments.
  //
  // Only the seats that moved: a seat whose pool did nothing this beat rests
  // at its own `pool` with no peak above it and nothing gained, which is what
  // `emptySide` already says and what a room reading a still pool wants.
  for (const [seat, was] of entering) {
    const side = bySeat.get(seat)
    if (!side) continue
    const moved = floating.filter((m) => m.seat === seat).map((m) => m.pool)
    side.raised = poolRaised(was, moved)
    side.gained = poolGained(was, moved)
  }
  for (const id of order) {
    const card = state.get(id)
    if (!card) continue
    const side = bySeat.get(card.seat)
    if (!side) continue
    const isCompanion = companionOf.get(card.seat) === card.id
    if (isCompanion) side.companion = card
    // The tax list. A companion sits in the command zone without ever having
    // been cast from it, so it is taken out here rather than priced.
    if (!isCompanion && (card.zone === 'command' || card.casts > 0)) {
      side.commanders.push(card)
    }
    // **An Aura or Equipment goes on its host, not in a row of its own.**
    //
    // That is where it goes at a real table: you slide the sword under the
    // creature carrying it, because the two are one thing now and reading them
    // apart is the reader's problem. A row of loose equipment is a list of
    // objects nobody can match to the creatures across the seam — which is
    // what the board drew before this, and it drew it for a format where
    // Voltron is a whole way to win.
    //
    // **Only when the host is actually on the table**, which is a rendering
    // rule rather than a claim about Magic. Forge fires the detach itself when
    // an attachment changes zones — measured on a real game: a Hammer of
    // Nazahn destroyed by Nature's Claim reported `graveyard` and
    // `attached_to: 0` on the same step — so this should never fire. If it
    // ever does, an attachment with nowhere to sit falls back to its own row,
    // which is a card in the wrong place rather than a card that vanished.
    const host = card.attachedTo ? state.get(card.attachedTo) : undefined
    if (host && onTheTable(host) && onTheTable(card)) {
      host.attachments.push(card)
      continue
    }
    // Still standing, for this one beat, in the row it is standing in. It is
    // deliberately **not** also drawn in the zone it has gone to: a card in
    // two places is a worse answer than a card a beat behind, and next beat it
    // is in the graveyard like anything else.
    if (card.leaving) {
      if (card.leaving === 'land') side.land.push(card)
      else side[rowFor(card)].push(card)
      continue
    }
    // A zone the browser does not draw — `gone`, or a word a newer server
    // learned — simply takes the card off the table rather than throwing.
    if (card.zone === 'battlefield') {
      // Sorted the way a player sorts their own side of a table. `rowFor`
      // argues every call in it; a mana rock answers 'land' from here, which
      // is why this runs before the zone check below rather than after.
      side[rowFor(card)].push(card)
      continue
    }
    const into = (ZONES as readonly string[]).includes(card.zone)
      ? (card.zone as Zone) : null
    // The command zone is drawn as places rather than as a pile, so the two
    // cards that have places of their own are kept out of it. What is left is
    // an emblem or whatever an effect put there, and that is still a pile.
    if (into === 'command' && (isCompanion || throned(board, card))) continue
    if (into && into !== 'battlefield') side[into].push(card)
  }
  // **The thrones, in the deck's order rather than the board's.** Filled last
  // and from `state` rather than from a zone, because a throne is drawn for a
  // commander that is out on the battlefield exactly as it is for one sitting
  // at home — standing empty is the whole thing it has to say.
  for (const seat of board.seats) {
    const side = bySeat.get(seat.seat)
    if (!side) continue
    for (const id of seat.commanders ?? []) {
      const card = state.get(id)
      if (card) side.thrones.push(card)
    }
  }
  return { turn: taken.get(active) ?? 0, active, sides, abilities, floating }
}

/**
 * The most moments one card's counter history will hold.
 *
 * **Bounded because the fold is not incremental.** A board is rebuilt from
 * step zero on every render — about a hundred and forty of them in a game — so
 * anything that grows per card is walked and rebuilt that many times.
 *
 * Two things keep it small, and the ceiling is only the backstop. Counters are
 * rare: the recorded match raises five counter events across two whole games,
 * and the work here is proportional to that count however long the game runs.
 * And `remember` folds repeats together, so a creature pumped nine times on
 * one turn is one moment rather than nine — which is also how a person would
 * say it. Reaching forty needs forty *distinct* turns on which some kind of
 * counter moved, and a game that long has other problems.
 *
 * Oldest first out, because a hover that is one line short at the top still
 * reads true; one that stopped recording would be silently wrong about the
 * counters actually on the card.
 */
const HISTORY_MOMENTS = 40

/**
 * Fold one counter move into a card's history.
 *
 * **Repeats on the same turn are one moment.** A creature that gains six
 * counters one at a time is a creature that gained six counters, and six lines
 * saying so is not an account — it is a log. Merging keeps the `was` of the
 * first and the `now` of the last, which is the whole of what happened.
 */
function remember(
  history: CounterMoment[], move: { kind: string; was: number; now: number },
  turn: number,
): void {
  const last = history[history.length - 1]
  if (last && last.kind === move.kind && last.turn === turn) {
    last.now = move.now
    return
  }
  history.push({ kind: move.kind, was: move.was, now: move.now, turn })
  if (history.length > HISTORY_MOMENTS) history.shift()
}

/** Whether this card has a throne of its own, and so is not part of the pile
 *  of everything else in the command zone. */
function throned(board: ForgeBoard, card: BoardCard): boolean {
  for (const seat of board.seats) {
    if (seat.seat === card.seat) {
      return (seat.commanders ?? []).includes(card.id)
    }
  }
  return false
}

/** Whether a card is standing on the battlefield — either row of it, or held
 *  there for the one beat that says it is leaving. */
function onTheTable(card: BoardCard): boolean {
  return card.zone === 'battlefield' || card.zone === 'land' || !!card.leaving
}

/**
 * Whether a beat's card and a board card are the same card.
 *
 * Not `===`, and the reason is a fact about the wire: Forge names a **face**,
 * never Scryfall's combined `A // B` (`events.go`, and `docs/FORGE.md`'s
 * fourth fact). The board's names come from the scribe and can carry the
 * combined spelling, so a transforming creature would attack, block and die
 * without a single mark ever landing on it — silently, and only in the decks
 * that play them. Comparing the front face costs one split.
 *
 * **Here rather than in a component**, because two callers ask it now and they
 * ask it from opposite ends: the board matching a beat against a card in a
 * row, and the centre stage looking a *cast* card up in the match's own card
 * list, since a cast beat carries a name and no id. Two copies of this ruling
 * would be two places for it to rot, and the rot would be invisible.
 */
export function sameCard(onBoard: string, inBeat: string): boolean {
  return onBoard === inBeat || onBoard.split(' // ')[0] === inBeat
}

/**
 * Whether a beat's mark belongs on this tile.
 *
 * **The id decides it, and the name only answers when there is no id.**
 *
 * A mark used to be matched on Forge's spelling alone, on the argument that
 * two copies of one name is a token or a basic and that marking both was a
 * better wrong answer than marking neither. That held while a beat had nothing
 * else to offer. It does now — `StagedBeat.id` carries the board id — and the
 * wrong answer was not rare: eight Cat Soldier Tokens swinging stand in
 * *several* piles, because `stackRow` tells identical-looking creatures apart
 * by their counters and their Equipment, and every pile sharing the spelling
 * lit up at once. Including the ones standing out of the fight (Aaron,
 * 2026-08-28: *"a stack of 8 tokens that is attacking show it 8 times"*).
 *
 * **A pile answers for every card in it.** One tile draws eight identical
 * creatures and the beat may name any of them, so this takes the stack's whole
 * `ids` rather than the representative's own — a tile that answered only for
 * the card it happens to draw would mark nothing seven times out of eight.
 *
 * The name is still the answer for a match played without the scribe, which
 * has no ids at all. There it marks every copy, which is exactly what it did
 * before and is still better than marking none.
 *
 * **Here rather than in the component**, for `sameCard`'s reason one function
 * up: a ruling written in two places is a ruling with two places to rot.
 */
export function markedHere(struck: { card: string; id?: number } | null,
  name: string, ids: readonly number[]): boolean {
  if (!struck) return false
  if (struck.id) return ids.includes(struck.id)
  return sameCard(name, struck.card)
}

/**
 * Which half of a card a beat named, or `-1` for a beat that named some other
 * card entirely.
 *
 * **Forge renames a card when its other half is cast, and this board learns a
 * card's name once.** Bonecrusher Giant is drawn into a hand under that name;
 * the instant its Adventure goes on the stack Forge's view calls the same
 * object *Stomp*, and the beat says Stomp. Nothing in `cards` is called Stomp,
 * so the middle of the arena drew a black card with a title on it — in the one
 * moment the room exists to show a spell (Aaron, 2026-08-28).
 *
 * `faces` is the answer and it comes from the pool: the server sends every name
 * the card answers to, so a beat naming any of them finds it. Zero is the front
 * — the name before the `//` — and one is the other one, which is the half a
 * room might want to point at, or the face it should hold up instead.
 *
 * **The list is asked first and the dictionary's own name is the fallback**,
 * which is the opposite of the order this used to run in and matters now that
 * the index picks a *picture*. The dictionary files a card under whatever face
 * Forge happened to name first, and that is not always the front: a card first
 * seen already on its back is filed under the back's name, and asking "is this
 * the name we know it by" answers *zero* for it — the front — and would hold up
 * the wrong painting of a card the room had every fact about. The list is
 * ordered as the card is printed, so an index into it means the same thing
 * whichever name the dictionary happens to carry.
 *
 * A card with no `faces` answers 0 or -1 and nothing else, which is nearly
 * every card, and is what the fallback is for.
 */
export function halfNamed(card: ForgeBoardCard, inBeat: string): number {
  const at = (card.faces ?? []).indexOf(inBeat)
  if (at >= 0) return at
  return sameCard(card.name, inBeat) ? 0 : -1
}

/**
 * The painting for one of a card's names.
 *
 * **Two questions, and they were one question for as long as a card had one
 * picture.** A Bonecrusher Giant and its Stomp are printed on the same piece of
 * cardboard, so answering to *Stomp* with the card's own image is showing the
 * card that was cast. A modal double-faced card is not like that: Agadeem's
 * Awakening and Agadeem, the Undercrypt are two paintings, and holding up the
 * sorcery for the land is the quiet version of the same fault the black plate
 * was the loud version of (Aaron, 2026-08-29: *"MDF cards are not rendering
 * intelligently"*).
 *
 * `face_images` is the server saying this card has a painting per face, and its
 * absence is the server saying it has one. **Never a guess either way**: it is
 * sent whole or not at all, so an index into it is either the face's own
 * painting or there is no such list and `image` is the truth for every name.
 *
 * The empty string is a real answer and the room already knows what to do with
 * it — a card with no painting is set in type on a plate, which is legible and
 * is not a claim about which half is being looked at.
 */
export function pictureOf(card: ForgeBoardCard, half: number): string {
  return (half >= 0 ? card.face_images?.[half] : undefined) ?? card.image ?? ''
}

/**
 * Which face a permanent is standing on the battlefield as, or `-1` for a card
 * this cannot answer for.
 *
 * **The type line is the only thing on this pipe that says a permanent turned
 * over.** Forge names a card by its current face, but this board's dictionary
 * learns a name once and never revises it — so the card that was drawn as
 * *Agadeem's Awakening* is still filed under that name when it is a land on the
 * battlefield. What the board *does* keep current is the type line, which
 * arrives as a change on the step the game announced it, and a modal
 * double-faced card going from Sorcery to Land is a sentence about which face
 * is now face up.
 *
 * **Only ever asked with a type line that just changed**, which matters: the
 * dictionary's own `types` is the card's *last* type line in the game rather
 * than its first, so a Delver of Secrets that flips on turn six is filed all
 * game as an Insectile Aberration. Reading that at step zero would turn the
 * card over before the game did. A change is a fact about the step it is on.
 *
 * The ladder is two rungs and a floor. The whole type line first, compared
 * loosely enough to survive two spellings of a dash — Forge writes
 * `Creature - Human Insect` and Scryfall writes `Creature — Human Insect`, and
 * they are the same sentence. Then the card types alone, which is what tells a
 * Sorcery from a Land when the subtypes disagree about something. And a floor
 * of `-1` when neither rung singles out exactly one face: two faces that are
 * both `Creature — Werewolf` genuinely cannot be told apart by this, and the
 * honest answer to that is to leave the card showing whatever it was showing
 * rather than to turn it over on a coin toss.
 */
export function faceInPlay(card: ForgeBoardCard, types: string): number {
  const kinds = card.face_types
  if (!card.face_images || !kinds
      || kinds.length !== card.face_images.length) {
    return -1
  }
  const live = plainType(types)
  for (const read of [(s: string) => s, cardTypes]) {
    let found = -1
    for (let i = 0; i < kinds.length; i++) {
      if (read(plainType(kinds[i] ?? '')) !== read(live)) continue
      if (found >= 0) return -1
      found = i
    }
    if (found >= 0) return found
  }
  return -1
}

/** A type line with the spelling arguments taken out of it: one case, one kind
 *  of dash, one kind of gap. The dash is the one that matters — Forge and
 *  Scryfall disagree about it on every card that has subtypes. */
function plainType(line: string): string {
  return line.toLowerCase().replace(/[\u2010-\u2015]/g, '-')
    .replace(/\s+/g, ' ').trim()
}

/** The card types alone — everything before the dash — which is the half of a
 *  type line that says what kind of thing a permanent is. */
function cardTypes(line: string): string {
  return (line.split('-')[0] ?? line).trim()
}

/** A run of identical cards, drawn as one stack with a count on it. */
export interface BoardStack {
  /** The first of them, which is what gets drawn. */
  card: BoardCard
  count: number
  /** Every card folded into this stack, by Forge instance id, in the order
   *  they arrived.
   *
   *  **`card` is one of them and the rest are invisible without this.** A pile
   *  merges on what a person can see across a table, and what a blocker is
   *  *facing* is deliberately not part of that key — a wall of five Saprolings
   *  is one wall however many attackers it is spread across. That was the
   *  right call for drawing the pile and it is not enough for drawing an
   *  **arrow**: pointing the whole wall at whichever attacker the first
   *  Saproling happened to block is a sentence the board cannot support.
   *
   *  So the ids travel, and `alignLanes` reads through them: a pile speaks for
   *  itself only when every card in it is facing the same way. */
  ids: number[]
}

/**
 * Collapse identical cards into stacks, the way a hand of nine Forests sits on
 * a real table.
 *
 * Nine separate Forests laid out flat is not what a board looks like and it is
 * not what anybody reads: it takes a whole row to say one thing, and on a
 * phone it takes several. A player stacks them and knows the count.
 *
 * **Identical means identical in play, not identical in name**, and that is
 * the whole difficulty. Three tapped Forests and six untapped ones are not
 * nine Forests — they are two piles, and which pile is which is exactly the
 * information somebody is looking at the board to get. So the key carries
 * everything a player would notice from across the table: the name, whether it
 * is turned, what it is currently worth in a fight, and what is sitting on it.
 * Two cards merge only when nothing visible tells them apart.
 *
 * The card's own instance id is deliberately **not** in the key — the id is
 * what makes two Forests different to Forge and what makes them the same to a
 * person.
 *
 * Order is the order the cards arrived in, taken from the first of each run,
 * so a stack does not jump across the row when its count changes.
 */
export function stackRow(cards: BoardCard[]): BoardStack[] {
  const out: BoardStack[] = []
  const at = new Map<string, BoardStack>()
  for (const card of cards) {
    const key = [
      card.name,
      card.tapped ? 't' : '',
      // **An attacking token pile and a blocking one are two piles**, which is
      // the ask this exists to answer (Aaron, 2026-08-26: *"make sure when it
      // comes to tokens that blocking and attacking token piles are separated
      // visually"*). Twelve Saprolings of which five are swinging is the one
      // fact anybody across the seam is looking for, and one stack of twelve
      // hides it completely.
      //
      // The *role* only, not the card each blocker is facing: two blockers on
      // two different attackers are still one wall to look at, and splitting
      // them would take the piles back apart card by card.
      card.combat,
      // A dying Saproling must not merge into the eight beside it — the whole
      // point of holding it is that somebody is looking at that one.
      card.leaving ?? '',
      card.power ?? '', card.toughness ?? '',
      card.counters.map((c) => `${c.kind}:${c.n}`).join('+'),
      // A bear carrying a sword is not the same card as the bear beside it,
      // and merging them would hide the sword *and* miscount the bears. Names
      // rather than ids, so two identically equipped tokens still stack —
      // which is the case this whole function exists for.
      //
      // **How many, as well as which**, and the count is not belt-and-braces:
      // a name can be empty. An attachment the server never put in the card
      // dictionary folds with `name: ''` (see `foldBoard`, which has always
      // allowed for it), and `[''].join('+')` is byte-for-byte the key of a
      // card carrying nothing whatsoever. So an equipped permanent merged into
      // the bare pile beside it, and the pile drew one sword over a count of
      // two — the sword hidden and the bears miscounted, which is the exact
      // pair of harms the line above exists to prevent.
      //
      // **It could only ever have shown on a token.** Commander is singleton,
      // so tokens are the only cards that repeat, and a stack of one is a
      // stack whose key never has to be right. That is why the report came in
      // as a fact about tokens (Aaron, 2026-08-26: *"equipment on a token
      // should put it in its own pile"*) and why the bare-versus-equipped test
      // above it passed the whole time.
      //
      // Sorted, because the order two swords were picked up in is not
      // something anybody can see across a table: the tuck in `FieldGeared`
      // shows a 7px corner per attachment, which says *how many* and never
      // *which came first*. Two creatures wearing the same two things are one
      // pile. `map` already copies, so this sorts the names and not the cards.
      card.attachments.length,
      card.attachments.map((a) => a.name).sort().join('+'),
    ].join('|')
    const already = at.get(key)
    if (already) {
      already.count++
      already.ids.push(card.id)
      continue
    }
    const stack: BoardStack = { card, count: 1, ids: [card.id] }
    at.set(key, stack)
    out.push(stack)
  }
  return out
}

/** A card's power and toughness, when it has any worth showing.
 *
 * Blank for anything that is not a creature: a Food token is a 0/0 in Forge's
 * view because it has no power at all, and printing "0/0" on an artifact is a
 * statement about it that its card does not make. */
export function fightingStats(card: BoardCard): string | null {
  if (!card.types.includes('Creature')) return null
  if (card.power == null || card.toughness == null) return null
  return `${card.power}/${card.toughness}`
}

/**
 * The two creature lanes, shuffled so that a clash lines up across the trench.
 *
 * **The two front lines face each other and nothing said which pair was which**
 * (Aaron, 2026-08-27). The board already knew — a blocker carries the board id
 * of the attacker it stopped — and drew both rows left-packed anyway, so a
 * blocker four cards along was standing opposite an attacker that had nothing
 * to do with it. Across a seam wide enough to hold the scoreboard, "who is
 * fighting whom" was a question you answered by hovering.
 *
 * **This used to hand back the clash list too, and the arrows that read it are
 * gone** — a fight is drawn on the centre stage now (`clashOf`, `StagedBout`).
 * The arrangement stays, because it is not the arrows: it is the board setting
 * itself up, so that a person who looks away from the stage and down at the
 * sand finds the blocker standing under the creature it stopped (Aaron,
 * 2026-08-28: *"the alignLanes is fine to stay, it prepares the board in a
 * way"*).
 *
 * **Only a clash moves anything, and only the blocker moves.** Aaron chose the
 * rule: the attacker lane is left exactly as it stands, each blocker slides to
 * sit under the attacker it stopped, and every other creature keeps the slot it
 * already had. A gap opens where a blocker came from and stays open — sand,
 * for as long as the combat lasts. The alternative was pulling clashes to the
 * left of both lanes, which lines them up by making every unblocked attacker
 * move sideways for a fight it is not in.
 *
 * **A lane with no resolvable clash is returned exactly as it came in**, dense
 * and gapless. This is the case in almost every beat of almost every game, and
 * it is why the board does not grow holes in normal play: the slots only exist
 * while somebody is being blocked.
 *
 * **A pile speaks only when it is unanimous.** `stackRow` merges blockers on
 * the role and not on the target, so five Saprolings blocking three different
 * attackers are one wall — see [BoardStack.ids]. Such a pile resolves to no
 * single attacker, so it neither moves nor gets an arrow, and the board says
 * nothing rather than something wrong. A wall that really is all facing one
 * attacker is unanimous and behaves like any other blocker.
 */
export interface AlignedLanes {
  /** Both lanes, the same length, `null` where the slot is empty sand. */
  far: (BoardStack | null)[]
  near: (BoardStack | null)[]
}

/** Which single attacker a stack is facing, or 0 when it is not unanimous. */
function facing(stack: BoardStack, cards: Map<number, BoardCard>): number {
  let one = 0
  for (const id of stack.ids) {
    const at = cards.get(id)?.blocking ?? 0
    if (at === 0) return 0
    if (one === 0) one = at
    else if (one !== at) return 0
  }
  return one
}

export function alignLanes(far: BoardStack[], near: BoardStack[],
  cards: BoardCard[]): AlignedLanes {
  const flat = new Map<number, BoardCard>()
  for (const card of cards) flat.set(card.id, card)
  const dense: AlignedLanes = { far, near }

  // Which lane is swinging. In a two-player game exactly one is, and anything
  // else — nobody attacking, or a payload that says both are — is a board this
  // has no business rearranging.
  const swinging = (lane: BoardStack[]) =>
    lane.some((s) => s.card.combat === 'attacking')
  const farSwings = swinging(far)
  const nearSwings = swinging(near)
  if (farSwings === nearSwings) return dense
  const attackers = farSwings ? far : near
  const blockers = farSwings ? near : far

  // Where each attacker stands, by every card id it stands for — a blocker
  // names the card it stopped, which need not be the one the pile draws.
  const slotOf = new Map<number, number>()
  attackers.forEach((stack, slot) => {
    if (stack.card.combat !== 'attacking') return
    for (const id of stack.ids) slotOf.set(id, slot)
  })

  const placed = new Map<number, BoardStack>()
  // Where the blockers land, which is what decides the lane's own shape.
  const filled = new Set<number>()
  const free = (from: number) => {
    let slot = Math.max(0, from)
    while (placed.has(slot)) slot++
    return slot
  }

  // The blockers first, because theirs are the only slots that are *chosen*.
  //
  // **A gang stands around what it is blocking, not off to one side of it.**
  // Several blockers on one attacker all want the one slot, and the obvious
  // rule — first one takes it, the rest trail to the right — puts a 14/14
  // trampler under the first cat with the other two standing over empty sand
  // beside it, which reads as one block and two bystanders. Measured on a real
  // board before this was changed. Centred, the attacker is in the middle of
  // the creatures that stopped it, which is what a gang block looks like.
  //
  // An even gang cannot straddle a slot, so it takes the attacker's own and
  // grows right: two blockers under an attacker leave it opposite the first.
  const gangs = new Map<number, BoardStack[]>()
  for (const stack of blockers) {
    const at = facing(stack, flat)
    const slot = at ? slotOf.get(at) : undefined
    if (slot === undefined) continue
    const gang = gangs.get(slot)
    if (gang) gang.push(stack)
    else gangs.set(slot, [stack])
  }
  const moved = new Set<BoardStack>()
  for (const [slot, gang] of [...gangs].sort((a, b) => a[0] - b[0])) {
    let want = Math.max(0, slot - Math.floor((gang.length - 1) / 2))
    for (const stack of gang) {
      const put = free(want)
      placed.set(put, stack)
      filled.add(put)
      moved.add(stack)
      want = put + 1
    }
  }
  // Nothing was blocking anything this board can resolve, so nothing moves.
  if (filled.size === 0) return dense

  // Everything else keeps the slot it had, and slides only if a blocker has
  // taken it.
  blockers.forEach((stack, slot) => {
    if (moved.has(stack)) return
    placed.set(free(slot), stack)
  })

  // **A slot is an index, so both lanes are laid out against the same ruler —
  // and a trailing gap is not a slot.** The two lanes need not come out the
  // same length: an empty slot only means anything when something stands
  // beyond it, and padding the shorter lane out to the longer would put
  // phantom cells on the end of a row that draws nothing there anyway.
  const span = Math.max(attackers.length,
    ...[...placed.keys()].map((s) => s + 1))
  const laid: (BoardStack | null)[] = []
  for (let slot = 0; slot < span; slot++) laid.push(placed.get(slot) ?? null)
  const held: (BoardStack | null)[] = []
  for (let slot = 0; slot < span; slot++) held.push(attackers[slot] ?? null)
  while (laid.length && laid[laid.length - 1] === null) laid.pop()
  while (held.length && held[held.length - 1] === null) held.pop()

  return {
    far: farSwings ? held : laid,
    near: farSwings ? laid : held,
  }
}

/**
 * One fight on the sand: the attacker, and everything standing in its way.
 *
 * **This is what replaced the arrows** (Aaron, 2026-08-28: *"the
 * attacker/blocker arrows go, replaced by a bout on the centre stage"*). An
 * arrow across the trench could say *these two are fighting* and nothing more
 * — it was a line drawn between two cards a person still had to find, on a
 * board where a gang block put three of them in a row. The bout takes the
 * middle of the arena instead and shows the fight at the size the stage draws
 * a spell: the attacker out of its own seat's edge, the wall ranked across
 * from it.
 *
 * **Read from the blocker up, because that is the direction the beat runs.**
 * Forge announces a block per blocker — three cats on a Ghalta is three
 * `block` beats, each naming one cat and carrying the attacker's id — so the
 * one thing every such beat holds is *this creature stopped that one*. Asked
 * from the blocker, the answer is exact; asked from the attacker, the room
 * would have to guess which of several fights the beat meant.
 *
 * **And the gang is read off the board rather than accumulated.** Each beat
 * re-asks the question and gets the whole wall as it stands at that step, so
 * the second cat's beat returns two cats and the third's returns three. That
 * is what makes the wall assemble on the stage without anything remembering
 * anything: the fold already holds the state, and scrubbing backwards is a
 * smaller gang rather than a gang that has to be un-built. It is the marks'
 * own rule — a picture is a function of the beat — applied to a group.
 *
 * **The wall is stacked, not enumerated** (Aaron, 2026-08-28: *"stacks of
 * tokens can stay stacked with their x number"*). Twelve Saprolings on one
 * attacker are one card with a twelve on it, which is both what a player sees
 * across a table and the only way the rank fits on a stage 940 pixels wide.
 * `stackRow` decides what identical means, and it already distinguishes an
 * attacking pile from a blocking one — filtered to this attacker first, so a
 * wall facing somebody else is not counted into this fight.
 *
 * Null whenever the question has no answer: a beat with no id, a creature that
 * is not blocking, an attacker that has left the board between the block and
 * the beat being read. Every one of those is a board this has no business
 * drawing a fight from.
 */
export interface Clash {
  /** The creature being blocked. */
  attacker: BoardCard
  /** Everything blocking it, stacked — the count is on the stack. */
  blockers: BoardStack[]
  /** Which half the attacker swung out of, so the stage can put it on that
   *  seat's own edge and rank the wall across from it. */
  swinging: 'far' | 'near'
}

export function clashOf(blockerId: number | undefined,
  far: BoardSide | null | undefined,
  near: BoardSide | null | undefined): Clash | null {
  return fightOf(blockerId, far, near, 'blocker')
}

/**
 * The fight a creature is in, whichever end of it the creature is standing at.
 *
 * **`clashOf` reads up from a blocker, and that is the wrong door for half the
 * questions.** A block beat names the blocker, so the declaration only ever
 * asks from that end — but a *death* names whoever died, and an attacker's own
 * `blocking` is 0, so asking `clashOf` about a dying attacker answers null
 * every time. Measured before it was believed: a probe over a ten-game match
 * reported "attacker died 0 times", which is not a fact about Cats and
 * Dinosaurs but a fact about which end the question was asked from.
 *
 * So `side` says which end the id is: `'blocker'` reads up to the attacker it
 * stopped, `'either'` accepts both and works out which. The declaration keeps
 * the narrow door because a block beat can only ever name a blocker, and a
 * narrow door is one fewer way to be wrong.
 */
export function fightOf(id: number | undefined,
  far: BoardSide | null | undefined,
  near: BoardSide | null | undefined,
  side: 'blocker' | 'either' = 'either'): Clash | null {
  if (!id) return null
  const sides: [BoardSide | null | undefined, 'far' | 'near'][] =
    [[far, 'far'], [near, 'near']]
  // Where every creature stands, and on whose side. Both halves in one map,
  // because a fight is the one thing on this board that spans the seam.
  const at = new Map<number, { card: BoardCard; side: 'far' | 'near' }>()
  for (const [side, which] of sides) {
    for (const card of side?.creatures ?? []) at.set(card.id, { card, side: which })
  }
  const here = at.get(id)
  if (!here) return null
  // Which end of the fight this id is standing at. A blocker names the card it
  // stopped; an attacker is the fight itself and names nobody.
  const attacker = here.card.blocking !== 0
    ? at.get(here.card.blocking)
    : side === 'either' && here.card.combat === ATTACKING ? here : undefined
  if (!attacker) return null
  // Everything facing this attacker, in board order so the rank does not
  // reshuffle itself as it grows.
  const wall: BoardCard[] = []
  for (const { card } of at.values()) {
    if (card.blocking === attacker.card.id) wall.push(card)
  }
  if (wall.length === 0) return null
  return {
    attacker: attacker.card,
    blockers: stackRow(wall),
    swinging: attacker.side,
  }
}
