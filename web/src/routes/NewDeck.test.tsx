/**
 * The theme interview — the create flow's third door (ADR 20).
 *
 * The other two doors both open onto "which of the 32 do you want", which is
 * the question somebody who has never played cannot answer. What is worth
 * pinning here is not the markup but the four claims the ADR makes:
 *
 * * The proposal is a **proposal**. Picking a commander fills in the create
 *   form; it does not make a deck. Nothing under `src/mtglab/claude/` can
 *   reach a write path and the UI has to tell the same story.
 * * A reading and a claim are **labelled differently**, all the way to the
 *   page. One of them can be wrong.
 * * A slot shows **the words it rests on**, because the server threw away
 *   every reading it could not find in the transcript and that check is worth
 *   nothing if its result is invisible.
 * * The floor is a floor: no proposing until three things are known.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import NewDeck from './NewDeck'

const navigate = vi.fn()
vi.mock('react-router-dom', async () => ({
  ...(await vi.importActual<typeof import('react-router-dom')>('react-router-dom')),
  useNavigate: () => navigate,
}))

vi.mock('../lib/api', async () => ({
  // Real: it is what turns the create response into the deck's address, and a
  // stub would let this navigate to the pre-ADR-22 path with every test
  // passing.
  deckUrl: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).deckUrl,
  api: {
    colors: vi.fn(), searchCards: vi.fn(), createDeck: vi.fn(),
    claudeStatus: vi.fn(), themeAsk: vi.fn(), themePropose: vi.fn(),
    job: vi.fn(), personas: vi.fn(), tarotReading: vi.fn(),
  },
  // The proposal is a background job now — it was measured at 226 seconds and
  // no hosted proxy holds a POST open that long. Faked rather than imported
  // for real, because the real poller closes over the `api` object this mock
  // replaces and would go looking for a `fetch` nobody set up.
  followJob: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    constructor(message: string, status: number) {
      super(message)
      this.status = status
    }
  },
}))

const { api, followJob } = await import('../lib/api')

const TAXONOMY = {
  colors: [{ code: 'G', name: 'Green', wants: 'growth', fears: 'artifice' }],
  tiers: [{ key: 'guild', label: 'Guilds', blurb: 'Two colours.' }],
  eras: [{ name: 'Ravnica', setting: 'a city', named: 'the guilds', story: 'ten of them' }],
  combinations: [{
    key: 'BG', name: 'Golgari', tier: 'guild', colors: ['B', 'G'], size: 2,
    tagline: 'Death and rebirth.', history: 'Ravnica, 2005.',
    aliases: ['Golgari'], verified_by: 'Scryfall',
    // The teaching fields, in the shape `/api/colors` serves them. Names and
    // a role only: the cards come from `/api/colors/{key}`, which this screen
    // never calls — it shows the short version and links across.
    lore: 'The Swarm holds the undercity, and the succession is a sacrifice.',
    champions: [{ card: 'Jarad, Golgari Lich Lord', role: 'Dead, and in charge.' }],
    signature: ["Assassin's Trophy"],
  }],
}

const STANCE = {
  preset: 'second-opinion', allows_calls: true, may_write: false,
  axes: [
    { axis: 'initiative', question: 'When?', level: 'volunteers', means: '…',
      levels: ['off', 'on-request', 'volunteers', 'interjects'] },
    { axis: 'scope', question: 'How far?', level: 'adjacent', means: '…',
      levels: ['flagged', 'adjacent', 'rethink'] },
    { axis: 'write', question: 'What?', level: 'none', means: '…',
      levels: ['none', 'proposes', 'applies'] },
  ],
}

const CLAUDE_STATUS = {
  installed: true, configured: true, model: 'claude-sonnet-5',
  stance: STANCE, ceiling: STANCE, default: STANCE, presets: [],
  never: 'No stance lets Claude write a card’s rationale.', modes: [],
}

/** Two things known, so the floor is not met yet. */
const PARTWAY = {
  answered_by: 'claude', mode: 'theme-conversation', model: 'claude-sonnet-5',
  asked: true, reason: '', stance: STANCE,
  question: 'What would you put on a wall?',
  fact: { text: 'Golgari debuted in 2005.', source: 'Wizards', url: 'https://w.test/g' },
  slots: [
    { kind: 'taste', value: 'epic desert science fiction', quote: 'Dune, easily' },
    { kind: 'posture', value: 'builds quietly', quote: 'quietly build something' },
  ],
  slots_dropped: 1, grounded: 2, floor: 3, may_propose: false,
  exchanges: 2, max_exchanges: 10,
  usage: { input_tokens: 10, output_tokens: 5 },
  never: 'These are questions about you. What you build is your call.',
}

