/**
 * The settings room's two master switches and its per-deck chips.
 *
 * What is worth pinning here is not that a button calls a function — it is the
 * three things this page could get wrong in ways nothing else would catch:
 *
 * 1. **Scope.** The shelf carries everybody's shared decks and the showcase.
 *    A page that offered a switch on somebody else's deck would be offering a
 *    refusal, and the server's own sweep resolves scope from the session, so
 *    the two would disagree silently.
 * 2. **The third state.** "Some of them" is the ordinary shape of a library,
 *    and `aria-pressed="mixed"` is the only part of the control that says so
 *    to a reader who cannot see the half-filled chip.
 * 3. **The honest refusal.** A showcase deck cannot be entered for the night
 *    games. The row must say so instead of drawing a switch that fails.
 */

import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeckTile } from '../lib/api'
import Settings from './Settings'

vi.mock('../lib/api', async () => ({
  // Real, both of them. `deckUrl` is the only thing that says where a deck
  // lives, and a stub would let every row's link point at nothing while the
  // assertions below still passed; `errorMessage` is the pure function that
  // turns a refusal into the sentence a person reads, and stubbing it would
  // hide a regression in exactly the path test 3 exists to check.
  deckUrl: (await vi.importActual<typeof import('../lib/api')>('../lib/api')).deckUrl,
  errorMessage: (await vi.importActual<typeof import('../lib/api')>('../lib/api')).errorMessage,
  api: {
    decks: vi.fn(),
    setShared: vi.fn(),
    setColiseumAtNight: vi.fn(),
    setEveryDeckShared: vi.fn(),
    setEveryDeckColiseumAtNight: vi.fn(),
  },
}))

const { api } = await import('../lib/api')

function deck(overrides: Partial<DeckTile> & { slug: string }): DeckTile {
  return {
    name: overrides.slug,
    owner: 'aaron',
    pilot: '',
    themes: [],
    archetype: null,
    shared: false,
    coliseum_at_night: false,
    showcase: false,
    status: 'built',
    stage: 'curated',
    // Yours by default: this page is about decks you can change, so the
    // fixture that describes "a deck" describes one of your own.
    writable: true,
    needs_rationale: 0,
    commander: ['Syr Gwyn, Hero of Ashvale'],
    companion: null,
    bracket: null,
    total_cards: 100,
    land_count: 36,
    strategy: '',
    art_crop: null,
    color_identity: [],
    errors: 0,
    warnings: 0,
    ...overrides,
  }
}

/** A mixed shelf: one deck shared, one not. The ordinary state, and the one
 *  the master switch has to describe as "some". */
const MIXED: DeckTile[] = [
  deck({ slug: 'gyome', name: 'Gyome, Master Chef', shared: true }),
  deck({ slug: 'arahbo', name: 'Arahbo, Roar of the World', shared: false }),
]

function renderSettings() {
  return render(<MemoryRouter><Settings /></MemoryRouter>)
}

/** The master switch for a column, found by the accessible name its component
 *  gives it rather than by position. */
function master(columnName: string): HTMLElement {
  return screen.getByRole('button', { name: `${columnName}: every deck` })
}

beforeEach(() => {
  // `mockReset`, not a fresh return value: a `vi.fn()` from a mock factory is
  // module-level state that outlives the test, so call counts accumulate and
  // "called once" quietly means "once this test, plus last test's".
  vi.mocked(api.decks).mockReset().mockResolvedValue(MIXED)
  vi.mocked(api.setShared).mockReset().mockResolvedValue({} as never)
  vi.mocked(api.setColiseumAtNight).mockReset().mockResolvedValue({} as never)
  vi.mocked(api.setEveryDeckShared).mockReset()
    .mockResolvedValue({ shared: true, changed: 2 })
  vi.mocked(api.setEveryDeckColiseumAtNight).mockReset()
    .mockResolvedValue({ coliseum_at_night: true, changed: 2 })
})

// Explicit, because Testing Library only registers auto-cleanup when the test
// framework exposes its hooks globally, and `vitest.config.ts` deliberately
// does not.
afterEach(cleanup)

