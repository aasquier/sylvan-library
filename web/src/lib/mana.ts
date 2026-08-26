/**
 * A mana pool, as a bag of pips — and the two questions a room asks of it.
 *
 * The wire spells a pool the way a player writes one on a napkin: `'GGW'` is
 * two green and one white, `''` is an empty pool, and `'CC'` is the two
 * colourless off a Sol Ring. That is a *multiset*, not a set and not a number,
 * and every operation here treats it as one — two green mana is two things you
 * can spend, and a pool that collapsed them to "green" would be unable to
 * answer the only question anybody has about a pool.
 *
 * ## Why this is arithmetic and not drawing
 *
 * `BoardStep.floating[]` is a *sequence*: every value a seat's pool took
 * between two beats, in order, because a pool fills and drains several times
 * inside one beat and the value it ends on is empty nearly every time (Go's
 * `board.floating` measured it: nine of ten). So the browser is handed the
 * whole movement and has to say what it *meant* — which is two different
 * questions, and they need two different answers:
 *
 * - **What arrived?** The mana that came in, which is what the middle of the
 *   arena flashes. That is the sum of the *rises* across the sequence, so a
 *   beat that filled, was spent, and filled again is credited with both fills
 *   rather than with the difference between its ends.
 * - **What was there to spend?** What the seat carried in plus everything that
 *   arrived, which is what the pool beside a hand fills to before it drains to
 *   what was left. `poolRaised` argues why this is a sum rather than the
 *   pool's high-water mark, and the argument is a measurement off a real match
 *   rather than a preference.
 *
 * Both are computed in `lib/board.ts`'s fold, which is the one place that
 * knows what the pool held going *into* the beat. A room that tried to work
 * either out from `BoardSide.pool` alone would be guessing at the half of the
 * story the fold already threw away.
 *
 * ## Colourless is a letter, not a hole
 *
 * `'C'` is a real pip with a real symbol, and it is the one this pipe has
 * already got wrong once: Forge writes colourless as `ManaAtom`'s own byte and
 * not as `MagicColor`'s, and a pool decoded off the wrong one comes through
 * empty. **An empty pool and a mis-read pool render identically**, which is
 * why nothing here silently drops a letter it does not recognise — see
 * `MANA_LETTERS`.
 *
 * The bottom of the file is the clock rather than the arithmetic: `usePoolFlow`
 * is what turns two values the fold settled into a row that fills and drains,
 * and it lives here rather than in a component file for `lib/reel.ts`'s reason
 * — a hook and a table of durations are not components, and oxlint's
 * fast-refresh rule is right about that.
 */

import { useEffect, useState } from 'react'

import { COLOR_NAMES } from './mtg'

/**
 * The letters a pool may be spelled with, in the order Magic writes them.
 *
 * WUBRG then colourless, which is the order every other mana row in this app
 * uses, so a pool and a cost read left to right the same way.
 *
 * **Nothing else is accepted, and that is a decision rather than a gap.** A
 * letter this does not know is a pipe that has changed under us, and the one
 * failure mode worth engineering against is the silent one: a pool that came
 * through mis-decoded looks exactly like a pool that is empty, so a drop that
 * said nothing would hide the bug it was caused by. Unknown letters are kept
 * and drawn as themselves — see `poolPips`.
 */
const MANA_LETTERS = 'WUBRGC'

/** One pip in a pool: its letter, and where the eye should find it. */
export interface Pip {
  /** `'W'`, `'U'`, `'B'`, `'R'`, `'G'`, `'C'` — or whatever the wire said. */
  symbol: string
  /** This pip's place in the drawn row, which is what a stagger is timed off
   *  and what React reconciles on. Stable for as long as the pool is: adding a
   *  green to `'GG'` leaves the first two exactly where they were, so only the
   *  arriving pip animates and the standing ones do not flinch. */
  at: number
}

/** How a pool sorts: Magic's own order, and anything unrecognised after it. */
function rank(letter: string): number {
  const i = MANA_LETTERS.indexOf(letter)
  return i < 0 ? MANA_LETTERS.length : i
}

/**
 * A pool string as the pips a row would draw, in Magic's own order.
 *
 * Sorted rather than left in the order the mana happened to arrive, because
 * the row is read as a *quantity* — `'GWG'` and `'GGW'` are the same pool, and
 * a row that reshuffled itself when a second green landed would look like the
 * white had moved. Sorting also means the standing pips keep their index when
 * one of their own colour arrives, which is what lets only the new pip animate.
 */
export function poolPips(pool: string): Pip[] {
  return [...pool]
    .filter((c) => c.trim() !== '')
    .sort((a, b) => rank(a) - rank(b) || (a < b ? -1 : a > b ? 1 : 0))
    .map((symbol, at) => ({ symbol, at }))
}

