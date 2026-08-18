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
