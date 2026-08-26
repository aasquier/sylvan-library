/**
 * The canopy's scroll arithmetic — thresholds, direction, the dead zone, and
 * the two things #162 made non-negotiable.
 *
 * These hold the *logic*, which is genuinely all here: which way the page
 * moved, whether it moved far enough to mean it, and whether the answer
 * changed. What they cannot hold is that any of it looks like anything — the
 * fold is a `transform` in `index.css` and jsdom has no layout, so the class
 * is the whole of what an assertion here can see. Aaron's eye on a real
 * browser is the guard for the other half (commandment 16), and a phone is
 * where the dead zone below is actually decided: a trackpad's alternating
 * one-pixel deltas and iOS's rubber band are what these constants are for,
 * and neither of them exists in this file.
 *
 * What the numbers below deliberately do NOT do is restate the constants. The
 * cases import them, so moving a threshold moves the tests with it and none of
 * these can pass against a number nobody meant.
 */

import { cleanup, fireEvent, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { CanopyState } from './canopy'
import {
  CANOPY_FURL_FLOOR,
  CANOPY_FURL_TOLERANCE,
  CANOPY_RETRACT_AT,
  useCanopyScroll,
} from './canopy'

afterEach(() => {
  cleanup()
  window.scrollY = 0
  vi.restoreAllMocks()
})

function scrollTo(y: number) {
  window.scrollY = y
  fireEvent.scroll(window)
}

/** Far enough down that both halves have long since made up their minds. */
const DEEP = CANOPY_FURL_FLOOR + 400

it('holds the whole canopy open at the top of the page', () => {
  const { result } = renderHook(() => useCanopyScroll())

  expect(result.current).toEqual({ retracted: false, furled: false })
})

it('withdraws the ivy the moment the page moves, long before the bar folds', () => {
  const { result } = renderHook(() => useCanopyScroll())

  scrollTo(CANOPY_RETRACT_AT + 1)

  // The vine hangs over the page's own first heading, so it goes at once.
  expect(result.current.retracted).toBe(true)
  // The bar does not: a nudge is not a decision to leave, and near the top the
  // nav is the one thing a reader still deciding where to go needs to see.
  expect(result.current.furled).toBe(false)
})

it('will not fold the bar while the page is still near its top', () => {
  const { result } = renderHook(() => useCanopyScroll())

  scrollTo(CANOPY_FURL_FLOOR)

  expect(result.current.furled).toBe(false)
})

it('folds the bar on the way down', () => {
  const { result } = renderHook(() => useCanopyScroll())

  scrollTo(DEEP)

  expect(result.current.furled).toBe(true)
})

it('brings the bar straight back on the way up, deep in a page', () => {
  // The half that matters. A bar recoverable only by travelling all the way
  // back to the top of the page is the trap this pattern is famous for; up is
  // up, wherever you are.
  const { result } = renderHook(() => useCanopyScroll())
  scrollTo(DEEP)
  expect(result.current.furled).toBe(true)

  scrollTo(DEEP - 200)

  expect(result.current.furled).toBe(false)
  // And the ivy stays rolled up, because it is still hanging over page rather
  // than over the top of the document. The two halves agree; they are not the
  // same answer.
  expect(result.current.retracted).toBe(true)
})

it('opens everything again on returning to the top', () => {
  const { result } = renderHook(() => useCanopyScroll())
  scrollTo(DEEP)

  scrollTo(0)

  expect(result.current).toEqual({ retracted: false, furled: false })
})

it('ignores movement inside the dead zone, which is where a trackpad lives', () => {
  const { result } = renderHook(() => useCanopyScroll())
  scrollTo(DEEP)

  scrollTo(DEEP - CANOPY_FURL_TOLERANCE)

  // Exactly the tolerance is not past it. A bare `y < lastY` comparison would
  // have flipped here, and would flip back on the next stray pixel.
  expect(result.current.furled).toBe(true)
})

it('accumulates small moves in one direction rather than swallowing them', () => {
  // The anchor moves only when the fold flips, which is what makes four
  // two-pixel nudges upward a gesture instead of four pieces of noise.
  // Re-anchoring on every event would look like the same code and would mean
  // the tolerance could never be reached — the bar would be unrecoverable on a
  // slow, steady scroll, which is exactly how a phone scrolls at the end of a
  // flick.
  const { result } = renderHook(() => useCanopyScroll())
  scrollTo(DEEP)
  expect(result.current.furled).toBe(true)

  for (let step = 1; step <= 4; step++) scrollTo(DEEP - step * 2)

  expect(result.current.furled).toBe(false)
})

it('reads an overscroll bounce past the top as the top, not as a scroll upward', () => {
  // iOS reports a negative `scrollY` while a page rubber-bands at its top.
  // Unclamped that is arithmetic about a position above the document, and the
  // bounce back down then reads as a deliberate scroll away from it.
  const { result } = renderHook(() => useCanopyScroll())

  scrollTo(-120)

  expect(result.current).toEqual({ retracted: false, furled: false })
})

it('costs no renders while the answer is unchanged', () => {
  let renders = 0
  const { result } = renderHook(() => {
    renders++
    return useCanopyScroll()
  })
  scrollTo(DEEP)
  expect(result.current.furled).toBe(true)
  const settled = renders

  // Twenty events that cross nothing. #162 is why this is a case rather than a
  // sentence in a docstring: what a scroll listener costs is renders, not
  // comparisons, and nothing else in the suite would notice this going wrong.
  for (let i = 0; i < 20; i++) scrollTo(DEEP + (i % 3))

  expect(renders).toBe(settled)
})

it('leaves nothing folded behind when the last reader goes', () => {
  // The store outlives the components reading it, and `useSyncExternalStore`
  // reads `getSnapshot` **during the render** but only subscribes a commit
  // later. Between those two moments the store's leftovers are the only answer
  // a bar can render with — so a fold left in there is a lie about a page this
  // reader has not scrolled, and the next mounting wears it.
  //
  // That is not hypothetical: it is how one test's scroll became the next
  // test's starting position and made the header-fold cases in `App.test.tsx`
  // intermittent (#324). Priming on subscribe cannot cover it, because priming
  // is the thing that has not happened yet.
  const bar = renderHook(() => useCanopyScroll())
  scrollTo(DEEP)
  expect(bar.result.current.furled).toBe(true)

  bar.unmount()
  window.scrollY = 0

  // Every render this second reader performs, in order. `result.current` is no
  // use here — it is the settled value, after the subscription exists. The
  // first entry is the one under test.
  const seen: CanopyState[] = []
  renderHook(() => {
    const state = useCanopyScroll()
    seen.push(state)
    return state
  })

  expect(seen[0]).toEqual({ retracted: false, furled: false })
})

it('keeps one scroll listener however many things are watching', () => {
  const added = vi.spyOn(window, 'addEventListener')
  const removed = vi.spyOn(window, 'removeEventListener')
  const scrolls = (spy: typeof added) =>
    spy.mock.calls.filter(([type]) => type === 'scroll').length

  const bar = renderHook(() => useCanopyScroll())
  const ivy = renderHook(() => useCanopyScroll())

  // Two consumers, one listener — the whole reason this is a store and not a
  // closure each component keeps for itself.
  expect(scrolls(added)).toBe(1)

  bar.unmount()
  expect(scrolls(removed)).toBe(0) // the ivy is still watching

  ivy.unmount()
  expect(scrolls(removed)).toBe(1) // and now nobody is
})
