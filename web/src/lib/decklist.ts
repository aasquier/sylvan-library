/** Writing a decklist, which is the other half of `decks/decklist.py`.
 *
 * That module parses the lists people paste; this one produces the list the
 * camera door hands back. They meet in the middle: what is written here is
 * fed straight into the box on the Import page and parsed there, so the
 * camera gets no privileged path into a deck — same preview, same gate,
 * same draft with its rationales still owed (ADR 13).
 */

/**
 * Chosen card names as decklist lines.
 *
 * Counted rather than repeated, because a stack of cards holds thirty
 * Forests and `30 Forest` is what every real export writes. Commander's
 * singleton rule is what makes this safe for everything else — and it is
 * also the reason a camera never has to read a quantity off anything, which
 * removes the hardest field on the card.
 *
 * Insertion order is kept, so the list reads back in the order the cards
 * came off the stack rather than in some order of the map's choosing.
 */
export function decklistLines(chosen: string[]): string[] {
  const counts = new Map<string, number>()
  for (const name of chosen) counts.set(name, (counts.get(name) ?? 0) + 1)
  return [...counts].map(([name, qty]) => `${qty} ${name}`)
}
