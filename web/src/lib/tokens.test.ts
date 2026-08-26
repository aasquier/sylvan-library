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
