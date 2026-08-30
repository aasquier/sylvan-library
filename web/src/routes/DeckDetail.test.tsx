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
import type {
  CommanderDossier, DeckDetail as Deck, DeckStats, Suggestions, ValidationReport,
} from '../lib/api'
import DeckDetail from './DeckDetail'

vi.mock('../lib/api', async () => ({
  // The real ones: both are pure, and a stub would let a regression in message
  // extraction — or in "is there actually a dossier here" — pass every test in
  // this file.
  errorMessage: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).errorMessage,
  hasDossier: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).hasDossier,
  // Real, because the dossier panel narrows on it: a job that 404s says "that
  // run is gone" rather than showing a bare status code.
  ApiError: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).ApiError,
  // Stubbed rather than real, and not for convenience. The actual `followJob`
  // closes over the module's own `api` binding, so importing it here would
  // reach past this mock and poll for real. Its polling is pinned in
  // `lib/api.test.ts`; what belongs here is what the panel does with the job.
  followJob: vi.fn(),
  api: {
    deck: vi.fn(), stats: vi.fn(), validate: vi.fn(), suggestions: vi.fn(),
    deckLog: vi.fn(),
    swapCard: vi.fn(), addCard: vi.fn(), entombCard: vi.fn(),
    entombCards: vi.fn(), returnCard: vi.fn(), exileCard: vi.fn(),
    setCardField: vi.fn(), setNote: vi.fn(), setDeckField: vi.fn(),
    setShared: vi.fn(),
    claudeStatus: vi.fn(), interview: vi.fn(), argue: vi.fn(),
    argueDeck: vi.fn(),
    commander: vi.fn(),
    dossier: vi.fn(), writeDossier: vi.fn(), printings: vi.fn(),
    job: vi.fn(), wheelSpin: vi.fn(),
  },
}))

const { api, followJob } = await import('../lib/api')

/** A finished job carrying `result`.
 *
 * Writing a dossier answers with a **job** now, not a dossier — it was
 * measured at 236 seconds on the deployed instance, where a synchronous POST
 * died in transit and took the answer with it.
 */
const job = (result: unknown, status: 'queued' | 'done' = 'done') => ({
  id: 'job-dossier', kind: 'claude.dossier', status,
  done: status === 'done' ? 1 : 0, total: 1,
  percent: status === 'done' ? 100 : 0,
  label: 'dossier: Goreclaw, Terror of Qal Sisma',
  result, partial: null, error: null,
  created_at: '2026-08-13T18:40:00+00:00',
})

const DECK = {
  slug: 'goreclaw-stompy',
  // The curated six are the maintainer's and shared by default (ADR 22) —
  // they are the showcase, and an instance whose showcase nobody could see
  // would be an instance with nothing on it.
  owner: 'aasquier',
  shared: true,
  name: 'Goreclaw — Mono-Green Stompy',
  // The owner's view. Every editing test below describes what the maintainer
  // sees; `READ_ONLY_DECK` is the other half.
  writable: true,
  commander: ['Goreclaw, Terror of Qal Sisma'],
  companion: null,
  bracket: 4,
  // ADR 37's labels. Present and empty rather than absent: the wire always
  // carries both, and this fixture is cast (`as unknown as Deck`), so a
  // missing field is not a type error here — it is a crash at render time.
  themes: [],
  archetype: null,
  total_cards: 99,
  land_count: 34,
  strategy: 'Mono-green big stompy.',
  art_crop: null,
  color_identity: ['G'],
  errors: 1,
  warnings: 0,
  notes: {},
  commander_card: null,
  pool_available: true,
  swap_board: [],
  graveyard: [],
  combos: [],
  cards: [{
    name: 'Primeval Titan', category: 'ramp', why: 'Ramp and threat in one card.',
    qty: 1, known: true, mana_cost: '{4}{G}{G}', cmc: 6, type_line: 'Creature — Giant',
    color_identity: ['G'], art_crop: 'https://example.test/titan.jpg',
  }],
} as unknown as Deck

const STATS = {
  slug: 'goreclaw-stompy', name: DECK.name, bracket: 4, total_cards: 99,
  land_count: 34, curve: { average_mv: 3.5, nonland_cards: 65, buckets: [] },
  categories: [], colors: [], types: { Creature: 30, Land: 34 },
  game_changers: { cards: [], count: 0, allowed: 3, bracket: 4, verdict: 'ok' },
  opening: {
    deck_size: 99, hand_size: 7,
    lands: {
      count: 34,
      distribution: [
        { lands: 0, chance: 0.04 }, { lands: 1, chance: 0.16 },
        { lands: 2, chance: 0.29 }, { lands: 3, chance: 0.28 },
        { lands: 4, chance: 0.16 }, { lands: 5, chance: 0.05 },
        { lands: 6, chance: 0.01 }, { lands: 7, chance: 0.001 },
      ],
      keepable: 0.73,
    },
    categories: [
      { category: 'ramp', count: 10, in_opening_hand: 0.55, by_turn_four: 0.72 },
    ],
    singleton: [
      { turn: 1, cards_seen: 8, chance: 8 / 99 },
      { turn: 4, cards_seen: 11, chance: 11 / 99 },
      { turn: 7, cards_seen: 14, chance: 14 / 99 },
      { turn: 10, cards_seen: 17, chance: 17 / 99 },
    ],
  },
} as DeckStats

const REPORT: ValidationReport = {
  ok: false,
  errors: [{ code: 'banned', message: 'not legal in Commander', card: 'Primeval Titan' }],
  warnings: [],
}

