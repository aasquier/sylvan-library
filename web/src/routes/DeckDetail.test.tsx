/** Replacement shortlists on the validation tab.
 *
 * The rendering is mostly markup, but two things here are real behaviour and
 * worth pinning: the shortlist is fetched lazily, because building one costs a
 * pool query per offending card and most visits never open this tab; and a
 * candidate is matched to its error by card name, so a deck with two problems
 * does not show one card's suggestions under the other's.
 */

import { cleanup, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeckDetail as Deck, DeckStats, Suggestions, ValidationReport } from '../lib/api'
import DeckDetail from './DeckDetail'

vi.mock('../lib/api', () => ({
  api: { deck: vi.fn(), stats: vi.fn(), validate: vi.fn(), suggestions: vi.fn() },
}))

const { api } = await import('../lib/api')

const DECK = {
  slug: 'goreclaw-stompy',
  name: 'Goreclaw — Mono-Green Stompy',
  commander: ['Goreclaw, Terror of Qal Sisma'],
  companion: null,
  bracket: 4,
  total_cards: 99,
  land_count: 34,
  strategy: 'Mono-green big stompy.',
  art_crop: null,
  color_identity: ['G'],
  errors: 1,
  warnings: 0,
  notes: {},
  commander_card: null,
  corpus_available: true,
  swap_board: [],
  cards: [{
    name: 'Primeval Titan', category: 'ramp', why: 'Ramp and threat in one card.',
    qty: 1, known: true, mana_cost: '{4}{G}{G}', cmc: 6, type_line: 'Creature — Giant',
    color_identity: ['G'], art_crop: 'https://example.test/titan.jpg',
  }],
} as unknown as Deck

const STATS = {
  slug: 'goreclaw-stompy', name: DECK.name, bracket: 4, total_cards: 99,
  land_count: 34, curve: { average_mv: 3.5, nonland_cards: 65, buckets: [] },
  categories: [], colors: [], types: {},
} as DeckStats

const REPORT: ValidationReport = {
  ok: false,
  errors: [{ code: 'banned', message: 'not legal in Commander', card: 'Primeval Titan' }],
  warnings: [],
}

const SHORTLIST: Suggestions = {
  slug: 'goreclaw-stompy',
  corpus_available: true,
  targets: [{
    card: 'Primeval Titan',
    code: 'banned',
    why: 'Ramp and threat in one card.',
    candidates: [{
      name: 'Cultivator Colossus', mana_cost: '{4}{G}{G}{G}', cmc: 7,
      type_line: 'Creature — Plant Beast', oracle_text: 'Trample.',
      color_identity: ['G'], image: null,
      art_crop: 'https://example.test/colossus.jpg', edhrec_rank: 1589,
      score: 0.7593,
      reasons: ['same card type (Creature)', 'shares trample', 'EDHREC rank 1,589'],
    }],
  }],
}

function renderDeck() {
  return render(
    <MemoryRouter initialEntries={['/decks/goreclaw-stompy']}>
      <Routes>
        <Route path="/decks/:slug" element={<DeckDetail />} />
      </Routes>
    </MemoryRouter>,
  )
}

/** Open the tab the shortlist lives on. */
function openValidation() {
  fireEvent.click(screen.getByRole('button', { name: 'Validation' }))
}

beforeEach(() => {
  // `mockReset`, not just a fresh return value. `restoreMocks` in
  // vitest.config.ts only touches spies created with `vi.spyOn`; a bare
  // `vi.fn()` from a mock factory is module-level state that survives between
  // tests, so call counts accumulate and "called once" quietly means "called
  // once this test, plus however many times the last one called it".
  vi.mocked(api.deck).mockReset().mockResolvedValue(DECK)
  vi.mocked(api.stats).mockReset().mockResolvedValue(STATS)
  vi.mocked(api.validate).mockReset().mockResolvedValue(REPORT)
  vi.mocked(api.suggestions).mockReset().mockResolvedValue(SHORTLIST)
})

afterEach(cleanup)

describe('DeckDetail validation tab', () => {
  it('does not fetch a shortlist until the tab is opened', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    expect(api.suggestions).not.toHaveBeenCalled()

    openValidation()
    await waitFor(() => expect(api.suggestions).toHaveBeenCalledWith('goreclaw-stompy'))
  })

  it('fetches the shortlist once, not on every tab switch', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    await waitFor(() => expect(api.suggestions).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('button', { name: 'Notes' }))
    openValidation()
    await waitFor(() => expect(screen.getByText('Cultivator Colossus')).toBeTruthy())
    expect(api.suggestions).toHaveBeenCalledTimes(1)
  })

  it('renders candidates under the error they belong to, with their reasons', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()

    const candidate = await screen.findByText('Cultivator Colossus')
    const block = candidate.closest('li')!
    expect(within(block).getByText(/same card type \(Creature\)/)).toBeTruthy()
    expect(within(block).getByText(/EDHREC rank 1,589/)).toBeTruthy()
  })

  it('says the shortlist is not a recommendation', async () => {
    // The whole design rests on this being a measurement the user overrules.
    // If the disclaimer ever quietly disappears, that is a change worth failing.
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    await screen.findByText('Cultivator Colossus')
    expect(screen.getByText(/not a recommendation/)).toBeTruthy()
  })

  it('shows the error alone when there is no shortlist for it', async () => {
    vi.mocked(api.suggestions).mockResolvedValue({
      slug: 'goreclaw-stompy', corpus_available: false, targets: [],
    })
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()

    await screen.findByText(/not legal in Commander/)
    expect(screen.queryByText(/not a recommendation/)).toBeNull()
  })
})
