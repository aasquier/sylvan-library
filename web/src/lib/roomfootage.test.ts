/**
 * The footage table is keyed by hand, like the paintings, and the one way it
 * fails is silently: a key that is not a persona's is a room that never
 * wears its loop and looks merely plain. These pin the table's shape against
 * the roster the paintings already answer for.
 */
import { describe, expect, it } from 'vitest'
import { personaAccent } from './personart'
import { roomFootage } from './roomfootage'

const FOOTAGE_ROOMS = ['barkeep', 'storyteller'] as const

describe('roomFootage', () => {
  it('answers a complete family for every room that has footage', () => {
    for (const key of FOOTAGE_ROOMS) {
      const f = roomFootage(key)
      expect(f, key).toBeDefined()
      // Four distinct files: the two encodes, the still, the bed.
      const urls = [f!.webm, f!.mp4, f!.still, f!.tone]
      for (const u of urls) expect(u, key).toMatch(/\S/)
      expect(new Set(urls).size).toBe(4)
      expect(f!.webm).toMatch(/\.webm$/)
      expect(f!.mp4).toMatch(/\.mp4$/)
      expect(f!.still).toMatch(/\.webp$/)
      expect(f!.tone).toMatch(/\.m4a$/)
    }
  })

  it('is keyed by personas the rest of the app already knows', () => {
    // Every footage key has an accent of its own, which is the cheapest proof
    // it is a real persona key rather than a typo that compiles.
    for (const key of FOOTAGE_ROOMS) {
      expect(personaAccent(key)).not.toBe(personaAccent('necromancer'))
    }
  })

  it('answers nothing for a room with no footage, including a voice the server adds tomorrow', () => {
    expect(roomFootage('plain')).toBeUndefined()
    expect(roomFootage('witch')).toBeUndefined()
    expect(roomFootage('necromancer')).toBeUndefined()
  })
})
