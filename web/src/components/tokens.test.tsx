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
import { MOST_OF_ONE_TOKEN } from '../lib/tokenshop'

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
  Reflect.deleteProperty(navigator, 'clipboard')
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

/** The section's own toggle. `/^Tokens/` rather than the whole name, because
 *  the count rides on the end of it once the shelf has been read — and never
 *  `getByTitle`, which is what this header used to be found by and is exactly
 *  the attribute the header no longer has. */
function toggle() {
  return screen.getByRole('button', { name: /^Tokens/ })
}

function unfold() {
  render(<TokenShelf deckRef={ref} />)
  fireEvent.click(toggle())
}

/**
 * A clipboard, or one that will not have it.
 *
 * jsdom ships neither, so both halves have to be installed by hand — and the
 * refusing half is the one worth the trouble. A `catch` that quietly does
 * nothing is the mistake this repo makes most often, and no green suite ever
 * sees it, because nothing ever drives the failure path. This drives it.
 */
function clipboard(answer: 'takes' | 'refuses') {
  const writeText = vi.fn(answer === 'takes'
    ? () => Promise.resolve()
    : () => Promise.reject(new Error('the document is not focused')))
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText }, configurable: true, writable: true,
  })
  return writeText
}

/** The one press this section has. Named for what it says at rest, because
 *  what it says after the press is the thing half these tests are checking. */
function shopButton() {
  return screen.findByRole('button', { name: /shopping list/i })
}

/** A tap, the way a phone makes one. jsdom has no PointerEvent of its own, so
 *  the type rides on a plain Event — which is all `CardHover` reads. */
function tap(el: Element) {
  for (const type of ['pointerdown', 'pointerup']) {
    const ev = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(ev, 'pointerType', { value: 'touch' })
    fireEvent(el, ev)
  }
}

// Folded is the default and folded costs nothing: no request, so no pictures
// and no work for a reader who never opens it.
it('asks for nothing until it is opened', async () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  render(<TokenShelf deckRef={ref} />)
  expect(read).not.toHaveBeenCalled()
  expect(screen.queryByText(/Made by/)).toBeNull()

  fireEvent.click(toggle())
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

// Aaron, 2026-08-28: "it would be nice if a hover in our token menu for a deck
// gave a card preview". A token face is drawn at 5.75rem and everything the
// token actually *does* is printed on it, at a size no phone can read.
it('holds the whole token face up to a hover', async () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  unfold()
  const face = await screen.findByAltText('Food')

  fireEvent.mouseEnter(face, { clientX: 20, clientY: 20 })
  // Two now: the plate's own face, and the card held beside the cursor.
  expect(screen.getAllByAltText('Food')).toHaveLength(2)

  fireEvent.mouseLeave(face)
  expect(screen.getAllByAltText('Food')).toHaveLength(1)
})

// **And the half of the room with no cursor at all**, which is the half that
// matters here: `tapOpens` is left at its default because a plate is inert,
// so nothing is being stolen from a tap that already meant something.
it('hands the token to a tap, for the half of the room with no cursor', async () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  unfold()
  const face = await screen.findByAltText('Food')
  expect(screen.queryByRole('dialog')).toBeNull()

  tap(face)
  expect(screen.getByRole('dialog', { name: 'Food' })).toBeTruthy()

  fireEvent.keyDown(window, { key: 'Escape' })
  expect(screen.queryByRole('dialog')).toBeNull()
})

// No painting opens no sheet — `CardSheet`'s own rule, one surface over. The
// blank plate is the honest likeness of a token nobody printed; enlarging it
// would be the same dashed rectangle at four times the size.
it('opens nothing for a token it has no picture of', async () => {
  read.mockResolvedValue(sheet({
    tokens: [plate('Elephant', {
      image: null, art_crop: null, artist: null,
      set_code: null, set_name: null, made_by: ['Terastodon'],
    })],
  }))
  unfold()
  await waitFor(() => {
    expect(document.querySelector('.token-face-blank')).not.toBeNull()
  })
  const blank = document.querySelector('.token-face-blank')
  if (!blank) throw new Error('no blank plate')

  tap(blank)
  fireEvent.mouseEnter(blank, { clientX: 20, clientY: 20 })
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.queryByRole('img')).toBeNull()
})

