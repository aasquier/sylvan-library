/**
 * The reading room's dead end, and the way out of it.
 *
 * A theme turn can come back with **no question**. The server deletes an
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
 *
 * **Every sentence this screen shows is now in the document twice** — once for
 * the eye and once in the live region for the ear — which is why so much of
 * this file asks for `findAllByText`. That is the property rather than a
 * duplicate to be tolerated: a reader and a reader-of-screens must not be told
 * two different things ("the interview, said out loud" below).
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, ThemeReport } from '../lib/api'
import { ThemeInterview } from './theme'

vi.mock('../lib/api', async () => {
  const real = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ApiError: real.ApiError,
    // The real one: it is how a server's own sentence keeps its wording on the
    // way to the screen, and a stub here would make every assertion below
    // about the stub instead.
    errorMessage: real.errorMessage,
    // Stubbed rather than real: the actual `followJob` closes over the
    // module's own `api` binding, so importing it would reach past this mock
    // and poll for real. Its polling is pinned in `lib/api.test.ts`.
    followJob: vi.fn(),
    api: { themeAsk: vi.fn(), themePropose: vi.fn(), claudeStatus: vi.fn() },
  }
})

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

const { ApiError, api, followJob } = await import('../lib/api')
const { fetchClaudeStatus } = await import('../lib/stance')

/** The live region, wherever it is standing. */
function region(): HTMLElement | null {
  return document.querySelector('[role="status"].sr-only')
}

const STANCE = {
  preset: 'consultant', allows_calls: true, may_write: false, axes: [],
}

const STATUS = {
  installed: true, configured: true, model: 'claude-sonnet-5',
  stance: STANCE, ceiling: STANCE, default: STANCE, presets: [],
  never: 'One rule holds at every setting: Claude never writes a card’s rationale on its own. On an import you can ask it to draft the ones you have not written, and every sentence it drafts is marked as Claude’s until you rewrite it.', modes: [],
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

    await screen.findAllByText('Nothing usable came back.')
    // The old screen: this button, disabled, and nothing else to press.
    expect(screen.getByRole('button', { name: 'Answer' })
      .hasAttribute('disabled')).toBe(true)
    expect(screen.getByRole('button', { name: 'Try that again' })).toBeTruthy()
  })

  it('re-asks the same transcript, so nothing is said twice', async () => {
    answersWith(report({ question: '', reason: 'Nothing usable came back.' }))
    renderRoom()
    await screen.findAllByText('Nothing usable came back.')
    expect(api.themeAsk).toHaveBeenCalledTimes(1)

    answersWith(report({ question: 'What have you rewatched most?' }))
    fireEvent.click(screen.getByRole('button', { name: 'Try that again' }))

    await screen.findAllByText('What have you rewatched most?')
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

    await screen.findAllByText('Nothing usable came back.')
    // A count of questions asked, printed beside a question that never
    // arrived, reads as blaming the reader for not answering it.
    expect(screen.queryByText(/questions at most/)).toBeNull()
  })

  it('offers a retry when the turn threw, and says so', async () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(api.themeAsk).mockRejectedValue(new Error('Load failed'))
    renderRoom()

    // **Not "Load failed", which is what this screen used to print.** That is
    // a browser's own account of a connection, written for somebody with the
    // devtools open, and commandment 10 says no technology backing this site
    // ever renders — a fault that reaches a beginner as two English words
    // they cannot act on is that rule's whole point.
    await screen.findByText(/The question did not make it through/)
    expect(screen.queryByText('Load failed')).toBeNull()
    // Not "Starting…" either, which under an error describes the one thing
    // that is definitely not happening.
    expect(screen.getByText('That question did not arrive.')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Try that again' })).toBeTruthy()
    // And the diagnosis is redirected rather than deleted: it goes where the
    // person who can fix it is reading.
    expect(logged.mock.calls.flat().some(
      (arg) => String(arg).includes('Load failed'))).toBe(true)
    logged.mockRestore()
  })
})

/**
 * What the room says when something does not arrive (commandment 10).
 *
 * The split is worth pinning because it is a judgement rather than a rule with
 * one side: a refusal the *server* wrote is a sentence about the interview and
 * survives untouched, and anything that is really a fact about the machinery —
 * a run the server no longer holds, a connection that never landed — gets the
 * room's words and sends its own account to the console.
 */
