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
  /** How many times this card has *left* the command zone.
   *
   *  Commander tax is two generic for each previous cast from the command
   *  zone, and Forge never reports the tax — but it reports every zone
   *  change, and a card going command-to-anywhere is that cast. Counted here
   *  rather than in the view because it is history: the view holds one folded
   *  moment, and by the time a commander is home again the casts that made it
   *  expensive have scrolled past. */
  casts: number
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
}

function emptySide(seat: ForgeBoardSeat): BoardSide {
  return {
    seat: seat.seat, slug: seat.slug, name: seat.name, life: seat.life,
    creatures: [], walkers: [], artifacts: [], enchantments: [], land: [],
    hand: [], graveyard: [], exile: [], thrones: [], companion: null,
    command: [], commanders: [],
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
  if (!board) return { turn: 0, active: 0, sides: [] }

  const named = new Map<number, ForgeBoardCard>()
  for (const card of board.cards) named.set(card.id, card)

  // The live state of every card that has been touched, plus the order they
  // were first touched in — so a battlefield does not reshuffle itself every
  // time somebody taps a land.
  const state = new Map<number, BoardCard>()
  const order: number[] = []
  const life = new Map<number, number>()
  for (const seat of board.seats) life.set(seat.seat, seat.life)

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
          power: null, toughness: null, counters: [], casts: 0,
          attachedTo: 0, attachments: [],
        }
        state.set(change.id, card)
        order.push(change.id)
      }
      if (change.zone) {
        // Before the assignment, because the transition is the fact: a card
        // that was in the command zone and is now anywhere else was cast from
        // it. (Put onto the battlefield without casting would over-count, and
        // Forge's AI does not do it with commanders.)
        if (card.zone === 'command' && change.zone !== 'command') {
          card.casts += 1
        }
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
        }
        card.zone = change.zone
      }
      if (change.seat) card.seat = change.seat
      if (change.tapped != null) card.tapped = change.tapped
      if (change.power != null) card.power = change.power
      if (change.toughness != null) card.toughness = change.toughness
      if (change.types) card.types = change.types
      if (change.counters) card.counters = change.counters
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
  return { turn: taken.get(active) ?? 0, active, sides }
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
      // A dying Saproling must not merge into the eight beside it — the whole
      // point of holding it is that somebody is looking at that one.
      card.leaving ?? '',
      card.power ?? '', card.toughness ?? '',
      card.counters.map((c) => `${c.kind}:${c.n}`).join('+'),
      // A bear carrying a sword is not the same card as the bear beside it,
      // and merging them would hide the sword *and* miscount the bears. Names
      // rather than ids, so two identically equipped tokens still stack —
      // which is the case this whole function exists for.
      card.attachments.map((a) => a.name).join('+'),
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