// **The header's sentence used to be a `title`**, which draws on hover and on
// nothing else: never on a phone, never on keyboard focus. It is a real
// control now, and this is the pin — a `title` coming back would pass every
// other test in this file.
it('explains what a token is to a hand that is not a mouse', () => {
  read.mockResolvedValue(sheet({ tokens: [plate('Food')] }))
  render(<TokenShelf deckRef={ref} />)

  expect(toggle().getAttribute('title')).toBeNull()
  const ask = screen.getByRole('button', { name: 'What a token is' })
  expect(ask.getAttribute('title')).toBeNull()
  expect(screen.queryByRole('tooltip')).toBeNull()

  // A tap pins it up; the same press a thumb makes, and the same press Enter
  // makes on a real button.
  fireEvent.click(ask)
  expect(screen.getByRole('tooltip').textContent)
    .toContain('this deck makes while you play')

  // And it does not fold the section on its way past.
  expect(read).not.toHaveBeenCalled()
})

// ---------------------------------------------------------- the shopping list
//
// Aaron, 2026-08-29: "an 'Export' button in the token area that can give you a
// TCGPlayer friendly list to help shop for tokens when needed."
//
// The list's own rules — the shop's name for a token, the merge across
// printings, the cap — are `lib/tokenshop.test.ts`, because they are
// arithmetic on a shape rather than anything a reader touches. What is here
// is the touching: the press, its two answers, and the two states that must
// offer nothing at all.

// The press does the thing, and then shows its work: the same text on the
// clipboard and on the page, and a sentence saying how much went across.
it('copies the list and says what went across', async () => {
  const wrote = clipboard('takes')
  read.mockResolvedValue(sheet({
    tokens: [
      plate('Food', { made_by: ['Gyome, Master Chef', 'The Shire'] }),
      plate('Treasure', { made_by: ['Smothering Tithe'] }),
    ],
  }))
  unfold()
  fireEvent.click(await shopButton())

  await waitFor(() => {
    expect(wrote).toHaveBeenCalledWith('2 Food Token\n1 Treasure Token\n')
  })
  await screen.findByText(/2 tokens to look for, 3 cards in all/)
  // Shown as well as copied, so nobody pastes a surprise — and byte for byte
  // the same text, not a prettier rendering of it.
  expect(document.querySelector('.token-shop-list')?.textContent)
    .toBe('2 Food Token\n1 Treasure Token\n')
})

// **A clipboard that says no must not read as a press that worked.** Nothing
// is broken here and nothing is lost, so this is not a red box — but the
// button may not sit there saying "Copied" over a clipboard that is empty,
// and the list has to be on screen to be taken by hand.
it('says so when the clipboard refuses, and leaves the list to be taken', async () => {
  clipboard('refuses')
  read.mockResolvedValue(sheet({
    tokens: [plate('Food', { made_by: ['Gyome, Master Chef'] })],
  }))
  unfold()
  fireEvent.click(await shopButton())

  await screen.findByText(/clipboard would not take it/i)
  expect(document.querySelector('.token-shop-list')?.textContent)
    .toBe('1 Food Token\n')
  expect(screen.queryByRole('button', { name: 'Copied' })).toBeNull()
  expect(document.body.textContent).not.toMatch(/Copied —/)
})

// Commandment 2, at the moment it bites hardest: a newcomer has no idea how
// many of a token they need, and this is exactly where to tell them.
//
// **The cap is written twice — `4` in the rule and "four" in the sentence —
// and nothing but this holds the two together.** A number recorded in prose
// is a claim that rots, and the way it rots here is silent: the list would
// quietly stop matching the paragraph explaining it. Change one and this
// fails, which is the whole job.
it('explains the number it chose, in words that match the rule', async () => {
  expect(MOST_OF_ONE_TOKEN).toBe(4)
  clipboard('takes')
  read.mockResolvedValue(sheet({
    tokens: [plate('Food', { made_by: ['Gyome, Master Chef'] })],
  }))
  unfold()
  fireEvent.click(await shopButton())

  await screen.findByText(/starting pile rather than a count/i)
  expect(document.body.textContent)
    .toContain('one for every card in your deck that makes it, up to four')
})

// **Absent, not disabled.** A deck that makes nothing has already been told so
// in its own sentence; a greyed-out Copy button beside it would be a second,
// worse way of saying the same thing.
it('offers no shopping list to a deck that makes nothing', async () => {
  read.mockResolvedValue(sheet())
  unfold()
  await screen.findByText(/Nothing in this deck makes a token/i)
  expect(screen.queryByRole('button', { name: /shopping list/i })).toBeNull()
})

// And the deploy window, where the honest answer is that nobody has looked
// yet — an empty list offered for sale there would be a claim about the deck.
it('offers no shopping list before the pool has been read', async () => {
  read.mockResolvedValue(sheet({ read: false }))
  unfold()
  await screen.findByText(/cannot say yet/i)
  expect(screen.queryByRole('button', { name: /shopping list/i })).toBeNull()
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
