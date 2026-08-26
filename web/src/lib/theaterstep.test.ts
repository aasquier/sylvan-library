/**
 * Stepping the replay a turn at a time.
 *
 * Aaron, 2026-08-26: *"I want our replay ability to also be able to advance
 * things 1 at a time, or maybe a player's turn at a time, not a full two
 * player turn."* Beat-at-a-time already worked; this is the other unit, and
 * the arithmetic lives in `lib/theater.ts` rather than in the transport so
 * these can reach it.
 *
 * The three cases that decide whether the controls feel right are all edges:
 * back from the very first turn, forward from the last, and a press that lands
 * exactly on a boundary.
 */

import { describe, expect, it } from 'vitest'

import { stepToTurn, turnMarks } from './theater'

/** A bout: two beats of play after each turn line, three turns. */
const bout = [
  { kind: 'turn' },   // 0 -> mark 1
  { kind: 'play' },
  { kind: 'attack' },
  { kind: 'turn' },   // 3 -> mark 4
  { kind: 'play' },
  { kind: 'dies' },
  { kind: 'turn' },   // 6 -> mark 7
  { kind: 'play' },
  { kind: 'win' },
]
const of = bout.length

describe('where a turn begins', () => {
  it('marks the beat after each turn line, so the turn is announced', () => {
    // `seek` takes a told-count. Landing on the marker's own index would stop
    // the instant *before* the turn is announced, which reads as nothing
    // having happened.
    expect(turnMarks(bout)).toEqual([1, 4, 7])
  })

  it('finds no turns in a bout that has not raised one', () => {
    expect(turnMarks([{ kind: 'play' }, { kind: 'attack' }])).toEqual([])
  })
})

describe('stepping a turn', () => {
  const marks = turnMarks(bout)

  it('goes back to the start of the turn you are in', () => {
    // Mid-turn (beat 5 is inside the second turn) lands on that turn's own
    // start rather than skipping past it. The media convention, and it means
    // the first press always moves something.
    expect(stepToTurn(marks, 5, of, 'back')).toBe(4)
  })

  it('goes back to the previous turn when already on a boundary', () => {
    expect(stepToTurn(marks, 4, of, 'back')).toBe(1)
  })

  it('goes back to the opening rather than nowhere', () => {
    // Before the first turn there is nothing to reach but the start, and the
    // start is a real place to stand -- an empty board, nothing played.
    expect(stepToTurn(marks, 1, of, 'back')).toBe(0)
    expect(stepToTurn(marks, 0, of, 'back')).toBe(0)
  })

  it('goes on to the next turn', () => {
    expect(stepToTurn(marks, 0, of, 'on')).toBe(1)
    expect(stepToTurn(marks, 1, of, 'on')).toBe(4)
    expect(stepToTurn(marks, 5, of, 'on')).toBe(7)
  })

  it('stops at the end of the bout rather than running into the next', () => {
    // Past the last turn line there is no further turn in THIS bout. The room
    // queues bouts and tells each to its end, so a turn step that crossed the
    // boundary would skip a whole fight somebody queued up to watch.
    expect(stepToTurn(marks, 7, of, 'on')).toBe(of)
    expect(stepToTurn(marks, 8, of, 'on')).toBe(of)
  })

  it('still moves in a bout that has raised no turn at all', () => {
    // The transport hides the pair in this state, but the rule must be total:
    // a control that could appear for one frame must not compute a nonsense
    // destination if it does.
    expect(stepToTurn([], 3, 9, 'on')).toBe(9)
    expect(stepToTurn([], 3, 9, 'back')).toBe(0)
  })

  it('steps one player’s turn, never a round', () => {
    // Forge prints a turn line per seat and alternates them, so consecutive
    // marks are ONE player's turn apart. Nothing here halves anything --
    // halving is the mistake `turnsTaken` records this project making once
    // already, and it would make every step skip an opponent's whole turn.
    const alternating = [
      { kind: 'turn' }, { kind: 'play' },
      { kind: 'turn' }, { kind: 'play' },
      { kind: 'turn' }, { kind: 'play' },
      { kind: 'turn' }, { kind: 'play' },
    ]
    const each = turnMarks(alternating)
    expect(each).toEqual([1, 3, 5, 7])
    // Four turn lines are four steps, not two.
    let at = 0
    const stops: number[] = []
    for (let i = 0; i < 4; i++) {
      at = stepToTurn(each, at, alternating.length, 'on')
      stops.push(at)
    }
    expect(stops).toEqual([1, 3, 5, 7])
  })
})
