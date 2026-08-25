/**
 * The Simulator, now that it is only arithmetic.
 *
 * Real games left this screen for `/coliseum` (Aaron's call, 2026-08-25), and
 * the properties left here are the ones a green backend suite cannot see from
 * its side of the wire:
 *
 * - **the Forge is not offered here at all**, and the screen says where it
 *   went — a capability that disappears with no forwarding address is how a
 *   newcomer concludes the tool cannot do a thing it does (commandment 2);
 * - a run names the cards it will leave out **before** it is paid for, and
 *   asks the gate nothing about a deck the shelf already called clean;
 * - the button answers the click rather than the job, because the gap between
 *   the two is where "did that land?" lives.
 *
 * Assertions match this file's own strings, not the payload's, for the
 * reason Research.test.tsx records: a test that asserts the server's text
 * back at itself is not testing the renderer.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeckTile } from '../lib/api'
import Simulator from './Simulator'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: { decks: vi.fn(), simMana: vi.fn(), simLands: vi.fn(),
           simPolicy: vi.fn(), simShelf: vi.fn(), job: vi.fn(),
           glossary: vi.fn(), validate: vi.fn() },
  }
})

const { api } = await import('../lib/api')

const DECKS = [
  { slug: 'goreclaw', owner: 'local', name: 'Goreclaw Stompy', pilot: '',
    writable: true },
  { slug: 'gyome', owner: 'local', name: 'Gyome Food', pilot: '',
    writable: true },
] as unknown as DeckTile[]

function mount() {
  return render(<MemoryRouter><Simulator /></MemoryRouter>)
}

beforeEach(() => {
  vi.mocked(api.decks).mockResolvedValue(DECKS)
  vi.mocked(api.glossary).mockResolvedValue(
    { terms: [] } as unknown as Awaited<ReturnType<typeof api.glossary>>)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('what this screen is for', () => {
  it('offers four simulations and no match', async () => {
    mount()
    await waitFor(() =>
      expect(screen.getByText('Mana & consistency')).toBeTruthy())
    expect(screen.getByText('Land count sweep')).toBeTruthy()
    expect(screen.getByText('The closed form')).toBeTruthy()
    expect(screen.getByText('Mulligan policy search')).toBeTruthy()
    // The one that left. Absent, and not as a disabled option either.
    expect(screen.queryByText('Real games (Forge)')).toBeNull()
    expect(screen.queryByLabelText(/Opponent/i)).toBeNull()
  })

  it('says where the real games went', async () => {
    mount()
    await waitFor(() =>
      expect(screen.getByText('Mana & consistency')).toBeTruthy())
    // A link somebody can follow, not a sentence naming a room with no door
    // in it.
    const door = screen.getByText('Coliseum').closest('a')
    expect(door).toBeTruthy()
    expect(door!.getAttribute('href')).toBe('/coliseum')
  })

  it('never asks the Forge gate, because it has nothing to gate', async () => {
    mount()
    await waitFor(() =>
      expect(screen.getByText('Mana & consistency')).toBeTruthy())
    // `forgeStatus` is not even on this screen's mocked api surface, so a
    // call would throw rather than return undefined — which is the assertion.
    expect((api as Record<string, unknown>).forgeStatus).toBeUndefined()
  })
})

/**
 * The two halves of punch list items 5 and 6, and both are about the screen
 * answering somebody *before* it has anything to report.
 */
describe('before a run', () => {
  /** A shelf whose second deck the gate has already complained about. */
  const FLAGGED = [
    DECKS[0]!,
    { ...DECKS[1]!, errors: 2, warnings: 0 },
  ] as unknown as DeckTile[]

  it('names the cards a run will leave out, without running anything', async () => {
    vi.mocked(api.decks).mockResolvedValue(FLAGGED)
    vi.mocked(api.validate).mockResolvedValue({
      ok: false,
      errors: [
        { code: 'unknown-card', message: 'not found in the local pool', card: 'Sol Rng' },
        { code: 'unknown-card', message: 'not found in the local pool', card: 'Path to Exil' },
      ],
      warnings: [],
    })
    mount()
    // Deck one is clean, so nothing is asked and nothing is said.
    await waitFor(() => expect(screen.getByText('Mana & consistency')).toBeTruthy())
    expect(screen.queryByText(/not in the card pool/)).toBeNull()

    fireEvent.change(screen.getByLabelText(/Deck/i),
                     { target: { value: 'local/gyome' } })
    await waitFor(() => expect(screen.getByText(/not in the card pool/)).toBeTruthy())
    // The names, because "2 errors" is not a thing anybody can act on.
    expect(screen.getByText(/Sol Rng, Path to Exil/)).toBeTruthy()
    // And no number for what is left: that belongs to the compiler, and
    // arrives with the results.
    expect(screen.queryByText(/97 of 99/)).toBeNull()
    expect(api.simMana).not.toHaveBeenCalled()
  })

  it('asks the gate nothing about a deck the shelf says is clean', async () => {
    mount()
    await waitFor(() => expect(screen.getByText('Mana & consistency')).toBeTruthy())
    expect(api.validate).not.toHaveBeenCalled()
  })

  it('answers the click rather than the job', async () => {
    // A submission that never comes back, which is the whole point: the gap
    // this covers is the one between the press and the job it produces.
    vi.mocked(api.simMana).mockReturnValue(new Promise(() => {}))
    mount()
    await waitFor(() => expect(screen.getByText('Mana & consistency')).toBeTruthy())

    const button = screen.getByText('Run simulation') as HTMLButtonElement
    expect(button.disabled).toBe(false)
    fireEvent.click(button)

    await waitFor(() => expect(screen.getByText('Running…')).toBeTruthy())
    expect((screen.getByText('Running…') as HTMLButtonElement).disabled).toBe(true)
    // And the other door out of this screen is shut too, so a second press
    // cannot start a second run behind the first.
    expect((screen.getByText('New sample').closest('button'))!.disabled).toBe(true)
  })
})
