/**
 * Where each half of a two-named card actually sits on its picture, and the
 * geometry of the glass that points at one.
 *
 * **Four layouts print two spells on one piece of cardboard**, and the middle
 * of the arena has to be able to say *this half, not that one* — a player told
 * "Cast Instant" while looking at a picture of a Giant has been answered with
 * the wrong card in every way but the literal one. #374 found the card; this
 * points at the half.
 *
 * ## Measured, never reasoned about
 *
 * Every number here was read off real Scryfall faces at 488x680 — the size the
 * pool hands the room — across several printings of each layout, and they
 * agree to within half a percent. The gutters are unmistakable in the pixels:
 * a split card's two halves are separated by a black band at 46.8-49.0% down,
 * an Aftermath card's by one at 53.5-56.2%, and a flip card has no band at all
 * because its halves are two text blocks with the art between them.
 *
 * ## The trap this file exists to hold: the halves are not in the same order
 *
 * A split card is **read sideways**, so `card_faces[0]` — the name before the
 * `//` — is the half you meet on the *left*, which in the portrait picture
 * Scryfall serves is the one at the **bottom**. An Aftermath card is read
 * upright and then turned, so its first face is at the **top**. Two layouts
 * Scryfall now calls by the same word, printing their halves in opposite
 * order. A room that assumed one order would draw the glass over Fire while
 * saying Ice, and it would be right half the time, which is the worst kind of
 * wrong to find.
 *
 * ## And the reason the layouts have to be told apart at all
 *
 * Scryfall reclassified Amonkhet's Aftermath cards as `layout: "split"` and
 * marks them with the `Aftermath` keyword instead. The server puts the older,
 * more specific word back on the wire (`go/internal/api/forge.go`), because
 * *where the halves are* is the only thing that reads a layout and the two are
 * not in the same places.
 */

/** One half's box, as percentages of the card's own frame. `.stage-frame` is
 *  the card — `aspect-ratio: 488 / 680` and nothing may argue with it — so a
 *  percentage here is a percentage of the printed card at every size the stage
 *  draws one at. */
export interface HalfBox {
  left: number
  top: number
  right: number
  bottom: number
}

/**
 * The measured boxes, by layout and by face index.
 *
 * Keyed by the index [halfNamed] answers: `0` is the face the card is filed
 * under — the name before the `//` — and `1` is the other one.
 *
 * **An Adventure has one entry and that is deliberate.** Its second half is a
 * small box inside the text box, and its *first* half is the whole card: a
 * Bonecrusher Giant being cast as a creature is the card, whole, and a glass
 * over all of it would be a magnifier pointing at nothing.
 */
export const HALF_BOXES: Record<string, Record<number, HalfBox>> = {
  // Bonecrusher Giant (CLB), read at 488x680: the adventure's frame runs
  // x 30-252 and y 424-608.
  adventure: {
    1: { left: 6.1, top: 62.4, right: 51.6, bottom: 89.4 },
  },
  // Fire // Ice (DMR), Assure // Assemble (RVR), Wear // Tear (MOC) and
  // Boom // Bust (TSR) all put the gutter at 46.8-49.0% and the halves at the
  // full width between the black borders. **Face 0 is the bottom one** — see
  // the file comment.
  split: {
    0: { left: 3.9, top: 49.0, right: 96.0, bottom: 92.8 },
    1: { left: 3.9, top: 2.9, right: 96.0, bottom: 46.9 },
  },
  // Cut // Ribbons, Never // Return and Prepare // Fight (all AKH) put their
  // gutter at 53.5-56.2%, and the upright half — face 0 — is on **top**.
  aftermath: {
    0: { left: 4.0, top: 2.9, right: 96.0, bottom: 53.6 },
    1: { left: 4.0, top: 56.2, right: 96.0, bottom: 92.8 },
  },
  // A flip card's halves are its two text blocks, with one shared painting
  // between them; the second block is printed upside down. Measured on
  // Bushi Tenderfoot and Akki Lavarunner (CHK), Erayo (SOK), Nezumi
  // Graverobber (CM2) and Budoka Gardener (C18) — the old Kamigawa frame ends
  // its art about three per cent lower than the modern one, so the lower box
  // starts at the modern boundary. Bleeding a little art into the glass is
  // nothing; clipping a half's own type line off the top of it is the fault
  // the glass exists to fix.
  flip: {
    0: { left: 5.5, top: 3.4, right: 94.5, bottom: 30.2 },
    1: { left: 5.5, top: 63.0, right: 94.5, bottom: 91.5 },
  },
}

