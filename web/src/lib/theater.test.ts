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
import { beatLine, legendName, shortName, theaterRows } from './theater'

function row(over: Partial<ForgeGameRow> = {}): ForgeGameRow {
  return {
    game: 1, winner: 'arahbo-cats', seconds: 6.2, turns: 9, draw: false,
    timed_out: false, ...over,
  }
}

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
