/**
 * The combos section: what it says to a reader, and what it writes for an
 * owner.
 *
 * Three properties carry the file. **The block is written whole** — there is no
 * per-entry route, because a combo's name is the cards it is made of and those
 * are the very thing an edit changes — so every control here has to send the
 * list as it will be afterwards, and getting that wrong silently deletes
 * somebody's other entries. **The near-miss's swap-board button does not write
 * a rationale** (rule 4, ADR 8): "the app can see what this card is for" is
 * exactly the reasoning that would break it, so on a curated deck the control
 * asks. And **the section is a disclosure, not a link** (commandment 20).
 */

import { cleanup, fireEvent, render, screen, waitFor, within }
  from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Combo, ComboCardRef, DeckRef, Glossary } from '../lib/api'
import { resetGlossaryCache } from '../lib/glossary'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: {
    setCombos: vi.fn(),
    addToBoard: vi.fn(),
    // `CardFinder` reaches for this the moment somebody types. The form does
    // not open in most of these, but a mock missing a method the tree can call
    // fails as an unrelated TypeError three tests later.
    suggestCards: vi.fn(),
    // `Term` looks the word up; a rejected glossary degrades to plain text,
    // which is the behaviour the vocabulary is built to have.
    glossary: vi.fn(),
  },
}))

const { api } = await import('../lib/api')
const { Combos } = await import('./combos')

const REF: DeckRef = { owner: 'aasquier', slug: 'arcades-walls' }

/** The one word this section leans on, so the vocabulary actually resolves
 *  here rather than degrading to plain text. Commandment 2 is the reason the
 *  term is marked up at all, and a suite that only ever saw the degraded form
 *  would not notice the affordance disappearing. */
const GLOSSARY: Glossary = {
  sections: [{ key: 'building', label: 'Building a deck', blurb: '' }],
  terms: [{
    key: 'combo', term: 'Combo',
    short: 'Two or three cards that, put together, do something none of them can do alone.',
    long: 'At length.', section: 'building', see_also: [],
  }],
}

function ref(name: string, in_deck = true): ComboCardRef {
  return { name, image: `https://example.test/${name}.jpg`, art_crop: null, in_deck }
}

const MACHINE: Combo = {
  cards: [ref('Axebane Guardian'), ref('High Alert')],
  produces: 'infinite colored mana',
  how: '1) Tap Axebane Guardian for X. 2) Pay {2}{W}{U} to untap it. 3) Repeat.',
  setup: 'six mana across two quiet turns',
  needs: null, cut: null,
}

const NEAR: Combo = {
  cards: [ref('Axebane Guardian')],
  produces: 'infinite colored mana',
  how: 'Equip and untap, over and over.',
  setup: 'four mana',
  needs: ref('Umbral Mantle', false),
  cut: ref('Suspicious Bookcase'),
}

function show(over: Partial<Parameters<typeof Combos>[0]> = {}) {
  return render(
    <Combos combos={[MACHINE]} deckRef={REF} stage="curated" identity={['G', 'W', 'U']}
            writable={false} onChanged={vi.fn()} {...over} />)
}

/** Open the fold. Driven through the real control rather than by setting
 *  state, because "is this operable" is half of what is being checked. */
function unfold() {
  const toggle = screen.getByRole('button', { name: /Combos/ })
  fireEvent.click(toggle)
  return toggle
}