describe('the shelf it shows', () => {
  it('shows the decks you own and leaves everybody else’s alone', async () => {
    vi.mocked(api.decks).mockResolvedValue([
      ...MIXED,
      deck({ slug: 'someone-elses', name: 'Not Yours', writable: false }),
      deck({ slug: 'the-showcase', name: 'On Display', writable: false, showcase: true }),
    ])
    renderSettings()
    expect(await screen.findByText('Gyome, Master Chef')).toBeTruthy()
    expect(screen.getByText('Arahbo, Roar of the World')).toBeTruthy()
    // A deck you cannot write has no switch to offer, so it is not here at
    // all — offering one would be offering a refusal.
    expect(screen.queryByText('Not Yours')).toBeNull()
    expect(screen.queryByText('On Display')).toBeNull()
  })

  it('says so plainly when you have no decks yet', async () => {
    vi.mocked(api.decks).mockResolvedValue([])
    renderSettings()
    expect(await screen.findByText(/no decks of your own yet/i)).toBeTruthy()
    // And offers the way out rather than leaving somebody on a dead page.
    expect(screen.getByRole('link', { name: 'Start a deck' })).toBeTruthy()
  })
})

describe('one deck at a time', () => {
  it('flips a deck’s visibility and re-reads the shelf', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')

    const chip = screen.getByRole('button', { name: 'Share: Arahbo, Roar of the World' })
    expect(chip.getAttribute('aria-pressed')).toBe('false')
    expect(chip.className).toContain('chip-toggle')

    fireEvent.click(chip)
    await waitFor(() => expect(api.setShared).toHaveBeenCalledWith(
      { owner: 'aaron', slug: 'arahbo' }, true))
    // The page re-reads rather than guessing: the write answers with the deck
    // and the shelf is what this page renders.
    await waitFor(() => expect(api.decks).toHaveBeenCalledTimes(2))
  })

  it('enters one deck for the night games', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')

    const chip = screen.getByRole('button',
      { name: 'Enter for the night games: Gyome, Master Chef' })
    expect(chip.getAttribute('aria-pressed')).toBe('false')
    fireEvent.click(chip)
    await waitFor(() => expect(api.setColiseumAtNight).toHaveBeenCalledWith(
      { owner: 'aaron', slug: 'gyome' }, true))
  })

  it('withdraws a deck that is already in', async () => {
    vi.mocked(api.decks).mockResolvedValue(
      [deck({ slug: 'gyome', name: 'Gyome, Master Chef', coliseum_at_night: true })])
    renderSettings()
    const chip = await screen.findByRole('button',
      { name: 'Enter for the night games: Gyome, Master Chef' })
    expect(chip.getAttribute('aria-pressed')).toBe('true')
    fireEvent.click(chip)
    await waitFor(() => expect(api.setColiseumAtNight).toHaveBeenCalledWith(
      { owner: 'aaron', slug: 'gyome' }, false))
  })

  it('offers a showcase deck a sentence rather than a switch it cannot use', async () => {
    vi.mocked(api.decks).mockResolvedValue(
      [deck({ slug: 'gyome', name: 'Gyome, Master Chef', showcase: true })])
    renderSettings()
    expect(await screen.findByText(/not yet open to showcase decks/i)).toBeTruthy()
    expect(screen.queryByRole('button',
      { name: 'Enter for the night games: Gyome, Master Chef' })).toBeNull()
    // Sharing still works on it — the two flags are independent, and only one
    // of them is out of reach here.
    expect(screen.getByRole('button',
      { name: 'Share: Gyome, Master Chef' })).toBeTruthy()
  })
})

