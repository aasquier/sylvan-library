/**
 * What the card finder can say about a card *before* anything is written.
 *
 * These two facts are the reason this file exists at all. The write path
 * refuses a card for exactly three reasons — the library does not know the
 * name, the card is banned in Commander, or its colour identity reaches
 * outside the commander's — and until now all three arrived at the same
 * moment: after a category, a quantity and a whole rationale had been typed.
 *
 * The first is answered by finding the name instead of recalling it. The other
 * two are facts about the card and this deck, knowable the instant a card is
 * highlighted, so they are said then. **They are said, not enforced**: the
 * authoritative refusal stays on the server, which is the one implementation
 * of the rule, and a card that will be refused is still shown and still
 * choosable — hiding it would leave somebody retyping a name that was right
 * all along, which is the same argument as "an invalid deck is simulated, not
 * refused".
 *
 * ## The correction, 2026-09-06
 *
 * "One implementation of the rule" was the intent and not the fact: the
 * identity half was worked out *here*, from `color_identity` against the
 * commander's colours. That is correct for exactly as long as colour identity
 * has no exceptions — and Mystery Booster Commander Edition printed eight
 * commanders that bend it (ADR 51). A mono-white Seluma deck may hold a
 * black-and-white Angel, and this file told the user, in a sentence written to
 * be believed by a beginner, that it could not. **A false refusal is the worst
 * thing this surface can say.** It is not a warning somebody argues with; it
 * is the sentence they trust, and then they stop.
 *
 * So the verdict now comes from the code that owns it — `card.playable`,
 * answered by the same `gate.Rulebreakers` the write path consults — and this
 * file writes the sentence. The local reading below survives as the fallback
 * for a card nobody measured: the commander field, the card-search page, and
 * a payload from before the key existed.
 *
 * Split out of `components/cardfinder.tsx` so they can be tested as what they
 * are — two pure readings of a card against a deck — rather than through a
 * combobox.
 */
import type { CardOffer } from './api'

/** The colours in `card` that this deck's commander cannot carry.
 *
 *  Read off `color_identity`, never derived from the mana cost: a back face,
 *  an activated ability or a hybrid pip can all put a colour in a card's
 *  identity that its cost never shows. */
export function outsideIdentity(card: CardOffer, identity: string[]): string[] {
  const allowed = new Set(identity)
  return card.color_identity.filter((c) => !allowed.has(c))
}

/**
 * The one-line reason this card cannot go in this deck, or `''` for one that
 * can.
 *
 * Written as a sentence somebody can act on, in the words the game uses, and
 * naming the card rather than a rule number — this is the surface where a
 * beginner adds their first card, and "identity violation" teaches nobody
 * anything (commandment 2).
 */
export function cardWarning(card: CardOffer, identity: string[]): string {
  const banned = card.playable ? card.playable.banned : !card.legal_commander
  if (banned) {
    return `${card.name} is banned in Commander, so this deck will not take it.`
  }
  // The server's answer when there is one, the local reading when nobody
  // asked. Never both: a card the library has cleared must not be second-
  // guessed here, which is the whole of the 2026-09-06 correction above.
  const outside = card.playable
    ? card.playable.outside
    : outsideIdentity(card, identity)
  if (outside.length > 0) {
    const colours = outside.map(colourWord).join(' and ')
    return `${card.name} is ${colours}, and your commander is not — `
      + 'so it cannot go in this deck.'
  }
  return ''
}

/**
 * The happy version of the same sentence: a card that is off-colour and legal
 * anyway, because the commander says so.
 *
 * Said out loud rather than left as silence, and that is the point of it. A
 * player who has been taught that colour identity is absolute — which is every
 * player until this set — sees a black Angel accepted into a white deck and
 * has to decide whether the library is broken. One line tells them it is a
 * rule, whose rule it is, and that their deck is fine (commandment 2).
 */
export function cardAllowance(card: CardOffer): string {
  const by = card.playable?.allowed_by
  if (!by || !card.playable?.ok) return ''
  return `${card.name} sits outside your commander's colours, and `
    + `${by}'s Rulebreaker allows it anyway.`
}

/** A colour letter as the word the game prints. Spelled out rather than left
 *  as `{B}` because the letter for black is the one a newcomer guesses
 *  wrong, and this sentence has to teach on its own. */
export function colourWord(code: string): string {
  return COLOUR_WORDS[code] ?? code
}

const COLOUR_WORDS: Record<string, string> = {
  W: 'white', U: 'blue', B: 'black', R: 'red', G: 'green',
}