const READY = {
  ...PARTWAY,
  question: 'Anything you already know you want?',
  slots: [...PARTWAY.slots,
          { kind: 'temperament', value: 'replans', quote: 'I just make a new plan' }],
  slots_dropped: 0, grounded: 3, may_propose: true, exchanges: 3,
}

/**
 * A turn, wrapped the way the route now answers one: as a **job**.
 *
 * `/api/claude/theme` was a plain POST returning the report until it was
 * measured — 4.3–37.7s on the instance with one at 133.8s, against a transport
 * ceiling known only to be at or below 236s, which is where the dossier broke.
 * These come back already `done`, which is what `followJob` short-circuits on,
 * so nothing here polls and the shape stays honest.
 */
const asked = (report: unknown) => ({ id: 'a1', status: 'done', result: report })

/**
 * What the real `followJob` does with a job that is already `done`: resolve it
 * and never poll.
 *
 * Every override below is written for the **proposal's** poller, and both
 * theme calls come through that one function now — so an override that does
 * not pass a finished job through answers the *conversation* with whatever the
 * proposal test wanted, and the interview never reaches the button being
 * clicked. Cost of the shared seam, paid once here.
 */
const settled = (job: unknown) =>
  ({ promise: Promise.resolve(job as never), cancel: () => {} })

// Named rather than reached for as `PROPOSAL.combinations[0]`. Two tests below
// assert on its `reading`, and a fixture's first element is a thing to name
// once, not to index twice.
const GOLGARI = {
  key: 'BG', name: 'Golgari', colors: ['B', 'G'], tier: 'guild',
  tagline: 'Death and rebirth.',
  reading: 'Dune is a book about what grows out of a wasteland.',
  grounding: 'Golgari is Ravnica’s guild of decay and regrowth.',
  source_ids: ['s1'],
  commanders: [{
    name: 'Gyome, Master Chef', prose: 'Feeds the table, then eats it.',
    source_ids: ['s1'], mana_cost: '{2}{B}{G}',
    type_line: 'Legendary Creature — Troll Warlock',
    oracle_text: 'Makes Food.', color_identity: ['B', 'G'],
    image: 'https://img.test/gyome.jpg', art_crop: 'https://img.test/gyome-art.jpg',
  }],
}

const PROPOSAL = {
  answered_by: 'claude', mode: 'theme-proposal', model: 'claude-sonnet-5',
  asked: true, reason: '', stance: STANCE,
  combinations: [GOLGARI],
  sources: [{ id: 's1', title: 'Golgari on Ravnica', url: 'https://w.test/golgari' }],
  sources_dropped: 0, commanders_dropped: 1, combinations_dropped: 1,
  searched: 4,
  slots: READY.slots,
  usage: { input_tokens: 100, output_tokens: 50 },
  never: 'The reading is Claude’s interpretation of what you said, not a finding.',
}

/**
 * The reader roster, as `/api/claude/personas` serves it — plus a third voice
 * nobody has written a line of TypeScript for.
 *
 * That third one is the assertion. ADR 21's claim is that adding a reader is a
 * `Persona` and a prompt server-side and *nothing else moves*; the moment this
 * list is hard-coded in the client again, `storyteller` stops appearing and
 * the test that looks for it fails.
 */
