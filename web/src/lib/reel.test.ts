/**
 * The room's clock.
 *
 * One property, and it is the one that made the board usable: a game's beats
 * must drain inside the time the next game takes to arrive. A column of
 * sentences falling behind just scrolls; a *board* falling behind is reset to
 * an empty field every time the next game lands, so a ten-game match showed
 * turn two, ten times, and never a game.
 */

import { describe, expect, it } from 'vitest'

import { pace } from './reel'

describe('how fast a beat is told', () => {
  it('spreads a game across the time a game actually takes', () => {
    // Measured on the fixture decks: about five seconds a game, about a
    // hundred and thirty beats. That is ~38ms each, and the whole game
    // finishes inside the window.
    expect(pace(130, 5000)).toBeCloseTo(5000 / 130, 5)
    expect(130 * pace(130, 5000)).toBeLessThanOrEqual(5000)

    // A slow pairing gets a leisurely board rather than a faster one.
    expect(pace(130, 40_000)).toBeGreaterThan(pace(130, 5000))
  })

  it('holds a steady rate as the queue drains', () => {
    // **The bug this replaced.** The argument is the time *remaining*, not the
    // game's whole budget, so a queue half drained with half its time left
    // asks for the same rate it started at. Dividing the whole budget by the
    // shrinking queue made every beat slower than the last: 126 beats over 50
    // seconds started at 396ms and asked 793ms with sixty left — pinned at the
    // ceiling, and a fifty-second game took minutes.
    // 200 beats over 50 seconds is 250ms each; half the beats with half the
    // time left, and five beats with 1250ms left, are both the same rate.
    const started = pace(200, 50_000)
    const halfway = pace(100, 25_000)
    const nearlyDone = pace(5, 1250)
    expect(halfway).toBeCloseTo(started, 5)
    expect(nearlyDone).toBeCloseTo(started, 5)
  })

  it('hurries when it has fallen behind, rather than giving up', () => {
    // Time left can go negative when a game outran its window. The floor is
    // what stops that becoming an instant flush of two hundred beats.
    expect(pace(80, -4000)).toBe(20)
    expect(pace(80, 200)).toBe(20)
  })

  it('never goes below a flicker or above reading speed', () => {
    // A game with a thousand beats in one second is not something to watch
    // at one millisecond a beat: the board's arrival animation cannot even
    // finish, and nothing is being *watched* any more.
    expect(pace(1000, 500)).toBe(20)
    // And a two-beat game over five minutes still reads at reading speed
    // rather than pausing for two and a half minutes on a land drop.
    expect(pace(2, 300_000)).toBe(420)
  })

  it('falls back to the backlog curve until a game has been timed', () => {
    // The first gap holds the JVM's boot and the card database — fifteen
    // seconds of no game at all — so there is no budget until a second game
    // has landed. Until then, deep queues hurry and shallow ones do not.
    expect(pace(130, null)).toBe(45)
    expect(pace(50, null)).toBe(90)
    expect(pace(20, null)).toBe(200)
    expect(pace(3, null)).toBe(420)
  })

  it('does not divide by an empty queue', () => {
    expect(pace(0, 5000)).toBe(420)
  })
})