const SHORTLIST: Suggestions = {
  slug: 'goreclaw-stompy',
  pool_available: true,
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

/** What `service.commander_dossier` counts off the pool. */
const DOSSIER: CommanderDossier = {
  slug: 'goreclaw-stompy',
  card: null,
  supertypes: ['Legendary', 'Creature'],
  subtypes: [{ name: 'Bear', total: 78, legendary: 26 }],
  other_cards: [{
    name: 'Surrak and Goreclaw', type_line: 'Legendary Creature — Human Bear',
    mana_cost: '{2}{R}{G}', image: 'https://example.test/surrak-full.jpg',
    art_crop: 'https://example.test/surrak.jpg',
  }],
  printings: { count: 12, first_released: '2018-07-13', first_set: 'Core Set 2019' },
}

/** A deck's history as the server sends it (ADR 28).
 *
 * Note what is *not* in it: no rationale text. `set-card` records that a
 * `why` changed and the words stay in `deck.yaml`, so there is no field here
 * for the panel to render one out of even if it wanted to.
 */
const DECK_LOG = {
  slug: 'goreclaw-stompy',
  entries: [
    { id: 3, created_at: '2026-08-15T09:30:00+00:00', actor: 'aasquier',
      action: 'entomb', summary: 'entombed Primeval Titan' },
    { id: 2, created_at: '2026-08-14T18:00:00+00:00', actor: null,
      action: 'set-card', summary: 'changed the rationale for Sol Ring' },
  ],
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

/** The deck's address (ADR 22). Every call this page makes is aimed at one of
 *  these rather than a bare slug, because a slug is unique per owner now and
 *  identifies nothing on its own. */
const REF = { owner: 'aasquier', slug: 'goreclaw-stompy' }

/** Where the dossier panel parks a run id. Owner-qualified for the same
 *  reason: two people's `goreclaw-stompy` are two decks and two runs. */
const JOB_KEY = 'mtglab-dossier-job:aasquier/goreclaw-stompy'

/**
 * The 99 opens folded now (punch list 2026-08-18 item 4 — see `stored` in
 * `DeckDetail`), so almost every test here would otherwise be asserting
 * against a page of closed signs. This seeds the stash the way a reader who
 * has opened the shelf leaves it: an *explicit* empty list per grouping, which
 * is the "I unfolded these" answer rather than the absent "never touched" one.
 *
 * The default itself is not tested through this helper for that exact reason —
 * `describe('DeckDetail rollup')` renders without it and asserts the closed
 * page directly.
 */
function renderUnfolded() {
  localStorage.setItem(
    'mtglab-99-collapsed:aasquier/goreclaw-stompy',
    JSON.stringify({ category: [], type: [], mv: [] }),
  )
  return renderDeck()
}

/**
 * Open the clearing. The wheel arrives folded on every visit now (punch list
 * 2026-08-18 item 2) — it is an amusement you go to, not a panel of the deck —
 * so anything testing the spin has to walk in first.
 */
function unfoldWheel() {
  fireEvent.click(screen.getByTitle('Unfold the Wheel of Fortune'))
}

/**
 * Drives an `ArmedButton` the way a person does: arm it, read the label it
 * changed to, then decide.
 *
 * Two `fireEvent.click` calls in a row land in the same millisecond, which is
 * a **double-click** — one gesture — and `ArmedButton` now refuses to treat
 * it as two decisions. That guard exists because the old behaviour was live:
 * one double-click on "Entomb 1 selected" moved a card to the graveyard of a
 * real deck on 2026-08-30. See `components/armedbutton.test.tsx`.
 *
 * So the clock moves between the two presses. It is mocked rather than waited
 * out because half a second per armed control is half a second this suite
 * would pay forever to prove nothing — the dwell itself is tested where it
 * lives, and what these tests are about is what happens *after* a confirm.
 */
function armThenConfirm(button: HTMLElement) {
  const at = Date.now()
  const clock = vi.spyOn(Date, 'now').mockReturnValue(at)
  fireEvent.click(button)
  clock.mockReturnValue(at + 1_000)
  fireEvent.click(button)
  clock.mockRestore()
}

function renderDeck() {
  return render(
    <MemoryRouter initialEntries={['/decks/aasquier/goreclaw-stompy']}>
      <Routes>
        <Route path="/decks/:owner/:slug" element={<DeckDetail />} />
      </Routes>
    </MemoryRouter>,
  )
}

/** Open the tab the shortlist lives on. */
function openValidation() {
  fireEvent.click(screen.getByRole('tab', { name: 'Validation' }))
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
  never: 'One rule holds at every setting: Claude never writes a card’s rationale on its own. On an import you can ask it to draft the ones you have not written, and every sentence it drafts is marked as Claude’s until you rewrite it.',
  modes: [{ name: 'rationale-interview', purpose: 'Asks about a slot.',
            tools: ['get_cards'], writes: [] }],
}

/** No dossier written yet — the state five of the six decks are in. */
const NO_DOSSIER = {
  answered_by: '', slug: 'goreclaw-stompy',
  commander: 'Goreclaw, Terror of Qal Sisma',
  dossier: {}, cached: false, generated_at: null,
}

/**
 * One that has been written, shaped as it comes back from the server: already
 * source-checked, with the counts of what was discarded on the way.
 */
const WRITTEN_DOSSIER = {
  answered_by: 'claude', slug: 'goreclaw-stompy',
  commander: 'Goreclaw, Terror of Qal Sisma',
  cached: false, generated_at: '2026-08-12T10:00:00+00:00',
  asked: true, reason: '', model: 'claude-sonnet-5',
  usage: { input_tokens: 800, output_tokens: 900 },
  never: 'This is Claude’s writing over cited pages. The card facts above it '
         + 'are the pool’s.',
  dossier: {
    who: { prose: 'A bear god of Qal Sisma.', source_ids: ['s1'] },
    archetype: { name: 'Mono-green stompy',
                 prose: 'Big creatures, cheaper.', source_ids: ['s1', 's2'] },
    competitors: [{
      name: 'Ghalta, Primal Hunger', prose: 'Bigger, and dumber about it.',
      source_ids: ['s2'], mana_cost: '{10}{G}{G}',
      type_line: 'Legendary Creature — Elder Dinosaur',
      image: 'https://cards.scryfall.io/normal/ghalta.jpg',
      art_crop: 'https://cards.scryfall.io/art_crop/ghalta.jpg',
      legal_commander: true, oracle_text: 'Trample',
    }],
    allies: { prose: 'The beasts of the frozen peaks follow where it hunts.',
              source_ids: ['s1'] },
    rivals: { prose: 'The lore pits the bear against the humans of Qal Sisma.',
              source_ids: ['s1'] },
    standing: { prose: 'A 2018 mythic that never left.', source_ids: ['s2'] },
    sources: [
      { id: 's1', title: 'Goreclaw | EDHREC', url: 'https://edhrec.com/g' },
      { id: 's2', title: 'Stompy primer', url: 'https://example.com/stompy' },
    ],
    sources_dropped: 0, competitors_dropped: 0, searched: 18,
  },
}

const PRINTINGS = {
  slug: 'goreclaw-stompy', commander: 'Goreclaw, Terror of Qal Sisma',
  selected: '',
  printings: [
    { id: 'p-blc', set_code: 'BLC', set_name: 'Bloomburrow Commander',
      collector_number: '212', rarity: 'rare', released_at: '2024-08-02',
      promo: false, image: 'https://cards.scryfall.io/normal/blc.jpg',
      art_crop: 'https://cards.scryfall.io/art_crop/blc.jpg',
      price_usd: 1.2, selected: false },
    { id: 'p-m19', set_code: 'M19', set_name: 'Core Set 2019',
      collector_number: '186', rarity: 'rare', released_at: '2018-07-13',
      promo: false, image: 'https://cards.scryfall.io/normal/m19.jpg',
      art_crop: 'https://cards.scryfall.io/art_crop/m19.jpg',
      price_usd: 2.4, selected: false },
  ],
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

const ARGUMENT = {
  answered_by: 'claude', mode: 'slot-argument',
  model: 'claude-sonnet-5', slug: 'goreclaw-stompy', card: 'Sol Ring',
  asked: true, reason: '', stance: STANCE,
  charges: [
    { claim: 'Six other cards already ramp for two or less.',
      ground: 'redundancy', fact: 'ramp: 7 against a target of 4-6.',
      strength: 'serious' },
  ],
  charges_dropped: 0,
  alternatives: [
    { name: 'Cultivator Colossus', mana_cost: '{4}{G}{G}{G}',
      type_line: 'Creature — Plant Elemental',
      oracle_text: 'Trample. When Cultivator Colossus enters…',
      color_identity: ['G'] },
  ],
  alternatives_dropped: {
    not_in_pool: [], banned: ['Primeval Titan'],
    off_colour: ['Ajani, Nacatl Pariah // Ajani, Nacatl Avenger'],
    already_in_deck: [], no_pool: [],
  },
  tool_calls: [], usage: { input_tokens: 10, output_tokens: 5 },
  never: 'This is the case against the card, and only that. A card that '
       + 'survives it still needs a rationale, and the rationale is yours to '
       + 'write.',
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
  vi.mocked(api.commander).mockReset().mockResolvedValue(DOSSIER)
  vi.mocked(api.swapCard).mockReset().mockResolvedValue({
    slug: 'goreclaw-stompy', swapped_out: 'Primeval Titan',
    swapped_in: 'Cultivator Colossus', why: 'because', ok: true,
    errors: [], warnings: [], stage: 'curated', total_cards: 99,
    needs_rationale: 0,
  })
  for (const fn of [api.addCard, api.entombCard, api.entombCards,
                    api.returnCard, api.exileCard, api.setCardField, api.setNote,
                    api.setDeckField]) {
    vi.mocked(fn).mockReset().mockResolvedValue(EDIT_RESULT)
  }
  // Answers with the whole deck rather than an `EditResult`: `shared` changes
  // who can see the deck and nothing the gate has an opinion about.
  vi.mocked(api.setShared).mockReset()
    .mockResolvedValue(DECK as unknown as Deck)
  // Installed and configured by default, so the interview panel renders its
  // button rather than its "not installed" note in most tests.
  vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(CLAUDE_STATUS)
  vi.mocked(api.interview).mockReset().mockResolvedValue(INTERVIEW)
  vi.mocked(api.argue).mockReset().mockResolvedValue(ARGUMENT)
  vi.mocked(api.argueDeck).mockReset()
  // No dossier stored by default — the ordinary state for five of the six
  // decks, and the one the collapsed panel has to handle without noise.
  vi.mocked(api.dossier).mockReset().mockResolvedValue(NO_DOSSIER)
  // Queued by default, so the ordinary path through these tests is the real
  // one: submit, poll, render. A job born `done` is the cache hit, and gets
  // its own test rather than being the default that hides the polling.
  vi.mocked(api.writeDossier).mockReset().mockResolvedValue(job(null, 'queued'))
  vi.mocked(followJob).mockReset().mockReturnValue({
    promise: Promise.resolve(job(WRITTEN_DOSSIER)),
    cancel: () => {},
  })
  vi.mocked(api.printings).mockReset().mockResolvedValue(PRINTINGS)
  vi.mocked(api.deckLog).mockReset().mockResolvedValue(DECK_LOG)
  // A run's id is parked here so a reload can reattach; without this a test
  // that leaves one behind makes the next one reattach to a job it never made.
  localStorage.clear()
})

afterEach(cleanup)

describe('DeckDetail validation tab', () => {
  it('does not fetch a shortlist until the tab is opened', async () => {
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    expect(api.suggestions).not.toHaveBeenCalled()

    openValidation()
    await waitFor(() => expect(api.suggestions).toHaveBeenCalledWith(REF))
  })

  it('fetches the shortlist once, not on every tab switch', async () => {
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    openValidation()
    await waitFor(() => expect(api.suggestions).toHaveBeenCalledTimes(1))

    fireEvent.click(screen.getByRole('tab', { name: 'Notes' }))
    openValidation()
    await waitFor(() => expect(screen.getByText('Cultivator Colossus')).toBeTruthy())
    expect(api.suggestions).toHaveBeenCalledTimes(1)
  })

  it('renders candidates under the error they belong to, with their reasons', async () => {
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    openValidation()

    const candidate = await screen.findByText('Cultivator Colossus')
    const block = candidate.closest('li')!
    expect(within(block).getByText(/same card type \(Creature\)/)).toBeTruthy()
    expect(within(block).getByText(/EDHREC rank 1,589/)).toBeTruthy()
  })

  it('says the shortlist is not a recommendation', async () => {
    // The whole design rests on this being a measurement the user overrules.
    // If the disclaimer ever quietly disappears, that is a change worth failing.
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    openValidation()
    await screen.findByText('Cultivator Colossus')
    expect(screen.getByText(/not a recommendation/)).toBeTruthy()
  })

  it('shows the error alone when there is no shortlist for it', async () => {
    vi.mocked(api.suggestions).mockResolvedValue({
      slug: 'goreclaw-stompy', pool_available: false, targets: [],
    })
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    openValidation()

    await screen.findByText(/not legal in Commander/)
    expect(screen.queryByText(/not a recommendation/)).toBeNull()
  })

  it('will not apply a swap until a rationale is written', async () => {
    // Rule 4 at the last place it can be enforced. A tool-written rationale is
    // the empty justification the rule exists to prevent, so the button stays
    // disabled rather than the app inventing one.
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
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
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    openValidation()
    fireEvent.click(await screen.findByRole('button', { name: 'Use this card' }))
    fireEvent.change(screen.getByRole('textbox'),
                     { target: { value: 'It ramps and it attacks.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply swap' }))

    await waitFor(() => expect(api.swapCard).toHaveBeenCalledWith(REF, {
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
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
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
    renderUnfolded()
    await screen.findByRole('tab', { name: 'Validation' })
    openValidation()
    const art = await screen.findByAltText('Cultivator Colossus')

    fireEvent.mouseEnter(art.parentElement!, { clientX: 10, clientY: 10 })
    const full = await screen.findAllByAltText('Cultivator Colossus')
    expect(full.length).toBe(2)
    expect(full.some((el) => el.getAttribute('src')?.includes('colossus-full'))).toBe(true)
  })

  // ------------------------------------------------------------- draft stage

  it('says nothing about drafts for a curated deck', async () => {
    renderUnfolded()
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
    renderUnfolded()
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
/**
 * The Wheel of Fortune (punch list 2026-08-15 item 9): server picks the
 * fate and the card, the client is theatre. The reveal waits for the wheel
 * to stop turning, so the test stops it by hand with a transitionend.
 */
describe('DeckDetail wheel of fortune', () => {
  it('spins, waits for the wheel to stop, then shows the card', async () => {
    vi.mocked(api.wheelSpin).mockResolvedValue({
      pool_available: true, symbol: 'cup', label: 'The Cup',
      meaning: 'The cup runneth over — a card that refills your hand.',
      seed: 7, answered_by: 'dice',
      caveat: 'The wheel is blind dice over the card pool. The rationale, '
            + 'if it earns one, is yours to write.',
      card: { name: 'Harmonize', mana_cost: '{2}{G}{G}', type_line: 'Sorcery',
              oracle_text: 'Draw three cards.', color_identity: ['G'],
              image: 'https://example.test/harmonize-full.jpg',
              art_crop: 'https://example.test/harmonize.jpg' },
    })
    renderUnfolded()
    await screen.findByText(DECK.name)
    unfoldWheel()
    fireEvent.click(screen.getByRole('button', { name: 'Spin the wheel' }))
    await waitFor(() => expect(vi.mocked(api.wheelSpin)).toHaveBeenCalled())

    // Before the wheel stops, no card; the suspense is the feature.
    expect(screen.queryByText('Harmonize')).toBeNull()
    fireEvent.transitionEnd(
      document.querySelector('.wheel-scene .wheel-disc') as Element)

    expect(await screen.findByText('Harmonize')).toBeTruthy()
    expect(screen.getByText('The Cup')).toBeTruthy()
    expect(screen.getByText(/blind dice/)).toBeTruthy()
    // Commandment 10 (sharpened 2026-08-17): no technology but Claude ever
    // renders — the seed rides the wire for tests and QA, never the page.
    expect(screen.queryByText(/Seed 7/)).toBeNull()
    expect(screen.queryByText(/Python/)).toBeNull()
  })

  it('reports an empty fate honestly', async () => {
    vi.mocked(api.wheelSpin).mockResolvedValue({
      pool_available: true, symbol: 'skull', label: 'The Skull',
      meaning: 'The skull grins.', seed: 3, answered_by: 'dice',
      caveat: 'Deterministic.', card: null,
      reason: 'The pool holds no legal card in these colours that answers '
            + 'to this fate.',
    })
    renderUnfolded()
    await screen.findByText(DECK.name)
    unfoldWheel()
    fireEvent.click(screen.getByRole('button', { name: 'Spin the wheel' }))
    await waitFor(() => expect(vi.mocked(api.wheelSpin)).toHaveBeenCalled())
    fireEvent.transitionEnd(
      document.querySelector('.wheel-scene .wheel-disc') as Element)
    expect(await screen.findByText(/no legal card in these colours/)).toBeTruthy()
  })

  it('credits the painting', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    unfoldWheel()
    expect(screen.getByText(/Daniel Gelon, Limited Edition Alpha/)).toBeTruthy()
  })

  it('arrives folded, and does not remember being opened', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    // Folded, the wheel is the card it came from: no clearing, no controls,
    // and — the point of the fold — no rain, crickets or storm timers.
    expect(screen.queryByRole('button', { name: 'Spin the wheel' })).toBeNull()

    unfoldWheel()
    expect(screen.getByRole('button', { name: 'Spin the wheel' })).toBeTruthy()

    // Leaving and coming back shuts it again. It used to persist, which is
    // the right shape for a preference and the wrong one for an amusement.
    cleanup()
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: 'Spin the wheel' })).toBeNull()
  })
})

/**
 * Where things sit on the list tab, which is a ruling and not a detail.
 *
 * Aaron, 2026-08-30: the tokens shelf moves above the two toys, and the toys
 * stay at the very bottom. The rule generalises past this one move — the Wheel
 * and the deal are the dessert trolley, so anything *about the deck* comes
 * before them — and the generalisation is the half worth pinning, because the
 * way this breaks is nobody re-reading a comment while appending a section to
 * the end of a 2,000-line render.
 *
 * So the assertion is the invariant rather than the pair: `.deck-toys` is the
 * **last** child of the list pane. A test that only checked "tokens before
 * toys" would stay green through exactly the mistake this guards.
 */
describe('DeckDetail list order', () => {
  it('keeps the toys at the very bottom, with the tokens shelf above them',
     async () => {
       renderUnfolded()
       await screen.findByText(DECK.name)

       const tokens = screen.getByRole('button', { name: /Tokens/ })
       const toys = document.querySelector('.deck-toys')
       expect(toys).toBeTruthy()
       // Both toys are in there, folded — this is the row, not a stray class.
       expect(within(toys as HTMLElement)
         .getByTitle('Unfold the Wheel of Fortune')).toBeTruthy()

       // The shelf comes first...
       expect(tokens.compareDocumentPosition(toys as Node)
              & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
       // ...and nothing at all comes after the toys.
       expect((toys as Element).parentElement?.lastElementChild).toBe(toys)
     })
})

/**
 * The stats tab's punch-list additions (2026-08-15 item 6): opening-hand
 * hypergeometrics, the type breakdown, and the Game Changers count — which
 * had been in the payload since the printed-stats branch with no screen.
 */
describe('DeckDetail stats tab', () => {
  async function openStats() {
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('tab', { name: 'Stats' }))
  }

  it('shows the opening-hand odds with their which-system caveat', async () => {
    await openStats()
    expect(screen.getByText('Opening hand odds')).toBeTruthy()
    expect(screen.getByText('73.0%')).toBeTruthy()
    // The caveat is the ADR 14 line: these are draw odds, castability is
    // the simulator's question.
    expect(screen.getByText(/no simulation, no opponent/i)).toBeTruthy()
  })

  it('renders the Game Changers verdict instead of hiding the count', async () => {
    await openStats()
    expect(screen.getByText('Game Changers')).toBeTruthy()
    expect(screen.getByText(/0 of 3 allowed/)).toBeTruthy()
  })

  it('says unknown when nobody could look, never zero', async () => {
    vi.mocked(api.stats).mockResolvedValue({
      ...STATS,
      game_changers: { cards: [], count: 0, allowed: null, bracket: null,
                       verdict: 'unknown' },
    } as DeckStats)
    await openStats()
    expect(screen.getByText('not checked')).toBeTruthy()
    expect(screen.getByText(/absent count is not a count of zero/i)).toBeTruthy()
  })

  it('shows the type breakdown', async () => {
    await openStats()
    expect(screen.getByText('Card types')).toBeTruthy()
    expect(screen.getByText('Creature')).toBeTruthy()
  })
})

describe('DeckDetail rationale editor', () => {
  /** The row for one card, anchored on its name text. The per-row buttons
   *  became the action bar (punch list item 9), so the old anchor — the
   *  remove button's title — no longer exists on a row. */
  function rowFor(card: string) {
    const el = screen.getAllByText(card).find((n) => n.closest('li'))
    return el!.closest('li')!
  }

  /** The new interaction: arm the action on the bar, then pick the card. */
  async function openEditorFor(card: string) {
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Write why' }))
    fireEvent.click(screen.getByRole('button', { name: `Write why: ${card}` }))
    return rowFor(card)
  }

  it('stays armed after a pick, for the next card', async () => {
    // Punch list 2026-08-15 item 3: an armed action is a mode, not a single
    // shot. Writing five rationales is five clicks, not five trips to the bar.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    await openEditorFor('Sol Ring')
    const next = screen.getByRole('button', { name: 'Write why: Forest' })
    fireEvent.click(next)
    expect(within(rowFor('Forest')).getByRole('textbox')).toBeTruthy()
  })

  it('a double-click acts and puts the bar away', async () => {
    // The two real clicks inside a double-click fire first and toggle the
    // editor open then closed; the dblclick must land it *open* and disarm.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Write why' }))
    const row = screen.getByRole('button', { name: 'Write why: Sol Ring' })
    fireEvent.click(row)
    fireEvent.click(row)
    fireEvent.doubleClick(row)
    expect(within(rowFor('Sol Ring')).getByRole('textbox')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Write why: /
    })).toBeNull()
  })

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
      REF, 'Sol Ring', 'why', 'Two mana for one.'))
  })

  it('loads an existing rationale for editing rather than starting blank', async () => {
    const row = await openEditorFor('Primeval Titan')
    expect((within(row).getByRole('textbox') as HTMLTextAreaElement).value)
      .toBe('Ramp and threat in one card.')
  })

  /**
   * The rationale interview, in the same column as the pool text.
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
      REF, { card: 'Sol Ring' }))
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

  it('offers nothing to press when the server has no key', async () => {
    // Two different answers, kept apart: no key, and nothing asked yet. A
    // server without Claude at all is the third and cannot happen — the
    // client is linked into the binary — so this asserts the state the door
    // can actually report.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    vi.mocked(api.claudeStatus).mockResolvedValue({
      ...CLAUDE_STATUS, configured: false,
    })
    const row = await openEditorFor('Sol Ring')

    await within(row).findByText(/no key to call with/)
    expect(within(row).queryByRole('button', { name: /ask for questions/i }))
      .toBeNull()
  })

  it('shows the card as the pool has it, beside the box', async () => {
    // Rule 1 made useful: you argue about the card against what it says, not
    // against what you remember it saying. Asserted on the type line because
    // `ManaText` splits the oracle text into symbol elements, so a plain
    // substring match on it would be testing the renderer instead.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    const row = await openEditorFor('Sol Ring')
    expect(within(row).getByText('Artifact')).toBeTruthy()
    expect(within(row).queryByText(/No card text in the pool/)).toBeNull()
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
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(rowFor('Primeval Titan')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /show the 2 that need one/i }))
    await waitFor(() => expect(
      screen.queryAllByText('Primeval Titan').filter((el) => el.closest('li')))
      .toHaveLength(0))
    expect(rowFor('Sol Ring')).toBeTruthy()
    expect(rowFor('Forest')).toBeTruthy()
  })

  it('entombs on the second click, never the first', async () => {
    // ADR 27's confirmation structure, through the action bar now: arming
    // Entomb and picking a card only marks the row — naming the consequence
    // — and one stray click mutates nothing. This is the test for the bug
    // that killed a handful of Gyome's cards in one afternoon.
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Entomb' }))
    const target = screen.getByRole('button', { name: 'Entomb: Primeval Titan' })
    fireEvent.click(target)

    expect(api.entombCard).not.toHaveBeenCalled()
    expect(screen.getByText(/click again to entomb/i)).toBeTruthy()

    fireEvent.click(target)
    await waitFor(() => expect(api.entombCard)
      .toHaveBeenCalledWith(REF, 'Primeval Titan'))
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
  })

  it('reports a refused entombment rather than silently doing nothing', async () => {
    vi.mocked(api.entombCard).mockRejectedValue(new Error('this deck is read-only'))
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Entomb' }))
    const target = screen.getByRole('button', { name: 'Entomb: Primeval Titan' })
    fireEvent.click(target)
    fireEvent.click(target)

    await screen.findByText(/read-only/)
  })
})

/**
 * The graveyard (ADR 27): entombed cards are deck state with two ways out,
 * and the bulk sweep is a mode you enter rather than checkboxes always there.
 */