const ROSTER = {
  personas: [
    { key: 'plain', label: 'Just talk to me',
      blurb: 'A few questions, and a suggestion at the end.', deals: false },
    { key: 'fortune-teller', label: 'Read my fortune',
      blurb: 'Three cards, and close attention.', deals: true },
    // `storyteller` used to be the unknown voice here, until it was written
    // for real (and got a tile painting). The unknown one has to stay
    // unknown — the assertion is that a voice with no client-side anything,
    // art included, still renders from the roster alone.
    { key: 'necromancer', label: 'Speak with the dead',
      blurb: 'A voice this file has never heard of.', deals: false },
  ],
  default: 'plain',
}

/** A dealt spread, with the first card reversed. */
const READING = {
  seed: 4242,
  cards: [
    { key: '13-death', name: 'Death', arcana: 'major', suit: null, number: 13,
      image: '/tarot/13-death.webp', reversed: true,
      slot: 'taste', position: 'The Root' },
    { key: 'swords-03', name: 'Three of Swords', arcana: 'minor',
      suit: 'swords', number: 3, image: '/tarot/swords-03.webp',
      reversed: false, slot: 'temperament', position: 'The Turning' },
    { key: 'wands-03', name: 'Three of Wands', arcana: 'minor',
      suit: 'wands', number: 3, image: '/tarot/wands-03.webp',
      reversed: false, slot: 'posture', position: 'The Table' },
  ],
}

function open() {
  return render(<MemoryRouter><NewDeck /></MemoryRouter>)
}

/**
 * Walk in through the Claude door and sit down with a named voice.
 *
 * One door now: "Help me decide" opens the persona grid, and the
 * fortune-teller tile is where the old tarot door went. The tile is found by
 * its blurb rather than its label, because a label is one word away from
 * matching two things.
 */
async function sitWith(reader: string) {
  open()
  fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
  const persona = ROSTER.personas.find((p) => p.key === reader)!
  const panel = (await screen.findByText(persona.blurb)).closest('button')!
  fireEvent.click(panel)
  return panel
}

/** The plain interview, which is the grid's first tile. */
async function enterTheme() {
  await sitWith('plain')
  return screen.findByText(PARTWAY.question)
}

/** The fortune-teller (or any other reader) at the same door. */
async function enterTarot(reader = 'fortune-teller') {
  return sitWith(reader)
}

/** The shuffle is a real 1.1s beat before the cards land, so anything waiting
 *  on the deal has to outlast it — `waitFor`'s default second would expire on
 *  the ceremony rather than on a failure. */
const PAST_THE_SHUFFLE = { timeout: 3000 }

beforeEach(() => {
  localStorage.clear()
  navigate.mockReset()
  vi.mocked(api.colors).mockReset().mockResolvedValue(TAXONOMY as never)
  vi.mocked(api.searchCards).mockReset().mockResolvedValue({ cards: [], total: 0 } as never)
  vi.mocked(api.createDeck).mockReset()
    .mockResolvedValue({ slug: 'x', owner: 'aasquier' } as never)
  vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(CLAUDE_STATUS as never)
  vi.mocked(api.themeAsk).mockReset().mockResolvedValue(asked(PARTWAY) as never)
  vi.mocked(api.themePropose).mockReset()
    .mockResolvedValue({ id: 'j1', status: 'queued' } as never)
  // Honours `initial` the way the real one does, which is not decoration: both
  // theme calls come through here now, and a turn is handed back a job that is
  // already `done`. A mock that ignored `initial` would answer every turn with
  // the *proposal*, and would also hide the one property that keeps a cheap
  // turn cheap — that it resolves without polling.
  vi.mocked(followJob).mockReset()
    .mockImplementation((id, _onTick, _ms, initial) => ({
      promise: Promise.resolve(
        (initial?.status === 'done'
          ? initial
          : { id, status: 'done', result: PROPOSAL }) as never),
      cancel: () => {},
    }))
  vi.mocked(api.personas).mockReset().mockResolvedValue(ROSTER as never)
  vi.mocked(api.tarotReading).mockReset().mockResolvedValue(READING as never)
})

