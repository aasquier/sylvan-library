/**
 * The net under the routed page.
 *
 * The failure this pins was watched happen (2026-08-16, Aaron's phone):
 * a lazy route chunk failed to fetch, the rejection reached no boundary,
 * and React unmounted the root — a black page whose navigation was dead
 * with it. The contract here: the first failure in a session reloads the
 * document once (a deploy just finished, a connection blinked — a fresh
 * load heals both), a failure after that renders a card that names no
 * chunk, module, or network, and the way out is a real `<a href="/">`
 * that owes nothing to the tree that fell over.
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

it('shows the card, not a second reload, when the guard is already spent', () => {
  sessionStorage.setItem('sylvan-route-reloaded', '1')
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
