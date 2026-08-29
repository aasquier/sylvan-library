/**
 * The bulk edit panel.
 *
 * jsdom has no layout engine, so nothing here can prove the plan is legible,
 * that a chip fits on a phone or that the rise animation lands — that is
 * Aaron's walk. What it can hold is the wiring and the copy, and for this
 * feature both are load-bearing:
 *
 *   - the preview writes nothing, so the confirm cannot be reached without one
 *   - the plan the confirm carries back is the plan that was on screen
 *   - the burials are named, and the copy says they can be brought back
 *   - a plan that changes nothing, or one with a blocked card, offers no
 *     confirm at all rather than a control that would only refuse
 */
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { BulkApplied, BulkPlan, BulkPreview } from '../lib/api'

vi.mock('../lib/api', async (importOriginal) => {
  const real = await importOriginal<typeof import('../lib/api')>()
  return { ...real, api: { ...real.api, bulkEdit: vi.fn() } }
})

const { api } = await import('../lib/api')
const { BulkEditPanel } = await import('./bulkedit')
const call = vi.mocked(api.bulkEdit)

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const ref = { owner: 'local', slug: 'a-deck' }

function plan(over: Partial<BulkPlan> = {}): BulkPlan {
  return {
    basis: 'the-deck-as-it-was',
    draft: false,
    add: [],
    rewrite: [],
    requantify: [],
    entomb: [],
    unchanged: [],
    merged: [],
    left: [],
    blocked: [],
    ...over,
  }
}

function preview(over: Partial<BulkPreview> = {}): BulkPreview {
  return {
    slug: 'a-deck',
    dry_run: true,
    plan: plan(),
    ready: true,
    unknown: [],
    did_you_mean: [],
    did_you_mean_skipped: 0,
    read: [],
    unreadable: [],
    skipped: [],
    outside: [],
    notes: [],
    ...over,
  }
}

function applied(): BulkApplied {
  return {
    slug: 'a-deck', stage: 'curated', total_cards: 99, needs_rationale: 0,
    ok: true, errors: [], warnings: [],
    plan: plan(), entombed: [],
    unknown: [], did_you_mean: [], did_you_mean_skipped: 0, read: [],
    unreadable: [], skipped: [], outside: [], notes: [],
  }
}

/** Open the panel, paste a list, and ask for the plan. */
async function look(answer: BulkPreview, onDone = vi.fn()) {
  call.mockResolvedValueOnce(answer)
  render(<BulkEditPanel deck={ref} onDone={onDone} />)
  fireEvent.click(screen.getByRole('button', { name: /Rewrite the 99/ }))
  fireEvent.change(screen.getByLabelText('The 99'),
    { target: { value: '1 Sol Ring "mine"\n' } })
  fireEvent.click(screen.getByRole('button', { name: /Show me what would change/ }))
  await screen.findByText('What this would do')
  return onDone
}

it('says nothing is deleted before anything is pasted', () => {
  render(<BulkEditPanel deck={ref} onDone={vi.fn()} />)
  fireEvent.click(screen.getByRole('button', { name: /Rewrite the 99/ }))
  // The one sentence a newcomer has to read before they press anything.
  expect(screen.getByText(/No card is ever deleted/)).toBeTruthy()
  expect(screen.getByText(/graveyard/)).toBeTruthy()
})

it('asks for a plan without writing, and only then offers the write', async () => {
  await look(preview({
    plan: plan({ entomb: ['Cultivate'], rewrite: [
      { name: 'Sol Ring', was: 'the old reason', why: 'mine', was_drafted: false },
    ] }),
  }))
  // The first call is the dry run, and it is the only call so far.
  expect(call).toHaveBeenCalledTimes(1)
  expect(call.mock.calls[0]?.[1]).toMatchObject({ dry_run: true })
  expect(call.mock.calls[0]?.[1]).not.toHaveProperty('basis')
})

it('shows the sentence leaving beside the sentence arriving', async () => {
  await look(preview({
    plan: plan({
      rewrite: [{
        name: 'Sol Ring', was: 'Two mana on turn one.', why: 'Still the best.',
        was_drafted: true,
      }],
    }),
  }))
  // Both halves, because a count of rewrites is a warning and the two
  // sentences are a decision.
  expect(screen.getByText(/Two mana on turn one\./)).toBeTruthy()
  expect(screen.getByText(/Still the best\./)).toBeTruthy()
  // And whose sentence is being replaced (ADR 41).
  expect(screen.getByText(/Claude drafted the old one/)).toBeTruthy()
})

