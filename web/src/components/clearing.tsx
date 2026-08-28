/**
 * The two faces of "clear the table" — the header's own control, and the row
 * in the settings panel that says what it is for.
 *
 * `lib/fullscreen.ts` carries the argument for why this is two things and not
 * one; the short version is that the call it presses **does not exist on an
 * iPhone**, so the control has to know when to stand down and hand the job to
 * the home screen instead. What that means here:
 *
 * - `ClearingButton` renders in the header **only where the screen can
 *   actually be filled**. A control that does nothing is worse than no
 *   control, and hiding it costs a phone nothing because a phone gets the
 *   better answer anyway.
 * - The sentence goes in the settings panel, where there is room for one and
 *   where the panel's own switch can carry it. `CLEARING_HINT` is what it says
 *   under the switch on a device that has the button; `ClearingNote` is the
 *   two-tap route to the same result on a device that cannot. Visible text,
 *   not a `title`: `components/hint.tsx` is the standing note on why a tooltip
 *   reaches exactly one of the three hands that arrive here, and the one that
 *   needs it least.
 *
 * The glyph is four corner brackets, and it is the whole state readout for a
 * sighted reader: they face outward while the page is boxed in and inward
 * while it owns the screen, and they lean the way they are about to go when a
 * pointer arrives (`.clearing-corner` in the stylesheet). `aria-pressed`
 * carries the same fact to a screen reader, with **one unchanging name** —
 * a toggle button that renames itself as it flips is announced as a different
 * control every press.
 */

import { useEffect } from 'react'

import { useClearing } from '../lib/fullscreen'

/** The key that does it without the pointer. Lower-cased before the compare,
 *  so a held Shift still works. */
const KEY = 'f'

/** Outward-facing brackets: the page is boxed in, pressing this opens it up.
 *  One quadrant; the other three are the same path rotated. */
const BOXED = 'M -6 -2.6 L -6 -6 L -2.6 -6'
/** Inward-facing: the page has the screen, pressing this gives it back. */
const OPEN = 'M -6 -2.6 L -2.6 -2.6 L -2.6 -6'

function ClearingGlyph({ on }: { on: boolean }) {
  return (
    <svg width="16" height="16" viewBox="-8 -8 16 16" aria-hidden
         fill="none" stroke="currentColor" strokeWidth="1.7"
         strokeLinecap="round" strokeLinejoin="round">
      {/* The rotation rides on the group and the hover lean on the path
          inside it. They cannot share an element: an SVG `transform`
          attribute and the CSS `transform` property are one property, so a
          stylesheet rule on this path would *replace* the rotation and stack
          all four brackets in the same corner. */}
      {[0, 90, 180, 270].map((angle) => (
        <g key={angle} transform={`rotate(${angle})`}>
          <path className="clearing-corner" d={on ? OPEN : BOXED} />
        </g>
      ))}
    </svg>
  )
}

/**
 * The header control. Nothing at all where the screen cannot be filled.
 *
 * The keyboard route lives here rather than in the shell because this is the
 * one component that renders exactly once and only where the key would work.
 * Every guard on it is about not stealing a letter from somebody typing: no
 * modifier held, no auto-repeat, and never while the caret is in a field —
 * this app has a card search, a rationale box on every card, and an import
 * pane that is nothing but a giant textarea.
 */
export function ClearingButton() {
  const { offered, on, toggle } = useClearing()

  useEffect(() => {
    if (!offered) return
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || e.repeat) return
      if (e.key.toLowerCase() !== KEY) return
      const at = e.target as HTMLElement | null
      const tag = at?.tagName
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT'
        || at?.isContentEditable) return
      e.preventDefault()
      toggle()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [offered, toggle])

  if (!offered) return null
  return (
    <button type="button"
            onClick={toggle}
            aria-pressed={on}
            aria-label="Clear the table"
            className="btn btn-quiet btn-sm clearing-btn">
      <ClearingGlyph on={on} />
    </button>
  )
}

/** What the switch says under its label where the button exists. Lives beside
 *  the note below so the two halves of one idea are edited together, and is
 *  rendered by the settings panel through its own `Toggle` — one switch
 *  component for every row in that panel, rather than a fifth hand-built one
 *  that drifts. */
export const CLEARING_HINT =
  'The library takes the whole screen and everything around it steps away. '
  + 'The F key does it too.'

/**
 * What a device with no such call is told instead — which is every iPhone.
 *
 * Not a shortcoming to apologise for and deliberately not phrased as one: the
 * home screen gives a *better* version of the same wish, because it lasts and
 * needs no control at all. Said as two taps rather than as a capability that
 * is missing (commandment 2).
 */
export function ClearingNote() {
  return (
    <div className="rounded-md px-1.5 py-1.5">
      <span className="block text-[12px] font-medium"
            style={{ color: 'var(--text-primary)' }}>Clear the table</span>
      <span className="block text-[11px]" style={{ color: 'var(--text-muted)' }}>
        Tap the share button and choose “Add to Home Screen”. The library then
        opens on its own from there, with nothing around it — no address bar,
        no tabs.
      </span>
    </div>
  )
}