describe('a failure a person reads', () => {
  it('keeps a refusal the server wrote, exactly as written', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(api.themePropose).mockRejectedValue(new ApiError(
      'that is as long as this conversation goes', 409))
    answersWith(report({
      question: 'And at the table?', may_propose: true, grounded: 3,
    }))
    renderRoom()

    fireEvent.click(await screen.findByRole('button', { name: 'Get my colours' }))
    // Nothing this file could put here would say more.
    expect(await screen.findByText('that is as long as this conversation goes'))
      .toBeTruthy()
    vi.mocked(console.error).mockRestore()
  })

  it('speaks for itself when the run is no longer there', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(api.themeAsk).mockRejectedValue(new ApiError('Not Found', 404))
    renderRoom()

    // A job lives in the server's memory and dies with it — which is true, and
    // is none of a player's business.
    await screen.findByText(/That question was lost on its way to you/)
    expect(screen.queryByText(/server/i)).toBeNull()
    vi.mocked(console.error).mockRestore()
  })
})

describe('while the proposal runs', () => {
  /* The clock lived only in the sidebar, and the sidebar stacks *below* the
     conversation on anything narrower than a laptop. Pressing the button made
     it disappear and left nothing on screen for the two-to-four minutes that
     followed — a working feature that looks exactly like a broken one. */
  it('says so in the column the button was in', async () => {
    answersWith(report({
      question: 'And at the table?', may_propose: true, grounded: 3,
      slots: [
        { kind: 'taste', value: 'fog', quote: 'fog' },
        { kind: 'temperament', value: 'planner', quote: 'planner' },
        { kind: 'posture', value: 'organiser', quote: 'organiser' },
      ],
    }))
    // Never resolves: the point is what shows while it is in flight.
    vi.mocked(api.themePropose).mockReturnValue(new Promise(() => {}) as never)
    renderRoom()

    fireEvent.click(await screen.findByRole('button', { name: 'Get my colours' }))
    // Two now — one where the button was, one in the sidebar it always had.
    expect(await screen.findAllByText(/Reading around…/)).toHaveLength(2)
  })
})

describe('a proposal that ran and came back empty', () => {
  /* The server's reason for an unparseable pour carries a wire token —
     "(stop reason: max_tokens)" — and that sentence is frozen by the crossing
     corpus, so the wire cannot change. The screen can: the room speaks its
     own sentence and the recorded reason goes to the console. `asked: false`
     reasons are the server explaining itself in a person's words and must
     keep passing through (the stance-off test above holds that half). */
  it('speaks in the room\'s words and sends the wire token to the console', async () => {
    const ask = job(report({
      question: 'And at the table?', may_propose: true, grounded: 3,
      slots: [
        { kind: 'taste', value: 'fog', quote: 'fog' },
        { kind: 'temperament', value: 'planner', quote: 'planner' },
        { kind: 'posture', value: 'organiser', quote: 'organiser' },
      ],
    }))
    const pour = {
      answered_by: 'claude', mode: 'theme-proposal', model: 'claude-sonnet-5',
      asked: true, reason: 'The answer did not parse (stop reason: max_tokens).',
      stance: STANCE, combinations: [], sources: [], slots: [], slots_dropped: 0,
    }
    const pourJob = {
      id: 'job-pour', kind: 'claude.theme.proposal', status: 'done', done: 1,
      total: 1, percent: 100, label: 'pour', result: pour, error: null,
      created_at: '2026-08-15T10:00:00+00:00',
    }
    // The first follow is the ask; every follow after it is the pour.
    vi.mocked(followJob)
      .mockReturnValueOnce({
        promise: Promise.resolve(ask) as never, cancel: () => {},
      } as never)
      .mockReturnValue({
        promise: Promise.resolve(pourJob) as never, cancel: () => {},
      } as never)
    vi.mocked(api.themePropose).mockResolvedValue({ id: 'job-pour' } as never)
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      renderRoom()
      fireEvent.click(await screen.findByRole('button', { name: 'Get my colours' }))
      // The room's own sentence, not the wire's.
      await screen.findByText('Nothing usable came back.')
      expect(screen.queryByText(/stop reason/)).toBeNull()
      expect(spy).toHaveBeenCalledWith(
        'the proposal came back empty:', pour.reason)
    } finally {
      spy.mockRestore()
    }
  })
})

