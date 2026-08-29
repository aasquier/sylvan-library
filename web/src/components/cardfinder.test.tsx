/**
 * The card finder, pinned where it is actually behaviour.
 *
 * What is worth a test here is everything the old text box did not do: one
 * request for a whole typed name rather than fourteen, an answer to a
 * *stale* question dropped rather than painted, the keyboard reaching every
 * row, and the two refusals said while the card is being chosen instead of
 * after a rationale has been written.
 *
 * The tiers themselves are not tested here — they are the server's, and
 * `internal/cards`' suite measures them against a real card pool. A component
 * test that asserted "Sol Rng finds Sol Ring" would only be asserting its own
 * fixture back at itself.
 */
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { act, useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError, type CardOffer } from '../lib/api'
import { CardFinder } from './cardfinder'

// `ApiError` is real rather than stubbed: the finder now asks whether a
// failure was a 401, and `e instanceof undefined` throws a TypeError -- which
// would turn a handled failure into an unhandled one inside the handler.
vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: { suggestCards: vi.fn() },
}))

const { api } = await import('../lib/api')
const suggestCards = api.suggestCards as unknown as ReturnType<typeof vi.fn>

function offer(over: Partial<CardOffer> & { name: string }): CardOffer {
  return {
    mana_cost: '{1}', type_line: 'Artifact', oracle_text: '{T}: Add {C}{C}.',
    color_identity: [], image: `https://cards.example/${over.name}.jpg`,
    artist: 'Myles Wohl', legal_commander: true, is_land: false,
    score: 0.99, via: 'holds', ...over,
  }
}

const SOL = offer({ name: 'Sol Ring' })
const LOTUS = offer({
  name: 'Black Lotus', legal_commander: false, artist: 'Chris Rahn',
  score: 1, via: 'exact',
})
const GUESS = offer({ name: 'Cultivate', via: 'near', score: 0.98, artist: 'Steven Belledin' })

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  suggestCards.mockReset()
  suggestCards.mockResolvedValue({ cards: [SOL] })
})

afterEach(() => {
  vi.useRealTimers()
  cleanup()
})

/** Type into the box and let the debounce elapse. */
async function type(text: string) {
  fireEvent.change(screen.getByRole('combobox'), { target: { value: text } })
  await act(async () => { await vi.advanceTimersByTimeAsync(400) })
}

/** The finder as a page actually holds it: **controlled**, with the chosen
 *  card owned by the parent. A harness that dropped `onChange` on the floor
 *  would leave `value` permanently null and quietly stop testing everything
 *  the panel does after a card is chosen. */
function finder(props: { identity?: string[] } = {}) {
  const seen = vi.fn()
  function Harness() {
    const [card, setCard] = useState<CardOffer | null>(null)
    return (
      <CardFinder value={card} identity={props.identity ?? ['G']}
                  onChange={(c) => { seen(c); setCard(c) }} />
    )
  }
  render(<Harness />)
  return seen
}

it('asks once for a whole typed name, not once per keystroke', async () => {
  finder()
  const box = screen.getByRole('combobox')
  for (const text of ['s', 'so', 'sol', 'sol ', 'sol r', 'sol ri', 'sol rin', 'sol ring']) {
    fireEvent.change(box, { target: { value: text } })
    await act(async () => { await vi.advanceTimersByTimeAsync(40) })
  }
  // Eight keystroke, none of them settled.
  expect(suggestCards).not.toHaveBeenCalled()
  await act(async () => { await vi.advanceTimersByTimeAsync(400) })
  expect(suggestCards).toHaveBeenCalledTimes(1)
  expect(suggestCards.mock.calls[0]?.[0]).toBe('sol ring')
})

it('says nothing at one letter, because every card is near one letter', async () => {
  finder()
  await type('s')
  expect(suggestCards).not.toHaveBeenCalled()
  expect(screen.queryByRole('listbox')).toBeNull()
})

