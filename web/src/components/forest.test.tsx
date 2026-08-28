/**
 * `SceneBackdrop`, and the one property of it a rendering test can hold.
 *
 * The room shipped in #118 and was invisible until the 2026-08-16 drive, for
 * two reasons that between them are the whole reason this file exists.
 *
 * The first is testable here and is what these cases pin: **the backdrop must
 * not render inside the routed page.** `App.tsx` wraps that page in
 * `.page-enter`, which animates a `transform`, and a transformed ancestor
 * becomes the containing block for every `position: fixed` descendant — so
 * rendered in place, `inset: 0` resolved against whichever masthead section
 * happened to contain it (measured 1232x376 against a 1600x1000 viewport) and
 * the lanes came out 58px wide. The fix is a portal to `document.body`, and a
 * portal is exactly the kind of thing a later refactor "simplifies" away, so
 * it gets a test that fails when it does.
 *
 * The second is **not** testable here, and saying so is the point: the lanes
 * were also 250x145 instead of 250x1000, because an `<img>` is a replaced
 * element and `top: 0; bottom: 0` with `height: auto` is over-constrained.
 * jsdom has no layout, so nothing in this file can catch that class of bug —
 * only a browser measuring a real box can, which is how it was found. The
 * guard for it is the comment in `index.css`, not an assertion here. A test
 * that asserted the element merely *renders* is what let both bugs ship: the
 * suite had 42 passing cases over this feature and not one asked how big it
 * was.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it } from 'vitest'
import { HeaderCanopy, SceneBackdrop } from './forest'

const ART = 'https://cards.scryfall.io/art_crop/front/0/a/test.jpg'

/**
 * The stylesheet, opened off the disk rather than imported.
 *
 * `import '../index.css?raw'` resolves to the **empty string** under vitest,
 * so the obvious guard reads nothing, finds nothing wrong and passes forever.
 * `lib/tokens.test.ts` carries the full argument for this escape hatch and for
 * why the path is relative to `web/`; this is the same three lines, doing the
 * same job for the one declaration below that no rendering test can reach.
 */
// @ts-expect-error -- node's types are out of scope for `src`; argued above.
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

beforeEach(() => {
  localStorage.clear()
})

afterEach(cleanup)

it('renders the room onto the body, not inside its parent', () => {
  const { container } = render(
    <div className="page-enter">
      <SceneBackdrop art={ART} />
    </div>,
  )

  const backdrop = document.querySelector('.scene-backdrop')
  expect(backdrop).not.toBeNull()
  // The assertion that matters: it is *not* under the animated wrapper. Both
  // halves are checked, because `container.querySelector` returning null
  // would also pass if the component had simply stopped rendering.
  expect(container.querySelector('.scene-backdrop')).toBeNull()
  expect(backdrop?.closest('.page-enter')).toBeNull()
})

it('carries both gallery lanes, and no third copy of the painting', () => {
  render(<SceneBackdrop art={ART} />)

  const images = [...document.querySelectorAll<HTMLImageElement>('.scene-backdrop img')]
  // **Two, and it used to be three.** The third was the full-viewport wash,
  // which was this painting again at `blur(11px)` — Scryfall's imagery
  // guidelines name blur outright, and ADR 48 replaced the image with nine
  // colours sampled out of it. The lanes are still the painting, and still
  // the same one, so the room adds no second source and needs no second
  // credit.
  expect(images).toHaveLength(2)
  expect(images.every((img) => img.getAttribute('src') === ART)).toBe(true)
  expect(document.querySelectorAll('.scene-lane')).toHaveLength(2)
})

it('does without a wash rather than breaking when the art cannot be read', () => {
  // jsdom has no canvas, so `useArtWash` takes the exact path a tainted or
  // failed read takes in a browser. The assertion is the fallback contract:
  // no wash element, no card image standing in for one, and the room's own
  // loop still rendering underneath. A missing wash is a quieter page.
  render(<SceneBackdrop art={ART} />)

  expect(document.querySelector('.scene-backdrop-wash')).toBeNull()
  expect(document.querySelector('.scene-backdrop')).not.toBeNull()
  expect(document.querySelectorAll('.scene-mist source').length)
    .toBeGreaterThan(0)
})

it('is decoration, so nothing in it reaches a screen reader', () => {
  render(<SceneBackdrop art={ART} />)

  expect(document.querySelector('.scene-backdrop')?.getAttribute('aria-hidden'))
    .toBe('true')
  // No alt text anywhere in the room: an `img` with alt="" inside an
  // aria-hidden container is silent, and that is the intent.
  const images = [...document.querySelectorAll<HTMLImageElement>('.scene-backdrop img')]
  expect(images.every((img) => img.getAttribute('alt') === '')).toBe(true)
  expect(screen.queryAllByRole('img')).toHaveLength(0)
})

