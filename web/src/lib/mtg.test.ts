/** Display helpers with real logic in them.
 *
 * `identityName` is the interesting one: colour-pair and triple names are not
 * derivable, they are a lookup table, and the lookup has to be
 * order-independent because a colour identity arrives as an arbitrary array.
 * It handles that with a rotation trick, which is exactly the kind of code
 * that works for the cases someone tried and quietly fails for the rest.
 */

import { describe, expect, it } from 'vitest'
import { identityName, manaSymbols, money, percent } from './mtg'

describe('identityName', () => {
  it('names the colourless and mono cases', () => {
    expect(identityName([])).toBe('Colorless')
    expect(identityName(['G'])).toBe('Mono-Green')
    expect(identityName(['U'])).toBe('Mono-Blue')
  })

  it('names all ten guilds', () => {
    expect(identityName(['W', 'U'])).toBe('Azorius')
    expect(identityName(['U', 'B'])).toBe('Dimir')
    expect(identityName(['B', 'R'])).toBe('Rakdos')
    expect(identityName(['R', 'G'])).toBe('Gruul')
    expect(identityName(['G', 'W'])).toBe('Selesnya')
    expect(identityName(['W', 'B'])).toBe('Orzhov')
    expect(identityName(['U', 'R'])).toBe('Izzet')
    expect(identityName(['B', 'G'])).toBe('Golgari')
    expect(identityName(['R', 'W'])).toBe('Boros')
    expect(identityName(['G', 'U'])).toBe('Simic')
  })

  it('names the shards and the wedges', () => {
    expect(identityName(['W', 'U', 'B'])).toBe('Esper')
    expect(identityName(['U', 'B', 'R'])).toBe('Grixis')
    expect(identityName(['B', 'R', 'G'])).toBe('Jund')
    expect(identityName(['R', 'G', 'W'])).toBe('Naya')
    expect(identityName(['G', 'W', 'U'])).toBe('Bant')
    expect(identityName(['W', 'B', 'G'])).toBe('Abzan')
    expect(identityName(['U', 'R', 'W'])).toBe('Jeskai')
    expect(identityName(['B', 'G', 'U'])).toBe('Sultai')
    expect(identityName(['R', 'W', 'B'])).toBe('Mardu')
    expect(identityName(['G', 'U', 'R'])).toBe('Temur')
  })

  it('names five colours', () => {
    expect(identityName(['W', 'U', 'B', 'R', 'G'])).toBe('Five-colour')
  })

  it('does not depend on the order the colours arrive in', () => {
    // A colour identity is a set; the API happens to sort it, but nothing in
    // the type says so.
    expect(identityName(['W', 'G'])).toBe('Selesnya')
    expect(identityName(['G', 'W'])).toBe('Selesnya')
    expect(identityName(['B', 'W', 'G'])).toBe('Abzan')
    expect(identityName(['G', 'B', 'W'])).toBe('Abzan')
    expect(identityName(['G', 'R', 'B'])).toBe('Jund')
  })

  it('covers the six curated decks', () => {
    expect(identityName(['G', 'W'])).toBe('Selesnya')        // Arahbo, Trostani
    expect(identityName(['R', 'G', 'W'])).toBe('Naya')       // Atla Palani
    expect(identityName(['G'])).toBe('Mono-Green')           // Goreclaw
    expect(identityName(['W', 'U', 'B'])).toBe('Esper')      // Tivit
    expect(identityName(['B', 'G'])).toBe('Golgari')         // Gyome
  })

  it('falls back to the colour letters for four-colour identities', () => {
    // Documenting a real gap rather than asserting it is right: the four-colour
    // names (Yore-Tiller, Glint-Eye, Dune-Brood, Ink-Treader, Witch-Maw) are
    // not in the table, so a four-colour deck would render as "WUBR" in the
    // library. None of the six curated decks is four-colour, so this is latent.
    expect(identityName(['W', 'U', 'B', 'R'])).toBe('WUBR')
    expect(identityName(['U', 'B', 'R', 'G'])).toBe('UBRG')
  })
})

describe('manaSymbols', () => {
  it('splits a cost into its symbols', () => {
    expect(manaSymbols('{2}{B}{G}')).toEqual(['2', 'B', 'G'])
    expect(manaSymbols('{X}{G}{G}')).toEqual(['X', 'G', 'G'])
  })

  it('keeps compound symbols whole', () => {
    // The pip renderer needs "U/P" as one symbol, not as "U", "/" and "P".
    expect(manaSymbols('{3}{U/P}')).toEqual(['3', 'U/P'])
    expect(manaSymbols('{G/W}{2/B}')).toEqual(['G/W', '2/B'])
  })

  it('treats a missing cost as no symbols', () => {
    // Lands have no mana cost; so does any card the corpus does not know.
    expect(manaSymbols(null)).toEqual([])
    expect(manaSymbols(undefined)).toEqual([])
    expect(manaSymbols('')).toEqual([])
  })
})

describe('percent', () => {
  it('formats a fraction with one decimal by default', () => {
    expect(percent(0.572)).toBe('57.2%')
    expect(percent(0)).toBe('0.0%')
    expect(percent(1)).toBe('100.0%')
  })

  it('honours an explicit digit count', () => {
    expect(percent(0.572, 0)).toBe('57%')
    expect(percent(0.5725, 2)).toBe('57.25%')
  })
})

describe('money', () => {
  it('renders an em dash when there is no price', () => {
    // A card the corpus has no printing price for must not render as "$0.00".
    expect(money(null)).toBe('—')
    expect(money(undefined)).toBe('—')
  })

  it('renders two decimal places either side of a dollar', () => {
    expect(money(0.35)).toBe('$0.35')
    expect(money(12.5)).toBe('$12.50')
    expect(money(0)).toBe('$0.00')
    expect(money(1)).toBe('$1.00')
  })
})
