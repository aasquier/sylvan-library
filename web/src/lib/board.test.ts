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
import type { BoardCard } from './board'
import { fightingStats, foldBoard, stackRow } from './board'

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

describe('sorting a side of the table', () => {
  /** One permanent, on the battlefield, described the way the wire does. */
  const sorted = (types: string, mana = false) => {
    const b: ForgeBoard = {
      seats: [{ seat: 1, slug: 'x', name: 'x', life: 40 }],
      cards: [{ id: 1, name: 'A card', types, seat: 1, mana }],
      steps: [{ changes: [{ id: 1, zone: 'battlefield', seat: 1 }] }],
    }
    const side = foldBoard(b, 1).sides[0]!
    return (['creatures', 'walkers', 'artifacts', 'enchantments',
      'land'] as const).find((row) => side[row].length > 0)
  }

  // The whole of Aaron's item 10, one line each. This used to be a single
  // "permanents" row holding all four of these — which is not how anybody lays
  // out a table, and is the reason you cannot find your own Equipment.
  it('gives artifacts and enchantments rows of their own', () => {
    expect(sorted('Artifact - Equipment')).toBe('artifacts')
    expect(sorted('Enchantment - Aura')).toBe('enchantments')
  })

  it('stands mana rocks back with the lands', () => {
    // "Mana producing artifacts could really stay back with the lands" — those
    // two rows together answer one question, and a Sol Ring answers it the
    // same way a Forest does. The flag is Scryfall's `produced_mana`; nothing
    // here reads rules text.
    expect(sorted('Artifact', true)).toBe('land')
    expect(sorted('Artifact', false)).toBe('artifacts')
  })

  it('keeps battles with the enchantments', () => {
    // Strictly neither, and rarely more than one on a board — a row of their
    // own would be an empty row all game.
    expect(sorted('Battle - Siege')).toBe('enchantments')
  })

  it('puts anything that is a creature at the front, whatever else it is', () => {
    // The front line is decided by what can be blocked, not by what is printed
    // first on a type line. A crewed Vehicle is in the fight; so is a mana dork
    // that would otherwise be filed under the lands for making mana.
    expect(sorted('Artifact Creature - Vehicle')).toBe('creatures')
    expect(sorted('Creature - Elf Druid', true)).toBe('creatures')
    expect(sorted('Enchantment Creature - Nymph')).toBe('creatures')
  })

  it('keeps a row for the planeswalkers, and for whatever comes next', () => {
    expect(sorted('Legendary Planeswalker - Ajani')).toBe('walkers')
    // A type nobody has invented yet still lands somewhere rather than falling
    // off the table.
    expect(sorted('Legendary Contraption')).toBe('walkers')
  })
})

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
    expect(one.sides[0]?.creatures).toHaveLength(0)

    const all = foldBoard(b, 3)
    // **The active player's own turn count, not Forge's.** Forge increments
    // once per player-turn and alternates seats, so its "turn 2" is seat 2's
    // *first*. Printing Forge's number made a seventh turn read as 14 or 15.
    expect(all.turn).toBe(1)
    expect(all.active).toBe(2)
    expect(all.sides[0]?.creatures.map((c) => c.name))
      .toEqual(['Gyome, Master Chef'])
    expect(all.sides[1]?.creatures.map((c) => c.name))
      .toEqual(['Dragonlord Atarka'])
  })

  it('counts each player their own turns, the way a player would', () => {
    // Forge: 1,2,3,4 alternating seats. A player: "my second turn".
    const b = board([
      { turn: 1, seat: 1, changes: [] },
      { turn: 2, seat: 2, changes: [] },
      { turn: 3, seat: 1, changes: [] },
      { turn: 4, seat: 2, changes: [] },
      { turn: 5, seat: 1, changes: [] },
    ])
    expect(foldBoard(b, 1).turn).toBe(1)
    expect(foldBoard(b, 2).turn).toBe(1)
    expect(foldBoard(b, 3).turn).toBe(2)
    expect(foldBoard(b, 4).turn).toBe(2)
    // Forge's turn 5 is seat 1's third — not "turn 5", which is what the seam
    // used to say and what Aaron caught.
    expect(foldBoard(b, 5).turn).toBe(3)
  })

  it('counts an extra turn as a turn, rather than halving', () => {
    // Time Warp: one player takes two in a row. Halving Forge's number would
    // credit the opponent with a turn they never took; counting per seat is
    // right in both cases.
    const b = board([
      { turn: 1, seat: 1, changes: [] },
      { turn: 2, seat: 2, changes: [] },
      { turn: 3, seat: 1, changes: [] },
      { turn: 4, seat: 1, changes: [] },
    ])
    expect(foldBoard(b, 4).turn).toBe(3)
    expect(foldBoard(b, 4).active).toBe(1)
  })

  it('never runs past the steps it was given', () => {
    const b = board([{ turn: 1, changes: [{ id: 11, zone: 'land', seat: 1 }] }])
    // A count beyond the end is what a room does when a game's beats are
    // drained faster than the next game arrives; it is not an error.
    expect(foldBoard(b, 99).sides[0]?.land).toHaveLength(1)
    expect(foldBoard(b, -3).sides[0]?.land).toHaveLength(0)
  })

  it('takes a card off the table when it is gone, one beat after it goes', () => {
    // `gone` is the server saying a card has left every zone drawn here — put
    // back into the library, most often. Leaving it where it was would be a
    // permanent nobody can remove.
    //
    // **But not on the beat it leaves.** A card that left the battlefield on
    // the step being drawn is held standing there, so a person watching sees
    // the departure rather than finding a hole where a permanent was. It is
    // `leaving` for exactly that beat and gone on the next one.
    const b = board([
      { changes: [{ id: 11, zone: 'land', seat: 1 }] },
      { changes: [{ id: 11, zone: 'gone' }] },
      { changes: [] },
    ])
    expect(foldBoard(b, 1).sides[0]?.land).toHaveLength(1)
    expect(foldBoard(b, 1).sides[0]?.land[0]?.leaving).toBeNull()

    const going = foldBoard(b, 2).sides[0]?.land
    expect(going, 'held for the beat that says it left').toHaveLength(1)
    expect(going?.[0]?.leaving).toBe('land')

    expect(foldBoard(b, 3).sides[0]?.land,
      'and off the table on the next').toHaveLength(0)
  })

  it('ignores a zone it has never heard of rather than throwing', () => {
    // Forge is not an API and neither is our own wire across a deploy. A
    // browser that crashed on an unfamiliar word would take the whole match
    // down to avoid drawing one card.
    const b = board([{ changes: [{ id: 11, zone: 'sideboard', seat: 1 }] }])
    const state = foldBoard(b, 1)
    expect(state.sides[0]?.land).toHaveLength(0)
    expect(state.sides[0]?.creatures).toHaveLength(0)
  })

  it('keeps a stable order, so a battlefield does not reshuffle on a tap', () => {
    const b = board([
      { changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { changes: [{ id: 12, zone: 'battlefield', seat: 1 }] },
      // A tap is a change to the *first* card; it must not send it to the end.
      { changes: [{ id: 10, tapped: true }] },
    ])
    // Gyome is a creature and a Food token is an artifact, so they sit in
    // different rows now — the order that matters is within a row.
    expect(foldBoard(b, 3).sides[0]?.creatures.map((c) => c.id)).toEqual([10])
    expect(foldBoard(b, 3).sides[0]?.artifacts.map((c: BoardCard) => c.id))
      .toEqual([12])
    expect(foldBoard(b, 3).sides[0]?.creatures[0]?.tapped).toBe(true)
  })

  it('follows a permanent that changes seats', () => {
    // A stolen creature sits on the thief's side of the table. The dictionary
    // says whose card it started as; the change says whose it is now.
    const b = board([
      { changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { changes: [{ id: 10, seat: 2 }] },
    ])
    const state = foldBoard(b, 2)
    expect(state.sides[0]?.creatures).toHaveLength(0)
    expect(state.sides[1]?.creatures.map((c) => c.name))
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
    const gyome = state.sides[0]?.creatures[0]
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
    const food = foldBoard(b, 1).sides[0]?.artifacts[0]
    expect(food?.token).toBe(true)
    expect(fightingStats(food!)).toBeNull()
  })

  it('stacks identical cards, the way they sit on a table', () => {
    // Commander is a singleton format, so the only things that ever repeat are
    // basic lands and tokens — which is precisely where a flat row is worst:
    // nine Forests and eight Food take the whole board and say two things.
    const forest = (id: number, tapped = false): BoardCard => ({
      id, name: 'Forest', token: false, types: 'Basic Land - Forest',
      image: '', art: '', artist: '', zone: 'land', seat: 1, tapped,
      mana: false, keywords: [], leaving: null, power: null,
      toughness: null, counters: [], casts: 0,
      attachedTo: 0, attachments: [],
    })
    const stacks = stackRow([forest(1), forest(2), forest(3)])
    expect(stacks).toHaveLength(1)
    expect(stacks[0]?.count).toBe(3)
    // The first card is the one drawn, so the pile keeps its place in the row.
    expect(stacks[0]?.card.id).toBe(1)
  })

  it('keeps tapped and untapped as separate piles', () => {
    // **The ruling that makes this worth testing.** Three tapped Forests and
    // six untapped ones are not nine Forests — they are two piles, and which
    // is which is the thing somebody is reading the board to find out. Merging
    // them would delete the answer to "can they still pay for that?"
    const forest = (id: number, tapped: boolean): BoardCard => ({
      id, name: 'Forest', token: false, types: 'Basic Land - Forest',
      image: '', art: '', artist: '', zone: 'land', seat: 1, tapped,
      mana: false, keywords: [], leaving: null, power: null,
      toughness: null, counters: [], casts: 0,
      attachedTo: 0, attachments: [],
    })
    const stacks = stackRow([
      forest(1, true), forest(2, false), forest(3, true), forest(4, false),
    ])
    expect(stacks).toHaveLength(2)
    expect(stacks.map((s) => [s.card.tapped, s.count]))
      .toEqual([[true, 2], [false, 2]])
  })

  it('keeps creatures apart when anything visible differs', () => {
    // A 1/1 Cat with a +1/+1 counter on it is not the same object as the 1/1
    // beside it, and a player can see that from across the table.
    const cat = (id: number, over: Partial<BoardCard> = {}): BoardCard => ({
      id, name: 'Cat Token', token: true, types: 'Creature - Cat',
      image: '', art: '', artist: '', zone: 'battlefield', seat: 1,
      tapped: false, mana: false, keywords: [], leaving: null, power: 1,
      toughness: 1, counters: [], casts: 0,
      attachedTo: 0, attachments: [], ...over,
    })
    const stacks = stackRow([
      cat(1),
      cat(2),
      cat(3, { power: 2, toughness: 2, counters: [{ kind: '+1/+1', n: 1 }] }),
    ])
    expect(stacks.map((s) => s.count)).toEqual([2, 1])
  })

  it('leaves a row of different cards alone', () => {
    const b = board([
      { changes: [
        { id: 10, zone: 'battlefield', seat: 1 },
        { id: 20, zone: 'battlefield', seat: 1 },
      ] },
    ])
    const bf = foldBoard(b, 1).sides[0]?.creatures ?? []
    expect(stackRow(bf).every((s) => s.count === 1)).toBe(true)
  })

  it('answers a match with no board at all', () => {
    // A worker without the scribe plays the match and reports no board. The
    // room draws the account alone rather than breaking.
    expect(foldBoard(null, 5)).toEqual({ turn: 0, active: 0, sides: [] })
  })
})