describe('DeckDetail graveyard', () => {
  const BURIED = {
    ...DECK,
    graveyard: [{
      name: 'Nissa, Who Shakes the World', category: 'ramp',
      why: 'Doubles every Forest.', qty: 1, known: true,
      mana_cost: '{3}{G}{G}', color_identity: ['G'],
    }],
  } as Deck

  it('lists the buried with their rationale, and both ways out', async () => {
    vi.mocked(api.deck).mockResolvedValue(BURIED)
    renderUnfolded()
    await screen.findByText(/graveyard/i)
    // Twice: once on the hover target, once as the row's name — same as a
    // living card's row.
    expect(screen.getAllByText('Nissa, Who Shakes the World').length)
      .toBeGreaterThan(0)
    // The why rides into the graveyard — it is what a return restores.
    expect(screen.getByText(/doubles every forest/i)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Return' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Exile' })).toBeTruthy()
  })

  it('returns a card in one click — restoring is not destructive', async () => {
    vi.mocked(api.deck).mockResolvedValue(BURIED)
    renderUnfolded()
    await screen.findByText(/graveyard/i)
    fireEvent.click(screen.getByRole('button', { name: 'Return' }))
    await waitFor(() => expect(api.returnCard)
      .toHaveBeenCalledWith(REF, 'Nissa, Who Shakes the World'))
  })

  it('exile arms first, like every permanent thing here', async () => {
    vi.mocked(api.deck).mockResolvedValue(BURIED)
    renderUnfolded()
    await screen.findByText(/graveyard/i)
    const button = screen.getByRole('button', { name: 'Exile' })
    // Armed and confirmed a second apart, because the two presses have to be
    // two decisions — see `armThenConfirm`. Written out rather than using the
    // helper so the arm can be caught in the act of having done nothing.
    const at = Date.now()
    const clock = vi.spyOn(Date, 'now').mockReturnValue(at)
    fireEvent.click(button)
    expect(api.exileCard).not.toHaveBeenCalled()
    clock.mockReturnValue(at + 1_000)
    fireEvent.click(button)
    clock.mockRestore()
    await waitFor(() => expect(api.exileCard)
      .toHaveBeenCalledWith(REF, 'Nissa, Who Shakes the World'))
  })

  it('offers no graveyard section while it is empty', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByText(/graveyard/i)).toBeNull()
  })

  it('sweeps the chosen cards in one all-or-nothing request', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    // Enter the mode, tick the card, then the armed two-step.
    fireEvent.click(screen.getByRole('button', { name: /bulk entomb/i }))
    fireEvent.click(screen.getByRole('checkbox',
      { name: /choose primeval titan/i }))
    const sweep = screen.getByRole('button', { name: /entomb 1 selected/i })
    const at = Date.now()
    const clock = vi.spyOn(Date, 'now').mockReturnValue(at)
    fireEvent.click(sweep)
    expect(api.entombCards).not.toHaveBeenCalled()
    clock.mockReturnValue(at + 1_000)
    fireEvent.click(sweep)
    clock.mockRestore()
    await waitFor(() => expect(api.entombCards)
      .toHaveBeenCalledWith(REF, ['Primeval Titan']))
    // Wait out the send-off and the re-read, so the deferred refresh cannot
    // leak into whichever test runs next and pad its call counts.
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2),
                  { timeout: 2000 })
  })

  it('keeps the checkboxes out of the way until the mode is entered', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('checkbox', { name: /choose/i })).toBeNull()
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
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: /promote/i })).toBeNull()
    expect(screen.getByText(/2 of 3 cards still need/)).toBeTruthy()
  })

  it('offers promotion once nothing is outstanding', async () => {
    vi.mocked(api.deck).mockResolvedValue(FINISHED)
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.getByText(/every card carries a rationale/i)).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: /promote to curated/i }))
    await waitFor(() => expect(api.setDeckField)
      .toHaveBeenCalledWith(REF, 'stage', 'curated'))
    // And the page re-reads, so the banner goes away on its own.
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
  })

  it('shows the server refusal rather than assuming it worked', async () => {
    // The UI hides the button when it knows better, but the rule lives in the
    // gate. If the two ever disagree, the gate wins and the user is told.
    vi.mocked(api.deck).mockResolvedValue(FINISHED)
    vi.mocked(api.setDeckField).mockRejectedValue(
      new Error('1 card(s) still have no `why` (Sol Ring)'))
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: /promote to curated/i }))

    await screen.findByText(/still have no `why`/)
  })

  it('shows no draft banner at all on a curated deck', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: /promote/i })).toBeNull()
    expect(screen.queryByText(/still need/)).toBeNull()
  })
})

