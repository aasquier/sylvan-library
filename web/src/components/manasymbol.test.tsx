/**
 * The official-symbol image and its fallback (ADR 33).
 *
 * jsdom never loads an image, so what can be pinned here is the contract
 * around it: the URL is built from the symbol with its punctuation dropped
 * (mirroring the server's shape check), a load error swaps in the caller's
 * fallback, and the failure is remembered at module level so a second
 * instance of the same symbol never mounts a doomed request. The tests use
 * distinct codes because that memo deliberately outlives each test.
 */

import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { OfficialSymbol } from './manasymbol'

describe('OfficialSymbol', () => {
  it('asks our own origin for the symbol, punctuation dropped', () => {
    const { container } = render(
      <OfficialSymbol symbol="W/U" size={16} fallback="wu" />)
    const img = container.querySelector('img')!
    expect(img.getAttribute('src')).toBe('/api/symbols/WU.svg')
    expect(img.getAttribute('alt')).toBe('')
  })

  it('falls back when the image cannot load, and remembers', () => {
    const { container } = render(
      <OfficialSymbol symbol="G/P" size={16} fallback={<span>phi</span>} />)
    fireEvent.error(container.querySelector('img')!)
    expect(screen.getByText('phi')).toBeTruthy()
    expect(container.querySelector('img')).toBeNull()

    // A second instance of the same symbol goes straight to the fallback --
    // the module remembers, so an offline 99 is one failed request per
    // symbol, not one per pip.
    const again = render(
      <OfficialSymbol symbol="G/P" size={16} fallback={<span>phi-two</span>} />)
    expect(again.container.querySelector('img')).toBeNull()
    expect(screen.getByText('phi-two')).toBeTruthy()
  })

  it('never asks for a shape the server would refuse', () => {
    const { container } = render(
      <OfficialSymbol symbol="{nope!}" size={16} fallback="?" />)
    expect(container.querySelector('img')).toBeNull()
    expect(container.textContent).toBe('?')
  })
})
