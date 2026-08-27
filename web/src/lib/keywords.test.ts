/**
 * Which keywords a board draws, and the two ways that list can be wrong.
 *
 * The map of drawings is typed against `DRAWN_KEYWORDS`, so "a word with no
 * picture" and "a picture for no word" are both typecheck failures rather than
 * anything a test could catch. What is left to hold here is the *matching* —
 * which is where the real fault lives, because the wire carries whatever
 * Scryfall spells and nothing guarantees that is the spelling this file used.
 */

import { describe, expect, it } from 'vitest'

import {
  DRAWN_KEYWORDS, KEYWORD_MEANS, drawableKeywords, keywordMeaning,
} from './keywords'

describe('the keywords a board can draw', () => {
  it('matches whatever case the wire happens to carry', () => {
    // **This is the fault worth testing.** Scryfall writes "First strike" and
    // "Flying"; rules text and a hand-written fixture write them lowercased.
    // A board that drew a sign for one spelling and nothing for the other
    // would be wrong only on some cards, which is the worst way to be wrong.
    expect(drawableKeywords(['Flying', 'First strike', 'Lifelink']))
      .toEqual(['flying', 'first strike', 'lifelink'])
    expect(drawableKeywords(['FLYING'])).toEqual(['flying'])
  })

  it('keeps a card in the order it lists them, so a corner is stable', () => {
    expect(drawableKeywords(['trample', 'flying']))
      .toEqual(['trample', 'flying'])
  })

  it('says nothing about a keyword it has no sign for', () => {
    // The wire sends every keyword a card has, unfiltered, precisely so that
    // adding a sign is one change in the browser. The cost is that most of
    // what arrives is not drawable, and silence is the right answer to it.
    expect(drawableKeywords(['Cycling', 'Kicker', 'Cascade'])).toEqual([])
    expect(drawableKeywords(['Kicker', 'Flying'])).toEqual(['flying'])
    expect(drawableKeywords([])).toEqual([])
  })

  it('draws a keyword once, however many times a card lists it', () => {
    expect(drawableKeywords(['Flying', 'flying', 'FLYING'])).toEqual(['flying'])
  })

  it('carries the six Aaron named', () => {
    // The originating ask, held literally: "first strike is a single sword,
    // double strike two swords, flying is a wing, lifelink a heart, vigilance
    // a castle, trample a stylized dinosaur footprint".
    for (const word of ['first strike', 'double strike', 'flying', 'lifelink',
      'vigilance', 'trample']) {
      expect(DRAWN_KEYWORDS, `${word} was asked for by name`).toContain(word)
    }
  })

  it('draws the two words that make combat damage mean something worse', () => {
    // Deathtouch and toxic can stand on one creature — `Bilious Skulldweller`
    // is the check case, and Scryfall lists its keywords as exactly
    // `['Toxic', 'Deathtouch']`. Both must survive the match, in the card's
    // own order, or a corner would say one of the two things a player most
    // needs to know before they block.
    expect(drawableKeywords(['Toxic', 'Deathtouch']))
      .toEqual(['toxic', 'deathtouch'])
    // Whether the skull and the viper are *distinguishable* at ten pixels is
    // Aaron's walk, not this suite's: jsdom has no layout and no rasteriser,
    // so nothing here can see a silhouette. The drawings argue themselves in
    // `components/keywords.tsx`.
  })

  it('reads toxic without its number, the way ward already does', () => {
    // "Toxic 1" and "Ward {2}" both carry an amount and Scryfall's `keywords`
    // array carries neither. A list entry spelled with a number would match
    // nothing at all, forever, and silently — the mark simply would not draw.
    expect(drawableKeywords(['Toxic'])).toEqual(['toxic'])
    expect(drawableKeywords(['Toxic 1'])).toEqual([])
    for (const word of DRAWN_KEYWORDS) {
      expect(word, `${word} carries a number`).not.toMatch(/\d/)
    }
  })

  it('spells every entry the way a lowercased Scryfall keyword is spelled', () => {
    // A word in the list that no lowercasing could ever produce is a sign that
    // will never be drawn — and nothing else would notice, because the map is
    // typed against this list rather than against reality.
    for (const word of DRAWN_KEYWORDS) {
      expect(word, `${word} would never match`).toBe(word.toLowerCase())
      expect(word.trim(), `${word} has stray space`).toBe(word)
    }
  })
})

/**
 * What a mark means, said in words.
 *
 * **Aaron's ask on the walk** (2026-08-27): a keyword a card was *given* is the
 * one a player cannot look up. The mark is thirteen pixels and the card face
 * behind it does not carry the word, so a newcomer sees a symbol appear on a
 * creature with nothing anywhere to explain it.
 *
 * The sentences themselves are prose and a suite cannot mark them right. What
 * it can hold is that there is one for every mark, that none of them smuggles
 * in a number the sign does not draw, and that the granted case says the thing
 * the card's own text would otherwise contradict.
 */
describe('what a keyword mark means', () => {
  it('has a sentence for every mark the board draws', () => {
    // The type already forces this and the type is the real gate. Held here as
    // well because a `Record` is satisfied by an empty string, and an empty
    // string renders as `vigilance — ` with nothing after the dash.
    for (const word of DRAWN_KEYWORDS) {
      expect(KEYWORD_MEANS[word]?.trim(), `${word} says nothing`)
        .toBeTruthy()
    }
  })

  it('says no numbers, the way the marks do not draw them', () => {
    // Ward and toxic both carry an amount, and the wire does not: Scryfall
    // spells them bare. A sentence promising "two poison counters" would be
    // the one place in this room quietly inventing a figure nobody sent.
    for (const word of DRAWN_KEYWORDS) {
      expect(KEYWORD_MEANS[word], `${word} quotes a number`)
        .not.toMatch(/\d/)
    }
  })

  it('tells a player the card is not wrong when the keyword was lent', () => {
    // The whole point. A Bronzehide Lion wearing vigilance has no vigilance
    // printed on it, and a player who checks will find the card and the mark
    // disagreeing — so the mark is the one that has to explain itself.
    const lent = keywordMeaning('vigilance', true)
    expect(lent).toContain('attacking does not tap it')
    expect(lent).toContain('not printed on this card')
    // And the ordinary case does not claim anything about where it came from.
    const printed = keywordMeaning('vigilance', false)
    expect(printed).toContain('attacking does not tap it')
    expect(printed).not.toContain('granted')
  })
})