/**
 * The hero.
 *
 * `art_crop` is 626x457 and the band it used to fill alone was 1200/260 — so
 * the page kept about a third of the commander's painting and took that third
 * out of the middle, which on a card drawn head-up is the part without the
 * head. No band ratio fixes that: anything square enough to show the painting
 * is 600px tall on a wide screen. The whole card, uncropped, is the fix, and
 * these pin that it is the card and not another crop.
 */
describe('DeckDetail hero', () => {
  it('shows the commander as a whole card, not a crop', async () => {
    vi.mocked(api.deck).mockResolvedValue({
      ...DECK,
      commander_card: {
        name: 'Goreclaw, Terror of Qal Sisma',
        category: 'commander', why: '', qty: 1, known: true,
        image: 'https://example.test/goreclaw-full.jpg',
        art_crop: 'https://example.test/goreclaw-crop.jpg',
      },
    } as unknown as Deck)
    renderUnfolded()

    const card = await screen.findByAltText('Goreclaw, Terror of Qal Sisma')
    expect(card.getAttribute('src')).toBe('https://example.test/goreclaw-full.jpg')
  })

  it('renders without a commander card when the pool has no art', async () => {
    // A fresh clone has no card pool, so `commander_card` is null and the hero
    // has nothing to lift out of the band. It must still render the deck.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.queryByAltText('Goreclaw, Terror of Qal Sisma')).toBeNull()
  })

  /** The credit line, whose two halves must name the same printing.
   *
   * They did not until 2026-08-19: the set came off the deck's chosen
   * printing and the painter off the pool's oracle row, so the Trostani deck
   * credited Sidharth Chaturvedi for Chippy's painting in one sentence. The
   * server is where that was fixed; what belongs here is that the line
   * survives a painter the pool cannot name, because the fix's own
   * degradation must not swallow the printing too.
   */
  const withCommander = (extra: Record<string, unknown>) =>
    vi.mocked(api.deck).mockResolvedValue({
      ...DECK,
      commander_card: {
        name: 'Goreclaw, Terror of Qal Sisma',
        category: 'commander', why: '', qty: 1, known: true,
        image: 'https://example.test/goreclaw-full.jpg',
        ...extra,
      },
    } as unknown as Deck)

  it('credits the painter and the printing together', async () => {
    withCommander({
      artist: 'Chris Rahn',
      printing: { set_name: 'Multiverse Legends', set_code: 'MUL' },
    })
    renderUnfolded()
    expect(await screen.findByText(/Art by Chris Rahn · Multiverse Legends/))
      .toBeTruthy()
  })

  it('names the printing even when the painter is unknown', async () => {
    // An un-refreshed pool answers NULL for every printing's artist. The
    // honest reading is a missing credit, never the oracle row's painter —
    // but the deck did pick a printing, and saying so is what stops the
    // degradation from looking like no choice was ever made.
    withCommander({
      artist: null,
      printing: { set_name: 'Multiverse Legends', set_code: 'MUL' },
    })
    renderUnfolded()
    expect(await screen.findByText('Multiverse Legends')).toBeTruthy()
    expect(screen.queryByText(/Art by/)).toBeNull()
  })
})

/**
 * The pool facts under the header.
 *
 * Every number in this panel was counted by `service.commander_dossier` over
 * the pool, and these tests exist to keep it that way: the panel renders
 * what it was handed and computes nothing, so a wrong figure is always a bug
 * in a query somebody can re-run.
 */
describe('DeckDetail commander facts', () => {
  it('renders the subtype counts it was given', async () => {
    renderUnfolded()
    await screen.findByText('Bear')
    expect(screen.getByText(/26 legendary, 78 in all/)).toBeTruthy()
  })

  it('shows the first printing as a year and a set', async () => {
    renderUnfolded()
    await screen.findByText('2018')
    expect(screen.getByText(/in Core Set 2019/)).toBeTruthy()
    expect(screen.getByText(/12 printings/)).toBeTruthy()
  })

  it('offers the other cards carrying the name', async () => {
    renderUnfolded()
    expect(await screen.findByText('Surrak and Goreclaw')).toBeTruthy()
  })

  it('renders nothing at all when the pool had nothing to say', async () => {
    // A fresh clone. An empty "About this commander" heading is worse than
    // no heading, and the deck page must still work.
    vi.mocked(api.commander).mockResolvedValue({
      slug: 'goreclaw-stompy', card: null, supertypes: [], subtypes: [],
      other_cards: [], printings: { count: 0, first_released: null, first_set: null },
    })
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.queryByText(/about this commander/i)).toBeNull()
  })

  it('does not take the deck down when the dossier request fails', async () => {
    vi.mocked(api.commander).mockRejectedValue(new Error('boom'))
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.queryByText(/about this commander/i)).toBeNull()
  })
})

/**
 * The commander dossier panel (ADR 19).
 *
 * The ADR's acceptance criterion for the UI is that the page shows the seams:
 * the counted strip stays separate, Claude's prose is labelled as Claude's,
 * and a claim from the web keeps its link. Those are the tests here — they are
 * not cosmetic assertions, they are the visible half of a rule the server
 * enforces structurally.
 */