describe('the command zone as places', () => {
  /** A seat that ran a pairing and a companion, with an emblem in the zone. */
  const zone: ForgeBoard = {
    seats: [{
      seat: 1, slug: 'pair', name: 'A pairing', life: 40,
      commanders: [1, 2], companion: 3,
    }],
    cards: [
      { id: 1, name: 'Thrasios, Triton Hero', seat: 1,
        types: 'Legendary Creature - Merfolk Wizard' },
      { id: 2, name: 'Tymna the Weaver', seat: 1,
        types: 'Legendary Creature - Human Cleric' },
      { id: 3, name: 'Kaheera, the Orphanguard', seat: 1,
        types: 'Legendary Creature - Cat Beast' },
      { id: 4, name: 'Emblem', seat: 1, types: 'Emblem' },
    ],
    steps: [
      { turn: 1, seat: 1, changes: [
        { id: 1, zone: 'command', seat: 1 },
        { id: 2, zone: 'command', seat: 1 },
        { id: 3, zone: 'command', seat: 1 },
        { id: 4, zone: 'command', seat: 1 },
      ] },
      { turn: 2, seat: 1, changes: [{ id: 2, zone: 'battlefield', seat: 1 }] },
    ],
  } as unknown as ForgeBoard

  it('holds a throne for every commander, wherever it is standing', () => {
    const out = foldBoard(zone, 2)
    const side = out.sides[0]
    // Tymna is on the battlefield and still has a chair. That is the whole
    // point of drawing the zone as places: an empty seat says *where she is*,
    // and a pile that simply dropped her would say nothing at all.
    expect(side?.thrones.map((c) => c.name))
      .toEqual(['Thrasios, Triton Hero', 'Tymna the Weaver'])
    expect(side?.thrones[1]?.zone).toBe('battlefield')
    expect(side?.creatures.map((c) => c.name)).toEqual(['Tymna the Weaver'])
  })

  it('keeps the companion out of the commanders it prices', () => {
    const side = foldBoard(zone, 2).sides[0]
    expect(side?.companion?.name).toBe('Kaheera, the Orphanguard')
    // A companion in the command zone is not a commander who has been cast
    // from it. Counting it as one put a price on the rail for a card that has
    // never been cast from anywhere.
    expect(side?.commanders.map((c) => c.name))
      .not.toContain('Kaheera, the Orphanguard')
  })

  it('leaves whatever else is in the zone as a pile', () => {
    const side = foldBoard(zone, 2).sides[0]
    // The two commanders and the companion have places of their own, so what
    // is left is the emblem — and it is still drawn, because a card the room
    // silently dropped would be a card nobody could find.
    expect(side?.command.map((c) => c.name)).toEqual(['Emblem'])
  })

  it('draws the whole zone as one pile when nobody named the seats', () => {
    // A board shaped by a server that predates this, or a mid-deploy skew.
    // Nothing is lost: the zone goes back to being a pile.
    const older = { ...zone, seats: [{ ...zone.seats[0]! }] }
    delete (older.seats[0] as { commanders?: number[] }).commanders
    delete (older.seats[0] as { companion?: number }).companion
    const side = foldBoard(older, 2).sides[0]
    expect(side?.thrones).toHaveLength(0)
    expect(side?.companion).toBeNull()
    expect(side?.command.map((c) => c.name).sort())
      .toEqual(['Emblem', 'Kaheera, the Orphanguard', 'Thrasios, Triton Hero'])
  })
})

