/**
 * Agatha's room, and the one claim it has to keep: **the pot reports the
 * grounding, it never performs it.**
 *
 * Everything the cauldron draws is derived from `slots` — the readiness
 * instrument's own answer, checked server-side against the querent's words —
 * so the properties worth pinning are the ones where a future edit could
 * quietly give the pot a second opinion: a socket that fills without a slot, a
 * count that is not the count of grounded kinds, a ready state that arrives
 * from somewhere other than the floor being met.
 *
 * The rest of this file is about what a person is actually told. Every reveal
 * in this room exists twice, once in colour and once in words, because a
 * socket that only goes green is a socket half the room cannot read — and the
 * live region and the printed caption must never be two wordings of the same
 * beat.
 *
 * jsdom computes no layout and no cascade, so nothing here is about how the
 * hut looks; commandment 14 is unambiguous that a green suite has not seen the
 * page. What it can see is the DOM, and the DOM is where the state lives.
 */

import { cleanup, fireEvent, render, screen, waitFor }
  from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { BrewReading, ClaudeStatus, ThemeReport, ThemeSlot } from '../lib/api'
import { ThemeInterview } from './theme'

vi.mock('../lib/api', async () => {
  const real = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return {
    ApiError: real.ApiError,
    errorMessage: real.errorMessage,
    brewsACauldron: real.brewsACauldron,
    // Stubbed rather than real, `theme.test.tsx`'s reason: the actual
    // `followJob` closes over the module's own `api` binding and would poll
    // past this mock.
    followJob: vi.fn(),
    api: { themeAsk: vi.fn(), themePropose: vi.fn(), claudeStatus: vi.fn() },
  }
})

vi.mock('../lib/stance', async () => {
  const actual = await vi.importActual<typeof import('../lib/stance')>(
    '../lib/stance')
  return { ...actual, fetchClaudeStatus: vi.fn() }
})

const { api, followJob } = await import('../lib/api')
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
  never: 'One rule holds at every setting.', modes: [],
} as unknown as ClaudeStatus

/** The pot, as `/api/brew/reading` fills it: one ingredient per place, in the
 *  order `brew.Order` lays them out. Plural and singular both, because half
 *  the real shelf is each and the room's copy has to survive both. */
const POT: BrewReading = {
  seed: 7,
  ingredients: [
    { key: 'rose-hips', name: 'Rose hips', slot: 'taste',
      position: 'The Base',
      note: 'go in whole and turn the water the colour of a bruise' },
    { key: 'wolfsbane', name: 'Wolfsbane', slot: 'temperament',
      position: 'The Heat', note: 'the hood of it comes off in the boil' },
    { key: 'mugwort', name: 'Mugwort', slot: 'posture',
      position: 'The Binding', note: 'holds whatever else is in there' },
  ],
}

function slot(kind: string, value: string): ThemeSlot {
  return { kind, value, quote: value } as ThemeSlot
}

function report(over: Partial<ThemeReport> = {}): ThemeReport {
  return {
    answered_by: 'claude', mode: 'theme-conversation', model: 'claude-sonnet-5',
    asked: true, reason: '', stance: STANCE as ThemeReport['stance'],
    persona: 'witch', question: '', fact: null, slots: [], slots_dropped: 0,
    grounded: 0, floor: 3, may_propose: false, exchanges: 0, max_exchanges: 10,
    usage: { input_tokens: 10, output_tokens: 10 },
    ...over,
  } as ThemeReport
}

const job = (result: ThemeReport) => ({
  id: 'job-theme', kind: 'claude.theme', status: 'done', done: 1, total: 1,
  percent: 100, label: 'theme', result, error: null,
  created_at: '2026-09-12T10:00:00+00:00',
})

/** The next turn the room will get. */
function answersWith(result: ThemeReport) {
  vi.mocked(followJob).mockReturnValue({
    promise: Promise.resolve(job(result)) as never,
    cancel: () => {},
  })
}

function hut(pot: BrewReading | null = POT) {
  return render(
    <ThemeInterview onPick={() => {}} onLeave={() => {}} persona="witch"
                    seed={7} pot={pot} />)
}

/** Answer the question in front of you, which is what moves the pot. */
async function answer(said: string, next: ThemeReport) {
  answersWith(next)
  const box = await screen.findByLabelText('Your answer')
  fireEvent.change(box, { target: { value: said } })
  fireEvent.click(screen.getByRole('button', { name: 'Answer' }))
}

function sockets(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>('.cauldron-socket')]
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(fetchClaudeStatus).mockReset().mockResolvedValue(STATUS)
  vi.mocked(api.themeAsk).mockReset().mockResolvedValue(job(report()) as never)
  vi.mocked(api.themePropose).mockReset()
  answersWith(report({ question: 'What do you always take too much of?' }))
})

afterEach(cleanup)