/**
 * How much larger the half is drawn under the glass.
 *
 * **One number for every layout**, which is what keeps this a magnifier rather
 * than four separate treatments. It is what makes the glass overhang the card
 * on a wide half — a split card's half is nearly the whole width, so a lens
 * over it has to be wider than the card, exactly the way a real one held over
 * a card is. On an Adventure's small box it stays comfortably inside.
 *
 * A dial, and Aaron's to move: it is the difference between a lens that
 * whispers and one that shouts, and that is a judgement made by looking.
 */
export const HALF_ZOOM = 1.3

/** Everything the stylesheet needs to draw one glass, in percentages.
 *
 *  Two rectangles: where the glass sits on the card, and where a second copy
 *  of the whole card sits inside the glass. The copy is what gets magnified —
 *  see [halfGlass] for why that is the shape rather than a background. */
export interface HalfGlass {
  /** The glass's own box, as `inset` percentages of the frame. Negative where
   *  the lens overhangs the card, which is not a mistake. */
  left: number
  top: number
  right: number
  bottom: number
  /** The copy of the card inside the glass, as percentages of **the glass**,
   *  laid so it exactly overlays the real card underneath. */
  cardLeft: number
  cardTop: number
  cardWidth: number
  cardHeight: number
  /** The point the copy is scaled about: the half's own centre, as
   *  percentages of the copy — which is to say, of the card. */
  originX: number
  originY: number
}

/**
 * The glass over one half: where it sits, and where the magnified copy of the
 * card sits inside it.
 *
 * ## Why a second copy of the card rather than a background
 *
 * The first cut of this was `background-size: 155% 155%` on the glass box, and
 * **it did the opposite of what it said**. A background percentage resolves
 * against the element it is on — the glass, not the card — so 155% of a box
 * that is itself 45% of the card drew the card at *seventy* per cent of its
 * rendered size: a magnifier that made everything smaller, showing the art and
 * the type line squeezed into the adventure's box. And 155% on both axes of a
 * box that is not the card's shape scaled the two axes differently, which is
 * a *distortion* of a card image — one of the four words ADR 32 and Scryfall's
 * imagery guidelines name outright. It shipped in #374 and nothing could see
 * it: no test can look, and the fault reads as "a slightly odd lens" rather
 * than as arithmetic pointing the wrong way.
 *
 * A copy of the card laid exactly over the real one and then scaled about the
 * half's centre cannot go wrong in either way. It is one uniform `scale()`, so
 * the shape is the card's shape by construction; and the glass box is the same
 * half scaled about the same point, so the copy always covers the glass and
 * what shows through it is exactly the region underneath, larger.
 *
 * ## The arithmetic
 *
 * The copy's placement inside the glass **does not depend on the zoom**, which
 * is the pleasing part: laying the frame over a box is a fact about the box.
 * The zoom moves only the glass's own rectangle, and `transform-origin` keeps
 * the half's centre nailed to the spot it is already on.
 */
export function halfGlass(box: HalfBox, zoom: number = HALF_ZOOM): HalfGlass {
  const wide = box.right - box.left
  const tall = box.bottom - box.top
  const midX = box.left + wide / 2
  const midY = box.top + tall / 2
  const glassWide = wide * zoom
  const glassTall = tall * zoom
  return {
    left: midX - glassWide / 2,
    top: midY - glassTall / 2,
    right: 100 - (midX + glassWide / 2),
    bottom: 100 - (midY + glassTall / 2),
    cardLeft: (-box.left / wide) * 100,
    cardTop: (-box.top / tall) * 100,
    cardWidth: (100 / wide) * 100,
    cardHeight: (100 / tall) * 100,
    originX: midX,
    originY: midY,
  }
}

/**
 * The glass for a card's half, or null when there is nothing to point at.
 *
 * Null is the ordinary answer and it is a real one: an ordinary card has one
 * half, an Adventure cast as its creature is the whole card, and a layout
 * nobody has measured gets the right picture and no rectangle over a guess.
 */
export function halfGlassFor(layout: string | undefined, half: number,
  zoom: number = HALF_ZOOM): HalfGlass | null {
  const box = layout ? HALF_BOXES[layout]?.[half] : undefined
  return box ? halfGlass(box, zoom) : null
}
