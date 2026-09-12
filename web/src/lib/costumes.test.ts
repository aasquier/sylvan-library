/**
 * What each room is wearing, and what a room nobody has dressed gets.
 *
 * The costume was a boolean and six ternaries in one component until there was
 * a second costumed room to build. Nothing here is about how the parchment
 * *looks* — jsdom computes no layout and commandment 14 is unambiguous that a
 * green suite has not seen the page. What it pins is the contract the record
 * replaced the boolean to get: a room with no costume falls all the way back
 * to the plain one, and a room with a costume never falls back by halves.
 */

import { describe, expect, it } from 'vitest'
import { PLAIN, costumeFor } from './costumes'

describe('the room’s costume', () => {
  it('hands a voice this build has never heard of the plain room', () => {
    // ADR 21's payoff, at this table: a reader added server-side arrives with
    // a key no client-side anything mentions, and must still render. The plain
    // room — not an empty one, and not a crash.
    expect(costumeFor('necromancer')).toBe(PLAIN)
    expect(costumeFor('')).toBe(PLAIN)
    expect(costumeFor('plain')).toBe(PLAIN)
  })

  it('leaves every undressed room in the plain chrome', () => {
    // The witch is the next room to be costumed and is deliberately not yet:
    // this branch generalises the mechanism and dresses nobody new.
    for (const key of ['witch', 'therapist', 'scientist', 'chef',
                       'storyteller', 'barkeep']) {
      expect(costumeFor(key)).toBe(PLAIN)
    }
  })

  it('dresses the fortune-teller in every place a costume can reach', () => {
    const seance = costumeFor('fortune-teller')
    // The empty string means "the plain chrome, whose paint stays at the call
    // site", so a half-dressed room is one that silently keeps a card surface
    // under a sheet of parchment. Every optional field is filled or none is.
    for (const dressed of [seance.question, seance.bubble, seance.note,
                           seance.quill]) {
      expect(dressed).not.toBe('')
    }
    expect(seance.scroll).toBe('seance-scroll')
    expect(seance.ink).toBe(true)
    expect(seance.mood).toBe('wisps')
  })

  it('gives every room a box for its question and a voice for the wait', () => {
    // The fields with no empty spelling: something has to carry the question
    // card, and a room that said nothing while it worked would read as broken.
    for (const key of ['plain', 'fortune-teller', 'witch', 'nobody']) {
      const costume = costumeFor(key)
      expect(costume.scroll).not.toBe('')
      expect(costume.placeholder).not.toBe('')
      expect(costume.thinking).not.toBe('')
      expect(costume.reading).not.toBe('')
      expect(costume.ready).not.toBe('')
      expect(costume.readyAction).not.toBe('')
      // The clock is appended by the caller, so the sentence has to end where
      // the seconds begin — "Reading around… 14s", never "Reading around…14s".
      expect(costume.reading.endsWith('…')).toBe(true)
    }
  })
})