describe('a permanent and what is attached to it', () => {
  /** A bear, a sword, and a land with an Aura on it. */
  const gear: ForgeBoard = {
    seats: [{ seat: 1, slug: 'x', name: 'x', life: 40 }],
    cards: [
      { id: 1, name: 'Ulvenwald Tracker', seat: 1, types: 'Creature - Bear' },
      { id: 2, name: 'Lightning Greaves', seat: 1, types: 'Artifact - Equipment' },
      { id: 3, name: 'Forest', seat: 1, types: 'Basic Land - Forest' },
      { id: 4, name: 'Utopia Sprawl', seat: 1, types: 'Enchantment - Aura' },
    ],
    steps: [
      { turn: 1, seat: 1, changes: [
        { id: 1, zone: 'battlefield', seat: 1 },
        { id: 2, zone: 'battlefield', seat: 1 },
        { id: 3, zone: 'land', seat: 1 },
        { id: 4, zone: 'battlefield', seat: 1 },
      ] },
      // Equipped, and the Aura lands on the Forest.
      { turn: 2, seat: 1, changes: [
        { id: 2, attached_to: 1 },
        { id: 4, attached_to: 3 },
      ] },
      // The sword comes off. Forge sends this itself; see `board.go`.
      { turn: 3, seat: 1, changes: [{ id: 2, attached_to: 0 }] },
    ],
  } as unknown as ForgeBoard

  it('puts an attachment on its host instead of in a row of its own', () => {
    const side = foldBoard(gear, 2).sides[0]
    expect(side?.creatures.map((c) => c.name)).toEqual(['Ulvenwald Tracker'])
    expect(side?.creatures[0]?.attachments.map((a) => a.name))
      .toEqual(['Lightning Greaves'])
    // And it is **not** also standing in the artifacts row. A card in two
    // places is worse than a card in the wrong one.
    expect(side?.artifacts).toHaveLength(0)
  })

  it('follows the host into whichever row the host is standing in', () => {
    // An Aura on a land rides the land row. The attachment does not get a
    // say in where it is drawn — it goes where its host goes, which is the
    // whole point of it being attached.
    const side = foldBoard(gear, 2).sides[0]
    const forest = side?.land.find((c) => c.name === 'Forest')
    expect(forest?.attachments.map((a) => a.name)).toEqual(['Utopia Sprawl'])
    expect(side?.enchantments).toHaveLength(0)
  })

  it('gives a detached attachment its own row back', () => {
    // **Zero is the detach and this is the half that matters.** Reading
    // `attached_to` as truthy would leave the sword drawn on the bear forever,
    // and the row it belongs in would stay empty — a card that is on the
    // battlefield and nowhere on the board.
    const side = foldBoard(gear, 3).sides[0]
    expect(side?.creatures[0]?.attachments).toHaveLength(0)
    expect(side?.artifacts.map((c) => c.name)).toEqual(['Lightning Greaves'])
  })

  it('does not merge an equipped creature with a bare one', () => {
    // Two identical bears, one carrying a sword, are two piles. Merging them
    // would hide the sword and miscount the bears in one stroke.
    const twin: ForgeBoard = {
      ...gear,
      cards: [...gear.cards,
        { id: 5, name: 'Ulvenwald Tracker', seat: 1, types: 'Creature - Bear' },
      ],
      steps: [gear.steps[0]!, {
        turn: 2, seat: 1, changes: [
          { id: 5, zone: 'battlefield', seat: 1 },
          { id: 2, attached_to: 1 },
        ],
      }],
    } as unknown as ForgeBoard
    const side = foldBoard(twin, 2).sides[0]
    const stacks = stackRow(side?.creatures ?? [])
    expect(stacks).toHaveLength(2)
    expect(stacks.map((s) => s.card.attachments.length)).toEqual([1, 0])
  })
})
