/**
 * Where a geared creature's fan of cards goes, and how wide it opens.
 *
 * Pure arithmetic, in `lib/` for the reason the whole directory exists: **the
 * one thing the web suite cannot check is layout.** jsdom lays nothing out, so
 * a component test can assert a fan was rendered and can never assert it
 * landed on the screen. Given a rectangle, a viewport and a count, the fan's
 * geometry is a function — and a function is checkable. `lib/hint.ts` is the
 * same argument at a smaller size, and this borrows its shape deliberately.
 *
 * `components/gearfan.tsx` is the component, and it carries the argument for
 * why the fan exists at all.
 */

/** One card's width when the fan has all the room it wants.
 *
 *  Chosen against what a person is asking. The question a fan answers is
 *  *what is my Cat wearing* — a name and a picture — and a Magic card sets its
 *  name legibly from about a hundred and forty pixels of width. The peek shows
 *  one card at 272 and can afford to; a fan shows four or five at once and
 *  cannot. */
export const FAN_W = 168

/** How much of a covered card shows: its left third, which is the band a
 *  Magic card puts its mana cost and the left edge of its art in.
 *
 *  It is the *fan* proportion rather than a pixel count so it survives the
 *  shrink below — a fan squeezed onto a phone stays the same shape, only
 *  smaller. */
export const FAN_SLICE = 0.35

/** The floor the shrink stops at. Under this a card's name is a grey smear
 *  and the fan has stopped answering the question it opened to answer — better
 *  to run to the edges of a narrow window than to draw six illegible cards. */
export const FAN_MIN_W = 104

/** How close to the edge of the window the fan may come. */
export const FAN_EDGE = 8

/** The gap between the creature and the fan it raised. Enough that the fan
 *  reads as belonging to that creature rather than as covering it. */
export const FAN_GAP = 10

/** How far the card under the pointer lifts out of the fan. Room has to be
 *  reserved for it above the cards, or the lift is clipped by the viewport
 *  clamp at exactly the moment somebody reaches for the top card. */
export const FAN_LIFT = 16

/** Room under the cards for the raised card's name. One line, which is what
 *  the caption sets — the name, and for an attachment what it is attached to,
 *  wrapped to a second line only on a phone. */
export const FAN_CAPTION = 34

/** A Magic card's proportions, which nothing here may argue with. */
const CARD_TALL = 680 / 488

/** How wide a fan of `n` cards opens at a given card width.
 *
 *  **Constant, whichever card is raised**, and that is the whole reason the
 *  raised card *lifts* rather than pushing its neighbours along. A fan that
 *  changed width as the pointer crossed it would reflow under the hand doing
 *  the crossing, and every card would be somewhere else by the time it was
 *  reached for. Pulling one card up out of a fan is also what a hand does. */
export function fanWidth(n: number, cardW: number): number {
  if (n <= 0) return 0
  return (n - 1) * cardW * FAN_SLICE + cardW
}

/** How tall the whole thing is: the cards, the room the raised one lifts into,
 *  and the line of type under them. */
export function fanHeight(cardW: number): number {
  return cardW * CARD_TALL + FAN_LIFT + FAN_CAPTION
}

/** Where the fan is, in viewport coordinates, and how big its cards had to
 *  become to fit. */
export interface FanPlace {
  left: number
  top: number
  /** One card's width. [FAN_W] when there is room, less on a narrow window,
   *  never below [FAN_MIN_W]. */
  cardW: number
  /** The whole fan's width, which is [fanWidth] at that card width — carried
   *  rather than recomputed, so the placement and the drawing cannot disagree
   *  about where the right-hand edge is. */
  width: number
  /** Whether the fan ended up under the creature rather than over it. */
  under: boolean
}

/**
 * Above the creature by preference, below it when there is no room, and never
 * off the side of the window.
 *
 * **Above first, for the keyword panel's reason and one of its own.** These
 * open off a card in a lane, and a fan that dropped would cover the rows under
 * it — which on this board is the other player's half. The one of its own is
 * that a creature carrying things is usually in the *creature* lane, which is
 * the front line: the row nearest the seam and the one with the most going on
 * under it.
 *
 * **The cards shrink before the fan clips.** A window narrower than the fan
 * wants is a phone, and a phone is exactly where somebody most needs to know
 * what their creature is wearing. Shrinking keeps every card whole and only
 * makes them smaller; clipping would hide the last one entirely, and the last
 * one is the most recently attached — the one that just happened.
 */
export function placeFan(at: DOMRect, wide: number, tall: number, n: number):
FanPlace {
  // **A hidden tab reports a viewport of zero** and every sum below would
  // inherit the lie — the same guard `placeHint` and `FieldPeek` carry, and
  // the same answer: a zero means "as much room as the fan wants" rather than
  // "no room". Nobody is looking at a hidden tab; they are looking the instant
  // it comes back, and what arrives must not be inside out.
  const room = wide > 0 ? wide : fanWidth(n, FAN_W) + 2 * FAN_EDGE
  const h = tall > 0 ? tall : fanHeight(FAN_W) + 2 * FAN_EDGE
  const have = Math.max(1, room - 2 * FAN_EDGE)

  // Shrink to fit, and stop at the floor: past it the fan runs to the window's
  // edges rather than drawing cards nobody can read.
  let cardW = FAN_W
  if (fanWidth(n, cardW) > have) {
    const denom = (n - 1) * FAN_SLICE + 1
    cardW = Math.max(FAN_MIN_W, have / denom)
  }
  const width = fanWidth(n, cardW)
  const height = fanHeight(cardW)

  const want = at.left + at.width / 2 - width / 2
  const left = Math.min(Math.max(want, FAN_EDGE),
    Math.max(FAN_EDGE, room - FAN_EDGE - width))

  const above = at.top - FAN_GAP - height
  if (above >= FAN_EDGE) {
    return { left, top: above, cardW, width, under: false }
  }
  // **Neither side fits: sit it as high as the window allows.** Downward, that
  // keeps the *cards* on the screen and lets the caption be the thing that
  // hangs off the bottom — the picture is the answer for anybody who can see
  // it, and the fan is still readable with its last line cut.
  const below = at.bottom + FAN_GAP
  return {
    left,
    top: Math.max(FAN_EDGE, Math.min(below, h - FAN_EDGE - height)),
    cardW,
    width,
    under: true,
  }
}