afterEach(cleanup)

describe('the three doors', () => {
  it('offers the one that assumes least first', async () => {
    open()
    const doors = await screen.findAllByRole('button',
      { name: /help me decide|take me through|i know what i want/i })
    expect(doors[0]?.textContent).toMatch(/help me decide/i)
  })

  it('does not remember the Claude door', async () => {
    // Landing back in a Claude conversation because you tried one last week is
    // a bill nobody asked for, so this is the one mode that is not sticky.
    await enterTheme()
    expect(localStorage.getItem('mtglab-new-deck-mode')).not.toBe('theme')
  })

  it('does not remember it for a dealt reader either', async () => {
    // Same door now, and the same argument goes double: this voice opens
    // with a shuffle and then spends money on the first question.
    await enterTarot()
    expect(localStorage.getItem('mtglab-new-deck-mode')).not.toBe('theme')
  })

  it('still remembers the other two', async () => {
    open()
    fireEvent.click(await screen.findByRole('button', { name: /i know what i want/i }))
    await waitFor(() => expect(
      localStorage.getItem('mtglab-new-deck-mode')).toBe('direct'))
  })
})

describe('the tarot door', () => {
  it('renders whatever readers the server has, not a list of its own', async () => {
    // ADR 21's payoff, as a test: `necromancer` exists nowhere in this app's
    // source — no label, no blurb, and (now that tiles carry paintings) no
    // art either. If it stops appearing, somebody has hard-coded the roster
    // and adding a voice is a frontend change again.
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
    expect(await screen.findByText('Speak with the dead')).toBeTruthy()
    expect(screen.getByText('A voice this file has never heard of.')).toBeTruthy()
  })

  it('deals face down, and names nothing until it is turned over', async () => {
    await enterTarot()
    // The places are announced; the cards are not. A spread that told you what
    // it had dealt before you turned it over would be a form with candles on.
    await screen.findByText('The Root', {}, PAST_THE_SHUFFLE)
    expect(screen.getByText('The Turning')).toBeTruthy()
    expect(screen.queryByText('Death')).toBeNull()
    expect(screen.getByRole('button', { name: 'Turn over The Root' })).toBeTruthy()
  })

  it('turns one card without turning the others', async () => {
    await enterTarot()
    fireEvent.click(await screen.findByRole(
      'button', { name: 'Turn over The Root' }, PAST_THE_SHUFFLE))
    expect(await screen.findByText('Death')).toBeTruthy()
    expect(screen.queryByText('Three of Swords')).toBeNull()
  })

  it('renders a reversed card the other way up', async () => {
    // The one claim in this whole door that a screenshot proves and a DOM
    // cannot: jsdom computes no transforms, so what is pinned here is the hook
    // the stylesheet rotates — `.tarot-face-front img.is-reversed` — sitting on
    // the image and only on the image. Put it on the face instead and a
    // reversed card spends the flip un-reversing itself and lands upright,
    // which looks exactly like nothing going wrong.
    await enterTarot()
    fireEvent.click(await screen.findByRole(
      'button', { name: 'Turn over The Root' }, PAST_THE_SHUFFLE))
    fireEvent.click(screen.getByRole('button', { name: 'Turn over The Turning' }))

    const death = await screen.findByAltText('Death')
    const swords = screen.getByAltText('Three of Swords')
    expect(death.className).toContain('is-reversed')
    expect(swords.className).not.toContain('is-reversed')
    expect(screen.getByText('reversed')).toBeTruthy()
  })

  it('sends the reader and the seed with every turn', async () => {
    // Both are client-held, for the reason the transcript is: the server keeps
    // no conversation. The seed is what makes three pictures cost one integer
    // — it re-deals the identical spread rather than carrying the cards.
    await enterTarot()
    for (const place of ['The Root', 'The Turning', 'The Table']) {
      fireEvent.click(await screen.findByRole(
        'button', { name: `Turn over ${place}` }, PAST_THE_SHUFFLE))
    }
    await waitFor(() => expect(api.themeAsk).toHaveBeenCalledWith({
      transcript: [], slots: [], persona: 'fortune-teller', seed: 4242,
    }), PAST_THE_SHUFFLE)
  })

  it('deals nothing for a reader who reads no cards', async () => {
    await enterTarot('plain')
    await screen.findByText(PARTWAY.question)
    expect(api.tarotReading).not.toHaveBeenCalled()
    expect(screen.queryByText('The Root')).toBeNull()
    expect(api.themeAsk).toHaveBeenCalledWith(
      { transcript: [], slots: [], persona: 'plain', seed: undefined })
  })

  it('will not carry one reader’s conversation into another’s', async () => {
    // A persona is fixed for a conversation (ADR 21): the transcript is resent
    // whole every turn, so a voice swapped halfway leaves every earlier answer
    // speaking in the old one. A stash left by a different reader is discarded
    // rather than adopted.
    localStorage.setItem('mtglab-theme-conversation', JSON.stringify({
      transcript: [{ role: 'assistant', text: 'A question the plain one asked' }],
      slots: PARTWAY.slots, job: null, proposal: null,
      persona: 'plain', seed: null,
    }))
    await enterTarot()
    for (const place of ['The Root', 'The Turning', 'The Table']) {
      fireEvent.click(await screen.findByRole(
        'button', { name: `Turn over ${place}` }, PAST_THE_SHUFFLE))
    }
    await waitFor(() => expect(api.themeAsk).toHaveBeenCalled(), PAST_THE_SHUFFLE)
    expect(screen.queryByText('A question the plain one asked')).toBeNull()
    expect(vi.mocked(api.themeAsk).mock.calls[0]?.[0].transcript).toEqual([])
  })

  it('re-deals the same spread rather than remembering the cards', async () => {
    // What survives a reload is three integers' worth: who is reading, which
    // seed, and which places have been turned. The pictures come back from the
    // server, which is why a reading needs no table anywhere.
    await enterTarot()
    await screen.findByText('The Root', {}, PAST_THE_SHUFFLE)
    // Waited for, not read once: the stash is written by an effect, and 'The
    // Root' rendering does not promise that effect has flushed with the
    // reading's values yet -- the mount writes a null table first. Read at the
    // wrong instant this saw {persona: null, seed: null}, which is what
    // skipped a deploy on 2026-08-14 (run 31866988085): green on the PR,
    // flaky on the push, under CI's slower threaded Vitest only.
    await waitFor(() => {
      const stashed = JSON.parse(localStorage.getItem('mtglab-tarot-table')!)
      expect(stashed).toEqual(
        { persona: 'fortune-teller', seed: 4242, turned: [] })
    }, PAST_THE_SHUFFLE)

    cleanup()
    open()
    // The stash remembers who was reading, so re-entering the door skips the
    // grid and goes straight back to the table.
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
    await screen.findByText('The Root')
    expect(api.tarotReading).toHaveBeenLastCalledWith(4242)
  })
})

