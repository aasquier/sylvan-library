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

vi.mock('../lib/api', () => ({
  api: { importDeck: vi.fn() },
  ApiError: class extends Error {},
}))

const { api } = await import('../lib/api')

function result(overrides: Partial<ImportResult> = {}): ImportResult {
  return {
    slug: 'arahbo-cats',
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
    expect(vi.mocked(api.importDeck).mock.calls[0][0]).toMatchObject({
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
    await waitFor(() => expect(navigate).toHaveBeenCalledWith('/decks/arahbo-cats'))
  })

  it('splits a comma-separated commander field into a partner pair', async () => {
    renderImport()
    paste('1 Sol Ring')
    fireEvent.change(screen.getByLabelText('Deck name'), { target: { value: 'Cats' } })
    fireEvent.change(screen.getByLabelText('Commander'), {
      target: { value: 'Ley Weaver, Lore Weaver' },
    })
    fireEvent.click(screen.getByText('Preview'))
    await waitFor(() => expect(api.importDeck).toHaveBeenCalled())
    expect(vi.mocked(api.importDeck).mock.calls[0][0].commander)
      .toEqual(['Ley Weaver', 'Lore Weaver'])
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
      expect(screen.getByText(/1 name the\s+corpus does not know/)).toBeTruthy())
    expect(screen.getByText('Sol Rng')).toBeTruthy()
    expect(screen.getByText(/Nothing was guessed/)).toBeTruthy()
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
