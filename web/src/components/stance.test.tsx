/**
 * The dial, and the four properties that are not layout.
 *
 * - **"Follow the deck" is a position and it is the default.** The per-deck
 *   default is real behaviour and this control is the only way back to it.
 * - **The presets come from the server.** Renaming one in `stance.py` must
 *   rename it here, because a list written into the component would offer
 *   levels the instance refuses.
 * - **A capped preset is shown, disabled, and labelled** — "the operator
 *   capped this" and "this does not exist" are different facts.
 * - **The `never` sentence is always present**, under every position. ADR 15
 *   says no stance widens it, so it is not conditional on the pin.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, StanceView } from '../lib/api'
import { StanceDial } from './stance'

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
    never: 'No stance lets Claude write a card’s rationale.',
    modes: [],
    ...over,
  }
}

describe('StanceDial', () => {
  it('offers "follow the deck" and selects it when nothing is pinned', () => {
    render(<StanceDial status={status()} pin={null} onPin={vi.fn()} />)
    const follow = screen.getByRole('radio', { name: /Follow the deck/ })
    expect((follow as HTMLInputElement).checked).toBe(true)
  })

  it('clears the pin back to the deck default', () => {
    // The direction that is easy to lose: pinning is obvious, un-pinning is
    // the one that needs a control, and `null` cannot be spelled as a preset.
    const onPin = vi.fn()
    render(<StanceDial status={status()} pin="consultant" onPin={onPin} />)
    fireEvent.click(screen.getByRole('radio', { name: /Follow the deck/ }))
    expect(onPin).toHaveBeenCalledWith(null)
  })

  it('renders one position per served preset, by the served name', () => {
    render(<StanceDial status={status()} pin={null} onPin={vi.fn()} />)
    // Three presets plus "follow the deck".
    expect(screen.getAllByRole('radio')).toHaveLength(4)
    expect(screen.getByRole('radio', { name: /consultant/ })).toBeTruthy()
    expect(screen.getByText('Speaks when spoken to.')).toBeTruthy()
  })

  it('disables a preset the deployment caps, and says which it is', () => {
    render(<StanceDial status={status()} pin={null} onPin={vi.fn()} />)
    const capped = screen.getByRole('radio', { name: /collaborator/ }) as HTMLInputElement
    expect(capped.disabled).toBe(true)
    expect(screen.getByText(/capped by this instance/)).toBeTruthy()
    // And the ones it does not cap stay selectable.
    expect((screen.getByRole('radio', { name: /consultant/ }) as HTMLInputElement).disabled)
      .toBe(false)
  })

  it('shows the resolved axes rather than the pin', () => {
    // `status.stance` is the server's answer after resolving and clamping.
    // Nothing here recomputes it — a second implementation of `stance.clamp`
    // would disagree silently, showing a level the instance never ran.
    render(<StanceDial status={status()} pin="collaborator" onPin={vi.fn()} />)
    expect(screen.getByText('When may it speak?')).toBeTruthy()
    expect(screen.getByText('on-request')).toBeTruthy()
    expect(screen.getByText(/Only when you ask it something/)).toBeTruthy()
  })

  it('says so when a pin is being narrowed', () => {
    render(<StanceDial status={status()} pin="collaborator" onPin={vi.fn()} />)
    expect(screen.getByText(/This instance caps the stance below that/)).toBeTruthy()
  })

  it('stays quiet about capping when the pin is honoured', () => {
    render(<StanceDial status={status()} pin="consultant" onPin={vi.fn()} />)
    expect(screen.queryByText(/This instance caps the stance below that/)).toBeNull()
  })

  it('shows the never-line under every position, including off', () => {
    // Served rather than written here, and unconditional: a sentence that
    // appeared only next to `collaborator` would read as a warning about that
    // preset rather than as the rule the dial does not reach.
    for (const pin of [null, 'off', 'consultant', 'collaborator']) {
      cleanup()
      render(<StanceDial status={status()} pin={pin} onPin={vi.fn()} />)
      expect(screen.getByText(/No stance lets Claude write a card’s rationale/))
        .toBeTruthy()
    }
  })
})
