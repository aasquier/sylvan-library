/** The edit panel's two doors onto a deck's two lists.
 *
 * **A card into the 99 and a card onto the swap board are different routes**,
 * and this file exists because the panel offered both in one select and knew
 * only one of them. `POST .../cards` refuses a deck file with no
 * `swap_board:` block -- an edit changes what a deck says, never what shape it
 * has (ADR 12) -- so picking "Swap board" on a deck that had never kept one
 * answered 422, and every deck starts that way. The control was in the panel
 * from the day the swap board was read-only; the route that can open a board
 * arrived later, and nothing connected the two.
 *
 * Nothing could have caught it: `DeckDetail.test.tsx` mocks `api.addCard` and
 * never drives this select, so the call it asserts is the call the panel was
 * wrongly making. The test that finds this one has to press the control and
 * watch **which function** is reached.
 */

import { cleanup, render, screen, fireEvent, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AddCardForm } from './deckedit'
import { api, type CardOffer } from '../lib/api'

vi.mock('../lib/api', () => ({
  api: {
    addCard: vi.fn(),
    addToBoard: vi.fn(),
    suggestCards: vi.fn(),
  },
  ApiError: class extends Error { status = 0 },
}))

const RESULT = { slug: 'cats', ok: true, errors: [], warnings: [], stage: 'draft',
                 total_cards: 100, needs_rationale: 0 }

// The finder asks the pool as soon as two characters are typed; every test
// here picks its card through that list, because a name this panel sends is a
// name the pool answered with (`CardFinder` argues it) and a stub that let a
// typed string through would be testing a panel this app does not have.
function offer(name: string): CardOffer {
  return {
    name, mana_cost: '{1}', type_line: 'Artifact', oracle_text: '',
    color_identity: [], image: null, artist: null, legal_commander: true,
    is_land: false, score: 1, via: 'exact',
  }
}

describe('AddCardForm', () => {
  beforeEach(() => {
    vi.mocked(api.suggestCards).mockResolvedValue({ cards: [offer('Sol Ring')] })
    vi.mocked(api.addCard).mockResolvedValue(RESULT as never)
    vi.mocked(api.addToBoard).mockResolvedValue(RESULT as never)
  })
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  async function openAndPick() {
    render(<AddCardForm deck={{ owner: 'aaron', slug: 'cats' }} stage="draft"
                        identity={['G']} onDone={() => {}} />)
    fireEvent.click(screen.getByText(/Add a card/i))
    fireEvent.change(screen.getByLabelText(/Card/i), { target: { value: 'Sol Ring' } })
    // By its ROLE in the listbox, not by its text: the finder draws the name
    // in the offer list *and* again on the card panel beside it, so a plain
    // text match finds two and picks neither.
    const offered = await screen.findByRole('option', { name: /Sol Ring/ })
    // `pointerDown`, not `click`: the row commits on pointer down so it wins
    // the race against the outside-click listener that closes the list
    // (`cardfinder.tsx` argues it). A `click` here selects nothing at all, and
    // the panel then refuses to submit for want of a card -- which looks
    // exactly like the bug this file is about, and is not.
    fireEvent.pointerDown(offered)
  }

  it('sends a card for the 99 to the cards route', async () => {
    await openAndPick()
    fireEvent.click(screen.getByRole('button', { name: 'Add card' }))
    await waitFor(() => expect(api.addCard).toHaveBeenCalled())
    expect(api.addToBoard).not.toHaveBeenCalled()
    expect(vi.mocked(api.addCard).mock.calls[0]?.[1].to).toBe('cards')
  })

  // The regression. Asserting on *which route* rather than on a rendered
  // string: the panel looked identical either way, and the only observable
  // difference was a 422 on a deck nobody had given a board to yet.
  it('sends a card for the swap board to the board route, which may open one',
     async () => {
    await openAndPick()
    fireEvent.change(screen.getByLabelText('Into'), { target: { value: 'swap_board' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add card' }))
    await waitFor(() => expect(api.addToBoard).toHaveBeenCalled())
    expect(api.addCard).not.toHaveBeenCalled()
    // No `to` on this call: the route *is* the destination, and a body that
    // still carried one would mean somebody had reintroduced the old shape.
    expect(vi.mocked(api.addToBoard).mock.calls[0]?.[1]).not.toHaveProperty('to')
  })
})
