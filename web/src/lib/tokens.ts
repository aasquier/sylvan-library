/**
 * Which tokens the board gives a *material* to.
 *
 * **The list lives here and the materials live in `index.css`**, on the gold
 * edge a token already wears — `.field-card-token.is-treasure` and its two
 * siblings. That is the whole reason this file is one function and no
 * pictures: the board draws Wizards' actual painting for every token, and the
 * only thing a browser adds is the light lying on it.
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
 *  name here must have a `.field-card-token.is-<name>` rule beside the gold
 *  edge in `index.css`. */
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
 * Always `field-card-token`, so every token keeps the thin gold edge it has
 * always had; a material is one more class on the same element. A token this
 * file has no opinion about comes back exactly as it went in, which is what
 * makes adding a material here unable to regress the others.
 */
export function tokenSigil(name: string, types?: string | null): string {
  const material = tokenMaterial(name, types)
  return material ? `field-card-token is-${material}` : 'field-card-token'
}
