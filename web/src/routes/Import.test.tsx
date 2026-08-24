/** The import page.
 *
 * Two things here are load-bearing rather than cosmetic, and both come from
 * ADR 13. The preview must be the *same* request the import sends, with
 * `dry_run` flipped — a preview that estimates is worse than none, because it
 * looks authoritative. And the page must never offer to write a rationale: the
 * result it shows ends on a count of the work still owed.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImportResult } from '../lib/api'
import Import from './Import'

const navigate = vi.fn()
vi.mock('react-router-dom', async () => ({
  ...(await vi.importActual<typeof import('react-router-dom')>('react-router-dom')),
  useNavigate: () => navigate,
}))

vi.mock('../lib/api', async () => ({
  errorMessage: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).errorMessage,
  // Real: it is what turns the response into the deck's address, and a stub
  // would let this navigate to the pre-ADR-22 path with every test passing.
  deckUrl: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).deckUrl,
  api: { importDeck: vi.fn() },
  ApiError: class extends Error {},
}))

const { api } = await import('../lib/api')

function result(overrides: Partial<ImportResult> = {}): ImportResult {
  return {
    slug: 'arahbo-cats',
    owner: 'aasquier',
    name: 'Arahbo — Cats',
    stage: 'draft',
    status: 'theoretical',
    created: false,
    commander: ['Arahbo, Roar of the World'],
    companion: null,
    total_cards: 99,
    land_count: 36,
    swap_board: [],
    needs_rationale: 85,
    unknown: [],
    read: [],
    did_you_mean: [],
    did_you_mean_skipped: 0,
    unreadable: [],
    skipped: [],
    notes: [],
    yaml: 'slug: arahbo-cats\nstage: draft\n',
    ok: true,
    errors: [],
    warnings: [],
    ...overrides,
  }
}

function renderImport() {
  return render(<MemoryRouter><Import /></MemoryRouter>)
}

function paste(text: string) {
  fireEvent.change(screen.getByLabelText('Decklist'), { target: { value: text } })
}

beforeEach(() => {
  navigate.mockReset()
  vi.mocked(api.importDeck).mockReset().mockResolvedValue(result())
})

afterEach(cleanup)

describe('Import', () => {
  it('will not submit an empty list, or one with no slug', () => {
    renderImport()
    expect(screen.getByText('Preview').closest('button')!.disabled).toBe(true)

    paste('1 Sol Ring')
    // Still no name, so no slug.
    expect(screen.getByText('Preview').closest('button')!.disabled).toBe(true)

    fireEvent.change(screen.getByLabelText('Deck name'), {
      target: { value: 'Arahbo — Cats' },
    })
    expect(screen.getByText('Preview').closest('button')!.disabled).toBe(false)
  })

  it('derives a slug from the name until the slug is edited by hand', () => {
    renderImport()
    fireEvent.change(screen.getByLabelText('Deck name'), {
      target: { value: 'Arahbo — Cats!' },
    })
    const slug = screen.getByLabelText('Slug') as HTMLInputElement
    expect(slug.value).toBe('arahbo-cats')

    fireEvent.change(slug, { target: { value: 'my-own-slug' } })
    fireEvent.change(screen.getByLabelText('Deck name'), {
      target: { value: 'Something Else' },
    })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('my-own-slug')
  })

  it('previews with dry_run and writes nothing', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0]).toMatchObject({
      slug: 'cats', text: '1 Sol Ring', dry_run: true,
    })
    expect(navigate).not.toHaveBeenCalled()
  })

  it('sends the identical payload when importing for real', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })

    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledTimes(1))

    vi.mocked(api.importDeck).mockResolvedValue(result({ created: true }))
    fireEvent.click(screen.getByText('Import as draft'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledTimes(2))

    const [previewed, imported] = vi.mocked(api.importDeck).mock.calls.map((c) => c[0])
    expect({ ...previewed, dry_run: false }).toEqual(imported)
  })

  it('goes to the new deck once it is created', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({ created: true }))
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Import as draft'))
    // Owner-qualified, and read off the response rather than assumed: the
    // server chooses which library a new deck lands in (ADR 22).
    await waitFor(() => expect(navigate)
      .toHaveBeenCalledWith('/decks/aasquier/arahbo-cats'))
  })

  /**
   * The commander field is sent WHOLE, and this test used to assert the
   * opposite.
   *
   * It split on commas here, before the request, because a partner pair is
   * two commanders — and `Ley Weaver, Lore Weaver` really is a pair. What the
   * old test could not see is that the same rule turned `Arahbo, Roar of the
   * World` into two commanders, neither of them a card: a comma is
   * punctuation inside most legendary names, and every deck in this library
   * is led by one.
   *
   * Telling those apart takes the card pool, which is on the other side of
   * this wire. So the client sends what was typed and the server decides by
   * looking both readings up (`deckimport.commanderReading`) — the pairing
   * still works, and now it works because the parts are cards rather than
   * because a comma was present.
   */
  it('sends the commander field whole, commas and all', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.change(screen.getByLabelText('Commander'), {
      target: { value: 'Arahbo, Roar of the World' },
    })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0].commander)
      .toEqual(['Arahbo, Roar of the World'])
  })

  it('sends a pairing whole too, and lets the pool decide', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.change(screen.getByLabelText('Commander'), {
      target: { value: 'Ley Weaver + Lore Weaver' },
    })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0].commander)
      .toEqual(['Ley Weaver + Lore Weaver'])
  })

  it('sends no commander at all when the field is blank', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0]?.[0].commander).toEqual([])
  })

  // -------------------------------------------------------------- the report

  it('leads on the count of rationales still owed, and offers to write none', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(screen.getByText(/85 cards will need a/)).toBeTruthy())
    expect(screen.getByText('draft')).toBeTruthy()
    expect(screen.queryByText(/generate|suggest|write .* for you/i)).toBeNull()
  })

  it('shows the gate errors the list already has', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({
      ok: false,
      errors: [{ code: 'banned', card: 'Primeval Titan',
                 message: 'not legal in Commander' }],
    }))
    renderImport()
    paste('1 Primeval Titan')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() => expect(screen.getByText(/not legal in Commander/)).toBeTruthy())
    expect(screen.getByText('1 error(s)')).toBeTruthy()
  })

  it('reports an unresolved name as unresolved, not as a suggestion', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({ unknown: ['Sol Rng'] }))
    renderImport()
    paste('1 Sol Rng')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))

    await waitFor(() =>
      expect(screen.getByText(/1 name the\s+pool does not know/)).toBeTruthy())
    expect(screen.getByText('Sol Rng')).toBeTruthy()
    // The name is reported as written and the list is untouched. This
    // mattered before shortlists existed and it matters more now.
    expect(screen.getByText(/nothing below has been applied/)).toBeTruthy()
    expect((screen.getByLabelText('Decklist') as HTMLTextAreaElement).value)
      .toBe('1 Sol Rng')
  })

  it('names the lines it could not read, with their numbers', async () => {
    vi.mocked(api.importDeck).mockResolvedValue(result({
      unreadable: [{ line: 7, text: '(LTC) 284' }],
    }))
    renderImport()
    paste('junk')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(screen.getByText('line 7: (LTC) 284')).toBeTruthy())
  })

  it('can show the deck.yaml it would write', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(screen.getByText(/Show the deck.yaml/)).toBeTruthy())

    fireEvent.click(screen.getByText(/Show the deck.yaml/))
    expect(screen.getByText(/stage: draft/)).toBeTruthy()
  })

  it('surfaces a refusal instead of failing silently', async () => {
    vi.mocked(api.importDeck).mockRejectedValue(new Error('no commander in this list'))
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Import as draft'))

    await waitFor(() => expect(screen.getByText(/no commander in this list/)).toBeTruthy())
    expect(navigate).not.toHaveBeenCalled()
  })
})

