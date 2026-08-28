/**
 * Where a mark's panel goes.
 *
 * **This is the half of the change a suite can actually hold.** jsdom applies
 * no stylesheet and lays nothing out, so a component test can prove the panel
 * was rendered and can never prove it landed on the screen — which is exactly
 * how this project once shipped zones drawn at zero pixels with everything
 * green. Placement was pulled out into a pure function so that the arithmetic
 * is checkable here and only the *look* is left to Aaron's walk.
 */

import { expect, it } from 'vitest'

import { HINT_EDGE, HINT_GAP, HINT_W, placeHint } from './hint'

/** A rectangle, without the twenty fields a real `DOMRect` carries and none of
 *  which this function reads. */
function rect(left: number, top: number, w = 13, h = 13): DOMRect {
  return {
    left, top, width: w, height: h,
    right: left + w, bottom: top + h, x: left, y: top,
    toJSON: () => ({}),
  } as DOMRect
}

const PANEL = { w: HINT_W, h: 60 }

it('stands the panel over the mark when there is room above it', () => {
  // Above by preference, because these marks live in the *upper* corner of a
  // card in a lane — a panel that drops covers the two rows under it, which on
  // this board is the other player's half.
  const at = placeHint(rect(400, 300), 1280, 800, PANEL)
  expect(at.under).toBe(false)
  expect(at.top).toBe(300 - HINT_GAP - PANEL.h)
  // Centred on the mark: a 13px chip at 400 has its middle at 406.5.
  expect(at.left).toBe(406.5 - HINT_W / 2)
  expect(at.width).toBe(HINT_W)
})

it('drops it under a mark near the top of the window', () => {
  // The far player's half is the top of this room, and every mark in it is
  // within a panel's height of the ceiling.
  const at = placeHint(rect(400, 20), 1280, 800, PANEL)
  expect(at.under).toBe(true)
  expect(at.top).toBe(20 + 13 + HINT_GAP)
})

it('keeps the panel inside the window at either edge', () => {
  // A card in the first slot of a lane on a 375-wide phone: centring the panel
  // on it puts most of it off the left of the screen.
  const near = placeHint(rect(6, 400), 375, 700, PANEL)
  expect(near.left).toBe(HINT_EDGE)
  const far = placeHint(rect(360, 400), 375, 700, PANEL)
  expect(far.left + far.width).toBeLessThanOrEqual(375 - HINT_EDGE)
  // A phone still has room for the panel at its full width — 375 holds 208 and
  // both margins with plenty over — so nothing narrows here. What narrows is a
  // window that genuinely cannot hold it, and then the panel takes what is
  // left rather than hanging off the side.
  expect(far.width).toBe(HINT_W)
  const tight = placeHint(rect(90, 400), 180, 700, PANEL)
  expect(tight.width).toBe(180 - 2 * HINT_EDGE)
  expect(tight.left).toBe(HINT_EDGE)
})

it('never comes out inside out in a hidden tab', () => {
  // **A background tab reports a viewport of zero** — `clientWidth`,
  // `clientHeight` and every `vw` with them — and a panel sized against that
  // arithmetic comes out negative, which is a box drawn inside out. Nobody is
  // looking at a hidden tab; they are looking the instant it comes back, and
  // what arrives must not be the broken thing.
  const at = placeHint(rect(0, 0), 0, 0, PANEL)
  expect(at.width).toBe(HINT_W)
  expect(at.top).toBeGreaterThanOrEqual(0)
  expect(at.left).toBeGreaterThanOrEqual(0)
})

it('keeps the first line on screen when neither side fits', () => {
  // A window shorter than the panel is not a real phone, but a keyboard on a
  // short landscape screen is — and the answer that keeps the *word* visible is
  // the right one, because a panel hanging off the bottom is still read from
  // the top down and one placed off the top is not read at all.
  const at = placeHint(rect(100, 10), 640, 90, PANEL)
  expect(at.top).toBeGreaterThanOrEqual(HINT_EDGE)
  expect(at.top).toBeLessThanOrEqual(90 - HINT_EDGE - PANEL.h)
})
