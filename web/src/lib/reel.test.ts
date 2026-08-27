/**
 * The room's clock, and its running order.
 *
 * Two properties, and Aaron caught both of them from the far side of a screen.
 *
 * **A speed control has to mean a fixed thing.** The first version multiplied a
 * *derived* pace — measured from how long games were taking — which collapsed
 * to a catch-up floor the moment a match ended, so "Slow" meant fifty
 * milliseconds a beat and every setting was unwatchable. The room has no reason
 * to chase the engine: the Forge plays flat out, and a finished match carries
 * every game with it, so falling behind is not falling behind — it is being
 * somewhere else in a recording that is not going anywhere.
 *
 * **And a series has to be told in order.** The second bout lands while the
 * first is still being told, and it used to replace it mid-sentence. These
 * tests drive that mechanism rather than its miss path: a bout is *told to its
 * end* while the next one waits, and the walk forward happens after a breath
 * rather than on the last beat.
 */

import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  BETWEEN_BOUTS, beatDelay, countRuns, useReel, type Arriving, type Speed,
  type StagedBeat,
} from './reel'

describe('how fast a beat is told', () => {
  it('is an absolute pace, not a multiplier on something derived', () => {
    // A game is about 130 beats, so these are roughly six and a half minutes,
    // one minute, and twenty seconds.
    expect(beatDelay('study')).toBe(3000)
    expect(beatDelay('play')).toBe(480)
    expect(beatDelay('fast')).toBe(150)
  })

  it('orders the speeds the way their names promise', () => {
    expect(beatDelay('study')).toBeGreaterThan(beatDelay('play'))
    expect(beatDelay('play')).toBeGreaterThan(beatDelay('fast'))
  })

  it('holds a studied beat long enough to actually read it', () => {
    // The property, rather than the number: Aaron asked for this twice —
    // "it moves quicker than the mind can keep up with", then "not slow enough,
    // lets slow it down by another 20-30%". A beat a newcomer is reading for
    // the first time — working out what "blocks Gyome with Ancient Tomb" even
    // means — needs seconds, not a moment. Below about two and a half this
    // stops being a study pace and goes back to being a fast one.
    expect(beatDelay('study')).toBeGreaterThanOrEqual(2500)
    // And it is a different intention from Watch rather than a nudge off it.
    expect(beatDelay('study')).toBeGreaterThanOrEqual(beatDelay('play') * 4)
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

/**
 * Moving the mark by hand.
 *
 * These drive the hook rather than a pure helper beside it, which is the
 * correction: the helper took a reel and returned a reel, and a test could
 * satisfy it while the control on screen was wired to nothing. The mark *is*
 * the reel's only state now, so setting it is the whole mechanism.
 */
describe('moving the mark by hand', () => {
  /** One bout, five beats long. */
  const bout = (game: number): Arriving => ({
    game, truncated: false, board: null,
    beats: Array.from({ length: 5 }, (_, i) => ({
      key: `${game}:${i}`, game, turn: 1, kind: 'land',
      who: 'gyome', text: `plays a land ${i}`,
    })),
  })

  const held = () => renderHook(
    () => useReel('j1', [1], bout, 'paused'))

  it('moves the mark across the beats in both directions', () => {
    const { result } = held()
    act(() => { result.current[1](4) })
    expect(result.current[0].told).toBe(4)
    expect(result.current[0].shown).toHaveLength(4)
    expect(result.current[0].queue).toHaveLength(1)

    // Backwards is the same operation, which is only true because the board is
    // a pure fold over the count — going back costs what going forward costs.
    act(() => { result.current[1](1) })
    expect(result.current[0].told).toBe(1)
    expect(result.current[0].shown).toHaveLength(1)
    expect(result.current[0].queue).toHaveLength(4)
  })

  it('clamps to the bout rather than running off either end', () => {
    const { result } = held()
    act(() => { result.current[1](99) })
    expect(result.current[0].told).toBe(5)
    act(() => { result.current[1](-4) })
    expect(result.current[0].told).toBe(0)
    expect(result.current[0].queue).toHaveLength(5)
  })

  it('keeps every beat, whichever side of the mark it is on', () => {
    const { result } = held()
    for (const to of [0, 1, 3, 5]) {
      act(() => { result.current[1](to) })
      const at = result.current[0]
      expect(at.shown.length + at.queue.length).toBe(5)
    }
  })

  it('keeps the beats with the match they were raised in', () => {
    const { result } = held()
    act(() => { result.current[1](4) })
    expect(result.current[0].match).toBe('j1')
  })
})

/**
 * The running order.
 *
 * `stage` is a resolver rather than a bag of beats, and that is the shape the
 * memory argument rests on: however far behind the room falls, only the bout
 * being told is ever held as staged beats. These tests hand it a match whose
 * bouts are three beats each and drive the clock by hand.
 */
describe('telling a series in order', () => {
  const BEAT = 150      // the `fast` pace, which is what these run at

  /** One bout, three beats long. */
  const bout = (game: number): Arriving => ({
    game,
    truncated: false,
    board: null,
    beats: Array.from({ length: 3 }, (_, i) => ({
      key: `${game}:${i}`, game, turn: 1, kind: 'land',
      who: 'gyome', text: `plays a land ${i}`,
    })),
  })

  /** Watch a match. `games` and `speed` are re-handed on every rerender, and
   *  `stage` is deliberately a fresh closure each time — a resolver rebuilt
   *  when a bout lands must not restart the bout being told. */
  function watch(games: number[], speed: Speed = 'fast', match = 'j1') {
    return renderHook(
      (p: { games: number[]; speed: Speed; match: string }) =>
        useReel(p.match, p.games, bout, p.speed),
      { initialProps: { games, speed, match } })
  }

  /** Let `n` beats be told. One tick each, because the next timeout is only
   *  scheduled once the effect has seen the last one land. */
  function told(n: number) {
    for (let i = 0; i < n; i++) act(() => { vi.advanceTimersByTime(BEAT) })
  }

  const breath = () => act(() => { vi.advanceTimersByTime(BETWEEN_BOUTS) })

  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('opens on the first bout without being asked', () => {
    const { result } = watch([1])
    expect(result.current[0].game).toBe(1)
    expect(result.current[0].queue).toHaveLength(3)
  })

  it('does not let an arriving bout clip the one being told', () => {
    const { result, rerender } = watch([1])
    told(1)
    expect(result.current[0].told).toBe(1)

    // Game two lands while game one is a third of the way through. This is
    // the exact moment that used to replace the reel wholesale, and the two
    // thirds of game one nobody ever saw.
    rerender({ games: [1, 2], speed: 'fast', match: 'j1' })
    expect(result.current[0].game, 'the bout being told was clipped').toBe(1)
    expect(result.current[0].told, 'the bout being told was restarted').toBe(1)

    // And it is told to its end.
    told(2)
    expect(result.current[0].told).toBe(3)
    expect(result.current[0].queue).toHaveLength(0)
  })

  it('walks on to the next bout after a breath, and not before', () => {
    const { result, rerender } = watch([1])
    rerender({ games: [1, 2], speed: 'fast', match: 'j1' })
    told(3)
    // Drained, but the outcome of the bout is still on screen: advancing here
    // would put the next opening hand up before anybody read how this ended.
    expect(result.current[0].game).toBe(1)

    breath()
    expect(result.current[0].game).toBe(2)
    expect(result.current[0].told, 'a new bout starts from its first beat')
      .toBe(0)
  })

  it('says how many bouts are waiting behind this one', () => {
    const { result } = watch([1, 2, 3])
    expect(result.current[2].waiting).toBe(2)
  })

  it('counts only the bouts still ahead, never the ones already told', () => {
    const { result } = watch([1, 2, 3])
    act(() => { result.current[2].pick(3) })
    // Two bouts sit behind this one and neither is waiting for anybody.
    expect(result.current[0].game).toBe(3)
    expect(result.current[2].waiting).toBe(0)
  })

  it('lets somebody skip ahead to the newest bout rather than be held', () => {
    // The trap this exists to prevent: game one being retold at a study pace
    // while game four is being fought, and no way out of it.
    const { result } = watch([1, 2, 3, 4])
    expect(result.current[0].game).toBe(1)
    act(() => { result.current[2].pick(4) })
    expect(result.current[0].game).toBe(4)
    expect(result.current[0].told).toBe(0)
  })

  it('does not wander on to the next fight while paused', () => {
    const { result, rerender } = watch([1, 2])
    told(3)
    rerender({ games: [1, 2], speed: 'paused', match: 'j1' })
    breath()
    breath()
    expect(result.current[0].game, 'a stopped room walked on by itself')
      .toBe(1)
  })

  it('does not show one match the last match’s board', () => {
    const { result, rerender } = watch([1, 2, 3])
    told(2)
    expect(result.current[0].game).toBe(1)

    // A second match, which has not raised a bout yet. The room is not
    // remounted between them, so without the match travelling *in* the reel
    // this is where the previous match's field would still be on screen.
    rerender({ games: [], speed: 'fast', match: 'j2' })
    expect(result.current[0].game).toBe(0)
    expect(result.current[0].shown).toHaveLength(0)
    expect(result.current[0].board).toBeNull()
  })

  it('does not carry a pick from one match into the next', () => {
    const { result, rerender } = watch([1, 2, 3])
    act(() => { result.current[2].pick(3) })
    expect(result.current[0].game).toBe(3)

    // The new match opens on its own first bout. A bare remembered number
    // would have sent the room hunting for a third bout this match may never
    // raise, and it would have sat empty until it did.
    rerender({ games: [1], speed: 'fast', match: 'j2' })
    expect(result.current[0].game).toBe(1)
  })
})

/**
 * Four tokens made at once is one thing that happened.
 *
 * **And the wire has no word for it.** Forge announces a token by moving a
 * card into a zone, one card at a time with one id each, so a trigger that
 * makes three Clue Tokens reaches the browser as three beats saying exactly
 * the same sentence in a row. There is no `amount` anywhere on that path, and
 * a number invented in a browser would be the room claiming a fact nothing
 * told it. So it is counted, and these hold the counting.
 */
describe('counting what arrives twice', () => {
  const beat = (over: Partial<StagedBeat> & { key: string }): StagedBeat => ({
    game: 1, turn: 3, kind: 'enters', who: 'Gyome', text: 'makes a Clue Token',
    card: 'Clue Token', ...over,
  })

  it('folds a run into its first beat and empties the rest', () => {
    const run = countRuns([
      beat({ key: 'a' }), beat({ key: 'b' }), beat({ key: 'c' }),
      beat({ key: 'd', card: 'Food Token', text: 'makes a Food Token' }),
    ])
    // The first carries the count; the followers carry nothing, so the stage
    // leaves the card that is already up alone instead of replaying it.
    expect(run.map((b) => b.run)).toEqual([3, 0, 0, 1])
  })

  it('keeps every beat, because the board still steps for each of them', () => {
    // **Nothing is removed and nothing is moved.** The board is folded to the
    // count of beats told and the server builds one step per beat, so dropping
    // a beat here would take a token off the sand. This only says which beat
    // of a run is the one worth drawing in the middle of the arena.
    const beats = [beat({ key: 'a' }), beat({ key: 'b' }), beat({ key: 'c' })]
    const run = countRuns(beats)
    expect(run).toHaveLength(3)
    expect(run.map((b) => b.key)).toEqual(['a', 'b', 'c'])
  })

  it('only folds beats that are next to each other', () => {
    // Two tokens made on turn three and two more on turn seven are four
    // tokens in two moments, and anything between them parts them — which is
    // what makes this arithmetic on the account rather than a guess about the
    // game.
    const run = countRuns([
      beat({ key: 'a' }), beat({ key: 'b' }),
      beat({ key: 'x', kind: 'cast', card: 'Sol Ring', text: 'casts Sol Ring' }),
      beat({ key: 'c' }), beat({ key: 'd' }), beat({ key: 'e' }),
    ])
    expect(run.map((b) => b.run)).toEqual([2, 0, 1, 3, 0, 0])
  })

  it('reads the whole sentence, so two different beats never merge', () => {
    // The identity is the kind, the player and the words — and `text` already
    // carries everything the beat said, numbers included. Two damages of
    // different sizes are two sentences, and the same event from two seats is
    // two events.
    const run = countRuns([
      beat({ key: 'a', kind: 'damage', card: 'Ball Lightning', who: null,
        text: 'deals 6 to Gyome' }),
      beat({ key: 'b', kind: 'damage', card: 'Ball Lightning', who: null,
        text: 'deals 3 to Gyome' }),
      beat({ key: 'c', who: 'Atla' }),
      beat({ key: 'd', who: 'Gyome' }),
    ])
    expect(run.map((b) => b.run)).toEqual([1, 1, 1, 1])
  })

  it('never folds a beat that names no card', () => {
    // **Time Warp is a real card.** A player taking two turns in a row is two
    // turns, not one turn twice, and a turn beat names no card to tell them
    // apart by — so a beat with nothing in its hand is folded with nothing.
    const turn = (key: string): StagedBeat => ({
      key, game: 1, turn: 3, kind: 'turn', who: 'Gyome', text: 'takes the turn',
    })
    expect(countRuns([turn('a'), turn('b'), turn('c')]).map((b) => b.run))
      .toEqual([1, 1, 1])
  })

  it('counts the run inside the reel, where the running order lives', () => {
    // Through the hook, because a pure helper is only a claim until something
    // calls it. Whether four identical beats are four moments or one moment
    // told once is a question about how the room tells a bout, and this hook
    // is what tells a bout.
    const bout = (game: number): Arriving => ({
      game, truncated: false, board: null,
      beats: [beat({ key: 'a' }), beat({ key: 'b' }), beat({ key: 'c' })],
    })
    const { result } = renderHook(() => useReel('j9', [1], bout, 'paused'))
    act(() => { result.current[1](3) })
    expect(result.current[0].shown.map((b) => b.run)).toEqual([3, 0, 0])
  })
})
