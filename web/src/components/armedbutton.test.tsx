/** The arm on every destructive control, and the gesture that used to beat it.
 *
 * `ArmedButton` is ADR 27's confirmation: the first click arms, the second
 * acts, and one accidental click costs nothing but a red button for four
 * seconds. Three callers wear it — "Start over" in the reading room, the bulk
 * entomb, and `Exile`, which is the only permanent delete left in the app and
 * whose neighbouring comment calls exiling "two deliberate steps by
 * construction".
 *
 * **It was one step.** A double-click is a single gesture that delivers two
 * `click` events, so it armed and confirmed in one stroke. Driven on the
 * deployed site on 2026-08-30 against a real deck: one `double_click` on
 * "Entomb 1 selected" moved a card to the graveyard, 98 cards to 97. Nothing
 * in the suite spoke, because every test that had ever pressed this component
 * pressed it the way a careful person does — twice, slowly, in separate acts.
 *
 * So the arm has a floor as well as a ceiling now, and these tests drive the
 * gesture rather than the intent: the two clicks land a measured distance
 * apart and the assertion is about which side of `DWELL` they fell.
 */

import { cleanup, render, screen, fireEvent, act } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ArmedButton } from './ui'

afterEach(cleanup)

/** Renders the button and hands back the spy and the element. */
function armed() {
  const onConfirm = vi.fn()
  render(
    <ArmedButton armedLabel="Gone forever?" onConfirm={onConfirm}>
      Exile
    </ArmedButton>,
  )
  return { onConfirm, button: () => screen.getByRole('button') }
}

/** Advances both the timers and the clock the dwell is measured against. */
async function wait(ms: number) {
  await act(async () => { await vi.advanceTimersByTimeAsync(ms) })
}

describe('the arm', () => {
  it('does nothing on the first click except arm', () => {
    const { onConfirm, button } = armed()

    expect(button().textContent).toBe('Exile')
    expect(button().getAttribute('aria-pressed')).toBe('false')

    fireEvent.click(button())

    expect(button().textContent, 'the label names the consequence').toBe('Gone forever?')
    expect(button().getAttribute('aria-pressed')).toBe('true')
    expect(onConfirm, 'and nothing has happened yet').not.toHaveBeenCalled()
  })

  it('refuses the back half of a double-click, and stays armed for a real one', async () => {
    vi.useFakeTimers()
    try {
      const { onConfirm, button } = armed()

      // The gesture: two clicks 40ms apart, which is one stroke of a finger
      // and well inside every platform's double-click threshold.
      fireEvent.click(button())
      await wait(40)
      fireEvent.click(button())

      expect(onConfirm, 'a double-click is one gesture, not two decisions')
        .not.toHaveBeenCalled()
      expect(button().getAttribute('aria-pressed'), 'and the arm is still up, not silently spent').toBe('true')

      // The person now looks at "Gone forever?", decides, and clicks. That
      // click must still work — the guard drops the stray event, never the
      // intent behind the next one.
      await wait(600)
      fireEvent.click(button())

      expect(onConfirm, 'the deliberate second click still fires').toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('fires on a second click once the dwell has passed', async () => {
    vi.useFakeTimers()
    try {
      const { onConfirm, button } = armed()

      fireEvent.click(button())
      await wait(600)
      fireEvent.click(button())

      expect(onConfirm).toHaveBeenCalledTimes(1)
      expect(button().getAttribute('aria-pressed'), 'and it disarms behind itself').toBe('false')
    } finally {
      vi.useRealTimers()
    }
  })

  it('still lets the arm time out rather than staying cocked', async () => {
    vi.useFakeTimers()
    try {
      const { onConfirm, button } = armed()

      fireEvent.click(button())
      expect(button().getAttribute('aria-pressed')).toBe('true')

      // The ceiling ADR 27 argued: a button left armed yesterday may not fire
      // today. Unchanged by the floor.
      await wait(4100)

      expect(button().getAttribute('aria-pressed'), 'the arm lapses on its own').toBe('false')
      expect(button().textContent).toBe('Exile')
      expect(onConfirm).not.toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })
})