describe('DeckDetail commander dossier', () => {
  it('is collapsed by default so the page does not get busy', async () => {
    renderUnfolded()
    expect(await screen.findByText(/who is goreclaw\?/i)).toBeTruthy()
    // The heading is there; the prose is not, until asked for.
    expect(screen.queryByText(/bear god of qal sisma/i)).toBeNull()
  })

  it('says whose writing it is without being opened', async () => {
    // Somebody who never expands it should still know what is inside and who
    // wrote it. The label is outside the collapsed panel for that reason.
    renderUnfolded()
    expect(await screen.findByText(/claude, with sources/i)).toBeTruthy()
  })

  it('costs nothing to open when nothing is stored', async () => {
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    await screen.findByText(/nothing written yet/i)
    // The free GET may be called; the paid POST must not be.
    expect(vi.mocked(api.writeDossier)).not.toHaveBeenCalled()
  })

  it('writes one on request and shows its prose', async () => {
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText(/bear god of qal sisma/i)).toBeTruthy()
    // The archetype's own name, not the deck's — which happens to contain the
    // same words, so this asserts on the exact casing the payload carries.
    expect(screen.getByText('Mono-green stompy')).toBeTruthy()
    expect(screen.getByText(/big creatures, cheaper/i)).toBeTruthy()
  })

  it('keeps every web claim next to a real link', async () => {
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    const source = await screen.findByText('Goreclaw | EDHREC')
    expect(source.getAttribute('href')).toBe('https://edhrec.com/g')
    // And it opens away from the app rather than navigating out of it.
    expect(source.getAttribute('rel')).toContain('noopener')
  })

  it('renders a competitor as a real card with its cost', async () => {
    // Competitors survived a card pool lookup server-side, so the name and
    // cost here are the pool's. A reader who doubts the sentence can hover
    // the card.
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText('Ghalta, Primal Hunger')).toBeTruthy()
  })

  it('renders the story allies and rivals as their own cited passages', async () => {
    // The story's allies and rivals are prose, not cards (a plot line is not
    // a pool row), labelled apart from the competitors so the two kinds of
    // rival never blur again — and the friends get a section too.
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText(/allies in the story/i)).toBeTruthy()
    expect(screen.getByText(/beasts of the frozen peaks/i)).toBeTruthy()
    expect(screen.getByText(/rivals in the story/i)).toBeTruthy()
    expect(screen.getByText(/pits the bear against the humans/i)).toBeTruthy()
  })

  it('shows what was discarded rather than hiding it', async () => {
    // A number that climbs is a prompt inventing citations. Nobody checks a
    // number they cannot see.
    vi.mocked(followJob).mockReturnValue({
      promise: Promise.resolve(job({
        ...WRITTEN_DOSSIER,
        dossier: { ...WRITTEN_DOSSIER.dossier, sources_dropped: 2, competitors_dropped: 1 },
      })),
      cancel: () => {},
    })
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText(/discarded before you saw it/i)).toBeTruthy()
  })

  it('offers no button at all when the stance is off', async () => {
    // ADR 15: off means no calls. A control that exists and refuses is a worse
    // answer than one that is honestly absent.
    vi.mocked(api.claudeStatus).mockResolvedValue({
      ...CLAUDE_STATUS,
      // `map` and not `[{ ...axes[0] }, ...slice(1)]`: same result, and it
      // says "turn the first axis off" without indexing a list whose first
      // element the checker has to take on trust.
      stance: { ...STANCE, axes: STANCE.axes.map(
        (a, i) => (i === 0 ? { ...a, level: 'off' } : a)) },
    })
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    expect(await screen.findByText(/claude is switched off here/i)).toBeTruthy()
    expect(screen.queryByText(/write the dossier/i)).toBeNull()
  })

  it('reports a refusal rather than rendering an unsourced dossier', async () => {
    // The server refuses when no cited page survived checking. The UI must
    // show that refusal, not an empty panel that looks like nothing happened.
    vi.mocked(followJob).mockReturnValue({
      promise: Promise.resolve(job({
        ...NO_DOSSIER,
        reason: 'No source survived checking, so there is nothing to stand '
                + 'behind the claims.',
      })),
      cancel: () => {},
    })
    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText(/no source survived checking/i)).toBeTruthy()
  })

  it('parks the run id while it works, and clears it once it settles', async () => {
    // The failure this replaced: a 236-second POST died in transit, the page
    // showed Safari's `Load failed`, and the finished dossier sat in the
    // server's cache with nothing left pointing at it.
    const seen: { at: string | null } = { at: null }
    vi.mocked(followJob).mockImplementation(() => {
      seen.at = localStorage.getItem(JOB_KEY)
      return { promise: Promise.resolve(job(WRITTEN_DOSSIER)), cancel: () => {} }
    })

    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText(/bear god of qal sisma/i)).toBeTruthy()

    expect(seen.at).toBe('job-dossier')
    // And gone once it landed, so a later visit does not chase a run that has
    // already finished and been evicted.
    await waitFor(() => expect(
      localStorage.getItem(JOB_KEY)).toBeNull())
  })

  it('reattaches to a run already in flight rather than paying twice', async () => {
    // The reattach path: this tab is a reload, arriving with the id in storage
    // and no memory of having asked. It must follow that run and must not
    // submit a second one.
    localStorage.setItem(JOB_KEY, 'job-dossier')

    renderUnfolded()
    // No click anywhere: it finds the id on mount, follows it, and opens
    // itself when it settles — a run that finishes into a collapsed panel has
    // produced nothing anybody can see.
    expect(await screen.findByText(/bear god of qal sisma/i)).toBeTruthy()
    expect(vi.mocked(api.writeDossier)).not.toHaveBeenCalled()
    // Two seconds, not the 400ms default: this runs for minutes, and polling
    // four times a second would be six hundred requests to watch one job.
    expect(vi.mocked(followJob)).toHaveBeenCalledWith(
      'job-dossier', expect.any(Function), 2000)
  })

  it('renders a stored dossier without polling for it at all', async () => {
    // A job born finished. ADR 19 caches on the commander's `oracle_id`, so a
    // hit is the answer rather than a substitute for one, and there is nothing
    // to watch.
    vi.mocked(api.writeDossier).mockResolvedValue(job(WRITTEN_DOSSIER))

    renderUnfolded()
    fireEvent.click(await screen.findByText(/who is goreclaw\?/i))
    fireEvent.click(await screen.findByText(/write the dossier/i))
    expect(await screen.findByText(/bear god of qal sisma/i)).toBeTruthy()
    expect(vi.mocked(followJob)).not.toHaveBeenCalled()
  })

  it('keeps the counted strip and the written prose apart', async () => {
    // The whole point of ADR 19's UI half: the pool's counted facts and
    // Claude's prose are adjacent, never merged into one voice.
    renderUnfolded()
    expect(await screen.findByText(/about this commander/i)).toBeTruthy()
    expect(await screen.findByText(/claude, with sources/i)).toBeTruthy()
  })
})

describe('DeckDetail art picker', () => {
  it('does not fetch printings until it is opened', async () => {
    // Goreclaw has twelve and most visits never open this.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(vi.mocked(api.printings)).not.toHaveBeenCalled()
  })

  it('lists the printings when opened', async () => {
    renderUnfolded()
    fireEvent.click(await screen.findByText(/change art/i))
    expect(await screen.findByText('BLC')).toBeTruthy()
    expect(screen.getByText('M19')).toBeTruthy()
  })

  it('says the choice lives in the deck file', async () => {
    // It is a deck property, not a viewer preference, and the copy has to say
    // so — otherwise it reads as a per-person display setting.
    renderUnfolded()
    fireEvent.click(await screen.findByText(/change art/i))
    expect(await screen.findByText(/travels with the deck/i)).toBeTruthy()
  })

  it('writes the pick through the ordinary deck-field edit', async () => {
    renderUnfolded()
    fireEvent.click(await screen.findByText(/change art/i))
    fireEvent.click(await screen.findByTitle(/Bloomburrow Commander/))
    await waitFor(() => {
      expect(vi.mocked(api.setDeckField)).toHaveBeenCalledWith(
        REF, 'commander_art', 'p-blc')
    })
  })

  it('a double-click swaps and closes the picker in one gesture', async () => {
    // Punch list 2026-08-15 item 3. The two real clicks inside it schedule
    // and reschedule the single-click save; the dblclick cancels the pending
    // beat, applies once, and folds the picker away.
    renderUnfolded()
    fireEvent.click(await screen.findByText(/change art/i))
    const tile = await screen.findByTitle(/Bloomburrow Commander/)
    fireEvent.click(tile)
    fireEvent.click(tile)
    fireEvent.doubleClick(tile)
    await waitFor(() => {
      expect(vi.mocked(api.setDeckField)).toHaveBeenCalledWith(
        REF, 'commander_art', 'p-blc')
    })
    expect(vi.mocked(api.setDeckField)).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(screen.queryByText(/travels with the deck/i)).toBeNull()
    })
  })

  it('clicking the printing already showing clears it back to the default', async () => {
    // How somebody gets back to the default without having to know which one
    // it was.
    vi.mocked(api.printings).mockResolvedValue({
      ...PRINTINGS, selected: 'p-blc',
      printings: PRINTINGS.printings.map((p) => ({
        ...p, selected: p.id === 'p-blc',
      })),
    })
    renderUnfolded()
    fireEvent.click(await screen.findByText(/change art/i))
    fireEvent.click(await screen.findByTitle(/Bloomburrow Commander/))
    await waitFor(() => {
      expect(vi.mocked(api.setDeckField)).toHaveBeenCalledWith(
        REF, 'commander_art', '')
    })
  })

  it('surfaces a refused pick instead of silently doing nothing', async () => {
    vi.mocked(api.setDeckField).mockRejectedValue(
      new Error('is not a printing of Goreclaw, Terror of Qal Sisma'))
    renderUnfolded()
    fireEvent.click(await screen.findByText(/change art/i))
    fireEvent.click(await screen.findByTitle(/Bloomburrow Commander/))
    expect(await screen.findByText(/is not a printing of/i)).toBeTruthy()
  })
})

/**
 * Alternate art for the 99 (punch list 2026-08-15 item 8): the commander's
 * picker, one card down. Reached through the action bar, written through the
 * ordinary card-field edit, pool-checked server-side.
 */
describe('DeckDetail card art', () => {
  it('opens a picker for any card and writes through the card field', async () => {
    vi.mocked(api.printings).mockResolvedValue(PRINTINGS as never)
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Card art' }))
    fireEvent.click(screen.getByRole('button', { name: 'Card art: Primeval Titan' }))

    await screen.findByText(/art for primeval titan/i)
    expect(api.printings).toHaveBeenCalledWith(REF, 'Primeval Titan')

    fireEvent.click(await screen.findByTitle(/Bloomburrow Commander/))
    await waitFor(() => expect(api.setCardField).toHaveBeenCalledWith(
      REF, 'Primeval Titan', 'art', 'p-blc'))
  })

  it('marks a card wearing a chosen printing', async () => {
    vi.mocked(api.deck).mockResolvedValue({
      ...DECK,
      cards: [{ ...DECK.cards[0]!, art: 'p-m19' }],
    } as unknown as Deck)
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.getByText(/chosen art/i)).toBeTruthy()
  })
})

/**
 * The complaint that moved this whole branch to the front of the queue.
 *
 * The rationale interview has worked since ADR 15 and the deck page never
 * said so — it was reachable only by opening the editor and finding a second
 * button inside it. These pin the fix: the page announces it, and the control
 * that announces it does the thing rather than revealing another control that
 * does the thing.
 */
