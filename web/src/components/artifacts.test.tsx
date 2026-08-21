/**
 * The deck page's artifact shelf, and the properties that matter:
 *
 * - **`baseline` is a readout, never recomputed.** The server compares the
 *   stored snapshot against the deck; this renders that answer. A second copy
 *   of the comparison here would disagree with the Python one silently — the
 *   rule the stance dial and the labels editor already follow.
 * - **`unknown` is a real third state**, not a stale boolean wearing a
 *   disguise. Every artifact on the volume was in it on 2026-08-21.
 * - **A draft's shelf is shut** (ADR 13) and offers no flag that opens it.
 * - **"Build anyway" answers the gate**, and only appears once the gate has
 *   actually refused — a standing invitation to ignore the gate is not one.
 * - **A reader may look and not build.** The artifacts are the shareable
 *   surface, so the list renders without a build control.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('../lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api')>()
  return {
    ...actual,
    api: {
      deckArtifacts: vi.fn(),
      deckArtifact: vi.fn(),
      buildArtifacts: vi.fn(),
    },
  }
})

const { api } = await import('../lib/api')
const { DeckArtifactsPanel } = await import('./artifacts')
import type { DeckArtifacts, DeckDetail, DeckRef } from '../lib/api'

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

function shelf(over: Partial<DeckArtifacts> = {}): DeckArtifacts {
  return {
    artifacts: [
      { name: 'primer-quick.md', size: 3441, built_at: '2026-08-13T14:58:00Z' },
      { name: 'moxfield.txt', size: 1681, built_at: '2026-08-13T14:58:00Z' },
    ],
    baseline: 'current', buildable: true, stage: 'curated',
    ...over,
  }
}

describe('DeckArtifactsPanel', () => {
  it('shuts the shelf on a draft and offers no way to force it open', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(
      shelf({ buildable: false, artifacts: [], stage: 'draft' }))
    render(<DeckArtifactsPanel deck={deck({ stage: 'draft' })} deckRef={ref} />)

    await screen.findByText(/artifacts stay shut/i)
    expect(screen.queryByRole('button', { name: /build/i })).toBeNull()
  })

  it('says a deck has never been built rather than showing an empty shelf', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(
      shelf({ artifacts: [], baseline: 'unknown' }))
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    await screen.findByText(/never been built/i)
    // Not "Unknown": with nothing built there is no baseline question to ask.
    expect(screen.queryByText('Unknown')).toBeNull()
    expect(screen.getByRole('button', { name: /build the five/i })).toBeTruthy()
  })

  it('renders the server\'s baseline verdict rather than deciding one', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf({ baseline: 'different' }))
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    await screen.findByText('Out of date')
    expect(screen.getByText(/deck has changed since these were built/i)).toBeTruthy()
  })

  it('keeps `unknown` as its own answer, distinct from out of date', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf({ baseline: 'unknown' }))
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    await screen.findByText('Unknown')
    expect(screen.getByText(/nothing can say whether they still match/i)).toBeTruthy()
    expect(screen.queryByText('Out of date')).toBeNull()
  })

  it('lets a reader look without offering them a build control', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf())
    render(<DeckArtifactsPanel deck={deck({ writable: false })} deckRef={ref} />)

    await screen.findByText('primer-quick.md')
    expect(screen.queryByRole('button', { name: /build/i })).toBeNull()
  })

  it('rebuilds and shows what came back', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf({ baseline: 'different' }))
    vi.mocked(api.buildArtifacts).mockResolvedValue(shelf({
      baseline: 'current',
      artifacts: [...shelf().artifacts,
                  { name: 'swaps.md', size: 900, built_at: '2026-08-21T16:00:00Z' }],
    }))
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    fireEvent.click(await screen.findByRole('button', { name: /rebuild/i }))
    await screen.findByText('Current')
    expect(vi.mocked(api.buildArtifacts).mock.calls[0]?.[1]).toBe(false)
    expect(screen.getByText('swaps.md')).toBeTruthy()
  })

  it('offers "build anyway" only after the gate has actually refused', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf())
    vi.mocked(api.buildArtifacts).mockRejectedValueOnce(
      new Error('the gate reports 1 error(s) on gyome-food.'))
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    await screen.findByText('primer-quick.md')
    expect(screen.queryByRole('button', { name: /build anyway/i })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /rebuild/i }))
    const anyway = await screen.findByRole('button', { name: /build anyway/i })

    vi.mocked(api.buildArtifacts).mockResolvedValue(shelf())
    fireEvent.click(anyway)
    await waitFor(() => {
      expect(vi.mocked(api.buildArtifacts).mock.calls[1]?.[1]).toBe(true)
    })
  })

  it('fetches one deliverable\'s text only when it is opened', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf())
    vi.mocked(api.deckArtifact).mockResolvedValue(
      { name: 'moxfield.txt', text: '1 Gyome, Master Chef\n36 Swamp\n' })
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    await screen.findByText('moxfield.txt')
    expect(api.deckArtifact).not.toHaveBeenCalled()

    fireEvent.click(screen.getByText('moxfield.txt'))
    await screen.findByText(/36 Swamp/)
    expect(vi.mocked(api.deckArtifact).mock.calls[0]?.[1]).toBe('moxfield.txt')
  })

  it('reports a failed read without taking the shelf down', async () => {
    vi.mocked(api.deckArtifacts).mockResolvedValue(shelf())
    vi.mocked(api.deckArtifact).mockRejectedValue(new Error('no artifact'))
    render(<DeckArtifactsPanel deck={deck()} deckRef={ref} />)

    fireEvent.click(await screen.findByText('primer-quick.md'))
    await screen.findByText(/no artifact/i)
    // The other card is still there: one bad read is not a dead panel.
    expect(screen.getByText('moxfield.txt')).toBeTruthy()
  })
})
