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
  command: BoardCard[]
  /** This seat's commanders, **wherever they are standing.**
   *
   *  `command` is the zone and holds only the ones currently home, which is
   *  the wrong list for the one question the zone is asked: what would it cost
   *  to get them back out. A commander on the battlefield still has a tax, and
   *  it is the tax that made the last cast expensive. Nothing but a commander
   *  begins in the command zone, so having-been-there is the whole test. */
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
    hand: [], graveyard: [], exile: [], command: [], commanders: [],
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
          mana: known?.mana ?? false,
          power: null, toughness: null, counters: [], casts: 0,
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
        card.zone = change.zone
      }
      if (change.seat) card.seat = change.seat
      if (change.tapped != null) card.tapped = change.tapped
      if (change.power != null) card.power = change.power
      if (change.toughness != null) card.toughness = change.toughness
      if (change.types) card.types = change.types
      if (change.counters) card.counters = change.counters
    }
  }

  const sides = board.seats.map(emptySide)
  const bySeat = new Map(sides.map((side) => [side.seat, side]))
  for (const [seat, total] of life) {
    const side = bySeat.get(seat)
    if (side) side.life = total
  }
  for (const id of order) {
    const card = state.get(id)
    if (!card) continue
    const side = bySeat.get(card.seat)
    if (!side) continue
    if (card.zone === 'command' || card.casts > 0) side.commanders.push(card)
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
    if (into && into !== 'battlefield') side[into].push(card)
  }
  return { turn: taken.get(active) ?? 0, active, sides }
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
      card.power ?? '', card.toughness ?? '',
      card.counters.map((c) => `${c.kind}:${c.n}`).join('+'),
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
