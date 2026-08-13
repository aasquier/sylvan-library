/**
 * The two inline vocabulary affordances.
 *
 * The property worth pinning is the degradation: a key with no entry renders
 * the words and nothing else. That is what makes it safe to mark a word up
 * before somebody has written its definition, and it is the difference between
 * a page that is ahead of its glossary and a page full of controls that open
 * nothing.
 *
 * The rest is about reach. Hover is unavailable on a touch screen and a
 * keyboard has no pointer, so the same panel has to open on hover, on focus
 * and on click — three routes to one piece of state, which is exactly the kind
 * of thing that works for the mouse and quietly fails for everyone else.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { Glossary } from '../lib/api'
import { resetGlossaryCache } from '../lib/glossary'
import { HelpTip, Term } from './term'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: { glossary: vi.fn() } }
})

const { api } = await import('../lib/api')

const GLOSSARY: Glossary = {
  sections: [{ key: 'format', label: 'The format', blurb: '' }],
  terms: [{
    key: 'mulligan', term: 'Mulligan',
    short: 'Taking a new opening hand, then bottoming a card.',
    long: 'The London mulligan, at length.',
    section: 'format', see_also: [],
  }],
}

beforeEach(() => {
  resetGlossaryCache()
  vi.mocked(api.glossary).mockResolvedValue(GLOSSARY)
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

it('opens the definition on hover', async () => {
  render(<p>You may <Term name="mulligan">mulligan</Term> once.</p>)
  const trigger = await screen.findByRole('button', { name: 'What is Mulligan?' })
  expect(screen.queryByRole('tooltip')).toBeNull()
  fireEvent.mouseEnter(trigger)
  expect(screen.getByRole('tooltip').textContent)
    .toContain('new opening hand')
  fireEvent.mouseLeave(trigger)
  expect(screen.queryByRole('tooltip')).toBeNull()
})

it('resets the type styling it inherits from a field label', async () => {
  // The simulator's labels are `uppercase tracking-wide` and the panel is a
  // descendant of the label, so a sentence of help arrived SHOUTED. Found by
  // hovering the real control, invisible to every other test here.
  render(
    <span className="uppercase tracking-wide">
      Min mana pieces<HelpTip name="mulligan" />
    </span>,
  )
  const trigger = await screen.findByRole('button', { name: 'What is Mulligan?' })
  fireEvent.mouseEnter(trigger)
  const panel = screen.getByRole('tooltip')
  expect(panel.style.textTransform).toBe('none')
  expect(panel.style.letterSpacing).toBe('normal')
})

it('opens on focus, for a keyboard with no pointer', async () => {
  render(<p><Term name="mulligan">mulligan</Term></p>)
  const trigger = await screen.findByRole('button', { name: 'What is Mulligan?' })
  fireEvent.focus(trigger)
  expect(screen.getByRole('tooltip')).toBeTruthy()
})

it('a click pins it open, so a touch screen can read it', async () => {
  render(<p><Term name="mulligan">mulligan</Term></p>)
  const trigger = await screen.findByRole('button', { name: 'What is Mulligan?' })
  fireEvent.click(trigger)
  // Pinned: the pointer leaving no longer closes it.
  fireEvent.mouseLeave(trigger)
  expect(screen.getByRole('tooltip')).toBeTruthy()
  fireEvent.keyDown(window, { key: 'Escape' })
  expect(screen.queryByRole('tooltip')).toBeNull()
})

it('renders the words alone when there is no such term', async () => {
  render(<p>A <Term name="no-such-key">widget</Term>.</p>)
  expect(await screen.findByText(/widget/)).toBeTruthy()
  expect(screen.queryByRole('button')).toBeNull()
})

it('the help mark disappears rather than opening nothing', async () => {
  const { container } = render(<HelpTip name="no-such-key" />)
  // Nothing at all, not a mark that does not respond.
  await Promise.resolve()
  expect(container.textContent).toBe('')
})

it('the help mark carries the term name for a screen reader', async () => {
  render(<HelpTip name="mulligan" />)
  expect(await screen.findByRole('button', { name: 'What is Mulligan?' }))
    .toBeTruthy()
})

it('asks for the glossary once however many marks are on the screen', async () => {
  render(
    <p>
      <Term name="mulligan">a</Term>
      <Term name="mulligan">b</Term>
      <HelpTip name="mulligan" />
    </p>,
  )
  await screen.findAllByRole('button', { name: 'What is Mulligan?' })
  expect(vi.mocked(api.glossary)).toHaveBeenCalledTimes(1)
})
