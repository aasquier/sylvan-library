/**
 * The swap board's mover — the promotion, composed from the board's side.
 *
 * What is behaviour here, and worth pinning: the chip exists only for a hand
 * that can write; it is a toggle that says which state it is in (commandment
 * 20); the out-card is chosen from the deck's own 99, grouped by category and
 * narrowed client-side — never the pool's finder; the board entry's `why` is
 * shown as context and **never prefilled** (rule 4 — it argued the opposite
 * decision); Apply stays off until a card is picked AND a fresh sentence is
 * typed; and the request that leaves is `api.swapCard` with the board card as
 * `into` — the same route as the straight swap, second door.
 *
 * The server's own rules (blank why, commander, graveyard doubles) are not
 * re-implemented client-side, so the refusal test asserts the server sentence
 * rendered verbatim rather than a local guess at one.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { Card, DeckRef } from '../lib/api'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: {
    swapCard: vi.fn(),
    addToBoard: vi.fn(),
    // `CardFinder` (inside the add form) reaches for this the moment somebody
    // types. Nothing here opens that form, but a mock missing a method the
    // tree can call fails as an unrelated TypeError later.
    suggestCards: vi.fn(),
  },
}))

const { api } = await import('../lib/api')
const { SwapBoard } = await import('./swapboard')

const REF: DeckRef = { owner: 'aasquier', slug: 'goreclaw-stompy' }

function card(name: string, category: string, why = ''): Card {
  return {
    name, category, why, qty: 1, known: true,
    mana_cost: '{2}{G}', cmc: 3, type_line: 'Creature — Beast',
    color_identity: ['G'], art_crop: `https://example.test/${name}.jpg`,
  }
}

/** The card on the bench, with the note it was benched with. */
const WAITING = card('Craterhoof Behemoth', 'threat',
  'Waiting for the curve to make room at eight.')

/** A small 99 spanning three categories, so the grouping is observable. */
const THE_99: Card[] = [
  card('Llanowar Elves', 'ramp'),
  card('Beast Whisperer', 'card-advantage'),
  card('End-Raze Forerunners', 'threat'),
  card('Forest', 'land'),
]

const OK = {
  slug: 'goreclaw-stompy', stage: 'curated', total_cards: 99,
  needs_rationale: 0, ok: true, errors: [], warnings: [],
  swapped_out: 'End-Raze Forerunners', swapped_in: 'Craterhoof Behemoth',
  why: 'The bigger finisher.', from: 'swap_board' as const,
}

function show(over: Partial<Parameters<typeof SwapBoard>[0]> = {}) {
  return render(
    <SwapBoard deck={[WAITING]} cards={THE_99} deckRef={REF} stage="curated"
               identity={['G']} total={99} writable
               onChanged={vi.fn()} {...over} />)
}

/** Open the fold, then the composer — driven through the real controls. */
function openComposer() {
  fireEvent.click(screen.getByRole('button', { name: /Swap board/ }))
  fireEvent.click(screen.getByRole('button', { name: 'Swap it in' }))
}

