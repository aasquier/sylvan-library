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

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'
import { ErrorNote, Spinner } from './ui'

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