it('names every card going to the graveyard and says they come back', async () => {
  await look(preview({ plan: plan({ entomb: ['Cultivate', 'Rampant Growth'] }) }))
  expect(screen.getByText('Cultivate')).toBeTruthy()
  expect(screen.getByText('Rampant Growth')).toBeTruthy()
  expect(screen.getByText(/bringing one back is one click/i)).toBeTruthy()
})

/** The interaction the preview exists for: a line the pool could not read is
 *  also a line that did not match, so the card it meant is in the burial list.
 *  Saying the two apart would be complete and silent. */
it('warns about unread lines where the burials are listed', async () => {
  await look(preview({
    plan: plan({ entomb: ['Sol Ring'] }),
    unknown: ['Sol Rin g'],
    did_you_mean: [{ written: 'Sol Rin g', candidates: [{ name: 'Sol Ring', score: 0.94 }] }],
  }))
  expect(screen.getByText(/not\s+read as a card/)).toBeTruthy()
  expect(screen.getByText(/fix the\s+spelling and look again rather than burying it/)).toBeTruthy()
})

it('carries the plan it showed back to the server on the confirm', async () => {
  const onDone = await look(preview({ plan: plan({ entomb: ['Cultivate'] }) }))
  call.mockResolvedValueOnce(applied())
  fireEvent.click(screen.getByRole('button', { name: /^Yes —/ }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(call).toHaveBeenCalledTimes(2)
  expect(call.mock.calls[1]?.[1]).toMatchObject({ basis: 'the-deck-as-it-was' })
  expect(call.mock.calls[1]?.[1]).not.toMatchObject({ dry_run: true })
})

it('offers no write when the plan changes nothing', async () => {
  await look(preview({ plan: plan({ unchanged: ['Sol Ring'] }), ready: false }))
  expect(screen.getByText(/already matches this deck/)).toBeTruthy()
  const button = screen.getByRole('button', { name: /Nothing to change/ })
  expect(button.hasAttribute('disabled')).toBe(true)
})

it('refuses the write while a card is blocked, and says what to type', async () => {
  await look(preview({
    ready: false,
    plan: plan({
      add: [],
      blocked: [{ name: 'Llanowar Reborn', reason: 'is new to a curated deck' }],
    }),
  }))
  expect(screen.getByText(/Llanowar Reborn/)).toBeTruthy()
  expect(screen.getByText(/Add one in quotes at the end of each line/)).toBeTruthy()
  const button = screen.getByRole('button', { name: /Nothing to change/ })
  expect(button.hasAttribute('disabled')).toBe(true)
})

/** The stale-deck answer, driven the way a person meets it: a plan on screen,
 *  a refusal from the server, and the plan taken away so the same button
 *  cannot be pressed again against a deck that has moved. */
it('drops the plan when the server says the deck moved', async () => {
  const onDone = await look(preview({ plan: plan({ entomb: ['Cultivate'] }) }))
  call.mockRejectedValueOnce(new Error(
    'this deck changed while the plan was on screen, so nothing was written.'))
  fireEvent.click(screen.getByRole('button', { name: /^Yes —/ }))
  await screen.findByText(/changed while the plan was on screen/)
  expect(onDone).not.toHaveBeenCalled()
  expect(screen.queryByText('What this would do')).toBeNull()
})

/** Editing the box after a plan has been shown takes the plan away. A plan
 *  describing the previous paste is worse than no plan at all. */
it('forgets the plan when the list is edited', async () => {
  await look(preview({ plan: plan({ entomb: ['Cultivate'] }) }))
  fireEvent.change(screen.getByLabelText('The 99'),
    { target: { value: '1 Sol Ring "mine"\n1 Cultivate "kept"\n' } })
  expect(screen.queryByText('What this would do')).toBeNull()
})

/** A line under a `Commander` or `SIDEBOARD` heading is somebody's card and is
 *  told it was not applied, rather than vanishing. */
it('reports the lines it did not read', async () => {
  await look(preview({
    outside: [{ line: 7, text: 'Goreclaw, Terror of Qal Sisma' }],
    unreadable: [{ line: 9, text: '(LTC) 284' }],
  }))
  expect(screen.getByText('Goreclaw, Terror of Qal Sisma')).toBeTruthy()
  expect(screen.getByText('(LTC) 284')).toBeTruthy()
  expect(screen.getByText(/works on the 99/)).toBeTruthy()
})
