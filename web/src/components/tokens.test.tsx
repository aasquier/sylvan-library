/**
 * The token shelf.
 *
 * jsdom has no layout engine, so nothing here can prove a plate is visible,
 * sized, dealt or unclipped — the appearance is Aaron's walk, not this file's.
 * What it can hold is the wiring and the copy: that a folded section asks for
 * nothing, that the three empty answers stay three different sentences, and
 * that no machinery reaches a reader.
 */
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { DeckTokens, TokenPlate } from '../lib/api'

vi.mock('../lib/api', async (importOriginal) => {
  const real = await importOriginal<typeof import('../lib/api')>()
  return { ...real, api: { ...real.api, deckTokens: vi.fn() } }
})

const { api } = await import('../lib/api')
const { TokenShelf } = await import('./tokens')
const read = vi.mocked(api.deckTokens)

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const ref = { owner: 'local', slug: 'a-deck' }

function plate(name: string, over: Partial<TokenPlate> = {}): TokenPlate {
  return {
    name,
    type_line: `Token Creature — ${name}`,
    image: `https://cards.scryfall.io/normal/front/a/b/${name}.jpg`,
    art_crop: `https://cards.scryfall.io/art_crop/front/a/b/${name}.jpg`,
    artist: 'Randy Gallegos',
    set_code: 'TELD',
    set_name: 'Throne of Eldraine Tokens',
    made_by: ['Gyome, Master Chef'],
    ...over,
  }
}

function sheet(over: Partial<DeckTokens> = {}): DeckTokens {
  return { pool_available: true, read: true, tokens: [], ...over }
}

function unfold() {
  render(<TokenShelf deckRef={ref} />)
  fireEvent.click(screen.getByTitle('What this deck makes'))
}

// Folded is the default and folded costs nothing: no request, so no pictures
// and no work for a reader who never opens it.
it('asks for nothing until it is opened', async () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  render(<TokenShelf deckRef={ref} />)
  expect(read).not.toHaveBeenCalled()
  expect(screen.queryByText(/Made by/)).toBeNull()

  fireEvent.click(screen.getByTitle('What this deck makes'))
  await waitFor(() => { expect(read).toHaveBeenCalledTimes(1) })
})

// The plate: what it is, which of the deck's cards make it, and who painted
// it (rule 9).
it('names the token, its makers and its painter', async () => {
  read.mockResolvedValue(sheet({
    tokens: [plate('Food', {
      type_line: 'Token Artifact — Food',
      made_by: ['Bag End Banquet', 'Gyome, Master Chef'],
    })],
  }))
  unfold()
  // The name is on the plate and again in the credit line, so it is never
  // a unique query here; the type line is.
  await screen.findByText('Token Artifact — Food')
  expect(screen.getByText(/Bag End Banquet, Gyome, Master Chef/)).toBeTruthy()
  expect(screen.getByText(/Randy Gallegos/)).toBeTruthy()
  expect(screen.getByText(/Throne of Eldraine Tokens/)).toBeTruthy()
})

// Commandment 2: somebody who has never played does not know what a token is,
// and this section is where they meet the word.
it('says what a token is without being asked', async () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  unfold()
  await screen.findAllByText('Food')
  expect(document.body.textContent).toContain(
    'made during the game rather than drawn from the deck')
})

// No printing here is a plate with the name written on it, never a dropped
// token and never a painting credited to nobody.
it('still names a token it has no picture of', async () => {
  read.mockResolvedValue(sheet({
    tokens: [plate('Elephant', {
      image: null, art_crop: null, artist: null,
      set_code: null, set_name: null, made_by: ['Terastodon'],
    })],
  }))
  unfold()
  await waitFor(() => { expect(screen.getAllByText('Elephant').length).toBeGreaterThan(0) })
  expect(screen.getByText(/Terastodon/)).toBeTruthy()
  expect(screen.queryByRole('img')).toBeNull()
  expect(document.body.textContent).not.toMatch(/\bby\s*,/)
})

// **The deploy window, and the one wrong answer this section must never
// give.** A pool that predates the reading has not looked; saying "makes
// nothing" there tells somebody something false about their own deck.
it('does not claim a deck makes nothing when the pool has not been read', async () => {
  read.mockResolvedValue(sheet({ read: false }))
  unfold()
  await screen.findByText(/cannot say yet/i)
  expect(document.body.textContent).not.toMatch(/makes a token/i)
})

// And the answer that genuinely is "nothing", which is a different sentence.
it('says so when the deck really makes nothing', async () => {
  read.mockResolvedValue(sheet())
  unfold()
  await screen.findByText(/Nothing in this deck makes a token/i)
  expect(document.body.textContent).not.toMatch(/cannot say yet/i)
})

// The degraded answer, in the page's own words rather than the server's —
// which name a command line at somebody who came here for cards
// (commandment 10).
it('never repeats the machinery in the server message', async () => {
  read.mockResolvedValue(sheet({
    pool_available: false, read: false,
    message: 'no card pool yet -- run `mtglab data refresh`',
  }))
  unfold()
  await screen.findByText(/the tokens cannot be looked up/i)
  expect(document.body.textContent).not.toMatch(/mtglab|refresh/i)
})

// A refusal is shown, not swallowed.
it('shows a refusal rather than an empty shelf', async () => {
  read.mockRejectedValue(new Error('the library could not answer that right now'))
  unfold()
  await screen.findByText(/could not answer/i)
})

// The fold is not remembered, and that is the ask: "collapsed by default".
it('opens folded again on a fresh mount', async () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  unfold()
  await screen.findAllByText('Food')
  cleanup()
  render(<TokenShelf deckRef={ref} />)
  expect(screen.queryAllByText('Food')).toHaveLength(0)
})
