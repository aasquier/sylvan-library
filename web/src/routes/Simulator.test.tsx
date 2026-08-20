/**
 * The Simulator's Forge mode (ADR 35), and the properties a green Python
 * suite cannot see from its side of the wire:
 *
 * - the mode is **honestly absent** when the gate says Forge is not
 *   installed — no greyed-out option, no excuse, nothing (the Ask Claude
 *   rule, applied to Tier 3);
 * - a failed gate ask reads as absence, never as an error page, because a
 *   deployed instance without Forge is the normal case, not a fault;
 * - the run asks for **both** decks by address, so the opponent is a real
 *   choice rather than decoration;
 * - a clocked-out game renders apart from draws and wins, because the
 *   server keeps them apart and a client that folds them undoes the
 *   distinction CLAUDE.md insists on.
 *
 * Assertions match this file's own strings, not the payload's, for the
 * reason Research.test.tsx records: a test that asserts the server's text
 * back at itself is not testing the renderer.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeckTile, ForgeResult, Job } from '../lib/api'
import Simulator from './Simulator'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: { decks: vi.fn(), forgeStatus: vi.fn(), simMana: vi.fn(),
           simLands: vi.fn(), simForge: vi.fn(), job: vi.fn(),
           glossary: vi.fn() },
  }
})

const { api } = await import('../lib/api')

const DECKS = [
  { slug: 'goreclaw', owner: 'local', name: 'Goreclaw Stompy', pilot: '',
    writable: true },
  { slug: 'gyome', owner: 'local', name: 'Gyome Food', pilot: '',
    writable: true },
] as unknown as DeckTile[]

const RESULT: ForgeResult = {
  decks: [
    { slug: 'goreclaw', name: 'Goreclaw Stompy', address: 'local/goreclaw',
      wins: 6 },
    { slug: 'gyome', name: 'Gyome Food', address: 'local/gyome', wins: 2 },
  ],
  games: 10, played: 10, draws: 1, timed_out: 1,
  median_seconds: 5.4, max_seconds: 37.8,
  startup_seconds: 9.2, wall_seconds: 71.0,
  clock: 300, seed: 7,
  rows: [
    { game: 1, winner: 'goreclaw', seconds: 5.4, turns: 9, draw: false,
      timed_out: false },
    { game: 2, winner: null, seconds: 300.0, turns: null, draw: false,
      timed_out: true },
    { game: 3, winner: null, seconds: 8.0, turns: 12, draw: true,
      timed_out: false },
  ],
  caveat: 'server caveat text',
}

const DONE: Job = {
  id: 'j1', kind: 'sim.forge', status: 'done', done: 10, total: 10,
  percent: 100, label: '', result: RESULT, error: null,
  created_at: '2026-08-20T00:00:00Z',
} as unknown as Job

function mount() {
  return render(<MemoryRouter><Simulator /></MemoryRouter>)
}

beforeEach(() => {
  vi.mocked(api.decks).mockResolvedValue(DECKS)
  vi.mocked(api.forgeStatus).mockResolvedValue({ available: true, why: null })
  vi.mocked(api.glossary).mockResolvedValue(
    { terms: [] } as unknown as Awaited<ReturnType<typeof api.glossary>>)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the gate', () => {
  it('offers real games only where Forge is installed', async () => {
    mount()
    await waitFor(() =>
      expect(screen.getByText('Real games (Forge)')).toBeTruthy())
  })

  it('is honestly absent when the gate says no', async () => {
    vi.mocked(api.forgeStatus).mockResolvedValue(
      { available: false, why: 'no jar' })
    mount()
    await waitFor(() => expect(api.forgeStatus).toHaveBeenCalled())
    expect(screen.queryByText('Real games (Forge)')).toBeNull()
    // The maintainer-facing reason must never render (commandment 10).
    expect(screen.queryByText(/no jar/)).toBeNull()
  })

  it('treats a failed ask as absence, not as an error', async () => {
    vi.mocked(api.forgeStatus).mockRejectedValue(new Error('boom'))
    mount()
    await waitFor(() => expect(api.forgeStatus).toHaveBeenCalled())
    expect(screen.queryByText('Real games (Forge)')).toBeNull()
    expect(screen.queryByText(/Simulation failed/)).toBeNull()
  })
})

describe('a match', () => {
  async function intoForgeMode() {
    mount()
    await waitFor(() =>
      expect(screen.getByText('Real games (Forge)')).toBeTruthy())
    fireEvent.change(screen.getByLabelText(/Simulation/i),
                     { target: { value: 'forge' } })
  }

  it('asks with both decks and the games count', async () => {
    vi.mocked(api.simForge).mockResolvedValue(DONE)
    await intoForgeMode()
    // The opponent seat defaults to the second deck along.
    expect(screen.getByLabelText(/Opponent/i)).toBeTruthy()
    fireEvent.click(screen.getByText('Run simulation'))
    await waitFor(() => expect(api.simForge).toHaveBeenCalledWith(
      expect.objectContaining({
        a_slug: 'goreclaw', a_owner: 'local',
        b_slug: 'gyome', b_owner: 'local', games: 10,
      })))
  })

  it('renders wins, and keeps clock-outs apart from draws', async () => {
    vi.mocked(api.simForge).mockResolvedValue(DONE)
    await intoForgeMode()
    fireEvent.click(screen.getByText('Run simulation'))
    await waitFor(() =>
      expect(screen.getByText('Goreclaw Stompy wins')).toBeTruthy())
    expect(screen.getByText('Gyome Food wins')).toBeTruthy()
    // The tile pair the distinction lives in.
    expect(screen.getByText('Hit the clock')).toBeTruthy()
    expect(screen.getByText('Draws')).toBeTruthy()
    // And per game: the clocked row is neither a draw nor a win.
    expect(screen.getByText('hit the clock')).toBeTruthy()
    expect(screen.getByText('draw')).toBeTruthy()
    // The wall-clock line speaks Magic, not machinery.
    expect(screen.getByText(/lighting\s+the forge/)).toBeTruthy()
  })
})
