/**
 * A mark that explains itself, driven the three ways a person arrives at it.
 *
 * **The assertions are about reach rather than about words.** The sentences
 * these panels carry were all readable before — they were `title` attributes,
 * and a test that read one off the DOM passed happily while a phone and a
 * keyboard got nothing at all. So what is held here is the mechanism: that the
 * trigger is operable, that a tap pins the panel up and a second tap puts it
 * down, and that Escape does not travel on to the card underneath.
 *
 * What no test here can hold is where any of it lands — jsdom has no layout.
 * `lib/hint.test.ts` holds the arithmetic; the pixels are Aaron's walk.
 */

import { cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { FieldHint } from './hint'

afterEach(cleanup)

/** The panel, wherever in the document it ended up — which is the body, and
 *  that is the point of it. */
const panel = () => document.body.querySelector('[role="tooltip"]')

function mark(props: Partial<Parameters<typeof FieldHint>[0]> = {}) {
  const { getByRole } = render(
    <FieldHint name="Flying" {...props}
               says="can only be blocked by creatures with flying or reach">
      <svg />
    </FieldHint>)
  return getByRole('button')
}

it('is a button, so the keyboard can reach it at all', () => {
  // The fault this replaced, stated as a test: the marks were `<span>`s with a
  // `title`, so a keyboard could not reach them and would have got nothing if
  // it had. Everything else in this file depends on this one line being true.
  const button = mark()
  expect(button.tagName).toBe('BUTTON')
  expect(button.getAttribute('aria-label')).toBe('Flying')
  expect(panel()).toBeNull()
})

it('raises the panel on focus and puts it down on blur', () => {
  const button = mark()
  fireEvent.focus(button)
  expect(panel()?.textContent).toContain('blocked by creatures with flying')
  expect(button.getAttribute('aria-describedby')).toBe(panel()?.id)
  fireEvent.blur(button)
  expect(panel()).toBeNull()
})

it('pins on a tap and lets the same tap put it down again', () => {
  // **A phone has no hover to leave**, so the panel a tap raises has to be
  // dismissible by the same finger. This is the fault the zone trays hit first
  // (Aaron, 2026-08-26: a tray was "awkward to get it to collapse again"), and
  // it arrives here for the same reason.
  const button = mark()
  fireEvent.pointerDown(button, { pointerType: 'touch' })
  fireEvent.pointerUp(button, { pointerType: 'touch' })
  fireEvent.click(button)
  expect(panel()).not.toBeNull()
  // The synthetic `mouseenter` a touch browser fires after a tap must not
  // re-raise what the next tap is about to close.
  fireEvent.mouseEnter(button)
  fireEvent.click(button)
  expect(panel()).toBeNull()
})

it('stays up when a pinned mark loses the pointer', () => {
  const button = mark()
  fireEvent.click(button)
  fireEvent.mouseLeave(button)
  expect(panel(), 'a pinned panel is not held up by the pointer').not.toBeNull()
})

it('shuts on a tap somewhere else, and on Escape', () => {
  const button = mark()
  fireEvent.click(button)
  fireEvent.pointerDown(document.body)
  expect(panel()).toBeNull()

  fireEvent.click(button)
  fireEvent.keyDown(document, { key: 'Escape' })
  expect(panel()).toBeNull()
})

it('keeps its keys, its taps and its focus to itself', () => {
  // **The marks stand inside `.field-card`**, which has handlers of its own for
  // pointer-up, key-down, focus and blur. Without this a tap on a mark would
  // also lift the whole card into a sheet, one Escape would put the panel
  // down, the card down and the pile it came out of shut — and reaching a mark
  // with the keyboard would arm the card's three-hundred-pixel hover preview
  // right where the panel is about to be.
  //
  // **Focus is the one that needs saying**, because `onFocus` and `onBlur` are
  // `focusin` and `focusout` and both bubble, which is not what the names
  // suggest and is exactly the sort of thing that is discovered on a live
  // board rather than here.
  const heard = vi.fn()
  const { getByRole } = render(
    <div onPointerUp={heard} onKeyDown={heard} onClick={heard}
         onFocus={heard} onBlur={heard}>
      <FieldHint name="Flying" says="blocked only by flying or reach">
        <svg />
      </FieldHint>
    </div>)
  const button = getByRole('button')
  fireEvent.pointerDown(button)
  fireEvent.pointerUp(button)
  fireEvent.click(button)
  fireEvent.keyDown(button, { key: 'Escape' })
  fireEvent.keyDown(button, { key: 'Enter' })
  fireEvent.focus(button)
  fireEvent.blur(button)
  expect(heard, 'the card underneath heard the mark being used').not
    .toHaveBeenCalled()
})

it('tells the card to stand its own preview down while a panel is up', () => {
  // The card's hover preview is a 300px face placed against the same card, so
  // the two panels would arrive on top of each other. The pair has to be
  // symmetric: a mark that raised a panel and never said it had put it down
  // leaves the card's preview muted for the rest of the match.
  const onOpen = vi.fn()
  const onShut = vi.fn()
  const button = mark({ onOpen, onShut })
  fireEvent.focus(button)
  expect(onOpen).toHaveBeenCalledTimes(1)
  expect(onShut).not.toHaveBeenCalled()
  fireEvent.blur(button)
  expect(onShut).toHaveBeenCalledTimes(1)
})

it('draws the lent line only when the keyword was lent', () => {
  const button = mark({ note: 'granted — not printed on this card' })
  fireEvent.focus(button)
  expect(panel()?.textContent).toContain('not printed on this card')
  cleanup()
  fireEvent.focus(mark())
  expect(panel()?.textContent).not.toContain('not printed')
})
