/**
 * What happens to a link made before decks had owners.
 *
 * `/decks/<slug>` addressed every deck for the whole life of this app, and
 * ADR 22 replaced it. The bundle ships with the server so no *client* is
 * stale — but a link somebody sent a friend is, a browser's history is, and
 * this instance has been driven for days. Answering those with "Nothing here"
 * would be the app losing decks that are still on the shelf.
 */

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useParams } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeckTile } from '../lib/api'
import DeckRedirect from './DeckRedirect'

vi.mock('../lib/api', async () => ({
  deckUrl: (await vi.importActual<typeof import('../lib/api')>('../lib/api')).deckUrl,
  api: { decks: vi.fn() },
}))

const { api } = await import('../lib/api')

function tile(owner: string, slug: string): DeckTile {
  return {
    owner, slug, name: `${owner}/${slug}`, shared: true, showcase: false,
    status: 'built', stage: 'curated', writable: false, needs_rationale: 0,
    commander: [], companion: null, bracket: null, total_cards: 99,
    land_count: 34, strategy: '', art_crop: null, color_identity: [],
    errors: 0, warnings: 0,
  }
}

/** Render the legacy route, with a landing page that reports where it went. */
function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/decks/:slug" element={<DeckRedirect />} />
        <Route path="/decks/:owner/:slug"
               element={<div>arrived at owner-qualified deck</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.mocked(api.decks).mockReset()
})

afterEach(cleanup)

describe('DeckRedirect', () => {
  it('sends a bare slug to the deck that has it', async () => {
    vi.mocked(api.decks).mockResolvedValue([tile('aasquier', 'goreclaw')])
    renderAt('/decks/goreclaw')
    expect(await screen.findByText('arrived at owner-qualified deck')).toBeTruthy()
  })

  it('prefers the first library the slug appears in', async () => {
    // Not arbitrary: `/api/decks` is the caller's own decks first, then the
    // showcase, then everybody else's, so first-match is exactly the
    // precedence a person wants — your own `goreclaw` before the
    // maintainer's, and the maintainer's before a stranger's.
    vi.mocked(api.decks).mockResolvedValue([
      tile('mitch', 'goreclaw'), tile('aasquier', 'goreclaw'),
    ])
    render(
      <MemoryRouter initialEntries={['/decks/goreclaw']}>
        <Routes>
          <Route path="/decks/:slug" element={<DeckRedirect />} />
          <Route path="/decks/:owner/:slug" element={<Landed />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(await screen.findByText('mitch/goreclaw')).toBeTruthy()
  })

  it('says the deck is not on any shelf rather than pretending to look', async () => {
    vi.mocked(api.decks).mockResolvedValue([tile('aasquier', 'goreclaw')])
    renderAt('/decks/nothing-like-this')
    await waitFor(() =>
      expect(screen.getByText(/No deck called/)).toBeTruthy())
    expect(screen.getByText('nothing-like-this')).toBeTruthy()
  })

  it('gives the same answer when the library cannot be listed', async () => {
    // A library that cannot be listed cannot be searched, and the honest
    // answer is the one an unknown slug gets: this link leads nowhere we can
    // reach. What it must not do is hang on a spinner forever.
    vi.mocked(api.decks).mockRejectedValue(new Error('boom'))
    renderAt('/decks/goreclaw')
    await waitFor(() => expect(screen.getByText(/No deck called/)).toBeTruthy())
  })
})

/** Reports the address it was reached at, so precedence is observable.
 *
 * Read from the route params rather than `window.location`: `MemoryRouter`
 * keeps its history in memory and never touches the browser's, so the address
 * bar in a test says nothing about where the app navigated to. */
function Landed() {
  const { owner, slug } = useParams()
  return <div>{`${owner}/${slug}`}</div>
}
