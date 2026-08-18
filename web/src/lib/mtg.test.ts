/** Display helpers with real logic in them.
 *
 * `identityName` is the interesting one: colour-pair and triple names are not
 * derivable, they are a lookup table, and the lookup has to be
 * order-independent because a colour identity arrives as an arbitrary array.
 * It handles that with a rotation trick, which is exactly the kind of code
 * that works for the cases someone tried and quietly fails for the rest.
 */

import { describe, expect, it } from 'vitest'
import { identityName, manaSymbols, money, percent, splitManaText, symbolName } from './mtg'

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

  it('names four-colour identities with the taxonomy\'s canonical names', () => {
    // These must match src/mtglab/colors.py, where the Scryfall/C16 names are
    // canonical and the Nephilim names are aliases -- the Start-a-deck grid
    // renders the taxonomy's name, so a divergent row here would show one
    // deck under two names in the same session. GET /api/colors is the same
    // table over the wire.
    expect(identityName(['W', 'U', 'B', 'R'])).toBe('Artifice')
    expect(identityName(['U', 'B', 'R', 'G'])).toBe('Chaos')
    expect(identityName(['W', 'B', 'R', 'G'])).toBe('Aggression')
    expect(identityName(['W', 'U', 'R', 'G'])).toBe('Altruism')
    expect(identityName(['W', 'U', 'B', 'G'])).toBe('Growth')
    // Order-independent like every other row in the table.
    expect(identityName(['G', 'R', 'B', 'U'])).toBe('Chaos')
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
    // Lands have no mana cost; so does any card the pool does not know.
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
    // A card the pool has no printing price for must not render as "$0.00".
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


describe('splitManaText', () => {
  /** Just the pips, for asserting on what got converted. */
  const pips = (s: string) => splitManaText(s).filter((p) => p.pip).map((p) => p.text)
  /** Round-trip: the visible characters, with pips as bare symbols. */
  const flat = (s: string) => splitManaText(s).map((p) => p.text).join('')

  it('converts the symbols that appear in the deck files', () => {
    expect(pips('A {G} activation for {1}{G}')).toEqual(['G', '1', 'G'])
    expect(pips('{T}: Add {C}{C}.')).toEqual(['T', 'C', 'C'])
  })

  it('expands a colour identity written as one brace', () => {
    // `validate.py` writes an identity as {GW} -- one brace, two colours,
    // because that is how Magic writes an identity. Two pips, not one blob.
    expect(pips('identity {GW} includes {W}, outside {G}')).toEqual(['G', 'W', 'W', 'G'])
  })

  it('keeps a multi-digit generic cost as a single pip', () => {
    expect(pips('{10} to cast')).toEqual(['10'])
  })

  it('keeps hybrid and Phyrexian symbols whole', () => {
    expect(pips('{G/W} or {U/P}')).toEqual(['G/W', 'U/P'])
    expect(pips('{2/W} is a twobrid')).toEqual(['2/W'])
  })

  it('leaves prose that merely contains braces alone', () => {
    // Turning an arbitrary brace into a pip would be the UI asserting it had
    // read something it did not.
    expect(pips('set the {note} key and {} nothing else')).toEqual([])
    expect(flat('set the {note} key')).toBe('set the {note} key')
  })

  it('loses no text around the symbols', () => {
    const prose = 'Pay {2}{G}{G}, then {T} it. Identity {BG}.'
    expect(flat(prose)).toBe('Pay 2GG, then T it. Identity BG.')
    expect(splitManaText(prose).filter((p) => !p.pip).map((p) => p.text))
      .toEqual(['Pay ', ', then ', ' it. Identity ', '.'])
  })

  it('handles a string that is only a symbol, and one with none', () => {
    expect(splitManaText('{G}')).toEqual([{ text: 'G', pip: true }])
    expect(splitManaText('no mana here')).toEqual([{ text: 'no mana here', pip: false }])
    expect(splitManaText('')).toEqual([])
  })

  it('normalises case, since prose is written by hand', () => {
    expect(pips('costs {g}{w}')).toEqual(['G', 'W'])
  })
})

describe('symbolName', () => {
  // Since ADR 33 every pip is a drawing, so the name is the only way any
  // symbol reads aloud — including the ones that used to be their own text.
  it('names the whole vocabulary the prose regex recognises', () => {
    expect(symbolName('G')).toBe('Green')
    expect(symbolName('C')).toBe('Colorless')
    expect(symbolName('T')).toBe('Tap')
    expect(symbolName('Q')).toBe('Untap')
    expect(symbolName('S')).toBe('Snow')
    expect(symbolName('E')).toBe('Energy')
    expect(symbolName('X')).toBe('X')
    expect(symbolName('3')).toBe('Generic 3')
    expect(symbolName('15')).toBe('Generic 15')
  })

  it('spells out hybrids the way a player would', () => {
    expect(symbolName('W/U')).toBe('White or Blue')
    expect(symbolName('2/W')).toBe('Two or White')
    expect(symbolName('G/P')).toBe('Phyrexian Green')
  })

  it('falls back to the symbol as written rather than inventing a name', () => {
    expect(symbolName('CHAOS')).toBe('CHAOS')
  })
})
