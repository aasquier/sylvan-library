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

import { DRAWN_KEYWORDS, drawableKeywords } from './keywords'

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
