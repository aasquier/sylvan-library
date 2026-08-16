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

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it } from 'vitest'
import { SceneBackdrop } from './forest'

const ART = 'https://cards.scryfall.io/art_crop/front/0/a/test.jpg'

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

it('carries the page art and both gallery lanes', () => {
  render(<SceneBackdrop art={ART} />)

  const images = [...document.querySelectorAll<HTMLImageElement>('.scene-backdrop img')]
  // The wash plus the two lanes, all the same painting — the room adds no
  // second source and so needs no second credit.
  expect(images).toHaveLength(3)
  expect(images.every((img) => img.getAttribute('src') === ART)).toBe(true)
  expect(document.querySelectorAll('.scene-lane')).toHaveLength(2)
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

it('is removed entirely when ambience is switched off', () => {
  // The opt-out key `lib/prefs.ts` serves the forest from. Off means gone,
  // not stilled — a frozen room is just a picture nobody asked for.
  localStorage.setItem('mtglab-ambience', '0')

  render(<SceneBackdrop art={ART} />)

  expect(document.querySelector('.scene-backdrop')).toBeNull()
})
