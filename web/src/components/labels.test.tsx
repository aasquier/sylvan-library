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
      describeDeck: vi.fn(),
    },
  }
})

const { api } = await import('../lib/api')
const { DeckLabels } = await import('./labels')
import type { ClaudeStatus, DeckDescriptionDraft, DeckDetail, DeckRef } from '../lib/api'

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

/**
 * Asking Claude which themes fit.
 *
 * The property under test is not "it ticks boxes" — it is **whose the boxes
 * stay**. Claude reads the list and turns chips on; the person is looking at
 * the same editor with the same save button, every chip is still a toggle, and
 * nothing reaches `deck.yaml` until they press save. That is the same line the
 * description panel holds one section up: it drafts, you keep the pen.
 */
describe('DeckLabels, asking Claude', () => {
  function draft(over: Partial<DeckDescriptionDraft> = {}): DeckDescriptionDraft {
    return {
      answered_by: 'claude', mode: 'deck-description', slug: 'gyome-food',
      asked: true, reason: '', stance: { preset: 'consultant', axes: [] } as never,
      strategy: 'It cooks.', themes: ['food'], themes_dropped: [],
      fact: 'Fourteen sacrifice outlets.', never: 'Nothing is saved until you save it.',
      ...over,
    }
  }

  // A status that says Claude can answer here: installed, keyed, and a dial
  // that is not off. All three are checked, because all three are ordinary
  // states in which the control must not appear at all.
  const READY = { installed: true, configured: true,
    stance: { preset: 'consultant', axes: [{ level: 'consultant' }] } } as unknown as ClaudeStatus

  async function openEditor(claude: ClaudeStatus | null = READY) {
    vi.mocked(api.themes).mockResolvedValue(VOCAB)
    render(<DeckLabels deck={deck({ themes: ['sacrifice'] })} deckRef={ref}
                       claude={claude} onRefresh={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: /Change themes|Declare themes/ }))
    await waitFor(() => screen.getByText(/What is this deck about/))
  }

  it('ticks what it read, and keeps what was already ticked', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(draft())
    await openEditor()
    fireEvent.click(screen.getByRole('button', { name: /Ask Claude which fit/ }))
    await waitFor(() => screen.getByText(/Claude read the list and ticked/))

    // Both are on: the one it read, and the one somebody had already chosen.
    // Replacing the selection is the obvious implementation and it throws away
    // a person's own labels for a suggestion they have not read yet.
    expect(screen.getByRole('button', { name: 'food' })
      .getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: 'sacrifice' })
      .getAttribute('aria-pressed')).toBe('true')
    // And nothing was written on the way.
    expect(api.setDeckField).not.toHaveBeenCalled()
  })

  it('leaves every ticked chip a toggle', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(draft())
    await openEditor()
    fireEvent.click(screen.getByRole('button', { name: /Ask Claude which fit/ }))
    await waitFor(() => screen.getByText(/Claude read the list and ticked/))
    fireEvent.click(screen.getByRole('button', { name: 'food' }))
    expect(screen.getByRole('button', { name: 'food' })
      .getAttribute('aria-pressed')).toBe('false')
  })

  it('says what it read that this library cannot file', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(
      draft({ themes: ['food'], themes_dropped: ['sparklecrunch'] }))
    await openEditor()
    fireEvent.click(screen.getByRole('button', { name: /Ask Claude which fit/ }))
    await waitFor(() => screen.getByText(/no label for yet/))
    expect(screen.getByText(/sparklecrunch/)).toBeTruthy()
  })

  // A dial set to stay silent is a real position, not a fault. `asked: false`
  // with a reason renders as the reason — a client that showed an error there
  // would tell somebody their instance is broken when their dial is merely
  // down.
  it('renders a silent dial as what it is', async () => {
    vi.mocked(api.describeDeck).mockResolvedValue(
      draft({ asked: false, themes: [], strategy: '',
              reason: 'Claude is set to stay silent for this deck.' }))
    await openEditor()
    fireEvent.click(screen.getByRole('button', { name: /Ask Claude which fit/ }))
    await waitFor(() => screen.getByText(/set to stay silent/))
    expect(screen.queryByText(/Claude read the list and ticked/)).toBeNull()
  })

  // An instance serving a payload from before the key existed. Undefined is
  // not an empty list, and `.length` on it takes the panel down.
  it('survives a payload with no dropped-themes key', async () => {
    const old = draft()
    delete (old as { themes_dropped?: string[] }).themes_dropped
    vi.mocked(api.describeDeck).mockResolvedValue(old)
    await openEditor()
    fireEvent.click(screen.getByRole('button', { name: /Ask Claude which fit/ }))
    await waitFor(() => screen.getByText(/Claude read the list and ticked/))
    expect(screen.queryByText(/no label for yet/)).toBeNull()
  })

  // Three ordinary states, one rule: no control at all. A button that can only
  // refuse tells a newcomer the site is broken when it is doing as it was told
  // (ADR 15, commandment 2). Found by walking it on an instance with no key.
  it.each([
    ['no key on this instance', { installed: true, configured: false,
      stance: { axes: [{ level: 'consultant' }] } }],
    ['the feature is not installed', { installed: false, configured: false,
      stance: { axes: [{ level: 'consultant' }] } }],
    ['the dial is set to stay silent', { installed: true, configured: true,
      stance: { axes: [{ level: 'off' }] } }],
    ['nothing is known yet', null],
  ])('offers nothing to ask when %s', async (_what, status) => {
    await openEditor(status as unknown as ClaudeStatus | null)
    expect(screen.queryByRole('button', { name: /Ask Claude which fit/ })).toBeNull()
    // ...and the blurb goes back to the sentence that is true without it.
    expect(screen.getByText(/nothing here is guessed from the decklist/)).toBeTruthy()
  })
})