describe('the master switch', () => {
  it('reads a half-shared shelf as mixed rather than as on or off', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    const all = master('Share')
    // The whole reason this control is three-state. `"mixed"` is a real
    // `aria-pressed` value and is the only half of this the ear ever gets.
    expect(all.getAttribute('aria-pressed')).toBe('mixed')
    expect(all.textContent).toContain('Some of them')
    expect(all.className).toContain('is-part')
  })

  it('takes a mixed shelf to all on in one press', async () => {
    // The two reads this test drives, queued **before** anything renders: the
    // mixed shelf the page opens on, then the uniform one the sweep leaves
    // behind. Swapping the mock after the click would race the re-read the
    // click itself starts.
    vi.mocked(api.decks)
      .mockResolvedValueOnce(MIXED)
      .mockResolvedValueOnce(MIXED.map((d) => ({ ...d, shared: true })))
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    expect(master('Share').getAttribute('aria-pressed')).toBe('mixed')

    fireEvent.click(master('Share'))
    await waitFor(() => expect(api.setEveryDeckShared).toHaveBeenCalledWith(true))

    // And the shelf it re-reads is what it draws: every chip on, and the
    // master saying so rather than still saying "some".
    await waitFor(() => expect(
      master('Share').getAttribute('aria-pressed')).toBe('true'))
    for (const name of ['Gyome, Master Chef', 'Arahbo, Roar of the World']) {
      expect(screen.getByRole('button', { name: `Share: ${name}` })
        .getAttribute('aria-pressed')).toBe('true')
    }
  })

  it('takes an all-on shelf back off', async () => {
    vi.mocked(api.decks).mockResolvedValue(MIXED.map((d) => ({ ...d, shared: true })))
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    const all = master('Share')
    expect(all.getAttribute('aria-pressed')).toBe('true')
    expect(all.textContent).toContain('All of them')
    fireEvent.click(all)
    await waitFor(() => expect(api.setEveryDeckShared).toHaveBeenCalledWith(false))
  })

  it('enters every deck for the night games from none', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    const all = master('Enter for the night games')
    expect(all.getAttribute('aria-pressed')).toBe('false')
    fireEvent.click(all)
    await waitFor(() => expect(api.setEveryDeckColiseumAtNight)
      .toHaveBeenCalledWith(true))
  })

  it('counts only the decks that can actually be entered', async () => {
    // One showcase deck, one ordinary one already in. The night master governs
    // the second alone, so it reads "all", not "some" — a switch that counted
    // the deck it cannot reach would report a refusal as a choice.
    vi.mocked(api.decks).mockResolvedValue([
      deck({ slug: 'showcase-one', name: 'On Display', showcase: true }),
      deck({ slug: 'mine', name: 'Mine', coliseum_at_night: true }),
    ])
    renderSettings()
    await screen.findByText('Mine')
    expect(master('Enter for the night games')
      .getAttribute('aria-pressed')).toBe('true')
    // Sharing still counts both, because both can be shared.
    expect(master('Share').getAttribute('aria-pressed')).toBe('false')
  })

  it('stands the night switch down when nothing can be entered', async () => {
    vi.mocked(api.decks).mockResolvedValue(
      [deck({ slug: 'showcase-one', name: 'On Display', showcase: true })])
    renderSettings()
    await screen.findByText('On Display')
    expect(master('Enter for the night games')
      .hasAttribute('disabled')).toBe(true)
    expect(screen.getByText(/none of your decks can be entered yet/i)).toBeTruthy()
  })
})

describe('when a write is refused', () => {
  it('says what the server said instead of looking as though it worked', async () => {
    // Built per call and marked handled on the spot. A rejected promise made
    // at mock-setup time is unhandled until the page happens to await it,
    // which Node reports as a failure of this file rather than of the code —
    // `.catch` returns a new promise, so the original still rejects for the
    // page, which is the real handler.
    vi.mocked(api.setColiseumAtNight).mockImplementation(() => {
      const promise = Promise.reject(new Error(
        'the showcase decks keep to daylight for now'))
      promise.catch(() => { /* the page is the real handler */ })
      return promise as never
    })
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    fireEvent.click(screen.getByRole('button',
      { name: 'Enter for the night games: Gyome, Master Chef' }))
    expect(await screen.findByText(/keep to daylight/)).toBeTruthy()
  })

  it('re-reads the shelf after a refused sweep rather than trusting its own guess',
    async () => {
      vi.mocked(api.setEveryDeckShared).mockImplementation(() => {
        const promise = Promise.reject(new Error('not yours to change'))
        promise.catch(() => { /* the page is the real handler */ })
        return promise as never
      })
      renderSettings()
      await screen.findByText('Gyome, Master Chef')
      fireEvent.click(master('Share'))
      expect(await screen.findByText(/not yours to change/)).toBeTruthy()
      // A sweep can refuse partway, so the shelf on screen has to come from
      // the server rather than from what this page hoped had happened.
      await waitFor(() => expect(api.decks).toHaveBeenCalledTimes(2))
    })
})

describe('the room’s own bones', () => {
  it('credits the painting it shows, in the room the picture is in', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    // Commandment 19: the artist and the printing, in words, beside the art.
    expect(screen.getByText(/Carl Critchlow/)).toBeTruthy()
    expect(screen.getByText(/Grand Coliseum/)).toBeTruthy()
  })

  it('never claims the night games are running', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    // The panel promises a thing that has not started. The copy has to be in
    // the future tense, and "the torches are not lit" is the sentence that
    // carries it.
    expect(screen.getByText(/torches are not lit yet/i)).toBeTruthy()
  })

  it('links each deck to itself and toggles beside it with buttons', async () => {
    renderSettings()
    await screen.findByText('Gyome, Master Chef')
    const row = screen.getByText('Gyome, Master Chef').closest('li')!
    // Commandment 20 both ways round in one row: the name goes somewhere, so
    // it is a link; the chips change this page, so they are buttons.
    expect(within(row).getByRole('link', { name: 'Gyome, Master Chef' })
      .getAttribute('href')).toBe('/decks/aaron/gyome')
    for (const chip of within(row).getAllByRole('button')) {
      expect(chip.getAttribute('aria-pressed')).toBeTruthy()
    }
  })
})