describe('a turn that finished on purpose', () => {
  it('offers no retry when the stance is off', async () => {
    answersWith(report({
      asked: false, question: '',
      reason: 'The stance is off, so no call was made.',
    }))
    renderRoom()

    await screen.findAllByText('The stance is off, so no call was made.')
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

    await screen.findAllByText(/as long as this conversation goes/)
    expect(screen.queryByRole('button', { name: 'Try that again' })).toBeNull()
  })

  /* The dead end Aaron and his sister walked into: ten shy answers, nothing
     grounded, and a screen whose only working controls threw the evening
     away. The conversation ending is fine; ending it with a live answer box
     that returns the same sentence forever, and a disabled button beside it,
     is the part that reads as "you answered wrong". */
  it('closes the answer box and opens a door when the floor was never met',
     async () => {
    const leave = vi.fn()
    answersWith(report({
      asked: false, question: '', exchanges: 10, may_propose: false,
      reason: 'That is 10 exchanges, which is as long as this conversation goes.',
    }))
    render(<ThemeInterview onPick={() => {}} onLeave={leave} />)

    await screen.findAllByText(/as long as this conversation goes/)
    expect(screen.queryByPlaceholderText(/However much or little/)).toBeNull()
    expect(screen.queryByRole('button', { name: 'Answer' })).toBeNull()

    const out = screen.getByRole('button', { name: /Pick colours with me/ })
    fireEvent.click(out)
    expect(leave).toHaveBeenCalled()
  })

  it('keeps the proposal in reach when the ceiling arrives already ready',
     async () => {
    answersWith(report({
      asked: false, question: '', exchanges: 10, may_propose: true, grounded: 3,
      reason: 'That is 10 exchanges, which is as long as this conversation goes.',
    }))
    renderRoom()

    await screen.findAllByText(/as long as this conversation goes/)
    // The way out is for the case with nothing to read from; with three
    // grounded answers the reading is the way out.
    expect(screen.queryByRole('button', { name: /Pick colours with me/ })).toBeNull()
    expect((screen.getByRole('button', { name: 'Suggest my colours' }) as
      HTMLButtonElement).disabled).toBe(false)
  })
})


/**
 * The interview, said out loud.
 *
 * Every sentence on this screen arrives after a wait of seconds and lands in
 * the same place, which is the exact shape a live region exists for — and this
 * component had none at all. The tarot table next door has had one since
 * green's pass, and the mechanism it needs is not the sentence: it is the
 * region being in the document *before* the sentence appears. A region that
 * mounts with its text already in it is initial content, which readers do not
 * announce, so it is declared above the early returns and rendered first in
 * every branch. That is what the first test here is really pinning.
 */
describe('the interview, said out loud', () => {
  it('has its region up before the question lands, not with it', async () => {
    // Held open, so the room is caught mid-question rather than after it.
    let land: (job: unknown) => void = () => {}
    vi.mocked(api.themeAsk).mockReturnValue(
      new Promise((resolve) => { land = resolve }) as never)
    answersWith(report({ question: 'What have you rewatched most?' }))
    renderRoom()

    // Standing, and silent, while the question is being written.
    await screen.findByText('Thinking…')
    const standing = region()
    expect(standing).not.toBeNull()
    expect(standing!.textContent).toBe('')

    land(job(report()))
    await screen.findAllByText('What have you rewatched most?')
    // The very same node, now carrying the question.
    expect(region()).toBe(standing)
    expect(region()!.textContent).toBe('What have you rewatched most?')
  })

  it('says the banner and the dropped readings in their own words', async () => {
    answersWith(report({
      question: 'And at the table?', may_propose: true, grounded: 3,
      slots_dropped: 2,
    }))
    renderRoom()

    await screen.findAllByText('And at the table?')
    // Never a second wording: each of these is a line the eye can read too.
    expect(region()!.textContent).toBe(
      'And at the table? That’s enough to go on. '
      + '2 readings did not match anything you said, and were dropped.')
  })
})

