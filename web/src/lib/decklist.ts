/** Writing a decklist, which is the other half of the server's decklist
 * parser.
 *
 * That parser reads the lists people paste; this one produces the list the
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

/**
 * The warning about photographs that never got a name.
 *
 * Here rather than in `camera.tsx` for two reasons, and the lint rule that
 * sent it here is only the first: a component file that also exports a plain
 * function loses Fast Refresh. The better one is that the count driving this
 * sentence comes from a real lens and a real reader, neither of which jsdom
 * has — inside the component the line is untestable, and untestable is
 * exactly how it came to read *"1 photograph still need a name — they will be
 * left behind."*
 *
 * **One is the ordinary reading here, not the edge case.** Cards are
 * photographed one at a time, so a single unnamed photograph is what a person
 * most often has in front of them; the noun agreed with the count and the
 * verb and the pronoun did not.
 */
export function owingNote(owing: number): string {
  return owing === 1
    ? '1 photograph still needs a name — it will be left behind.'
    : `${owing} photographs still need a name — they will be left behind.`
}
