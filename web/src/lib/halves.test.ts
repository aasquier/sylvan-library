import { describe, expect, it } from 'vitest'

import { HALF_BOXES, type HalfBox, halfGlass, halfGlassFor } from './halves'

/** Where a point on the card ends up once the glass has magnified it: the
 *  copy is laid over the frame and scaled about the half's centre, so a point
 *  `p` (in frame percentages) lands at `origin + (p - origin) * zoom`.
 *
 *  Written out rather than imported, because a test that reuses the code's own
 *  arithmetic proves the code agrees with itself and nothing else. */
function magnified(at: number, origin: number, zoom: number): number {
  return origin + (at - origin) * zoom
}

describe('the measured boxes', () => {
  it('puts a split card\'s first face at the BOTTOM and an aftermath\'s at the '
    + 'top', () => {
    // The trap this file exists for. Scryfall calls both layouts `split`; a
    // split card is read sideways so `Fire // Ice` prints Fire on the left,
    // which is the bottom of the portrait picture, and an aftermath card is
    // read upright so `Cut // Ribbons` prints Cut on top. A room that assumed
    // one order would point at the wrong half of exactly half of them.
    //
    // Asked of each box's **centre** rather than its top edge, which is the
    // claim being made: a split card's gutter sits at 47-49%, so the lower
    // half's top edge is a hair under the halfway line and an assertion about
    // it would be measuring the gutter rather than the order.
    const mid = (b?: HalfBox) => ((b as HalfBox).top + (b as HalfBox).bottom) / 2
    expect(mid(HALF_BOXES.split?.[0])).toBeGreaterThan(50)
    expect(mid(HALF_BOXES.split?.[1])).toBeLessThan(50)
    expect(mid(HALF_BOXES.aftermath?.[0])).toBeLessThan(50)
    expect(mid(HALF_BOXES.aftermath?.[1])).toBeGreaterThan(50)
    // And the flip card, whose halves are the two text blocks.
    expect(mid(HALF_BOXES.flip?.[0])).toBeLessThan(50)
    expect(mid(HALF_BOXES.flip?.[1])).toBeGreaterThan(50)
  })

  it('never lets two halves of one card overlap', () => {
    for (const [layout, halves] of Object.entries(HALF_BOXES)) {
      const boxes = Object.values(halves)
      if (boxes.length < 2) continue
      const [a, b] = [...boxes].sort((x, y) => x.top - y.top) as [HalfBox,
        HalfBox]
      expect(a.bottom, `${layout}: the upper half ends before the lower begins`)
        .toBeLessThan(b.top)
    }
  })

  it('keeps every box on the card', () => {
    for (const [layout, halves] of Object.entries(HALF_BOXES)) {
      for (const [half, box] of Object.entries(halves)) {
        const where = `${layout} half ${half}`
        expect(box.left, where).toBeGreaterThanOrEqual(0)
        expect(box.top, where).toBeGreaterThanOrEqual(0)
        expect(box.right, where).toBeLessThanOrEqual(100)
        expect(box.bottom, where).toBeLessThanOrEqual(100)
        expect(box.right, where).toBeGreaterThan(box.left)
        expect(box.bottom, where).toBeGreaterThan(box.top)
      }
    }
  })

  it('gives an Adventure a glass on its second half only', () => {
    // Casting a Bonecrusher Giant as a creature is casting the whole card, and
    // a magnifier over all of it points at nothing.
    expect(HALF_BOXES.adventure?.[0]).toBeUndefined()
    expect(HALF_BOXES.adventure?.[1]).toBeDefined()
  })
})

