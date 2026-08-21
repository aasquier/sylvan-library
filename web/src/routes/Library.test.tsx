/** The library's filter and sort, and how it renders the gate counts.
 *
 * The filtering is a `useMemo` over three independent controls, which is the
 * kind of code that is obviously correct until two of them are set at once.
 * The gate badge matters more: `errors: null` means the pool was missing and
 * the gate never ran, which is not the same as passing, and the whole reason
 * `/api/decks` carries those counts at all.
 */

import { cleanup, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeckTile, Health } from '../lib/api'
import Library from './Library'

vi.mock('../lib/api', async () => ({
  // Real, and it has to be: the shelf's links are the only thing that says
  // where a deck lives, and a stub would let them all point at the pre-ADR-22
  // path while every assertion below still passed.
  deckUrl: (await vi.importActual<typeof import('../lib/api')>('../lib/api')).deckUrl,
  api: { decks: vi.fn(), health: vi.fn(), deleteDeck: vi.fn(),
         colors: vi.fn(), lore: vi.fn() },
}))

const { api } = await import('../lib/api')
const { resetLoreCache } = await import('../lib/lore')

function deck(overrides: Partial<DeckTile> & { slug: string }): DeckTile {
  return {
    name: overrides.slug,
    // The maintainer's own instance by default: their decks, their showcase.
    // A reader's view of somebody else's is `writable: false`, and the browse
    // tab's subject is `writable: false, showcase: false`.
    owner: 'aasquier',
    pilot: '',
    // Unlabelled by default (ADR 37): declaring themes is a deliberate act,
    // so the fixture that describes "a deck" describes one nobody has
    // labelled yet.
    themes: [],
    archetype: null,
    shared: true,
    showcase: true,
    status: 'built',
    stage: 'curated',
    // The maintainer's own view by default, so existing tests describe the
    // library as its owner sees it. A reader's view is `writable: false`.
    writable: true,
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

const DECKS: DeckTile[] = [
  deck({ slug: 'tivit', name: 'Tivit', bracket: 5, color_identity: ['W', 'U', 'B'],
         total_cards: 100, status: 'theoretical' }),
  deck({ slug: 'arahbo', name: 'Arahbo', bracket: 3, color_identity: ['G', 'W'],
         total_cards: 99 }),
  deck({ slug: 'goreclaw', name: 'Goreclaw', bracket: 4, color_identity: ['G'],
         total_cards: 99, errors: 1 }),
]

const HEALTHY: Health = { pool: true, oracle_cards: 35000, printings: 107000 }

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
  // The shelf-fact strip is decorative and must never block the shelf, so
  // the default here is the shelves failing to load. The one test about the
  // strip supplies its own.
  vi.mocked(api.colors).mockReset().mockRejectedValue(new Error('no taxonomy'))
  resetLoreCache()
  vi.mocked(api.lore).mockReset().mockRejectedValue(new Error('no shelves'))
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

  it('filters by pilot, with untagged as a real answer', async () => {
    // The household tag (second punch list, item 10). The control exists only
    // once somebody has tagged a pilot — a single-player library never grows
    // a filter about nobody — and 'Untagged' is a position, not the filter
    // off.
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'tivit', name: 'Tivit', pilot: "Mark's Wife" }),
      deck({ slug: 'arahbo', name: 'Arahbo', pilot: 'The Kids' }),
      deck({ slug: 'goreclaw', name: 'Goreclaw' }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    fireEvent.change(screen.getByLabelText('Pilot'), { target: { value: "Mark's Wife" } })
    expect(shownNames()).toEqual(['Tivit'])
    fireEvent.change(screen.getByLabelText('Pilot'), { target: { value: 'none' } })
    expect(shownNames()).toEqual(['Goreclaw'])
    // And the tag shows on the tile.
    fireEvent.change(screen.getByLabelText('Pilot'), { target: { value: 'all' } })
    expect(screen.getByText("pilot · Mark's Wife")).toBeTruthy()
  })

  it('grows no pilot filter while nobody is tagged', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    expect(screen.queryByLabelText('Pilot')).toBeNull()
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
    // `errors: null` means the pool was unavailable. `/api/decks` goes to the
    // trouble of carrying that distinction; rendering it identically to a clean
    // deck throws it away, which is the exact failure the counts exist to stop.
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'unchecked', name: 'Unchecked', errors: null, warnings: null }),
    ])
    vi.mocked(api.health).mockResolvedValue({
      pool: false, oracle_cards: 0, printings: 0,
      message: 'no card pool yet -- run `mtglab data refresh`',
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

  it('offers one fact from the shelves, with the paragraph behind a door', async () => {
    // The shelf strip (second punch list, item 7): one lore fact at a time,
    // `more` behind "Tell me more", cards resolved by the server. Decorative —
    // the default mock rejects, and every other test proves the shelf
    // renders without it.
    resetLoreCache()
    vi.mocked(api.lore).mockResolvedValue({
      volumes: [{ key: 'history', label: 'History', blurb: 'Where it began.' }],
      facts: [{
        key: 'alpha-93', volume: 'history',
        fact: 'Magic reached shops in the summer of 1993.',
        more: 'And sold out almost immediately.',
        cards: [{ name: 'Black Lotus', mana_cost: '{0}', type_line: 'Artifact',
                  oracle_text: '', color_identity: [] }],
        learn: { tab: 'words', key: 'commander' },
      }],
      pool: true, dropped: 0,
    })
    renderLibrary()
    await screen.findByText(/from the shelves/i)
    expect(screen.getByText('History')).toBeTruthy()
    expect(screen.getByText(/summer of 1993/)).toBeTruthy()
    // The paragraph and the cards wait behind the door — a hook, not a page.
    expect(screen.queryByText(/sold out almost immediately/)).toBeNull()
    expect(screen.queryByText('Black Lotus')).toBeNull()
    fireEvent.click(screen.getByText('Tell me more'))
    expect(screen.getByText(/sold out almost immediately/)).toBeTruthy()
    expect(screen.getByText('Black Lotus')).toBeTruthy()
    expect(screen.getByText(/in the learn room/i).closest('a')!
      .getAttribute('href')).toBe('/learn?tab=words')
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
    // Since ADR 33 every pip is a drawing, the numeral in `{1}` included —
    // it no longer contributes its character to the page. What it does
    // contribute is a name.
    expect(card.textContent).not.toContain('1.')
    expect(within(card).getByLabelText('Generic 1')).toBeTruthy()
    // Each pip carries the colour's name, which is what makes it readable
    // without the letter being spelled out in the prose. Both the tooltip and
    // the accessible name, because they serve different readers and the letter
    // that used to serve the second one is no longer there.
    expect(within(card).getByTitle('Green')).toBeTruthy()
    expect(within(card).getByTitle('White')).toBeTruthy()
    expect(within(card).getByLabelText('Green')).toBeTruthy()
    expect(within(card).getByLabelText('White')).toBeTruthy()
  })

  it('names every pip, numerals and X included', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'cats', name: 'Cats', color_identity: [],
             strategy: 'Pay {2} or {X}, then {B}.' }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(1))
    const card = screen.getByText('Cats').closest('a')!
    // ADR 33: a generic cost and `{X}` are official symbols like any other,
    // so they read through their accessible names rather than as characters.
    // "Generic 2" rather than "2" because a bare numeral read aloud in prose
    // is indistinguishable from prose.
    expect(within(card).getByLabelText('Generic 2')).toBeTruthy()
    expect(within(card).getByLabelText('X')).toBeTruthy()
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
    fireEvent.click(screen.getByRole('button', { name: `Entomb ${name}` }))
    return screen.getByRole('dialog')
  }

  it('will not delete until the word is typed', async () => {
    const dialog = await openDialogFor('Goreclaw')
    const confirm = within(dialog).getByRole('button', { name: /entomb this deck/i })
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
    const confirm = within(dialog).getByRole('button', { name: /entomb this deck/i })
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'BURY' } })
    expect(confirm.hasAttribute('disabled')).toBe(false)
  })

  it('still accepts the slug, in any case, for anyone who prefers it', async () => {
    const dialog = await openDialogFor('Goreclaw')
    const confirm = within(dialog).getByRole('button', { name: /entomb this deck/i })
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
    fireEvent.click(within(dialog).getByRole('button', { name: /entomb this deck/i }))

    await waitFor(() => expect(api.deleteDeck).toHaveBeenCalledWith(
      expect.objectContaining({ owner: 'aasquier', slug: 'goreclaw' }), 'bury'))
  })

  it('drops the deck from the shelf and says where it went', async () => {
    const dialog = await openDialogFor('Goreclaw')
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'goreclaw' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /entomb this deck/i }))

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
    fireEvent.click(within(dialog).getByRole('button', { name: /entomb this deck/i }))

    await within(dialog).findByText(/read-only/)
    expect(shownNames()).toContain('Goreclaw')
  })
})

