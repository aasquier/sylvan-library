/**
 * The pin, and the two ways a stored preset name goes stale.
 *
 * The distinction these pin is the reason `effectivePin` takes the whole
 * status rather than a list of names: **a preset this build does not serve and
 * a preset this deployment has capped fail in opposite directions**, and
 * treating them alike breaks one of them.
 *
 * An unknown name is a 422 from `Stance.from_obj` on every Claude call — the
 * feature stays broken until somebody clears their browser storage, which
 * nobody will think to do. A capped name is not an error at all: the server
 * clamps and answers, so sending it is correct and the only thing owed is
 * saying so. Drop the first, keep the second.
 */

import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, StanceView } from './api'
import { effectivePin, fetchClaudeStatus, isCapped } from './stance'

vi.mock('./api', async () => {
  const actual = await vi.importActual<typeof import('./api')>('./api')
  return { ...actual, api: { claudeStatus: vi.fn() } }
})

const { ApiError, api } = await import('./api')

function view(preset: string | null): StanceView {
  return { preset, allows_calls: true, may_write: false, axes: [] }
}

/** Only the fields these two functions read; the rest is the route's business. */
function status(presets: { name: string; available: boolean }[]): ClaudeStatus {
  return {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: view('consultant'), ceiling: view('collaborator'),
    default: view('consultant'),
    presets: presets.map((p) => ({
      name: p.name, blurb: `${p.name} blurb`, stance: view(p.name),
      available: p.available,
    })),
    never: 'One rule holds at every setting: Claude never writes a card’s rationale on its own. On an import you can ask it to draft the ones you have not written, and every sentence it drafts is marked as Claude’s until you rewrite it.',
    modes: [],
  }
}

const ROSTER = status([
  { name: 'off', available: true },
  { name: 'consultant', available: true },
  { name: 'second-opinion', available: true },
  { name: 'collaborator', available: false },   // an instance with a ceiling
])

describe('effectivePin', () => {
  beforeEach(() => localStorage.clear())

  it('sends nothing when nothing is pinned', () => {
    // The load-bearing case. `undefined` means the surface uses its own
    // default, and those defaults are real behaviour: a theoretical deck opens
    // wider than a built one, and the theme interview opens wider still. A
    // dial that sent `off` here would silently switch the feature off for
    // everyone who had never touched it.
    expect(effectivePin(null, ROSTER)).toBeUndefined()
  })

  it('sends a pin this deployment serves', () => {
    expect(effectivePin('consultant', ROSTER)).toBe('consultant')
  })

  it('drops a preset the roster no longer has', () => {
    // Renamed or removed between builds. Sending it would 422 every Claude
    // call on the page, so the preference is what gives way.
    expect(effectivePin('collaborater', ROSTER)).toBeUndefined()
    expect(effectivePin('second_opinion', ROSTER)).toBeUndefined()
  })

  it('still sends a preset the deployment has capped', () => {
    // Not an error: the server clamps and answers. Dropping it would quietly
    // demote the user to the deck's default, which is a *different* stance
    // from the clamped one they asked for and would be invisible either way.
    expect(effectivePin('collaborator', ROSTER)).toBe('collaborator')
  })

  it('sends the pin unchecked when the roster has not arrived', () => {
    // Status is fetched, so it is null on first paint. Withholding the pin
    // until it lands would make the first call after a reload quieter than
    // the one before it, which is the kind of intermittence nobody diagnoses.
    expect(effectivePin('consultant', null)).toBe('consultant')
  })
})

describe('fetchClaudeStatus', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.mocked(api.claudeStatus).mockReset()
  })

  it('asks with the pin and returns what comes back', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(ROSTER)
    const clear = vi.fn()

    await fetchClaudeStatus({ slug: 'gyome-food' }, 'consultant', clear)

    expect(api.claudeStatus).toHaveBeenCalledWith({
      slug: 'gyome-food', stance: 'consultant',
    })
    expect(clear).not.toHaveBeenCalled()
  })

  it('clears a refused pin and retries without it', async () => {
    // The lock-out this exists to prevent: every Claude panel gates on this
    // call, so a preset renamed between builds would not show an error — it
    // would remove the dial, which is the only control able to clear the pin.
    vi.mocked(api.claudeStatus)
      .mockRejectedValueOnce(new ApiError('not a stance preset', 422))
      .mockResolvedValueOnce(ROSTER)
    const clear = vi.fn()

    const got = await fetchClaudeStatus({ slug: 'gyome-food' }, 'renamed', clear)

    expect(clear).toHaveBeenCalledOnce()
    expect(got).toBe(ROSTER)
    // Second attempt carries no stance at all, rather than the bad one again.
    expect(vi.mocked(api.claudeStatus).mock.calls[1]?.[0]).toEqual({
      slug: 'gyome-food',
    })
  })

  it('does not swallow a failure that is not about the stance', async () => {
    // A 500 means the instance is unwell. Retrying bare would relabel that as
    // "you have no Claude surface" and throw away the pin for no reason.
    vi.mocked(api.claudeStatus).mockRejectedValue(new ApiError('boom', 500))
    const clear = vi.fn()

    await expect(fetchClaudeStatus({}, 'consultant', clear)).rejects.toThrow('boom')
    expect(clear).not.toHaveBeenCalled()
    expect(api.claudeStatus).toHaveBeenCalledOnce()
  })

  it('does not retry a 422 that arrived without a pin', async () => {
    // Then the 422 is about something else entirely — a slug, an owner — and
    // retrying identically would just fail twice.
    vi.mocked(api.claudeStatus).mockRejectedValue(new ApiError('bad deck', 422))

    await expect(fetchClaudeStatus({ slug: 'nope' }, null, vi.fn()))
      .rejects.toThrow('bad deck')
    expect(api.claudeStatus).toHaveBeenCalledOnce()
  })
})

describe('isCapped', () => {
  it('is true only for a pin the deployment will narrow', () => {
    expect(isCapped('collaborator', ROSTER)).toBe(true)
    expect(isCapped('consultant', ROSTER)).toBe(false)
  })

  it('is false when nothing is pinned or nothing is known', () => {
    expect(isCapped(null, ROSTER)).toBe(false)
    expect(isCapped('collaborator', null)).toBe(false)
    // An unknown name is dropped rather than sent, so it is not "capped" —
    // saying so would put a message about the instance's ceiling under a
    // preference that is not being applied at all.
    expect(isCapped('nonesuch', ROSTER)).toBe(false)
  })
})
