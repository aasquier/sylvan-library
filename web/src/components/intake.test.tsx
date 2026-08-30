import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ClaudeStatus, IntakeSheet, StanceView } from '../lib/api'

vi.mock('../lib/api', async () => ({
  ...(await vi.importActual<typeof import('../lib/api')>('../lib/api')),
  api: { claudeStatus: vi.fn() },
}))

// Written before `./intake` is pulled in, and that ordering is the point:
// `lib/stance` reads the stored pin once, at module load, and keeps it outside
// React. A `setItem` after the import would be a preference nothing had read.
localStorage.setItem('mtglab-stance', 'collaborator')

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

function show(mayWrite: boolean, value: IntakeSheet = {}, over: Partial<ClaudeStatus> = {},
              running = false) {
  vi.mocked(api.claudeStatus).mockResolvedValue(status(mayWrite, over))
  const onChange = vi.fn<(u: (p: IntakeSheet) => IntakeSheet) => void>()
  render(<IntakeChoices value={value} onChange={onChange} slug="arahbo-cats"
                        running={running} />)
  return onChange
}

describe('IntakeChoices', () => {
  afterEach(cleanup)
  beforeEach(() => vi.mocked(api.claudeStatus).mockReset())

  // ADR 41's first gate: the sheet is a question about THIS deck, so nothing
  // on it is on until somebody turns it on.
  it('starts with everything off', async () => {
    show(true)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Sort the cards' })).toBeTruthy())
    for (const label of ['Sort the cards', 'Draft the reasons', 'Describe the deck',
      'Read up on your commander', 'Argue with every card']) {
      expect(screen.getByRole('button', { name: label }).getAttribute('aria-pressed'))
        .toBe('false')
    }
  })

  // ADR 41's second gate, as the user meets it. Absent rather than disabled:
  // a greyed control reads as "this is broken", and the sentence beside it is
  // the thing that actually helps.
  it('does not offer to draft reasons when the stance may not write', async () => {
    show(false)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Sort the cards' })).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Draft the reasons' })).toBeNull()
    expect(screen.getByText(/may not change anything/)).toBeTruthy()
    // And it says where the setting is, because a control somebody was told
    // about and cannot find is worse than one that was never mentioned.
    expect(screen.getByText(/stance dial/)).toBeTruthy()
  })

  it('offers it, and no explanation, when the stance may write', async () => {
    show(true)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Draft the reasons' })).toBeTruthy())
    expect(screen.queryByText(/may not change anything/)).toBeNull()
  })

  // Every control answers the hand that reaches for it (commandment 17): these
  // are `.chip-toggle`s with a real pressed state, not bare buttons wearing an
  // inline style no `:hover` can reach.
  it('gives every toggle the chip family and a pressed state', async () => {
    const onChange = show(true, { categories: true })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Sort the cards' })).toBeTruthy())

    const sort = screen.getByRole('button', { name: 'Sort the cards' })
    expect(sort.className).toContain('chip-toggle')
    expect(sort.className).toContain('is-on')
    expect(sort.getAttribute('aria-pressed')).toBe('true')

    const argue = screen.getByRole('button', { name: 'Argue with every card' })
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
    expect(screen.queryByRole('button', { name: 'Sort the cards' })).toBeNull()
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

    expect(screen.queryByRole('button', { name: 'Draft the reasons' })).toBeNull()
    // Nothing else is claimed while the answer is outstanding either: the
    // stand-down sentence would be a statement about a setting nobody has
    // read yet.
    expect(screen.queryByText(/exactly as you pasted it/)).toBeNull()
    expect(screen.queryByText(/may not change anything/)).toBeNull()

    // ...and then it opens, which is what makes the assertion above a
    // statement about ordering rather than about a broken component.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Draft the reasons' })).toBeTruthy())
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
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Sort the cards' })).toBeTruthy())
    expect(screen.queryByText(/Claude is turned off/)).toBeNull()
    expect(screen.queryByText(/exactly as you pasted it/)).toBeNull()
    // Drafting stays shut, because closed is the safe direction for a control
    // gated on a permission — but without asserting a setting nobody read.
    expect(screen.queryByRole('button', { name: 'Draft the reasons' })).toBeNull()
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
    expect(screen.queryByRole('button', { name: 'Sort the cards' })).toBeNull()
  })
})

