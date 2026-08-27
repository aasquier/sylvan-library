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
 *   name, which is what fits in a feed row.
 * - **`beatLine` turns one beat into English**, and the two things worth
 *   holding are that a kind it has never heard of is still rendered, and that
 *   `who` is the player the sentence is *about* rather than the seat the wire
 *   happened to carry.
 */

import { describe, expect, it } from 'vitest'
import type { ForgeBeat, ForgeGameRow } from './api'
import { beatLine, shortName, theaterRows } from './theater'

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
  it.each([
    ['Arahbo, Roar of the World — Cats', 'Arahbo'],
    ['Goreclaw, Terror of Qal Sisma — Mono-Green Stompy', 'Goreclaw'],
    ['Gyome, Master Chef — Food (Mitch)', 'Gyome'],
    ['Trostani tokens', 'Trostani tokens'],
  ])('shortens %s to %s', (full, short) => {
    expect(shortName(full)).toBe(short)
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
