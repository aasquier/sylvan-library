import { describe, expect, it } from 'vitest'

import { FAN_EDGE, FAN_GAP, FAN_MIN_W, FAN_SLICE, FAN_W, fanHeight, fanWidth,
  placeFan } from './gearfan'

/** A creature standing where a creature stands: in a lane, part way down a
 *  desktop window. */
function card(x = 500, y = 300, w = 58, h = 81): DOMRect {
  return { left: x, top: y, right: x + w, bottom: y + h, width: w, height: h,
    x, y, toJSON: () => ({}) } as DOMRect
}

describe('fanWidth', () => {
  it('is one card for one card', () => {
    expect(fanWidth(1, 100)).toBe(100)
  })

  it('adds a slice for each one after the first', () => {
    expect(fanWidth(3, 100)).toBeCloseTo(100 + 2 * 100 * FAN_SLICE)
  })

  it('is nothing for nothing', () => {
    expect(fanWidth(0, 100)).toBe(0)
  })
})

describe('placeFan', () => {
  it('opens above the creature when there is room', () => {
    // Above by preference: these open off a card in a lane, and a fan that
    // dropped would cover the rows under it — which on this board is the other
    // player's half.
    // Far enough down the window for the fan to fit above it, which at full
    // size is about 284 pixels — a creature in the near half of the board.
    const at = placeFan(card(500, 480), 1440, 900, 3)
    expect(at.under).toBe(false)
    expect(at.top + fanHeight(at.cardW)).toBeCloseTo(480 - FAN_GAP)
  })

  it('drops below when the creature is near the top', () => {
    const at = placeFan(card(500, 12), 1440, 900, 3)
    expect(at.under).toBe(true)
    expect(at.top).toBeGreaterThanOrEqual(FAN_EDGE)
  })

  it('centres on the creature', () => {
    const at = placeFan(card(500, 300), 1440, 900, 3)
    expect(at.left + at.width / 2).toBeCloseTo(500 + 29)
  })

  it('never runs off either side', () => {
    const left = placeFan(card(4, 300), 1440, 900, 4)
    expect(left.left).toBeGreaterThanOrEqual(FAN_EDGE)
    const right = placeFan(card(1400, 300), 1440, 900, 4)
    expect(right.left + right.width).toBeLessThanOrEqual(1440 - FAN_EDGE)
  })

  it('keeps its cards full size when the window has room', () => {
    expect(placeFan(card(), 1440, 900, 4).cardW).toBe(FAN_W)
  })

  it('shrinks the cards rather than clipping the last one', () => {
    // A narrow window is a phone, which is exactly where somebody most needs
    // to know what their creature is wearing. Clipping would hide the last
    // card — the most recently attached, the one that just happened.
    const at = placeFan(card(180, 300), 375, 700, 5)
    expect(at.cardW).toBeLessThan(FAN_W)
    expect(at.width).toBeLessThanOrEqual(375 - 2 * FAN_EDGE + 0.001)
    expect(at.width).toBeCloseTo(fanWidth(5, at.cardW))
  })

  it('stops shrinking at the floor and runs to the edges instead', () => {
    // Past the floor a card's name is a grey smear and the fan has stopped
    // answering the question it opened to answer.
    const at = placeFan(card(180, 300), 320, 700, 9)
    expect(at.cardW).toBe(FAN_MIN_W)
  })

  it('survives a hidden tab reporting a viewport of zero', () => {
    // The guard `placeHint` and `FieldPeek` both carry: a zero means "as much
    // room as the fan wants", never "no room". Nobody is looking at a hidden
    // tab; they are looking the instant it comes back, and what arrives must
    // not be inside out.
    const at = placeFan(card(0, 0, 0, 0), 0, 0, 3)
    expect(at.cardW).toBe(FAN_W)
    expect(at.left).toBeGreaterThanOrEqual(FAN_EDGE)
    expect(Number.isFinite(at.top)).toBe(true)
  })

  it('carries the width it placed, rather than leaving it to be recomputed', () => {
    // The placement and the drawing cannot be allowed to disagree about where
    // the right-hand edge is.
    for (const n of [2, 3, 5]) {
      const at = placeFan(card(), 1440, 900, n)
      expect(at.width).toBeCloseTo(fanWidth(n, at.cardW))
    }
  })
})
