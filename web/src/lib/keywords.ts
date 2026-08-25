/**
 * Which keywords the board draws a sign for.
 *
 * **The list lives here and the drawings live in `components/keywords.tsx`**,
 * and they cannot drift apart: the map over there is typed
 * `Record<DrawnKeyword, Glyph>`, so adding a word to this array fails the
 * typecheck until somebody draws it, and drawing one nobody listed fails too.
 * That is the whole reason for two files — a list of names and a set of
 * pictures that are *supposed* to be the same set is exactly the pair that
 * quietly stops being the same set.
 *
 * **Only what is visible on a battlefield.** Flash and the cast-time keywords
 * are not here: they say something about a card that has already happened by
 * the time it is standing on the sand, and a mark answering a question nobody
 * is asking is a mark in the way of one they are. Protection is absent for a
 * different reason — protection *from* something cannot be said without saying
 * from what, and a ten-pixel corner has no room to.
 */

/** In the order a card's own keyword list happens to hold them; this array's
 *  order is not a priority, only a spelling. */
export const DRAWN_KEYWORDS = [
  'flying', 'first strike', 'double strike', 'deathtouch', 'lifelink',
  'vigilance', 'trample', 'haste', 'menace', 'reach', 'hexproof',
  'indestructible', 'ward', 'defender',
] as const

export type DrawnKeyword = (typeof DRAWN_KEYWORDS)[number]

const DRAWN = new Set<string>(DRAWN_KEYWORDS)

/**
 * The keywords on a card that this board can draw, deduplicated, in the card's
 * own order.
 *
 * **Lowercased before matching**, because Scryfall spells them "First strike"
 * and rules text spells them "first strike", and which one reaches a browser
 * is not something a board should be sensitive to.
 *
 * Separate from the drawing so a caller can ask *whether there are any* — or
 * name them in a tooltip, which is how a screen reader gets them, since the
 * marks themselves ride an `aria-hidden` arm — without mounting an icon.
 */
export function drawableKeywords(keywords: string[]): DrawnKeyword[] {
  const seen = new Set<string>()
  const out: DrawnKeyword[] = []
  for (const word of keywords) {
    const key = word.toLowerCase()
    if (!DRAWN.has(key) || seen.has(key)) continue
    seen.add(key)
    out.push(key as DrawnKeyword)
  }
  return out
}
