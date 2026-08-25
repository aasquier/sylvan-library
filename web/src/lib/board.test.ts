/**
 * Folding a board out of deltas.
 *
 * The rulings about the *game* are all in Go and tested there against a real
 * recorded match. What is left for this file to hold is the arithmetic: that
 * applying `n` steps gives the board at beat `n`, that a card lands on the
 * right side of the table, and that the two states the server can put a card
 * in which this file must not draw — `gone`, and a zone a newer server
 * invented — take it off the table rather than throwing.
 */

import { describe, expect, it } from 'vitest'

import type { ForgeBoard } from './api'
import { fightingStats, foldBoard } from './board'

/** A two-seat board with whatever steps a test needs. */
function board(steps: ForgeBoard['steps']): ForgeBoard {
  return {
    seats: [
      { seat: 1, slug: 'gyome', name: 'Gyome, Master Chef — Food', life: 40 },
      { seat: 2, slug: 'atla', name: 'Atla Palani — Eggs', life: 40 },
    ],
    cards: [
      { id: 10, name: 'Gyome, Master Chef', types: 'Legendary Creature - Troll Warlock', seat: 1 },
      { id: 11, name: 'Forest', types: 'Basic Land - Forest', seat: 1 },
      { id: 12, name: 'Food Token', token: true, types: 'Artifact - Food', seat: 1 },
      { id: 20, name: 'Dragonlord Atarka', types: 'Legendary Creature - Elder Dragon', seat: 2 },
    ],
    steps,
  }
}

describe('the board at a moment', () => {
  it('is folded to exactly the number of beats that have been told', () => {
    const b = board([
      { turn: 1, seat: 1, changes: [{ id: 11, zone: 'land', seat: 1 }] },
      { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { turn: 2, seat: 2, changes: [{ id: 20, zone: 'battlefield', seat: 2 }] },
    ])

    // Nothing told yet is an empty field with both players still at forty.
    const nothing = foldBoard(b, 0)
    expect(nothing.sides[0]?.land).toHaveLength(0)
    expect(nothing.sides[0]?.life).toBe(40)

    const one = foldBoard(b, 1)
    expect(one.sides[0]?.land.map((c) => c.name)).toEqual(['Forest'])
    expect(one.sides[0]?.battlefield).toHaveLength(0)

    const all = foldBoard(b, 3)
    expect(all.turn).toBe(2)
    expect(all.active).toBe(2)
    expect(all.sides[0]?.battlefield.map((c) => c.name))
      .toEqual(['Gyome, Master Chef'])
    expect(all.sides[1]?.battlefield.map((c) => c.name))
      .toEqual(['Dragonlord Atarka'])
  })

  it('never runs past the steps it was given', () => {
    const b = board([{ turn: 1, changes: [{ id: 11, zone: 'land', seat: 1 }] }])
    // A count beyond the end is what a room does when a game's beats are
    // drained faster than the next game arrives; it is not an error.
    expect(foldBoard(b, 99).sides[0]?.land).toHaveLength(1)
    expect(foldBoard(b, -3).sides[0]?.land).toHaveLength(0)
  })

  it('takes a card off the table when it is gone', () => {
    // `gone` is the server saying a card has left every zone drawn here — put
    // back into the library, most often. Leaving it where it was would be a
    // permanent nobody can remove.
    const b = board([
      { changes: [{ id: 11, zone: 'land', seat: 1 }] },
      { changes: [{ id: 11, zone: 'gone' }] },
    ])
    expect(foldBoard(b, 1).sides[0]?.land).toHaveLength(1)
    expect(foldBoard(b, 2).sides[0]?.land).toHaveLength(0)
  })

  it('ignores a zone it has never heard of rather than throwing', () => {
    // Forge is not an API and neither is our own wire across a deploy. A
    // browser that crashed on an unfamiliar word would take the whole match
    // down to avoid drawing one card.
    const b = board([{ changes: [{ id: 11, zone: 'sideboard', seat: 1 }] }])
    const state = foldBoard(b, 1)
    expect(state.sides[0]?.land).toHaveLength(0)
    expect(state.sides[0]?.battlefield).toHaveLength(0)
  })

  it('keeps a stable order, so a battlefield does not reshuffle on a tap', () => {
    const b = board([
      { changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { changes: [{ id: 12, zone: 'battlefield', seat: 1 }] },
      // A tap is a change to the *first* card; it must not send it to the end.
      { changes: [{ id: 10, tapped: true }] },
    ])
    expect(foldBoard(b, 3).sides[0]?.battlefield.map((c) => c.id))
      .toEqual([10, 12])
    expect(foldBoard(b, 3).sides[0]?.battlefield[0]?.tapped).toBe(true)
  })

  it('follows a permanent that changes seats', () => {
    // A stolen creature sits on the thief's side of the table. The dictionary
    // says whose card it started as; the change says whose it is now.
    const b = board([
      { changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { changes: [{ id: 10, seat: 2 }] },
    ])
    const state = foldBoard(b, 2)
    expect(state.sides[0]?.battlefield).toHaveLength(0)
    expect(state.sides[1]?.battlefield.map((c) => c.name))
      .toEqual(['Gyome, Master Chef'])
  })

  it('carries life, counters and the fighting stats', () => {
    const b = board([
      { changes: [{ id: 10, zone: 'battlefield', seat: 1, power: 5, toughness: 3 }] },
      {
        life: [{ seat: 2, life: 33 }],
        changes: [{ id: 10, power: 6, toughness: 4, counters: [{ kind: '+1/+1', n: 1 }] }],
      },
    ])
    const state = foldBoard(b, 2)
    const gyome = state.sides[0]?.battlefield[0]
    expect(gyome?.counters).toEqual([{ kind: '+1/+1', n: 1 }])
    expect(fightingStats(gyome!)).toBe('6/4')
    expect(state.sides[1]?.life).toBe(33)
  })

  it('says nothing about the power of something that is not a creature', () => {
    // Forge reports a Food token as 0/0 because it has no power at all.
    // Printing "0/0" on an artifact is a claim its card does not make.
    const b = board([
      { changes: [{ id: 12, zone: 'battlefield', seat: 1, power: 0, toughness: 0 }] },
    ])
    const food = foldBoard(b, 1).sides[0]?.battlefield[0]
    expect(food?.token).toBe(true)
    expect(fightingStats(food!)).toBeNull()
  })

  it('answers a match with no board at all', () => {
    // A worker without the scribe plays the match and reports no board. The
    // room draws the account alone rather than breaking.
    expect(foldBoard(null, 5)).toEqual({ turn: 0, active: 0, sides: [] })
  })
})