describe('the three places', () => {
  it('are named before anything is in them, and say so in words', async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    const [base, heat, binding] = sockets()
    // The place is always named: a newcomer can see there are three, what
    // each is called, and that one is still to come.
    expect(base?.textContent).toContain('The Base')
    expect(heat?.textContent).toContain('The Heat')
    expect(binding?.textContent).toContain('The Binding')
    // And what is missing is missing in words, not only in colour.
    for (const s of sockets()) {
      expect(s.className).not.toContain('is-in')
      expect(s.textContent).toContain('still to come')
      expect(s.textContent).not.toContain('Rose hips')
    }
  })

  it('carry the ingredient once its slot is grounded, in the pot order',
     async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    await answer('horror films', report({
      question: 'And when it goes wrong?',
      slots: [slot('taste', 'horror films')], grounded: 1,
    }))
    await waitFor(() => {
      expect(sockets()[0]?.className).toContain('is-in')
    })
    const [base, heat] = sockets()
    expect(base?.textContent).toContain('Rose hips')
    // The note rides along as a title: flavour for a pointer that stops,
    // never a thing you must hover to understand.
    expect(base?.getAttribute('title')).toContain('the colour of a bruise')
    // And the other two have not moved. A pot that filled a socket the
    // transcript did not ground would be the whole failure this room is
    // written around.
    expect(heat?.className).not.toContain('is-in')
    expect(heat?.getAttribute('title')).toBeNull()
  })

  it('are a readout and not a control', async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    // Commandment 20's test, both halves: a socket does not take you anywhere
    // and it does not change what is in front of you, so it is neither a
    // button nor a link.
    for (const s of sockets()) {
      expect(s.querySelector('button')).toBeNull()
      expect(s.querySelector('a')).toBeNull()
      expect(s.tagName).toBe('LI')
    }
  })
})

describe('what the room says out loud', () => {
  it('announces the landing led by the place, and prints the same string',
     async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    await answer('horror films', report({
      question: 'And when it goes wrong?',
      slots: [slot('taste', 'horror films')], grounded: 1,
    }))
    const line = 'The Base takes the rose hips. One of three — the brew '
      + 'darkens and thickens.'
    // Said...
    await waitFor(() => {
      expect(region()?.textContent).toContain(line)
    })
    // ...and printed, in exactly the same words. Never a second wording.
    expect(document.querySelector('.cauldron-beat-line')?.textContent)
      .toBe(line)
    // The question rides with it: the beat is what just happened, the
    // question is what is being asked now, and a reader gets both.
    expect(region()?.textContent).toContain('And when it goes wrong?')
  })

  it('counts the second and third landings, and names the colour each time',
     async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    await answer('a', report({
      question: 'Q2', slots: [slot('taste', 'a')], grounded: 1,
    }))
    await waitFor(() => expect(sockets()[0]?.className).toContain('is-in'))
    await answer('b', report({
      question: 'Q3',
      slots: [slot('taste', 'a'), slot('temperament', 'b')], grounded: 2,
    }))
    await waitFor(() => {
      expect(region()?.textContent).toContain('The Heat takes the wolfsbane. '
        + 'Two of three — the brew turns from brown to a sickly olive.')
    })
    await answer('c', report({
      question: 'Q4', may_propose: true, grounded: 3,
      slots: [slot('taste', 'a'), slot('temperament', 'b'),
              slot('posture', 'c')],
    }))
    await waitFor(() => {
      expect(region()?.textContent).toContain('The Binding takes the mugwort. '
        + 'Three of three — it comes up bright green and settles. '
        + 'Ready to pour.')
    })
  })

  it('carries the pot’s whole state on the strip, for moving to rather '
     + 'than only for hearing', async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    const strip = screen.getByRole('img', { name: /The pot\./ })
    // The same words the sockets print, never a second wording of them.
    expect(strip.getAttribute('aria-label')).toBe(
      'The pot. The Base: still to come. The Heat: still to come. '
      + 'The Binding: still to come.')
    await answer('horror films', report({
      question: 'And?', slots: [slot('taste', 'horror films')], grounded: 1,
    }))
    await waitFor(() => {
      expect(screen.getByRole('img', { name: /The pot\./ })
        .getAttribute('aria-label')).toBe(
        'The pot. The Base: Rose hips. The Heat: still to come. '
        + 'The Binding: still to come.')
    })
  })
})

