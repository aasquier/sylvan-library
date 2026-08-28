import { describe, expect, it } from 'vitest'

import { ARCANA, type Manner, type Scene, sceneFor } from './stage'

/** Every scene the arena can open onto, so a new one cannot be added without
 *  this file deciding what reaches it. */
const SCENES: Scene[] = ['road', 'crypt', 'field', 'forge', 'temple',
  'veiling', 'tower', 'magician']

/** Forge's own spelling, which is what reaches this function on a real board:
 *  a hyphen rather than an em dash, and the subtypes still on the line. */
const TYPES = {
  creature: 'Legendary Creature - Cat Warrior',
  artifact: 'Artifact - Equipment',
  rock: 'Artifact',
  instant: 'Instant',
  sorcery: 'Sorcery',
  enchantment: 'Enchantment - Aura',
  land: 'Land',
  walker: 'Legendary Planeswalker - Teferi',
  adventure: 'Instant - Adventure',
}

describe('a departure is a departure whatever the card was', () => {
  it('sends every exile down the same road', () => {
    // Where a thing has gone is the whole of what these two beats are about,
    // and a Bolt and a Dragon leave by the same door.
    for (const types of Object.values(TYPES)) {
      expect(sceneFor('exiled', types)).toBe('road')
    }
    expect(sceneFor('exiled', undefined)).toBe('road')
  })

  it('sends a companion down it too', () => {
    expect(sceneFor('companion', TYPES.creature)).toBe('road')
  })

  it('takes every death to the same vault', () => {
    for (const types of Object.values(TYPES)) {
      expect(sceneFor('dies', types)).toBe('crypt')
    }
  })
})

describe('an arrival is about what arrived', () => {
  it('opens the valley for a creature however it got there', () => {
    // The whole of Aaron's ask on 2026-08-28. A creature nobody cast used to
    // get a battlefield and an identical creature somebody paid seven mana for
    // got a glow — the manner decided, and the manner is the wrong question.
    for (const manner of ['cast', 'put', 'made'] as Manner[]) {
      expect(sceneFor(manner, TYPES.creature), manner).toBe('field')
    }
  })

  it('opens the forge for an artifact however it got there', () => {
    for (const manner of ['cast', 'put', 'made'] as Manner[]) {
      expect(sceneFor(manner, TYPES.rock), manner).toBe('forge')
    }
    // A conjured Treasure is an artifact arriving, which is the same event.
    expect(sceneFor('made', 'Token Artifact - Treasure')).toBe('forge')
  })

  it('calls an artifact creature a creature', () => {
    // `castType`'s priority order, and it is the right answer: an Artifact
    // Creature is what a player calls a creature, it attacks, and it belongs
    // in the fight rather than on the anvil.
    expect(sceneFor('cast', 'Artifact Creature - Golem')).toBe('field')
  })
})

describe('a card that never lands is drawn from the arcana', () => {
  it('gives an instant the Tower and a sorcery the Magician', () => {
    // It happens to you, or you do it. Aaron's suggestion for the first, and
    // the pair is the rule: an event has no place to arrive at, so it gets a
    // shape rather than a somewhere.
    expect(sceneFor('cast', TYPES.instant)).toBe('tower')
    expect(sceneFor('cast', TYPES.sorcery)).toBe('magician')
  })

  it('judges an Adventure by the half being cast', () => {
    // The type line reaching `sceneFor` is `face_types[half]` when the card
    // prints two, so casting Bonecrusher Giant opens a valley and casting
    // Stomp draws the Tower — off one picture, on two different beats.
    expect(sceneFor('cast', TYPES.adventure)).toBe('tower')
    expect(sceneFor('cast', 'Creature - Giant')).toBe('field')
  })

  it('has a picture for every arcanum it can name, and no others', () => {
    // The one way this can go wrong silently: a scene named here with no file
    // behind it draws an empty box in the middle of the arena.
    for (const scene of SCENES) {
      const drawn = ARCANA[scene]
      if (scene === 'tower' || scene === 'magician') {
        expect(drawn, scene).toBeTruthy()
        expect(drawn, scene).toMatch(/^\/tarot\/\d\d-[a-z-]+\.webp$/)
      } else {
        expect(drawn, `${scene} is a place, not a card`).toBeUndefined()
      }
    }
  })
})

describe('what draws nothing, and means to', () => {
  it('leaves a land, a planeswalker and a sacrifice alone', () => {
    // A land arrives every turn, which is the strongest reason of the three:
    // a scene on every land drop would be the arena flashing at a person for
    // the most ordinary thing in Magic.
    expect(sceneFor('cast', TYPES.land)).toBeNull()
    expect(sceneFor('put', TYPES.land)).toBeNull()
    expect(sceneFor('cast', TYPES.walker)).toBeNull()
    expect(sceneFor('sacrificed', TYPES.rock)).toBeNull()
  })

  it('leaves an Equipment being strapped on alone', () => {
    // An Aura laid on a creature and an Equipment strapped to one are two
    // different events. The sword already existed and has been picked up —
    // opening the forge for it would say it was just made, which is not what
    // happened, and opening the rite would call a sword a blessing.
    expect(sceneFor('attach', TYPES.artifact)).toBeNull()
    expect(sceneFor('attach', 'Artifact - Equipment')).toBeNull()
  })

  it('draws nothing for a card nobody recorded a type line for', () => {
    // A third answer rather than a guess, which is this board's habit
    // everywhere else it is handed a hole.
    expect(sceneFor('cast', undefined)).toBeNull()
    expect(sceneFor('put', '')).toBeNull()
  })

})

describe('an enchantment and an Aura are two different events', () => {
  it('opens the standing place for an enchantment', () => {
    // The one permanent that is not a thing you could pick up: it is a
    // condition laid on the place, so it gets the place.
    expect(sceneFor('cast', 'Enchantment')).toBe('temple')
    expect(sceneFor('cast', 'Legendary Enchantment')).toBe('temple')
    expect(sceneFor('put', 'Enchantment - Shrine')).toBe('temple')
  })

  it('opens the rite for an Aura, whichever beat reports it', () => {
    // **`castType` cannot tell these apart and that is why the subtype is
    // read.** A Bear Umbra and a Rhystic Study are both *Enchantment* to it,
    // and one is laid on a creature while the other settles over the table.
    expect(sceneFor('cast', TYPES.enchantment)).toBe('veiling')
    expect(sceneFor('attach', TYPES.enchantment)).toBe('veiling')
    // Scryfall's em dash as well as Forge's hyphen: which one reaches a
    // browser is not something a board should be sensitive to.
    expect(sceneFor('cast', 'Enchantment — Aura')).toBe('veiling')
  })

  it('does not mistake a word that merely contains "aura"', () => {
    // The guard is a word boundary rather than a substring, because Magic has
    // a Minotaur, a Centaur and a Saurian, and every one of them would
    // otherwise be married.
    expect(sceneFor('cast', 'Creature - Minotaur')).toBe('field')
    expect(sceneFor('cast', 'Creature - Centaur Druid')).toBe('field')
    expect(sceneFor('cast', 'Enchantment Creature - Saurian')).toBe('field')
  })
})
