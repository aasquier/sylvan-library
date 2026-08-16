/**
 * The reading room's dead end, and the way out of it.
 *
 * A theme turn can come back with **no question**. `theme.py` deletes an
 * answer that does not end in a question mark — a declarative sentence here is
 * the mode telling somebody what they think instead of asking — and it reports
 * the same empty question when the JSON does not parse or the model declines.
 * Measured at roughly one opening turn in six.
 *
 * The screen that produced was unrecoverable: the reason rendered where the
 * question goes, Answer was disabled because there was nothing to answer, and
 * the auto-ask effect would not fire again because `awaited` already held this
 * transcript length. The only control left was "Start over", which throws away
 * the conversation to fix one bad turn.
 *
 * So the properties here are: a blanked turn offers a retry, the retry re-asks
 * **the same transcript** (a failed turn appends nothing, so nothing is said
 * twice), and the two states that report a reason and are *not* retryable —
 * the stance being off, and the exchange ceiling — do not offer one, because
 * asking again would get the same answer.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, ThemeReport } from '../lib/api'
import { ThemeInterview } from './theme'

vi.mock('../lib/api', async () => ({
  ApiError: (await vi.importActual<typeof import('../lib/api')>(
    '../lib/api')).ApiError,
  // Stubbed rather than real: the actual `followJob` closes over the module's
  // own `api` binding, so importing it would reach past this mock and poll for
  // real. Its polling is pinned in `lib/api.test.ts`.
  followJob: vi.fn(),
  api: { themeAsk: vi.fn(), themePropose: vi.fn(), claudeStatus: vi.fn() },
}))

vi.mock('../lib/stance', async () => {
  const actual = await vi.importActual<typeof import('../lib/stance')>(
    '../lib/stance')
  return {
    ...actual,
    // The status fetch is the gate every Claude panel opens on. Stubbed so
    // these tests are about the turn, not about the dial.
    fetchClaudeStatus: vi.fn(),
  }
})

const { api, followJob } = await import('../lib/api')
const { fetchClaudeStatus } = await import('../lib/stance')

const STANCE = {
  preset: 'consultant', allows_calls: true, may_write: false, axes: [],
}

const STATUS = {
  installed: true, configured: true, model: 'claude-sonnet-5',
  stance: STANCE, ceiling: STANCE, default: STANCE, presets: [],
  never: 'No stance lets Claude write a card’s rationale.', modes: [],
} as unknown as ClaudeStatus

function report(over: Partial<ThemeReport> = {}): ThemeReport {
  return {
    answered_by: 'claude', mode: 'theme-conversation', model: 'claude-sonnet-5',
    asked: true, reason: '', stance: STANCE as ThemeReport['stance'],
    persona: 'plain', question: '', fact: null, slots: [], slots_dropped: 0,
    grounded: 0, floor: 3, may_propose: false, exchanges: 0, max_exchanges: 10,
    usage: { input_tokens: 10, output_tokens: 10 },
    ...over,
  } as ThemeReport
}

/** A finished job carrying a report — the shape `followJob` resolves with. */
const job = (result: ThemeReport) => ({
  id: 'job-theme', kind: 'claude.theme', status: 'done', done: 1, total: 1,
  percent: 100, label: 'theme', result, error: null,
  created_at: '2026-08-15T10:00:00+00:00',
})

/** The next turn the component will get. */
function answersWith(result: ThemeReport) {
  vi.mocked(followJob).mockReturnValue({
    promise: Promise.resolve(job(result)) as never,
    cancel: () => {},
  })
}

function renderRoom() {
  return render(<ThemeInterview onPick={() => {}} onLeave={() => {}} />)
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(fetchClaudeStatus).mockReset().mockResolvedValue(STATUS)
  vi.mocked(api.themeAsk).mockReset().mockResolvedValue(
    job(report()) as never)
  vi.mocked(api.themePropose).mockReset()
  answersWith(report({ question: 'What have you rewatched most?' }))
})

afterEach(cleanup)

describe('a turn that produced no question', () => {
  it('offers a retry instead of a dead end', async () => {
    answersWith(report({ question: '', reason: 'Nothing usable came back.' }))
    renderRoom()

    await screen.findByText('Nothing usable came back.')
    // The old screen: this button, disabled, and nothing else to press.
    expect(screen.getByRole('button', { name: 'Answer' })
      .hasAttribute('disabled')).toBe(true)
    expect(screen.getByRole('button', { name: 'Try that again' })).toBeTruthy()
  })

  it('re-asks the same transcript, so nothing is said twice', async () => {
    answersWith(report({ question: '', reason: 'Nothing usable came back.' }))
    renderRoom()
    await screen.findByText('Nothing usable came back.')
    expect(api.themeAsk).toHaveBeenCalledTimes(1)

    answersWith(report({ question: 'What have you rewatched most?' }))
    fireEvent.click(screen.getByRole('button', { name: 'Try that again' }))

    await screen.findByText('What have you rewatched most?')
    expect(api.themeAsk).toHaveBeenCalledTimes(2)
    // A failed turn appends no assistant turn, so the retry sends exactly what
    // the first attempt sent — an empty conversation, at the opening question.
    for (const call of vi.mocked(api.themeAsk).mock.calls) {
      expect(call[0].transcript).toEqual([])
    }
    // And once a question arrives the way out is gone again.
    expect(screen.queryByRole('button', { name: 'Try that again' })).toBeNull()
  })

  it('drops the question counter while there is no question', async () => {
    answersWith(report({ question: '', reason: 'Nothing usable came back.',
                         exchanges: 2 }))
    renderRoom()

    await screen.findByText('Nothing usable came back.')
    // A count of questions asked, printed beside a question that never
    // arrived, reads as blaming the reader for not answering it.
    expect(screen.queryByText(/questions at most/)).toBeNull()
  })

  it('offers a retry when the turn threw, and says so', async () => {
    vi.mocked(api.themeAsk).mockRejectedValue(new Error('Load failed'))
    renderRoom()

    await screen.findByText('Load failed')
    // Not "Starting…", which under a red error line describes the one thing
    // that is definitely not happening.
    expect(screen.getByText('That question did not arrive.')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Try that again' })).toBeTruthy()
  })
})

describe('a turn that finished on purpose', () => {
  it('offers no retry when the stance is off', async () => {
    answersWith(report({
      asked: false, question: '',
      reason: 'The stance is off, so no call was made.',
    }))
    renderRoom()

    await screen.findByText('The stance is off, so no call was made.')
    // Nothing was asked, so asking again gets the same answer. The fix is the
    // stance menu, and a retry button here would point away from it.
    expect(screen.queryByRole('button', { name: 'Try that again' })).toBeNull()
  })

  it('offers no retry at the exchange ceiling', async () => {
    answersWith(report({
      asked: false, question: '', exchanges: 10,
      reason: 'That is 10 exchanges, which is as long as this conversation goes.',
    }))
    renderRoom()

    await screen.findByText(/as long as this conversation goes/)
    expect(screen.queryByRole('button', { name: 'Try that again' })).toBeNull()
  })
})
