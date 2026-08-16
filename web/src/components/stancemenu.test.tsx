/**
 * The header stance menu — the dial's four properties, in their new home:
 *
 * - **"Follow the deck" is a position and the default**, and this menu is the
 *   only control that can give it back once overridden.
 * - **The presets come from the server**; the menu adds friendly labels but
 *   never its own roster.
 * - **A capped preset is shown, disabled, and labelled.**
 * - **The `never` sentence is always present.**
 *
 * Plus the property the move added: the menu is **self-gating** — on an
 * instance without Claude it renders nothing, because a setting over a feature
 * that does not exist here is a control over nothing.
 */

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, StanceView } from '../lib/api'
import { useStance } from '../lib/stance'
import { StanceMenu } from './stancemenu'

vi.mock('../lib/api', async () => {
  const real = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...real, api: { ...real.api, claudeStatus: vi.fn() } }
})

const { api } = await import('../lib/api')
const claudeStatus = vi.mocked(api.claudeStatus)

function view(preset: string | null): StanceView {
  return { preset, allows_calls: preset !== 'off', may_write: false, axes: [] }
}

function status(over: Partial<ClaudeStatus> = {}): ClaudeStatus {
  return {
    installed: true, configured: true, model: 'claude-sonnet-5',
    stance: view('consultant'),
    ceiling: view('collaborator'),
    default: view('consultant'),
    presets: [
      { name: 'off', blurb: 'No calls, ever.', stance: view('off'), available: true },
      { name: 'second-opinion', blurb: 'A second pair of eyes.', stance: view('second-opinion'), available: true },
      { name: 'collaborator', blurb: 'Batches edits.', stance: view('collaborator'), available: false },
    ],
    never: 'One rule holds at every setting: Claude never writes a card’s rationale. The why is always yours.',
    modes: [],
    ...over,
  }
}

/** The pin store lives outside React; put it back between tests. */
function ResetPin() {
  const [, setPin] = useStance()
  useEffect(() => { setPin(null) }, [setPin])
  return null
}

afterEach(() => {
  cleanup()
  localStorage.clear()
})

beforeEach(() => {
  claudeStatus.mockReset()
  render(<ResetPin />)
  cleanup()
})

async function open() {
  render(<StanceMenu />)
  const trigger = await screen.findByRole('button', { name: /Claude/ })
  fireEvent.click(trigger)
  return trigger
}

describe('StanceMenu', () => {
  it('renders nothing when Claude is not configured here', async () => {
    claudeStatus.mockResolvedValue(status({ configured: false }))
    render(<StanceMenu />)
    // Settle the fetch, then assert absence.
    await act(async () => {})
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('starts the slider at "follow the deck" when nothing is pinned', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    const slider = screen.getByRole('slider') as HTMLInputElement
    // One track: the auto position, then the server's three presets.
    expect(slider.min).toBe('0')
    expect(slider.max).toBe('3')
    expect(slider.value).toBe('0')
    expect(slider.getAttribute('aria-valuetext')).toMatch(/Follow the deck/)
  })

  it('labels detents in friendly words, never the wire token', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    fireEvent.change(screen.getByRole('slider'), { target: { value: '2' } })
    expect(screen.getByText('Second opinion')).toBeTruthy()
    expect(screen.queryByText('second-opinion')).toBeNull()
    expect(screen.getByText('A second pair of eyes.')).toBeTruthy()
  })

  it('pins a preset, and can slide back to the deck default', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    fireEvent.change(screen.getByRole('slider'), { target: { value: '2' } })
    expect(localStorage.getItem('mtglab-stance')).toBe('second-opinion')

    fireEvent.change(screen.getByRole('slider'), { target: { value: '0' } })
    expect(localStorage.getItem('mtglab-stance')).toBeNull()
  })

  it('a capped detent peeks but never pins', async () => {
    // "Shown, disabled, and labelled", slider-shaped: the busy end of the
    // track is still there to look at, the readout says the operator limited
    // it, and nothing lands in the pin store.
    claudeStatus.mockResolvedValue(status())
    await open()
    fireEvent.change(screen.getByRole('slider'), { target: { value: '3' } })
    expect(screen.getByText('Collaborator')).toBeTruthy()
    expect(screen.getByText(/limited on this server/)).toBeTruthy()
    expect(localStorage.getItem('mtglab-stance')).toBeNull()
  })

  it('shows the never-line under every position', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    expect(screen.getByText(/One rule holds at every setting/))
      .toBeTruthy()
  })
})
