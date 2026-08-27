/**
 * Which tokens the board gives a *material* to.
 *
 * **The material is a moment, not a coat of paint**, and that is the one thing
 * to know before changing anything here. Each of the three used to stand a
 * museum plate — a goblet, a dish, a magnifying glass — on the card for as
 * long as the token was on the battlefield, with a slow light walking across
 * it. Aaron, 2026-08-27: *"why do I still see them overlayed on the card
 * statically? They should only appear as the animation when they are being
 * sacrificed. Like how the shield or sword appear"*. He is right twice over: a
 * Treasure sitting on the sand is not doing anything, so an object announcing
 * it every moment is decoration over Wizards' own painting; and spending the
 * object's whole presence on the idle state left the beat that actually
 * *matters* — the token being cracked — with almost nothing left to say.
 *
 * So the three objects are **marks** now, raised by the sacrifice beat and
 * drawn by `components/board.tsx` beside the sword, the shield and the skull.
 * What this file decides is unchanged and is asked one beat later:
 * `tokenMaterial` answers *what a token is made of*, and `markOf` turns that
 * answer into which picture falls across the card.
 *
 * A fourth material is still two edits — a name below, and a
 * `.field-mark-<verb>` rule beside the other marks — plus the arm of `markOf`
 * that names the verb.
 *
 * The board draws Wizards' own painting untouched underneath, and now for
 * rather longer: nothing filters, blurs or recolours the art at any point,
 * which is the line ADR 32 draws.
 *
 * **Three, because three is what the pool says is common.** Counting printings
 * that carry art, in the pool this app already ships against: Treasure 98,
 * Food 63, Clue 51 — and then a cliff, to Blood at 9, Gold at 8, Powerstone
 * and Map at 2 apiece. Aaron named Treasure, Food and Clue and asked what else
 * was ubiquitous; the honest measured answer is *nothing else is*, and a
 * fourth material would be a fourth thing to maintain for a token most people
 * will never see. Adding one later is a name in `TOKEN_MATERIALS` and a block
 * of CSS, in that order — the type below makes skipping the second half a
 * typecheck failure.
 *
 * **Creature tokens are deliberately absent.** A Spirit, a Soldier and a
 * Beast share nothing to animate except being tokens, which the gold edge
 * already says; and they are the tokens that need saying least, because a
 * creature carries a power/toughness loupe, keyword marks and a turn when it
 * taps. The artifact tokens are the ones with no numbers on them, and they are
 * the ones a person new to the game cannot tell apart at fifty-eight pixels.
 */

/** In no particular order; this array is a spelling, not a priority. Every
 *  name here must reach a mark: `markOf` in `components/board.tsx` turns it
 *  into a verb, and that verb must have a `.field-mark-<verb>` rule that draws
 *  a committed object and times it from its own `--mark-life-*`.
 *  `tokens.test.ts` reads the stylesheet off disk and holds those halves
 *  together, because a material with no rule behind it is a sacrifice that
 *  animates nothing and nothing whatever says so. */
export const TOKEN_MATERIALS = ['treasure', 'food', 'clue'] as const

export type TokenMaterial = (typeof TOKEN_MATERIALS)[number]

const MATERIALS = new Set<string>(TOKEN_MATERIALS)

/**
 * What a token is made of, or `null` for one this board has no material for.
 *
 * **The type line is asked first and the name second**, because the type line
 * is the rules answer and the name is a label. Forge spells a type line
 * `"Artifact - Food"` — an ASCII hyphen with spaces, subtypes after it — so a
 * Food that arrives under some other name is still a Food, which is the case
 * the name test would miss.
 *
 * **The name test strips Forge's suffix.** Forge says "Food Token" where
 * Scryfall says "Food", and this is the browser-side twin of `pool.TokenName`
 * in `go/internal/pool/tokens.go`, which strips the same suffix for the same
 * reason on the way to the art. The dictionary that reaches this file is keyed
 * by *Forge's* spelling (`go/internal/api/forge.go`, where the art is put back
 * under it), so a match against the bare Scryfall name would never fire.
 */
export function tokenMaterial(
  name: string,
  types?: string | null,
): TokenMaterial | null {
  const cut = types ? types.toLowerCase().indexOf(' - ') : -1
  if (cut >= 0 && types) {
    for (const sub of types.slice(cut + 3).toLowerCase().split(/\s+/)) {
      if (MATERIALS.has(sub)) return sub as TokenMaterial
    }
  }
  const bare = name.toLowerCase().replace(/ token$/, '').trim()
  return MATERIALS.has(bare) ? (bare as TokenMaterial) : null
}

/**
 * The class list for the edge a token wears.
 *
 * **One class, for every token, and it takes no arguments any more.** It used
 * to add `is-treasure` and its two siblings so that the stylesheet could stand
 * an object on the card; with the objects moved to the marks there is nothing
 * left for a per-material class to do, and a class that selects nothing is the
 * kind of thing that survives three refactors and then gets a rule written
 * against it by mistake.
 *
 * Kept as a function rather than folded into a string literal at the call site
 * because the gold edge is a *decision about tokens* — one place says which
 * class a token wears, and `tokens.test.ts` holds it.
 */
export function tokenSigil(): string {
  return 'field-card-token'
}
