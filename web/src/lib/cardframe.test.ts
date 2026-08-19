/** Where the words are on a Magic card.
 *
 * There used to be a `parseCorner` here with nine tests around it, all
 * passing, all wrong. It pulled a set code out of the corner using string
 * rules, and a real capture returns `LTCENLIK` — the code welded to the
 * language tag and the artist's initials. No client-side rule finds `LTC`
 * in that, because "is this a set code" is a question about which 986
 * exist. That job is `cards/identify.py`'s now, and its tests run against
 * the verbatim output of a real reader on a real card.
 *
 * What is left here is what the browser genuinely owns: the rectangles, and
 * tidying a title that nothing will correct.
 */

import { describe, expect, it } from 'vitest'
import { CARD_ASPECT, cleanTitle, CORNER, pixels, TITLE } from './cardframe'

describe('tidying a title', () => {
  it('collapses what a crop edge adds', () => {
    expect(cleanTitle('  Craterhoof   Behemoth \n')).toBe('Craterhoof Behemoth')
  })

  it('drops the frame the crop caught', () => {
    // The real capture of a Sol Ring came back with a bracket on the front.
    expect(cleanTitle('( Sol Ring')).toBe('Sol Ring')
  })

  it('keeps the punctuation that is part of a name', () => {
    expect(cleanTitle('Gyome, Master Chef')).toBe('Gyome, Master Chef')
    expect(cleanTitle("Urza's Saga")).toBe("Urza's Saga")
  })

  it('never corrects a name', () => {
    // Correcting is the pool's job, and the reason the title tier offers a
    // shortlist rather than an answer.
    expect(cleanTitle('Craterhoof Behernoth')).toBe('Craterhoof Behernoth')
  })

  it('has nothing to say about an unreadable crop', () => {
    expect(cleanTitle('   \n  ')).toBe('')
  })
})

describe('the crops', () => {
  /* The corner numbers come from measuring a real card's pixel rows, not
     from looking at a card and estimating. The estimate was 0.875 and the
     truth is 0.935 — a sixth of the card out, far enough that the reader
     was returning rules text that looked exactly like a set code. */
  const INK = { line1: [0.9353, 0.9471], line2: [0.9529, 0.9647],
                left: 0.066, right: 0.453 }

  it('contains the ink that was actually measured', () => {
    expect(CORNER.top).toBeLessThan(INK.line1[0]!)
    expect(CORNER.bottom).toBeGreaterThan(INK.line2[1]!)
    expect(CORNER.left).toBeLessThan(INK.left)
    expect(CORNER.right).toBeGreaterThan(INK.right)
  })

  it('stays out of the rules text box, which is what broke it before', () => {
    // The text box ends at 0.927 on the card that was measured. Crossing
    // that line is how `TTBS` and `MAS` got read as set codes.
    expect(CORNER.top).toBeGreaterThan(0.927)
  })

  it('are inside the card, and do not overlap', () => {
    for (const region of [TITLE, CORNER]) {
      expect(region.left).toBeGreaterThanOrEqual(0)
      expect(region.right).toBeLessThanOrEqual(1)
      expect(region.top).toBeGreaterThanOrEqual(0)
      expect(region.bottom).toBeLessThanOrEqual(1)
      expect(region.right).toBeGreaterThan(region.left)
      expect(region.bottom).toBeGreaterThan(region.top)
    }
    expect(CORNER.top).toBeGreaterThan(TITLE.bottom)
  })

  it('is the shape of a Magic card', () => {
    expect(CARD_ASPECT).toBeCloseTo(0.716, 3)
  })

  it('rounds outward, so no descender is clipped', () => {
    const box = pixels({ left: 0.101, top: 0.101, right: 0.899, bottom: 0.899 },
                       1000, 1000)
    expect(box.left).toBe(101)
    expect(box.top).toBe(101)
    expect(box.left + box.width).toBe(899)
  })

  it('never produces an empty crop', () => {
    const box = pixels(TITLE, 4, 4)
    expect(box.width).toBeGreaterThan(0)
    expect(box.height).toBeGreaterThan(0)
  })
})
