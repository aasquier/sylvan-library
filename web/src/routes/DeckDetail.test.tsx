/** Replacement shortlists on the validation tab.
 *
 * The rendering is mostly markup, but two things here are real behaviour and
 * worth pinning: the shortlist is fetched lazily, because building one costs a
 * pool query per offending card and most visits never open this tab; and a
 * candidate is matched to its error by card name, so a deck with two problems
 * does not show one card's suggestions under the other's.
 */

import { cleanup, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { DeckDetail as Deck, DeckStats, Suggestions, ValidationReport } from '../lib/api'
import DeckDetail from './DeckDetail'

vi.mock('../lib/api', () => ({
  api: {
    deck: vi.fn(), stats: vi.fn(), validate: vi.fn(), suggestions: vi.fn(),
    swapCard: vi.fn(), addCard: vi.fn(), removeCard: vi.fn(),
    setCardField: vi.fn(), setNote: vi.fn(), setDeckField: vi.fn(),
    claudeStatus: vi.fn(), interview: vi.fn(),
  },
}))

const { api } = await import('../lib/api')

const DECK = {
  slug: 'goreclaw-stompy',
  name: 'Goreclaw — Mono-Green Stompy',
  commander: ['Goreclaw, Terror of Qal Sisma'],
  companion: null,
  bracket: 4,
  total_cards: 99,
  land_count: 34,
  strategy: 'Mono-green big stompy.',
  art_crop: null,
  color_identity: ['G'],
  errors: 1,
  warnings: 0,
  notes: {},
  commander_card: null,
  corpus_available: true,
  swap_board: [],
  cards: [{
    name: 'Primeval Titan', category: 'ramp', why: 'Ramp and threat in one card.',
    qty: 1, known: true, mana_cost: '{4}{G}{G}', cmc: 6, type_line: 'Creature — Giant',
    color_identity: ['G'], art_crop: 'https://example.test/titan.jpg',
  }],
} as unknown as Deck

const STATS = {
  slug: 'goreclaw-stompy', name: DECK.name, bracket: 4, total_cards: 99,
  land_count: 34, curve: { average_mv: 3.5, nonland_cards: 65, buckets: [] },
  categories: [], colors: [], types: {},
} as DeckStats

const REPORT: ValidationReport = {
  ok: false,
  errors: [{ code: 'banned', message: 'not legal in Commander', card: 'Primeval Titan' }],
  warnings: [],
}

const SHORTLIST: Suggestions = {
  slug: 'goreclaw-stompy',
  corpus_available: true,
  targets: [{
    card: 'Primeval Titan',
    code: 'banned',
    why: 'Ramp and threat in one card.',
    candidates: [{
      name: 'Cultivator Colossus', mana_cost: '{4}{G}{G}{G}', cmc: 7,
      type_line: 'Creature — Plant Beast', oracle_text: 'Trample.',
      color_identity: ['G'], image: 'https://example.test/colossus-full.jpg',
      art_crop: 'https://example.test/colossus.jpg', edhrec_rank: 1589,
      score: 0.7593,
      reasons: ['same card type (Creature)', 'shares trample', 'EDHREC rank 1,589'],
    }],
  }],
}

const EDIT_RESULT = {
  slug: 'goreclaw-stompy', stage: 'curated', total_cards: 99,
  needs_rationale: 0, ok: true, errors: [], warnings: [],
}

/** A draft, mid-import: two of its three cards still owe a rationale. */
const DRAFT = {
  ...DECK,
  stage: 'draft',
  needs_rationale: 2,
  cards: [
    DECK.cards[0],
    { name: 'Sol Ring', category: 'ramp', why: '', qty: 1, known: true,
      mana_cost: '{1}', cmc: 1, type_line: 'Artifact',
      oracle_text: '{T}: Add {C}{C}.', color_identity: [] },
    { name: 'Forest', category: 'land', why: '', qty: 30, known: true,
      type_line: 'Basic Land — Forest', oracle_text: '{T}: Add {G}.',
      color_identity: ['G'] },
  ],
} as unknown as Deck

function renderDeck() {
  return render(
    <MemoryRouter initialEntries={['/decks/goreclaw-stompy']}>
      <Routes>
        <Route path="/decks/:slug" element={<DeckDetail />} />
      </Routes>
    </MemoryRouter>,
  )
}

/** Open the tab the shortlist lives on. */
function openValidation() {
  fireEvent.click(screen.getByRole('button', { name: 'Validation' }))
}

/** A stance rendered the way `mtglab.claude.stance.describe` renders one. */
const STANCE = {
  preset: 'consultant', allows_calls: true, may_write: false,
  axes: [
    { axis: 'initiative', question: 'When may it speak?', level: 'on-request',
      means: 'Only when you ask it something.',
      levels: ['off', 'on-request', 'volunteers', 'interjects'] },
    { axis: 'scope', question: 'How far?', level: 'flagged',
      means: 'Only the cards the gate already flagged.',
      levels: ['flagged', 'adjacent', 'rethink'] },
    { axis: 'write', question: 'What may it change?', level: 'none',
      means: 'Nothing. It talks; you type.',
      levels: ['none', 'proposes', 'applies'] },
  ],
}

const CLAUDE_STATUS = {
  installed: true, configured: true, model: 'claude-sonnet-5',
  stance: STANCE, ceiling: STANCE, default: STANCE, presets: [],
  never: 'No stance lets Claude write a card’s rationale.',
  modes: [{ name: 'rationale-interview', purpose: 'Asks about a slot.',
            tools: ['get_cards'], writes: [] }],
}

const INTERVIEW = {
  answered_by: 'claude', mode: 'rationale-interview',
  model: 'claude-sonnet-5', slug: 'goreclaw-stompy', card: 'Sol Ring',
  asked: true, reason: '', stance: STANCE,
  questions: [
    { question: 'What is it accelerating you into?', angle: 'role',
      fact: 'Adds two colourless.' },
  ],
  questions_dropped: 0,
  tool_calls: [], usage: { input_tokens: 10, output_tokens: 5 },
  never: 'These are questions. The rationale is yours to write.',
}

beforeEach(() => {
  // `mockReset`, not just a fresh return value. `restoreMocks` in
  // vitest.config.ts only touches spies created with `vi.spyOn`; a bare
  // `vi.fn()` from a mock factory is module-level state that survives between
  // tests, so call counts accumulate and "called once" quietly means "called
  // once this test, plus however many times the last one called it".
  vi.mocked(api.deck).mockReset().mockResolvedValue(DECK)
  vi.mocked(api.stats).mockReset().mockResolvedValue(STATS)
  vi.mocked(api.validate).mockReset().mockResolvedValue(REPORT)
  vi.mocked(api.suggestions).mockReset().mockResolvedValue(SHORTLIST)
  vi.mocked(api.swapCard).mockReset().mockResolvedValue({
    slug: 'goreclaw-stompy', swapped_out: 'Primeval Titan',
    swapped_in: 'Cultivator Colossus', why: 'because', ok: true,
    errors: [], warnings: [], stage: 'curated', total_cards: 99,
    needs_rationale: 0,
  })
  for (const fn of [api.addCard, api.removeCard, api.setCardField, api.setNote,
                    api.setDeckField]) {
    vi.mocked(fn).mockReset().mockResolvedValue(EDIT_RESULT)
  }
  // Installed and configured by default, so the interview panel renders its
  // button rather than its "not installed" note in most tests.
  vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(CLAUDE_STATUS)
  vi.mocked(api.interview).mockReset().mockResolvedValue(INTERVIEW)
})

afterEach(cleanup)

describe('DeckDetail validation tab', () => {
  it('does not fetch a shortlist until the tab is opened', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    expect(api.suggestions).not.toHaveBeenCalled()

    openValidation()
    await waitFor(() => expect(api.suggestions).toHaveBeenCalledWith('goreclaw-stompy'))
  })

  it('fetches the shortlist once, not on every tab switch', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    await waitFor(() => expect(api.suggestions).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('button', { name: 'Notes' }))
    openValidation()
    await waitFor(() => expect(screen.getByText('Cultivator Colossus')).toBeTruthy())
    expect(api.suggestions).toHaveBeenCalledTimes(1)
  })

  it('renders candidates under the error they belong to, with their reasons', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()

    const candidate = await screen.findByText('Cultivator Colossus')
    const block = candidate.closest('li')!
    expect(within(block).getByText(/same card type \(Creature\)/)).toBeTruthy()
    expect(within(block).getByText(/EDHREC rank 1,589/)).toBeTruthy()
  })

  it('says the shortlist is not a recommendation', async () => {
    // The whole design rests on this being a measurement the user overrules.
    // If the disclaimer ever quietly disappears, that is a change worth failing.
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    await screen.findByText('Cultivator Colossus')
    expect(screen.getByText(/not a recommendation/)).toBeTruthy()
  })

  it('shows the error alone when there is no shortlist for it', async () => {
    vi.mocked(api.suggestions).mockResolvedValue({
      slug: 'goreclaw-stompy', corpus_available: false, targets: [],
    })
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()

    await screen.findByText(/not legal in Commander/)
    expect(screen.queryByText(/not a recommendation/)).toBeNull()
  })

  it('will not apply a swap until a rationale is written', async () => {
    // Rule 4 at the last place it can be enforced. A tool-written rationale is
    // the empty justification the rule exists to prevent, so the button stays
    // disabled rather than the app inventing one.
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    fireEvent.click(await screen.findByRole('button', { name: 'Use this card' }))

    const apply = screen.getByRole('button', { name: 'Apply swap' })
    expect(apply.hasAttribute('disabled')).toBe(true)
    fireEvent.click(apply)
    expect(api.swapCard).not.toHaveBeenCalled()

    fireEvent.change(screen.getByRole('textbox'),
                     { target: { value: 'It ramps and it attacks.' } })
    expect(screen.getByRole('button', { name: 'Apply swap' }).hasAttribute('disabled'))
      .toBe(false)
  })

  it('sends the swap the user composed, and refetches what it invalidated', async () => {
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    fireEvent.click(await screen.findByRole('button', { name: 'Use this card' }))
    fireEvent.change(screen.getByRole('textbox'),
                     { target: { value: 'It ramps and it attacks.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply swap' }))

    await waitFor(() => expect(api.swapCard).toHaveBeenCalledWith('goreclaw-stompy', {
      out: 'Primeval Titan',
      into: 'Cultivator Colossus',
      why: 'It ramps and it attacks.',
    }))
    // The gate result and the deck are both stale the moment a swap lands.
    await waitFor(() => expect(api.validate).toHaveBeenCalledTimes(2))
    expect(api.deck).toHaveBeenCalledTimes(2)
  })

  it('surfaces a refusal instead of pretending the swap worked', async () => {
    vi.mocked(api.swapCard).mockRejectedValue(
      new Error("'Rhystic Study' identity {U} is outside the commander's {G}"))
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    fireEvent.click(await screen.findByRole('button', { name: 'Use this card' }))
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'nope' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply swap' }))

    await waitFor(() => expect(screen.getByText(/outside the commander/)).toBeTruthy())
    expect(api.deck).toHaveBeenCalledTimes(1)
  })

  it('offers the full card on hover for a suggestion, not just the art', async () => {
    // Accepting a suggestion means reading its rules text, and the shortlist
    // shows only art and a score. Same affordance as the decklist rows.
    renderDeck()
    await screen.findByRole('button', { name: 'Validation' })
    openValidation()
    const art = await screen.findByAltText('Cultivator Colossus')

    fireEvent.mouseEnter(art.parentElement!, { clientX: 10, clientY: 10 })
    const full = await screen.findAllByAltText('Cultivator Colossus')
    expect(full.length).toBe(2)
    expect(full.some((el) => el.getAttribute('src')?.includes('colossus-full'))).toBe(true)
  })

  // ------------------------------------------------------------- draft stage

  it('says nothing about drafts for a curated deck', async () => {
    renderDeck()
    await screen.findByText(DECK.name)
    expect(screen.queryByText(/still need a/)).toBeNull()
    expect(screen.queryByText('no rationale yet')).toBeNull()
  })

  it('leads a draft with the count it owes, and marks each blank card', async () => {
    // ADR 13: a to-do list with a number on it. The banner carries the number;
    // the per-card marks are where the work actually is.
    vi.mocked(api.deck).mockResolvedValue({
      ...DECK,
      stage: 'draft',
      needs_rationale: 1,
      cards: [{ ...DECK.cards[0], why: '' }],
    } as unknown as Deck)
    renderDeck()
    await screen.findByText(DECK.name)

    expect(screen.getByText(/1 of 1 cards still need/)).toBeTruthy()
    expect(screen.getByText('draft')).toBeTruthy()
    expect(screen.getByText('no rationale yet')).toBeTruthy()
    // And it says how to get out, which the badge alone does not.
    expect(screen.getByText(/becomes a promotion you can make here/)).toBeTruthy()
    // Not yet, though: nothing offers to promote a deck that still owes work.
    expect(screen.queryByRole('button', { name: /promote/i })).toBeNull()
  })
})