/** How many pips a pool holds. Cheaper than building the row for a comparison,
 *  and it is the comparison the widest-value question is decided on. */
export function poolSize(pool: string): number {
  let n = 0
  for (const c of pool) if (c.trim() !== '') n++
  return n
}

/**
 * What one pool holds that another does not — as a pool.
 *
 * Multiset difference, so `'GGW'` less `'G'` is `'GW'` and not `'W'`: the
 * point of a pool is that two green are two things, and a difference that
 * cancelled them by colour would report a spent green as a spent everything.
 * Never negative — a letter the second pool has more of simply does not
 * appear, which is what makes this safe to hand a drain.
 */
export function poolMinus(pool: string, taken: string): string {
  const left = new Map<string, number>()
  for (const c of taken) {
    if (c.trim() === '') continue
    left.set(c, (left.get(c) ?? 0) + 1)
  }
  let out = ''
  for (const c of pool) {
    if (c.trim() === '') continue
    const owed = left.get(c) ?? 0
    if (owed > 0) {
      left.set(c, owed - 1)
      continue
    }
    out += c
  }
  return out
}

/**
 * **The mana that arrived**, across a beat's whole movement.
 *
 * `was` is what the pool held going into the beat and `moved` is every value
 * it took inside it, in order. Every step that is a *rise* contributes what it
 * added; every step that is a drain contributes nothing. So a beat that tapped
 * three lands and spent them reports three mana arriving, and so does a beat
 * that tapped two, spent them, and tapped one more — which is the case a plain
 * before-and-after subtraction gets wrong, and it is not rare: a ritual and the
 * spell it paid for land inside one beat all the time.
 *
 * Nothing here is interpolated. Every value in `moved` is a state Forge
 * announced, and this only ever reads the differences between them.
 */
export function poolGained(was: string, moved: readonly string[]): string {
  let from = was
  let got = ''
  for (const now of moved) {
    got += poolMinus(now, from)
    from = now
  }
  return got
}

/**
 * **The mana this seat had to spend across the beat** — what it carried in,
 * plus everything that arrived.
 *
 * This is what a pool drawn beside a hand fills *to* before it drains to what
 * was left, and it is Aaron's phrase rather than a term of art: *"the symbols
 * should appear as available mana to cast if that is possible."*
 *
 * **The obvious answer is the wrong one, and a real match is what settled
 * it.** The first version of this took the *peak* — the widest single value
 * the pool ever held — on the reasoning that a pool holding one green twice
 * never held two. That reasoning is impeccable and the picture it draws is
 * useless, because of how Forge actually pays for a spell. Measured on a
 * Goreclaw match, 108 beats, the pool sequence for a five-mana turn is:
 *
 *     'G', '', 'G', '', 'G', '', 'G', '', 'G', ''
 *
 * Forge taps one land, spends that mana, taps the next. **The instantaneous
 * peak is one, for every spell in the game**, so a row drawn from it would
 * flicker a single pip five times inside one beat — a strobe that says
 * nothing, where five pips filling and draining says exactly what happened.
 *
 * So the row shows the mana that *passed through*, which is the true and
 * useful reading: five mana were raised and five were spent. It is not a claim
 * that five stood there at once, and nothing in the drawing suggests one —
 * they arrive together and leave together, which is the shape of paying for a
 * spell.
 */
export function poolRaised(was: string, moved: readonly string[]): string {
  return was + poolGained(was, moved)
}

/** Small numbers in words, because "2 green" is a spreadsheet and a person
 *  says "two green". Past six a pool is a rarity and a numeral is clearer than
 *  a word nobody was expecting to read. */
const COUNTED = ['no', 'one', 'two', 'three', 'four', 'five', 'six']

/**
 * **A pool in words**, for anybody not looking at fourteen-pixel pictures.
 *
 * "two green and one white", which is what a player says out loud — never
 * "GGW", which is shorthand for people who already know, and never a list of
 * five colours where "any colour" is what is meant. Commandment 2 is the whole
 * reason this exists: the pips are a drawing, and a drawing is a thing you have
 * to already know how to read.
 */
export function poolSaid(pool: string): string {
  const by = new Map<string, number>()
  for (const pip of poolPips(pool)) {
    by.set(pip.symbol, (by.get(pip.symbol) ?? 0) + 1)
  }
  if (!by.size) return 'empty'
  const said = [...by].map(([symbol, n]) => {
    const name = symbol === 'C' ? 'colourless'
      : (COLOR_NAMES[symbol] ?? symbol).toLowerCase()
    return `${COUNTED[n] ?? n} ${name}`
  })
  const last = said.pop() ?? ''
  return said.length ? `${said.join(', ')} and ${last}` : last
}

