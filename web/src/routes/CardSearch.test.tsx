/**
 * The card search says how many came back.
 *
 * It always *showed* how many — a small line above the grid — and that is
 * exactly the shape the 08-24 live-region pass could not fix from one place.
 * `Spinner` carries `role="status"`, so a search announces that it started and
 * then unmounts with the answer; the count arrives in an ordinary paragraph
 * nobody is told to go and read. So somebody using a screen reader typed a
 * filter, heard "Searching…", and then heard nothing at all — which is
 * indistinguishable from a search that never finished, on the one screen a
 * newcomer uses to find out what the game has in it.
 *
 * What is pinned here is the mechanism rather than the wording: a region that
 * is **already standing** before the answer lands (a region that mounts with
 * its sentence in it is initial content, which readers do not announce), that
 * empties between searches so the next arrival is a change, and that says the
 * same thing the visible line says. A count is the answer; "search finished"
 * would be a worse sentence honestly rendered.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Card } from '../lib/api'
import CardSearch from './CardSearch'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { searchCards: vi.fn() } }
})

const { api } = await import('../lib/api')

function card(name: string): Card {
  return {
    name, mana_cost: '{G}', type_line: 'Creature — Bear', price_usd: 0.25,
    art_crop: null, image: null, edhrec_rank: null, reserved: false,
  } as unknown as Card
}

/** However many cards the shelves are pretending to hold. */
function shelves(n: number) {
  vi.mocked(api.searchCards).mockResolvedValue({
    cards: Array.from({ length: n }, (_, i) => card(`Bear ${i + 1}`)),
  } as unknown as Awaited<ReturnType<typeof api.searchCards>>)
}

/** The live region, which is mounted whether or not anything has arrived. */
function said(): string {
  return document.querySelector('[role="status"].sr-only')?.textContent ?? ''
}

beforeEach(() => shelves(3))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the card search, said out loud', () => {
  it('has its live region up before the first answer lands', () => {
    render(<CardSearch />)
    // Before the debounce has even fired: standing, and holding nothing.
    expect(document.querySelector('[role="status"].sr-only')).not.toBeNull()
    expect(said()).toBe('')
  })

  it('counts what came back, in the words the page already uses', async () => {
    render(<CardSearch />)
    await waitFor(() => expect(said()).toBe('3 results.'))
    // The same sentence a sighted person reads, so the two are told the same
    // thing about the same page.
    expect(screen.getByText(/^3 results$/)).toBeTruthy()
  })

  it('says one card in the singular', async () => {
    shelves(1)
    render(<CardSearch />)
    await waitFor(() => expect(said()).toBe('1 result.'))
  })

  it('says a capped list is capped, because the rest are not missing', async () => {
    // Sixty is the limit the page asks for, so sixty means "and probably
    // more" — the visible line says so and the spoken one must, or a reader
    // user is told a wrong total rather than an incomplete one.
    shelves(60)
    render(<CardSearch />)
    await waitFor(() => expect(said())
      .toBe('60 results, and the list is capped — narrow the filters for more.'))
  })

  it('says nothing matched rather than falling silent', async () => {
    shelves(0)
    render(<CardSearch />)
    await waitFor(() => expect(said()).toBe('Nothing matched. Try loosening a filter.'))
  })

  it('says nothing at all when the search failed', async () => {
    // The trap this pins: a failed search leaves the previous results standing
    // and clears `busy`, so a region keyed only on those two announces the old
    // count as though it were a new answer — arriving right behind
    // `ErrorNote`'s `role="alert"`, so the reader hears the failure and then
    // hears a number. The grid keeps the stale cards on purpose; the spoken
    // sentence cannot carry "these are from before".
    render(<CardSearch />)
    await waitFor(() => expect(said()).toBe('3 results.'))

    vi.mocked(api.searchCards).mockRejectedValue(new Error('the shelves are shut'))
    fireEvent.change(screen.getByLabelText(/Text/i), { target: { value: 'bear' } })

    await screen.findByText(/the shelves are shut/)
    expect(said()).toBe('')
    // And the cards really are still on the page — otherwise this test would
    // be passing on an emptied grid rather than on the guard.
    expect(screen.getAllByText('Bear 1').length).toBeGreaterThan(0)
  })

  it('empties while the next search is out, so the next answer is a change', async () => {
    render(<CardSearch />)
    await waitFor(() => expect(said()).toBe('3 results.'))

    // A search that does not come back. The region has to go quiet: a polite
    // region that still holds the last count announces nothing when the same
    // count returns, and "3 results" twice running is a real search anybody
    // might do twice.
    vi.mocked(api.searchCards).mockReturnValue(new Promise(() => {}))
    fireEvent.change(screen.getByLabelText(/Text/i), { target: { value: 'bear' } })
    await waitFor(() => expect(said()).toBe(''))
  })
})