/**
 * The rationale editor.
 *
 * The first test in here is the one that matters most, and it is not about
 * rendering: it pins that a card with no `why` opens an *empty* box. Rule 4
 * and ADR 12 rule 3 say the tool never authors a rationale, and the cheapest
 * way to break that is a placeholder that is really a first draft. This test
 * fails if anyone ever pre-fills the field.
 */
describe('DeckDetail rationale editor', () => {
  /** The row for one card.
   *
   * Anchored on the remove button's title rather than the card name, which
   * appears twice in a row -- once on the art's hover target and once on the
   * name itself. */
  function rowFor(card: string) {
    return screen.getByTitle(`Remove ${card} from the deck`).closest('li')!
  }

  async function openEditorFor(card: string) {
    renderDeck()
    await screen.findByText(DECK.name)
    const row = rowFor(card)
    fireEvent.click(within(row).getByRole('button', { name: /why/i }))
    return row
  }

  it('opens an empty box for a card that has no rationale', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    const box = within(row).getByRole('textbox') as HTMLTextAreaElement

    expect(box.value).toBe('')
    // The prompt is a question, not a draft. If this ever reads like a
    // rationale, the tool has started writing them.
    expect(box.placeholder).toMatch(/\?$/)
    expect(within(row).getByRole('button', { name: /save rationale/i })
      .hasAttribute('disabled')).toBe(true)
  })

  it('writes exactly what the user typed', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    fireEvent.change(within(row).getByRole('textbox'),
                     { target: { value: '  Two mana for one.  ' } })
    fireEvent.click(within(row).getByRole('button', { name: /save rationale/i }))

    await waitFor(() => expect(api.setCardField).toHaveBeenCalledWith(
      'goreclaw-stompy', 'Sol Ring', 'why', 'Two mana for one.'))
  })

  it('loads an existing rationale for editing rather than starting blank', async () => {
    const row = await openEditorFor('Primeval Titan')
    expect((within(row).getByRole('textbox') as HTMLTextAreaElement).value)
      .toBe('Ramp and threat in one card.')
  })

  /**
   * The rationale interview, in the same column as the corpus text.
   *
   * The test that matters here is the second one: asking for questions must
   * leave the textarea exactly as the user left it. Every other guard on this
   * boundary is server-side — the mode has no write tool, the response schema
   * has no field for a rationale, non-questions are dropped — and this is the
   * one that would catch somebody adding a "use this" button in the UI.
   */
  it('does not ask anything until the button is pressed', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    // Opening the editor may check what is installed; it may not spend money.
    await waitFor(() => expect(api.claudeStatus).toHaveBeenCalled())
    expect(api.interview).not.toHaveBeenCalled()

    fireEvent.click(await within(row).findByRole(
      'button', { name: /ask for questions/i }))
    await waitFor(() => expect(api.interview).toHaveBeenCalledWith(
      'goreclaw-stompy', { card: 'Sol Ring' }))
  })

  it('renders the questions beside the box and puts nothing in it', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    const box = within(row).getByRole('textbox') as HTMLTextAreaElement
    fireEvent.change(box, { target: { value: 'my own words' } })

    fireEvent.click(await within(row).findByRole(
      'button', { name: /ask for questions/i }))
    await within(row).findByText('What is it accelerating you into?')

    // The whole rule, in one assertion: questions arrived, the field did not
    // change, and there is no control offering to change it.
    expect(box.value).toBe('my own words')
    expect(within(row).queryByRole('button', { name: /use this|insert|copy/i }))
      .toBeNull()
  })

  it('labels the answer as Claude’s rather than the gate’s', async () => {
    // ADR 14 boundary 3. The gate's output is reproducible and this is not,
    // and a user must be able to tell which one they are reading.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    fireEvent.click(await within(row).findByRole(
      'button', { name: /ask for questions/i }))

    await within(row).findByText(/not the gate/)
    expect(within(row).getByText(/yours to write/)).toBeTruthy()
  })

  it('reports dropped answers rather than hiding them', async () => {
    // A model that has started writing rationales instead of asking questions
    // should be visible, not silently filtered.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    vi.mocked(api.interview).mockResolvedValue({
      ...INTERVIEW, questions: [], questions_dropped: 2,
    })
    const row = await openEditorFor('Sol Ring')
    fireEvent.click(await within(row).findByRole(
      'button', { name: /ask for questions/i }))

    await within(row).findByText(/2 answers were not a question/)
  })

  it('says a stance of off made no call, rather than showing an empty list', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    vi.mocked(api.interview).mockResolvedValue({
      ...INTERVIEW, asked: false, questions: [],
      reason: 'The stance is off, so no call was made.',
    })
    const row = await openEditorFor('Sol Ring')
    fireEvent.click(await within(row).findByRole(
      'button', { name: /ask for questions/i }))

    await within(row).findByText(/stance is off/)
  })

  it('offers nothing to press when the SDK is not installed', async () => {
    // Three different answers, kept apart: not installed, no key, and nothing
    // asked yet. Collapsing them tells someone their key is missing when they
    // simply never installed the extra.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    vi.mocked(api.claudeStatus).mockResolvedValue({
      ...CLAUDE_STATUS, installed: false,
    })
    const row = await openEditorFor('Sol Ring')

    await within(row).findByText(/not installed/)
    expect(within(row).queryByRole('button', { name: /ask for questions/i }))
      .toBeNull()
  })

  it('shows the card as the corpus has it, beside the box', async () => {
    // Rule 1 made useful: you argue about the card against what it says, not
    // against what you remember it saying. Asserted on the type line because
    // `ManaText` splits the oracle text into symbol elements, so a plain
    // substring match on it would be testing the renderer instead.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    expect(within(row).getByText('Artifact')).toBeTruthy()
    expect(within(row).queryByText(/No corpus text/)).toBeNull()
  })

  it('surfaces a refusal instead of pretending the edit landed', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    vi.mocked(api.setCardField).mockRejectedValue(
      new Error('a card in a curated deck needs a `why`'))
    const row = await openEditorFor('Sol Ring')
    fireEvent.change(within(row).getByRole('textbox'), { target: { value: 'x' } })
    fireEvent.click(within(row).getByRole('button', { name: /save rationale/i }))

    await screen.findByText(/needs a `why`/)
  })

  it('re-reads the deck after a successful edit', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    fireEvent.change(within(row).getByRole('textbox'), { target: { value: 'Fast mana.' } })
    fireEvent.click(within(row).getByRole('button', { name: /save rationale/i }))

    // Once on mount, once after the write: the page shows more than the gate
    // verdict the write returns, so a stale curve beside a fresh gate is worse
    // than a second round trip.
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
    expect(api.validate).toHaveBeenCalledTimes(2)
  })

  it('filters the list down to the cards a draft still owes', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    renderDeck()
    await screen.findByText(DECK.name)
    expect(rowFor('Primeval Titan')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /show the 2 that need one/i }))
    await waitFor(() => expect(
      screen.queryByTitle('Remove Primeval Titan from the deck')).toBeNull())
    expect(rowFor('Sol Ring')).toBeTruthy()
    expect(rowFor('Forest')).toBeTruthy()
  })

  it('removes a card and re-reads the deck', async () => {
    renderDeck()
    await screen.findByText(DECK.name)
    const row = rowFor('Primeval Titan')
    fireEvent.click(within(row).getByRole('button', { name: 'Remove' }))

    await waitFor(() => expect(api.removeCard)
      .toHaveBeenCalledWith('goreclaw-stompy', 'Primeval Titan'))
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
  })

  it('reports a refused removal rather than silently doing nothing', async () => {
    vi.mocked(api.removeCard).mockRejectedValue(new Error('this deck is read-only'))
    renderDeck()
    await screen.findByText(DECK.name)
    const row = rowFor('Primeval Titan')
    fireEvent.click(within(row).getByRole('button', { name: 'Remove' }))

    await screen.findByText(/read-only/)
  })
})

