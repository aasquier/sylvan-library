/**
 * The About Claude page, and the honesty properties worth pinning:
 *
 * - **every hotlinked painting renders its artist's name** — the credit is
 *   the licence made visible, and a gallery piece added without one should
 *   fail here rather than ship uncredited;
 * - the commander exhibits show **the pool's card, not the prose's memory
 *   of it** — the case renders only an exact name match, so a lookalike
 *   from the search is not dressed up as the favourite;
 * - with no pool to ask, the case **says so** instead of quoting rules text
 *   from recall — the same rule the Keeper's exhibit follows.
 */

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Card } from '../lib/api'
import { COMMANDERS, GALLERY } from '../lib/claudefavorites'
import AboutClaude from './AboutClaude'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { searchCards: vi.fn() } }
})

const { api } = await import('../lib/api')

const KWAIN: Card = {
  name: 'Kwain, Itinerant Meddler', category: '', why: '', qty: 1, known: true,
  mana_cost: '{W}{U}', type_line: 'Legendary Creature — Rabbit Wizard',
  oracle_text: '{T}: Each player may draw a card, then each player who drew '
    + 'a card this way gains 1 life.',
  color_identity: ['U', 'W'], image: null, art_crop: null,
}

const TATYOVA: Card = {
  name: 'Tatyova, Benthic Druid', category: '', why: '', qty: 1, known: true,
  mana_cost: '{3}{G}{U}', type_line: 'Legendary Creature — Merfolk Druid',
  oracle_text: 'Landfall — Whenever a land you control enters, you gain 1 '
    + 'life and draw a card.',
  color_identity: ['G', 'U'], image: null, art_crop: null,
}

/** A near-miss the case must not exhibit as the real card. */
const IMPOSTOR: Card = { ...KWAIN, name: 'Kwain the Builder' }

function renderPage() {
  return render(<MemoryRouter><AboutClaude /></MemoryRouter>)
}

beforeEach(() => {
  vi.mocked(api.searchCards).mockImplementation(({ q }) =>
    Promise.resolve({
      cards: q === 'Kwain' ? [IMPOSTOR, KWAIN] : [TATYOVA],
      total: 2,
    }))
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('the page', () => {
  it('renders the masthead, the bio, and the colour pair', async () => {
    renderPage()
    expect(screen.getByRole('heading', { level: 1, name: 'About Claude' }))
      .toBeTruthy()
    expect(screen.getByText(/I’m Claude\./)).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Simic' })).toBeTruthy()
    // The deep link goes to the guild's own page, not a copy of its story.
    const link = screen.getByRole('link', { name: /the Combine/i })
    expect(link.getAttribute('href')).toBe('/learn?tab=colors&c=UG')
    await waitFor(() =>
      expect(screen.getByText('Legendary Creature — Rabbit Wizard')).toBeTruthy())
  })

  it('credits every painting in the gallery by artist name', () => {
    renderPage()
    expect(GALLERY.length).toBeGreaterThan(0)
    for (const piece of GALLERY) {
      expect(screen.getByText(new RegExp(piece.artist)),
             `${piece.name} renders no credit for ${piece.artist}`).toBeTruthy()
      // The image itself describes the painting rather than decorating.
      expect(screen.getByAltText(piece.alt)).toBeTruthy()
    }
  })
})

describe('the exhibit cases', () => {
  it('render the pool’s own text for an exact name match only', async () => {
    renderPage()
    await waitFor(() =>
      expect(screen.getByText(/Each player may draw a card/)).toBeTruthy())
    expect(screen.getByText(/Landfall/)).toBeTruthy()
    // The near-miss from the search never appears.
    expect(screen.queryByText('Kwain the Builder')).toBeNull()
    // My reasons render beside the cards, not instead of them.
    for (const pick of COMMANDERS) {
      expect(screen.getByText(pick.why)).toBeTruthy()
    }
  })

  it('say so honestly when the pool cannot answer', async () => {
    vi.mocked(api.searchCards).mockRejectedValue(new Error('no pool'))
    renderPage()
    await waitFor(() => expect(
      screen.getAllByText(/The pool has no record to show here/).length,
    ).toBe(COMMANDERS.length))
    // The names still render — it is the rules text that is never recalled.
    expect(screen.getByText('Kwain, Itinerant Meddler')).toBeTruthy()
    expect(screen.getByText('Tatyova, Benthic Druid')).toBeTruthy()
  })
})
