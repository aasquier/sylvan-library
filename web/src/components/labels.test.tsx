/**
 * The deck-page labels editor, and the four properties that matter:
 *
 * - **An unlabelled deck somebody else owns says nothing.** "No themes" is
 *   not a fact worth a line on a deck the reader cannot label.
 * - **The archetype is a readout, never a prediction.** ADR 37 makes it a
 *   reading of the declared themes, and that reading is the server's. While
 *   editing, the editor names which ticked words are class words and stops
 *   there — it does not compute a winner. A second copy of worst-piloted-wins
 *   living here would disagree with the served one silently.
 * - **The vocabulary is served, not copied.** The chips come from
 *   `GET /api/themes`; nothing in the client holds a theme list.
 * - **What is saved is sorted**, so a `deck.yaml` diff shows what changed
 *   rather than what was clicked in what order.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return {
    ...actual,
    api: {
      themes: vi.fn(),
      setDeckField: vi.fn(),
    },
  }
})

const { api } = await import('../lib/api')
const { DeckLabels } = await import('./labels')
import type { DeckDetail, DeckRef } from '../lib/api'

afterEach(() => { cleanup(); vi.clearAllMocks() })

const ref: DeckRef = { owner: 'local', slug: 'gyome-food' }

function deck(over: Partial<DeckDetail> = {}): DeckDetail {
  return {
    owner: 'local', slug: 'gyome-food', name: 'Gyome, Master Chef',
    shared: true, pilot: '', status: 'built', stage: 'curated',
    writable: true, needs_rationale: 0, commander: ['Gyome, Master Chef'],
    companion: null, bracket: 4, themes: [], archetype: null,
    total_cards: 99, land_count: 36, strategy: '', art_crop: null,
    color_identity: ['B', 'G'], errors: 0, warnings: 0,
    ...over,
  } as DeckDetail
}

const VOCAB = {
  themes: ['aggro', 'midrange', 'control', 'combo', 'food', 'sacrifice'],
  archetypes: ['aggro', 'midrange', 'control', 'combo'],
}

describe('DeckLabels', () => {
  it('says nothing about an unlabelled deck the reader cannot label', () => {
    const { container } = render(
      <DeckLabels deck={deck({ themes: [], writable: false })}
                  deckRef={ref} onRefresh={() => {}} />)
    expect(container.innerHTML).toBe('')
  })

  // The crash this component shipped with for about an hour. `DeckDetail`'s
  // own fixtures are cast (`as unknown as Deck`), so a payload with no
  // `themes` was not a type error -- it was `.length` on undefined, thrown
  // inside the deck page, taking the whole route down over a label line.
  // A deploy changes both halves and the browser is the half that lies, so a
  // new bundle really can put this question to a server that has not
  // restarted yet.
  it('survives a payload that carries no themes at all', () => {
    const bare = { ...deck(), writable: true }
    delete (bare as { themes?: string[] }).themes
    expect(() => render(
      <DeckLabels deck={bare as DeckDetail} deckRef={ref} onRefresh={() => {}} />
    )).not.toThrow()
    expect(screen.getByRole('button', { name: /declare themes/i })).toBeTruthy()
  })

  it('reads out the declared themes and the archetype the server resolved', () => {
    render(<DeckLabels deck={deck({ themes: ['food', 'sacrifice', 'combo'],
                                    archetype: 'combo', writable: false })}
                       deckRef={ref} onRefresh={() => {}} />)
    expect(screen.getByText('food')).toBeTruthy()
    expect(screen.getByText('sacrifice')).toBeTruthy()
    // The readout names the server's answer, and says whose reading it is.
    expect(screen.getByText(/the boards read this as/)).toBeTruthy()
  })

  it('offers the vocabulary from the server, never a copy held here', async () => {
    vi.mocked(api.themes).mockResolvedValue(VOCAB)
    render(<DeckLabels deck={deck()} deckRef={ref} onRefresh={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /declare themes/i }))
    await waitFor(() => expect(api.themes).toHaveBeenCalled())
    // Every chip the editor shows came out of that response.
    for (const t of VOCAB.themes) {
      expect(screen.getByRole('button', { name: t })).toBeTruthy()
    }
  })

  it('does not predict the archetype while editing', async () => {
    vi.mocked(api.themes).mockResolvedValue(VOCAB)
    render(<DeckLabels deck={deck()} deckRef={ref} onRefresh={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /declare themes/i }))
    await waitFor(() => expect(api.themes).toHaveBeenCalled())

    fireEvent.click(screen.getByRole('button', { name: 'control' }))
    fireEvent.click(screen.getByRole('button', { name: 'combo' }))

    // It names the class words it will be read from -- and does not announce
    // a winner. If this ever asserts "combo", the reading has been copied
    // into TypeScript and ADR 37 now lives in two places.
    expect(screen.getByText(/board will be read from/)).toBeTruthy()
    expect(screen.queryByText(/the boards read this as/)).toBeNull()
  })

  it('saves the chosen themes sorted, and refreshes', async () => {
    vi.mocked(api.themes).mockResolvedValue(VOCAB)
    vi.mocked(api.setDeckField).mockResolvedValue({} as never)
    const onRefresh = vi.fn()
    render(<DeckLabels deck={deck({ themes: ['sacrifice'] })} deckRef={ref}
                       onRefresh={onRefresh} />)
    fireEvent.click(screen.getByRole('button', { name: /change themes/i }))
    await waitFor(() => expect(api.themes).toHaveBeenCalled())

    fireEvent.click(screen.getByRole('button', { name: 'food' }))
    fireEvent.click(screen.getByRole('button', { name: 'combo' }))
    fireEvent.click(screen.getByRole('button', { name: /save themes/i }))

    await waitFor(() => expect(api.setDeckField).toHaveBeenCalledWith(
      ref, 'themes', ['combo', 'food', 'sacrifice']))
    await waitFor(() => expect(onRefresh).toHaveBeenCalled())
  })

  it('untoggles a declared theme', async () => {
    vi.mocked(api.themes).mockResolvedValue(VOCAB)
    vi.mocked(api.setDeckField).mockResolvedValue({} as never)
    render(<DeckLabels deck={deck({ themes: ['food', 'combo'] })} deckRef={ref}
                       onRefresh={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /change themes/i }))
    await waitFor(() => expect(api.themes).toHaveBeenCalled())

    expect(screen.getByRole('button', { name: 'food' })
      .getAttribute('aria-pressed')).toBe('true')
    fireEvent.click(screen.getByRole('button', { name: 'food' }))
    expect(screen.getByRole('button', { name: 'food' })
      .getAttribute('aria-pressed')).toBe('false')

    fireEvent.click(screen.getByRole('button', { name: /save themes/i }))
    await waitFor(() => expect(api.setDeckField).toHaveBeenCalledWith(
      ref, 'themes', ['combo']))
  })
})
