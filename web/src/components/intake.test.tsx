import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ClaudeStatus, IntakeSheet, StanceView } from '../lib/api'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: { claudeStatus: vi.fn() },
}))

const { api } = await import('../lib/api')
const { IntakeChoices } = await import('./intake')

function view(preset: string, mayWrite: boolean): StanceView {
  return { preset, allows_calls: true, may_write: mayWrite, axes: [] }
}

function status(mayWrite: boolean, over: Partial<ClaudeStatus> = {}): ClaudeStatus {
  return {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: view('collaborator', mayWrite),
    ceiling: view('collaborator', true),
    default: view('consultant', false),
    presets: [], never: '', modes: [], ...over,
  }
}

function show(mayWrite: boolean, value: IntakeSheet = {}, over: Partial<ClaudeStatus> = {}) {
  vi.mocked(api.claudeStatus).mockResolvedValue(status(mayWrite, over))
  const onChange = vi.fn<(u: (p: IntakeSheet) => IntakeSheet) => void>()
  render(<IntakeChoices value={value} onChange={onChange} slug="arahbo-cats" />)
  return onChange
}

describe('IntakeChoices', () => {
  afterEach(cleanup)
  beforeEach(() => vi.mocked(api.claudeStatus).mockReset())

  // ADR 41's first gate: the sheet is a question about THIS deck, so nothing
  // on it is on until somebody turns it on.
  it('starts with everything off', async () => {
    show(true)
    await waitFor(() => expect(screen.getByText('Sort the cards')).toBeTruthy())
    for (const label of ['Sort the cards', 'Draft the reasons', 'Describe the deck',
      'Read up on your commander', 'Argue with every card']) {
      expect(screen.getByText(label).closest('button')!.getAttribute('aria-pressed'))
        .toBe('false')
    }
  })

  // ADR 41's second gate, as the user meets it. Absent rather than disabled:
  // a greyed control reads as "this is broken", and the sentence beside it is
  // the thing that actually helps.
  it('does not offer to draft reasons when the stance may not write', async () => {
    show(false)
    await waitFor(() => expect(screen.getByText('Sort the cards')).toBeTruthy())
    expect(screen.queryByText('Draft the reasons')).toBeNull()
    expect(screen.getByText(/may not change anything/)).toBeTruthy()
    // And it says where the setting is, because a control somebody was told
    // about and cannot find is worse than one that was never mentioned.
    expect(screen.getByText(/stance dial/)).toBeTruthy()
  })

  it('offers it, and no explanation, when the stance may write', async () => {
    show(true)
    await waitFor(() => expect(screen.getByText('Draft the reasons')).toBeTruthy())
    expect(screen.queryByText(/may not change anything/)).toBeNull()
  })

  // Every control answers the hand that reaches for it (commandment 17): these
  // are `.chip-toggle`s with a real pressed state, not bare buttons wearing an
  // inline style no `:hover` can reach.
  it('gives every toggle the chip family and a pressed state', async () => {
    const onChange = show(true, { categories: true })
    await waitFor(() => expect(screen.getByText('Sort the cards')).toBeTruthy())

    const sort = screen.getByText('Sort the cards').closest('button')!
    expect(sort.className).toContain('chip-toggle')
    expect(sort.className).toContain('is-on')
    expect(sort.getAttribute('aria-pressed')).toBe('true')

    const argue = screen.getByText('Argue with every card').closest('button')!
    expect(argue.className).toContain('chip-toggle')
    expect(argue.className).not.toContain('is-on')

    // The updater form, so two chips toggled inside one frame do not both
    // read the same stale prop and lose the first.
    fireEvent.click(argue)
    const update = vi.mocked(onChange).mock.calls[0]![0]
    expect(update({ categories: true })).toEqual({ categories: true, argue: true })
    expect(update({ categories: true, dossier: true }))
      .toEqual({ categories: true, dossier: true, argue: true })
  })

  // What a chosen action will do is said on the page, not on hover: half this
  // audience is on a phone and a `title` reaches neither them nor a keyboard.
  it('explains the actions that are on, and only those', async () => {
    show(true, { rationales: true })
    await waitFor(() => expect(screen.getByText(/first pass at why each card/)).toBeTruthy())
    expect(screen.getByText(/marked as Claude’s in the file/)).toBeTruthy()
    expect(screen.queryByText(/what the deck is trying to do/)).toBeNull()
  })

  // With no credential nothing on the sheet can run, so it stands down whole
  // rather than offering five controls that would each fail the same way.
  it('stands the whole sheet down when Claude is not configured', async () => {
    show(true, {}, { configured: false })
    await waitFor(() =>
      expect(screen.getByText(/exactly as you pasted it/)).toBeTruthy())
    expect(screen.queryByText('Sort the cards')).toBeNull()
  })

  // **The gate is closed before it is opened, not after.** While the dial has
  // not answered, the drafting toggle is absent -- and this is the same code
  // path a dial that never answers takes, because both leave `status` null and
  // `may_write` therefore false. The safe direction for a control gated on a
  // permission is closed, and the dangerous version of this component is the
  // one that renders the toggle optimistically and hides it a moment later.
  it('keeps the drafting toggle shut until the dial says otherwise', async () => {
    // Answers `may_write: true`, so what the first paint shows is decided by
    // the component's default and not by the answer -- effects run after the
    // first render, so this assertion is the pre-answer state and it is
    // deterministic rather than a race.
    vi.mocked(api.claudeStatus).mockResolvedValue(status(true))
    render(<IntakeChoices value={{}} onChange={vi.fn()} slug="arahbo-cats" />)

    expect(screen.queryByText('Draft the reasons')).toBeNull()
    // Nothing else is claimed while the answer is outstanding either: the
    // stand-down sentence would be a statement about a setting nobody has
    // read yet.
    expect(screen.queryByText(/exactly as you pasted it/)).toBeNull()
    expect(screen.queryByText(/may not change anything/)).toBeNull()

    // ...and then it opens, which is what makes the assertion above a
    // statement about ordering rather than about a broken component.
    await waitFor(() => expect(screen.getByText('Draft the reasons')).toBeTruthy())
  })
})