describe('halfGlass', () => {
  const box: HalfBox = { left: 20, top: 30, right: 60, bottom: 70 }

  it('is the half itself at a zoom of one', () => {
    const g = halfGlass(box, 1)
    expect(g.left).toBeCloseTo(20)
    expect(g.top).toBeCloseTo(30)
    expect(g.right).toBeCloseTo(40)   // inset from the right edge
    expect(g.bottom).toBeCloseTo(30)
  })

  it('grows about the half\'s own centre, so the centre does not move', () => {
    const one = halfGlass(box, 1)
    const two = halfGlass(box, 2)
    const mid = (g: ReturnType<typeof halfGlass>) =>
      [(g.left + (100 - g.right)) / 2, (g.top + (100 - g.bottom)) / 2]
    expect(mid(two)[0]).toBeCloseTo(mid(one)[0] as number)
    expect(mid(two)[1]).toBeCloseTo(mid(one)[1] as number)
  })

  it('lays the copy exactly over the real card, whatever the zoom', () => {
    // The copy's placement is a fact about the box and not about the zoom —
    // scaling is what the transform is for. If these ever start disagreeing,
    // the glass is showing a card that is not the card underneath it.
    for (const zoom of [1, 1.3, 2.4]) {
      const g = halfGlass(box, zoom)
      // The copy spans the frame: its left edge sits at -left/width of the box
      // and it is 100/width of the box wide, so the frame's right edge lands
      // exactly where the card's does.
      expect(g.cardLeft + g.cardWidth).toBeCloseTo(
        ((100 - box.left) / (box.right - box.left)) * 100)
      expect(g.cardTop + g.cardHeight).toBeCloseTo(
        ((100 - box.top) / (box.bottom - box.top)) * 100)
    }
  })

  it('scales about the half\'s centre', () => {
    const g = halfGlass(box, 1.3)
    expect(g.originX).toBeCloseTo(40)
    expect(g.originY).toBeCloseTo(50)
  })

  it('always covers its own glass, so no corner shows through', () => {
    // The copy is the whole card scaled about the half's centre and the glass
    // is the half scaled about the same point, so the first contains the
    // second by construction. Checked as geometry rather than trusted: a glass
    // with a gap in it would draw the arena floor through a card.
    for (const halves of Object.values(HALF_BOXES)) {
      for (const box of Object.values(halves)) {
        const zoom = 1.3
        const g = halfGlass(box, zoom)
        const cardLeft = magnified(0, g.originX, zoom)
        const cardTop = magnified(0, g.originY, zoom)
        const cardRight = magnified(100, g.originX, zoom)
        const cardBottom = magnified(100, g.originY, zoom)
        expect(cardLeft).toBeLessThanOrEqual(g.left + 1e-9)
        expect(cardTop).toBeLessThanOrEqual(g.top + 1e-9)
        expect(cardRight).toBeGreaterThanOrEqual(100 - g.right - 1e-9)
        expect(cardBottom).toBeGreaterThanOrEqual(100 - g.bottom - 1e-9)
      }
    }
  })

  it('shows the whole half and nothing of the other one', () => {
    // The two things the glass has to be right about at once: a half whose own
    // name is clipped off the top of the lens has failed at the one job, and a
    // lens showing both halves has not said which was cast.
    for (const [layout, halves] of Object.entries(HALF_BOXES)) {
      for (const [key, box] of Object.entries(halves)) {
        const zoom = 1.3
        const g = halfGlass(box, zoom)
        const where = `${layout} half ${key}`
        // Every corner of the half lands inside the glass.
        expect(magnified(box.left, g.originX, zoom), where)
          .toBeGreaterThanOrEqual(g.left - 1e-9)
        expect(magnified(box.right, g.originX, zoom), where)
          .toBeLessThanOrEqual(100 - g.right + 1e-9)
        expect(magnified(box.top, g.originY, zoom), where)
          .toBeGreaterThanOrEqual(g.top - 1e-9)
        expect(magnified(box.bottom, g.originY, zoom), where)
          .toBeLessThanOrEqual(100 - g.bottom + 1e-9)
        // And the other half of the same card is nowhere in it.
        const other = Object.entries(halves)
          .find(([k]) => k !== key)?.[1]
        if (!other) continue
        const otherTop = magnified(other.top, g.originY, zoom)
        const otherBottom = magnified(other.bottom, g.originY, zoom)
        const above = otherBottom <= g.top + 1e-9
        const below = otherTop >= 100 - g.bottom - 1e-9
        expect(above || below, `${where}: the other half is outside the glass`)
          .toBe(true)
      }
    }
  })
})

describe('halfGlassFor', () => {
  it('answers null for an ordinary card', () => {
    expect(halfGlassFor(undefined, 0)).toBeNull()
    expect(halfGlassFor('normal', 0)).toBeNull()
  })

  it('answers null for a beat that named no half at all', () => {
    // `halfNamed` says -1 when the beat is about some other card entirely.
    expect(halfGlassFor('split', -1)).toBeNull()
  })

  it('answers null for a layout nobody has measured', () => {
    // A rectangle over a guess is worse than no rectangle.
    expect(halfGlassFor('transform', 1)).toBeNull()
    expect(halfGlassFor('meld', 1)).toBeNull()
  })

  it('answers null for an Adventure cast as its creature', () => {
    expect(halfGlassFor('adventure', 0)).toBeNull()
    expect(halfGlassFor('adventure', 1)).not.toBeNull()
  })

  it('gives a split card\'s two halves two different glasses', () => {
    const bottom = halfGlassFor('split', 0)
    const top = halfGlassFor('split', 1)
    expect(bottom?.top).toBeGreaterThan(top?.top as number)
  })
})
