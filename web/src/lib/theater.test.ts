/**
 * The theater's two pure functions (`components/theater.test.tsx` covers the
 * stage they dress):
 *
 * - **`theaterRows` is total.** It is handed `Job.partial`, which is
 *   `unknown` and legitimately arrives as `null` (before the first tick, and
 *   again the moment the job finishes), as an object with no `rows` at all (a
 *   pre-theater worker streaming counts — the skew `worker.run_match`
 *   tolerates on purpose), and as the real payload. All three are "no rows
 *   yet" and none of them may throw.
 * - **`spell` is a duration in a person's units**, and `whatIsLeft` is the
 *   only thing on the stage that makes a claim about the future: how much of
 *   a match is still to come, measured off the games it has already fought.
 *   Its properties are all about refusing to overpromise — a median rather
 *   than a mean, the slow half of an even sample, and silence until there are
 *   two games to measure.
 * - **`shortName` is how a deck is referred to out loud** — the general's
 *   name, which is what fits in a feed row. The cases below are mostly about
 *   when it must *not* shorten: it decides by looking the general up rather
 *   than by cutting at a comma, because a deck's name is prose and "Life, Uh,
 *   Finds a Way" is not a deck called "Life".
 * - **`legendName` is the same cut for a card**, where the comma really is a
 *   separator because Wizards printed it there.
 * - **`beatLine` turns one beat into English**, and the two things worth
 *   holding are that a kind it has never heard of is still rendered, and that
 *   `who` is the player the sentence is *about* rather than the seat the wire
 *   happened to carry.
 */

import { describe, expect, it } from 'vitest'
import type { ForgeBeat, ForgeGameRow } from './api'
import {
  beatLine, legendName, shortName, spell, theaterRows, whatIsLeft,
} from './theater'

function row(over: Partial<ForgeGameRow> = {}): ForgeGameRow {
  return {
    game: 1, winner: 'arahbo-cats', seconds: 6.2, turns: 9, draw: false,
    timed_out: false, ...over,
  }
}

describe('spell', () => {
  // A unit a machine chose, turned into one a person uses — and the only
  // interesting case is the boundary, because "0m 47s" is a worse sentence
  // than "47s" and "5m 0s" is a worse one than "5m".
  it('stays in seconds under a minute and loses an empty remainder', () => {
    expect(spell(47)).toBe('47s')
    expect(spell(59.4)).toBe('59s')
    expect(spell(60)).toBe('1m')
    expect(spell(300)).toBe('5m')
    expect(spell(312)).toBe('5m 12s')
  })

  it('never spells a negative stretch of time', () => {
    expect(spell(-5)).toBe('0s')
  })
})

describe('whatIsLeft', () => {
  // The headline property: the figure is this match's own pace, so it is the
  // median of the games already fought multiplied by the games still to come.
  it('measures the rest of the match off the games already fought', () => {
    const fought = [row({ game: 1, seconds: 30 }), row({ game: 2, seconds: 30 }),
      row({ game: 3, seconds: 30 })]
    expect(whatIsLeft(fought, 10)).toEqual({ games: 7, seconds: 210 })
  })

  // **The median, never the mean**, which is the house's rule for Forge's
  // numbers and here is what stops one wide board where a pilot thought for
  // two minutes from doubling the figure for the quick games around it. The
  // mean of these five is 54s and would have said 162s.
  it('is not dragged by one long board', () => {
    const fought = [row({ seconds: 10 }), row({ seconds: 10 }),
      row({ seconds: 20 }), row({ seconds: 10 }), row({ seconds: 220 })]
    // Five fought of eight: three to come at a median of 10s is 30s. The mean
    // of the same five is 54s and would have promised 162s.
    expect(whatIsLeft(fought, 8)?.seconds).toBe(30)
  })

  // On an even sample it takes the upper of the two middles rather than
  // averaging them: deliberately pessimistic, because a wait that outlives
  // its own estimate reads as a broken page and one that comes in early
  // costs nobody anything.
  it('rounds an even sample the slow way', () => {
    expect(whatIsLeft([row({ seconds: 10 }), row({ seconds: 40 })], 3))
      .toEqual({ games: 1, seconds: 40 })
  })

  // A draw and a clock-out are not wins, but they are minutes somebody sat
  // through — and the question here is time rather than score.
  it('counts a draw and a clock-out toward the pace', () => {
    const fought = [row({ seconds: 100, winner: null, draw: true }),
      row({ seconds: 100, winner: null, timed_out: true })]
    expect(whatIsLeft(fought, 4)?.seconds).toBe(200)
  })

  // Two is the floor and it is a real decision: a first game is the slowest
  // of the match because the forge lights from cold, so an estimate drawn
  // from it alone promises a wait about double the truth. Nothing is better
  // than a figure like that.
  it('says nothing from a sample of one', () => {
    expect(whatIsLeft([row({ seconds: 180 })], 10)).toBeNull()
    expect(whatIsLeft([], 10)).toBeNull()
  })

  it('says nothing once the last game has landed', () => {
    const fought = [row({ game: 1 }), row({ game: 2 })]
    expect(whatIsLeft(fought, 2)).toBeNull()
    // And nothing on a match that somehow reported more games than it was
    // asked for, rather than a negative count of bouts to come.
    expect(whatIsLeft(fought, 1)).toBeNull()
  })
})