/* ------------------------------------------- watching a pool fill and drain */

/**
 * How long the pool stands full before it is spent, at this pace.
 *
 * **The draining is the point, so the two halves are paced and not snapped.**
 * Aaron asked to watch mana appear beside a hand and then be *depleted on the
 * cast itself* — and both of those are movements, so a pool that changed value
 * between two frames would have answered the question by refusing it.
 *
 * A share of the beat, floored and capped, which is the same shape
 * `stageLife` uses and for the same two reasons at the two ends. **The floor is
 * the important one**: under about a tenth of a second a fill is not a fill,
 * it is a flicker, and half a fade is worse than none because it draws the eye
 * to something already gone. At the fast pace the floors add up to slightly
 * more than the beat, so a new beat cuts the last one's drain short — which is
 * what every paced thing in this room looks like at 150ms and reads as flow
 * rather than as a fault.
 */
export function poolFill(beat: number): number {
  // Paused is not a slow pace, it is the absence of one. Nothing is arriving to
  // cut this short, so it takes the length somebody reading it would want.
  if (beat === 0) return FILL_CAP
  return Math.max(FILL_FLOOR, Math.min(FILL_CAP, beat * FILL_SHARE))
}

/** ...and how long the mana that was spent takes to leave. Longer than the
 *  fill, deliberately: mana arriving is a small bright event and mana being
 *  spent is the thing the row exists to show, so the drain is the half that
 *  gets the time. */
export function poolDrain(beat: number): number {
  if (beat === 0) return DRAIN_CAP
  return Math.max(DRAIN_FLOOR, Math.min(DRAIN_CAP, beat * DRAIN_SHARE))
}

const FILL_SHARE = 0.34
const FILL_FLOOR = 110
const FILL_CAP = 900
const DRAIN_SHARE = 0.4
const DRAIN_FLOOR = 130
const DRAIN_CAP = 520

/** A pool in motion: what is standing there, and what is on its way out. */
export interface PoolFlow {
  /** The mana that is in the pool right now, drawn as a row. */
  held: Pip[]
  /** The mana that has just been spent, drawn draining out of the row. Empty
   *  except during the drain, and never overlapping `held` — a pip is in one
   *  list or the other, so nothing is ever drawn twice. */
  spent: Pip[]
}

/**
 * Fill a pool to what stood in it, then drain it to what is left.
 *
 * The two facts come from the fold (`BoardSide.peak` and `BoardSide.pool`) and
 * this is only the clock between them. Three things it must not do, which is
 * very nearly the whole hook:
 *
 * - **It must never strand a pip.** Mana sitting beside a hand that the game no
 *   longer has is the room lying about the one number a player is counting.
 *   Every timeout is cleared by the effect that set it, and the drain is
 *   started from state rather than from a closure, so there is no path where a
 *   spent pip is left with nothing coming to remove it.
 * - **It must not animate a pool that did not move.** A seat holding two green
 *   across four beats should show two green sitting still, not two green
 *   re-arriving four times. `peak === rest` skips the drain entirely, and the
 *   pips reconcile by index so the standing ones never flinch.
 * - **The fill lands with the beat, not a frame after it.** Chosen during the
 *   render and only the *taking away* is a timer — the same call `useStaged`
 *   and `useHeldMark` make, for the same reason. Mana that appeared half a
 *   frame after the sentence that spent it would read as the wrong order.
 */
export function usePoolFlow(peak: string, rest: string, beat: number,
  key: string): PoolFlow {
  const [held, setHeld] = useState(rest)
  const [spent, setSpent] = useState('')
  const [wasKey, setWasKey] = useState<string | null>(null)
  // The drain waiting to happen, carrying both ends of it. An object rather
  // than a flag so the effect never has to read `held` out of a closure that
  // may already be stale — the bug this shape exists to make impossible.
  const [draining, setDraining] = useState<{ from: string; to: string } | null>(
    null)
  if (key !== wasKey) {
    setWasKey(key)
    setHeld(peak)
    setSpent('')
    setDraining(peak === rest ? null : { from: peak, to: rest })
  }
  const fill = poolFill(beat)
  const drain = poolDrain(beat)
  useEffect(() => {
    if (!draining) return
    const spend = window.setTimeout(() => {
      setHeld(draining.to)
      setSpent(poolMinus(draining.from, draining.to))
      setDraining(null)
    }, fill)
    return () => { window.clearTimeout(spend) }
  }, [draining, fill])
  useEffect(() => {
    if (!spent) return
    const gone = window.setTimeout(() => setSpent(''), drain)
    return () => { window.clearTimeout(gone) }
  }, [spent, drain])
  return { held: poolPips(held), spent: poolPips(spent) }
}
