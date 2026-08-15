/**
 * The corner sprout.
 *
 * Two properties carry the design. **It never opens itself** — the bubble
 * exists only after a click, because an uninvited pop-up is how a gift
 * becomes an irritation. And **it renders nothing at all without a
 * glossary** — a button that opens onto "loading…" is a promise made before
 * checking it can be kept, and the App shell mounts this on every page of
 * an instance that may have no working API at all.
 */

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { Glossary } from '../lib/api'
import { resetGlossaryCache } from '../lib/glossary'
import { LibraryWhisper } from './whisper'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { glossary: vi.fn() } }
})

const { api } = await import('../lib/api')

const GLOSSARY: Glossary = {
  sections: [{ key: 'format', label: 'The format', blurb: '' }],
  terms: [
    {
      key: 'mulligan', term: 'Mulligan',
      short: 'Taking a new opening hand, then bottoming a card.',
      long: '', section: 'format', see_also: [],
    },
    {
      key: 'commander', term: 'Commander',
      short: 'The legendary creature the deck is built around.',
      long: '', section: 'format', see_also: [],
    },
  ],
}

beforeEach(() => {
  resetGlossaryCache()
  vi.mocked(api.glossary).mockResolvedValue(GLOSSARY)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

it('renders nothing until the glossary arrives, then only the sprout', async () => {
  render(<LibraryWhisper />)
  expect(screen.queryByRole('button')).toBeNull()
  const sprout = await screen.findByRole('button', { name: /whisper from the library/i })
  expect(sprout).toBeTruthy()
  // Closed by default: the whisper is offered, never imposed.
  expect(screen.queryByRole('note')).toBeNull()
})

it('opens on click, walks to a different term, and closes on Escape', async () => {
  render(<LibraryWhisper />)
  const sprout = await screen.findByRole('button', { name: /whisper from the library/i })

  fireEvent.click(sprout)
  const note = screen.getByRole('note')
  const first = note.textContent
  expect(first).toMatch(/Mulligan|Commander/)

  fireEvent.click(screen.getByRole('button', { name: 'Another leaf' }))
  await waitFor(() => {
    expect(screen.getByRole('note').textContent).not.toBe(first)
  })

  fireEvent.keyDown(document, { key: 'Escape' })
  await waitFor(() => {
    expect(screen.queryByRole('note')).toBeNull()
  })
})

it('renders nothing at all when the glossary cannot be fetched', async () => {
  resetGlossaryCache()
  vi.mocked(api.glossary).mockRejectedValue(new Error('no server'))
  render(<LibraryWhisper />)
  // Give the rejected fetch its microtask; the component must stay empty.
  await new Promise((r) => setTimeout(r, 0))
  expect(screen.queryByRole('button')).toBeNull()
})
