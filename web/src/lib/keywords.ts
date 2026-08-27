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
 *
 * **A keyword that carries a number still gets a mark, and the mark does not
 * say the number.** Ward has said this since the list was written — the sigil
 * means *there is a toll*, and what the toll is stays on the card — and toxic
 * joins it on the same terms. Scryfall's `keywords` array spells both without
 * their number ("Ward", "Toxic"), so the amount was never on the wire to draw
 * anyway; a corner that answers *does this creature poison me* is the question
 * a player is actually asking across the table, and *how much* is the one they
 * ask afterwards, with the card already in their hand.
 */

/** In the order a card's own keyword list happens to hold them; this array's
 *  order is not a priority, only a spelling.
 *
 *  Deathtouch and toxic sit next to each other on purpose: they are the two
 *  words that make ordinary combat damage mean something worse than its
 *  number, they appear together on real cards (Bilious Skulldweller carries
 *  both), and two marks that can stand in one corner are exactly the pair that
 *  must not look alike. `components/keywords.tsx` draws them a skull and a
 *  viper and argues the split there. */
export const DRAWN_KEYWORDS = [
  'flying', 'first strike', 'double strike', 'deathtouch', 'toxic', 'lifelink',
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

/**
 * Every keyword this board can draw for a creature, said in words, with the
 * lent ones saying so.
 *
 * **One phrasing, in one place.** The marks are pictures on an `aria-hidden`
 * arm, so a sentence is the whole accessible account of them *and* the whole
 * of what a pointer resting on one gets — and the card's own tooltip says the
 * same list one element up. Three places, one spelling: change it here.
 *
 * A bare parenthetical, with no room in the phrasing for a culprit. `granted`
 * is `BoardCard.granted`, which is the printing subtracted from the live set
 * and carries no source, so *that* it was given is the whole of what may be
 * said. Matched lowercased, because `drawableKeywords` answers lowercased and
 * Forge does not.
 */
export function keywordWords(keywords: string[], granted: string[] = []) {
  const lent = new Set(granted.map((k) => k.toLowerCase()))
  return drawableKeywords(keywords)
    .map((k) => (lent.has(k) ? `${k} (granted)` : k))
}
