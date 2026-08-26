/**
 * A pool is a bag, and every wrong answer here looks like an empty pool.
 *
 * That is the whole reason this file is separate from the board's own tests:
 * the two failure modes this arithmetic has — a multiset treated as a set, and
 * a letter silently dropped — both render as *nothing on the screen*, which is
 * exactly what a pool looks like when it is genuinely empty nine times in ten.
 * A rendering test cannot tell those apart. This one can.
 */

import { expect, it } from 'vitest'

import { poolGained, poolMinus, poolPips, poolRaised, poolSaid, poolSize }
  from './mana'

it('reads a pool as a bag of mana and not as a set of colours', () => {
  // **Two green are two things you can spend.** A pool folded to its distinct
  // colours would say a player with `'GG'` could cast a one-drop and not a
  // two-drop, which is the only question anybody asks a pool.
  expect(poolSize('GG')).toBe(2)
  expect(poolPips('GG').map((p) => p.symbol)).toEqual(['G', 'G'])
  expect(poolPips('')).toEqual([])
})

it('sorts a pool the way every other mana row in the app reads', () => {
  // WUBRG then colourless, so a pool and a cost are read left to right the
  // same way — and so the standing pips keep their index when one of their own
  // colour arrives, which is what lets only the new pip animate.
  expect(poolPips('GWU').map((p) => p.symbol)).toEqual(['W', 'U', 'G'])
  expect(poolPips('CG').map((p) => p.symbol)).toEqual(['G', 'C'])
  expect(poolPips('GWG').map((p) => p.at)).toEqual([0, 1, 2])
})

it('keeps colourless, because a dropped letter looks like an empty pool', () => {
  // **The bug this pipe has already had once.** Forge writes colourless as
  // `ManaAtom`'s own byte rather than `MagicColor`'s, and a pool decoded off
  // the wrong one arrives empty — live data showed `'CC'` off a Sol Ring. An
  // empty pool and a mis-decoded pool render identically, so nothing here is
  // allowed to quietly discard a letter it was not expecting.
  expect(poolSize('CC')).toBe(2)
  expect(poolPips('CC').map((p) => p.symbol)).toEqual(['C', 'C'])
  // And a letter nothing recognises is kept rather than swallowed, for the
  // same reason: a pipe that changed under us should show something odd, not
  // show nothing at all.
  expect(poolPips('GX').map((p) => p.symbol)).toEqual(['G', 'X'])
})

it('subtracts one pool from another without cancelling by colour', () => {
  // Multiset difference. `'GGW'` less one green is `'GW'` — a difference that
  // cancelled by colour would report a spent green as a spent everything.
  expect(poolMinus('GGW', 'G')).toBe('GW')
  expect(poolMinus('GGW', 'GG')).toBe('W')
  expect(poolMinus('GGW', '')).toBe('GGW')
  // Never negative: safe to hand a drain that took more than it should have.
  expect(poolMinus('G', 'GGG')).toBe('')
})

it('credits every rise, not the difference between the ends', () => {
  // **The case a before-and-after subtraction gets wrong, and it is not rare.**
  // A ritual and the spell it paid for land inside one beat all the time: the
  // pool filled, emptied, and filled again, and four mana really did arrive
  // even though the beat began and ended on nothing.
  expect(poolGained('', ['GG', '', 'GG'])).toBe('GGGG')
  // The ordinary case: three lands tapped and the lot spent.
  expect(poolGained('', ['G', 'GG', 'GGG', ''])).toBe('GGG')
  // **A pool carried into the beat is not credited again.** Two green were
  // already there; only the white is news.
  expect(poolGained('GG', ['GGW'])).toBe('W')
  // A beat that only drained gained nothing.
  expect(poolGained('GG', [''])).toBe('')
  expect(poolGained('', [])).toBe('')
})

it('counts what was there to spend, carried in and arrived alike', () => {
  // **The recorded sequence that decided this, off a real Goreclaw match.**
  // Forge taps one land, spends that mana, and taps the next — so a five-mana
  // turn reaches the browser as five separate arrivals, and the pool is never
  // holding more than one at a time. Drawing the *instantaneous peak* would
  // flicker one pip five times inside a single beat; drawing what was raised
  // shows the five mana that paid for the spell.
  expect(poolRaised('', ['G', '', 'G', '', 'G', '', 'G', '', 'G', '']))
    .toBe('GGGGG')
  // What was carried in is there to spend too, and is not double-counted when
  // the beat's own values include it.
  expect(poolRaised('GG', ['GGW'])).toBe('GGW')
  expect(poolRaised('GG', [''])).toBe('GG')
  // A beat where nothing moved leaves what the seat is holding.
  expect(poolRaised('G', [])).toBe('G')
  expect(poolRaised('', [])).toBe('')
})

it('says a pool in the words a player would use', () => {
  // The pips are a drawing, and a drawing is a thing you have to already know
  // how to read (commandment 2).
  expect(poolSaid('')).toBe('empty')
  expect(poolSaid('G')).toBe('one green')
  expect(poolSaid('GG')).toBe('two green')
  // **The sentence reads the drawing**, which is the rule `producedName`
  // already follows: the row is drawn WUBRG, so the words are said WUBRG. Two
  // orders for one pool is one order too many — somebody hearing "two green
  // and one white" while looking at a white pip first has been told the row
  // moved when it did not.
  expect(poolSaid('GGW')).toBe('one white and two green')
  expect(poolSaid('WUBRG'))
    .toBe('one white, one blue, one black, one red and one green')
  expect(poolSaid('CC')).toBe('two colourless')
  // Past the small numbers a numeral is clearer than a word nobody expected.
  expect(poolSaid('GGGGGGG')).toBe('7 green')
})
