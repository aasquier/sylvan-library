/**
 * Which tokens get a material, and the one way that answer goes wrong.
 *
 * **The fault worth testing is the spelling.** The board's card dictionary is
 * keyed by *Forge's* name, and Forge writes "Treasure Token" where Scryfall
 * writes "Treasure". A matcher written against the Scryfall spelling typechecks
 * perfectly, passes any test that hands it a tidy name, and then lights up
 * nothing at all on a real board — which is exactly the shape of bug that
 * reaches production, because the only witness is a screenshot.
 *
 * The second fault is the opposite one: a material leaking onto a token that
 * should not have it. Creature tokens outnumber artifact tokens on most
 * boards, and a Spirit that glinted like gold would be worse than a Spirit
 * that did nothing.
 */

import { describe, expect, it } from 'vitest'

import { TOKEN_MATERIALS, tokenMaterial, tokenSigil } from './tokens'

/**
 * The stylesheet, opened rather than imported.
 *
 * `import '../index.css?raw'` resolves to the **empty string** under vitest,
 * and so does `?inline` — both measured here, not assumed — because Vite's
 * CSS plugin answers the request before either loader sees it. A guard
 * written the obvious way therefore reads nothing, finds nothing wrong, and
 * passes forever: a check that cannot fail, which is the precise thing this
 * file exists to keep out of the CSS below.
 *
 * `src` deliberately carries no node types (`tsconfig.app.json` lists only
 * `vite/client`), so a component cannot reach the filesystem by accident.
 * Widening that for every file in order to open one file in one test is the
 * wrong trade, so the suppression on the next line is the whole of the escape
 * hatch — and `new URL` instead of `path.resolve` is what keeps it to one.
 * If a future config does bring node's types in, this directive goes unused
 * and the checker says so; the fix then is to delete the comment.
 */
// @ts-expect-error -- node's types are out of scope for `src`; argued above.
const nodeFs = await import('node:fs')
// **Relative to the working directory, and neither `import.meta.url` nor a
// `URL` will do.** Both were tried. `readFileSync` refuses a jsdom `URL` with
// "The URL must be of scheme file" — a true sentence about the wrong thing,
// since jsdom installs its own `URL` class and node's check is for node's —
// and resolving `'../index.css'` against `import.meta.url` through that same
// jsdom `URL` yields `/src/index.css`, a path at the root of the disk. The
// suite's working directory is `web/` by construction: the only supported way
// to run it is `npm --prefix web run test`, and vitest started anywhere else
// loads no config at all. If that ever stops being true this throws ENOENT
// and names the file, which is the loud failure a silent one was traded for.
const CSS: string = nodeFs.readFileSync('src/index.css', 'utf8')

/**
 * The body of the first rule that lists `sel` **and** declares `needs`.
 *
 * Both halves matter. Selector lists are grouped here — the three materials
 * share one `overflow: hidden` rule — so a plain search for
 * `.field-card-token.is-clue` finds the group and returns a block with no
 * object in it, which is a test that reports a missing picture that is
 * actually there. And the same selector reappears inside
 * `@media (prefers-reduced-motion)`, so "first match" alone is not enough
 * either. Naming the property that has to be present picks the one rule the
 * question is about.
 */
function ruleFor(sel: string, needs: string): string {
  const css = CSS.replace(/\/\*[\s\S]*?\*\//g, '')
  for (const m of css.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selectors = (m[1] ?? '').split(',').map((s: string) => s.trim())
    const body = m[2] ?? ''
    if (selectors.includes(sel) && declaration(body, needs) !== '') return body
  }
  return ''
}

function declaration(block: string, prop: string): string {
  const m = new RegExp(`(?:^|;|\\n)\\s*${prop}\\s*:([^;}]*)`, 'i').exec(block)
  return (m?.[1] ?? '').replace(/\s+/g, ' ').trim()
}

/** The first background/mask layer of a comma-separated list. */
function firstLayer(block: string, prop: string): string {
  return (declaration(block, prop).split(',')[0] ?? '').trim()
}

/** The file a `url('...')` names, or `''` if there is none. */
function assetIn(value: string): string {
  return /url\('([^']+)'\)/.exec(value)?.[1] ?? ''
}

describe('the material a token is made of', () => {
  it('reads Forge\'s spelling, which is the one that actually arrives', () => {
    // **The bug this file exists for.** `go/internal/api/forge.go` puts token
    // art back under Forge's name, so "Treasure Token" is what a browser sees.
    expect(tokenMaterial('Treasure Token')).toBe('treasure')
    expect(tokenMaterial('Food Token')).toBe('food')
    expect(tokenMaterial('Clue Token')).toBe('clue')
  })

  it('reads the Scryfall spelling too, so neither end has to promise', () => {
    expect(tokenMaterial('Treasure')).toBe('treasure')
    expect(tokenMaterial('food')).toBe('food')
  })

  it('prefers the type line, because that is the rules answer', () => {
    // Forge's own format, ASCII hyphen and all. A Food under some other name
    // is still a Food, and this is the half of the test that proves the name
    // is the fallback rather than the whole mechanism.
    expect(tokenMaterial('Gingerbrute', 'Artifact Creature - Food'))
      .toBe('food')
    expect(tokenMaterial('Whatever', 'Artifact - Treasure')).toBe('treasure')
  })

  it('has no opinion about a creature token', () => {
    // The common ones, measured: Spirit 103 printings, Soldier 96, Zombie 96.
    // All of them carry a loupe and keyword marks already.
    for (const name of ['Spirit Token', 'Soldier Token', 'Zombie Token',
      'Beast Token', 'Dragon Token']) {
      expect(tokenMaterial(name, 'Creature - Spirit')).toBeNull()
    }
  })

  it('does not match a card that merely starts with a material word', () => {
    // "Treasure Map" is a real card. Whole-name matching is what keeps it out;
    // a `startsWith` would not.
    expect(tokenMaterial('Treasure Map', 'Artifact')).toBeNull()
    expect(tokenMaterial('Food Chain', 'Enchantment')).toBeNull()
  })

  it('survives an absent type line, which the wire marks omitempty', () => {
    expect(tokenMaterial('Clue Token', undefined)).toBe('clue')
    expect(tokenMaterial('Clue Token', null)).toBe('clue')
    expect(tokenMaterial('Clue Token', '')).toBe('clue')
  })
})