describe('the brew', () => {
  it('steps a level per grounded slot, and never past the number of slots',
     async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    const window = () => document.querySelector('.cauldron-strip')
    expect(window()?.getAttribute('data-level')).toBe('0')
    await answer('a', report({
      question: 'Q2', slots: [slot('taste', 'a')], grounded: 1,
    }))
    await waitFor(() => {
      expect(window()?.getAttribute('data-level')).toBe('1')
    })
    // `anchor` is ADR 20's fourth kind and the pot has no place for it, so a
    // fourth grounded slot must not turn into a fourth ingredient.
    await answer('b', report({
      question: 'Q3', grounded: 2,
      slots: [slot('taste', 'a'), slot('anchor', 'b')],
    }))
    await waitFor(() => {
      expect(region()?.textContent).toContain('Q3')
    })
    expect(window()?.getAttribute('data-level')).toBe('1')
  })

  it('comes up ready a breath after the third lands, and the pour lights up',
     async () => {
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    await answer('a', report({
      question: 'Q', may_propose: true, grounded: 3,
      slots: [slot('taste', 'a'), slot('temperament', 'b'),
              slot('posture', 'c')],
    }))
    // The banner is the interview's and arrives with the report; the pot's
    // own ready state waits for the colour to land, which is the whole
    // argument for the beat.
    const banner = await screen.findAllByText(
      'Three things in the pot, and it has stopped arguing with itself.')
    expect(banner.length).toBeGreaterThan(0)
    await waitFor(() => {
      expect(document.querySelector('.cauldron-strip')?.className)
        .toContain('is-ready')
    })
    // The one action the room is for wears the room's own tailoring, not the
    // page's accent button (commandment 17, and `costume.action`).
    const pour = screen.getByRole('button', { name: 'Pour it out' })
    expect(pour.className).toContain('btn-copper-hot')
    expect(pour.getAttribute('style')).toBeNull()
  })

  it('does not replay three drops for a conversation restored from a stash',
     async () => {
    // The stash `theme.tsx` would have written: three slots already grounded,
    // under the same voice and the same seed.
    localStorage.setItem('mtglab-theme-conversation', JSON.stringify({
      persona: 'witch', seed: 7, job: null, proposal: null, facts: [],
      transcript: [{ role: 'assistant', text: 'Where were we?' }],
      slots: [slot('taste', 'a'), slot('temperament', 'b'),
              slot('posture', 'c')],
    }))
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    // Seated, not dropped: the room opens at its level with everything in it
    // and nothing in the air. `tarot.tsx`'s `turnedHere` is the precedent —
    // only a grounding that happens here, in front of somebody, gets its arc.
    for (const s of sockets()) expect(s.className).toContain('is-in')
    expect(document.querySelector('.cauldron-toss')).toBeNull()
    expect(document.querySelector('.cauldron-beat-line')).toBeNull()
  })
})

describe('the pour', () => {
  it('runs on the plate and says so, before the reading is worked out',
     async () => {
    vi.mocked(api.themePropose).mockResolvedValue(
      { id: 'job-propose' } as never)
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    await answer('a', report({
      question: 'Q', may_propose: true, grounded: 3,
      slots: [slot('taste', 'a'), slot('temperament', 'b'),
              slot('posture', 'c')],
    }))
    const pour = await screen.findByRole('button', { name: 'Pour it out' })
    fireEvent.click(pour)
    await waitFor(() => {
      expect(document.querySelector('.cauldron-strip')?.className)
        .toContain('is-serving')
    })
    expect(region()?.textContent)
      .toContain('Pouring. She reads around while it cools.')
  })

  it('hands the potion over in words, and stops reciting what has gone',
     async () => {
    vi.mocked(api.themePropose).mockResolvedValue(
      { id: 'job-propose' } as never)
    hut()
    await waitFor(() => expect(sockets()).toHaveLength(3))
    await answer('a', report({
      question: 'The question that will not be on screen any more',
      may_propose: true, grounded: 3,
      slots: [slot('taste', 'a'), slot('temperament', 'b'),
              slot('posture', 'c')],
    }))
    const pour = await screen.findByRole('button', { name: 'Pour it out' })
    // The proposal, when the job it started comes back.
    vi.mocked(followJob).mockReturnValue({
      promise: Promise.resolve({
        id: 'job-propose', kind: 'claude.theme', status: 'done', done: 1,
        total: 1, percent: 100, label: 'theme', error: null,
        created_at: '2026-09-12T10:00:00+00:00',
        result: {
          combinations: [], sources: [], reason: 'nothing usable', never: '',
          searched: 0, commanders_dropped: 0, combinations_dropped: 0,
          sources_dropped: 0,
        },
      }) as never,
      cancel: () => {},
    })
    fireEvent.click(pour)
    const poured = 'Poured. Three of them on the bench — take the one you '
      + 'want to carry.'
    await waitFor(() => {
      expect(region()?.textContent).toBe(poured)
    })
    // Said, and on the page in the same words.
    expect(screen.getAllByText(poured).length).toBeGreaterThan(1)
    // And the question is gone from the region with the slate it was on: a
    // sentence an eye cannot find is the one thing this region may not say.
    expect(region()?.textContent)
      .not.toContain('The question that will not be on screen any more')
  })
})

describe('a room with nothing on its table', () => {
  it('has no pot, no sockets and no line', async () => {
    hut(null)
    // `findAll`, because every sentence this screen shows is in the document
    // twice — once for the eye and once in the live region for the ear, which
    // is the property rather than a duplicate to be tolerated.
    await screen.findAllByText('What do you always take too much of?')
    expect(sockets()).toHaveLength(0)
    expect(document.querySelector('.cauldron-strip')).toBeNull()
    expect(document.querySelector('.cauldron-beat-line')).toBeNull()
  })
})