it('drifts the mood the room asked for, and mist when it asked nothing', () => {
  // The mood is the whole prop: wrong sources here means the Laboratory's
  // candlelight silently reverts to forest mist, which no other test sees.
  render(<SceneBackdrop art={ART} mood="embers" />)
  let sources = [...document.querySelectorAll('.scene-mist source')]
  expect(sources.length).toBeGreaterThan(0)
  expect(sources.every((s) => s.getAttribute('src')?.includes('embers')))
    .toBe(true)
  cleanup()

  render(<SceneBackdrop art={ART} mood="wisps" />)
  sources = [...document.querySelectorAll('.scene-mist source')]
  expect(sources.every((s) => s.getAttribute('src')?.includes('wisps')))
    .toBe(true)
  cleanup()

  render(<SceneBackdrop art={ART} />)
  sources = [...document.querySelectorAll('.scene-mist source')]
  expect(sources.every((s) => s.getAttribute('src')?.includes('mist')))
    .toBe(true)
})

it('is removed entirely when ambience is switched off', () => {
  // The opt-out key `lib/prefs.ts` serves the forest from. Off means gone,
  // not stilled — a frozen room is just a picture nobody asked for.
  localStorage.setItem('mtglab-ambience', '0')

  render(<SceneBackdrop art={ART} />)

  expect(document.querySelector('.scene-backdrop')).toBeNull()
})

/*
 * The canopy's retraction (punch list 2026-08-18 item 3).
 *
 * What these can hold is the *class*, which is the whole of the logic: the
 * threshold, the listener and the bail-out. What they cannot hold is that the
 * class does anything — the roll-up is a `transform` in `index.css` and jsdom
 * has no layout, which is this file's opening lesson wearing a second hat.
 * Aaron's eye on a real browser is the guard for that half (commandment 16).
 */
function scrollTo(y: number) {
  window.scrollY = y
  fireEvent.scroll(window)
}

it('withdraws the growth as soon as the page moves', () => {
  render(<HeaderCanopy />)
  const canopy = document.querySelector('.header-canopy')
  expect(canopy?.className).not.toContain('is-retracted')

  scrollTo(40)
  expect(canopy?.className).toContain('is-retracted')

  // And unrolls on the way back, so the top of every page still has its vine.
  scrollTo(0)
  expect(canopy?.className).not.toContain('is-retracted')
})

it('arrives retracted when the page is restored mid-scroll', () => {
  // Routing can restore a scroll position before this mounts, in which case
  // no scroll event is ever fired and a canopy that waited for one would hang
  // over the page until the reader happened to move.
  window.scrollY = 400

  render(<HeaderCanopy />)

  expect(document.querySelector('.header-canopy')?.className)
    .toContain('is-retracted')
  window.scrollY = 0
})

/*
 * **The vine was widening every page on the site, and only arithmetic in a
 * live browser could say so.** The canopy box is the full width of the
 * viewport and two things inside it reach past its right edge: the sway
 * translates the leaf band 4px, and a shed leaf is placed at the x of the tap
 * that shook it loose — a thumb on the theme button lands within 12px of the
 * edge, and the leaf is 26 to 42px wide and drifts another 24 as it falls.
 * Measured at 375px: 5px of horizontal scroll from the sway alone, on every
 * route, on a 13-second cycle. A page that rubber-bands sideways is the first
 * thing anybody notices on a phone and the thing that makes the rest of it
 * feel cheap.
 *
 * This file's opening lesson said the guard for that class of bug "is the
 * comment in `index.css`, not an assertion here". For the one property the
 * whole fix rests on, it can be both — and the assertion is the half that
 * fails when somebody tidies the declaration away.
 */
it('clips the vine sideways, so nothing it holds can widen the page', () => {
  // `clip` and not `hidden`, and that is not a preference. `overflow-x: hidden`
  // forces the cross axis to `auto`, which would trap the shed leaves in an
  // 84px box they are supposed to fall 150px out of; `clip` is the one value
  // that pairs with a `visible` cross axis. Cut at the sides, open at the foot.
  expect(declares('.header-canopy', 'overflow-x')).toBe('clip')
})

it('hangs the leaf band wider than the box it sways inside', () => {
  // `inset: 0` plus the sway's `translateX(4px)` bares a 4px strip of nothing
  // at the left edge at one end of every cycle — one percent of a 375px phone,
  // and invisible on the 1400px desktop it was written on. The band repeats on
  // x, so the overhang costs a few more tiles of leaf and nothing else.
  const inset = declares('.canopy-photo', 'inset')
  expect(inset).toMatch(/-\d/)
})
