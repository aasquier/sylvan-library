/**
 * What the pot says, and the one place it says it.
 *
 * The caption across the foot of the plate and the interview's live region
 * print the **same string** — never a second wording of it — which is
 * `components/theme.tsx`'s standing rule for the séance one room over and the
 * reason this is a table rather than two sentences written twice.
 *
 * It lives in `lib/` beside `claudecopy.ts` and `motion.ts` for their reason:
 * these are not components, two components need each of them, and oxlint's
 * fast-refresh rule is right about what that means.
 */

import type { BrewIngredient } from './api'

/**
 * What the pot says when something goes in. One sentence per step: the count
 * is the thing a newcomer needs, and the colour is the thing they may not be
 * able to see.
 *
 * **Led by the PLACE, not by the ingredient**, and that is not a style choice.
 * The tarot's live region already announces `${position}: ${face_name}`, so
 * the two costumed rooms rhyme — and it also dodges a real trap: the shelf
 * holds "Quince" beside "Rose hips" and "Damson skins" beside "Angelica
 * stems", so any copy of the form *"the X goes in"* is ungrammatical for half
 * of it and no amount of care at the call site fixes that.
 *
 * `place` and `name` are both the server's words. The count and the colour
 * are ours.
 */
const STIRRED: ((name: string, place: string) => string)[] = [
  (name, place) =>
    `${place} takes the ${name}. One of three — the brew darkens and thickens.`,
  (name, place) =>
    `${place} takes the ${name}. Two of three — the brew turns from brown to a sickly olive.`,
  (name, place) =>
    `${place} takes the ${name}. Three of three — it comes up bright green and settles. Ready to pour.`,
]

/** Before anything has gone in. */
export const POT_IS_ON = 'The pot is on. Nothing in it yet but the boil.'

/** The pour, and what is on the bench afterwards. */
export const POURING = 'Pouring. She reads around while it cools.'
export const POURED =
  'Poured. Three of them on the bench — take the one you want to carry.'

/** What the pot says about the thing that has just gone in, or `''` for a
 *  count this room has no line for — which is a slot kind the pot was not
 *  built around rather than a failure worth a sentence. */
export function stirredLine(ingredient: BrewIngredient, count: number): string {
  const say = STIRRED[count - 1]
  if (!say) return ''
  return say(ingredient.name.toLowerCase(), ingredient.position)
}

/** What an empty place says, in the one place it says it. The socket prints
 *  this and [potLabel] repeats it, so the ear and the eye are told the same
 *  thing in the same words rather than "empty" and "still to come". */
export const STILL_TO_COME = 'still to come'

/** The pot's state in words, for the ear and for anything that cannot see the
 *  ring: "The Base: Rose hips. The Heat: still to come." */
export function potLabel(
  ingredients: BrewIngredient[], grounded: string[],
): string {
  return ingredients
    .map((i) =>
      `${i.position}: ${grounded.includes(i.slot) ? i.name : STILL_TO_COME}.`)
    .join(' ')
}

/** Long enough for the ladle, the dip and the vial to clear the frame. Here
 *  rather than in the component because the interview starts the pour and the
 *  room plays it, and a beat two files disagree about is a beat. */
export const SERVE_MS = 4400
