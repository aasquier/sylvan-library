import type { Combo, ComboDraft } from './api'

/**
 * The combos block's pure parts: the heading a combo derives, the steps a
 * `how` is written as, and the two shapes the block moves between.
 *
 * Here rather than in `components/combos.tsx` because they are functions
 * rather than components — the module boundary is what keeps fast refresh
 * working, and it is also what makes them testable without rendering a
 * section.
 */

/**
 * The heading: the pieces, joined with " + ".
 *
 * A combo has no name field. It is called after the cards it is made of, which
 * is how anybody who plays the deck refers to it and the one heading that
 * cannot go stale when a piece is swapped. Derived here exactly as the server
 * derives it (`deck.Combo.Heading`), because it is the entry's only name and
 * two spellings of a name is one too many.
 */
export function headingOf(combo: Combo): string {
  return combo.cards.map((card) => card.name).join(' + ')
}

/**
 * The steps a `how` is written as, or null when it is not written as a list.
 *
 * The block reads "1) … 2) … 3) …" because that is how a person types one and
 * how the intake council emits one, and a wall of prose is the hardest thing on
 * the deck page to follow at a table. Numbering is *detected* rather than
 * imposed: a `how` written as a sentence stays a sentence, because a renderer
 * that renumbered somebody's prose would be inventing a structure they did not
 * write.
 *
 * **The test is that the text *begins* with a marker**, not that it contains
 * one, and the difference is a real bug this had before the test caught it:
 * counting markers reads "You need 5) defenders for this to go infinite" as a
 * two-step list whose first step has no number at all. A numbered list starts
 * at its first step; a sentence with a stray "5)" in the middle of it is a
 * sentence.
 */
export function steps(how: string): string[] | null {
  const text = how.trim()
  // A marker is a number followed by a close paren and a space. `\b` before the
  // digits is what keeps `{2}` and a mana value out of it.
  if (!/^\d{1,2}\)\s/.test(text)) return null
  // Split *before* each marker rather than on it, so the marker travels with
  // the step it introduces and can be stripped exactly once.
  return text.split(/\s*(?=\b\d{1,2}\)\s)/g)
    .map((part) => part.trim()).filter(Boolean)
    .map((part) => part.replace(/^\d{1,2}\)\s*/, ''))
}

/** A combo as it goes back to the server: names, not resolved references. */
export function toDraft(combo: Combo): ComboDraft {
  return {
    cards: combo.cards.map((card) => card.name),
    produces: combo.produces, how: combo.how, setup: combo.setup,
    needs: combo.needs?.name ?? '', cut: combo.cut?.name ?? '',
  }
}

/** An empty entry, for the form that is adding one. */
export function blankDraft(): ComboDraft {
  return { cards: [], produces: '', how: '', setup: '', needs: '', cut: '' }
}

/**
 * The whole block with one entry replaced, and with one removed.
 *
 * Both return drafts rather than combos, because the block is written whole:
 * there is no per-entry route, so every edit is "here is the list afterwards".
 */
export function withEntryAt(combos: Combo[], at: number, entry: ComboDraft): ComboDraft[] {
  return combos.map((combo, i) => (i === at ? entry : toDraft(combo)))
}

export function withoutEntryAt(combos: Combo[], at: number): ComboDraft[] {
  return combos.filter((_, i) => i !== at).map(toDraft)
}
