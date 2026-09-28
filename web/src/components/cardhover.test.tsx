/**
 * The lag fix for the 99 (feature 2 of the 2026-08-18 batch): `CardHover`
 * used to register a capture-phase window scroll listener *per instance, on
 * mount* — two per card row, ~200 on a deck's cards tab, every one of them
 * run on every scroll frame. The listener exists only to clear an open
 * preview, so it is registered only while one is showing. This test is the
 * regression pin: mounting many hovers adds no listeners at all.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { CardHover } from './ui'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const card = (name: string) => ({ name, image: `https://example.test/${name}.jpg` })

function scrollListenerCount(spy: ReturnType<typeof vi.spyOn>): number {
  return spy.mock.calls.filter(([type]: unknown[]) => type === 'scroll').length
}

it('registers no scroll listener until a preview is actually showing', () => {
  const added = vi.spyOn(window, 'addEventListener')
  render(
    <>
      {Array.from({ length: 40 }, (_, i) => (
        <CardHover key={i} card={card(`c${i}`)}>
          <span>row {i}</span>
        </CardHover>
      ))}
    </>,
  )
  // Forty mounted hovers, zero listeners — this is the whole fix.
  expect(scrollListenerCount(added)).toBe(0)

  fireEvent.mouseEnter(screen.getByText('row 3'), { clientX: 10, clientY: 10 })
  expect(scrollListenerCount(added)).toBe(1)
})

it('removes the listener when the preview closes', () => {
  const added = vi.spyOn(window, 'addEventListener')
  const removed = vi.spyOn(window, 'removeEventListener')
  render(
    <CardHover card={card('lion')}>
      <span>row</span>
    </CardHover>,
  )
  const row = screen.getByText('row')
  fireEvent.mouseEnter(row, { clientX: 10, clientY: 10 })
  expect(scrollListenerCount(added)).toBe(1)

  fireEvent.mouseLeave(row)
  expect(scrollListenerCount(removed)).toBe(1)
})

/**
 * And the third hand.
 *
 * Counted on a real deck page on 2026-09-27: a row in the 99 held no link, no
 * button, no input and no `tabindex` — nothing at all. The mouse had the
 * cursor preview and a thumb had the sheet, and a person on a keyboard could
 * not land on a single card, let alone see the painting, which is the one
 * thing that answers *what is this card* (commandment 2).
 */

it('takes no tab stop unless the caller asks for one', () => {
  render(
    <CardHover card={card('bear')}>
      <span>Bear</span>
    </CardHover>,
  )
  // The default is unchanged on purpose: twenty-odd other call sites wrap a
  // control already, or a wrapper with no box to draw a ring around.
  expect(screen.queryByRole('button')).toBeNull()
})

it('is reachable, and Enter holds the card up', () => {
  render(
    <CardHover card={card('bear')} reachable>
      <span>Bear</span>
    </CardHover>,
  )
  const hold = screen.getByRole('button')
  expect(hold.tabIndex).toBe(0)
  expect(hold.className).toMatch(/\bcard-reach\b/)

  fireEvent.keyDown(hold, { key: 'Enter' })
  // The centred sheet, not the cursor preview: a keyboard has no cursor for
  // the preview to sit beside.
  expect(screen.getByRole('dialog', { name: 'bear' })).toBeTruthy()
})

it('gives a keyboard nothing to land on when there is no painting to show', () => {
  render(
    <CardHover card={{ name: 'unpainted' }} reachable>
      <span>Unpainted</span>
    </CardHover>,
  )
  // A stop that opens nothing is worse than no stop; the row is still read.
  expect(screen.queryByRole('button')).toBeNull()
  expect(screen.getByText('Unpainted')).toBeTruthy()
})
