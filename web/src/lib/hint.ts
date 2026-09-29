/**
 * Where a mark's explaining panel goes.
 *
 * Pure arithmetic, on purpose and in `lib/` for the reason the whole of this
 * directory exists: **the one thing the web suite cannot check is layout.**
 * jsdom applies no stylesheet and lays nothing out, so a component test can
 * assert a panel was rendered and can never assert it landed on the screen —
 * which is how this project once shipped zones drawn at zero pixels with 734
 * tests green. Given a rectangle and a viewport, placement is a function, and
 * a function is checkable. Measuring the real browser is still the other half
 * and always will be.
 *
 * `components/hint.tsx` is the component that uses this and carries the
 * argument for why these panels exist at all.
 */

/** How wide the panel wants to be. A sentence from `KEYWORD_MEANS` runs to
 *  about ninety characters, which sets two or three lines at this width —
 *  short enough to read standing up, which is the whole brief. */
export const HINT_W = 208
/** How close to the edge of the window the panel may come. */
export const HINT_EDGE = 8
/** The gap between the mark and the panel it raised. Enough that the panel
 *  reads as *pointing at* the mark rather than as covering it. */
export const HINT_GAP = 8

/** Where the panel is, in viewport coordinates. */
export interface HintPlace {
  left: number
  top: number
  width: number
  /** Whether the panel ended up under its mark rather than over it, so the
   *  notch can be drawn on the edge that faces the mark. */
  under: boolean
}

/**
 * Above the mark by preference, below it when there is no room above, and
 * never off the side of the window.
 *
 * Above first because these marks live in the *upper* corner of a card in a
 * lane, and a panel that drops covers the two rows of cards under it — which
 * on this board is the other player's half.
 */
export function placeHint(at: DOMRect, wide: number, tall: number,
  panel: { w: number; h: number }): HintPlace {
  // **A hidden tab reports a viewport of zero** and every sum below would
  // inherit the lie — the same guard `FieldPeek` carries, and the same answer:
  // a zero means "as much room as the panel wants" rather than "no room".
  // Nobody is looking at a hidden tab; they are looking the instant it comes
  // back, and what arrives must not be inside out.
  const w = wide > 0 ? wide : panel.w + 2 * HINT_EDGE
  const h = tall > 0 ? tall : panel.h + 2 * HINT_EDGE
  const width = Math.max(1, Math.min(panel.w, w - 2 * HINT_EDGE))
  const want = at.left + at.width / 2 - width / 2
  const left = Math.min(Math.max(want, HINT_EDGE),
    Math.max(HINT_EDGE, w - HINT_EDGE - width))
  const above = at.top - HINT_GAP - panel.h
  if (above >= HINT_EDGE) return { left, top: above, width, under: false }
  // **Neither side fits: sit it as low as the window allows.** That keeps the
  // first line — the word itself — on the screen. A panel hanging off the
  // bottom is still readable from the top down; one placed off the top is not
  // readable at all.
  const below = at.bottom + HINT_GAP
  return {
    left,
    top: Math.min(below, Math.max(HINT_EDGE, h - HINT_EDGE - panel.h)),
    width,
    under: true,
  }
}

/**
 * How wide the glossary's panel wants to be. Wider than a board mark's,
 * because these hold a whole definition rather than a keyword's sentence, and
 * it is the width the panel has always had (`w-64`).
 */
export const TIP_W = 256

/**
 * The same arithmetic, the other way up: **below** the word by preference,
 * above it when there is no room below.
 *
 * Below first because a glossary mark sits in a line of prose or beside a
 * label, and a panel that rises covers the sentence the reader was in the
 * middle of. A board mark is the opposite case and `placeHint` is its answer;
 * the two share the horizontal clamp because falling off the side of a window
 * is not a matter of taste.
 *
 * **The clamp is why this function exists at all.** The panel was
 * `absolute; left: 0; width: 16rem` inside the trigger, which is fine for a
 * mark on the left of a wide screen and is a document 34 pixels wider than the
 * window for a mark halfway across a phone — measured on the deck page at
 * 375, with one category's help open.
 */
export function placeTip(at: DOMRect, wide: number, tall: number,
  panel: { w: number; h: number }): HintPlace {
  // The hidden-tab guard `placeHint` carries, for the same reason.
  const w = wide > 0 ? wide : panel.w + 2 * HINT_EDGE
  const h = tall > 0 ? tall : panel.h + 2 * HINT_EDGE
  const width = Math.max(1, Math.min(panel.w, w - 2 * HINT_EDGE))
  const want = at.left + at.width / 2 - width / 2
  const left = Math.min(Math.max(want, HINT_EDGE),
    Math.max(HINT_EDGE, w - HINT_EDGE - width))
  const below = at.bottom + HINT_GAP
  if (below + panel.h <= h - HINT_EDGE) {
    return { left, top: below, width, under: true }
  }
  const above = at.top - HINT_GAP - panel.h
  // Neither side fits: keep the first line — the word itself — on the screen,
  // which is `placeHint`'s ruling and holds here for the same reason.
  return {
    left,
    top: Math.max(HINT_EDGE, Math.min(above, h - HINT_EDGE - panel.h)),
    width,
    under: false,
  }
}
