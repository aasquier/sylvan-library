/**
 * The room's clock.
 *
 * One property, and it is the one Aaron kept catching: a speed control has to
 * mean a fixed thing. The first version multiplied a *derived* pace — measured
 * from how long games were taking — which collapsed to a catch-up floor the
 * moment a match ended, so "Slow" meant fifty milliseconds a beat and every
 * setting was unwatchable.
 *
 * The room has no reason to chase the engine. The Forge plays flat out, and a
 * finished match carries every game with it, so falling behind is not falling
 * behind — it is being somewhere else in a recording that is not going
 * anywhere.
 */

import { describe, expect, it } from 'vitest'

import { beatDelay, seekReel } from './reel'

describe('how fast a beat is told', () => {
  it('is an absolute pace, not a multiplier on something derived', () => {
    // A game is about 130 beats, so these are roughly two and a half minutes,
    // one minute, and twenty seconds.
    expect(beatDelay('slow')).toBe(1200)
    expect(beatDelay('play')).toBe(480)
    expect(beatDelay('fast')).toBe(150)
  })

  it('orders the speeds the way their names promise', () => {
    expect(beatDelay('slow')).toBeGreaterThan(beatDelay('play'))
    expect(beatDelay('play')).toBeGreaterThan(beatDelay('fast'))
  })

  it('holds a beat long enough to look at, even at its fastest', () => {
    // The floor is a real limit rather than a round number: below about a
    // tenth of a second a beat is a flicker, the board's arrival animation
    // cannot finish, and nothing is being *watched* any more.
    expect(beatDelay('fast')).toBeGreaterThanOrEqual(100)
  })

  it('schedules nothing at all when paused', () => {
    expect(beatDelay('paused')).toBe(0)
  })
})

describe('moving the mark by hand', () => {
  const beat = (n: number) => ({
    key: `1:${n}`, game: 1, turn: 1, kind: 'land',
    who: 'gyome', text: `plays a land ${n}`,
  })
  const reel = {
    shown: [beat(1), beat(2)],
    queue: [beat(3), beat(4), beat(5)],
    game: 1, truncated: false, told: 2, board: null,
  }

  it('moves beats across the mark in both directions', () => {
    const forward = seekReel(reel, 4)
    expect(forward.told).toBe(4)
    expect(forward.shown).toHaveLength(4)
    expect(forward.queue).toHaveLength(1)

    // Backwards is the same operation, which is only true because the board is
    // a pure fold over the count — going back costs what going forward costs.
    const back = seekReel(reel, 1)
    expect(back.told).toBe(1)
    expect(back.shown).toHaveLength(1)
    expect(back.queue).toHaveLength(4)
  })

  it('clamps to the game rather than running off either end', () => {
    expect(seekReel(reel, 99).told).toBe(5)
    expect(seekReel(reel, -4).told).toBe(0)
    expect(seekReel(reel, -4).queue).toHaveLength(5)
  })

  it('keeps every beat, whichever side of the mark it is on', () => {
    for (const to of [0, 1, 3, 5]) {
      const at = seekReel(reel, to)
      expect(at.shown.length + at.queue.length).toBe(5)
    }
  })
})
