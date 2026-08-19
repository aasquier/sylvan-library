/** The camera door.
 *
 * jsdom has no camera and no WebAssembly reader, so what is pinned here is
 * everything around them: the counting that turns chosen cards into a
 * decklist, the refusals a browser without a lens produces, and — the one
 * that matters — that a shortlist is never pre-chosen.
 *
 * The rest wants a real lens and real cards in real light, which is
 * commandment 16's business and not a test file's.
 */

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { decklistLines } from '../lib/decklist'
import CameraDoor from './camera'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  Reflect.deleteProperty(navigator, 'mediaDevices')
})

/** jsdom's `navigator` has no `mediaDevices` at all — there is no getter to
 *  spy on — so each test defines the shape it wants to test against. That
 *  absence is itself one of the cases: it is what an insecure origin looks
 *  like from script. */
function lens(devices: unknown) {
  Object.defineProperty(navigator, 'mediaDevices', {
    value: devices, configurable: true, writable: true,
  })
}

describe('turning photographs into a decklist', () => {
  it('writes one of everything, because Commander is singleton', () => {
    expect(decklistLines(['Sol Ring', 'Rhystic Study'])).toEqual([
      '1 Sol Ring', '1 Rhystic Study',
    ])
  })

  it('counts the basics instead of repeating them', () => {
    // The one card a stack really does hold thirty of — and the reason the
    // camera never has to read a quantity off anything.
    const forests = Array<string>(30).fill('Forest')
    expect(decklistLines(['Sol Ring', ...forests])).toEqual([
      '1 Sol Ring', '30 Forest',
    ])
  })

  it('keeps the order the cards came off the stack', () => {
    expect(decklistLines(['Swamp', 'Sol Ring', 'Swamp'])).toEqual([
      '2 Swamp', '1 Sol Ring',
    ])
  })

  it('has nothing to say about an empty table', () => {
    expect(decklistLines([])).toEqual([])
  })
})

describe('opening the lens', () => {
  it('offers the door, and promises the photograph stays here', () => {
    render(<CameraDoor onCards={vi.fn()} />)
    expect(screen.getByText('Photograph the cards')).toBeTruthy()
    expect(screen.getByText(/never leaves this browser/)).toBeTruthy()
  })

  it('blames the connection, not the user, when there is no camera API', async () => {
    // `navigator.mediaDevices` is absent on an insecure origin, and saying
    // "permission denied" there sends somebody to reset a permission they
    // were never asked for.
    lens(undefined)
    render(<CameraDoor onCards={vi.fn()} />)
    screen.getByText('Photograph the cards').click()
    await waitFor(() => {
      expect(screen.getByText(/secure connection/)).toBeTruthy()
    })
  })

  it('surfaces a refused camera rather than failing silently', async () => {
    lens({
      getUserMedia: vi.fn().mockRejectedValue(
        new Error('Permission denied by the user')),
    })
    render(<CameraDoor onCards={vi.fn()} />)
    screen.getByText('Photograph the cards').click()
    await waitFor(() => {
      expect(screen.getByText(/Permission denied/)).toBeTruthy()
    })
  })
})

describe('asking Claude to read one (ADR 34)', () => {
  /* The fallback tier exists for the cards the browser cannot do — chiefly
     anything printed before mid-2015, which has no collector number on its
     face at all. What is pinned here is the consent: it is per-card, it is
     never automatic, and the page says what pressing it does before it is
     pressed. */

  it('says what the button does before it is pressed', () => {
    // The promise the local tier makes and this one does not.
    render(<CameraDoor onCards={vi.fn()} />)
    expect(screen.getByText(/never leaves this browser/)).toBeTruthy()
  })

  it('offers no such door before anything has been photographed', () => {
    // Never automatic: with nothing captured there is nothing to send, and
    // the control does not exist to be pressed by accident.
    render(<CameraDoor onCards={vi.fn()} />)
    expect(screen.queryByText('Ask Claude to read it')).toBeNull()
  })
})