// **The bug Aaron hit on 2026-08-29**, reported as "I wasn't prompted with any
// intake options".
//
// The import page passed `slug={effectiveSlug}` — the name the deck WILL have
// — and `/api/claude?slug=…` resolves the deck and 404s when there is not one.
// The catch set `status` to null, `configured` fell back to `false`, and the
// sheet rendered "Claude is turned off for this deck": a statement about
// somebody's settings, made by something that could not read their settings.
// The same shape as the card finder reporting a failure as an absence, found
// the same day.
//
// Two tests, because the fix has two halves and either alone leaves a hole.
describe('when the dial cannot be read', () => {
  afterEach(cleanup)
  beforeEach(() => vi.mocked(api.claudeStatus).mockReset())

  it('does not claim Claude is turned off', async () => {
    // Resolves null rather than rejecting: `status === null` is the state the
    // component actually branches on, and it is what a failed read leaves
    // behind. Driving it this way reaches the same branch without a rejection
    // for the runner to count as an escape.
    vi.mocked(api.claudeStatus).mockResolvedValue(null as never)
    render(<IntakeChoices value={{}} onChange={vi.fn()} />)

    // The four that were never gated are still offered: the server decides
    // what it will do, and hiding them would be guessing the other way.
    await waitFor(() => expect(screen.getByText('Sort the cards')).toBeTruthy())
    expect(screen.queryByText(/Claude is turned off/)).toBeNull()
    expect(screen.queryByText(/exactly as you pasted it/)).toBeNull()
    // Drafting stays shut, because closed is the safe direction for a control
    // gated on a permission — but without asserting a setting nobody read.
    expect(screen.queryByText('Draft the reasons')).toBeNull()
    expect(screen.queryByText(/may not change anything/)).toBeNull()
  })

  // **The cause, pinned.** `/api/claude?slug=…` resolves the deck and 404s
  // when there is not one, and on the import screen there never is one yet:
  // the deck is created by the button. The `intake` surface exists exactly so
  // the dial can answer without a deck, and passing a would-be slug undoes it.
  it('asks as the intake surface and never about a deck that does not exist',
     async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(status(true))
    render(<IntakeChoices value={{}} onChange={vi.fn()} />)
    await waitFor(() => expect(api.claudeStatus).toHaveBeenCalled())

    const sent = vi.mocked(api.claudeStatus).mock.calls[0]![0]!
    expect(sent.surface).toBe('intake')
    expect(sent.slug).toBeUndefined()
  })

  it('still stands down when the dial actually says Claude is off', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(
      status(false, { configured: false }))
    render(<IntakeChoices value={{}} onChange={vi.fn()} />)
    await waitFor(() =>
      expect(screen.getByText(/exactly as you pasted it/)).toBeTruthy())
    expect(screen.queryByText('Sort the cards')).toBeNull()
  })
})