describe('DeckDetail rationale interview discoverability', () => {
  it('says the interview exists', async () => {
    renderUnfolded()
    expect(await screen.findByText(/stuck on a/i)).toBeTruthy()
  })

  it('says what the rule is in the same breath', async () => {
    // The first question anybody asks is whether it writes the answer, and
    // rule 4 is easier to keep when the UI states it unprompted.
    renderUnfolded()
    expect(await screen.findByText(/no setting lets it write the rationale for you/i))
      .toBeTruthy()
  })

  it('offers the interview from the action bar', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude' }))
    expect(screen.getByRole('button', { name: 'Ask Claude: Primeval Titan' }))
      .toBeTruthy()
  })

  it('asks straight away rather than revealing a second button', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude' }))
    fireEvent.click(screen.getByRole('button', { name: 'Ask Claude: Primeval Titan' }))

    await waitFor(() => expect(api.interview).toHaveBeenCalledWith(
      REF, { card: 'Primeval Titan' }))
  })

  it('still opens the editor without spending anything', async () => {
    // "Write why" must stay free. A page where opening a text box costs a call
    // is a page nobody opens a text box on.
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Write why' }))
    fireEvent.click(screen.getByRole('button', { name: 'Write why: Primeval Titan' }))

    await screen.findByRole('button', { name: /save rationale/i })
    expect(api.interview).not.toHaveBeenCalled()
  })

  it('is honestly absent when the surface is switched off', async () => {
    // ADR 15: a control that appears and then refuses is worse than one that
    // is not there. `off` means no calls, so it means no button.
    const off = { ...STANCE, preset: 'off', allows_calls: false,
                  axes: [{ ...STANCE.axes[0], level: 'off' },
                         STANCE.axes[1], STANCE.axes[2]] }
    vi.mocked(api.claudeStatus).mockResolvedValue(
      { ...CLAUDE_STATUS, stance: off } as never)
    renderUnfolded()
    await screen.findByText(DECK.name)

    expect(screen.queryByRole('button', { name: /ask claude/i })).toBeNull()
    expect(screen.queryByText(/stuck on a/i)).toBeNull()
  })

  it('is honestly absent when there is no key', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(
      { ...CLAUDE_STATUS, configured: false } as never)
    renderUnfolded()
    await screen.findByText(DECK.name)

    expect(screen.queryByRole('button', { name: /ask claude/i })).toBeNull()
  })
})

/**
 * A deck somebody may read but not change.
 *
 * The mirror of the server's own write-gate tests, and it exists for the same
 * reason: before the write gate landed, every account that could see the
 * curated six could also edit and delete them, and no test on either side of
 * the wire ever logged in as a second person to find out.
 *
 * Hiding a control is a courtesy and never the defence — the server refuses
 * independently with a 403. What these pin is that the app does not offer
 * somebody a button that cannot work.
 */
describe('DeckDetail for a reader', () => {
  beforeEach(() => {
    vi.mocked(api.deck).mockReset()
      .mockResolvedValue({ ...DECK, writable: false } as unknown as Deck)
  })

  it('shows the deck', async () => {
    // The control. If this fails the gate has gone too far: the library is
    // meant to be readable by everyone, and only writing was taken away.
    renderUnfolded()
    expect(await screen.findByText('Goreclaw — Mono-Green Stompy')).toBeTruthy()
  })

  it('offers no way to edit or remove a card', async () => {
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    // The action bar is the write surface now, so its absence is the whole
    // assertion: no bar, no Write why, no Entomb, nothing to arm.
    expect(screen.queryByText('Card actions')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Write why' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Entomb' })).toBeNull()
  })

  it('does not offer the rationale interview', async () => {
    // Gated with the writes although it only asks questions: its whole point
    // is helping somebody write a `why` they could not save.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.queryByText('Ask Claude')).toBeNull()
  })

  it('still shows the notes it cannot edit', async () => {
    // The distinction worth pinning: a note is the deck's *thinking*, so it
    // renders for a reader. Only the Edit control goes.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    fireEvent.click(screen.getByRole('tab', { name: 'Notes' }))
    expect(screen.queryByText('Edit')).toBeNull()
  })

  it('still shows the swap shortlist, without the swap', async () => {
    // Analysis stays; acting on it goes. Reading why a card is a candidate is
    // worth having without the power to make the change.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    openValidation()
    expect(screen.queryByText('Use this card')).toBeNull()
  })
})

/**
 * Sharing, and what the page says about whose deck this is (ADR 22).
 *
 * The toggle is the only control in the app that changes who can see
 * something, and it is the only way a deck in the SQL tier ever becomes
 * visible to anybody — those are created private, so without this the browse
 * tab is permanently empty and the sharing half of ADR 22 is unreachable.
 */
describe('DeckDetail sharing', () => {
  it('offers to share a private deck, and says who can see it now', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, shared: false } as unknown as Deck)
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.getByRole('button', { name: 'Share this deck' })).toBeTruthy()
    expect(screen.getByText('Only you can see it.')).toBeTruthy()
  })

  it('offers to take a shared deck back, and says who can see it now', async () => {
    // The label is what the click will *do*, not what the deck currently is —
    // a button labelled with a state is the one people press expecting to
    // select it.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.getByRole('button', { name: 'Make private' })).toBeTruthy()
    expect(screen.getByText('Anyone signed in here can read it.')).toBeTruthy()
  })

  it('sends the opposite of what the deck is, at the deck\'s own address', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, shared: false } as unknown as Deck)
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    fireEvent.click(screen.getByRole('button', { name: 'Share this deck' }))
    await waitFor(() => expect(api.setShared).toHaveBeenCalledWith(REF, true))
    // And re-reads, so the label and the sentence under it both move.
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
  })

  it('reports a refusal rather than looking as though it worked', async () => {
    vi.mocked(api.setShared).mockRejectedValue(
      new Error('goreclaw-stompy is not yours to change'))
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    fireEvent.click(screen.getByRole('button', { name: 'Make private' }))
    expect(await screen.findByText(/not yours to change/)).toBeTruthy()
  })

  it('is absent for a reader, who is told whose deck this is instead', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, writable: false } as unknown as Deck)
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.queryByRole('button', { name: /share this deck|make private/i }))
      .toBeNull()
    expect(screen.getByText(/you can read this deck, not change it/i)).toBeTruthy()
    expect(screen.getByText('aasquier')).toBeTruthy()
  })

  it('says nothing about ownership on your own deck', async () => {
    // It would be a line telling you your own username.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.queryByText(/you can read this deck, not change it/i)).toBeNull()
  })

  it('links the simulator at the deck, owner and all', async () => {
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.getByText('Simulate this deck').getAttribute('href'))
      .toBe('/simulate?owner=aasquier&deck=goreclaw-stompy')
  })

  it('seats the deck in the coliseum\'s first chair and leaves the second',
     async () => {
    // The Coliseum reads `?a=` and `?b=` as whole addresses — an owner and a
    // slug in one parameter, not two positional strings — and this fills only
    // the first, because the opponent is not a deck page's to choose. Driven
    // against the running room rather than assumed: `?a=` alone wins the
    // Champion's chair and the room seats its own default opposite, so the
    // link lands on a fight that is ready rather than on a shut gate.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    expect(screen.getByText('Send it to the Coliseum').getAttribute('href'))
      .toBe('/coliseum?a=aasquier/goreclaw-stompy')
  })

  it('gives both rooms a voice that answers a hand', async () => {
    // Commandment 17, and the reason this pair exists: the simulate link was
    // an inline `background` a `:hover` could never reach. jsdom has no
    // stylesheet, so what a test can check is that the control asks for one —
    // a `.btn` face rather than a colour written on the element.
    renderUnfolded()
    await screen.findByText('Goreclaw — Mono-Green Stompy')
    for (const label of ['Simulate this deck', 'Send it to the Coliseum']) {
      const link = screen.getByText(label)
      expect([...link.classList], `${label} wears no button face`)
        .toContain('btn')
      expect(link.getAttribute('style'), `${label} colours itself inline`)
        .toBeNull()
    }
  })
})

/**
 * The slot argument (ADR 25), on the card row rather than in the editor.
 *
 * Its guards are almost all server-side — the response schema has no field for
 * a reason to keep the card, an uncited charge is dropped, and every named
 * alternative is checked against the pool, the ban list and the deck's colour
 * identity before it is serialised. What belongs *here* is the part only a
 * renderer can get wrong: this is a one-sided argument and the page has to say
 * so, and nothing on it may put text into a rationale.
 */
describe('DeckDetail slot argument', () => {
  function rowFor(card: string) {
    const el = screen.getAllByText(card).find((n) => n.closest('li'))
    return el!.closest('li')!
  }

  async function argueAbout(card: string) {
    // The draft, because it is the fixture carrying Sol Ring — and because a
    // card owing a rationale is the one somebody is most likely to argue
    // about before deciding whether to write one.
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Argue slot' }))
    fireEvent.click(screen.getByRole('button', { name: `Argue slot: ${card}` }))
    return rowFor(card)
  }

  it('asks once when the panel is opened, not on page load', async () => {
    vi.mocked(api.deck).mockResolvedValue(DRAFT)
    renderUnfolded()
    await screen.findByText(DECK.name)
    // Rendering the deck must not spend money. The control says "argue"; the
    // pick is the consent.
    expect(api.argue).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Argue slot' }))
    fireEvent.click(screen.getByRole('button', { name: 'Argue slot: Sol Ring' }))
    await waitFor(() => expect(api.argue).toHaveBeenCalledWith(
      REF, { card: 'Sol Ring' }))
    expect(api.argue).toHaveBeenCalledTimes(1)
  })

  it('renders the case against and offers nothing that writes a rationale', async () => {
    const row = await argueAbout('Sol Ring')
    await within(row).findByText('Six other cards already ramp for two or less.')
    // The citation is rendered too: a charge is an argument because it rests
    // on something, and hiding the fact would leave only the assertion.
    expect(within(row).getByText(/ramp: 7 against a target/)).toBeTruthy()

    // The rule, adjusted for ADR 11: an alternative can be *taken* — that is
    // the swap tested below — but until somebody chooses one there is no
    // textbox here, and never a control that inserts model text into one.
    expect(within(row).queryByRole('textbox')).toBeNull()
    expect(within(row).queryByRole(
      'button', { name: /insert|copy|write it for me/i })).toBeNull()
  })

  it('takes an alternative to the swap composer, whose box opens empty', async () => {
    const row = await argueAbout('Sol Ring')
    await within(row).findByText('Cultivator Colossus')
    fireEvent.click(within(row).getByRole('button', { name: 'Use this card' }))

    // Rule 4, at the moment it is easiest to break: the argue payload is full
    // of plausible sentences about this exact card, and none of them may
    // arrive in the box.
    const box = within(row).getByRole('textbox') as HTMLTextAreaElement
    expect(box.value).toBe('')
    const apply = within(row).getByRole('button', { name: /apply swap/i })
    expect((apply as HTMLButtonElement).disabled).toBe(true)
  })

  it('swaps with the user’s words once they write them', async () => {
    const row = await argueAbout('Sol Ring')
    await within(row).findByText('Cultivator Colossus')
    fireEvent.click(within(row).getByRole('button', { name: 'Use this card' }))
    fireEvent.change(within(row).getByRole('textbox'),
      { target: { value: 'Lands onto the battlefield untapped and swings.' } })
    fireEvent.click(within(row).getByRole('button', { name: /apply swap/i }))

    await waitFor(() => expect(api.swapCard).toHaveBeenCalledWith(
      REF, { out: 'Sol Ring', into: 'Cultivator Colossus',
             why: 'Lands onto the battlefield untapped and swings.' }))
    // The argued card is gone, so the page refreshes and the panel closes.
    await waitFor(() => expect(api.deck).toHaveBeenCalledTimes(2))
  })

  it('says it is the case against, never an assessment', async () => {
    // The misreading this design makes possible is somebody taking a
    // one-sided argument for a verdict, so naming it is the mitigation.
    //
    // Matched **exactly**, on the panel's own heading. A loose
    // `/the case against/i` passes on the `never` sentence in the payload,
    // which means it keeps passing when the heading is relabelled
    // "Assessment" — verified by doing exactly that. A test that asserts the
    // server's own text back at itself is not testing the renderer.
    const row = await argueAbout('Sol Ring')
    await within(row).findByText('The case against')
    // Awaited, not `getByText`: the heading above renders as soon as the
    // status check resolves, and `report.never` only when the argue call
    // does. A sync assertion here lands in that gap under load — this test
    // was the suite's one flake until it waited.
    await within(row).findByText(/rationale is yours to write/)
  })

  it('labels the answer as Claude’s rather than the gate’s', async () => {
    const row = await argueAbout('Sol Ring')
    await within(row).findByText(/not the gate/)
  })

  it('names which alternatives were dropped and why', async () => {
    // Two different failures. "You invented that card" is about the model;
    // "that card is off-colour" is about the deck. A single count says
    // neither, which is why the payload keeps them apart and this renders them
    // apart.
    const row = await argueAbout('Sol Ring')
    await within(row).findByText('Cultivator Colossus')
    expect(within(row).getByText(/banned in Commander: Primeval Titan/)).toBeTruthy()
    expect(within(row).getByText(/colour identity: Ajani, Nacatl Pariah/)).toBeTruthy()
  })

  it('reports dropped charges rather than hiding them', async () => {
    vi.mocked(api.argue).mockResolvedValue({
      ...ARGUMENT, charges: [], charges_dropped: 2,
    })
    const row = await argueAbout('Sol Ring')
    await within(row).findByText(/2 charges cited nothing/)
  })

  it('says a stance of off made no call, rather than an empty case', async () => {
    vi.mocked(api.argue).mockResolvedValue({
      ...ARGUMENT, asked: false, charges: [], alternatives: [],
      reason: 'The stance is off, so no call was made.',
    })
    const row = await argueAbout('Sol Ring')
    await within(row).findByText(/stance is off/)
  })

  it('is not offered on a deck you cannot edit', async () => {
    // Gated with the writes, and it is a spend argument: arguing a slot on
    // somebody else's deck spends a call to reach a conclusion you cannot act
    // on.
    vi.mocked(api.deck).mockResolvedValue({ ...DRAFT, writable: false } as Deck)
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: /argue slot/i })).toBeNull()
  })
})

