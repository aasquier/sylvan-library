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
      mana: false, makes: [], keywords: [], leaving: null, power: null,
      toughness: null, counters: [], counterHistory: [], combat: '',
      attacking: 0, blocking: 0, casts: 0, attachedTo: 0, attachments: [],
      live: [], granted: [], fate: '', copiedBy: 0,
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
      mana: false, makes: [], keywords: [], leaving: null, power: null,
      toughness: null, counters: [], counterHistory: [], combat: '',
      attacking: 0, blocking: 0, casts: 0, attachedTo: 0, attachments: [],
      live: [], granted: [], fate: '', copiedBy: 0,
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
      tapped: false, mana: false, makes: [], keywords: [], leaving: null,
      power: 1,
      toughness: 1, counters: [], counterHistory: [], combat: '',
      attacking: 0, blocking: 0, casts: 0, attachedTo: 0, attachments: [],
      live: [], granted: [], fate: '', copiedBy: 0,
      ...over,
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
    expect(foldBoard(null, 5)).toEqual({
      turn: 0, active: 0, sides: [], abilities: [], floating: [],
    })
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
      // The cast, and its price. `casts` is the server's count of the times
      // this card has left the command zone; the browser used to derive it by
      // watching the same transition, which was a reading of the game made in
      // the one file that makes none.
      { turn: 2, seat: 1, changes: [
        { id: 2, zone: 'battlefield', seat: 1, casts: 1 },
      ] },
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

  it('takes the tax from the server rather than counting it here', () => {
    // Tymna has been cast once, and the count that says so is the server's.
    const side = foldBoard(zone, 2).sides[0]
    expect(side?.commanders.find((c) => c.name === 'Tymna the Weaver')?.casts)
      .toBe(1)

    // **And it really is read, not derived.** The same board with the count
    // taken off the wire prices her at nothing: the transition is still there
    // to watch, and nothing here watches it. That is the point — Forge reports
    // no tax, so counting the transitions is a reading of the game, and every
    // reading belongs on the other side of the wire (ADR 14).
    const silent = {
      ...zone,
      steps: [zone.steps[0]!, {
        turn: 2, seat: 1,
        changes: [{ id: 2, zone: 'battlefield', seat: 1 }],
      }],
    } as unknown as ForgeBoard
    const quiet = foldBoard(silent, 2).sides[0]
    expect(quiet?.commanders.map((c) => c.casts) ?? []).not.toContain(1)
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

  /** Two Beast tokens and whatever the steps hang on them. Tokens because
   *  Commander is singleton: they are the only cards that ever repeat, so a
   *  stack of more than one is a thing only tokens can be. */
  const beasts = (steps: unknown[], extra: unknown[] = []): ForgeBoard => ({
    seats: [{ seat: 1, slug: 'x', name: 'x', life: 40 }],
    cards: [
      { id: 1, name: 'Beast Token', token: true, seat: 1, types: 'Creature - Beast' },
      { id: 2, name: 'Beast Token', token: true, seat: 1, types: 'Creature - Beast' },
      ...extra,
    ],
    steps: [
      { turn: 1, seat: 1, changes: [
        { id: 1, zone: 'battlefield', seat: 1, power: 3, toughness: 3 },
        { id: 2, zone: 'battlefield', seat: 1, power: 3, toughness: 3 },
      ] },
      ...steps,
    ],
  } as unknown as ForgeBoard)

  it('gives an equipped token its own pile', () => {
    // Aaron, 2026-08-26: *"equipment on a token should put it in its own
    // pile"*. A 3/3 Beast holding a sword is a different object from the 3/3
    // beside it — different power, different behaviour — and a player has to
    // be able to see which is which.
    const b = beasts([
      { turn: 2, seat: 1, changes: [{ id: 3, attached_to: 1 }] },
    ], [{ id: 3, name: 'Bonesplitter', seat: 1, types: 'Artifact - Equipment' }])
    ;(b.steps[0]!.changes as unknown[]).push(
      { id: 3, zone: 'battlefield', seat: 1 })
    const stacks = stackRow(foldBoard(b, 2).sides[0]?.creatures ?? [])
    expect(stacks.map((s) => [s.count, s.card.attachments.length]))
      .toEqual([[1, 1], [1, 0]])
  })

  it('splits an equipped token off even when the sword has no name', () => {
    // **The bug, and it is a bug about the key rather than about equipment.**
    // A card the server never put in the dictionary folds with an empty name,
    // and a key built from names alone renders that as the empty string —
    // which is exactly what a card carrying *nothing* contributes. So the two
    // piles collided, and the survivor drew one sword above a count of two.
    //
    // Only tokens could ever show it: a stack of one is a stack whose key was
    // never tested. Hence a token here, and hence the count in the key.
    const b = beasts([
      // No dictionary entry for id 3 at all — the case `foldBoard`'s
      // `known?.name ?? ''` has always allowed for.
      { turn: 2, seat: 1, changes: [{ id: 3, attached_to: 1 }] },
    ])
    ;(b.steps[0]!.changes as unknown[]).push(
      { id: 3, zone: 'battlefield', seat: 1 })
    const stacks = stackRow(foldBoard(b, 2).sides[0]?.creatures ?? [])
    expect(stacks.map((s) => [s.count, s.card.attachments.length]))
      .toEqual([[1, 1], [1, 0]])
  })

  it('still stacks two tokens carrying the same thing', () => {
    // The other half of the ruling, and the half a naive fix breaks: two Bears
    // each holding their own Bonesplitter are the same as each other. Keying
    // on the instance id would split them and hand back the row of loose
    // objects this whole function exists to fold up.
    const b = beasts([
      { turn: 2, seat: 1, changes: [
        { id: 3, attached_to: 1 }, { id: 4, attached_to: 2 },
      ] },
    ], [
      { id: 3, name: 'Bonesplitter', seat: 1, types: 'Artifact - Equipment' },
      { id: 4, name: 'Bonesplitter', seat: 1, types: 'Artifact - Equipment' },
    ])
    ;(b.steps[0]!.changes as unknown[]).push(
      { id: 3, zone: 'battlefield', seat: 1 },
      { id: 4, zone: 'battlefield', seat: 1 })
    const stacks = stackRow(foldBoard(b, 2).sides[0]?.creatures ?? [])
    expect(stacks).toHaveLength(1)
    expect(stacks[0]?.count).toBe(2)
  })

  it('does not mind which order two creatures picked the same gear up in', () => {
    // The tuck shows a corner per attachment: how many, never which came
    // first. Two creatures wearing the same two things are one pile, and the
    // sort in the key is what makes the attach order stop mattering.
    const b = beasts([
      { turn: 2, seat: 1, changes: [
        { id: 3, attached_to: 1 }, { id: 4, attached_to: 1 },
        { id: 5, attached_to: 2 }, { id: 6, attached_to: 2 },
      ] },
    ], [
      { id: 3, name: 'Rancor', seat: 1, types: 'Enchantment - Aura' },
      { id: 4, name: 'Bonesplitter', seat: 1, types: 'Artifact - Equipment' },
      { id: 5, name: 'Bonesplitter', seat: 1, types: 'Artifact - Equipment' },
      { id: 6, name: 'Rancor', seat: 1, types: 'Enchantment - Aura' },
    ])
    ;(b.steps[0]!.changes as unknown[]).push(
      { id: 3, zone: 'battlefield', seat: 1 },
      { id: 4, zone: 'battlefield', seat: 1 },
      { id: 5, zone: 'battlefield', seat: 1 },
      { id: 6, zone: 'battlefield', seat: 1 })
    const stacks = stackRow(foldBoard(b, 2).sides[0]?.creatures ?? [])
    expect(stacks).toHaveLength(1)
    expect(stacks[0]?.count).toBe(2)
  })
})

