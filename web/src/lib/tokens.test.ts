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
  it('is the gold edge and nothing else, for every token alike', () => {
    // **The material no longer rides the card**, so no token gets a second
    // class here — see `tokenSigil`. A Treasure and a Spirit wear the same
    // edge, and what a Treasure is made of is asked one beat later, by the
    // mark.
    expect(tokenSigil()).toBe('field-card-token')
  })

  it('leaves no stylesheet rule selecting a material on a card', () => {
    // The static objects were removed; a rule left behind selecting one would
    // put a goblet back on the sand and typecheck perfectly on the way.
    for (const material of TOKEN_MATERIALS) {
      expect(CSS, `.field-card-token.is-${material} still has a rule`)
        .not.toContain(`.field-card-token.is-${material}`)
    }
  })
})

/**
 * **The mark each material is drawn as.**
 *
 * Every name on `TOKEN_MATERIALS` has to reach a picture. The chain is
 * `tokenMaterial` -> `markOf` -> a `.field-mark-<verb>` rule, and only the
 * first link can be typechecked: the verb is a string in one file and a
 * selector in another, and a material whose rule was never written animates
 * nothing at all on a real sacrifice. jsdom has no layout, so no rendering
 * test can see it either — the only witness is a screenshot of a beat that
 * happens about five times in six games.
 *
 * So the stylesheet is opened and the three halves held together here: the
 * verb has a rule, the rule draws a committed object, and the rule times that
 * object from its own `--mark-life-*` rather than a hard-coded duration —
 * which is what keeps a mark honest when somebody changes the pace.
 */
const MARK_VERBS: Record<string, string> = {
  treasure: 'spent', food: 'eaten', clue: 'cracked',
}

describe('the mark a material is sacrificed as', () => {
  it('names a verb for every material on the list', () => {
    // If a fourth material is added and this map is not, the tests below
    // cannot even ask the question — so it is asked here first.
    for (const material of TOKEN_MATERIALS) {
      expect(MARK_VERBS[material], `no verb for ${material}`).toBeDefined()
    }
  })

  it.each(TOKEN_MATERIALS)('draws %s a committed object', (material) => {
    const verb = MARK_VERBS[material]
    const block = ruleFor(`.field-mark-${verb} img`, 'animation')
    expect(block, `no .field-mark-${verb} img rule`).not.toBe('')
    // Sized by width, never by height: `.field-mark` is a grid sized from its
    // own item, so a percentage height is cyclic. Argued at
    // `.field-mark-attacks img`, and this is the guard on it.
    expect(declaration(block, 'width')).toMatch(/%$/)
  })

  it.each(TOKEN_MATERIALS)('times %s from its own mark life', (material) => {
    const verb = MARK_VERBS[material]
    const block = ruleFor(`.field-mark-${verb} img`, 'animation')
    // A hard-coded duration here is a mark that ignores the transport: the
    // room hands the length down as a custom property precisely so that Fast
    // does not leave an object sitting on a board thirteen beats past its own.
    expect(declaration(block, 'animation'))
      .toContain(`var(--mark-life-${verb}`)
  })

  it('gives every material an object that is a committed webp', () => {
    // The three pictures are museum plates cut and committed under ADR 29.
    // They are imported by `components/board.tsx` rather than reached from CSS
    // now, so what is checked here is that the files are still the ones the
    // recipes describe — a hotlink or a gradient standing in for a thing would
    // be commandment 5 going quietly.
    for (const art of ['aurum', 'ferculum', 'lens']) {
      expect(nodeFs.existsSync(`src/assets/coliseum/${art}.webp`),
        `${art}.webp is missing`).toBe(true)
      expect(nodeFs.existsSync(`src/assets/coliseum/${art}.recipe.yaml`),
        `${art} has no recipe beside it`).toBe(true)
    }
  })
})
