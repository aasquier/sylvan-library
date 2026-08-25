/**
 * The two shared primitives that speak for every waiting surface in the app.
 *
 * Commandment 2's "shut out" includes shut out by a screen reader, and until
 * this pair carried a role the app had **no live region anywhere**: not one
 * `aria-live`, `role="status"`, `role="alert"` or `aria-busy` in `web/src`,
 * across thirty-odd surfaces where an answer arrives after an await. A sighted
 * person sees a ring start turning and sees the answer replace it; somebody
 * using a reader got silence, then silence.
 *
 * `Spinner` and `ErrorNote` are where the fix belongs because they are the
 * whole app's waiting and the whole app's refusal — twenty and twenty-three
 * call sites — so the roles are stated once and reach all of them. These tests
 * hold that: an announcement is not a thing you can see in a screenshot, so if
 * a refactor drops the role nothing else in the suite would ever notice.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { CardHover, ErrorNote, Spinner } from './ui'

afterEach(cleanup)

it('says what it is waiting for, in a region a reader is watching', () => {
  render(<Spinner label="Reading the colour guide…" />)
  // Found by role, not by class: what matters is the thing a screen reader
  // walks, and `status` is what makes the label an announcement rather than
  // text that happens to appear.
  expect(screen.getByRole('status').textContent).toBe('Reading the colour guide…')
})

it('announces nothing when there is nothing to announce', () => {
  render(<Spinner />)
  // A wait with no words is still a live region -- it just has nothing to say,
  // which is correct. The assertion that matters is that it does not invent
  // something: an unlabelled spinner must not read out its own markup.
  expect(screen.getByRole('status').textContent).toBe('')
})

it('keeps the turning ring out of the reader\'s way', () => {
  render(<Spinner label="Gathering the readers…" />)
  const ring = document.querySelector('.animate-spin')
  expect(ring).not.toBeNull()
  // The ring is the picture of the wait; the label is the wait. Without this
  // the region has a child element with no accessible name inside it, which is
  // noise in the one place noise costs the most.
  expect(ring?.getAttribute('aria-hidden')).toBe('true')
})

it('interrupts for a refusal, because a failure that waits its turn is a wait', () => {
  render(<ErrorNote>The reading could not be dealt.</ErrorNote>)
  // `alert` rather than `status`: everywhere else polite is right, and here it
  // is not -- somebody is still waiting for an answer that is never coming.
  expect(screen.getByRole('alert').textContent).toBe('The reading could not be dealt.')
})

/** A tap, the way a phone makes one. jsdom has no PointerEvent of its own, so
 *  the type rides on a plain Event — which is all the handler reads. */
function tap(el: Element) {
  for (const type of ['pointerdown', 'pointerup']) {
    const ev = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(ev, 'pointerType', { value: 'touch' })
    fireEvent(el, ev)
  }
}

const CARD = { name: 'Jareth, Leonine Titan', image: 'https://example.test/j.jpg' }

it('hands the whole card to a tap, for the half of the room with no cursor', () => {
  // `CardHover` was `onMouseEnter` and nothing else, which meant that on a
  // phone the card behind a 96x64 crop was unreachable in twenty-three places.
  // A hover-only mechanism locks out every touch user, and here it was the
  // only answer to "what is this card" — the newcomer's first question.
  render(<CardHover card={CARD}><span>tile</span></CardHover>)
  expect(screen.queryByRole('dialog')).toBeNull()

  tap(screen.getByText('tile'))

  const sheet = screen.getByRole('dialog', { name: CARD.name })
  expect(sheet).toBeTruthy()
  expect(screen.getByAltText(CARD.name).getAttribute('src')).toBe(CARD.image)
})

it('leaves the tap alone where the tile is already a control', () => {
  // Four of the twenty-three wrap a real button — the commander picker on the
  // create flow, and the reading room's tiles. There a tap already means
  // *choose this*, and stealing it would break the newcomer's most important
  // decision to show them a picture they never asked for.
  const chosen: string[] = []
  render(
    <CardHover card={CARD} tapOpens={false}>
      <button type="button" onClick={() => chosen.push(CARD.name)}>pick</button>
    </CardHover>)

  tap(screen.getByRole('button', { name: 'pick' }))
  expect(screen.queryByRole('dialog')).toBeNull()

  fireEvent.click(screen.getByRole('button', { name: 'pick' }))
  expect(chosen).toEqual([CARD.name])
})

it('closes the held card on a tap anywhere and on Escape', () => {
  render(<CardHover card={CARD}><span>tile</span></CardHover>)

  tap(screen.getByText('tile'))
  fireEvent.click(screen.getByRole('dialog'))
  expect(screen.queryByRole('dialog')).toBeNull()

  tap(screen.getByText('tile'))
  fireEvent.keyDown(window, { key: 'Escape' })
  expect(screen.queryByRole('dialog')).toBeNull()
})
