/**
 * The stance readout, and the properties that survived the dial's move to the
 * header (`settings.test.tsx` covers the control itself):
 *
 * - **The axes are the server's resolved answer.** `status.stance` is what
 *   `/api/claude` said after clamping; nothing here recomputes it, and the
 *   popover renders it as served.
 * - **A narrowed pin is said out loud** — phrased as the instance's decision,
 *   not the user's mistake.
 * - **The `never` sentence is served and always present.**
 * - **No raw wire tokens.** `second-opinion` and `on-request` are enum values,
 *   not labels; the readout renders the friendly names from `lib/claudecopy`.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { ClaudeStatus, StanceView } from '../lib/api'
import { StanceReadout } from './stance'

afterEach(cleanup)

function view(preset: string | null, axes: StanceView['axes'] = []): StanceView {
  return { preset, allows_calls: preset !== 'off', may_write: false, axes }
}

function status(over: Partial<ClaudeStatus> = {}): ClaudeStatus {
  return {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: view('consultant', [{
      axis: 'initiative', question: 'When may it speak?',
      level: 'on-request', means: 'Only when you ask it something.',
      levels: ['off', 'on-request', 'volunteers', 'interjects'],
    }]),
    ceiling: view('collaborator'),
    default: view('consultant'),
    presets: [
      { name: 'off', blurb: 'No calls, ever.', stance: view('off'), available: true },
      { name: 'consultant', blurb: 'Speaks when spoken to.', stance: view('consultant'), available: true },
      { name: 'collaborator', blurb: 'Batches edits.', stance: view('collaborator'), available: false },
    ],
    never: 'One rule holds at every setting: Claude never writes a card’s rationale on its own. On an import you can ask it to draft the ones you have not written, and every sentence it drafts is marked as Claude’s until you rewrite it.',
    modes: [],
    ...over,
  }
}

describe('StanceReadout', () => {
  it('names the resolved position, not the pin, and says it follows the deck', () => {
    render(<StanceReadout status={status()} pin={null} />)
    expect(screen.getByText(/Consultant · following the deck/)).toBeTruthy()
  })

  it('shows just the resolved name when a pin is honoured', () => {
    render(<StanceReadout status={status()} pin="consultant" />)
    expect(screen.getByText('Consultant')).toBeTruthy()
    expect(screen.queryByText(/limited/)).toBeNull()
  })

  it('says when the instance narrowed the pin, naming both positions', () => {
    render(<StanceReadout status={status()} pin="collaborator" />)
    expect(screen.getByText(/Consultant · limited from Collaborator/)).toBeTruthy()
  })

  it('renders the served axes in the popover, in friendly words', () => {
    // `status.stance` is the resolved-and-clamped answer; a second
    // implementation of the clamp here would disagree silently.
    render(<StanceReadout status={status()} pin={null} />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByText('When may it speak?')).toBeTruthy()
    expect(screen.getByText('answers when asked')).toBeTruthy()
    expect(screen.getByText(/Only when you ask it something/)).toBeTruthy()
    // The wire token itself never renders.
    expect(screen.queryByText('on-request')).toBeNull()
  })

  it('keeps the never-line, served and unconditional', () => {
    for (const pin of [null, 'off', 'collaborator']) {
      cleanup()
      render(<StanceReadout status={status()} pin={pin} />)
      fireEvent.click(screen.getByRole('button'))
      expect(screen.getByText(/One rule holds at every setting/))
        .toBeTruthy()
    }
  })

  it('points at the header menu, which is where the control went', () => {
    render(<StanceReadout status={status()} pin={null} />)
    fireEvent.click(screen.getByRole('button'))
    expect(screen.getByText(/Claude menu in the header/)).toBeTruthy()
  })
})
