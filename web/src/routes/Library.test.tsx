/** The library's filter and sort, and how it renders the gate counts.
 *
 * The filtering is a `useMemo` over three independent controls, which is the
 * kind of code that is obviously correct until two of them are set at once.
 * The gate badge matters more: `errors: null` means the corpus was missing and
 * the gate never ran, which is not the same as passing, and the whole reason
 * `/api/decks` carries those counts at all.
 */

import { cleanup, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeckSummary, Health } from '../lib/api'
import Library from './Library'

vi.mock('../lib/api', () => ({
  api: { decks: vi.fn(), health: vi.fn() },
}))

const { api } = await import('../lib/api')

function deck(overrides: Partial<DeckSummary> & { slug: string }): DeckSummary {
  return {
    name: overrides.slug,
    status: 'built',
    stage: 'curated',
    needs_rationale: 0,
    commander: ['Some Commander'],
    companion: null,
    bracket: 4,
    total_cards: 99,
    land_count: 34,
    strategy: '',
    art_crop: null,
    color_identity: ['G'],
    errors: 0,
    warnings: 0,
    ...overrides,
  }
}

const DECKS: DeckSummary[] = [
  deck({ slug: 'tivit', name: 'Tivit', bracket: 5, color_identity: ['W', 'U', 'B'],
         total_cards: 100, status: 'theoretical' }),
  deck({ slug: 'arahbo', name: 'Arahbo', bracket: 3, color_identity: ['G', 'W'],
         total_cards: 99 }),
  deck({ slug: 'goreclaw', name: 'Goreclaw', bracket: 4, color_identity: ['G'],
         total_cards: 99, errors: 1 }),
]

const HEALTHY: Health = { corpus: true, oracle_cards: 35000, printings: 107000 }

function renderLibrary() {
  return render(<MemoryRouter><Library /></MemoryRouter>)
}

/** Deck names in the order they are rendered. */
function shownNames(): string[] {
  return screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent ?? '')
}

beforeEach(() => {
  // Reset, not merely re-stub: a `vi.fn()` from a mock factory is module-level
  // state that outlives the test, so call history accumulates. Nothing here
  // asserts a call count today, and this is what stops the first test that
  // does from being mystifying.
  vi.mocked(api.decks).mockReset().mockResolvedValue(DECKS)
  vi.mocked(api.health).mockReset().mockResolvedValue(HEALTHY)
})

// Explicit, because Testing Library only registers auto-cleanup when the test
// framework exposes its hooks globally, and `vitest.config.ts` deliberately
// does not. Without this every render stays mounted and the queries below
// match the previous test's DOM as well as this one's.
afterEach(cleanup)