describe('the edge a token wears', () => {
  it('always keeps the gold edge, material or not', () => {
    // A material is one more class on the element that already draws the
    // edge — never a replacement for it. This is what makes adding a fourth
    // material unable to regress the tokens that have none.
    expect(tokenSigil('Spirit Token')).toBe('field-card-token')
    expect(tokenSigil('Treasure Token'))
      .toBe('field-card-token is-treasure')
  })

  it('names a class for every material on the list', () => {
    // The CSS half cannot be typechecked, so this is the closest thing to a
    // gate on "somebody added a name and forgot to draw it": at minimum the
    // class has to be well-formed and distinct.
    const classes = TOKEN_MATERIALS.map((m) => tokenSigil(m))
    expect(new Set(classes).size).toBe(TOKEN_MATERIALS.length)
    for (const c of classes) expect(c).toMatch(/^field-card-token is-\w+$/)
  })
})

/**
 * **The object and the light that is clipped to it.**
 *
 * Each material stands a committed cutout on the card and then masks its
 * ambient layer to that cutout's own alpha, so what brightens is the gold of
 * the cup rather than the card behind it. That only works while four values
 * agree across two rules: the same file, the same size, the same position, in
 * the element's `background-*` and in the `::before`'s `mask-*`.
 *
 * Nothing else can notice when they stop agreeing. jsdom has no layout, so
 * the suite cannot see it; a browser does not error, it just draws the light
 * beside the thing — a highlight sliding through empty space next to a
 * goblet, which is exactly the kind of fault that ships because it looks like
 * a rendering quirk rather than a bug. So the numbers are held equal here,
 * where a diff has to change both or fail.
 */
describe('the object a material stands on the card', () => {
  it.each(TOKEN_MATERIALS)('gives %s an object and a shadow', (material) => {
    const block = ruleFor(`.field-card-token.is-${material}`, 'background-image')
    expect(block, `no rule for is-${material}`).not.toBe('')
    const image = declaration(block, 'background-image')
    // A committed cutout, not a hotlink and not a gradient standing in for a
    // thing — commandment 5, and ADR 29 for where it may come from.
    expect(image).toMatch(/url\('\.\/assets\/coliseum\/[a-z]+\.webp'\)/)
    // Two layers: the object, and the contact shadow under its foot. One
    // layer means somebody deleted the shadow and the object is a sticker.
    expect(declaration(block, 'background-size').split(',')).toHaveLength(2)
    expect(declaration(block, 'background-position').split(',')).toHaveLength(2)
  })

  it.each(TOKEN_MATERIALS)('clips %s\'s light to that same object', (material) => {
    const object = ruleFor(`.field-card-token.is-${material}`, 'background-image')
    const light = ruleFor(`.field-card-token.is-${material}::before`, 'mask-image')
    expect(light, `no ::before for is-${material}`).not.toBe('')

    const asset = assetIn(declaration(object, 'background-image'))
    expect(asset, 'the object has no picture').not.toBe('')
    expect(assetIn(declaration(light, 'mask-image')),
      'the mask is a different picture').toBe(asset)

    // The object is always the *first* background layer; the shadow is second.
    const size = firstLayer(object, 'background-size')
    const at = firstLayer(object, 'background-position')
    expect(declaration(light, 'mask-size')).toBe(size)
    expect(declaration(light, 'mask-position')).toBe(at)

    // Safari still wants the prefix, and a mask that only one engine applies
    // is a light that lands on the whole card in the other one.
    expect(declaration(light, '-webkit-mask-image')).toBe(
      declaration(light, 'mask-image'))
    expect(declaration(light, '-webkit-mask-size')).toBe(size)
    expect(declaration(light, '-webkit-mask-position')).toBe(at)
  })

  it('never moves a masked layer by transform', () => {
    // A `transform` takes the mask along with it, so the light would travel
    // with its own silhouette and never arrive on the object. The sweep is
    // drawn by moving `background-position` for exactly this reason, and this
    // is the note that survives the next person who reaches for translateX.
    for (const material of TOKEN_MATERIALS) {
      const light = ruleFor(`.field-card-token.is-${material}::before`, 'mask-image')
      expect(declaration(light, 'transform'),
        `is-${material}::before transforms a masked layer`).toBe('')
    }
  })
})