// `artifacts.test.tsx`'s shape: clearing after rather than before, because a
// `beforeEach` reset makes the refusal tests below report their caught
// rejection as an unhandled one.
describe('Combos', () => {
  // The glossary is memoised at module scope, so the cache is cleared as well
  // as the mock re-armed — `term.test.tsx`'s shape, and the reason a second
  // test in this file would otherwise see the first one's table.
  beforeEach(() => {
    resetGlossaryCache()
    vi.mocked(api.glossary).mockResolvedValue(GLOSSARY)
  })
  afterEach(() => { cleanup(); vi.clearAllMocks() })

  it('is a disclosure that says which state it is in', () => {
    show()
    const toggle = screen.getByRole('button', { name: /Combos/ })
    // Commandment 20: a control that changes this page is a button, and it
    // says whether it is open. Not a link, which would promise a destination.
    expect(toggle.tagName).toBe('BUTTON')
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByText(/infinite colored mana/)).toBeNull()

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(screen.getByText(/infinite colored mana/)).toBeTruthy()
  })

  // **The fault the full suite found, and it was not a small one.** With no
  // default, `combos.length` throws during render and React unwinds the whole
  // deck page — the 99, the swap board, the graveyard, all of it — over a
  // section that had nothing to show. The case is reachable for real: a deploy
  // changes both halves, and for a few seconds a browser can hold this bundle
  // against the server that came before it.
  it('draws nothing rather than throwing when the deck has no combos field', () => {
    const { container } = render(
      <Combos combos={undefined} deckRef={REF} stage="curated" identity={['G']}
              writable={false} onChanged={vi.fn()} />)
    expect(container.querySelector('section')).toBeNull()

    // And the owner still gets the shelf, because a server with no combos
    // block is a deck that catalogues nothing — which is what the empty state
    // already says.
    cleanup()
    render(
      <Combos combos={undefined} deckRef={REF} stage="curated" identity={['G']}
              writable onChanged={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Combos' })).toBeTruthy()
  })

  it('hides itself entirely from a reader with nothing to read', () => {
    const { container } = show({ combos: [], writable: false })
    // Like the graveyard: an empty shelf is furniture to somebody who cannot
    // press anything on it.
    expect(container.querySelector('section')).toBeNull()
  })

  it('gives the owner an empty shelf that explains itself', () => {
    show({ combos: [], writable: true })
    unfold()
    // Not "No combos." — the one screen where an explanation is going to be
    // read must not be spent reporting emptiness (commandment 2).
    expect(screen.getByText(/small machine/)).toBeTruthy()
    expect(screen.getByText(/changes nothing about the deck/)).toBeTruthy()
    // And it says the reassuring thing a newcomer needs most.
    expect(screen.getByText(/Plenty of good decks have none at all/)).toBeTruthy()
    expect(screen.getByRole('button', { name: '+ Catalogue a combo' })).toBeTruthy()
    // No count beside a heading nobody has used. A "0" reads as a score.
    expect(screen.getByRole('button', { name: 'Combos' })).toBeTruthy()
  })

  it('heads an entry with its pieces and no name of its own', () => {
    show()
    unfold()
    const heading = screen.getByRole('heading', { level: 4 })
    // The heading is the cards, joined — the entry's only name, derived so it
    // cannot disagree with the pieces.
    expect(heading.textContent?.replace(/\s+/g, ' ').trim())
      .toBe('Axebane Guardian + High Alert')
  })

  it('draws the instructions as steps, and prose as prose', () => {
    show()
    unfold()
    const steps = document.querySelectorAll('.combo-steps li')
    expect(steps).toHaveLength(3)
    // The markers are stripped: the browser numbers a real ordered list, so
    // "1) 1) Tap" is what leaving them in would produce.
    expect(steps[0]?.textContent).toContain('Tap Axebane Guardian for X')
    expect(steps[0]?.textContent).not.toContain('1)')
    cleanup()

    // A `how` written as a sentence stays one.
    show({ combos: [NEAR] })
    unfold()
    expect(document.querySelectorAll('.combo-steps li')).toHaveLength(0)
    expect(screen.getByText(/Equip and untap, over and over/)).toBeTruthy()
  })

  it('marks a near-miss and states the trade in plain words', () => {
    const { container } = show({ combos: [NEAR] })
    unfold()
    // Visually distinct, held as the class rather than as pixels: jsdom has no
    // layout, and which class this component puts on the entry is the half a
    // suite can actually answer.
    expect(container.querySelector('.combo-entry.is-near')).toBeTruthy()
    const trade = container.querySelector('.combo-trade') as HTMLElement
    expect(trade).toBeTruthy()
    const said = trade.textContent?.replace(/\s+/g, ' ') ?? ''
    expect(said).toContain('One card away.')
    expect(said).toContain('Bring in Umbral Mantle')
    expect(said).toContain('cut Suspicious Bookcase')
    // The card it is waiting for is marked as one the deck does not have.
    expect(within(trade).getByText(/not in the deck/)).toBeTruthy()
  })

  it('says when a near-miss names no cut, rather than saying nothing', () => {
    show({ combos: [{ ...NEAR, cut: null }] })
    unfold()
    // Aaron's rule is that the trade is always part of the entry. When the
    // file does not carry one, the page says so — the gate warns about the
    // same absence, and a page that stayed quiet would leave the warning with
    // nothing to point at.
    expect(screen.getByText(/Nothing is named to come out for it yet/)).toBeTruthy()
  })

  it('offers a reader no controls at all', () => {
    show({ combos: [NEAR], writable: false })
    unfold()
    for (const name of [/Weigh /, /^Edit$/, /^Remove$/, /Catalogue/]) {
      expect(screen.queryByRole('button', { name }),
        `a reader was offered ${String(name)}`).toBeNull()
    }
  })

  it('shows the mark on an entry Claude drafted', () => {
    show({ combos: [{ ...MACHINE, by: 'claude' }] })
    unfold()
    // The same words the rationale carries one section up: a reader should
    // meet one sentence about who wrote a thing, not two.
    expect(screen.getByText('Claude drafted this')).toBeTruthy()
    cleanup()

    show()
    unfold()
    expect(screen.queryByText('Claude drafted this'),
      'an unmarked entry claims nothing').toBeNull()
  })

  /* ------------------------------------------------------ the swap board */

  it('asks for a rationale before weighing the card a combo needs', async () => {
    vi.mocked(api.addToBoard).mockImplementation(() =>
      Promise.resolve({ slug: 'arcades-walls' } as never))
    show({ combos: [NEAR], writable: true, stage: 'curated' })
    unfold()

    // **Rule 4, ADR 8: no surface writes a `why` unasked.** The press opens a
    // box; it does not compose a sentence out of what the combo says.
    fireEvent.click(screen.getByRole('button', { name: 'Weigh Umbral Mantle' }))
    expect(api.addToBoard).not.toHaveBeenCalled()
    const box = screen.getByLabelText(/Why you are weighing it/) as HTMLTextAreaElement
    expect(box.value, 'the box opens empty, not prefilled').toBe('')
    // And the save is off until there is a sentence to save.
    const save = screen.getByRole('button', { name: 'Put it on the swap board' })
    expect((save as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(box, { target: { value: 'Finishes the Guardian loop.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Put it on the swap board' }))
    await waitFor(() => expect(api.addToBoard).toHaveBeenCalled())
    expect(api.addToBoard).toHaveBeenCalledWith(REF, {
      name: 'Umbral Mantle', category: 'threat', why: 'Finishes the Guardian loop.',
    })
    expect(screen.getByText('Umbral Mantle is on the swap board.')).toBeTruthy()
  })

  it('goes straight through on a draft, which owes its reasons later', async () => {
    vi.mocked(api.addToBoard).mockImplementation(() =>
      Promise.resolve({ slug: 'arcades-walls' } as never))
    show({ combos: [NEAR], writable: true, stage: 'draft' })
    unfold()

    // The swap board's own rule (ADR 13), read off the same field rather than
    // decided again here: a draft is honestly incomplete.
    fireEvent.click(screen.getByRole('button', { name: 'Weigh Umbral Mantle' }))
    await waitFor(() => expect(api.addToBoard).toHaveBeenCalled())
    expect(vi.mocked(api.addToBoard).mock.calls[0]?.[1].why).toBe('')
  })

  it('shows the server’s own refusal when the board says no', async () => {
    vi.mocked(api.addToBoard).mockImplementation(() =>
      Promise.reject(new Error('Umbral Mantle is already in this deck')))
    show({ combos: [NEAR], writable: true, stage: 'draft' })
    unfold()

    fireEvent.click(screen.getByRole('button', { name: 'Weigh Umbral Mantle' }))
    await waitFor(() => expect(screen.getByRole('alert').textContent)
      .toContain('already in this deck'))
  })

  /* ----------------------------------------------------- writing the block */

  it('removes an entry only on the second press, and sends the rest', async () => {
    vi.mocked(api.setCombos).mockImplementation(() =>
      Promise.resolve({ slug: 'arcades-walls' } as never))
    const onChanged = vi.fn()
    show({ combos: [MACHINE, NEAR], writable: true, onChanged })
    unfold()

    const remove = screen.getAllByRole('button', { name: 'Remove' })[0] as HTMLElement
    fireEvent.click(remove)
    expect(api.setCombos, 'one press arms it and writes nothing').not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: 'Press again to remove' })).toBeTruthy()
    // And there is a way back out of an armed control.
    expect(screen.getByRole('button', { name: 'Keep it' })).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Press again to remove' }))
    await waitFor(() => expect(api.setCombos).toHaveBeenCalled())
    // **The whole block, minus one.** A write that sent only the removal would
    // delete the entry beside it.
    const sent = vi.mocked(api.setCombos).mock.calls[0]?.[1]
    expect(sent).toHaveLength(1)
    expect(sent?.[0]?.cards).toEqual(['Axebane Guardian'])
    expect(sent?.[0]?.needs).toBe('Umbral Mantle')
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
  })

  it('shows the server’s refusal when a removal is turned down', async () => {
    // **The bug this found.** `onRemove` was typed `() => void`, so TypeScript
    // erased the promise, the `await` had nothing to await, and a refused
    // removal became an unhandled rejection with no sentence anywhere on the
    // page. The entry also has to come back out of its armed state, or the
    // only control left says "Press again to remove" over a failure.
    vi.mocked(api.setCombos).mockImplementation(() =>
      Promise.reject(new Error('deck.yaml does not parse')))
    show({ combos: [MACHINE], writable: true })
    unfold()

    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    fireEvent.click(screen.getByRole('button', { name: 'Press again to remove' }))
    await waitFor(() => expect(screen.getByRole('alert').textContent)
      .toContain('does not parse'))
    expect(screen.getByRole('button', { name: 'Remove' }),
      'a failed removal disarms rather than staying cocked').toBeTruthy()
  })

  it('edits one entry in place and sends every other one untouched', async () => {
    vi.mocked(api.setCombos).mockImplementation(() =>
      Promise.resolve({ slug: 'arcades-walls' } as never))
    show({ combos: [MACHINE, NEAR], writable: true })
    unfold()

    fireEvent.click(screen.getAllByRole('button', { name: 'Edit' })[1] as HTMLElement)
    const produces = screen.getByPlaceholderText(/infinite colored mana; infinite untaps/)
    fireEvent.change(produces, { target: { value: 'infinite mana, at last' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save the combo' }))

    await waitFor(() => expect(api.setCombos).toHaveBeenCalled())
    const sent = vi.mocked(api.setCombos).mock.calls[0]?.[1]
    expect(sent).toHaveLength(2)
    expect(sent?.[0]?.produces, 'the untouched entry went back as it was')
      .toBe('infinite colored mana')
    expect(sent?.[1]?.produces).toBe('infinite mana, at last')
    // The trade survived an edit that was not about it.
    expect(sent?.[1]?.cut).toBe('Suspicious Bookcase')
  })

  it('refuses to save an entry with no pieces or nothing produced, and says why', () => {
    show({ combos: [], writable: true })
    unfold()
    fireEvent.click(screen.getByRole('button', { name: '+ Catalogue a combo' }))

    const save = screen.getByRole('button', { name: 'Catalogue it' }) as HTMLButtonElement
    // Off, rather than pressed and refused — the two the server insists on are
    // said here so the refusal is never a surprise.
    expect(save.disabled).toBe(true)
    expect(screen.getByText('Name at least one piece.')).toBeTruthy()
  })

  it('offers the near-miss fields as a setting that says it is on', () => {
    show({ combos: [], writable: true })
    unfold()
    fireEvent.click(screen.getByRole('button', { name: '+ Catalogue a combo' }))

    const toggle = screen.getByRole('button', { name: 'This deck is one card short' })
    // Commandment 20 again: a setting is a button that says which state it is
    // in, and it answers the hand (`.chip-toggle`, never a bare button).
    expect(toggle.getAttribute('aria-pressed')).toBe('false')
    expect(toggle.className).toContain('chip-toggle')
    expect(screen.queryByText(/only a suggestion once there is a slot/)).toBeNull()

    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByText(/only a suggestion once there is a slot/)).toBeTruthy()
  })

  it('keeps what was typed when the server refuses the block', async () => {
    vi.mocked(api.setCombos).mockImplementation(() =>
      Promise.reject(new Error('combo 1: say what it produces')))
    show({ combos: [MACHINE], writable: true })
    unfold()

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    const produces = screen.getByPlaceholderText(/infinite colored mana; infinite untaps/)
    fireEvent.change(produces, { target: { value: 'a rewrite' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save the combo' }))

    await waitFor(() => expect(screen.getByRole('alert').textContent)
      .toContain('say what it produces'))
    // A refused save must not throw the typing away.
    const kept = screen.getByPlaceholderText(/infinite colored mana; infinite untaps/)
    expect((kept as HTMLInputElement).value).toBe('a rewrite')
  })
})