describe('Library', () => {
  it('sorts by name by default', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    expect(shownNames()).toEqual(['Arahbo', 'Goreclaw', 'Tivit'])
  })

  it('sorts by bracket, highest first', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Sort'), { target: { value: 'bracket' } })
    expect(shownNames()).toEqual(['Tivit', 'Goreclaw', 'Arahbo'])
  })

  it('sorts by card count, largest first', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Sort'), { target: { value: 'size' } })
    expect(shownNames()[0]).toBe('Tivit')            // 100 cards, partner pair
  })

  it('filters by bracket', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Bracket'), { target: { value: '5' } })
    expect(shownNames()).toEqual(['Tivit'])
  })

  it('filters by colour as a membership test, not an exact identity', async () => {
    // Asking for green must return mono-green and Selesnya alike -- the
    // question is "can I sleeve this up", not "is this deck exactly green".
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: 'G' } })
    expect(shownNames()).toEqual(['Arahbo', 'Goreclaw'])
  })

  it('applies the bracket and colour filters together', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: 'G' } })
    fireEvent.change(screen.getByLabelText('Bracket'), { target: { value: '4' } })
    expect(shownNames()).toEqual(['Goreclaw'])
  })

  it('says so when nothing matches, rather than rendering an empty grid', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: 'R' } })
    expect(screen.queryAllByRole('heading', { level: 2 })).toHaveLength(0)
    expect(screen.getByText('No decks match those filters.')).toBeTruthy()
  })

  it('offers only the brackets that decks actually use', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    const options = within(screen.getByLabelText('Bracket'))
      .getAllByRole('option').map((o) => o.textContent)
    expect(options).toEqual(['All brackets', 'Bracket 3', 'Bracket 4', 'Bracket 5'])
  })

  // ------------------------------------------------------------ gate counts

  it('flags a deck that fails the gate, with the count pluralised', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'one', name: 'One', errors: 1 }),
      deck({ slug: 'two', name: 'Two', errors: 2 }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    expect(screen.getByText('1 error')).toBeTruthy()
    expect(screen.getByText('2 errors')).toBeTruthy()
  })

  it('shows no error badge for a clean deck', async () => {
    vi.mocked(api.decks).mockResolvedValue([deck({ slug: 'clean', name: 'Clean' })])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(1))
    expect(screen.queryByText(/error/)).toBeNull()
    expect(screen.queryByText('not checked')).toBeNull()
  })

  it('distinguishes "the gate did not run" from "the deck passed"', async () => {
    // `errors: null` means the corpus was unavailable. `/api/decks` goes to the
    // trouble of carrying that distinction; rendering it identically to a clean
    // deck throws it away, which is the exact failure the counts exist to stop.
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'unchecked', name: 'Unchecked', errors: null, warnings: null }),
    ])
    vi.mocked(api.health).mockResolvedValue({
      corpus: false, oracle_cards: 0, printings: 0,
      message: 'no corpus yet -- run `mtglab data refresh`',
    })
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(1))
    expect(screen.getByText('not checked')).toBeTruthy()
  })

  // ------------------------------------------------------------ load states

  it('reports a failed load instead of showing an empty library', async () => {
    vi.mocked(api.decks).mockRejectedValue(new Error('boom'))
    renderLibrary()
    await waitFor(() => expect(screen.getByText(/Could not load decks/)).toBeTruthy())
    expect(screen.queryByText('No decks match those filters.')).toBeNull()
  })

  // ---------------------------------------------------------- built vs theory

  it('marks a theoretical deck and leaves a built one unmarked', async () => {
    // The difference decides whether you can sit down and play it, so a list
    // you are only thinking about must not look like a box of cards.
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    const tivit = screen.getByText('Tivit').closest('a')!
    const goreclaw = screen.getByText('Goreclaw').closest('a')!
    expect(within(tivit).getByText('theory')).toBeTruthy()
    expect(within(goreclaw).queryByText('theory')).toBeNull()
  })

  it('filters the shelf down to what is actually built', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'built' } })
    expect(shownNames()).toEqual(['Arahbo', 'Goreclaw'])

    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'theoretical' } })
    expect(shownNames()).toEqual(['Tivit'])
  })

  it('combines the status filter with the others', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'built' } })
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: 'G' } })
    expect(shownNames()).toEqual(['Arahbo', 'Goreclaw'])
  })

  // ---------------------------------------------------------- draft vs curated
  //
  // Orthogonal to built vs theory: `status` is whether the cards exist,
  // `stage` is whether anyone has reasoned about them (ADR 13). A draft that
  // renders exactly like a curated deck hides the whole distinction.

  it('marks a draft with the number of rationales it still owes', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'fresh', name: 'Fresh', stage: 'draft', needs_rationale: 17 }),
      deck({ slug: 'done', name: 'Done' }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    const fresh = screen.getByText('Fresh').closest('a')!
    const done = screen.getByText('Done').closest('a')!
    expect(within(fresh).getByText('draft · 17')).toBeTruthy()
    expect(within(done).queryByText(/draft/)).toBeNull()
  })

  it('a draft is not the same thing as a theory, and both can be true', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'both', name: 'Both', status: 'theoretical',
             stage: 'draft', needs_rationale: 4 }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(1))
    const card = screen.getByText('Both').closest('a')!
    expect(within(card).getByText('theory')).toBeTruthy()
    expect(within(card).getByText('draft · 4')).toBeTruthy()
  })

  it('filters the shelf by stage, independently of status', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'a', name: 'Adraft', stage: 'draft', needs_rationale: 2 }),
      deck({ slug: 'b', name: 'Bcurated' }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    fireEvent.change(screen.getByLabelText('Stage'), { target: { value: 'draft' } })
    expect(shownNames()).toEqual(['Adraft'])
    fireEvent.change(screen.getByLabelText('Stage'), { target: { value: 'curated' } })
    expect(shownNames()).toEqual(['Bcurated'])
  })

  it('offers a way in to the importer', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    const link = screen.getByText('Import a decklist').closest('a')!
    expect(link.getAttribute('href')).toBe('/import')
  })
})
