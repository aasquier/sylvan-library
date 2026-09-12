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
    for (const key of ['therapist', 'scientist', 'chef',
                       'storyteller', 'barkeep']) {
      expect(costumeFor(key)).toBe(PLAIN)
    }
  })

  it('dresses the witch in her own room, and not in the reader’s', () => {
    const hut = costumeFor('witch')
    expect(hut).not.toBe(PLAIN)
    for (const dressed of [hut.question, hut.bubble, hut.note, hut.quill]) {
      expect(dressed).not.toBe('')
    }
    expect(hut.scroll).toBe('cauldron-slate')
    // Her weather is what a fire does, not what a crystal ball does.
    expect(hut.mood).toBe('embers')
    // **She does not write.** `InkText` is the fortune-teller's wet-ink reveal
    // on parchment and it stays hers; the witch listens and puts things in a
    // pot, and her questions print. The two costumed rooms are meant to
    // contrast rather than to rhyme.
    expect(hut.ink).toBe(false)
    // The one action the room is for wears the room (commandment 17): a
    // `.btn-accent-2` on a witch's hut is the page's furniture standing in
    // somebody else's house.
    expect(hut.action).toBe('btn-copper-hot')
    // And technology is not in the building (commandment 10) — her register is
    // dry, warm and a little wicked, and none of it names a machine.
    for (const said of [hut.thinking, hut.reading, hut.ready, hut.readyAction,
                        hut.placeholder, hut.emptyReply]) {
      expect(said.toLowerCase()).not.toMatch(
        /server|model|api|token|seed|json|claude/)
    }
    // A turn that came back with nothing is the room's to explain, not the
    // app's: "Nothing usable came back." is a sentence about software.
    expect(hut.emptyReply).toBe('The steam took that one — say it again, plainer.')
    expect(hut.emptyReply).not.toBe(PLAIN.emptyReply)
  })

  it('lets an undressed room say nothing about its own button', () => {
    // `action` is the newest field and the one most likely to be filled in by
    // reflex. Empty means the app's accent button, whose paint stays at the
    // call site — the record's floor, and the reason a plain room never has to
    // restate the default here.
    expect(PLAIN.action).toBe('')
    expect(costumeFor('fortune-teller').action).toBe('')
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
      // A turn can always come back with nothing to ask, so every room needs
      // something to say about it. The plain room's is the app's own sentence
      // and that is the floor the field defaults to, never an empty string:
      // a blank here is a question card with nothing in it.
      expect(costume.emptyReply).not.toBe('')
      // The clock is appended by the caller, so the sentence has to end where
      // the seconds begin — "Reading around… 14s", never "Reading around…14s".
      expect(costume.reading.endsWith('…')).toBe(true)
    }
    expect(PLAIN.emptyReply).toBe('Nothing usable came back.')
  })
})