describe('counters, and the account of how they got there', () => {
  it('takes the counters off a card that has left the battlefield', () => {
    // **Aaron's bug, 2026-08-26:** *"counters are following things into exile,
    // the graveyard, and the command zone, they fall off a creature when they
    // move to any of those zones"* — which is rule 400.7, and the object that
    // arrives in the graveyard is not the one that had them.
    //
    // The shedding is decided on the server, where every reading of the game
    // is. What was wrong on this side is the *reading*: an empty set was taken
    // for "nothing changed" and the old counters survived it. Read two beats
    // after the death, because the beat of the death itself is the hold below.
    const b = board([
      { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { turn: 1, seat: 1, changes: [{ id: 10, counters: [{ kind: '+1/+1', n: 2 }] }] },
      { turn: 2, seat: 2, changes: [{ id: 10, zone: 'graveyard', counters: [] }] },
      { turn: 2, seat: 2, changes: [] },
    ])
    expect(foldBoard(b, 2).sides[0]?.creatures[0]?.counters)
      .toEqual([{ kind: '+1/+1', n: 2 }])
    const dead = foldBoard(b, 4).sides[0]?.graveyard[0]
    expect(dead?.name).toBe('Gyome, Master Chef')
    expect(dead?.counters, 'a new object arrives with nothing on it')
      .toEqual([])
  })

  it('keeps them for the one beat the creature is drawn dying', () => {
    // The board deliberately stands a dead creature back up for the beat that
    // says it died, so the skull has a card to land on rather than a hole. In
    // that instant somebody is looking straight at the creature — and it is
    // the creature as it died, counters and all. Stripping them on the same
    // beat would be a beat early, and the whole reason for the hold is that a
    // beat matters.
    const b = board([
      { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      { turn: 1, seat: 1, changes: [{ id: 10, counters: [{ kind: '+1/+1', n: 2 }] }] },
      { turn: 2, seat: 2, changes: [{ id: 10, zone: 'graveyard', counters: [] }] },
      { turn: 2, seat: 2, changes: [] },
    ])
    const dying = foldBoard(b, 3).sides[0]?.creatures[0]
    expect(dying?.leaving, 'held on the sand for this beat').toBe('battlefield')
    expect(dying?.counters).toEqual([{ kind: '+1/+1', n: 2 }])
  })

  it('remembers when each counter arrived, and on whose turn', () => {
    // The hover's account. A set of `+1/+1: 3` cannot say when the three
    // arrived, because by then the arithmetic has scrolled past.
    const b = board([
      { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      {
        turn: 1,
        seat: 1,
        changes: [{
          id: 10,
          counters: [{ kind: '+1/+1', n: 2 }],
          counter_moves: [{ kind: '+1/+1', was: 0, now: 2 }],
        }],
      },
      { turn: 3, seat: 1, changes: [] },
      {
        turn: 3,
        seat: 1,
        changes: [{
          id: 10,
          counters: [{ kind: '+1/+1', n: 3 }],
          counter_moves: [{ kind: '+1/+1', was: 2, now: 3 }],
        }],
      },
    ])
    const gyome = foldBoard(b, 4).sides[0]?.creatures[0]
    // The turn is the one a player would say — this seat's own count, not
    // Forge's, which counts each player's turn separately. Forge's 1 and 3 are
    // seat one's first and second.
    expect(gyome?.counterHistory).toEqual([
      { kind: '+1/+1', was: 0, now: 2, turn: 1 },
      { kind: '+1/+1', was: 2, now: 3, turn: 2 },
    ])
  })

  it('folds a run of counters on one turn into one moment', () => {
    // Three separate +1/+1 counters on one turn is one thing that happened,
    // and it is what a person would say happened. Three lines is a log, not an
    // account — and it is what bounds the history, which the fold rebuilds
    // from step zero on every render.
    const b = board([
      { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      ...[1, 2, 3].map((n) => ({
        turn: 1,
        seat: 1,
        changes: [{
          id: 10,
          counters: [{ kind: '+1/+1', n }],
          counter_moves: [{ kind: '+1/+1', was: n - 1, now: n }],
        }],
      })),
    ])
    expect(foldBoard(b, 4).sides[0]?.creatures[0]?.counterHistory)
      .toEqual([{ kind: '+1/+1', was: 0, now: 3, turn: 1 }])
  })

  it('drops the account when the counters are gone', () => {
    // A card with nothing on it has nothing to explain. It is also what keeps
    // a history from outliving the object it describes: the same empty set the
    // server sends when a creature changes zones clears the story too, so a
    // creature that dies and comes back starts a new one.
    const b = board([
      { turn: 1, seat: 1, changes: [{ id: 10, zone: 'battlefield', seat: 1 }] },
      {
        turn: 1,
        seat: 1,
        changes: [{
          id: 10,
          counters: [{ kind: '+1/+1', n: 1 }],
          counter_moves: [{ kind: '+1/+1', was: 0, now: 1 }],
        }],
      },
      { turn: 2, seat: 1, changes: [{ id: 10, counters: [] }] },
    ])
    const gyome = foldBoard(b, 3).sides[0]?.creatures[0]
    expect(gyome?.counters).toEqual([])
    expect(gyome?.counterHistory).toEqual([])
  })
})

describe('the fight', () => {
  /** Gyome swinging at seat 2, and an Egg thrown in front of him. */
  const combat: ForgeBoard = {
    seats: [
      { seat: 1, slug: 'gyome', name: 'Gyome — Food', life: 40 },
      { seat: 2, slug: 'atla', name: 'Atla — Eggs', life: 40 },
    ],
    cards: [
      { id: 10, name: 'Gyome, Master Chef', seat: 1,
        types: 'Legendary Creature - Troll Warlock' },
      { id: 21, name: 'Egg Token', token: true, seat: 2,
        types: 'Creature - Egg' },
    ],
    steps: [
      { turn: 3, seat: 1, changes: [
        { id: 10, zone: 'battlefield', seat: 1 },
        { id: 21, zone: 'battlefield', seat: 2 },
      ] },
      { turn: 3, seat: 1, changes: [
        { id: 10, combat: 'attacking', attacking: 2 },
        { id: 21, combat: 'blocking', blocking: 10 },
      ] },
      { turn: 4, seat: 2, changes: [
        { id: 10, combat: '', attacking: 0 },
        { id: 21, combat: '', blocking: 0 },
      ] },
    ],
  } as unknown as ForgeBoard

  it('says who is swinging and who is in the way', () => {
    // The board could not draw a fight at all: `attack` and `block` were beats
    // and nothing else, so the account said who was swinging and the picture
    // never did.
    const state = foldBoard(combat, 2)
    const attacker = state.sides[0]?.creatures[0]
    expect(attacker?.combat).toBe('attacking')
    expect(attacker?.attacking, 'the seat under attack').toBe(2)
    const blocker = state.sides[1]?.creatures[0]
    expect(blocker?.combat).toBe('blocking')
    // **By id, not by name.** Two Egg Tokens are one name between them, and a
    // blocker paired to "the attacker called Egg Token" is paired to whichever
    // one is drawn first.
    expect(blocker?.blocking).toBe(10)
  })

  it('takes them out of the fight when the turn turns over', () => {
    // The empty string is a real value and `undefined` is not the same thing:
    // read as "nothing changed", a sword mark would stay on a creature for the
    // rest of the game.
    const state = foldBoard(combat, 3)
    expect(state.sides[0]?.creatures[0]?.combat).toBe('')
    expect(state.sides[0]?.creatures[0]?.attacking).toBe(0)
    expect(state.sides[1]?.creatures[0]?.blocking).toBe(0)
  })

  it('stacks an attacking token pile apart from a blocking one', () => {
    // Aaron, 2026-08-26: *"make sure when it comes to tokens that blocking and
    // attacking token piles are separated visually"*. Twelve Saprolings of
    // which five are swinging is the one fact anybody across the seam wants,
    // and a single stack of twelve deletes it.
    const saproling = (id: number, combat: string): BoardCard => ({
      id, name: 'Saproling Token', token: true, types: 'Creature - Saproling',
      image: '', art: '', artist: '', zone: 'battlefield', seat: 1,
      tapped: false, mana: false, makes: [], keywords: [], leaving: null,
      power: 1,
      toughness: 1, counters: [], counterHistory: [], combat,
      attacking: 0, blocking: 0, casts: 0, attachedTo: 0, attachments: [],
      live: [], granted: [], fate: '', copiedBy: 0,
    })
    const stacks = stackRow([
      saproling(1, 'attacking'), saproling(2, ''),
      saproling(3, 'attacking'), saproling(4, 'blocking'),
    ])
    expect(stacks.map((s) => [s.card.combat, s.count]))
      .toEqual([['attacking', 2], ['', 1], ['blocking', 1]])
  })
})

describe('what a beat says beyond where the cards are', () => {
  // The four things Forge's bus started saying in ADR 45. The rulings about
  // the *game* are Go's and are tested there against a real recorded match;
  // what is left here is the arithmetic of folding them.

  it('keeps every value a mana pool took, and rests on the last', () => {
    // A pool fills and empties several times between two beats, so the value a
    // step *ends* on is almost always empty — measured on a real match at nine
    // empty out of ten. The resting total is what a rail draws; the sequence
    // is what an arrival animates, and both are the same field.
    const b = board([
      { floating: [
        { seat: 1, pool: 'W' }, { seat: 1, pool: '' },
        { seat: 1, pool: 'CC' }, { seat: 1, pool: 'C' },
      ] },
    ])
    const state = foldBoard(b, 1)
    expect(state.sides[0]?.pool).toBe('C')
    expect(state.floating.map((m) => m.pool)).toEqual(['W', '', 'CC', 'C'])
  })

  it('drops the pool movement once the beat has passed', () => {
    // `floating` belongs to the beat being drawn, not to the game so far —
    // otherwise mana that was spent two turns ago goes on arriving forever.
    const b = board([
      { floating: [{ seat: 1, pool: 'GG' }] },
      { changes: [{ id: 11, zone: 'land', seat: 1 }] },
    ])
    const state = foldBoard(b, 2)
    expect(state.floating).toEqual([])
    // The pool itself is state and stays where it was left.
    expect(state.sides[0]?.pool).toBe('GG')
  })

  it('reads the abilities of the beat being drawn and no others', () => {
    // Using an ability is a moment rather than a state, which is what makes an
    // eminence trigger drawable at all: its commander never leaves the command
    // zone, so nothing else in the stream says it acted.
    const b = board([
      { abilities: [{ id: 10, seat: 1, zone: 'Command', trigger: true }] },
      { changes: [{ id: 11, zone: 'land', seat: 1 }] },
    ])
    expect(foldBoard(b, 1).abilities).toEqual([
      { id: 10, seat: 1, zone: 'Command', trigger: true },
    ])
    expect(foldBoard(b, 2).abilities).toEqual([])
  })

  it('takes a granted keyword and an empty set as different answers', () => {
    // An empty array means a creature has lost the last keyword something was
    // giving it, and `undefined` means this step said nothing about keywords —
    // the same distinction `counters` turns on, and the same bug if either is
    // read as truthy.
    const b = board([
      { changes: [{ id: 10, zone: 'battlefield', seat: 1,
        live: ['Vigilance'], granted: ['Vigilance'] }] },
      { changes: [{ id: 10, tapped: true }] },
      { changes: [{ id: 10, live: [], granted: [] }] },
    ])
    const gained = foldBoard(b, 1).sides[0]?.creatures[0]
    expect(gained?.granted).toEqual(['Vigilance'])
    // A step that says nothing about keywords leaves them alone.
    expect(foldBoard(b, 2).sides[0]?.creatures[0]?.granted)
      .toEqual(['Vigilance'])
    // And the grant going away is published rather than left on the card.
    expect(foldBoard(b, 3).sides[0]?.creatures[0]?.granted).toEqual([])
  })

  it('carries how a permanent left, when Forge said so', () => {
    const b = board([
      { changes: [{ id: 11, zone: 'land', seat: 1 }] },
      { changes: [{ id: 11, zone: 'gone', fate: 'sacrificed' }] },
    ])
    // Held on the sand for the beat that says it is leaving, which is where a
    // mark has something to land on.
    expect(foldBoard(b, 2).sides[0]?.land[0]?.fate).toBe('sacrificed')
  })

  it('names the card whose ability made a token a copy', () => {
    // Populate's whole signal: its presence is the copy. **Not** what the
    // token was copied from — a Centaur Token populated by Growing Ranks names
    // Growing Ranks, and the permanent it duplicated never crosses the wire.
    const b: ForgeBoard = {
      seats: [{ seat: 1, slug: 'x', name: 'x', life: 40 }],
      cards: [
        { id: 158, name: 'Growing Ranks', types: 'Enchantment', seat: 1 },
        { id: 212, name: 'Centaur Token', token: true, seat: 1,
          types: 'Creature - Centaur', copied_by: 158 },
        { id: 210, name: 'Centaur Token', token: true, seat: 1,
          types: 'Creature - Centaur' },
      ],
      steps: [{ changes: [
        { id: 212, zone: 'battlefield', seat: 1 },
        { id: 210, zone: 'battlefield', seat: 1 },
      ] }],
    }
    const creatures = foldBoard(b, 1).sides[0]!.creatures
    expect(creatures.find((c) => c.id === 212)?.copiedBy).toBe(158)
    expect(creatures.find((c) => c.id === 210)?.copiedBy).toBe(0)
  })
})