/**
 * One room, one voice — the contradiction `lib/costumes.ts` was built to
 * delete.
 *
 * The asking line keyed on the persona and the proposing line on whether a
 * spread had been dealt, so the fortune-teller's own table said "Reading the
 * cards… 14s" in the column and "Reading around… 14s" in the sidebar at the
 * same moment. Two voices, one room, four inches apart.
 */
describe('a costumed room', () => {
  it('says one thing in both places while it reads', async () => {
    answersWith(report({
      question: 'And at the table?', may_propose: true, grounded: 3,
    }))
    // Never resolves: the point is what shows while it is in flight.
    vi.mocked(api.themePropose).mockReturnValue(new Promise(() => {}) as never)
    render(<ThemeInterview onPick={() => {}} onLeave={() => {}}
                           persona="fortune-teller" seed={7} />)

    fireEvent.click(await screen.findByRole('button', { name: 'Read my cards' }))
    // The column's banner and the sidebar's button, in the reader's words.
    expect(await screen.findAllByText(/Reading the cards…/)).toHaveLength(2)
    expect(screen.queryByText(/Reading around…/)).toBeNull()
  })

  it('says one thing in both places in the hut too', async () => {
    // The second costumed room, and the same property: the banner in the
    // column and the button in the sidebar read one field, so a room cannot
    // answer in two voices however many rooms there are.
    answersWith(report({
      question: 'And at the table?', may_propose: true, grounded: 3,
    }))
    vi.mocked(api.themePropose).mockReturnValue(new Promise(() => {}) as never)
    render(<ThemeInterview onPick={() => {}} onLeave={() => {}}
                           persona="witch" seed={7} />)

    fireEvent.click(await screen.findByRole('button', { name: 'Pour it out' }))
    expect(await screen.findAllByText(/Bringing it up to a boil…/))
      .toHaveLength(2)
    expect(screen.queryByText(/Reading around…/)).toBeNull()
  })

  it('leaves the plain rooms exactly as they were', async () => {
    answersWith(report({ question: 'What have you rewatched most?' }))
    render(<ThemeInterview onPick={() => {}} onLeave={() => {}}
                           persona="therapist" />)

    await screen.findAllByText('What have you rewatched most?')
    // No costume: the plain answer box, and the question printed rather than
    // written. Six of the eight voices are undressed and every one of them
    // gets exactly this.
    expect(screen.getByPlaceholderText(/However much or little/)).toBeTruthy()
    expect(document.querySelector('.seance-scroll')).toBeNull()
    expect(document.querySelector('.cauldron-slate')).toBeNull()
    expect(document.querySelector('.ink-text')).toBeNull()
  })
})

describe('who a fun fact is credited to', () => {
  /**
   * Three origins reach `FactNote` and the old code could only tell two
   * apart. It keyed on `url`, so anything without one was captioned "From
   * this tool's own colour reference data" — true of a taxonomy fact and
   * flatly false of the fortune-teller's deck history, which arrives with a
   * real book citation and no link. Adding the tarot corpus would have
   * credited Pamela Colman Smith to a table of Magic colours.
   */
  const withFact = (fact: ThemeReport['fact']) =>
    report({ question: 'What draws you in?', fact })

  it('links a fact that came from a page', async () => {
    answersWith(withFact({
      text: 'A thing a page said.', source: 'A Page',
      url: 'https://example.test/a',
    } as NonNullable<ThemeReport['fact']>))
    renderRoom()

    const link = await screen.findByRole('link', { name: 'A Page' })
    expect(link.getAttribute('href')).toBe('https://example.test/a')
  })

  it('names the colour data when that is what it was', async () => {
    answersWith(withFact({
      text: 'Selesnya is white-green.', source: 'taxonomy', url: '',
    } as NonNullable<ThemeReport['fact']>))
    renderRoom()

    expect(await screen.findByText(/colour reference data/)).toBeTruthy()
  })

  it('prints a real citation instead, and never calls it colour data',
     async () => {
    answersWith(withFact({
      text: 'Pamela Colman Smith drew all 78 cards and was paid a flat fee.',
      source: 'Kaplan and others, Pamela Colman Smith: The Untold Story',
      url: '',
    } as NonNullable<ThemeReport['fact']>))
    renderRoom()

    expect(await screen.findByText(/Kaplan and others/)).toBeTruthy()
    expect(screen.queryByText(/colour reference data/)).toBeNull()
  })
})