describe('the theme conversation', () => {
  it('opens itself rather than waiting to be prompted', async () => {
    await enterTheme()
    // `persona` rides along even on the door that never offers a choice, and
    // `seed` is absent because this reader is dealt no cards (ADR 21). Both
    // are client-held for the reason the transcript is: the server keeps no
    // conversation, so everything that makes this the same one has to travel.
    expect(api.themeAsk).toHaveBeenCalledWith(
      { transcript: [], slots: [], persona: 'plain', seed: undefined })
  })

  it('asks about you, and this one is not about Magic', async () => {
    await enterTheme()
    expect(screen.getByText(PARTWAY.question)).toBeTruthy()
  })

  it('shows the words each reading rests on', async () => {
    // The grounding check server-side is worth nothing if its result is
    // invisible: somebody should be able to see the interview being held to
    // what they actually said.
    await enterTheme()
    expect(screen.getByText(/because you said “Dune, easily”/)).toBeTruthy()
  })

  it('reports readings that matched nothing they said', async () => {
    await enterTheme()
    expect(screen.getByText(/1 reading did not match anything you said/i))
      .toBeTruthy()
  })

  it('sends the whole conversation back, because the client holds it', async () => {
    await enterTheme()
    fireEvent.change(screen.getByRole('textbox', { name: '' }),
                     { target: { value: 'Dune, easily' } })
    fireEvent.click(screen.getByRole('button', { name: 'Answer' }))

    await waitFor(() => expect(api.themeAsk).toHaveBeenLastCalledWith({
      transcript: [{ role: 'assistant', text: PARTWAY.question },
                   { role: 'user', text: 'Dune, easily' }],
      slots: PARTWAY.slots,
      persona: 'plain',
      seed: undefined,
    }))
  })

  it('will not propose below the floor', async () => {
    await enterTheme()
    const button = screen.getByRole('button', { name: /suggest my colours/i })
    expect(button.hasAttribute('disabled')).toBe(true)
    expect(screen.getByText(/2 of 3 things known/i)).toBeTruthy()
  })

  it('opens up once three things are known', async () => {
    vi.mocked(api.themeAsk).mockResolvedValue(asked(READY) as never)
    await sitWith('plain')
    await screen.findByText(READY.question)

    expect(screen.getByRole('button', { name: /suggest my colours/i })
      .hasAttribute('disabled')).toBe(false)
  })

  it('sources the fun fact it volunteers', async () => {
    await enterTheme()
    const link = screen.getByRole('link', { name: 'Wizards' })
    expect(link.getAttribute('href')).toBe('https://w.test/g')
  })
})

