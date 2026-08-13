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
  api: { decks: vi.fn(), health: vi.fn(), deleteDeck: vi.fn() },
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
  vi.mocked(api.deleteDeck).mockReset().mockResolvedValue({
    slug: 'goreclaw', name: 'Goreclaw', deleted: true,
    moved_to: 'decks/.trash/goreclaw-20260811T220000Z',
    total_cards: 99, stage: 'curated', status: 'built',
  })
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

  // ------------------------------------------------------------- first run
  //
  // "You own nothing yet" and "your filters matched nothing" are different
  // situations with different exits, and the second one used to answer for
  // both. Until import existed there was nothing to offer here anyway.

  it('greets an empty library rather than blaming the filters', async () => {
    vi.mocked(api.decks).mockResolvedValue([])
    renderLibrary()
    await waitFor(() => expect(screen.getByText('Nothing on the shelf yet.')).toBeTruthy())
    expect(screen.queryByText('No decks match those filters.')).toBeNull()
    expect(screen.getByText(/Sylvan Library/)).toBeTruthy()
  })

  it('still blames the filters when the library is not empty', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Color'), { target: { value: 'R' } })
    expect(screen.getByText('No decks match those filters.')).toBeTruthy()
    expect(screen.queryByText('Nothing on the shelf yet.')).toBeNull()
  })

  it('hides the decorative art from screen readers', async () => {
    // It carries no information the heading does not; announcing it is noise.
    vi.mocked(api.decks).mockResolvedValue([])
    const { container } = renderLibrary()
    await waitFor(() => expect(screen.getByText('Nothing on the shelf yet.')).toBeTruthy())
    const art = container.querySelector('img')!
    expect(art.getAttribute('alt')).toBe('')
    expect(art.getAttribute('aria-hidden')).toBe('true')
  })

  it('keeps its heading, and shows the painting once', async () => {
    // The nameplate normally carries the `h1`, and it is deliberately not
    // rendered here — `FirstRun` is the same painting at full size and two of
    // them stacked is the app introducing itself twice. Suppressing it took
    // the page's only top-level heading with it, which is the regression this
    // pins: exactly one `h1`, and exactly one copy of the art.
    vi.mocked(api.decks).mockResolvedValue([])
    const { container } = renderLibrary()
    await waitFor(() => expect(screen.getByText('Nothing on the shelf yet.')).toBeTruthy())
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Deck library')
    expect(container.querySelectorAll('img')).toHaveLength(1)
  })

  it('names the painting for a screen reader on a stocked shelf', async () => {
    // The nameplate's copy is the subject rather than a backdrop — it is shown
    // whole and at its own ratio — so unlike `FirstRun`'s it gets a real
    // description instead of being hidden.
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    const art = screen.getByRole('heading', { level: 1 })
      .closest('section')!.querySelector('img')!
    expect(art.getAttribute('aria-hidden')).toBeNull()
    expect(art.getAttribute('alt')).toMatch(/Yeong-Hao Han/)
  })
})

// ---------------------------------------------------------------- mana pips

describe('Library mana pips', () => {
  it('draws the symbols in a strategy blurb instead of spelling them', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      // Colourless identity, so the only titled pips on the card are the ones
      // the strategy produced -- the identity row draws its own otherwise.
      deck({ slug: 'cats', name: 'Cats', color_identity: [],
             strategy: 'A {G}{W} deck that wants {1}.' }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(1))
    const card = screen.getByText('Cats').closest('a')!
    // The braces are gone and the prose either side of them survives. Asserted
    // around the pips rather than through them: a colour pip is a drawing now
    // and contributes no text, where it used to contribute its letter.
    expect(card.textContent).not.toContain('{G}')
    expect(card.textContent).toContain('A ')
    expect(card.textContent).toContain(' deck that wants ')
    // A generic cost has no icon and stays a numeral, which is the branch in
    // `Pip` that decides between a glyph and a character.
    expect(card.textContent).toContain('1.')
    // Each pip carries the colour's name, which is what makes it readable
    // without the letter being spelled out in the prose. Both the tooltip and
    // the accessible name, because they serve different readers and the letter
    // that used to serve the second one is no longer there.
    expect(within(card).getByTitle('Green')).toBeTruthy()
    expect(within(card).getByTitle('White')).toBeTruthy()
    expect(within(card).getByLabelText('Green')).toBeTruthy()
    expect(within(card).getByLabelText('White')).toBeTruthy()
  })

  it('draws a colour identity but leaves numerals and X as characters', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'cats', name: 'Cats', color_identity: [],
             strategy: 'Pay {2} or {X}, then {B}.' }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(1))
    const card = screen.getByText('Cats').closest('a')!
    // Nothing but the five colours has an icon: a generic cost is a number and
    // `{X}` is a letter, so both keep their character and neither acquires a
    // drawing that would be asserting a meaning they do not have.
    expect(card.textContent).toContain('2')
    expect(card.textContent).toContain('X')
    expect(within(card).getByLabelText('Black')).toBeTruthy()
    expect(within(card).queryByLabelText('2')).toBeNull()
  })
})

