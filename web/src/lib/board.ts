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
 * **It decides nothing.** Every judgement about the game — which zone a card
 * is in, whether a land row or a battlefield row, what happens when a card
 * leaves a zone it had already left, that the stack is not a zone at all and
 * the library is never sent — was made in Go, against a recorded match, and is
 * argued in `go/internal/sim/tier3/board.go`. What is left here is arithmetic:
 * take the deltas, put the cards where they say. A browser that had to know
 * any Magic to draw this would be a second place for those rulings to drift.
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
  power: number | null
  toughness: number | null
  counters: { kind: string; n: number }[]
}

/**
 * One side of the table, in the rows a Magic board actually has.
 *
 * **The battlefield is two rows, not one**, and which row a permanent sits in
 * is the oldest layout convention the game has: creatures stand at the front,
 * where combat happens, and everything else sits behind them. Every digital
 * client does it and so does every kitchen table — you put your blockers where
 * you can see what they are facing. Lands go furthest from the middle because
 * they are the row you touch and nobody else does.
 *
 * So a side reads, from the middle of the table outward:
 * **creatures · permanents · lands · hand** — and the two players' creature
 * rows end up adjacent across the seam, which is where the game is.
 */
export interface BoardSide {
  seat: number
  slug: string | null
  name: string
  life: number
  /** The front line. */
  creatures: BoardCard[]
  /** Artifacts, enchantments, planeswalkers, battles — everything on the
   *  battlefield that is not a creature and not a land. */
  permanents: BoardCard[]
  land: BoardCard[]
  hand: BoardCard[]
  graveyard: BoardCard[]
  exile: BoardCard[]
  command: BoardCard[]
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
    creatures: [], permanents: [], land: [], hand: [], graveyard: [],
    exile: [], command: [],
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
          power: null, toughness: null, counters: [],
        }
        state.set(change.id, card)
        order.push(change.id)
      }
      if (change.zone) card.zone = change.zone
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
    // A zone the browser does not draw — `gone`, or a word a newer server
    // learned — simply takes the card off the table rather than throwing.
    if (card.zone === 'battlefield') {
      // The front line, or behind it. An animated land is on the battlefield
      // *and* a creature, and it belongs at the front with the things that
      // can be blocked.
      const row = card.types.includes('Creature') ? 'creatures' : 'permanents'
      side[row].push(card)
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
