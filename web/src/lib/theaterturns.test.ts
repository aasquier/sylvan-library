/**
 * Forge's turn number against the one a person says out loud.
 *
 * Measured on a real match (2026-08-25): Forge increments once per
 * player-turn and alternates, so its "turn 15" was the first player's eighth.
 * Every game length this app reported was about double what anybody counting
 * at a table would say. These pin the translation, and the one case where
 * halving is wrong and counting is right.
 */

import { describe, expect, it } from 'vitest'

import type { ForgeBeat } from './api'
import { playerTurns, turnsTaken } from './theater'

/** A turn beat, as the wire carries it: Forge's global number and who took it. */
function turn(n: number, who: string): ForgeBeat {
  return { kind: 'turn', turn: n, who, against: null }
}

describe('the turn a player would name', () => {
  it('counts each seat’s own turns, not Forge’s running total', () => {
    // The measured shape: strict alternation, fifteen player-turns.
    const beats: ForgeBeat[] = []
    for (let n = 1; n <= 15; n++) beats.push(turn(n, n % 2 ? 'gyome' : 'atla'))
    const turns = playerTurns(beats)

    // Gyome takes Forge's 1, 3, 5 … 15 — which are Gyome's 1st through 8th.
    expect(turns.get(1)).toBe(1)
    expect(turns.get(3)).toBe(2)
    expect(turns.get(15)).toBe(8)
    // And Atla's 2, 4 … 14 are Atla's 1st through 7th.
    expect(turns.get(2)).toBe(1)
    expect(turns.get(14)).toBe(7)
  })

  it('survives an extra turn, which halving does not', () => {
    // Time Warp: seat one takes Forge's 3 and 4 back to back. Halving would
    // call turn 4 "round 2" and credit the opponent with a turn they never
    // took; counting gives seat one their third turn, which is what happened.
    const beats = [
      turn(1, 'gyome'), turn(2, 'atla'),
      turn(3, 'gyome'), turn(4, 'gyome'),
      turn(5, 'atla'),
    ]
    const turns = playerTurns(beats)
    expect(turns.get(3)).toBe(2)
    expect(turns.get(4)).toBe(3)
    // The opponent is still only on their second.
    expect(turns.get(5)).toBe(2)
  })

  it('leaves a turn nobody is credited with as Forge numbered it', () => {
    const turns = playerTurns([{ kind: 'turn', turn: 4, who: null, against: null }])
    expect(turns.get(4)).toBe(1)
  })

  it('ignores everything that is not a turn', () => {
    const turns = playerTurns([
      turn(1, 'gyome'),
      { kind: 'land', turn: 1, who: 'gyome', against: null, card: 'Forest' },
      { kind: 'cast', turn: 1, who: 'gyome', against: null, card: 'Llanowar Elves' },
      turn(2, 'atla'),
    ])
    expect(turns.size).toBe(2)
    expect(turns.get(1)).toBe(1)
    expect(turns.get(2)).toBe(1)
  })
})

describe('how long a finished game took', () => {
  it('halves a heads-up game, because a row carries no seats', () => {
    // The measured game: Forge said 15, the players took 8 and 7.
    expect(turnsTaken(15)).toBe(8)
    expect(turnsTaken(14)).toBe(7)
    expect(turnsTaken(1)).toBe(1)
  })

  it('divides a pod by its seats', () => {
    // The CLI plays four-player pods, where turn 15 is round four.
    expect(turnsTaken(15, 4)).toBe(4)
    expect(turnsTaken(16, 4)).toBe(4)
  })

  it('passes a missing count through rather than inventing a zero', () => {
    // A clocked-out game has no turn count at all, and "0 turns" would be a
    // claim about a game that certainly took some.
    expect(turnsTaken(null)).toBeNull()
  })
})
