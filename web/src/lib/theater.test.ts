/**
 * The theater's two pure functions (`components/theater.test.tsx` covers the
 * stage they dress):
 *
 * - **`theaterRows` is total.** It is handed `Job.partial`, which is
 *   `unknown` and legitimately arrives as `null` (before the first tick, and
 *   again the moment the job finishes), as an object with no `rows` at all (a
 *   pre-theater worker streaming counts — the skew `worker.run_match`
 *   tolerates on purpose), and as the real payload. All three are "no rows
 *   yet" and none of them may throw.
 * - **`shortName` is how a deck is referred to out loud** — the general's
 *   name, which is what fits in a feed row.
 */

import { describe, expect, it } from 'vitest'
import type { ForgeGameRow } from './api'
import { shortName, theaterRows } from './theater'

function row(over: Partial<ForgeGameRow> = {}): ForgeGameRow {
  return {
    game: 1, winner: 'arahbo-cats', seconds: 6.2, turns: 9, draw: false,
    timed_out: false, ...over,
  }
}

describe('theaterRows', () => {
  it('reads the rows out of a real partial', () => {
    expect(theaterRows({ rows: [row(), row({ game: 2 })] })).toHaveLength(2)
  })

  // The three shapes that are all "nothing yet". `null` is what the server
  // sends before the first tick and again once it clears the partial on
  // completion; the bare object is a shim that streams counts and no rows.
  it.each([
    ['null', null],
    ['undefined', undefined],
    ['a number', 7],
    ['a payload with no rows', {}],
    ['a payload whose rows are not a list', { rows: 'soon' }],
  ])('is empty and does not throw for %s', (_label, partial) => {
    expect(theaterRows(partial)).toEqual([])
  })
})

describe('shortName', () => {
  it.each([
    ['Arahbo, Roar of the World — Cats', 'Arahbo'],
    ['Goreclaw, Terror of Qal Sisma — Mono-Green Stompy', 'Goreclaw'],
    ['Gyome, Master Chef — Food (Mitch)', 'Gyome'],
    ['Trostani tokens', 'Trostani tokens'],
  ])('shortens %s to %s', (full, short) => {
    expect(shortName(full)).toBe(short)
  })
})
