/**
 * The net under the routed page.
 *
 * Two failures are pinned here, a day apart and both watched on Aaron's
 * phone. On 2026-08-16 a lazy route chunk failed to fetch, the rejection
 * reached no boundary, and React unmounted the root — a black page whose
 * navigation was dead with it. On 2026-08-19 the net itself was the fault:
 * the reload guard was written once and cleared by nothing, so a tab that
 * had ever seen one blip could never be healed by a reload again, and every
 * later hiccup went straight to the card. iOS Safari holds a tab's
 * `sessionStorage` for weeks, which is how one deploy restart in the morning
 * broke deck navigation all day.
 *
 * The contract: a failure with no reload behind it in the last minute
 * reloads the document once, a failure that follows a reload renders a card
 * that names no chunk, module, or network, and the way out is a real
 * `<a href="/">` that owes nothing to the tree that fell over.
 */

import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { RouteErrorBoundary } from './boundary'

function Bomb(): never {
  throw new Error('chunk failed to load')
}

const reload = vi.fn()

beforeEach(() => {
  sessionStorage.clear()
  reload.mockClear()
  // jsdom's location.reload is not configurable per-property, so swap the
  // whole object; each test that needs it restores nothing — jsdom is torn
  // down per file and the double is inert.
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...window.location, reload },
  })
  // React logs every caught error to the console by design; the suite
  // treats stderr noise as a smell, so keep these two quiet on purpose.
  vi.spyOn(console, 'error').mockImplementation(() => undefined)
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('renders its children when nothing throws', () => {
  render(
    <RouteErrorBoundary>
      <p>the page</p>
    </RouteErrorBoundary>,
  )
  expect(screen.getByText('the page')).toBeTruthy()
  expect(reload).not.toHaveBeenCalled()
})

it('reloads once, silently, on the first failure of a session', () => {
  render(
    <RouteErrorBoundary>
      <Bomb />
    </RouteErrorBoundary>,
  )
  expect(reload).toHaveBeenCalledTimes(1)
  // Silently: no card while the reload is on its way.
  expect(screen.queryByText(/slipped off the shelf/i)).toBeNull()
})

it('shows the card, not a second reload, when a reload just happened', () => {
  sessionStorage.setItem('sylvan-route-reloaded', String(Date.now()))
  render(
    <RouteErrorBoundary>
      <Bomb />
    </RouteErrorBoundary>,
  )
  expect(reload).not.toHaveBeenCalled()
  expect(screen.getByText(/slipped off the shelf/i)).toBeTruthy()
  // The way back is a document load, not a router link: it must work even
  // though the tree it is rendered in just proved untrustworthy.
  const link = screen.getByRole('link', { name: /return to the library/i })
  expect(link.getAttribute('href')).toBe('/')
})

it('heals again once the episode behind the last reload has passed', () => {
  // The 2026-08-19 bug in one line: this stamp used to be a bare '1' that
  // outlived the failure it described, and a phone that had seen one blip
  // was never offered a reload again for the life of the tab.
  sessionStorage.setItem('sylvan-route-reloaded', String(Date.now() - 120_000))
  render(
    <RouteErrorBoundary>
      <Bomb />
    </RouteErrorBoundary>,
  )
  expect(reload).toHaveBeenCalledTimes(1)
  expect(screen.queryByText(/slipped off the shelf/i)).toBeNull()
})

it('treats the one-way flag left by the older build as long expired', () => {
  // Tabs open since before the fix carry a literal '1'. It reads as one
  // millisecond past the epoch, so those tabs get the heal they have been
  // denied rather than the card they have been stuck on.
  sessionStorage.setItem('sylvan-route-reloaded', '1')
  render(
    <RouteErrorBoundary>
      <Bomb />
    </RouteErrorBoundary>,
  )
  expect(reload).toHaveBeenCalledTimes(1)
})

it('shows the card when the stamp cannot be read as a time', () => {
  // Declining to reload is the safe side of an unreadable guard: a reload
  // that cannot be recorded is a reload that can repeat forever.
  sessionStorage.setItem('sylvan-route-reloaded', 'once upon a time')
  render(
    <RouteErrorBoundary>
      <Bomb />
    </RouteErrorBoundary>,
  )
  expect(reload).not.toHaveBeenCalled()
  expect(screen.getByText(/slipped off the shelf/i)).toBeTruthy()
})

it('shows the card rather than risk a reload loop when the guard cannot be written', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('private browsing')
  })
  render(
    <RouteErrorBoundary>
      <Bomb />
    </RouteErrorBoundary>,
  )
  expect(reload).not.toHaveBeenCalled()
  expect(screen.getByText(/slipped off the shelf/i)).toBeTruthy()
})
