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
import type { DeckTile, EntombedDeck, Health } from '../lib/api'
import Library from './Library'

vi.mock('../lib/api', async () => ({
  // Real, and it has to be: the shelf's links are the only thing that says
  // where a deck lives, and a stub would let them all point at the pre-ADR-22
  // path while every assertion below still passed.
  deckUrl: (await vi.importActual<typeof import('../lib/api')>('../lib/api')).deckUrl,
  api: { decks: vi.fn(), health: vi.fn(), deleteDeck: vi.fn(),
         colors: vi.fn(), lore: vi.fn(),
         entombed: vi.fn(), returnEntombed: vi.fn() },
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
    // Two fields, and the handle is opaque on purpose: it names an entry in
    // the crypt and nothing else. This used to be `moved_to:
    // 'decks/.trash/goreclaw-20260811T220000Z'` and the page printed it.
    recoverable: true, crypt_id: 'a1b2c3d4e5f60718',
    total_cards: 99, stage: 'curated', status: 'built',
  })
  // An empty crypt is the default, so the tests that are not about the crypt
  // describe a library nobody has deleted anything from.
  vi.mocked(api.entombed).mockReset().mockResolvedValue({ entombed: [] })
  vi.mocked(api.returnEntombed).mockReset().mockResolvedValue(
    { slug: 'goreclaw', name: 'Goreclaw', restored: true })
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
 * **the way back is on screen** — a deletion is only survivable if you can get
 * the deck back.
 *
 * That last clause used to be *"if you can see where it went"*, and it was
 * satisfied by printing `decks/.trash/goreclaw-20260811T220000Z` on the page
 * with the advice that the deletion was "reversible from the shell". The
 * requirement was right and the answer was a leak (commandment 10) and, for
 * anybody without a terminal, no answer at all. Same requirement, met with a
 * button: `denies the player a path` holds the leak closed, and `offers the
 * way back` holds the requirement it was standing in for.
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

  async function entomb(name: string) {
    const dialog = await openDialogFor(name)
    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'bury' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /entomb this deck/i }))
  }

  it('drops the deck from the shelf and offers the way back', async () => {
    await entomb('Goreclaw')

    await waitFor(() => expect(shownNames()).toEqual(['Arahbo', 'Tivit']))
    // "Deleted" and "recoverable" are separate facts, and the second one is
    // the reason anyone presses the button without dread.
    const notice = await screen.findByRole('status')
    expect(notice.textContent).toMatch(/rests in your crypt/i)
    const back = within(notice).getByRole('button', { name: /return it/i })

    fireEvent.click(back)
    await waitFor(() =>
      expect(api.returnEntombed).toHaveBeenCalledWith('a1b2c3d4e5f60718'))
  })

  // **The regression.** Nothing about a deletion tells the player where their
  // deck is on a disk, or suggests they open a shell — not the dialog they
  // read before pressing the button, and not the notice they read after.
  it('denies the player a path, a trash directory or a shell', async () => {
    const dialog = await openDialogFor('Goreclaw')
    const forbidden = /\.trash|decks\/|shell|terminal|filesystem|file system|directory|folder/i
    expect(dialog.textContent ?? '').not.toMatch(forbidden)

    fireEvent.change(within(dialog).getByRole('textbox'),
                     { target: { value: 'bury' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /entomb this deck/i }))

    const notice = await screen.findByRole('status')
    expect(notice.textContent ?? '').not.toMatch(forbidden)
    // And the opaque handle is a handle, not a caption: it is passed to the
    // server and never drawn.
    expect(notice.textContent ?? '').not.toContain('a1b2c3d4e5f60718')
  })

  // A server that buried the deck but could not hand back a handle says so by
  // omission, and the page says the true thing instead of offering a button
  // that cannot work. The claim "there is no way back" would be false, and is
  // not made.
  it('points at the crypt when no handle came back', async () => {
    vi.mocked(api.deleteDeck).mockResolvedValue({
      slug: 'goreclaw', name: 'Goreclaw', deleted: true,
      recoverable: false, crypt_id: '',
      total_cards: 99, stage: 'curated', status: 'built',
    })
    await entomb('Goreclaw')

    const notice = await screen.findByRole('status')
    expect(notice.textContent).toMatch(/raise it again from the Crypt tab/i)
    expect(within(notice).queryByRole('button', { name: /return it/i })).toBeNull()
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

  it('wraps the strip, so a third shelf can never widen a phone', async () => {
    // Two tabs and their counts fit a 375px phone with room to spare today,
    // which is exactly why this is worth a line: the same strip on the deck
    // page grew to six tabs and pushed 81px of horizontal scroll onto every
    // page of the site before anybody noticed. One word stops that happening
    // here, and it costs nothing while there are only two.
    //
    // jsdom has no layout, so this holds the class and never the width — see
    // the note over the matching case in `DeckDetail.test.tsx`.
    vi.mocked(api.decks).mockResolvedValue(MIXED)
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(2))
    const strip = screen.getByRole('tablist', { name: 'Whose decks' })
    expect(strip.className).toContain('flex-wrap')
    expect(strip.className).toContain('flex ')
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

/**
 * The crypt.
 *
 * ADR 27 gave a *card* both halves — entomb, and a graveyard to return from.
 * A *deck* had only the first, and what stood in for the second was a line of
 * copy naming the folder it had been moved to. This is the missing half, and
 * three of these tests are about what the page must not claim rather than
 * about what it shows:
 *
 *  - a crypt that could not be read is **not** an empty crypt, and the tab
 *    carries no count rather than a zero;
 *  - a burial with no recorded time is not "just now";
 *  - a refusal is shown rather than swallowed, and the deck stays buried.
 *
 * Every one of those is this repo's most-repeated bug — a fallback rendered as
 * a confident claim — aimed at the one screen somebody opens to check that
 * work they thought they had lost is still there.
 */
describe('Library crypt', () => {
  const HANDLE = 'a1b2c3d4e5f60718'

  /** A fixture built when the test runs, not when the file is collected.
   *
   * `entombed_at` is relative to *now*, and the suite takes a minute and a
   * half to reach here — a constant fixed at module scope drifted from "5
   * minutes ago" to "7 minutes ago" while the tests before it ran, which is a
   * flake that would have arrived later and looked like a rendering bug. */
  const buried = (over: Partial<EntombedDeck> = {}): EntombedDeck => ({
    id: HANDLE,
    slug: 'goreclaw',
    name: 'Goreclaw',
    total_cards: 99,
    commander: ['Goreclaw, Terror of Qal Sisma'],
    entombed_at: new Date(Date.now() - 5 * 60_000).toISOString(),
    ...over,
  })

  async function openCrypt() {
    renderLibrary()
    const tab = await screen.findByRole('tab', { name: /crypt/i })
    fireEvent.click(tab)
    return tab
  }

  it('offers no crypt tab when nothing is buried', async () => {
    renderLibrary()
    await waitFor(() => expect(shownNames()).toHaveLength(3))
    expect(screen.queryByRole('tab', { name: /crypt/i })).toBeNull()
  })

  it('lists an entombed deck and brings it back', async () => {
    vi.mocked(api.entombed).mockResolvedValue({ entombed: [buried()] })
    await openCrypt()

    expect(await screen.findByText(/99 cards/)).toBeTruthy()
    expect(screen.getByText(/entombed 5 minutes ago/)).toBeTruthy()

    // The shelf and the crypt are both re-read from the server afterwards
    // rather than patched here: a deck tile carries the gate's counts and its
    // art, and this page is not the authority on either.
    vi.mocked(api.entombed).mockResolvedValue({ entombed: [] })
    fireEvent.click(screen.getByRole('button', { name: /^return$/i }))
    await waitFor(() => expect(api.returnEntombed).toHaveBeenCalledWith(HANDLE))
    await waitFor(() => expect(api.decks).toHaveBeenCalledTimes(2))
  })

  // Found by walking it: returning the last buried deck empties the crypt, the
  // tab that was only there because something was buried disappeared, and the
  // player was left standing in "nothing rests here" with no way back to their
  // shelf. A tab somebody is standing on is never taken away.
  it('keeps the way out when returning the last deck empties the crypt', async () => {
    vi.mocked(api.entombed).mockResolvedValue({ entombed: [buried()] })
    await openCrypt()

    vi.mocked(api.entombed).mockResolvedValue({ entombed: [] })
    fireEvent.click(await screen.findByRole('button', { name: /^return$/i }))

    expect(await screen.findByText(/nothing rests here/i)).toBeTruthy()
    // Both tabs still there, and the empty count is honest — the crypt was
    // read, and it is empty.
    expect(screen.getByRole('tab', { name: /my decks/i })).toBeTruthy()
    expect(screen.getByRole('tab', { name: /crypt/i }).textContent).toMatch(/0/)
  })

  it('says nothing about when a deck with no recorded burial went in', async () => {
    vi.mocked(api.entombed).mockResolvedValue(
      { entombed: [buried({ entombed_at: null })] })
    await openCrypt()

    expect(await screen.findByText(/99 cards/)).toBeTruthy()
    // No date, no "just now", no "Invalid Date" — the row says the deck is
    // entombed and still there, and claims nothing about when.
    expect(screen.queryByText(/entombed \d/i)).toBeNull()
    expect(screen.getByText(/entombed, and still here/i)).toBeTruthy()
  })

  // A count of zero is what the server sends when it could not read the deck
  // file, and "0 cards" beside a deck somebody knows had 99 is the worst
  // sentence available on the screen they came to for reassurance. The row
  // leaves the count out instead.
  it('does not claim a deck it could not measure is empty', async () => {
    vi.mocked(api.entombed).mockResolvedValue(
      { entombed: [buried({ total_cards: 0, commander: [] })] })
    await openCrypt()

    expect(await screen.findByText(/entombed 5 minutes ago/)).toBeTruthy()
    // Read off the row, not off the page: the masthead says "35,000 cards in
    // the local pool", and a loose `/0 cards/` matches *that* — which is how
    // this assertion passed against the wrong element on the first run.
    expect(screen.getByRole('listitem').textContent).not.toMatch(/cards/)
  })

  it('does not report a crypt it could not read as an empty one', async () => {
    vi.mocked(api.entombed).mockRejectedValue(new Error('the library is asleep'))
    const tab = await openCrypt()

    // The tab is offered — something might be in there — but it makes no claim
    // about how many, which a `0` would.
    expect(tab.textContent).not.toMatch(/\d/)
    expect(await screen.findByText(/could not be read/i)).toBeTruthy()
    expect(screen.queryByText(/nothing rests here/i)).toBeNull()
  })

  it('shows a refusal and leaves the deck buried', async () => {
    vi.mocked(api.entombed).mockResolvedValue({ entombed: [buried()] })
    vi.mocked(api.returnEntombed).mockRejectedValue(
      new Error('a deck called "goreclaw" is already on your shelf'))
    await openCrypt()

    fireEvent.click(await screen.findByRole('button', { name: /^return$/i }))
    expect(await screen.findByText(/already on your shelf/)).toBeTruthy()
    // Still listed: a refused return changed nothing.
    expect(screen.getByText(/99 cards/)).toBeTruthy()
  })

  // The crypt names no path either — the leak was in the deletion's copy, and
  // this is the room that replaced it.
  it('names no path, no trash directory and no shell', async () => {
    vi.mocked(api.entombed).mockResolvedValue({ entombed: [buried()] })
    await openCrypt()
    await screen.findByText(/99 cards/)
    const page = document.body.textContent ?? ''
    expect(page).not.toMatch(/\.trash|decks\/|shell|terminal|filesystem|directory|folder/i)
    expect(page).not.toContain(HANDLE)
  })
})
