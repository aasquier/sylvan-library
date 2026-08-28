/**
 * Clearing the table: the page takes the whole screen, and everything the
 * device draws around it goes away.
 *
 * Aaron, 2026-08-28: *"Can we do a fullscreen mode? I hate seeing all the web
 * tabs and what not."* That is one wish with **two different answers**, and
 * the split is the whole reason this file has a comment this long.
 *
 * ## The screen-filling call does not exist on an iPhone
 *
 * `Element.requestFullscreen` is not implemented on iPhone at all — not
 * unprefixed, not `webkit`-prefixed, not on any element. (An iPad has it; a
 * `<video>` has its own private door via `webkitEnterFullscreen`; an arbitrary
 * `<div>` on a phone has nothing.) So on the one device this was asked from,
 * there is no call to make: a button wired to it would be a button that does
 * nothing, which is worse than no button.
 *
 * The phone's answer is therefore not here at all — it is `web/index.html` and
 * `web/public/manifest.json`. Added to the home screen, the library launches
 * as its own thing with no address bar and no tabs, which is the exact wish,
 * permanently, with no control to press. `standing()` below is how the rest of
 * the app *notices* that has happened, so it does not go on offering a way to
 * do something already done.
 *
 * ## Where the call does exist, three things have to be true
 *
 * **1. It is asked for, never assumed.** Every engine requires a user gesture
 * — the request rejects outside one — so this can never be restored from a
 * saved preference on load. There is deliberately no `localStorage` key here;
 * it would be a preference the app could not honour.
 *
 * **2. The document is the authority, not us.** Escape, F11, the phone
 * rotating, the OS taking over — all of them leave fullscreen without asking,
 * and a control holding its own `useState` copy would then be lying about the
 * state of the screen. So the state is read back out of the document and the
 * subscription is the browser's own `fullscreenchange`, through
 * `useSyncExternalStore` — the same shape `lib/prefs.ts` uses, for the same
 * reason: the truth is outside React.
 *
 * **3. Both spellings, in both directions.** Safari answered only the
 * `webkit` names until 16.4 and still answers them; a build that reads
 * `document.fullscreenElement` and listens for `webkitfullscreenchange` — or
 * either mismatched pair — is a control that enters fullscreen and then
 * insists it did not. This repo's declared browser floor is 16.4 (see
 * `web/README.md`), so the prefixed half is belt to the unprefixed braces
 * rather than a browser anybody is promising to serve — it is kept because it
 * is four identifiers and because in-app browsers lag their engine.
 *
 * ## Why the whole document and not a subtree
 *
 * `document.documentElement`, always. The shell portals `position: fixed`
 * layers straight to `document.body` (`SceneBackdrop` does, and
 * `ForestAmbience` paints behind everything) — and a fixed element that is not
 * a descendant of the fullscreen element is simply not painted while
 * fullscreen is on. Fullscreening anything smaller would take the forest,
 * the weather and every portalled panel off the screen at once.
 */

import { useCallback, useSyncExternalStore } from 'react'

/** The document, with the names Safari has and the standard does not. */
type WebkitDocument = Document & {
  webkitFullscreenElement?: Element | null
  webkitFullscreenEnabled?: boolean
  webkitExitFullscreen?: () => Promise<void> | void
}

/** An element, likewise. */
type WebkitElement = HTMLElement & {
  webkitRequestFullscreen?: () => Promise<void> | void
}

/** The `standalone` flag iOS set for home-screen apps years before
 *  `display-mode` existed, and still sets. */
type LegacyNavigator = Navigator & { standalone?: boolean }

const EVENTS = ['fullscreenchange', 'webkitfullscreenchange'] as const

function doc(): WebkitDocument {
  return document as WebkitDocument
}

/**
 * Whether this browser will fill the screen on request.
 *
 * `fullscreenEnabled` rather than the presence of the method, because it is
 * the question that also answers "and is it allowed here" — inside an iframe
 * without the permission it is `false` while the method still exists, and a
 * control offered there would reject on every press.
 */
export function canClear(): boolean {
  if (typeof document === 'undefined') return false
  const d = doc()
  const allowed = d.fullscreenEnabled === true || d.webkitFullscreenEnabled === true
  const root = document.documentElement as WebkitElement
  const asks = typeof root.requestFullscreen === 'function'
    || typeof root.webkitRequestFullscreen === 'function'
  return allowed && asks
}

/** Whether the screen is filled right now. */
export function cleared(): boolean {
  if (typeof document === 'undefined') return false
  const d = doc()
  return Boolean(d.fullscreenElement ?? d.webkitFullscreenElement)
}

/**
 * Whether the app was launched from a home screen rather than opened in a
 * browser — in which case the chrome is already gone and there is nothing to
 * offer. The media query is the modern answer and `navigator.standalone` is
 * the one iOS has always had; either is enough.
 */
export function standing(): boolean {
  if (typeof window === 'undefined') return false
  const nav = navigator as LegacyNavigator
  if (nav.standalone === true) return true
  // Optional-chained: jsdom has no `matchMedia` at all, and the tests that
  // stub it hand back a bare `{ matches }` — so this asks for one field and
  // survives both.
  return ['standalone', 'fullscreen'].some(
    (mode) => window.matchMedia?.(`(display-mode: ${mode})`)?.matches === true)
}

function subscribe(fn: () => void): () => void {
  EVENTS.forEach((e) => document.addEventListener(e, fn))
  return () => EVENTS.forEach((e) => document.removeEventListener(e, fn))
}

/** Fill the screen, or give it back. Returns nothing and throws nothing: a
 *  request the engine refuses simply fires no event, so the control stays
 *  where it was rather than flickering into a state the screen is not in. */
export function toggleCleared(): void {
  const d = doc()
  // Branched rather than `a?.() ?? b?.()`: the prefixed calls are the old
  // ones and several of them return `undefined` rather than a promise, so a
  // nullish chain would run *both* on the browsers that have both.
  const ask = cleared()
    ? (d.exitFullscreen ?? d.webkitExitFullscreen)?.bind(d)
    : (() => {
      const root = document.documentElement as WebkitElement
      const fn = root.requestFullscreen ?? root.webkitRequestFullscreen
      return fn?.bind(root)
    })()
  if (!ask) return
  try {
    void Promise.resolve(ask()).catch(() => {})
  } catch {
    // A synchronous throw from an engine that never returned a promise.
  }
}

export interface Clearing {
  /** True where the screen can be filled on request, and there is still
   *  something around the page worth clearing. False on a phone that has no
   *  such call, and false inside a frame that is not allowed one. */
  offered: boolean
  /** True when the app was launched from a home screen, so the chrome is
   *  already gone and every offer of this is an offer to do nothing. */
  homescreen: boolean
  /** True while the screen is filled. Follows the document, so Escape and F11
   *  move it exactly as a press of the control does. */
  on: boolean
  /** Must be called from inside a user gesture. */
  toggle: () => void
}

/** The control's whole state. */
export function useClearing(): Clearing {
  const on = useSyncExternalStore(subscribe, cleared, () => false)
  const toggle = useCallback(() => toggleCleared(), [])
  // Neither of the two capabilities can change for the life of the document —
  // a browser does not grow the call, and a home-screen launch is decided
  // before the first paint — so they are read rather than subscribed to.
  const homescreen = standing()
  return { offered: canClear() && !homescreen, homescreen, on, toggle }
}
