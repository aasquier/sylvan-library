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
    creatures: [], walkers: [], artifacts: [], enchantments: [], land: [],
    hand: [], graveyard: [], exile: [], thrones: [], companion: null,
    command: [], commanders: [], pool: '',
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
  // Each seat's floating mana, and the two transients that belong to the beat
  // being drawn rather than to the game so far — see `BoardState`.
  const pool = new Map<number, string>()
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
    // **The pool folds to its last value and the movement is kept only for the
    // beat being drawn now.** A seat appears more than once in one step — mana
    // arriving and being spent — so the last entry is where the pool came to
    // rest, and the sequence itself is only worth anything at the moment it is
    // happening. Both are `step.floating`; which one a room wants depends on
    // whether it is drawing a pool or an arrival.
    for (const moved of step.floating ?? []) pool.set(moved.seat, moved.pool)
    if (i === upTo - 1) {
      floating = step.floating ?? []
      abilities = (step.abilities ?? []).map((used) => ({
        id: used.id, seat: used.seat ?? 0, zone: used.zone ?? '',
        trigger: used.trigger ?? false,
      }))
    }
    for (const change of step.changes ?? []) {
      let card = state.get(change.id)
      if (!card) {
        const known = named.get(change.id)
        card = {
          id: change.id,
          name: known?.name ?? '',
          token: known?.token ?? false,
          types: known?.types ?? '',
          image: known?.image ?? '',
          art: known?.art ?? '',
          artist: known?.artist ?? '',
          zone: 'gone', seat: known?.seat ?? 0, tapped: false,
          mana: known?.mana ?? false, keywords: known?.keywords ?? [],
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
      if (change.types) card.types = change.types
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
  for (const [seat, held] of pool) {
    const side = bySeat.get(seat)
    if (side) side.pool = held
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

/** A run of identical cards, drawn as one stack with a count on it. */
export interface BoardStack {
  /** The first of them, which is what gets drawn. */
  card: BoardCard
  count: number
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
      continue
    }
    const stack: BoardStack = { card, count: 1 }
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
