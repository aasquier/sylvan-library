/**
 * The Research page, and the properties a screenshot would not catch.
 *
 * [ADR 26](../../../docs/adr/0026-research-answers-about-magic-not-about-your-deck.md)
 * makes four claims that live partly in this file, because a client that
 * rendered the payload wrong would undo them without the server noticing:
 *
 * - a card the pool **has** and a card it **lacks** must render as visibly
 *   different things, because one is a rule-1 fact and the other is a claim
 *   resting on a page;
 * - `cards_unresolved` is **not** filed with the dropped counts, because it is
 *   the one that is not a fault;
 * - a refused answer renders its reason rather than an empty page;
 * - the stance readout asks for `surface: 'research'`, because with no slug
 *   `/api/claude` otherwise answers `off` for a surface about to run at
 *   `second-opinion` — the fault the create flow already shipped once.
 *
 * Every assertion below matches **this file's own strings**, not the payload's.
 * The slot-argument branch shipped a test that passed against a relabelled
 * heading because it matched a sentence the *server* had sent, and a test
 * asserting the server's text back at itself is not testing the renderer.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ClaudeStatus, Job, ResearchReport } from '../lib/api'
import Research from './Research'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ...actual,
    api: { claudeStatus: vi.fn(), research: vi.fn(), job: vi.fn() },
  }
})

const { api } = await import('../lib/api')

const STATUS: ClaudeStatus = {
  installed: true,
  configured: true,
  model: 'claude-sonnet-5',
  stance: {
    preset: 'second-opinion', allows_calls: true, may_write: false,
    axes: [
      { axis: 'initiative', question: 'When does it speak?', level: 'volunteers', means: 'x', levels: ['a', 'b'] },
      { axis: 'scope', question: 'How far may it range?', level: 'adjacent', means: 'x', levels: ['a', 'b'] },
      { axis: 'write', question: 'What may it change?', level: 'none', means: 'x', levels: ['a', 'b'] },
    ],
  },
  ceiling: {
    preset: 'collaborator', allows_calls: true, may_write: true,
    axes: [
      { axis: 'initiative', question: 'When does it speak?', level: 'volunteers', means: 'x', levels: ['a', 'b'] },
      { axis: 'scope', question: 'How far may it range?', level: 'rethink', means: 'x', levels: ['a', 'b'] },
      { axis: 'write', question: 'What may it change?', level: 'proposes', means: 'x', levels: ['a', 'b'] },
    ],
  },
  default: {
    preset: 'second-opinion', allows_calls: true, may_write: false,
    axes: [
      { axis: 'initiative', question: 'When does it speak?', level: 'volunteers', means: 'x', levels: ['a', 'b'] },
      { axis: 'scope', question: 'How far may it range?', level: 'adjacent', means: 'x', levels: ['a', 'b'] },
      { axis: 'write', question: 'What may it change?', level: 'none', means: 'x', levels: ['a', 'b'] },
    ],
  },
  presets: [],
  never: 'No stance lets Claude write a card’s rationale.',
  modes: [],
}

function report(over: Partial<ResearchReport> = {}): ResearchReport {
  return {
    answered_by: 'claude',
    mode: 'research',
    model: 'claude-sonnet-5',
    question: 'Is Goreclaw still played?',
    asked: true,
    reason: '',
    research: {
      answer: 'It is still a staple of green stompy lists.',
      findings: [
        { claim: 'Primers rate it a top-ten green commander.', source_ids: ['s1'] },
      ],
      cards: [
        {
          name: 'Goreclaw, Terror of Qal Sisma',
          in_pool: true,
          mana_cost: '{4}{G}',
          type_line: 'Legendary Creature — Bear',
          oracle_text: 'Creature spells you cast cost less to cast.',
          legal_commander: true,
        },
        { name: 'Sporeback Wurmcaller', in_pool: false },
      ],
      confidence: 'contested',
      sources: [{ id: 's1', title: 'A green primer', url: 'https://example.com/a' }],
      sources_dropped: 0,
      findings_dropped: 0,
      cards_unresolved: 1,
      searched: 6,
    },
    generated_at: '2026-08-14T12:00:00+00:00',
    never: 'This is Claude’s reading of the cited pages.',
    ...over,
  }
}

function job(result: unknown): Job {
  return {
    id: 'j1', kind: 'claude.research', status: 'done', done: 1, total: 1,
    percent: 100, label: 'research: …', result, error: null,
    created_at: '2026-08-14T12:00:00+00:00',
  }
}

function draw() {
  return render(<MemoryRouter><Research /></MemoryRouter>)
}

async function askSomething(text = 'Is Goreclaw still played?') {
  fireEvent.change(screen.getByLabelText('Your question'), {
    target: { value: text },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Ask' }))
}

beforeEach(() => {
  vi.mocked(api.claudeStatus).mockResolvedValue(STATUS)
  vi.mocked(api.research).mockResolvedValue(job(report()))
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('the deck boundary', () => {
  it('says on the page that it cannot see your decks', async () => {
    draw()
    // ADR 26 said before the first question rather than as an apology after
    // one. Matched on this file's wording, not the payload's `never`.
    expect(await screen.findByText(/It cannot see your decks/)).toBeTruthy()
  })

  it('never sends a deck with the question', async () => {
    draw()
    await askSomething()
    await waitFor(() => expect(api.research).toHaveBeenCalled())
    const sent = vi.mocked(api.research).mock.calls[0]?.[0] ?? {}
    // The assertion that fails when somebody adds deck awareness here. There
    // is no owner and no slug in the request because there is nowhere for one
    // to go — the mode cannot reach a library at all.
    expect(Object.keys(sent).sort()).toEqual(['question', 'stance'])
  })
})

describe('the stance readout', () => {
  it('asks for its own surface rather than letting the server answer off', async () => {
    draw()
    await waitFor(() => expect(api.claudeStatus).toHaveBeenCalled())
    expect(vi.mocked(api.claudeStatus).mock.calls[0]?.[0])
      .toMatchObject({ surface: 'research' })
  })
})

describe('the answer', () => {
  it('renders the answer with its confidence', async () => {
    draw()
    await askSomething()
    expect(await screen.findByText(/still a staple of green stompy/)).toBeTruthy()
    // The renderer's own gloss on the enum, not the enum echoed back.
    expect(screen.getByText(/informed people disagree/)).toBeTruthy()
  })

  it('links each finding to the page it came from', async () => {
    draw()
    await askSomething()
    expect(await screen.findByText(/top-ten green commander/)).toBeTruthy()
    expect(screen.getByTitle('A green primer').getAttribute('href'))
      .toBe('#src-s1')
    expect(screen.getByRole('link', { name: 'A green primer' })
      .getAttribute('href')).toBe('https://example.com/a')
  })

  it('states how many pages were read and how many were cited', async () => {
    draw()
    await askSomething()
    expect(await screen.findByText('Sources — 6 pages read, 1 cited')).toBeTruthy()
  })
})

describe('the two kinds of card fact', () => {
  it('shows the pool’s own text for a card the pool has', async () => {
    draw()
    await askSomething()
    expect(await screen.findByText('Goreclaw, Terror of Qal Sisma')).toBeTruthy()
    expect(screen.getByText(/Creature spells you cast cost less/)).toBeTruthy()
  })

  it('keeps a card the pool lacks, and says what that means', async () => {
    draw()
    await askSomething()
    // **Kept**, which is the difference from the dossier — a spoiler is the
    // question here, not an error.
    expect(await screen.findByText('Sporeback Wurmcaller')).toBeTruthy()
    // And labelled, in this file's words. Everything said about it upstream
    // rests on a page rather than on a lookup.
    expect(screen.getByText(/Not in the card pool/)).toBeTruthy()
  })

  it('does not file the unresolved count with the dropped counts', async () => {
    draw()
    await askSomething()
    await screen.findByText('Sporeback Wurmcaller')
    // `cards_unresolved` is 1 and both dropped counts are 0, so the "discarded"
    // line must be absent — filing them together would report a working
    // spoiler answer as a fault.
    expect(screen.queryByText(/Discarded before you saw it/)).toBeNull()
    expect(screen.getByText(/not in the pool yet/)).toBeTruthy()
  })

  it('shows the dropped counts when there are any', async () => {
    const thin = report()
    vi.mocked(api.research).mockResolvedValue(job({
      ...thin,
      research: { ...thin.research, sources_dropped: 2, findings_dropped: 1 },
    }))
    draw()
    await askSomething()
    expect(await screen.findByText(/Discarded before you saw it/)).toBeTruthy()
  })
})

describe('a refusal', () => {
  it('renders the reason rather than an empty page', async () => {
    vi.mocked(api.research).mockResolvedValue(job({
      ...report(),
      research: {},
      reason: 'No source survived checking, so there is nothing to stand '
            + 'behind an answer.',
    }))
    draw()
    await askSomething()
    expect(await screen.findByText(/No source survived checking/)).toBeTruthy()
    expect(screen.queryByText('Answer')).toBeNull()
  })
})

describe('when the surface is unavailable', () => {
  it('says a key is missing rather than disabling the box silently', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue({ ...STATUS, configured: false })
    draw()
    expect(await screen.findByText(/No API key is configured/)).toBeTruthy()
    fireEvent.change(screen.getByLabelText('Your question'), {
      target: { value: 'anything' },
    })
    expect(screen.getByRole('button', { name: 'Ask' })
      .hasAttribute('disabled')).toBe(true)
  })
})