/**
 * **The bug Aaron hit on 2026-08-29**: the sheet offered "Draft the reasons"
 * and the server answered that his stance was set to change nothing.
 *
 * This component asked the dial with his pin and showed the toggle because the
 * answer said that stance may write. The page that submits asked nothing and
 * sent nothing, so the server resolved the deck's own default — `consultant`,
 * write `none` — and refused. One gate, two different questions.
 *
 * `onStance` is what closes it: the sheet reports the value it asked with, and
 * whoever submits carries that one rather than fetching a second answer. What
 * these tests hold is the *equality* — what went to the dial is what comes
 * back out — because a report of some other correct-looking value would rebuild
 * the bug while passing any assertion about a literal.
 */
describe('the stance the sheet decided with', () => {
  afterEach(cleanup)
  beforeEach(() => vi.mocked(api.claudeStatus).mockReset())

  it('reports the stance it asked the dial with', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(status(true))
    const onStance = vi.fn<(stance: string | undefined) => void>()
    render(<IntakeChoices value={{}} onChange={vi.fn()} onStance={onStance} />)
    await waitFor(() => expect(api.claudeStatus).toHaveBeenCalled())

    const asked = vi.mocked(api.claudeStatus).mock.calls.at(-1)![0]!.stance
    // The premise: a real pin is in play, so the equality below is not two
    // undefineds agreeing — which is the state the broken page was in.
    expect(asked).toBe('collaborator')
    expect(onStance).toHaveBeenCalledWith(asked)
  })

  // A dial that will not answer still leaves four actions on the sheet, and
  // they run at the user's stance like everything else. Reporting only on a
  // successful answer would send those four with no stance at all — the same
  // silence that caused this, narrowed rather than fixed.
  //
  // Resolves null rather than rejecting, for the reason the describe above
  // gives: `status === null` is the state the component branches on, and it is
  // what a failed read leaves behind, without handing the runner a rejection
  // to count as an escape.
  it('reports it even when the dial cannot be read', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(null as never)
    const onStance = vi.fn<(stance: string | undefined) => void>()
    render(<IntakeChoices value={{}} onChange={vi.fn()} onStance={onStance} />)

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Sort the cards' })).toBeTruthy())
    expect(onStance).toHaveBeenCalledWith('collaborator')
  })

  // The sheet works without anybody listening: `onStance` is optional because
  // the four ungated actions predate it, and a component that threw on a
  // missing callback would take the whole import screen down with it.
  it('renders perfectly well with nobody listening', async () => {
    vi.mocked(api.claudeStatus).mockResolvedValue(status(true))
    render(<IntakeChoices value={{}} onChange={vi.fn()} />)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Draft the reasons' })).toBeTruthy())
  })
})

/* The sheet is locked once its work is running.
 *
 * An intake over ninety-nine cards is two or three minutes, and the sheet
 * stays on screen for all of it — what was asked for is the most useful thing
 * to be reading while it happens. But the request left when the button was
 * pressed, so a chip toggled now changes nothing on the server: press "Draft
 * the reasons" off mid-run and it would go grey exactly as though it had been
 * called off, while eighty-four rationales arrived anyway.
 *
 * Driven rather than asserted about, because "it is disabled" and "pressing it
 * does nothing" are different claims and only the second one matters.
 */
describe('while the work is running', () => {
  afterEach(cleanup)
  beforeEach(() => vi.mocked(api.claudeStatus).mockReset())

  it('locks every chip, and a press changes nothing', async () => {
    const onChange = show(true, { rationales: true }, {}, true)
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Draft the reasons' })).toBeTruthy())

    for (const label of ['Sort the cards', 'Draft the reasons', 'Describe the deck',
      'Read up on your commander', 'Argue with every card']) {
      const chip = screen.getByRole('button', { name: label })
      expect(chip.hasAttribute('disabled'), `${label} is locked`).toBe(true)
      fireEvent.click(chip)
    }
    expect(onChange, 'no press reached the sheet').not.toHaveBeenCalled()
  })

  it('still says which ones were asked for', async () => {
    show(true, { rationales: true }, {}, true)
    const chip = await screen.findByRole('button', { name: 'Draft the reasons' })
    // Locked, but still legibly the thing that was chosen — the sheet is a
    // record of the request while the request is being carried out.
    expect(chip.getAttribute('aria-pressed')).toBe('true')
    expect(chip.className).toContain('is-on')
  })

  it('leaves the chips live before anything has been submitted', async () => {
    const onChange = show(true)
    const chip = await screen.findByRole('button', { name: 'Sort the cards' })
    expect(chip.hasAttribute('disabled')).toBe(false)
    fireEvent.click(chip)
    expect(onChange).toHaveBeenCalledTimes(1)
  })
})