describe('the proposal', () => {
  async function propose() {
    vi.mocked(api.themeAsk).mockResolvedValue(asked(READY) as never)
    await sitWith('plain')
    await screen.findByText(READY.question)
    fireEvent.click(screen.getByRole('button', { name: /suggest my colours/i }))
    return screen.findByText(GOLGARI.reading)
  }

  it('submits a job and reads the answer off it', async () => {
    // The deploy blocker this shape exists for: 226 seconds of synchronous
    // POST does not survive a hosted proxy. The POST now returns a job and the
    // proposal arrives from the poller.
    await propose()
    // The last call, not the first: the conversation turn that got us to the
    // button is a job of its own now and reached this poller before the
    // proposal did.
    expect(vi.mocked(followJob).mock.calls.at(-1)?.[0]).toBe('j1')
  })

  it('reattaches to a run in flight instead of paying for a second', async () => {
    // A reload during those minutes must not resubmit — that is a second four
    // minutes and a second bill for an answer already being written. The id is
    // in the same stash the transcript is.
    localStorage.setItem('mtglab-theme-conversation', JSON.stringify({
      transcript: [{ role: 'assistant', text: READY.question }],
      slots: READY.slots, job: 'j9', proposal: null,
    }))
    await sitWith('plain')

    await screen.findByText(GOLGARI.reading)
    expect(vi.mocked(followJob).mock.calls[0]?.[0]).toBe('j9')
    expect(api.themePropose).not.toHaveBeenCalled()
  })

  it('clocks the run, not this tab', async () => {
    // Found by reloading during a real 226-second run: the timer restarted at
    // 0s against a job already a minute old, which is exactly the confusion a
    // clock was put there to remove. The job carries its own `created_at`.
    vi.mocked(followJob).mockImplementation((_id, onTick, _ms, initial) => {
      if (initial?.status === 'done') return settled(initial)
      onTick({
        id: 'j9', kind: 'claude.theme.proposal', status: 'running',
        done: 3, total: 8, percent: 38, label: '', result: null, error: null,
        created_at: new Date(Date.now() - 90_000).toISOString(),
      })
      return { promise: new Promise(() => {}), cancel: () => {} }
    })
    vi.mocked(api.themeAsk).mockResolvedValue(asked(READY) as never)
    await sitWith('plain')
    await screen.findByText(READY.question)
    fireEvent.click(screen.getByRole('button', { name: /suggest my colours/i }))

    await waitFor(
      () => expect(screen.getByRole('button', { name: /reading around… 9\ds/i }))
        .toBeTruthy(),
      { timeout: 2000 })
  })

  it('says a lost run is lost rather than showing a bare 404', async () => {
    // Jobs live in the server's memory and die with it (`api/jobs.py`). A
    // restart mid-run is the one way this fails that is nobody's mistake.
    const { ApiError } = await import('../lib/api')
    vi.mocked(followJob).mockImplementation((_id, _onTick, _ms, initial) => (
      initial?.status === 'done' ? settled(initial) : {
        promise: Promise.reject(new ApiError('no job', 404)),
        cancel: () => {},
      }))
    vi.mocked(api.themeAsk).mockResolvedValue(asked(READY) as never)
    await sitWith('plain')
    await screen.findByText(READY.question)
    fireEvent.click(screen.getByRole('button', { name: /suggest my colours/i }))

    expect(await screen.findByText(/that run is gone/i)).toBeTruthy()
  })

  it('keeps the reading and the claim visibly apart', async () => {
    // One of these can be wrong and the other cannot. Merging them is the
    // blended paragraph ADR 19 rejected, reached by accident.
    await propose()
    expect(screen.getByText(/an interpretation, not a finding/i)).toBeTruthy()
    expect(screen.getByText(/what is actually true about these colours/i))
      .toBeTruthy()
  })

  it('links the page a claim rests on', async () => {
    await propose()
    expect(screen.getByRole('link', { name: 'Golgari on Ravnica' })
      .getAttribute('href')).toBe('https://w.test/golgari')
  })

  it('reports the cards that did not resolve', async () => {
    await propose()
    expect(screen.getByText(/1 named card did not resolve/i)).toBeTruthy()
  })

  it('reports a whole suggestion it lost', async () => {
    // Measured on a real run: a combination goes when every legend named for
    // it has a subset identity, and half the proposal vanished silently.
    await propose()
    expect(screen.getByText(/1 further suggestion lost every commander/i))
      .toBeTruthy()
  })

  it('proposes — it does not create', async () => {
    // The whole shape of ADR 20 in one assertion. Picking a commander fills in
    // the form that already existed; the deck is made by the person whose deck
    // it is, with the button that was always there.
    await propose()
    fireEvent.click(screen.getByRole('button', { name: /Gyome, Master Chef/ }))

    expect(await screen.findByRole('button', { name: /create the deck/i }))
      .toBeTruthy()
    expect(api.createDeck).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()
  })

  it('carries the pick into the name the deck starts with', async () => {
    await propose()
    fireEvent.click(screen.getByRole('button', { name: /Gyome, Master Chef/ }))
    await screen.findByRole('button', { name: /create the deck/i })

    fireEvent.click(screen.getByRole('button', { name: /create the deck/i }))
    await waitFor(() => expect(api.createDeck).toHaveBeenCalledWith({
      slug: 'gyome-master-chef',
      commander: ['Gyome, Master Chef'],
      name: 'Gyome, Master Chef',
    }))
  })
})

describe('when the surface is not available', () => {
  it('says which of the two things is missing', async () => {
    // The grid itself renders either way — it costs nothing — and the answer
    // arrives when a voice is picked, from the interview's own status check.
    vi.mocked(api.claudeStatus).mockResolvedValue(
      { ...CLAUDE_STATUS, configured: false } as never)
    await sitWith('plain')

    expect(await screen.findByText(/ANTHROPIC_API_KEY/)).toBeTruthy()
    expect(api.themeAsk).not.toHaveBeenCalled()
  })

  it('offers the door that always works', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(
      { ...CLAUDE_STATUS, installed: false } as never)
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))

    fireEvent.click(await screen.findByRole('button', { name: /pick colours myself/i }))
    expect(await screen.findByRole('button', { name: /build golgari/i })).toBeTruthy()
  })
})
