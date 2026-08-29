/**
 * The deck's tokens, as a list somebody can take to a shop.
 *
 * Aaron, 2026-08-29: *"I want there to be an 'Export' button in the token area
 * that can give you a TCGPlayer friendly list to help shop for tokens when
 * needed."* `components/tokens.tsx` owns the press and the words around it;
 * this owns the list itself, which is the half with rules in it.
 *
 * **The list is deliberately *not* the shelf written out.** The shelf beside
 * it is a row of printings — Arahbo's deck makes four different Cats, from
 * four different sets, and shows four plates because they are four different
 * paintings. A shop has one product called Cat Token. So the two disagree on
 * purpose, and every rule below is one of the ways they disagree.
 */
import type { TokenPlate } from './api'

/**
 * The most of any one token this list will ever ask somebody to buy.
 *
 * **The number the whole feature turns on.** Gyome's food deck has twenty
 * cards in it that make a Food, and a list that said `20 Food Token` would be
 * both true and useless: those twenty are made across a whole game and eaten
 * as they go, so the table never holds twenty at once. Four is a playset — the
 * one quantity every Magic player already reads without being taught it — and
 * it errs small on purpose. A second order costs a week; a pile of cardboard
 * nobody puts down costs money, and an app that charges nobody a penny should
 * not talk anybody into spending one either.
 *
 * The cap is stated to the reader in words rather than applied behind their
 * back, so somebody who knows their deck leans on Treasures can type a bigger
 * number in themselves before they paste.
 */
export const MOST_OF_ONE_TOKEN = 4

/** One line of the shopping list: how many, and the name they are sold under. */
export interface ShoppingLine {
  qty: number
  name: string
}

/**
 * The name a shop's catalogue keeps this token under.
 *
 * **Shops file a token as "Beast Token", never as "Beast"** — checked against
 * real listings rather than assumed, across every shape the pool actually
 * produces: Beast Token, Knight Token, Cat Token, Cat Soldier Token, Treasure
 * Token, Copy Token, and named ones like Sacred Cat Token. Scryfall's name is
 * the bare word, so this suffix is the whole translation between the two —
 * and without it a line would miss, or land on a real card wearing the word.
 *
 * The guard is for a name that already ends in it, so nothing ever comes out
 * as "Something Token Token".
 */
function shopName(name: string): string {
  const trimmed = name.trim()
  return /\btokens?$/i.test(trimmed) ? trimmed : `${trimmed} Token`
}

/**
 * The deck's tokens as a shopping list.
 *
 * **Merged by the name a shop sells them under, and that is not cosmetic.**
 * Four lines saying `1 Cat Token` arrive in a basket as four separate entries
 * of the same thing; one line saying `3 Cat Token` is the same order,
 * correctly stated.
 *
 * **The quantity is how many of the deck's own cards make it**, deduplicated
 * across the merge (one card that makes two different Spirits is still one
 * card), floored at one and capped at [MOST_OF_ONE_TOKEN]. It is the signal
 * the shelf itself already trusts: the server sorts the plates by exactly this
 * number, on the argument that the token a deck makes most is *"the one
 * somebody has to find a pile of before they sit down"*.
 *
 * **The shelf's order is kept**, rather than re-sorted by quantity. The list
 * renders directly above the plates it was made from, and a reader checking
 * one against the other should not have to hunt.
 */
export function shoppingList(tokens: TokenPlate[]): ShoppingLine[] {
  const order: string[] = []
  const makers = new Map<string, Set<string>>()
  for (const token of tokens) {
    const name = shopName(token.name)
    let mine = makers.get(name)
    if (!mine) {
      mine = new Set<string>()
      makers.set(name, mine)
      order.push(name)
    }
    for (const maker of token.made_by) mine.add(maker)
  }
  return order.map((name) => ({
    name,
    // The `?? 0` cannot happen — every name in `order` was put there beside
    // its own set — and the `max` is what makes it harmless rather than a
    // quantity of nought if it ever did. A token the server sent with no
    // makers at all still earns its line.
    qty: Math.min(Math.max(makers.get(name)?.size ?? 0, 1), MOST_OF_ONE_TOKEN),
  }))
}

/**
 * The list as the text that goes on the clipboard.
 *
 * **`<quantity> <name>`, one per line, and nothing else** — no set code, no
 * collector number, no set name. A bulk-add box takes all three
 * (`1 Lightning Bolt [SLD] 84` is its own documented example), and for a
 * *token* every one of them would make the list worse:
 *
 *   - The set code we hold is Scryfall's token set — `TELD`, `T2XM`, `MPR` —
 *     and shops do not file tokens in a set of their own at all. Their Beast
 *     Token sits under Wilds of Eldraine, not under a "Wilds of Eldraine
 *     Tokens". Every annotated line would be a line that misses.
 *   - The printing we hold was chosen to be *recognisable*, not to be in
 *     stock: the shelf deliberately draws a token's earliest painting, so a
 *     Food is always Eldraine's pie. Pinning a shopper to a 2005 Elephant
 *     because that is the picture we drew is the tail wagging the dog.
 *   - Nothing about a token is printing-sensitive. It is not in the deck, it
 *     has no legality, and any Beast Token is a Beast token. What a shopper
 *     wants is whichever is cheapest and to hand — which is exactly what a
 *     bare name asks for.
 *
 * The trailing newline is `MoxfieldText`'s, for its reason: the file this
 * imitates ends in one, and a box that trims whitespace does not mind. An
 * empty list gets no newline either, so that nothing is ever a blank line
 * somebody has to wonder about — the shelf does not offer this list at all
 * when there is nothing to buy, and if it ever did, empty should read as
 * empty.
 */
export function shoppingText(lines: ShoppingLine[]): string {
  if (lines.length === 0) return ''
  return lines.map((line) => `${String(line.qty)} ${line.name}`).join('\n') + '\n'
}
