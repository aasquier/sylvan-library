/**
 * The canopy's answer to scrolling — one reading of the page's position,
 * shared by everything that hangs at the top of every route.
 *
 * The canopy is two things: the header bar, and the ivy draped along its
 * underside. They are one piece of furniture and they have to move like one,
 * so they read the same number from the same event rather than each keeping a
 * listener and hoping the two agree. (`HeaderCanopy` had the only listener
 * until the 2026-08-26 punch list asked for the bar to fold away too. A second
 * listener would have been the obvious edit and the wrong one — see #162,
 * where scroll listeners nobody counted brought this page to its knees.)
 *
 * So this is a store, not a hook-shaped closure: **one** window listener for
 * the whole app, however many components subscribe, attached when the first
 * one arrives and removed when the last one leaves. The rest of the discipline
 * from that original hook survives intact, because it is the whole reason this
 * is cheap:
 *
 *   - **Passive**, so a touch scroll is never waiting on us.
 *   - **No layout read per event.** `window.scrollY` is a scroll-position
 *     read, not a geometry one; nothing here calls `getBoundingClientRect`,
 *     so no event can force a synchronous layout.
 *   - **No state write unless the answer changed.** The snapshot keeps its
 *     object identity when both flags match, and `useSyncExternalStore`
 *     compares by identity — so a flick of the wheel costs no renders at all.
 *
 * Two flags, because the two halves want different thresholds:
 *
 *   `retracted` — the ivy rolls up almost immediately (8px). It hangs over
 *     the page's own first heading, so the moment the page moves it is in the
 *     way. Unchanged behaviour from the 2026-08-18 punch list.
 *
 *   `furled` — the bar folds up into the top edge, and unlike the ivy it is
 *     *direction-aware*: down folds it, up brings it straight back wherever
 *     you are, and near the top it is always open. A bar that only came back
 *     at the top of a page would be a trap, which is the failure this pattern
 *     is famous for.
 */

import { useSyncExternalStore } from 'react'

/** How far the page may move before the growth withdraws. Small on purpose:
 *  the ask was "as soon as you start scrolling", and anything larger reads as
 *  a lag rather than as a response. */
export const CANOPY_RETRACT_AT = 8

/** Under this the bar is always open, whichever way you are going. Enough room
 *  that the first nudge of a page never folds the chrome — the top of a page is
 *  where a reader is still deciding where to go, and that is the one moment the
 *  nav has to be there.
 *
 *  Deliberately NOT the bar's own height: measured at 375x812 the bar is 201px
 *  (a wordmark row and four wrapped rows of nav), and waiting that long to fold
 *  would spend a quarter of a phone screen answering a gesture already made.
 *  This is a distance the *reader* has travelled, not a size the bar has. It
 *  can be smaller than the bar because `sticky` pins the bar from the first
 *  pixel of scroll, so there is no range where it is still in its flow
 *  position and a transform would look odd. */
export const CANOPY_FURL_FLOOR = 96

/** Travel in one direction before the fold flips, in px.
 *
 *  A bare `y > lastY` comparison flickers, and not in the rig: a trackpad
 *  emits alternating one-pixel deltas, and iOS rubber-banding runs the number
 *  backwards through a bounce. Six pixels is under the smallest deliberate
 *  gesture and over all of that noise. */
export const CANOPY_FURL_TOLERANCE = 6

export interface CanopyState {
  /** The ivy has rolled up into the header. */
  retracted: boolean
  /** The header itself has folded up into the top edge. */
  furled: boolean
}

const OPEN: Readonly<CanopyState> = { retracted: false, furled: false }

const listeners = new Set<() => void>()

let snapshot: CanopyState = OPEN
/** The position the next reading is measured against. It moves only when the
 *  fold actually flips, so slow travel *accumulates* to the tolerance instead
 *  of being swallowed a pixel at a time. */
let anchor = 0

/** iOS overscroll reports a negative `scrollY` at the top of a page, and a
 *  negative number here would read as "scrolled up from nowhere". The floor is
 *  the fix, and it is why a bounce at the top cannot flicker. */
function clampedScrollY(): number {
  return Math.max(0, window.scrollY)
}

function read() {
  const y = clampedScrollY()
  let furled = snapshot.furled

  if (y <= CANOPY_FURL_FLOOR) {
    // The top of the page: open, unconditionally, and re-anchored so the first
    // real move away is measured from here.
    furled = false
    anchor = y
  } else {
    const delta = y - anchor
    if (delta > CANOPY_FURL_TOLERANCE) {
      furled = true
      anchor = y
    } else if (delta < -CANOPY_FURL_TOLERANCE) {
      furled = false
      anchor = y
    }
    // Inside the dead zone nothing moves, the anchor included.
  }

  const retracted = y > CANOPY_RETRACT_AT
  if (snapshot.retracted === retracted && snapshot.furled === furled) return
  snapshot = { retracted, furled }
  for (const notify of listeners) notify()
}

function subscribe(notify: () => void): () => void {
  if (listeners.size === 0) {
    window.addEventListener('scroll', read, { passive: true })
    // Routing can restore a scroll position before this mounts, in which case
    // no scroll event is ever fired and a canopy that waited for one would
    // hang over the page until the reader happened to move. A fresh mount has
    // no direction history either, so the fold starts open and earns its way
    // shut — never inherit a fold nobody in this session performed.
    snapshot = OPEN
    anchor = clampedScrollY()
    read()
  }
  listeners.add(notify)
  return () => {
    listeners.delete(notify)
    if (listeners.size === 0) {
      window.removeEventListener('scroll', read)
      // And put the store back the way it was found. Priming above already
      // says a fresh mount inherits no fold — but priming happens on
      // *subscribe*, and `useSyncExternalStore` subscribes in an effect, one
      // commit after it has read `getSnapshot` to render with. In that window
      // these two variables are the only answer available to a bar that is
      // already on the page, and a leftover fold there is a claim about a
      // scroll the reader looking at it never made.
      //
      // Nothing is watching the page now and nothing will update these until
      // somebody does, so leaving them warm buys nothing and costs exactly
      // that lie. Found because it made the header-fold cases in
      // `App.test.tsx` intermittent: with no subscriber attached the fold one
      // case performed was still sitting here for the next one to render.
      snapshot = OPEN
      anchor = 0
    }
  }
}

function getSnapshot(): CanopyState {
  return snapshot
}

/**
 * Read the canopy's state, and keep reading it. Every consumer gets the same
 * two booleans from the same listener, so the bar and the vine cannot
 * disagree about where the page is.
 */
export function useCanopyScroll(): CanopyState {
  return useSyncExternalStore(subscribe, getSnapshot)
}