describe('theaterRows', () => {
  it('reads the rows out of a real partial', () => {
    expect(theaterRows({ rows: [row(), row({ game: 2 })] })).toHaveLength(2)
  })

  // The three shapes that are all "nothing yet". `null` is what the server
  // sends before the first tick and again once it clears the partial on
  // completion; the bare object is a shim that streams counts and no rows.
  it.each([
    ['null', null],
    ['undefined', undefined],
    ['a number', 7],
    ['a payload with no rows', {}],
    ['a payload whose rows are not a list', { rows: 'soon' }],
  ])('is empty and does not throw for %s', (_label, partial) => {
    expect(theaterRows(partial)).toEqual([])
  })
})

describe('shortName', () => {
  // The dash is a real separator and everything past it goes, always. The
  // comma is the one under test.
  it.each<[string, string[] | undefined, string]>([
    // Named for its general: the epithet is the general's title, so it goes.
    ['Arahbo, Roar of the World — Cats', ['Arahbo, Roar of the World'],
      'Arahbo'],
    ['Atla Palani, Nest Tender — Dinos', ['Atla Palani, Nest Tender'],
      'Atla Palani'],
    ['Goreclaw, Terror of Qal Sisma — Stompy', ['Goreclaw, Terror of Qal Sisma'],
      'Goreclaw'],
    // **The bug.** Aaron's Atla Palani deck is a line from a film and its
    // commas are a sentence's, not a legend's. Cut by punctuation it read
    // "Life" in every Coliseum control; the general's name is nowhere near
    // the front of it, so nothing is cut.
    ['Life, Uh, Finds a Way', ['Atla Palani, Nest Tender'],
      'Life, Uh, Finds a Way'],
    // The same title with a theme on the end: the dash still separates.
    ['Life, Uh, Finds a Way — Dinos', ['Atla Palani, Nest Tender'],
      'Life, Uh, Finds a Way'],
    // Case-insensitive, because a deck's title is typed by a person and a
    // commander's name is copied off a card.
    ['arahbo, roar of the world — Cats', ['Arahbo, Roar of the World'],
      'arahbo'],
    // A partner pair is two chances to be named for your general.
    ['Ravos, Soultender — Aristocrats',
      ['Tymna the Weaver', 'Ravos, Soultender'], 'Ravos'],
    // Nothing to match against: the safe direction is the whole title. This
    // is the shelf not having loaded yet, and a stranger's deck the room only
    // knows Forge's own name for.
    ['Arahbo, Roar of the World — Cats', undefined,
      'Arahbo, Roar of the World'],
    ['Life, Uh, Finds a Way', undefined, 'Life, Uh, Finds a Way'],
    // No comma at all: already short, and the commander is beside the point.
    ['Trostani tokens', undefined, 'Trostani tokens'],
    ['Gyome — Food (Mitch)', ['Gyome, Master Chef'], 'Gyome'],
    // Empty in, empty out — the guard the first cut of this function had and
    // this one keeps.
    ['', ['Atla Palani, Nest Tender'], ''],
    ['— Cats', ['Atla Palani, Nest Tender'], '— Cats'],
  ])('shortens %s under %s to %s', (full, commander, short) => {
    expect(shortName(full, commander)).toBe(short)
  })

  // The commander may arrive as one name rather than a list, which is what a
  // caller holding a single general has.
  it('takes a bare commander name as well as a list', () => {
    expect(shortName('Atla Palani, Nest Tender — Dinos', 'Atla Palani'))
      .toBe('Atla Palani')
    expect(shortName('Life, Uh, Finds a Way', 'Atla Palani, Nest Tender'))
      .toBe('Life, Uh, Finds a Way')
  })

  // A deck whose title *starts* with a word that is not its general keeps
  // every word of it, even when the general is named later on.
  it('does not cut on a comma the general is not in front of', () => {
    expect(shortName('Ready, Atla Palani, Fire', ['Atla Palani, Nest Tender']))
      .toBe('Ready, Atla Palani, Fire')
  })
})