/**
 * The deck review — the slot argument swept over a selection, as a job.
 *
 * The panel's obligations are the sweep's economics: nothing runs on open,
 * the count is stated before the click, one job covers the selection, a
 * stored job id reattaches instead of paying twice, and the queue renders
 * through the same `SlotArgumentBody` the per-card panel uses — so the swap
 * path (and only the swap path) can change the deck.
 */
describe('DeckDetail deck review', () => {
  const REVIEW = {
    slug: 'goreclaw-stompy', asked: true, reason: '', total: 2,
    reports: [
      { ...ARGUMENT, card: 'Primeval Titan' },
      { ...ARGUMENT, card: 'Sol Ring' },
    ],
    errors: { 'Vorinclex, Voice of Hunger': 'the model was rate limited' },
  }

  async function openPanel() {
    renderUnfolded()
    await screen.findByText(DECK.name)
    fireEvent.click(screen.getByRole('button', { name: 'Review with Claude' }))
  }

  it('opens with spells preselected and runs nothing until asked', async () => {
    await openPanel()
    // DECK's one card is a spell, so the button counts it — and no job has
    // been submitted by merely opening the panel.
    expect(screen.getByRole('button', { name: /argue 1 slot/i })).toBeTruthy()
    expect(screen.getByText(/1 card → 1 Claude conversation/)).toBeTruthy()
    expect(api.argueDeck).not.toHaveBeenCalled()
  })

  it('starts one job for the selection and follows it', async () => {
    vi.mocked(api.argueDeck).mockResolvedValue(job(null, 'queued'))
    vi.mocked(followJob).mockReturnValue({
      promise: Promise.resolve(job(REVIEW)),
      cancel: () => {},
    })
    await openPanel()
    fireEvent.click(screen.getByRole('button', { name: /argue 1 slot/i }))

    await waitFor(() => expect(api.argueDeck).toHaveBeenCalledWith(
      REF, { cards: ['Primeval Titan'] }))
    // The queue rendered, through the shared body: one entry per report, so
    // the shared charge text appears once per argued card.
    const charges = await screen.findAllByText(
      'Six other cards already ramp for two or less.')
    expect(charges).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Use this card' }).length)
      .toBeGreaterThan(0)
    // A failed card is reported against its name, not silently dropped.
    expect(screen.getByText(/rate limited/)).toBeTruthy()
  })

  it('reattaches to a stored run instead of paying twice', async () => {
    localStorage.setItem('mtglab-deck-review:aasquier/goreclaw-stompy', 'job-9')
    vi.mocked(followJob).mockReturnValue({
      promise: Promise.resolve(job(REVIEW)),
      cancel: () => {},
    })
    renderUnfolded()
    await screen.findByText(DECK.name)

    await waitFor(() => expect(vi.mocked(followJob)).toHaveBeenCalledWith(
      'job-9', expect.any(Function), 2000))
    expect(api.argueDeck).not.toHaveBeenCalled()
    // Finished: the id is spent, so a reload does not chase a dead job.
    await waitFor(() => expect(
      localStorage.getItem('mtglab-deck-review:aasquier/goreclaw-stompy')).toBeNull())
  })

  it('is not offered on a deck you cannot edit', async () => {
    vi.mocked(api.deck).mockResolvedValue({ ...DECK, writable: false } as Deck)
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(screen.queryByRole('button', { name: 'Review with Claude' })).toBeNull()
  })
})

/** The history tab (ADR 28).
 *
 * Three things are behaviour rather than markup. It is fetched **lazily**, on
 * the same terms as the shortlist beside it — most visits never open it. It is
 * **re-fetched after an edit**, because an entry appearing is the whole point
 * and a panel the user is watching must not go stale behind them. And an
 * unasked history and an empty one are **not the same screen**: collapsing
 * them tells somebody their deck has no history while it is still loading.
 */
describe('deck history', () => {
  function openHistory() {
    fireEvent.click(screen.getByRole('tab', { name: 'History' }))
  }

  it('is not fetched until the tab is opened', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    expect(api.deckLog).not.toHaveBeenCalled()

    openHistory()
    await waitFor(() => expect(api.deckLog).toHaveBeenCalledWith(REF))
  })

  it('shows the server’s sentence, the actor and the verb', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    openHistory()

    await screen.findByText('entombed Primeval Titan')
    expect(screen.getByText('changed the rationale for Sol Ring')).toBeTruthy()
    expect(screen.getByText('entomb')).toBeTruthy()
    expect(screen.getByText('aasquier')).toBeTruthy()
    // A null actor is whoever is at the machine, and is named rather than
    // left blank — an unnamed actor is not an unknown one.
    expect(screen.getByText('you, locally')).toBeTruthy()
  })

  it('never shows a rationale, because it is never sent one', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    openHistory()

    await screen.findByText('changed the rationale for Sol Ring')
    // The card's `why` is on the deck, and the entry about it says only that
    // it changed. Rule 4's text lives in deck.yaml.
    expect(screen.queryByText(/Ramp and threat in one card/)).toBeNull()
  })

  it('distinguishes “nothing recorded” from “not asked yet”', async () => {
    vi.mocked(api.deckLog).mockResolvedValue({
      slug: 'goreclaw-stompy', entries: [],
    })
    renderUnfolded()
    await screen.findByText(DECK.name)
    openHistory()

    await screen.findByText(/Nothing recorded yet/)
    // The sentence that stops an empty panel reading as a lost history: these
    // decks were edited for months before the log existed. It used to say
    // those edits were "in git", which ADR 30 had already made false and
    // Commandment 10 forbids naming to a reader either way — and this
    // assertion pinned the wrong words, which is the whole trap: a test
    // written against a claim cannot tell you the claim is wrong.
    expect(screen.getByText(/left no trace but the deck itself/)).toBeTruthy()
  })

  it('re-reads itself after an edit', async () => {
    renderUnfolded()
    await screen.findByText(DECK.name)
    openHistory()
    await waitFor(() => expect(api.deckLog).toHaveBeenCalledTimes(1))

    // Any edit will do; `refresh` is what every one of them calls.
    fireEvent.click(screen.getByRole('tab', { name: 'The 99' }))
    fireEvent.click(screen.getByRole('button', { name: 'Entomb' }))
    armThenConfirm(screen.getByRole('button', { name: /Primeval Titan/ }))
    await waitFor(() => expect(api.entombCard).toHaveBeenCalled())
    await waitFor(() => expect(api.deckLog).toHaveBeenCalledTimes(2))
  })

  it('is shown on a deck you cannot edit', async () => {
    // Reading a shared deck's history is reading the deck (ADR 28) — the
    // server gates it by the same source, so the client must not hide it.
    vi.mocked(api.deck).mockResolvedValue({ ...DECK, writable: false } as Deck)
    renderUnfolded()
    await screen.findByText(DECK.name)
    openHistory()
    await screen.findByText('entombed Primeval Titan')
  })
})

describe('the 99 rolls up', () => {
  // Feature 2 of the 2026-08-18 batch: each category folds away behind its
  // header, and the fold survives leaving — the 99 is a place you arrange.
  // The 2026-08-18 punch list then flipped the *default*: it arrives folded.
  // These are the tests that render bare, without `renderUnfolded`.
  it('opens folded, so the page you land on is the deck\'s shape', async () => {
    renderDeck()
    // The signs are there and the cards are not — this is the whole feature.
    const header = await screen.findByRole('button', { name: /Ramp/ })
    expect(header.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByText('Primeval Titan')).toBeNull()
    // And with everything shut, the one verb offers the other direction.
    expect(screen.getByRole('button', { name: 'Unfold all' })).toBeTruthy()
  })

  it('folds a category away behind its header', async () => {
    renderUnfolded()
    await screen.findByText('Primeval Titan')
    const header = screen.getByRole('button', { name: /Ramp/ })
    expect(header.getAttribute('aria-expanded')).toBe('true')

    fireEvent.click(header)
    expect(screen.queryByText('Primeval Titan')).toBeNull()
    expect(header.getAttribute('aria-expanded')).toBe('false')

    fireEvent.click(header)
    expect(screen.getByText('Primeval Titan')).toBeTruthy()
  })

  it('remembers an unfolded shelf across a visit', async () => {
    // The direction that matters now the default is closed: an *explicit*
    // empty list is "I opened these", and must not be read back as the absent
    // "never touched" and re-folded. Two `[]` that behave differently is
    // exactly the bug this pins.
    renderUnfolded()
    await screen.findByText('Primeval Titan')
    cleanup()

    renderDeck()
    expect(await screen.findByText('Primeval Titan')).toBeTruthy()
  })

  it('remembers a fold the way the wheel used to remember its own', async () => {
    renderUnfolded()
    await screen.findByText('Primeval Titan')
    fireEvent.click(screen.getByRole('button', { name: /Ramp/ }))
    cleanup()

    renderDeck()
    await screen.findByRole('button', { name: /Ramp/ })
    // The row list is unmounted, not hidden — the header still names the
    // group and its count, so nothing is lost, only quiet.
    expect(screen.queryByText('Primeval Titan')).toBeNull()
  })
})