// The failure this guards is invisible on a fast connection and constant on a
// slow one: an answer to four letters ago landing on top of the current list.
it('drops an answer to a question that is no longer being asked', async () => {
  let settleFirst: ((v: { cards: CardOffer[] }) => void) | null = null
  suggestCards.mockImplementationOnce(() => new Promise((res) => { settleFirst = res }))
  suggestCards.mockResolvedValueOnce({ cards: [LOTUS] })
  finder()

  await type('sol')
  await type('black lotus')
  await waitFor(() => expect(screen.getByRole('option', { name: /Black Lotus/ })).toBeTruthy())

  // The first request answers now, late and wrong.
  await act(async () => { settleFirst?.({ cards: [SOL] }) })
  expect(screen.queryByRole('option', { name: /Sol Ring/ })).toBeNull()
  expect(screen.getByRole('option', { name: /Black Lotus/ })).toBeTruthy()
})

it('reaches every row from the keyboard and chooses with Enter', async () => {
  suggestCards.mockResolvedValue({ cards: [SOL, LOTUS, GUESS] })
  const onChange = finder()
  await type('c')
  await type('card')
  const box = screen.getByRole('combobox')
  await waitFor(() => expect(screen.getAllByRole('option')).toHaveLength(3))

  // The active row is announced through aria-activedescendant, which is the
  // only way a screen reader learns the cursor moved: the rows are not
  // focusable and must not be.
  const activeName = () => {
    const id = box.getAttribute('aria-activedescendant')
    return id ? document.getElementById(id)?.textContent ?? '' : ''
  }
  expect(activeName()).toContain('Sol Ring')
  fireEvent.keyDown(box, { key: 'ArrowDown' })
  expect(activeName()).toContain('Black Lotus')
  fireEvent.keyDown(box, { key: 'ArrowDown' })
  expect(activeName()).toContain('Cultivate')
  // Wrapping, both ways.
  fireEvent.keyDown(box, { key: 'ArrowDown' })
  expect(activeName()).toContain('Sol Ring')
  fireEvent.keyDown(box, { key: 'ArrowUp' })
  expect(activeName()).toContain('Cultivate')

  fireEvent.keyDown(box, { key: 'Enter' })
  expect(onChange).toHaveBeenCalledWith(GUESS)
  expect((box as HTMLInputElement).value).toBe('Cultivate')
  expect(screen.queryByRole('listbox')).toBeNull()
})

// Two presses, two jobs. One Escape that cleared the box would throw away a
// name somebody had nearly finished typing.
it('closes on Escape and clears on the second one', async () => {
  finder()
  await type('sol ring')
  const box = screen.getByRole('combobox')
  await waitFor(() => expect(screen.getByRole('listbox')).toBeTruthy())

  fireEvent.keyDown(box, { key: 'Escape' })
  expect(screen.queryByRole('listbox')).toBeNull()
  expect((box as HTMLInputElement).value).toBe('sol ring')

  fireEvent.keyDown(box, { key: 'Escape' })
  expect((box as HTMLInputElement).value).toBe('')
})

// The picture, and the person who painted it. A card image without a credit
// is the violation (ADR 6's hot-link terms, ADR 32, commandment 9), and the
// image is Scryfall's own URL rather than anything we host or transform.
it('shows the card being chosen and credits its painter', async () => {
  finder()
  await type('sol ring')
  const picture = await screen.findByAltText('Sol Ring')
  expect(picture.getAttribute('src')).toBe('https://cards.example/Sol Ring.jpg')
  expect(screen.getByText(/art by Myles Wohl/)).toBeTruthy()
  // Nothing filters, crops or dims the painting.
  expect(picture.className).not.toMatch(/blur|grayscale|opacity|sepia|saturate/)
})

// The preview follows the cursor, which is the part that makes this teach
// rather than merely work.
it('changes the card as the cursor moves down the list', async () => {
  suggestCards.mockResolvedValue({ cards: [SOL, LOTUS] })
  finder()
  await type('card')
  await screen.findByAltText('Sol Ring')
  fireEvent.keyDown(screen.getByRole('combobox'), { key: 'ArrowDown' })
  expect(screen.getByAltText('Black Lotus')).toBeTruthy()
  expect(screen.getByText(/art by Chris Rahn/)).toBeTruthy()
})

// Said while the card is being chosen, not after a rationale has been typed.
it('marks a banned card, and marks one outside the commander’s colours', async () => {
  suggestCards.mockResolvedValue({ cards: [LOTUS] })
  finder()
  await type('black lotus')
  await waitFor(() => expect(screen.getByText(/banned in Commander/)).toBeTruthy())
  cleanup()

  suggestCards.mockResolvedValue({
    cards: [offer({ name: 'Counterspell', color_identity: ['U'] })],
  })
  finder({ identity: ['G'] })
  await type('counterspell')
  // Named in words rather than as a letter: this is the surface a beginner
  // adds their first card on.
  await waitFor(() => expect(screen.getByText(/is blue, and your commander is not/)).toBeTruthy())
})