describe('the deck that exists nowhere online', () => {
  /* Every import path is text, so a deck that only exists as a stack of
     cards has nothing to paste — and that is the newcomer's deck. Until the
     camera lands (ROADMAP item 14) this paragraph is the whole feature, so
     it is pinned: it names the apps, and it states the one cap that bites
     at card ninety rather than at card one. */

  beforeEach(() => { cleanup() })

  it('tells someone holding the cards where to start', () => {
    render(<MemoryRouter><Import /></MemoryRouter>)
    expect(screen.getByText(/Only have the cards\?/)).toBeTruthy()
    expect(screen.getByText('Dragon Shield MTG Scanner')).toBeTruthy()
    expect(screen.getByText('ManaBox')).toBeTruthy()
  })

  it('states the export cap rather than letting it be discovered', () => {
    render(<MemoryRouter><Import /></MemoryRouter>)
    expect(screen.getByText(/100 cards a session/)).toBeTruthy()
  })
})

/**
 * The shortlist beside a name the pool does not know.
 *
 * The strictness is the feature -- `deckimport` guesses nothing, ever -- so
 * everything here is about the person accepting a correction, and about the
 * pasted list staying the one source of what gets written.
 */
describe('did you mean', () => {
  const TYPOS = result({
    unknown: ['Sol Rng', 'Cultivate', 'Wgrsdlkj'],
    did_you_mean: [
      { written: 'Sol Rng',
        candidates: [{ name: 'Sol Ring', score: 0.975 }] },
      { written: 'Cultivate',
        candidates: [{ name: 'Cultivator Drone', score: 0.905 },
                     { name: 'Cultivator Colossus', score: 0.902 }] },
    ],
    did_you_mean_skipped: 0,
  })

  async function preview(text: string, payload = TYPOS) {
    vi.mocked(api.importDeck).mockResolvedValue(payload)
    renderImport()
    paste(text)
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
    await screen.findByRole('heading', { name: /does not know/ })
  }

  it('offers the near names, and applies none of them by itself', async () => {
    await preview('1 Sol Rng\n1 Cultivate\n1 Wgrsdlkj')
    expect(screen.getByRole('button', { name: 'Sol Ring' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Cultivator Drone' })).toBeTruthy()
    expect((screen.getByLabelText('Decklist') as HTMLTextAreaElement).value)
      .toContain('Sol Rng')
  })

  it('says so when nothing in the pool is close', async () => {
    await preview('1 Sol Rng\n1 Cultivate\n1 Wgrsdlkj')
    expect(screen.getByText('Wgrsdlkj')).toBeTruthy()
    expect(screen.getByText(/nothing in the pool is close to this one/))
      .toBeTruthy()
  })

  it('warns that a miss can be a new card rather than a typo', async () => {
    await preview('1 Sol Rng')
    expect(screen.getByText(/printed since this pool was last refreshed/))
      .toBeTruthy()
  })

  it('rewrites the pasted list when a name is pressed, and previews again',
     async () => {
    await preview('4 Forest\n1 Sol Rng\n1 Cultivate')
    vi.mocked(api.importDeck).mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Sol Ring' }))

    const box = screen.getByLabelText('Decklist') as HTMLTextAreaElement
    expect(box.value).toBe('4 Forest\n1 Sol Ring\n1 Cultivate')
    // And the preview is re-run from the corrected list rather than from
    // state that has not landed yet.
    await waitFor(() => expect(api.importDeck).toHaveBeenCalledWith(
      expect.objectContaining({
        text: '4 Forest\n1 Sol Ring\n1 Cultivate', dry_run: true })))
  })

  it('rewrites whole names, never substrings of a card that was right',
     async () => {
    // `Cultivate` is the misspelling here AND a substring of a correct card
    // on the line above it. A bare replace would corrupt the good one.
    await preview('1 Cultivator Colossus\n1 Cultivate')
    fireEvent.click(screen.getByRole('button', { name: 'Cultivator Drone' }))
    expect((screen.getByLabelText('Decklist') as HTMLTextAreaElement).value)
      .toBe('1 Cultivator Colossus\n1 Cultivator Drone')
  })

  it('reports the misses it did not check rather than hiding the cap',
     async () => {
    await preview('1 Sol Rng', result({
      unknown: ['Sol Rng'],
      did_you_mean: [{ written: 'Sol Rng',
        candidates: [{ name: 'Sol Ring', score: 0.975 }] }],
      did_you_mean_skipped: 20,
    }))
    expect(screen.getByText(/20 more went unchecked/)).toBeTruthy()
  })
})

/**
 * The correction itself, which happens on the server and is only reported
 * here.
 *
 * Aaron's ruling, 2026-08-24: do the matching on the backend and do not let
 * misspelled things in. So by the time this page renders, the deck already
 * holds the real card -- and the whole obligation on the client is to say so
 * where somebody will see it.
 */
describe('names that were read', () => {
  const READ = result({
    read: [
      { written: 'Sol Rng', read: 'Sol Ring', score: 0.975 },
      { written: 'Rhystic Studdy', read: 'Rhystic Study', score: 0.9857 },
    ],
  })

  async function preview(payload: ImportResult) {
    vi.mocked(api.importDeck).mockResolvedValue(payload)
    renderImport()
    paste('1 Sol Rng\n1 Rhystic Studdy')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.click(screen.getByText('Preview'))
  }

  it('says what was read, and as what', async () => {
    await preview(READ)
    await waitFor(() =>
      expect(screen.getByText(/2 names were read as the card/)).toBeTruthy())
    expect(screen.getByText('Sol Rng')).toBeTruthy()
    expect(screen.getByText('Sol Ring')).toBeTruthy()
    expect(screen.getByText('Rhystic Study')).toBeTruthy()
  })

  it('says the deck holds the real card, not the string that was typed',
     async () => {
    await preview(READ)
    await waitFor(() =>
      expect(screen.getByText(/its cost, its colours and its\s+legality/))
        .toBeTruthy())
  })

  it('counts one correction in the singular', async () => {
    await preview(result({
      read: [{ written: 'Sol Rng', read: 'Sol Ring', score: 0.975 }],
    }))
    await waitFor(() =>
      expect(screen.getByText(/1 name was read as the card/)).toBeTruthy())
  })

  it('says nothing at all when nothing needed reading', async () => {
    await preview(result())
    await waitFor(() => expect(screen.getByText(/99 cards in the 99/)).toBeTruthy())
    expect(screen.queryByText(/read as the card/)).toBeNull()
  })
})
