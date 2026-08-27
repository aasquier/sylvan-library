/**
 * The loupe's size is a fraction of the card, and both of the numbers on it
 * are tied to one declaration.
 *
 * **jsdom has no layout, so nothing here can measure a collision** — this bug
 * was found by driving a real browser at 375 and reading
 * `getBoundingClientRect` off nine lenses. What a test *can* hold is the
 * structure that made the collision possible: a constant. `--field-lens` was a
 * flat `26px` while a lane hands its cards whatever width it can afford, so
 * the glass took 45% of a card at the desk and 57% of one on a phone — and a
 * turned card's glass swings out past its own edge by `lens - (w/2 - |corner
 * offset|)`, which at a 46px card is 9.4px against a 7px gap. Measured:
 * Elder Gargaroth's lens and a tapped Fauna Shaman's, overlapping 2.5x19.2px.
 *
 * **This is the second time a constant measured against one card size has been
 * the bug.** #363 moved the counters off the neighbour's loupe reasoning "at
 * 58x81 the card is still 56 pixels wide at the height the chips sit" — true,
 * and never true on a phone. So the assertion is deliberately about *kind*
 * rather than value: these have to be expressed in terms of something that
 * moves, and a number typed in px is the failure however carefully it was
 * chosen.
 */

import { expect, it } from 'vitest'

// @ts-expect-error -- node's types are out of scope for `src`; the argument for
// this escape hatch is in `lib/tokens.test.ts`, and the path is relative to
// `web/` because that is where vitest runs.
const nodeFs = await import('node:fs')
const CSS: string = nodeFs.readFileSync('src/index.css', 'utf8')

/** The body of the first rule that lists `sel` and declares `prop`. */
function declares(sel: string, prop: string): string {
  const css = CSS.replace(/\/\*[\s\S]*?\*\//g, '')
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selectors = (m[1] ?? '').split(',').map((s: string) => s.trim())
    const found = new RegExp(`(?:^|;|\\n)\\s*${prop}\\s*:([^;}]*)`, 'i')
      .exec(m[2] ?? '')
    if (selectors.includes(sel) && found) {
      return (found[1] ?? '').replace(/\s+/g, ' ').trim()
    }
  }
  return ''
}

it('sizes the loupe from the card rather than from a number', () => {
  const lens = declares('.field-card', '--field-lens')
  expect(lens, '--field-lens is not declared on .field-card').toBeTruthy()
  expect(lens, 'the loupe is a constant again; a lane that shrinks its cards '
    + 'will hand a turned one a glass that reaches its neighbour')
    .toContain('--field-card-full')
  // The desktop ceiling survives, so a wide board is unchanged to the pixel.
  expect(lens, 'the 26px ceiling is gone, so a wide card grows its glass')
    .toContain('26px')
})

it('sizes the figures on the glass from the glass', () => {
  const font = declares('.field-card-lens-pt', 'font-size')
  expect(font, '.field-card-lens-pt declares no font-size').toBeTruthy()
  // A flat `0.6rem` on a circle that is not a flat size: a 12/12 overflowed
  // its own lens by two pixels at 375 before any of this moved. Most creatures
  // are one digit a side and fit at any size, which is why it went unseen.
  expect(font, 'the figures are a fixed size on a glass that is not, so a '
    + 'two-digit power and toughness overflows its own circle')
    .toContain('--field-lens')
})