describe('legendName', () => {
  // The unconditional cut, which is right for a card because Wizards printed
  // the comma. `lib/stage.ts`'s wall is the caller; a board naming two legends
  // reads as four names without it.
  it.each([
    ['Brimaz, King of Oreskos', 'Brimaz'],
    ['Arahbo, Roar of the World', 'Arahbo'],
    ['Sacred Cat', 'Sacred Cat'],
    ['Cat Token', 'Cat Token'],
    ['', ''],
  ])('calls %s %s', (full, short) => {
    expect(legendName(full)).toBe(short)
  })
})

describe('beatLine', () => {
  const name = (slug: string) => slug === 'gyome-food' ? 'Gyome' : 'Atla'
  const beat = (over: Partial<ForgeBeat> & { kind: string }): ForgeBeat =>
    ({ who: 'gyome-food', against: null, ...over })

  it('says a companion came from outside the game', () => {
    // **The sentence exists because the room said nothing at all.** Aaron
    // watched Kaheera land in a hand and thought the engine had cheated — a
    // companion waits outside the game and its controller pays {3} to bring it
    // in, and a hand gaining a card it was never dealt, unremarked, is a
    // beginner being shown a game that cheats (commandment 2).
    //
    // "outside the game" rather than "the command zone": both are true, and
    // one of them is a zone name that means nothing until you already know the
    // answer.
    expect(beatLine(beat({ kind: 'companion',
      card: 'Kaheera, the Orphanguard' }), name))
      .toEqual({ who: 'Gyome',
        text: 'calls in Kaheera, the Orphanguard from outside the game' })
  })

  it('gives a sacrifice its own word, and its player', () => {
    // Not `dies`: rule 700.4 gives that word to a creature or planeswalker put
    // into a graveyard from the battlefield, and a Treasure cracked for mana
    // does neither. The player is the subject because a sacrifice is a thing
    // somebody chose.
    expect(beatLine(beat({ kind: 'sacrificed', card: 'Food Token' }), name))
      .toEqual({ who: 'Gyome', text: 'sacrifices Food Token' })
  })

  it('tells an ability somebody used from one the game raised', () => {
    // The wire knows which — `trigger` is the scribe reading Forge's own flag
    // — and they are two different sentences. An ability activated is a thing
    // a player did; one that triggered happened by itself, so the card is the
    // subject exactly as it is for a death, and composing it as "<player>
    // <card> triggers" would put two subjects in one line.
    expect(beatLine(beat({ kind: 'ability', card: 'Skullclamp' }), name))
      .toEqual({ who: 'Gyome', text: 'uses Skullclamp' })
    expect(beatLine(beat({ kind: 'ability', trigger: true,
      card: 'Gyome, Master Chef' }), name))
      .toEqual({ who: null, text: 'Gyome, Master Chef triggers' })
  })

  it('leaves a death and an exile without a player', () => {
    // The property the plate on the centre stage rests on: a creature dying is
    // not something its controller did, so nothing downstream can put their
    // name in front of it.
    expect(beatLine(beat({ kind: 'dies', card: 'Fleecemane Lion' }), name).who)
      .toBeNull()
    expect(beatLine(beat({ kind: 'exiled', card: 'Sol Ring' }), name).who)
      .toBeNull()
  })

  it('renders a kind it has never heard of rather than dropping it', () => {
    // Forge is not an API. A release that adds a beat reads as a plain line
    // here instead of as a gap in the game — and this is a safety net rather
    // than a destination: it prints the kind's own name, which is the wire
    // showing through to a user, and every kind that reaches it is one that
    // still needs a sentence written for it.
    expect(beatLine(beat({ kind: 'phased', card: 'Teferi\'s Veil' }), name))
      .toEqual({ who: 'Gyome', text: 'phased: Teferi\'s Veil' })
    expect(beatLine(beat({ kind: 'phased' }), name).text).toBe('phased')
  })
})
