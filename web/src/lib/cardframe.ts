/** Where the words are on a Magic card, and what to make of them.
 *
 * Pure geometry and pure text — no camera, no OCR engine, no network. That
 * split is deliberate: the two things most likely to be *wrong* here are the
 * crop rectangles and the corner parser, and both can be tested without a
 * webcam or four megabytes of WebAssembly.
 *
 * ## Why the corner rather than the title
 *
 * A card's bottom-left carries its collector number and set code in a tiny
 * fixed-position alphabet:
 *
 *     284/281 C
 *     LTC • EN   Mike Burns
 *
 * Reading those two fields is worth more than reading the title, because the
 * pair is a *lookup* — one row of `printings`, no judgement — while a title
 * is a similarity whose right and wrong answers score in overlapping ranges
 * (the server's card reader carries the measurement). It is also
 * language-independent: the set code of a Japanese Sol Ring is still `LTC`.
 *
 * ## Why the title is cropped anyway
 *
 * **Cards printed before mid-2015 have no collector number on the face at
 * all.** The bottom-left info line arrived with the Magic Origins frame, so
 * every dual land, every Ravnica shock, every Innistrad flip card reads
 * nothing at all down there. Those cards are exactly the deep cuts this
 * library is full of, so the title crop is not a fallback bolted on — it is
 * the only tier that works on half of Magic.
 */

/** A crop, in fractions of the card's own width and height.
 *
 * Fractions rather than pixels because the card in the viewfinder is
 * whatever size the phone's camera and the user's hands made it.
 */
export interface Region {
  left: number
  top: number
  right: number
  bottom: number
}

/** 63mm x 88mm, the size a Magic card has been since 1993. */
export const CARD_ASPECT = 63 / 88

/** The name bar of a modern frame, with room for the mana cost's gutter cut
 *  off the right — pips are not letters and only confuse a line reader. */
export const TITLE: Region = { left: 0.055, top: 0.038, right: 0.78, bottom: 0.105 }

/** The bottom-left info block: collector number, then set code and language.
 *
 * **Measured, not guessed, and the guess was badly wrong.** Reading the
 * pixel rows of a real card put the two lines at 0.9353–0.9471 and
 * 0.9529–0.9647 of the card's height, and their ink between 0.066 and 0.453
 * of its width. The first version of this constant started at 0.875 — inside
 * the rules text box — and the reader duly returned the bottom of the rules
 * text: `TTBS`, `MAS`. It looked exactly like a set code and was a sentence.
 *
 * A little air is left around the measurement because a photograph is not a
 * scan; a card held by hand is never perfectly square to the lens.
 */
export const CORNER: Region = { left: 0.045, top: 0.928, right: 0.49, bottom: 0.972 }

/** Characters the corner can contain. Everything the reader is allowed to
 *  see there is a digit, a capital, a slash or a star, and saying so is the
 *  single largest accuracy win available on a four-millimetre glyph. */
export const CORNER_ALPHABET =
  '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ/'

/** Why there is no `parseCorner` here.
 *
 * There was one, and it was wrong in a way no test of it could show. The
 * bottom-left of a card prints the set code, the language and the artist on
 * one line, and a real reader returns them welded together — an actual
 * capture came back as `LTCENLIK`. Pulling `LTC` out of that is not string
 * work, it is a question about which of 986 codes exist, and the answer
 * lives in the pool.
 *
 * So the corner crosses the wire as text and the server's card reader reads
 * it. The browser's job is to find the pixels; deciding what they say is the
 * pool's, which is `ADR 14` applied to a camera.
 */

/**
 * Tidy a title read.
 *
 * Only the damage OCR reliably does to a card name: collapsed whitespace,
 * and the leading/trailing junk a crop's edge produces. Nothing here tries
 * to *correct* a name — that is the pool's job, and the whole reason the
 * title tier offers a shortlist instead of an answer.
 */
export function cleanTitle(text: string): string {
  return text
    .replace(/[\r\n]+/g, ' ')
    .replace(/\s+/g, ' ')
    .replace(/^[^A-Za-z]+/, '')
    .replace(/[^A-Za-z.'!,\-’)\]]+$/, '')
    .trim()
}

/**
 * The pixel rectangle a `Region` names inside a source of this size.
 *
 * Rounded outward, so a crop never loses the row of pixels a glyph's
 * descender is sitting on.
 */
export function pixels(region: Region, width: number, height: number) {
  const left = Math.max(0, Math.floor(region.left * width))
  const top = Math.max(0, Math.floor(region.top * height))
  const right = Math.min(width, Math.ceil(region.right * width))
  const bottom = Math.min(height, Math.ceil(region.bottom * height))
  return { left, top, width: Math.max(1, right - left), height: Math.max(1, bottom - top) }
}