describe('the tabs on a phone', () => {
  /*
   * **The widest horizontal overflow in the app, and nothing in this file
   * could see it.** Six tabs want 432px of row; a 375px phone has 327 to give
   * inside the page gutters, so History hung 81px off the right edge and the
   * whole page scrolled sideways — measured in a real browser as
   * `documentElement.scrollWidth - clientWidth`, which is the only honest
   * witness there is. jsdom has no layout engine, so what follows holds the
   * class and not one pixel of the consequence; the pixels are Aaron's walk
   * (commandment 16) and the numbers are in the pull request.
   *
   * Worth pinning even so, because the class is the whole fix and it is one
   * word long. A tidy-up that folds these utilities into a shared string, or a
   * copy-paste from a strip that never needed it, drops that word in silence
   * and the bug comes back with a green suite behind it.
   */
  it('wraps rather than running off the side', async () => {
    renderDeck()
    const history = await screen.findByRole('tab', { name: 'History' })
    const strip = history.parentElement

    expect(strip?.className).toContain('flex-wrap')
    // Both halves: `flex-wrap` on something that is not a flex row does
    // nothing at all, and would still pass an assertion about the one word.
    expect(strip?.className).toContain('flex ')
    // And all six are still in the row that wraps. A strip that "fixed" this
    // by dropping tabs on a phone would be the worse bug — a newcomer does
    // not go looking for a tab they have never seen (commandment 2) — and so
    // would one that solved it by scrolling, which hides the last two behind
    // a gesture nobody is told about.
    for (const name of ['The 99', 'Stats', 'Validation', 'Notes', 'Artifacts',
      'History']) {
      expect(screen.getByRole('tab', { name }).parentElement).toBe(strip)
    }
  })

  it('folds and unfolds everything from one control', async () => {
    renderUnfolded()
    await screen.findByText('Primeval Titan')
    fireEvent.click(screen.getByRole('button', { name: 'Fold all' }))
    expect(screen.queryByText('Primeval Titan')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Unfold all' }))
    expect(screen.getByText('Primeval Titan')).toBeTruthy()
  })
})

/**
 * The bench, and the sentence that explains an absence.
 *
 * `swap_board` has been in the deck model, the importer's section table and
 * the edit panel since long before the deck page existed, and the page never
 * rendered it — so the only way to see what a deck was considering was to
 * read the YAML.
 *
 * The read-only line is the other half of the same session's finding: the
 * header already says "Shared by <owner>", and Aaron still read the missing
 * action bar as a regression. A muted line four screens above the controls
 * cannot answer a question somebody only forms where the controls used to be.
 */
describe('DeckDetail swap board', () => {
  const BENCH = [
    { name: 'Skullclamp', category: 'engines', qty: 1, known: true,
      why: 'Two mana of draw-two on every one-toughness creature.',
      mana_cost: '{1}', cmc: 1, type_line: 'Artifact — Equipment',
      color_identity: [], art_crop: 'https://example.test/clamp.jpg' },
    { name: 'Sylvan Library', category: 'card-advantage', qty: 1, known: true,
      why: '', mana_cost: '{1}{G}', cmc: 2, type_line: 'Enchantment',
      color_identity: ['G'], art_crop: null },
  ]

  /** Open the fold. Collapsed is the section's resting state now, so every
   *  assertion about its contents has to press the heading first. */
  async function openBoard() {
    fireEvent.click(await screen.findByRole('button', { name: /Swap board/ }))
  }

  // **The empty board is the owner's and nobody else's.** It used to be absent
  // from both, which is what left a deck that had never kept a board with
  // nowhere to start one (Aaron, 2026-08-29). A reader still gets nothing: an
  // empty shelf is an invitation to whoever can fill it and furniture to
  // everybody else.
  it('offers an empty board to the owner, so there is somewhere to start one',
     async () => {
    renderDeck()
    await screen.findByRole('tab', { name: 'Validation' })
    expect(screen.queryByText(/Swap board/)).not.toBeNull()
  })

  it('is absent to a reader while the deck has nothing on it', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, writable: false } as unknown as Deck)
    renderDeck()
    await screen.findByRole('tab', { name: 'Validation' })
    expect(screen.queryByText(/Swap board/)).toBeNull()
  })

  // **Collapsed by default**, like the token shelf below it (Aaron,
  // 2026-08-29: "right now the Sideboard doesn't collapse like the other
  // areas. Its default should be collapsed"). The heading is there; what is
  // under it is not, until somebody asks.
  it('is folded until it is opened', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, swap_board: BENCH } as unknown as Deck)
    renderDeck()
    await screen.findByText(/Swap board/)
    expect(screen.queryByText('Skullclamp')).toBeNull()

    await openBoard()
    expect(screen.getAllByText('Skullclamp').length).toBeGreaterThan(0)
  })

  it('renders the cards being considered, and says they are outside the 99',
     async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, swap_board: BENCH } as unknown as Deck)
    renderDeck()
    await screen.findByText(/Swap board/)
    await openBoard()
    // `getAllBy`, because a card renders its name in the row and again in
    // the hover card that rides it.
    expect(screen.getAllByText('Skullclamp').length).toBeGreaterThan(0)
    // A card with no rationale is fine here and says nothing about it: the
    // bench carries no obligation, and "no rationale yet" is the 99's warning.
    expect(screen.getAllByText('Sylvan Library').length).toBeGreaterThan(0)
    expect(screen.queryByText('no rationale yet')).toBeNull()
    expect(screen.getByText(/outside the 99/)).toBeTruthy()
  })

  it('shows the bench to a reader, the way the graveyard is shown', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, writable: false, swap_board: BENCH } as unknown as Deck)
    renderDeck()
    await screen.findByText(/Swap board/)
    await openBoard()
    expect(screen.getAllByText('Skullclamp').length).toBeGreaterThan(0)
  })

  // The empty state is the whole feature for a deck that has none, so it says
  // what a swap board *is* before it offers to start one (commandment 2). A
  // reader who has never kept a maybeboard has to learn something here or the
  // button is a dare.
  it('explains what a board is before it offers to start one', async () => {
    renderDeck()
    await screen.findByText(/Swap board/)
    await openBoard()
    expect(screen.getByText(/shortlist it never cut anything for/)).toBeTruthy()
    // And the reassurance, which is the sentence that gets the button pressed:
    // putting a card here does nothing to the deck.
    expect(screen.getByText(/changes nothing about the deck/)).toBeTruthy()
    expect(screen.getByRole('button', { name: /Start a swap board/ })).toBeTruthy()
  })

  // A deck that already has one is offered the same control saying the other
  // thing, because "start" is wrong once there is something to add to.
  it('offers to weigh another card once the board has one', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, swap_board: BENCH } as unknown as Deck)
    renderDeck()
    await screen.findByText(/Swap board/)
    await openBoard()
    expect(screen.getByRole('button', { name: /Weigh another card/ })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Start a swap board/ })).toBeNull()
  })

  // The reader gets the list and no way to change it. The board grew its first
  // control on 2026-08-29 and this is the line that keeps it the owner's.
  it('gives a reader no way to add to it', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, writable: false, swap_board: BENCH } as unknown as Deck)
    renderDeck()
    await screen.findByText(/Swap board/)
    await openBoard()
    expect(screen.queryByRole('button', { name: /Weigh another card/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Start a swap board/ })).toBeNull()
  })
})

describe('a deck somebody else owns', () => {
  it('says why the controls are gone, where the controls were', async () => {
    vi.mocked(api.deck).mockResolvedValue(
      { ...DECK, writable: false } as unknown as Deck)
    renderDeck()
    await screen.findByRole('tab', { name: 'Validation' })
    // Named, so it is a fact about a person rather than a permission error.
    expect(screen.getByText(/This is aasquier’s deck, and you are reading it/))
      .toBeTruthy()
    // And it names what is missing, since that is the question being asked.
    expect(screen.getByText(/entombing one, writing a/)).toBeTruthy()
    expect(screen.queryByText('Bulk entomb…')).toBeNull()
  })

  it('says nothing of the kind on your own deck', async () => {
    renderDeck()
    await screen.findByRole('tab', { name: 'Validation' })
    expect(screen.queryByText(/you are reading it/)).toBeNull()
    expect(screen.getByText('Bulk entomb…')).toBeTruthy()
  })
})

/* Whose sentence this is (ADR 41).
 *
 * A drafted `why` satisfies `curated` — Aaron ruled that on 2026-08-28 — so
 * `why_by` is the only thing separating a rationale Claude wrote from one the
 * owner wrote. It was written faithfully into `deck.yaml` and then dropped on
 * the way to the browser: `deckread.CardJSON` had no such field, so the page
 * anybody actually reads their deck on could not tell them.
 *
 * Both directions, because a mark that never shows and a mark that shows on
 * everything are the same bug to a reader.
 */
describe('a rationale says who drafted it', () => {
  it('marks the ones Claude wrote', async () => {
    vi.mocked(api.deck).mockResolvedValue({
      ...DECK,
      cards: [{ ...DECK.cards[0], why_by: 'claude' }],
    } as unknown as Deck)
    renderUnfolded()
    expect(await screen.findByText('Ramp and threat in one card.')).toBeTruthy()
    expect(screen.getByText('Claude drafted this')).toBeTruthy()
  })

  it('says nothing about a rationale a person wrote', async () => {
    renderUnfolded()
    expect(await screen.findByText('Ramp and threat in one card.')).toBeTruthy()
    // The unmarked case is the overwhelmingly common one, and it must read as
    // an absence rather than as a claim about authorship.
    expect(screen.queryByText('Claude drafted this')).toBeNull()
  })
})