/**
 * Deleting a deck.
 *
 * The safeguard is that the confirmation is a typed word rather than a yes/no:
 * an "Are you sure?" is answered identically by someone who read it and
 * someone who clicked through it. These tests pin that the button stays
 * disabled until the answer matches, that cancelling calls nothing, and that
 * the response's `moved_to` is shown — a deletion is only survivable if you
 * can see where it went.
 *
 * Two of them are regression tests for the same bug, and it is worth naming
 * because it did not look like a bug. The dialog used to demand the slug in a
 * label styled `uppercase`, so `ishai-ojutai-dragonspeaker` appeared as
 * `ISHAI-OJUTAI-DRAGONSPEAKER` over a case-sensitive comparison: typing
 * exactly what was on screen left the button disabled and said nothing about
 * why, which reads as a broken control rather than as a refusal. `bury` is the
 * short answer that replaced it, and the match ignores case so that what is on
 * screen is always an accepted answer.
 */
describe('Library deck deletion', () => {
  async function openDialogFor(name: string) {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.click(screen.getByRole('button', { name: `Delete ${name}` }))
    return screen.getByRole('dialog')
  }

  it('will not delete until the word is typed', async () => {
    const dialog = await openDialogFor('Goreclaw')
    const confirm = within(dialog).getByRole('button', { name: /delete this deck/i })
    expect(confirm.hasAttribute('disabled')).toBe(true)

    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'bur' } })
    expect(confirm.hasAttribute('disabled')).toBe(true)

    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'bury' } })
    expect(confirm.hasAttribute('disabled')).toBe(false)
  })

  it('accepts the word whatever case it is typed in', async () => {
    // The regression. Whatever the label renders has to be an answer the
    // dialog takes, and CSS is free to change how a label renders.
    const dialog = await openDialogFor('Goreclaw')
    const confirm = within(dialog).getByRole('button', { name: /delete this deck/i })
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'BURY' } })
    expect(confirm.hasAttribute('disabled')).toBe(false)
  })

  it('still accepts the slug, in any case, for anyone who prefers it', async () => {
    const dialog = await openDialogFor('Goreclaw')
    const confirm = within(dialog).getByRole('button', { name: /delete this deck/i })
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'GORECLAW' } })
    expect(confirm.hasAttribute('disabled')).toBe(false)
  })

  it('never asks for a literal it renders in a different case', async () => {
    // The bug itself, pinned at its cause rather than at its symptom: the
    // label named the string to retype and CSS uppercased it. Any
    // `text-transform` on the element holding that literal reintroduces it.
    const dialog = await openDialogFor('Goreclaw')
    const literal = within(dialog).getByText('bury', { selector: 'code' })
    expect(literal.textContent).toBe('bury')
    for (let el: HTMLElement | null = literal; el; el = el.parentElement) {
      expect(el.className).not.toMatch(/\buppercase\b|\blowercase\b|\bcapitalize\b/)
      if (el === dialog) break
    }
  })

  it('sends the normalised answer as the confirmation', async () => {
    const dialog = await openDialogFor('Goreclaw')
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: '  BURY ' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /delete this deck/i }))

    await waitFor(() => expect(api.deleteDeck).toHaveBeenCalledWith(
      'goreclaw', 'bury'))
  })

  it('drops the deck from the shelf and says where it went', async () => {
    const dialog = await openDialogFor('Goreclaw')
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'goreclaw' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /delete this deck/i }))

    await waitFor(() => expect(shownNames()).toEqual(['Arahbo', 'Tivit']))
    // "Deleted" and "recoverable" are separate facts, and the second one is
    // the reason anyone presses the button without dread.
    expect(screen.getByText(/decks\/\.trash\/goreclaw-/)).toBeTruthy()
  })

  it('cancelling deletes nothing', async () => {
    const dialog = await openDialogFor('Goreclaw')
    fireEvent.click(within(dialog).getByRole('button', { name: /cancel/i }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.deleteDeck).not.toHaveBeenCalled()
    expect(shownNames()).toContain('Goreclaw')
  })

  it('keeps the deck on the shelf when the server refuses', async () => {
    vi.mocked(api.deleteDeck).mockRejectedValue(
      new Error('this library is read-only'))
    const dialog = await openDialogFor('Goreclaw')
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'goreclaw' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /delete this deck/i }))

    await within(dialog).findByText(/read-only/)
    expect(shownNames()).toContain('Goreclaw')
  })
})