// `combos.test.tsx`'s shape: clearing after rather than before, so a refusal
// test's caught rejection is not reported as an unhandled one.
describe('the swap board mover', () => {
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('offers a reader no mover at all', () => {
    show({ writable: false })
    fireEvent.click(screen.getByRole('button', { name: /Swap board/ }))
    expect(screen.queryByRole('button', { name: 'Swap it in' })).toBeNull()
  })

  it('is a toggle that says which state it is in', () => {
    show()
    fireEvent.click(screen.getByRole('button', { name: /Swap board/ }))
    const chip = screen.getByRole('button', { name: 'Swap it in' })
    // Commandment 20: it changes this page, so it is a button that says so —
    // and it wears the action-chip material, not a bare element (17).
    expect(chip.getAttribute('aria-pressed')).toBe('false')
    expect(chip.className).toContain('card-action')
    expect(screen.queryByText(/pick which card makes room/)).toBeNull()

    fireEvent.click(chip)
    expect(chip.getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByText(/pick which card makes room/)).toBeTruthy()

    fireEvent.click(chip)
    expect(chip.getAttribute('aria-pressed')).toBe('false')
    expect(screen.queryByText(/pick which card makes room/)).toBeNull()
  })

  it('shows the bench note as context and opens the why box empty', () => {
    show()
    openComposer()
    // The board `why` argued why the card was NOT in the deck. It is read,
    // not reused: the box below collects the reversal, from a person.
    expect(screen.getByText(/Why it was waiting:/)).toBeTruthy()
    expect(screen.getAllByText(/Waiting for the curve/).length)
      .toBeGreaterThanOrEqual(1)
    const box = screen.getByLabelText('Why it earns the slot') as HTMLTextAreaElement
    expect(box.value, 'the box opens empty, never prefilled').toBe('')
  })

  it('offers the 99 grouped by category, and the filter narrows by name', () => {
    show()
    openComposer()
    // The fixed vocabulary's own labels, in its own order.
    for (const label of ['Lands', 'Ramp', 'Card advantage', 'Threats']) {
      expect(screen.getByText(label)).toBeTruthy()
    }
    expect(screen.getByRole('button', { name: 'Forest' })).toBeTruthy()

    fireEvent.change(screen.getByLabelText(/Narrow the 4 by name/),
                     { target: { value: 'llan' } })
    expect(screen.getByRole('button', { name: 'Llanowar Elves' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Forest' })).toBeNull()
    expect(screen.queryByText('Lands'), 'an emptied group folds away').toBeNull()

    // A filter that matches nothing says so, with the way back out.
    fireEvent.change(screen.getByLabelText(/Narrow the 4 by name/),
                     { target: { value: 'zzz' } })
    expect(screen.getByText(/No card in the deck answers to that/)).toBeTruthy()
  })

  it('stays off until a card is picked AND a fresh why is typed', () => {
    show()
    openComposer()
    const apply = () =>
      screen.getByRole('button', { name: /Apply swap|Swapping/ }) as HTMLButtonElement

    expect(apply().disabled).toBe(true)

    // A sentence alone is not enough — nothing has been picked to make room.
    fireEvent.change(screen.getByLabelText('Why it earns the slot'),
                     { target: { value: 'The bigger finisher.' } })
    expect(apply().disabled).toBe(true)

    // And the pick states its consequence in plain words before the press.
    fireEvent.click(screen.getByRole('button', { name: 'End-Raze Forerunners' }))
    expect(screen.getByText(/goes to the graveyard with its reason kept/)).toBeTruthy()
    expect(apply().disabled).toBe(false)

    // Un-picking arms it back off — the pick is a toggle too.
    fireEvent.click(screen.getByRole('button', { name: 'End-Raze Forerunners' }))
    expect(apply().disabled).toBe(true)
  })

  it('sends the promotion through the swap route and reports back', async () => {
    let settle: (v: typeof OK) => void = () => {}
    vi.mocked(api.swapCard).mockImplementation(() =>
      new Promise((res) => { settle = res }) as never)
    const onChanged = vi.fn()
    show({ onChanged })
    openComposer()

    fireEvent.click(screen.getByRole('button', { name: 'End-Raze Forerunners' }))
    fireEvent.change(screen.getByLabelText('Why it earns the slot'),
                     { target: { value: '  The bigger finisher.  ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply swap' }))

    // The board card is `into` — the route's second door tells the doors
    // apart by exactly this — and the why travels trimmed.
    expect(api.swapCard).toHaveBeenCalledWith(REF, {
      out: 'End-Raze Forerunners', into: 'Craterhoof Behemoth',
      why: 'The bigger finisher.',
    })
    // Busy while the server holds the request: the label says so and the
    // button will not fire twice.
    const busy = screen.getByRole('button', { name: 'Swapping…' }) as HTMLButtonElement
    expect(busy.disabled).toBe(true)
    expect(onChanged).not.toHaveBeenCalled()

    settle(OK)
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
  })

  it('renders the server’s refusal verbatim and lets go of busy', async () => {
    vi.mocked(api.swapCard).mockImplementation(() =>
      Promise.reject(new Error("'Craterhoof Behemoth' is the commander")))
    const onChanged = vi.fn()
    show({ onChanged })
    openComposer()

    fireEvent.click(screen.getByRole('button', { name: 'End-Raze Forerunners' }))
    fireEvent.change(screen.getByLabelText('Why it earns the slot'),
                     { target: { value: 'The bigger finisher.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply swap' }))

    await waitFor(() => expect(screen.getByRole('alert').textContent)
      .toContain('is the commander'))
    // The refusal hands the form back: same pick, same typing, ready to fix.
    expect(screen.getByRole('button', { name: 'Apply swap' })).toBeTruthy()
    expect(onChanged).not.toHaveBeenCalled()
  })
})
