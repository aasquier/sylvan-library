/**
 * The addresses of the 32, checked against the real table.
 *
 * **These cases read the served JSON rather than a fixture**, and that is the
 * whole point of them. The slug scheme is generated from names, keys and
 * aliases — there is deliberately no table of 32 slugs in the source — so the
 * only question worth asking is whether that generation produces 32 distinct,
 * stable, unambiguous addresses *for the data that actually ships*. A fixture
 * of three rows can only prove the code runs.
 *
 * The import is the same `data/colors.json` the Go binary embeds, so a session
 * that adds an alias colliding with somebody else's page fails here rather
 * than shipping a bookmark that opens the wrong colour.
 */

import { describe, expect, it } from 'vitest'
import type { Combination } from './api'
import { comboSlug, resolveSlug, slugForKey, slugify, slugsFor } from './colors'

// The one authority, read rather than copied: the very file the server
// embeds and serves. `?raw` and a parse rather than a JSON import, because
// this project's TypeScript does not enable `resolveJsonModule` and a test
// is the wrong reason to turn a compiler option on for the whole app.
import tableJSON from '../../../go/internal/reference/data/colors.json?raw'

const COMBINATIONS = (
  JSON.parse(tableJSON) as { combinations: Combination[] }
).combinations

describe('the slug scheme, over the table that ships', () => {
  it('has all thirty-two of them', () => {
    expect(COMBINATIONS).toHaveLength(32)
  })

  it('gives every combination an address, and no two the same', () => {
    const slugs = COMBINATIONS.map(comboSlug)
    expect(slugs.every((s) => s.length > 0)).toBe(true)
    expect(new Set(slugs).size).toBe(32)
  })

  it('names the twenty-five that are places, and letters the seven that are not',
    () => {
      const at = (key: string) =>
        comboSlug(COMBINATIONS.find((c) => c.key === key)!)
      // A guild, a shard, a clan and a colour: all called what Magic calls
      // them, and the colour without the "mono" that belongs to the deck
      // rather than to the colour.
      expect(at('BG')).toBe('golgari')
      expect(at('WUG')).toBe('bant')
      expect(at('WBG')).toBe('abzan')
      expect(at('W')).toBe('white')
      // The four-colour sets are arithmetic and their names are product
      // labels two sources disagree about, so the URL picks neither.
      expect(at('WUBR')).toBe('wubr')
      expect(at('WUBRG')).toBe('wubrg')
      // ...and colourless takes a word, because `c` reads as a typo.
      expect(at('C')).toBe('colourless')
    })

  it('answers to a colour key, so every ?c= link that was shared still lands',
    () => {
      for (const combo of COMBINATIONS) {
        expect(slugForKey(COMBINATIONS, combo.key)).toBe(comboSlug(combo))
        const found = resolveSlug(COMBINATIONS, combo.key.toLowerCase())
        expect(found?.combo.key).toBe(combo.key)
      }
    })

  it('answers to its name, its aliases and the American spelling', () => {
    const asked = (s: string) => resolveSlug(COMBINATIONS, s)?.combo.key
    expect(asked('mono-white')).toBe('W')
    expect(asked('artifice')).toBe('WUBR')
    expect(asked('yore-tiller')).toBe('WUBR')
    expect(asked('witch-maw')).toBe('WUBG')
    expect(asked('colorless')).toBe('C')
    expect(asked('five-color')).toBe('WUBRG')
    // Case and stray punctuation, because a link pasted out of a chat window
    // arrives however it arrives.
    expect(asked('GOLGARI')).toBe('BG')
  })

  it('never lets one spelling mean two combinations', () => {
    const seen = new Map<string, string>()
    const clashes: string[] = []
    for (const combo of COMBINATIONS) {
      for (const slug of slugsFor(combo)) {
        const already = seen.get(slug)
        if (already && already !== combo.key) {
          clashes.push(`${slug}: ${already} and ${combo.key}`)
        }
        seen.set(slug, combo.key)
      }
    }
    expect(clashes).toEqual([])
  })

  it('says which of a combination’s spellings is the one it lives at', () => {
    const canonical = resolveSlug(COMBINATIONS, 'golgari')
    expect(canonical?.canonical).toBe(true)
    const other = resolveSlug(COMBINATIONS, 'bg')
    expect(other?.canonical).toBe(false)
    expect(other?.slug).toBe('golgari')
  })

  it('resolves nothing for a segment that names nothing', () => {
    expect(resolveSlug(COMBINATIONS, 'grixis-but-cooler')).toBeNull()
    expect(resolveSlug(COMBINATIONS, '')).toBeNull()
    // `X` is not a colour, and the server's own canonicaliser would fold it to
    // colourless. A page address must not: `/colors/x` naming Colourless would
    // be a typo silently answered with the wrong room.
    expect(resolveSlug(COMBINATIONS, 'x')).toBeNull()
  })
})

describe('slugify', () => {
  it('makes a URL segment out of anything', () => {
    expect(slugify('Ravnica: City of Guilds')).toBe('ravnica-city-of-guilds')
    expect(slugify("Assassin's Trophy")).toBe('assassin-s-trophy')
    expect(slugify('  --Bant--  ')).toBe('bant')
  })
})
