/**
 * The table's geometry, at the one place a stylesheet cannot defend itself.
 *
 * Nothing here is about how the felt looks — jsdom computes no layout, and
 * commandment 14 is unambiguous that a green suite has not seen the page.
 * What it pins is the **containing box** the gold frame is measured from,
 * because that is a fact about the DOM and it is what went wrong.
 *
 * `.tarot-place` is `inset: calc(-1 * var(--place-inset))`, so it is exactly
 * as big as whatever box it sits in. It used to sit in `.tarot-slot`, and the
 * slot is the same box as the card only while the legend is out of flow —
 * which it is on a pointer and is **not** on a phone, where `(hover: hover)`
 * leaves the place, the name and the reversal in flow so a newcomer can read
 * them without a pointer to rest. So on a phone each frame grew by however
 * many lines that card's legend ran to: three cards carrying two, three and
 * four lines printed three different-sized places on the cloth, and they
 * crossed each other (Aaron, 2026-08-24).
 *
 * The fix is a box that holds the card and nothing else. A future edit that
 * flattens it away, or that moves the legend inside it, puts the bug back
 * with nothing looking wrong — which is what this file is for.
 */

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TarotTable } from './tarot'
import { api, type TarotDrawn } from '../lib/api'

vi.mock('../lib/api', async (importOriginal) => {
  const real = await importOriginal<typeof import('../lib/api')>()
  return { ...real, api: { ...real.api, personas: vi.fn(), tarotReading: vi.fn() } }
})

afterEach(cleanup)

/** A card in a slot. `note` is deliberately long on one of them, because the
 *  legend's line count is the variable that broke the frame. */
function drawn(key: string, position: string, note: string | null): TarotDrawn {
  return {
    key, name: key, face_name: key, arcana: 'minor', suit: 'swords', number: 4,
    image: `/tarot/${key}.webp`, artist: null, after: null, note,
    reversed: false, slot: 'taste', position,
  }
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(api.personas).mockResolvedValue({
    default: 'plain',
    personas: [{ key: 'fortune-teller', label: 'Read my fortune',
                 blurb: 'Three cards.', deals: true }],
  })
  vi.mocked(api.tarotReading).mockResolvedValue({
    seed: 7,
    cards: [
      drawn('four-of-swords', 'THE ROOT', null),
      drawn('four-of-cups', 'THE TURNING', 'A long note that runs to several '
        + 'lines under the card, which is the whole point of this fixture.'),
      drawn('ten-of-pentacles', 'THE TABLE', null),
    ],
  })
})

/** Sit down at the dealing reader's table and wait for the three cards. */
async function deal() {
  render(<TarotTable onPick={() => {}} onLeave={() => {}} />)
  const reader = await screen.findByRole('button', { name: /Read my fortune/ })
  reader.click()
  await waitFor(() => {
    expect(document.querySelectorAll('.tarot-slot')).toHaveLength(3)
  }, { timeout: 4000 })
  return [...document.querySelectorAll<HTMLElement>('.tarot-slot')]
}

describe('the place printed on the cloth', () => {
  it('is measured from a box holding the card alone', async () => {
    const slots = await deal()
    for (const slot of slots) {
      const seat = slot.querySelector('.tarot-seat')
      expect(seat).not.toBeNull()
      // The frame and the card are in it...
      expect(seat!.querySelector('.tarot-place')).not.toBeNull()
      expect(seat!.querySelector('.tarot-hinge')).not.toBeNull()
      // ...and the legend, whose height varies per card, is not.
      expect(seat!.querySelector('.tarot-legend')).toBeNull()
      expect(slot.querySelector(':scope > .tarot-legend')).not.toBeNull()
    }
  })
})