// A card that is fine says so too. "Nothing happened" is the state the old
// box left people in.
it('says so when the card is fine', async () => {
  suggestCards.mockResolvedValue({ cards: [SOL] })
  finder()
  await type('sol ring')
  const box = screen.getByRole('combobox')
  await waitFor(() => expect(screen.getByRole('listbox')).toBeTruthy())
  fireEvent.keyDown(box, { key: 'Enter' })
  await waitFor(() => expect(screen.getByText(/inside your commander/)).toBeTruthy())
})

it('offers a way out when nothing matches, instead of an empty box', async () => {
  suggestCards.mockResolvedValue({ cards: [] })
  finder()
  await type('qwertyuiop')
  await waitFor(() =>
    expect(screen.getByText(/No card in the library is spelled anything like that/)).toBeTruthy())
})

// The guess is labelled once, above the first guessed row. Labelling every
// row would shout, and labelling none would present a guess as a find.
it('says which rows are guesses, once', async () => {
  suggestCards.mockResolvedValue({ cards: [SOL, GUESS, offer({ name: 'Cultivator Colossus', via: 'near' })] })
  finder()
  await type('cultivater')
  await waitFor(() => expect(screen.getAllByRole('option')).toHaveLength(3))
  expect(screen.getAllByText('did you mean')).toHaveLength(1)
})

// ---- a failure is not an absence -------------------------------------------

// **The bug Aaron hit on 2026-08-29**, reported as "it said even Sol Ring was
// not found". The box had one empty state and every failure fell into it: a
// 401, a 500, a server restarting through a deploy and a genuinely unknown
// card all produced "no card in the library is spelled anything like that".
//
// That sentence is a claim about the library, made by something that could not
// reach the library. These tests are the guard: a failure has to say it was a
// failure, and it must never say anything about the card that was typed.
describe('when the library cannot be reached', () => {
  it('does not claim the card does not exist', async () => {
    suggestCards.mockRejectedValue(new Error('connection reset'))
    render(<CardFinder value={null} onChange={() => {}} identity={[]} />)
    await type('Sol Ring')

    expect(screen.queryByText(/spelled anything like that/)).toBeNull()
    expect(screen.getByText(/did not answer just then/)).toBeTruthy()
    // Twice on purpose: the note somebody reads and the live region somebody
    // hears. A failure that is only visible is a failure a screen reader meets
    // as silence, which is the state this whole component exists to end.
    expect(screen.getAllByText(/says nothing about the card you typed/))
      .toHaveLength(2)
  })

  it('names a lost session, because that one has a next step in it', async () => {
    suggestCards.mockRejectedValue(new ApiError('authentication required', 401))
    render(<CardFinder value={null} onChange={() => {}} identity={[]} />)
    await type('Sol Ring')

    expect(screen.getByText(/signed out/)).toBeTruthy()
    expect(screen.getByText(/nothing you typed here has been lost/)).toBeTruthy()
    expect(screen.queryByText(/spelled anything like that/)).toBeNull()
  })

  it('still says nothing matched when the library really answered nothing', async () => {
    suggestCards.mockResolvedValue({ cards: [] })
    render(<CardFinder value={null} onChange={() => {}} identity={[]} />)
    await type('qqqqzzz')

    expect(screen.getByText(/spelled anything like that/)).toBeTruthy()
    expect(screen.queryByText(/did not answer just then/)).toBeNull()
  })

  // A failure followed by a good answer must clear, or one blip poisons the
  // box until it is closed.
  it('recovers on the next answer', async () => {
    suggestCards.mockRejectedValue(new Error('down'))
    render(<CardFinder value={null} onChange={() => {}} identity={[]} />)
    await type('Sol')
    expect(screen.getByText(/did not answer just then/)).toBeTruthy()

    suggestCards.mockResolvedValue({ cards: [SOL] })
    await type('Sol Ring')
    expect(screen.queryByText(/did not answer just then/)).toBeNull()
    expect(screen.getByText('Sol Ring')).toBeTruthy()
  })
})
