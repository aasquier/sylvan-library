/**
 * The opening-hand table, at the level a suite can honestly reach.
 *
 * jsdom has no layout engine, so nothing here can prove a card is visible,
 * sized, tilted or unclipped — **the appearance is Aaron's walk, not this
 * file's**. What is asserted is behaviour and wiring: that nothing is asked
 * of the server until somebody presses, that a press deals and a second
 * press deals again, that a card these lands never pay for says so rather
 * than showing a blank, and that no keep-or-mulligan verdict is ever
 * rendered (ADR 14 — the whole point of the panel).
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { DealtCard, OpeningHand } from '../lib/api'

vi.mock('../lib/api', async (importOriginal) => {
  const real = await importOriginal<typeof import('../lib/api')>()
  return { ...real, api: { ...real.api, dealOpeningHand: vi.fn() } }
})

const { api } = await import('../lib/api')
const { OpeningHandDeal } = await import('./openinghand')
const deal = vi.mocked(api.dealOpeningHand)

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const ref = { owner: 'local', slug: 'a-deck' }

function card(name: string, over: Partial<DealtCard> = {}): DealtCard {
  return {
    name,
    mana_cost: '{1}{G}',
    type_line: 'Creature — Bear',
    image: `https://cards.example.test/${name}.jpg`,
    mana_value: 2,
    is_land: false,
    turn: 2,
    ...over,
  }
}

function hand(over: Partial<OpeningHand> = {}): OpeningHand {
  return {
    pool_available: true,
    cards: [
      card('Forest', { is_land: true, turn: null, mana_cost: null, type_line: 'Basic Land' }),
      card('Forest', { is_land: true, turn: null, mana_cost: null, type_line: 'Basic Land' }),
      card('Grizzly Bears'),
      card('Craterhoof Behemoth', { turn: null, mana_value: 8 }),
      card('Llanowar Elves', { turn: 1, mana_value: 1 }),
      card('Regal Behemoth', { turn: null, mana_value: 6 }),
      card('Sol Ring', { turn: null, mana_value: 1 }),
    ],
    reading: {
      lands: 2,
      spells: 5,
      colors_covered: ['G'],
      colors_missing: ['W'],
      first_spell_turn: 1,
      castable_by_horizon: 2,
      horizon: 3,
    },
    deck_size: 99,
    declared_size: 99,
    unresolved_count: 0,
    commander: card('Goreclaw, Terror of Qal Sisma', { turn: null }),
    answered_by: 'dice and counting',
    caveat: 'Seven cards off a real shuffle, then plain counting.',
    ...over,
  }
}

function unfold() {
  render(<OpeningHandDeal deckRef={ref} />)
  fireEvent.click(screen.getByTitle(/deal yourself an opening hand/i))
}

// Folded, the panel is one picture and no request: opening a deck page must
// not fetch a hand nobody asked for.
it('asks the server for nothing until somebody presses deal', () => {
  render(<OpeningHandDeal deckRef={ref} />)
  expect(deal).not.toHaveBeenCalled()
  fireEvent.click(screen.getByTitle(/deal yourself an opening hand/i))
  expect(deal).not.toHaveBeenCalled()
  expect(screen.getByRole('button', { name: /deal a hand/i })).toBeTruthy()
})

it('deals seven, and deals again on the second press', async () => {
  deal.mockResolvedValue(hand())
  unfold()
  fireEvent.click(screen.getByRole('button', { name: /deal a hand/i }))
  await waitFor(() => expect(screen.getAllByRole('img').length).toBeGreaterThan(6))
  expect(deal).toHaveBeenCalledTimes(1)

  // The label changes, because the second press is a different sentence.
  const again = await screen.findByRole('button', { name: /deal again/i })
  fireEvent.click(again)
  await waitFor(() => expect(deal).toHaveBeenCalledTimes(2))
})

// Three states, three captions. The one that matters is the third: a spell
// these lands never pay for is the common case a blank would hide.
it('says what each card is waiting on, including the ones out of reach', async () => {
  deal.mockResolvedValue(hand())
  unfold()
  fireEvent.click(screen.getByRole('button', { name: /deal a hand/i }))
  expect(await screen.findByText('Turn 1')).toBeTruthy()
  expect(screen.getByText('Turn 2')).toBeTruthy()
  expect(screen.getAllByText('Land').length).toBe(2)
  // The Behemoths and the Sol Ring: two lands never pay for any of them.
  expect(screen.getAllByText('Out of reach').length).toBe(3)
})

// ADR 14 in a test: the panel counts and never concludes. A verdict here
// would teach a newcomer to ask the app instead of reading the hand, so the
// words that would carry one are asserted absent.
it('renders no keep-or-mulligan verdict of its own', async () => {
  deal.mockResolvedValue(hand())
  unfold()
  fireEvent.click(screen.getByRole('button', { name: /deal a hand/i }))
  await screen.findByText('Turn 1')
  const text = document.body.textContent ?? ''
  for (const verdict of [/\bkeep it\b/i, /\bmulligan this\b/i, /\bthrow it back\b/i,
                         /\bgood hand\b/i, /\bbad hand\b/i, /\bshould\b/i]) {
    expect(text).not.toMatch(verdict)
  }
  // And the caveat that says whose numbers these are does render.
  expect(text).toContain('plain counting')
})

// No pool degrades into a sentence rather than an empty table or a crash.
it('says why there are no cards when the library is not there', async () => {
  deal.mockResolvedValue({
    pool_available: false, cards: [], message: 'no card pool yet',
  })
  unfold()
  fireEvent.click(screen.getByRole('button', { name: /deal a hand/i }))
  expect(await screen.findByText(/no card pool yet/i)).toBeTruthy()
})

// A refusal is shown, not swallowed: a button that visibly does nothing is
// the worst failure a toy can have.
it('shows a refusal instead of quietly doing nothing', async () => {
  deal.mockRejectedValue(new Error('the library could not answer that right now'))
  unfold()
  fireEvent.click(screen.getByRole('button', { name: /deal a hand/i }))
  expect(await screen.findByText(/could not answer/i)).toBeTruthy()
})
