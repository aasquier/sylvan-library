/**
 * The settings gear, and inside it the stance slider's four properties in
 * their newest home:
 *
 * - **"Follow the deck" is a position and the default**, and this panel is
 *   the only control that can give it back once overridden.
 * - **The presets come from the server**; the panel adds friendly labels but
 *   never its own roster.
 * - **A capped preset is shown, disabled, and labelled** — slider-shaped,
 *   that means it peeks and never pins.
 * - **The `never` sentence is always present.**
 *
 * Plus the gear's own properties: it renders *regardless* of Claude — theme,
 * ambience and sound are about the person, not any feature's availability —
 * while the Claude section self-gates exactly as the old menu did; the
 * ambience switch writes the opt-out key `lib/prefs.ts` serves the forest
 * from; and the sound switch flips the same key as the tarot table's.
 */

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ClaudeStatus, StanceView } from '../lib/api'
import { useStance } from '../lib/stance'
import { SettingsMenu } from './settings'

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
    never: 'One rule holds at every setting: Claude never writes a card’s rationale on its own. On an import you can ask it to draft the ones you have not written, and every sentence it drafts is marked as Claude’s until you rewrite it.',
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
  render(<SettingsMenu theme="light" onToggleTheme={() => {}} />)
  const trigger = await screen.findByRole('button', { name: 'Settings' })
  fireEvent.click(trigger)
  return trigger
}

describe('SettingsMenu', () => {
  it('renders the gear even without Claude, but no Claude section', async () => {
    claudeStatus.mockResolvedValue(status({ configured: false }))
    await open()
    // The person's own rows are unconditional…
    expect(screen.getByRole('switch', { name: /Ambience/ })).toBeTruthy()
    expect(screen.getByRole('switch', { name: /Table sound/ })).toBeTruthy()
    expect(screen.getByRole('switch', { name: /Dark mode/ })).toBeTruthy()
    // …and the feature-gated one is honestly absent.
    await act(async () => {})
    expect(screen.queryByRole('slider')).toBeNull()
    expect(screen.queryByText(/How much should Claude do/)).toBeNull()
  })

  it('the ambience switch writes the opt-out key, and only the opt-out', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    const row = screen.getByRole('switch', { name: /Ambience/ })
    expect(row.getAttribute('aria-checked')).toBe('true')
    fireEvent.click(row)
    expect(localStorage.getItem('mtglab-ambience')).toBe('0')
    fireEvent.click(screen.getByRole('switch', { name: /Ambience/ }))
    // On is the absence of the key — the default is the app's character.
    expect(localStorage.getItem('mtglab-ambience')).toBeNull()
  })

  it('the sound switch flips the tarot table’s own key', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    fireEvent.click(screen.getByRole('switch', { name: /Table sound/ }))
    expect(localStorage.getItem('mtglab-table-sound')).toBe('1')
    fireEvent.click(screen.getByRole('switch', { name: /Table sound/ }))
    expect(localStorage.getItem('mtglab-table-sound')).toBeNull()
  })

  it('starts the slider at "follow the deck" when nothing is pinned', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    const slider = await screen.findByRole('slider') as HTMLInputElement
    // One track: the auto position, then the server's three presets.
    expect(slider.min).toBe('0')
    expect(slider.max).toBe('3')
    expect(slider.value).toBe('0')
    expect(slider.getAttribute('aria-valuetext')).toMatch(/Follow the deck/)
  })

  it('labels detents in friendly words, never the wire token', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    fireEvent.change(await screen.findByRole('slider'), { target: { value: '2' } })
    expect(screen.getByText('Second opinion')).toBeTruthy()
    expect(screen.queryByText('second-opinion')).toBeNull()
    expect(screen.getByText('A second pair of eyes.')).toBeTruthy()
  })

  it('pins a preset, and can slide back to the deck default', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    fireEvent.change(await screen.findByRole('slider'), { target: { value: '2' } })
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
    fireEvent.change(await screen.findByRole('slider'), { target: { value: '3' } })
    expect(screen.getByText('Collaborator')).toBeTruthy()
    expect(screen.getByText(/limited on this server/)).toBeTruthy()
    expect(localStorage.getItem('mtglab-stance')).toBeNull()
  })

  it('shows the never-line under every position', async () => {
    claudeStatus.mockResolvedValue(status())
    await open()
    await screen.findByRole('slider')
    expect(screen.getByText(/One rule holds at every setting/))
      .toBeTruthy()
  })
})