/**
 * Two shelves, and which decks land on which (ADR 22).
 *
 * The rule the whole tab rests on is that both tests come from the server:
 * `writable` is the caller's own decks, `showcase` is the curated six's owner.
 * Neither is a comparison this client could make — it is never told who the
 * maintainer is — so a browser that tried to infer the split from the order of
 * the response would be reading an ordering that is not a contract.
 */
describe('Library, browsing by player', () => {
  /** The maintainer's showcase, this reader's own deck, and two strangers'. */
  const MIXED: DeckTile[] = [
    deck({ slug: 'my-deck', name: 'Mine', owner: 'mitch', showcase: false,
           writable: true, shared: false }),
    deck({ slug: 'goreclaw', name: 'Goreclaw', owner: 'aasquier',
           showcase: true, writable: false }),
    deck({ slug: 'zoe-deck', name: 'Zoe deck', owner: 'zoe',
           showcase: false, writable: false }),
    deck({ slug: 'amy-deck', name: 'Amy deck', owner: 'amy',
           showcase: false, writable: false }),
  ]

  /** Deck names on the browse shelf, where they sit under an owner's `h2`. */
  function browsedNames(): string[] {
    return screen.getAllByRole('heading', { level: 3 }).map((h) => h.textContent ?? '')
  }

  function openBrowse() {
    fireEvent.click(screen.getByRole('tab', { name: /other players/i }))
  }

  it('offers no tabs at all when nobody else has shared anything', async () => {
    // The default: a laptop, and any instance where the showcase is the only
    // library. A tab strip here would be the "something in the way" ADR 22
    // asked the browse view not to be.
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    expect(screen.queryByRole('tab')).toBeNull()
  })

  it('keeps your own decks and the showcase together, and the rest behind a tab',
     async () => {
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    // The showcase is "always visible", so it is here rather than filed under
    // its owner's username with the strangers.
    await waitFor(() => expect(shownNames()).toEqual(['Goreclaw', 'Mine']))
    expect(screen.queryByText('Zoe deck')).toBeNull()

    openBrowse()
    expect(browsedNames()).toEqual(['Amy deck', 'Zoe deck'])
    expect(screen.queryByText('Mine')).toBeNull()
  })

  it('groups the browse shelf by username, alphabetically', async () => {
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    openBrowse()

    const owners = screen.getAllByRole('heading', { level: 2 })
      .map((h) => h.textContent ?? '')
    expect(owners[0]).toContain('amy')
    expect(owners[1]).toContain('zoe')
  })

  it('counts each shelf on its own tab', async () => {
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    expect(screen.getByRole('tab', { name: /my decks/i }).textContent).toContain('2')
    expect(screen.getByRole('tab', { name: /other players/i }).textContent).toContain('2')
  })

  it('keeps the filters working inside a group', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      ...MIXED,
      deck({ slug: 'zoe-two', name: 'Zoe two', owner: 'zoe', showcase: false,
             writable: false, bracket: 5 }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    openBrowse()
    expect(browsedNames()).toHaveLength(3)

    fireEvent.change(screen.getByLabelText('Bracket'), { target: { value: '5' } })
    expect(browsedNames()).toEqual(['Zoe two'])
  })

  // ------------------------------------------------------------ what a tile says

  it('names the owner on somebody else\'s deck and not on your own', async () => {
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    const showcase = screen.getByText('Goreclaw').closest('a')!
    const own = screen.getByText('Mine').closest('a')!
    expect(within(showcase).getByText('aasquier')).toBeTruthy()
    // Not "mitch" on the reader's own tile: a label reading "yours" on every
    // deck is not information.
    expect(within(own).queryByText('mitch')).toBeNull()
  })

  it('marks your own deck private, and never claims that about anyone else\'s',
     async () => {
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    // `shared: false` on the reader's own deck: only they can see it.
    expect(within(screen.getByText('Mine').closest('a')!)
      .getByText('private')).toBeTruthy()
    // Somebody else's private deck is not in this response at all, so the
    // badge must never be rendered from another owner's `shared`.
    openBrowse()
    expect(screen.queryByText('private')).toBeNull()
  })

  it('links to the deck under its owner, not to a bare slug', async () => {
    // The failure this catches is the whole reason the browser half cannot be
    // skipped: `/decks/goreclaw` is a route that no longer exists server-side.
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    expect(screen.getByText('Goreclaw').closest('a')!.getAttribute('href'))
      .toBe('/decks/aasquier/goreclaw')
    expect(screen.getByText('Mine').closest('a')!.getAttribute('href'))
      .toBe('/decks/mitch/my-deck')
  })

  it('deletes the deck of the owner whose tile was clicked', async () => {
    // Two owners with the same slug: the shelf must drop one tile, not both.
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'goreclaw', name: 'My Goreclaw', owner: 'mitch',
             showcase: false, writable: true }),
      deck({ slug: 'goreclaw', name: 'Their Goreclaw', owner: 'aasquier',
             showcase: true, writable: false }),
    ])
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    fireEvent.click(screen.getByRole('button', { name: 'Entomb My Goreclaw' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'bury' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /entomb this deck/i }))

    await waitFor(() => expect(shownNames()).toEqual(['Their Goreclaw']))
    expect(api.deleteDeck).toHaveBeenCalledWith(
      expect.objectContaining({ owner: 'mitch', slug: 'goreclaw' }), 'bury')
  })
})
