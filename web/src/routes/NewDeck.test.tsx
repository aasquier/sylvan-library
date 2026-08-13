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

vi.mock('../lib/api', () => ({
  api: {
    colors: vi.fn(), searchCards: vi.fn(), createDeck: vi.fn(),
    claudeStatus: vi.fn(), themeAsk: vi.fn(), themePropose: vi.fn(),
    job: vi.fn(),
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

const PROPOSAL = {
  answered_by: 'claude', mode: 'theme-proposal', model: 'claude-sonnet-5',
  asked: true, reason: '', stance: STANCE,
  combinations: [{
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
  }],
  sources: [{ id: 's1', title: 'Golgari on Ravnica', url: 'https://w.test/golgari' }],
  sources_dropped: 0, commanders_dropped: 1, combinations_dropped: 1,
  searched: 4,
  slots: READY.slots,
  usage: { input_tokens: 100, output_tokens: 50 },
  never: 'The reading is Claude’s interpretation of what you said, not a finding.',
}

function open() {
  return render(<MemoryRouter><NewDeck /></MemoryRouter>)
}

/** Walk in through the third door. */
async function enterTheme() {
  open()
  fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
  return screen.findByText(PARTWAY.question)
}

beforeEach(() => {
  localStorage.clear()
  navigate.mockReset()
  vi.mocked(api.colors).mockReset().mockResolvedValue(TAXONOMY as never)
  vi.mocked(api.searchCards).mockReset().mockResolvedValue({ cards: [], total: 0 } as never)
  vi.mocked(api.createDeck).mockReset().mockResolvedValue({ slug: 'x' } as never)
  vi.mocked(api.claudeStatus).mockReset().mockResolvedValue(CLAUDE_STATUS as never)
  vi.mocked(api.themeAsk).mockReset().mockResolvedValue(PARTWAY as never)
  vi.mocked(api.themePropose).mockReset()
    .mockResolvedValue({ id: 'j1', status: 'queued' } as never)
  vi.mocked(followJob).mockReset().mockImplementation((id) => ({
    promise: Promise.resolve(
      { id, status: 'done', result: PROPOSAL } as never),
    cancel: () => {},
  }))
})

afterEach(cleanup)

describe('the three doors', () => {
  it('offers the one that assumes least first', async () => {
    open()
    const doors = await screen.findAllByRole('button',
      { name: /help me decide|take me through|i know what i want/i })
    expect(doors[0].textContent).toMatch(/help me decide/i)
  })

  it('does not remember the theme door', async () => {
    // Landing back in a Claude conversation because you tried one last week is
    // a bill nobody asked for, so this is the one mode that is not sticky.
    await enterTheme()
    expect(localStorage.getItem('mtglab-new-deck-mode')).not.toBe('theme')
  })

  it('still remembers the other two', async () => {
    open()
    fireEvent.click(await screen.findByRole('button', { name: /i know what i want/i }))
    await waitFor(() => expect(
      localStorage.getItem('mtglab-new-deck-mode')).toBe('direct'))
  })
})

describe('the theme conversation', () => {
  it('opens itself rather than waiting to be prompted', async () => {
    await enterTheme()
    expect(api.themeAsk).toHaveBeenCalledWith(
      { transcript: [], slots: [] })
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
    }))
  })

  it('will not propose below the floor', async () => {
    await enterTheme()
    const button = screen.getByRole('button', { name: /suggest my colours/i })
    expect(button.hasAttribute('disabled')).toBe(true)
    expect(screen.getByText(/2 of 3 things known/i)).toBeTruthy()
  })

  it('opens up once three things are known', async () => {
    vi.mocked(api.themeAsk).mockResolvedValue(READY as never)
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
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
    vi.mocked(api.themeAsk).mockResolvedValue(READY as never)
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
    await screen.findByText(READY.question)
    fireEvent.click(screen.getByRole('button', { name: /suggest my colours/i }))
    return screen.findByText(PROPOSAL.combinations[0].reading)
  }

  it('submits a job and reads the answer off it', async () => {
    // The deploy blocker this shape exists for: 226 seconds of synchronous
    // POST does not survive a hosted proxy. The POST now returns a job and the
    // proposal arrives from the poller.
    await propose()
    expect(vi.mocked(followJob).mock.calls[0]?.[0]).toBe('j1')
  })

  it('reattaches to a run in flight instead of paying for a second', async () => {
    // A reload during those minutes must not resubmit — that is a second four
    // minutes and a second bill for an answer already being written. The id is
    // in the same stash the transcript is.
    localStorage.setItem('mtglab-theme-conversation', JSON.stringify({
      transcript: [{ role: 'assistant', text: READY.question }],
      slots: READY.slots, job: 'j9', proposal: null,
    }))
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))

    await screen.findByText(PROPOSAL.combinations[0].reading)
    expect(vi.mocked(followJob).mock.calls[0]?.[0]).toBe('j9')
    expect(api.themePropose).not.toHaveBeenCalled()
  })

  it('clocks the run, not this tab', async () => {
    // Found by reloading during a real 226-second run: the timer restarted at
    // 0s against a job already a minute old, which is exactly the confusion a
    // clock was put there to remove. The job carries its own `created_at`.
    vi.mocked(followJob).mockImplementation((_id, onTick) => {
      onTick({
        id: 'j9', kind: 'claude.theme.proposal', status: 'running',
        done: 3, total: 8, percent: 38, label: '', result: null, error: null,
        created_at: new Date(Date.now() - 90_000).toISOString(),
      })
      return { promise: new Promise(() => {}), cancel: () => {} }
    })
    vi.mocked(api.themeAsk).mockResolvedValue(READY as never)
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
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
    vi.mocked(followJob).mockImplementation(() => ({
      promise: Promise.reject(new ApiError('no job', 404)),
      cancel: () => {},
    }))
    vi.mocked(api.themeAsk).mockResolvedValue(READY as never)
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))
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
    vi.mocked(api.claudeStatus).mockResolvedValue(
      { ...CLAUDE_STATUS, configured: false } as never)
    open()
    fireEvent.click(await screen.findByRole('button', { name: /help me decide/i }))

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