/**
 * Promotion — the last step of an import.
 *
 * The button only exists once the work is done, and the server refuses it
 * anyway if it is not. Both halves matter: the UI should not offer an action
 * that will be rejected, and it must not be the thing enforcing the rule.
 */
describe('DeckDetail promotion', () => {
  const FINISHED = { ...DRAFT, needs_rationale: 0,
                     cards: DRAFT.cards.map((c) => ({ ...c, why: 'A reason.' })) } as Deck

  it('offers no promotion while cards still owe a rationale', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    renderDeck()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: /promote/i })).toBeNull()
    expect(screen.getByText(/2 of 3 cards still need/)).toBeTruthy()
  })

  it('offers promotion once nothing is outstanding', async () => {
    vi.mocked(api.deck).mockResolvedValue(FINISHED)
    renderDeck()
    await screen.findByText(DECK.name)
    expect(screen.getByText(/every card carries a rationale/i)).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /promote to curated/i }))
    await waitFor(() => expect(api.setDeckField)
      .toHaveBeenCalledWith('goreclaw-stompy', 'stage', 'curated'))
    // And the page re-reads, so the banner goes away on its own.
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
  })

  it('shows the server refusal rather than assuming it worked', async () => {
    // The UI hides the button when it knows better, but the rule lives in the
    // gate. If the two ever disagree, the gate wins and the user is told.
    vi.mocked(api.deck).mockResolvedValue(FINISHED)
    vi.mocked(api.setDeckField).mockRejectedValue(
      new Error('1 card(s) still have no `why` (Sol Ring)'))
    renderDeck()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: /promote to curated/i }))

    await screen.findByText(/still have no `why`/)
  })

  it('shows no draft banner at all on a curated deck', async () => {
    renderDeck()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: /promote/i })).toBeNull()
    expect(screen.queryByText(/still need/)).toBeNull()
  })
})
